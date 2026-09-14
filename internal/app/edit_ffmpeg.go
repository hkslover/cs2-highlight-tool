package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"cs2-highlight-tool-v2/internal/config"
	editdomain "cs2-highlight-tool-v2/internal/edit"
	"cs2-highlight-tool-v2/internal/ffmpegprofile"
)

func (a *App) newEditRunner(tracker *composeProgressTracker) editdomain.Runner {
	commandFactory := editdomain.CommandFactory(exec.CommandContext)
	if a != nil && a.editCommandFactoryOverride != nil {
		commandFactory = a.editCommandFactoryOverride
	}
	var progress editdomain.ProgressSink
	if tracker != nil {
		progress = func(event editdomain.ProgressEvent) {
			switch event.Kind {
			case editdomain.ProgressStageStart:
				tracker.stageStart(event.Label)
			case editdomain.ProgressStageProgress:
				tracker.stageProgress(event.Ratio)
			case editdomain.ProgressStageDone:
				tracker.stageDone()
			}
		}
	}
	return editdomain.Runner{
		CommandContext:   commandFactory,
		ConfigureProcess: configureNoWindowProcess,
		ProbeTimeout:     editProbeTimeout,
		ComposeTimeout:   editComposeTimeout,
		Progress:         progress,
	}
}

func (a *App) ProbeClipDuration(videoPath string) (float64, error) {
	// A probe only needs shared file use: it may coexist with other read-only
	// work and does not take the single-compose slot. The workspace context
	// plus a finite timeout bound the command without killing normal work.
	workCtx, release, _, fileErr := a.beginManagedWorkspaceTaskUse()
	if fileErr != nil {
		return 0, fileErr
	}
	defer release()

	videoPath = strings.TrimSpace(videoPath)
	if videoPath == "" {
		return 0, fmt.Errorf("video path is empty")
	}
	if _, err := os.Stat(videoPath); err != nil {
		return 0, fmt.Errorf("video file not found: %s", videoPath)
	}

	ffprobeExe := a.resolveFFprobeExe()
	if ffprobeExe == "" {
		return 0, fmt.Errorf("ffprobe not found")
	}
	if _, err := os.Stat(ffprobeExe); err != nil {
		return 0, fmt.Errorf("ffprobe not found at %s", ffprobeExe)
	}

	probeCtx, cancel := context.WithTimeout(workCtx, editProbeTimeout)
	defer cancel()
	return a.newEditRunner(nil).ProbeDuration(probeCtx, ffprobeExe, videoPath)
}

func probeDurationByFFprobe(ctx context.Context, ffprobeExe string, videoPath string) (float64, error) {
	return probeDurationByFFprobeWithFactory(ctx, ffprobeExe, videoPath, editdomain.CommandFactory(exec.CommandContext))
}

func probeVideoStreamInfo(ctx context.Context, ffprobeExe, videoPath string) (probedVideoInfo, error) {
	return probeVideoStreamInfoWithFactory(ctx, ffprobeExe, videoPath, editdomain.CommandFactory(exec.CommandContext))
}

func probeDurationByFFprobeWithFactory(ctx context.Context, ffprobeExe string, videoPath string, commandFactory editdomain.CommandFactory) (float64, error) {
	return (editdomain.Runner{CommandContext: commandFactory, ConfigureProcess: configureNoWindowProcess, ProbeTimeout: editProbeTimeout}).ProbeDuration(ctx, ffprobeExe, videoPath)
}

func probeVideoStreamInfoWithFactory(ctx context.Context, ffprobeExe, videoPath string, commandFactory editdomain.CommandFactory) (probedVideoInfo, error) {
	return (editdomain.Runner{CommandContext: commandFactory, ConfigureProcess: configureNoWindowProcess, ProbeTimeout: editProbeTimeout}).ProbeVideoStreamInfo(ctx, ffprobeExe, videoPath)
}

func (a *App) resolveEditOutputPaths() (string, string, editEncodeSettings) {
	encode := editEncodeSettings{
		FPS:         config.DefaultEditFPS,
		Quality:     config.DefaultEditQuality,
		VideoPreset: config.DefaultVideoPreset,
		Caps:        ffmpegprofile.CapabilitiesFromEncoders(nil),
	}

	cfg, err := a.loadConfig()
	if err != nil {
		return "", "", encode
	}
	recordOutputDir := config.CleanPath(cfg.RecordOutputDir)
	editDir := filepath.Join(recordOutputDir, "edit")
	ffmpegExe := config.JoinExe(config.CleanPath(cfg.FFmpegDir), "ffmpeg.exe")
	encode = resolveEditEncodeSettings(cfg.EditFPS, cfg.EditQuality, cfg.VideoPreset, cfg.FFmpegDetectedEncoders)
	return editDir, ffmpegExe, encode
}

func (a *App) resolveFFprobeExe() string {
	cfg, err := a.loadConfig()
	if err != nil {
		return ""
	}
	return config.JoinExe(config.CleanPath(cfg.FFmpegDir), "ffprobe.exe")
}

func resolveEditEncodeSettings(fps int, quality string, videoPreset string, detectedEncoders []string) editEncodeSettings {
	return editdomain.NormalizeEncodeSettings(fps, quality, videoPreset, detectedEncoders, editdomain.EncodeLimits{
		DefaultFPS: config.DefaultEditFPS,
		MinFPS:     config.MinEditFPS,
		MaxFPS:     config.MaxEditFPS,
	})
}

func buildEditRetryProfiles(encode editEncodeSettings) []ffmpegprofile.Profile {
	return editdomain.BuildRetryProfiles(encode)
}

func withFFmpegProgressArgs(args []string) []string {
	return editdomain.WithProgressArgs(args)
}

func runFFmpegCommandWithProgress(cmd *exec.Cmd, expectedDurationSeconds float64, tracker *composeProgressTracker) ([]byte, error) {
	var sink editdomain.ProgressSink
	if tracker != nil {
		sink = func(event editdomain.ProgressEvent) {
			if event.Kind == editdomain.ProgressStageProgress {
				tracker.stageProgress(event.Ratio)
			}
		}
	}
	return editdomain.RunCommandWithProgress(cmd, expectedDurationSeconds, sink)
}

func parseFFmpegProgressSeconds(key, value string) (float64, bool) {
	return editdomain.ParseProgressSeconds(key, value)
}
