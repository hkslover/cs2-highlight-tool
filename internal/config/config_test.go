package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConfigCreateLoadSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.CS2Exe = filepath.Join(dir, "cs2.exe")
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CS2Exe != cfg.CS2Exe {
		t.Fatalf("CS2Exe = %q, want %q", loaded.CS2Exe, cfg.CS2Exe)
	}
}

func TestLoadOrCreateDoesNotRewriteNormalizedConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := Save(path, Default(dir)); err != nil {
		t.Fatalf("save config: %v", err)
	}
	fixedTime := time.Unix(123, 456)
	if err := os.Chtimes(path, fixedTime, fixedTime); err != nil {
		t.Fatalf("set config timestamp: %v", err)
	}
	if _, err := LoadOrCreate(path, dir); err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if !info.ModTime().Equal(fixedTime) {
		t.Fatalf("normalized config was rewritten: modtime=%v want=%v", info.ModTime(), fixedTime)
	}
}

func TestSaveConcurrentWritersAlwaysLeaveValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(version int) {
			defer wg.Done()
			cfg := Default(dir)
			cfg.LastChangelogVersion = string(rune('a' + version))
			if err := Save(path, cfg); err != nil {
				t.Errorf("Save: %v", err)
			}
		}(i)
	}
	wg.Wait()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("final config is invalid JSON: %v", err)
	}
}

func TestApplyDefaultsPreservesConfiguredPaths(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		CS2Dir:           filepath.Join(dir, "custom", "cs2dir"),
		CS2Exe:           filepath.Join(dir, "custom", "cs2.exe"),
		HLAEExe:          filepath.Join(dir, "hlae", "custom.exe"),
		PluginDLL:        filepath.Join(dir, "plugin", "custom.dll"),
		FFmpegDir:        filepath.Join(dir, "ffmpeg", "bin"),
		DownloadSource:   defaultDownloadSource(),
		LaunchResolution: "16:9",
	}

	ApplyDefaults(cfg, dir)

	if cfg.HLAEExe != filepath.Clean(cfg.HLAEExe) {
		t.Fatalf("HLAEExe was not cleaned: %q", cfg.HLAEExe)
	}
	if cfg.HLAEExe != filepath.Join(dir, "hlae", "custom.exe") {
		t.Fatalf("HLAEExe overwritten, got %q", cfg.HLAEExe)
	}
	if cfg.PluginDLL != filepath.Join(dir, "plugin", "custom.dll") {
		t.Fatalf("PluginDLL overwritten, got %q", cfg.PluginDLL)
	}
	if cfg.FFmpegDir != filepath.Join(dir, "ffmpeg", "bin") {
		t.Fatalf("FFmpegDir overwritten, got %q", cfg.FFmpegDir)
	}
	if cfg.DownloadSource != defaultDownloadSource() {
		t.Fatalf("DownloadSource overwritten, got %q", cfg.DownloadSource)
	}
	if cfg.LaunchResolution != "16:9" {
		t.Fatalf("LaunchResolution overwritten, got %q", cfg.LaunchResolution)
	}
	if cfg.KillerPreSeconds != DefaultKillerPreSeconds {
		t.Fatalf("default KillerPreSeconds not applied, got %v", cfg.KillerPreSeconds)
	}
	if cfg.KillerPostSeconds != DefaultKillerPostSeconds {
		t.Fatalf("default KillerPostSeconds not applied, got %v", cfg.KillerPostSeconds)
	}
	if cfg.VictimPreSeconds != DefaultVictimPreSeconds {
		t.Fatalf("default VictimPreSeconds not applied, got %v", cfg.VictimPreSeconds)
	}
	if cfg.VictimPostSeconds != DefaultVictimPostSeconds {
		t.Fatalf("default VictimPostSeconds not applied, got %v", cfg.VictimPostSeconds)
	}
}

func TestApplyDefaultsFillsMissingValues(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{}

	ApplyDefaults(cfg, dir)

	if cfg.HLAEExe != filepath.Join(dir, "hlae", "HLAE.exe") {
		t.Fatalf("default HLAEExe not applied, got %q", cfg.HLAEExe)
	}
	if cfg.PluginDLL != filepath.Join(dir, "plugin", "server.dll") {
		t.Fatalf("default PluginDLL not applied, got %q", cfg.PluginDLL)
	}
	if cfg.FFmpegDir != filepath.Join(dir, "ffmpeg", "bin") {
		t.Fatalf("default FFmpegDir not applied, got %q", cfg.FFmpegDir)
	}
	if cfg.DownloadSource != defaultDownloadSource() {
		t.Fatalf("default DownloadSource not applied, got %q", cfg.DownloadSource)
	}
	if cfg.KillerPreSeconds != DefaultKillerPreSeconds {
		t.Fatalf("default KillerPreSeconds not applied, got %v", cfg.KillerPreSeconds)
	}
	if cfg.KillerPostSeconds != DefaultKillerPostSeconds {
		t.Fatalf("default KillerPostSeconds not applied, got %v", cfg.KillerPostSeconds)
	}
	if cfg.VictimPreSeconds != DefaultVictimPreSeconds {
		t.Fatalf("default VictimPreSeconds not applied, got %v", cfg.VictimPreSeconds)
	}
	if cfg.VictimPostSeconds != DefaultVictimPostSeconds {
		t.Fatalf("default VictimPostSeconds not applied, got %v", cfg.VictimPostSeconds)
	}
	if cfg.ClipActionSettings == nil {
		t.Fatalf("default ClipActionSettings not applied")
	}
	if !cfg.ClipActionSettings.EnableVoiceIndices || cfg.ClipActionSettings.VoiceIndicesValue != -1 {
		t.Fatalf("default ClipActionSettings voice_indices not applied: %+v", *cfg.ClipActionSettings)
	}
	if !cfg.ClipActionSettings.EnableVoiceIndicesH || cfg.ClipActionSettings.VoiceIndicesHValue != -1 {
		t.Fatalf("default ClipActionSettings voice_indices_h not applied: %+v", *cfg.ClipActionSettings)
	}
	if cfg.RecordFPS != DefaultRecordFPS {
		t.Fatalf("default RecordFPS not applied: %d", cfg.RecordFPS)
	}
	if cfg.EditFPS != DefaultEditFPS {
		t.Fatalf("default EditFPS not applied: %d", cfg.EditFPS)
	}
	if cfg.EditQuality != DefaultEditQuality {
		t.Fatalf("default EditQuality not applied: %q", cfg.EditQuality)
	}
	if cfg.RecordQuality != DefaultRecordQuality {
		t.Fatalf("default RecordQuality not applied: %q", cfg.RecordQuality)
	}
	if cfg.VideoPreset != DefaultVideoPreset {
		t.Fatalf("default VideoPreset not applied: %q", cfg.VideoPreset)
	}
	if cfg.LaunchResolution != DefaultLaunchResolution {
		t.Fatalf("default LaunchResolution not applied: %q", cfg.LaunchResolution)
	}
	if cfg.RecordOutputDir != filepath.Join(dir, "outputs") {
		t.Fatalf("default RecordOutputDir not applied: %q", cfg.RecordOutputDir)
	}
	if cfg.FiveEPlayerName != "" {
		t.Fatalf("default FiveEPlayerName should be empty: %q", cfg.FiveEPlayerName)
	}
	if cfg.PovRadarEnabled {
		t.Fatalf("default PovRadarEnabled should be false")
	}
}

func TestLoadOrCreate_LegacyConfigDefaultsPovRadarDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"record_fps":60}`), 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatalf("LoadOrCreate legacy config: %v", err)
	}
	if cfg.PovRadarEnabled {
		t.Fatalf("legacy config should default PovRadarEnabled=false")
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	if !strings.Contains(string(saved), `"pov_radar_enabled": false`) {
		t.Fatalf("migrated config should persist pov_radar_enabled=false: %s", saved)
	}
}

func TestApplyDefaultsPreservesSupportedLaunchResolutions(t *testing.T) {
	dir := t.TempDir()
	tests := []string{
		"16:9",
		"4:3",
		"4:3_1280x960",
	}

	for _, launchResolution := range tests {
		t.Run(launchResolution, func(t *testing.T) {
			cfg := &Config{LaunchResolution: launchResolution}

			ApplyDefaults(cfg, dir)

			if cfg.LaunchResolution != launchResolution {
				t.Fatalf("LaunchResolution = %q, want %q", cfg.LaunchResolution, launchResolution)
			}
		})
	}
}

func TestApplyDefaults_ForcesManagedRecordOutputDir(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		RecordOutputDir: filepath.Join(dir, "custom", "outputs"),
	}

	ApplyDefaults(cfg, dir)

	if cfg.RecordOutputDir != filepath.Join(dir, "outputs") {
		t.Fatalf("RecordOutputDir should be fixed under exeDir, got %q", cfg.RecordOutputDir)
	}
}

func TestApplyDefaults_ClampsEditSettings(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		EditFPS:       10,
		EditQuality:   "invalid",
		RecordQuality: "invalid",
	}

	ApplyDefaults(cfg, dir)
	if cfg.EditFPS != MinEditFPS {
		t.Fatalf("EditFPS should clamp to min, got %d", cfg.EditFPS)
	}
	if cfg.EditQuality != DefaultEditQuality {
		t.Fatalf("EditQuality should fallback to default, got %q", cfg.EditQuality)
	}
	if cfg.RecordQuality != DefaultRecordQuality {
		t.Fatalf("RecordQuality should fallback to default, got %q", cfg.RecordQuality)
	}

	cfg.EditFPS = 1000
	cfg.EditQuality = "ultra"
	cfg.RecordQuality = "standard"
	ApplyDefaults(cfg, dir)
	if cfg.EditFPS != MaxEditFPS {
		t.Fatalf("EditFPS should clamp to max, got %d", cfg.EditFPS)
	}
	if cfg.EditQuality != "ultra" {
		t.Fatalf("EditQuality should persist valid value, got %q", cfg.EditQuality)
	}
	if cfg.RecordQuality != "standard" {
		t.Fatalf("RecordQuality should persist valid value, got %q", cfg.RecordQuality)
	}
}

func TestApplyDefaults_TrimsFiveEPlayerName(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		FiveEPlayerName: "  molim  ",
	}
	ApplyDefaults(cfg, dir)
	if cfg.FiveEPlayerName != "molim" {
		t.Fatalf("FiveEPlayerName should be trimmed, got %q", cfg.FiveEPlayerName)
	}
}

func TestLoadOrCreate_CompatibleWithLegacySourceManualField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	payload := `{
  "download_source": "` + defaultDownloadSource() + `",
  "source_manual": true
}`
	if err := os.WriteFile(path, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DownloadSource != defaultDownloadSource() {
		t.Fatalf("download source = %q, want %q", cfg.DownloadSource, defaultDownloadSource())
	}
}

func TestLoadOrCreate_DropsLegacyHLAEVersionFieldOnSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	payload := `{
  "hlae_exe": "` + filepath.ToSlash(filepath.Join(dir, "hlae", "HLAE.exe")) + `",
  "hlae_version": "9.9.9"
}`
	if err := os.WriteFile(path, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HLAEExe != filepath.Join(dir, "hlae", "HLAE.exe") {
		t.Fatalf("hlae exe = %q, want %q", cfg.HLAEExe, filepath.Join(dir, "hlae", "HLAE.exe"))
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "hlae_version") {
		t.Fatalf("saved config should not contain hlae_version, got: %s", string(saved))
	}
}

func TestLoadOrCreate_DropsLegacyPluginVersionFieldOnSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	payload := `{
  "plugin_dll": "` + filepath.ToSlash(filepath.Join(dir, "plugin", "server.dll")) + `",
  "plugin_version": "9.9.9"
}`
	if err := os.WriteFile(path, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PluginDLL != filepath.Join(dir, "plugin", "server.dll") {
		t.Fatalf("plugin dll = %q, want %q", cfg.PluginDLL, filepath.Join(dir, "plugin", "server.dll"))
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "plugin_version") {
		t.Fatalf("saved config should not contain plugin_version, got: %s", string(saved))
	}
}

func TestLoadOrCreate_FillsClipActionSettingsForLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	payload := `{
  "killer_pre_seconds": 5,
  "killer_post_seconds": 5
}`
	if err := os.WriteFile(path, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	settings := ResolveClipActionSettings(cfg)
	if !settings.EnableVoiceIndices || settings.VoiceIndicesValue != -1 {
		t.Fatalf("voice_indices defaults not applied: %+v", settings)
	}
	if !settings.EnableVoiceIndicesH || settings.VoiceIndicesHValue != -1 {
		t.Fatalf("voice_indices_h defaults not applied: %+v", settings)
	}
	if cfg.VictimPreSeconds != DefaultVictimPreSeconds || cfg.VictimPostSeconds != DefaultVictimPostSeconds {
		t.Fatalf("victim pre/post defaults not applied: pre=%v post=%v", cfg.VictimPreSeconds, cfg.VictimPostSeconds)
	}
	if !cfg.AutoAddVictimView {
		t.Fatalf("auto_add_victim_view default not applied")
	}
	if cfg.LaunchResolution != DefaultLaunchResolution {
		t.Fatalf("launch_resolution default not applied")
	}
	if !cfg.EnableSpecShowXray {
		t.Fatalf("enable_spec_show_xray_zero default not applied")
	}
	if cfg.HideAllUI {
		t.Fatalf("hide_all_ui should default to false")
	}
	if cfg.UseShoulderCamera {
		t.Fatalf("use_shoulder_camera should default to false")
	}
	if cfg.DisableClouds {
		t.Fatalf("disable_clouds should default to false")
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "clip_action_settings") {
		t.Fatalf("saved config should contain clip_action_settings, got: %s", string(saved))
	}
	if !strings.Contains(string(saved), "victim_pre_seconds") || !strings.Contains(string(saved), "auto_add_victim_view") {
		t.Fatalf("saved config should contain victim/auto settings, got: %s", string(saved))
	}
	if !strings.Contains(string(saved), "record_output_dir") || !strings.Contains(string(saved), "enable_spec_show_xray_zero") {
		t.Fatalf("saved config should contain new hlae fields, got: %s", string(saved))
	}
	if !strings.Contains(string(saved), "launch_resolution") {
		t.Fatalf("saved config should contain launch_resolution, got: %s", string(saved))
	}
	if !strings.Contains(string(saved), "hide_all_ui") {
		t.Fatalf("saved config should contain hide_all_ui, got: %s", string(saved))
	}
	if !strings.Contains(string(saved), "use_shoulder_camera") {
		t.Fatalf("saved config should contain use_shoulder_camera, got: %s", string(saved))
	}
	if !strings.Contains(string(saved), "disable_clouds") {
		t.Fatalf("saved config should contain disable_clouds, got: %s", string(saved))
	}
}

func TestLoadOrCreate_NormalizesClipWindowSeconds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	payload := `{
  "killer_pre_seconds": 20,
  "killer_post_seconds": 22.9,
  "victim_pre_seconds": 19.5,
  "victim_post_seconds": 0.3
}`
	if err := os.WriteFile(path, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KillerPreSeconds != 20 || cfg.KillerPostSeconds != 20 {
		t.Fatalf("killer window seconds should normalize to [1,20]: pre=%v post=%v", cfg.KillerPreSeconds, cfg.KillerPostSeconds)
	}
	if cfg.VictimPreSeconds != 19.5 || cfg.VictimPostSeconds != 1 {
		t.Fatalf("victim window seconds should normalize to [1,20]: pre=%v post=%v", cfg.VictimPreSeconds, cfg.VictimPostSeconds)
	}
}

func TestLoadOrCreate_LegacyConfigBackfillsRecordingFieldDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	payload := `{
  "killer_pre_seconds": 5
}`
	if err := os.WriteFile(path, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SkyBlackout {
		t.Fatalf("legacy config should backfill sky_blackout=true, got %v", cfg.SkyBlackout)
	}
	if cfg.KillFeedLifetime != DefaultKillFeedLifetime {
		t.Fatalf("legacy config should backfill kill_feed_lifetime=%d, got %d", DefaultKillFeedLifetime, cfg.KillFeedLifetime)
	}
	if cfg.BlockKillFeed {
		t.Fatalf("block_kill_feed should default to false")
	}
	if cfg.DisableClouds {
		t.Fatalf("legacy config should default disable_clouds to false")
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "sky_blackout") || !strings.Contains(string(saved), "disable_clouds") || !strings.Contains(string(saved), "kill_feed_lifetime") || !strings.Contains(string(saved), "block_kill_feed") {
		t.Fatalf("saved config should contain new recording fields, got: %s", string(saved))
	}
}

func TestLoadOrCreate_ExplicitFalseSkyBlackoutIsPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	payload := `{
  "sky_blackout": false,
  "kill_feed_lifetime": 7
}`
	if err := os.WriteFile(path, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SkyBlackout {
		t.Fatalf("explicit sky_blackout=false should be preserved, got %v", cfg.SkyBlackout)
	}
	if cfg.KillFeedLifetime != 7 {
		t.Fatalf("explicit kill_feed_lifetime=7 should be preserved, got %d", cfg.KillFeedLifetime)
	}
}

func TestApplyDefaults_KillFeedLifetimeClamp(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{KillFeedLifetime: 0}
	ApplyDefaults(cfg, dir)
	if cfg.KillFeedLifetime != MinKillFeedLifetime {
		t.Fatalf("zero KillFeedLifetime should clamp to %d, got %d", MinKillFeedLifetime, cfg.KillFeedLifetime)
	}

	cfg = &Config{KillFeedLifetime: 999}
	ApplyDefaults(cfg, dir)
	if cfg.KillFeedLifetime != MaxKillFeedLifetime {
		t.Fatalf("large KillFeedLifetime should clamp to %d, got %d", MaxKillFeedLifetime, cfg.KillFeedLifetime)
	}

	cfg = &Config{KillFeedLifetime: 5}
	ApplyDefaults(cfg, dir)
	if cfg.KillFeedLifetime != 5 {
		t.Fatalf("valid KillFeedLifetime should be preserved, got %d", cfg.KillFeedLifetime)
	}
}

func TestResolveClipActionSettings_RequiresBothEnableFlags(t *testing.T) {
	cfg := &Config{
		ClipActionSettings: &ClipActionSettings{
			EnableVoiceIndices:  true,
			VoiceIndicesValue:   -1,
			EnableVoiceIndicesH: false,
			VoiceIndicesHValue:  -1,
		},
	}
	settings := ResolveClipActionSettings(cfg)
	if settings.EnableVoiceIndices || settings.EnableVoiceIndicesH {
		t.Fatalf("expected both flags false when only one is enabled: %+v", settings)
	}
}

func TestEnsureFirstInstallChangelogSeed_SeedsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	seeded, err := EnsureFirstInstallChangelogSeed(path, dir, "2.0.2")
	if err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	if !seeded {
		t.Fatalf("expected seeded=true for missing config")
	}
	cfg, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.LastChangelogVersion != "2.0.2" {
		t.Fatalf("expected seeded version 2.0.2, got %q", cfg.LastChangelogVersion)
	}
}

func TestEnsureFirstInstallChangelogSeed_NoopWhenConfigExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatalf("initial load failed: %v", err)
	}
	if cfg.LastChangelogVersion != "" {
		t.Fatalf("expected empty LastChangelogVersion in fresh default cfg, got %q", cfg.LastChangelogVersion)
	}
	seeded, err := EnsureFirstInstallChangelogSeed(path, dir, "2.0.2")
	if err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	if seeded {
		t.Fatalf("expected seeded=false when config already exists")
	}
	loaded, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if loaded.LastChangelogVersion != "" {
		t.Fatalf("seed must not overwrite existing config, got %q", loaded.LastChangelogVersion)
	}
}

func TestEnsureFirstInstallChangelogSeed_EmptyVersionIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	seeded, err := EnsureFirstInstallChangelogSeed(path, dir, "  ")
	if err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	if seeded {
		t.Fatalf("expected seeded=false for empty version")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected config.json to remain absent when version is empty, stat err=%v", err)
	}
}

func TestConfigJSONOmitsEmptyLastChangelogVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if _, err := LoadOrCreate(path, dir); err != nil {
		t.Fatalf("load failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if strings.Contains(string(data), "last_changelog_version") {
		t.Fatalf("expected empty LastChangelogVersion to be omitted via omitempty, got %s", data)
	}
}

func TestConfigJSONRoundtripsLastChangelogVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	cfg.LastChangelogVersion = "2.0.3"
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	loaded, err := LoadOrCreate(path, dir)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if loaded.LastChangelogVersion != "2.0.3" {
		t.Fatalf("expected LastChangelogVersion=2.0.3, got %q", loaded.LastChangelogVersion)
	}
}
