package app

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

// newEditSafetyTestApp builds an App whose fallback workspace root is exeDir
// and whose FFmpeg/FFprobe stubs exist, so ConcatEditClips reaches the
// command seam without a real toolchain.
func newEditSafetyTestApp(t *testing.T) *App {
	t.Helper()
	exeDir := t.TempDir()
	writeEditSafetyToolchain(t, exeDir)
	return &App{exeDir: exeDir}
}

func writeEditSafetyToolchain(t *testing.T, dataDir string) {
	t.Helper()
	binDir := filepath.Join(dataDir, "ffmpeg", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ffmpeg.exe", "ffprobe.exe"} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("stub"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func writeEditSafetyClips(t *testing.T, dir string) []EditConcatClip {
	t.Helper()
	clips := make([]EditConcatClip, 0, 2)
	for _, name := range []string{"a.mp4", "b.mp4"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		clips = append(clips, EditConcatClip{VideoPath: path, Duration: 2.5})
	}
	return clips
}

func editSafetyOutputDir(app *App) string {
	return filepath.Join(app.dataRoot(), "outputs", "edit")
}

func stubEditFFmpegCommand(t *testing.T, factory func(ctx context.Context, name string, args ...string) *exec.Cmd) {
	t.Helper()
	old := ffmpegCommandContext
	ffmpegCommandContext = factory
	t.Cleanup(func() { ffmpegCommandContext = old })
}

func newEditSafetyHelperCommand(ctx context.Context, mode string, extraEnv []string, args []string) *exec.Cmd {
	all := append([]string{"-test.run=TestHelperProcessEditSafety", "--"}, args...)
	cmd := exec.CommandContext(ctx, os.Args[0], all...)
	cmd.Env = append(append(os.Environ(), "GO_WANT_HELPER_PROCESS_EDIT_SAFETY=1", "EDIT_SAFETY_MODE="+mode), extraEnv...)
	return cmd
}

// fakeEditSafetyGateCommand blocks the helper process until the release file
// appears instead of relying on wall-clock sleeps. extraEnv lets a probe test
// request valid ffprobe stdout and skip the "write the last argument" step.
func fakeEditSafetyGateCommand(startedPath, releasePath string, extraEnv ...string) func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		env := append([]string{
			"EDIT_SAFETY_STARTED=" + startedPath,
			"EDIT_SAFETY_RELEASE=" + releasePath,
		}, extraEnv...)
		return newEditSafetyHelperCommand(ctx, "gate", env, args)
	}
}

func fakeEditSafetyNoOutputCommand() func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		return newEditSafetyHelperCommand(ctx, "no_output", nil, args)
	}
}

// fakeEditSafetyDetachedGateCommand mirrors the gate fake but deliberately
// does not bind the child to the task context: the test cancels the workspace
// while the encoder still runs and only then lets it exit successfully, which
// is the "FFmpeg finished after cancellation" interleaving.
func fakeEditSafetyDetachedGateCommand(startedPath, releasePath string) func(context.Context, string, ...string) *exec.Cmd {
	return func(_ context.Context, _ string, args ...string) *exec.Cmd {
		all := append([]string{"-test.run=TestHelperProcessEditSafety", "--"}, args...)
		cmd := exec.Command(os.Args[0], all...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS_EDIT_SAFETY=1",
			"EDIT_SAFETY_MODE=gate",
			"EDIT_SAFETY_STARTED="+startedPath,
			"EDIT_SAFETY_RELEASE="+releasePath,
		)
		return cmd
	}
}

func waitForPath(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func assertEditOutputDirEmpty(t *testing.T, app *App) {
	t.Helper()
	entries, err := os.ReadDir(editSafetyOutputDir(app))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("edit output directory is not empty: %v", names)
	}
}

func TestConcatEditClipsRejectsSecondTaskWhileFirstIsRunning(t *testing.T) {
	app := newEditSafetyTestApp(t)
	clips := writeEditSafetyClips(t, app.exeDir)
	started := filepath.Join(app.exeDir, "ffmpeg.started")
	release := filepath.Join(app.exeDir, "ffmpeg.release")
	stubEditFFmpegCommand(t, fakeEditSafetyGateCommand(started, release))

	type result struct {
		path string
		err  error
	}
	firstDone := make(chan result, 1)
	go func() {
		path, err := app.ConcatEditClips(EditConcatRequest{Clips: clips})
		firstDone <- result{path: path, err: err}
	}()
	waitForPath(t, started, 10*time.Second)

	// B5: the backend must reject a second active compose instead of letting
	// both tasks pick the same output and progress stream.
	if _, err := app.ConcatEditClips(EditConcatRequest{Clips: clips}); err == nil || !strings.Contains(err.Error(), "正在进行") {
		t.Fatalf("second compose error = %v, want busy rejection", err)
	}
	// A blocked compose owns the workspace, so directory clearing must refuse.
	if _, err := app.ClearOutputsDirectory(); err == nil {
		t.Fatal("directory clear must be rejected while a compose holds the workspace")
	}
	if activity := app.GetWorkActivity(); !activity.StorageBusy {
		t.Fatalf("compose task should keep storage busy: %+v", activity)
	}

	if err := os.WriteFile(release, []byte("go"), 0o644); err != nil {
		t.Fatal(err)
	}
	var first result
	select {
	case first = <-firstDone:
	case <-time.After(10 * time.Second):
		t.Fatal("first compose did not finish after release")
	}
	if first.err != nil {
		t.Fatalf("first compose failed: %v", first.err)
	}
	if _, err := os.Stat(first.path); err != nil {
		t.Fatalf("first output missing: %v", err)
	}

	// The single-compose slot must be reusable once the first task returned.
	if _, err := app.ConcatEditClips(EditConcatRequest{Clips: clips}); err != nil {
		t.Fatalf("compose after completion: %v", err)
	}
}

func TestConcatEditClipsSuccessiveTasksKeepDistinctOutputs(t *testing.T) {
	app := newEditSafetyTestApp(t)
	clips := writeEditSafetyClips(t, app.exeDir)
	var counter int32
	stubEditFFmpegCommand(t, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		n := atomic.AddInt32(&counter, 1)
		return newEditSafetyHelperCommand(ctx, "echo", []string{"EDIT_SAFETY_OUTPUT=" + strconv.Itoa(int(n))}, args)
	})

	first, err := app.ConcatEditClips(EditConcatRequest{Clips: clips})
	if err != nil {
		t.Fatalf("first compose: %v", err)
	}
	second, err := app.ConcatEditClips(EditConcatRequest{Clips: clips})
	if err != nil {
		t.Fatalf("second compose: %v", err)
	}
	if first == second {
		t.Fatalf("successive composes reused output path %q", first)
	}

	firstContent, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read first output: %v", err)
	}
	if string(firstContent) != "1" {
		t.Fatalf("first artifact content = %q, want %q (a later task overwrote it)", firstContent, "1")
	}
	secondContent, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("read second output: %v", err)
	}
	if string(secondContent) != "2" {
		t.Fatalf("second artifact content = %q, want %q", secondContent, "2")
	}

	if items := app.getProduceHistorySnapshot().Items; len(items) != 2 {
		t.Fatalf("history items = %d, want 2", len(items))
	}
}

func TestReserveEditOutputPathNeverReusesExistingFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	suffixes := []string{"aaaaaaaaaaaa", "bbbbbbbbbbbb"}
	old := editOutputNameSuffix
	editOutputNameSuffix = func() string {
		if len(suffixes) == 0 {
			return "cccccccccccc"
		}
		next := suffixes[0]
		suffixes = suffixes[1:]
		return next
	}
	t.Cleanup(func() { editOutputNameSuffix = old })

	existing := filepath.Join(dir, "edit_"+now.Format("20060102_150405")+"_aaaaaaaaaaaa.mp4")
	if err := os.WriteFile(existing, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, reservedInfo, err := reserveEditOutputPath(dir, now)
	if err != nil {
		t.Fatalf("reserveEditOutputPath: %v", err)
	}
	if got == existing {
		t.Fatalf("reservation reused the existing path %q", existing)
	}
	if content, readErr := os.ReadFile(existing); readErr != nil || string(content) != "previous" {
		t.Fatalf("existing video changed: %q, %v", content, readErr)
	}
	info, statErr := os.Lstat(got)
	if statErr != nil {
		t.Fatalf("reserved placeholder missing: %v", statErr)
	}
	if reservedInfo == nil || !os.SameFile(reservedInfo, info) {
		t.Fatalf("reservation identity does not match the reserved file: %#v vs %#v", reservedInfo, info)
	}
}

func TestEditComposeTaskCommitRefusesCanceledTask(t *testing.T) {
	dir := t.TempDir()
	task := &editComposeTask{workspaceRoot: dir}
	if err := task.prepare(dir); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	task.ctx = ctx
	task.cancel = cancel
	if err := os.WriteFile(task.tempOutput, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	cancel()

	if err := task.commit(); err == nil || !strings.Contains(err.Error(), "取消") {
		t.Fatalf("commit error = %v, want cancellation", err)
	}
	if task.committed {
		t.Fatal("canceled commit marked the task committed")
	}
	finalOutput := task.finalOutput
	task.cleanup()
	if _, err := os.Stat(finalOutput); !os.IsNotExist(err) {
		t.Fatalf("canceled task left its placeholder: %v", err)
	}
}

func TestEditComposeTaskCleanupPreservesReplacedRegularFile(t *testing.T) {
	dir := t.TempDir()
	task := &editComposeTask{workspaceRoot: dir}
	if err := task.prepare(dir); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	tempDir := task.tempDir
	if err := os.Remove(task.finalOutput); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(task.finalOutput, []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}

	task.cleanup()
	if content, err := os.ReadFile(task.finalOutput); err != nil || string(content) != "foreign" {
		t.Fatalf("cleanup removed a replaced regular file: %q, %v", content, err)
	}
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("cleanup left the task temp directory: %v", err)
	}
}

func TestEditComposeTaskCommitRefusesReplacedRegularFile(t *testing.T) {
	dir := t.TempDir()
	task := &editComposeTask{workspaceRoot: dir}
	if err := task.prepare(dir); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	tempDir := task.tempDir
	if err := os.Remove(task.finalOutput); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(task.finalOutput, []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(task.tempOutput, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := task.commit(); err == nil || !strings.Contains(err.Error(), "已被替换") {
		t.Fatalf("commit error = %v, want replaced-path rejection", err)
	}
	if task.committed {
		t.Fatal("failed commit marked the task committed")
	}
	if content, err := os.ReadFile(task.finalOutput); err != nil || string(content) != "foreign" {
		t.Fatalf("commit replaced a foreign file: %q, %v", content, err)
	}
	task.cleanup()
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("cleanup left the task temp directory: %v", err)
	}
	if content, err := os.ReadFile(task.finalOutput); err != nil || string(content) != "foreign" {
		t.Fatalf("cleanup removed the replaced regular file: %q, %v", content, err)
	}
}

func TestEditComposeTaskCommitRefusesReplacedSymlink(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(dir, "foreign.mp4")
	if err := os.WriteFile(foreign, []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := &editComposeTask{workspaceRoot: dir}
	if err := task.prepare(dir); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := os.Remove(task.finalOutput); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, task.finalOutput); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}
	if err := os.WriteFile(task.tempOutput, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := task.commit(); err == nil || !strings.Contains(err.Error(), "已被替换") {
		t.Fatalf("commit error = %v, want replaced-path rejection", err)
	}
	if task.committed {
		t.Fatal("failed commit marked the task committed")
	}
	if resolved, err := os.Readlink(task.finalOutput); err != nil || resolved != foreign {
		t.Fatalf("commit replaced the symlink: %q, %v", resolved, err)
	}
	task.cleanup()
	if resolved, err := os.Readlink(task.finalOutput); err != nil || resolved != foreign {
		t.Fatalf("cleanup removed the replaced symlink: %q, %v", resolved, err)
	}
	if content, err := os.ReadFile(foreign); err != nil || string(content) != "foreign" {
		t.Fatalf("symlink target changed: %q, %v", content, err)
	}
}

func TestConcatEditClipsFailureLeavesNoArtifactOrHistory(t *testing.T) {
	app := newEditSafetyTestApp(t)
	clips := writeEditSafetyClips(t, app.exeDir)
	stubEditFFmpegCommand(t, fakeFFmpegCommandFailContext)

	if _, err := app.ConcatEditClips(EditConcatRequest{Clips: clips}); err == nil {
		t.Fatal("expected compose failure")
	}
	if items := app.getProduceHistorySnapshot().Items; len(items) != 0 {
		t.Fatalf("failed compose recorded history: %+v", items)
	}
	assertEditOutputDirEmpty(t, app)

	// The failed attempt must release the slot, so a retry reaches FFmpeg
	// again instead of being rejected as busy.
	if _, err := app.ConcatEditClips(EditConcatRequest{Clips: clips}); err == nil {
		t.Fatal("retry after failure should fail the same way, not succeed or report busy")
	}
}

func TestEditComposeTaskCommitFailureDoesNotPublishOrDeleteForeignPath(t *testing.T) {
	dir := t.TempDir()
	task := &editComposeTask{workspaceRoot: dir}
	if err := task.prepare(dir); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	// Replace this task's own placeholder with a directory: the identity
	// check must refuse to publish, and cleanup must leave the directory.
	if err := os.Remove(task.finalOutput); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(task.finalOutput, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(task.tempOutput, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := task.commit(); err == nil {
		t.Fatal("commit onto a directory should fail")
	}
	if task.committed {
		t.Fatal("failed commit marked the task as committed")
	}
	task.cleanup()
	if _, err := os.Stat(task.tempOutput); !os.IsNotExist(err) {
		t.Fatalf("failed commit left the intermediate video: %v", err)
	}
	if info, err := os.Stat(task.finalOutput); err != nil || !info.IsDir() {
		t.Fatalf("cleanup removed a non-owned path: info=%v err=%v", info, err)
	}
}

func TestConcatEditClipsMissingIntermediateIsNotPublished(t *testing.T) {
	app := newEditSafetyAppWithoutCommandOutput(t)
	clips := writeEditSafetyClips(t, app.exeDir)

	_, err := app.ConcatEditClips(EditConcatRequest{Clips: clips})
	if err == nil || !strings.Contains(err.Error(), "output video not created") {
		t.Fatalf("compose error = %v, want missing-intermediate failure", err)
	}
	if items := app.getProduceHistorySnapshot().Items; len(items) != 0 {
		t.Fatalf("unverified intermediate recorded history: %+v", items)
	}
	assertEditOutputDirEmpty(t, app)
}

func newEditSafetyAppWithoutCommandOutput(t *testing.T) *App {
	t.Helper()
	app := newEditSafetyTestApp(t)
	stubEditFFmpegCommand(t, fakeEditSafetyNoOutputCommand())
	return app
}

func TestConcatEditClipsTransitionPathPublishesOnce(t *testing.T) {
	app := newEditSafetyTestApp(t)
	clips := writeEditSafetyClips(t, app.exeDir)
	probeJSON := `{"streams":[{"codec_type":"video","duration":"2.500","width":1920,"height":1080,"sample_aspect_ratio":"1:1","display_aspect_ratio":"16:9"}],"format":{"duration":"2.500"}}`
	stubEditFFmpegCommand(t, func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if strings.Contains(filepath.Base(name), "ffprobe") {
			return newEditSafetyHelperCommand(ctx, "", []string{
				"EDIT_SAFETY_STDOUT=" + probeJSON,
				"EDIT_SAFETY_SKIP_OUTPUT=1",
			}, args)
		}
		return newEditSafetyHelperCommand(ctx, "echo", []string{"EDIT_SAFETY_OUTPUT=transition"}, args)
	})

	out, err := app.ConcatEditClips(EditConcatRequest{
		Clips:       clips,
		Transitions: []EditConcatTransition{{Type: "fade", Duration: 0.3, AfterIndex: 0}},
	})
	if err != nil {
		t.Fatalf("transition compose: %v", err)
	}
	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read transition output: %v", err)
	}
	if string(content) != "transition" {
		t.Fatalf("transition artifact content = %q, want %q", content, "transition")
	}
	items := app.getProduceHistorySnapshot().Items
	if len(items) != 1 || items[0].VideoPath != out {
		t.Fatalf("transition history = %+v, want one entry for %q", items, out)
	}
	// Only the published artifact may remain: the private task dir, the
	// concat list and any attempt temp file are gone.
	entries, err := os.ReadDir(editSafetyOutputDir(app))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(out) {
		t.Fatalf("edit output directory = %v, want only %s", entryNames(entries), filepath.Base(out))
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestProbeClipDurationBlocksDirectoryClearAndReleasesUse(t *testing.T) {
	app := newEditSafetyTestApp(t)
	clip := writeEditSafetyClips(t, app.exeDir)[0].VideoPath
	started := filepath.Join(app.exeDir, "probe.started")
	release := filepath.Join(app.exeDir, "probe.release")
	stubEditFFmpegCommand(t, fakeEditSafetyGateCommand(started, release,
		"EDIT_SAFETY_STDOUT=2.500000", "EDIT_SAFETY_SKIP_OUTPUT=1"))

	probeDone := make(chan error, 1)
	go func() {
		_, err := app.ProbeClipDuration(clip)
		probeDone <- err
	}()
	waitForPath(t, started, 10*time.Second)

	if _, err := app.ClearOutputsDirectory(); err == nil {
		t.Fatal("directory clear must be rejected while a probe holds workspace use")
	}
	if err := os.WriteFile(release, []byte("go"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-probeDone:
		if err != nil {
			t.Fatalf("probe after release: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("probe did not finish after release")
	}

	releaseUse, err := app.beginManagedFileUse()
	if err != nil {
		t.Fatalf("probe leaked its workspace use: %v", err)
	}
	releaseUse()
}

func TestProbeClipDurationTimeoutIsClassifiedAsProbeFailure(t *testing.T) {
	app := newEditSafetyTestApp(t)
	clip := writeEditSafetyClips(t, app.exeDir)[0].VideoPath
	started := filepath.Join(app.exeDir, "probe.started")
	release := filepath.Join(app.exeDir, "probe.release") // never created
	var calls int32
	stubEditFFmpegCommand(t, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		atomic.AddInt32(&calls, 1)
		return newEditSafetyHelperCommand(ctx, "gate", []string{
			"EDIT_SAFETY_STARTED=" + started,
			"EDIT_SAFETY_RELEASE=" + release,
		}, args)
	})

	oldTimeout := editProbeTimeout
	editProbeTimeout = 200 * time.Millisecond
	t.Cleanup(func() { editProbeTimeout = oldTimeout })

	startedAt := time.Now()
	_, err := app.ProbeClipDuration(clip)
	elapsed := time.Since(startedAt)
	if err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("probe error = %v, want timeout classification", err)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("probe timeout took %s, want the bounded probe timeout", elapsed)
	}
	// The command was launched and then killed by the timeout; the factory is
	// called synchronously, so this does not depend on child-process timing.
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("timed-out probe started %d commands, want 1", got)
	}

	releaseUse, useErr := app.beginManagedFileUse()
	if useErr != nil {
		t.Fatalf("timed-out probe leaked workspace use: %v", useErr)
	}
	releaseUse()
}

func TestProbeHelpersDoNotStartCommandWithCanceledContext(t *testing.T) {
	var calls int32
	stubEditFFmpegCommand(t, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		atomic.AddInt32(&calls, 1)
		return newEditSafetyHelperCommand(ctx, "no_output", nil, args)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := probeDurationByFFprobe(ctx, "ffprobe", "clip.mp4"); err == nil {
		t.Fatal("canceled duration probe should fail")
	}
	if _, err := probeVideoStreamInfo(ctx, "ffprobe", "clip.mp4"); err == nil {
		t.Fatal("canceled stream probe should fail")
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("canceled probes started %d commands, want 0", got)
	}
}

func TestConcatEditClipsWorkspaceCloseCancelsRunningFFmpeg(t *testing.T) {
	dataDir := t.TempDir()
	app := newWorkspaceLifecycleTestApp(t, dataDir)
	writeEditSafetyToolchain(t, dataDir)
	clips := writeEditSafetyClips(t, dataDir)
	started := filepath.Join(dataDir, "ffmpeg.started")
	release := filepath.Join(dataDir, "ffmpeg.release") // never created
	stubEditFFmpegCommand(t, fakeEditSafetyGateCommand(started, release))

	errCh := make(chan error, 1)
	go func() {
		_, err := app.ConcatEditClips(EditConcatRequest{Clips: clips})
		errCh <- err
	}()
	waitForPath(t, started, 10*time.Second)

	session := app.workspaceSnapshot().session
	if session == nil {
		t.Fatal("test app has no workspace session")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.close(stopCtx); err != nil {
		t.Fatalf("session.close did not wait for the compose task: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("compose succeeded after workspace close")
		}
		if !strings.Contains(err.Error(), "取消") {
			t.Fatalf("compose error = %v, want cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("compose did not stop after workspace close")
	}
	if items := app.getProduceHistorySnapshot().Items; len(items) != 0 {
		t.Fatalf("canceled compose recorded history: %+v", items)
	}
	assertEditOutputDirEmpty(t, app)
}

func TestConcatEditClipsCancelAfterFFmpegSuccessDoesNotPublish(t *testing.T) {
	probeJSON := `{"streams":[{"codec_type":"video","duration":"2.500","width":1920,"height":1080,"sample_aspect_ratio":"1:1","display_aspect_ratio":"16:9"}],"format":{"duration":"2.500"}}`
	for _, withTransitions := range []bool{false, true} {
		name := "hard_cut"
		if withTransitions {
			name = "transitions"
		}
		t.Run(name, func(t *testing.T) {
			dataDir := t.TempDir()
			app := newWorkspaceLifecycleTestApp(t, dataDir)
			writeEditSafetyToolchain(t, dataDir)
			clips := writeEditSafetyClips(t, dataDir)
			started := filepath.Join(dataDir, "ffmpeg.started")
			release := filepath.Join(dataDir, "ffmpeg.release")
			gate := fakeEditSafetyDetachedGateCommand(started, release)
			stubEditFFmpegCommand(t, func(ctx context.Context, command string, args ...string) *exec.Cmd {
				if strings.Contains(filepath.Base(command), "ffprobe") {
					return newEditSafetyHelperCommand(ctx, "", []string{
						"EDIT_SAFETY_STDOUT=" + probeJSON,
						"EDIT_SAFETY_SKIP_OUTPUT=1",
					}, args)
				}
				return gate(ctx, command, args...)
			})

			request := EditConcatRequest{Clips: clips}
			if withTransitions {
				request.Transitions = []EditConcatTransition{{Type: "fade", Duration: 0.3, AfterIndex: 0}}
			}

			errCh := make(chan error, 1)
			go func() {
				_, err := app.ConcatEditClips(request)
				errCh <- err
			}()
			waitForPath(t, started, 10*time.Second)

			session := app.workspaceSnapshot().session
			if session == nil {
				t.Fatal("test app has no workspace session")
			}
			closed := make(chan error, 1)
			go func() {
				stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				closed <- session.close(stopCtx)
			}()
			select {
			case <-session.ctx.Done():
			case <-time.After(5 * time.Second):
				t.Fatal("workspace context was not canceled")
			}

			// FFmpeg now exits successfully after the workspace was canceled.
			if err := os.WriteFile(release, []byte("go"), 0o644); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-errCh:
				if err == nil {
					t.Fatal("compose published after the workspace was canceled")
				}
				if !strings.Contains(err.Error(), "取消") {
					t.Fatalf("compose error = %v, want cancellation", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("compose did not stop")
			}
			if err := <-closed; err != nil {
				t.Fatalf("session.close: %v", err)
			}
			if items := app.getProduceHistorySnapshot().Items; len(items) != 0 {
				t.Fatalf("canceled compose recorded history: %+v", items)
			}
			assertEditOutputDirEmpty(t, app)
		})
	}
}

func TestConcatEditClipsCancelDoesNotRetryNextEncoderProfile(t *testing.T) {
	dataDir := t.TempDir()
	app := newWorkspaceLifecycleTestApp(t, dataDir)
	writeEditSafetyToolchain(t, dataDir)
	clips := writeEditSafetyClips(t, dataDir)

	if _, err := app.updateConfig(func(cfg *config.Config) error {
		cfg.FFmpegDetectedEncoders = []string{"h264_nvenc", "libx264"}
		cfg.VideoPreset = config.DefaultVideoPreset
		return nil
	}); err != nil {
		t.Fatalf("seed detected encoders: %v", err)
	}
	_, _, encode := app.resolveEditOutputPaths()
	if profiles := buildEditRetryProfiles(encode); len(profiles) < 2 {
		t.Fatalf("test setup needs a multi-profile retry chain, got %+v", profiles)
	}

	started := filepath.Join(dataDir, "ffmpeg.started")
	release := filepath.Join(dataDir, "ffmpeg.release") // never created
	var calls int32
	stubEditFFmpegCommand(t, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		atomic.AddInt32(&calls, 1)
		return newEditSafetyHelperCommand(ctx, "gate", []string{
			"EDIT_SAFETY_STARTED=" + started,
			"EDIT_SAFETY_RELEASE=" + release,
		}, args)
	})

	errCh := make(chan error, 1)
	go func() {
		_, err := app.ConcatEditClips(EditConcatRequest{Clips: clips})
		errCh <- err
	}()
	waitForPath(t, started, 10*time.Second)

	session := app.workspaceSnapshot().session
	if session == nil {
		t.Fatal("test app has no workspace session")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.close(stopCtx); err != nil {
		t.Fatalf("session.close did not wait for the compose task: %v", err)
	}
	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "取消") {
			t.Fatalf("canceled compose error = %v, want cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("compose did not stop after workspace close")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("canceled compose started %d ffmpeg attempts, want 1 (no encoder retry)", got)
	}
}

func TestHelperProcessEditSafety(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_EDIT_SAFETY") != "1" {
		return
	}
	mode := os.Getenv("EDIT_SAFETY_MODE")
	switch mode {
	case "gate":
		if started := os.Getenv("EDIT_SAFETY_STARTED"); started != "" {
			_ = os.WriteFile(started, []byte("started"), 0o644)
		}
		release := os.Getenv("EDIT_SAFETY_RELEASE")
		deadline := time.Now().Add(60 * time.Second)
		for release != "" && time.Now().Before(deadline) {
			if _, err := os.Stat(release); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	case "no_output":
		os.Exit(0)
	}
	if stdout := os.Getenv("EDIT_SAFETY_STDOUT"); stdout != "" {
		_, _ = fmt.Fprintln(os.Stdout, stdout)
	}
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	output := os.Args[len(os.Args)-1]
	if output == "-" || os.Getenv("EDIT_SAFETY_SKIP_OUTPUT") == "1" {
		os.Exit(0)
	}
	payload := os.Getenv("EDIT_SAFETY_OUTPUT")
	if payload == "" {
		payload = "ok"
	}
	if err := os.WriteFile(output, []byte(payload), 0o644); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
