package app

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/demo"
	"cs2-highlight-tool-v2/internal/producegame"
	"cs2-highlight-tool-v2/internal/producews"

	"github.com/gorilla/websocket"
)

func setBrandEnabled(t *testing.T, exeDir string, enabled bool) {
	t.Helper()
	path := filepath.Join(exeDir, "config.json")
	cfg, err := config.LoadOrCreate(path, exeDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.BrandEnabled = enabled
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

func TestProduceBrand_MountedAndRestoredWhenEnabled(t *testing.T) {
	for _, povEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("pov=%t", povEnabled), func(t *testing.T) {
			env := setupProducePluginTestEnvironment(t)
			setBrandEnabled(t, env.exeDir, true)
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

func TestProduceBrand_DisabledSkipsInstallAndTemporarilyUnmountsSearchPath(t *testing.T) {
	for _, hasPreexisting := range []bool{false, true} {
		t.Run(fmt.Sprintf("preexisting=%t", hasPreexisting), func(t *testing.T) {
			env := setupProducePluginTestEnvironment(t)
			setBrandEnabled(t, env.exeDir, false)
			setPovHudEnabled(t, env.exeDir, false)

			brandPath := filepath.Join(filepath.Dir(env.gameInfoPath), "brand.vpk")
			userBytes := []byte("user brand")
			originalGameInfo := env.originalGameInfo
			if hasPreexisting {
				originalGameInfo = strings.Replace(
					originalGameInfo,
					"Game\tcsgo\n",
					"Game\tcsgo/brand.vpk\n\t\tGame\tcsgo\n",
					1,
				)
				if err := os.WriteFile(env.gameInfoPath, []byte(originalGameInfo), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(brandPath, userBytes, 0644); err != nil {
					t.Fatal(err)
				}
			}

			app := newTestApp(t, env.exeDir)
			epoch := app.beginProduceEnvironmentPrep()
			if err := app.prepareGameInfoForProduce(); err != nil {
				t.Fatal(err)
			}
			if err := app.prepareBrandForProduce(); err != nil {
				t.Fatal(err)
			}

			content, err := os.ReadFile(env.gameInfoPath)
			if err != nil {
				t.Fatal(err)
			}
			if producegame.HasSearchPath(string(content), producegame.SearchPathBrand) {
				t.Fatalf("disabled brand must not remain mounted in gameinfo.gi:\n%s", content)
			}
			if !producegame.HasSearchPath(string(content), producegame.SearchPathPlugin) {
				t.Fatalf("plugin search path should remain mounted:\n%s", content)
			}
			if app.produceState.brand != (brandSessionState{}) {
				t.Fatalf("brand session state should stay zero when disabled, got %+v", app.produceState.brand)
			}
			if hasPreexisting {
				currentBrand, err := os.ReadFile(brandPath)
				if err != nil || !bytes.Equal(currentBrand, userBytes) {
					t.Fatalf("existing user brand.vpk was modified while disabled: %v", err)
				}
			} else if _, err := os.Stat(brandPath); !os.IsNotExist(err) {
				t.Fatalf("brand.vpk must not be created when disabled, stat err=%v", err)
			}

			if err := app.forceRestoreProduceEnvironmentForEpoch(epoch); err != nil {
				t.Fatal(err)
			}
			restored, err := os.ReadFile(env.gameInfoPath)
			if err != nil || string(restored) != originalGameInfo {
				t.Fatalf("gameinfo not restored: %v, %s", err, restored)
			}
			if hasPreexisting {
				currentBrand, err := os.ReadFile(brandPath)
				if err != nil || !bytes.Equal(currentBrand, userBytes) {
					t.Fatalf("existing user brand.vpk was removed on restore: %v", err)
				}
			}
		})
	}
}

func TestGetClipSettings_BrandEnabledRoundTrip(t *testing.T) {
	exeDir := t.TempDir()
	cfg := config.Default(exeDir)
	if err := config.Save(filepath.Join(exeDir, "config.json"), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	app := newTestApp(t, exeDir)
	current, err := app.GetClipSettings()
	if err != nil {
		t.Fatalf("GetClipSettings: %v", err)
	}
	if !current.BrandEnabled {
		t.Fatalf("default BrandEnabled should be true, got %v", current.BrandEnabled)
	}

	current.BrandEnabled = false
	saved, err := app.SaveClipSettings(*current)
	if err != nil {
		t.Fatalf("SaveClipSettings(false): %v", err)
	}
	if saved.BrandEnabled {
		t.Fatalf("SaveClipSettings should round-trip BrandEnabled=false, got %v", saved.BrandEnabled)
	}

	reloaded, err := app.GetClipSettings()
	if err != nil {
		t.Fatalf("GetClipSettings(reload): %v", err)
	}
	if reloaded.BrandEnabled {
		t.Fatalf("BrandEnabled=false should persist across reload, got %v", reloaded.BrandEnabled)
	}

	reloaded.BrandEnabled = true
	if _, err := app.SaveClipSettings(*reloaded); err != nil {
		t.Fatalf("SaveClipSettings(true): %v", err)
	}
	reloaded2, err := app.GetClipSettings()
	if err != nil {
		t.Fatalf("GetClipSettings(on reload): %v", err)
	}
	if !reloaded2.BrandEnabled {
		t.Fatalf("BrandEnabled=true should persist across reload, got %v", reloaded2.BrandEnabled)
	}
}

func TestGeneratePluginJSONBatchAndLaunchHLAE_RespectsBrandEnabledSetting(t *testing.T) {
	for _, brandEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("brand=%t", brandEnabled), func(t *testing.T) {
			exeDir := t.TempDir()
			demoPath, cs2Exe := prepareLaunchTestEnvironment(t, exeDir)
			produceW := producews.NewDefault(nil)
			if err := produceW.Start(); err != nil {
				t.Fatalf("start produce websocket: %v", err)
			}
			defer produceW.Stop()

			app := newTestApp(t, exeDir, func(a *App) { a.produceW = produceW })
			current, err := app.GetClipSettings()
			if err != nil {
				t.Fatalf("GetClipSettings: %v", err)
			}
			current.BrandEnabled = brandEnabled
			if _, err := app.SaveClipSettings(*current); err != nil {
				t.Fatalf("SaveClipSettings: %v", err)
			}

			gameInfoPath := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(cs2Exe))), "csgo", "gameinfo.gi")
			brandPath := filepath.Join(filepath.Dir(gameInfoPath), "brand.vpk")
			var observedGameInfo string
			var observedBrandExists bool
			var gameConn *websocket.Conn
			t.Cleanup(func() {
				if gameConn != nil {
					_ = gameConn.Close()
				}
			})

			oldLaunchCmd := launchHLAECommand
			oldListFn := listCS2PIDsFn
			oldCloseFn := closeCS2ProcessByPIDFn
			t.Cleanup(func() {
				launchHLAECommand = oldLaunchCmd
				listCS2PIDsFn = oldListFn
				closeCS2ProcessByPIDFn = oldCloseFn
			})
			launchHLAECommand = func(name string, args ...string) *exec.Cmd {
				content, readErr := os.ReadFile(gameInfoPath)
				if readErr != nil {
					t.Fatalf("read gameinfo during launch: %v", readErr)
				}
				observedGameInfo = string(content)
				_, statErr := os.Stat(brandPath)
				observedBrandExists = statErr == nil

				wsURL := url.URL{Scheme: "ws", Host: produceW.Address(), Path: "/", RawQuery: "process=game"}
				conn, _, dialErr := websocket.DefaultDialer.Dial(wsURL.String(), nil)
				if dialErr != nil {
					t.Fatalf("dial game websocket: %v", dialErr)
				}
				gameConn = conn
				return helperLaunchCommand(name, args...)
			}
			listCalls := 0
			listCS2PIDsFn = func() ([]int, error) {
				listCalls++
				if listCalls == 1 {
					return []int{1000}, nil
				}
				return []int{1000, 1001}, nil
			}
			closeCS2ProcessByPIDFn = func(pid int) error { return nil }

			result, err := app.GeneratePluginJSONBatchAndLaunchHLAE(GeneratePluginJSONBatchRequest{
				Jobs: []GeneratePluginJSONRequest{{
					DemoPath:      demoPath,
					TickRate:      64,
					SelectedItems: []SelectedClipItem{{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7}}},
				}},
			})
			if err != nil {
				t.Fatalf("GeneratePluginJSONBatchAndLaunchHLAE: %v", err)
			}
			if !result.LaunchStarted {
				t.Fatalf("expected launch to succeed, got %+v", result)
			}
			if err := app.stopProduceSessionWorker(); err != nil {
				t.Fatalf("stopProduceSessionWorker: %v", err)
			}
			if producegame.HasSearchPath(observedGameInfo, producegame.SearchPathBrand) != brandEnabled {
				t.Fatalf("mounted SearchPathBrand = %v, want %v:\n%s",
					producegame.HasSearchPath(observedGameInfo, producegame.SearchPathBrand),
					brandEnabled,
					observedGameInfo,
				)
			}
			if observedBrandExists != brandEnabled {
				t.Fatalf("brand.vpk exists during launch = %v, want %v", observedBrandExists, brandEnabled)
			}
			if _, statErr := os.Stat(brandPath); !os.IsNotExist(statErr) {
				t.Fatalf("brand.vpk must not remain after teardown, stat err=%v", statErr)
			}
		})
	}
}
