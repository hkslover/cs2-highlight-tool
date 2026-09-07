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

	"cs2-highlight-tool-v2/internal/demo"
	"cs2-highlight-tool-v2/internal/producews"
)

func TestPIDDetectionFailureRetainsEnvironmentUntilProcessesAreGone(t *testing.T) {
	a := &App{exeDir: t.TempDir(), produceW: producews.NewDefault(nil)}
	demoPath, cs2Exe := prepareLaunchTestEnvironment(t, a.exeDir)
	gameInfo := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(cs2Exe))), "csgo", "gameinfo.gi")
	original, err := os.ReadFile(gameInfo)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.produceW.Start(); err != nil {
		t.Fatal(err)
	}
	defer a.produceW.Stop()
	oldCommand, oldList, oldClose := launchHLAECommand, listCS2PIDsFn, closeCS2ProcessByPIDFn
	t.Cleanup(func() { launchHLAECommand, listCS2PIDsFn, closeCS2ProcessByPIDFn = oldCommand, oldList, oldClose })
	launchHLAECommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=TestHelperProcessPendingHLAE", "--", "pending-hlae")
	}
	calls := 0
	listCS2PIDsFn = func() ([]int, error) {
		calls++
		if calls == 1 {
			return nil, nil
		}
		return nil, errors.New("enumeration unavailable")
	}
	closeCS2ProcessByPIDFn = func(pid int) error { t.Fatalf("must not kill unowned PID %d", pid); return nil }
	result, err := a.GeneratePluginJSONBatchAndLaunchHLAE(GeneratePluginJSONBatchRequest{
		Jobs: []GeneratePluginJSONRequest{{DemoPath: demoPath, TickRate: 64, SelectedItems: []SelectedClipItem{{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7}}}}},
	})
	if err != nil || result.LaunchStarted {
		t.Fatalf("launch: %+v, %v", result, err)
	}
	runtime := a.produceState.runtime
	if runtime == nil || runtime.pendingLaunch == nil {
		t.Fatal("unknown-PID launch ownership was discarded")
	}
	select {
	case <-runtime.pendingLaunch.done:
	default:
		t.Fatal("launcher must exit before checking child processes")
	}
	if _, err := os.Stat(gameInfo + produceGameInfoBackupSuffix); err != nil {
		t.Fatalf("backup lost: %v", err)
	}
	current, err := os.ReadFile(gameInfo)
	if err != nil || string(current) == string(original) {
		t.Fatalf("environment restored without exit proof: %v", err)
	}

	// A later successful enumeration still cannot claim an unrelated PID.
	listCS2PIDsFn = func() ([]int, error) { return []int{12345}, nil }
	if err := a.stopProduceSessionWorker(); err == nil {
		t.Fatal("restored while CS2 remained alive")
	}
	if a.produceState.runtime != runtime {
		t.Fatal("retry discarded runtime")
	}
	if _, err := os.Stat(gameInfo + produceGameInfoBackupSuffix); err != nil {
		t.Fatal(err)
	}

	listCS2PIDsFn = func() ([]int, error) { return nil, nil }
	if err := a.stopProduceSessionWorker(); err != nil {
		t.Fatalf("retry after exit: %v", err)
	}
	if a.produceState.runtime != nil {
		t.Fatal("successful recovery did not release ownership")
	}
	restored, err := os.ReadFile(gameInfo)
	if err != nil || string(restored) != string(original) {
		t.Fatalf("original environment not restored: %v", err)
	}
	if _, err := os.Stat(gameInfo + produceGameInfoBackupSuffix); !os.IsNotExist(err) {
		t.Fatalf("backup remains: %v", err)
	}
}

func TestHelperProcessPendingHLAE(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != "pending-hlae" {
		return
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

func TestLaunchHLAEGamePassesProduceWSPort(t *testing.T) {
	exeDir := t.TempDir()
	prepareLaunchTestEnvironment(t, exeDir)

	produceW := producews.NewDefault(nil)
	if err := produceW.Start(); err != nil {
		t.Fatalf("Start produce websocket: %v", err)
	}
	defer produceW.Stop()

	wantPort, err := produceW.Port()
	if err != nil {
		t.Fatalf("Port: %v", err)
	}

	envFile := filepath.Join(t.TempDir(), "hlae-env.txt")

	originalLaunchCommand := launchHLAECommand
	originalListPIDs := listCS2PIDsFn
	launchHLAECommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=TestHelperProcessLaunchHLAEEnv", "--", envFile)
	}
	listCalls := 0
	listCS2PIDsFn = func() ([]int, error) {
		listCalls++
		if listCalls == 1 {
			return []int{1000}, nil
		}
		return []int{1000, 1001}, nil
	}
	t.Cleanup(func() {
		launchHLAECommand = originalLaunchCommand
		listCS2PIDsFn = originalListPIDs
	})

	app := &App{
		exeDir:   exeDir,
		produceW: produceW,
	}
	if _, err := app.launchHLAEGame(); err != nil {
		t.Fatalf("launchHLAEGame: %v", err)
	}

	var gotBytes []byte
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		gotBytes, err = os.ReadFile(envFile)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("read helper environment: %v", err)
	}
	parts := strings.Split(strings.TrimSpace(string(gotBytes)), "\n")
	if len(parts) != 2 {
		t.Fatalf("helper environment format = %q", gotBytes)
	}
	if got, want := parts[0], strconv.Itoa(wantPort); got != want {
		t.Fatalf("CSDM_WS_PORT = %q, want %q", got, want)
	}
	if got, want := parts[1], filepath.Join(exeDir, "logs", "cs2-server-plugin.log"); got != want {
		t.Fatalf("CSDM_LOG_PATH = %q, want %q", got, want)
	}
}

func TestHelperProcessLaunchHLAEEnv(t *testing.T) {
	if len(os.Args) < 3 || !strings.Contains(strings.Join(os.Args, " "), "-test.run=TestHelperProcessLaunchHLAEEnv") {
		return
	}
	path := os.Args[len(os.Args)-1]
	content := os.Getenv("CSDM_WS_PORT") + "\n" + os.Getenv("CSDM_LOG_PATH")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write helper environment: %v", err)
	}
	os.Exit(0)
}
