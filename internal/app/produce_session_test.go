package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/demo"
	"cs2-highlight-tool-v2/internal/plugingen"
	"cs2-highlight-tool-v2/internal/producemerge"
	"cs2-highlight-tool-v2/internal/producews"

	"github.com/gorilla/websocket"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestRunProduceSessionWorkerNaturalCompletionClosesDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	state := &produceSessionRuntime{
		cancel:        cancel,
		done:          make(chan struct{}),
		taskCh:        make(chan mergeTask, 1),
		pollInterval:  time.Millisecond,
		queueStopped:  true,
		closeDone:     true,
		seenCompleted: make(map[string]struct{}),
		demoSubDirs:   make(map[string]string),
		plansByTake:   make(map[string]ProduceTakePlan),
	}
	app := &App{produceW: producews.NewDefault(nil)}
	state.workerWG.Add(1)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		defer state.workerWG.Done()
		app.mergeWorker(ctx, state)
	}()

	runnerDone := make(chan struct{})
	go func() {
		defer close(runnerDone)
		app.runProduceSessionWorker(ctx, state)
	}()

	select {
	case <-runnerDone:
	case <-time.After(time.Second):
		t.Fatal("natural completion did not stop the session runner")
	}
	select {
	case <-state.done:
	case <-time.After(time.Second):
		t.Fatal("session done channel was not closed")
	}
	select {
	case <-workerDone:
	case <-time.After(time.Second):
		t.Fatal("merge worker did not stop with the session")
	}
}

func TestPrepareAndRestoreGameInfoForProduce(t *testing.T) {
	exeDir := t.TempDir()
	cs2Root := t.TempDir()
	cs2Exe := filepath.Join(cs2Root, "game", "bin", "win64", "cs2.exe")
	if err := os.MkdirAll(filepath.Dir(cs2Exe), 0755); err != nil {
		t.Fatalf("mkdir cs2 exe dir: %v", err)
	}
	if err := os.WriteFile(cs2Exe, []byte("exe"), 0644); err != nil {
		t.Fatalf("write cs2 exe: %v", err)
	}

	gameInfoPath := filepath.Join(cs2Root, "game", "csgo", "gameinfo.gi")
	if err := os.MkdirAll(filepath.Dir(gameInfoPath), 0755); err != nil {
		t.Fatalf("mkdir gameinfo dir: %v", err)
	}
	original := "FileSystem\n{\n\tSearchPaths\n\t{\n\t\tGame\tcsgo\n\t}\n}\n"
	if err := os.WriteFile(gameInfoPath, []byte(original), 0644); err != nil {
		t.Fatalf("write gameinfo: %v", err)
	}

	cfg := config.Default(exeDir)
	cfg.CS2Exe = cs2Exe
	cfg.CS2Dir = cs2Root
	if err := config.Save(filepath.Join(exeDir, "config.json"), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	app := &App{exeDir: exeDir}
	if err := app.prepareGameInfoForProduce(); err != nil {
		t.Fatalf("prepare gameinfo: %v", err)
	}

	updatedBytes, err := os.ReadFile(gameInfoPath)
	if err != nil {
		t.Fatalf("read updated gameinfo: %v", err)
	}
	updated := string(updatedBytes)
	if !strings.Contains(updated, "Game\tcsgo/plugin") {
		t.Fatalf("expected injected search path, got:\n%s", updated)
	}
	backupPath := gameInfoPath + produceGameInfoBackupSuffix
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("expected backup file, stat error: %v", err)
	}

	if err := app.forceRestoreGameInfoForProduce(); err != nil {
		t.Fatalf("restore gameinfo: %v", err)
	}
	restoredBytes, err := os.ReadFile(gameInfoPath)
	if err != nil {
		t.Fatalf("read restored gameinfo: %v", err)
	}
	if string(restoredBytes) != original {
		t.Fatalf("restored gameinfo mismatch:\n%s", string(restoredBytes))
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("backup file should be removed, stat err=%v", err)
	}
}

func TestPrepareGameInfoForProduce_NoBackupWhenAlreadyInjected(t *testing.T) {
	exeDir := t.TempDir()
	cs2Root := t.TempDir()
	cs2Exe := filepath.Join(cs2Root, "cs2.exe")
	if err := os.WriteFile(cs2Exe, []byte("exe"), 0644); err != nil {
		t.Fatalf("write cs2 exe: %v", err)
	}

	gameInfoPath := filepath.Join(cs2Root, "game", "csgo", "gameinfo.gi")
	if err := os.MkdirAll(filepath.Dir(gameInfoPath), 0755); err != nil {
		t.Fatalf("mkdir gameinfo dir: %v", err)
	}
	content := "Game\tcsgo/plugin\nGame\tcsgo/pov.vpk\nGame\tcsgo\n"
	if err := os.WriteFile(gameInfoPath, []byte(content), 0644); err != nil {
		t.Fatalf("write gameinfo: %v", err)
	}

	cfg := config.Default(exeDir)
	cfg.CS2Exe = cs2Exe
	cfg.CS2Dir = cs2Root
	if err := config.Save(filepath.Join(exeDir, "config.json"), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	app := &App{exeDir: exeDir}
	if err := app.prepareGameInfoForProduce(); err != nil {
		t.Fatalf("prepare gameinfo: %v", err)
	}
	if _, err := os.Stat(gameInfoPath + produceGameInfoBackupSuffix); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup file, stat err=%v", err)
	}
	if err := app.forceRestoreGameInfoForProduce(); err != nil {
		t.Fatalf("restore should be idempotent: %v", err)
	}
}

func TestPrepareGameInfoForProduce_RecoversStaleBackupBeforeReinjecting(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	firstApp := &App{exeDir: env.exeDir}
	if err := firstApp.prepareGameInfoForProduce(); err != nil {
		t.Fatalf("first prepare gameinfo: %v", err)
	}
	backupPath := env.gameInfoPath + produceGameInfoBackupSuffix
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("expected stale backup, stat err=%v", err)
	}

	secondApp := &App{exeDir: env.exeDir}
	if err := secondApp.prepareGameInfoForProduce(); err != nil {
		t.Fatalf("second prepare gameinfo: %v", err)
	}
	content, err := os.ReadFile(env.gameInfoPath)
	if err != nil {
		t.Fatalf("read re-injected gameinfo: %v", err)
	}
	if !strings.Contains(string(content), "Game\tcsgo/plugin") {
		t.Fatalf("re-injected gameinfo missing plugin path:\n%s", content)
	}
	if err := secondApp.forceRestoreGameInfoForProduce(); err != nil {
		t.Fatalf("restore after stale backup recovery: %v", err)
	}
	restored, err := os.ReadFile(env.gameInfoPath)
	if err != nil {
		t.Fatalf("read restored gameinfo: %v", err)
	}
	if string(restored) != env.originalGameInfo {
		t.Fatalf("restored gameinfo mismatch:\n%s", restored)
	}
}

func TestPrepareAndRestorePluginDLLForProduce_NewTarget(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	app := &App{exeDir: env.exeDir}

	if err := app.preparePluginDLLForProduce(); err != nil {
		t.Fatalf("preparePluginDLLForProduce: %v", err)
	}
	targetPayload, err := os.ReadFile(env.targetDLLPath)
	if err != nil {
		t.Fatalf("read target dll: %v", err)
	}
	if string(targetPayload) != "plugin-new" {
		t.Fatalf("target dll payload mismatch: %q", string(targetPayload))
	}

	if err := app.forceRestorePluginDLLForProduce(); err != nil {
		t.Fatalf("forceRestorePluginDLLForProduce: %v", err)
	}
	if _, err := os.Stat(env.targetDLLPath); !os.IsNotExist(err) {
		t.Fatalf("target dll should be removed after restore, stat err=%v", err)
	}
	if _, err := os.Stat(env.binDirPath); !os.IsNotExist(err) {
		t.Fatalf("plugin bin dir should be removed when empty, stat err=%v", err)
	}
	if _, err := os.Stat(env.pluginDirPath); !os.IsNotExist(err) {
		t.Fatalf("plugin dir should be removed when empty, stat err=%v", err)
	}
}

func TestPrepareAndRestorePluginDLLForProduce_BackupExistingTarget(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	if err := os.MkdirAll(env.binDirPath, 0755); err != nil {
		t.Fatalf("mkdir plugin bin dir: %v", err)
	}
	if err := os.WriteFile(env.targetDLLPath, []byte("plugin-old"), 0644); err != nil {
		t.Fatalf("write existing target dll: %v", err)
	}

	app := &App{exeDir: env.exeDir}
	if err := app.preparePluginDLLForProduce(); err != nil {
		t.Fatalf("preparePluginDLLForProduce: %v", err)
	}

	targetPayload, err := os.ReadFile(env.targetDLLPath)
	if err != nil {
		t.Fatalf("read injected target dll: %v", err)
	}
	if string(targetPayload) != "plugin-new" {
		t.Fatalf("expected target to be overwritten by new plugin dll, got %q", string(targetPayload))
	}
	backupPath := env.targetDLLPath + producePluginDLLBackupSuffix
	backupPayload, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup dll: %v", err)
	}
	if string(backupPayload) != "plugin-old" {
		t.Fatalf("backup payload mismatch: %q", string(backupPayload))
	}

	if err := app.forceRestorePluginDLLForProduce(); err != nil {
		t.Fatalf("forceRestorePluginDLLForProduce: %v", err)
	}
	restoredPayload, err := os.ReadFile(env.targetDLLPath)
	if err != nil {
		t.Fatalf("read restored target dll: %v", err)
	}
	if string(restoredPayload) != "plugin-old" {
		t.Fatalf("restored payload mismatch: %q", string(restoredPayload))
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("backup path should be removed after restore, stat err=%v", err)
	}
}

func TestPreparePluginDLLForProduce_UsesDebugOverrideDLL(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	overridePath := filepath.Join(t.TempDir(), "debug-plugin.dll")
	if err := os.WriteFile(overridePath, []byte("plugin-debug"), 0644); err != nil {
		t.Fatalf("write debug plugin dll: %v", err)
	}

	app := &App{exeDir: env.exeDir}
	if _, err := app.setDebugPluginDLLOverride(overridePath); err != nil {
		t.Fatalf("setDebugPluginDLLOverride: %v", err)
	}
	if err := app.preparePluginDLLForProduce(); err != nil {
		t.Fatalf("preparePluginDLLForProduce: %v", err)
	}

	targetPayload, err := os.ReadFile(env.targetDLLPath)
	if err != nil {
		t.Fatalf("read target dll: %v", err)
	}
	if string(targetPayload) != "plugin-debug" {
		t.Fatalf("target should use debug plugin override, got %q", string(targetPayload))
	}
}

func TestPreparePluginDLLForProduce_ClearDebugOverrideUsesConfiguredDLL(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	overridePath := filepath.Join(t.TempDir(), "debug-plugin.dll")
	if err := os.WriteFile(overridePath, []byte("plugin-debug"), 0644); err != nil {
		t.Fatalf("write debug plugin dll: %v", err)
	}

	app := &App{exeDir: env.exeDir}
	if _, err := app.setDebugPluginDLLOverride(overridePath); err != nil {
		t.Fatalf("setDebugPluginDLLOverride: %v", err)
	}
	app.ClearDebugPluginDLLOverride()
	if err := app.preparePluginDLLForProduce(); err != nil {
		t.Fatalf("preparePluginDLLForProduce: %v", err)
	}

	targetPayload, err := os.ReadFile(env.targetDLLPath)
	if err != nil {
		t.Fatalf("read target dll: %v", err)
	}
	if string(targetPayload) != "plugin-new" {
		t.Fatalf("target should use configured plugin dll after clearing override, got %q", string(targetPayload))
	}
}

func TestForceRestoreProduceEnvironmentForProduce_RestoresGameInfoAndPluginDLL(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	app := &App{exeDir: env.exeDir}

	if err := app.prepareGameInfoForProduce(); err != nil {
		t.Fatalf("prepareGameInfoForProduce: %v", err)
	}
	if err := app.preparePluginDLLForProduce(); err != nil {
		t.Fatalf("preparePluginDLLForProduce: %v", err)
	}

	injectedGameInfo, err := os.ReadFile(env.gameInfoPath)
	if err != nil {
		t.Fatalf("read injected gameinfo: %v", err)
	}
	if !strings.Contains(string(injectedGameInfo), "Game\tcsgo/plugin") {
		t.Fatalf("gameinfo should contain plugin search path after prepare")
	}
	if _, err := os.Stat(env.targetDLLPath); err != nil {
		t.Fatalf("target dll should exist after prepare, stat err=%v", err)
	}

	if err := app.forceRestoreProduceEnvironmentForProduce(); err != nil {
		t.Fatalf("forceRestoreProduceEnvironmentForProduce: %v", err)
	}

	restoredGameInfo, err := os.ReadFile(env.gameInfoPath)
	if err != nil {
		t.Fatalf("read restored gameinfo: %v", err)
	}
	if string(restoredGameInfo) != env.originalGameInfo {
		t.Fatalf("gameinfo not restored, got:\n%s", string(restoredGameInfo))
	}
	if _, err := os.Stat(env.gameInfoPath + produceGameInfoBackupSuffix); !os.IsNotExist(err) {
		t.Fatalf("gameinfo backup should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(env.targetDLLPath); !os.IsNotExist(err) {
		t.Fatalf("target dll should be removed after restore, stat err=%v", err)
	}
}

func TestGetGameInfoHealthAndRepairGameInfoWithoutBackup(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	app := &App{exeDir: env.exeDir, dataDir: env.exeDir}
	stale := strings.Replace(env.originalGameInfo, "Game\tcsgo", "Game\tcsgo/plugin\n\t\tGame\tcsgo", 1)
	if err := os.WriteFile(env.gameInfoPath, []byte(stale), 0644); err != nil {
		t.Fatalf("write stale gameinfo: %v", err)
	}

	health, err := app.GetGameInfoHealth()
	if err != nil {
		t.Fatalf("GetGameInfoHealth: %v", err)
	}
	if health.Status != "needs_repair" {
		t.Fatalf("health status = %q, want needs_repair", health.Status)
	}
	if !health.NeedsRepair {
		t.Fatal("expected NeedsRepair=true")
	}
	if health.GameInfoPath != env.gameInfoPath {
		t.Fatalf("gameinfo path = %q, want %q", health.GameInfoPath, env.gameInfoPath)
	}

	repaired, err := app.RepairGameInfo()
	if err != nil {
		t.Fatalf("RepairGameInfo: %v", err)
	}
	if repaired.Status != "ok" {
		t.Fatalf("repaired status = %q, want ok", repaired.Status)
	}
	if repaired.NeedsRepair {
		t.Fatal("expected NeedsRepair=false after repair")
	}
	contentBytes, err := os.ReadFile(env.gameInfoPath)
	if err != nil {
		t.Fatalf("read repaired gameinfo: %v", err)
	}
	content := string(contentBytes)
	if strings.Contains(content, "Game\tcsgo/plugin") {
		t.Fatalf("plugin search path should be removed:\n%s", content)
	}
	if !strings.Contains(content, "Game\tcsgo") {
		t.Fatalf("original csgo search path should remain:\n%s", content)
	}
}

func TestRepairGameInfoNoopWhenHealthy(t *testing.T) {
	env := setupProducePluginTestEnvironment(t)
	app := &App{exeDir: env.exeDir, dataDir: env.exeDir}

	health, err := app.RepairGameInfo()
	if err != nil {
		t.Fatalf("RepairGameInfo: %v", err)
	}
	if health.Status != "ok" {
		t.Fatalf("health status = %q, want ok", health.Status)
	}
	if health.NeedsRepair {
		t.Fatal("expected healthy gameinfo to need no repair")
	}
	contentBytes, err := os.ReadFile(env.gameInfoPath)
	if err != nil {
		t.Fatalf("read gameinfo: %v", err)
	}
	if string(contentBytes) != env.originalGameInfo {
		t.Fatalf("healthy gameinfo should be unchanged:\n%s", string(contentBytes))
	}
}

func TestGetGameInfoHealth_UninitializedDoesNotCreateConfig(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	health, err := app.GetGameInfoHealth()
	if err != nil {
		t.Fatalf("GetGameInfoHealth: %v", err)
	}
	if health.Status != "unknown" {
		t.Fatalf("health status = %q, want unknown", health.Status)
	}
	if health.NeedsRepair {
		t.Fatal("uninitialized workspace should not need repair")
	}
	if _, err := os.Stat(filepath.Join(exeDir, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("GetGameInfoHealth should not create config.json before workspace init, stat err=%v", err)
	}
}

func TestGeneratePluginJSONBatchAndLaunchHLAE_PluginPrepareFailureRollsBackGameInfo(t *testing.T) {
	exeDir := t.TempDir()
	cs2Root := t.TempDir()
	cs2Exe := filepath.Join(cs2Root, "game", "bin", "win64", "cs2.exe")
	if err := os.MkdirAll(filepath.Dir(cs2Exe), 0755); err != nil {
		t.Fatalf("mkdir cs2 exe dir: %v", err)
	}
	if err := os.WriteFile(cs2Exe, []byte("exe"), 0644); err != nil {
		t.Fatalf("write cs2 exe: %v", err)
	}

	gameInfoPath := filepath.Join(cs2Root, "game", "csgo", "gameinfo.gi")
	if err := os.MkdirAll(filepath.Dir(gameInfoPath), 0755); err != nil {
		t.Fatalf("mkdir gameinfo dir: %v", err)
	}
	original := "FileSystem\n{\n\tSearchPaths\n\t{\n\t\tGame\tcsgo\n\t}\n}\n"
	if err := os.WriteFile(gameInfoPath, []byte(original), 0644); err != nil {
		t.Fatalf("write gameinfo: %v", err)
	}

	cfg := config.Default(exeDir)
	cfg.CS2Dir = cs2Root
	cfg.CS2Exe = cs2Exe
	cfg.PluginDLL = filepath.Join(exeDir, "plugin", "missing.dll")
	if err := config.Save(filepath.Join(exeDir, "config.json"), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	demoPath := writeProduceDemoFile(t)
	app := &App{exeDir: exeDir}
	result, err := app.GeneratePluginJSONBatchAndLaunchHLAE(GeneratePluginJSONBatchRequest{
		Jobs: []GeneratePluginJSONRequest{
			{
				DemoPath: demoPath,
				TickRate: 64,
				SelectedItems: []SelectedClipItem{{
					Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
					IncludeVictim: false,
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSONBatchAndLaunchHLAE: %v", err)
	}
	if result.LaunchStarted {
		t.Fatalf("launch should fail when plugin dll is missing: %+v", result)
	}
	if !strings.Contains(result.LaunchError, "插件 DLL 不存在") {
		t.Fatalf("unexpected launch error: %q", result.LaunchError)
	}

	restoredGameInfo, err := os.ReadFile(gameInfoPath)
	if err != nil {
		t.Fatalf("read restored gameinfo: %v", err)
	}
	if string(restoredGameInfo) != original {
		t.Fatalf("gameinfo should be rolled back after plugin prepare failure, got:\n%s", string(restoredGameInfo))
	}
	if _, err := os.Stat(gameInfoPath + produceGameInfoBackupSuffix); !os.IsNotExist(err) {
		t.Fatalf("gameinfo backup should be cleaned after rollback, stat err=%v", err)
	}
}

func TestMergeTakeVideoAudio_SuccessKeepsSourceFiles(t *testing.T) {
	old := producemerge.FFmpegCommand
	producemerge.FFmpegCommand = fakeFFmpegCommandSuccess
	t.Cleanup(func() {
		producemerge.FFmpegCommand = old
	})

	dir := t.TempDir()
	videoPath := filepath.Join(dir, "take0001.mp4")
	if err := os.WriteFile(videoPath, []byte("video"), 0644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	audioDir := filepath.Join(dir, "take0001")
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		t.Fatalf("mkdir audio dir: %v", err)
	}
	audioPath := filepath.Join(audioDir, "audio.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	ffmpegExe := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}

	finalVideoPath, err := mergeTakeVideoAudio(ffmpegExe, videoPath, audioPath)
	if err != nil {
		t.Fatalf("mergeTakeVideoAudio: %v", err)
	}
	if finalVideoPath == videoPath {
		t.Fatalf("final video path should be renamed, got %q", finalVideoPath)
	}
	matched, matchErr := regexp.MatchString(`^\d{6}(_\d+)?\.mp4$`, filepath.Base(finalVideoPath))
	if matchErr != nil || !matched {
		t.Fatalf("unexpected final filename: %q", finalVideoPath)
	}
	merged, err := os.ReadFile(finalVideoPath)
	if err != nil {
		t.Fatalf("read merged video: %v", err)
	}
	if string(merged) != "muxed" {
		t.Fatalf("unexpected merged payload: %q", string(merged))
	}
	if _, err := os.Stat(videoPath); err != nil {
		t.Fatalf("source video should remain, stat err=%v", err)
	}
	if _, err := os.Stat(audioPath); err != nil {
		t.Fatalf("audio should remain, stat err=%v", err)
	}
	if _, err := os.Stat(audioDir); err != nil {
		t.Fatalf("audio dir should remain, stat err=%v", err)
	}
}

func TestMergeTakeVideoAudio_FailureKeepsSourceFiles(t *testing.T) {
	old := producemerge.FFmpegCommand
	producemerge.FFmpegCommand = fakeFFmpegCommandFail
	t.Cleanup(func() {
		producemerge.FFmpegCommand = old
	})

	dir := t.TempDir()
	videoPath := filepath.Join(dir, "take0002.mp4")
	if err := os.WriteFile(videoPath, []byte("video"), 0644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	audioDir := filepath.Join(dir, "take0002")
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		t.Fatalf("mkdir audio dir: %v", err)
	}
	audioPath := filepath.Join(audioDir, "audio.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	ffmpegExe := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}

	_, err := mergeTakeVideoAudio(ffmpegExe, videoPath, audioPath)
	if err == nil {
		t.Fatalf("expected merge error")
	}
	if _, statErr := os.Stat(videoPath); statErr != nil {
		t.Fatalf("video should remain on failure: %v", statErr)
	}
	if _, statErr := os.Stat(audioPath); statErr != nil {
		t.Fatalf("audio should remain on failure: %v", statErr)
	}
}

func TestNextMergedVideoPath_AppendsNumericSuffixWhenConflicted(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 2, 12, 34, 56, 0, time.Local)
	if err := os.WriteFile(filepath.Join(dir, "123456.mp4"), []byte("x"), 0644); err != nil {
		t.Fatalf("write base file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "123456_01.mp4"), []byte("x"), 0644); err != nil {
		t.Fatalf("write suffix file: %v", err)
	}
	path, err := nextMergedVideoPath(dir, now)
	if err != nil {
		t.Fatalf("nextMergedVideoPath: %v", err)
	}
	if !strings.HasSuffix(path, "123456_02.mp4") {
		t.Fatalf("unexpected path: %q", path)
	}
}

func TestWaitForTakeFilesReady_SuccessWhenFilesStabilize(t *testing.T) {
	oldCtx := producemerge.FFmpegCommandContext
	producemerge.FFmpegCommandContext = fakeFFmpegCommandSuccessContext
	t.Cleanup(func() {
		producemerge.FFmpegCommandContext = oldCtx
	})

	dir := t.TempDir()
	ffmpegExe := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}
	videoPath := filepath.Join(dir, "take0003.mp4")
	audioDir := filepath.Join(dir, "take0003")
	audioPath := filepath.Join(audioDir, "audio.wav")
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		t.Fatalf("mkdir audio dir: %v", err)
	}

	go func() {
		time.Sleep(120 * time.Millisecond)
		_ = os.WriteFile(videoPath, []byte("v1"), 0644)
		_ = os.WriteFile(audioPath, []byte("a1"), 0644)
		time.Sleep(140 * time.Millisecond)
		_ = os.WriteFile(videoPath, []byte("v1-extend"), 0644)
		_ = os.WriteFile(audioPath, []byte("a1-extend"), 0644)
	}()

	err := waitForTakeFilesReady(context.Background(), ffmpegExe, videoPath, audioPath, 2*time.Second, 80*time.Millisecond)
	if err != nil {
		t.Fatalf("waitForTakeFilesReady: %v", err)
	}
}

func TestWaitForTakeFilesReady_Timeout(t *testing.T) {
	dir := t.TempDir()
	ffmpegExe := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}
	videoPath := filepath.Join(dir, "take0004.mp4")
	audioPath := filepath.Join(dir, "take0004", "audio.wav")
	if err := os.WriteFile(videoPath, []byte("video"), 0644); err != nil {
		t.Fatalf("write video: %v", err)
	}

	err := waitForTakeFilesReady(context.Background(), ffmpegExe, videoPath, audioPath, 300*time.Millisecond, 60*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitForTakeFilesReady_TimeoutWhenProbeFails(t *testing.T) {
	oldCtx := producemerge.FFmpegCommandContext
	producemerge.FFmpegCommandContext = fakeFFmpegCommandFailContext
	t.Cleanup(func() {
		producemerge.FFmpegCommandContext = oldCtx
	})

	dir := t.TempDir()
	ffmpegExe := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(ffmpegExe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}
	videoPath := filepath.Join(dir, "take0005.mp4")
	audioDir := filepath.Join(dir, "take0005")
	audioPath := filepath.Join(audioDir, "audio.wav")
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		t.Fatalf("mkdir audio dir: %v", err)
	}
	if err := os.WriteFile(videoPath, []byte("video"), 0644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	err := waitForTakeFilesReady(context.Background(), ffmpegExe, videoPath, audioPath, 500*time.Millisecond, 80*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !strings.Contains(err.Error(), "ffmpeg probe") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitForTakeFilesReady_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitForTakeFilesReady(ctx, "ffmpeg.exe", "video.mp4", "audio.wav", time.Second, 50*time.Millisecond)
	if err == nil {
		t.Fatalf("expected cancelled error")
	}
	if !strings.Contains(err.Error(), "已取消") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDispatchMergeTasks_ResolvesPathsByDemoSubdir(t *testing.T) {
	app := &App{
		produceState: produceSessionState{
			takeFiles: make(map[string]ProduceTakeFile),
		},
	}
	state := &produceSessionRuntime{
		batchDir: "D:/outputs/20260101_120000",
		demoSubDirs: map[string]string{
			"demoA.dem": "demo_a",
			"demoB.dem": "demo_b",
		},
		taskCh: make(chan mergeTask, 4),
		endedQueue: []pendingCompletedTake{
			{plan: ProduceTakePlan{DemoPath: "demoA.dem", TakeIndex: 1, View: "killer"}},
			{plan: ProduceTakePlan{DemoPath: "demoB.dem", TakeIndex: 2, TakeName: "take0002", View: "victim"}},
		},
	}

	app.dispatchMergeTasks(state)

	if state.pendingTaskCnt.Load() != 2 {
		t.Fatalf("pending tasks=%d want 2", state.pendingTaskCnt.Load())
	}
	if len(state.endedQueue) != 0 {
		t.Fatalf("queue should be drained, ended=%d", len(state.endedQueue))
	}

	task1 := <-state.taskCh
	task2 := <-state.taskCh
	if task1.plan.TakeIndex != 1 || !strings.HasSuffix(strings.ToLower(task1.videoPath), "demo_a/take0000.mp4") {
		t.Fatalf("unexpected first pair: %+v", task1)
	}
	if !strings.HasSuffix(strings.ToLower(task1.audioPath), "demo_a/take0000/audio.wav") {
		t.Fatalf("unexpected first audio path: %+v", task1)
	}
	if task2.plan.TakeIndex != 2 || !strings.HasSuffix(strings.ToLower(task2.videoPath), "demo_b/take0002.mp4") {
		t.Fatalf("unexpected second pair: %+v", task2)
	}
	if !strings.HasSuffix(strings.ToLower(task2.audioPath), "demo_b/take0002/audio.wav") {
		t.Fatalf("unexpected second audio path: %+v", task2)
	}
}

func TestDispatchMergeTasks_ChannelFullKeepsTaskForRetry(t *testing.T) {
	app := &App{
		produceState: produceSessionState{
			takeFiles: make(map[string]ProduceTakeFile),
		},
	}
	plan := ProduceTakePlan{DemoPath: "demoA.dem", TakeIndex: 1, View: "killer"}
	app.updateTakeFileEntry(plan, func(file *ProduceTakeFile) {
		file.Status = "recorded"
		file.Error = ""
		file.UpdatedAtMs = nowMs()
	})
	state := &produceSessionRuntime{
		batchDir: "D:/outputs/20260101_120000",
		demoSubDirs: map[string]string{
			"demoA.dem": "demo_a",
		},
		taskCh: make(chan mergeTask, 1),
		endedQueue: []pendingCompletedTake{
			{plan: plan},
		},
	}
	state.taskCh <- mergeTask{
		plan:      ProduceTakePlan{DemoPath: "busy.dem", TakeIndex: 99, View: "killer"},
		videoPath: "busy.mp4",
		audioPath: "busy.wav",
	}

	app.dispatchMergeTasks(state)

	if state.pendingTaskCnt.Load() != 0 {
		t.Fatalf("pending tasks=%d want 0", state.pendingTaskCnt.Load())
	}
	if len(state.endedQueue) != 1 {
		t.Fatalf("ended queue should remain for retry, ended=%d", len(state.endedQueue))
	}

	snapshot := app.GetProduceTakeFiles()
	if len(snapshot.Items) != 1 {
		t.Fatalf("take file items=%d want 1", len(snapshot.Items))
	}
	item := snapshot.Items[0]
	if item.Status != "recorded" {
		t.Fatalf("take file status=%q want recorded", item.Status)
	}
	if item.Error != "" {
		t.Fatalf("take file error should be empty, got %q", item.Error)
	}
	if item.VideoPath != "" || item.AudioPath != "" {
		t.Fatalf("video/audio path should stay empty before task enqueued, got video=%q audio=%q", item.VideoPath, item.AudioPath)
	}
}

func TestCleanupProduceTemporaryFiles_RemovesOnlyIntermediateArtifacts(t *testing.T) {
	batchDir := t.TempDir()
	demoDir := filepath.Join(batchDir, "demo_a")
	takeVideo := filepath.Join(demoDir, "take0000.mp4")
	takeDir := filepath.Join(demoDir, "take0000")
	takeAudio := filepath.Join(takeDir, "audio.wav")
	finalVideo := filepath.Join(demoDir, "120000.mp4")
	tmpMux := filepath.Join(demoDir, "120000_01.mux.tmp.mp4")

	if err := os.MkdirAll(takeDir, 0755); err != nil {
		t.Fatalf("mkdir take dir: %v", err)
	}
	if err := os.WriteFile(takeVideo, []byte("take-video"), 0644); err != nil {
		t.Fatalf("write take video: %v", err)
	}
	if err := os.WriteFile(takeAudio, []byte("take-audio"), 0644); err != nil {
		t.Fatalf("write take audio: %v", err)
	}
	if err := os.WriteFile(finalVideo, []byte("final-video"), 0644); err != nil {
		t.Fatalf("write final video: %v", err)
	}
	if err := os.WriteFile(tmpMux, []byte("tmp-video"), 0644); err != nil {
		t.Fatalf("write tmp mux: %v", err)
	}

	app := &App{}
	state := &produceSessionRuntime{
		batchDir:    batchDir,
		demoSubDirs: map[string]string{"demoA.dem": "demo_a"},
		plansByTake: map[string]ProduceTakePlan{
			"demoA.dem#1": {
				DemoPath:  "demoA.dem",
				TakeIndex: 1,
				TakeName:  "take0000",
				View:      "killer",
			},
		},
	}

	app.cleanupProduceTemporaryFiles(state)

	if _, err := os.Stat(finalVideo); err != nil {
		t.Fatalf("final video should be kept, stat err=%v", err)
	}
	if _, err := os.Stat(takeVideo); !os.IsNotExist(err) {
		t.Fatalf("take video should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(takeAudio); !os.IsNotExist(err) {
		t.Fatalf("take audio should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(tmpMux); !os.IsNotExist(err) {
		t.Fatalf("tmp mux file should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(takeDir); !os.IsNotExist(err) {
		t.Fatalf("take dir should be removed when empty, stat err=%v", err)
	}
}

func TestCleanupProduceTemporaryFiles_PreservesFailedTakeArtifacts(t *testing.T) {
	batchDir := t.TempDir()
	demoDir := filepath.Join(batchDir, "demo_a")
	takeVideo := filepath.Join(demoDir, "take0000.mp4")
	takeAudio := filepath.Join(demoDir, "take0000", "audio.wav")
	if err := os.MkdirAll(filepath.Dir(takeAudio), 0755); err != nil {
		t.Fatalf("mkdir take dir: %v", err)
	}
	if err := os.WriteFile(takeVideo, []byte("take-video"), 0644); err != nil {
		t.Fatalf("write take video: %v", err)
	}
	if err := os.WriteFile(takeAudio, []byte("take-audio"), 0644); err != nil {
		t.Fatalf("write take audio: %v", err)
	}

	plan := ProduceTakePlan{
		DemoPath:  "demoA.dem",
		TakeIndex: 1,
		TakeName:  "take0000",
		View:      "killer",
	}
	app := &App{
		produceState: produceSessionState{
			takeFiles: map[string]ProduceTakeFile{
				takeFileKey(plan.DemoPath, plan.TakeIndex, plan.View): {
					DemoPath:  plan.DemoPath,
					TakeIndex: plan.TakeIndex,
					View:      plan.View,
					Status:    "failed",
				},
			},
		},
	}
	state := &produceSessionRuntime{
		batchDir:    batchDir,
		demoSubDirs: map[string]string{"demoA.dem": "demo_a"},
		plansByTake: map[string]ProduceTakePlan{"demoA.dem#1": plan},
	}

	app.cleanupProduceTemporaryFiles(state)

	if _, err := os.Stat(takeVideo); err != nil {
		t.Fatalf("failed take video should be preserved, stat err=%v", err)
	}
	if _, err := os.Stat(takeAudio); err != nil {
		t.Fatalf("failed take audio should be preserved, stat err=%v", err)
	}
}

func TestCleanupProduceTemporaryFiles_KeepIntermediateFiles(t *testing.T) {
	batchDir := t.TempDir()
	demoDir := filepath.Join(batchDir, "demo_a")
	takeVideo := filepath.Join(demoDir, "take0000.mp4")
	takeDir := filepath.Join(demoDir, "take0000")
	takeAudio := filepath.Join(takeDir, "audio.wav")
	finalVideo := filepath.Join(demoDir, "120000.mp4")
	tmpMux := filepath.Join(demoDir, "120000_01.mux.tmp.mp4")

	if err := os.MkdirAll(takeDir, 0755); err != nil {
		t.Fatalf("mkdir take dir: %v", err)
	}
	if err := os.WriteFile(takeVideo, []byte("take-video"), 0644); err != nil {
		t.Fatalf("write take video: %v", err)
	}
	if err := os.WriteFile(takeAudio, []byte("take-audio"), 0644); err != nil {
		t.Fatalf("write take audio: %v", err)
	}
	if err := os.WriteFile(finalVideo, []byte("final-video"), 0644); err != nil {
		t.Fatalf("write final video: %v", err)
	}
	if err := os.WriteFile(tmpMux, []byte("tmp-video"), 0644); err != nil {
		t.Fatalf("write tmp mux: %v", err)
	}

	app := &App{}
	state := &produceSessionRuntime{
		batchDir:              batchDir,
		demoSubDirs:           map[string]string{"demoA.dem": "demo_a"},
		keepIntermediateFiles: true,
		plansByTake: map[string]ProduceTakePlan{
			"demoA.dem#1": {
				DemoPath:  "demoA.dem",
				TakeIndex: 1,
				TakeName:  "take0000",
				View:      "killer",
			},
		},
	}

	app.cleanupProduceTemporaryFiles(state)

	if _, err := os.Stat(finalVideo); err != nil {
		t.Fatalf("final video should be kept, stat err=%v", err)
	}
	if _, err := os.Stat(takeVideo); err != nil {
		t.Fatalf("take video should be kept, stat err=%v", err)
	}
	if _, err := os.Stat(takeAudio); err != nil {
		t.Fatalf("take audio should be kept, stat err=%v", err)
	}
	if _, err := os.Stat(tmpMux); !os.IsNotExist(err) {
		t.Fatalf("tmp mux file should be removed, stat err=%v", err)
	}
}

func TestRequestCloseCS2Process_InvokesCloserOnce(t *testing.T) {
	old := closeCS2ProcessByPIDFn
	calls := 0
	gotPID := 0
	closeCS2ProcessByPIDFn = func(pid int) error {
		calls++
		gotPID = pid
		return nil
	}
	t.Cleanup(func() {
		closeCS2ProcessByPIDFn = old
	})

	app := &App{}
	state := &produceSessionRuntime{cs2PID: 9527, queueSucceeded: true}
	app.requestCloseCS2Process(state)
	app.requestCloseCS2Process(state)

	if !state.closeRequested || !state.closeDone {
		t.Fatalf("close state should be marked done: %+v", state)
	}
	if calls != 1 || gotPID != 9527 {
		t.Fatalf("close calls mismatch: calls=%d pid=%d", calls, gotPID)
	}
}

func TestRequestCloseCS2Process_UsesPluginGracefulExitBeforePIDFallback(t *testing.T) {
	svc := producews.New("127.0.0.1:0", nil)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start produce websocket: %v", err)
	}
	defer svc.Stop()

	u := url.URL{Scheme: "ws", Host: svc.Address(), Path: "/", RawQuery: "process=game"}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("connect plugin client: %v", err)
	}
	defer conn.Close()

	old := closeCS2ProcessByPIDFn
	closeCalls := 0
	closeCS2ProcessByPIDFn = func(int) error {
		closeCalls++
		return nil
	}
	t.Cleanup(func() { closeCS2ProcessByPIDFn = old })

	app := &App{produceW: svc}
	state := &produceSessionRuntime{cs2PID: 9527, queueSucceeded: true}
	app.requestCloseCS2Process(state)
	if !state.closeRequested || state.closeDone || !state.gracefulExit {
		t.Fatalf("unexpected close state after graceful request: %+v", state)
	}
	if closeCalls != 0 {
		t.Fatalf("PID fallback called before plugin acknowledgement: %d", closeCalls)
	}

	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	var command struct {
		Name    string `json:"name"`
		Payload struct {
			RequestID string `json:"request_id"`
		} `json:"payload"`
	}
	if err := conn.ReadJSON(&command); err != nil {
		t.Fatalf("read graceful exit command: %v", err)
	}
	if command.Name != "end_produce_session" || command.Payload.RequestID == "" {
		t.Fatalf("unexpected graceful exit command: %+v", command)
	}
	if err := conn.WriteJSON(map[string]any{
		"name": "session_exit_ack",
		"payload": map[string]any{
			"request_id": command.Payload.RequestID,
		},
	}); err != nil {
		t.Fatalf("write graceful exit acknowledgement: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for !svc.GracefulExitStatus().Acknowledged && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !svc.GracefulExitStatus().Acknowledged {
		t.Fatal("service did not accept graceful exit acknowledgement")
	}
	_ = conn.Close()
	deadline = time.Now().Add(time.Second)
	for !svc.GracefulExitStatus().Completed && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !svc.GracefulExitStatus().Completed {
		t.Fatal("service did not classify acknowledged close as completed")
	}

	app.advanceCloseCS2Process(state, time.Now())
	if !state.closeDone || closeCalls != 0 {
		t.Fatalf("completed graceful close should skip PID fallback: state=%+v calls=%d", state, closeCalls)
	}
}

func TestAdvanceCloseCS2Process_FallsBackAfterGracefulExitTimeout(t *testing.T) {
	old := closeCS2ProcessByPIDFn
	closeCalls := 0
	closeCS2ProcessByPIDFn = func(pid int) error {
		closeCalls++
		if pid != 9527 {
			t.Fatalf("pid=%d want 9527", pid)
		}
		return nil
	}
	t.Cleanup(func() { closeCS2ProcessByPIDFn = old })

	app := &App{}
	state := &produceSessionRuntime{
		cs2PID:         9527,
		closeRequested: true,
		gracefulExit:   true,
		gracefulExitAt: time.Now().Add(-time.Millisecond),
	}
	app.advanceCloseCS2Process(state, time.Now())
	if !state.closeDone || state.gracefulExit || closeCalls != 1 {
		t.Fatalf("graceful timeout should use PID fallback once: state=%+v calls=%d", state, closeCalls)
	}
}

func TestRequestCloseCS2Process_InvalidPIDSkipsClose(t *testing.T) {
	old := closeCS2ProcessByPIDFn
	calls := 0
	closeCS2ProcessByPIDFn = func(pid int) error {
		calls++
		return nil
	}
	t.Cleanup(func() {
		closeCS2ProcessByPIDFn = old
	})

	app := &App{}
	state := &produceSessionRuntime{cs2PID: 0}
	app.requestCloseCS2Process(state)
	if !state.closeRequested || !state.closeDone {
		t.Fatalf("close state should be marked done for invalid pid: %+v", state)
	}
	if calls != 0 {
		t.Fatalf("closer should not be called for invalid pid, calls=%d", calls)
	}
}

func TestCanStopProduceSession_RequiresQueueStoppedCloseDoneAndDrained(t *testing.T) {
	app := &App{}
	state := &produceSessionRuntime{}

	if app.canStopProduceSession(state) {
		t.Fatalf("should not stop when queue is not stopped")
	}
	state.queueStopped = true
	if app.canStopProduceSession(state) {
		t.Fatalf("should not stop when close is not done")
	}
	state.closeDone = true
	state.pendingTaskCnt.Add(1)
	if app.canStopProduceSession(state) {
		t.Fatalf("should not stop when merge tasks are still pending")
	}
	state.pendingTaskCnt.Add(-1)
	if !app.canStopProduceSession(state) {
		t.Fatalf("should stop when queue is stopped, close is done, and work is drained")
	}
	state.endedQueue = append(state.endedQueue, pendingCompletedTake{
		plan: ProduceTakePlan{DemoPath: "demo.dem", TakeIndex: 1},
	})
	if app.canStopProduceSession(state) {
		t.Fatalf("should not stop when there are queued completed takes")
	}
}

func TestAddProduceHistoryEntry_UsesRuntimeKillSnapshot(t *testing.T) {
	app := &App{
		produceState: produceSessionState{
			historyItems:    make(map[string]ProduceHistoryItem),
			historyKeyIndex: make(map[string]struct{}),
		},
	}
	state := &produceSessionRuntime{
		killsByDemo: map[string]map[string]demo.ClipKill{
			"demoA.dem": {
				"k1": {ID: "k1", Round: 5, KillerName: "A", VictimName: "B", WeaponName: "ak47"},
				"k2": {ID: "k2", Round: 9, KillerName: "C", VictimName: "D", WeaponName: "awp"},
			},
		},
	}
	plan := ProduceTakePlan{
		DemoPath:  "demoA.dem",
		TakeIndex: 1,
		TakeName:  "take0000",
		View:      "killer",
		SpecMode:  1,
		KillIDs:   []string{"k1", "k2"},
	}

	app.addProduceHistoryEntry(state, plan, "D:/clips/120000.mp4")
	snapshot := app.getProduceHistorySnapshot()
	if len(snapshot.Items) != 1 {
		t.Fatalf("history items=%d want 1", len(snapshot.Items))
	}
	item := snapshot.Items[0]
	if len(item.Kills) != 2 {
		t.Fatalf("history kills=%d want 2", len(item.Kills))
	}
	if item.Kills[0].ID != "k1" || item.Kills[0].Round != 5 {
		t.Fatalf("unexpected first kill: %+v", item.Kills[0])
	}
	if item.Kills[1].ID != "k2" || item.Kills[1].Round != 9 {
		t.Fatalf("unexpected second kill: %+v", item.Kills[1])
	}
}

func TestExportProduceHistoryVideos_Success(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	srcA := filepath.Join(sourceDir, "clip-a.mp4")
	srcB := filepath.Join(sourceDir, "clip-b.mp4")
	if err := os.WriteFile(srcA, []byte("A"), 0644); err != nil {
		t.Fatalf("write srcA: %v", err)
	}
	if err := os.WriteFile(srcB, []byte("B"), 0644); err != nil {
		t.Fatalf("write srcB: %v", err)
	}

	app := &App{
		produceState: produceSessionState{
			historyItems:    make(map[string]ProduceHistoryItem),
			historyOrder:    make([]string, 0, 2),
			historyKeyIndex: make(map[string]struct{}),
		},
	}

	itemA := ProduceHistoryItem{
		DemoPath:  "demo-a.dem",
		TakeIndex: 1,
		View:      "killer",
		SpecMode:  1,
		KillIDs:   []string{"k1"},
		VideoPath: srcA,
	}
	itemB := ProduceHistoryItem{
		DemoPath:  "demo-b.dem",
		TakeIndex: 2,
		View:      "victim",
		SpecMode:  1,
		KillIDs:   []string{"k2"},
		VideoPath: srcB,
	}
	keyA := plugingen.BuildProduceHistoryKey(itemA.DemoPath, itemA.View, itemA.SpecMode, itemA.KillIDs)
	keyB := plugingen.BuildProduceHistoryKey(itemB.DemoPath, itemB.View, itemB.SpecMode, itemB.KillIDs)
	app.produceState.historyItems[keyA] = itemA
	app.produceState.historyItems[keyB] = itemB
	app.produceState.historyOrder = append(app.produceState.historyOrder, keyA, keyB)
	app.produceState.historyKeyIndex[keyA] = struct{}{}
	app.produceState.historyKeyIndex[keyB] = struct{}{}

	old := openDirectoryDialog
	openDirectoryDialog = func(_ context.Context, _ wailsruntime.OpenDialogOptions) (string, error) {
		return targetDir, nil
	}
	t.Cleanup(func() { openDirectoryDialog = old })

	result, err := app.ExportProduceHistoryVideos()
	if err != nil {
		t.Fatalf("ExportProduceHistoryVideos: %v", err)
	}
	if result.Cancelled {
		t.Fatalf("result should not be cancelled")
	}
	if result.Total != 2 || result.Moved != 2 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	destA := filepath.Join(targetDir, "clip-a.mp4")
	destB := filepath.Join(targetDir, "clip-b.mp4")
	if _, err := os.Stat(destA); err != nil {
		t.Fatalf("destA should exist: %v", err)
	}
	if _, err := os.Stat(destB); err != nil {
		t.Fatalf("destB should exist: %v", err)
	}
	if _, err := os.Stat(srcA); err != nil {
		t.Fatalf("srcA should remain after copy, stat err=%v", err)
	}
	if _, err := os.Stat(srcB); err != nil {
		t.Fatalf("srcB should remain after copy, stat err=%v", err)
	}

	snapshot := app.getProduceHistorySnapshot()
	paths := map[string]string{}
	for _, item := range snapshot.Items {
		paths[item.DemoPath] = item.VideoPath
	}
	if paths["demo-a.dem"] != srcA || paths["demo-b.dem"] != srcB {
		t.Fatalf("history video paths should remain source paths: %+v", paths)
	}
}

func TestExportProduceHistoryVideos_OverwriteSameName(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	src := filepath.Join(sourceDir, "clip.mp4")
	dest := filepath.Join(targetDir, "clip.mp4")
	if err := os.WriteFile(src, []byte("new-content"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(dest, []byte("old-content"), 0644); err != nil {
		t.Fatalf("write existing target: %v", err)
	}

	app := &App{
		produceState: produceSessionState{
			historyItems:    make(map[string]ProduceHistoryItem),
			historyOrder:    make([]string, 0, 1),
			historyKeyIndex: make(map[string]struct{}),
		},
	}
	item := ProduceHistoryItem{
		DemoPath:  "demo.dem",
		TakeIndex: 1,
		View:      "killer",
		SpecMode:  1,
		KillIDs:   []string{"k1"},
		VideoPath: src,
	}
	key := plugingen.BuildProduceHistoryKey(item.DemoPath, item.View, item.SpecMode, item.KillIDs)
	app.produceState.historyItems[key] = item
	app.produceState.historyOrder = append(app.produceState.historyOrder, key)
	app.produceState.historyKeyIndex[key] = struct{}{}

	old := openDirectoryDialog
	openDirectoryDialog = func(_ context.Context, _ wailsruntime.OpenDialogOptions) (string, error) {
		return targetDir, nil
	}
	t.Cleanup(func() { openDirectoryDialog = old })

	result, err := app.ExportProduceHistoryVideos()
	if err != nil {
		t.Fatalf("ExportProduceHistoryVideos: %v", err)
	}
	if result.Moved != 1 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	content, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(content) != "new-content" {
		t.Fatalf("target not overwritten, got=%q", string(content))
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should remain after copy: %v", err)
	}
}

func TestExportProduceHistoryVideos_PartialFailure(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	srcOK := filepath.Join(sourceDir, "ok.mp4")
	missing := filepath.Join(sourceDir, "missing.mp4")
	if err := os.WriteFile(srcOK, []byte("ok"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	app := &App{
		produceState: produceSessionState{
			historyItems:    make(map[string]ProduceHistoryItem),
			historyOrder:    make([]string, 0, 2),
			historyKeyIndex: make(map[string]struct{}),
		},
	}
	itemOK := ProduceHistoryItem{
		DemoPath:  "demo-ok.dem",
		TakeIndex: 1,
		View:      "killer",
		SpecMode:  1,
		KillIDs:   []string{"k1"},
		VideoPath: srcOK,
	}
	itemMissing := ProduceHistoryItem{
		DemoPath:  "demo-missing.dem",
		TakeIndex: 2,
		View:      "killer",
		SpecMode:  1,
		KillIDs:   []string{"k2"},
		VideoPath: missing,
	}
	keyOK := plugingen.BuildProduceHistoryKey(itemOK.DemoPath, itemOK.View, itemOK.SpecMode, itemOK.KillIDs)
	keyMissing := plugingen.BuildProduceHistoryKey(itemMissing.DemoPath, itemMissing.View, itemMissing.SpecMode, itemMissing.KillIDs)
	app.produceState.historyItems[keyOK] = itemOK
	app.produceState.historyItems[keyMissing] = itemMissing
	app.produceState.historyOrder = append(app.produceState.historyOrder, keyOK, keyMissing)
	app.produceState.historyKeyIndex[keyOK] = struct{}{}
	app.produceState.historyKeyIndex[keyMissing] = struct{}{}

	old := openDirectoryDialog
	openDirectoryDialog = func(_ context.Context, _ wailsruntime.OpenDialogOptions) (string, error) {
		return targetDir, nil
	}
	t.Cleanup(func() { openDirectoryDialog = old })

	result, err := app.ExportProduceHistoryVideos()
	if err != nil {
		t.Fatalf("ExportProduceHistoryVideos: %v", err)
	}
	if result.Total != 2 || result.Moved != 1 || result.Failed != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.Errors) == 0 {
		t.Fatalf("expected error messages, got none")
	}

	destOK := filepath.Join(targetDir, "ok.mp4")
	if _, err := os.Stat(destOK); err != nil {
		t.Fatalf("copied file missing in target: %v", err)
	}
	if _, err := os.Stat(srcOK); err != nil {
		t.Fatalf("source file should remain after copy: %v", err)
	}

	snapshot := app.getProduceHistorySnapshot()
	paths := map[string]string{}
	for _, item := range snapshot.Items {
		paths[item.DemoPath] = item.VideoPath
	}
	if paths["demo-ok.dem"] != srcOK {
		t.Fatalf("copied item path should remain source path: %+v", paths)
	}
	if paths["demo-missing.dem"] != missing {
		t.Fatalf("missing item path should remain old path: %+v", paths)
	}
}

func TestExportProduceHistoryVideos_Cancelled(t *testing.T) {
	sourceDir := t.TempDir()
	src := filepath.Join(sourceDir, "clip.mp4")
	if err := os.WriteFile(src, []byte("video"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	app := &App{
		produceState: produceSessionState{
			historyItems:    make(map[string]ProduceHistoryItem),
			historyOrder:    make([]string, 0, 1),
			historyKeyIndex: make(map[string]struct{}),
		},
	}
	item := ProduceHistoryItem{
		DemoPath:  "demo.dem",
		TakeIndex: 1,
		View:      "killer",
		SpecMode:  1,
		KillIDs:   []string{"k1"},
		VideoPath: src,
	}
	key := plugingen.BuildProduceHistoryKey(item.DemoPath, item.View, item.SpecMode, item.KillIDs)
	app.produceState.historyItems[key] = item
	app.produceState.historyOrder = append(app.produceState.historyOrder, key)
	app.produceState.historyKeyIndex[key] = struct{}{}

	old := openDirectoryDialog
	openDirectoryDialog = func(_ context.Context, _ wailsruntime.OpenDialogOptions) (string, error) {
		return "", nil
	}
	t.Cleanup(func() { openDirectoryDialog = old })

	result, err := app.ExportProduceHistoryVideos()
	if err != nil {
		t.Fatalf("ExportProduceHistoryVideos: %v", err)
	}
	if !result.Cancelled {
		t.Fatalf("expected cancelled=true, got %+v", result)
	}
	if result.Moved != 0 || result.Failed != 0 {
		t.Fatalf("cancelled export should not move files: %+v", result)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should remain untouched: %v", err)
	}

	snapshot := app.getProduceHistorySnapshot()
	if len(snapshot.Items) != 1 || snapshot.Items[0].VideoPath != src {
		t.Fatalf("history should remain unchanged: %+v", snapshot.Items)
	}
}

func fakeFFmpegCommandSuccess(command string, args ...string) *exec.Cmd {
	all := append([]string{"-test.run=TestHelperProcessFFmpeg", "--", command}, args...)
	cmd := exec.Command(os.Args[0], all...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS_FFMPEG=1", "FFMPEG_HELPER_MODE=success")
	return cmd
}

func fakeFFmpegCommandFail(command string, args ...string) *exec.Cmd {
	all := append([]string{"-test.run=TestHelperProcessFFmpeg", "--", command}, args...)
	cmd := exec.Command(os.Args[0], all...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS_FFMPEG=1", "FFMPEG_HELPER_MODE=fail")
	return cmd
}

func fakeFFmpegCommandSuccessContext(_ context.Context, command string, args ...string) *exec.Cmd {
	return fakeFFmpegCommandSuccess(command, args...)
}

func fakeFFmpegCommandFailContext(_ context.Context, command string, args ...string) *exec.Cmd {
	return fakeFFmpegCommandFail(command, args...)
}

func TestHelperProcessFFmpeg(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_FFMPEG") != "1" {
		return
	}
	mode := os.Getenv("FFMPEG_HELPER_MODE")
	if mode == "fail" {
		_, _ = fmt.Fprintln(os.Stderr, "simulated ffmpeg failure")
		os.Exit(2)
	}
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	outputPath := os.Args[len(os.Args)-1]
	if outputPath == "-" {
		os.Exit(0)
	}
	if err := os.WriteFile(outputPath, []byte("muxed"), 0644); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	os.Exit(0)
}

type producePluginTestEnvironment struct {
	exeDir           string
	gameInfoPath     string
	originalGameInfo string
	pluginDirPath    string
	binDirPath       string
	targetDLLPath    string
}

func setupProducePluginTestEnvironment(t *testing.T) producePluginTestEnvironment {
	t.Helper()
	exeDir := t.TempDir()
	cs2Root := t.TempDir()
	cs2Exe := filepath.Join(cs2Root, "game", "bin", "win64", "cs2.exe")
	if err := os.MkdirAll(filepath.Dir(cs2Exe), 0755); err != nil {
		t.Fatalf("mkdir cs2 exe dir: %v", err)
	}
	if err := os.WriteFile(cs2Exe, []byte("exe"), 0644); err != nil {
		t.Fatalf("write cs2 exe: %v", err)
	}

	gameInfoPath := filepath.Join(cs2Root, "game", "csgo", "gameinfo.gi")
	if err := os.MkdirAll(filepath.Dir(gameInfoPath), 0755); err != nil {
		t.Fatalf("mkdir gameinfo dir: %v", err)
	}
	originalGameInfo := "FileSystem\n{\n\tSearchPaths\n\t{\n\t\tGame\tcsgo\n\t}\n}\n"
	if err := os.WriteFile(gameInfoPath, []byte(originalGameInfo), 0644); err != nil {
		t.Fatalf("write gameinfo: %v", err)
	}

	pluginSourcePath := filepath.Join(exeDir, "plugin", "server.dll")
	if err := os.MkdirAll(filepath.Dir(pluginSourcePath), 0755); err != nil {
		t.Fatalf("mkdir plugin source dir: %v", err)
	}
	if err := os.WriteFile(pluginSourcePath, []byte("plugin-new"), 0644); err != nil {
		t.Fatalf("write plugin source dll: %v", err)
	}

	cfg := config.Default(exeDir)
	cfg.CS2Dir = cs2Root
	cfg.CS2Exe = cs2Exe
	cfg.PluginDLL = pluginSourcePath
	if err := config.Save(filepath.Join(exeDir, "config.json"), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	pluginDirPath := filepath.Join(cs2Root, "game", "csgo", "plugin")
	binDirPath := filepath.Join(pluginDirPath, "bin")
	return producePluginTestEnvironment{
		exeDir:           exeDir,
		gameInfoPath:     gameInfoPath,
		originalGameInfo: originalGameInfo,
		pluginDirPath:    pluginDirPath,
		binDirPath:       binDirPath,
		targetDLLPath:    filepath.Join(binDirPath, "server.dll"),
	}
}

func writeProduceDemoFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "match.dem")
	if err := os.WriteFile(path, []byte("demo"), 0644); err != nil {
		t.Fatalf("write demo file: %v", err)
	}
	return path
}
