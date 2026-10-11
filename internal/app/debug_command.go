package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/producews"
)

// Package-level seams for the interactive debug protocol. Tests shorten the
// receipt timeout and the first-ready window without a real plugin.
var (
	debugCommandTimeout = 8 * time.Second
	debugReadyTimeout   = 20 * time.Second
)

type debugSessionReadyPayload struct {
	SessionID       string `json:"session_id"`
	ProtocolVersion int    `json:"protocol_version"`
}

type debugCommandResultPayload struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type debugExecutePayload struct {
	RequestID string   `json:"request_id"`
	Commands  []string `json:"commands"`
}

// debugPendingCommand binds one in-flight receipt to the connection it was
// sent on. A result from another connection, or a reconnect that replaces the
// connection, must not satisfy or keep alive a request that belongs to the old
// transport.
type debugPendingCommand struct {
	connID uint64
	waiter chan DebugCommandResponse
}

func newDebugRequestID() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "req-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "req-" + strconv.FormatInt(time.Now().UnixMilli(), 36) + "-" + hex.EncodeToString(buf[:])
}

// attachDebugMessageHandler wires the produce transport's debug sink to this
// App. It is idempotent and must be called again whenever produceW is replaced.
func (a *App) attachDebugMessageHandler() {
	if a == nil || a.produceW == nil {
		return
	}
	a.produceW.SetDebugMessageHandler(a.handleDebugIncomingMessage)
}

// handleDebugIncomingMessage runs on the websocket read goroutine. It only
// updates session state and hands results to waiting callers; it never calls
// Wails synchronously and never touches the produce queue handlers.
func (a *App) handleDebugIncomingMessage(message producews.DebugIncomingMessage) {
	switch message.Name {
	case "debug_session_ready":
		a.handleDebugSessionReady(message)
	case "debug_command_result":
		a.handleDebugCommandResult(message)
	}
}

// emitDebugSessionStateAsync decouples the debug-protocol read goroutine from
// the potentially blocking Wails emitter, mirroring the producews event worker.
func (a *App) emitDebugSessionStateAsync() {
	if a == nil || a.ctx == nil {
		return
	}
	go a.emitDebugSessionState()
}

func (a *App) handleDebugSessionReady(message producews.DebugIncomingMessage) {
	var payload debugSessionReadyPayload
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		return
	}
	rt := a.debugRuntimeForAdmission()
	if rt == nil || debugRuntimeDone(rt) {
		return
	}

	rt.mu.Lock()
	// An old readLoop may already have decoded a handshake when its connection
	// is replaced. Check the live transport while serializing runtime updates,
	// before either readiness or protocol errors can replace the new state.
	if a.produceW == nil || message.ConnID == 0 || a.produceW.GameConnID() != message.ConnID {
		rt.mu.Unlock()
		return
	}
	if !rt.request.InjectPlugin {
		rt.mu.Unlock()
		return
	}
	sessionID := strings.TrimSpace(payload.SessionID)
	// A leftover game connection from another session must never become ready
	// for this one. Only the exact frozen session id is accepted.
	if sessionID == "" || sessionID != rt.sessionID {
		rt.mu.Unlock()
		return
	}
	if payload.ProtocolVersion != DebugProtocolVersion {
		rt.debugReady = false
		rt.debugReadyConnID = 0
		rt.debugReadySessionID = ""
		rt.debugProtocolError = "插件调试协议版本不受支持，请更新插件"
		rt.updatedAtMs = nowMs()
		rt.mu.Unlock()
		a.emitDebugSessionStateAsync()
		return
	}
	// A handshake on a new connection supersedes the previous one even when the
	// asynchronous transport refresh has not run yet. Fail receipts bound to any
	// other connection so they cannot keep waiting on a dead transport.
	if rt.debugReadyConnID != 0 && rt.debugReadyConnID != message.ConnID {
		a.failDebugPendingForConnLocked(rt, message.ConnID, "插件连接已重建，命令未执行")
	}
	rt.debugReady = true
	rt.debugReadyConnID = message.ConnID
	rt.debugReadySessionID = sessionID
	rt.debugProtocolError = ""
	rt.updatedAtMs = nowMs()
	rt.mu.Unlock()
	a.emitDebugSessionStateAsync()
}

func (a *App) handleDebugCommandResult(message producews.DebugIncomingMessage) {
	var payload debugCommandResultPayload
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		return
	}
	requestID := strings.TrimSpace(payload.RequestID)
	if requestID == "" {
		return
	}
	rt := a.debugRuntimeForAdmission()
	if rt == nil {
		return
	}
	rt.mu.Lock()
	if a.produceW == nil || message.ConnID == 0 || a.produceW.GameConnID() != message.ConnID {
		rt.mu.Unlock()
		return
	}
	pending := rt.debugPending[requestID]
	// Only a receipt that arrived on the same connection the request was sent
	// on may satisfy it; a late result from a replaced connection is ignored.
	if pending != nil && pending.connID == message.ConnID {
		delete(rt.debugPending, requestID)
	} else {
		pending = nil
	}
	rt.mu.Unlock()
	if pending == nil {
		return
	}
	status := strings.TrimSpace(payload.Status)
	if status != DebugCommandStatusExecuted {
		status = DebugCommandStatusFailed
	}
	select {
	case pending.waiter <- DebugCommandResponse{
		RequestID: requestID,
		Status:    status,
		Error:     strings.TrimSpace(payload.Error),
	}:
	default:
	}
}

// failDebugCommandChannelLocked invalidates readiness and fails every pending
// receipt. The caller must hold rt.mu. Failures are delivered through the same
// buffered channel as a plugin receipt so a waiter never blocks on a dead
// transport; no command is ever resent automatically.
func (a *App) failDebugCommandChannelLocked(rt *debugSessionRuntime, reason string) {
	if rt == nil {
		return
	}
	rt.debugReady = false
	rt.debugReadyConnID = 0
	rt.debugReadySessionID = ""
	a.failDebugPendingForConnLocked(rt, 0, reason)
}

// failDebugPendingForConnLocked fails every pending receipt that does not
// belong to keepConnID. keepConnID 0 fails all of them. The caller must hold
// rt.mu.
func (a *App) failDebugPendingForConnLocked(rt *debugSessionRuntime, keepConnID uint64, reason string) {
	if rt == nil {
		return
	}
	for requestID, pending := range rt.debugPending {
		if pending == nil || pending.connID == keepConnID {
			continue
		}
		delete(rt.debugPending, requestID)
		select {
		case pending.waiter <- DebugCommandResponse{
			RequestID: requestID,
			Status:    DebugCommandStatusFailed,
			Error:     reason,
		}:
		default:
		}
	}
}

// stopDebugReadyTimerLocked stops the first-ready deadline timer. The caller
// must hold rt.mu.
func (a *App) stopDebugReadyTimerLocked(rt *debugSessionRuntime) {
	if rt == nil || rt.debugReadyTimer == nil {
		return
	}
	rt.debugReadyTimer.Stop()
	rt.debugReadyTimer = nil
}

// onDebugReadyTimeout actively publishes the first-ready timeout so the page
// does not depend on a later snapshot read or an unrelated event. It is a
// no-op once the session is ready, replaced or already finished.
func (a *App) onDebugReadyTimeout(rt *debugSessionRuntime) {
	if rt == nil || debugRuntimeDone(rt) {
		return
	}
	if a.debugRuntimeForAdmission() != rt {
		return
	}
	rt.mu.Lock()
	if rt.debugReady {
		rt.mu.Unlock()
		return
	}
	rt.updatedAtMs = nowMs()
	rt.mu.Unlock()
	a.emitDebugSessionStateAsync()
}

// SendDebugConsoleCommand sends one debug request and waits for the game-thread
// receipt. It refuses to run without a current, ready debug session, and a
// timeout or disconnect fails the request instead of resending it.
func (a *App) SendDebugConsoleCommand(req DebugCommandRequest) (DebugCommandResponse, error) {
	commands := normalizeDebugCommands(req.Commands)
	if len(commands) == 0 {
		return DebugCommandResponse{}, fmt.Errorf("请输入要执行的命令")
	}

	rt := a.debugRuntimeForAdmission()
	if rt == nil || debugRuntimeDone(rt) {
		return DebugCommandResponse{}, fmt.Errorf("未启动调试会话，无法发送命令")
	}

	rt.mu.Lock()
	if !rt.request.InjectPlugin {
		rt.mu.Unlock()
		return DebugCommandResponse{}, fmt.Errorf("当前调试会话未启用插件，无法远程执行命令")
	}
	if !rt.debugReady {
		protocolError := rt.debugProtocolError
		rt.mu.Unlock()
		if protocolError != "" {
			return DebugCommandResponse{}, fmt.Errorf("%s", protocolError)
		}
		return DebugCommandResponse{}, fmt.Errorf("插件调试协议未就绪，请确认已加载支持调试的插件")
	}
	readyConnID := rt.debugReadyConnID
	rt.mu.Unlock()

	if a.produceW == nil {
		return DebugCommandResponse{}, fmt.Errorf("制作 WebSocket 服务未初始化")
	}

	requestID := newDebugRequestID()
	waiter := make(chan DebugCommandResponse, 1)
	rt.mu.Lock()
	if rt.debugPending == nil {
		rt.debugPending = make(map[string]*debugPendingCommand)
	}
	rt.debugPending[requestID] = &debugPendingCommand{connID: readyConnID, waiter: waiter}
	rt.mu.Unlock()

	if err := a.produceW.SendCommandToConn(readyConnID, "debug_execute", debugExecutePayload{
		RequestID: requestID,
		Commands:  commands,
	}); err != nil {
		rt.mu.Lock()
		delete(rt.debugPending, requestID)
		rt.mu.Unlock()
		return DebugCommandResponse{}, fmt.Errorf("向插件发送命令失败: %w", err)
	}

	timer := time.NewTimer(debugCommandTimeout)
	defer timer.Stop()
	select {
	case result := <-waiter:
		result.Commands = commands
		if result.Error == "" && result.Status != DebugCommandStatusExecuted {
			result.Error = "插件执行命令失败"
		}
		return result, nil
	case <-timer.C:
		rt.mu.Lock()
		delete(rt.debugPending, requestID)
		rt.mu.Unlock()
		return DebugCommandResponse{}, fmt.Errorf("等待插件执行回执超时，命令可能未执行")
	case <-rt.done:
		return DebugCommandResponse{}, fmt.Errorf("调试会话已结束，命令未执行")
	}
}

// normalizeDebugCommands trims whitespace and drops empty entries. A custom
// input with semicolons stays a single element so the engine receives the
// original string, including quotes.
func normalizeDebugCommands(commands []string) []string {
	result := make([]string, 0, len(commands))
	for _, command := range commands {
		trimmed := strings.TrimSpace(command)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}
