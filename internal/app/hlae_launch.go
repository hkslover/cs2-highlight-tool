package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/config"
)

var launchHLAECommand = exec.Command

// A successful launcher Start followed by failed PID detection is still an
// owned launch. Keep its handle until it exits; never equate unknown PID to
// proof that no game process exists.
type pendingHLAELaunch struct {
	process *os.Process
	done    <-chan struct{}
}

type hlaeLaunchError struct {
	err     error
	pending *pendingHLAELaunch
}

func (e *hlaeLaunchError) Error() string { return e.err.Error() }
func (e *hlaeLaunchError) Unwrap() error { return e.err }

func (p *pendingHLAELaunch) stop() error {
	select {
	case <-p.done:
	default:
		if err := p.process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("关闭 HLAE 启动器失败: %w", err)
		}
		select {
		case <-p.done:
		case <-time.After(3 * time.Second):
			return fmt.Errorf("等待 HLAE 启动器退出超时")
		}
	}
	// Detection failed, so no CS2 PID can safely be claimed as ours. Do not
	// kill unrelated games. Keep the backups until enumeration succeeds and
	// all CS2 processes are gone, including any child started before Kill.
	pids, err := listCS2PIDsFn()
	if err != nil {
		return fmt.Errorf("无法确认 CS2 已退出，已保留环境备份: %w", err)
	}
	for _, pid := range pids {
		if pid > 0 {
			return fmt.Errorf("仍有 CS2 进程，无法确认启动归属；请关闭 CS2 后重试，环境备份已保留")
		}
	}
	return nil
}

type launchJobContext struct {
	job      GeneratePluginJSONRequest
	baseItem GeneratePluginJSONBatchItemResult
	allItems *normalizedSelectedItems
	plans    []ProduceTakePlan
}

func (a *App) launchHLAEGame() (int, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return 0, err
	}

	hlaeExe := config.CleanPath(cfg.HLAEExe)
	if hlaeExe == "" {
		return 0, fmt.Errorf("HLAE 路径为空")
	}
	if _, err := os.Stat(hlaeExe); err != nil {
		return 0, fmt.Errorf("HLAE 不存在: %s", hlaeExe)
	}
	hookDLL := filepath.Join(filepath.Dir(hlaeExe), "x64", "AfxHookSource2.dll")
	if _, err := os.Stat(hookDLL); err != nil {
		return 0, fmt.Errorf("AfxHookSource2.dll 不存在: %s", hookDLL)
	}

	cs2Exe, err := resolveCS2ExeForLaunch(cfg)
	if err != nil {
		return 0, err
	}
	if _, err := os.Stat(cs2Exe); err != nil {
		return 0, fmt.Errorf("CS2 不存在: %s", cs2Exe)
	}

	beforePIDs, err := listCS2PIDsFn()
	if err != nil {
		return 0, fmt.Errorf("枚举 cs2 进程失败: %w", err)
	}
	if a.produceW == nil {
		return 0, fmt.Errorf("produce websocket server is not available")
	}
	port, err := a.produceW.Port()
	if err != nil {
		return 0, fmt.Errorf("获取 produce websocket 端口失败: %w", err)
	}
	pluginLogPath := a.dataPath("logs", "cs2-server-plugin.log")
	if err := os.MkdirAll(filepath.Dir(pluginLogPath), 0o755); err != nil {
		return 0, fmt.Errorf("创建插件日志目录失败: %w", err)
	}

	cmdLine := buildHLAECommandLine(cfg.LaunchResolution)
	args := []string{
		"-noGui", "-autoStart", "-noConfig",
		"-afxDisableSteamStorage", "-customLoader",
		"-hookDllPath", hookDLL,
		"-programPath", cs2Exe,
		"-cmdLine", cmdLine,
	}

	cmd := launchHLAECommand(hlaeExe, args...)
	cmd.Env = append(
		os.Environ(),
		fmt.Sprintf("CSDM_WS_PORT=%d", port),
		"CSDM_LOG_PATH="+pluginLogPath,
	)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("启动 HLAE 失败: %w", err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	pid, err := waitForNewCS2PID(snapshotPIDSet(beforePIDs), cs2ProcessDetectTimeout, cs2ProcessDetectPollInterval)
	if err != nil {
		return 0, &hlaeLaunchError{
			err:     fmt.Errorf("启动 HLAE 后未识别到新的 cs2.exe 进程: %w", err),
			pending: &pendingHLAELaunch{process: cmd.Process, done: done},
		}
	}
	return pid, nil
}

func buildHLAECommandLine(launchResolution string) string {
	cmdLine := "-insecure -novid -low -high +sv_lan 1 -coop_fullscreen -worldwide -console"
	switch strings.TrimSpace(launchResolution) {
	case config.LaunchResolution4x3:
		cmdLine += " -w 1440 -h 1080"
	case config.LaunchResolution4x3Low:
		cmdLine += " -w 1280 -h 960"
	}
	return cmdLine
}

func resolveCS2ExeForLaunch(cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置为空")
	}
	candidates := make([]string, 0, 5)
	if cleaned := config.CleanPath(cfg.CS2Exe); cleaned != "" {
		candidates = append(candidates, cleaned)
	}
	if cleaned := config.CleanPath(cfg.CS2Dir); cleaned != "" {
		candidates = append(candidates,
			filepath.Join(cleaned, "cs2.exe"),
			filepath.Join(cleaned, "game", "bin", "win64", "cs2.exe"),
		)
	}
	for _, candidate := range candidates {
		if strings.ToLower(filepath.Base(candidate)) != "cs2.exe" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("请选择包含 cs2.exe 的 CS2 安装目录")
}
