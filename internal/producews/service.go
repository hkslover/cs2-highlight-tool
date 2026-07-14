package producews

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultListenAddr        = "127.0.0.1:0"
	defaultAckTimeout        = 8 * time.Second
	defaultConnectWaitTimout = 30 * time.Second
	defaultDemoSwitchDelay   = 800 * time.Millisecond
)

// retryExhaustedMessage is the user-facing Chinese message written to
// wsState.LastError when the supervisor has exhausted its retry budget.
const retryExhaustedMessage = "WebSocket 服务监听失败，重试 5 次失败，请检查占用进程后重启 app"

// backoffSequence is the supervisor's exponential backoff schedule between
// retry attempts. Total window ≈ 15.5s for 5 attempts.
var backoffSequence = []time.Duration{
	500 * time.Millisecond,
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
}

const maxRetryAttempts = 5

// listenFn is the package-level network listener factory. Tests may override
// it to simulate transient/persistent Listen failures. Production code uses
// net.Listen.
var listenFn = net.Listen

type EventEmitter func(name string, payload any)

type WSState struct {
	Address     string `json:"address"`
	Connected   bool   `json:"connected"`
	LastError   string `json:"last_error,omitempty"`
	UpdatedAtMs int64  `json:"updated_at_ms"`
}

type QueueState struct {
	Running         bool     `json:"running"`
	Total           int      `json:"total"`
	Completed       int      `json:"completed"`
	CurrentIndex    int      `json:"current_index"`
	CurrentDemoPath string   `json:"current_demo_path,omitempty"`
	PendingAck      bool     `json:"pending_ack"`
	LastError       string   `json:"last_error,omitempty"`
	Demos           []string `json:"demos,omitempty"`
	UpdatedAtMs     int64    `json:"updated_at_ms"`
}

type TakeStatus struct {
	DemoPath    string `json:"demo_path,omitempty"`
	TakeIndex   int    `json:"take_index,omitempty"`
	TakeName    string `json:"take_name,omitempty"`
	RecordPhase string `json:"record_phase,omitempty"`
	Status      string `json:"status"`
	Tick        int    `json:"tick,omitempty"`
	Cmd         string `json:"cmd,omitempty"`
	TsMs        int64  `json:"ts_ms"`
}

type TakeStatusSnapshot struct {
	Items          []TakeStatus `json:"items"`
	TotalTakes     int          `json:"total_takes"`
	StartedTakes   int          `json:"started_takes"`
	CompletedTakes int          `json:"completed_takes"`
	LastEvent      *TakeStatus  `json:"last_event,omitempty"`
	UpdatedAtMs    int64        `json:"updated_at_ms"`
}

type incomingMessage struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

type outgoingMessage struct {
	Name    string `json:"name"`
	Payload any    `json:"payload,omitempty"`
}

type recordStatusPayload struct {
	DemoPath    string `json:"demo_path"`
	TakeIndex   int    `json:"take_index"`
	TakeName    string `json:"take_name"`
	RecordPhase string `json:"record_phase"`
	Cmd         string `json:"cmd"`
	Tick        int    `json:"tick"`
	TsMs        int64  `json:"ts_ms"`
}

type demoEventPayload struct {
	DemoPath string `json:"demo_path"`
	Reason   string `json:"reason"`
	TsMs     int64  `json:"ts_ms"`
}

type Service struct {
	addr string
	emit EventEmitter

	mu sync.Mutex
	// gorilla/websocket permits one concurrent writer at a time. Keep this
	// separate from mu so a stalled network write cannot block state readers.
	writeMu sync.Mutex

	writeTimeout time.Duration
	writeJSONFn  func(*websocket.Conn, outgoingMessage, time.Duration) error

	server          *http.Server
	listener        net.Listener
	started         bool
	address         string
	gameConn        *websocket.Conn
	gameConnID      uint64
	nextConnID      uint64
	ackTimer        *time.Timer
	ackTimeout      time.Duration
	connectWait     time.Duration
	demoSwitchDelay time.Duration
	upgrader        websocket.Upgrader
	wsState         WSState
	queueState      QueueState
	takeStates      map[string]TakeStatus
	takeOrder       []string
	lastTakeEvent   *TakeStatus

	supervisorCtx    context.Context
	supervisorCancel context.CancelFunc
	supervisorWG     sync.WaitGroup
}

func New(addr string, emit EventEmitter) *Service {
	if strings.TrimSpace(addr) == "" {
		addr = defaultListenAddr
	}
	s := &Service{
		addr:            addr,
		emit:            emit,
		ackTimeout:      defaultAckTimeout,
		connectWait:     defaultConnectWaitTimout,
		demoSwitchDelay: defaultDemoSwitchDelay,
		writeTimeout:    5 * time.Second,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		takeStates: make(map[string]TakeStatus),
		queueState: QueueState{
			CurrentIndex: -1,
		},
	}
	s.writeJSONFn = writeJSONWithDeadline
	return s
}

func NewDefault(emit EventEmitter) *Service {
	return New(defaultListenAddr, emit)
}

func (s *Service) SetEmitter(emit EventEmitter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emit = emit
}

func (s *Service) SetAckTimeout(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if timeout > 0 {
		s.ackTimeout = timeout
	}
}

func (s *Service) SetConnectWaitTimeout(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if timeout > 0 {
		s.connectWait = timeout
	}
}

func (s *Service) SetDemoSwitchDelay(delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if delay >= 0 {
		s.demoSwitchDelay = delay
	}
}

// Start brings the WebSocket server up. The first Listen attempt is performed
// synchronously so callers preserve the existing contract: success returns nil,
// failure returns the listen error. In both cases a background supervisor
// goroutine takes over to:
//   - retry Listen with exponential backoff if the first attempt failed; and
//   - re-establish Listen + Serve when an active Serve() returns mid-flight.
//
// The supervisor stops after maxRetryAttempts failed Listen attempts in a row
// or when Stop() is called.
func (s *Service) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}

	listener, err := listenFn("tcp", s.addr)
	if err != nil {
		// First attempt failed. Preserve the original contract: set
		// LastError, emit state, return err. The supervisor will take over
		// retries asynchronously starting from attempt=1.
		s.wsState.LastError = err.Error()
		s.wsState.UpdatedAtMs = nowMs()
		s.emitWSStateLocked()
		s.startSupervisorLocked(nil, 1)
		s.mu.Unlock()
		return err
	}

	s.adoptListenerLocked(listener)
	s.startSupervisorLocked(listener, 0)
	s.mu.Unlock()
	return nil
}

// adoptListenerLocked initialises wsState, http.Server, and address fields
// from a freshly-listening net.Listener. Caller must hold s.mu.
func (s *Service) adoptListenerLocked(listener net.Listener) {
	s.listener = listener
	s.address = listener.Addr().String()
	s.started = true
	s.wsState = WSState{
		Address:     s.address,
		Connected:   false,
		UpdatedAtMs: nowMs(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebSocket)
	s.server = &http.Server{Handler: mux}
	s.emitWSStateLocked()
}

// startSupervisorLocked spawns the supervisor goroutine. If initialListener is
// non-nil, the supervisor skips the first Listen attempt and starts Serve()
// directly using it. attempt is the starting retry attempt count when
// initialListener is nil. Caller must hold s.mu.
func (s *Service) startSupervisorLocked(initialListener net.Listener, attempt int) {
	if s.supervisorCancel != nil {
		// Should not happen — defensive.
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.supervisorCtx = ctx
	s.supervisorCancel = cancel
	s.supervisorWG.Add(1)
	go s.runSupervisor(ctx, initialListener, attempt)
}

// runSupervisor is the supervisor main loop. It re-establishes Listen + Serve
// after failures, bounded by maxRetryAttempts within each failure burst. A
// successful Serve start resets the attempt counter to 0 so each new burst
// gets a fresh budget.
//
// The supervisor exits when:
//   - retries are exhausted (writes a clear LastError); or
//   - the context is cancelled (Stop() was called).
//
// s.mu MUST NOT be held while calling listenFn / server.Serve / time.After.
func (s *Service) runSupervisor(ctx context.Context, initialListener net.Listener, startAttempt int) {
	defer s.supervisorWG.Done()

	listener := initialListener
	attempt := startAttempt

	for {
		// Acquire a listener if we don't already have one.
		if listener == nil {
			select {
			case <-ctx.Done():
				return
			default:
			}

			newListener, err := listenFn("tcp", s.addr)
			if err != nil {
				attempt++
				exhausted := attempt > maxRetryAttempts

				s.mu.Lock()
				if exhausted {
					s.wsState.LastError = retryExhaustedMessage
				} else {
					s.wsState.LastError = err.Error()
				}
				s.wsState.Connected = false
				s.wsState.UpdatedAtMs = nowMs()
				s.emitWSStateLocked()
				s.mu.Unlock()

				if exhausted {
					return
				}
				if !sleepCtx(ctx, backoffForAttempt(attempt)) {
					return
				}
				continue
			}

			// Stop may have raced with the successful listen — discard the
			// new listener if we've been cancelled.
			s.mu.Lock()
			select {
			case <-ctx.Done():
				s.mu.Unlock()
				_ = newListener.Close()
				return
			default:
			}
			s.adoptListenerLocked(newListener)
			s.mu.Unlock()
			listener = newListener
		}

		// Reset attempt counter — each failure burst gets a fresh budget.
		attempt = 0

		// Serve blocks until the listener is closed or an unrecoverable error
		// occurs. http.Server.Serve always returns a non-nil error.
		serveErr := s.server.Serve(listener)
		listener = nil
		s.mu.Lock()
		s.listener = nil
		s.mu.Unlock()

		// If Stop() was called, exit immediately.
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Serve exited unexpectedly — record the cause and try to re-listen.
		s.mu.Lock()
		if serveErr != nil {
			s.wsState.LastError = "serve exited: " + serveErr.Error()
		} else {
			s.wsState.LastError = "serve exited"
		}
		s.wsState.Connected = false
		s.wsState.UpdatedAtMs = nowMs()
		s.emitWSStateLocked()
		s.mu.Unlock()

		attempt = 1
		if !sleepCtx(ctx, backoffForAttempt(attempt)) {
			return
		}
	}
}

// sleepCtx sleeps for d or until ctx is cancelled. Returns false if ctx was
// cancelled (caller should abort), true if the sleep completed.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// backoffForAttempt returns the backoff duration for the given 1-based attempt
// number. Clamps to the last entry if attempt exceeds the sequence length.
func backoffForAttempt(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	if attempt > len(backoffSequence) {
		return backoffSequence[len(backoffSequence)-1]
	}
	return backoffSequence[attempt-1]
}

func (s *Service) Stop() error {
	s.mu.Lock()
	cancel := s.supervisorCancel
	s.supervisorCancel = nil
	s.supervisorCtx = nil
	// Cancel the supervisor's context while still holding s.mu so that any
	// supervisor goroutine racing on post-listenFn lock acquisition observes
	// ctx.Done() and discards its freshly-acquired listener instead of
	// adopting it and swapping in a new http.Server we'd leak. cancel() is a
	// non-blocking signal (close on a channel), safe to call under lock.
	if cancel != nil {
		cancel()
	}

	if !s.started {
		s.mu.Unlock()
		if cancel != nil {
			s.supervisorWG.Wait()
		}
		return nil
	}
	server := s.server
	s.listener = nil
	conn := s.gameConn
	s.gameConn = nil
	s.started = false
	s.stopAckTimerLocked()
	s.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	var closeErr error
	if server != nil {
		closeErr = server.Close()
	}
	// Wait for the supervisor goroutine to exit so callers see a clean state.
	s.supervisorWG.Wait()
	return closeErr
}

func (s *Service) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.address
}

// Port returns the TCP port currently held by the WebSocket listener.
// The listener stays open for the lifetime of the service, so callers can
// safely pass this port to the process that launches the game plugin.
func (s *Service) Port() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started || s.listener == nil {
		return 0, fmt.Errorf("produce websocket server is not started")
	}

	addr, ok := s.listener.Addr().(*net.TCPAddr)
	if !ok || addr.Port <= 0 {
		return 0, fmt.Errorf("produce websocket port is unavailable")
	}
	return addr.Port, nil
}

func (s *Service) StartQueue(demoPaths []string) error {
	demos := normalizeDemoPaths(demoPaths)
	if len(demos) == 0 {
		return fmt.Errorf("no demos to enqueue")
	}

	if err := s.waitForConnection(); err != nil {
		return err
	}

	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return fmt.Errorf("produce websocket server is not started")
	}
	if s.queueState.Running {
		s.mu.Unlock()
		return fmt.Errorf("produce queue is already running")
	}
	s.resetTakeStateLocked()
	s.queueState = QueueState{
		Running:      true,
		Total:        len(demos),
		Completed:    0,
		CurrentIndex: -1,
		PendingAck:   false,
		Demos:        append([]string(nil), demos...),
		UpdatedAtMs:  nowMs(),
	}
	s.emitQueueStateLocked()
	s.emitTakeSnapshotLocked()
	s.mu.Unlock()

	s.dispatchNextDemo()
	return nil
}

func (s *Service) GetWSState() WSState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.wsState
}

func (s *Service) GetQueueState() QueueState {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.queueState
	if len(state.Demos) > 0 {
		state.Demos = append([]string(nil), state.Demos...)
	}
	return state
}

func (s *Service) GetTakeSnapshot() TakeStatusSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.takeSnapshotLocked()
}

func (s *Service) SendCommand(name string, payload any) error {
	command := strings.TrimSpace(name)
	if command == "" {
		return fmt.Errorf("command name is empty")
	}

	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return fmt.Errorf("produce websocket server is not started")
	}
	if s.gameConn == nil {
		s.mu.Unlock()
		return fmt.Errorf("game websocket not connected")
	}
	conn := s.gameConn
	writeTimeout := s.writeTimeout
	s.mu.Unlock()
	return s.writeOutgoing(conn, outgoingMessage{
		Name:    command,
		Payload: payload,
	}, writeTimeout)
}

func (s *Service) waitForConnection() error {
	s.mu.Lock()
	waitTimeout := s.connectWait
	s.mu.Unlock()
	deadline := time.Now().Add(waitTimeout)
	for {
		s.mu.Lock()
		started := s.started
		connected := s.gameConn != nil
		running := s.queueState.Running
		s.mu.Unlock()

		if !started {
			return fmt.Errorf("produce websocket server is not started")
		}
		if running {
			return fmt.Errorf("produce queue is already running")
		}
		if connected {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("game websocket not connected")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *Service) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	process := strings.TrimSpace(r.URL.Query().Get("process"))
	if process != "" && process != "game" {
		http.Error(w, "unsupported process", http.StatusBadRequest)
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.mu.Lock()
		s.wsState.LastError = err.Error()
		s.wsState.UpdatedAtMs = nowMs()
		s.emitWSStateLocked()
		s.mu.Unlock()
		return
	}

	var oldConn *websocket.Conn
	var id uint64
	s.mu.Lock()
	oldConn = s.gameConn
	s.nextConnID++
	id = s.nextConnID
	s.gameConnID = id
	s.gameConn = conn
	s.wsState.Connected = true
	s.wsState.LastError = ""
	s.wsState.UpdatedAtMs = nowMs()
	s.emitWSStateLocked()
	s.mu.Unlock()

	if oldConn != nil {
		_ = oldConn.Close()
	}

	go s.readLoop(id, conn)
}

func (s *Service) readLoop(connID uint64, conn *websocket.Conn) {
	defer s.onConnectionClosed(connID, conn)
	for {
		var message incomingMessage
		if err := conn.ReadJSON(&message); err != nil {
			return
		}
		s.handleIncomingMessage(message)
	}
}

func (s *Service) onConnectionClosed(connID uint64, conn *websocket.Conn) {
	_ = conn.Close()

	s.mu.Lock()
	if s.gameConnID == connID {
		s.gameConn = nil
		s.wsState.Connected = false
		s.wsState.UpdatedAtMs = nowMs()
		s.emitWSStateLocked()
		if s.queueState.Running {
			s.failQueueLocked("game websocket disconnected")
		}
	}
	s.mu.Unlock()
}

func (s *Service) handleIncomingMessage(message incomingMessage) {
	switch message.Name {
	case "status":
		s.handleStatusAck()
	case "record_status":
		var payload recordStatusPayload
		if err := json.Unmarshal(message.Payload, &payload); err == nil {
			s.handleRecordStatus(payload)
		}
	case "demo_started":
		var payload demoEventPayload
		if err := json.Unmarshal(message.Payload, &payload); err == nil {
			s.handleDemoStarted(payload)
		}
	case "demo_done":
		var payload demoEventPayload
		if err := json.Unmarshal(message.Payload, &payload); err == nil {
			s.handleDemoDone(payload)
		}
	}
}

func (s *Service) handleStatusAck() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.queueState.Running || !s.queueState.PendingAck {
		return
	}
	s.queueState.PendingAck = false
	s.queueState.UpdatedAtMs = nowMs()
	s.stopAckTimerLocked()
	s.emitQueueStateLocked()
}

func (s *Service) handleRecordStatus(payload recordStatusPayload) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if payload.DemoPath == "" {
		payload.DemoPath = s.queueState.CurrentDemoPath
	}
	if payload.TsMs <= 0 {
		payload.TsMs = nowMs()
	}

	status := "pending"
	switch strings.ToLower(strings.TrimSpace(payload.RecordPhase)) {
	case "start":
		status = "recording"
	case "end":
		status = "completed"
	}

	key := s.takeKey(payload)
	current, found := s.takeStates[key]
	if !found {
		s.takeOrder = append(s.takeOrder, key)
	}
	if payload.TakeIndex > 0 {
		current.TakeIndex = payload.TakeIndex
	}
	if payload.TakeName != "" {
		current.TakeName = payload.TakeName
	}
	if payload.DemoPath != "" {
		current.DemoPath = payload.DemoPath
	}
	current.RecordPhase = payload.RecordPhase
	current.Status = status
	current.Tick = payload.Tick
	current.Cmd = payload.Cmd
	current.TsMs = payload.TsMs
	s.takeStates[key] = current

	event := current
	s.lastTakeEvent = &event
	s.emitTakeSnapshotLocked()
}

func (s *Service) handleDemoStarted(payload demoEventPayload) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if payload.DemoPath == "" {
		return
	}
	if s.queueState.CurrentDemoPath == "" {
		s.queueState.CurrentDemoPath = payload.DemoPath
		s.queueState.UpdatedAtMs = nowMs()
		s.emitQueueStateLocked()
	}
}

func (s *Service) handleDemoDone(payload demoEventPayload) {
	s.mu.Lock()
	if !s.queueState.Running {
		s.mu.Unlock()
		return
	}
	if s.queueState.CurrentIndex < 0 || s.queueState.CurrentIndex >= len(s.queueState.Demos) {
		s.mu.Unlock()
		return
	}
	// Only accept one done event for the current queue index.
	if s.queueState.Completed != s.queueState.CurrentIndex {
		s.mu.Unlock()
		return
	}
	if payload.DemoPath != "" &&
		s.queueState.CurrentDemoPath != "" &&
		payload.DemoPath != s.queueState.CurrentDemoPath {
		s.mu.Unlock()
		return
	}

	s.queueState.Completed++
	s.queueState.PendingAck = false
	s.queueState.UpdatedAtMs = nowMs()
	s.stopAckTimerLocked()
	s.emitQueueStateLocked()
	s.mu.Unlock()

	s.dispatchNextDemoAfterDelay()
}

func (s *Service) dispatchNextDemoAfterDelay() {
	s.mu.Lock()
	delay := s.demoSwitchDelay
	s.mu.Unlock()
	if delay <= 0 {
		s.dispatchNextDemo()
		return
	}
	time.AfterFunc(delay, func() {
		s.dispatchNextDemo()
	})
}

func (s *Service) dispatchNextDemo() {
	s.mu.Lock()
	if !s.queueState.Running {
		s.mu.Unlock()
		return
	}
	if s.queueState.Completed >= s.queueState.Total {
		s.finishQueueLocked()
		s.mu.Unlock()
		return
	}
	if s.gameConn == nil {
		s.failQueueLocked("game websocket disconnected")
		s.mu.Unlock()
		return
	}

	nextIndex := s.queueState.Completed
	nextDemo := s.queueState.Demos[nextIndex]
	s.queueState.CurrentIndex = nextIndex
	s.queueState.CurrentDemoPath = nextDemo
	s.queueState.PendingAck = true
	s.queueState.UpdatedAtMs = nowMs()
	conn := s.gameConn
	connID := s.gameConnID
	writeTimeout := s.writeTimeout
	s.mu.Unlock()

	if err := s.writeOutgoing(conn, outgoingMessage{
		Name:    "playdemo",
		Payload: nextDemo,
	}, writeTimeout); err != nil {
		s.mu.Lock()
		if s.queueState.Running &&
			s.queueState.CurrentIndex == nextIndex &&
			s.queueState.CurrentDemoPath == nextDemo &&
			s.gameConnID == connID {
			s.failQueueLocked("failed to send playdemo: " + err.Error())
		}
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	if s.queueState.Running &&
		s.queueState.PendingAck &&
		s.queueState.CurrentIndex == nextIndex &&
		s.queueState.CurrentDemoPath == nextDemo &&
		s.gameConnID == connID {
		s.startAckTimerLocked(nextDemo)
		s.emitQueueStateLocked()
	}
	s.mu.Unlock()
}

func (s *Service) writeOutgoing(conn *websocket.Conn, message outgoingMessage, timeout time.Duration) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	writeJSON := s.writeJSONFn
	if writeJSON == nil {
		writeJSON = writeJSONWithDeadline
	}
	return writeJSON(conn, message, timeout)
}

func writeJSONWithDeadline(conn *websocket.Conn, message outgoingMessage, timeout time.Duration) error {
	if timeout > 0 {
		if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			return fmt.Errorf("设置 websocket 写超时失败: %w", err)
		}
	}
	return conn.WriteJSON(message)
}

func (s *Service) startAckTimerLocked(expectedDemo string) {
	s.stopAckTimerLocked()
	timeout := s.ackTimeout
	s.ackTimer = time.AfterFunc(timeout, func() {
		s.onAckTimeout(expectedDemo)
	})
}

func (s *Service) stopAckTimerLocked() {
	if s.ackTimer != nil {
		s.ackTimer.Stop()
		s.ackTimer = nil
	}
}

func (s *Service) onAckTimeout(expectedDemo string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.queueState.Running || !s.queueState.PendingAck {
		return
	}
	if s.queueState.CurrentDemoPath != expectedDemo {
		return
	}
	s.failQueueLocked("playdemo ack timeout: " + expectedDemo)
}

func (s *Service) finishQueueLocked() {
	s.stopAckTimerLocked()
	s.queueState.Running = false
	s.queueState.PendingAck = false
	s.queueState.CurrentIndex = -1
	s.queueState.CurrentDemoPath = ""
	s.queueState.UpdatedAtMs = nowMs()
	s.emitQueueStateLocked()
}

func (s *Service) failQueueLocked(reason string) {
	s.stopAckTimerLocked()
	s.queueState.Running = false
	s.queueState.PendingAck = false
	s.queueState.CurrentIndex = -1
	s.queueState.CurrentDemoPath = ""
	s.queueState.LastError = reason
	s.queueState.UpdatedAtMs = nowMs()
	takeChanged := s.resetRecordingTakesToPendingLocked()
	s.emitQueueStateLocked()
	if takeChanged {
		s.emitTakeSnapshotLocked()
	}
}

func (s *Service) resetRecordingTakesToPendingLocked() bool {
	changed := false
	for key, state := range s.takeStates {
		if state.Status != "recording" {
			continue
		}
		state.Status = "pending"
		state.RecordPhase = ""
		state.Cmd = ""
		state.Tick = 0
		state.TsMs = nowMs()
		s.takeStates[key] = state
		changed = true
	}
	if changed && s.lastTakeEvent != nil && s.lastTakeEvent.Status == "recording" {
		clone := *s.lastTakeEvent
		clone.Status = "pending"
		clone.RecordPhase = ""
		clone.Cmd = ""
		clone.Tick = 0
		clone.TsMs = nowMs()
		s.lastTakeEvent = &clone
	}
	return changed
}

func (s *Service) resetTakeStateLocked() {
	s.takeStates = make(map[string]TakeStatus)
	s.takeOrder = nil
	s.lastTakeEvent = nil
}

func (s *Service) takeKey(payload recordStatusPayload) string {
	if payload.DemoPath != "" && payload.TakeIndex > 0 {
		return fmt.Sprintf("%s#idx#%d", payload.DemoPath, payload.TakeIndex)
	}
	if payload.DemoPath != "" && payload.TakeName != "" {
		return fmt.Sprintf("%s#name#%s", payload.DemoPath, payload.TakeName)
	}
	return fmt.Sprintf("%s#legacy#%s#%d", payload.DemoPath, payload.RecordPhase, payload.Tick)
}

func (s *Service) takeSnapshotLocked() TakeStatusSnapshot {
	items := make([]TakeStatus, 0, len(s.takeOrder))
	started := 0
	completed := 0
	for _, key := range s.takeOrder {
		state, ok := s.takeStates[key]
		if !ok {
			continue
		}
		items = append(items, state)
		if state.Status == "recording" || state.Status == "completed" {
			started++
		}
		if state.Status == "completed" {
			completed++
		}
	}
	var lastEvent *TakeStatus
	if s.lastTakeEvent != nil {
		clone := *s.lastTakeEvent
		lastEvent = &clone
	}
	return TakeStatusSnapshot{
		Items:          items,
		TotalTakes:     len(items),
		StartedTakes:   started,
		CompletedTakes: completed,
		LastEvent:      lastEvent,
		UpdatedAtMs:    nowMs(),
	}
}

func (s *Service) emitWSStateLocked() {
	if s.emit == nil {
		return
	}
	s.emit("produce_ws_state_changed", s.wsState)
}

func (s *Service) emitQueueStateLocked() {
	if s.emit == nil {
		return
	}
	state := s.queueState
	if len(state.Demos) > 0 {
		state.Demos = append([]string(nil), state.Demos...)
	}
	s.emit("produce_queue_state_changed", state)
}

func (s *Service) emitTakeSnapshotLocked() {
	if s.emit == nil {
		return
	}
	s.emit("produce_take_status_changed", s.takeSnapshotLocked())
}

func normalizeDemoPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		cleaned := strings.TrimSpace(path)
		if cleaned == "" {
			continue
		}
		result = append(result, cleaned)
	}
	return result
}

func nowMs() int64 {
	return time.Now().UnixMilli()
}
