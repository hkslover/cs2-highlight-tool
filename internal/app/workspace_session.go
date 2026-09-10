package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/envsetup"
)

var workspaceSessionStopWait = 10 * time.Second

// workspaceSession is the private identity of one selected workspace. Its
// root, generation and envsetup service never change for the lifetime of the
// instance. App keeps legacy fields as mirrors for the Wails-facing code and
// older focused tests, but new work snapshots this object before doing I/O.
type workspaceSession struct {
	root       string
	generation uint64
	service    *envsetup.Service
	store      *config.Store
	ctx        context.Context
	cancel     context.CancelFunc

	taskMu      sync.Mutex
	activeTasks int
	tasksClosed bool
	taskWG      sync.WaitGroup
}

func newWorkspaceSession(root string, generation uint64, service *envsetup.Service) *workspaceSession {
	ctx, cancel := context.WithCancel(context.Background())
	session := &workspaceSession{
		root:       root,
		generation: generation,
		service:    service,
		store:      serviceStore(service),
		ctx:        ctx,
		cancel:     cancel,
	}
	if service != nil {
		service.BindLifecycleContext(ctx)
	}
	return session
}

func serviceStore(service *envsetup.Service) *config.Store {
	if service == nil {
		return nil
	}
	return service.ConfigStore()
}

func (s *workspaceSession) isClosed() bool {
	if s == nil {
		return true
	}
	s.taskMu.Lock()
	closed := s.tasksClosed
	s.taskMu.Unlock()
	return closed
}

// beginTask admits one app-owned operation and returns the immutable
// lifecycle context it belongs to. The registration happens before callers
// launch a goroutine, so close cannot race Wait with Add.
func (s *workspaceSession) beginTask() (context.Context, func(), bool) {
	if s == nil {
		return context.Background(), func() {}, false
	}
	s.taskMu.Lock()
	if s.tasksClosed {
		ctx := s.ctx
		s.taskMu.Unlock()
		if ctx == nil {
			ctx = context.Background()
		}
		return ctx, func() {}, false
	}
	s.activeTasks++
	s.taskWG.Add(1)
	ctx := s.ctx
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

// closeIfIdle atomically closes app admission and the envsetup admission only
// when no task is active. Reset uses this non-canceling path: active user work
// is rejected by the outer App gate and must finish before deletion.
func (s *workspaceSession) closeIfIdle() bool {
	if s == nil {
		return true
	}
	s.taskMu.Lock()
	if s.tasksClosed {
		idle := s.activeTasks == 0
		s.taskMu.Unlock()
		if !idle || s.service == nil {
			return idle
		}
		return s.service.CloseIfIdle()
	}
	if s.activeTasks > 0 {
		s.taskMu.Unlock()
		return false
	}
	if s.service != nil && !s.service.CloseIfIdle() {
		s.taskMu.Unlock()
		return false
	}
	s.tasksClosed = true
	cancel := s.cancel
	s.taskMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return true
}

// close cancels the session and waits for all app-owned work. It is used by
// application shutdown; a timeout leaves the session closed so no new work
// can enter and a later call can retry the wait.
func (s *workspaceSession) close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.taskMu.Lock()
	if !s.tasksClosed {
		s.tasksClosed = true
	}
	cancel := s.cancel
	s.taskMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	stopCtx := ctx
	if _, hasDeadline := stopCtx.Deadline(); !hasDeadline {
		var stopCancel context.CancelFunc
		stopCtx, stopCancel = context.WithTimeout(stopCtx, workspaceSessionStopWait)
		defer stopCancel()
	}
	if s.service != nil {
		if err := s.service.Stop(stopCtx); err != nil {
			return err
		}
	}

	done := make(chan struct{})
	go func() {
		s.taskWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-stopCtx.Done():
		return fmt.Errorf("停止工作目录任务超时: %w", stopCtx.Err())
	}
}

type workspaceSnapshot struct {
	session       *workspaceSession
	service       *envsetup.Service
	store         *config.Store
	root          string
	generation    uint64
	exeDir        string
	pendingReset  bool
	resetComplete bool
}

// workspaceSnapshot reads the short-lived App identity lock only. Callers
// must use the returned root/service for the whole operation and must not
// reacquire serviceMu while holding a file/activity lock.
func (a *App) workspaceSnapshot() workspaceSnapshot {
	if a == nil {
		return workspaceSnapshot{}
	}
	a.serviceMu.Lock()
	snapshot := workspaceSnapshot{
		session:       a.workspace,
		service:       a.service,
		store:         a.configStore,
		root:          a.dataDir,
		generation:    a.workspaceGeneration,
		exeDir:        a.exeDir,
		pendingReset:  a.workspaceResetPendingPath != "" || a.workspaceResetRegistryPending,
		resetComplete: a.workspaceResetCompleted,
	}
	a.serviceMu.Unlock()
	if snapshot.session != nil {
		snapshot.root = snapshot.session.root
		snapshot.service = snapshot.session.service
		snapshot.store = snapshot.session.store
		snapshot.generation = snapshot.session.generation
	}
	if snapshot.store == nil {
		snapshot.store = serviceStore(snapshot.service)
	}
	return snapshot
}

// ensureWorkspaceSessionLocked lazily upgrades manually constructed App
// fixtures (which still set service/dataDir directly) to the same identity and
// task gate used by production construction. The caller must hold serviceMu.
func (a *App) ensureWorkspaceSessionLocked() *workspaceSession {
	if a == nil || a.workspace != nil || a.service == nil || a.dataDir == "" {
		if a == nil {
			return nil
		}
		return a.workspace
	}
	a.workspaceGeneration++
	a.workspace = newWorkspaceSession(a.dataDir, a.workspaceGeneration, a.service)
	a.configStore = a.workspace.store
	return a.workspace
}

func (a *App) ensureWorkspaceSession() *workspaceSession {
	if a == nil {
		return nil
	}
	a.serviceMu.Lock()
	session := a.ensureWorkspaceSessionLocked()
	a.serviceMu.Unlock()
	return session
}

// installWorkspaceLocked installs a new immutable workspace identity. The
// caller must hold serviceMu and must have already passed the lifecycle gate.
func (a *App) installWorkspaceLocked(root string, service *envsetup.Service) *workspaceSession {
	a.workspaceGeneration++
	session := newWorkspaceSession(root, a.workspaceGeneration, service)
	a.workspace = session
	a.dataDir = root
	a.service = service
	a.configStore = session.store
	if a.configStore == nil && root != "" {
		a.configStore = config.NewStore(filepath.Join(root, "config.json"), root)
	}
	a.workspaceResetPendingPath = ""
	a.workspaceResetRegistryPending = false
	a.workspaceResetCompleted = false
	return session
}

// configStoreForWorkspace returns the store fixed to one workspace. Production
// callers use the store injected into Service; manually constructed test/dev
// Apps lazily create one mirror so their read-modify-write operations still
// serialize. A live workspace never permits a different root to replace its
// store.
func (a *App) configStoreForWorkspace(dataDir string, service *envsetup.Service) *config.Store {
	if a == nil || dataDir == "" {
		return nil
	}
	root := filepath.Clean(dataDir)
	if service != nil {
		store := serviceStore(service)
		if store == nil {
			return nil
		}
		if store.DataRoot() != "" && filepath.Clean(store.DataRoot()) != root {
			return nil
		}
		a.serviceMu.Lock()
		a.configStore = store
		a.serviceMu.Unlock()
		return store
	}

	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.workspace != nil {
		if filepath.Clean(a.workspace.root) != root || a.workspace.isClosed() {
			return nil
		}
		if a.workspace.store != nil {
			return a.workspace.store
		}
	}
	if a.configStore != nil {
		if a.configStore.DataRoot() == "" || filepath.Clean(a.configStore.DataRoot()) == root {
			return a.configStore
		}
		return nil
	}
	store := config.NewStore(filepath.Join(root, "config.json"), root)
	a.configStore = store
	return store
}

// clearProduceWorkspaceState drops in-memory take/history indexes that belong
// to the removed workspace. The existing snapshot events make the frontend
// discard the old list immediately after a successful reset.
func (a *App) clearProduceWorkspaceState() {
	if a == nil {
		return
	}
	a.produceStateMu.Lock()
	a.produceState = produceSessionState{}
	a.produceStateMu.Unlock()
	a.emitTakeFilesSnapshot(ProduceTakeFileSnapshot{Items: make([]ProduceTakeFile, 0), UpdatedAtMs: nowMs()})
	a.emitProduceHistorySnapshot(ProduceHistorySnapshot{Items: make([]ProduceHistoryItem, 0), UpdatedAtMs: nowMs()})
}
