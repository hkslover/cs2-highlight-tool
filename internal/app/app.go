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
	// workspace is the immutable identity used by new operations. service and
	// dataDir/configStore remain mirrors for compatibility with existing App
	// consumers and are always replaced together with the workspace identity.
	workspace           *workspaceSession
	workspaceGeneration uint64
	configStore         *config.Store
	// platformImports coordinates same-key platform demo imports for the
	// installed workspace identity. It is replaced together with that identity
	// (guarded by serviceMu) so tasks from a removed workspace can never be
	// joined by a new one.
	platformImports *platformImportCoordinator

	produceStateMu sync.Mutex
	produceState   produceSessionState

	// produceLaunchMu serializes the produce launch pipeline (take-file reset,
	// queue check, old-session stop, environment preparation, runtime install)
	// so two concurrent GeneratePluginJSONBatch / AndLaunchHLAE requests can
	// never interleave and corrupt each other's environment or take state.
	produceLaunchMu sync.Mutex

	managedFilesMu sync.Mutex
	// managedFileUsers counts every operation that may touch the managed
	// workspace, including startup tasks registered by the App. The name is
	// retained for compatibility with the existing file-use tests.
	managedFileUsers     int
	managedFilesClearing bool

	// A failed reset detaches the old service so it cannot write into a
	// partially removed directory. Keep enough state to make ResetWorkspace
	// retryable without allowing a new workspace to be installed in between.
	workspaceResetPendingPath     string
	workspaceResetRegistryPending bool
	workspaceResetCompleted       bool

	// produceEnvEpoch is the monotonically increasing produce-environment
	// generation counter (see beginProduceEnvironmentPrep). Guarded by
	// produceStateMu.
	produceEnvEpoch uint64

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
	a.configureProduceDiagnostics(a.dataRoot())
	return a
}

func (a *App) configureProduceDiagnostics(dataDir string) {
	if a == nil || a.produceW == nil {
		return
	}
	if dataDir == "" {
		dataDir = a.exeDir
	}
	a.produceW.SetDiagnostics(producews.NewDiagnostics(dataDir))
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
		store := config.NewStore(filepath.Join(stored, "config.json"), stored)
		svc := envsetup.NewWithDataDirAndStore(a.exeDir, stored, a.version, store)
		a.serviceMu.Lock()
		a.installWorkspaceLocked(stored, svc)
		a.serviceMu.Unlock()
		a.seedFirstInstallChangelogAt(stored, a.version)
		return
	}

	// 非 Windows 兜底：UserConfigDir
	fallback := fallbackDataDirForDev(a.exeDir)
	if fallback != "" {
		_ = os.MkdirAll(fallback, 0o755)
		store := config.NewStore(filepath.Join(fallback, "config.json"), fallback)
		svc := envsetup.NewWithDataDirAndStore(a.exeDir, fallback, a.version, store)
		a.serviceMu.Lock()
		a.installWorkspaceLocked(fallback, svc)
		a.serviceMu.Unlock()
		a.seedFirstInstallChangelogAt(fallback, a.version)
	}
}

// seedFirstInstallChangelog 在首装时把 LastChangelogVersion 预设为当前版本，
// 避免新用户首次进入主界面被弹出"更新日志"。仅在 config.json 不存在时生效。
// 必须在 dataDir 已设置、任何 LoadOrCreate 之前调用。失败仅吞噬：下一次
// LoadOrCreate 会以同等原因再次失败并自然把错误带回前端。
func (a *App) seedFirstInstallChangelog() {
	if a == nil {
		return
	}
	a.serviceMu.Lock()
	dataDir := a.dataDir
	version := a.version
	a.serviceMu.Unlock()
	a.seedFirstInstallChangelogAt(dataDir, version)
}

func (a *App) seedFirstInstallChangelogAt(dataDir string, version string) {
	if a == nil || dataDir == "" || version == "" {
		return
	}
	store := a.configStoreForWorkspace(dataDir, nil)
	if store == nil {
		return
	}
	_, _ = store.EnsureFirstInstallChangelogSeed(version)
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

	a.ensureWorkspaceSession()
	releaseWorkspace, _, workspaceErr := a.beginManagedWorkspaceUse()
	if workspaceErr == nil {
		defer releaseWorkspace()
		svc := a.workspaceSnapshot().service
		if svc != nil {
			svc.Startup(ctx)
			return
		}
	} else if snapshot := a.workspaceSnapshot(); snapshot.service != nil && a.ctx != nil {
		wruntime.LogError(a.ctx, fmt.Sprintf("启动工作目录服务失败: %v", workspaceErr))
	}

	// service 为空：未初始化工作目录，发出 workspace_init mode 状态。
	a.emitWorkspaceInitState()
}

func (a *App) Shutdown(ctx context.Context) {
	// Serialize shutdown with the launch pipeline and stop admitting new managed
	// workspace work. Existing file users are allowed to finish; the session
	// close below cancels/waits for app-owned background tasks.
	releaseShutdown := a.beginWorkspaceShutdown()
	if releaseShutdown != nil {
		defer releaseShutdown()
	}

	// Never restore game files while an owned CS2 process may still be alive.
	// A failed stop retains the runtime and its backups for next-start recovery.
	if err := a.stopProduceSessionWorker(); err != nil {
		if ctx != nil {
			wruntime.LogError(ctx, fmt.Sprintf("stop produce session on shutdown failed: %v", err))
		}
	} else if err := a.forceRestoreProduceEnvironmentForProduce(); err != nil {
		wruntime.LogError(ctx, fmt.Sprintf("restore produce environment failed: %v", err))
	}
	if session := a.workspaceSnapshot().session; session != nil {
		if err := session.close(ctx); err != nil && ctx != nil {
			wruntime.LogError(ctx, fmt.Sprintf("stop workspace session failed: %v", err))
		}
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
	if a == nil {
		return ""
	}
	snapshot := a.workspaceSnapshot()
	if snapshot.session != nil {
		if snapshot.session.isClosed() {
			return ""
		}
		return snapshot.root
	}
	if snapshot.root != "" {
		return snapshot.root
	}
	if snapshot.pendingReset || snapshot.resetComplete {
		return ""
	}
	return snapshot.exeDir
}

func (a *App) dataPath(elem ...string) string {
	parts := append([]string{a.dataRoot()}, elem...)
	return filepath.Join(parts...)
}

func (a *App) configPath() string {
	return a.dataPath("config.json")
}

func (a *App) loadConfig() (*config.Config, error) {
	release, dataDir, err := a.beginManagedWorkspaceUse()
	if err != nil {
		return nil, err
	}
	defer release()
	snapshot := a.workspaceSnapshot()
	store := a.configStoreForWorkspace(dataDir, snapshot.service)
	if store == nil {
		return nil, fmt.Errorf("配置存储未初始化")
	}
	return store.Snapshot()
}

func (a *App) updateConfig(mutate func(*config.Config) error) (*config.Config, error) {
	release, dataDir, err := a.beginManagedWorkspaceUse()
	if err != nil {
		return nil, err
	}
	defer release()
	snapshot := a.workspaceSnapshot()
	store := a.configStoreForWorkspace(dataDir, snapshot.service)
	if store == nil {
		return nil, fmt.Errorf("配置存储未初始化")
	}
	cfg, revision, err := store.UpdateWithRevision(mutate)
	if err != nil {
		return nil, err
	}
	if snapshot.service != nil {
		snapshot.service.ApplyConfigSnapshot(cfg, revision)
	}
	return cfg, nil
}
