package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"cs2-highlight-tool-v2/internal/config"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Package-level seams in the same style as the produce pipeline: tests shorten
// timeouts or drive process enumeration without a real CS2 install.

var debugSessionStopWait = 10 * time.Second
var debugProcessPollInterval = time.Second

// debugSessionRuntime is the private identity of one interactive debug session.
// It owns the environment epoch, the launched CS2 PID (or the unconfirmed HLAE
// launcher handle), the workspace file reservation and the lifecycle context.
// Its status is separate from the produce queue and startup state machines.
type debugSessionRuntime struct {
	sessionID string
	request   DebugLaunchRequest
	envEpoch  uint64

	cancel context.CancelFunc
	done   chan struct{}

	mu               sync.Mutex
	status           string
	lastError        string
	cs2PID           int
	pendingLaunch    *pendingHLAELaunch
	processStarted   bool
	processClosed    bool
	teardownErr      error
	releaseWorkspace func()
	updatedAtMs      int64

	// Interactive debug protocol state. Readiness is only true after the plugin
	// confirms this exact session id over a connection that is still current;
	// a connected transport is not proof of the debug protocol.
	debugReady          bool
	debugReadyConnID    uint64
	debugReadySessionID string
	debugProtocolError  string
	debugReadyDeadline  time.Time
	debugReadyTimer     *time.Timer
	debugPending        map[string]*debugPendingCommand

	// Demo playback state is only ever set from a plugin `debug_demo_state`
	// transition (or cleared when the transport that observed it is gone).
	debugDemoPath     string
	debugDemoPlaying  bool
	debugDemoTick     int
	debugDemoRevision uint64
}

func newDebugSessionID() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "dbg-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "dbg-" + strconv.FormatInt(time.Now().UnixMilli(), 36) + "-" + hex.EncodeToString(buf[:])
}

func debugRuntimeDone(rt *debugSessionRuntime) bool {
	if rt == nil {
		return true
	}
	select {
	case <-rt.done:
		return true
	default:
		return false
	}
}

func (a *App) debugRuntimeForAdmission() *debugSessionRuntime {
	if a == nil {
		return nil
	}
	a.debugStateMu.Lock()
	defer a.debugStateMu.Unlock()
	return a.debugRuntime
}

// debugBusyError is the admission check for work that must not overlap a live
// debug session. A finished runtime whose teardown failed is intentionally not
// busy here: callers retry the teardown through stopDebugSessionWorker instead
// of being rejected forever.
func (a *App) debugBusyError() error {
	rt := a.debugRuntimeForAdmission()
	if rt != nil && !debugRuntimeDone(rt) {
		return fmt.Errorf("调试会话正在进行中，请先关闭调试游戏")
	}
	return nil
}

// debugRetainedForActivity reports whether the debug workbench still owns game
// files or a process, including a finished runtime with a failed teardown.
func (a *App) debugRetainedForActivity() bool {
	rt := a.debugRuntimeForAdmission()
	if rt == nil {
		return false
	}
	if !debugRuntimeDone(rt) {
		return true
	}
	rt.mu.Lock()
	teardownErr := rt.teardownErr
	rt.mu.Unlock()
	return teardownErr != nil
}

func (a *App) installDebugRuntime(rt *debugSessionRuntime) {
	a.debugStateMu.Lock()
	a.debugRuntime = rt
	a.debugStateMu.Unlock()
}

func (a *App) emitDebugSessionState() {
	if a == nil || a.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "debug_session_state_changed", a.GetDebugSessionState())
}

// refreshDebugPluginState re-emits the debug snapshot after a transport change
// and invalidates debug-protocol readiness that the new transport no longer
// proves. A reconnect must not leave the page accepting commands on a
// connection that has already been replaced.
func (a *App) refreshDebugPluginState() {
	rt := a.debugRuntimeForAdmission()
	if rt == nil || debugRuntimeDone(rt) {
		return
	}
	rt.mu.Lock()
	if !rt.request.InjectPlugin {
		rt.mu.Unlock()
		return
	}
	connected := a.produceW != nil && a.produceW.GetWSState().Connected
	switch {
	case !connected:
		a.failDebugCommandChannelLocked(rt, "插件连接已断开，命令未执行")
		// Playback state cannot be confirmed without the plugin that observed
		// it. Clear it instead of leaving a stale "playing" on the page.
		rt.debugDemoPlaying = false
	case rt.debugReady && a.produceW != nil && a.produceW.GameConnID() != rt.debugReadyConnID:
		a.failDebugCommandChannelLocked(rt, "插件连接已重建，等待调试协议就绪")
		rt.debugDemoPlaying = false
	}
	rt.updatedAtMs = nowMs()
	rt.mu.Unlock()
	a.emitDebugSessionState()
}

// GetDebugSessionState returns the current public snapshot. It never infers a
// plugin debug connection from the process state.
func (a *App) GetDebugSessionState() DebugSessionState {
	if a == nil {
		return DebugSessionState{Status: DebugStatusIdle, PluginMessage: "未启动调试会话", UpdatedAtMs: nowMs()}
	}
	return a.debugSessionSnapshot(a.debugRuntimeForAdmission())
}

func (a *App) debugSessionSnapshot(rt *debugSessionRuntime) DebugSessionState {
	if rt == nil {
		return DebugSessionState{
			Status:        DebugStatusIdle,
			PluginMessage: "未启动调试会话",
			UpdatedAtMs:   nowMs(),
		}
	}
	rt.mu.Lock()
	state := DebugSessionState{
		SessionID:        rt.sessionID,
		Status:           rt.status,
		InjectPlugin:     rt.request.InjectPlugin,
		PovHudEnabled:    rt.request.PovHudEnabled,
		BrandEnabled:     rt.request.BrandEnabled,
		PovRadarEnabled:  rt.request.PovRadarEnabled,
		LaunchResolution: rt.request.LaunchResolution,
		ProcessStarted:   rt.processStarted,
		CS2PID:           rt.cs2PID,
		DemoPath:         rt.debugDemoPath,
		DemoPlaying:      rt.debugDemoPlaying,
		DemoTick:         rt.debugDemoTick,
		Error:            rt.lastError,
		UpdatedAtMs:      rt.updatedAtMs,
	}
	rt.mu.Unlock()
	a.fillDebugPluginState(rt, &state)
	return state
}

// fillDebugPluginState separates transport connection from debug-protocol
// readiness. Readiness requires the plugin's `debug_session_ready` handshake
// for this session over the live connection; a reconnect invalidates it until
// the plugin resends the handshake. The first-ready deadline is reported even
// when the transport never connected, so an old DLL or a missing handshake is
// visible instead of an indefinite "waiting" message.
func (a *App) fillDebugPluginState(rt *debugSessionRuntime, state *DebugSessionState) {
	state.PluginEnabled = state.InjectPlugin
	if !state.InjectPlugin {
		state.PluginConnected = false
		state.PluginReady = false
		state.PluginMessage = "未启用插件"
		return
	}
	connected := a.produceW != nil && a.produceW.GetWSState().Connected
	state.PluginConnected = connected
	state.PluginReady = false
	if rt == nil {
		state.PluginMessage = "等待插件连接"
		return
	}

	rt.mu.Lock()
	ready := rt.debugReady
	readyConnID := rt.debugReadyConnID
	protocolError := rt.debugProtocolError
	readyDeadline := rt.debugReadyDeadline
	rt.mu.Unlock()

	if protocolError != "" {
		state.PluginMessage = protocolError
		return
	}
	if ready && connected && a.produceW != nil && a.produceW.GameConnID() == readyConnID {
		state.PluginReady = true
		state.PluginMessage = "插件调试协议已就绪"
		return
	}
	if !readyDeadline.IsZero() && time.Now().After(readyDeadline) {
		state.PluginMessage = "插件未连接或不支持调试协议"
		return
	}
	if !connected {
		state.PluginMessage = "等待插件连接"
		return
	}
	state.PluginMessage = "插件已连接，等待调试协议就绪"
}

// LaunchDebugGame prepares the selected environment and starts HLAE. Both modes
// inject AfxHookSource2.dll; inject_plugin only controls this project's plugin
// transport. The request is frozen into the session and never written to the
// persisted config.
func (a *App) LaunchDebugGame(req DebugLaunchRequest) (DebugSessionState, error) {
	a.produceLaunchMu.Lock()
	defer a.produceLaunchMu.Unlock()

	if err := a.produceBusyError(); err != nil {
		return a.GetDebugSessionState(), err
	}
	// Retry a finished-but-failed teardown of either runtime before preparing a
	// new environment. Both modes share the same gameinfo/DLL/VPK state, so a
	// retained produce session must be released before this launch can safely
	// advance the environment epoch.
	if err := a.stopDebugSessionWorker(); err != nil {
		return a.GetDebugSessionState(), err
	}
	if err := a.stopProduceSessionWorker(); err != nil {
		return a.GetDebugSessionState(), err
	}

	// The debug protocol handler must be installed before the plugin can
	// connect, so a fresh service (tests replace produceW) is covered too.
	a.attachDebugMessageHandler()

	normalized := normalizeDebugLaunchRequest(req)

	workCtx, releaseWorkspace, _, err := a.beginManagedWorkspaceTaskUse()
	if err != nil {
		return a.GetDebugSessionState(), err
	}
	envEpoch := a.beginProduceEnvironmentPrep()

	sessionID := newDebugSessionID()
	ctx, cancel := context.WithCancel(workCtx)
	rt := &debugSessionRuntime{
		sessionID:        sessionID,
		request:          normalized,
		envEpoch:         envEpoch,
		cancel:           cancel,
		done:             make(chan struct{}),
		releaseWorkspace: releaseWorkspace,
		status:           DebugStatusStarting,
		updatedAtMs:      nowMs(),
	}
	a.installDebugRuntime(rt)
	a.emitDebugSessionState()

	cfg, err := a.loadConfig()
	if err != nil {
		return a.failDebugLaunch(rt, err)
	}

	if err := a.prepareGameInfoWithConfig(cfg, gameInfoPrepOptions{
		InjectPlugin:  normalized.InjectPlugin,
		BrandEnabled:  normalized.BrandEnabled,
		PovHudEnabled: normalized.PovHudEnabled,
	}); err != nil {
		return a.failDebugLaunch(rt, err)
	}
	if normalized.InjectPlugin {
		if err := a.preparePluginDLLWithConfig(cfg); err != nil {
			return a.failDebugLaunch(rt, err)
		}
	}
	if err := a.preparePovWithConfig(cfg, normalized.PovHudEnabled); err != nil {
		return a.failDebugLaunch(rt, err)
	}
	if err := a.prepareBrandWithConfig(cfg, normalized.BrandEnabled); err != nil {
		return a.failDebugLaunch(rt, err)
	}

	cs2PID, err := a.launchHLAEGameWithOptions(cfg, hlaeLaunchOptions{
		LaunchResolution: normalized.LaunchResolution,
		PluginEnabled:    normalized.InjectPlugin,
		DebugSessionID:   sessionID,
	})
	if err != nil {
		return a.failDebugLaunch(rt, err)
	}

	rt.mu.Lock()
	rt.cs2PID = cs2PID
	rt.processStarted = true
	rt.status = DebugStatusRunning
	if normalized.InjectPlugin {
		rt.debugReadyDeadline = time.Now().Add(debugReadyTimeout)
		rt.debugReadyTimer = time.AfterFunc(debugReadyTimeout, func() {
			a.onDebugReadyTimeout(rt)
		})
	}
	rt.updatedAtMs = nowMs()
	rt.mu.Unlock()

	go a.runDebugSessionWatcher(ctx, rt)
	a.emitDebugSessionState()
	return a.GetDebugSessionState(), nil
}

// StopDebugGame ends the current session: it stops the watcher, closes the
// owned process, restores the environment and releases the workspace
// reservation. A failed teardown keeps the runtime so the call can be retried.
func (a *App) StopDebugGame() (DebugSessionState, error) {
	a.produceLaunchMu.Lock()
	defer a.produceLaunchMu.Unlock()

	rt := a.debugRuntimeForAdmission()
	if rt == nil {
		return a.GetDebugSessionState(), nil
	}
	rt.mu.Lock()
	if rt.status == DebugStatusRunning || rt.status == DebugStatusStarting {
		rt.status = DebugStatusStopping
		rt.updatedAtMs = nowMs()
	}
	rt.mu.Unlock()
	a.emitDebugSessionState()

	if err := a.stopDebugSessionWorker(); err != nil {
		return a.GetDebugSessionState(), err
	}
	return a.GetDebugSessionState(), nil
}

// stopDebugSessionWorker cancels the watcher, waits for it to finish, retries
// the teardown synchronously and releases the runtime only on success. It
// mirrors stopProduceSessionWorker so a transient PID or file-lock failure is
// retryable instead of silently dropping ownership.
func (a *App) stopDebugSessionWorker() error {
	rt := a.debugRuntimeForAdmission()
	if rt == nil {
		return nil
	}
	if rt.cancel != nil {
		rt.cancel()
	}
	// Unblock any command waiting on a receipt before waiting for the watcher.
	rt.mu.Lock()
	a.failDebugCommandChannelLocked(rt, "调试会话已结束")
	a.stopDebugReadyTimerLocked(rt)
	rt.mu.Unlock()
	select {
	case <-rt.done:
	case <-time.After(debugSessionStopWait):
		return fmt.Errorf("调试会话在 %s 内未完全退出，请稍后重试", debugSessionStopWait)
	}

	if err := a.debugTeardown(rt); err != nil {
		rt.mu.Lock()
		rt.teardownErr = err
		rt.status = DebugStatusFailed
		rt.lastError = err.Error()
		rt.updatedAtMs = nowMs()
		rt.mu.Unlock()
		a.emitDebugSessionState()
		return err
	}

	rt.mu.Lock()
	rt.teardownErr = nil
	rt.updatedAtMs = nowMs()
	rt.mu.Unlock()
	a.releaseDebugWorkspace(rt)
	a.debugStateMu.Lock()
	if a.debugRuntime == rt {
		a.debugRuntime = nil
	}
	a.debugStateMu.Unlock()
	a.emitDebugSessionState()
	return nil
}

// failDebugLaunch records a failed launch, attempts the rollback and closes the
// runtime. An unconfirmed launcher handle or a failed restore is retained with
// its environment backups so StopDebugGame can retry.
func (a *App) failDebugLaunch(rt *debugSessionRuntime, launchErr error) (DebugSessionState, error) {
	var hlaeErr *hlaeLaunchError
	var pending *pendingHLAELaunch
	if errors.As(launchErr, &hlaeErr) {
		pending = hlaeErr.pending
	}
	rt.mu.Lock()
	rt.pendingLaunch = pending
	rt.status = DebugStatusFailed
	rt.lastError = launchErr.Error()
	rt.updatedAtMs = nowMs()
	rt.mu.Unlock()

	teardownErr := a.debugTeardown(rt)
	rt.mu.Lock()
	rt.teardownErr = teardownErr
	if teardownErr != nil {
		rt.lastError = fmt.Sprintf("%s; 恢复调试环境失败: %v", launchErr.Error(), teardownErr)
	}
	rt.updatedAtMs = nowMs()
	rt.mu.Unlock()
	if rt.cancel != nil {
		rt.cancel()
	}
	close(rt.done)
	if teardownErr == nil {
		a.releaseDebugWorkspace(rt)
	}
	a.emitDebugSessionState()
	return a.GetDebugSessionState(), launchErr
}

// runDebugSessionWatcher detects a user-driven game exit (manual close or a
// custom `quit`) and runs the same teardown as StopDebugGame. It exits on
// context cancellation as well, so a stop request never has to wait for a poll.
func (a *App) runDebugSessionWatcher(ctx context.Context, rt *debugSessionRuntime) {
	defer close(rt.done)
	defer func() {
		teardownErr := a.debugTeardown(rt)
		rt.mu.Lock()
		a.stopDebugReadyTimerLocked(rt)
		rt.teardownErr = teardownErr
		if teardownErr != nil {
			rt.status = DebugStatusFailed
			rt.lastError = teardownErr.Error()
		} else {
			rt.status = DebugStatusIdle
			rt.lastError = ""
		}
		rt.updatedAtMs = nowMs()
		rt.mu.Unlock()
		if teardownErr == nil {
			a.releaseDebugWorkspace(rt)
		}
		a.emitDebugSessionState()
	}()

	ticker := time.NewTicker(debugProcessPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rt.mu.Lock()
			pid := rt.cs2PID
			rt.mu.Unlock()
			if pid <= 0 {
				continue
			}
			pids, err := listCS2PIDsFn()
			if err != nil {
				// Enumeration failure must not be mistaken for a game exit.
				continue
			}
			if !pidInList(pids, pid) {
				return
			}
		}
	}
}

func (a *App) debugTeardown(rt *debugSessionRuntime) error {
	if rt == nil {
		return nil
	}
	if err := a.forceCloseDebugProcess(rt); err != nil {
		return err
	}
	if err := a.forceRestoreProduceEnvironmentForEpoch(rt.envEpoch); err != nil {
		return err
	}
	return nil
}

func (a *App) forceCloseDebugProcess(rt *debugSessionRuntime) error {
	if rt == nil {
		return nil
	}
	rt.mu.Lock()
	if rt.processClosed {
		rt.mu.Unlock()
		return nil
	}
	pending := rt.pendingLaunch
	pid := rt.cs2PID
	rt.mu.Unlock()

	if pending != nil {
		if err := pending.stop(); err != nil {
			return err
		}
		rt.mu.Lock()
		rt.pendingLaunch = nil
		rt.mu.Unlock()
	}
	if pid > 0 {
		if err := closeCS2ProcessByPIDFn(pid); err != nil {
			return err
		}
	}
	rt.mu.Lock()
	rt.processClosed = true
	rt.mu.Unlock()
	return nil
}

func (a *App) releaseDebugWorkspace(rt *debugSessionRuntime) {
	if rt == nil {
		return
	}
	rt.mu.Lock()
	release := rt.releaseWorkspace
	rt.releaseWorkspace = nil
	rt.mu.Unlock()
	if release != nil {
		release()
	}
}

func normalizeDebugLaunchRequest(req DebugLaunchRequest) DebugLaunchRequest {
	resolution := strings.TrimSpace(req.LaunchResolution)
	if resolution == "" || !config.IsSupportedLaunchResolution(resolution) {
		resolution = config.DefaultLaunchResolution
	}
	req.LaunchResolution = resolution
	return req
}

func pidInList(pids []int, pid int) bool {
	for _, candidate := range pids {
		if candidate == pid {
			return true
		}
	}
	return false
}
