package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/producews"
)

// PlayDebugDemo plays one managed Demo in the running debug session. It never
// generates or loads a produce sequence file: the plugin clears queued
// sequences and issues a quoted `playdemo` on the game thread. The returned
// receipt only proves the command was handed to the engine; the actual
// playback state arrives later through `debug_demo_state`.
func (a *App) PlayDebugDemo(req DebugPlayDemoRequest) (DebugPlayDemoResponse, error) {
	rt, connID, err := a.debugReadyRuntime()
	if err != nil {
		return DebugPlayDemoResponse{}, err
	}
	rt.mu.Lock()
	demoRevision := rt.debugDemoRevision
	rt.mu.Unlock()
	demoPath, err := a.resolveManagedDebugDemoPath(req.DemoPath)
	if err != nil {
		return DebugPlayDemoResponse{}, err
	}
	commands := normalizeDebugCommands(req.Commands)

	result, err := a.sendDebugRequestOnRuntime(rt, connID, "debug_play_demo", func(requestID string) any {
		return debugPlayDemoPayload{RequestID: requestID, DemoPath: demoPath, Commands: commands}
	})
	if err != nil {
		return DebugPlayDemoResponse{}, err
	}
	if result.Error == "" && result.Status != DebugCommandStatusExecuted {
		result.Error = "插件执行播放命令失败"
	}

	// Record the requested Demo for display. DemoPlaying is intentionally not
	// set here: only the plugin-observed transition may claim real playback.
	if result.Status == DebugCommandStatusExecuted {
		recordedPath := a.recordDebugDemoRequest(rt, connID, demoRevision, demoPath)
		if recordedPath {
			a.emitDebugSessionState()
		}
	}

	return DebugPlayDemoResponse{
		RequestID: result.RequestID,
		Status:    result.Status,
		Error:     result.Error,
		DemoPath:  demoPath,
	}, nil
}

const debugPlayConfirmationTimeoutMessage = "等待 Demo 播放确认超时，请检查游戏播放状态后重试"

// The sender holds rt.mu and registers the actual wire request before sending.
func (a *App) startDebugPlaybackPendingLocked(rt *debugSessionRuntime, connID uint64, requestID, demoPath string) {
	rt.debugDemoPlayPending = true
	rt.debugDemoPlayPendingPath = demoPath
	rt.debugDemoPlayRequestID = requestID
	rt.debugDemoPlayConnID = connID
	rt.debugDemoPlayPendingDeadline = time.Now().Add(debugPlayConfirmTimeout)
	if rt.lastError == debugPlayConfirmationTimeoutMessage {
		rt.lastError = ""
	}
	rt.debugDemoPlayTimer = time.AfterFunc(debugPlayConfirmTimeout, func() {
		a.onDebugPlaybackTimeout(rt, connID, requestID)
	})
	rt.updatedAtMs = nowMs()
}

func clearDebugPlaybackPendingLocked(rt *debugSessionRuntime) {
	if rt.debugDemoPlayTimer != nil {
		rt.debugDemoPlayTimer.Stop()
		rt.debugDemoPlayTimer = nil
	}
	rt.debugDemoPlayPending = false
	rt.debugDemoPlayPendingPath = ""
	rt.debugDemoPlayRequestID = ""
	rt.debugDemoPlayConnID = 0
	rt.debugDemoPlayPendingDeadline = time.Time{}
}

func (a *App) onDebugPlaybackTimeout(rt *debugSessionRuntime, connID uint64, requestID string) {
	if rt == nil || a.debugRuntimeForAdmission() != rt {
		return
	}
	rt.mu.Lock()
	if debugRuntimeDone(rt) || !rt.debugDemoPlayPending || rt.debugDemoPlayRequestID != requestID ||
		rt.debugDemoPlayConnID != connID || a.produceW == nil || a.produceW.GameConnID() != connID {
		rt.mu.Unlock()
		return
	}
	clearDebugPlaybackPendingLocked(rt)
	if rt.lastError == "" {
		rt.lastError = debugPlayConfirmationTimeoutMessage
	}
	rt.updatedAtMs = nowMs()
	rt.mu.Unlock()
	a.emitDebugSessionStateAsync()
}

// Failed or timed-out receipts may clear only their own request, never a retry.
func (a *App) finishFailedDebugPlayback(rt *debugSessionRuntime, requestID string) {
	rt.mu.Lock()
	changed := rt.debugDemoPlayPending && rt.debugDemoPlayRequestID == requestID
	if changed {
		clearDebugPlaybackPendingLocked(rt)
		rt.updatedAtMs = nowMs()
	}
	rt.mu.Unlock()
	if changed && a.debugRuntimeForAdmission() == rt {
		a.emitDebugSessionStateAsync()
	}
}

// Record a receipt only in the runtime and connection that sent the request.
// Never fetch a replacement runtime as the destination of an old result.
func (a *App) recordDebugDemoRequest(rt *debugSessionRuntime, connID, demoRevision uint64, demoPath string) bool {
	if rt == nil || a.debugRuntimeForAdmission() != rt {
		return false
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if debugRuntimeDone(rt) || !rt.debugReady || rt.debugReadyConnID != connID || a.produceW == nil || a.produceW.GameConnID() != connID {
		return false
	}
	// A receipt must not relabel an already-playing Demo or roll back an
	// engine observation received while the caller was waiting for the receipt.
	if rt.debugDemoPlaying || rt.debugDemoRevision != demoRevision {
		return false
	}
	rt.debugDemoPath = demoPath
	rt.updatedAtMs = nowMs()
	return true
}

// resolveManagedDebugDemoPath validates that the request only targets a Demo
// file inside the current workspace's managed `demo` directory. A stale or
// hand-crafted path outside the workspace is rejected instead of being handed
// to the game as a console command.
func (a *App) resolveManagedDebugDemoPath(rawPath string) (string, error) {
	release, dataDir, err := a.beginManagedWorkspaceUse()
	if err != nil {
		return "", err
	}
	defer release()
	return resolveManagedDebugDemoPathIn(dataDir, rawPath)
}

// resolveManagedDebugDemoPathIn is the validation half of
// resolveManagedDebugDemoPath; the caller already holds the workspace use
// reservation and passes its frozen root.
func resolveManagedDebugDemoPathIn(dataDir string, rawPath string) (string, error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return "", fmt.Errorf("请先选择一个 Demo")
	}
	if err := validateDebugPlaybackPath(path); err != nil {
		return "", err
	}
	if strings.TrimSpace(dataDir) == "" {
		return "", workspaceNotInitializedErr()
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析 Demo 路径失败: %w", err)
	}
	absPath = filepath.Clean(absPath)
	demoRoot := filepath.Join(dataDir, "demo")
	if !editPathWithin(demoRoot, absPath) {
		return "", fmt.Errorf("只能使用当前工作目录中已导入的 Demo")
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("Demo 文件不可用: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("Demo 路径不是文件: %s", absPath)
	}
	if !strings.EqualFold(filepath.Ext(absPath), ".dem") {
		return "", fmt.Errorf("不是有效的 Demo 文件: %s", absPath)
	}
	// Literal containment is not enough: an entry inside `demo` may itself be
	// a symbolic link (or, on Windows, a directory junction/reparse point)
	// that leaves the workspace, and both os.Stat and the file manager follow
	// it. Compare the fully resolved paths instead and fail closed when the
	// chain cannot be resolved.
	if err := ensureManagedRealPathWithin(demoRoot, absPath); err != nil {
		return "", err
	}
	return absPath, nil
}

// ensureManagedRealPathWithin rejects a managed path whose fully resolved
// target escapes the fully resolved managed root. EvalSymlinks resolves every
// component, including Windows reparse points; a path that cannot be resolved
// is rejected rather than opened.
func ensureManagedRealPathWithin(root string, target string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("解析受管控目录真实路径失败: %w", err)
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return fmt.Errorf("解析受管控文件真实路径失败: %w", err)
	}
	if !editPathWithin(realRoot, realTarget) {
		return fmt.Errorf("受管控路径经链接指向目录外，已拒绝")
	}
	return nil
}

// OpenDemoInFolder opens the parent directory of one managed Demo. It is the
// narrow counterpart of OpenDemoDirectory (which only opens the Demo root) and
// of OpenProducedClipInFolder (which only accepts produced videos): the target
// must pass the same managed-path validation as playback, so nested platform
// directories, Chinese characters and spaces all work without exposing paths
// outside the workspace.
func (a *App) OpenDemoInFolder(demoPath string) error {
	release, dataDir, err := a.beginManagedWorkspaceUse()
	if err != nil {
		return err
	}
	defer release()

	path, err := resolveManagedDebugDemoPathIn(dataDir, demoPath)
	if err != nil {
		return err
	}
	return openManagedDirectory(filepath.Dir(path), "Demo 目录")
}

// validateDebugPlaybackPath rejects a path that cannot be safely composed into
// the quoted `playdemo "path"` console command. The plugin owns the quoting, so
// the desktop refuses characters that would break out of it instead of
// constructing a bare `playdemo path`.
func validateDebugPlaybackPath(path string) error {
	if strings.ContainsAny(path, "\"\r\n") {
		return fmt.Errorf("Demo 路径包含不能用于播放命令的字符")
	}
	for _, r := range path {
		if r < 0x20 {
			return fmt.Errorf("Demo 路径包含控制字符")
		}
	}
	return nil
}

// handleDebugDemoState records the plugin-observed playback transition. It runs
// on the websocket read goroutine and only accepts a state that belongs to the
// current session and connection; a leftover game connection can never change
// this session's playback state.
func (a *App) handleDebugDemoState(message producews.DebugIncomingMessage) {
	var payload DebugDemoState
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		return
	}
	rt := a.debugRuntimeForAdmission()
	if rt == nil || debugRuntimeDone(rt) {
		return
	}

	rt.mu.Lock()
	if a.produceW == nil || message.ConnID == 0 || a.produceW.GameConnID() != message.ConnID {
		rt.mu.Unlock()
		return
	}
	if !rt.request.InjectPlugin {
		rt.mu.Unlock()
		return
	}
	sessionID := strings.TrimSpace(payload.SessionID)
	if sessionID == "" || sessionID != rt.sessionID {
		rt.mu.Unlock()
		return
	}

	demoPath := strings.TrimSpace(payload.DemoPath)
	// A pause is only meaningful while the Demo is actually loaded; a stopped
	// playback can never keep reporting a paused state.
	paused := payload.Paused && payload.Playing
	// An observation of the requested Demo actually playing is the only proof
	// that lets pause/resume act again. It also covers the same-file replay
	// case, where the plugin only emits this message once its playback gate has
	// confirmed the reload.
	confirmedPending := rt.debugDemoPlayPending && payload.Playing && demoPath != "" &&
		rt.debugDemoPlayPendingPath == demoPath
	if confirmedPending {
		clearDebugPlaybackPendingLocked(rt)
	}
	rt.debugDemoRevision++
	changed := confirmedPending || rt.debugDemoPlaying != payload.Playing || rt.debugDemoPaused != paused || rt.debugDemoPath != demoPath
	rt.debugDemoPlaying = payload.Playing
	rt.debugDemoPaused = paused
	if demoPath != "" {
		rt.debugDemoPath = demoPath
	}
	if !changed {
		rt.mu.Unlock()
		return
	}
	rt.updatedAtMs = nowMs()
	rt.mu.Unlock()
	a.emitDebugSessionStateAsync()
}
