package envsetup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/download"
	"cs2-highlight-tool-v2/internal/endpoints"
)

func (s *Service) RunStartupChecks() StartupState {
	s.emitLogWithFields("info", "用户触发启动检查", logFields{
		Component: "startup",
		Action:    "run_startup_checks",
	})

	s.mu.Lock()
	if s.state.Running {
		state := s.state.clone()
		s.mu.Unlock()
		s.emitLogWithFields("warning", "启动检查已在运行中，忽略重复请求", logFields{
			Component: "startup",
			Action:    "run_startup_checks",
		})
		return state
	}
	s.state.Running = true
	s.state.CanEnterMain = false
	s.state.Mode = ModeStartup
	s.state.Phase = phaseDetectingSource
	s.state.FatalError = ""
	if s.state.SourceStep.Source == "" {
		s.state.SourceStep.Source = string(defaultDownloadSource())
	}
	s.state.SourceStep.Status = statusChecking
	s.state.SourceStep.Error = ""
	s.state.SourceStep.Message = "正在检查统一更新源"
	s.resetTaskStepsLocked()
	s.releaseSnapshot = nil
	s.mu.Unlock()
	s.emitState()

	defer func() {
		s.mu.Lock()
		s.state.Running = false
		s.mu.Unlock()
		s.emitState()
	}()

	ensureDirsStart := s.logStepStart("startup", "prepare_dirs", "ensure", "", 0, nil)
	if err := s.ensureWorkDirs(); err != nil {
		s.logStepFail("startup", "prepare_dirs", "ensure", "", 0, ensureDirsStart, err, nil)
		s.setFatalError(err)
		return s.GetStartupState()
	}
	s.logStepDone("startup", "prepare_dirs", "ensure", "", 0, ensureDirsStart, nil)

	loadConfigStart := s.logStepStart("startup", "load_config", "read", "", 0, map[string]string{
		"config_path": s.configPath,
	})
	cfg, err := config.LoadOrCreate(s.configPath, s.dataDir)
	if err != nil {
		s.logStepFail("startup", "load_config", "read", "", 0, loadConfigStart, err, map[string]string{
			"config_path": s.configPath,
		})
		s.setFatalError(err)
		return s.GetStartupState()
	}
	s.logStepDone("startup", "load_config", "read", "", 0, loadConfigStart, map[string]string{
		"config_path": s.configPath,
	})
	s.mu.Lock()
	s.config = cfg
	s.mu.Unlock()
	s.updateConfig(cfg)

	sourceDetectStart := s.logStepStart("source", "detect", "unified_source", "", 1, nil)
	source, countryCode, message, detectErr := s.resolveStartupSource()
	if detectErr != nil {
		s.logStepFail("source", "detect", "unified_source", "", 1, sourceDetectStart, detectErr, nil)
		s.setFatalError(detectErr)
		return s.GetStartupState()
	}
	s.logStepDone("source", "detect", "unified_source", string(source), 1, sourceDetectStart, map[string]string{
		"message": message,
	})

	releaseFetchStart := s.logStepStart("source", "fetch_release", "request_unified", string(source), 1, map[string]string{
		"api_url": endpoints.APIURLFor("app", string(source)),
	})
	releaseErr := s.ensureReleaseSnapshot(source, true)
	if releaseErr != nil {
		s.logStepFail("source", "fetch_release", "request_unified", string(source), 1, releaseFetchStart, releaseErr, nil)
	} else {
		s.logStepDone("source", "fetch_release", "request_unified", string(source), 1, releaseFetchStart, nil)
	}

	sourceMessage := message
	s.mu.Lock()
	if releaseErr != nil {
		s.state.SourceStep.Status = statusFailed
		s.state.SourceStep.Error = releaseErr.Error()
		s.state.SourceStep.Message = "统一更新源请求失败，继续使用本地安装能力"
		sourceMessage = s.state.SourceStep.Message
	} else {
		s.state.SourceStep.Status = statusReady
		s.state.SourceStep.Error = ""
		s.state.SourceStep.Message = message
		sourceMessage = s.state.SourceStep.Message
	}
	s.state.SourceStep.Source = string(source)
	s.state.SourceStep.CountryCode = strings.ToUpper(strings.TrimSpace(countryCode))
	s.state.Phase = phaseRunningTasks
	s.updateManualURLsLocked(source)
	s.mu.Unlock()
	s.emitLogWithFields("info", fmt.Sprintf("下载源: %s (%s)", source, sourceMessage), logFields{
		Component: "source",
		Stage:     "select",
		Action:    "ready_unified",
		Source:    string(source),
	})
	s.emitState()

	runner := s.runTasksFn
	if runner == nil {
		runner = s.runTasksDefault
	}
	taskStart := s.logStepStart("startup", "run_tasks", "execute", string(source), 0, nil)
	runner(source)
	s.logStepDone("startup", "run_tasks", "execute", string(source), 0, taskStart, nil)
	s.updatePhaseByReadiness()
	return s.GetStartupState()
}

func (s *Service) ensureWorkDirs() error {
	for _, dir := range []string{"temp", "hlae", "plugin", "ffmpeg", "updates", "logs", filepath.Join("demo", "raw")} {
		if err := os.MkdirAll(filepath.Join(s.dataDir, dir), 0755); err != nil {
			return fmt.Errorf("创建目录失败 %s: %w", dir, err)
		}
	}
	return nil
}

func (s *Service) resolveStartupSource() (DownloadSource, string, string, error) {
	resolved, err := resolveDownloadSource(nil)
	if err != nil {
		return "", "", "", err
	}
	source := normalizeDownloadSource(string(resolved.Source))
	countryCode := strings.ToUpper(strings.TrimSpace(resolved.CountryCode))
	message := strings.TrimSpace(resolved.Message)
	if message == "" {
		message = fmt.Sprintf("统一更新源已就绪：%s", strings.ToUpper(string(source)))
	}
	return source, countryCode, message, nil
}

func normalizeDownloadSource(source string) DownloadSource {
	source = strings.ToLower(strings.TrimSpace(source))
	for _, supported := range endpoints.SupportedReleaseSources() {
		if source == supported {
			return DownloadSource(source)
		}
	}
	return defaultDownloadSource()
}

func (s *Service) runTasksDefault(source DownloadSource) {
	source = normalizeDownloadSource(string(source))
	s.emitLogWithFields("info", "开始执行组件检查任务", logFields{
		Component: "startup",
		Stage:     "tasks",
		Action:    "start",
		Source:    string(source),
	})

	s.mu.Lock()
	s.state.SelfUpdate = SelfUpdateState{
		Status:  statusChecking,
		Current: s.version,
		URL:     endpoints.ManualURLFor("self_update", string(source)),
	}
	s.mu.Unlock()
	s.emitState()

	jobs := []struct {
		id string
		fn func() error
	}{
		{id: componentHLAE, fn: s.ensureHLAEWithFallback},
		{id: componentPlugin, fn: s.ensurePluginWithFallback},
		{id: componentFFmpeg, fn: s.ensureFFmpeg},
		{id: componentCS2, fn: s.ensureCS2Path},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.checkSelfUpdate(source)
	}()

	for _, job := range jobs {
		job := job
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runComponent(job.id, job.fn)
		}()
	}
	wg.Wait()
	s.emitLogWithFields("info", "组件检查任务执行完成", logFields{
		Component: "startup",
		Stage:     "tasks",
		Action:    "done",
		Source:    string(source),
	})

	s.refreshCanEnterMain()
}

func (s *Service) ensureHLAEWithFallback() error {
	source := s.currentSource()
	stageStarted := s.logStepStart(componentHLAE, "release_source", "load_unified", string(source), 0, nil)
	err := s.ensureHLAE(source)
	if err == nil {
		s.logStepDone(componentHLAE, "release_source", "load_unified", string(source), 0, stageStarted, nil)
		return nil
	}
	if errors.Is(err, download.ErrCanceled) {
		return err
	}
	s.logStepFail(componentHLAE, "release_source", "load_unified", string(source), 0, stageStarted, err, nil)
	cfg := s.currentConfig()
	localVersion, localErr := resolveInstalledHLAEVersion(cfg.HLAEExe)
	if localErr == nil {
		message := "最新版本获取失败，当前使用本地版本，可能导致后续功能异常"
		s.updateStep(componentHLAE, func(step *ComponentStatus) {
			step.Status = statusWarning
			step.LocalVersion = localVersion
			step.Path = cfg.HLAEExe
			step.Error = message
		})
		s.emitLogWithFields("warning", fmt.Sprintf("HLAE: %s", message), logFields{
			Component: componentHLAE,
			Stage:     "fallback_local",
			Action:    "use_local",
			Source:    string(source),
			Error:     message,
		})
		return nil
	}
	return err
}

func (s *Service) ensurePluginWithFallback() error {
	source := s.currentSource()
	stageStarted := s.logStepStart(componentPlugin, "release_source", "load_unified", string(source), 0, nil)
	err := s.ensurePlugin(source)
	if err == nil {
		s.logStepDone(componentPlugin, "release_source", "load_unified", string(source), 0, stageStarted, nil)
		return nil
	}
	if errors.Is(err, download.ErrCanceled) {
		return err
	}
	s.logStepFail(componentPlugin, "release_source", "load_unified", string(source), 0, stageStarted, err, nil)
	cfg := s.currentConfig()
	localVersion, localErr := resolveInstalledPluginVersion(cfg.PluginDLL)
	if localErr == nil {
		message := "最新版本获取失败，当前使用本地版本，可能导致后续功能异常"
		s.updateStep(componentPlugin, func(step *ComponentStatus) {
			step.Status = statusWarning
			step.LocalVersion = localVersion
			step.Path = cfg.PluginDLL
			step.Error = message
		})
		s.emitLogWithFields("warning", fmt.Sprintf("插件: %s", message), logFields{
			Component: componentPlugin,
			Stage:     "fallback_local",
			Action:    "use_local",
			Source:    string(source),
			Error:     message,
		})
		return nil
	}
	return err
}

func defaultDownloadSource() DownloadSource {
	supported := endpoints.SupportedReleaseSources()
	if len(supported) == 0 {
		return DownloadSourceCustom
	}
	return DownloadSource(supported[0])
}
