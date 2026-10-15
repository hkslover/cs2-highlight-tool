package app

import (
	"fmt"
	"strings"
	"time"
)

func validateDebugLaunchRequest(req DebugLaunchRequest) error {
	if req.LaunchArguments != nil && strings.ContainsAny(*req.LaunchArguments, "\x00\r\n") {
		return fmt.Errorf("启动参数不能包含换行或 NUL 字符")
	}
	commands := normalizeDebugCommands(req.StartupCommands)
	if len(commands) > 0 && !req.InjectPlugin {
		return fmt.Errorf("启动后执行指令需要启用插件")
	}
	for _, command := range commands {
		if strings.ContainsAny(command, "\x00\r\n") {
			return fmt.Errorf("启动指令每项只能包含一行，且不能包含 NUL 字符")
		}
	}
	return nil
}

func debugLaunchArguments(req DebugLaunchRequest) string {
	if req.LaunchArguments == nil {
		return defaultHLAEArguments
	}
	return *req.LaunchArguments
}

// Called only by the session watcher: one attempt per launch, never a retry on
// reconnect. It uses the frozen runtime and connection, not whichever session
// happens to be current when a delayed receipt arrives.
func (a *App) executeDebugStartupCommands(rt *debugSessionRuntime) {
	rt.mu.Lock()
	if rt.status != DebugStatusRunning || len(rt.request.StartupCommands) == 0 || rt.startupCommandResult != nil {
		rt.mu.Unlock()
		return
	}
	commands := append([]string(nil), rt.request.StartupCommands...)
	connID := rt.debugReadyConnID
	ready := rt.debugReady
	failure := rt.debugProtocolError
	if !ready && failure == "" && !rt.debugReadyDeadline.IsZero() && !time.Now().Before(rt.debugReadyDeadline) {
		failure = "等待插件就绪超时，启动指令未执行"
	}
	rt.mu.Unlock()
	if !ready && failure == "" {
		return
	}
	result := DebugCommandResponse{Status: DebugCommandStatusFailed, Commands: commands, Error: failure}
	if ready {
		receipt, err := a.sendDebugRequestOnRuntime(rt, connID, "debug_execute", func(id string) any {
			return debugExecutePayload{RequestID: id, Commands: commands}
		})
		if err != nil {
			result.Error = err.Error()
		} else {
			result = receipt
			result.Commands = commands
			if result.Status != DebugCommandStatusExecuted && result.Error == "" {
				result.Error = "插件执行启动指令失败"
			}
		}
	}
	rt.mu.Lock()
	rt.startupCommandResult = &result
	rt.updatedAtMs = a.nextDebugStamp()
	rt.mu.Unlock()
	a.emitDebugSessionState()
}
