package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/ffmpegprofile"
)

var ffmpegCommand = exec.Command
var ffmpegCommandContext = exec.CommandContext

type probedVideoInfo struct {
	Duration           float64
	Width              int
	Height             int
	SampleAspectRatio  string
	DisplayAspectRatio string
}

func (a *App) ProbeClipDuration(videoPath string) (float64, error) {
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

	return probeDurationByFFprobe(ffprobeExe, videoPath)
}

func probeDurationByFFprobe(ffprobeExe string, videoPath string) (float64, error) {
	cmd := ffmpegCommand(
		ffprobeExe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)
	configureNoWindowProcess(cmd)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	raw := strings.TrimSpace(string(out))
	duration, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("parse ffprobe duration failed: %s", raw)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("invalid ffprobe duration: %s", raw)
	}

	return math.Round(duration*1000) / 1000, nil
}

func probeVideoStreamInfo(ffprobeExe, videoPath string) (probedVideoInfo, error) {
	cmd := ffmpegCommand(
		ffprobeExe,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=duration,width,height,sample_aspect_ratio,display_aspect_ratio",
		"-show_entries", "format=duration",
		"-of", "json",
		videoPath,
	)
	configureNoWindowProcess(cmd)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return probedVideoInfo{}, fmt.Errorf("ffprobe video stream failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	var payload struct {
		Streams []struct {
			Duration           json.RawMessage `json:"duration"`
			Width              int             `json:"width"`
			Height             int             `json:"height"`
			SampleAspectRatio  string          `json:"sample_aspect_ratio"`
			DisplayAspectRatio string          `json:"display_aspect_ratio"`
		} `json:"streams"`
		Format struct {
			Duration json.RawMessage `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return probedVideoInfo{}, fmt.Errorf("parse ffprobe video stream failed: %w", err)
	}
	if len(payload.Streams) == 0 {
		return probedVideoInfo{}, fmt.Errorf("ffprobe returned no video stream")
	}

	stream := payload.Streams[0]
	duration, ok := parseFFProbeDurationValue(stream.Duration)
	if !ok {
		duration, ok = parseFFProbeDurationValue(payload.Format.Duration)
	}
	if !ok {
		return probedVideoInfo{}, fmt.Errorf("ffprobe returned invalid video duration")
	}
	if stream.Width <= 0 || stream.Height <= 0 {
		return probedVideoInfo{}, fmt.Errorf("ffprobe returned invalid video resolution: %dx%d", stream.Width, stream.Height)
	}

	return probedVideoInfo{
		Duration:           duration,
		Width:              stream.Width,
		Height:             stream.Height,
		SampleAspectRatio:  strings.TrimSpace(stream.SampleAspectRatio),
		DisplayAspectRatio: strings.TrimSpace(stream.DisplayAspectRatio),
	}, nil
}

func parseFFProbeDurationValue(raw json.RawMessage) (float64, bool) {
	value := strings.TrimSpace(string(raw))
	value = strings.Trim(value, `"`)
	if value == "" || strings.EqualFold(value, "N/A") {
		return 0, false
	}
	duration, err := strconv.ParseFloat(value, 64)
	if err != nil || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return 0, false
	}
	return duration, true
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
	nextFPS := fps
	if nextFPS <= 0 {
		nextFPS = config.DefaultEditFPS
	}
	if nextFPS < config.MinEditFPS {
		nextFPS = config.MinEditFPS
	}
	if nextFPS > config.MaxEditFPS {
		nextFPS = config.MaxEditFPS
	}

	nextQuality := ffmpegprofile.NormalizeEditQuality(quality)
	nextPreset := ffmpegprofile.NormalizeUserPreset(videoPreset)
	caps := ffmpegprofile.CapabilitiesFromEncoders(detectedEncoders)

	return editEncodeSettings{
		FPS:         nextFPS,
		Quality:     nextQuality,
		VideoPreset: nextPreset,
		Caps:        caps,
	}
}

func buildEditRetryProfiles(encode editEncodeSettings) []ffmpegprofile.Profile {
	return ffmpegprofile.BuildRetryChain(encode.VideoPreset, encode.Caps)
}

func withFFmpegProgressArgs(args []string) []string {
	if len(args) == 0 {
		return []string{"-progress", "pipe:1", "-nostats"}
	}
	last := args[len(args)-1]
	rebuilt := make([]string, 0, len(args)+3)
	rebuilt = append(rebuilt, args[:len(args)-1]...)
	rebuilt = append(rebuilt, "-progress", "pipe:1", "-nostats", last)
	return rebuilt
}

func runFFmpegCommandWithProgress(cmd *exec.Cmd, expectedDurationSeconds float64, tracker *composeProgressTracker) ([]byte, error) {
	if tracker == nil {
		return cmd.CombinedOutput()
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var readWG sync.WaitGroup

	readWG.Add(1)
	go func() {
		defer readWG.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			stdoutBuf.WriteString(line)
			stdoutBuf.WriteByte('\n')
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key == "progress" && value == "end" {
				tracker.stageProgress(1)
				continue
			}
			outSeconds, parsed := parseFFmpegProgressSeconds(key, value)
			if !parsed || expectedDurationSeconds <= 0 {
				continue
			}
			tracker.stageProgress(outSeconds / expectedDurationSeconds)
		}
	}()

	readWG.Add(1)
	go func() {
		defer readWG.Done()
		_, _ = io.Copy(&stderrBuf, stderrPipe)
	}()

	waitErr := cmd.Wait()
	readWG.Wait()

	combined := append([]byte{}, stdoutBuf.Bytes()...)
	combined = append(combined, stderrBuf.Bytes()...)
	if waitErr != nil {
		return combined, waitErr
	}
	return combined, nil
}

func parseFFmpegProgressSeconds(key, value string) (float64, bool) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	switch key {
	case "out_time_us", "out_time_ms":
		raw, err := strconv.ParseInt(value, 10, 64)
		if err != nil || raw < 0 {
			return 0, false
		}
		return float64(raw) / 1_000_000, true
	case "out_time":
		parts := strings.Split(value, ":")
		if len(parts) != 3 {
			return 0, false
		}
		hours, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, false
		}
		minutes, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, false
		}
		seconds, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return 0, false
		}
		return hours*3600 + minutes*60 + seconds, true
	default:
		return 0, false
	}
}
