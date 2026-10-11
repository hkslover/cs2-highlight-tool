package app

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/producews"

	"github.com/gorilla/websocket"
)

// startDebugPluginSession launches a plugin-mode debug session backed by a real
// produce websocket service. It does not connect a fake plugin, so callers can
// observe the first-ready timeout before any transport exists.
func startDebugPluginSession(t *testing.T, wireEmitter bool) (*App, *producews.Service) {
	t.Helper()
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	_, _ = prepareLaunchTestEnvironment(t, app.exeDir)
	overrideDebugSeams(t)

	produceW := producews.NewDefault(nil)
	if err := produceW.Start(); err != nil {
		t.Fatalf("Start produce websocket: %v", err)
	}
	t.Cleanup(func() { _ = produceW.Stop() })
	app.produceW = produceW
	if wireEmitter {
		// Production wires this in App.Startup; tests replace produceW directly,
		// so reinstall the transport->debug-snapshot refresh here.
		produceW.SetEmitter(func(name string, _ any) {
			if name == "produce_ws_state_changed" {
				app.refreshDebugPluginState()
			}
		})
	}

	launchHLAECommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=TestHelperProcessLaunchHLAE", "--")
	}
	listCalls := 0
	listCS2PIDsFn = func() ([]int, error) {
		listCalls++
		if listCalls == 1 {
			return nil, nil
		}
		return []int{9000 + listCalls}, nil
	}
	closeCS2ProcessByPIDFn = func(pid int) error { return nil }

	if _, err := app.LaunchDebugGame(DebugLaunchRequest{
		InjectPlugin:     true,
		BrandEnabled:     true,
		LaunchResolution: config.LaunchResolution4x3,
	}); err != nil {
		t.Fatalf("LaunchDebugGame: %v", err)
	}
	t.Cleanup(func() { _, _ = app.StopDebugGame() })
	return app, produceW
}

func dialDebugPlugin(t *testing.T, addr string) *websocket.Conn {
	t.Helper()
	u := url.URL{Scheme: "ws", Host: addr, Path: "/", RawQuery: "process=game"}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("connect fake plugin: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// launchDebugPluginSessionForCommandTest starts a plugin-mode debug session and
// returns the fake plugin connection. It mirrors the production wiring closely
// enough to exercise the ready handshake and receipt correlation.
func launchDebugPluginSessionForCommandTest(t *testing.T) (*App, *producews.Service, *websocket.Conn) {
	t.Helper()
	app, produceW := startDebugPluginSession(t, true)
	return app, produceW, dialDebugPlugin(t, produceW.Address())
}

func sendDebugReady(t *testing.T, conn *websocket.Conn, sessionID string, protocolVersion int) {
	t.Helper()
	if err := conn.WriteJSON(map[string]any{
		"name": "debug_session_ready",
		"payload": map[string]any{
			"session_id":       sessionID,
			"protocol_version": protocolVersion,
		},
	}); err != nil {
		t.Fatalf("write debug_session_ready: %v", err)
	}
}

func waitForPluginReady(t *testing.T, app *App) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if app.GetDebugSessionState().PluginReady {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("plugin never became ready: %+v", app.GetDebugSessionState())
}

type debugExecuteWire struct {
	Name    string `json:"name"`
	Payload struct {
		RequestID string   `json:"request_id"`
		Commands  []string `json:"commands"`
	} `json:"payload"`
}

func TestSendDebugConsoleCommandRoundTrip(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID

	// Before the handshake the command must be refused with a visible error.
	if _, err := app.SendDebugConsoleCommand(DebugCommandRequest{Commands: []string{"sv_cheats 1"}}); err == nil {
		t.Fatal("command was accepted before the debug protocol was ready")
	}

	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	type callResult struct {
		response DebugCommandResponse
		err      error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		response, err := app.SendDebugConsoleCommand(DebugCommandRequest{
			Commands: []string{"sv_cheats 1; mirv_streams record start"},
		})
		resultCh <- callResult{response: response, err: err}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var command debugExecuteWire
	if err := conn.ReadJSON(&command); err != nil {
		t.Fatalf("read debug_execute: %v", err)
	}
	if command.Name != "debug_execute" {
		t.Fatalf("received %q, want debug_execute", command.Name)
	}
	if command.Payload.RequestID == "" {
		t.Fatal("debug_execute carried no request id")
	}
	if len(command.Payload.Commands) != 1 || command.Payload.Commands[0] != "sv_cheats 1; mirv_streams record start" {
		t.Fatalf("commands were not preserved as one element: %#v", command.Payload.Commands)
	}

	if err := conn.WriteJSON(map[string]any{
		"name": "debug_command_result",
		"payload": map[string]any{
			"request_id": command.Payload.RequestID,
			"status":     "executed",
		},
	}); err != nil {
		t.Fatalf("write debug_command_result: %v", err)
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("SendDebugConsoleCommand: %v", result.err)
		}
		if result.response.Status != DebugCommandStatusExecuted {
			t.Fatalf("status = %q", result.response.Status)
		}
		if result.response.RequestID != command.Payload.RequestID {
			t.Fatalf("request id = %q, want %q", result.response.RequestID, command.Payload.RequestID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SendDebugConsoleCommand did not return")
	}
}

func TestSendDebugConsoleCommandIgnoresForeignSessionReady(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)

	sendDebugReady(t, conn, "dbg-some-other-session", DebugProtocolVersion)
	time.Sleep(100 * time.Millisecond)
	if app.GetDebugSessionState().PluginReady {
		t.Fatal("a foreign session id was accepted as this session's readiness")
	}

	if _, err := app.SendDebugConsoleCommand(DebugCommandRequest{Commands: []string{"quit"}}); err == nil {
		t.Fatal("command was accepted for a foreign ready session")
	}
}

func TestSendDebugConsoleCommandRejectsUnsupportedProtocol(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID

	sendDebugReady(t, conn, sessionID, DebugProtocolVersion+1)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !app.GetDebugSessionState().PluginReady {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if app.GetDebugSessionState().PluginReady {
		t.Fatal("an unsupported protocol version was accepted")
	}
	if _, err := app.SendDebugConsoleCommand(DebugCommandRequest{Commands: []string{"quit"}}); err == nil {
		t.Fatal("command was accepted for an unsupported protocol")
	}
}

func TestSendDebugConsoleCommandFailsPendingOnDisconnect(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	type callResult struct {
		response DebugCommandResponse
		err      error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		response, err := app.SendDebugConsoleCommand(DebugCommandRequest{Commands: []string{"quit"}})
		resultCh <- callResult{response: response, err: err}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var command debugExecuteWire
	if err := conn.ReadJSON(&command); err != nil {
		t.Fatalf("read debug_execute: %v", err)
	}
	// Drop the transport before replying; the pending request must fail rather
	// than wait for the full timeout.
	_ = conn.Close()

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("unexpected transport error: %v", result.err)
		}
		if result.response.Status != DebugCommandStatusFailed {
			t.Fatalf("status = %q, want failed", result.response.Status)
		}
		if result.response.Error == "" {
			t.Fatal("disconnect failure carried no error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending command was not failed on disconnect")
	}
}

func TestSendDebugConsoleCommandTimesOutWithoutResend(t *testing.T) {
	oldTimeout := debugCommandTimeout
	debugCommandTimeout = 250 * time.Millisecond
	t.Cleanup(func() { debugCommandTimeout = oldTimeout })

	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)

	if _, err := app.SendDebugConsoleCommand(DebugCommandRequest{Commands: []string{"quit"}}); err == nil {
		t.Fatal("expected a timeout error")
	}

	// The command was sent exactly once; a timeout must not trigger a resend.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var command debugExecuteWire
	if err := conn.ReadJSON(&command); err != nil {
		t.Fatalf("read debug_execute: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var extra json.RawMessage
	if err := conn.ReadJSON(&extra); err == nil {
		t.Fatal("timeout resent the command")
	}
}

func TestSendDebugConsoleCommandRejectsNoPluginSession(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	_, _ = prepareLaunchTestEnvironment(t, app.exeDir)
	overrideDebugSeams(t)

	launchHLAECommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=TestHelperProcessLaunchHLAE", "--")
	}
	listCalls := 0
	listCS2PIDsFn = func() ([]int, error) {
		listCalls++
		if listCalls == 1 {
			return nil, nil
		}
		return []int{9500}, nil
	}
	closeCS2ProcessByPIDFn = func(pid int) error { return nil }

	if _, err := app.LaunchDebugGame(DebugLaunchRequest{InjectPlugin: false, BrandEnabled: false}); err != nil {
		t.Fatalf("LaunchDebugGame: %v", err)
	}
	defer func() { _, _ = app.StopDebugGame() }()

	if _, err := app.SendDebugConsoleCommand(DebugCommandRequest{Commands: []string{"quit"}}); err == nil {
		t.Fatal("command was accepted in a no-plugin session")
	}
}

func TestNormalizeDebugCommands(t *testing.T) {
	got := normalizeDebugCommands([]string{"  sv_cheats 1  ", "", "   ", "mirv_streams record start"})
	want := []string{"sv_cheats 1", "mirv_streams record start"}
	if len(got) != len(want) {
		t.Fatalf("normalized %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalized[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if len(normalizeDebugCommands([]string{" ", "\t"})) != 0 {
		t.Fatal("whitespace-only input should normalize to no commands")
	}
}

func debugReadyPayloadJSON(t *testing.T, sessionID string, protocolVersion int) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"session_id":       sessionID,
		"protocol_version": protocolVersion,
	})
	if err != nil {
		t.Fatalf("marshal ready payload: %v", err)
	}
	return raw
}

func debugResultPayloadJSON(t *testing.T, requestID string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"request_id": requestID,
		"status":     "executed",
	})
	if err != nil {
		t.Fatalf("marshal result payload: %v", err)
	}
	return raw
}

// installCommandTestRuntime installs a bare runtime so the connection-binding
// rules can be exercised without a websocket.
func installCommandTestRuntime(t *testing.T, app *App, sessionID string) *debugSessionRuntime {
	t.Helper()
	rt := &debugSessionRuntime{
		sessionID: sessionID,
		request:   DebugLaunchRequest{InjectPlugin: true},
		done:      make(chan struct{}),
		status:    DebugStatusRunning,
	}
	app.installDebugRuntime(rt)
	t.Cleanup(func() {
		app.debugStateMu.Lock()
		if app.debugRuntime == rt {
			app.debugRuntime = nil
		}
		app.debugStateMu.Unlock()
	})
	return rt
}

func TestDebugCommandResultMustMatchConnection(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	startCommandTestTransport(t, app)
	connID := connectCommandTestTransport(t, app)
	rt := installCommandTestRuntime(t, app, "dbg-conn")
	waiter := make(chan DebugCommandResponse, 1)
	rt.mu.Lock()
	rt.debugReady = true
	rt.debugReadyConnID = connID
	rt.debugPending = map[string]*debugPendingCommand{
		"req-1": {connID: connID, waiter: waiter},
	}
	rt.mu.Unlock()

	// A receipt on a different connection must not satisfy the request.
	app.handleDebugCommandResult(producews.DebugIncomingMessage{
		Name:    "debug_command_result",
		ConnID:  connID + 1,
		Payload: debugResultPayloadJSON(t, "req-1"),
	})
	select {
	case <-waiter:
		t.Fatal("a receipt from another connection satisfied the request")
	default:
	}

	app.handleDebugCommandResult(producews.DebugIncomingMessage{
		Name:    "debug_command_result",
		ConnID:  connID,
		Payload: debugResultPayloadJSON(t, "req-1"),
	})
	select {
	case result := <-waiter:
		if result.Status != DebugCommandStatusExecuted {
			t.Fatalf("status = %q", result.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("the matching connection receipt was not delivered")
	}
}

func TestDebugSessionReadyOnNewConnectionFailsOldPending(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	startCommandTestTransport(t, app)
	oldConnID := connectCommandTestTransport(t, app)
	rt := installCommandTestRuntime(t, app, "dbg-reconnect")
	waiter := make(chan DebugCommandResponse, 1)
	rt.mu.Lock()
	rt.debugReady = true
	rt.debugReadyConnID = oldConnID
	rt.debugPending = map[string]*debugPendingCommand{
		"req-old": {connID: oldConnID, waiter: waiter},
	}
	rt.mu.Unlock()
	newConnID := connectCommandTestTransport(t, app)

	// The new handshake arrives before any asynchronous transport refresh; it
	// must fail the request bound to the replaced connection immediately.
	app.handleDebugSessionReady(producews.DebugIncomingMessage{
		Name:    "debug_session_ready",
		ConnID:  newConnID,
		Payload: debugReadyPayloadJSON(t, "dbg-reconnect", DebugProtocolVersion),
	})

	select {
	case result := <-waiter:
		if result.Status != DebugCommandStatusFailed || result.Error == "" {
			t.Fatalf("old request was not failed: %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("old request kept waiting after the connection was replaced")
	}
	rt.mu.Lock()
	readyConnID := rt.debugReadyConnID
	ready := rt.debugReady
	rt.mu.Unlock()
	if !ready || readyConnID != newConnID {
		t.Fatalf("ready state = %v conn = %d, want ready on conn %d", ready, readyConnID, newConnID)
	}
}

// Keep the transport emitter detached so tests can deliver messages in the
// ordering that races with asynchronous connection-state refresh in production.
func startCommandTestTransport(t *testing.T, app *App) {
	t.Helper()
	service := producews.NewDefault(nil)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	app.produceW = service
	t.Cleanup(func() { _ = service.Stop() })
}

func connectCommandTestTransport(t *testing.T, app *App) uint64 {
	t.Helper()
	previous := app.produceW.GameConnID()
	dialDebugPlugin(t, app.produceW.Address())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if id := app.produceW.GameConnID(); id != 0 && id != previous {
			return id
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("new plugin connection was not admitted")
	return 0
}

func TestDebugDelayedOldMessagesCannotReplaceNewConnection(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	startCommandTestTransport(t, app)
	rt := installCommandTestRuntime(t, app, "dbg-delayed")
	oldConnID := connectCommandTestTransport(t, app)
	app.handleDebugSessionReady(producews.DebugIncomingMessage{
		ConnID: oldConnID, Payload: debugReadyPayloadJSON(t, rt.sessionID, DebugProtocolVersion),
	})
	newConnID := connectCommandTestTransport(t, app)
	app.handleDebugSessionReady(producews.DebugIncomingMessage{
		ConnID: newConnID, Payload: debugReadyPayloadJSON(t, rt.sessionID, DebugProtocolVersion),
	})
	currentWaiter := make(chan DebugCommandResponse, 1)
	oldWaiter := make(chan DebugCommandResponse, 1)
	rt.mu.Lock()
	rt.debugPending = map[string]*debugPendingCommand{
		"req-current": {connID: newConnID, waiter: currentWaiter},
		"req-old":     {connID: oldConnID, waiter: oldWaiter},
	}
	rt.mu.Unlock()
	// A decoded old handshake can resume after the new one; neither valid nor
	// unsupported protocol versions may overwrite current readiness or requests.
	for _, version := range []int{DebugProtocolVersion, DebugProtocolVersion + 1} {
		app.handleDebugSessionReady(producews.DebugIncomingMessage{
			ConnID: oldConnID, Payload: debugReadyPayloadJSON(t, rt.sessionID, version),
		})
	}
	app.handleDebugCommandResult(producews.DebugIncomingMessage{
		ConnID: oldConnID, Payload: debugResultPayloadJSON(t, "req-old"),
	})
	state := app.GetDebugSessionState()
	if !state.PluginReady {
		t.Fatalf("old messages destroyed current readiness: %+v", state)
	}
	for _, waiter := range []chan DebugCommandResponse{currentWaiter, oldWaiter} {
		select {
		case result := <-waiter:
			t.Fatalf("old message changed a pending request: %+v", result)
		default:
		}
	}
	app.handleDebugCommandResult(producews.DebugIncomingMessage{
		ConnID: newConnID, Payload: debugResultPayloadJSON(t, "req-current"),
	})
	select {
	case result := <-currentWaiter:
		if result.Status != DebugCommandStatusExecuted {
			t.Fatalf("current receipt failed: %+v", result)
		}
	default:
		t.Fatal("current receipt was not delivered")
	}
}

func TestDebugReadyTimeoutActivelyPublishes(t *testing.T) {
	old := debugReadyTimeout
	debugReadyTimeout = 80 * time.Millisecond
	t.Cleanup(func() { debugReadyTimeout = old })

	app, _ := startDebugPluginSession(t, true)
	before := app.GetDebugSessionState().UpdatedAtMs

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if app.GetDebugSessionState().UpdatedAtMs > before {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	state := app.GetDebugSessionState()
	if state.UpdatedAtMs <= before {
		t.Fatal("the first-ready timeout did not actively update the snapshot")
	}
	if state.PluginMessage != "插件未连接或不支持调试协议" {
		t.Fatalf("plugin message = %q, want the first-ready timeout", state.PluginMessage)
	}
}
