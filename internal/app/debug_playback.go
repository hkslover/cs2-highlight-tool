package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		if a.recordDebugDemoRequest(rt, connID, demoRevision, demoPath) {
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
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return "", fmt.Errorf("请先选择一个 Demo")
	}
	if err := validateDebugPlaybackPath(path); err != nil {
		return "", err
	}

	release, dataDir, err := a.beginManagedWorkspaceUse()
	if err != nil {
		return "", err
	}
	defer release()
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
		return "", fmt.Errorf("只能播放当前工作目录中已导入的 Demo")
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
	return absPath, nil
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
	rt.debugDemoRevision++
	changed := rt.debugDemoPlaying != payload.Playing || rt.debugDemoPath != demoPath
	rt.debugDemoPlaying = payload.Playing
	rt.debugDemoTick = payload.Tick
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
