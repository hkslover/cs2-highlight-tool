package app

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"cs2-highlight-tool-v2/internal/config"
)

func planItemByKey(t *testing.T, plan *DebugConsolePlan, key string) DebugSettingPlan {
	t.Helper()
	if plan == nil {
		t.Fatal("plan is nil")
	}
	for _, item := range plan.Settings {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("plan item %q not found", key)
	return DebugSettingPlan{}
}

func TestGetDebugConsolePlanNormalizesDraftAndResolvesAuto(t *testing.T) {
	a := newTestAppInTempDir(t)
	if _, err := a.updateConfig(func(cfg *config.Config) error {
		cfg.FFmpegDetectedPreset = "n1"
		cfg.FFmpegDetectedEncoders = []string{"hevc_nvenc"}
		return nil
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	plan, err := a.GetDebugConsolePlan(DebugSettingsDraft{
		VideoPreset:      "auto",
		RecordQuality:    "",
		RecordFPS:        0,
		KillFeedLifetime: 99,
		NoFlashStrength:  2.5,
		LaunchResolution: "bogus",
		SkyBlackout:      false,
		DisableClouds:    true,
		PovRadarEnabled:  true,
		EnableVoice:      true,
	})
	if err != nil {
		t.Fatalf("GetDebugConsolePlan: %v", err)
	}

	if plan.EffectiveVideoPreset != "n1" || plan.EffectiveVideoEncoder != "hevc_nvenc" {
		t.Fatalf("unexpected effective video settings: %+v", plan)
	}
	if plan.EffectiveVideoStatus != "ready" {
		t.Fatalf("effective status=%q want ready", plan.EffectiveVideoStatus)
	}

	wantOutputDir := filepath.Join(a.dataRoot(), "outputs", "debug")
	if filepath.Clean(plan.RecordOutputDir) != filepath.Clean(wantOutputDir) {
		t.Fatalf("record output dir=%q want %q", plan.RecordOutputDir, wantOutputDir)
	}
	if !containsSubstring(plan.Recording, "mirv_streams record fps 60") {
		t.Fatalf("recording fps was not normalized: %#v", plan.Recording)
	}
	if !containsSubstring(plan.Recording, "mirv_streams settings add ffmpeg debug_n1_") {
		t.Fatalf("auto preset was not resolved in the recording commands: %#v", plan.Recording)
	}
	// `bogus` falls back to 4:3, which keeps the real-pixel stretch.
	if !containsSubstring(plan.Recording, "-vf scale=trunc(ih*16/9/2)*2:ih") {
		t.Fatalf("4:3 stretch missing from the recording commands: %#v", plan.Recording)
	}

	lifetime := planItemByKey(t, plan, "kill_feed_lifetime")
	if !reflect.DeepEqual(lifetime.Commands, []string{"mirv_deathmsg lifetime 10"}) {
		t.Fatalf("lifetime commands=%#v want the clamped maximum", lifetime.Commands)
	}
	noFlash := planItemByKey(t, plan, "noflash_strength")
	if !reflect.DeepEqual(noFlash.Commands, []string{"mirv_noflash 1.00"}) {
		t.Fatalf("noflash commands=%#v want the clamped maximum strength", noFlash.Commands)
	}
	if !reflect.DeepEqual(noFlash.Off, []string{"mirv_noflash 0.00"}) {
		t.Fatalf("noflash restore=%#v want an explicit zero strength", noFlash.Off)
	}
	sky := planItemByKey(t, plan, "sky_blackout")
	if !reflect.DeepEqual(sky.Commands, []string{"r_drawskybox 1"}) {
		t.Fatalf("sky commands=%#v want the explicit restore", sky.Commands)
	}
	clouds := planItemByKey(t, plan, "disable_clouds")
	if !reflect.DeepEqual(clouds.Commands, []string{"mirv_sky clouds draw 0"}) {
		t.Fatalf("cloud commands=%#v", clouds.Commands)
	}
	voice := planItemByKey(t, plan, "enable_voice")
	wantVoice := []string{"tv_listen_voice_indices -1", "tv_listen_voice_indices_h -1"}
	if !reflect.DeepEqual(voice.Commands, wantVoice) {
		t.Fatalf("voice commands=%#v want %#v", voice.Commands, wantVoice)
	}

	if len(plan.ApplyAll) == 0 {
		t.Fatal("apply-all must contain the runtime settings")
	}
	for _, command := range plan.ApplyAll {
		if strings.TrimSpace(command) == "" {
			t.Fatalf("apply-all contains an empty command: %#v", plan.ApplyAll)
		}
	}
}

func TestGetDebugConsolePlanUsesSessionLaunchResolution(t *testing.T) {
	a := newTestAppInTempDir(t)
	rt := &debugSessionRuntime{
		sessionID: "dbg-test",
		request:   DebugLaunchRequest{LaunchResolution: "16:9", InjectPlugin: true},
		done:      make(chan struct{}),
		status:    DebugStatusRunning,
	}
	a.installDebugRuntime(rt)

	plan, err := a.GetDebugConsolePlan(DebugSettingsDraft{
		VideoPreset:      "c1",
		RecordQuality:    "high",
		RecordFPS:        60,
		KillFeedLifetime: 4,
		LaunchResolution: "4:3",
	})
	if err != nil {
		t.Fatalf("GetDebugConsolePlan: %v", err)
	}
	if containsSubstring(plan.Recording, "scale=trunc") {
		t.Fatalf("running session resolution must win over the draft: %#v", plan.Recording)
	}
}

func TestGetDebugConsolePlanRecordingSettingsNameTracksQuality(t *testing.T) {
	a := newTestAppInTempDir(t)
	draft := DebugSettingsDraft{
		VideoPreset:      "c1",
		RecordQuality:    "high",
		RecordFPS:        60,
		KillFeedLifetime: 4,
		LaunchResolution: "16:9",
	}

	settingsName := func(plan *DebugConsolePlan) string {
		t.Helper()
		for _, command := range plan.Recording {
			if strings.HasPrefix(command, "mirv_streams settings add ffmpeg ") {
				fields := strings.Fields(command)
				if len(fields) >= 5 {
					return fields[4]
				}
			}
		}
		t.Fatalf("no ffmpeg settings command in %#v", plan.Recording)
		return ""
	}

	first, err := a.GetDebugConsolePlan(draft)
	if err != nil {
		t.Fatalf("first plan: %v", err)
	}
	firstName := settingsName(first)
	if !strings.HasPrefix(firstName, "debug_c1_high_") {
		t.Fatalf("debug settings name=%q want a parameter-derived name", firstName)
	}
	if !containsSubstring(first.Recording, "mirv_streams record screen settings "+firstName) {
		t.Fatalf("screen must bind to the same settings name: %#v", first.Recording)
	}

	repeated, err := a.GetDebugConsolePlan(draft)
	if err != nil {
		t.Fatalf("repeated plan: %v", err)
	}
	if repeatedName := settingsName(repeated); repeatedName != firstName {
		t.Fatalf("identical draft must reuse the settings name: %q vs %q", repeatedName, firstName)
	}

	upgraded := draft
	upgraded.RecordQuality = "ultra"
	changed, err := a.GetDebugConsolePlan(upgraded)
	if err != nil {
		t.Fatalf("changed plan: %v", err)
	}
	if changedName := settingsName(changed); changedName == firstName {
		t.Fatalf("quality change must create a new HLAE settings entry, got %q", changedName)
	}
}

func containsSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
