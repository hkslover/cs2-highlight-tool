package app

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/producews"

	"github.com/gorilla/websocket"
)

type debugPlayDemoWire struct {
	Name    string `json:"name"`
	Payload struct {
		RequestID string   `json:"request_id"`
		DemoPath  string   `json:"demo_path"`
		Commands  []string `json:"commands"`
	} `json:"payload"`
}

func writeManagedDebugDemo(t *testing.T, root string, rel string) string {
	t.Helper()
	path := filepath.Join(root, "demo", rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir managed demo dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("demo"), 0o644); err != nil {
		t.Fatalf("write managed demo: %v", err)
	}
	return path
}

func writeDebugDemoState(t *testing.T, conn *websocket.Conn, sessionID, demoPath string, playing, paused bool) {
	t.Helper()
	if err := conn.WriteJSON(map[string]any{
		"name": "debug_demo_state",
		"payload": map[string]any{
			"session_id": sessionID,
			"demo_path":  demoPath,
			"playing":    playing,
			"paused":     paused,
		},
	}); err != nil {
		t.Fatalf("write debug_demo_state: %v", err)
	}
}

func debugDemoStatePayloadJSON(t *testing.T, sessionID, demoPath string, playing, paused bool) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"session_id": sessionID,
		"demo_path":  demoPath,
		"playing":    playing,
		"paused":     paused,
	})
	if err != nil {
		t.Fatalf("marshal demo state payload: %v", err)
	}
	return raw
}

func waitForDebugDemoPlaying(t *testing.T, app *App, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if app.GetDebugSessionState().DemoPlaying == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("demo_playing never became %v: %+v", want, app.GetDebugSessionState())
}

func waitForDebugDemoPaused(t *testing.T, app *App, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if app.GetDebugSessionState().DemoPaused == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("demo_paused never became %v: %+v", want, app.GetDebugSessionState())
}

// playDebugDemoAndAck performs one real PlayDebugDemo over the fake plugin
// transport and answers its receipt, so callers can assert the pending playback
// window that follows a receipt.
func playDebugDemoAndAck(t *testing.T, app *App, conn *websocket.Conn, demoPath string) DebugPlayDemoResponse {
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
	if wire.Payload.DemoPath != demoPath {
		t.Fatalf("wire demo_path = %q, want %q", wire.Payload.DemoPath, demoPath)
	}
	if err := conn.WriteJSON(map[string]any{
		"name": "debug_command_result",
		"payload": map[string]any{
			"request_id": wire.Payload.RequestID,
			"status":     "executed",
		},
	}); err != nil {
		t.Fatalf("write debug_command_result: %v", err)
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("PlayDebugDemo: %v", result.err)
		}
		return result.response
	case <-time.After(3 * time.Second):
		t.Fatal("PlayDebugDemo did not return")
		return DebugPlayDemoResponse{}
	}
}

func TestPlayDebugDemoSendsManagedPathAndReceipt(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	demoPath := writeManagedDebugDemo(t, app.exeDir, filepath.Join("raw", "hash", "match with 空格.dem"))

	type callResult struct {
		response DebugPlayDemoResponse
		err      error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		response, err := app.PlayDebugDemo(DebugPlayDemoRequest{
			DemoPath: demoPath,
			Commands: []string{"csdm_radar_pov 1", "  r_drawskybox 0  "},
		})
		resultCh <- callResult{response: response, err: err}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var wire debugPlayDemoWire
	if err := conn.ReadJSON(&wire); err != nil {
		t.Fatalf("read debug_play_demo: %v", err)
	}
	if wire.Name != "debug_play_demo" {
		t.Fatalf("received %q, want debug_play_demo", wire.Name)
	}
	if wire.Payload.DemoPath != demoPath {
		t.Fatalf("demo_path = %q, want %q", wire.Payload.DemoPath, demoPath)
	}
	if len(wire.Payload.Commands) != 2 || wire.Payload.Commands[0] != "csdm_radar_pov 1" || wire.Payload.Commands[1] != "r_drawskybox 0" {
		t.Fatalf("commands were not normalized: %#v", wire.Payload.Commands)
	}

	if err := conn.WriteJSON(map[string]any{
		"name": "debug_command_result",
		"payload": map[string]any{
			"request_id": wire.Payload.RequestID,
			"status":     "executed",
		},
	}); err != nil {
		t.Fatalf("write debug_command_result: %v", err)
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("PlayDebugDemo: %v", result.err)
		}
		if result.response.Status != DebugCommandStatusExecuted {
			t.Fatalf("status = %q", result.response.Status)
		}
		if result.response.DemoPath != demoPath {
			t.Fatalf("response demo path = %q, want %q", result.response.DemoPath, demoPath)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("PlayDebugDemo did not return")
	}

	state := app.GetDebugSessionState()
	if state.DemoPath != demoPath {
		t.Fatalf("snapshot demo path = %q, want %q", state.DemoPath, demoPath)
	}
	if state.DemoPlaying {
		t.Fatal("a play receipt must not claim actual playback")
	}
}

func TestPlayDebugDemoLeavesSequenceSidecarUntouched(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	demoPath := writeManagedDebugDemo(t, app.exeDir, filepath.Join("raw", "match.dem"))
	sidecarPath := demoPath + ".json"
	original := []byte(`[{"actions":[{"tick":10,"cmd":"disconnect"}]}]`)
	if err := os.WriteFile(sidecarPath, original, 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

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
		t.Fatalf("write debug_command_result: %v", err)
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("PlayDebugDemo: %v", result.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("PlayDebugDemo did not return")
	}

	got, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("debug playback removed the sequence sidecar: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("debug playback rewrote the sequence sidecar: %q", got)
	}
}

func TestPlayDebugDemoRejectsUnmanagedPath(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	outside := writeDemoFile(t)
	if _, err := app.PlayDebugDemo(DebugPlayDemoRequest{DemoPath: outside}); err == nil {
		t.Fatal("an unmanaged demo path was accepted")
	}
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var raw json.RawMessage
	if err := conn.ReadJSON(&raw); err == nil {
		t.Fatal("a rejected play request still sent a message")
	}
}

func TestPlayDebugDemoRejectsMissingFile(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	missing := filepath.Join(app.exeDir, "demo", "raw", "gone.dem")
	if _, err := app.PlayDebugDemo(DebugPlayDemoRequest{DemoPath: missing}); err == nil {
		t.Fatal("a missing demo file was accepted")
	}
}

func TestPlayDebugDemoRejectsPathThatCannotBeQuoted(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows filenames cannot contain a double quote, so the fixture path
		// cannot be created there. TestValidateDebugPlaybackPathRejectsUnquotablePaths
		// covers the validator on every platform.
		t.Skip("Windows filenames cannot contain a double quote")
	}
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	quoted := writeManagedDebugDemo(t, app.exeDir, filepath.Join("raw", `we"ird.dem`))
	if _, err := app.PlayDebugDemo(DebugPlayDemoRequest{DemoPath: quoted}); err == nil {
		t.Fatal("a path that cannot be safely quoted was accepted")
	}
}

// TestValidateDebugPlaybackPathRejectsUnquotablePaths exercises the quoting
// guard directly so the check stays covered on Windows, where the file-based
// test above cannot create its fixture.
func TestValidateDebugPlaybackPathRejectsUnquotablePaths(t *testing.T) {
	for _, path := range []string{`we"ird.dem`, "line\nbreak.dem", "carriage\rreturn.dem", "bell\x07.dem"} {
		if err := validateDebugPlaybackPath(path); err == nil {
			t.Fatalf("path %q was accepted although it cannot be safely quoted", path)
		}
	}
	if err := validateDebugPlaybackPath(filepath.Join("raw", "normal.dem")); err != nil {
		t.Fatalf("a normal path was rejected: %v", err)
	}
}

func TestPlayDebugDemoRequiresReadyPlugin(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	demoPath := writeManagedDebugDemo(t, app.exeDir, filepath.Join("raw", "a.dem"))

	// Before the handshake the request must be refused with a visible error.
	if _, err := app.PlayDebugDemo(DebugPlayDemoRequest{DemoPath: demoPath}); err == nil {
		t.Fatal("play was accepted before the debug protocol was ready")
	}

	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var raw json.RawMessage
	if err := conn.ReadJSON(&raw); err == nil {
		t.Fatal("a refused play request still sent a message")
	}
}

func TestDebugPlayReceiptStaysBoundToRuntimeAndConnection(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	startCommandTestTransport(t, app)
	connID := connectCommandTestTransport(t, app)
	old := installCommandTestRuntime(t, app, "dbg-old")
	old.debugReady = true
	old.debugReadyConnID = connID
	if !app.recordDebugDemoRequest(old, connID, 0, "/old.dem") {
		t.Fatal("current receipt was not recorded")
	}
	current := installCommandTestRuntime(t, app, "dbg-new")
	current.debugReady = true
	current.debugReadyConnID = connID
	if app.recordDebugDemoRequest(old, connID, 0, "/late.dem") {
		t.Fatal("receipt from replaced runtime was accepted")
	}
	if current.debugDemoPath != "" {
		t.Fatalf("old receipt polluted new runtime: %q", current.debugDemoPath)
	}
	if _, err := app.sendDebugRequestOnRuntime(old, connID, "debug_play_demo", func(string) any {
		t.Fatal("request from replaced runtime was sent")
		return nil
	}); err == nil {
		t.Fatal("replaced runtime was admitted")
	}
	newConnID := connectCommandTestTransport(t, app)
	current.debugReadyConnID = newConnID
	if app.recordDebugDemoRequest(current, connID, 0, "/old-connection.dem") {
		t.Fatal("receipt from replaced connection was accepted")
	}
	close(current.done)
	if app.recordDebugDemoRequest(current, newConnID, 0, "/closed.dem") {
		t.Fatal("receipt from closed runtime was accepted")
	}
}

func TestDebugPlayReceiptCannotRelabelOrOverwriteObservedPlayback(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	startCommandTestTransport(t, app)
	connID := connectCommandTestTransport(t, app)
	rt := installCommandTestRuntime(t, app, "dbg-observed")
	rt.debugReady = true
	rt.debugReadyConnID = connID
	app.handleDebugDemoState(producews.DebugIncomingMessage{
		ConnID:  connID,
		Payload: debugDemoStatePayloadJSON(t, rt.sessionID, "/playing.dem", true, false),
	})
	if app.recordDebugDemoRequest(rt, connID, rt.debugDemoRevision, "/requested.dem") {
		t.Fatal("receipt relabeled the previous Demo as the requested one")
	}
	if rt.debugDemoPath != "/playing.dem" || !rt.debugDemoPlaying {
		t.Fatalf("receipt changed actual playback: %q, %v", rt.debugDemoPath, rt.debugDemoPlaying)
	}
	revision := rt.debugDemoRevision
	app.handleDebugDemoState(producews.DebugIncomingMessage{
		ConnID:  connID,
		Payload: debugDemoStatePayloadJSON(t, rt.sessionID, "/finished.dem", false, false),
	})
	if app.recordDebugDemoRequest(rt, connID, revision, "/late.dem") {
		t.Fatal("late receipt overwrote a newer stop observation")
	}
	if rt.debugDemoPath != "/finished.dem" || rt.debugDemoPlaying {
		t.Fatal("newer playback observation was lost")
	}
}

func TestDebugDemoStateUpdatesPlayingState(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	demoPath := writeManagedDebugDemo(t, app.exeDir, filepath.Join("raw", "a.dem"))
	writeDebugDemoState(t, conn, sessionID, demoPath, true, false)
	waitForDebugDemoPlaying(t, app, true)

	state := app.GetDebugSessionState()
	if state.DemoPath != demoPath || state.DemoPaused {
		t.Fatalf("playback state was not applied: %+v", state)
	}

	// Pause is an engine fact reported by the plugin; the host stores it
	// without flipping any local button state.
	writeDebugDemoState(t, conn, sessionID, demoPath, true, true)
	waitForDebugDemoPaused(t, app, true)
	if !app.GetDebugSessionState().DemoPlaying {
		t.Fatal("a paused Demo must still count as loaded and playing")
	}

	writeDebugDemoState(t, conn, sessionID, demoPath, true, false)
	waitForDebugDemoPaused(t, app, false)

	writeDebugDemoState(t, conn, sessionID, demoPath, false, false)
	waitForDebugDemoPlaying(t, app, false)
}

// The host must tolerate an observation from an older DLL that still carries
// the removed tick field and has no paused field: playback is applied and the
// pause stays false instead of being inferred.
func TestDebugDemoStateIgnoresLegacyTickField(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	startCommandTestTransport(t, app)
	connID := connectCommandTestTransport(t, app)
	rt := installCommandTestRuntime(t, app, "dbg-legacy-tick")
	rt.debugReady = true
	rt.debugReadyConnID = connID

	app.handleDebugDemoState(producews.DebugIncomingMessage{
		ConnID: connID,
		Payload: []byte(`{"session_id":"` + rt.sessionID +
			`","demo_path":"/legacy.dem","playing":true,"tick":1234}`),
	})
	state := app.GetDebugSessionState()
	if !state.DemoPlaying || state.DemoPath != "/legacy.dem" {
		t.Fatalf("a legacy observation was dropped: %+v", state)
	}
	if state.DemoPaused {
		t.Fatalf("a missing pause field was treated as a verified pause: %+v", state)
	}
}

// A stopped playback can never keep a stale paused flag, even when a message
// still carries paused=true.
func TestDebugDemoStateClearsPausedWhenPlaybackStops(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	startCommandTestTransport(t, app)
	connID := connectCommandTestTransport(t, app)
	rt := installCommandTestRuntime(t, app, "dbg-paused-stop")
	rt.debugReady = true
	rt.debugReadyConnID = connID

	app.handleDebugDemoState(producews.DebugIncomingMessage{
		ConnID:  connID,
		Payload: debugDemoStatePayloadJSON(t, rt.sessionID, "/paused.dem", true, true),
	})
	if !rt.debugDemoPaused {
		t.Fatal("engine pause was not recorded")
	}

	app.handleDebugDemoState(producews.DebugIncomingMessage{
		ConnID:  connID,
		Payload: debugDemoStatePayloadJSON(t, rt.sessionID, "/paused.dem", false, true),
	})
	state := app.GetDebugSessionState()
	if state.DemoPlaying || state.DemoPaused {
		t.Fatalf("a stopped Demo kept a playback flag: %+v", state)
	}
}

func TestDebugDemoStateIgnoresForeignSession(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	writeDebugDemoState(t, conn, "dbg-some-other-session", "/foreign.dem", true, true)
	time.Sleep(150 * time.Millisecond)
	state := app.GetDebugSessionState()
	if state.DemoPlaying || state.DemoPaused || state.DemoPath != "" {
		t.Fatalf("a foreign session changed this session's playback state: %+v", state)
	}
}

func TestDebugDemoStateClearedWhenTransportDisconnects(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	writeDebugDemoState(t, conn, sessionID, "/a.dem", true, true)
	waitForDebugDemoPlaying(t, app, true)
	waitForDebugDemoPaused(t, app, true)

	_ = conn.Close()
	// The plugin that observed playback is gone, so the page must not keep
	// showing a stale "playing" or a stale pause.
	waitForDebugDemoPlaying(t, app, false)
	waitForDebugDemoPaused(t, app, false)
}

func TestDebugDemoStateRequiresCurrentConnection(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	startCommandTestTransport(t, app)
	rt := installCommandTestRuntime(t, app, "dbg-demo-state")
	oldConnID := connectCommandTestTransport(t, app)
	app.handleDebugSessionReady(producews.DebugIncomingMessage{
		ConnID: oldConnID, Payload: debugReadyPayloadJSON(t, rt.sessionID, DebugProtocolVersion),
	})
	newConnID := connectCommandTestTransport(t, app)

	// A state decoded on the replaced connection must not change the session.
	app.handleDebugDemoState(producews.DebugIncomingMessage{
		ConnID:  oldConnID,
		Payload: debugDemoStatePayloadJSON(t, rt.sessionID, "/old.dem", true, true),
	})
	state := app.GetDebugSessionState()
	if state.DemoPlaying || state.DemoPaused || state.DemoPath != "" {
		t.Fatalf("an old connection changed the playback state: %+v", state)
	}

	app.handleDebugDemoState(producews.DebugIncomingMessage{
		ConnID:  newConnID,
		Payload: debugDemoStatePayloadJSON(t, rt.sessionID, "/new.dem", true, true),
	})
	state = app.GetDebugSessionState()
	if !state.DemoPlaying || !state.DemoPaused || state.DemoPath != "/new.dem" {
		t.Fatalf("current connection state was not applied: %+v", state)
	}
}

func TestResolveManagedDebugDemoPathRejectsNonDemoExtension(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	path := writeManagedDebugDemo(t, app.exeDir, filepath.Join("raw", "notes.txt"))
	if _, err := app.resolveManagedDebugDemoPath(path); err == nil {
		t.Fatal("a non-.dem file was accepted")
	}
}

func TestResolveManagedDebugDemoPathRejectsDirectory(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	dir := filepath.Join(app.exeDir, "demo", "raw", "subdir.dem")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir demo dir: %v", err)
	}
	if _, err := app.resolveManagedDebugDemoPath(dir); err == nil {
		t.Fatal("a directory was accepted as a demo file")
	}
}

// captureDirectoryOpen replaces the file-manager launch with a recorder so the
// test asserts the exact directory OpenDemoInFolder would open.
func captureDirectoryOpen(t *testing.T, opened *[]string) {
	t.Helper()
	previous := startDirectoryOpenFn
	startDirectoryOpenFn = func(cmd *exec.Cmd) error {
		*opened = append(*opened, cmd.Args[len(cmd.Args)-1])
		return nil
	}
	t.Cleanup(func() { startDirectoryOpenFn = previous })
}

func TestOpenDemoInFolderOpensParentDirectory(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	path := writeManagedDebugDemo(t, app.exeDir, filepath.Join("wanmei", "中文 空格 比赛", "match.dem"))

	var opened []string
	captureDirectoryOpen(t, &opened)
	if err := app.OpenDemoInFolder(path); err != nil {
		t.Fatalf("OpenDemoInFolder: %v", err)
	}
	if len(opened) != 1 || opened[0] != filepath.Dir(path) {
		t.Fatalf("unexpected opened directories: %#v (want %q)", opened, filepath.Dir(path))
	}

	// A stale or hand-crafted path outside the managed Demo directory, a
	// missing file, a non-.dem file and an empty selection must all be refused
	// without opening anything.
	outside := writeManagedDebugDemo(t, t.TempDir(), filepath.Join("raw", "outside.dem"))
	missing := filepath.Join(app.exeDir, "demo", "raw", "missing.dem")
	notDemo := writeManagedDebugDemo(t, app.exeDir, filepath.Join("raw", "notes.txt"))
	for _, target := range []string{outside, missing, notDemo, "   "} {
		if err := app.OpenDemoInFolder(target); err == nil {
			t.Fatalf("OpenDemoInFolder accepted %q", target)
		}
	}
	if len(opened) != 1 {
		t.Fatalf("a rejected path opened a directory: %#v", opened)
	}
}

// A Demo entry inside the managed tree may itself be a link that leaves the
// workspace; both the folder action and playback validation must reject the
// resolved escape while keeping links that stay inside usable.
func TestManagedDemoPathsRejectLinkedEscapes(t *testing.T) {
	app := newTestApp(t, t.TempDir())

	outsideDir := filepath.Join(t.TempDir(), "outside 目录")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside dir: %v", err)
	}
	outsideDemo := filepath.Join(outsideDir, "match.dem")
	if err := os.WriteFile(outsideDemo, []byte("demo"), 0o644); err != nil {
		t.Fatalf("write outside demo: %v", err)
	}

	demoRoot := filepath.Join(app.exeDir, "demo")
	insideDir := filepath.Join(demoRoot, "raw", "inside")
	if err := os.MkdirAll(insideDir, 0o755); err != nil {
		t.Fatalf("mkdir managed demo dir: %v", err)
	}
	insideDemo := filepath.Join(insideDir, "inside.dem")
	if err := os.WriteFile(insideDemo, []byte("demo"), 0o644); err != nil {
		t.Fatalf("write managed demo: %v", err)
	}

	fileLink := filepath.Join(demoRoot, "linked.dem")
	if err := os.Symlink(outsideDemo, fileLink); err != nil {
		t.Skipf("symlinks are unavailable on this platform: %v", err)
	}
	dirLink := filepath.Join(demoRoot, "linked-dir")
	if err := os.Symlink(outsideDir, dirLink); err != nil {
		t.Fatalf("create directory symlink: %v", err)
	}
	insideLink := filepath.Join(demoRoot, "inside-link.dem")
	if err := os.Symlink(insideDemo, insideLink); err != nil {
		t.Fatalf("create managed file symlink: %v", err)
	}

	var opened []string
	captureDirectoryOpen(t, &opened)
	for _, target := range []string{fileLink, filepath.Join(dirLink, "match.dem")} {
		if err := app.OpenDemoInFolder(target); err == nil {
			t.Fatalf("OpenDemoInFolder accepted a linked path outside the workspace: %s", target)
		}
		if _, err := app.resolveManagedDebugDemoPath(target); err == nil {
			t.Fatalf("playback validation accepted a linked path outside the workspace: %s", target)
		}
	}
	if len(opened) != 0 {
		t.Fatalf("a rejected linked path opened a directory: %#v", opened)
	}

	// A link that resolves inside the managed Demo root is still usable.
	if err := app.OpenDemoInFolder(insideLink); err != nil {
		t.Fatalf("OpenDemoInFolder rejected an inside link: %v", err)
	}
	if len(opened) != 1 || opened[0] != demoRoot {
		t.Fatalf("unexpected opened directories: %#v (want %q)", opened, demoRoot)
	}
}

// A play receipt only proves the plugin queued `playdemo`. Until an engine
// observation of the requested Demo arrives, the pause/resume command must stay
// disabled because the last observation may still describe the previous Demo.
func TestDebugPlayReceiptStaysPendingUntilEngineConfirmsTarget(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)
	connID := app.produceW.GameConnID()

	previousPath := writeManagedDebugDemo(t, app.exeDir, filepath.Join("raw", "previous.dem"))
	targetPath := writeManagedDebugDemo(t, app.exeDir, filepath.Join("raw", "target.dem"))

	writeDebugDemoState(t, conn, sessionID, previousPath, true, false)
	waitForDebugDemoPlaying(t, app, true)

	playDebugDemoAndAck(t, app, conn, targetPath)
	if !app.GetDebugSessionState().DemoPlayPending {
		t.Fatalf("a play receipt did not mark the request pending: %+v", app.GetDebugSessionState())
	}

	// An observation of the previous Demo (here its pause state changed) must
	// not clear the pending request.
	app.handleDebugDemoState(producews.DebugIncomingMessage{
		ConnID:  connID,
		Payload: debugDemoStatePayloadJSON(t, sessionID, previousPath, true, true),
	})
	if !app.GetDebugSessionState().DemoPlayPending {
		t.Fatal("an observation of the previous Demo cleared the pending play request")
	}

	// Only an observation of the requested Demo playing clears it.
	app.handleDebugDemoState(producews.DebugIncomingMessage{
		ConnID:  connID,
		Payload: debugDemoStatePayloadJSON(t, sessionID, targetPath, true, false),
	})
	state := app.GetDebugSessionState()
	if state.DemoPlayPending {
		t.Fatalf("engine confirmation did not clear the pending play request: %+v", state)
	}
	if !state.DemoPlaying || state.DemoPath != targetPath {
		t.Fatalf("engine confirmation was not applied: %+v", state)
	}
}

// The pending window is bounded: a play request that never gets confirmed must
// not disable pause/resume forever.
func TestDebugPlayPendingExpiresWithoutConfirmation(t *testing.T) {
	previousTimeout := debugPlayConfirmTimeout
	debugPlayConfirmTimeout = 60 * time.Millisecond
	t.Cleanup(func() { debugPlayConfirmTimeout = previousTimeout })

	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	demoPath := writeManagedDebugDemo(t, app.exeDir, filepath.Join("raw", "expire.dem"))
	playDebugDemoAndAck(t, app, conn, demoPath)
	if !app.GetDebugSessionState().DemoPlayPending {
		t.Fatal("play receipt did not mark the request pending")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && app.GetDebugSessionState().DemoPlayPending {
		time.Sleep(5 * time.Millisecond)
	}
	if app.GetDebugSessionState().DemoPlayPending {
		t.Fatal("the pending play request never expired")
	}
}

func TestDebugPlayPendingClearedWhenTransportDrops(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	demoPath := writeManagedDebugDemo(t, app.exeDir, filepath.Join("raw", "pending.dem"))
	playDebugDemoAndAck(t, app, conn, demoPath)
	if !app.GetDebugSessionState().DemoPlayPending {
		t.Fatal("play receipt did not mark the request pending")
	}

	_ = conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !app.GetDebugSessionState().DemoPlayPending {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the pending play request survived the transport disconnect")
}

// Only the connection that sent the request may mark it pending; a stale
// receipt (or a runtime that is no longer current) must not disable the button
// for the live session.
func TestDebugPlayPendingOnlyTracksSendingConnection(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	startCommandTestTransport(t, app)
	connID := connectCommandTestTransport(t, app)
	rt := installCommandTestRuntime(t, app, "dbg-pending-conn")
	rt.debugReady = true
	rt.debugReadyConnID = connID

	if app.markDebugPlaybackPending(rt, connID+1, "/stale.dem") {
		t.Fatal("a receipt from another connection marked the request pending")
	}
	if app.GetDebugSessionState().DemoPlayPending {
		t.Fatal("a stale receipt changed the pending state")
	}

	if !app.markDebugPlaybackPending(rt, connID, "/live.dem") {
		t.Fatal("the sending connection did not mark the request pending")
	}
	state := app.GetDebugSessionState()
	if !state.DemoPlayPending {
		t.Fatalf("pending play request was not reported: %+v", state)
	}
}
