package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"cs2-highlight-tool-v2/internal/config"
)

var launchHLAECommand = exec.Command

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

	cmdLine := buildHLAECommandLine(cfg.LaunchResolution)
	args := []string{
		"-noGui", "-autoStart", "-noConfig",
		"-afxDisableSteamStorage", "-customLoader",
		"-hookDllPath", hookDLL,
		"-programPath", cs2Exe,
		"-cmdLine", cmdLine,
	}

	cmd := launchHLAECommand(hlaeExe, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("CSDM_WS_PORT=%d", port))
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("启动 HLAE 失败: %w", err)
	}

	pid, err := waitForNewCS2PID(snapshotPIDSet(beforePIDs), cs2ProcessDetectTimeout, cs2ProcessDetectPollInterval)
	if err != nil {
		return 0, fmt.Errorf("启动 HLAE 后未识别到新的 cs2.exe 进程: %w", err)
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
