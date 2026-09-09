package app

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cs2-highlight-tool-v2/internal/envsetup"
	"cs2-highlight-tool-v2/internal/producews"
)

func newWorkspaceLifecycleTestApp(t *testing.T, dataDir string) *App {
	t.Helper()
	exeDir := t.TempDir()
	return &App{
		exeDir:   exeDir,
		dataDir:  dataDir,
		service:  envsetup.NewWithDataDir(exeDir, dataDir, "test"),
		produceW: producews.NewDefault(nil),
	}
}

func TestResetWorkspaceRejectsManagedFileUseAndPreservesWorkspace(t *testing.T) {
	dataDir := t.TempDir()
	sentinel := filepath.Join(dataDir, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newWorkspaceLifecycleTestApp(t, dataDir)
	originalService := a.service

	release, err := a.beginManagedFileUse()
	if err != nil {
		t.Fatal(err)
	}
	resetErr := a.ResetWorkspace()
	if resetErr == nil || !strings.Contains(resetErr.Error(), "正在使用文件") {
		t.Fatalf("ResetWorkspace error = %v, want managed-use rejection", resetErr)
	}
	if got, readErr := os.ReadFile(sentinel); readErr != nil || string(got) != "keep me" {
		t.Fatalf("sentinel changed after rejected reset: %q, %v", got, readErr)
	}
	if a.dataDir != dataDir || a.service != originalService {
		t.Fatalf("workspace changed after rejected reset: dataDir=%q service_same=%v", a.dataDir, a.service == originalService)
	}

	release()
	if err := a.ResetWorkspace(); err != nil {
		t.Fatalf("ResetWorkspace after release: %v", err)
	}
	if _, err := os.Stat(dataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dataDir still exists after successful reset: %v", err)
	}
	if state := a.GetWorkspaceState(); state.Initialized || state.DataDir != "" {
		t.Fatalf("workspace state after reset: %+v", state)
	}
	if runtime.GOOS != "windows" {
		if _, err := a.beginManagedFileUse(); err == nil || !strings.Contains(err.Error(), "工作目录") {
			t.Fatalf("reset workspace unexpectedly fell back to exeDir: %v", err)
		}
	}
}

func TestSetWorkspaceDirRejectsInitializedAppBeforePathSideEffects(t *testing.T) {
	dataDir := t.TempDir()
	a := newWorkspaceLifecycleTestApp(t, dataDir)
	originalService := a.service
	target := filepath.Join(t.TempDir(), "new-workspace")

	err := a.SetWorkspaceDir(target)
	if err == nil || !strings.Contains(err.Error(), "已初始化") {
		t.Fatalf("SetWorkspaceDir error = %v, want initialized rejection", err)
	}
	if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rejected SetWorkspaceDir touched target: %v", statErr)
	}
	if a.dataDir != dataDir || a.service != originalService {
		t.Fatalf("workspace changed after rejected set: dataDir=%q service_same=%v", a.dataDir, a.service == originalService)
	}
}

func TestResetWorkspaceRejectsRegisteredBackgroundTask(t *testing.T) {
	dataDir := t.TempDir()
	a := newWorkspaceLifecycleTestApp(t, dataDir)

	// This is the same reservation used by SetWorkspaceDir before it launches
	// RunStartupChecks. It models a blocked startup task without network or CS2.
	releaseTask := a.reserveManagedWorkspaceTask()

	err := a.ResetWorkspace()
	if err == nil || !strings.Contains(err.Error(), "正在使用文件") {
		t.Fatalf("ResetWorkspace error = %v, want registered-task rejection", err)
	}
	if _, statErr := os.Stat(dataDir); statErr != nil {
		t.Fatalf("dataDir changed while task was registered: %v", statErr)
	}
	releaseTask()
	if err := a.ResetWorkspace(); err != nil {
		t.Fatalf("ResetWorkspace after background task exits: %v", err)
	}
	if _, statErr := os.Stat(dataDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("dataDir still exists after task release: %v", statErr)
	}
}

func TestResetWorkspaceDetachesServiceWhenRemovalFailsAndCanRetry(t *testing.T) {
	dataDir := t.TempDir()
	sentinel := filepath.Join(dataDir, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("preserve for retry"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newWorkspaceLifecycleTestApp(t, dataDir)
	originalRemove := removeWorkspaceDir
	removeWorkspaceDir = func(string) error { return errors.New("remove blocked") }
	t.Cleanup(func() { removeWorkspaceDir = originalRemove })

	if err := a.ResetWorkspace(); err == nil || !strings.Contains(err.Error(), "删除工作目录失败") {
		t.Fatalf("first reset error = %v, want removal failure", err)
	}
	if a.service != nil || a.dataDir != "" || a.workspaceResetPendingPath != dataDir {
		t.Fatalf("failed reset left old workspace operational: service=%v dataDir=%q pending=%q", a.service, a.dataDir, a.workspaceResetPendingPath)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel missing after injected failure: %v", err)
	}

	removeWorkspaceDir = os.RemoveAll
	if err := a.ResetWorkspace(); err != nil {
		t.Fatalf("retry reset: %v", err)
	}
	if _, err := os.Stat(dataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dataDir still exists after retry: %v", err)
	}
}

func TestResetWorkspaceRejectsFailedProduceTeardown(t *testing.T) {
	dataDir := t.TempDir()
	sentinel := filepath.Join(dataDir, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep during teardown retry"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newWorkspaceLifecycleTestApp(t, dataDir)
	done := make(chan struct{})
	close(done)
	a.produceState.runtime = &produceSessionRuntime{
		done:        done,
		teardownErr: errors.New("teardown still owns backups"),
	}

	err := a.ResetWorkspace()
	if err == nil || !strings.Contains(err.Error(), "收尾尚未成功") {
		t.Fatalf("ResetWorkspace error = %v, want failed-teardown rejection", err)
	}
	if _, statErr := os.Stat(sentinel); statErr != nil {
		t.Fatalf("sentinel missing after failed-teardown rejection: %v", statErr)
	}
}

func TestManagedWorkspaceUseRequiresSelectedDirectoryOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only selected-directory behavior")
	}
	a := &App{exeDir: t.TempDir()}
	if _, err := a.beginManagedFileUse(); err == nil || !strings.Contains(err.Error(), "工作目录") {
		t.Fatalf("beginManagedFileUse error = %v, want uninitialized workspace error", err)
	}
}
