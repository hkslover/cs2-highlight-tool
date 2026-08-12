package app

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/ffmpegprofile"
)

func TestNormalizeEditTransitions_ExplicitPlacement(t *testing.T) {
	transitions, err := normalizeEditTransitions(3, []EditConcatTransition{{
		Type:       "fade",
		Duration:   0.5,
		AfterIndex: 0,
	}})
	if err != nil {
		t.Fatalf("normalizeEditTransitions: %v", err)
	}
	if len(transitions) != 1 {
		t.Fatalf("transition size=%d want 1", len(transitions))
	}
	transition, ok := transitions[0]
	if !ok {
		t.Fatalf("missing transition at gap index 0")
	}
	if transition.Type != "fade" || transition.Duration != 0.5 {
		t.Fatalf("unexpected transition: %+v", transition)
	}
}

func TestNormalizeEditTransitions_LegacySequential(t *testing.T) {
	transitions, err := normalizeEditTransitions(3, []EditConcatTransition{
		{Type: "fade", Duration: 0.3},
		{Type: "fade", Duration: 1},
	})
	if err != nil {
		t.Fatalf("normalizeEditTransitions: %v", err)
	}
	if len(transitions) != 2 {
		t.Fatalf("transition size=%d want 2", len(transitions))
	}
	if transitions[0].AfterIndex != 0 || transitions[1].AfterIndex != 1 {
		t.Fatalf("unexpected sequential transition mapping: %+v", transitions)
	}
}

func TestBuildTransitionFilterGraph_FadeUsesAlignedOffsetAndResolution(t *testing.T) {
	plan, err := buildTransitionFilterGraph([]resolvedEditClip{
		{VideoPath: "a.mp4", Duration: 3.203, Width: 1920, Height: 1080},
		{VideoPath: "b.mp4", Duration: 2.801, Width: 1280, Height: 720},
	}, map[int]EditConcatTransition{
		0: {Type: "fade", Duration: 0.301},
	}, 60)
	if err != nil {
		t.Fatalf("buildTransitionFilterGraph: %v", err)
	}
	if !strings.Contains(plan.Filter, "xfade=transition=fade:duration=0.300000:offset=2.900000[v]") {
		t.Fatalf("filter should contain aligned xfade offset, got: %s", plan.Filter)
	}
	if !strings.Contains(plan.Filter, "acrossfade=d=0.300000:c1=tri:c2=tri[a]") {
		t.Fatalf("filter should contain acrossfade, got: %s", plan.Filter)
	}
	if strings.Count(plan.Filter, "scale=1920:1080") != 2 {
		t.Fatalf("every input should be scaled to first resolution, got: %s", plan.Filter)
	}
	if !strings.Contains(plan.Filter, "trim=duration=3.200000") || !strings.Contains(plan.Filter, "trim=duration=2.800000") {
		t.Fatalf("filter should use frame-aligned durations, got: %s", plan.Filter)
	}
	if math.Abs(plan.TotalDuration-5.7) > 1e-9 {
		t.Fatalf("total duration=%f want 5.7", plan.TotalDuration)
	}
}

func TestBuildTransitionFilterGraph_PreservesTargetSampleAspectRatio(t *testing.T) {
	plan, err := buildTransitionFilterGraph([]resolvedEditClip{
		{Duration: 3, Width: 1440, Height: 1080, SampleAspectRatio: "4:3", DisplayAspectRatio: "16:9"},
		{Duration: 2, Width: 1280, Height: 960, SampleAspectRatio: "1:1", DisplayAspectRatio: "4:3"},
	}, map[int]EditConcatTransition{
		0: {Type: "fade", Duration: 0.5},
	}, 60)
	if err != nil {
		t.Fatalf("buildTransitionFilterGraph: %v", err)
	}
	if strings.Count(plan.Filter, "setsar=4/3") != 2 {
		t.Fatalf("every input should use the first clip SAR, got: %s", plan.Filter)
	}
	if strings.Contains(plan.Filter, "setsar=1,") || strings.Contains(plan.Filter, "setsar=1/1,") {
		t.Fatalf("transition graph must not force square pixels for stretched 4:3 clips, got: %s", plan.Filter)
	}
}

func TestBuildTransitionFilterGraph_MixesFadeAndHardCut(t *testing.T) {
	plan, err := buildTransitionFilterGraph([]resolvedEditClip{
		{Duration: 3, Width: 1920, Height: 1080},
		{Duration: 2, Width: 1920, Height: 1080},
		{Duration: 4, Width: 1920, Height: 1080},
	}, map[int]EditConcatTransition{
		0: {Type: "fade", Duration: 0.5},
	}, 60)
	if err != nil {
		t.Fatalf("buildTransitionFilterGraph: %v", err)
	}
	if !strings.Contains(plan.Filter, "xfade=transition=fade:duration=0.500000:offset=2.500000[vx0]") {
		t.Fatalf("missing non-final xfade, got: %s", plan.Filter)
	}
	if !strings.Contains(plan.Filter, "[vx0][ax0][v2][a2]concat=n=2:v=1:a=1[v][a]") {
		t.Fatalf("missing final hard-cut concat, got: %s", plan.Filter)
	}
	if math.Abs(plan.TotalDuration-8.5) > 1e-9 {
		t.Fatalf("total duration=%f want 8.5", plan.TotalDuration)
	}
}

func TestBuildTransitionFilterGraph_AllHardCuts(t *testing.T) {
	plan, err := buildTransitionFilterGraph([]resolvedEditClip{
		{Duration: 3, Width: 1920, Height: 1080},
		{Duration: 2, Width: 1920, Height: 1080},
		{Duration: 4, Width: 1920, Height: 1080},
	}, nil, 60)
	if err != nil {
		t.Fatalf("buildTransitionFilterGraph: %v", err)
	}
	if strings.Count(plan.Filter, "concat=n=2:v=1:a=1") != 2 {
		t.Fatalf("hard-cut graph should contain two concat filters, got: %s", plan.Filter)
	}
	if math.Abs(plan.TotalDuration-9) > 1e-9 {
		t.Fatalf("total duration=%f want 9", plan.TotalDuration)
	}
}

func TestEditComposeStageCount_TransitionIsSingleStage(t *testing.T) {
	if got := editComposeStageCount(4, true); got != 1 {
		t.Fatalf("transition stage count=%d want 1", got)
	}
}

func TestBuildTransitionFilterGraph_RejectsInvalidDurations(t *testing.T) {
	tests := []struct {
		name  string
		clips []resolvedEditClip
		trans map[int]EditConcatTransition
		want  string
	}{
		{
			name: "fade exceeds left",
			clips: []resolvedEditClip{
				{Duration: 1, Width: 1920, Height: 1080},
				{Duration: 2, Width: 1920, Height: 1080},
			},
			trans: map[int]EditConcatTransition{0: {Type: "fade", Duration: 1}},
			want:  "exceeds clip durations",
		},
		{
			name: "fade exceeds right",
			clips: []resolvedEditClip{
				{Duration: 2, Width: 1920, Height: 1080},
				{Duration: 0.5, Width: 1920, Height: 1080},
			},
			trans: map[int]EditConcatTransition{0: {Type: "fade", Duration: 0.5}},
			want:  "exceeds clip durations",
		},
		{
			name: "clip too short",
			clips: []resolvedEditClip{
				{Duration: 0.01, Width: 1920, Height: 1080},
				{Duration: 2, Width: 1920, Height: 1080},
			},
			want: "too short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildTransitionFilterGraph(tt.clips, tt.trans, 60)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v want substring %q", err, tt.want)
			}
		})
	}
}

func TestProbeVideoStreamInfo_StreamDurationPriorityAndFormatFallback(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    probedVideoInfo
		wantErr bool
	}{
		{
			name:    "stream duration has priority",
			payload: `{"streams":[{"duration":"2.345","width":1440,"height":1080,"sample_aspect_ratio":"4:3","display_aspect_ratio":"16:9"}],"format":{"duration":"9.000"}}`,
			want:    probedVideoInfo{Duration: 2.345, Width: 1440, Height: 1080, SampleAspectRatio: "4:3", DisplayAspectRatio: "16:9"},
		},
		{
			name:    "format duration fallback",
			payload: `{"streams":[{"duration":"N/A","width":1280,"height":720}],"format":{"duration":"4.500"}}`,
			want:    probedVideoInfo{Duration: 4.5, Width: 1280, Height: 720},
		},
		{
			name:    "invalid duration",
			payload: `{"streams":[{"duration":"N/A","width":1280,"height":720}],"format":{"duration":"N/A"}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := ffmpegCommand
			ffmpegCommand = fakeFFProbeCommandJSON
			t.Cleanup(func() { ffmpegCommand = old })
			t.Setenv("EDIT_FFPROBE_JSON", tt.payload)

			got, err := probeVideoStreamInfo("ffprobe", "clip.mp4")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected probe error")
				}
				return
			}
			if err != nil {
				t.Fatalf("probeVideoStreamInfo: %v", err)
			}
			if got != tt.want {
				t.Fatalf("probe result=%+v want %+v", got, tt.want)
			}
		})
	}
}

func TestResolveEditClips_TransitionPathAlwaysProbes(t *testing.T) {
	exeDir := t.TempDir()
	ffprobeDir := filepath.Join(exeDir, "ffmpeg", "bin")
	if err := os.MkdirAll(ffprobeDir, 0755); err != nil {
		t.Fatalf("create ffprobe dir: %v", err)
	}
	ffprobePath := filepath.Join(ffprobeDir, "ffprobe.exe")
	if err := os.WriteFile(ffprobePath, []byte("stub"), 0755); err != nil {
		t.Fatalf("write ffprobe stub: %v", err)
	}
	clipPath := filepath.Join(exeDir, "clip.mp4")
	if err := os.WriteFile(clipPath, []byte("clip"), 0644); err != nil {
		t.Fatalf("write clip: %v", err)
	}

	old := ffmpegCommand
	ffmpegCommand = fakeFFProbeCommandJSON
	t.Cleanup(func() { ffmpegCommand = old })
	t.Setenv("EDIT_FFPROBE_JSON", `{"streams":[{"duration":"4.250","width":1600,"height":900,"sample_aspect_ratio":"1:1","display_aspect_ratio":"16:9"}],"format":{"duration":"8.000"}}`)

	app := &App{exeDir: exeDir}
	got, err := app.resolveEditClips([]EditConcatClip{{VideoPath: clipPath, Duration: 99}}, true)
	if err != nil {
		t.Fatalf("resolveEditClips: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("resolved clips=%d want 1", len(got))
	}
	if got[0].Duration != 4.25 || got[0].Width != 1600 || got[0].Height != 900 || got[0].SampleAspectRatio != "1:1" || got[0].DisplayAspectRatio != "16:9" {
		t.Fatalf("resolved clip=%+v; probe result should override request duration", got[0])
	}
}

func TestEditClipSampleAspectRatio(t *testing.T) {
	tests := []struct {
		name string
		clip resolvedEditClip
		want string
	}{
		{
			name: "sample aspect ratio takes precedence",
			clip: resolvedEditClip{Width: 1440, Height: 1080, SampleAspectRatio: "4:3", DisplayAspectRatio: "4:3"},
			want: "4/3",
		},
		{
			name: "display aspect ratio derives sample aspect ratio",
			clip: resolvedEditClip{Width: 1440, Height: 1080, DisplayAspectRatio: "16:9"},
			want: "4/3",
		},
		{
			name: "invalid metadata falls back to square pixels",
			clip: resolvedEditClip{Width: 1440, Height: 1080, SampleAspectRatio: "N/A", DisplayAspectRatio: "N/A"},
			want: "1/1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := editClipSampleAspectRatio(tt.clip); got != tt.want {
				t.Fatalf("editClipSampleAspectRatio()=%q want %q", got, tt.want)
			}
		})
	}
}

func fakeFFProbeCommandJSON(_ string, _ ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessEditFFProbe", "--")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS_EDIT_FFPROBE=1")
	return cmd
}

func TestHelperProcessEditFFProbe(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_EDIT_FFPROBE") != "1" {
		return
	}
	_, _ = fmt.Fprint(os.Stdout, os.Getenv("EDIT_FFPROBE_JSON"))
	os.Exit(0)
}

func TestConcatEditClips_AddsEditedHistoryEntry(t *testing.T) {
	old := ffmpegCommand
	ffmpegCommand = fakeFFmpegCommandSuccess
	t.Cleanup(func() {
		ffmpegCommand = old
	})

	exeDir := t.TempDir()
	ffmpegDir := filepath.Join(exeDir, "ffmpeg", "bin")
	if err := os.MkdirAll(ffmpegDir, 0755); err != nil {
		t.Fatalf("create ffmpeg dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ffmpegDir, "ffmpeg.exe"), []byte("stub"), 0755); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}

	clipA := filepath.Join(exeDir, "a.mp4")
	clipB := filepath.Join(exeDir, "b.mp4")
	if err := os.WriteFile(clipA, []byte("a"), 0644); err != nil {
		t.Fatalf("write clipA: %v", err)
	}
	if err := os.WriteFile(clipB, []byte("b"), 0644); err != nil {
		t.Fatalf("write clipB: %v", err)
	}

	app := &App{exeDir: exeDir}
	outPath, err := app.ConcatEditClips(EditConcatRequest{
		Clips: []EditConcatClip{
			{VideoPath: clipA, Duration: 3.2},
			{VideoPath: clipB, Duration: 2.8},
		},
	})
	if err != nil {
		t.Fatalf("ConcatEditClips: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("output should exist: %v", err)
	}

	snapshot := app.getProduceHistorySnapshot()
	if len(snapshot.Items) != 1 {
		t.Fatalf("history items=%d want 1", len(snapshot.Items))
	}
	item := snapshot.Items[0]
	if item.HistoryType != produceHistoryTypeEdited {
		t.Fatalf("history type=%q want %q", item.HistoryType, produceHistoryTypeEdited)
	}
	if item.SourceLabel != "edit_timeline" {
		t.Fatalf("source label=%q want %q", item.SourceLabel, "edit_timeline")
	}
	if item.VideoPath != outPath {
		t.Fatalf("video path=%q want %q", item.VideoPath, outPath)
	}
}

func TestResolveEditEncodeSettings_QualityMapping(t *testing.T) {
	tests := []struct {
		name     string
		fps      int
		quality  string
		preset   string
		encoders []string
		wantFPS  int
		wantQ    string
		wantP    string
	}{
		{name: "standard", fps: 120, quality: "standard", preset: "n1", wantFPS: 120, wantQ: "standard", wantP: "n1"},
		{name: "high", fps: 60, quality: "high", preset: "a1", wantFPS: 60, wantQ: "high", wantP: "a1"},
		{name: "ultra", fps: 90, quality: "ultra", preset: "i1", wantFPS: 90, wantQ: "ultra", wantP: "i1"},
		{name: "invalid quality fallback", fps: 90, quality: "bad", preset: "n1", wantFPS: 90, wantQ: "high", wantP: "n1"},
		{name: "invalid preset fallback", fps: 90, quality: "high", preset: "bad", wantFPS: 90, wantQ: "high", wantP: "auto"},
		{name: "fps min clamp", fps: 10, quality: "high", preset: "n1", wantFPS: config.MinEditFPS, wantQ: "high", wantP: "n1"},
		{name: "fps max clamp", fps: 1000, quality: "high", preset: "n1", wantFPS: config.MaxEditFPS, wantQ: "high", wantP: "n1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveEditEncodeSettings(tt.fps, tt.quality, tt.preset, tt.encoders)
			if got.FPS != tt.wantFPS || got.Quality != tt.wantQ || got.VideoPreset != tt.wantP {
				t.Fatalf("resolveEditEncodeSettings()=%+v want fps=%d quality=%s preset=%s", got, tt.wantFPS, tt.wantQ, tt.wantP)
			}
		})
	}
}

func TestResolveEditOutputPaths_UsesSavedEditSettings(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	if _, err := app.SaveClipSettings(ClipSettings{
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		EditFPS:           144,
		EditQuality:       "ultra",
	}); err != nil {
		t.Fatalf("SaveClipSettings: %v", err)
	}

	_, _, encode := app.resolveEditOutputPaths()
	if encode.FPS != 144 {
		t.Fatalf("encode fps=%d want 144", encode.FPS)
	}
	if encode.Quality != ffmpegprofile.EditQualityUltra {
		t.Fatalf("encode quality=%q want ultra", encode.Quality)
	}
	if encode.VideoPreset != ffmpegprofile.UserPresetAuto {
		t.Fatalf("encode video preset=%q want auto", encode.VideoPreset)
	}
}

func TestBuildEditRetryProfiles_ManualPresetFallbackToH264ThenCPU(t *testing.T) {
	encode := editEncodeSettings{
		FPS:         60,
		Quality:     ffmpegprofile.EditQualityHigh,
		VideoPreset: ffmpegprofile.UserPresetN1,
		Caps:        ffmpegprofile.CapabilitiesFromEncoders([]string{"h264_nvenc", "libx264"}),
	}
	profiles := buildEditRetryProfiles(encode)
	if len(profiles) != 3 {
		t.Fatalf("retry profiles len=%d want 3", len(profiles))
	}
	if profiles[0].ID != "n1" || profiles[1].ID != "n1_h264" || profiles[2].ID != "c1" {
		t.Fatalf("unexpected retry profiles: %+v", profiles)
	}
}

func TestParseFFmpegProgressSeconds(t *testing.T) {
	if got, ok := parseFFmpegProgressSeconds("out_time_us", "1500000"); !ok || got != 1.5 {
		t.Fatalf("out_time_us parse failed: ok=%v got=%f", ok, got)
	}
	if got, ok := parseFFmpegProgressSeconds("out_time", "00:00:02.500000"); !ok || got != 2.5 {
		t.Fatalf("out_time parse failed: ok=%v got=%f", ok, got)
	}
	if _, ok := parseFFmpegProgressSeconds("out_time_us", "bad"); ok {
		t.Fatal("invalid out_time_us should return ok=false")
	}
}

func TestRunFFmpegCommandWithProgress_SuccessWithProgressOutput(t *testing.T) {
	tracker := newComposeProgressTracker(nil, 1)
	payloads := make([]composeProgressPayload, 0)
	tracker.emitHook = func(next composeProgressPayload) {
		payloads = append(payloads, next)
	}
	tracker.stageStart("test")

	outputPath := filepath.Join(t.TempDir(), "out.mp4")
	cmd := fakeEditProgressFFmpegCommand("success_with_progress", outputPath)
	out, err := runFFmpegCommandWithProgress(cmd, 3, tracker)
	if err != nil {
		t.Fatalf("runFFmpegCommandWithProgress error: %v output=%s", err, string(out))
	}
	tracker.stageDone()

	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("output file should exist: %v", err)
	}
	if len(payloads) == 0 {
		t.Fatal("expected progress payloads")
	}
	last := payloads[len(payloads)-1]
	if !last.Active {
		t.Fatalf("stageDone payload should be active=true, got %+v", last)
	}
	if last.Percent < 100 {
		t.Fatalf("expected percent >= 100 after stageDone, got %f", last.Percent)
	}
}

func TestRunFFmpegCommandWithProgress_SuccessWithoutProgressOutput(t *testing.T) {
	tracker := newComposeProgressTracker(nil, 1)
	payloads := make([]composeProgressPayload, 0)
	tracker.emitHook = func(next composeProgressPayload) {
		payloads = append(payloads, next)
	}
	tracker.stageStart("test")

	outputPath := filepath.Join(t.TempDir(), "out.mp4")
	cmd := fakeEditProgressFFmpegCommand("success_no_progress", outputPath)
	out, err := runFFmpegCommandWithProgress(cmd, 3, tracker)
	if err != nil {
		t.Fatalf("runFFmpegCommandWithProgress error: %v output=%s", err, string(out))
	}
	tracker.stageDone()

	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("output file should exist: %v", err)
	}
	if len(payloads) == 0 {
		t.Fatal("expected payload emitted at stage start/done")
	}
}

func TestRunFFmpegCommandWithProgress_Failure(t *testing.T) {
	tracker := newComposeProgressTracker(nil, 1)
	tracker.stageStart("test")

	outputPath := filepath.Join(t.TempDir(), "out.mp4")
	cmd := fakeEditProgressFFmpegCommand("fail", outputPath)
	out, err := runFFmpegCommandWithProgress(cmd, 3, tracker)
	if err == nil {
		t.Fatalf("expected error, output=%s", string(out))
	}
	if !strings.Contains(string(out), "simulated edit ffmpeg failure") {
		t.Fatalf("unexpected output: %s", string(out))
	}
}

func fakeEditProgressFFmpegCommand(mode string, outputPath string) *exec.Cmd {
	all := []string{"-test.run=TestHelperProcessEditFFmpegProgress", "--", "ffmpeg", outputPath}
	cmd := exec.Command(os.Args[0], all...)
	cmd.Env = append(
		os.Environ(),
		"GO_WANT_HELPER_PROCESS_EDIT_FFMPEG_PROGRESS=1",
		"EDIT_FFMPEG_PROGRESS_MODE="+mode,
	)
	return cmd
}

func TestHelperProcessEditFFmpegProgress(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_EDIT_FFMPEG_PROGRESS") != "1" {
		return
	}
	mode := strings.TrimSpace(os.Getenv("EDIT_FFMPEG_PROGRESS_MODE"))
	if mode == "fail" {
		_, _ = fmt.Fprintln(os.Stderr, "simulated edit ffmpeg failure")
		os.Exit(2)
	}
	if mode == "success_with_progress" {
		_, _ = fmt.Fprintln(os.Stdout, "out_time_us=1000000")
		_, _ = fmt.Fprintln(os.Stdout, "progress=continue")
		time.Sleep(10 * time.Millisecond)
		_, _ = fmt.Fprintln(os.Stdout, "out_time_us=2500000")
		_, _ = fmt.Fprintln(os.Stdout, "progress=continue")
		time.Sleep(10 * time.Millisecond)
		_, _ = fmt.Fprintln(os.Stdout, "out_time_us=3000000")
		_, _ = fmt.Fprintln(os.Stdout, "progress=end")
	}
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	output := os.Args[len(os.Args)-1]
	if output != "-" {
		if err := os.WriteFile(output, []byte("ok"), 0o644); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(2)
		}
	}
	os.Exit(0)
}
