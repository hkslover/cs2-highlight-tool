package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"cs2-highlight-tool-v2/internal/appdata"
	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/envsetup"
	"cs2-highlight-tool-v2/internal/producews"
	"cs2-highlight-tool-v2/internal/release"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx      context.Context
	exeDir   string
	dataDir  string
	version  string
	service  *envsetup.Service
	produceW *producews.Service

	serviceMu sync.Mutex
	// configMu serializes App-level read-modify-write operations on config.json.
	// It intentionally does not cover external work; callers load or persist
	// through the helpers below and release the lock before doing other I/O.
	configMu sync.Mutex

	produceStateMu sync.Mutex
	produceState   produceSessionState

	debugPluginDLLMu       sync.Mutex
	debugPluginDLLOverride string
}

func New(wailsConfigData []byte) *App {
	paths := appdata.ResolveExeOnly(resolveExecutableDir())
	version := release.CurrentAppVersion(wailsConfigData)
	a := &App{
		exeDir:   paths.ExeDir,
		version:  version,
		produceW: producews.NewDefault(nil),
	}
	a.initWorkspaceLocked()
	return a
}

// initWorkspaceLocked 解析 dataDir 来源：
// 1) Windows: 读注册表 → 校验目录存在且通过 ValidateDataDir → 构造 service。
// 2) 非 Windows: 兜底 UserConfigDir + AppDataDirName，service 始终可用（保 wails dev）。
// 3) 失败/未初始化 → dataDir/service 留空，App 进入 workspace_init 模式。
//
// 注意：DataDir 校验失败或目录被外部删除会清理注册表 value，
// 让用户被引导回初始化流程（PRD 提及的 corner case）。
func (a *App) initWorkspaceLocked() {
	if runtime.GOOS == "windows" {
		stored, err := appdata.ReadDataDirFromRegistry()
		if err != nil {
			return
		}
		stored = sanitizePathString(stored)
		if stored == "" {
			return
		}
		// 校验：必须存在且通过 ValidateDataDir（注意：ValidateDataDir 要求"目录已存在则为空"，
		// 已使用过的 DataDir 不为空，所以这里改用更宽松的"存在 + 字符白名单"检查）。
		if !isUsableDataDir(stored) {
			// 注册表中残留的值无效，清理并回到 workspace_init
			_ = appdata.DeleteDataDirFromRegistry()
			return
		}
		a.dataDir = stored
		a.seedFirstInstallChangelog()
		a.service = envsetup.NewWithDataDir(a.exeDir, stored, a.version)
		return
	}

	// 非 Windows 兜底：UserConfigDir
	fallback := fallbackDataDirForDev(a.exeDir)
	if fallback != "" {
		_ = os.MkdirAll(fallback, 0o755)
		a.dataDir = fallback
		a.seedFirstInstallChangelog()
		a.service = envsetup.NewWithDataDir(a.exeDir, fallback, a.version)
	}
}

// seedFirstInstallChangelog 在首装时把 LastChangelogVersion 预设为当前版本，
// 避免新用户首次进入主界面被弹出"更新日志"。仅在 config.json 不存在时生效。
// 必须在 dataDir 已设置、任何 LoadOrCreate 之前调用。失败仅吞噬：下一次
// LoadOrCreate 会以同等原因再次失败并自然把错误带回前端。
func (a *App) seedFirstInstallChangelog() {
	if a.dataDir == "" || a.version == "" {
		return
	}
	_, _ = config.EnsureFirstInstallChangelogSeed(a.configPath(), a.dataDir, a.version)
}

// isUsableDataDir 用于"已初始化"分支：目录存在 + 字符白名单 + 非磁盘根 + 长度合规。
// 不要求目录为空（因为已使用过）。
func isUsableDataDir(path string) bool {
	if path == "" {
		return false
	}
	if appdata.IsDiskRoot(path) {
		return false
	}
	for _, r := range path {
		if r > 127 {
			return false
		}
	}
	if len(path) > appdata.MaxDataDirLength {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	return true
}

// fallbackDataDirForDev 非 Windows 平台的兜底 DataDir：
// 用 os.UserConfigDir() + AppDataDirName。
func fallbackDataDirForDev(exeDir string) string {
	if cfg, err := os.UserConfigDir(); err == nil && cfg != "" {
		return filepath.Join(cfg, appdata.AppDataDirName)
	}
	return exeDir
}

func sanitizePathString(s string) string {
	out := []rune{}
	for _, r := range s {
		if r == 0 {
			continue
		}
		out = append(out, r)
	}
	return filepath.Clean(string(out))
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.produceW.SetEmitter(func(name string, payload any) {
		wruntime.EventsEmit(ctx, name, payload)
	})
	if err := a.produceW.Start(); err != nil {
		wruntime.LogError(ctx, fmt.Sprintf("start produce websocket server failed: %v", err))
	}

	a.serviceMu.Lock()
	svc := a.service
	a.serviceMu.Unlock()

	if svc != nil {
		svc.Startup(ctx)
		return
	}

	// service 为空：未初始化工作目录，发出 workspace_init mode 状态。
	a.emitWorkspaceInitState()
}

func (a *App) Shutdown(ctx context.Context) {
	a.stopProduceSessionWorker()
	if err := a.forceRestoreProduceEnvironmentForProduce(); err != nil {
		wruntime.LogError(ctx, fmt.Sprintf("restore produce environment failed: %v", err))
	}
	if err := a.produceW.Stop(); err != nil {
		wruntime.LogError(ctx, fmt.Sprintf("stop produce websocket server failed: %v", err))
	}
}

func resolveExecutableDir() string {
	exePath, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	return filepath.Dir(exePath)
}

func (a *App) dataRoot() string {
	if a != nil && a.dataDir != "" {
		return a.dataDir
	}
	if a != nil {
		return a.exeDir
	}
	return ""
}

func (a *App) dataPath(elem ...string) string {
	parts := append([]string{a.dataRoot()}, elem...)
	return filepath.Join(parts...)
}

func (a *App) configPath() string {
	return a.dataPath("config.json")
}

func (a *App) loadConfig() (*config.Config, error) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	return config.LoadOrCreate(a.configPath(), a.dataRoot())
}

func (a *App) updateConfig(mutate func(*config.Config) error) (*config.Config, error) {
	a.configMu.Lock()
	defer a.configMu.Unlock()

	path := a.configPath()
	cfg, err := config.LoadOrCreate(path, a.dataRoot())
	if err != nil {
		return nil, err
	}
	if mutate != nil {
		if err := mutate(cfg); err != nil {
			return nil, err
		}
	}
	if err := config.Save(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
