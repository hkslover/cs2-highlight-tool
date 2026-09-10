package envsetup

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceStopKeepsClosedBoundaryAfterTimeout(t *testing.T) {
	svc := NewWithDataDir(t.TempDir(), t.TempDir(), "test")
	_, release, ok := svc.beginTaskIfOpen()
	if !ok {
		t.Fatal("service task was not admitted")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	err := svc.Stop(stopCtx)
	cancel()
	if err == nil {
		t.Fatal("Stop unexpectedly succeeded while a task was held")
	}
	release()
	if _, _, admitted := svc.beginTaskIfOpen(); admitted {
		t.Fatal("timed-out service reopened task admission")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop error = %v, want deadline exceeded", err)
	}
	if err := svc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop retry after task release: %v", err)
	}
}

func TestServiceLifecycleContextCancelsDownloadGroups(t *testing.T) {
	svc := NewWithDataDir(t.TempDir(), t.TempDir(), "test")
	_, release, ok := svc.beginTaskIfOpen()
	if !ok {
		t.Fatal("service task was not admitted")
	}
	active, ctx, cancelCause := svc.beginDownloadGroup(componentFFmpeg)
	if active == nil || ctx == nil {
		t.Fatal("download group was not initialized")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stopDone := make(chan error, 1)
	go func() { stopDone <- svc.Stop(stopCtx) }()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("service stop did not cancel download context")
	}
	cancelCause(errDownloadFinished)
	svc.endDownloadGroup(componentFFmpeg, active)
	release()
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
