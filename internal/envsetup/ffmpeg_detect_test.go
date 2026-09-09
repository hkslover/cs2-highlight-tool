package envsetup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/config"
)

func TestEnsureFFmpeg_DetectProfilesCached(t *testing.T) {
	exeDir := t.TempDir()
	ffmpegDir := filepath.Join(exeDir, "ffmpeg", "bin")
	ffmpegExe := filepath.Join(ffmpegDir, "ffmpeg.exe")
	if err := os.MkdirAll(ffmpegDir, 0o755); err != nil {
		t.Fatalf("mkdir ffmpeg dir: %v", err)
	}
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write ffmpeg exe: %v", err)
	}

	svc := New(exeDir, "1.0.0")
	svc.Startup(nil)

	old := ffmpegDetectCommandContext
	ffmpegDetectCommandContext = fakeDetectCommandContext("hevc_nvenc,h264_nvenc,libx264")
	t.Cleanup(func() {
		ffmpegDetectCommandContext = old
	})

	if err := svc.ensureFFmpeg(); err != nil {
		t.Fatalf("ensureFFmpeg: %v", err)
	}
	waitFFmpegDetectDone(t, svc, 5*time.Second)

	state := svc.GetStartupState()
	ffmpegStep := findStepByID(state.Steps, componentFFmpeg)
	if ffmpegStep == nil || ffmpegStep.Status != statusReady {
		t.Fatalf("ffmpeg step not ready: %+v", ffmpegStep)
	}

	cfg, err := config.LoadOrCreate(filepath.Join(exeDir, "config.json"), exeDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.FFmpegDetectedPreset != "n1" {
		t.Fatalf("detected preset = %q, want n1", cfg.FFmpegDetectedPreset)
	}
	if !containsString(cfg.FFmpegDetectedEncoders, "hevc_nvenc") {
		t.Fatalf("expected hevc_nvenc in detected encoders: %+v", cfg.FFmpegDetectedEncoders)
	}

	logs := svc.logsSnapshot()
	if !hasStructuredLog(logs, componentFFmpeg, "detect_profile", "probe_encoders") {
		t.Fatalf("missing detect_profile/probe_encoders log")
	}
	if !hasStructuredLog(logs, componentFFmpeg, "detect_profile", "persist_cache") {
		t.Fatalf("missing detect_profile/persist_cache log")
	}
}

func TestEnsureFFmpeg_DetectFailureDoesNotFailComponent(t *testing.T) {
	exeDir := t.TempDir()
	ffmpegDir := filepath.Join(exeDir, "ffmpeg", "bin")
	ffmpegExe := filepath.Join(ffmpegDir, "ffmpeg.exe")
	if err := os.MkdirAll(ffmpegDir, 0o755); err != nil {
		t.Fatalf("mkdir ffmpeg dir: %v", err)
	}
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write ffmpeg exe: %v", err)
	}

	svc := New(exeDir, "1.0.0")
	svc.Startup(nil)

	old := ffmpegDetectCommandContext
	ffmpegDetectCommandContext = fakeDetectCommandContext("")
	t.Cleanup(func() {
		ffmpegDetectCommandContext = old
	})

	if err := svc.ensureFFmpeg(); err != nil {
		t.Fatalf("ensureFFmpeg: %v", err)
	}
	waitFFmpegDetectDone(t, svc, 5*time.Second)

	state := svc.GetStartupState()
	ffmpegStep := findStepByID(state.Steps, componentFFmpeg)
	if ffmpegStep == nil || ffmpegStep.Status != statusReady {
		t.Fatalf("ffmpeg step not ready: %+v", ffmpegStep)
	}

	cfg, err := config.LoadOrCreate(filepath.Join(exeDir, "config.json"), exeDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.FFmpegDetectedPreset != "c1" {
		t.Fatalf("detected preset = %q, want c1", cfg.FFmpegDetectedPreset)
	}
	if !containsString(cfg.FFmpegDetectedEncoders, "libx264") {
		t.Fatalf("expected libx264 fallback in detected encoders: %+v", cfg.FFmpegDetectedEncoders)
	}
}

func fakeDetectCommandContext(available string) func(context.Context, string, ...string) *exec.Cmd {
	return fakeDetectCommandContextWithOptions(available, 0, nil)
}

func fakeDetectCommandContextWithOptions(available string, delay time.Duration, onCall func()) func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if onCall != nil {
			onCall()
		}
		all := append([]string{"-test.run=TestHelperProcessDetectProfiles", "--", command}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], all...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS_ENVSETUP_FFMPEG_DETECT=1",
			"ENVSETUP_FFMPEG_DETECT_AVAILABLE="+available,
			"ENVSETUP_FFMPEG_DETECT_DELAY_MS="+strconv.FormatInt(delay.Milliseconds(), 10),
		)
		return cmd
	}
}

func TestHelperProcessDetectProfiles(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_ENVSETUP_FFMPEG_DETECT") != "1" {
		return
	}
	if raw := strings.TrimSpace(os.Getenv("ENVSETUP_FFMPEG_DETECT_DELAY_MS")); raw != "" {
		delayMS, err := strconv.Atoi(raw)
		if err == nil && delayMS > 0 {
			time.Sleep(time.Duration(delayMS) * time.Millisecond)
		}
	}

	sep := -1
	for i, arg := range os.Args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep < 0 || sep+2 > len(os.Args) {
		_, _ = fmt.Fprintln(os.Stderr, "missing helper separator")
		os.Exit(2)
	}
	ffArgs := os.Args[sep+2:]
	encoder := ""
	for i := 0; i < len(ffArgs)-1; i++ {
		if ffArgs[i] == "-c:v" {
			encoder = strings.ToLower(strings.TrimSpace(ffArgs[i+1]))
			break
		}
	}
	if encoder == "" {
		_, _ = fmt.Fprintln(os.Stderr, "missing encoder")
		os.Exit(2)
	}
	allowed := make(map[string]struct{})
	for _, token := range strings.Split(os.Getenv("ENVSETUP_FFMPEG_DETECT_AVAILABLE"), ",") {
		normalized := strings.ToLower(strings.TrimSpace(token))
		if normalized == "" {
			continue
		}
		allowed[normalized] = struct{}{}
	}
	if _, ok := allowed[encoder]; ok {
		os.Exit(0)
	}
	_, _ = fmt.Fprintf(os.Stderr, "encoder unavailable: %s\n", encoder)
	os.Exit(2)
}

func TestEnsureFFmpeg_ReturnsBeforeSlowDetectFinishes(t *testing.T) {
	exeDir := t.TempDir()
	ffmpegDir := filepath.Join(exeDir, "ffmpeg", "bin")
	ffmpegExe := filepath.Join(ffmpegDir, "ffmpeg.exe")
	if err := os.MkdirAll(ffmpegDir, 0o755); err != nil {
		t.Fatalf("mkdir ffmpeg dir: %v", err)
	}
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write ffmpeg exe: %v", err)
	}

	svc := New(exeDir, "1.0.0")
	svc.Startup(nil)

	old := ffmpegDetectCommandContext
	ffmpegDetectCommandContext = fakeDetectCommandContextWithOptions("hevc_nvenc,h264_nvenc,libx264", 400*time.Millisecond, nil)
	t.Cleanup(func() {
		ffmpegDetectCommandContext = old
	})

	start := time.Now()
	if err := svc.ensureFFmpeg(); err != nil {
		t.Fatalf("ensureFFmpeg: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("ensureFFmpeg elapsed=%s, expected async return before detect completes", elapsed)
	}

	waitFFmpegDetectDone(t, svc, 10*time.Second)
	cfg, err := config.LoadOrCreate(filepath.Join(exeDir, "config.json"), exeDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.FFmpegDetectedPreset == "" {
		t.Fatal("detected preset should be written after async detect")
	}
}

func TestScheduleFFmpegCapabilityDetection_SingleFlight(t *testing.T) {
	exeDir := t.TempDir()
	ffmpegDir := filepath.Join(exeDir, "ffmpeg", "bin")
	ffmpegExe := filepath.Join(ffmpegDir, "ffmpeg.exe")
	if err := os.MkdirAll(ffmpegDir, 0o755); err != nil {
		t.Fatalf("mkdir ffmpeg dir: %v", err)
	}
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write ffmpeg exe: %v", err)
	}

	svc := New(exeDir, "1.0.0")
	svc.Startup(nil)

	var probeCalls atomic.Int32
	old := ffmpegDetectCommandContext
	ffmpegDetectCommandContext = fakeDetectCommandContextWithOptions("libx264", 500*time.Millisecond, func() {
		probeCalls.Add(1)
	})
	t.Cleanup(func() {
		ffmpegDetectCommandContext = old
	})

	svc.scheduleFFmpegCapabilityDetection(ffmpegExe)
	svc.scheduleFFmpegCapabilityDetection(ffmpegExe)
	svc.scheduleFFmpegCapabilityDetection(ffmpegExe)
	waitFFmpegDetectDone(t, svc, 10*time.Second)

	if got := probeCalls.Load(); got != 7 {
		t.Fatalf("probe command calls = %d, want 7 (single detection run)", got)
	}
}

func TestStopFFmpegCapabilityDetectionCancelsRunningProbe(t *testing.T) {
	exeDir := t.TempDir()
	ffmpegDir := filepath.Join(exeDir, "ffmpeg", "bin")
	ffmpegExe := filepath.Join(ffmpegDir, "ffmpeg.exe")
	if err := os.MkdirAll(ffmpegDir, 0o755); err != nil {
		t.Fatalf("mkdir ffmpeg dir: %v", err)
	}
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write ffmpeg exe: %v", err)
	}

	svc := New(exeDir, "1.0.0")
	svc.Startup(nil)

	probeStarted := make(chan struct{})
	var probeCalls atomic.Int32
	old := ffmpegDetectCommandContext
	ffmpegDetectCommandContext = fakeDetectCommandContextWithOptions("libx264", 5*time.Second, func() {
		if probeCalls.Add(1) == 1 {
			close(probeStarted)
		}
	})
	t.Cleanup(func() {
		ffmpegDetectCommandContext = old
	})

	svc.scheduleFFmpegCapabilityDetection(ffmpegExe)
	select {
	case <-probeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("probe did not start")
	}
	if !svc.HasActiveTasks() {
		t.Fatal("active FFmpeg probe was not registered as a service task")
	}

	start := time.Now()
	svc.stopFFmpegCapabilityDetection()
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("stopFFmpegCapabilityDetection elapsed=%s, want under 1s", elapsed)
	}
	waitFFmpegDetectDone(t, svc, 2*time.Second)
	if svc.HasActiveTasks() {
		t.Fatal("service task remained active after FFmpeg probe stopped")
	}

	cfg, err := config.LoadOrCreate(filepath.Join(exeDir, "config.json"), exeDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.FFmpegDetectedPreset != "" {
		t.Fatalf("detected preset = %q, want empty after canceled detect", cfg.FFmpegDetectedPreset)
	}
}

func waitFFmpegDetectDone(t *testing.T, svc *Service, timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		svc.ffmpegDetectWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("wait ffmpeg detect timeout after %s", timeout)
	}
}

func findStepByID(steps []ComponentStatus, id string) *ComponentStatus {
	for i := range steps {
		if strings.TrimSpace(steps[i].ID) == strings.TrimSpace(id) {
			return &steps[i]
		}
	}
	return nil
}

func containsString(items []string, target string) bool {
	target = strings.TrimSpace(strings.ToLower(target))
	for _, item := range items {
		if strings.TrimSpace(strings.ToLower(item)) == target {
			return true
		}
	}
	return false
}
