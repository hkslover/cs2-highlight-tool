package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/producegame"
)

// helper: rewrite config.json to flip PovHudEnabled on/off.
func setPovHudEnabled(t *testing.T, exeDir string, enabled bool) {
	t.Helper()
	path := filepath.Join(exeDir, "config.json")
	cfg, err := config.LoadOrCreate(path, exeDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.PovHudEnabled = enabled
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

func povVPKPath(env producePluginTestEnvironment) string {
	return filepath.Join(filepath.Dir(env.gameInfoPath), "pov.vpk")
}

// TestPrepareGameInfoForProduce_InjectsPluginAndPovWhenEnabled verifies that
// when PovHudEnabled=true, prepareGameInfoForProduce injects both
// csgo/plugin and csgo/pov.vpk in a single backup + write, and the restore path
// brings gameinfo.gi back to its exact original bytes.
func TestPrepareGameInfoForProduce_InjectsPluginAndPovWhenEnabled(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	setPovHudEnabled(t, env.exeDir, true)

	app := newTestApp(t, env.exeDir)
	if err := app.prepareGameInfoForProduce(); err != nil {
		t.Fatalf("prepareGameInfoForProduce: %v", err)
	}
	updated, err := os.ReadFile(env.gameInfoPath)
	if err != nil {
		t.Fatalf("read gameinfo: %v", err)
	}
	content := string(updated)
	if !producegame.HasSearchPath(content, producegame.SearchPathPlugin) {
		t.Fatalf("expected csgo/plugin to be injected:\n%s", content)
	}
	if !producegame.HasSearchPath(content, producegame.SearchPathPOV) {
		t.Fatalf("expected csgo/pov.vpk to be injected:\n%s", content)
	}
	backupPath := env.gameInfoPath + produceGameInfoBackupSuffix
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("expected single backup file, stat err=%v", err)
	}

	if err := app.forceRestoreGameInfoForProduce(); err != nil {
		t.Fatalf("forceRestoreGameInfoForProduce: %v", err)
	}
	restored, err := os.ReadFile(env.gameInfoPath)
	if err != nil {
		t.Fatalf("read restored gameinfo: %v", err)
	}
	if string(restored) != env.originalGameInfo {
		t.Fatalf("gameinfo not restored to original bytes:\n%s", string(restored))
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("backup should be removed, stat err=%v", err)
	}
}

// TestPrepareGameInfoForProduce_DoesNotInjectPovWhenDisabled is a regression
// guard: PovHudEnabled=false must produce exactly the legacy plugin-only output.
func TestPrepareGameInfoForProduce_DoesNotInjectPovWhenDisabled(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	setPovHudEnabled(t, env.exeDir, false)

	app := newTestApp(t, env.exeDir)
	if err := app.prepareGameInfoForProduce(); err != nil {
		t.Fatalf("prepareGameInfoForProduce: %v", err)
	}
	updated, err := os.ReadFile(env.gameInfoPath)
	if err != nil {
		t.Fatalf("read gameinfo: %v", err)
	}
	content := string(updated)
	if !producegame.HasSearchPath(content, producegame.SearchPathPlugin) {
		t.Fatalf("plugin path should be injected:\n%s", content)
	}
	if producegame.HasSearchPath(content, producegame.SearchPathPOV) {
		t.Fatalf("pov path must NOT be injected when toggle is off:\n%s", content)
	}
}

// TestPreparePovForProduce_WritesVPKWhenAbsent covers the canonical happy path:
// toggle on + no pre-existing vpk → embed bytes are written and vpkInstalled=true.
func TestPreparePovForProduce_WritesVPKWhenAbsent(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	setPovHudEnabled(t, env.exeDir, true)

	app := newTestApp(t, env.exeDir)
	if err := app.preparePovForProduce(); err != nil {
		t.Fatalf("preparePovForProduce: %v", err)
	}
	vpkPath := povVPKPath(env)
	written, err := os.ReadFile(vpkPath)
	if err != nil {
		t.Fatalf("read pov.vpk: %v", err)
	}
	if len(written) != len(producegame.PovVPK) {
		t.Fatalf("vpk bytes size = %d, want %d", len(written), len(producegame.PovVPK))
	}
	if !app.produceState.pov.vpkInstalled {
		t.Fatalf("vpkInstalled should be true after writing")
	}
	if app.produceState.pov.vpkPath != vpkPath {
		t.Fatalf("vpkPath = %q, want %q", app.produceState.pov.vpkPath, vpkPath)
	}

	if err := app.forceRestorePovForProduce(); err != nil {
		t.Fatalf("forceRestorePovForProduce: %v", err)
	}
	if _, err := os.Stat(vpkPath); !os.IsNotExist(err) {
		t.Fatalf("pov.vpk should be deleted after restore, stat err=%v", err)
	}
	if app.produceState.pov != (povSessionState{}) {
		t.Fatalf("pov session state should be reset after restore, got %+v", app.produceState.pov)
	}
	// Guarantee D3: no .cs2ht_pov.bak ever produced.
	if _, err := os.Stat(vpkPath + ".cs2ht_pov.bak"); !os.IsNotExist(err) {
		t.Fatalf(".cs2ht_pov.bak must NOT exist; stat err=%v", err)
	}
}

// TestPreparePovForProduce_LeavesExistingUserVPK covers Decision D3: when a
// pre-existing csgo/pov.vpk is present we must not overwrite it, and restore
// must leave it alone with its original bytes intact.
func TestPreparePovForProduce_LeavesExistingUserVPK(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	setPovHudEnabled(t, env.exeDir, true)

	vpkPath := povVPKPath(env)
	userBytes := []byte("user-supplied-pov-vpk-stub")
	if err := os.WriteFile(vpkPath, userBytes, 0644); err != nil {
		t.Fatalf("write existing vpk: %v", err)
	}

	app := newTestApp(t, env.exeDir)
	if err := app.preparePovForProduce(); err != nil {
		t.Fatalf("preparePovForProduce: %v", err)
	}
	if app.produceState.pov.vpkInstalled {
		t.Fatalf("vpkInstalled should be false when pre-existing vpk is found")
	}

	// User's file must be untouched.
	current, err := os.ReadFile(vpkPath)
	if err != nil {
		t.Fatalf("read existing vpk: %v", err)
	}
	if string(current) != string(userBytes) {
		t.Fatalf("user vpk overwritten; got %q want %q", string(current), string(userBytes))
	}

	if err := app.forceRestorePovForProduce(); err != nil {
		t.Fatalf("forceRestorePovForProduce: %v", err)
	}
	final, err := os.ReadFile(vpkPath)
	if err != nil {
		t.Fatalf("read vpk after restore: %v", err)
	}
	if string(final) != string(userBytes) {
		t.Fatalf("restore deleted user vpk; got %q want %q", string(final), string(userBytes))
	}
}

// TestPreparePovForProduce_NoopWhenDisabled is a regression guard: with the
// toggle off, prepare must not write csgo/pov.vpk and restore must not touch
// it. Behaviour should be identical to the plugin-only path.
func TestPreparePovForProduce_NoopWhenDisabled(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	setPovHudEnabled(t, env.exeDir, false)

	app := newTestApp(t, env.exeDir)
	if err := app.preparePovForProduce(); err != nil {
		t.Fatalf("preparePovForProduce: %v", err)
	}
	vpkPath := povVPKPath(env)
	if _, err := os.Stat(vpkPath); !os.IsNotExist(err) {
		t.Fatalf("pov.vpk must not be written when toggle is off, stat err=%v", err)
	}
	if app.produceState.pov != (povSessionState{}) {
		t.Fatalf("pov session state should remain zero, got %+v", app.produceState.pov)
	}
	if err := app.forceRestorePovForProduce(); err != nil {
		t.Fatalf("forceRestorePovForProduce should be a no-op when disabled: %v", err)
	}
}

// TestForceRestoreProduceEnvironmentForProduce_CleansPluginAndPovAndGameInfo
// covers the full launch-failure rollback: with PovHudEnabled=true, all three
// sub-systems (plugin DLL, POV vpk, gameinfo) must be cleanly restored, and
// only the gameinfo .cs2ht_produce.bak backup is ever used (D3).
func TestForceRestoreProduceEnvironmentForProduce_CleansPluginAndPovAndGameInfo(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	setPovHudEnabled(t, env.exeDir, true)

	app := newTestApp(t, env.exeDir)
	if err := app.prepareGameInfoForProduce(); err != nil {
		t.Fatalf("prepareGameInfoForProduce: %v", err)
	}
	if err := app.preparePluginDLLForProduce(); err != nil {
		t.Fatalf("preparePluginDLLForProduce: %v", err)
	}
	if err := app.preparePovForProduce(); err != nil {
		t.Fatalf("preparePovForProduce: %v", err)
	}

	gameInfoContent, err := os.ReadFile(env.gameInfoPath)
	if err != nil {
		t.Fatalf("read gameinfo: %v", err)
	}
	if !producegame.HasSearchPath(string(gameInfoContent), producegame.SearchPathPlugin) ||
		!producegame.HasSearchPath(string(gameInfoContent), producegame.SearchPathPOV) {
		t.Fatalf("gameinfo should hold both plugin + pov after prepare:\n%s", string(gameInfoContent))
	}
	vpkPath := povVPKPath(env)
	if _, err := os.Stat(vpkPath); err != nil {
		t.Fatalf("pov.vpk should exist after prepare: %v", err)
	}

	if err := app.forceRestoreProduceEnvironmentForProduce(); err != nil {
		t.Fatalf("forceRestoreProduceEnvironmentForProduce: %v", err)
	}

	restored, err := os.ReadFile(env.gameInfoPath)
	if err != nil {
		t.Fatalf("read restored gameinfo: %v", err)
	}
	if string(restored) != env.originalGameInfo {
		t.Fatalf("gameinfo not restored:\n%s", string(restored))
	}
	if _, err := os.Stat(env.gameInfoPath + produceGameInfoBackupSuffix); !os.IsNotExist(err) {
		t.Fatalf("gameinfo backup should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(vpkPath); !os.IsNotExist(err) {
		t.Fatalf("pov.vpk should be deleted after restore, stat err=%v", err)
	}
	// Confirm no .cs2ht_pov.bak file was ever introduced anywhere in csgo dir.
	csgoDir := filepath.Dir(env.gameInfoPath)
	entries, err := os.ReadDir(csgoDir)
	if err != nil {
		t.Fatalf("read csgo dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".cs2ht_pov.bak") {
			t.Fatalf("unexpected POV backup file present: %s", e.Name())
		}
	}
	if _, err := os.Stat(env.targetDLLPath); !os.IsNotExist(err) {
		t.Fatalf("plugin DLL should be removed after restore, stat err=%v", err)
	}
}

// TestGetClipSettings_PovHudEnabledRoundTrip verifies that the toggle survives
// a Save → Get cycle through the Wails-bound clip settings API.
func TestGetClipSettings_PovHudEnabledRoundTrip(t *testing.T) {
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
	if !current.PovHudEnabled {
		t.Fatalf("default PovHudEnabled should be true, got %v", current.PovHudEnabled)
	}

	current.PovHudEnabled = false
	saved, err := app.SaveClipSettings(*current)
	if err != nil {
		t.Fatalf("SaveClipSettings: %v", err)
	}
	if saved.PovHudEnabled {
		t.Fatalf("SaveClipSettings should round-trip PovHudEnabled=false, got %v", saved.PovHudEnabled)
	}

	reloaded, err := app.GetClipSettings()
	if err != nil {
		t.Fatalf("GetClipSettings (reload): %v", err)
	}
	if reloaded.PovHudEnabled {
		t.Fatalf("PovHudEnabled should persist across reload, got %v", reloaded.PovHudEnabled)
	}

	// Flip on again to verify both directions.
	reloaded.PovHudEnabled = true
	if _, err := app.SaveClipSettings(*reloaded); err != nil {
		t.Fatalf("SaveClipSettings (on): %v", err)
	}
	reloaded2, err := app.GetClipSettings()
	if err != nil {
		t.Fatalf("GetClipSettings (on reload): %v", err)
	}
	if !reloaded2.PovHudEnabled {
		t.Fatalf("PovHudEnabled should round-trip to true, got %v", reloaded2.PovHudEnabled)
	}
}
