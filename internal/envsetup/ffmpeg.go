package envsetup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/download"
	"cs2-highlight-tool-v2/internal/endpoints"
	"cs2-highlight-tool-v2/internal/ffmpegprofile"
)

var ffmpegDetectCommandContext = exec.CommandContext
var ffmpegDetectAsyncRunner = func(task func()) {
	go task()
}

func (s *Service) ensureFFmpeg() error {
	ffmpegDir := filepath.Join(s.dataDir, "ffmpeg", "bin")
	ffmpegExe := filepath.Join(ffmpegDir, "ffmpeg.exe")
	checkStarted := s.logStepStart(componentFFmpeg, "check", "validate_local", string(s.currentSource()), 0, map[string]string{
		"local_exe": ffmpegExe,
	})
	if _, err := os.Stat(ffmpegExe); err == nil {
		s.logStepDone(componentFFmpeg, "check", "validate_local", string(s.currentSource()), 0, checkStarted, map[string]string{
			"local_ready": "true",
		})
		persistStarted := s.logStepStart(componentFFmpeg, "persist_config", "write", string(s.currentSource()), 0, nil)
		_, err := s.persistConfig(func(next *config.Config) error {
			next.FFmpegDir = ffmpegDir
			return nil
		})
		if err != nil {
			s.logStepFail(componentFFmpeg, "persist_config", "write", string(s.currentSource()), 0, persistStarted, err, nil)
			return err
		}
		s.logStepDone(componentFFmpeg, "persist_config", "write", string(s.currentSource()), 0, persistStarted, map[string]string{
			"config_ffmpeg_dir": ffmpegDir,
		})
		s.updateStep(componentFFmpeg, func(step *ComponentStatus) {
			step.Status = statusReady
			step.Path = ffmpegExe
			step.Error = ""
		})
		s.emitLogWithFields("info", "ffmpeg 已就绪（本地已存在）", logFields{
			Component: componentFFmpeg,
			Stage:     "ready",
			Action:    "component_ready",
			Meta: map[string]string{
				"path": ffmpegExe,
			},
		})
		s.tryWriteHLAEFfmpegIni(ffmpegExe)
		s.scheduleFFmpegCapabilityDetection(ffmpegExe)
		return nil
	}
	s.logStepDone(componentFFmpeg, "check", "validate_local", string(s.currentSource()), 0, checkStarted, map[string]string{
		"local_ready": "false",
	})

	s.updateStep(componentFFmpeg, func(step *ComponentStatus) { step.Status = statusDownloading })
	tempArchive := filepath.Join(s.dataDir, "temp", "ffmpeg-fixed.7z")
	if err := s.downloadFile(componentFFmpeg, endpoints.FFmpegFixedDownloadURL, tempArchive); err != nil {
		return fmt.Errorf("下载 ffmpeg 失败: %w", err)
	}
	return s.installFFmpegFromArchive(tempArchive)
}

func (s *Service) installFFmpegFromArchive(path string) error {
	s.updateStep(componentFFmpeg, func(step *ComponentStatus) { step.Status = statusInstalling })
	extractDir := filepath.Join(s.dataDir, "temp", "ffmpeg_extract")
	_ = os.RemoveAll(extractDir)
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(extractDir)
	extractStarted := s.logStepStart(componentFFmpeg, "extract", "unarchive", string(s.currentSource()), 0, map[string]string{
		"archive_path": path,
		"extract_dir":  extractDir,
	})
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip":
		if err := download.Unzip(path, extractDir); err != nil {
			s.logStepFail(componentFFmpeg, "extract", "unarchive", string(s.currentSource()), 0, extractStarted, err, map[string]string{
				"archive_type": "zip",
			})
			return err
		}
	default:
		if err := download.Extract7z(path, extractDir); err != nil {
			s.logStepFail(componentFFmpeg, "extract", "unarchive", string(s.currentSource()), 0, extractStarted, err, map[string]string{
				"archive_type": "7z",
			})
			return err
		}
	}
	s.logStepDone(componentFFmpeg, "extract", "unarchive", string(s.currentSource()), 0, extractStarted, nil)

	validateStarted := s.logStepStart(componentFFmpeg, "validate", "verify_archive", string(s.currentSource()), 0, nil)
	ffmpegExe, err := download.FindFile(extractDir, "ffmpeg.exe")
	if err != nil {
		s.logStepFail(componentFFmpeg, "validate", "verify_archive", string(s.currentSource()), 0, validateStarted, err, nil)
		return err
	}
	binDir := filepath.Dir(ffmpegExe)
	root := filepath.Dir(binDir)
	targetRoot := filepath.Join(s.dataDir, "ffmpeg")
	if err := download.ReplaceDirWithContents(root, targetRoot); err != nil {
		s.logStepFail(componentFFmpeg, "validate", "verify_archive", string(s.currentSource()), 0, validateStarted, err, nil)
		return err
	}
	ffmpegDir := filepath.Join(targetRoot, "bin")
	if _, err := os.Stat(filepath.Join(ffmpegDir, "ffmpeg.exe")); err != nil {
		s.logStepFail(componentFFmpeg, "validate", "verify_archive", string(s.currentSource()), 0, validateStarted, err, nil)
		return fmt.Errorf("ffmpeg.exe 安装后仍不存在")
	}
	s.logStepDone(componentFFmpeg, "validate", "verify_archive", string(s.currentSource()), 0, validateStarted, map[string]string{
		"target_dir": ffmpegDir,
	})

	persistStarted := s.logStepStart(componentFFmpeg, "persist_config", "write", string(s.currentSource()), 0, nil)
	cfg, err := s.persistConfig(func(next *config.Config) error {
		next.FFmpegDir = ffmpegDir
		return nil
	})
	if err != nil {
		s.logStepFail(componentFFmpeg, "persist_config", "write", string(s.currentSource()), 0, persistStarted, err, nil)
		return err
	}
	s.logStepDone(componentFFmpeg, "persist_config", "write", string(s.currentSource()), 0, persistStarted, map[string]string{
		"config_ffmpeg_dir": cfg.FFmpegDir,
	})
	s.updateStep(componentFFmpeg, func(step *ComponentStatus) {
		step.Status = statusReady
		step.Path = filepath.Join(cfg.FFmpegDir, "ffmpeg.exe")
		step.Error = ""
	})
	s.emitLogWithFields("info", "ffmpeg 安装完成", logFields{
		Component: componentFFmpeg,
		Stage:     "ready",
		Action:    "component_ready",
		Source:    string(s.currentSource()),
		Meta: map[string]string{
			"path": filepath.Join(cfg.FFmpegDir, "ffmpeg.exe"),
		},
	})
	s.tryWriteHLAEFfmpegIni(filepath.Join(cfg.FFmpegDir, "ffmpeg.exe"))
	s.scheduleFFmpegCapabilityDetection(filepath.Join(cfg.FFmpegDir, "ffmpeg.exe"))
	return nil
}

func (s *Service) scheduleFFmpegCapabilityDetection(ffmpegExe string) {
	ffmpegExe = strings.TrimSpace(ffmpegExe)
	if ffmpegExe == "" {
		return
	}

	s.ffmpegDetectMu.Lock()
	if s.ffmpegDetectRunning {
		s.ffmpegDetectMu.Unlock()
		s.emitLogWithFields("info", "ffmpeg 能力探测任务已在运行，跳过重复调度", logFields{
			Component: componentFFmpeg,
			Stage:     "detect_profile",
			Action:    "skip_duplicate",
			Meta: map[string]string{
				"ffmpeg_exe": ffmpegExe,
			},
		})
		return
	}
	detectCtx, releaseTask, ok := s.beginTaskIfOpen()
	if !ok {
		s.ffmpegDetectMu.Unlock()
		return
	}
	s.ffmpegDetectRunning = true
	// Keep a dedicated cancel for reinstall while deriving the task from the
	// service lifecycle context. The probe exits when either one is canceled.
	probeCtx, probeCancel := context.WithCancel(detectCtx)
	s.ffmpegDetectWG.Add(1)
	s.ffmpegDetectCancel = probeCancel
	s.ffmpegDetectMu.Unlock()

	ffmpegDetectAsyncRunner(func() {
		defer func() {
			releaseTask()
			s.ffmpegDetectMu.Lock()
			s.ffmpegDetectRunning = false
			s.ffmpegDetectCancel = nil
			s.ffmpegDetectMu.Unlock()
			s.ffmpegDetectWG.Done()
		}()
		s.detectAndCacheFFmpegCapabilities(probeCtx, ffmpegExe)
	})
}

func (s *Service) stopFFmpegCapabilityDetection() {
	s.ffmpegDetectMu.Lock()
	cancel := s.ffmpegDetectCancel
	running := s.ffmpegDetectRunning
	s.ffmpegDetectMu.Unlock()
	if !running || cancel == nil {
		return
	}
	s.emitLogWithFields("info", "正在停止 ffmpeg 能力探测", logFields{
		Component: componentFFmpeg,
		Stage:     "detect_profile",
		Action:    "cancel_probe",
	})
	cancel()
	s.ffmpegDetectWG.Wait()
}

func (s *Service) detectAndCacheFFmpegCapabilities(ctx context.Context, ffmpegExe string) {
	if ctx == nil {
		ctx = s.lifecycleContext()
	}
	if s.isStopped() {
		return
	}
	started := s.logStepStart(componentFFmpeg, "detect_profile", "probe_encoders", string(s.currentSource()), 0, map[string]string{
		"ffmpeg_exe": ffmpegExe,
	})
	caps, err := ffmpegprofile.DetectCapabilities(ctx, ffmpegExe, ffmpegDetectCommandContext)
	if s.isStopped() {
		return
	}
	if errors.Is(err, context.Canceled) {
		s.logStepDone(componentFFmpeg, "detect_profile", "probe_encoders", string(s.currentSource()), 0, started, map[string]string{
			"canceled": "true",
		})
		return
	}
	if err != nil {
		s.logStepFail(componentFFmpeg, "detect_profile", "probe_encoders", string(s.currentSource()), 0, started, err, map[string]string{
			"ffmpeg_exe": ffmpegExe,
		})
		s.emitLogWithFields("warning", "ffmpeg 能力探测失败，将回退默认编码策略", logFields{
			Component: componentFFmpeg,
			Stage:     "detect_profile",
			Action:    "probe_encoders",
			Error:     err.Error(),
		})
		return
	}

	resolved := ffmpegprofile.ResolveProfile(ffmpegprofile.UserPresetAuto, caps)
	detectedPreset := strings.TrimSpace(resolved.SelectedProfile.ID)
	if detectedPreset == "" {
		detectedPreset = ffmpegprofile.UserPresetC1
	}
	encoders := caps.AvailableEncoders()
	if len(encoders) == 0 {
		encoders = []string{"libx264"}
	}

	persistStarted := s.logStepStart(componentFFmpeg, "detect_profile", "persist_cache", string(s.currentSource()), 0, map[string]string{
		"detected_preset": detectedPreset,
	})
	_, persistErr := s.persistConfig(func(next *config.Config) error {
		next.FFmpegDetectedPreset = detectedPreset
		next.FFmpegDetectedEncoders = append([]string(nil), encoders...)
		next.FFmpegDetectedAt = caps.TestedAt.Format(time.RFC3339)
		return nil
	})
	if persistErr != nil {
		s.logStepFail(componentFFmpeg, "detect_profile", "persist_cache", string(s.currentSource()), 0, persistStarted, persistErr, map[string]string{
			"detected_preset": detectedPreset,
		})
		s.emitLogWithFields("warning", "ffmpeg 能力缓存写入失败，将在运行时回退默认策略", logFields{
			Component: componentFFmpeg,
			Stage:     "detect_profile",
			Action:    "persist_cache",
			Error:     persistErr.Error(),
		})
		return
	}

	s.logStepDone(componentFFmpeg, "detect_profile", "persist_cache", string(s.currentSource()), 0, persistStarted, map[string]string{
		"detected_preset": detectedPreset,
		"encoders":        strings.Join(encoders, ","),
	})
	s.logStepDone(componentFFmpeg, "detect_profile", "probe_encoders", string(s.currentSource()), 0, started, map[string]string{
		"detected_preset": detectedPreset,
		"encoders":        strings.Join(encoders, ","),
	})
	s.emitLogWithFields("info", fmt.Sprintf("ffmpeg 编码能力探测完成，自动模式将优先使用 %s", detectedPreset), logFields{
		Component: componentFFmpeg,
		Stage:     "detect_profile",
		Action:    "ready",
		Meta: map[string]string{
			"detected_preset": detectedPreset,
			"encoders":        strings.Join(encoders, ","),
		},
	})
}
