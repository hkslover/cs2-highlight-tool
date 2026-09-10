package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"cs2-highlight-tool-v2/internal/appdata"
	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/envsetup"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// appSubDir 是应用在用户选择的父目录下自动创建的子目录名。
const appSubDir = "cs2HighLightTool"

// Keep destructive/external operations replaceable in focused tests. The
// production implementations remain the appdata/os functions below.
var (
	writeDataDirToRegistry    = appdata.WriteDataDirToRegistry
	deleteDataDirFromRegistry = appdata.DeleteDataDirFromRegistry
	removeWorkspaceDir        = os.RemoveAll
)

// appendAppSubdir 在 parent 末尾追加 appSubDir。
// 若路径末段已是 appSubDir，直接返回原值（幂等）。
func appendAppSubdir(parent string) string {
	if filepath.Base(parent) == appSubDir {
		return parent
	}
	return filepath.Join(parent, appSubDir)
}

// WorkspaceValidateResult 是 ValidateWorkspaceDir 的返回结构。
// 使用 struct 代替 (bool, string) 双返回值，确保 Wails v2 绑定层
// 正确序列化两个字段（双返回值会生成 Promise<boolean|string> 联合类型导致 string 丢失）。
type WorkspaceValidateResult struct {
	OK           bool   `json:"ok"`
	ErrorMessage string `json:"errorMessage"`
}

// WorkspaceState 描述当前工作目录初始化状态。
// 前端用于决定是否显示 WorkspaceInitModal。
type WorkspaceState struct {
	Initialized bool   `json:"initialized"`
	DataDir     string `json:"data_dir"`
	Error       string `json:"error"`
}

// GetWorkspaceState 返回当前工作目录初始化状态。
func (a *App) GetWorkspaceState() WorkspaceState {
	snapshot := a.workspaceSnapshot()
	ws := WorkspaceState{
		Initialized: snapshot.service != nil && snapshot.root != "",
		DataDir:     snapshot.root,
	}
	return ws
}

// PickWorkspaceDir 打开系统目录选择对话框，让用户选择父目录。
// 返回路径已自动追加 cs2HighLightTool 子目录；用户取消返回 ("", nil)。
func (a *App) PickWorkspaceDir() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("应用尚未启动")
	}
	selected, err := wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:                "选择父目录（程序将自动创建 cs2HighLightTool 子目录）",
		CanCreateDirectories: true,
	})
	if err != nil {
		return "", fmt.Errorf("打开目录对话框失败: %w", err)
	}
	if selected == "" {
		return "", nil
	}
	return appendAppSubdir(selected), nil
}

// ValidateWorkspaceDir 实时校验用户选择的目录，返回 WorkspaceValidateResult。
func (a *App) ValidateWorkspaceDir(path string) WorkspaceValidateResult {
	if err := appdata.ValidateDataDir(path); err != nil {
		return WorkspaceValidateResult{OK: false, ErrorMessage: err.Error()}
	}
	return WorkspaceValidateResult{OK: true}
}

// SetWorkspaceDir 接受用户最终选择，写注册表，
// 清理 legacy 数据，构造 service 并触发 RunStartupChecks。
func (a *App) SetWorkspaceDir(path string) error {
	releaseTransition, err := a.beginWorkspaceTransition()
	if err != nil {
		return err
	}
	defer releaseTransition()

	snapshot := a.workspaceSnapshot()
	pendingReset := snapshot.pendingReset
	initialized := snapshot.service != nil || snapshot.root != ""
	if pendingReset {
		return fmt.Errorf("工作目录重置尚未完成，请先重试重置")
	}
	if initialized {
		return fmt.Errorf("工作目录已初始化，不能在运行中切换；请先重置工作目录")
	}

	if err := appdata.ValidateDataDir(path); err != nil {
		return err
	}

	// Windows 写注册表。非 Windows 平台略过（registry_other.go 返回 unsupported），
	// 视为本地开发兜底场景，依然允许设置。
	if runtime.GOOS == "windows" {
		if err := writeDataDirToRegistry(path); err != nil {
			return fmt.Errorf("写入注册表失败: %w", err)
		}
	}

	// 构造 service 并启动。先提交不可变 workspace session，再登记所有
	// 即将启动的后台任务；整个过程仍在生命周期排他资格内。
	store := config.NewStore(filepath.Join(path, "config.json"), path)
	svc := envsetup.NewWithDataDirAndStore(a.exeDir, path, a.version, store)
	a.serviceMu.Lock()
	session := a.installWorkspaceLocked(path, svc)
	a.serviceMu.Unlock()
	a.seedFirstInstallChangelogAt(path, a.version)
	a.configureProduceDiagnostics(path)

	// 后台清理 legacy 数据，失败仅 log，不阻塞主流程。
	exeDir := a.exeDir
	legacyRelease := a.reserveManagedWorkspaceTaskForSession(session)
	go func() {
		defer legacyRelease()
		if err := appdata.CleanupLegacyData(exeDir); err != nil && !session.isClosed() {
			if a.ctx != nil {
				wruntime.LogWarning(a.ctx, fmt.Sprintf("cleanup legacy app data failed: %v", err))
			}
		}
	}()

	if a.ctx != nil {
		svc.Startup(a.ctx)
	}

	// 触发启动检查（异步）
	startupRelease := a.reserveManagedWorkspaceTaskForSession(session)
	go func() {
		defer startupRelease()
		svc.RunStartupChecks()
	}()
	return nil
}

// ResetWorkspace 删除当前 DataDir + 清注册表 + 重置 service。
// 调用后前端会收到 mode=workspace_init 状态。
func (a *App) ResetWorkspace() error {
	releaseTransition, err := a.beginWorkspaceTransition()
	if err != nil {
		return err
	}
	defer releaseTransition()

	a.serviceMu.Lock()
	configuredDataDir := a.dataDir
	pendingDataDir := a.workspaceResetPendingPath
	service := a.service
	session := a.workspace
	registryPending := a.workspaceResetRegistryPending
	a.serviceMu.Unlock()
	if session != nil {
		configuredDataDir = session.root
		service = session.service
	}
	if configuredDataDir == "" && pendingDataDir == "" && service == nil && !registryPending {
		// Already uninitialized: keep reset idempotent without repeating
		// registry/diagnostics side effects.
		a.emitWorkspaceInitState()
		return nil
	}
	dataDir := configuredDataDir
	if dataDir == "" {
		dataDir = pendingDataDir
	}

	if service != nil && service.HasActiveTasks() {
		return fmt.Errorf("启动检查或后台组件任务仍在运行，请完成后再试")
	}
	if session != nil && !session.closeIfIdle() {
		return fmt.Errorf("启动检查或后台组件任务仍在运行，请完成后再试")
	}
	if session == nil && service != nil && !service.CloseIfIdle() {
		return fmt.Errorf("启动检查或后台组件任务仍在运行，请完成后再试")
	}

	if dataDir != "" {
		if err := removeWorkspaceDir(dataDir); err != nil {
			a.detachWorkspaceForReset(dataDir)
			a.emitWorkspaceInitState()
			return fmt.Errorf("删除工作目录失败: %w", err)
		}
	}

	if runtime.GOOS == "windows" {
		if err := deleteDataDirFromRegistry(); err != nil {
			a.detachWorkspaceForReset(dataDir)
			a.emitWorkspaceInitState()
			return fmt.Errorf("清除注册表失败: %w", err)
		}
	}

	a.serviceMu.Lock()
	a.service = nil
	a.dataDir = ""
	a.configStore = nil
	a.workspace = nil
	a.workspaceGeneration++
	a.platformImports = nil
	a.workspaceResetPendingPath = ""
	a.workspaceResetRegistryPending = false
	a.workspaceResetCompleted = true
	a.serviceMu.Unlock()
	a.configureProduceDiagnostics(a.exeDir)
	a.clearProduceWorkspaceState()

	a.emitWorkspaceInitState()
	return nil
}

// detachWorkspaceForReset makes a failed/partial reset non-operational. The
// old service is deliberately dropped before returning so it cannot recreate
// files in the removed directory; the saved path and registry flag let a
// later ResetWorkspace retry the remaining cleanup.
func (a *App) detachWorkspaceForReset(dataDir string) {
	a.serviceMu.Lock()
	a.service = nil
	a.dataDir = ""
	a.configStore = nil
	a.workspace = nil
	a.workspaceGeneration++
	a.platformImports = nil
	a.workspaceResetPendingPath = dataDir
	a.workspaceResetRegistryPending = runtime.GOOS == "windows"
	a.serviceMu.Unlock()
	a.configureProduceDiagnostics(a.exeDir)
}

// ExitApp 退出应用，供初始化 modal 的"退出"按钮调用。
func (a *App) ExitApp() {
	if a.ctx != nil {
		wruntime.Quit(a.ctx)
	}
}

// emitWorkspaceInitState 向前端发出 mode=workspace_init 的最小 StartupState。
func (a *App) emitWorkspaceInitState() {
	if a.ctx == nil {
		return
	}
	state := envsetup.StartupState{
		Mode: envsetup.ModeWorkspaceInit,
	}
	wruntime.EventsEmit(a.ctx, "startup_state_changed", state)
}
