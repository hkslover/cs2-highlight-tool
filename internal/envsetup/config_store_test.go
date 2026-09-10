package envsetup

import (
	"context"
	"errors"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/config"
)

func TestServicePersistConfigPublishesCommittedSnapshot(t *testing.T) {
	exeDir := t.TempDir()
	dataDir := t.TempDir()
	svc := NewWithDataDir(exeDir, dataDir, "test")
	svc.Startup(nil)
	if _, err := svc.persistConfig(func(cfg *config.Config) error {
		cfg.FFmpegDetectedPreset = "n1_h264"
		cfg.FFmpegDetectedEncoders = []string{"libx264"}
		return nil
	}); err != nil {
		t.Fatalf("persist config: %v", err)
	}
	stored, err := svc.ConfigStore().Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if stored.FFmpegDetectedPreset != "n1_h264" || len(stored.FFmpegDetectedEncoders) != 1 {
		t.Fatalf("persisted detection cache missing: %+v", stored)
	}
	current := svc.currentConfig()
	if current.FFmpegDetectedPreset != stored.FFmpegDetectedPreset {
		t.Fatalf("service snapshot diverged from Store: %+v vs %+v", current, stored)
	}
}

func TestServiceConfigPublicationRejectsOlderRevision(t *testing.T) {
	exeDir := t.TempDir()
	dataDir := t.TempDir()
	svc := NewWithDataDir(exeDir, dataDir, "test")
	store := svc.ConfigStore()
	if _, _, err := store.SnapshotWithRevision(); err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}
	old, oldRevision, err := store.SnapshotWithRevision()
	if err != nil {
		t.Fatalf("old snapshot: %v", err)
	}
	newer, newerRevision, err := store.UpdateWithRevision(func(cfg *config.Config) error {
		cfg.RecordQuality = "ultra"
		return nil
	})
	if err != nil {
		t.Fatalf("newer update: %v", err)
	}
	svc.ApplyConfigSnapshot(newer, newerRevision)
	svc.ApplyConfigSnapshot(old, oldRevision)
	if got := svc.currentConfig().RecordQuality; got != "ultra" {
		t.Fatalf("older revision replaced newer snapshot: %q", got)
	}
}

func TestServiceStopClosesConfigStore(t *testing.T) {
	svc := NewWithDataDir(t.TempDir(), t.TempDir(), "test")
	store := svc.ConfigStore()
	if err := svc.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, err := store.Update(func(*config.Config) error { return nil }); !errors.Is(err, config.ErrStoreClosed) {
		t.Fatalf("store update after Stop = %v, want ErrStoreClosed", err)
	}
}

func TestServiceStopTimeoutStillClosesConfigStore(t *testing.T) {
	svc := NewWithDataDir(t.TempDir(), t.TempDir(), "test")
	store := svc.ConfigStore()
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
	if _, updateErr := store.Update(func(*config.Config) error { return nil }); !errors.Is(updateErr, config.ErrStoreClosed) {
		t.Fatalf("store update after timed-out Stop = %v, want ErrStoreClosed", updateErr)
	}
	release()
	if err := svc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop retry: %v", err)
	}
}
