package producemerge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// ---- test helpers ----

func fakeFFmpegCommandSuccess(command string, args ...string) *exec.Cmd {
	all := append([]string{"-test.run=TestHelperProcessFFmpeg", "--", command}, args...)
	cmd := exec.Command(os.Args[0], all...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS_FFMPEG=1", "FFMPEG_HELPER_MODE=success")
	return cmd
}

func fakeFFmpegCommandFail(command string, args ...string) *exec.Cmd {
	all := append([]string{"-test.run=TestHelperProcessFFmpeg", "--", command}, args...)
	cmd := exec.Command(os.Args[0], all...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS_FFMPEG=1", "FFMPEG_HELPER_MODE=fail")
	return cmd
}

func fakeFFmpegCommandSuccessContext(_ context.Context, command string, args ...string) *exec.Cmd {
	return fakeFFmpegCommandSuccess(command, args...)
}

func fakeFFmpegCommandFailContext(_ context.Context, command string, args ...string) *exec.Cmd {
	return fakeFFmpegCommandFail(command, args...)
}

// fakeFFmpegCommandBlockingContext simulates a hung ffmpeg: the helper writes
// the (partial) output file and then sleeps until the context kills it. Probe
// invocations (output "-") still exit immediately so WaitForTakeFilesReady
// can pass.
func fakeFFmpegCommandBlockingContext(ctx context.Context, command string, args ...string) *exec.Cmd {
	all := append([]string{"-test.run=TestHelperProcessFFmpeg", "--", command}, args...)
	cmd := exec.CommandContext(ctx, os.Args[0], all...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS_FFMPEG=1", "FFMPEG_HELPER_MODE=block")
	return cmd
}

func fakeFFmpegCommandBlockingProbeContext(ctx context.Context, command string, args ...string) *exec.Cmd {
	all := append([]string{"-test.run=TestHelperProcessFFmpeg", "--", command}, args...)
	cmd := exec.CommandContext(ctx, os.Args[0], all...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS_FFMPEG=1", "FFMPEG_HELPER_MODE=blockall")
	return cmd
}

// fakeFFmpegCommandFailAfterWriteContext simulates an ffmpeg that produces a
// partial output file and then fails, so tests can assert the partial
// ".mux.tmp.mp4" is cleaned up.
func fakeFFmpegCommandFailAfterWriteContext(_ context.Context, command string, args ...string) *exec.Cmd {
	all := append([]string{"-test.run=TestHelperProcessFFmpeg", "--", command}, args...)
	cmd := exec.Command(os.Args[0], all...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS_FFMPEG=1", "FFMPEG_HELPER_MODE=failafterwrite")
	return cmd
}

// fakeFFmpegCommandFailWithDirContext simulates an ffmpeg that leaves an
// unremovable artifact (a non-empty directory) at the tmp output path, so
// tests can assert the cleanup failure is reported instead of swallowed.
func fakeFFmpegCommandFailWithDirContext(_ context.Context, command string, args ...string) *exec.Cmd {
	all := append([]string{"-test.run=TestHelperProcessFFmpeg", "--", command}, args...)
	cmd := exec.Command(os.Args[0], all...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS_FFMPEG=1", "FFMPEG_HELPER_MODE=failwithdir")
	return cmd
}

func TestHelperProcessFFmpeg(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_FFMPEG") != "1" {
		return
	}
	mode := os.Getenv("FFMPEG_HELPER_MODE")
	if mode == "fail" {
		_, _ = fmt.Fprintln(os.Stderr, "simulated ffmpeg failure")
		os.Exit(2)
	}
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	outputPath := os.Args[len(os.Args)-1]
	if mode == "blockall" {
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	}
	if outputPath == "-" {
		os.Exit(0)
	}
	if mode == "block" {
		// Write a partial output so tests can verify cleanup, then hang
		// until the context kills this helper process.
		_ = os.WriteFile(outputPath, []byte("partial"), 0644)
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	}
	if mode == "failafterwrite" {
		_ = os.WriteFile(outputPath, []byte("partial"), 0644)
		_, _ = fmt.Fprintln(os.Stderr, "simulated ffmpeg failure after write")
		os.Exit(2)
	}
	if mode == "failwithdir" {
		// Leave an unremovable artifact: a non-empty directory at the tmp
		// output path.
		_ = os.MkdirAll(filepath.Join(outputPath, "x"), 0755)
		_, _ = fmt.Fprintln(os.Stderr, "simulated ffmpeg failure leaving directory")
		os.Exit(2)
	}
	if err := os.WriteFile(outputPath, []byte("muxed"), 0644); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	os.Exit(0)
}

// ---- NextMergedVideoPath tests ----

func TestNextMergedVideoPath_ReturnsBaseWhenNoConflict(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 2, 12, 34, 56, 0, time.Local)
	path, err := NextMergedVideoPath(dir, now)
	if err != nil {
		t.Fatalf("NextMergedVideoPath: %v", err)
	}
	if !strings.HasSuffix(path, "123456.mp4") {
		t.Fatalf("unexpected path: %q", path)
	}
}

func TestNextMergedVideoPath_AppendsNumericSuffixWhenConflicted(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 2, 12, 34, 56, 0, time.Local)
	if err := os.WriteFile(filepath.Join(dir, "123456.mp4"), []byte("x"), 0644); err != nil {
		t.Fatalf("write base file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "123456_01.mp4"), []byte("x"), 0644); err != nil {
		t.Fatalf("write suffix file: %v", err)
	}
	path, err := NextMergedVideoPath(dir, now)
	if err != nil {
		t.Fatalf("NextMergedVideoPath: %v", err)
	}
	if !strings.HasSuffix(path, "123456_02.mp4") {
		t.Fatalf("unexpected path: %q", path)
	}
}

// ---- MergeTakeVideoAudio tests ----

func TestMergeTakeVideoAudio_SuccessKeepsSourceFiles(t *testing.T) {
	old := FFmpegCommandContext
	FFmpegCommandContext = fakeFFmpegCommandSuccessContext
	t.Cleanup(func() { FFmpegCommandContext = old })

	dir := t.TempDir()
	videoPath := filepath.Join(dir, "take0001.mp4")
	if err := os.WriteFile(videoPath, []byte("video"), 0644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	audioDir := filepath.Join(dir, "take0001")
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		t.Fatalf("mkdir audio dir: %v", err)
	}
	audioPath := filepath.Join(audioDir, "audio.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	ffmpegExe := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}

	finalVideoPath, err := MergeTakeVideoAudio(ffmpegExe, videoPath, audioPath)
	if err != nil {
		t.Fatalf("MergeTakeVideoAudio: %v", err)
	}
	if finalVideoPath == videoPath {
		t.Fatalf("final video path should be renamed, got %q", finalVideoPath)
	}
	matched, matchErr := regexp.MatchString(`^\d{6}(_\d+)?\.mp4$`, filepath.Base(finalVideoPath))
	if matchErr != nil || !matched {
		t.Fatalf("unexpected final filename: %q", finalVideoPath)
	}
	merged, err := os.ReadFile(finalVideoPath)
	if err != nil {
		t.Fatalf("read merged video: %v", err)
	}
	if string(merged) != "muxed" {
		t.Fatalf("unexpected merged payload: %q", string(merged))
	}
	if _, err := os.Stat(videoPath); err != nil {
		t.Fatalf("source video should remain, stat err=%v", err)
	}
	if _, err := os.Stat(audioPath); err != nil {
		t.Fatalf("audio should remain, stat err=%v", err)
	}
	if _, err := os.Stat(audioDir); err != nil {
		t.Fatalf("audio dir should remain, stat err=%v", err)
	}
}

func TestMergeTakeVideoAudio_FailureKeepsSourceFiles(t *testing.T) {
	old := FFmpegCommandContext
	FFmpegCommandContext = fakeFFmpegCommandFailContext
	t.Cleanup(func() { FFmpegCommandContext = old })

	dir := t.TempDir()
	videoPath := filepath.Join(dir, "take0002.mp4")
	if err := os.WriteFile(videoPath, []byte("video"), 0644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	audioDir := filepath.Join(dir, "take0002")
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		t.Fatalf("mkdir audio dir: %v", err)
	}
	audioPath := filepath.Join(audioDir, "audio.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	ffmpegExe := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}

	_, err := MergeTakeVideoAudio(ffmpegExe, videoPath, audioPath)
	if err == nil {
		t.Fatal("expected merge error")
	}
	if _, statErr := os.Stat(videoPath); statErr != nil {
		t.Fatalf("video should remain on failure: %v", statErr)
	}
	if _, statErr := os.Stat(audioPath); statErr != nil {
		t.Fatalf("audio should remain on failure: %v", statErr)
	}
}

func TestMergeTakeVideoAudio_ErrorWhenVideoPathEmpty(t *testing.T) {
	_, err := MergeTakeVideoAudio("ffmpeg.exe", "", "audio.wav")
	if err == nil {
		t.Fatal("expected error for empty video path")
	}
}

func TestMergeTakeVideoAudio_ErrorWhenVideoMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := MergeTakeVideoAudio("ffmpeg.exe", filepath.Join(dir, "missing.mp4"), "audio.wav")
	if err == nil {
		t.Fatal("expected error for missing video")
	}
}

// ---- MergeTakeVideoAudioContext cancel / timeout / cleanup tests ----

func writeMergeTestInputs(t *testing.T, dir string) (videoPath string, audioPath string, ffmpegExe string) {
	t.Helper()
	videoPath = filepath.Join(dir, "take0001.mp4")
	if err := os.WriteFile(videoPath, []byte("video"), 0644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	audioDir := filepath.Join(dir, "take0001")
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		t.Fatalf("mkdir audio dir: %v", err)
	}
	audioPath = filepath.Join(audioDir, "audio.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	ffmpegExe = filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}
	return videoPath, audioPath, ffmpegExe
}

func assertNoMuxTmpFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".mux.tmp.mp4") {
			t.Fatalf("orphan tmp mux file left behind: %s", entry.Name())
		}
	}
}

func TestMergeTakeVideoAudioContext_CancelKillsFFmpegAndCleansTmp(t *testing.T) {
	old := FFmpegCommandContext
	FFmpegCommandContext = fakeFFmpegCommandBlockingContext
	t.Cleanup(func() { FFmpegCommandContext = old })

	dir := t.TempDir()
	videoPath, audioPath, ffmpegExe := writeMergeTestInputs(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	_, err := MergeTakeVideoAudioContext(ctx, ffmpegExe, videoPath, audioPath)
	if err == nil {
		t.Fatal("expected merge error after cancel")
	}
	if !strings.Contains(err.Error(), "已取消") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoMuxTmpFiles(t, dir)
}

func TestMergeTakeVideoAudioContext_TimeoutKillsFFmpegAndCleansTmp(t *testing.T) {
	old := FFmpegCommandContext
	FFmpegCommandContext = fakeFFmpegCommandBlockingContext
	t.Cleanup(func() { FFmpegCommandContext = old })
	oldTimeout := MergeTimeout
	MergeTimeout = 150 * time.Millisecond
	t.Cleanup(func() { MergeTimeout = oldTimeout })

	dir := t.TempDir()
	videoPath, audioPath, ffmpegExe := writeMergeTestInputs(t, dir)

	_, err := MergeTakeVideoAudioContext(context.Background(), ffmpegExe, videoPath, audioPath)
	if err == nil {
		t.Fatal("expected merge timeout error")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoMuxTmpFiles(t, dir)
}

func TestMergeTakeVideoAudioContext_FailureCleansTmp(t *testing.T) {
	old := FFmpegCommandContext
	FFmpegCommandContext = fakeFFmpegCommandFailAfterWriteContext
	t.Cleanup(func() { FFmpegCommandContext = old })

	dir := t.TempDir()
	videoPath, audioPath, ffmpegExe := writeMergeTestInputs(t, dir)

	_, err := MergeTakeVideoAudioContext(context.Background(), ffmpegExe, videoPath, audioPath)
	if err == nil {
		t.Fatal("expected merge failure")
	}
	if !strings.Contains(err.Error(), "合成失败") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoMuxTmpFiles(t, dir)
}

func TestMergeTakeVideoAudioContext_CleanupFailureIsReported(t *testing.T) {
	old := FFmpegCommandContext
	FFmpegCommandContext = fakeFFmpegCommandFailWithDirContext
	t.Cleanup(func() { FFmpegCommandContext = old })

	dir := t.TempDir()
	videoPath, audioPath, ffmpegExe := writeMergeTestInputs(t, dir)

	_, err := MergeTakeVideoAudioContext(context.Background(), ffmpegExe, videoPath, audioPath)
	if err == nil {
		t.Fatal("expected merge failure")
	}
	if !strings.Contains(err.Error(), "合成失败") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "清理临时合成文件失败") {
		t.Fatalf("cleanup failure should be reported, got: %v", err)
	}
}

// ---- WaitForTakeFilesReady tests ----

func TestWaitForTakeFilesReady_SuccessWhenFilesStabilize(t *testing.T) {
	oldCtx := FFmpegCommandContext
	FFmpegCommandContext = fakeFFmpegCommandSuccessContext
	t.Cleanup(func() { FFmpegCommandContext = oldCtx })

	dir := t.TempDir()
	ffmpegExe := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}
	videoPath := filepath.Join(dir, "take0003.mp4")
	audioDir := filepath.Join(dir, "take0003")
	audioPath := filepath.Join(audioDir, "audio.wav")
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		t.Fatalf("mkdir audio dir: %v", err)
	}

	go func() {
		time.Sleep(120 * time.Millisecond)
		_ = os.WriteFile(videoPath, []byte("v1"), 0644)
		_ = os.WriteFile(audioPath, []byte("a1"), 0644)
		time.Sleep(140 * time.Millisecond)
		_ = os.WriteFile(videoPath, []byte("v1-extend"), 0644)
		_ = os.WriteFile(audioPath, []byte("a1-extend"), 0644)
	}()

	err := WaitForTakeFilesReady(context.Background(), ffmpegExe, videoPath, audioPath, 2*time.Second, 80*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForTakeFilesReady: %v", err)
	}
}

func TestWaitForTakeFilesReady_Timeout(t *testing.T) {
	dir := t.TempDir()
	ffmpegExe := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}
	videoPath := filepath.Join(dir, "take0004.mp4")
	audioPath := filepath.Join(dir, "take0004", "audio.wav")
	if err := os.WriteFile(videoPath, []byte("video"), 0644); err != nil {
		t.Fatalf("write video: %v", err)
	}

	err := WaitForTakeFilesReady(context.Background(), ffmpegExe, videoPath, audioPath, 300*time.Millisecond, 60*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitForTakeFilesReady_TimeoutWhenProbeFails(t *testing.T) {
	oldCtx := FFmpegCommandContext
	FFmpegCommandContext = fakeFFmpegCommandFailContext
	t.Cleanup(func() { FFmpegCommandContext = oldCtx })

	dir := t.TempDir()
	ffmpegExe := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}
	videoPath := filepath.Join(dir, "take0005.mp4")
	audioDir := filepath.Join(dir, "take0005")
	audioPath := filepath.Join(audioDir, "audio.wav")
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		t.Fatalf("mkdir audio dir: %v", err)
	}
	if err := os.WriteFile(videoPath, []byte("video"), 0644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	err := WaitForTakeFilesReady(context.Background(), ffmpegExe, videoPath, audioPath, 500*time.Millisecond, 80*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "ffmpeg probe") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitForTakeFilesReady_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := WaitForTakeFilesReady(ctx, "ffmpeg.exe", "video.mp4", "audio.wav", time.Second, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected cancelled error")
	}
	if !strings.Contains(err.Error(), "已取消") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitForTakeFilesReady_CancelKillsBlockedProbe(t *testing.T) {
	oldCtx := FFmpegCommandContext
	FFmpegCommandContext = fakeFFmpegCommandBlockingProbeContext
	t.Cleanup(func() { FFmpegCommandContext = oldCtx })

	dir := t.TempDir()
	videoPath, audioPath, ffmpegExe := writeMergeTestInputs(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	started := time.Now()
	err := WaitForTakeFilesReady(ctx, ffmpegExe, videoPath, audioPath, 5*time.Second, 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "取消") {
		t.Fatalf("expected cancellation error, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("blocked probe was not killed promptly: %v", elapsed)
	}
}
