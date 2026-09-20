package app

import (
	"os"
	"path/filepath"
	"testing"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/envsetup"
)

// newTestApp 构造一个已安装工作目录身份的 App，与生产装配保持一致：真实
// workspace session + 该根目录下的 config store，根目录取 exeDir。
//
// 历史上这些用例依赖 beginManagedWorkspaceTaskUse 里的非 Windows 兜底
// （dataDir 为空时退回 exeDir），于是整套 internal/app 测试只在 macOS/Linux
// 通过，在 Windows 上全部报“请先完成工作目录初始化”。显式安装既保留原来的
// 根目录语义，也不再依赖平台分支。
func newTestApp(t *testing.T, exeDir string, configure ...func(*App)) *App {
	t.Helper()
	if exeDir == "" {
		t.Fatal("newTestApp requires a non-empty exeDir")
	}
	a := &App{exeDir: exeDir}
	for _, apply := range configure {
		apply(a)
	}
	installTestWorkspace(t, a, exeDir)
	return a
}

// newTestAppInTempDir 等价于 newTestApp(t, t.TempDir())，用于只关心“有一个
// 隔离工作目录”的用例。
func newTestAppInTempDir(t *testing.T) *App {
	t.Helper()
	return newTestApp(t, t.TempDir())
}

// newTestAppWithRoot 用于需要把工作目录与 exeDir 分开的用例。
func newTestAppWithRoot(t *testing.T, exeDir string, root string, configure ...func(*App)) *App {
	t.Helper()
	if exeDir == "" || root == "" {
		t.Fatal("newTestAppWithRoot requires non-empty exeDir and root")
	}
	a := &App{exeDir: exeDir}
	for _, apply := range configure {
		apply(a)
	}
	installTestWorkspace(t, a, root)
	return a
}

// installTestWorkspace 安装工作目录身份，等价于生产侧 installWorkspaceLocked
// 的装配方式（session 持有 Store，App 与 Service 共享同一个 Store）。
func installTestWorkspace(t *testing.T, a *App, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create test workspace root: %v", err)
	}
	store := config.NewStore(filepath.Join(root, "config.json"), root)
	svc := envsetup.NewWithDataDirAndStore(a.exeDir, root, "test", store)
	a.serviceMu.Lock()
	a.installWorkspaceLocked(root, svc)
	a.serviceMu.Unlock()
}
