package producews

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type wsMessage struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

func TestService_AcceptsLegacyClientWithoutProcessQuery(t *testing.T) {
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
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("Dial legacy client: %v", err)
	}
	defer conn.Close()

	waitFor(t, 2*time.Second, func() bool {
		return svc.GetWSState().Connected
	})
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
		return !state.Running && strings.Contains(state.LastError, "ack timeout")
	})
}

func TestService_DisconnectStopsQueue(t *testing.T) {
	svc := New("127.0.0.1:0", nil)
	svc.SetAckTimeout(2 * time.Second)
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
		return !state.Running && strings.Contains(state.LastError, "disconnected")
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
