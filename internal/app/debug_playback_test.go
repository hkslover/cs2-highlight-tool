package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

func writeDebugDemoState(t *testing.T, conn *websocket.Conn, sessionID, demoPath string, playing bool, tick int) {
	t.Helper()
	if err := conn.WriteJSON(map[string]any{
		"name": "debug_demo_state",
		"payload": map[string]any{
			"session_id": sessionID,
			"demo_path":  demoPath,
			"playing":    playing,
			"tick":       tick,
		},
	}); err != nil {
		t.Fatalf("write debug_demo_state: %v", err)
	}
}

func debugDemoStatePayloadJSON(t *testing.T, sessionID, demoPath string, playing bool, tick int) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"session_id": sessionID,
		"demo_path":  demoPath,
		"playing":    playing,
		"tick":       tick,
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
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	quoted := writeManagedDebugDemo(t, app.exeDir, filepath.Join("raw", `we"ird.dem`))
	if _, err := app.PlayDebugDemo(DebugPlayDemoRequest{DemoPath: quoted}); err == nil {
		t.Fatal("a path that cannot be safely quoted was accepted")
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
		Payload: debugDemoStatePayloadJSON(t, rt.sessionID, "/playing.dem", true, 42),
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
		Payload: debugDemoStatePayloadJSON(t, rt.sessionID, "/finished.dem", false, 100),
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
	writeDebugDemoState(t, conn, sessionID, demoPath, true, 42)
	waitForDebugDemoPlaying(t, app, true)

	state := app.GetDebugSessionState()
	if state.DemoTick != 42 || state.DemoPath != demoPath {
		t.Fatalf("playback state was not applied: %+v", state)
	}

	writeDebugDemoState(t, conn, sessionID, demoPath, false, 500)
	waitForDebugDemoPlaying(t, app, false)
}

func TestDebugDemoStateIgnoresForeignSession(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	writeDebugDemoState(t, conn, "dbg-some-other-session", "/foreign.dem", true, 10)
	time.Sleep(150 * time.Millisecond)
	state := app.GetDebugSessionState()
	if state.DemoPlaying || state.DemoTick == 10 {
		t.Fatalf("a foreign session changed this session's playback state: %+v", state)
	}
}

func TestDebugDemoStateClearedWhenTransportDisconnects(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	writeDebugDemoState(t, conn, sessionID, "/a.dem", true, 5)
	waitForDebugDemoPlaying(t, app, true)

	_ = conn.Close()
	// The plugin that observed playback is gone, so the page must not keep
	// showing a stale "playing".
	waitForDebugDemoPlaying(t, app, false)
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
		Payload: debugDemoStatePayloadJSON(t, rt.sessionID, "/old.dem", true, 99),
	})
	state := app.GetDebugSessionState()
	if state.DemoPlaying || state.DemoTick == 99 {
		t.Fatalf("an old connection changed the playback state: %+v", state)
	}

	app.handleDebugDemoState(producews.DebugIncomingMessage{
		ConnID:  newConnID,
		Payload: debugDemoStatePayloadJSON(t, rt.sessionID, "/new.dem", true, 7),
	})
	state = app.GetDebugSessionState()
	if !state.DemoPlaying || state.DemoTick != 7 || state.DemoPath != "/new.dem" {
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
