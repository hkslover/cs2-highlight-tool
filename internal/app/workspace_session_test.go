package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/envsetup"
)

func TestWorkspaceSessionCloseCancelsTasksAndRejectsNewWork(t *testing.T) {
	root := t.TempDir()
	svc := envsetup.NewWithDataDir(t.TempDir(), root, "test")
	session := newWorkspaceSession(root, 1, svc)

	taskCtx, release, ok := session.beginTask()
	if !ok {
		t.Fatal("session task was not admitted")
	}
	taskDone := make(chan struct{})
	go func() {
		<-taskCtx.Done()
		release()
		close(taskDone)
	}()

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.close(stopCtx); err != nil {
		t.Fatalf("session.close: %v", err)
	}
	select {
	case <-taskDone:
	case <-time.After(time.Second):
		t.Fatal("session task did not observe cancellation")
	}
	if _, _, admitted := session.beginTask(); admitted {
		t.Fatal("closed session admitted a new task")
	}
}

func TestResetWorkspaceClearsProduceIndexes(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "sentinel.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newWorkspaceLifecycleTestApp(t, dataDir)
	a.produceState.takeFiles = map[string]ProduceTakeFile{
		"take": {DemoPath: "demo", TakeIndex: 1, Status: "ready"},
	}
	a.produceState.takeFileOrder = []string{"take"}
	a.produceState.historyItems = map[string]ProduceHistoryItem{
		"history": {DemoPath: "demo", TakeIndex: 1, VideoPath: "clip.mp4"},
	}
	a.produceState.historyOrder = []string{"history"}
	a.produceState.historyKeyIndex = map[string]struct{}{"history": {}}

	if err := a.ResetWorkspace(); err != nil {
		t.Fatalf("ResetWorkspace: %v", err)
	}
	a.produceStateMu.Lock()
	defer a.produceStateMu.Unlock()
	if len(a.produceState.takeFiles) != 0 || len(a.produceState.historyItems) != 0 || len(a.produceState.historyKeyIndex) != 0 {
		t.Fatalf("workspace indexes were not cleared: %+v", a.produceState)
	}
}
