package app

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/config"
)

func TestBuildHLAECommandLineFromArguments(t *testing.T) {
	// The debug workbench's arguments are the whole command line: nothing is
	// appended, so a session cannot end up with two conflicting -w/-h pairs.
	for _, tc := range []struct {
		name      string
		arguments *string
		want      string
	}{
		{"default", nil, defaultDebugLaunchArguments},
		{"custom", stringPointer(`-insecure -console +echo "a b" -w 1920 -h 1080`), `-insecure -console +echo "a b" -w 1920 -h 1080`},
		{"empty", stringPointer(""), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildHLAECommandLineFromArguments(tc.arguments); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestLaunchResolutionFromArguments(t *testing.T) {
	for _, tc := range []struct {
		name      string
		arguments string
		want      string
	}{
		{"no resolution", "-insecure -console", config.DefaultLaunchResolution},
		{"4:3 1440x1080", "-console -w 1440 -h 1080", config.LaunchResolution4x3},
		{"4:3 1280x960", "-console -w 1280 -h 960", config.LaunchResolution4x3Low},
		{"other 4:3 size", "-console -w 1024 -h 768", config.LaunchResolution4x3},
		{"16:9", "-console -w 1920 -h 1080", config.LaunchResolution16x9},
		{"last pair wins", "-w 1920 -h 1080 -w 1280 -h 960", config.LaunchResolution4x3Low},
		{"long flags", "-width 1920 -height 1080", config.LaunchResolution16x9},
		{"invalid value", "-w abc -h 960", config.DefaultLaunchResolution},
		{"missing value", "-w", config.DefaultLaunchResolution},
		{"quoted dimensions", `-w "1280" -h "960"`, config.LaunchResolution4x3Low},
		{"quoted console text", `-w 1280 -h 960 +echo "test -w 1920 -h 1080 test"`, config.LaunchResolution4x3Low},
		{"console text before dimensions", `+echo "test -w 1920 -h 1080 test" -w 1280 -h 960`, config.LaunchResolution4x3Low},
		{"empty quoted argument", `-w 1280 -h 960 +echo ""`, config.LaunchResolution4x3Low},
		{"unterminated console quote", `-w 1280 -h 960 +echo "test -w 1920 -h 1080`, config.LaunchResolution4x3Low},
		{"escaped quote in console text", `-w 1280 -h 960 +echo "test \"quoted\" -w 1920 -h 1080 test"`, config.LaunchResolution4x3Low},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := launchResolutionFromArguments(tc.arguments); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestDebugLaunchRequestDerivesResolutionFromArguments(t *testing.T) {
	// A client-provided launch_resolution is ignored: the arguments own it.
	arguments := "-console -w 1920 -h 1080"
	req := normalizeDebugLaunchRequest(DebugLaunchRequest{
		LaunchResolution: config.LaunchResolution4x3,
		LaunchArguments:  &arguments,
	})
	if req.LaunchResolution != config.LaunchResolution16x9 {
		t.Fatalf("launch resolution = %q, want 16:9 from the arguments", req.LaunchResolution)
	}

	legacy := normalizeDebugLaunchRequest(DebugLaunchRequest{})
	if *legacy.LaunchArguments != defaultDebugLaunchArguments {
		t.Fatalf("default arguments = %q, want %q", *legacy.LaunchArguments, defaultDebugLaunchArguments)
	}
	if legacy.LaunchResolution != config.LaunchResolution4x3Low {
		t.Fatalf("default resolution = %q, want the 1280x960 profile", legacy.LaunchResolution)
	}
}

func TestDebugLaunchArgumentsFreezeCallerInputs(t *testing.T) {
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
