package producews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultListenAddr         = "127.0.0.1:0"
	defaultAckTimeout         = 8 * time.Second
	defaultConnectWaitTimout  = 30 * time.Second
	defaultDemoSwitchDelay    = 800 * time.Millisecond
	defaultReconnectGrace     = 15 * time.Second
	defaultReplayWindow       = 1200 * time.Millisecond
	defaultHeartbeatInterval  = 15 * time.Second
	defaultPongWait           = 45 * time.Second
	defaultReadLimit          = int64(1 << 20)
	defaultExpectedExitWindow = 15 * time.Second
)

// retryExhaustedMessage is the user-facing Chinese message written to
// wsState.LastError when the supervisor has exhausted its retry budget.
const retryExhaustedMessage = "WebSocket 服务监听失败，重试 5 次失败，请检查占用进程后重启 app"

const rebindRetryExhaustedMessage = "WebSocket 服务无法重绑原端口，插件仍指向旧端口，需要重启制作会话"

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

type gracefulExitRequestPayload struct {
	RequestID string `json:"request_id"`
}

type gracefulExitAckPayload struct {
	RequestID string `json:"request_id"`
}

// GracefulExitStatus is an internal app-to-producews snapshot. It deliberately
// stays outside WSState so a normal game shutdown does not add a new
// frontend-facing connection state.
type GracefulExitStatus struct {
	Requested    bool
	Acknowledged bool
	Completed    bool
}

type gracefulExitState struct {
	requestID    string
	connID       uint64
	requested    bool
	acknowledged bool
	expected     bool
	completed    bool
	deadline     time.Time
}

type emittedEvent struct {
	emit    EventEmitter
	name    string
	payload any
}

type closeClassification struct {
	kind        string
	description string
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

	server             *http.Server
	listener           net.Listener
	started            bool
	address            string
	relistenAddr       string
	gameConn           *websocket.Conn
	gameConnID         uint64
	gameConnStartedAt  int64
	nextConnID         uint64
	ackTimer           *time.Timer
	reconnectTimer     *time.Timer
	reconnectDeadline  time.Time
	replayTimer        *time.Timer
	ackTimeout         time.Duration
	connectWait        time.Duration
	demoSwitchDelay    time.Duration
	reconnectGrace     time.Duration
	replayWindow       time.Duration
	heartbeatInterval  time.Duration
	pongWait           time.Duration
	readLimit          int64
	expectedExitWindow time.Duration
	gracefulExit       gracefulExitState
	heartbeatConnID    uint64
	heartbeatCancel    context.CancelFunc
	heartbeatWG        sync.WaitGroup
	upgrader           websocket.Upgrader
	wsState            WSState
	queueState         QueueState
	takeStates         map[string]TakeStatus
	takeOrder          []string
	lastTakeEvent      *TakeStatus
	diagnostics        *Diagnostics
	reportedIncidents  map[string]string

	supervisorCtx    context.Context
	supervisorCancel context.CancelFunc
	supervisorWG     sync.WaitGroup
	eventCtx         context.Context
	eventCancel      context.CancelFunc
	eventQueue       chan emittedEvent
	eventWG          sync.WaitGroup
}

func New(addr string, emit EventEmitter) *Service {
	if strings.TrimSpace(addr) == "" {
		addr = defaultListenAddr
	}
	s := &Service{
		addr:               addr,
		emit:               emit,
		ackTimeout:         defaultAckTimeout,
		connectWait:        defaultConnectWaitTimout,
		demoSwitchDelay:    defaultDemoSwitchDelay,
		reconnectGrace:     defaultReconnectGrace,
		replayWindow:       defaultReplayWindow,
		heartbeatInterval:  defaultHeartbeatInterval,
		pongWait:           defaultPongWait,
		readLimit:          defaultReadLimit,
		expectedExitWindow: defaultExpectedExitWindow,
		writeTimeout:       5 * time.Second,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		takeStates:        make(map[string]TakeStatus),
		reportedIncidents: make(map[string]string),
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

// SetDiagnostics installs the data-dir-backed diagnostic sink. The sink is
// independent from the service state lock and can safely be replaced before a
// development workspace has been initialized.
func (s *Service) SetDiagnostics(diagnostics *Diagnostics) {
	s.mu.Lock()
	previous := s.diagnostics
	s.diagnostics = diagnostics
	started := s.started
	s.mu.Unlock()

	if previous != nil && previous != diagnostics {
		previous.Stop()
	}
	if diagnostics != nil && started {
		diagnostics.Start()
	}
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

// SetReconnectGrace configures the bounded recovery window after a game
// connection drops. It is primarily useful to keep recovery integration tests
// fast; production uses the 15 second default.
func (s *Service) SetReconnectGrace(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if timeout > 0 {
		s.reconnectGrace = timeout
	}
}

// SetReplayWindow configures how long a reconnected plugin may replay durable
// events before a pending playdemo command is sent again.
func (s *Service) SetReplayWindow(delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if delay >= 0 {
		s.replayWindow = delay
	}
}

func (s *Service) startDiagnostics() {
	s.mu.Lock()
	diagnostics := s.diagnostics
	s.mu.Unlock()
	if diagnostics != nil {
		diagnostics.Start()
	}
}

// startEventWorkerLocked decouples Wails runtime event emission from Service
// state transitions. Callers may enqueue a copied snapshot while holding mu;
// only the worker invokes the potentially blocking emitter.
func (s *Service) startEventWorkerLocked() {
	if s.eventCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	queue := make(chan emittedEvent, 256)
	s.eventCtx = ctx
	s.eventCancel = cancel
	s.eventQueue = queue
	s.eventWG.Add(1)
	go func() {
		defer s.eventWG.Done()
		for {
			select {
			case event := <-queue:
				if event.emit != nil {
					event.emit(event.name, event.payload)
				}
			case <-ctx.Done():
				for {
					select {
					case event := <-queue:
						if event.emit != nil {
							event.emit(event.name, event.payload)
						}
					default:
						return
					}
				}
			}
		}
	}()
}

func (s *Service) queueEventLocked(name string, payload any) {
	if s.emit == nil || s.eventQueue == nil || s.eventCancel == nil {
		return
	}
	event := emittedEvent{emit: s.emit, name: name, payload: payload}
	select {
	case s.eventQueue <- event:
	default:
		// State getters remain the authoritative recovery path. Dropping a
		// transient event is preferable to holding the state mutex behind a
		// blocked Wails runtime callback.
	}
}

// recordDiagnostic must be called without Service.mu held. Diagnostics owns a
// separate bounded queue, so filesystem failures cannot block service state.
func (s *Service) recordDiagnostic(level, stage, action, message string, err error, meta map[string]string) {
	s.mu.Lock()
	diagnostics := s.diagnostics
	s.mu.Unlock()
	if diagnostics == nil {
		return
	}
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	diagnostics.Record(DiagnosticEvent{
		Level:     level,
		Component: "producews",
		Stage:     stage,
		Action:    action,
		Message:   message,
		Error:     errText,
		Meta:      meta,
	})
}

// captureIncident snapshots state under Service.mu, then writes the report
// after unlocking. Each queue/serve incident key is written at most once and
// returns the existing report path to later stages of the same incident.
func (s *Service) captureIncident(category, reason string) string {
	s.mu.Lock()
	diagnostics := s.diagnostics
	if diagnostics == nil {
		s.mu.Unlock()
		return ""
	}
	queue := s.queueState
	if len(queue.Demos) > 0 {
		queue.Demos = append([]string(nil), queue.Demos...)
	}
	if s.reportedIncidents == nil {
		s.reportedIncidents = make(map[string]string)
	}
	// A reconnect timeout is the continuation of the original read error, not
	// a second incident. One category per queue/service incident keeps the
	// report list useful while still allowing independent queue and serve
	// failures to be captured.
	key := category
	if path, exists := s.reportedIncidents[key]; exists {
		s.mu.Unlock()
		return path
	}
	// Reserve the category before file I/O so concurrent close/timer paths
	// cannot generate duplicate reports for the same incident.
	s.reportedIncidents[key] = ""
	ws := s.wsState
	takes := s.takeSnapshotLocked()
	s.mu.Unlock()

	path, err := diagnostics.WriteIncident(reason, ws, queue, takes)
	if err != nil {
		diagnostics.Record(DiagnosticEvent{
			Level:     "error",
			Component: "producews",
			Stage:     "incident",
			Action:    "write_failed",
			Message:   "制作 WebSocket 诊断报告写入失败",
			Error:     err.Error(),
		})
		return ""
	}
	s.mu.Lock()
	s.reportedIncidents[key] = path
	s.mu.Unlock()
	diagnostics.Record(DiagnosticEvent{
		Level:     "info",
		Component: "producews",
		Stage:     "incident",
		Action:    "written",
		Message:   "制作 WebSocket 诊断报告已写入",
		Meta:      map[string]string{"report_path": path},
	})
	return path
}

func messagePreview(raw []byte) string {
	const maxPreviewBytes = 512
	if len(raw) > maxPreviewBytes {
		raw = raw[:maxPreviewBytes]
	}
	return strconv.QuoteToASCII(string(raw))
}

func classifyCloseError(err error) closeClassification {
	if err == nil {
		return closeClassification{kind: "normal_close", description: "正常关闭"}
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		description := fmt.Sprintf("远端关闭 code=%d", closeErr.Code)
		if text := strings.TrimSpace(closeErr.Text); text != "" {
			description += " " + text
		}
		if closeErr.Code == websocket.CloseNormalClosure || closeErr.Code == websocket.CloseGoingAway {
			return closeClassification{kind: "normal_close", description: description}
		}
		return closeClassification{kind: "abnormal_close", description: description}
	}
	if errors.Is(err, io.EOF) {
		return closeClassification{kind: "transport_eof", description: "传输 EOF"}
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		if networkErr.Timeout() {
			return closeClassification{kind: "transport_timeout", description: "传输超时"}
		}
		return closeClassification{kind: "transport_error", description: "传输错误"}
	}
	if websocket.IsUnexpectedCloseError(err) {
		return closeClassification{kind: "protocol_error", description: "协议错误"}
	}
	return closeClassification{kind: "read_error", description: "读取错误"}
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
	s.startDiagnostics()

	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	if !isLoopbackListenAddr(s.addr) {
		s.mu.Unlock()
		return fmt.Errorf("produce websocket listener must use a loopback address: %s", s.addr)
	}
	s.startEventWorkerLocked()

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
		s.recordDiagnostic("error", "listen", "initial_listen_failed", "制作 WebSocket 初次监听失败", err, nil)
		return err
	}

	s.adoptListenerLocked(listener)
	s.startSupervisorLocked(listener, 0)
	s.mu.Unlock()
	s.recordDiagnostic("info", "listen", "listening", "制作 WebSocket 已开始监听", nil, map[string]string{"address": listener.Addr().String()})
	return nil
}

// adoptListenerLocked initialises wsState, http.Server, and address fields
// from a freshly-listening net.Listener. Caller must hold s.mu.
func (s *Service) adoptListenerLocked(listener net.Listener) {
	s.listener = listener
	s.address = listener.Addr().String()
	// The first successful :0 bind chooses the plugin session port. Every
	// subsequent supervisor restart must rebind exactly this concrete address;
	// silently choosing another random port would strand the already-launched
	// plugin on the old endpoint.
	if s.relistenAddr == "" {
		s.relistenAddr = s.address
	}
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

			s.mu.Lock()
			listenAddr := s.relistenAddr
			if listenAddr == "" {
				listenAddr = s.addr
			}
			s.mu.Unlock()
			newListener, err := listenFn("tcp", listenAddr)
			if err != nil {
				attempt++
				exhausted := attempt > maxRetryAttempts

				s.mu.Lock()
				if exhausted {
					if s.relistenAddr != "" {
						s.wsState.LastError = rebindRetryExhaustedMessage
					} else {
						s.wsState.LastError = retryExhaustedMessage
					}
				} else {
					s.wsState.LastError = err.Error()
				}
				s.wsState.Connected = false
				s.wsState.UpdatedAtMs = nowMs()
				s.emitWSStateLocked()
				s.mu.Unlock()
				s.recordDiagnostic("warning", "listen", "retry_failed", "制作 WebSocket 监听重试失败", err, map[string]string{
					"attempt": strconv.Itoa(attempt),
				})

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
			s.recordDiagnostic("info", "listen", "retry_listening", "制作 WebSocket 监听已恢复", nil, map[string]string{
				"address": newListener.Addr().String(),
				"attempt": strconv.Itoa(attempt),
			})
			listener = newListener
		}

		// Reset attempt counter — each failure burst gets a fresh budget.
		attempt = 0

		// Serve blocks until the listener is closed or an unrecoverable error
		// occurs. http.Server.Serve always returns a non-nil error.
		s.mu.Lock()
		server := s.server
		s.mu.Unlock()
		if server == nil {
			return
		}
		s.recordDiagnostic("info", "serve", "started", "制作 WebSocket 服务已开始接受连接", nil, map[string]string{
			"address": listener.Addr().String(),
		})
		serveErr := server.Serve(listener)
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
		s.recordDiagnostic("error", "serve", "serve_exited", "制作 WebSocket 服务异常退出", serveErr, nil)
		s.captureIncident("serve", "制作 WebSocket 服务异常退出")

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
	eventCancel := s.eventCancel
	s.eventCancel = nil
	s.eventCtx = nil
	if eventCancel != nil {
		eventCancel()
	}
	diagnostics := s.diagnostics

	if !s.started {
		heartbeatCancel := s.stopHeartbeatLocked(s.heartbeatConnID)
		s.stopAckTimerLocked()
		s.stopReconnectTimerLocked()
		s.stopReplayTimerLocked()
		s.mu.Unlock()
		if heartbeatCancel != nil {
			heartbeatCancel()
			s.heartbeatWG.Wait()
		}
		if cancel != nil {
			s.supervisorWG.Wait()
		}
		if eventCancel != nil {
			s.eventWG.Wait()
		}
		if diagnostics != nil {
			diagnostics.Stop()
		}
		return nil
	}
	server := s.server
	s.listener = nil
	conn := s.gameConn
	s.gameConn = nil
	s.gameConnStartedAt = 0
	s.started = false
	s.stopAckTimerLocked()
	s.stopReconnectTimerLocked()
	s.stopReplayTimerLocked()
	heartbeatCancel := s.stopHeartbeatLocked(s.heartbeatConnID)
	s.address = ""
	s.relistenAddr = ""
	s.mu.Unlock()

	if heartbeatCancel != nil {
		heartbeatCancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
	var closeErr error
	if server != nil {
		closeErr = server.Close()
	}
	// Wait for the supervisor goroutine to exit so callers see a clean state.
	s.supervisorWG.Wait()
	if eventCancel != nil {
		s.eventWG.Wait()
	}
	if diagnostics != nil {
		diagnostics.Stop()
	}
	s.heartbeatWG.Wait()
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

	if !s.started || s.relistenAddr == "" {
		return 0, fmt.Errorf("produce websocket server is not started")
	}

	_, portText, err := net.SplitHostPort(s.relistenAddr)
	if err != nil {
		return 0, fmt.Errorf("produce websocket port is unavailable: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		return 0, fmt.Errorf("produce websocket port is unavailable")
	}
	return port, nil
}

func isLoopbackListenAddr(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
	s.reportedIncidents = make(map[string]string)
	s.gracefulExit = gracefulExitState{}
	s.stopReconnectTimerLocked()
	s.stopReplayTimerLocked()
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
	s.recordDiagnostic("info", "queue", "started", "制作队列已启动", nil, map[string]string{
		"demo_count": strconv.Itoa(len(demos)),
	})

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

// GracefulExitStatus returns the current terminal-session close handshake
// snapshot for the app's session worker. It is not exposed to the frontend.
func (s *Service) GracefulExitStatus() GracefulExitStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return GracefulExitStatus{
		Requested:    s.gracefulExit.requested,
		Acknowledged: s.gracefulExit.acknowledged,
		Completed:    s.gracefulExit.completed,
	}
}

// RequestGracefulExit asks a current plugin to end the completed CS2 session.
// It only issues a typed protocol command after the queue has stopped; it does
// not expose arbitrary game-console execution to the loopback transport.
func (s *Service) RequestGracefulExit() error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return fmt.Errorf("produce websocket server is not started")
	}
	if s.queueState.Running {
		s.mu.Unlock()
		return fmt.Errorf("制作队列尚未完成，不能结束游戏会话")
	}
	if s.gameConn == nil {
		s.mu.Unlock()
		return fmt.Errorf("game websocket not connected")
	}
	if s.gracefulExit.requested && !s.gracefulExit.completed {
		s.mu.Unlock()
		return fmt.Errorf("游戏会话结束请求已发送")
	}
	conn := s.gameConn
	connID := s.gameConnID
	requestID := fmt.Sprintf("%d-%d", connID, nowMs())
	s.gracefulExit = gracefulExitState{
		requestID: requestID,
		connID:    connID,
		requested: true,
	}
	writeTimeout := s.writeTimeout
	s.mu.Unlock()

	err := s.writeOutgoing(conn, outgoingMessage{
		Name:    "end_produce_session",
		Payload: gracefulExitRequestPayload{RequestID: requestID},
	}, writeTimeout)
	if err != nil {
		s.mu.Lock()
		if s.gracefulExit.requestID == requestID && !s.gracefulExit.acknowledged && !s.gracefulExit.completed {
			s.gracefulExit = gracefulExitState{}
		}
		s.mu.Unlock()
		s.recordDiagnostic("warning", "shutdown", "request_failed", "向游戏插件发送有序退出请求失败", err, nil)
		return fmt.Errorf("向游戏插件发送有序退出请求失败: %w", err)
	}
	s.recordDiagnostic("info", "shutdown", "requested", "已请求游戏插件有序结束会话", nil, map[string]string{
		"conn_id":    strconv.FormatUint(connID, 10),
		"request_id": requestID,
	})
	return nil
}

// ExpectGracefulExitFallback marks the current request as an expected close
// before the app uses its PID-based fallback. This keeps the fallback's known
// disconnect separate from an unexpected recording-time transport failure.
func (s *Service) ExpectGracefulExitFallback() bool {
	s.mu.Lock()
	if !s.gracefulExit.requested || s.gracefulExit.completed {
		s.mu.Unlock()
		return false
	}
	s.gracefulExit.expected = true
	s.gracefulExit.deadline = time.Now().Add(s.expectedExitWindow)
	requestID := s.gracefulExit.requestID
	s.mu.Unlock()
	s.recordDiagnostic("warning", "shutdown", "pid_fallback", "游戏插件未及时完成有序退出，准备按 PID 关闭 CS2", nil, map[string]string{
		"request_id": requestID,
	})
	return true
}

// ExportDiagnostics produces a sanitized support report without opening any
// UI. The Wails binding owns SaveFileDialog and file selection.
func (s *Service) ExportDiagnostics() string {
	s.mu.Lock()
	diagnostics := s.diagnostics
	ws := s.wsState
	queue := s.queueState
	if len(queue.Demos) > 0 {
		queue.Demos = append([]string(nil), queue.Demos...)
	}
	takes := s.takeSnapshotLocked()
	s.mu.Unlock()
	if diagnostics == nil {
		return buildDiagnosticExport(time.Now(), ws, queue, takes, DiagnosticsSnapshot{}, nil, nil, "")
	}
	return diagnostics.ExportText(ws, queue, takes)
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
	err := s.writeOutgoing(conn, outgoingMessage{
		Name:    command,
		Payload: payload,
	}, writeTimeout)
	if err != nil {
		s.recordDiagnostic("error", "write", "command_failed", "向游戏插件发送 WebSocket 命令失败", err, map[string]string{"name": command})
		return err
	}
	s.recordDiagnostic("info", "write", "command_sent", "已向游戏插件发送 WebSocket 命令", nil, map[string]string{"name": command})
	return nil
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
	switch process {
	case "game":
		// The current plugin contract.
	case "":
		// easywsclient expects a slash before a query string. Older plugin DLLs
		// used ws://localhost:<port>?process=game, which the library serializes
		// as GET / and therefore drops the query. The listener itself is bound to
		// loopback only, so keep this narrowly scoped compatibility path until
		// every shipped DLL has the corrected URL.
		s.recordDiagnostic("warning", "connection", "legacy_process_missing", "兼容未携带 process=game 的旧版本机插件连接", nil, map[string]string{
			"remote_addr": r.RemoteAddr,
		})
	default:
		s.recordDiagnostic("warning", "connection", "rejected_process", "拒绝非游戏插件 WebSocket 连接", nil, map[string]string{
			"process":     process,
			"remote_addr": r.RemoteAddr,
		})
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
		s.recordDiagnostic("warning", "connection", "upgrade_failed", "游戏插件 WebSocket 升级失败", err, nil)
		return
	}
	conn.SetReadLimit(s.readLimit)
	if err := conn.SetReadDeadline(time.Now().Add(s.pongWait)); err != nil {
		_ = conn.Close()
		s.recordDiagnostic("warning", "connection", "read_deadline_failed", "设置游戏插件 WebSocket 读取超时失败", err, nil)
		return
	}

	var oldConn *websocket.Conn
	var previousHeartbeatCancel context.CancelFunc
	var id uint64
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		_ = conn.Close()
		return
	}
	oldConn = s.gameConn
	previousHeartbeatCancel = s.stopHeartbeatLocked(s.gameConnID)
	s.nextConnID++
	id = s.nextConnID
	s.gameConnID = id
	s.gameConn = conn
	s.gameConnStartedAt = nowMs()
	s.wsState.Connected = true
	s.wsState.LastError = ""
	s.wsState.UpdatedAtMs = nowMs()
	pongWait := s.pongWait
	conn.SetPongHandler(func(payload string) error {
		if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
			return err
		}
		meta := map[string]string{
			"conn_id":    strconv.FormatUint(id, 10),
			"pong_at_ms": strconv.FormatInt(nowMs(), 10),
		}
		if sentAt, err := strconv.ParseInt(payload, 10, 64); err == nil && sentAt > 0 {
			meta["rtt_ms"] = strconv.FormatInt(maxInt64(0, nowMs()-sentAt), 10)
		}
		s.recordDiagnostic("info", "heartbeat", "pong_received", "已收到游戏插件 WebSocket pong", nil, meta)
		return nil
	})
	s.stopReconnectTimerLocked()
	s.stopReplayTimerLocked()
	s.startHeartbeatLocked(id, conn)
	replayPendingAck := s.queueState.Running && s.queueState.PendingAck
	dispatchAfterReconnect := s.queueState.Running && !s.queueState.PendingAck &&
		(s.queueState.CurrentIndex < 0 || s.queueState.Completed > s.queueState.CurrentIndex)
	if replayPendingAck {
		s.startReplayTimerLocked(id, s.queueState.CurrentDemoPath)
	}
	s.emitWSStateLocked()
	s.mu.Unlock()
	if previousHeartbeatCancel != nil {
		// Cancellation is non-blocking. The old goroutine exits once it leaves
		// its ticker select; the connID guard prevents it writing to this conn.
		previousHeartbeatCancel()
	}
	s.recordDiagnostic("info", "connection", "connected", "游戏插件 WebSocket 已连接", nil, map[string]string{
		"conn_id":     strconv.FormatUint(id, 10),
		"remote_addr": conn.RemoteAddr().String(),
	})

	if oldConn != nil {
		_ = oldConn.Close()
	}

	go s.readLoop(id, conn)
	if dispatchAfterReconnect {
		go s.dispatchNextDemo()
	}
}

func (s *Service) readLoop(connID uint64, conn *websocket.Conn) {
	var closeErr error
	defer func() { s.onConnectionClosed(connID, conn, closeErr) }()
	for {
		messageType, raw, err := conn.ReadMessage()
		if err != nil {
			closeErr = err
			return
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			s.recordDiagnostic("warning", "message", "unsupported_frame", "已忽略不支持的 WebSocket 帧类型", nil, map[string]string{
				"conn_id":      strconv.FormatUint(connID, 10),
				"message_type": strconv.Itoa(messageType),
				"size_bytes":   strconv.Itoa(len(raw)),
			})
			continue
		}
		var message incomingMessage
		if err := json.Unmarshal(raw, &message); err != nil {
			s.recordDiagnostic("warning", "message", "invalid_envelope", "已忽略无效 WebSocket 消息封装", err, map[string]string{
				"conn_id":      strconv.FormatUint(connID, 10),
				"message_type": strconv.Itoa(messageType),
				"size_bytes":   strconv.Itoa(len(raw)),
				"preview":      messagePreview(raw),
			})
			continue
		}
		known, payloadErr := s.handleIncomingMessage(message)
		if !known {
			s.recordDiagnostic("warning", "message", "unknown_name", "已忽略未知 WebSocket 消息", nil, map[string]string{
				"conn_id":    strconv.FormatUint(connID, 10),
				"name":       message.Name,
				"size_bytes": strconv.Itoa(len(raw)),
			})
			continue
		}
		if payloadErr != nil {
			s.recordDiagnostic("warning", "message", "invalid_payload", "已忽略无效 WebSocket 业务消息", payloadErr, map[string]string{
				"conn_id":    strconv.FormatUint(connID, 10),
				"name":       message.Name,
				"size_bytes": strconv.Itoa(len(raw)),
			})
			continue
		}
		s.recordDiagnostic("info", "message", "received", "已接收游戏插件 WebSocket 消息", nil, map[string]string{
			"conn_id":    strconv.FormatUint(connID, 10),
			"name":       message.Name,
			"size_bytes": strconv.Itoa(len(raw)),
		})
	}
}

func (s *Service) onConnectionClosed(connID uint64, conn *websocket.Conn, closeErr error) {
	_ = conn.Close()

	classification := classifyCloseError(closeErr)
	remoteAddr := conn.RemoteAddr().String()
	queueWaitingForReconnect := false
	grace := time.Duration(0)
	superseded := false
	serviceStopped := false
	expectedExit := false
	exitRequestID := ""
	connectionDurationMs := int64(0)
	s.mu.Lock()
	if s.gameConnID == connID && s.gameConn == conn {
		connectionDurationMs = maxInt64(0, nowMs()-s.gameConnStartedAt)
		s.gameConn = nil
		s.gameConnStartedAt = 0
		heartbeatCancel := s.stopHeartbeatLocked(connID)
		if heartbeatCancel != nil {
			heartbeatCancel()
		}
		s.wsState.Connected = false
		if s.gracefulExit.requested && s.gracefulExit.expected &&
			s.gracefulExit.connID == connID && time.Now().Before(s.gracefulExit.deadline) {
			expectedExit = true
			exitRequestID = s.gracefulExit.requestID
			s.gracefulExit.completed = true
			s.wsState.LastError = ""
		} else {
			s.wsState.LastError = "游戏插件 WebSocket 已断开（" + classification.description + "）"
		}
		s.wsState.UpdatedAtMs = nowMs()
		s.emitWSStateLocked()
		if s.queueState.Running && !expectedExit {
			s.stopAckTimerLocked()
			s.startReconnectTimerLocked()
			s.queueState.UpdatedAtMs = nowMs()
			s.emitQueueStateLocked()
			queueWaitingForReconnect = true
			grace = s.reconnectGrace
		}
	} else if !s.started && s.gameConnID == connID {
		serviceStopped = true
	} else {
		superseded = true
	}
	s.mu.Unlock()

	if superseded {
		s.recordDiagnostic("info", "connection", "superseded", "旧 WebSocket 连接已被新连接顶替", closeErr, map[string]string{
			"conn_id":     strconv.FormatUint(connID, 10),
			"remote_addr": remoteAddr,
		})
		return
	}
	if serviceStopped {
		s.recordDiagnostic("info", "connection", "service_stopped", "制作 WebSocket 服务已主动停止连接", closeErr, map[string]string{
			"conn_id":     strconv.FormatUint(connID, 10),
			"remote_addr": remoteAddr,
			"close_kind":  "service_stopped",
		})
		return
	}
	if expectedExit {
		s.recordDiagnostic("info", "shutdown", "completed", "制作完成后游戏插件已按预期断开", closeErr, map[string]string{
			"conn_id":     strconv.FormatUint(connID, 10),
			"remote_addr": remoteAddr,
			"close_kind":  classification.kind,
			"request_id":  exitRequestID,
		})
		return
	}
	meta := map[string]string{
		"conn_id":     strconv.FormatUint(connID, 10),
		"remote_addr": remoteAddr,
		"close_kind":  classification.kind,
	}
	if connectionDurationMs > 0 {
		meta["duration_ms"] = strconv.FormatInt(connectionDurationMs, 10)
	}
	s.recordDiagnostic("warning", "connection", "closed", "游戏插件 WebSocket 已断开", closeErr, meta)
	s.captureIncident("connection", "游戏插件 WebSocket 连接已断开")
	if queueWaitingForReconnect {
		s.recordDiagnostic("info", "reconnect", "grace_started", "已开始等待游戏插件 WebSocket 重连", nil, map[string]string{
			"grace_ms": strconv.FormatInt(grace.Milliseconds(), 10),
		})
	}
}

func (s *Service) handleIncomingMessage(message incomingMessage) (bool, error) {
	switch message.Name {
	case "status":
		s.handleStatusAck()
		return true, nil
	case "record_status":
		var payload recordStatusPayload
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			return true, err
		}
		s.handleRecordStatus(payload)
		return true, nil
	case "demo_started":
		var payload demoEventPayload
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			return true, err
		}
		s.handleDemoStarted(payload)
		return true, nil
	case "demo_done":
		var payload demoEventPayload
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			return true, err
		}
		s.handleDemoDone(payload)
		return true, nil
	case "session_exit_ack":
		var payload gracefulExitAckPayload
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			return true, err
		}
		if err := s.handleGracefulExitAck(payload); err != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

func (s *Service) handleGracefulExitAck(payload gracefulExitAckPayload) error {
	requestID := strings.TrimSpace(payload.RequestID)
	if requestID == "" {
		return fmt.Errorf("游戏插件有序退出确认缺少 request_id")
	}

	s.mu.Lock()
	if !s.gracefulExit.requested || s.gracefulExit.requestID != requestID || s.gracefulExit.completed {
		s.mu.Unlock()
		return fmt.Errorf("收到不匹配的游戏插件有序退出确认")
	}
	s.gracefulExit.acknowledged = true
	s.gracefulExit.expected = true
	s.gracefulExit.deadline = time.Now().Add(s.expectedExitWindow)
	connID := s.gracefulExit.connID
	s.mu.Unlock()
	s.recordDiagnostic("info", "shutdown", "acknowledged", "游戏插件已确认将有序退出", nil, map[string]string{
		"conn_id":    strconv.FormatUint(connID, 10),
		"request_id": requestID,
	})
	return nil
}

func (s *Service) handleStatusAck() {
	s.mu.Lock()
	if !s.queueState.Running || !s.queueState.PendingAck {
		s.mu.Unlock()
		return
	}
	s.queueState.PendingAck = false
	s.queueState.UpdatedAtMs = nowMs()
	s.stopAckTimerLocked()
	s.stopReplayTimerLocked()
	s.emitQueueStateLocked()
	s.mu.Unlock()
	s.recordDiagnostic("info", "queue", "ack_received", "已收到 playdemo 确认", nil, nil)
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
	if payload.DemoPath == "" {
		s.mu.Unlock()
		return
	}
	updated := false
	if s.queueState.CurrentDemoPath == "" {
		s.queueState.CurrentDemoPath = payload.DemoPath
		s.queueState.UpdatedAtMs = nowMs()
		s.emitQueueStateLocked()
		updated = true
	}
	s.mu.Unlock()
	if updated {
		s.recordDiagnostic("info", "queue", "demo_started", "游戏插件已开始加载 Demo", nil, map[string]string{"demo_path": payload.DemoPath})
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
	s.stopReplayTimerLocked()
	s.emitQueueStateLocked()
	s.mu.Unlock()
	s.recordDiagnostic("info", "queue", "demo_done", "游戏插件已完成 Demo", nil, map[string]string{"demo_path": payload.DemoPath})

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
		s.recordDiagnostic("info", "queue", "finished", "制作队列已完成", nil, nil)
		return
	}
	if s.gameConn == nil {
		s.startReconnectTimerLocked()
		s.mu.Unlock()
		s.recordDiagnostic("warning", "queue", "dispatch_waiting_for_connection", "切换 Demo 时游戏插件未连接，等待重连", nil, nil)
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
		s.recordDiagnostic("warning", "write", "playdemo_failed", "派发 playdemo 失败，等待连接恢复", err, map[string]string{"demo_path": nextDemo})
		// A failed write means this connection is no longer reliable. Closing
		// it sends the normal read-loop through the single reconnect-grace
		// path instead of creating a second queue-failure path here.
		_ = conn.Close()
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
	s.recordDiagnostic("info", "write", "playdemo_sent", "已向游戏插件派发 playdemo", nil, map[string]string{"demo_path": nextDemo})
}

func (s *Service) startReconnectTimerLocked() {
	if !s.queueState.Running || s.reconnectTimer != nil {
		return
	}
	grace := s.reconnectGrace
	if grace <= 0 {
		grace = defaultReconnectGrace
	}
	s.reconnectDeadline = time.Now().Add(grace)
	s.reconnectTimer = time.AfterFunc(grace, s.onReconnectTimeout)
}

func (s *Service) stopReconnectTimerLocked() {
	if s.reconnectTimer != nil {
		s.reconnectTimer.Stop()
		s.reconnectTimer = nil
	}
	s.reconnectDeadline = time.Time{}
}

func (s *Service) onReconnectTimeout() {
	s.mu.Lock()
	if s.reconnectTimer == nil || !s.queueState.Running || s.gameConn != nil {
		s.mu.Unlock()
		return
	}
	grace := s.reconnectGrace
	s.reconnectTimer = nil
	s.reconnectDeadline = time.Time{}
	reason := fmt.Sprintf("等待游戏插件 WebSocket 重连 %d 秒超时，制作队列已停止", int(grace.Round(time.Second).Seconds()))
	s.failQueueLocked(reason)
	s.mu.Unlock()
	s.recordDiagnostic("error", "reconnect", "grace_expired", "等待游戏插件 WebSocket 重连超时", nil, map[string]string{
		"grace_ms": strconv.FormatInt(grace.Milliseconds(), 10),
	})
	if reportPath := s.captureIncident("connection", "等待游戏插件 WebSocket 重连超时"); reportPath != "" {
		s.mu.Lock()
		if !s.queueState.Running && s.queueState.LastError == reason {
			s.queueState.LastError = reason + "；诊断报告: " + filepath.Base(reportPath)
			s.queueState.UpdatedAtMs = nowMs()
			s.emitQueueStateLocked()
		}
		s.mu.Unlock()
	}
}

func (s *Service) startReplayTimerLocked(connID uint64, demoPath string) {
	s.stopReplayTimerLocked()
	delay := s.replayWindow
	if delay <= 0 {
		delay = defaultReplayWindow
	}
	s.replayTimer = time.AfterFunc(delay, func() {
		s.resendPendingAckDemo(connID, demoPath)
	})
}

func (s *Service) stopReplayTimerLocked() {
	if s.replayTimer != nil {
		s.replayTimer.Stop()
		s.replayTimer = nil
	}
}

func (s *Service) resendPendingAckDemo(connID uint64, demoPath string) {
	s.mu.Lock()
	s.replayTimer = nil
	if !s.queueState.Running || !s.queueState.PendingAck ||
		s.queueState.CurrentDemoPath != demoPath || s.gameConn == nil || s.gameConnID != connID {
		s.mu.Unlock()
		return
	}
	conn := s.gameConn
	writeTimeout := s.writeTimeout
	s.mu.Unlock()

	if err := s.writeOutgoing(conn, outgoingMessage{Name: "playdemo", Payload: demoPath}, writeTimeout); err != nil {
		s.recordDiagnostic("warning", "write", "playdemo_replay_failed", "重发 playdemo 失败，等待连接恢复", err, map[string]string{"demo_path": demoPath})
		_ = conn.Close()
		return
	}

	s.mu.Lock()
	if s.queueState.Running && s.queueState.PendingAck &&
		s.queueState.CurrentDemoPath == demoPath && s.gameConnID == connID && s.gameConn == conn {
		s.startAckTimerLocked(demoPath)
		s.emitQueueStateLocked()
	}
	s.mu.Unlock()
	s.recordDiagnostic("info", "reconnect", "playdemo_resent", "重连后已重发 playdemo", nil, map[string]string{"demo_path": demoPath})
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

func (s *Service) startHeartbeatLocked(connID uint64, conn *websocket.Conn) {
	if s.heartbeatCancel != nil {
		s.heartbeatCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.heartbeatConnID = connID
	s.heartbeatCancel = cancel
	interval := s.heartbeatInterval
	if interval <= 0 {
		interval = defaultHeartbeatInterval
	}
	s.heartbeatWG.Add(1)
	go func() {
		defer s.heartbeatWG.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.mu.Lock()
				current := s.started && s.gameConnID == connID && s.gameConn == conn
				s.mu.Unlock()
				if !current {
					return
				}
				payload := strconv.FormatInt(nowMs(), 10)
				s.writeMu.Lock()
				err := conn.WriteControl(websocket.PingMessage, []byte(payload), time.Now().Add(s.writeTimeout))
				s.writeMu.Unlock()
				if err != nil {
					s.recordDiagnostic("warning", "heartbeat", "ping_failed", "发送游戏插件 WebSocket ping 失败", err, map[string]string{
						"conn_id": strconv.FormatUint(connID, 10),
					})
					_ = conn.Close()
					return
				}
				s.recordDiagnostic("info", "heartbeat", "ping_sent", "已发送游戏插件 WebSocket ping", nil, map[string]string{
					"conn_id": strconv.FormatUint(connID, 10),
				})
			}
		}
	}()
}

// stopHeartbeatLocked detaches the active heartbeat for connID and returns
// its cancellation function. Callers may invoke the returned cancellation
// after releasing Service.mu; cancellation itself is non-blocking.
func (s *Service) stopHeartbeatLocked(connID uint64) context.CancelFunc {
	if s.heartbeatCancel == nil || s.heartbeatConnID != connID {
		return nil
	}
	cancel := s.heartbeatCancel
	s.heartbeatCancel = nil
	s.heartbeatConnID = 0
	return cancel
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
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
	if !s.queueState.Running || !s.queueState.PendingAck {
		s.mu.Unlock()
		return
	}
	if s.queueState.CurrentDemoPath != expectedDemo {
		s.mu.Unlock()
		return
	}
	s.failQueueLocked("等待 playdemo 确认超时: " + expectedDemo)
	s.mu.Unlock()
	s.recordDiagnostic("error", "queue", "ack_timeout", "等待 playdemo 确认超时", nil, map[string]string{"demo_path": expectedDemo})
	s.captureIncident("ack_timeout", "等待 playdemo 确认超时")
}

func (s *Service) finishQueueLocked() {
	s.stopAckTimerLocked()
	s.stopReconnectTimerLocked()
	s.stopReplayTimerLocked()
	s.queueState.Running = false
	s.queueState.PendingAck = false
	s.queueState.CurrentIndex = -1
	s.queueState.CurrentDemoPath = ""
	s.queueState.UpdatedAtMs = nowMs()
	s.emitQueueStateLocked()
}

func (s *Service) failQueueLocked(reason string) {
	s.stopAckTimerLocked()
	s.stopReconnectTimerLocked()
	s.stopReplayTimerLocked()
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
	s.queueEventLocked("produce_ws_state_changed", s.wsState)
}

func (s *Service) emitQueueStateLocked() {
	state := s.queueState
	if len(state.Demos) > 0 {
		state.Demos = append([]string(nil), state.Demos...)
	}
	s.queueEventLocked("produce_queue_state_changed", state)
}

func (s *Service) emitTakeSnapshotLocked() {
	s.queueEventLocked("produce_take_status_changed", s.takeSnapshotLocked())
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
