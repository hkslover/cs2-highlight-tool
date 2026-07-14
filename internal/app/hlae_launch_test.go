package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/producews"
)

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
