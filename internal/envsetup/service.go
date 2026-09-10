package envsetup

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/logging"
	"cs2-highlight-tool-v2/internal/release"
)

type Service struct {
	ctx        context.Context
	exeDir     string
	dataDir    string
	configPath string
	store      *config.Store
	config     *config.Config
	version    string

	state          StartupState
	mu             sync.Mutex
	configRevision uint64
	storeMu        sync.Mutex

	runTasksFn      func(source DownloadSource)
	logger          logging.Logger
	logs            []LogMessage
	releaseSnapshot *release.UnifiedLatest

	cancelMap map[string]*activeDownloadCancel
	cancelMu  sync.Mutex

	ffmpegDetectMu      sync.Mutex
	ffmpegDetectRunning bool
	ffmpegDetectCancel  context.CancelFunc
	ffmpegDetectWG      sync.WaitGroup

	// activeTasks covers synchronous startup actions and child work that may
	// outlive their parent call (currently the FFmpeg capability probe). App
	// uses it as a final guard before deleting the workspace.
	taskMu          sync.Mutex
	activeTasks     int
	taskWG          sync.WaitGroup
	tasksClosed     bool
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
}

// serviceStopWait bounds a stop request that did not receive a context with a
// deadline. A closed service remains closed after this timeout; callers can
// retry Stop later without reopening admission for the old workspace.
var serviceStopWait = 10 * time.Second

var errServiceStopped = errors.New("工作目录服务已停止")

type activeDownloadCancel struct {
	cancel context.CancelFunc
	ctx    context.Context
}

func New(exeDir string, version string) *Service {
	return NewWithDataDir(exeDir, exeDir, version)
}

func NewWithDataDir(exeDir string, dataDir string, version string) *Service {
	if dataDir == "" {
		dataDir = exeDir
	}
	store := config.NewStore(filepath.Join(dataDir, "config.json"), dataDir)
	return NewWithDataDirAndStore(exeDir, dataDir, version, store)
}

// NewWithDataDirAndStore constructs a service with the workspace's shared
// configuration store. NewWithDataDir remains the compatibility constructor
// for focused tests and development callers; production workspace creation
// constructs the Store once and injects it here.
func NewWithDataDirAndStore(exeDir string, dataDir string, version string, store *config.Store) *Service {
	if dataDir == "" {
		dataDir = exeDir
	}
	if store == nil {
		store = config.NewStore(filepath.Join(dataDir, "config.json"), dataDir)
	}
	cfg := config.Default(dataDir)
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	s := &Service{
		exeDir:          exeDir,
		dataDir:         dataDir,
		configPath:      store.Path(),
		store:           store,
		config:          cfg,
		version:         version,
		state:           newStartupState(cfg, version),
		cancelMap:       make(map[string]*activeDownloadCancel),
		lifecycleCtx:    lifecycleCtx,
		lifecycleCancel: lifecycleCancel,
	}
	s.logger = logging.NewSlogAdapter(logging.Options{
		Sink: s.appendLogEntry,
	})
	s.runTasksFn = s.runTasksDefault
	return s
}

// ConfigStore exposes the immutable store identity to the owning App
// workspace. It is not a Wails method; callers must use Store's Snapshot or
// Update methods rather than retaining mutable Config pointers.
func (s *Service) ConfigStore() *config.Store {
	if s == nil {
		return nil
	}
	s.storeMu.Lock()
	store := s.store
	s.storeMu.Unlock()
	return store
}

// BindLifecycleContext attaches a workspace-owned parent context to this
// service. It is intentionally called by the app's private workspace session,
// not exposed as a Wails method. A service with active work or a closed
// admission gate cannot be rebound, which keeps an old instance from being
// silently reused by a new workspace.
func (s *Service) BindLifecycleContext(parent context.Context) {
	if s == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	s.taskMu.Lock()
	if s.tasksClosed || s.activeTasks > 0 {
		s.taskMu.Unlock()
		return
	}
	oldCancel := s.lifecycleCancel
	lifecycleCtx, lifecycleCancel := context.WithCancel(parent)
	s.lifecycleCtx = lifecycleCtx
	s.lifecycleCancel = lifecycleCancel
	s.taskMu.Unlock()
	if oldCancel != nil {
		oldCancel()
	}
}

func (s *Service) lifecycleContext() context.Context {
	if s == nil {
		return context.Background()
	}
	s.taskMu.Lock()
	ctx := s.lifecycleCtx
	s.taskMu.Unlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (s *Service) isStopped() bool {
	if s == nil {
		return true
	}
	s.taskMu.Lock()
	closed := s.tasksClosed
	s.taskMu.Unlock()
	return closed
}

func (s *Service) beginTaskIfOpen() (context.Context, func(), bool) {
	if s == nil {
		return context.Background(), func() {}, false
	}
	s.taskMu.Lock()
	if s.tasksClosed {
		ctx := s.lifecycleCtx
		s.taskMu.Unlock()
		if ctx == nil {
			ctx = context.Background()
		}
		return ctx, func() {}, false
	}
	s.activeTasks++
	s.taskWG.Add(1)
	ctx := s.lifecycleCtx
	s.taskMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			s.taskMu.Lock()
			if s.activeTasks > 0 {
				s.activeTasks--
			}
			s.taskMu.Unlock()
			s.taskWG.Done()
		})
	}, true
}

func (s *Service) beginTask() func() {
	_, release, _ := s.beginTaskIfOpen()
	return release
}

// HasActiveTasks reports startup work that can still read or write the
// service's data directory. It is intentionally a snapshot; callers that
// need exclusion must hold the App workspace admission gate as well.
func (s *Service) HasActiveTasks() bool {
	if s == nil {
		return false
	}
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	return s.activeTasks > 0
}

// CloseIfIdle atomically stops task admission only when no task is active.
// Reset uses this non-canceling boundary so an active user operation is
// rejected rather than forcefully interrupted before deletion.
func (s *Service) CloseIfIdle() bool {
	if s == nil {
		return true
	}
	s.taskMu.Lock()
	if s.tasksClosed || s.activeTasks > 0 {
		closed := s.tasksClosed && s.activeTasks == 0
		s.taskMu.Unlock()
		return closed
	}
	s.tasksClosed = true
	cancel := s.lifecycleCancel
	s.taskMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if store := s.ConfigStore(); store != nil {
		store.Close()
	}
	return true
}

// Stop closes task admission, cancels the workspace lifecycle context and
// waits for every task registered before the close. It is variadic so focused
// tests can call Stop() while production callers can pass a bounded context.
// A timeout never reopens the service; the closed instance remains available
// for a later retry of Stop.
func (s *Service) Stop(contexts ...context.Context) error {
	if s == nil {
		return nil
	}
	ctx := context.Background()
	if len(contexts) > 0 && contexts[0] != nil {
		ctx = contexts[0]
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, serviceStopWait)
		defer cancel()
	}

	s.taskMu.Lock()
	s.tasksClosed = true
	cancelLifecycle := s.lifecycleCancel
	s.taskMu.Unlock()
	// Close the config boundary as soon as admission closes. Tasks already in
	// flight may finish their cancellation path, but a timed-out Stop must not
	// leave a closed workspace able to publish another config write.
	if store := s.ConfigStore(); store != nil {
		store.Close()
	}
	if cancelLifecycle != nil {
		cancelLifecycle()
	}

	done := make(chan struct{})
	go func() {
		s.taskWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("停止工作目录服务超时: %w", ctx.Err())
	}
}

func (s *Service) Startup(ctx context.Context) {
	if s == nil {
		return
	}
	_, releaseTask, ok := s.beginTaskIfOpen()
	if !ok {
		return
	}
	defer releaseTask()
	s.ctx = ctx
	if s.exeDir == "" {
		return
	}
	store := s.ConfigStore()
	var cfg *config.Config
	var revision uint64
	var err error
	if store != nil {
		cfg, revision, err = store.SnapshotWithRevision()
	} else {
		err = fmt.Errorf("配置存储未初始化")
	}
	if err != nil {
		cfg = config.Default(s.dataDir)
		s.emitLog("error", fmt.Sprintf("加载配置失败: %v", err))
	}
	if s.isStopped() {
		return
	}
	s.mu.Lock()
	if revision != 0 && revision < s.configRevision && s.config != nil {
		// An App transaction may have committed and published a newer snapshot
		// while Startup was reading the file. Keep the newer in-memory fact
		// source instead of restoring the older read result.
		cfg = config.Clone(s.config)
		revision = s.configRevision
	}
	cfg = config.Clone(cfg)
	s.config = cfg
	s.configRevision = revision
	s.state = newStartupState(cfg, s.version)
	s.logs = nil
	s.releaseSnapshot = nil
	s.mu.Unlock()
	s.emitState()
}

func (s *Service) GetStartupState() StartupState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.clone()
}
