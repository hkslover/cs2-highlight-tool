package envsetup

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/logging"
	"cs2-highlight-tool-v2/internal/release"
)

type Service struct {
	ctx        context.Context
	exeDir     string
	dataDir    string
	configPath string
	config     *config.Config
	version    string

	state    StartupState
	mu       sync.Mutex
	configMu sync.Mutex

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
	taskMu      sync.Mutex
	activeTasks int
}

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
	cfg := config.Default(dataDir)
	s := &Service{
		exeDir:     exeDir,
		dataDir:    dataDir,
		configPath: filepath.Join(dataDir, "config.json"),
		config:     cfg,
		version:    version,
		state:      newStartupState(cfg, version),
		cancelMap:  make(map[string]*activeDownloadCancel),
	}
	s.logger = logging.NewSlogAdapter(logging.Options{
		Sink: s.appendLogEntry,
	})
	s.runTasksFn = s.runTasksDefault
	return s
}

func (s *Service) beginTask() func() {
	if s == nil {
		return func() {}
	}
	s.taskMu.Lock()
	s.activeTasks++
	s.taskMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.taskMu.Lock()
			if s.activeTasks > 0 {
				s.activeTasks--
			}
			s.taskMu.Unlock()
		})
	}
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

func (s *Service) Startup(ctx context.Context) {
	s.ctx = ctx
	if s.exeDir == "" {
		return
	}
	cfg, err := config.LoadOrCreate(s.configPath, s.dataDir)
	if err != nil {
		cfg = config.Default(s.dataDir)
		s.emitLog("error", fmt.Sprintf("加载配置失败: %v", err))
	}
	s.mu.Lock()
	s.config = cfg
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
