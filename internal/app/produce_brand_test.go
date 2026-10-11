package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cs2-highlight-tool-v2/internal/producegame"
)

func TestProduceBrand_AlwaysMountedAndRestored(t *testing.T) {
	for _, povEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("pov=%t", povEnabled), func(t *testing.T) {
			env := setupProducePluginTestEnvironment(t)
			setPovHudEnabled(t, env.exeDir, povEnabled)
			app := newTestApp(t, env.exeDir)
			brandPath := filepath.Join(filepath.Dir(env.gameInfoPath), "brand.vpk")
			for cycle := 0; cycle < 2; cycle++ {
				epoch := app.beginProduceEnvironmentPrep()
				if err := app.prepareGameInfoForProduce(); err != nil {
					t.Fatal(err)
				}
				if err := app.preparePluginDLLForProduce(); err != nil {
					t.Fatal(err)
				}
				if err := app.preparePovForProduce(); err != nil {
					t.Fatal(err)
				}
				if err := app.prepareBrandForProduce(); err != nil {
					t.Fatal(err)
				}
				// Repeated preparation must retain ownership for teardown.
				if err := app.prepareBrandForProduce(); err != nil {
					t.Fatal(err)
				}
				content, err := os.ReadFile(env.gameInfoPath)
				if err != nil {
					t.Fatal(err)
				}
				if countSearchPathEntries(string(content), producegame.SearchPathBrand) != 1 {
					t.Fatalf("expected exactly one brand mount: %s", content)
				}
				if povEnabled && strings.Index(string(content), producegame.SearchPathBrand) > strings.Index(string(content), producegame.SearchPathPOV) {
					t.Fatalf("brand must precede POV: %s", content)
				}
				brand, err := os.ReadFile(brandPath)
				if err != nil || !bytes.Equal(brand, producegame.BrandVPK) {
					t.Fatalf("brand asset mismatch: %v", err)
				}
				if err := app.forceRestoreProduceEnvironmentForEpoch(epoch); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(brandPath); !os.IsNotExist(err) {
					t.Fatalf("brand not removed: %v", err)
				}
				restored, err := os.ReadFile(env.gameInfoPath)
				if err != nil || string(restored) != env.originalGameInfo {
					t.Fatalf("gameinfo not restored: %v, %s", err, restored)
				}
			}
		})
	}
}

func TestProduceBrand_PreservesExistingUserVPK(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	app := newTestApp(t, env.exeDir)
	path := filepath.Join(filepath.Dir(env.gameInfoPath), "brand.vpk")
	userBytes := []byte("user brand")
	if err := os.WriteFile(path, userBytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := app.prepareGameInfoForProduce(); err != nil {
		t.Fatal(err)
	}
	if err := app.prepareBrandForProduce(); err != nil {
		t.Fatal(err)
	}
	if err := app.forceRestoreProduceEnvironmentForProduce(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(content, userBytes) {
		t.Fatalf("user brand modified: %v", err)
	}
}

func TestProduceBrand_PrepareFailureRollsBackEnvironment(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	setPovHudEnabled(t, env.exeDir, true)
	app := newTestApp(t, env.exeDir)
	epoch := app.beginProduceEnvironmentPrep()
	if err := app.prepareGameInfoForProduce(); err != nil {
		t.Fatal(err)
	}
	if err := app.preparePluginDLLForProduce(); err != nil {
		t.Fatal(err)
	}
	if err := app.preparePovForProduce(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(env.gameInfoPath), "brand.vpk")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := app.prepareBrandForProduce(); err == nil {
		t.Fatal("directory must block brand preparation")
	}
	if err := app.rollbackProduceLaunchEnvironment(0, epoch); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{env.targetDLLPath, povVPKPath(env)} {
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("rollback left %s: %v", target, err)
		}
	}
	content, err := os.ReadFile(env.gameInfoPath)
	if err != nil || string(content) != env.originalGameInfo {
		t.Fatalf("rollback did not restore gameinfo: %v", err)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("existing directory changed: %v", err)
	}
}

func TestProduceBrand_RestoreEpochAndFailureRetry(t *testing.T) {
	app := &App{}
	old := app.beginProduceEnvironmentPrep()
	owner := app.beginProduceEnvironmentPrep()
	path := filepath.Join(t.TempDir(), "brand.vpk")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(path, "locked")
	if err := os.WriteFile(child, []byte("block removal"), 0644); err != nil {
		t.Fatal(err)
	}
	app.produceState.brand = brandSessionState{vpkPath: path, vpkInstalled: true}
	if err := app.forceRestoreProduceEnvironmentForEpoch(old); err != nil {
		t.Fatal(err)
	}
	if !app.produceState.brand.vpkInstalled {
		t.Fatal("old epoch cleared brand ownership")
	}
	if err := app.forceRestoreProduceEnvironmentForEpoch(owner); err == nil {
		t.Fatal("expected removal failure")
	}
	if !app.produceState.brand.vpkInstalled {
		t.Fatal("failure must retain cleanup state")
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := app.forceRestoreProduceEnvironmentForEpoch(owner); err != nil {
		t.Fatal(err)
	}
	if app.produceState.brand != (brandSessionState{}) {
		t.Fatal("retry did not clear brand state")
	}
}

func TestProduceBrand_ResidualMountHealthAndRepair(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	app := newTestApp(t, env.exeDir)
	content := "// Game csgo/brand.vpk\nGame csgo/brand.vpk\n" + env.originalGameInfo
	if err := os.WriteFile(env.gameInfoPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	health, err := app.GetGameInfoHealth()
	if err != nil || !health.NeedsRepair {
		t.Fatalf("brand residual not detected: %+v, %v", health, err)
	}
	health, err = app.RepairGameInfo()
	if err != nil || health.NeedsRepair {
		t.Fatalf("brand repair failed: %+v, %v", health, err)
	}
	repaired, err := os.ReadFile(env.gameInfoPath)
	if err != nil || string(repaired) != "// Game csgo/brand.vpk\n"+env.originalGameInfo {
		t.Fatalf("repair changed unrelated content: %v, %s", err, repaired)
	}
}
