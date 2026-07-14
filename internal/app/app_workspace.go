package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"cs2-highlight-tool-v2/internal/appdata"
	"cs2-highlight-tool-v2/internal/envsetup"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// appSubDir 是应用在用户选择的父目录下自动创建的子目录名。
const appSubDir = "cs2HighLightTool"

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
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	ws := WorkspaceState{
		Initialized: a.service != nil,
		DataDir:     a.dataDir,
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
	if err := appdata.ValidateDataDir(path); err != nil {
		return err
	}

	// Windows 写注册表。非 Windows 平台略过（registry_other.go 返回 unsupported），
	// 视为本地开发兜底场景，依然允许设置。
	if runtime.GOOS == "windows" {
		if err := appdata.WriteDataDirToRegistry(path); err != nil {
			return fmt.Errorf("写入注册表失败: %w", err)
		}
	}

	// 后台清理 legacy 数据，失败仅 log，不阻塞主流程。
	exeDir := a.exeDir
	go func() {
		if err := appdata.CleanupLegacyData(exeDir); err != nil {
			if a.ctx != nil {
				wruntime.LogWarning(a.ctx, fmt.Sprintf("cleanup legacy app data failed: %v", err))
			}
		}
	}()

	// 构造 service 并启动
	a.serviceMu.Lock()
	a.dataDir = path
	a.seedFirstInstallChangelog()
	a.service = envsetup.NewWithDataDir(a.exeDir, path, a.version)
	svc := a.service
	a.serviceMu.Unlock()
	a.configureProduceDiagnostics(path)

	if a.ctx != nil {
		svc.Startup(a.ctx)
	}

	// 触发启动检查（异步）
	go func() {
		svc.RunStartupChecks()
	}()
	return nil
}

// ResetWorkspace 删除当前 DataDir + 清注册表 + 重置 service。
// 调用后前端会收到 mode=workspace_init 状态。
func (a *App) ResetWorkspace() error {
	a.serviceMu.Lock()
	dataDir := a.dataDir
	a.serviceMu.Unlock()

	if dataDir == "" {
		// 已经未初始化，幂等
		a.emitWorkspaceInitState()
		return nil
	}

	if err := os.RemoveAll(dataDir); err != nil {
		return fmt.Errorf("删除工作目录失败: %w", err)
	}

	if runtime.GOOS == "windows" {
		if err := appdata.DeleteDataDirFromRegistry(); err != nil {
			return fmt.Errorf("清除注册表失败: %w", err)
		}
	}

	a.serviceMu.Lock()
	a.service = nil
	a.dataDir = ""
	a.serviceMu.Unlock()
	a.configureProduceDiagnostics(a.exeDir)

	a.emitWorkspaceInitState()
	return nil
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
