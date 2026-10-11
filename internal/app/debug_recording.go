package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/ffmpegprofile"
	"cs2-highlight-tool-v2/internal/producemerge"
	"cs2-highlight-tool-v2/internal/producews"
)

// Package-level seams for the manual debug recording workbench. Tests shorten
// the file wait and the finalize window instead of using a real recorder.
var (
	debugRecordingFileReadyTimeout   = 30 * time.Second
	debugRecordingFileStableInterval = 250 * time.Millisecond
	// debugRecordingFinalizeTimeout bounds how long StopDebugGame waits for a
	// recording boundary observation and its merge before it records an explicit
	// failure and leaves the raw files retryable.
	debugRecordingFinalizeTimeout = 90 * time.Second
	debugRecordingMergePollWait   = 100 * time.Millisecond
)

// debugRecordingAdmissionHook runs once after the playback facts were read and
// before the segment slot is reserved. Production leaves it nil; tests use it
// to interleave a Demo play request into the preparation window and prove the
// shared workbench operation lock keeps both operations from passing their own
// checks and then acting on a stale Demo.
var debugRecordingAdmissionHook func()

// debugRecordingTask is one manual recording segment. The store is App-scoped
// rather than session-scoped so a failed merge stays retryable after the game
// is closed; it is cleared when the workspace changes. Every mutable field is
// guarded by App.debugRecordingsMu.
type debugRecordingTask struct {
	id               string
	sessionID        string
	demoPath         string
	takeName         string
	directory        string
	videoPath        string
	audioPath        string
	finalPath        string
	status           string
	lastError        string
	startTick        int
	endTick          int
	startedAtMs      int64
	updatedAtMs      int64
	keepIntermediate bool

	// mergeCancel cancels the current bounded merge; mergeDone closes after the
	// merge goroutine wrote its final status. A retry replaces both. mergeAttempt
	// is the identity of the current merge run so a cancelled run that is still
	// exiting cannot overwrite the state of a newer attempt.
	mergeCancel  context.CancelFunc
	mergeDone    chan struct{}
	mergeAttempt uint64
}

func newDebugRecordingID() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "rec-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "rec-" + strconv.FormatInt(time.Now().UnixMilli(), 36) + "-" + hex.EncodeToString(buf[:])
}

func isActiveDebugRecordingStatus(status string) bool {
	switch status {
	case DebugRecordingStatusStarting, DebugRecordingStatusRecording,
		DebugRecordingStatusStopping, DebugRecordingStatusMerging:
		return true
	default:
		return false
	}
}

// debugRecordingTakePaths returns the expected HLAE output paths for one
// recording directory. HLAE appends `\take` plus a 4-digit take index to the
// configured record name (advancedfx `CAfxStreams::RecordStart` +
// `SuggestTakePath`) and writes the screen stream through
// `{AFX_STREAM_PATH}`, so the video lands in `<recordName>/take0000.mp4` and
// `startmovie` writes `<recordName>/take0000/audio.wav`. Every manual segment
// uses a fresh unique record directory, so the take index is always 0000. The
// rule lives in this one helper because it depends on HLAE behavior and must be
// confirmed on Windows with a real recording; the desktop must not guess that
// the record name itself is the final file name.
func debugRecordingTakePaths(recordingDir string) (videoPath string, audioPath string) {
	takeDir := filepath.Join(recordingDir, "take0000")
	return takeDir + ".mp4", filepath.Join(takeDir, "audio.wav")
}

func (a *App) debugRecordingTaskByID(id string) *debugRecordingTask {
	if a == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	a.debugRecordingsMu.Lock()
	defer a.debugRecordingsMu.Unlock()
	return a.debugRecordings[id]
}

// debugActiveRecordingTask returns the single active segment, if any. The
// workbench allows one segment at a time: a new start is rejected until the
// current segment completed, or its failed merge was retried and finished.
func (a *App) debugActiveRecordingTask() *debugRecordingTask {
	if a == nil {
		return nil
	}
	a.debugRecordingsMu.Lock()
	defer a.debugRecordingsMu.Unlock()
	for _, id := range a.debugRecordingOrder {
		task := a.debugRecordings[id]
		if task != nil && isActiveDebugRecordingStatus(task.status) {
			return task
		}
	}
	return nil
}

func (a *App) activeDebugRecordingForSession(sessionID string) *debugRecordingTask {
	if a == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	a.debugRecordingsMu.Lock()
	defer a.debugRecordingsMu.Unlock()
	for _, id := range a.debugRecordingOrder {
		task := a.debugRecordings[id]
		if task == nil || task.sessionID != sessionID {
			continue
		}
		if isActiveDebugRecordingStatus(task.status) {
			return task
		}
	}
	return nil
}

// debugRecordingBusyError rejects work that must not overlap a manual
// recording or its bounded merge. It is separate from debugBusyError, which
// only covers a live session.
func (a *App) debugRecordingBusyError() error {
	if a.debugActiveRecordingTask() != nil {
		return fmt.Errorf("调试录制或合并正在进行中，请等待完成")
	}
	return nil
}

// debugRecordingsSnapshot projects the store into the public DTO list, the
// single active segment and the newest stamp carried by any task. The stamp
// feeds the snapshot version so a recording-only change still advances
// DebugSessionState.UpdatedAtMs.
func (a *App) debugRecordingsSnapshot() (*DebugRecordingStatus, []DebugRecordingStatus, int64) {
	items := []DebugRecordingStatus{}
	if a == nil {
		return nil, items, 0
	}
	a.debugRecordingsMu.Lock()
	defer a.debugRecordingsMu.Unlock()
	items = make([]DebugRecordingStatus, 0, len(a.debugRecordingOrder))
	var active *DebugRecordingStatus
	var latestMs int64
	for _, id := range a.debugRecordingOrder {
		task := a.debugRecordings[id]
		if task == nil {
			continue
		}
		item := task.statusSnapshot()
		items = append(items, item)
		if item.UpdatedAtMs > latestMs {
			latestMs = item.UpdatedAtMs
		}
		if active == nil && isActiveDebugRecordingStatus(task.status) {
			copyItem := item
			active = &copyItem
		}
	}
	return active, items, latestMs
}

func (t *debugRecordingTask) statusSnapshot() DebugRecordingStatus {
	if t == nil {
		return DebugRecordingStatus{}
	}
	return DebugRecordingStatus{
		RecordingID: t.id,
		DemoPath:    t.demoPath,
		TakeName:    t.takeName,
		Status:      t.status,
		Error:       t.lastError,
		Directory:   t.directory,
		VideoPath:   t.videoPath,
		AudioPath:   t.audioPath,
		FinalPath:   t.finalPath,
		StartTick:   t.startTick,
		EndTick:     t.endTick,
		StartedAtMs: t.startedAtMs,
		UpdatedAtMs: t.updatedAtMs,
	}
}

// resetDebugRecordings drops every result of the previous workspace. A running
// merge is canceled; its goroutine is awaited by the workspace task reservation
// it holds, so this must only be called inside a workspace transition that
// already excluded managed file users.
func (a *App) resetDebugRecordings() {
	if a == nil {
		return
	}
	a.debugRecordingsMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(a.debugRecordings))
	for _, task := range a.debugRecordings {
		if task != nil && task.mergeCancel != nil {
			cancels = append(cancels, task.mergeCancel)
		}
	}
	a.debugRecordings = map[string]*debugRecordingTask{}
	a.debugRecordingOrder = nil
	// Clearing the list is itself a state change; stamp it so the snapshot
	// version does not move backwards when no task remains.
	a.bumpDebugSnapshotFloor()
	a.debugRecordingsMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

// debugRecordingEncode freezes the segment's encode configuration. The launch
// resolution comes from the session (already locked at launch); `auto` is
// resolved with the backend probe cache before the shared recording helper
// builds the same parameters as formal production, including the 4:3 real
// pixel stretch.
type debugRecordingEncoding struct {
	fps          int
	quality      string
	videoPreset  string
	ffmpegParams string
}

func (a *App) debugRecordingEncode(resolution string, req DebugRecordingRequest) (debugRecordingEncoding, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return debugRecordingEncoding{}, err
	}
	draft := normalizeDebugSettingsDraft(DebugSettingsDraft{
		RecordFPS:        req.RecordFPS,
		RecordQuality:    req.RecordQuality,
		VideoPreset:      req.VideoPreset,
		LaunchResolution: resolution,
	})
	resolved := ffmpegprofile.ResolveVideoPreset(
		draft.VideoPreset,
		cfg.FFmpegDetectedPreset,
		cfg.FFmpegDetectedEncoders,
	)
	videoPreset, ffmpegParams, err := clipsjson.BuildRecordEncoding(
		resolved.EffectivePreset,
		draft.RecordQuality,
		draft.LaunchResolution,
	)
	if err != nil {
		return debugRecordingEncoding{}, err
	}
	return debugRecordingEncoding{
		fps:          draft.RecordFPS,
		quality:      draft.RecordQuality,
		videoPreset:  videoPreset,
		ffmpegParams: ffmpegParams,
	}, nil
}

// StartDebugRecording freezes the segment configuration, allocates a unique
// managed directory and asks the plugin's game thread to run the recording
// initialization commands plus the final start. It only returns the current
// status; the plugin's boundary observation (or its receipt fallback) is what
// switches the segment to recording, and the produced files are only trusted
// after the file wait and ffprobe.
func (a *App) StartDebugRecording(req DebugRecordingRequest) (DebugSessionState, error) {
	// Serialize with Demo switching and stop/retry: without one critical
	// section, a stop arriving while the start is still preparing could fail
	// before the plugin recording exists, and the later start would leave a
	// segment nobody can stop any more.
	a.debugWorkbenchOpMu.Lock()
	defer a.debugWorkbenchOpMu.Unlock()

	rt, connID, err := a.debugPublicRuntime()
	if err != nil {
		return a.GetDebugSessionState(), err
	}

	rt.mu.Lock()
	demoPath := strings.TrimSpace(rt.debugDemoPath)
	playing := rt.debugDemoPlaying
	playPending := rt.debugDemoPlayPending
	sessionID := rt.sessionID
	resolution := rt.request.LaunchResolution
	rt.mu.Unlock()

	if playPending {
		return a.GetDebugSessionState(), fmt.Errorf("正在等待 Demo 播放确认，请稍后再开始录制")
	}
	if !playing || demoPath == "" {
		return a.GetDebugSessionState(), fmt.Errorf("请先播放 Demo 并确认播放状态后再开始录制")
	}
	if err := validateDebugPlaybackPath(demoPath); err != nil {
		return a.GetDebugSessionState(), err
	}
	if debugRecordingAdmissionHook != nil {
		debugRecordingAdmissionHook()
	}

	release, dataDir, err := a.beginManagedWorkspaceUse()
	if err != nil {
		return a.GetDebugSessionState(), err
	}
	defer release()

	recordingID := newDebugRecordingID()
	directory := filepath.Join(config.CleanPath(dataDir), "outputs", "debug", sessionID, recordingID)
	videoPath, audioPath := debugRecordingTakePaths(directory)
	takeName := "take0000"

	task := &debugRecordingTask{
		id:               recordingID,
		sessionID:        sessionID,
		demoPath:         demoPath,
		takeName:         takeName,
		directory:        directory,
		videoPath:        videoPath,
		audioPath:        audioPath,
		status:           DebugRecordingStatusStarting,
		startedAtMs:      nowMs(),
		updatedAtMs:      a.nextDebugStamp(),
		keepIntermediate: req.KeepIntermediateFiles,
	}
	// Reserve the single segment slot atomically: two concurrent starts must not
	// both pass the admission check before either registers its task.
	if err := a.reserveDebugRecordingTask(task); err != nil {
		return a.GetDebugSessionState(), err
	}
	a.emitDebugSessionState()

	if err := os.MkdirAll(directory, 0755); err != nil {
		createErr := fmt.Errorf("创建调试录制目录失败: %w", err)
		a.failDebugRecordingIfStatus(task, []string{DebugRecordingStatusStarting}, createErr)
		return a.GetDebugSessionState(), createErr
	}

	encoding, err := a.debugRecordingEncode(resolution, req)
	if err != nil {
		a.failDebugRecordingIfStatus(task, []string{DebugRecordingStatusStarting}, err)
		return a.GetDebugSessionState(), err
	}

	commands := clipsjson.RecordInitCommands(clipsjson.RecordInitOptions{
		RecordFPS:    encoding.fps,
		VideoPreset:  encoding.videoPreset,
		FFmpegParams: encoding.ffmpegParams,
		RecordName:   directory,
		// A repeated start in one game process must create a fresh HLAE settings
		// entry: `mirv_streams settings add ffmpeg` refuses an existing name.
		SettingsName: clipsjson.DebugRecordSettingsName(encoding.videoPreset, encoding.quality, encoding.ffmpegParams),
	})
	commands = append(commands, "mirv_streams record start")

	// A plugin event (disconnect/game exit) can still invalidate the segment
	// while this operation is preparing, and this operation itself never holds
	// the event path. Never send a start for a segment that is no longer
	// waiting for its boundary.
	a.debugRecordingsMu.Lock()
	stillStarting := task.status == DebugRecordingStatusStarting
	a.debugRecordingsMu.Unlock()
	if !stillStarting {
		return a.GetDebugSessionState(), fmt.Errorf("录制准备期间状态已变化，未发送录制命令")
	}

	result, err := a.sendDebugRequestOnRuntime(rt, connID, "debug_record_start", func(requestID string) any {
		return debugRecordStartPayload{
			RequestID:   requestID,
			RecordingID: recordingID,
			DemoPath:    demoPath,
			TakeName:    takeName,
			Commands:    commands,
		}
	})
	if err != nil {
		if a.failDebugRecordingIfStatus(task, []string{DebugRecordingStatusStarting}, err) {
			a.bestEffortStopDebugRecording(rt, recordingID)
		}
		return a.GetDebugSessionState(), err
	}
	if result.Status != DebugCommandStatusExecuted {
		startErr := fmt.Errorf("%s", debugResultError(result.Error, "插件执行录制命令失败"))
		if a.failDebugRecordingIfStatus(task, []string{DebugRecordingStatusStarting}, startErr) {
			a.bestEffortStopDebugRecording(rt, recordingID)
		}
		return a.GetDebugSessionState(), startErr
	}
	// The plugin answers only after the game thread ran the ordered batch, so a
	// still-starting segment may confirm the start from the receipt when the
	// observation message did not arrive. Copying only advances the lifecycle;
	// it never claims that a file exists.
	a.confirmDebugRecordingStartedFromReceipt(task)
	return a.GetDebugSessionState(), nil
}

// bestEffortStopDebugRecording asks the plugin to end a segment whose start
// result was lost. The stop carries the exact recording id, and the plugin
// only ends a matching active segment, so it can never stop a recording that
// the user created through a custom console command. Failures are ignored: the
// segment is already marked failed with its known files kept.
func (a *App) bestEffortStopDebugRecording(rt *debugSessionRuntime, recordingID string) {
	go func() {
		_ = a.sendDebugRecordStop(rt, recordingID)
	}()
}

// reserveDebugRecordingTask atomically installs one segment when no other
// segment is active. It is the admission gate shared by concurrent starts.
func (a *App) reserveDebugRecordingTask(task *debugRecordingTask) error {
	if a == nil || task == nil {
		return fmt.Errorf("录制任务不存在")
	}
	a.debugRecordingsMu.Lock()
	defer a.debugRecordingsMu.Unlock()
	for _, id := range a.debugRecordingOrder {
		existing := a.debugRecordings[id]
		if existing != nil && isActiveDebugRecordingStatus(existing.status) {
			return fmt.Errorf("调试录制或合并正在进行中，请等待完成")
		}
	}
	if a.debugRecordings == nil {
		a.debugRecordings = make(map[string]*debugRecordingTask)
	}
	task.updatedAtMs = a.nextDebugStamp()
	a.debugRecordings[task.id] = task
	a.debugRecordingOrder = append(a.debugRecordingOrder, task.id)
	return nil
}

// StopDebugRecording ends the active segment and returns without waiting for
// the merge. The matching end observation launches the bounded file wait and
// mux task asynchronously, so the API stays responsive.
func (a *App) StopDebugRecording() (DebugSessionState, error) {
	a.debugWorkbenchOpMu.Lock()
	defer a.debugWorkbenchOpMu.Unlock()

	rt, connID, err := a.debugPublicRuntime()
	if err != nil {
		return a.GetDebugSessionState(), err
	}
	task := a.activeDebugRecordingForSession(rt.sessionID)
	if task == nil {
		return a.GetDebugSessionState(), fmt.Errorf("当前没有正在进行的调试录制")
	}

	a.debugRecordingsMu.Lock()
	if task.status != DebugRecordingStatusStarting && task.status != DebugRecordingStatusRecording {
		status := task.status
		a.debugRecordingsMu.Unlock()
		return a.GetDebugSessionState(), fmt.Errorf("当前录制状态为 %s，无法停止", status)
	}
	task.status = DebugRecordingStatusStopping
	task.updatedAtMs = a.nextDebugStamp()
	recordingID := task.id
	a.debugRecordingsMu.Unlock()
	a.emitDebugSessionState()

	result, err := a.sendDebugRequestOnRuntime(rt, connID, "debug_record_stop", func(requestID string) any {
		return debugRecordStopPayload{RequestID: requestID, RecordingID: recordingID}
	})
	if err != nil {
		a.failDebugRecordingIfStatus(task, []string{DebugRecordingStatusStopping}, err)
		return a.GetDebugSessionState(), err
	}
	if result.Status != DebugCommandStatusExecuted {
		stopErr := fmt.Errorf("%s", debugResultError(result.Error, "插件停止录制失败"))
		a.failDebugRecordingIfStatus(task, []string{DebugRecordingStatusStopping}, stopErr)
		return a.GetDebugSessionState(), stopErr
	}
	a.markDebugRecordingEndedFromReceipt(task)
	return a.GetDebugSessionState(), nil
}

// RetryDebugRecordingMerge reuses the recorded source paths of one failed
// segment. It never re-records and never touches another segment's files.
func (a *App) RetryDebugRecordingMerge(recordingID string) (DebugSessionState, error) {
	a.debugWorkbenchOpMu.Lock()
	defer a.debugWorkbenchOpMu.Unlock()

	task := a.debugRecordingTaskByID(recordingID)
	if task == nil {
		return a.GetDebugSessionState(), fmt.Errorf("未找到该调试录制")
	}
	a.debugRecordingsMu.Lock()
	status := task.status
	videoPath := task.videoPath
	audioPath := task.audioPath
	a.debugRecordingsMu.Unlock()
	if status != DebugRecordingStatusFailed {
		return a.GetDebugSessionState(), fmt.Errorf("只有合并失败的录制可以重试")
	}
	if err := debugRecordingSourcesReadable(videoPath, audioPath); err != nil {
		return a.GetDebugSessionState(), err
	}
	if err := a.launchDebugRecordingMerge(task); err != nil {
		return a.GetDebugSessionState(), err
	}
	return a.GetDebugSessionState(), nil
}

// debugRecordingSourcesReadable checks that both raw files exist as non-empty
// regular files. It is a cheap pre-flight for a retry; the real proof stays the
// ffprobe inside WaitForTakeFilesReady.
func debugRecordingSourcesReadable(videoPath string, audioPath string) error {
	for _, path := range []string{videoPath, audioPath} {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("录制源文件不可读，无法重试合并: %s", path)
		}
		if info.IsDir() || info.Size() == 0 {
			return fmt.Errorf("录制源文件不完整，无法重试合并: %s", path)
		}
	}
	return nil
}

// OpenDebugRecordingInFolder locates one segment: the completed video's folder
// when it exists, otherwise the segment directory that keeps the retryable raw
// files. The target must stay inside the managed debug output root.
func (a *App) OpenDebugRecordingInFolder(recordingID string) error {
	task := a.debugRecordingTaskByID(recordingID)
	if task == nil {
		return fmt.Errorf("未找到该调试录制")
	}
	release, dataDir, err := a.beginManagedWorkspaceUse()
	if err != nil {
		return err
	}
	defer release()

	a.debugRecordingsMu.Lock()
	finalPath := strings.TrimSpace(task.finalPath)
	directory := strings.TrimSpace(task.directory)
	a.debugRecordingsMu.Unlock()

	target := directory
	if finalPath != "" {
		if _, statErr := os.Stat(finalPath); statErr == nil {
			target = filepath.Dir(finalPath)
		}
	}
	if target == "" {
		return fmt.Errorf("该调试录制没有可定位的目录")
	}

	root := filepath.Join(config.CleanPath(dataDir), "outputs", "debug")
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("解析录制目录失败: %w", err)
	}
	if !editPathWithin(root, absTarget) {
		return fmt.Errorf("录制目录不在受管控输出目录内")
	}
	if err := ensureManagedRealPathWithin(root, absTarget); err != nil {
		return err
	}
	return openManagedDirectory(absTarget, "录制目录")
}

// launchDebugRecordingMerge reserves the workspace, flips the segment to
// merging and starts the bounded file-wait + mux goroutine. The reservation is
// held until the goroutine exits and mergeDone closes after the final status
// was written. A segment already merging is not started twice, and a second
// active segment (recording or merging) is rejected so only one bounded merge
// can own the workbench at a time.
func (a *App) launchDebugRecordingMerge(task *debugRecordingTask) error {
	if task == nil {
		return fmt.Errorf("录制任务不存在")
	}
	workCtx, release, _, err := a.beginManagedWorkspaceTaskUse()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(workCtx)
	done := make(chan struct{})

	a.debugRecordingsMu.Lock()
	if task.status == DebugRecordingStatusMerging {
		a.debugRecordingsMu.Unlock()
		cancel()
		release()
		return nil
	}
	for _, id := range a.debugRecordingOrder {
		if id == task.id {
			continue
		}
		other := a.debugRecordings[id]
		if other != nil && isActiveDebugRecordingStatus(other.status) {
			a.debugRecordingsMu.Unlock()
			cancel()
			release()
			return fmt.Errorf("已有调试录制或合并正在进行中，请等待完成")
		}
	}
	task.mergeAttempt++
	attempt := task.mergeAttempt
	task.status = DebugRecordingStatusMerging
	task.lastError = ""
	task.updatedAtMs = a.nextDebugStamp()
	task.mergeCancel = cancel
	task.mergeDone = done
	// Snapshot the bounded-wait seams while the launch is serialized, so the
	// goroutine never reads a package-level test/of-production seam after the
	// caller that owns it may have restored it.
	readyTimeout := debugRecordingFileReadyTimeout
	stableInterval := debugRecordingFileStableInterval
	a.debugRecordingsMu.Unlock()

	go func() {
		defer func() {
			cancel()
			release()
			close(done)
		}()
		a.runDebugRecordingMergeTask(ctx, task, attempt, readyTimeout, stableInterval)
	}()
	a.emitDebugSessionStateAsync()
	return nil
}

func (a *App) runDebugRecordingMergeTask(ctx context.Context, task *debugRecordingTask, attempt uint64, readyTimeout, stableInterval time.Duration) {
	a.debugRecordingsMu.Lock()
	videoPath := task.videoPath
	audioPath := task.audioPath
	a.debugRecordingsMu.Unlock()

	ffmpegExe, err := a.debugRecordingFFmpegExe()
	if err == nil {
		err = producemerge.WaitForTakeFilesReady(
			ctx,
			ffmpegExe,
			videoPath,
			audioPath,
			readyTimeout,
			stableInterval,
		)
	}
	var finalPath string
	if err == nil {
		finalPath, err = producemerge.MergeTakeVideoAudioContext(ctx, ffmpegExe, videoPath, audioPath)
	}
	a.completeDebugRecordingMerge(task, attempt, finalPath, err)
}

func (a *App) completeDebugRecordingMerge(task *debugRecordingTask, attempt uint64, finalPath string, mergeErr error) {
	if task == nil {
		return
	}
	a.debugRecordingsMu.Lock()
	// A cancelled run that is still exiting must never overwrite the status or
	// the cancel handle of a newer attempt.
	if task.mergeAttempt != attempt {
		a.debugRecordingsMu.Unlock()
		return
	}
	task.mergeCancel = nil
	task.updatedAtMs = a.nextDebugStamp()
	keepIntermediate := task.keepIntermediate
	videoPath := task.videoPath
	audioPath := task.audioPath
	if mergeErr != nil {
		task.status = DebugRecordingStatusFailed
		task.lastError = mergeErr.Error()
	} else {
		task.status = DebugRecordingStatusCompleted
		task.finalPath = finalPath
		task.lastError = ""
	}
	a.debugRecordingsMu.Unlock()

	if mergeErr == nil && !keepIntermediate {
		a.cleanupDebugRecordingSources(videoPath, audioPath)
	}
	a.emitDebugSessionState()
}

// cleanupDebugRecordingSources removes only the raw files this segment
// registered after a successful merge. The published mux output sits in the
// same segment directory and is never removed, and no other segment directory
// is scanned.
func (a *App) cleanupDebugRecordingSources(videoPath string, audioPath string) {
	for _, path := range []string{videoPath, audioPath} {
		if err := removeFileIfExists(path); err != nil {
			a.logProduceCleanupError(path, err)
		}
	}
	if strings.TrimSpace(audioPath) != "" {
		takeDir := filepath.Dir(audioPath)
		if err := removeEmptyDirUpward(takeDir, filepath.Dir(takeDir)); err != nil {
			a.logProduceCleanupError(takeDir, err)
		}
	}
}

func (a *App) debugRecordingFFmpegExe() (string, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return "", err
	}
	return config.JoinExe(config.CleanPath(cfg.FFmpegDir), "ffmpeg.exe"), nil
}

// cancelDebugRecordingMerge cancels the current bounded merge. It deliberately
// does not write the failure itself: the merge goroutine owns its own final
// status (guarded by its attempt id), so a cancelled run can never race a
// newer retry or leave a stale status behind.
func (a *App) cancelDebugRecordingMerge(task *debugRecordingTask) {
	if task == nil {
		return
	}
	a.debugRecordingsMu.Lock()
	cancel := task.mergeCancel
	a.debugRecordingsMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// failDebugRecordingIfStatus moves one segment to failed only from an allowed
// status. It keeps the known source paths (and never deletes them), so the
// page can offer RetryDebugRecordingMerge once the files are readable. It
// reports whether the transition happened.
func (a *App) failDebugRecordingIfStatus(task *debugRecordingTask, statuses []string, cause error) bool {
	if task == nil || cause == nil {
		return false
	}
	a.debugRecordingsMu.Lock()
	matched := false
	for _, status := range statuses {
		if task.status == status {
			matched = true
			break
		}
	}
	if matched {
		task.status = DebugRecordingStatusFailed
		task.lastError = cause.Error()
		task.updatedAtMs = a.nextDebugStamp()
	}
	a.debugRecordingsMu.Unlock()
	if matched {
		a.emitDebugSessionState()
	}
	return matched
}

// markDebugRecordingStarted switches the segment to recording only from the
// plugin's start boundary observation for this recording id.
func (a *App) markDebugRecordingStarted(task *debugRecordingTask, tick int) {
	if task == nil {
		return
	}
	a.debugRecordingsMu.Lock()
	if task.status != DebugRecordingStatusStarting {
		a.debugRecordingsMu.Unlock()
		return
	}
	task.status = DebugRecordingStatusRecording
	if tick > 0 {
		task.startTick = tick
	}
	task.updatedAtMs = a.nextDebugStamp()
	a.debugRecordingsMu.Unlock()
	a.emitDebugSessionStateAsync()
}

// confirmDebugRecordingStartedFromReceipt advances a still-starting segment
// after a successful start receipt. The plugin sends its receipt after the game
// thread ran the whole batch, so this only mirrors an observation that was
// lost; it never claims that a file exists.
func (a *App) confirmDebugRecordingStartedFromReceipt(task *debugRecordingTask) {
	if task == nil {
		return
	}
	a.debugRecordingsMu.Lock()
	changed := task.status == DebugRecordingStatusStarting
	if changed {
		task.status = DebugRecordingStatusRecording
		task.updatedAtMs = a.nextDebugStamp()
	}
	a.debugRecordingsMu.Unlock()
	if changed {
		a.emitDebugSessionState()
	}
}

// markDebugRecordingEnded records the end boundary and starts the bounded
// merge. A late duplicate (for example a replayed observation for a finished
// segment) is ignored instead of resurrecting the segment.
func (a *App) markDebugRecordingEnded(task *debugRecordingTask, tick int) {
	if task == nil {
		return
	}
	a.debugRecordingsMu.Lock()
	switch task.status {
	case DebugRecordingStatusCompleted, DebugRecordingStatusFailed, DebugRecordingStatusMerging:
		a.debugRecordingsMu.Unlock()
		return
	}
	if tick > 0 {
		task.endTick = tick
	}
	a.debugRecordingsMu.Unlock()
	if err := a.launchDebugRecordingMerge(task); err != nil {
		a.failDebugRecordingIfStatus(task, []string{
			DebugRecordingStatusStarting, DebugRecordingStatusRecording, DebugRecordingStatusStopping,
		}, err)
	}
}

// markDebugRecordingEndedFromReceipt advances a segment whose end observation
// was lost. The stop receipt is emitted by the plugin only after it ran the end
// boundary, and the merge launcher is idempotent.
func (a *App) markDebugRecordingEndedFromReceipt(task *debugRecordingTask) {
	if task == nil {
		return
	}
	a.debugRecordingsMu.Lock()
	status := task.status
	a.debugRecordingsMu.Unlock()
	switch status {
	case DebugRecordingStatusStarting, DebugRecordingStatusRecording, DebugRecordingStatusStopping:
		if err := a.launchDebugRecordingMerge(task); err != nil {
			a.failDebugRecordingIfStatus(task, []string{status}, err)
		}
	}
}

// handleDebugRecordStatus accepts a manual recording boundary only from the
// live plugin connection and the exact session/recording pair. It never enters
// the produce queue ACK or take/history paths.
func (a *App) handleDebugRecordStatus(message producews.DebugIncomingMessage) {
	var payload debugRecordStatusPayload
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		return
	}
	recordingID := strings.TrimSpace(payload.RecordingID)
	sessionID := strings.TrimSpace(payload.SessionID)
	if recordingID == "" || sessionID == "" {
		return
	}
	rt := a.debugRuntimeForAdmission()
	if rt == nil || debugRuntimeDone(rt) {
		return
	}
	if a.produceW == nil || message.ConnID == 0 || a.produceW.GameConnID() != message.ConnID {
		return
	}
	rt.mu.Lock()
	currentSession := rt.sessionID
	injected := rt.request.InjectPlugin
	rt.mu.Unlock()
	if !injected || currentSession != sessionID {
		return
	}

	task := a.debugRecordingTaskByID(recordingID)
	if task == nil {
		return
	}
	a.debugRecordingsMu.Lock()
	taskSession := task.sessionID
	a.debugRecordingsMu.Unlock()
	if taskSession != sessionID {
		return
	}

	switch strings.TrimSpace(payload.RecordPhase) {
	case "start":
		a.markDebugRecordingStarted(task, payload.Tick)
	case "end":
		a.markDebugRecordingEnded(task, payload.Tick)
	}
}

// failActiveDebugRecordings marks the session's starting/recording/stopping
// segments as failed while keeping their known files. A merge that is already
// running does not depend on the game and keeps its own lifecycle.
func (a *App) failActiveDebugRecordings(sessionID string, reason string) {
	if a == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	changed := false
	a.debugRecordingsMu.Lock()
	for _, task := range a.debugRecordings {
		if task == nil || task.sessionID != sessionID {
			continue
		}
		switch task.status {
		case DebugRecordingStatusStarting, DebugRecordingStatusRecording, DebugRecordingStatusStopping:
			task.status = DebugRecordingStatusFailed
			task.lastError = reason
			task.updatedAtMs = a.nextDebugStamp()
			changed = true
		}
	}
	a.debugRecordingsMu.Unlock()
	if changed {
		a.emitDebugSessionStateAsync()
	}
}

// finalizeDebugRecordingsForSession ends the active segment and waits for its
// bounded merge before the game environment is torn down. The stop request is
// serialized with start/play/retry under the workbench operation lock; the wait
// itself is lock-free so a slow merge never blocks a concurrent stop request.
// A merge that cannot finish in time is cancelled and writes its own failure,
// keeping the raw files retryable.
func (a *App) finalizeDebugRecordingsForSession(rt *debugSessionRuntime) {
	if rt == nil {
		return
	}
	a.debugWorkbenchOpMu.Lock()
	task := a.activeDebugRecordingForSession(rt.sessionID)
	if task == nil {
		a.debugWorkbenchOpMu.Unlock()
		return
	}

	a.debugRecordingsMu.Lock()
	status := task.status
	a.debugRecordingsMu.Unlock()
	if status == DebugRecordingStatusStarting || status == DebugRecordingStatusRecording {
		if err := a.sendDebugRecordStop(rt, task.id); err != nil {
			a.failDebugRecordingIfStatus(task, []string{
				DebugRecordingStatusStarting, DebugRecordingStatusRecording, DebugRecordingStatusStopping,
			}, err)
		} else {
			a.markDebugRecordingEndedFromReceipt(task)
		}
	}
	a.debugWorkbenchOpMu.Unlock()

	deadline := time.Now().Add(debugRecordingFinalizeTimeout)
	for {
		a.debugRecordingsMu.Lock()
		status = task.status
		done := task.mergeDone
		a.debugRecordingsMu.Unlock()

		switch status {
		case DebugRecordingStatusMerging:
			if done != nil {
				select {
				case <-done:
					continue
				case <-time.After(debugRecordingMergePollWait):
				}
			}
		case DebugRecordingStatusStarting, DebugRecordingStatusRecording, DebugRecordingStatusStopping:
			// The end boundary has not been observed yet.
		default:
			return
		}

		if time.Now().After(deadline) {
			if status == DebugRecordingStatusMerging {
				// Cancel and wait for the bounded merge to exit; its own
				// completion writes the failed state, so a retry can only start
				// after the old goroutine and its ffmpeg child are gone.
				a.cancelDebugRecordingMerge(task)
				if done != nil {
					select {
					case <-done:
					case <-time.After(debugSessionStopWait):
					}
				}
				return
			}
			a.failDebugRecordingIfStatus(task, []string{
				DebugRecordingStatusStarting, DebugRecordingStatusRecording, DebugRecordingStatusStopping,
			}, fmt.Errorf("结束调试录制超时，源文件已保留"))
			return
		}
		time.Sleep(debugRecordingMergePollWait)
	}
}

// sendDebugRecordStop sends the stop request for one segment on the session's
// ready debug connection.
func (a *App) sendDebugRecordStop(rt *debugSessionRuntime, recordingID string) error {
	_, connID, err := a.debugReadyRuntime()
	if err != nil {
		return err
	}
	result, err := a.sendDebugRequestOnRuntime(rt, connID, "debug_record_stop", func(requestID string) any {
		return debugRecordStopPayload{RequestID: requestID, RecordingID: recordingID}
	})
	if err != nil {
		return err
	}
	if result.Status != DebugCommandStatusExecuted {
		return fmt.Errorf("%s", debugResultError(result.Error, "插件停止录制失败"))
	}
	return nil
}

// abortDebugRecordingsForSession is the cancel/shutdown path: the session is
// going away without a graceful end, so the active segment (if any) is marked
// failed with its known files kept. It never resends start/stop.
func (a *App) abortDebugRecordingsForSession(rt *debugSessionRuntime) {
	if rt == nil {
		return
	}
	a.failActiveDebugRecordings(rt.sessionID, "调试会话已结束或被取消，录制未正常停止")
}

func debugResultError(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed
	}
	return fallback
}
