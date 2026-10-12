package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/producews"
)

// Convenience for the existing connection-isolation tests. Production starts
// this state under the sender's admission lock, using the wire request ID.
func (a *App) markDebugPlaybackPending(rt *debugSessionRuntime, connID uint64, demoPath string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if a.debugRuntimeForAdmission() != rt || a.produceW.GameConnID() != connID || rt.debugReadyConnID != connID || rt.debugDemoPlayPending {
		return false
	}
	a.startDebugPlaybackPendingLocked(rt, connID, newDebugRequestID(), demoPath)
	return true
}

func TestDebugPlayConfirmationBeforeReceiptDoesNotRearmPending(t *testing.T) {
	app, _, conn := launchDebugPluginSessionForCommandTest(t)
	sessionID := app.GetDebugSessionState().SessionID
	sendDebugReady(t, conn, sessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)
	demoPath := writeManagedDebugDemo(t, app.exeDir, filepath.Join("raw", "early.dem"))
	// The same-path old observation must not bypass the new replay wait.
	app.handleDebugDemoState(producews.DebugIncomingMessage{
		ConnID:  app.produceW.GameConnID(),
		Payload: debugDemoStatePayloadJSON(t, sessionID, demoPath, true, false),
	})
	result := make(chan error, 1)
	go func() {
		_, err := app.PlayDebugDemo(DebugPlayDemoRequest{DemoPath: demoPath})
		result <- err
	}()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var wire debugPlayDemoWire
	if err := conn.ReadJSON(&wire); err != nil {
		t.Fatal(err)
	}
	if !app.GetDebugSessionState().DemoPlayPending {
		t.Fatal("pending was not registered before transmission")
	}
	// Apply confirmation synchronously before releasing the receipt waiter.
	app.handleDebugDemoState(producews.DebugIncomingMessage{
		ConnID:  app.produceW.GameConnID(),
		Payload: debugDemoStatePayloadJSON(t, sessionID, demoPath, true, false),
	})
	if err := conn.WriteJSON(map[string]any{
		"name":    "debug_command_result",
		"payload": map[string]any{"request_id": wire.Payload.RequestID, "status": "executed"},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("play receipt did not complete")
	}
	if app.GetDebugSessionState().DemoPlayPending {
		t.Fatal("receipt rearmed confirmed playback")
	}
}

func TestDebugPlaybackTimeoutPublishesWithoutSnapshotPolling(t *testing.T) {
	oldTimeout, oldEmitter := debugPlayConfirmTimeout, debugSessionEventsEmit
	debugPlayConfirmTimeout = 40 * time.Millisecond
	events := make(chan DebugSessionState, 4)
	debugSessionEventsEmit = func(_ context.Context, name string, data ...interface{}) {
		if name == "debug_session_state_changed" {
			events <- data[0].(DebugSessionState)
		}
	}
	t.Cleanup(func() { debugPlayConfirmTimeout, debugSessionEventsEmit = oldTimeout, oldEmitter })
	app := newTestApp(t, t.TempDir())
	startCommandTestTransport(t, app)
	connID := connectCommandTestTransport(t, app)
	rt := installCommandTestRuntime(t, app, "dbg-timeout-event")
	rt.debugReady, rt.debugReadyConnID = true, connID
	app.ctx = context.Background()
	rt.mu.Lock()
	aTime := rt.updatedAtMs
	app.startDebugPlaybackPendingLocked(rt, connID, "timeout-request", "/target.dem")
	rt.mu.Unlock()
	select {
	case state := <-events:
		if state.DemoPlayPending || state.Error != debugPlayConfirmationTimeoutMessage || state.UpdatedAtMs <= aTime {
			t.Fatalf("unexpected timeout event: %+v", state)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout did not publish a state event")
	}
}

func TestOldPlaybackTimerAndFailureCannotClearRetry(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	startCommandTestTransport(t, app)
	connID := connectCommandTestTransport(t, app)
	rt := installCommandTestRuntime(t, app, "dbg-retry")
	rt.debugReady, rt.debugReadyConnID = true, connID
	rt.mu.Lock()
	app.startDebugPlaybackPendingLocked(rt, connID, "old", "/same.dem")
	clearDebugPlaybackPendingLocked(rt)
	app.startDebugPlaybackPendingLocked(rt, connID, "new", "/same.dem")
	rt.mu.Unlock()
	t.Cleanup(func() { rt.mu.Lock(); clearDebugPlaybackPendingLocked(rt); rt.mu.Unlock() })
	app.onDebugPlaybackTimeout(rt, connID, "old")
	app.finishFailedDebugPlayback(rt, "old")
	if !app.GetDebugSessionState().DemoPlayPending {
		t.Fatal("old completion cleared retry")
	}
	app.finishFailedDebugPlayback(rt, "new")
	if app.GetDebugSessionState().DemoPlayPending {
		t.Fatal("current failure did not clear pending")
	}
}
