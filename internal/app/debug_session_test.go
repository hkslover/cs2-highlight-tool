package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/producegame"
	"cs2-highlight-tool-v2/internal/producews"
)

// overrideDebugSeams installs deterministic process/launch seams for one test
// and restores every package-level override afterwards.
func overrideDebugSeams(t *testing.T) {
	t.Helper()
	oldCommand, oldList, oldClose := launchHLAECommand, listCS2PIDsFn, closeCS2ProcessByPIDFn
	oldPoll, oldWait := debugProcessPollInterval, debugSessionStopWait
	debugProcessPollInterval = time.Hour
	debugSessionStopWait = 5 * time.Second
	t.Cleanup(func() {
		launchHLAECommand, listCS2PIDsFn, closeCS2ProcessByPIDFn = oldCommand, oldList, oldClose
		debugProcessPollInterval, debugSessionStopWait = oldPoll, oldWait
	})
}

func debugGameInfoPath(cs2Exe string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(cs2Exe))), "csgo", "gameinfo.gi")
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func argValue(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func envHasPrefix(env []string, prefix string) bool {
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

// TestLaunchDebugGameNoPluginMode verifies the no-plugin contract: HLAE still
// loads AfxHookSource2.dll, no produce websocket is required and no plugin
// transport environment is exported.
func TestLaunchDebugGameNoPluginMode(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	_, cs2Exe := prepareLaunchTestEnvironment(t, app.exeDir)
	overrideDebugSeams(t)

	var captured *exec.Cmd
	var capturedArgs []string
	launchHLAECommand = func(_ string, args ...string) *exec.Cmd {
		capturedArgs = append([]string(nil), args...)
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessLaunchHLAE", "--")
		captured = cmd
		return cmd
	}
	listCalls := 0
	listCS2PIDsFn = func() ([]int, error) {
		listCalls++
		if listCalls == 1 {
			return nil, nil
		}
		return []int{4242}, nil
	}
	closeCS2ProcessByPIDFn = func(pid int) error { return nil }

	state, err := app.LaunchDebugGame(DebugLaunchRequest{
		InjectPlugin:     false,
		BrandEnabled:     false,
		PovHudEnabled:    false,
		LaunchResolution: config.LaunchResolution4x3,
		LaunchArguments:  stringPointer(`-insecure -console +echo "custom args"`),
	})
	if err != nil {
		t.Fatalf("LaunchDebugGame: %v", err)
	}
	if state.Status != DebugStatusRunning || state.CS2PID != 4242 || !state.ProcessStarted {
		t.Fatalf("unexpected state: %+v", state)
	}
	if state.PluginEnabled || state.PluginConnected || state.PluginReady {
		t.Fatalf("no-plugin session reported a plugin transport: %+v", state)
	}
	if state.PluginMessage != "未启用插件" {
		t.Fatalf("plugin message = %q", state.PluginMessage)
	}
	if state.PluginState != DebugPluginStateDisabled {
		t.Fatalf("plugin state = %q, want disabled for a no-plugin session", state.PluginState)
	}
	if captured == nil {
		t.Fatal("HLAE launcher was not invoked")
	}
	if got := argValue(capturedArgs, "-cmdLine"); got != `-insecure -console +echo "custom args" -w 1440 -h 1080` {
		t.Fatalf("custom CS2 arguments not forwarded as one value: %q", got)
	}
	if !hasArg(capturedArgs, "-hookDllPath") {
		t.Fatalf("HLAE hook DLL was not passed: %v", capturedArgs)
	}
	for _, prefix := range []string{"CSDM_WS_PORT=", "CSDM_LOG_PATH=", "CSDM_DEBUG_SESSION="} {
		if envHasPrefix(captured.Env, prefix) {
			t.Fatalf("no-plugin launch exported %s", prefix)
		}
	}
	content := readFileString(t, debugGameInfoPath(cs2Exe))
	for _, path := range knownInjectedSearchPaths() {
		if producegame.HasSearchPath(content, path) {
			t.Fatalf("no-plugin launch left %s in gameinfo", path)
		}
	}
	if _, err := os.Stat(debugGameInfoPath(cs2Exe) + produceGameInfoBackupSuffix); !os.IsNotExist(err) {
		t.Fatalf("unexpected gameinfo backup: %v", err)
	}

	if _, err := app.StopDebugGame(); err != nil {
		t.Fatalf("StopDebugGame: %v", err)
	}
	if app.debugRuntimeForAdmission() != nil {
		t.Fatal("stopped session kept its runtime")
	}
}

// TestLaunchDebugGameRemovesResidualPluginPathAndRestores verifies that a
// no-plugin session temporarily removes a leftover plugin search path inside
// its own gameinfo backup and restores the exact original on exit.
func TestLaunchDebugGameRemovesResidualPluginPathAndRestores(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	_, cs2Exe := prepareLaunchTestEnvironment(t, app.exeDir)
	overrideDebugSeams(t)

	gameInfo := debugGameInfoPath(cs2Exe)
	original := readFileString(t, gameInfo)
	injected, ok := producegame.InjectSearchPath(original, producegame.SearchPathPlugin)
	if !ok {
		t.Fatal("failed to seed a residual plugin search path")
	}
	if err := os.WriteFile(gameInfo, []byte(injected), 0644); err != nil {
		t.Fatal(err)
	}

	launchHLAECommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=TestHelperProcessLaunchHLAE", "--")
	}
	listCalls := 0
	listCS2PIDsFn = func() ([]int, error) {
		listCalls++
		if listCalls == 1 {
			return nil, nil
		}
		return []int{5150}, nil
	}
	closeCS2ProcessByPIDFn = func(pid int) error { return nil }

	if _, err := app.LaunchDebugGame(DebugLaunchRequest{InjectPlugin: false, BrandEnabled: false, PovHudEnabled: false}); err != nil {
		t.Fatalf("LaunchDebugGame: %v", err)
	}
	if producegame.HasSearchPath(readFileString(t, gameInfo), producegame.SearchPathPlugin) {
		t.Fatal("residual plugin path was not removed for the no-plugin session")
	}
	if _, err := os.Stat(gameInfo + produceGameInfoBackupSuffix); err != nil {
		t.Fatalf("expected a gameinfo backup: %v", err)
	}

	if _, err := app.StopDebugGame(); err != nil {
		t.Fatalf("StopDebugGame: %v", err)
	}
	if got := readFileString(t, gameInfo); got != injected {
		t.Fatalf("gameinfo was not restored to the pre-session content:\n%s", got)
	}
	if _, err := os.Stat(gameInfo + produceGameInfoBackupSuffix); !os.IsNotExist(err) {
		t.Fatalf("backup was not cleaned: %v", err)
	}
}

// TestLaunchDebugGamePluginModeCarriesTransportAndSession verifies the plugin
// mode launch contract: fixed loopback port, plugin log path and the debug
// session marker, plus the plugin DLL/search path mounting.
func TestLaunchDebugGamePluginModeCarriesTransportAndSession(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	_, cs2Exe := prepareLaunchTestEnvironment(t, app.exeDir)
	overrideDebugSeams(t)

	produceW := producews.NewDefault(nil)
	if err := produceW.Start(); err != nil {
		t.Fatalf("Start produce websocket: %v", err)
	}
	defer produceW.Stop()
	app.produceW = produceW
	wantPort, err := produceW.Port()
	if err != nil {
		t.Fatalf("Port: %v", err)
	}

	var captured *exec.Cmd
	var capturedArgs []string
	launchHLAECommand = func(_ string, args ...string) *exec.Cmd {
		capturedArgs = append([]string(nil), args...)
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessLaunchHLAE", "--")
		captured = cmd
		return cmd
	}
	listCalls := 0
	listCS2PIDsFn = func() ([]int, error) {
		listCalls++
		if listCalls == 1 {
			return nil, nil
		}
		return []int{6161}, nil
	}
	closeCS2ProcessByPIDFn = func(pid int) error { return nil }

	state, err := app.LaunchDebugGame(DebugLaunchRequest{
		InjectPlugin:     true,
		BrandEnabled:     true,
		PovHudEnabled:    false,
		LaunchResolution: config.LaunchResolution4x3Low,
	})
	if err != nil {
		t.Fatalf("LaunchDebugGame: %v", err)
	}
	if !state.PluginEnabled {
		t.Fatal("plugin mode did not report the plugin as enabled")
	}
	if state.PluginReady {
		t.Fatal("step 02 must not report the plugin debug protocol as ready")
	}
	if captured == nil {
		t.Fatal("HLAE launcher was not invoked")
	}
	if got := envValue(captured.Env, "CSDM_WS_PORT"); got != strconv.Itoa(wantPort) {
		t.Fatalf("CSDM_WS_PORT = %q, want %d", got, wantPort)
	}
	if envValue(captured.Env, "CSDM_LOG_PATH") == "" {
		t.Fatal("CSDM_LOG_PATH was not exported")
	}
	if got := envValue(captured.Env, "CSDM_DEBUG_SESSION"); got != state.SessionID {
		t.Fatalf("CSDM_DEBUG_SESSION = %q, want %q", got, state.SessionID)
	}
	if cmdLine := argValue(capturedArgs, "-cmdLine"); !strings.Contains(cmdLine, "-w 1280 -h 960") {
		t.Fatalf("resolution args missing from cmdLine %q", cmdLine)
	}
	content := readFileString(t, debugGameInfoPath(cs2Exe))
	for _, path := range []string{producegame.SearchPathPlugin, producegame.SearchPathBrand} {
		if !producegame.HasSearchPath(content, path) {
			t.Fatalf("plugin mode did not mount %s", path)
		}
	}
	if producegame.HasSearchPath(content, producegame.SearchPathPOV) {
		t.Fatal("plugin mode mounted POV although it was disabled")
	}

	if _, err := app.StopDebugGame(); err != nil {
		t.Fatalf("StopDebugGame: %v", err)
	}
}

// TestDebugSessionBlocksProduceAndWorkspaceTransition verifies that a live
// debug session reuses the backend admission gate instead of relying on a
// disabled frontend button.
func TestDebugSessionBlocksProduceAndWorkspaceTransition(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	_, _ = prepareLaunchTestEnvironment(t, app.exeDir)
	overrideDebugSeams(t)

	launchHLAECommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=TestHelperProcessLaunchHLAE", "--")
	}
	listCalls := 0
	listCS2PIDsFn = func() ([]int, error) {
		listCalls++
		if listCalls == 1 {
			return nil, nil
		}
		return []int{7001}, nil
	}
	closeCS2ProcessByPIDFn = func(pid int) error { return nil }

	if _, err := app.LaunchDebugGame(DebugLaunchRequest{InjectPlugin: false, BrandEnabled: false}); err != nil {
		t.Fatalf("LaunchDebugGame: %v", err)
	}
	if _, err := app.LaunchDebugGame(DebugLaunchRequest{InjectPlugin: false}); err == nil {
		t.Fatal("a second debug launch was admitted while a session was running")
	}
	if _, err := app.beginManagedExclusion(true); err == nil {
		t.Fatal("directory clearing was admitted while a debug session was running")
	}
	result, err := app.GeneratePluginJSONBatchAndLaunchHLAE(GeneratePluginJSONBatchRequest{
		Jobs: []GeneratePluginJSONRequest{{DemoPath: writeDemoFile(t), TickRate: 64}},
	})
	if err != nil {
		t.Fatalf("produce launch returned a hard error: %v", err)
	}
	if result.LaunchStarted || result.LaunchError == "" {
		t.Fatalf("produce launch was admitted while a debug session was running: %+v", result)
	}

	if _, err := app.StopDebugGame(); err != nil {
		t.Fatalf("StopDebugGame: %v", err)
	}
	if err := app.debugBusyError(); err != nil {
		t.Fatalf("debug admission was not released: %v", err)
	}
}

// TestDebugUnknownPIDRetainsEnvironmentUntilExit verifies that a launch whose
// PID cannot be confirmed keeps the environment backups and the launcher handle
// until a later stop can prove all CS2 processes are gone.
func TestDebugUnknownPIDRetainsEnvironmentUntilExit(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	_, cs2Exe := prepareLaunchTestEnvironment(t, app.exeDir)
	overrideDebugSeams(t)

	gameInfo := debugGameInfoPath(cs2Exe)
	original := readFileString(t, gameInfo)

	launchHLAECommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=TestHelperProcessPendingHLAE", "--", "pending-hlae")
	}
	listCalls := 0
	listCS2PIDsFn = func() ([]int, error) {
		listCalls++
		switch listCalls {
		case 1:
			return nil, nil
		case 2, 3:
			return nil, errors.New("enumeration unavailable")
		default:
			return nil, nil
		}
	}
	closeCS2ProcessByPIDFn = func(pid int) error { t.Fatalf("must not kill unowned PID %d", pid); return nil }

	_, err := app.LaunchDebugGame(DebugLaunchRequest{InjectPlugin: false, BrandEnabled: true, PovHudEnabled: false})
	if err == nil {
		t.Fatal("launch with an unconfirmable PID reported success")
	}
	rt := app.debugRuntimeForAdmission()
	if rt == nil {
		t.Fatal("unknown-PID ownership was discarded")
	}
	rt.mu.Lock()
	pending := rt.pendingLaunch
	rt.mu.Unlock()
	if pending == nil {
		t.Fatal("launcher handle was not retained")
	}
	if !app.debugRetainedForActivity() {
		t.Fatal("retained environment is not reported as busy")
	}
	if _, statErr := os.Stat(gameInfo + produceGameInfoBackupSuffix); statErr != nil {
		t.Fatalf("gameinfo backup was lost: %v", statErr)
	}
	if got := readFileString(t, gameInfo); got == original {
		t.Fatal("environment was restored without exit proof")
	}

	if _, err := app.StopDebugGame(); err != nil {
		t.Fatalf("retry stop after exit: %v", err)
	}
	if app.debugRuntimeForAdmission() != nil {
		t.Fatal("successful retry did not release the runtime")
	}
	if got := readFileString(t, gameInfo); got != original {
		t.Fatal("original environment was not restored after exit")
	}
	if _, err := os.Stat(gameInfo + produceGameInfoBackupSuffix); !os.IsNotExist(err) {
		t.Fatalf("backup remains after successful restore: %v", err)
	}
}

// TestPrepareGameInfoForProduceKeepsPluginAndBrand pins the formal produce
// contract after the debug parameterization: plugin and brand stay forced.
func TestPrepareGameInfoForProduceKeepsPluginAndBrand(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	_, cs2Exe := prepareLaunchTestEnvironment(t, app.exeDir)

	if err := app.prepareGameInfoForProduce(); err != nil {
		t.Fatalf("prepareGameInfoForProduce: %v", err)
	}
	content := readFileString(t, debugGameInfoPath(cs2Exe))
	for _, path := range []string{producegame.SearchPathPlugin, producegame.SearchPathBrand} {
		if !producegame.HasSearchPath(content, path) {
			t.Fatalf("formal produce did not mount %s", path)
		}
	}
}

// TestStoppingDebugSessionKeepsSnapshotVersionMonotonic covers the release of
// the session runtime: the final idle snapshot must carry a version newer than
// the last running/stopping snapshot. Without the floor stamp the idle snapshot
// could only report the recording-list version, the frontend merge rules would
// reject it, and the page would keep showing the closed session and its PID.
func TestStoppingDebugSessionKeepsSnapshotVersionMonotonic(t *testing.T) {
	app, _ := startDebugPluginSession(t, true)
	running := app.GetDebugSessionState()
	if running.Status != DebugStatusRunning || running.UpdatedAtMs <= 0 {
		t.Fatalf("unexpected running snapshot: %+v", running)
	}

	stopped, err := app.StopDebugGame()
	if err != nil {
		t.Fatalf("StopDebugGame: %v", err)
	}
	if stopped.UpdatedAtMs <= running.UpdatedAtMs {
		t.Fatalf("idle snapshot version did not advance: %d -> %d", running.UpdatedAtMs, stopped.UpdatedAtMs)
	}
	if stopped.Status != DebugStatusIdle || stopped.SessionID != "" || stopped.ProcessStarted || stopped.CS2PID != 0 {
		t.Fatalf("idle snapshot still describes the closed session: %+v", stopped)
	}

	// The returned value and the next read agree, so the frontend can apply the
	// final idle event and clear the stale session facts.
	asserted := app.GetDebugSessionState()
	if asserted.UpdatedAtMs != stopped.UpdatedAtMs {
		t.Fatalf("idle snapshot version drifted: %d -> %d", stopped.UpdatedAtMs, asserted.UpdatedAtMs)
	}
	if asserted.Status != DebugStatusIdle || asserted.SessionID != "" || asserted.CS2PID != 0 {
		t.Fatalf("read-back snapshot still describes the closed session: %+v", asserted)
	}
}

func TestDelayedDebugSnapshotAfterStopUsesIdleState(t *testing.T) {
	app, _ := startDebugPluginSession(t, true)
	// A reader can be paused after capturing the runtime, before projecting it.
	captured := app.debugRuntimeForAdmission()
	stopped, err := app.StopDebugGame()
	if err != nil {
		t.Fatalf("StopDebugGame: %v", err)
	}
	delayed := app.debugSessionSnapshot(captured)
	if delayed.SessionID != "" || delayed.Status != DebugStatusIdle || delayed.ProcessStarted || delayed.CS2PID != 0 {
		t.Fatalf("delayed read restored the stopped session: %+v", delayed)
	}
	if delayed.UpdatedAtMs != stopped.UpdatedAtMs {
		t.Fatalf("unchanged idle state has different versions: %d -> %d", stopped.UpdatedAtMs, delayed.UpdatedAtMs)
	}
}

func TestDelayedDebugSnapshotAfterReplacementUsesCurrentSession(t *testing.T) {
	app := newTestAppInTempDir(t)
	old := &debugSessionRuntime{
		sessionID: "old-session", status: DebugStatusRunning,
		processStarted: true, cs2PID: 9002,
	}
	app.installDebugRuntime(old)
	before := app.GetDebugSessionState()
	captured := app.debugRuntimeForAdmission()
	app.clearDebugRuntime(old)
	replacement := &debugSessionRuntime{
		sessionID: "new-session", status: DebugStatusStarting,
	}
	app.installDebugRuntime(replacement)

	for _, delayedRuntime := range []*debugSessionRuntime{captured, nil} {
		delayed := app.debugSessionSnapshot(delayedRuntime)
		if delayed.SessionID != replacement.sessionID || delayed.Status != DebugStatusStarting || delayed.ProcessStarted || delayed.CS2PID != 0 {
			t.Fatalf("delayed read described an earlier runtime: %+v", delayed)
		}
		if delayed.UpdatedAtMs <= before.UpdatedAtMs {
			t.Fatalf("replacement did not advance the version: %d -> %d", before.UpdatedAtMs, delayed.UpdatedAtMs)
		}
	}
}
