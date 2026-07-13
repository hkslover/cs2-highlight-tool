package app

import (
	"path/filepath"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/config"
)

func TestAppUpdateConfigSerializesReadModifyWrite(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
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
	<-firstEntered

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
		t.Fatal("second config update ran while first update held configMu")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)

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
