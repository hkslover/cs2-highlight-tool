package app

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/envsetup"
)

func TestAppUpdateConfigSerializesReadModifyWrite(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	defer release()
	firstErr := make(chan error, 1)
	go func() {
		_, err := app.updateConfig(func(cfg *config.Config) error {
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
		t.Fatal("first config update did not enter mutate")
	}

	secondDone := make(chan struct{})
	secondErr := make(chan error, 1)
	go func() {
		_, err := app.updateConfig(func(cfg *config.Config) error {
			cfg.FiveEPlayerName = "second"
			return nil
		})
		close(secondDone)
		secondErr <- err
	}()
	select {
	case <-secondDone:
		t.Fatal("second config update ran while first Store transaction was held")
	case <-time.After(50 * time.Millisecond):
	}
	release()

	if err := <-firstErr; err != nil {
		t.Fatalf("first update: %v", err)
	}
	if err := <-secondErr; err != nil {
		t.Fatalf("second update: %v", err)
	}
	cfg, err := config.LoadOrCreate(filepath.Join(exeDir, "config.json"), exeDir)
	if err != nil {
		t.Fatalf("load final config: %v", err)
	}
	if cfg.LastChangelogVersion != "first" || cfg.FiveEPlayerName != "second" {
		t.Fatalf("concurrent updates lost data: %+v", cfg)
	}
}

func TestAppAndServiceShareWorkspaceConfigStore(t *testing.T) {
	exeDir := t.TempDir()
	dataDir := t.TempDir()
	store := config.NewStore(filepath.Join(dataDir, "config.json"), dataDir)
	svc := envsetup.NewWithDataDirAndStore(exeDir, dataDir, "test", store)
	app := &App{exeDir: exeDir, dataDir: dataDir, service: svc}
	app.ensureWorkspaceSession()

	snapshot := app.workspaceSnapshot()
	if snapshot.store != store || svc.ConfigStore() != store {
		t.Fatalf("workspace did not retain one Store instance: app=%p service=%p want=%p", snapshot.store, svc.ConfigStore(), store)
	}
	if _, err := app.updateConfig(func(cfg *config.Config) error {
		cfg.RecordQuality = "ultra"
		return nil
	}); err != nil {
		t.Fatalf("app update: %v", err)
	}
	if _, err := store.Update(func(cfg *config.Config) error {
		cfg.FFmpegDetectedPreset = "n1_h264"
		cfg.FFmpegDetectedEncoders = []string{"libx264"}
		return nil
	}); err != nil {
		t.Fatalf("service update: %v", err)
	}
	cfg, err := app.loadConfig()
	if err != nil {
		t.Fatalf("load final config: %v", err)
	}
	if cfg.RecordQuality != "ultra" || cfg.FFmpegDetectedPreset != "n1_h264" || len(cfg.FFmpegDetectedEncoders) != 1 {
		t.Fatalf("shared Store lost cross-layer fields: %+v", cfg)
	}
}

func TestAppAndServiceConfigTransactionsPreserveBothFieldsInEitherOrder(t *testing.T) {
	for _, serviceFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "service-first", false: "app-first"}[serviceFirst], func(t *testing.T) {
			exeDir := t.TempDir()
			dataDir := t.TempDir()
			store := config.NewStore(filepath.Join(dataDir, "config.json"), dataDir)
			svc := envsetup.NewWithDataDirAndStore(exeDir, dataDir, "test", store)
			app := &App{exeDir: exeDir, dataDir: dataDir, service: svc}
			app.ensureWorkspaceSession()

			firstEntered := make(chan struct{})
			secondEntered := make(chan struct{})
			releaseFirst := make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
			defer release()
			firstErr := make(chan error, 1)
			secondErr := make(chan error, 1)
			if serviceFirst {
				go func() {
					_, err := store.Update(func(cfg *config.Config) error {
						cfg.FFmpegDetectedPreset = "n1_h264"
						close(firstEntered)
						<-releaseFirst
						return nil
					})
					firstErr <- err
				}()
			} else {
				go func() {
					_, err := app.updateConfig(func(cfg *config.Config) error {
						cfg.RecordQuality = "ultra"
						close(firstEntered)
						<-releaseFirst
						return nil
					})
					firstErr <- err
				}()
			}
			select {
			case <-firstEntered:
			case <-time.After(time.Second):
				t.Fatal("first transaction did not enter mutate")
			}

			if serviceFirst {
				go func() {
					_, err := app.updateConfig(func(cfg *config.Config) error {
						close(secondEntered)
						cfg.RecordQuality = "ultra"
						return nil
					})
					secondErr <- err
				}()
			} else {
				go func() {
					_, err := store.Update(func(cfg *config.Config) error {
						close(secondEntered)
						cfg.FFmpegDetectedPreset = "n1_h264"
						return nil
					})
					secondErr <- err
				}()
			}
			select {
			case <-secondEntered:
				t.Fatal("second cross-layer transaction entered before first committed")
			case <-time.After(50 * time.Millisecond):
			}
			release()
			if err := <-firstErr; err != nil {
				t.Fatalf("first transaction: %v", err)
			}
			if err := <-secondErr; err != nil {
				t.Fatalf("second transaction: %v", err)
			}
			cfg, err := store.Snapshot()
			if err != nil {
				t.Fatalf("final snapshot: %v", err)
			}
			if cfg.RecordQuality != "ultra" || cfg.FFmpegDetectedPreset != "n1_h264" {
				t.Fatalf("cross-layer transaction lost a field: %+v", cfg)
			}
		})
	}
}
