package envsetup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cs2-highlight-tool-v2/internal/config"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (s *Service) RetryStartupComponent(componentID string) StartupState {
	componentID = strings.TrimSpace(componentID)
	s.emitLogWithFields("info", "用户触发重试组件", logFields{
		Component: componentID,
		Action:    "user_retry",
	})

	s.mu.Lock()
	if s.state.Phase == phaseReady {
		s.state.Phase = phaseRunningTasks
	}
	s.mu.Unlock()
	s.emitState()

	switch componentID {
	case componentHLAE:
		s.runComponent(componentHLAE, func() error {
			return s.ensureHLAEWithFallback()
		})
	case componentPlugin:
		s.runComponent(componentPlugin, func() error {
			return s.ensurePluginWithFallback()
		})
	case componentFFmpeg:
		s.runComponent(componentFFmpeg, func() error { return s.ensureFFmpeg() })
	case componentCS2:
		s.runComponent(componentCS2, s.ensureCS2Path)
	default:
		s.emitLog("warning", "未知检查项: "+componentID)
	}
	s.refreshCanEnterMain()
	s.updatePhaseByReadiness()
	return s.GetStartupState()
}

func (s *Service) ReinstallStartupComponent(componentID string) (StartupState, error) {
	componentID = strings.TrimSpace(componentID)
	s.emitLogWithFields("info", "用户触发重装组件", logFields{
		Component: componentID,
		Action:    "user_reinstall",
	})
	if !isReinstallableComponent(componentID) {
		return s.GetStartupState(), fmt.Errorf("不支持重装组件: %s", componentID)
	}

	s.mu.Lock()
	canReinstall := s.isFullyReadyLocked()
	if canReinstall {
		if s.state.Phase == phaseReady {
			s.state.Phase = phaseRunningTasks
		}
		s.state.Running = true
		s.state.CanEnterMain = false
	}
	s.mu.Unlock()
	if !canReinstall {
		return s.GetStartupState(), fmt.Errorf("仅可在全部检测成功后执行重装")
	}
	s.emitState()
	defer func() {
		s.mu.Lock()
		s.state.Running = false
		s.mu.Unlock()
		s.emitState()
	}()

	switch componentID {
	case componentHLAE:
		s.runComponent(componentHLAE, s.reinstallHLAE)
	case componentPlugin:
		s.runComponent(componentPlugin, s.reinstallPlugin)
	case componentFFmpeg:
		s.runComponent(componentFFmpeg, s.reinstallFFmpeg)
	}
	s.refreshCanEnterMain()
	s.updatePhaseByReadiness()
	return s.GetStartupState(), nil
}

func (s *Service) CancelStartupDownload(componentID string) StartupState {
	componentID = strings.TrimSpace(componentID)
	s.emitLogWithFields("info", "用户触发取消下载", logFields{
		Component: componentID,
		Action:    "cancel_download",
	})

	if !isCancelableDownloadComponent(componentID) {
		s.emitLogWithFields("warning", "当前组件不支持取消下载", logFields{
			Component: componentID,
			Action:    "cancel_download",
		})
		return s.GetStartupState()
	}

	s.cancelMu.Lock()
	active, exists := s.cancelMap[componentID]
	s.cancelMu.Unlock()

	if !exists || active == nil || active.cancel == nil {
		s.emitLogWithFields("warning", "当前组件没有正在进行的下载", logFields{
			Component: componentID,
			Action:    "cancel_download",
		})
		return s.GetStartupState()
	}

	active.cancel()
	if !active.canceledByUser() {
		// 竞速已经提交胜者或下载已经完成，本次取消不再生效，避免界面先显示“已取消”又继续安装。
		s.emitLogWithFields("info", "当前组件下载已结束，忽略取消请求", logFields{
			Component: componentID,
			Action:    "cancel_download",
		})
		return s.GetStartupState()
	}
	s.updateStep(componentID, func(step *ComponentStatus) {
		step.Status = statusFailed
		step.Error = downloadCanceledMessage
	})
	s.emitProgress(componentID, false, 0, false)
	s.refreshCanEnterMain()
	s.updatePhaseByReadiness()
	return s.GetStartupState()
}

func isCancelableDownloadComponent(componentID string) bool {
	switch componentID {
	case componentHLAE, componentPlugin, componentFFmpeg:
		return true
	default:
		return false
	}
}

func (s *Service) OpenManualDownload(componentID string) error {
	s.emitLogWithFields("info", "用户打开手动下载页", logFields{
		Component: componentID,
		Action:    "open_manual_download",
	})
	url := ""
	s.mu.Lock()
	if componentID == "self_update" {
		url = s.state.SelfUpdate.URL
	} else if step := s.findStepLocked(componentID); step != nil {
		url = step.ManualURL
	}
	s.mu.Unlock()
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("当前步骤没有可用的手动下载链接")
	}
	runtime.BrowserOpenURL(s.ctx, url)
	s.emitLogWithFields("info", "已打开手动下载页", logFields{
		Component: componentID,
		Action:    "open_manual_download",
		Meta: map[string]string{
			"url": url,
		},
	})
	return nil
}

func (s *Service) OpenExternalURL(rawURL string) error {
	url, ok := normalizeExternalOpenURL(rawURL)
	if !ok {
		return fmt.Errorf("无效外部链接")
	}
	s.emitLogWithFields("info", "用户打开外部链接", logFields{
		Component: "ads",
		Action:    "open_external_url",
		Meta: map[string]string{
			"url": url,
		},
	})
	runtime.BrowserOpenURL(s.ctx, url)
	return nil
}

func (s *Service) ImportManualDownload(componentID string) StartupState {
	s.emitLogWithFields("info", "用户触发手动导入", logFields{
		Component: componentID,
		Action:    "manual_import",
	})
	path, err := s.pickManualFile(componentID)
	if err != nil {
		s.failStep(componentID, err, "")
		return s.GetStartupState()
	}
	if path == "" {
		s.emitLogWithFields("warning", "手动导入已取消", logFields{
			Component: componentID,
			Action:    "manual_import",
		})
		return s.GetStartupState()
	}
	s.emitLogWithFields("info", "手动导入文件已选择", logFields{
		Component: componentID,
		Action:    "manual_import",
		Meta: map[string]string{
			"path": path,
		},
	})

	s.mu.Lock()
	if s.state.Phase == phaseReady {
		s.state.Phase = phaseRunningTasks
	}
	s.mu.Unlock()
	s.emitState()

	switch componentID {
	case componentHLAE:
		s.runComponent(componentHLAE, func() error { return s.installHLAEFromArchive(path) })
	case componentPlugin:
		s.runComponent(componentPlugin, func() error { return s.installPluginFromFile(path) })
	case componentFFmpeg:
		s.runComponent(componentFFmpeg, func() error { return s.installFFmpegFromArchive(path) })
	default:
		s.failStep(componentID, fmt.Errorf("不支持手动导入: %s", componentID), "")
	}
	s.refreshCanEnterMain()
	s.updatePhaseByReadiness()
	return s.GetStartupState()
}

func (s *Service) PickCS2Path() StartupState {
	s.emitLogWithFields("info", "用户触发选择 CS2 路径", logFields{
		Component: componentCS2,
		Action:    "pick_path",
	})
	selected, err := runtime.OpenDirectoryDialog(s.ctx, runtime.OpenDialogOptions{
		Title:                "选择 CS2 安装目录",
		CanCreateDirectories: false,
	})
	if err != nil {
		s.failStep(componentCS2, err, "")
		return s.GetStartupState()
	}
	if selected == "" {
		s.emitLogWithFields("warning", "选择 CS2 路径已取消", logFields{
			Component: componentCS2,
			Action:    "pick_path",
		})
		return s.GetStartupState()
	}
	s.emitLogWithFields("info", "用户已选择 CS2 目录", logFields{
		Component: componentCS2,
		Action:    "pick_path",
		Meta: map[string]string{
			"selected": selected,
		},
	})
	if err := s.saveCS2Path(selected); err != nil {
		s.failStep(componentCS2, err, "")
		return s.GetStartupState()
	}

	s.mu.Lock()
	if s.state.Phase == phaseReady {
		s.state.Phase = phaseRunningTasks
	}
	s.mu.Unlock()
	s.emitState()

	s.runComponent(componentCS2, s.ensureCS2Path)
	s.refreshCanEnterMain()
	s.updatePhaseByReadiness()
	return s.GetStartupState()
}

func (s *Service) EnterMainApp() error {
	s.emitLogWithFields("info", "用户尝试进入主页面", logFields{
		Component: "startup",
		Action:    "enter_main_app",
	})
	s.refreshCanEnterMain()
	s.updatePhaseByReadiness()
	s.mu.Lock()
	canEnter := s.state.CanEnterMain
	if canEnter {
		s.state.Mode = ModeMain
	}
	s.mu.Unlock()
	if !canEnter {
		return fmt.Errorf("环境准备尚未完成")
	}
	runtime.WindowSetTitle(s.ctx, "CS2 Highlight Tool v2")
	s.emitState()
	s.emitLogWithFields("info", "已进入主页面", logFields{
		Component: "startup",
		Action:    "enter_main_app",
	})
	return nil
}

func isReinstallableComponent(componentID string) bool {
	switch componentID {
	case componentHLAE, componentPlugin, componentFFmpeg:
		return true
	default:
		return false
	}
}

func (s *Service) reinstallHLAE() error {
	targetDir := filepath.Join(s.dataDir, "hlae")
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("删除 HLAE 目录失败: %w", err)
	}
	defaults := config.Default(s.dataDir)
	if _, err := s.persistConfig(func(next *config.Config) error {
		next.HLAEExe = defaults.HLAEExe
		return nil
	}); err != nil {
		return err
	}
	return s.ensureHLAEWithFallback()
}

func (s *Service) reinstallPlugin() error {
	targetDir := filepath.Join(s.dataDir, "plugin")
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("删除插件目录失败: %w", err)
	}
	defaults := config.Default(s.dataDir)
	if _, err := s.persistConfig(func(next *config.Config) error {
		next.PluginDLL = defaults.PluginDLL
		return nil
	}); err != nil {
		return err
	}
	return s.ensurePluginWithFallback()
}

func (s *Service) reinstallFFmpeg() error {
	s.stopFFmpegCapabilityDetection()
	targetDir := filepath.Join(s.dataDir, "ffmpeg")
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("删除 ffmpeg 目录失败: %w", err)
	}
	defaults := config.Default(s.dataDir)
	if _, err := s.persistConfig(func(next *config.Config) error {
		next.FFmpegDir = defaults.FFmpegDir
		return nil
	}); err != nil {
		return err
	}
	return s.ensureFFmpeg()
}
