package config

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStoreUpdateSerializesReadModifyWrite(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "config.json"), dir)
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	defer release()
	firstErr := make(chan error, 1)
	go func() {
		_, err := store.Update(func(cfg *Config) error {
			cfg.LastChangelogVersion = "first"
			close(firstEntered)
			<-releaseFirst
			return nil
		})
		firstErr <- err
	}()
	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first update did not enter mutate")
	}

	secondEntered := make(chan struct{})
	secondErr := make(chan error, 1)
	go func() {
		_, err := store.Update(func(cfg *Config) error {
			close(secondEntered)
			cfg.FiveEPlayerName = "second"
			return nil
		})
		secondErr <- err
	}()
	select {
	case <-secondEntered:
		t.Fatal("second update entered mutate while first transaction was held")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	if err := <-firstErr; err != nil {
		t.Fatalf("first update: %v", err)
	}
	if err := <-secondErr; err != nil {
		t.Fatalf("second update: %v", err)
	}

	cfg, err := store.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if cfg.LastChangelogVersion != "first" || cfg.FiveEPlayerName != "second" {
		t.Fatalf("serialized updates lost data: %+v", cfg)
	}
}

func TestStoreSnapshotCopiesMutableFields(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "config.json"), dir)
	if _, err := store.Update(func(cfg *Config) error {
		cfg.FFmpegDetectedEncoders = []string{"h264_nvenc", "libx264"}
		cfg.ClipActionSettings = Ptr(ClipActionSettings{EnableVoiceIndices: true, EnableVoiceIndicesH: true})
		return nil
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	first, err := store.Snapshot()
	if err != nil {
		t.Fatalf("first snapshot: %v", err)
	}
	first.FFmpegDetectedEncoders[0] = "mutated"
	first.FFmpegDetectedEncoders = append(first.FFmpegDetectedEncoders, "extra")
	first.ClipActionSettings.EnableVoiceIndices = false

	second, err := store.Snapshot()
	if err != nil {
		t.Fatalf("second snapshot: %v", err)
	}
	if second.FFmpegDetectedEncoders[0] != "h264_nvenc" || len(second.FFmpegDetectedEncoders) != 2 {
		t.Fatalf("encoder slice leaked through snapshot: %+v", second.FFmpegDetectedEncoders)
	}
	if second.ClipActionSettings == nil || !second.ClipActionSettings.EnableVoiceIndices {
		t.Fatalf("clip action pointer leaked through snapshot: %+v", second.ClipActionSettings)
	}
}

func TestStoreFailedTransactionsDoNotPublish(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "config.json"), dir)
	if _, err := store.Snapshot(); err != nil {
		t.Fatalf("initialize config: %v", err)
	}
	mutateErr := errors.New("mutate failed")
	if _, err := store.Update(func(cfg *Config) error {
		cfg.FiveEPlayerName = "not committed"
		return mutateErr
	}); !errors.Is(err, mutateErr) {
		t.Fatalf("mutate error = %v, want %v", err, mutateErr)
	}
	cfg, err := store.Snapshot()
	if err != nil {
		t.Fatalf("snapshot after mutate failure: %v", err)
	}
	if cfg.FiveEPlayerName != "" {
		t.Fatalf("failed mutate was published: %q", cfg.FiveEPlayerName)
	}

	saveErr := errors.New("save failed")
	store.saveFn = func(string, *Config) error { return saveErr }
	if _, err := store.Update(func(cfg *Config) error {
		cfg.FiveEPlayerName = "not saved"
		return nil
	}); !errors.Is(err, saveErr) {
		t.Fatalf("save error = %v, want %v", err, saveErr)
	}
	store.saveFn = nil
	cfg, err = store.Snapshot()
	if err != nil {
		t.Fatalf("snapshot after save failure: %v", err)
	}
	if cfg.FiveEPlayerName != "" {
		t.Fatalf("failed save was published: %q", cfg.FiveEPlayerName)
	}

	firstWriteDir := t.TempDir()
	firstWriteStore := NewStore(filepath.Join(firstWriteDir, "config.json"), firstWriteDir)
	firstWriteStore.saveFn = func(string, *Config) error { return saveErr }
	if _, err := firstWriteStore.Update(func(cfg *Config) error {
		cfg.FiveEPlayerName = "never written"
		return nil
	}); !errors.Is(err, saveErr) {
		t.Fatalf("first-write save error = %v, want %v", err, saveErr)
	}
	if _, err := firstWriteStore.Snapshot(); !errors.Is(err, saveErr) {
		t.Fatalf("first-write snapshot error = %v, want %v", err, saveErr)
	}
}

func TestStoreWorkspaceIdentityAndClose(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	first := NewStore(filepath.Join(firstDir, "config.json"), firstDir)
	second := NewStore(filepath.Join(secondDir, "config.json"), secondDir)
	if _, err := first.Update(func(cfg *Config) error {
		cfg.FiveEPlayerName = "first"
		return nil
	}); err != nil {
		t.Fatalf("first update: %v", err)
	}
	if _, err := second.Update(func(cfg *Config) error {
		cfg.FiveEPlayerName = "second"
		return nil
	}); err != nil {
		t.Fatalf("second update: %v", err)
	}
	first.Close()
	if _, err := first.Update(func(cfg *Config) error { return nil }); !errors.Is(err, ErrStoreClosed) {
		t.Fatalf("closed update error = %v, want ErrStoreClosed", err)
	}
	if _, err := second.Snapshot(); err != nil {
		t.Fatalf("independent store closed unexpectedly: %v", err)
	}
	if first.Path() == second.Path() || first.DataRoot() == second.DataRoot() {
		t.Fatal("workspace stores unexpectedly share identity")
	}
}
