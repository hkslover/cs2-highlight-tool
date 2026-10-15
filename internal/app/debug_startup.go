package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/config"
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
		return defaultDebugLaunchArguments
	}
	return *req.LaunchArguments
}

// launchResolutionFromArguments maps the CS2 backbuffer resolution carried by
// the debug launch arguments onto the recording profile. Both 4:3 sizes keep
// the real-pixel stretch, a 16:9 backbuffer must not be stretched. Without an
// explicit -w/-h the game falls back to its own video settings and the 4:3
// profile stays the default, because its stretch filter is a no-op for 16:9
// input.
func launchResolutionFromArguments(arguments string) string {
	width, height := renderResolutionFromArguments(arguments)
	if width <= 0 || height <= 0 {
		return config.DefaultLaunchResolution
	}
	if width*3 != height*4 {
		return config.LaunchResolution16x9
	}
	if width == 1280 && height == 960 {
		return config.LaunchResolution4x3Low
	}
	return config.LaunchResolution4x3
}

// renderResolutionFromArguments reads the -w/-h pair CS2 accepts on its command
// line. The last occurrence wins, matching how a repeated engine switch is
// resolved. Values that are not a positive integer are ignored.
func renderResolutionFromArguments(arguments string) (int, int) {
	fields := splitDebugLaunchArguments(arguments)
	width, height := 0, 0
	for index := 0; index < len(fields); index++ {
		switch {
		case strings.EqualFold(fields[index], "-w"), strings.EqualFold(fields[index], "-width"):
			if value, ok := positiveIntField(fields, index+1); ok {
				width = value
			}
		case strings.EqualFold(fields[index], "-h"), strings.EqualFold(fields[index], "-height"):
			if value, ok := positiveIntField(fields, index+1); ok {
				height = value
			}
		}
	}
	return width, height
}

// splitDebugLaunchArguments only inspects arguments; it never rewrites the
// command line passed to CS2. Keep the quote/backslash rules in sync with the
// frontend splitLaunchArguments helper.
func splitDebugLaunchArguments(arguments string) []string {
	var fields []string
	var field strings.Builder
	quoted, started := false, false
	for index := 0; index < len(arguments); index++ {
		character := arguments[index]
		if !quoted && strings.ContainsRune(" \t\r\n", rune(character)) {
			if started {
				fields = append(fields, field.String())
			}
			field.Reset()
			started = false
			continue
		}
		started = true
		switch character {
		case '\\':
			end := index
			for end < len(arguments) && arguments[end] == '\\' {
				end++
			}
			count := end - index
			if end < len(arguments) && arguments[end] == '"' {
				field.WriteString(strings.Repeat("\\", count/2))
				if count%2 == 0 {
					quoted = !quoted
				} else {
					field.WriteByte('"')
				}
				index = end
			} else {
				field.WriteString(strings.Repeat("\\", count))
				index = end - 1
			}
		case '"':
			quoted = !quoted
		default:
			field.WriteByte(character)
		}
	}
	if started {
		fields = append(fields, field.String())
	}
	return fields
}

func positiveIntField(fields []string, index int) (int, bool) {
	if index >= len(fields) {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(fields[index]))
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
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
