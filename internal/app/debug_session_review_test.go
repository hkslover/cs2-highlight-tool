package app

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/producews"
)

// launchDebugForReview starts a no-plugin debug session with branding enabled
// so the gameinfo backup and brand VPK are owned by the session. It returns
// the gameinfo path and the pre-launch content.
func launchDebugForReview(t *testing.T, app *App) (string, string) {
	t.Helper()
	_, cs2Exe := prepareLaunchTestEnvironment(t, app.exeDir)
	overrideDebugSeams(t)

	gameInfo := debugGameInfoPath(cs2Exe)
	original := readFileString(t, gameInfo)

	launchHLAECommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=TestHelperProcessLaunchHLAE", "--")
	}
	listCalls := 0
	listCS2PIDsFn = func() ([]int, error) {
		listCalls++
		if listCalls == 1 {
			return nil, nil
		}
		return []int{9191}, nil
	}
	if _, err := app.LaunchDebugGame(DebugLaunchRequest{
		InjectPlugin:     false,
		BrandEnabled:     true,
		PovHudEnabled:    false,
		LaunchResolution: config.LaunchResolution4x3,
	}); err != nil {
		t.Fatalf("LaunchDebugGame: %v", err)
	}
	return gameInfo, original
}

// TestShutdownKeepsDebugEnvironmentWhenStopFails pins the P1 shutdown fix: a
// failed debug stop retains the environment and the shutdown safety restore
// must not run the shared epoch-0 restore behind it.
func TestShutdownKeepsDebugEnvironmentWhenStopFails(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	gameInfo, original := launchDebugForReview(t, app)
	modified := readFileString(t, gameInfo)
	if modified == original {
		t.Fatal("debug launch did not modify gameinfo; the test cannot observe retention")
	}

	closeCS2ProcessByPIDFn = func(pid int) error { return errors.New("close failed") }
	stopErr := app.stopDebugSessionWorker()
	if stopErr == nil {
		t.Fatal("expected the debug stop to fail")
	}
	if app.debugRuntimeForAdmission() == nil {
		t.Fatal("failed teardown released the runtime")
	}
	if _, err := os.Stat(gameInfo + produceGameInfoBackupSuffix); err != nil {
		t.Fatalf("gameinfo backup was lost: %v", err)
	}

	if err := app.restoreSharedEnvironmentAfterShutdown(stopErr, nil); err != nil {
		t.Fatalf("shutdown restore helper returned an error: %v", err)
	}
	if got := readFileString(t, gameInfo); got != modified {
		t.Fatal("shutdown restored the environment although the debug process was not confirmed exited")
	}
	if _, err := os.Stat(gameInfo + produceGameInfoBackupSuffix); err != nil {
		t.Fatalf("shutdown consumed the gameinfo backup: %v", err)
	}

	// A later successful retry must release the runtime and restore the files.
	closeCS2ProcessByPIDFn = func(pid int) error { return nil }
	if _, err := app.StopDebugGame(); err != nil {
		t.Fatalf("retry stop: %v", err)
	}
	if app.debugRuntimeForAdmission() != nil {
		t.Fatal("successful retry did not release the runtime")
	}
	if got := readFileString(t, gameInfo); got != original {
		t.Fatalf("original gameinfo was not restored:\n%s", got)
	}
}

// TestRetainedDebugSessionBlocksProduceLaunch pins the P1 cross-mode fix: the
// produce pipeline must retry the retained debug teardown and abort when it
// cannot confirm the old game process exited.
func TestRetainedDebugSessionBlocksProduceLaunch(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	gameInfo, _ := launchDebugForReview(t, app)
	modified := readFileString(t, gameInfo)

	closeCS2ProcessByPIDFn = func(pid int) error { return errors.New("close failed") }
	if err := app.stopDebugSessionWorker(); err == nil {
		t.Fatal("expected the debug stop to fail")
	}

	launched := false
	launchHLAECommand = func(_ string, _ ...string) *exec.Cmd {
		launched = true
		return exec.Command(os.Args[0], "-test.run=TestHelperProcessLaunchHLAE", "--")
	}
	result, err := app.GeneratePluginJSONBatchAndLaunchHLAE(GeneratePluginJSONBatchRequest{
		Jobs: []GeneratePluginJSONRequest{{DemoPath: writeDemoFile(t), TickRate: 64}},
	})
	if err != nil {
		t.Fatalf("produce launch returned a hard error: %v", err)
	}
	if result.LaunchStarted || result.LaunchError == "" {
		t.Fatalf("produce launch was admitted while a failed debug teardown retained the environment: %+v", result)
	}
	if launched {
		t.Fatal("produce pipeline started HLAE although the retained debug session was not released")
	}
	if got := readFileString(t, gameInfo); got != modified {
		t.Fatal("produce pipeline overwrote the retained debug environment")
	}
	if _, err := os.Stat(gameInfo + produceGameInfoBackupSuffix); err != nil {
		t.Fatalf("produce pipeline consumed the debug backup: %v", err)
	}

	closeCS2ProcessByPIDFn = func(pid int) error { return nil }
	if err := app.stopDebugSessionWorker(); err != nil {
		t.Fatalf("cleanup stop: %v", err)
	}
}

// TestRetainedProduceSessionBlocksDebugLaunch pins the other direction: the
// debug launch must retry the retained produce teardown and abort before
// advancing the environment epoch or starting HLAE.
func TestRetainedProduceSessionBlocksDebugLaunch(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	prepareLaunchTestEnvironment(t, app.exeDir)
	overrideDebugSeams(t)

	closeCS2ProcessByPIDFn = func(pid int) error { return errors.New("close failed") }
	envEpoch := app.beginProduceEnvironmentPrep()
	if err := app.rollbackProduceLaunchEnvironment(4242, envEpoch); err == nil {
		t.Fatal("expected the produce rollback to fail")
	}
	rt := app.produceState.runtime
	if rt == nil || rt.teardownErr == nil {
		t.Fatal("produce rollback did not retain the runtime")
	}

	launched := false
	launchHLAECommand = func(_ string, _ ...string) *exec.Cmd {
		launched = true
		return exec.Command(os.Args[0], "-test.run=TestHelperProcessLaunchHLAE", "--")
	}
	if _, err := app.LaunchDebugGame(DebugLaunchRequest{InjectPlugin: false}); err == nil {
		t.Fatal("debug launch was admitted while a failed produce teardown retained the environment")
	}
	if launched {
		t.Fatal("debug launch started HLAE although the retained produce session was not released")
	}
	if app.produceState.runtime != rt {
		t.Fatal("rejected debug launch discarded the retained produce runtime")
	}

	closeCS2ProcessByPIDFn = func(pid int) error { return nil }
	if err := app.stopProduceSessionWorker(); err != nil {
		t.Fatalf("cleanup produce stop: %v", err)
	}
	if app.produceState.runtime != nil {
		t.Fatal("cleanup did not release the produce runtime")
	}
}

// TestRefreshDebugPluginStateUpdatesSnapshot pins the transport-refresh
// contract: a websocket connection change re-emits the debug snapshot with a
// newer ordering timestamp for plugin sessions and is a no-op without a plugin.
func TestRefreshDebugPluginStateUpdatesSnapshot(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	_, _ = prepareLaunchTestEnvironment(t, app.exeDir)
	overrideDebugSeams(t)

	produceW := producews.NewDefault(nil)
	if err := produceW.Start(); err != nil {
		t.Fatalf("Start produce websocket: %v", err)
	}
	defer produceW.Stop()
	app.produceW = produceW

	launchHLAECommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=TestHelperProcessLaunchHLAE", "--")
	}
	listCalls := 0
	listCS2PIDsFn = func() ([]int, error) {
		listCalls++
		if listCalls == 1 {
			return nil, nil
		}
		// Each launch must observe a PID that was not present before it.
		return []int{8000 + listCalls}, nil
	}
	closeCS2ProcessByPIDFn = func(pid int) error { return nil }

	if _, err := app.LaunchDebugGame(DebugLaunchRequest{
		InjectPlugin:     true,
		BrandEnabled:     true,
		LaunchResolution: config.LaunchResolution4x3,
	}); err != nil {
		t.Fatalf("LaunchDebugGame: %v", err)
	}

	before := app.GetDebugSessionState().UpdatedAtMs
	time.Sleep(2 * time.Millisecond)
	app.refreshDebugPluginState()
	if after := app.GetDebugSessionState().UpdatedAtMs; after <= before {
		t.Fatalf("plugin refresh did not advance the snapshot timestamp: %d -> %d", before, after)
	}

	if _, err := app.StopDebugGame(); err != nil {
		t.Fatalf("StopDebugGame: %v", err)
	}

	// A no-plugin session has no transport to refresh.
	if _, err := app.LaunchDebugGame(DebugLaunchRequest{InjectPlugin: false, BrandEnabled: true}); err != nil {
		t.Fatalf("LaunchDebugGame (no plugin): %v", err)
	}
	noPluginBefore := app.GetDebugSessionState().UpdatedAtMs
	time.Sleep(2 * time.Millisecond)
	app.refreshDebugPluginState()
	if after := app.GetDebugSessionState().UpdatedAtMs; after != noPluginBefore {
		t.Fatalf("no-plugin refresh changed the snapshot timestamp: %d -> %d", noPluginBefore, after)
	}
	if _, err := app.StopDebugGame(); err != nil {
		t.Fatalf("StopDebugGame (no plugin): %v", err)
	}
}

// TestDebugRetainedSessionBlocksDirectoryClear verifies the explicit retained
// debug guard in the directory-clearing admission path.
func TestDebugRetainedSessionBlocksDirectoryClear(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	launchDebugForReview(t, app)
	if err := app.stopDebugSessionWorker(); err == nil {
		t.Fatal("expected the debug stop to fail")
	}

	if _, err := app.beginManagedExclusion(true); err == nil {
		t.Fatal("directory clearing was admitted while a failed debug teardown retained the environment")
	}

	closeCS2ProcessByPIDFn = func(pid int) error { return nil }
	if err := app.stopDebugSessionWorker(); err != nil {
		t.Fatalf("cleanup stop: %v", err)
	}
	release, err := app.beginManagedExclusion(true)
	if err != nil {
		t.Fatalf("directory clearing stayed blocked after release: %v", err)
	}
	release()
}
