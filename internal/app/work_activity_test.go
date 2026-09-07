package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBatchGenerationRejectsComposingSessionWithoutSideEffects(t *testing.T) {
	for _, launch := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "launch"}[launch], func(t *testing.T) {
			a := &App{dataDir: t.TempDir()}
			cancelled := false
			a.produceState.runtime = &produceSessionRuntime{
				done: make(chan struct{}), cancel: func() { cancelled = true },
				queueStopped: true, compositionPhase: true,
			}
			a.produceState.takeFiles = map[string]ProduceTakeFile{"old": {Status: "processing"}}
			req := GeneratePluginJSONBatchRequest{Jobs: []GeneratePluginJSONRequest{{DemoPath: "missing.dem"}}}
			var err error
			if launch {
				result, callErr := a.GeneratePluginJSONBatchAndLaunchHLAE(req)
				if callErr != nil {
					t.Fatal(callErr)
				}
				if result.LaunchStarted || result.Results == nil || len(result.Results) != 0 {
					t.Fatalf("unexpected result: %+v", result)
				}
				err = errors.New(result.LaunchError)
			} else {
				_, err = a.GeneratePluginJSONBatch(req)
			}
			if err == nil || !strings.Contains(err.Error(), "仍在合成或收尾") {
				t.Fatalf("unexpected error: %v", err)
			}
			if cancelled || a.produceState.takeFiles["old"].Status != "processing" {
				t.Fatal("active session was changed")
			}
			entries, err := os.ReadDir(a.dataDir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("rejected request wrote files: %v, %v", entries, err)
			}
		})
	}
}

func TestDirectoryClearPreservesFilesWhileInUse(t *testing.T) {
	for _, mode := range []string{"composing", "teardown_failed", "file_user", "launching"} {
		t.Run(mode, func(t *testing.T) {
			a := &App{dataDir: t.TempDir()}
			for _, dir := range []string{"outputs", "demo"} {
				writeTestFile(t, filepath.Join(a.dataDir, dir, "keep.mp4"), 10)
			}
			switch mode {
			case "composing":
				a.produceState.runtime = &produceSessionRuntime{done: make(chan struct{})}
			case "teardown_failed":
				done := make(chan struct{})
				close(done)
				a.produceState.runtime = &produceSessionRuntime{done: done, teardownErr: errors.New("locked")}
			case "file_user":
				release, err := a.beginManagedFileUse()
				if err != nil {
					t.Fatal(err)
				}
				defer release()
			case "launching":
				a.produceLaunchMu.Lock()
				defer a.produceLaunchMu.Unlock()
			}
			if !a.GetWorkActivity().StorageBusy {
				t.Fatal("storage should be busy")
			}
			if _, err := a.ClearOutputsDirectory(); err == nil {
				t.Fatal("output clear should fail")
			}
			if _, err := a.ClearDemoDirectory(); err == nil {
				t.Fatal("demo clear should fail")
			}
			for _, dir := range []string{"outputs", "demo"} {
				if _, err := os.Stat(filepath.Join(a.dataDir, dir, "keep.mp4")); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestDirectoryClearReservationExcludesNewFileUsers(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	release, err := a.beginManagedDirectoryClear()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.beginManagedFileUse(); err == nil {
		t.Fatal("file use entered during clear")
	}
	if _, err := a.ImportWanmeiMatch("invalid"); err == nil || !strings.Contains(err.Error(), "正在清理") {
		t.Fatalf("import bypassed reservation: %v", err)
	}
	if _, err := a.ImportFiveEMatch("invalid"); err == nil || !strings.Contains(err.Error(), "正在清理") {
		t.Fatalf("import bypassed reservation: %v", err)
	}
	if _, err := a.ConcatEditClips(EditConcatRequest{}); err == nil || !strings.Contains(err.Error(), "正在清理") {
		t.Fatalf("edit bypassed reservation: %v", err)
	}
	if _, err := a.ParseDemoFile("missing.dem"); err == nil || !strings.Contains(err.Error(), "正在清理") {
		t.Fatalf("parse bypassed reservation: %v", err)
	}
	release()
	if _, err := a.ClearOutputsDirectory(); err != nil {
		t.Fatal(err)
	}
	if a.GetWorkActivity().StorageBusy {
		t.Fatal("clear reservation was not released")
	}
}

func TestWorkActivityKeepsBusyUntilTeardownCompletes(t *testing.T) {
	a := &App{}
	done := make(chan struct{})
	a.produceState.runtime = &produceSessionRuntime{done: done, queueStopped: true}
	if !a.GetWorkActivity().ProduceBusy {
		t.Fatal("queue completion must not end produce busy")
	}
	close(done)
	if state := a.GetWorkActivity(); state.ProduceBusy || state.StorageBusy {
		t.Fatalf("completed session still busy: %+v", state)
	}
	// Failed teardown may be retried by launching, but files remain protected.
	a.produceState.runtime.teardownErr = errors.New("locked")
	if state := a.GetWorkActivity(); state.ProduceBusy || !state.StorageBusy {
		t.Fatalf("failed teardown state: %+v", state)
	}
}

func TestFailedTeardownRetryPreservesTakeStateBeforeGeneration(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	done := make(chan struct{})
	close(done)
	a.produceState.runtime = &produceSessionRuntime{
		done: done, cancel: func() {}, cs2PID: 12345, teardownErr: errors.New("close failed"),
	}
	a.produceState.takeFiles = map[string]ProduceTakeFile{"old": {Status: "failed"}}
	oldClose := closeCS2ProcessByPIDFn
	closeCS2ProcessByPIDFn = func(int) error { return errors.New("close still failing") }
	defer func() { closeCS2ProcessByPIDFn = oldClose }()
	result, err := a.GeneratePluginJSONBatchAndLaunchHLAE(GeneratePluginJSONBatchRequest{
		Jobs: []GeneratePluginJSONRequest{{DemoPath: "missing.dem"}},
	})
	if err != nil || result.LaunchStarted || !strings.Contains(result.LaunchError, "close still failing") {
		t.Fatalf("retry result: %+v, %v", result, err)
	}
	if a.produceState.takeFiles["old"].Status != "failed" {
		t.Fatal("old failure state was reset")
	}
	entries, err := os.ReadDir(a.dataDir)
	if err != nil || len(entries) > 0 {
		t.Fatalf("failed retry wrote files: %v, %v", entries, err)
	}
}
