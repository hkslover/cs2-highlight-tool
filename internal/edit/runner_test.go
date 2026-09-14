package edit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cs2-highlight-tool-v2/internal/ffmpegprofile"
)

func TestRunnerCancellationStopsBeforeRetryOrCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	runner := Runner{CommandContext: func(context.Context, string, ...string) *exec.Cmd {
		calls++
		return exec.Command("definitely-not-started")
	}}
	_, err := runner.ComposeSimple(ctx, "ffmpeg", []ResolvedClip{{VideoPath: "clip.mp4", Duration: 1}}, "list.txt", "output.mp4", EncodeSettings{FPS: 60})
	if err == nil || !strings.Contains(err.Error(), "取消") {
		t.Fatalf("error=%v want cancellation", err)
	}
	if calls != 0 {
		t.Fatalf("command factory calls=%d want 0", calls)
	}
}

func TestProgressHelpersPreserveOutputPathAndParseTime(t *testing.T) {
	args := WithProgressArgs([]string{"-i", "clip.mp4", "output.mp4"})
	if args[len(args)-1] != "output.mp4" || !strings.Contains(strings.Join(args, " "), "-progress pipe:1 -nostats") {
		t.Fatalf("progress args=%v", args)
	}
	if got, ok := ParseProgressSeconds("out_time_us", "1500000"); !ok || got != 1.5 {
		t.Fatalf("microsecond progress=%v ok=%v", got, ok)
	}
	if got, ok := ParseProgressSeconds("out_time", "00:00:02.500000"); !ok || got != 2.5 {
		t.Fatalf("clock progress=%v ok=%v", got, ok)
	}
}

func TestRunnerRetriesAfterEncodeFailure(t *testing.T) {
	tempDir := t.TempDir()
	listPath := filepath.Join(tempDir, "concat.txt")
	outputPath := filepath.Join(tempDir, "output.mp4")
	callCount := 0
	attemptArgs := make([][]string, 0, 2)
	progress := make([]ProgressEvent, 0, 8)
	runner := Runner{
		CommandContext: func(ctx context.Context, _ string, args ...string) *exec.Cmd {
			callCount++
			attemptArgs = append(attemptArgs, append([]string(nil), args...))
			mode := "success"
			if callCount == 1 {
				mode = "fail"
			}
			return newEditRunnerHelperCommand(ctx, mode, args)
		},
		Progress: func(event ProgressEvent) {
			progress = append(progress, event)
		},
	}
	encode := EncodeSettings{
		FPS:         60,
		Quality:     ffmpegprofile.EditQualityHigh,
		VideoPreset: ffmpegprofile.UserPresetAuto,
		Caps:        ffmpegprofile.CapabilitiesFromEncoders([]string{"h264_nvenc", "libx264"}),
	}

	if _, err := runner.ComposeSimple(context.Background(), "ffmpeg", []ResolvedClip{{VideoPath: filepath.Join(tempDir, "clip.mp4"), Duration: 1}}, listPath, outputPath, encode); err != nil {
		t.Fatalf("ComposeSimple: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("encoder attempts=%d, want first failure then second success", callCount)
	}
	if !strings.Contains(strings.Join(attemptArgs[0], " "), "h264_nvenc") || !strings.Contains(strings.Join(attemptArgs[1], " "), "libx264") {
		t.Fatalf("retry profiles=%v, want h264_nvenc then libx264", attemptArgs)
	}
	var ratios []float64
	for _, event := range progress {
		if event.Kind == ProgressStageProgress {
			ratios = append(ratios, event.Ratio)
		}
	}
	if len(ratios) < 3 || ratios[0] != 0.8 || ratios[1] != 0.1 || ratios[len(ratios)-1] != 1 {
		t.Fatalf("retry progress ratios=%v, want first-attempt 0.8 then fallback 0.1..1", ratios)
	}
}

func newEditRunnerHelperCommand(ctx context.Context, mode string, args []string) *exec.Cmd {
	all := append([]string{"-test.run=TestHelperProcessEditRunner", "--"}, args...)
	cmd := exec.CommandContext(ctx, os.Args[0], all...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS_EDIT_RUNNER=1", "EDIT_RUNNER_MODE="+mode)
	return cmd
}

func TestHelperProcessEditRunner(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_EDIT_RUNNER") != "1" {
		return
	}
	mode := os.Getenv("EDIT_RUNNER_MODE")
	if mode == "fail" {
		_, _ = fmt.Fprintln(os.Stdout, "out_time_us=800000")
		_, _ = fmt.Fprintln(os.Stdout, "progress=continue")
		_, _ = fmt.Fprintln(os.Stderr, "simulated first encoder failure")
		os.Exit(2)
	}
	_, _ = fmt.Fprintln(os.Stdout, "out_time_us=100000")
	_, _ = fmt.Fprintln(os.Stdout, "progress=continue")
	if len(os.Args) > 1 {
		output := os.Args[len(os.Args)-1]
		if output != "-" {
			_ = os.WriteFile(output, []byte("ok"), 0o644)
		}
	}
	_, _ = fmt.Fprintln(os.Stdout, "out_time_us=1000000")
	_, _ = fmt.Fprintln(os.Stdout, "progress=end")
	os.Exit(0)
}
