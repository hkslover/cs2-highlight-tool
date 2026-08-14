package producews

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type wsMessage struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

func TestService_AcceptsLegacyClientWithoutGameProcessQuery(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	u := url.URL{
		Scheme: "ws",
		Host:   svc.Address(),
		Path:   "/",
	}
	conn, response, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if conn != nil {
		defer conn.Close()
	}
	if err != nil {
		t.Fatalf("Dial without process=game: %v (response=%#v)", err, response)
	}
	if !svc.GetWSState().Connected {
		t.Fatal("legacy client connected but service is not marked connected")
	}
}

func TestService_RejectsClientWithUnsupportedProcessQuery(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	u := url.URL{
		Scheme:   "ws",
		Host:     svc.Address(),
		Path:     "/",
		RawQuery: "process=browser",
	}
	conn, response, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if conn != nil {
		defer conn.Close()
	}
	if err == nil {
		t.Fatal("Dial with unsupported process unexpectedly succeeded")
	}
	if response == nil || response.StatusCode != 400 {
		t.Fatalf("Dial response = %#v, want HTTP 400", response)
	}
}

func TestService_InvalidEnvelopeAndPayloadDoNotDisconnectClient(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"name":`)); err != nil {
		t.Fatalf("write invalid envelope: %v", err)
	}
	mustWriteJSON(t, conn, map[string]any{
		"name":    "record_status",
		"payload": "not-an-object",
	})
	mustWriteJSON(t, conn, map[string]any{
		"name":    "unknown_plugin_message",
		"payload": map[string]any{"ignored": true},
	})
	mustWriteJSON(t, conn, map[string]any{
		"name": "record_status",
		"payload": map[string]any{
			"demo_path":    "valid.dem",
			"take_index":   1,
			"record_phase": "start",
		},
	})

	waitFor(t, 2*time.Second, func() bool {
		return svc.GetWSState().Connected && svc.GetTakeSnapshot().TotalTakes == 1
	})
}

func TestService_DisconnectWritesSingleIncidentReport(t *testing.T) {
	diagnostics := NewDiagnostics(t.TempDir())
	svc := New("127.0.0.1:0", nil)
	svc.SetReconnectGrace(100 * time.Millisecond)
	svc.SetDiagnostics(diagnostics)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	if err := svc.StartQueue([]string{"incident.dem"}); err != nil {
		t.Fatalf("StartQueue: %v", err)
	}
	_ = mustReadWSMessage(t, conn)
	_ = conn.Close()

	waitFor(t, 2*time.Second, func() bool {
		state := svc.GetQueueState()
		if state.Running {
			return false
		}
		paths, _ := filepath.Glob(filepath.Join(diagnostics.logDir, "producews-incident-*.txt"))
		return len(paths) == 1 && strings.Contains(state.LastError, "诊断报告:")
	})
}

func TestService_RejectsNonLoopbackListener(t *testing.T) {
	svc := New("0.0.0.0:0", nil)
	if err := svc.Start(); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("Start non-loopback error = %v, want loopback validation", err)
	}
}

func TestService_StartQueue_StatusAckAndDemoDoneDispatchesNext(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	defer conn.Close()

	if err := svc.StartQueue([]string{"a.dem", "b.dem"}); err != nil {
		t.Fatalf("StartQueue: %v", err)
	}

	first := mustReadWSMessage(t, conn)
	if first.Name != "playdemo" {
		t.Fatalf("first message name = %q, want playdemo", first.Name)
	}
	if payload := mustStringPayload(t, first.Payload); payload != "a.dem" {
		t.Fatalf("first payload = %q, want a.dem", payload)
	}

	mustWriteJSON(t, conn, map[string]any{
		"name":    "status",
		"payload": "ok",
	})
	waitFor(t, 2*time.Second, func() bool {
		state := svc.GetQueueState()
		return state.Running && !state.PendingAck && state.CurrentDemoPath == "a.dem"
	})

	mustWriteJSON(t, conn, map[string]any{
		"name": "demo_done",
		"payload": map[string]any{
			"demo_path": "a.dem",
			"reason":    "disconnect",
			"ts_ms":     time.Now().UnixMilli(),
		},
	})

	second := mustReadWSMessage(t, conn)
	if second.Name != "playdemo" {
		t.Fatalf("second message name = %q, want playdemo", second.Name)
	}
	if payload := mustStringPayload(t, second.Payload); payload != "b.dem" {
		t.Fatalf("second payload = %q, want b.dem", payload)
	}

	mustWriteJSON(t, conn, map[string]any{
		"name":    "status",
		"payload": "ok",
	})
	mustWriteJSON(t, conn, map[string]any{
		"name": "demo_done",
		"payload": map[string]any{
			"demo_path": "b.dem",
			"reason":    "disconnect",
			"ts_ms":     time.Now().UnixMilli(),
		},
	})
	waitFor(t, 2*time.Second, func() bool {
		state := svc.GetQueueState()
		return !state.Running && state.Completed == 2
	})
}

func TestService_SendCommandDoesNotHoldStateLockDuringWrite(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.started = true
	svc.gameConn = &websocket.Conn{}
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	svc.writeJSONFn = func(_ *websocket.Conn, _ outgoingMessage, _ time.Duration) error {
		close(writeStarted)
		<-releaseWrite
		return nil
	}

	commandErr := make(chan error, 1)
	go func() {
		commandErr <- svc.SendCommand("test", "payload")
	}()
	select {
	case <-writeStarted:
	case <-time.After(time.Second):
		t.Fatal("write did not start")
	}

	stateDone := make(chan struct{})
	go func() {
		_ = svc.GetWSState()
		close(stateDone)
	}()
	select {
	case <-stateDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("GetWSState blocked behind websocket write")
	}

	close(releaseWrite)
	if err := <-commandErr; err != nil {
		t.Fatalf("SendCommand: %v", err)
	}
}

func TestService_DispatchNextDemoDoesNotHoldStateLockDuringWrite(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.started = true
	svc.gameConn = &websocket.Conn{}
	svc.gameConnID = 1
	svc.queueState = QueueState{
		Running:      true,
		Total:        1,
		CurrentIndex: -1,
		Demos:        []string{"demo.dem"},
	}
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	svc.writeJSONFn = func(_ *websocket.Conn, _ outgoingMessage, _ time.Duration) error {
		close(writeStarted)
		<-releaseWrite
		return nil
	}

	dispatchDone := make(chan struct{})
	go func() {
		svc.dispatchNextDemo()
		close(dispatchDone)
	}()
	select {
	case <-writeStarted:
	case <-time.After(time.Second):
		t.Fatal("dispatch write did not start")
	}

	stateDone := make(chan struct{})
	go func() {
		_ = svc.GetQueueState()
		close(stateDone)
	}()
	select {
	case <-stateDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("GetQueueState blocked behind websocket write")
	}

	close(releaseWrite)
	select {
	case <-dispatchDone:
	case <-time.After(time.Second):
		t.Fatal("dispatch did not finish after write release")
	}
}

func TestService_RecordStatusUpdatesTakeSnapshot(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	defer conn.Close()

	mustWriteJSON(t, conn, map[string]any{
		"name": "record_status",
		"payload": map[string]any{
			"demo_path":    "a.dem",
			"take_index":   1,
			"take_name":    "take0001",
			"record_phase": "start",
			"cmd":          "mirv_streams record start",
			"tick":         123,
			"ts_ms":        time.Now().UnixMilli(),
		},
	})

	waitFor(t, 2*time.Second, func() bool {
		snapshot := svc.GetTakeSnapshot()
		return snapshot.TotalTakes == 1 && snapshot.Items[0].Status == "recording"
	})

	mustWriteJSON(t, conn, map[string]any{
		"name": "record_status",
		"payload": map[string]any{
			"demo_path":    "a.dem",
			"take_index":   1,
			"take_name":    "take0001",
			"record_phase": "end",
			"cmd":          "mirv_streams record end",
			"tick":         256,
			"ts_ms":        time.Now().UnixMilli(),
		},
	})

	waitFor(t, 2*time.Second, func() bool {
		snapshot := svc.GetTakeSnapshot()
		return snapshot.TotalTakes == 1 &&
			snapshot.CompletedTakes == 1 &&
			snapshot.Items[0].Status == "completed"
	})
}

func TestService_DemoSwitchDelayAppliesBetweenDemos(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.SetDemoSwitchDelay(150 * time.Millisecond)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	defer conn.Close()

	if err := svc.StartQueue([]string{"a.dem", "b.dem"}); err != nil {
		t.Fatalf("StartQueue: %v", err)
	}
	_ = mustReadWSMessage(t, conn)

	mustWriteJSON(t, conn, map[string]any{
		"name":    "status",
		"payload": "ok",
	})
	mustWriteJSON(t, conn, map[string]any{
		"name": "demo_done",
		"payload": map[string]any{
			"demo_path": "a.dem",
			"reason":    "disconnect",
			"ts_ms":     time.Now().UnixMilli(),
		},
	})

	time.Sleep(60 * time.Millisecond)
	earlyState := svc.GetQueueState()
	if earlyState.CurrentIndex != 0 || earlyState.PendingAck {
		t.Fatalf("unexpected early switch state: %+v", earlyState)
	}
	waitFor(t, 2*time.Second, func() bool {
		state := svc.GetQueueState()
		return state.Running &&
			state.CurrentIndex == 1 &&
			state.PendingAck &&
			state.CurrentDemoPath == "b.dem"
	})

	second := mustReadWSMessage(t, conn)
	if second.Name != "playdemo" {
		t.Fatalf("second message name = %q, want playdemo", second.Name)
	}
	if payload := mustStringPayload(t, second.Payload); payload != "b.dem" {
		t.Fatalf("second payload = %q, want b.dem", payload)
	}
}

func TestService_DuplicateDemoDoneDoesNotSkipQueue(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.SetDemoSwitchDelay(150 * time.Millisecond)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	defer conn.Close()

	if err := svc.StartQueue([]string{"a.dem", "b.dem"}); err != nil {
		t.Fatalf("StartQueue: %v", err)
	}
	_ = mustReadWSMessage(t, conn)

	mustWriteJSON(t, conn, map[string]any{
		"name":    "status",
		"payload": "ok",
	})
	donePayload := map[string]any{
		"name": "demo_done",
		"payload": map[string]any{
			"demo_path": "a.dem",
			"reason":    "disconnect",
			"ts_ms":     time.Now().UnixMilli(),
		},
	}
	mustWriteJSON(t, conn, donePayload)
	mustWriteJSON(t, conn, donePayload)

	second := mustReadWSMessage(t, conn)
	if second.Name != "playdemo" {
		t.Fatalf("second message name = %q, want playdemo", second.Name)
	}
	if payload := mustStringPayload(t, second.Payload); payload != "b.dem" {
		t.Fatalf("second payload = %q, want b.dem", payload)
	}

	mustWriteJSON(t, conn, map[string]any{
		"name":    "status",
		"payload": "ok",
	})
	mustWriteJSON(t, conn, map[string]any{
		"name": "demo_done",
		"payload": map[string]any{
			"demo_path": "b.dem",
			"reason":    "disconnect",
			"ts_ms":     time.Now().UnixMilli(),
		},
	})

	waitFor(t, 2*time.Second, func() bool {
		state := svc.GetQueueState()
		return !state.Running && state.Completed == 2
	})
}

func TestService_AckTimeoutStopsQueue(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.SetAckTimeout(150 * time.Millisecond)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	defer conn.Close()

	if err := svc.StartQueue([]string{"timeout.dem"}); err != nil {
		t.Fatalf("StartQueue: %v", err)
	}
	_ = mustReadWSMessage(t, conn)

	waitFor(t, 2*time.Second, func() bool {
		state := svc.GetQueueState()
		return !state.Running && strings.Contains(state.LastError, "确认超时")
	})
}

func TestService_DisconnectStopsQueue(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.SetAckTimeout(2 * time.Second)
	svc.SetReconnectGrace(100 * time.Millisecond)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())

	if err := svc.StartQueue([]string{"disconnect.dem"}); err != nil {
		t.Fatalf("StartQueue: %v", err)
	}
	_ = mustReadWSMessage(t, conn)

	mustWriteJSON(t, conn, map[string]any{
		"name":    "status",
		"payload": "ok",
	})
	waitFor(t, 2*time.Second, func() bool {
		state := svc.GetQueueState()
		return state.Running && !state.PendingAck
	})
	mustWriteJSON(t, conn, map[string]any{
		"name": "record_status",
		"payload": map[string]any{
			"demo_path":    "disconnect.dem",
			"take_index":   1,
			"take_name":    "take0001",
			"record_phase": "start",
			"cmd":          "mirv_streams record start",
			"tick":         100,
			"ts_ms":        time.Now().UnixMilli(),
		},
	})
	mustWriteJSON(t, conn, map[string]any{
		"name": "record_status",
		"payload": map[string]any{
			"demo_path":    "disconnect.dem",
			"take_index":   2,
			"take_name":    "take0002",
			"record_phase": "start",
			"cmd":          "mirv_streams record start",
			"tick":         120,
			"ts_ms":        time.Now().UnixMilli(),
		},
	})
	mustWriteJSON(t, conn, map[string]any{
		"name": "record_status",
		"payload": map[string]any{
			"demo_path":    "disconnect.dem",
			"take_index":   2,
			"take_name":    "take0002",
			"record_phase": "end",
			"cmd":          "mirv_streams record end",
			"tick":         150,
			"ts_ms":        time.Now().UnixMilli(),
		},
	})

	_ = conn.Close()

	waitFor(t, 2*time.Second, func() bool {
		state := svc.GetQueueState()
		return !state.Running && strings.Contains(state.LastError, "WebSocket")
	})
	waitFor(t, 2*time.Second, func() bool {
		snapshot := svc.GetTakeSnapshot()
		if len(snapshot.Items) < 2 {
			return false
		}
		statusByTake := map[int]string{}
		for _, item := range snapshot.Items {
			statusByTake[item.TakeIndex] = item.Status
		}
		return statusByTake[1] == "pending" && statusByTake[2] == "completed"
	})
}

func TestService_ReconnectBeforeAckResendsSameDemo(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.SetReconnectGrace(time.Second)
	svc.SetReplayWindow(25 * time.Millisecond)
	svc.SetAckTimeout(time.Second)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	firstConn := mustConnectGameClient(t, svc.Address())
	if err := svc.StartQueue([]string{"retry-same.dem"}); err != nil {
		t.Fatalf("StartQueue: %v", err)
	}
	first := mustReadWSMessage(t, firstConn)
	if got := mustStringPayload(t, first.Payload); first.Name != "playdemo" || got != "retry-same.dem" {
		t.Fatalf("initial message = %+v, want playdemo retry-same.dem", first)
	}
	_ = firstConn.Close()

	waitFor(t, time.Second, func() bool {
		state := svc.GetQueueState()
		return state.Running && state.PendingAck && !svc.GetWSState().Connected
	})

	secondConn := mustConnectGameClient(t, svc.Address())
	defer secondConn.Close()
	replayed := mustReadWSMessage(t, secondConn)
	if got := mustStringPayload(t, replayed.Payload); replayed.Name != "playdemo" || got != "retry-same.dem" {
		t.Fatalf("replayed message = %+v, want same playdemo", replayed)
	}
	mustWriteJSON(t, secondConn, map[string]any{"name": "status", "payload": "ok"})
	waitFor(t, time.Second, func() bool {
		state := svc.GetQueueState()
		return state.Running && !state.PendingAck && state.CurrentIndex == 0 && state.Completed == 0
	})
}

func TestService_ReconnectAfterAckAcceptsDurableDemoDone(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.SetReconnectGrace(time.Second)
	svc.SetReplayWindow(25 * time.Millisecond)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	firstConn := mustConnectGameClient(t, svc.Address())
	if err := svc.StartQueue([]string{"durable.dem"}); err != nil {
		t.Fatalf("StartQueue: %v", err)
	}
	_ = mustReadWSMessage(t, firstConn)
	mustWriteJSON(t, firstConn, map[string]any{"name": "status", "payload": "ok"})
	waitFor(t, time.Second, func() bool {
		state := svc.GetQueueState()
		return state.Running && !state.PendingAck
	})
	_ = firstConn.Close()

	secondConn := mustConnectGameClient(t, svc.Address())
	defer secondConn.Close()
	mustWriteJSON(t, secondConn, map[string]any{
		"name": "demo_done",
		"payload": map[string]any{
			"demo_path": "durable.dem",
			"reason":    "plugin_replay",
			"ts_ms":     time.Now().UnixMilli(),
		},
	})
	waitFor(t, time.Second, func() bool {
		state := svc.GetQueueState()
		return !state.Running && state.Completed == 1 && state.LastError == ""
	})
}

func TestService_ReconnectAfterAckWithoutDemoDoneRecoversQueue(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.SetReconnectGrace(time.Second)
	svc.SetReplayWindow(25 * time.Millisecond)
	svc.SetDemoSwitchDelay(25 * time.Millisecond)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	firstConn := mustConnectGameClient(t, svc.Address())
	if err := svc.StartQueue([]string{"a.dem", "b.dem"}); err != nil {
		t.Fatalf("StartQueue: %v", err)
	}
	first := mustReadWSMessage(t, firstConn)
	if got := mustStringPayload(t, first.Payload); first.Name != "playdemo" || got != "a.dem" {
		t.Fatalf("initial message = %+v, want playdemo a.dem", first)
	}
	mustWriteJSON(t, firstConn, map[string]any{"name": "status", "payload": "ok"})
	waitFor(t, time.Second, func() bool {
		state := svc.GetQueueState()
		return state.Running && !state.PendingAck && state.CurrentIndex == 0
	})

	// The plugin finishes a.dem while disconnected: its demo_done is lost with
	// the dead connection. After reconnecting within the grace window the
	// queue must not hang — once the resume window expires without a replayed
	// demo_done, the service falls back to replaying the current demo.
	_ = firstConn.Close()

	secondConn := mustConnectGameClient(t, svc.Address())
	defer secondConn.Close()
	replayed := mustReadWSMessage(t, secondConn)
	if got := mustStringPayload(t, replayed.Payload); replayed.Name != "playdemo" || got != "a.dem" {
		t.Fatalf("recovery message = %+v, want replayed playdemo a.dem", replayed)
	}
	mustWriteJSON(t, secondConn, map[string]any{"name": "status", "payload": "ok"})
	waitFor(t, time.Second, func() bool {
		state := svc.GetQueueState()
		return state.Running && !state.PendingAck
	})
	mustWriteJSON(t, secondConn, map[string]any{
		"name": "demo_done",
		"payload": map[string]any{
			"demo_path": "a.dem",
			"reason":    "disconnect",
			"ts_ms":     time.Now().UnixMilli(),
		},
	})

	next := mustReadWSMessage(t, secondConn)
	if got := mustStringPayload(t, next.Payload); next.Name != "playdemo" || got != "b.dem" {
		t.Fatalf("next message = %+v, want playdemo b.dem", next)
	}
	mustWriteJSON(t, secondConn, map[string]any{"name": "status", "payload": "ok"})
	mustWriteJSON(t, secondConn, map[string]any{
		"name": "demo_done",
		"payload": map[string]any{
			"demo_path": "b.dem",
			"reason":    "disconnect",
			"ts_ms":     time.Now().UnixMilli(),
		},
	})
	waitFor(t, 2*time.Second, func() bool {
		state := svc.GetQueueState()
		return !state.Running && state.Completed == 2 && state.LastError == ""
	})
}

func TestService_DispatchNextDemoSkipsWhenAckAlreadyPending(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.started = true
	svc.gameConn = &websocket.Conn{}
	svc.gameConnID = 1
	svc.queueState = QueueState{
		Running:      true,
		Total:        1,
		Completed:    0,
		CurrentIndex: 0,
		PendingAck:   true,
		Demos:        []string{"demo.dem"},
	}
	writeCalled := false
	svc.writeJSONFn = func(_ *websocket.Conn, _ outgoingMessage, _ time.Duration) error {
		writeCalled = true
		return nil
	}

	svc.dispatchNextDemo()

	svc.mu.Lock()
	pending := svc.queueState.PendingAck
	svc.mu.Unlock()
	if writeCalled {
		t.Fatal("dispatchNextDemo wrote playdemo while an ack was already pending")
	}
	if !pending {
		t.Fatal("pending ack state should be preserved")
	}
}

func TestService_ReconnectAfterAckDoesNotReplayWhenDemoDoneArrives(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.SetReconnectGrace(time.Second)
	// A long resume window guarantees the durable demo_done (sent immediately
	// after the reconnect) always lands first, even on a loaded CI machine;
	// the assertion is that it stops the fallback replay.
	svc.SetReplayWindow(2 * time.Second)
	svc.SetDemoSwitchDelay(25 * time.Millisecond)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	firstConn := mustConnectGameClient(t, svc.Address())
	if err := svc.StartQueue([]string{"a.dem", "b.dem"}); err != nil {
		t.Fatalf("StartQueue: %v", err)
	}
	first := mustReadWSMessage(t, firstConn)
	if got := mustStringPayload(t, first.Payload); first.Name != "playdemo" || got != "a.dem" {
		t.Fatalf("initial message = %+v, want playdemo a.dem", first)
	}
	mustWriteJSON(t, firstConn, map[string]any{"name": "status", "payload": "ok"})
	waitFor(t, time.Second, func() bool {
		state := svc.GetQueueState()
		return state.Running && !state.PendingAck
	})
	_ = firstConn.Close()

	secondConn := mustConnectGameClient(t, svc.Address())
	defer secondConn.Close()
	mustWriteJSON(t, secondConn, map[string]any{
		"name": "demo_done",
		"payload": map[string]any{
			"demo_path": "a.dem",
			"reason":    "plugin_replay",
			"ts_ms":     time.Now().UnixMilli(),
		},
	})

	// The durable demo_done arrived inside the resume window: the queue must
	// advance straight to the next demo without replaying a.dem.
	next := mustReadWSMessage(t, secondConn)
	if got := mustStringPayload(t, next.Payload); next.Name != "playdemo" || got != "b.dem" {
		t.Fatalf("message after durable demo_done = %+v, want playdemo b.dem", next)
	}
	mustWriteJSON(t, secondConn, map[string]any{"name": "status", "payload": "ok"})
	waitFor(t, time.Second, func() bool {
		state := svc.GetQueueState()
		return state.Running && !state.PendingAck && state.CurrentDemoPath == "b.dem"
	})

	// While the queue is still recording b.dem, give any incorrect replay of
	// a.dem ample time to surface. Heartbeats are 15s apart, so nothing
	// legitimate should arrive in this window.
	_ = secondConn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	var msg wsMessage
	if err := secondConn.ReadJSON(&msg); err == nil {
		t.Fatalf("unexpected message while queue continued to next demo: %+v", msg)
	}
}

func TestService_MidDemoResumeStaleCallbackDoesNotClearNewTimer(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.started = true
	svc.gameConn = &websocket.Conn{}
	svc.gameConnID = 1
	svc.queueState = QueueState{
		Running:         true,
		Total:           1,
		Completed:       0,
		CurrentIndex:    0,
		CurrentDemoPath: "a.dem",
		PendingAck:      false,
		Demos:           []string{"a.dem"},
	}
	// Use a long window so the real timers never fire during the test.
	svc.replayWindow = time.Hour
	writeCalled := false
	svc.writeJSONFn = func(_ *websocket.Conn, _ outgoingMessage, _ time.Duration) error {
		writeCalled = true
		return nil
	}

	svc.mu.Lock()
	svc.startMidDemoResumeTimerLocked(1, "a.dem")
	firstGen := svc.midDemoResumeGen
	svc.mu.Unlock()

	// A second connection supersedes the first timer.
	svc.gameConnID = 2
	svc.mu.Lock()
	svc.startMidDemoResumeTimerLocked(2, "a.dem")
	secondGen := svc.midDemoResumeGen
	secondTimer := svc.midDemoResumeTimer
	svc.mu.Unlock()

	// The first timer's callback had already fired and was waiting for mu; it
	// now runs late. It must neither clear the newer timer handle nor write.
	svc.resumeAckedMidDemo(1, "a.dem", firstGen)

	svc.mu.Lock()
	timer := svc.midDemoResumeTimer
	gen := svc.midDemoResumeGen
	svc.mu.Unlock()
	if timer != secondTimer {
		t.Fatal("stale callback cleared the newer timer handle")
	}
	if gen != secondGen {
		t.Fatalf("generation changed by stale callback: got %d, want %d", gen, secondGen)
	}
	if writeCalled {
		t.Fatal("stale callback dispatched a playdemo write")
	}
}

func TestService_HeartbeatPongKeepsConnectionAlive(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.heartbeatInterval = 15 * time.Millisecond
	svc.pongWait = 75 * time.Millisecond
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	defer conn.Close()
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	time.Sleep(225 * time.Millisecond)
	if !svc.GetWSState().Connected {
		t.Fatalf("connection closed despite client pong handling: %+v", svc.GetWSState())
	}
	_ = conn.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("client read loop did not stop")
	}
}

func TestService_HeartbeatDeadlineClosesUnresponsiveClient(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.heartbeatInterval = 15 * time.Millisecond
	svc.pongWait = 75 * time.Millisecond
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	defer conn.Close()
	waitFor(t, time.Second, func() bool {
		return !svc.GetWSState().Connected
	})
}

func TestService_SendCommand_DeliversToGameClient(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	defer conn.Close()

	waitFor(t, 2*time.Second, func() bool {
		return svc.GetWSState().Connected
	})

	if err := svc.SendCommand("quit", nil); err != nil {
		t.Fatalf("SendCommand: %v", err)
	}

	msg := mustReadWSMessage(t, conn)
	if msg.Name != "quit" {
		t.Fatalf("message name=%q want quit", msg.Name)
	}
}

func TestService_GracefulExitAckClassifiesFollowingDisconnectAsExpected(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	defer conn.Close()
	waitFor(t, time.Second, func() bool { return svc.GetWSState().Connected })

	if err := svc.RequestGracefulExit(); err != nil {
		t.Fatalf("RequestGracefulExit: %v", err)
	}
	message := mustReadWSMessage(t, conn)
	if message.Name != "end_produce_session" {
		t.Fatalf("message name=%q want end_produce_session", message.Name)
	}
	var request gracefulExitRequestPayload
	if err := json.Unmarshal(message.Payload, &request); err != nil {
		t.Fatalf("request payload: %v", err)
	}
	if request.RequestID == "" {
		t.Fatal("graceful exit request_id is empty")
	}
	mustWriteJSON(t, conn, map[string]any{
		"name": "session_exit_ack",
		"payload": map[string]any{
			"request_id": request.RequestID,
		},
	})
	waitFor(t, time.Second, func() bool {
		return svc.GracefulExitStatus().Acknowledged
	})

	_ = conn.Close()
	waitFor(t, time.Second, func() bool {
		return svc.GracefulExitStatus().Completed && !svc.GetWSState().Connected
	})
	if got := svc.GetWSState().LastError; got != "" {
		t.Fatalf("LastError=%q, want empty for expected graceful exit", got)
	}
}

func TestService_GracefulExitWithoutAckKeepsUnexpectedDisconnectError(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	defer conn.Close()
	if err := svc.RequestGracefulExit(); err != nil {
		t.Fatalf("RequestGracefulExit: %v", err)
	}
	_ = mustReadWSMessage(t, conn)
	_ = conn.Close()
	waitFor(t, time.Second, func() bool { return !svc.GetWSState().Connected })
	if got := svc.GetWSState().LastError; !strings.Contains(got, "游戏插件 WebSocket 已断开") {
		t.Fatalf("LastError=%q, want unexpected disconnect", got)
	}
	if svc.GracefulExitStatus().Completed {
		t.Fatal("unacknowledged disconnect must not be marked completed")
	}
}

func TestService_GracefulExitFallbackClassifiesKnownPIDCloseAsExpected(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	defer conn.Close()
	if err := svc.RequestGracefulExit(); err != nil {
		t.Fatalf("RequestGracefulExit: %v", err)
	}
	_ = mustReadWSMessage(t, conn)
	if !svc.ExpectGracefulExitFallback() {
		t.Fatal("ExpectGracefulExitFallback returned false for pending request")
	}
	_ = conn.Close()
	waitFor(t, time.Second, func() bool {
		return svc.GracefulExitStatus().Completed && !svc.GetWSState().Connected
	})
	if got := svc.GetWSState().LastError; got != "" {
		t.Fatalf("LastError=%q, want empty for expected PID fallback close", got)
	}
}

func TestService_RequestGracefulExitRejectsRunningQueue(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	conn := mustConnectGameClient(t, svc.Address())
	defer conn.Close()
	if err := svc.StartQueue([]string{"still-recording.dem"}); err != nil {
		t.Fatalf("StartQueue: %v", err)
	}
	_ = mustReadWSMessage(t, conn)
	if err := svc.RequestGracefulExit(); err == nil || !strings.Contains(err.Error(), "尚未完成") {
		t.Fatalf("RequestGracefulExit error=%v, want active queue rejection", err)
	}
}

func mustConnectGameClient(t *testing.T, addr string) *websocket.Conn {
	t.Helper()
	u := url.URL{
		Scheme:   "ws",
		Host:     addr,
		Path:     "/",
		RawQuery: "process=game",
	}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	return conn
}

func mustReadWSMessage(t *testing.T, conn *websocket.Conn) wsMessage {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg wsMessage
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	return msg
}

func mustStringPayload(t *testing.T, payload json.RawMessage) string {
	t.Helper()
	var value string
	if err := json.Unmarshal(payload, &value); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	return value
}

func mustWriteJSON(t *testing.T, conn *websocket.Conn, payload any) {
	t.Helper()
	if err := conn.WriteJSON(payload); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
