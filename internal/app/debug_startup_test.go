package app

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDebugLaunchArguments(t *testing.T) {
	for _, tc := range []struct {
		name, resolution string
		arguments        *string
		want             string
	}{
		{"legacy", "4:3", nil, defaultHLAEArguments + " -w 1440 -h 1080"},
		{"custom", "4:3_1280x960", stringPointer(`-insecure -console +echo "a b"`), `-insecure -console +echo "a b" -w 1280 -h 960`},
		{"empty", "16:9", stringPointer(""), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildHLAECommandLineWithArguments(tc.resolution, tc.arguments); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
	// Normalization freezes caller-owned inputs, including the command slice.
	arguments := "-console"
	commands := []string{" echo \"a;b\"; fps_max 120 ", " "}
	req := normalizeDebugLaunchRequest(DebugLaunchRequest{LaunchArguments: &arguments, StartupCommands: commands})
	arguments = "changed"
	commands[0] = "changed"
	if *req.LaunchArguments != "-console" || !reflect.DeepEqual(req.StartupCommands, []string{`echo "a;b"; fps_max 120`}) {
		t.Fatalf("not frozen: %+v", req)
	}
}

func stringPointer(value string) *string { return &value }

func TestDebugStartupValidationBeforeEnvironmentPreparation(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	for _, req := range []DebugLaunchRequest{
		{StartupCommands: []string{"echo test"}},
		{LaunchArguments: stringPointer("-console\n-foo")},
		{InjectPlugin: true, StartupCommands: []string{"echo a\x00b"}},
	} {
		if _, err := app.LaunchDebugGame(req); err == nil {
			t.Fatal("invalid launch accepted")
		}
		if app.debugRuntimeForAdmission() != nil {
			t.Fatal("invalid request installed runtime")
		}
	}
}

func waitForStartupResult(t *testing.T, app *App) *DebugCommandResponse {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if result := app.GetDebugSessionState().StartupCommandResult; result != nil {
			return result
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("startup commands never finished")
	return nil
}

func TestDebugStartupCommandsOnceAfterReady(t *testing.T) {
	commands := []string{"sv_cheats 1", `echo "a;b"; fps_max 120`}
	app, ws := startDebugPluginSession(t, true, DebugLaunchRequest{InjectPlugin: true, StartupCommands: commands})
	if result := app.GetDebugSessionState().StartupCommandResult; result != nil {
		t.Fatalf("executed before ready: %+v", result)
	}
	conn := dialDebugPlugin(t, ws.Address())
	sendDebugReady(t, conn, app.GetDebugSessionState().SessionID, DebugProtocolVersion)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var wire debugExecuteWire
	if err := conn.ReadJSON(&wire); err != nil {
		t.Fatal(err)
	}
	if wire.Name != "debug_execute" || !reflect.DeepEqual(wire.Payload.Commands, commands) {
		t.Fatalf("unexpected command: %+v", wire)
	}
	if err := conn.WriteJSON(map[string]any{"name": "debug_command_result", "payload": map[string]any{"request_id": wire.Payload.RequestID, "status": "executed"}}); err != nil {
		t.Fatal(err)
	}
	result := waitForStartupResult(t, app)
	if result.Status != "executed" {
		t.Fatalf("result: %+v", result)
	}
	_ = conn.Close()
	replacement := dialDebugPlugin(t, ws.Address())
	sendDebugReady(t, replacement, app.GetDebugSessionState().SessionID, DebugProtocolVersion)
	waitForPluginReady(t, app)
	_ = replacement.SetReadDeadline(time.Now().Add(350 * time.Millisecond))
	if err := replacement.ReadJSON(&wire); err == nil {
		t.Fatal("startup commands resent on reconnect")
	}
	if app.GetDebugSessionState().StartupCommandResult.RequestID != result.RequestID {
		t.Fatal("result replaced on reconnect")
	}
}

func TestDebugStartupCommandsReadyTimeout(t *testing.T) {
	old := debugReadyTimeout
	debugReadyTimeout = 30 * time.Millisecond
	t.Cleanup(func() { debugReadyTimeout = old })
	app, _ := startDebugPluginSession(t, true, DebugLaunchRequest{InjectPlugin: true, StartupCommands: []string{"echo test"}})
	result := waitForStartupResult(t, app)
	if result.Status != "failed" || !strings.Contains(result.Error, "超时") {
		t.Fatalf("result: %+v", result)
	}
}

func TestDebugStartupCommandsDisconnectFailsWithoutRetry(t *testing.T) {
	app, ws := startDebugPluginSession(t, true, DebugLaunchRequest{InjectPlugin: true, StartupCommands: []string{"echo test"}})
	conn := dialDebugPlugin(t, ws.Address())
	sendDebugReady(t, conn, app.GetDebugSessionState().SessionID, DebugProtocolVersion)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var wire debugExecuteWire
	if err := conn.ReadJSON(&wire); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	result := waitForStartupResult(t, app)
	if result.Status != "failed" || result.Error == "" {
		t.Fatalf("result: %+v", result)
	}
}
