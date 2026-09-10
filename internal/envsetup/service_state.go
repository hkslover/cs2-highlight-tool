package envsetup

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/download"
	"cs2-highlight-tool-v2/internal/endpoints"
	"cs2-highlight-tool-v2/internal/release"
)

var (
	errDownloadCanceledByUser = errors.New(downloadCanceledMessage)
	errDownloadFinished       = errors.New("下载已结束")
)

func (s *Service) ensureReleaseSnapshot(source DownloadSource, force bool) error {
	if s.isStopped() {
		return errServiceStopped
	}
	source = normalizeDownloadSource(string(source))
	s.mu.Lock()
	cached := s.releaseSnapshot
	s.mu.Unlock()
	if cached != nil && !force {
		return nil
	}
	apiURL := endpoints.APIURLFor("app", string(source))
	snapshot, err := release.FetchUnifiedLatest(apiURL)
	if err != nil {
		return err
	}
	if s.isStopped() {
		return errServiceStopped
	}
	validationErrors := append([]string(nil), snapshot.AdValidationErrors...)
	mappedAds := mapReleaseAds(snapshot.Ads.Items)
	if shouldUseDebugStartupAds() {
		mappedAds = debugStartupAds()
		validationErrors = nil
		s.emitLogWithFields("info", "已启用后端内置测试广告数据", logFields{
			Component: "ads",
			Stage:     "load",
			Action:    "use_debug_ads",
			Meta: map[string]string{
				"env":   debugStartupAdsEnv,
				"count": fmt.Sprintf("%d", len(mappedAds)),
			},
		})
	}
	s.mu.Lock()
	s.releaseSnapshot = snapshot
	s.state.Ads = mappedAds
	s.mu.Unlock()
	for _, reason := range validationErrors {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			continue
		}
		s.emitLogWithFields("warning", "广告条目已忽略: "+reason, logFields{
			Component: "ads",
			Stage:     "validate",
			Action:    "skip_item",
			Error:     reason,
		})
	}
	return nil
}

func (s *Service) componentReleaseInfo(source DownloadSource, componentID string) (*release.Info, error) {
	if s.isStopped() {
		return nil, errServiceStopped
	}
	if err := s.ensureReleaseSnapshot(source, false); err != nil {
		return nil, err
	}
	s.mu.Lock()
	snapshot := s.releaseSnapshot
	s.mu.Unlock()
	info, ok := snapshot.ComponentInfoBySource(componentID, string(source))
	if !ok || info == nil {
		componentKey, _ := release.ComponentKey(componentID)
		sourceError := strings.TrimSpace(snapshot.ComponentSourceError(componentID, string(source)))
		if sourceError != "" {
			return nil, fmt.Errorf("统一 Release 响应缺少组件数据: %s (%s)", componentKey, sourceError)
		}
		return nil, fmt.Errorf("统一 Release 响应缺少组件数据: %s", componentKey)
	}
	return info, nil
}

func (s *Service) resetTaskStepsLocked() {
	source := s.state.SourceStep.Source
	if source == "" {
		source = string(defaultDownloadSource())
	}
	for i := range s.state.Steps {
		s.state.Steps[i].Status = statusPending
		s.state.Steps[i].Error = ""
		s.state.Steps[i].RemoteVersion = ""
		s.state.Steps[i].ManualURL = endpoints.ManualURLFor(s.state.Steps[i].ID, source)
	}
	s.state.FatalError = ""
	s.state.Ads = nil
	s.state.SelfUpdate = SelfUpdateState{
		Status:  statusPending,
		Current: s.version,
		URL:     endpoints.ManualURLFor("self_update", source),
	}
}

func (s *Service) runComponent(componentID string, fn func() error) {
	if s.isStopped() {
		return
	}
	stepStarted := s.logStepStart(componentID, "check", "run_component", string(s.currentSource()), 0, nil)
	s.updateStep(componentID, func(step *ComponentStatus) {
		step.Status = statusChecking
		step.Error = ""
	})
	if err := fn(); err != nil {
		if s.isStopped() {
			return
		}
		source := s.currentSource()
		s.logStepFail(componentID, "check", "run_component", string(source), 0, stepStarted, err, nil)
		s.failStep(componentID, err, endpoints.ManualURLFor(componentID, string(source)))
		return
	}
	s.logStepDone(componentID, "check", "run_component", string(s.currentSource()), 0, stepStarted, nil)
}

func (s *Service) currentSource() DownloadSource {
	s.mu.Lock()
	defer s.mu.Unlock()
	return normalizeDownloadSource(s.state.SourceStep.Source)
}

func (s *Service) currentCountryCode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.ToUpper(strings.TrimSpace(s.state.SourceStep.CountryCode))
}

func (s *Service) setFatalError(err error) {
	if err == nil || s.isStopped() {
		return
	}
	s.mu.Lock()
	s.state.FatalError = err.Error()
	s.state.CanEnterMain = false
	s.mu.Unlock()
	s.emitLogWithFields("error", err.Error(), logFields{
		Component: "startup",
		Stage:     "fatal",
		Action:    "set_fatal_error",
		Error:     err.Error(),
	})
	s.emitState()
}

func (s *Service) failStep(componentID string, err error, manualURL string) {
	if err == nil || s.isStopped() {
		return
	}
	s.updateStep(componentID, func(step *ComponentStatus) {
		step.Status = statusFailed
		step.Error = err.Error()
		if manualURL != "" {
			step.ManualURL = manualURL
		}
	})
	s.emitLogWithFields("error", err.Error(), logFields{
		Component: componentID,
		Stage:     "failed",
		Action:    "set_step_failed",
		Error:     err.Error(),
		Meta: map[string]string{
			"manual_url": manualURL,
		},
	})
}

func (s *Service) updateStep(componentID string, mutate func(*ComponentStatus)) {
	if s.isStopped() {
		return
	}
	s.mu.Lock()
	if step := s.findStepLocked(componentID); step != nil {
		mutate(step)
	}
	s.mu.Unlock()
	s.emitState()
}

func (s *Service) updateConfig(cfg *config.Config) {
	if cfg == nil || s.isStopped() {
		return
	}
	s.mu.Lock()
	s.state.Config = *cfg
	for i := range s.state.Steps {
		switch s.state.Steps[i].ID {
		case componentHLAE:
			s.state.Steps[i].Path = cfg.HLAEExe
		case componentPlugin:
			s.state.Steps[i].Path = cfg.PluginDLL
		case componentFFmpeg:
			s.state.Steps[i].Path = filepath.Join(cfg.FFmpegDir, "ffmpeg.exe")
		case componentCS2:
			s.state.Steps[i].Path = cfg.CS2Exe
		}
	}
	s.mu.Unlock()
	s.emitState()
}

func (s *Service) currentConfig() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.config == nil {
		return *config.Default(s.dataDir)
	}
	return *s.config
}

func (s *Service) persistConfig(mutate func(*config.Config) error) (*config.Config, error) {
	if s.isStopped() {
		return nil, errServiceStopped
	}
	s.configMu.Lock()
	defer s.configMu.Unlock()
	if s.isStopped() {
		return nil, errServiceStopped
	}

	cfg, err := config.LoadOrCreate(s.configPath, s.dataDir)
	if err != nil {
		return nil, err
	}
	if mutate != nil {
		if err := mutate(cfg); err != nil {
			return nil, err
		}
	}
	if err := config.Save(s.configPath, cfg); err != nil {
		return nil, err
	}
	if s.isStopped() {
		return nil, errServiceStopped
	}
	s.mu.Lock()
	s.config = cfg
	s.mu.Unlock()
	s.updateConfig(cfg)
	return cfg, nil
}

func (s *Service) findStepLocked(componentID string) *ComponentStatus {
	for i := range s.state.Steps {
		if s.state.Steps[i].ID == componentID {
			return &s.state.Steps[i]
		}
	}
	return nil
}

func (s *Service) updateManualURLsLocked(source DownloadSource) {
	source = normalizeDownloadSource(string(source))
	for i := range s.state.Steps {
		s.state.Steps[i].ManualURL = endpoints.ManualURLFor(s.state.Steps[i].ID, string(source))
	}
	s.state.SelfUpdate.URL = endpoints.ManualURLFor("self_update", string(source))
}

func (s *Service) refreshCanEnterMain() {
	if s.isStopped() {
		return
	}
	s.mu.Lock()
	ready := s.state.FatalError == "" && !s.state.SelfUpdate.Available
	warnings := make([]string, 0, 2)
	for _, step := range s.state.Steps {
		switch step.Status {
		case statusReady:
			continue
		case statusWarning:
			line := strings.TrimSpace(step.Error)
			if line == "" {
				line = "最新版本获取失败，当前使用本地版本"
			}
			warnings = append(warnings, fmt.Sprintf("%s: %s", step.Name, line))
		default:
			ready = false
		}
	}
	s.state.EntryNotice = ""
	if ready && len(warnings) > 0 {
		s.state.EntryNotice = "最新版本获取失败，当前将使用本地版本继续运行（离线可用）。部分功能可能异常，建议联网后重试更新。\n" + strings.Join(warnings, "\n")
	}
	s.state.CanEnterMain = ready
	s.mu.Unlock()
	s.emitState()
}

func (s *Service) isFullyReadyLocked() bool {
	if s.state.FatalError != "" || s.state.SelfUpdate.Available || s.state.SelfUpdate.Status != statusReady {
		return false
	}
	for _, step := range s.state.Steps {
		if step.Status != statusReady {
			return false
		}
	}
	return true
}

func (s *Service) updatePhaseByReadiness() {
	if s.isStopped() {
		return
	}
	s.mu.Lock()
	if s.state.CanEnterMain {
		s.state.Phase = phaseReady
	} else if s.state.Phase != phaseWaitingSource && s.state.Phase != phaseDetectingSource {
		s.state.Phase = phaseRunningTasks
	}
	s.mu.Unlock()
	s.emitState()
}

func (s *Service) beginDownloadGroup(componentID string) (*activeDownloadCancel, context.Context, context.CancelCauseFunc) {
	ctx, cancelCause := context.WithCancelCause(s.lifecycleContext())
	active := &activeDownloadCancel{
		cancel: func() { cancelCause(errDownloadCanceledByUser) },
		ctx:    ctx,
	}

	s.cancelMu.Lock()
	if oldCancel, exists := s.cancelMap[componentID]; exists {
		if oldCancel != nil && oldCancel.cancel != nil {
			oldCancel.cancel()
		}
	}
	s.cancelMap[componentID] = active
	s.cancelMu.Unlock()
	return active, ctx, cancelCause
}

func (a *activeDownloadCancel) canceledByUser() bool {
	if a == nil || a.ctx == nil {
		return false
	}
	return errors.Is(context.Cause(a.ctx), errDownloadCanceledByUser)
}

func (s *Service) endDownloadGroup(componentID string, active *activeDownloadCancel) {
	s.cancelMu.Lock()
	if s.cancelMap[componentID] == active {
		delete(s.cancelMap, componentID)
	}
	s.cancelMu.Unlock()
}

func (s *Service) downloadFile(componentID string, url string, targetPath string) error {
	active, ctx, cancelCause := s.beginDownloadGroup(componentID)

	started := s.logStepStart(componentID, "download", "download_asset", string(s.currentSource()), 0, map[string]string{
		"url":    url,
		"target": targetPath,
	})
	err := download.FileWithContext(ctx, url, targetPath, func(active bool, percent float64, indeterminate bool) {
		s.emitProgress(componentID, active, percent, indeterminate)
	})

	// 统一提交：先结束取消仲裁，再移除取消注册。
	// 如果用户取消先于提交发生，即使文件已经下载完成，也以取消为准。
	cancelCause(errDownloadFinished)
	if isUserCancelCause(ctx) {
		err = download.ErrCanceled
	}
	s.endDownloadGroup(componentID, active)
	if err == nil && s.isStopped() {
		err = errServiceStopped
	}

	if err != nil {
		s.logStepFail(componentID, "download", "download_asset", string(s.currentSource()), 0, started, err, map[string]string{
			"url":    url,
			"target": targetPath,
		})
		return err
	}
	s.logStepDone(componentID, "download", "download_asset", string(s.currentSource()), 0, started, map[string]string{
		"url":    url,
		"target": targetPath,
	})
	return nil
}

func (s *Service) logsSnapshot() []LogMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]LogMessage(nil), s.logs...)
}
