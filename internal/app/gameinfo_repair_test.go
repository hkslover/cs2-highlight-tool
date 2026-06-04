package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cs2-highlight-tool-v2/internal/config"
)

func TestEnsureCleanGameInfoBackupCreatesBackupFromCleanFile(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	app := &App{exeDir: env.exeDir, dataDir: env.exeDir}

	if err := app.ensureCleanGameInfoBackup(); err != nil {
		t.Fatalf("ensureCleanGameInfoBackup: %v", err)
	}

	backupBytes, err := os.ReadFile(app.cleanGameInfoBackupPath())
	if err != nil {
		t.Fatalf("read clean backup: %v", err)
	}
	if string(backupBytes) != env.originalGameInfo {
		t.Fatalf("backup content mismatch:\n%s", string(backupBytes))
	}
	if strings.Contains(string(backupBytes), "csgo/plugin") {
		t.Fatalf("backup must not contain plugin search path:\n%s", string(backupBytes))
	}
}

func TestEnsureCleanGameInfoBackupDoesNotOverwriteCleanBackupWithInjectedFile(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	app := &App{exeDir: env.exeDir, dataDir: env.exeDir}

	if err := app.ensureCleanGameInfoBackup(); err != nil {
		t.Fatalf("ensureCleanGameInfoBackup clean: %v", err)
	}
	injected := strings.Replace(env.originalGameInfo, "Game\tcsgo", "Game\tcsgo/plugin\n\t\tGame\tcsgo", 1)
	if err := os.WriteFile(env.gameInfoPath, []byte(injected), 0644); err != nil {
		t.Fatalf("write injected gameinfo: %v", err)
	}

	if err := app.ensureCleanGameInfoBackup(); err != nil {
		t.Fatalf("ensureCleanGameInfoBackup injected: %v", err)
	}

	backupBytes, err := os.ReadFile(app.cleanGameInfoBackupPath())
	if err != nil {
		t.Fatalf("read clean backup: %v", err)
	}
	if string(backupBytes) != env.originalGameInfo {
		t.Fatalf("injected gameinfo overwrote clean backup:\n%s", string(backupBytes))
	}
}

func TestEnsureCleanGameInfoBackupRefreshesBackupFromNewCleanFile(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	app := &App{exeDir: env.exeDir, dataDir: env.exeDir}
	if err := app.ensureCleanGameInfoBackup(); err != nil {
		t.Fatalf("ensureCleanGameInfoBackup initial: %v", err)
	}
	nextClean := strings.Replace(env.originalGameInfo, "Game\tcsgo\n", "Game\tcsgo\n\t\tGame\tcsgo_lv\n", 1)
	if err := os.WriteFile(env.gameInfoPath, []byte(nextClean), 0644); err != nil {
		t.Fatalf("write updated clean gameinfo: %v", err)
	}

	if err := app.ensureCleanGameInfoBackup(); err != nil {
		t.Fatalf("ensureCleanGameInfoBackup updated: %v", err)
	}

	backupBytes, err := os.ReadFile(app.cleanGameInfoBackupPath())
	if err != nil {
		t.Fatalf("read clean backup: %v", err)
	}
	if string(backupBytes) != nextClean {
		t.Fatalf("backup was not refreshed from current clean gameinfo:\n%s", string(backupBytes))
	}
}

func TestGetGameInfoStatusReportsAbnormalAndRepairRestoresBackup(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	app := &App{exeDir: env.exeDir, dataDir: env.exeDir}
	if err := app.ensureCleanGameInfoBackup(); err != nil {
		t.Fatalf("ensureCleanGameInfoBackup: %v", err)
	}
	injected := strings.Replace(env.originalGameInfo, "Game\tcsgo", "Game\tcsgo/plugin\n\t\tGame\tcsgo", 1)
	if err := os.WriteFile(env.gameInfoPath, []byte(injected), 0644); err != nil {
		t.Fatalf("write injected gameinfo: %v", err)
	}

	status, err := app.GetGameInfoStatus()
	if err != nil {
		t.Fatalf("GetGameInfoStatus: %v", err)
	}
	if status.Status != "abnormal" || status.Clean || !status.HasBackup || !status.CanRepair {
		t.Fatalf("unexpected abnormal status: %+v", status)
	}

	repaired, err := app.RepairGameInfoFromBackup()
	if err != nil {
		t.Fatalf("RepairGameInfoFromBackup: %v", err)
	}
	if repaired.Status != "normal" || !repaired.Clean || !repaired.HasBackup || repaired.CanRepair {
		t.Fatalf("unexpected repaired status: %+v", repaired)
	}
	restoredBytes, err := os.ReadFile(env.gameInfoPath)
	if err != nil {
		t.Fatalf("read restored gameinfo: %v", err)
	}
	if string(restoredBytes) != env.originalGameInfo {
		t.Fatalf("gameinfo was not restored from clean backup:\n%s", string(restoredBytes))
	}
}

func TestRepairGameInfoFromBackupFailsWhenBackupContainsPluginPath(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	app := &App{exeDir: env.exeDir, dataDir: env.exeDir}
	if err := os.MkdirAll(filepath.Dir(app.cleanGameInfoBackupPath()), 0755); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}
	if err := os.WriteFile(app.cleanGameInfoBackupPath(), []byte("Game\tcsgo/plugin\nGame\tcsgo\n"), 0644); err != nil {
		t.Fatalf("write dirty backup: %v", err)
	}
	if _, err := app.RepairGameInfoFromBackup(); err == nil {
		t.Fatalf("expected dirty backup repair to fail")
	}
}

func TestPrepareGameInfoForProduceCreatesPersistentCleanBackupBeforeInjecting(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	app := &App{exeDir: env.exeDir, dataDir: env.exeDir}

	if err := app.prepareGameInfoForProduce(); err != nil {
		t.Fatalf("prepareGameInfoForProduce: %v", err)
	}

	backupBytes, err := os.ReadFile(app.cleanGameInfoBackupPath())
	if err != nil {
		t.Fatalf("read persistent clean backup: %v", err)
	}
	if string(backupBytes) != env.originalGameInfo {
		t.Fatalf("persistent backup mismatch:\n%s", string(backupBytes))
	}
}

func TestGetGameInfoStatusReportsUnavailableWhenCS2Missing(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Default(dataDir)
	cfg.CS2Dir = filepath.Join(dataDir, "missing")
	cfg.CS2Exe = ""
	if err := config.Save(filepath.Join(dataDir, "config.json"), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	app := &App{exeDir: dataDir, dataDir: dataDir}

	status, err := app.GetGameInfoStatus()
	if err != nil {
		t.Fatalf("GetGameInfoStatus should be non-fatal: %v", err)
	}
	if status.Status != "unavailable" || status.Clean || status.CanRepair {
		t.Fatalf("unexpected unavailable status: %+v", status)
	}
}
