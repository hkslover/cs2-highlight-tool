package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cs2-highlight-tool-v2/internal/config"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type DebugPluginDLLOverrideState struct {
	Active bool   `json:"active"`
	Path   string `json:"path"`
}

func (a *App) GetDebugPluginDLLOverride() DebugPluginDLLOverrideState {
	return a.debugPluginDLLOverrideState()
}

func (a *App) PickDebugPluginDLLOverride() (DebugPluginDLLOverrideState, error) {
	if a.ctx == nil {
		return a.debugPluginDLLOverrideState(), fmt.Errorf("应用尚未启动")
	}
	selected, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择调试插件 DLL",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "DLL Files (*.dll)", Pattern: "*.dll"},
		},
	})
	if err != nil {
		return a.debugPluginDLLOverrideState(), fmt.Errorf("打开 DLL 选择对话框失败: %w", err)
	}
	if strings.TrimSpace(selected) == "" {
		return a.debugPluginDLLOverrideState(), nil
	}
	return a.setDebugPluginDLLOverride(selected)
}

func (a *App) ClearDebugPluginDLLOverride() DebugPluginDLLOverrideState {
	if a == nil {
		return DebugPluginDLLOverrideState{}
	}
	a.debugPluginDLLMu.Lock()
	a.debugPluginDLLOverride = ""
	a.debugPluginDLLMu.Unlock()
	return DebugPluginDLLOverrideState{}
}

func (a *App) setDebugPluginDLLOverride(path string) (DebugPluginDLLOverrideState, error) {
	cleaned, err := validateDebugPluginDLLPath(path)
	if err != nil {
		return a.debugPluginDLLOverrideState(), err
	}
	a.debugPluginDLLMu.Lock()
	a.debugPluginDLLOverride = cleaned
	a.debugPluginDLLMu.Unlock()
	return DebugPluginDLLOverrideState{Active: true, Path: cleaned}, nil
}

func (a *App) resolvePluginDLLSourcePath(cfg *config.Config) string {
	if override := a.debugPluginDLLOverridePath(); override != "" {
		return override
	}
	if cfg == nil {
		return ""
	}
	return config.CleanPath(cfg.PluginDLL)
}

func (a *App) debugPluginDLLOverrideState() DebugPluginDLLOverrideState {
	path := a.debugPluginDLLOverridePath()
	return DebugPluginDLLOverrideState{
		Active: path != "",
		Path:   path,
	}
}

func (a *App) debugPluginDLLOverridePath() string {
	if a == nil {
		return ""
	}
	a.debugPluginDLLMu.Lock()
	defer a.debugPluginDLLMu.Unlock()
	return a.debugPluginDLLOverride
}

func validateDebugPluginDLLPath(path string) (string, error) {
	cleaned := config.CleanPath(path)
	if cleaned == "" {
		return "", fmt.Errorf("调试插件 DLL 路径为空")
	}
	if !strings.EqualFold(filepath.Ext(cleaned), ".dll") {
		return "", fmt.Errorf("请选择 DLL 文件: %s", cleaned)
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("调试插件 DLL 不存在: %s", cleaned)
		}
		return "", fmt.Errorf("读取调试插件 DLL 失败: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("调试插件 DLL 路径不是文件: %s", cleaned)
	}
	return cleaned, nil
}
