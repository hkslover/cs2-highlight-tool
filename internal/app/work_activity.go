package app

import "fmt"

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

// Reserve file use without holding a state mutex over I/O. Imports, parsing,
// editing and exports may coexist; directory deletion must be exclusive.
func (a *App) beginManagedFileUse() (func(), error) {
	a.managedFilesMu.Lock()
	defer a.managedFilesMu.Unlock()
	if a.managedFilesClearing {
		return nil, fmt.Errorf("正在清理目录，请完成后再试")
	}
	a.managedFileUsers++
	return func() {
		a.managedFilesMu.Lock()
		a.managedFileUsers--
		a.managedFilesMu.Unlock()
	}, nil
}

func (a *App) beginManagedDirectoryClear() (func(), error) {
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
	a.managedFilesMu.Lock()
	if a.managedFileUsers > 0 || a.managedFilesClearing {
		a.managedFilesMu.Unlock()
		a.produceLaunchMu.Unlock()
		return nil, fmt.Errorf("导入、解析、合成或导出任务正在使用文件，暂时不能清理目录")
	}
	a.managedFilesClearing = true
	a.managedFilesMu.Unlock()
	return func() {
		a.managedFilesMu.Lock()
		a.managedFilesClearing = false
		a.managedFilesMu.Unlock()
		a.produceLaunchMu.Unlock()
	}, nil
}
