package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/producemerge"

	"github.com/gorilla/websocket"
)

// shortenDebugRecordingSeams keeps the file-wait/finalize windows short so a
// test never waits for a real recorder or the production 30 s/90 s defaults.
func shortenDebugRecordingSeams(t *testing.T) {
	t.Helper()
	oldReady, oldStable := debugRecordingFileReadyTimeout, debugRecordingFileStableInterval
	oldFinalize, oldPoll := debugRecordingFinalizeTimeout, debugRecordingMergePollWait
	debugRecordingFileReadyTimeout = 500 * time.Millisecond
	debugRecordingFileStableInterval = 20 * time.Millisecond
	debugRecordingFinalizeTimeout = 3 * time.Second
	debugRecordingMergePollWait = 10 * time.Millisecond
	t.Cleanup(func() {
		debugRecordingFileReadyTimeout, debugRecordingFileStableInterval = oldReady, oldStable
		debugRecordingFinalizeTimeout, debugRecordingMergePollWait = oldFinalize, oldPoll
	})
}

// startDebugRecordingTestSession starts a ready plugin-mode debug session and
// points FFmpeg at a stub binary so the debug merge pre-flight checks pass.
// Merge work itself is faked by the caller through
// producemerge.FFmpegCommandContext.
func startDebugRecordingTestSession(t *testing.T) (*App, *websocket.Conn) {
	t.Helper()
	shortenDebugRecordingSeams(t)
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	ffmpegDir := filepath.Join(app.exeDir, "ffmpeg", "bin")
	if err := os.MkdirAll(ffmpegDir, 0o755); err != nil {
		t.Fatalf("mkdir ffmpeg dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ffmpegDir, "ffmpeg.exe"), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}

	return app, conn
}

type debugRecordStartWire struct {
	Name    string `json:"name"`
	Payload struct {
		RequestID   string   `json:"request_id"`
		RecordingID string   `json:"recording_id"`
		DemoPath    string   `json:"demo_path"`
		TakeName    string   `json:"take_name"`
		Commands    []string `json:"commands"`
	} `json:"payload"`
}

type debugRecordStopWire struct {
	Name    string `json:"name"`
	Payload struct {
		RequestID   string `json:"request_id"`
		RecordingID string `json:"recording_id"`
	} `json:"payload"`
}

// beginDebugPlayback drives one PlayDebugDemo to confirmed engine playback.
func beginDebugPlayback(t *testing.T, app *App, conn *websocket.Conn) string {
	t.Helper()
	demoPath := writeManagedDebugDemo(t, app.dataRoot(), "match.dem")
	response, err := playDebugDemoWithFakePlugin(t, app, conn, demoPath)
	if err != nil {
		t.Fatalf("PlayDebugDemo: %v", err)
	}
	if response.Status != DebugCommandStatusExecuted {
		t.Fatalf("play status = %q", response.Status)
	}
	if err := conn.WriteJSON(map[string]any{
		"name": "debug_demo_state",
		"payload": map[string]any{
			"session_id": app.GetDebugSessionState().SessionID,
			"demo_path":  demoPath,
			"playing":    true,
			"paused":     false,
		},
	}); err != nil {
		t.Fatalf("write debug_demo_state: %v", err)
	}
	waitForDebugDemoPlaying(t, app, true)
	return demoPath
}

// playDebugDemoWithFakePlugin runs PlayDebugDemo in the background and answers
// its receipt from the fake plugin connection.
func playDebugDemoWithFakePlugin(t *testing.T, app *App, conn *websocket.Conn, demoPath string) (DebugPlayDemoResponse, error) {
	t.Helper()
	type callResult struct {
		response DebugPlayDemoResponse
		err      error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		response, err := app.PlayDebugDemo(DebugPlayDemoRequest{DemoPath: demoPath})
		resultCh <- callResult{response: response, err: err}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var wire debugPlayDemoWire
	if err := conn.ReadJSON(&wire); err != nil {
		t.Fatalf("read debug_play_demo: %v", err)
	}
	if err := conn.WriteJSON(map[string]any{
		"name": "debug_command_result",
		"payload": map[string]any{
			"request_id": wire.Payload.RequestID,
			"status":     "executed",
		},
	}); err != nil {
		t.Fatalf("write play receipt: %v", err)
	}
	result := <-resultCh
	return result.response, result.err
}

// startDebugRecordingWithFakePlugin runs StartDebugRecording in the background,
// answers `debug_record_start`, and returns the wire payload plus the API error.
func startDebugRecordingWithFakePlugin(t *testing.T, app *App, conn *websocket.Conn, sendStartStatus bool) (debugRecordStartWire, error) {
	t.Helper()
	type callResult struct {
		state DebugSessionState
		err   error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		state, err := app.StartDebugRecording(DebugRecordingRequest{
			RecordFPS:     60,
			RecordQuality: "high",
			VideoPreset:   "c1",
		})
		resultCh <- callResult{state: state, err: err}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var wire debugRecordStartWire
	if err := conn.ReadJSON(&wire); err != nil {
		t.Fatalf("read debug_record_start: %v", err)
	}
	if sendStartStatus {
		if err := conn.WriteJSON(map[string]any{
			"name": "debug_record_status",
			"payload": map[string]any{
				"session_id":   app.GetDebugSessionState().SessionID,
				"recording_id": wire.Payload.RecordingID,
				"demo_path":    wire.Payload.DemoPath,
				"take_name":    wire.Payload.TakeName,
				"record_phase": "start",
				"tick":         100,
			},
		}); err != nil {
			t.Fatalf("write debug_record_status start: %v", err)
		}
	}
	if err := conn.WriteJSON(map[string]any{
		"name": "debug_command_result",
		"payload": map[string]any{
			"request_id": wire.Payload.RequestID,
			"status":     "executed",
		},
	}); err != nil {
		t.Fatalf("write record start receipt: %v", err)
	}
	result := <-resultCh
	return wire, result.err
}

// stopDebugRecordingWithFakePlugin runs StopDebugRecording in the background
// and answers both its end observation and its receipt.
func stopDebugRecordingWithFakePlugin(t *testing.T, app *App, conn *websocket.Conn, endTick int) error {
	t.Helper()
	type callResult struct {
		state DebugSessionState
		err   error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		state, err := app.StopDebugRecording()
		resultCh <- callResult{state: state, err: err}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var wire debugRecordStopWire
	if err := conn.ReadJSON(&wire); err != nil {
		t.Fatalf("read debug_record_stop: %v", err)
	}
	if err := conn.WriteJSON(map[string]any{
		"name": "debug_record_status",
		"payload": map[string]any{
			"session_id":   app.GetDebugSessionState().SessionID,
			"recording_id": wire.Payload.RecordingID,
			"record_phase": "end",
			"tick":         endTick,
		},
	}); err != nil {
		t.Fatalf("write debug_record_status end: %v", err)
	}
	if err := conn.WriteJSON(map[string]any{
		"name": "debug_command_result",
		"payload": map[string]any{
			"request_id": wire.Payload.RequestID,
			"status":     "executed",
		},
	}); err != nil {
		t.Fatalf("write record stop receipt: %v", err)
	}
	result := <-resultCh
	return result.err
}

func findDebugRecording(t *testing.T, app *App, recordingID string) DebugRecordingStatus {
	t.Helper()
	for _, item := range app.GetDebugSessionState().Recordings {
		if item.RecordingID == recordingID {
			return item
		}
	}
	t.Fatalf("recording %s not found in %+v", recordingID, app.GetDebugSessionState().Recordings)
	return DebugRecordingStatus{}
}

func waitForDebugRecordingStatus(t *testing.T, app *App, recordingID, want string) DebugRecordingStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last DebugRecordingStatus
	for time.Now().Before(deadline) {
		last = findDebugRecording(t, app, recordingID)
		if last.Status == want {
			return last
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("recording %s status = %q, want %q (last %+v)", recordingID, last.Status, want, last)
	return DebugRecordingStatus{}
}

func TestDebugRecordingTakePathsFollowHLAERule(t *testing.T) {
	dir := filepath.Join("outputs", "debug", "dbg-1", "rec-1")
	videoPath, audioPath := debugRecordingTakePaths(dir)
	wantVideo := filepath.Join(dir, "take0000") + ".mp4"
	wantAudio := filepath.Join(dir, "take0000", "audio.wav")
	if videoPath != wantVideo {
		t.Fatalf("video path = %q, want %q", videoPath, wantVideo)
	}
	if audioPath != wantAudio {
		t.Fatalf("audio path = %q, want %q", audioPath, wantAudio)
	}
}

func TestStartDebugRecordingBuildsSharedInitCommandsAndTracksBoundary(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	demoPath := beginDebugPlayback(t, app, conn)

	wire, err := startDebugRecordingWithFakePlugin(t, app, conn, true)
	if err != nil {
		t.Fatalf("StartDebugRecording: %v", err)
	}
	if wire.Name != "debug_record_start" {
		t.Fatalf("wire name = %q", wire.Name)
	}
	if wire.Payload.DemoPath != demoPath {
		t.Fatalf("wire demo_path = %q, want %q", wire.Payload.DemoPath, demoPath)
	}
	if wire.Payload.TakeName != "take0000" {
		t.Fatalf("wire take_name = %q, want take0000", wire.Payload.TakeName)
	}
	joined := strings.Join(wire.Payload.Commands, "\n")
	for _, want := range []string{
		"mirv_streams record screen enabled 1",
		"mirv_streams record fps 60",
		"mirv_streams record startMovieWav 1",
		"mirv_streams settings add ffmpeg debug_c1_high_",
		"mirv_streams record screen settings debug_c1_high_",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("commands missing %q:\n%s", want, joined)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(wire.Payload.Commands[len(wire.Payload.Commands)-1]), "mirv_streams record start") {
		t.Fatalf("last command must be the recording start boundary:\n%s", joined)
	}
	// 4:3 is the frozen launch resolution, so the real pixel stretch stays.
	if !strings.Contains(joined, "scale=trunc(ih*16/9/2)*2:ih") {
		t.Fatalf("4:3 recording must keep the real pixel stretch:\n%s", joined)
	}
	// The record name is the unique per-segment directory, not a shared prefix.
	recordDir := filepath.Join(app.dataRoot(), "outputs", "debug", app.GetDebugSessionState().SessionID, wire.Payload.RecordingID)
	if !strings.Contains(joined, strings.ReplaceAll(recordDir, "\\", "/")) {
		t.Fatalf("record name must point at %s:\n%s", recordDir, joined)
	}

	state := app.GetDebugSessionState()
	if state.Recording == nil || state.Recording.RecordingID != wire.Payload.RecordingID {
		t.Fatalf("active recording = %+v, want %s", state.Recording, wire.Payload.RecordingID)
	}
	recording := waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusRecording)
	if recording.StartTick != 100 {
		t.Fatalf("start tick = %d, want the plugin-observed tick", recording.StartTick)
	}
	wantVideo, _ := debugRecordingTakePaths(recordDir)
	if recording.VideoPath != wantVideo {
		t.Fatalf("video path = %q", recording.VideoPath)
	}
}

func TestStartDebugRecordingRequiresPlayingDemo(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	if _, err := app.StartDebugRecording(DebugRecordingRequest{RecordFPS: 60, RecordQuality: "high", VideoPreset: "c1"}); err == nil {
		t.Fatal("recording started without a confirmed playing Demo")
	}
}

func TestStartDebugRecordingRejectsSecondSegment(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	beginDebugPlayback(t, app, conn)
	if _, err := startDebugRecordingWithFakePlugin(t, app, conn, true); err != nil {
		t.Fatalf("StartDebugRecording: %v", err)
	}
	if _, err := app.StartDebugRecording(DebugRecordingRequest{RecordFPS: 60, RecordQuality: "high", VideoPreset: "c1"}); err == nil {
		t.Fatal("a second segment was accepted while one is active")
	}
	if _, err := app.PlayDebugDemo(DebugPlayDemoRequest{DemoPath: filepath.Join(app.dataRoot(), "demo", "match.dem")}); err == nil {
		t.Fatal("Demo switch was accepted during an active recording")
	}
}

func TestStopDebugRecordingMergesAndCleansSources(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	beginDebugPlayback(t, app, conn)
	wire, err := startDebugRecordingWithFakePlugin(t, app, conn, true)
	if err != nil {
		t.Fatalf("StartDebugRecording: %v", err)
	}
	waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusRecording)

	recording := findDebugRecording(t, app, wire.Payload.RecordingID)
	for _, path := range []string{recording.VideoPath, recording.AudioPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir raw file dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("raw"), 0o644); err != nil {
			t.Fatalf("write raw file: %v", err)
		}
	}

	oldContext := producemerge.FFmpegCommandContext
	producemerge.FFmpegCommandContext = fakeFFmpegCommandSuccessContext
	t.Cleanup(func() { producemerge.FFmpegCommandContext = oldContext })

	if err := stopDebugRecordingWithFakePlugin(t, app, conn, 200); err != nil {
		t.Fatalf("StopDebugRecording: %v", err)
	}
	completed := waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusCompleted)
	if completed.FinalPath == "" {
		t.Fatal("completed recording has no final path")
	}
	if _, err := os.Stat(completed.FinalPath); err != nil {
		t.Fatalf("final video missing: %v", err)
	}
	// keep_intermediate_files defaults to false, so the raw sources are gone
	// but the published mux output stays.
	if _, err := os.Stat(recording.VideoPath); !os.IsNotExist(err) {
		t.Fatalf("raw video still exists after a successful merge: %v", err)
	}
	if _, err := os.Stat(recording.AudioPath); !os.IsNotExist(err) {
		t.Fatalf("raw audio still exists after a successful merge: %v", err)
	}
	if err := app.OpenDebugRecordingInFolder(wire.Payload.RecordingID); err != nil {
		t.Fatalf("OpenDebugRecordingInFolder: %v", err)
	}
}

func TestStopDebugRecordingKeepsSourcesWhenRequested(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	beginDebugPlayback(t, app, conn)

	type callResult struct {
		state DebugSessionState
		err   error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		state, err := app.StartDebugRecording(DebugRecordingRequest{
			RecordFPS:             60,
			RecordQuality:         "high",
			VideoPreset:           "c1",
			KeepIntermediateFiles: true,
		})
		resultCh <- callResult{state: state, err: err}
	}()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var wire debugRecordStartWire
	if err := conn.ReadJSON(&wire); err != nil {
		t.Fatalf("read debug_record_start: %v", err)
	}
	if err := conn.WriteJSON(map[string]any{
		"name":    "debug_command_result",
		"payload": map[string]any{"request_id": wire.Payload.RequestID, "status": "executed"},
	}); err != nil {
		t.Fatalf("write start receipt: %v", err)
	}
	if result := <-resultCh; result.err != nil {
		t.Fatalf("StartDebugRecording: %v", result.err)
	}

	recording := findDebugRecording(t, app, wire.Payload.RecordingID)
	for _, path := range []string{recording.VideoPath, recording.AudioPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir raw file dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("raw"), 0o644); err != nil {
			t.Fatalf("write raw file: %v", err)
		}
	}

	oldContext := producemerge.FFmpegCommandContext
	producemerge.FFmpegCommandContext = fakeFFmpegCommandSuccessContext
	t.Cleanup(func() { producemerge.FFmpegCommandContext = oldContext })

	if err := stopDebugRecordingWithFakePlugin(t, app, conn, 200); err != nil {
		t.Fatalf("StopDebugRecording: %v", err)
	}
	waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusCompleted)
	for _, path := range []string{recording.VideoPath, recording.AudioPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("raw source %s was removed despite keep_intermediate_files: %v", path, err)
		}
	}
}

func TestStopDebugRecordingKeepsSourcesOnMergeFailureAndRetries(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	beginDebugPlayback(t, app, conn)
	wire, err := startDebugRecordingWithFakePlugin(t, app, conn, true)
	if err != nil {
		t.Fatalf("StartDebugRecording: %v", err)
	}
	waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusRecording)
	recording := findDebugRecording(t, app, wire.Payload.RecordingID)
	for _, path := range []string{recording.VideoPath, recording.AudioPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir raw file dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("raw"), 0o644); err != nil {
			t.Fatalf("write raw file: %v", err)
		}
	}

	oldContext := producemerge.FFmpegCommandContext
	producemerge.FFmpegCommandContext = fakeFFmpegCommandFailContext
	t.Cleanup(func() { producemerge.FFmpegCommandContext = oldContext })

	if err := stopDebugRecordingWithFakePlugin(t, app, conn, 200); err != nil {
		t.Fatalf("StopDebugRecording: %v", err)
	}
	failed := waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusFailed)
	if failed.Error == "" {
		t.Fatal("failed merge must report a visible error")
	}
	for _, path := range []string{recording.VideoPath, recording.AudioPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("retryable source %s was removed: %v", path, err)
		}
	}

	// A retry reuses the same segment files instead of re-recording.
	producemerge.FFmpegCommandContext = fakeFFmpegCommandSuccessContext
	if _, err := app.RetryDebugRecordingMerge(wire.Payload.RecordingID); err != nil {
		t.Fatalf("RetryDebugRecordingMerge: %v", err)
	}
	retried := waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusCompleted)
	if retried.FinalPath == "" {
		t.Fatal("retried merge did not publish a final video")
	}
	// Retrying a completed segment is refused.
	if _, err := app.RetryDebugRecordingMerge(wire.Payload.RecordingID); err == nil {
		t.Fatal("retry of a completed recording was accepted")
	}
}

func TestRetryDebugRecordingMergeRequiresReadableSources(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	beginDebugPlayback(t, app, conn)
	wire, err := startDebugRecordingWithFakePlugin(t, app, conn, true)
	if err != nil {
		t.Fatalf("StartDebugRecording: %v", err)
	}
	// Stop without creating the raw files: the file wait fails and keeps no
	// readable source, so a retry must be refused with a visible error.
	if err := stopDebugRecordingWithFakePlugin(t, app, conn, 200); err != nil {
		t.Fatalf("StopDebugRecording: %v", err)
	}
	waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusFailed)
	if _, err := app.RetryDebugRecordingMerge(wire.Payload.RecordingID); err == nil {
		t.Fatal("retry with unreadable sources was accepted")
	}
	if _, err := app.RetryDebugRecordingMerge("rec-missing"); err == nil {
		t.Fatal("retry of an unknown recording was accepted")
	}
}

func TestDebugRecordingDisconnectMarksSegmentFailed(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	beginDebugPlayback(t, app, conn)
	wire, err := startDebugRecordingWithFakePlugin(t, app, conn, true)
	if err != nil {
		t.Fatalf("StartDebugRecording: %v", err)
	}
	waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusRecording)

	if err := conn.Close(); err != nil {
		t.Fatalf("close fake plugin connection: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && app.produceW.GetWSState().Connected {
		time.Sleep(time.Millisecond)
	}
	app.refreshDebugPluginState()
	failed := waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusFailed)
	if failed.Error == "" {
		t.Fatal("disconnect failure must carry a visible error")
	}
	if app.GetDebugSessionState().Recording != nil {
		t.Fatal("failed segment is still reported as the active recording")
	}
}

func TestDebugRecordStatusIgnoresUnknownOrForeignRecordingID(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	beginDebugPlayback(t, app, conn)
	wire, err := startDebugRecordingWithFakePlugin(t, app, conn, false)
	if err != nil {
		t.Fatalf("StartDebugRecording: %v", err)
	}
	// The receipt fallback already advanced the segment because the plugin
	// answers after the boundary ran.
	started := waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusRecording)

	// A foreign recording id must not touch this segment.
	if err := conn.WriteJSON(map[string]any{
		"name": "debug_record_status",
		"payload": map[string]any{
			"session_id":   app.GetDebugSessionState().SessionID,
			"recording_id": "rec-other",
			"record_phase": "end",
			"tick":         300,
		},
	}); err != nil {
		t.Fatalf("write foreign record status: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if got := findDebugRecording(t, app, wire.Payload.RecordingID); got.Status != started.Status {
		t.Fatalf("foreign status changed the segment: %+v", got)
	}
}

func TestStopDebugGameFinalizesActiveRecording(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	beginDebugPlayback(t, app, conn)
	wire, err := startDebugRecordingWithFakePlugin(t, app, conn, true)
	if err != nil {
		t.Fatalf("StartDebugRecording: %v", err)
	}
	waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusRecording)
	recording := findDebugRecording(t, app, wire.Payload.RecordingID)
	for _, path := range []string{recording.VideoPath, recording.AudioPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir raw file dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("raw"), 0o644); err != nil {
			t.Fatalf("write raw file: %v", err)
		}
	}

	oldContext := producemerge.FFmpegCommandContext
	producemerge.FFmpegCommandContext = fakeFFmpegCommandSuccessContext
	t.Cleanup(func() { producemerge.FFmpegCommandContext = oldContext })

	// StopDebugGame must request the end, let the bounded merge finish, and only
	// then tear the session down.
	type callResult struct {
		state DebugSessionState
		err   error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		state, err := app.StopDebugGame()
		resultCh <- callResult{state: state, err: err}
	}()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var stopWire debugRecordStopWire
	if err := conn.ReadJSON(&stopWire); err != nil {
		t.Fatalf("read debug_record_stop during StopDebugGame: %v", err)
	}
	if err := conn.WriteJSON(map[string]any{
		"name":    "debug_command_result",
		"payload": map[string]any{"request_id": stopWire.Payload.RequestID, "status": "executed"},
	}); err != nil {
		t.Fatalf("write stop receipt: %v", err)
	}
	result := <-resultCh
	if result.err != nil {
		t.Fatalf("StopDebugGame: %v", result.err)
	}
	completed := findDebugRecording(t, app, wire.Payload.RecordingID)
	if completed.Status != DebugRecordingStatusCompleted {
		t.Fatalf("recording status after StopDebugGame = %q, want completed (%s)", completed.Status, completed.Error)
	}
	if result.state.Status == DebugStatusRunning {
		t.Fatalf("debug session still running after StopDebugGame: %+v", result.state)
	}
}

func TestResetDebugRecordingsClearsWorkspaceResults(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	beginDebugPlayback(t, app, conn)
	wire, err := startDebugRecordingWithFakePlugin(t, app, conn, true)
	if err != nil {
		t.Fatalf("StartDebugRecording: %v", err)
	}
	if len(app.GetDebugSessionState().Recordings) == 0 {
		t.Fatal("recording was not listed")
	}
	app.resetDebugRecordings()
	if got := findDebugRecordingIDPresent(app, wire.Payload.RecordingID); got {
		t.Fatal("recording survived a workspace-scope reset")
	}
}

func findDebugRecordingIDPresent(app *App, recordingID string) bool {
	for _, item := range app.GetDebugSessionState().Recordings {
		if item.RecordingID == recordingID {
			return true
		}
	}
	return false
}

// injectDebugRecordingTask installs a task directly. It models a pre-existing
// failed segment next to a live one without going through the single-segment
// admission gate.
func injectDebugRecordingTask(t *testing.T, app *App, task *debugRecordingTask) {
	t.Helper()
	if task.status == "" {
		task.status = DebugRecordingStatusFailed
	}
	task.startedAtMs = nowMs()
	// Use the shared clock so the injected task participates in the snapshot
	// version ordering exactly like a production mutation.
	task.updatedAtMs = app.nextDebugStamp()
	app.debugRecordingsMu.Lock()
	defer app.debugRecordingsMu.Unlock()
	if app.debugRecordings == nil {
		app.debugRecordings = make(map[string]*debugRecordingTask)
	}
	app.debugRecordings[task.id] = task
	app.debugRecordingOrder = append(app.debugRecordingOrder, task.id)
}

func writeReadableDebugSources(t *testing.T, videoPath, audioPath string) {
	t.Helper()
	for _, path := range []string{videoPath, audioPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir raw file dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("raw"), 0o644); err != nil {
			t.Fatalf("write raw file: %v", err)
		}
	}
}

// writeDebugRecordStatus answers one boundary observation for a wire request.
func writeDebugRecordStatus(t *testing.T, conn *websocket.Conn, sessionID, recordingID, phase string, tick int) {
	t.Helper()
	if err := conn.WriteJSON(map[string]any{
		"name": "debug_record_status",
		"payload": map[string]any{
			"session_id":   sessionID,
			"recording_id": recordingID,
			"record_phase": phase,
			"tick":         tick,
		},
	}); err != nil {
		t.Fatalf("write debug_record_status %s: %v", phase, err)
	}
}

func writeDebugCommandResult(t *testing.T, conn *websocket.Conn, requestID, status string) {
	t.Helper()
	if err := conn.WriteJSON(map[string]any{
		"name":    "debug_command_result",
		"payload": map[string]any{"request_id": requestID, "status": status},
	}); err != nil {
		t.Fatalf("write debug_command_result: %v", err)
	}
}

func overrideFFmpegWithSuccess(t *testing.T) {
	t.Helper()
	oldContext := producemerge.FFmpegCommandContext
	producemerge.FFmpegCommandContext = fakeFFmpegCommandSuccessContext
	t.Cleanup(func() { producemerge.FFmpegCommandContext = oldContext })
}

// waitForDebugRecordingIdle waits until no segment is active (recording or
// merging). Tests must call it before returning when they may have left a merge
// running: otherwise the test cleanup can restore the ffmpeg/timeout seams
// while the merge goroutine still reads them, which is a test-lifetime data
// race, not a production race.
func waitForDebugRecordingIdle(t *testing.T, app *App) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if app.debugActiveRecordingTask() == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("debug recording did not become idle: %+v", app.GetDebugSessionState().Recording)
}

// TestStopWaitsForInFlightStartAndStillEndsTheSegment covers the review's
// highest-risk interleaving: a stop click while the start is still preparing.
// The stop must queue behind the start (not fail before the plugin recording
// exists) and then end the very segment that started.
func TestStopWaitsForInFlightStartAndStillEndsTheSegment(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	beginDebugPlayback(t, app, conn)
	sessionID := app.GetDebugSessionState().SessionID

	type startResult struct {
		state DebugSessionState
		err   error
	}
	startCh := make(chan startResult, 1)
	go func() {
		state, err := app.StartDebugRecording(DebugRecordingRequest{
			RecordFPS: 60, RecordQuality: "high", VideoPreset: "c1",
		})
		startCh <- startResult{state: state, err: err}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var startWire debugRecordStartWire
	if err := conn.ReadJSON(&startWire); err != nil {
		t.Fatalf("read debug_record_start: %v", err)
	}

	stopCh := make(chan error, 1)
	go func() {
		_, err := app.StopDebugRecording()
		stopCh <- err
	}()

	// While the start receipt is unanswered, the queued stop must not have
	// registered a plugin request yet: sending one now would be refused as
	// "no matching recording" and lose the segment.
	time.Sleep(150 * time.Millisecond)
	rt := app.debugRuntimeForAdmission()
	rt.mu.Lock()
	pending := len(rt.debugPending)
	rt.mu.Unlock()
	if pending != 1 {
		t.Fatalf("pending debug requests = %d, want only the in-flight start", pending)
	}
	select {
	case err := <-stopCh:
		t.Fatalf("stop returned before the in-flight start finished: %v", err)
	default:
	}

	writeDebugRecordStatus(t, conn, sessionID, startWire.Payload.RecordingID, "start", 100)
	writeDebugCommandResult(t, conn, startWire.Payload.RequestID, "executed")
	if result := <-startCh; result.err != nil {
		t.Fatalf("StartDebugRecording: %v", result.err)
	}

	// The queued stop now ends the recording that just started.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var stopWire debugRecordStopWire
	if err := conn.ReadJSON(&stopWire); err != nil {
		t.Fatalf("read debug_record_stop: %v", err)
	}
	if stopWire.Payload.RecordingID != startWire.Payload.RecordingID {
		t.Fatalf("stop recording id = %q, want %q", stopWire.Payload.RecordingID, startWire.Payload.RecordingID)
	}
	writeDebugRecordStatus(t, conn, sessionID, stopWire.Payload.RecordingID, "end", 200)
	writeDebugCommandResult(t, conn, stopWire.Payload.RequestID, "executed")
	if err := <-stopCh; err != nil {
		t.Fatalf("StopDebugRecording: %v", err)
	}

	// The segment left the recording state through its own end boundary. The
	// files are missing in this mock, so the merge is allowed to fail, but it
	// must never be the "no matching recording" path.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		recording := findDebugRecording(t, app, startWire.Payload.RecordingID)
		if recording.Status != DebugRecordingStatusStarting && recording.Status != DebugRecordingStatusRecording {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	recording := findDebugRecording(t, app, startWire.Payload.RecordingID)
	if recording.Status == DebugRecordingStatusStarting || recording.Status == DebugRecordingStatusRecording {
		t.Fatalf("segment never left recording: %+v", recording)
	}
	// The end boundary may have launched the merge; wait for it before the test
	// returns so no goroutine outlives the restored seams.
	waitForDebugRecordingIdle(t, app)
}

// TestPlayDebugDemoWaitsForInFlightStartAndIsThenRejected verifies the Demo
// switch and the recording start share one critical section.
func TestPlayDebugDemoWaitsForInFlightStartAndIsThenRejected(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	demoPath := beginDebugPlayback(t, app, conn)
	sessionID := app.GetDebugSessionState().SessionID

	startCh := make(chan error, 1)
	go func() {
		_, err := app.StartDebugRecording(DebugRecordingRequest{
			RecordFPS: 60, RecordQuality: "high", VideoPreset: "c1",
		})
		startCh <- err
	}()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var startWire debugRecordStartWire
	if err := conn.ReadJSON(&startWire); err != nil {
		t.Fatalf("read debug_record_start: %v", err)
	}

	playCh := make(chan error, 1)
	go func() {
		_, err := app.PlayDebugDemo(DebugPlayDemoRequest{DemoPath: demoPath})
		playCh <- err
	}()
	// No play request may register while the recording start owns the critical
	// section; reading the websocket would corrupt the test connection on a
	// timeout, so inspect the pending request table instead.
	time.Sleep(150 * time.Millisecond)
	rt := app.debugRuntimeForAdmission()
	rt.mu.Lock()
	pending := len(rt.debugPending)
	rt.mu.Unlock()
	if pending != 1 {
		t.Fatalf("pending debug requests = %d, want only the in-flight start", pending)
	}

	writeDebugRecordStatus(t, conn, sessionID, startWire.Payload.RecordingID, "start", 100)
	writeDebugCommandResult(t, conn, startWire.Payload.RequestID, "executed")
	if err := <-startCh; err != nil {
		t.Fatalf("StartDebugRecording: %v", err)
	}
	if err := <-playCh; err == nil {
		t.Fatal("Demo switch was accepted while a recording is active")
	}
}

// TestPlayCannotInterleaveWithRecordingStartPreparation pins the exact window
// from the review: the start has read the playback facts but has not reserved
// the segment yet. A Demo switch must not be able to pass its own busy check in
// that window and change the Demo under the preparing recording.
func TestPlayCannotInterleaveWithRecordingStartPreparation(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	demoPath := beginDebugPlayback(t, app, conn)
	sessionID := app.GetDebugSessionState().SessionID

	startInHook := make(chan struct{})
	releaseHook := make(chan struct{})
	oldHook := debugRecordingAdmissionHook
	debugRecordingAdmissionHook = func() {
		close(startInHook)
		<-releaseHook
	}
	t.Cleanup(func() {
		debugRecordingAdmissionHook = oldHook
		select {
		case <-releaseHook:
		default:
			close(releaseHook)
		}
	})

	startCh := make(chan error, 1)
	go func() {
		_, err := app.StartDebugRecording(DebugRecordingRequest{
			RecordFPS: 60, RecordQuality: "high", VideoPreset: "c1",
		})
		startCh <- err
	}()
	<-startInHook

	playCh := make(chan error, 1)
	go func() {
		_, err := app.PlayDebugDemo(DebugPlayDemoRequest{DemoPath: demoPath})
		playCh <- err
	}()

	// The play request must not register while the start owns the operation
	// lock: pending requests stay empty and no play wait is published.
	time.Sleep(150 * time.Millisecond)
	rt := app.debugRuntimeForAdmission()
	rt.mu.Lock()
	playPending := rt.debugDemoPlayPending
	pending := len(rt.debugPending)
	rt.mu.Unlock()
	if playPending || pending != 0 {
		t.Fatalf("play slipped into the recording preparation window: pending=%d play_pending=%v", pending, playPending)
	}

	close(releaseHook)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var startWire debugRecordStartWire
	if err := conn.ReadJSON(&startWire); err != nil {
		t.Fatalf("read debug_record_start: %v", err)
	}
	writeDebugRecordStatus(t, conn, sessionID, startWire.Payload.RecordingID, "start", 100)
	writeDebugCommandResult(t, conn, startWire.Payload.RequestID, "executed")
	if err := <-startCh; err != nil {
		t.Fatalf("StartDebugRecording: %v", err)
	}
	if err := <-playCh; err == nil {
		t.Fatal("Demo switch was accepted while a recording is active")
	}
}

// TestRetryDebugRecordingMergeSharesSingleSegmentAdmission verifies a failed
// segment cannot be re-merged next to an active recording or a second retry.
func TestRetryDebugRecordingMergeSharesSingleSegmentAdmission(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	beginDebugPlayback(t, app, conn)

	sourcesDir := t.TempDir()
	videoPath := filepath.Join(sourcesDir, "take0000.mp4")
	audioPath := filepath.Join(sourcesDir, "take0000", "audio.wav")
	writeReadableDebugSources(t, videoPath, audioPath)
	injectDebugRecordingTask(t, app, &debugRecordingTask{
		id: "rec-old", sessionID: "dbg-old", directory: sourcesDir,
		videoPath: videoPath, audioPath: audioPath,
		status: DebugRecordingStatusFailed,
	})

	wire, err := startDebugRecordingWithFakePlugin(t, app, conn, true)
	if err != nil {
		t.Fatalf("StartDebugRecording: %v", err)
	}
	if _, err := app.RetryDebugRecordingMerge("rec-old"); err == nil {
		t.Fatal("retry was accepted while another segment is recording")
	}

	// Finish the active segment, then the retry is allowed and becomes the only
	// active merge.
	if err := stopDebugRecordingWithFakePlugin(t, app, conn, 200); err != nil {
		t.Fatalf("StopDebugRecording: %v", err)
	}
	waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusFailed)
	overrideFFmpegWithSuccess(t)
	if _, err := app.RetryDebugRecordingMerge("rec-old"); err != nil {
		t.Fatalf("retry after the active segment finished: %v", err)
	}
	waitForDebugRecordingStatus(t, app, "rec-old", DebugRecordingStatusCompleted)
}

// TestConcurrentRetriesKeepOneBoundedMerge verifies two failed segments cannot
// start two merges at the same time.
func TestConcurrentRetriesKeepOneBoundedMerge(t *testing.T) {
	app, _ := startDebugRecordingTestSession(t)
	overrideFFmpegWithSuccess(t)

	for _, id := range []string{"rec-a", "rec-b"} {
		sourcesDir := t.TempDir()
		videoPath := filepath.Join(sourcesDir, "take0000.mp4")
		audioPath := filepath.Join(sourcesDir, "take0000", "audio.wav")
		writeReadableDebugSources(t, videoPath, audioPath)
		injectDebugRecordingTask(t, app, &debugRecordingTask{
			id: id, sessionID: "dbg-old", directory: sourcesDir,
			videoPath: videoPath, audioPath: audioPath,
			status: DebugRecordingStatusFailed,
		})
	}

	results := make(chan error, 2)
	for _, id := range []string{"rec-a", "rec-b"} {
		go func(recordingID string) {
			_, err := app.RetryDebugRecordingMerge(recordingID)
			results <- err
		}(id)
	}
	succeeded, rejected := 0, 0
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			rejected++
		} else {
			succeeded++
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent retries = %d succeeded / %d rejected, want 1/1", succeeded, rejected)
	}
	// The winning merge runs in the background; do not return while its goroutine
	// can still read the seams the test cleanup restores.
	waitForDebugRecordingIdle(t, app)
}

// TestStaleMergeAttemptCannotOverwriteNewerStatus is the unit guard behind the
// cancel/retry race: a cancelled run that is still exiting must not write its
// result over a newer attempt.
func TestStaleMergeAttemptCannotOverwriteNewerStatus(t *testing.T) {
	app := newTestAppInTempDir(t)
	task := &debugRecordingTask{id: "rec-1", sessionID: "s", status: DebugRecordingStatusMerging, mergeAttempt: 2}
	injectDebugRecordingTask(t, app, task)

	app.completeDebugRecordingMerge(task, 1, "/old/final.mp4", nil)
	app.debugRecordingsMu.Lock()
	status, finalPath := task.status, task.finalPath
	app.debugRecordingsMu.Unlock()
	if status != DebugRecordingStatusMerging || finalPath != "" {
		t.Fatalf("stale attempt wrote state: status=%q final=%q", status, finalPath)
	}

	app.completeDebugRecordingMerge(task, 2, "/new/final.mp4", nil)
	app.debugRecordingsMu.Lock()
	status, finalPath = task.status, task.finalPath
	app.debugRecordingsMu.Unlock()
	if status != DebugRecordingStatusCompleted || finalPath != "/new/final.mp4" {
		t.Fatalf("current attempt was not committed: status=%q final=%q", status, finalPath)
	}
}

func TestReserveDebugRecordingTaskIsAtomicAdmission(t *testing.T) {
	app := newTestAppInTempDir(t)
	first := &debugRecordingTask{id: "rec-1", sessionID: "s", status: DebugRecordingStatusStarting}
	if err := app.reserveDebugRecordingTask(first); err != nil {
		t.Fatalf("reserve first segment: %v", err)
	}
	second := &debugRecordingTask{id: "rec-2", sessionID: "s", status: DebugRecordingStatusStarting}
	if err := app.reserveDebugRecordingTask(second); err == nil {
		t.Fatal("a second active segment was reserved while one is active")
	}
	// A finished segment (including a failed merge waiting for retry) must not
	// block the next recording.
	app.debugRecordingsMu.Lock()
	first.status = DebugRecordingStatusFailed
	app.debugRecordingsMu.Unlock()
	if err := app.reserveDebugRecordingTask(second); err != nil {
		t.Fatalf("reserve after the previous segment finished: %v", err)
	}
}

func TestLateDebugRecordStatusCannotResurrectFinishedSegment(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	beginDebugPlayback(t, app, conn)
	wire, err := startDebugRecordingWithFakePlugin(t, app, conn, true)
	if err != nil {
		t.Fatalf("StartDebugRecording: %v", err)
	}
	waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusRecording)
	recording := findDebugRecording(t, app, wire.Payload.RecordingID)
	for _, path := range []string{recording.VideoPath, recording.AudioPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir raw file dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("raw"), 0o644); err != nil {
			t.Fatalf("write raw file: %v", err)
		}
	}
	oldContext := producemerge.FFmpegCommandContext
	producemerge.FFmpegCommandContext = fakeFFmpegCommandSuccessContext
	t.Cleanup(func() { producemerge.FFmpegCommandContext = oldContext })

	if err := stopDebugRecordingWithFakePlugin(t, app, conn, 200); err != nil {
		t.Fatalf("StopDebugRecording: %v", err)
	}
	waitForDebugRecordingStatus(t, app, wire.Payload.RecordingID, DebugRecordingStatusCompleted)

	// A duplicate/late end observation (for example a plugin replay) must not
	// move a finished segment back into merging or change its result.
	if err := conn.WriteJSON(map[string]any{
		"name": "debug_record_status",
		"payload": map[string]any{
			"session_id":   app.GetDebugSessionState().SessionID,
			"recording_id": wire.Payload.RecordingID,
			"record_phase": "end",
			"tick":         999,
		},
	}); err != nil {
		t.Fatalf("write late end status: %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	late := findDebugRecording(t, app, wire.Payload.RecordingID)
	if late.Status != DebugRecordingStatusCompleted {
		t.Fatalf("late status changed the finished segment: %+v", late)
	}
}

// TestDebugStampIsStrictlyIncreasing guards the shared clock behind the
// snapshot version: two mutations in the same millisecond must still get
// different, increasing stamps or the frontend would drop the second one.
func TestDebugStampIsStrictlyIncreasing(t *testing.T) {
	app := newTestAppInTempDir(t)
	prev := int64(0)
	for i := 0; i < 200; i++ {
		next := app.nextDebugStamp()
		if next <= prev {
			t.Fatalf("stamp %d did not advance past %d", next, prev)
		}
		prev = next
	}
}

// TestRecordingOnlyChangeAdvancesSnapshotVersion is the review's second issue:
// a recording mutation must bump DebugSessionState.UpdatedAtMs even when the
// session itself did not change.
func TestRecordingOnlyChangeAdvancesSnapshotVersion(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	beginDebugPlayback(t, app, conn)

	sourcesDir := t.TempDir()
	videoPath := filepath.Join(sourcesDir, "take0000.mp4")
	audioPath := filepath.Join(sourcesDir, "take0000", "audio.wav")
	writeReadableDebugSources(t, videoPath, audioPath)
	injectDebugRecordingTask(t, app, &debugRecordingTask{
		id: "rec-version", sessionID: "dbg-old", directory: sourcesDir,
		videoPath: videoPath, audioPath: audioPath,
		status: DebugRecordingStatusFailed,
	})

	before := app.GetDebugSessionState().UpdatedAtMs
	overrideFFmpegWithSuccess(t)
	if _, err := app.RetryDebugRecordingMerge("rec-version"); err != nil {
		t.Fatalf("RetryDebugRecordingMerge: %v", err)
	}
	after := app.GetDebugSessionState().UpdatedAtMs
	if after <= before {
		t.Fatalf("recording-only change did not advance the snapshot version: %d -> %d", before, after)
	}
	waitForDebugRecordingIdle(t, app)

	// Clearing the list is also a state change and must keep moving forward.
	beforeReset := app.GetDebugSessionState().UpdatedAtMs
	app.resetDebugRecordings()
	if afterReset := app.GetDebugSessionState().UpdatedAtMs; afterReset <= beforeReset {
		t.Fatalf("clearing the recording list moved the version backwards: %d -> %d", beforeReset, afterReset)
	}
}

// TestStartDebugRecordingRejectedWhileSessionIsStopping pins the closure of
// public admission: once StopDebugGame marked the runtime stopping, no new
// recording, Demo switch or console command may slip into the environment
// teardown window (for example after the last segment drained). The internal
// stop path stays open.
func TestStartDebugRecordingRejectedWhileSessionIsStopping(t *testing.T) {
	app, conn := startDebugRecordingTestSession(t)
	demoPath := beginDebugPlayback(t, app, conn)

	rt := app.debugRuntimeForAdmission()
	rt.mu.Lock()
	rt.status = DebugStatusStopping
	rt.updatedAtMs = app.nextDebugStamp()
	rt.mu.Unlock()

	assertRefusedPromptlyWhileStopping(t, "recording start", func() error {
		_, err := app.StartDebugRecording(DebugRecordingRequest{
			RecordFPS: 60, RecordQuality: "high", VideoPreset: "c1",
		})
		return err
	})
	assertRefusedPromptlyWhileStopping(t, "Demo switch", func() error {
		_, err := app.PlayDebugDemo(DebugPlayDemoRequest{DemoPath: demoPath})
		return err
	})
	assertRefusedPromptlyWhileStopping(t, "console command", func() error {
		_, err := app.SendDebugConsoleCommand(DebugCommandRequest{Commands: []string{"sv_cheats 1"}})
		return err
	})

	if app.debugActiveRecordingTask() != nil {
		t.Fatalf("a segment was reserved during shutdown: %+v", app.GetDebugSessionState().Recording)
	}
	rt.mu.Lock()
	pending := len(rt.debugPending)
	rt.mu.Unlock()
	if pending != 0 {
		t.Fatalf("a request reached the plugin while stopping: pending=%d", pending)
	}
}

// assertRefusedPromptlyWhileStopping fails when an operation is accepted or
// blocks waiting for a receipt instead of being refused by the admission gate.
func assertRefusedPromptlyWhileStopping(t *testing.T, name string, call func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- call() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("%s was accepted while the session is stopping", name)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("%s did not return promptly while the session is stopping", name)
	}
}

func TestDebugRecordingSourcesReadableRejectsEmptyFile(t *testing.T) {
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "take0000.mp4")
	audioPath := filepath.Join(dir, "take0000", "audio.wav")
	if err := os.MkdirAll(filepath.Dir(audioPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(videoPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := debugRecordingSourcesReadable(videoPath, audioPath); err == nil {
		t.Fatal("empty video was accepted as a readable source")
	}
	if err := os.WriteFile(videoPath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := debugRecordingSourcesReadable(videoPath, audioPath); err != nil {
		t.Fatalf("readable sources rejected: %v", err)
	}
	if err := debugRecordingSourcesReadable(videoPath, filepath.Join(dir, "missing.wav")); err == nil {
		t.Fatal("missing audio was accepted as a readable source")
	}
}
