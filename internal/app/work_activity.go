package app

import (
	"fmt"
	"runtime"
	"sync"
)

// WorkActivity covers the whole produce lifecycle, not just the plugin queue.
type WorkActivity struct {
	ProduceBusy bool `json:"produce_busy"`
	StorageBusy bool `json:"storage_busy"`
}

func (a *App) GetWorkActivity() WorkActivity {
	// Do not wait behind a launch or directory deletion just to render buttons.
	if !a.produceLaunchMu.TryLock() {
		return WorkActivity{ProduceBusy: true, StorageBusy: true}
	}
	defer a.produceLaunchMu.Unlock()
	busy := a.produceBusyError() != nil
	a.produceStateMu.Lock()
	runtime := a.produceState.runtime
	a.produceStateMu.Unlock()
	// teardownErr is only read after done closes. A failed teardown retains
	// ownership of files, but a new launch may retry that completed teardown.
	retained := runtime != nil && (!produceRuntimeDone(runtime) || runtime.teardownErr != nil)
	a.managedFilesMu.Lock()
	storageBusy := a.managedFileUsers > 0 || a.managedFilesClearing
	a.managedFilesMu.Unlock()
	return WorkActivity{ProduceBusy: busy, StorageBusy: busy || retained || storageBusy}
}

// Caller holds produceLaunchMu. Never cancel an active runtime to make room
// for another launch: queue completion starts composition, not session exit.
func (a *App) produceBusyError() error {
	if a.produceW != nil && a.produceW.GetQueueState().Running {
		return fmt.Errorf("当前已有制作队列在运行中")
	}
	a.produceStateMu.Lock()
	runtime := a.produceState.runtime
	a.produceStateMu.Unlock()
	if !produceRuntimeDone(runtime) {
		return fmt.Errorf("当前制作会话仍在合成或收尾，请完成后再试")
	}
	return nil
}

// reserveManagedResourceLocked records one operation that may touch the
// managed workspace. The caller must hold managedFilesMu. A once guard keeps
// a repeated defer from corrupting the shared count.
func (a *App) reserveManagedResourceLocked() func() {
	a.managedFileUsers++
	var once sync.Once
	return func() {
		once.Do(func() {
			a.managedFilesMu.Lock()
			if a.managedFileUsers > 0 {
				a.managedFileUsers--
			}
			a.managedFilesMu.Unlock()
		})
	}
}

// reserveManagedWorkspaceTask is used by SetWorkspaceDir while it owns the
// transition reservation. It intentionally bypasses managedFilesClearing:
// the transition owner must register the goroutine it is about to launch
// before releasing the exclusive reservation.
func (a *App) reserveManagedWorkspaceTask() func() {
	a.managedFilesMu.Lock()
	release := a.reserveManagedResourceLocked()
	a.managedFilesMu.Unlock()
	return release
}

// managedWorkspaceRoot keeps the explicit non-Windows development fallback
// used by headless tests and wails dev. Windows requires a selected dataDir;
// exeDir is never treated as a user workspace there.
func (a *App) managedWorkspaceRoot() string {
	if a == nil {
		return ""
	}
	a.serviceMu.Lock()
	dataDir := a.dataDir
	exeDir := a.exeDir
	pending := a.workspaceResetPendingPath != "" || a.workspaceResetRegistryPending
	resetCompleted := a.workspaceResetCompleted
	a.serviceMu.Unlock()
	if dataDir != "" {
		return dataDir
	}
	if pending || resetCompleted {
		return ""
	}
	if runtime.GOOS != "windows" {
		return exeDir
	}
	return ""
}

// beginManagedWorkspaceUse reserves a stable workspace root for the duration
// of one operation. The reservation prevents ResetWorkspace from changing
// dataDir while the caller is doing I/O.
func (a *App) beginManagedWorkspaceUse() (func(), string, error) {
	if a == nil {
		return nil, "", workspaceNotInitializedErr()
	}
	a.managedFilesMu.Lock()
	defer a.managedFilesMu.Unlock()
	if a.managedFilesClearing {
		return nil, "", fmt.Errorf("正在清理目录，请完成后再试")
	}
	dataDir := a.managedWorkspaceRoot()
	if dataDir == "" {
		return nil, "", workspaceNotInitializedErr()
	}
	return a.reserveManagedResourceLocked(), dataDir, nil
}

// Reserve file use without holding a state mutex over I/O. Imports, parsing,
// editing and exports may coexist; directory deletion must be exclusive.
func (a *App) beginManagedFileUse() (func(), error) {
	release, _, err := a.beginManagedWorkspaceUse()
	return release, err
}

// beginManagedExternalUse protects an operation that does not need the
// managed workspace root (for example copying a history video to a user
// selected directory) but still must not overlap a reset.
func (a *App) beginManagedExternalUse() (func(), error) {
	if a == nil {
		return nil, workspaceNotInitializedErr()
	}
	a.managedFilesMu.Lock()
	defer a.managedFilesMu.Unlock()
	if a.managedFilesClearing {
		return nil, fmt.Errorf("正在清理目录，请完成后再试")
	}
	return a.reserveManagedResourceLocked(), nil
}

func (a *App) beginManagedDirectoryClear() (func(), error) {
	return a.beginManagedExclusion(true)
}

// beginWorkspaceTransition is the single admission gate for SetWorkspaceDir
// and ResetWorkspace. It excludes file users and the produce launch pipeline,
// and it remains held until the caller finishes all disk, registry and App
// state changes.
func (a *App) beginWorkspaceTransition() (func(), error) {
	return a.beginManagedExclusion(false)
}

func (a *App) beginManagedExclusion(requireWorkspace bool) (func(), error) {
	if !a.produceLaunchMu.TryLock() {
		return nil, fmt.Errorf("正在启动制作或清理目录，请完成后再试")
	}
	if err := a.produceBusyError(); err != nil {
		a.produceLaunchMu.Unlock()
		return nil, err
	}
	a.produceStateMu.Lock()
	runtime := a.produceState.runtime
	a.produceStateMu.Unlock()
	if runtime != nil && runtime.teardownErr != nil {
		a.produceLaunchMu.Unlock()
		return nil, fmt.Errorf("制作会话收尾尚未成功，请先重试制作以恢复环境")
	}
	if requireWorkspace {
		dataDir := a.managedWorkspaceRoot()
		if dataDir == "" {
			a.produceLaunchMu.Unlock()
			return nil, workspaceNotInitializedErr()
		}
	}
	a.managedFilesMu.Lock()
	if a.managedFileUsers > 0 || a.managedFilesClearing {
		a.managedFilesMu.Unlock()
		a.produceLaunchMu.Unlock()
		return nil, fmt.Errorf("导入、解析、合成或导出任务正在使用文件，暂时不能清理目录")
	}
	a.managedFilesClearing = true
	a.managedFilesMu.Unlock()

	// App-owned startup calls are counted in managedFileUsers. This check also
	// covers a Service task started directly by a test or another internal
	// caller, so a reset cannot delete its data directory underneath it.
	a.serviceMu.Lock()
	svc := a.service
	a.serviceMu.Unlock()
	if svc != nil && svc.HasActiveTasks() {
		a.managedFilesMu.Lock()
		a.managedFilesClearing = false
		a.managedFilesMu.Unlock()
		a.produceLaunchMu.Unlock()
		return nil, fmt.Errorf("启动检查或后台组件任务仍在运行，请完成后再试")
	}

	return func() {
		a.managedFilesMu.Lock()
		a.managedFilesClearing = false
		a.managedFilesMu.Unlock()
		a.produceLaunchMu.Unlock()
	}, nil
}
