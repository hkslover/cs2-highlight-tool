package clipsjson

import (
	"fmt"
	"strings"
	"testing"

	"cs2-highlight-tool-v2/internal/demo"
)

func TestBuild_AddsHLAEBootstrapCommands(t *testing.T) {
	result, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11},
		IncludeVictim: true,
	}}, BuildOptions{
		TickRate:           64,
		KillerPreSeconds:   1,
		KillerPostSeconds:  1,
		VictimPreSeconds:   1,
		VictimPostSeconds:  1,
		RecordFPS:          60,
		VideoPreset:        "n1",
		RecordOutputDir:    `D:/clips/output`,
		RecordBatchName:    "20260422_131500",
		EnableSpecShowXray: true,
		ActionSettings: ActionSettings{
			EnableVoiceIndices:  true,
			VoiceIndicesValue:   -1,
			EnableVoiceIndicesH: true,
			VoiceIndicesHValue:  -1,
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Sequences) != 3 {
		t.Fatalf("sequence len=%d want 3", len(result.Sequences))
	}

	bootstrap := result.Sequences[0].Actions
	assertHasAction(t, bootstrap, "r_show_build_info 0")
	assertHasAction(t, bootstrap, "cl_trueview_show_status 0")
	assertHasAction(t, bootstrap, "engine_no_focus_sleep 0")
	assertHasAction(t, bootstrap, "cl_demo_predict 0")
	assertHasAction(t, bootstrap, "spec_show_xray 0")
	assertHasAction(t, bootstrap, "tv_listen_voice_indices -1")
	assertHasAction(t, bootstrap, "tv_listen_voice_indices_h -1")
	assertHasAction(t, bootstrap, "mirv_streams record screen enabled 1")
	assertHasAction(t, bootstrap, "mirv_streams record fps 60")
	assertHasAction(t, bootstrap, "mirv_streams record startMovieWav 1")
	assertHasPrefixAction(t, bootstrap, "mirv_streams settings add ffmpeg n1 ")
	assertHasAction(t, bootstrap, "mirv_streams record screen settings n1")
	assertHasAction(t, bootstrap, `mirv_streams record name "D:/clips/output/20260422_131500"`)
	assertHasAction(t, bootstrap, "go_to_next_sequence")
}

func TestBuild_SpecShowXrayCanBeDisabled(t *testing.T) {
	result, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:           64,
		KillerPreSeconds:   1,
		KillerPostSeconds:  1,
		RecordFPS:          60,
		VideoPreset:        "n1",
		RecordOutputDir:    `D:/clips/output`,
		RecordBatchName:    "20260422_131500",
		EnableSpecShowXray: false,
		ActionSettings:     ActionSettings{VoiceIndicesValue: 0, VoiceIndicesHValue: 0},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	assertHasAction(t, result.Sequences[0].Actions, "spec_show_xray -1")
	assertNoAction(t, result.Sequences[0].Actions, "spec_show_xray 0")
}

func TestBuild_DrawOnlyDeathnoticesOnlyWhenHideAllUIEnabled(t *testing.T) {
	enabled, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		HideAllUI:         true,
	})
	if err != nil {
		t.Fatalf("Build enabled hide_all_ui: %v", err)
	}
	assertHasAction(t, enabled.Sequences[0].Actions, "cl_draw_only_deathnotices 1")

	disabled, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		HideAllUI:         false,
	})
	if err != nil {
		t.Fatalf("Build disabled hide_all_ui: %v", err)
	}
	assertNoPrefixAction(t, disabled.Sequences[0].Actions, "cl_draw_only_deathnotices")
}

func TestBuild_ShoulderCameraCommandBeforeBuildInfoOnlyWhenEnabled(t *testing.T) {
	shoulderCameraCommand := "cam_command 1;cam_idealdist 30;cam_idealyaw 0;cam_idealpitch 0;c_thirdpersonshoulder 1;c_thirdpersonshoulderaimdist 300;c_thirdpersonshoulderdist 40;c_thirdpersonshoulderheight 2;c_thirdpersonshoulderoffset 20;"
	enabled, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:           64,
		KillerPreSeconds:   1,
		KillerPostSeconds:  1,
		RecordFPS:          60,
		VideoPreset:        "n1",
		RecordOutputDir:    `D:/clips/output`,
		UseShoulderCamera:  true,
		KillFeedLifetime:   4,
		EnableSpecShowXray: true,
	})
	if err != nil {
		t.Fatalf("Build enabled use_shoulder_camera: %v", err)
	}
	bootstrap := enabled.Sequences[0].Actions
	if len(bootstrap) < 2 {
		t.Fatalf("bootstrap should contain at least two actions: %#v", bootstrap)
	}
	if bootstrap[0].Cmd != shoulderCameraCommand {
		t.Fatalf("shoulder camera command should be first action, got %#v", bootstrap)
	}
	if bootstrap[1].Cmd != "r_show_build_info 0" {
		t.Fatalf("r_show_build_info should follow shoulder camera command, got %#v", bootstrap[:2])
	}

	disabled, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:           64,
		KillerPreSeconds:   1,
		KillerPostSeconds:  1,
		RecordFPS:          60,
		VideoPreset:        "n1",
		RecordOutputDir:    `D:/clips/output`,
		UseShoulderCamera:  false,
		KillFeedLifetime:   4,
		EnableSpecShowXray: true,
	})
	if err != nil {
		t.Fatalf("Build disabled use_shoulder_camera: %v", err)
	}
	assertNoPrefixAction(t, disabled.Sequences[0].Actions, "cam_command")
	assertNoPrefixAction(t, disabled.Sequences[0].Actions, "c_thirdpersonshoulder")
}

func TestBuild_SkyBlackoutAndCloudsAreIndependent(t *testing.T) {
	tests := []struct {
		name             string
		skyBlackout      bool
		disableClouds    bool
		wantSkybox       bool
		wantCloudsHidden bool
	}{
		{name: "sky blackout only", skyBlackout: true, wantSkybox: true},
		{name: "disable clouds only", disableClouds: true, wantCloudsHidden: true},
		{name: "both enabled", skyBlackout: true, disableClouds: true, wantSkybox: true, wantCloudsHidden: true},
		{name: "both disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Build([]Item{{
				Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
				IncludeVictim: false,
			}}, BuildOptions{
				TickRate:          64,
				KillerPreSeconds:  1,
				KillerPostSeconds: 1,
				RecordFPS:         60,
				VideoPreset:       "n1",
				RecordOutputDir:   `D:/clips/output`,
				SkyBlackout:       tt.skyBlackout,
				DisableClouds:     tt.disableClouds,
				KillFeedLifetime:  4,
			})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}

			if tt.wantSkybox {
				assertHasAction(t, result.Sequences[0].Actions, "r_drawskybox 0")
			} else {
				assertNoAction(t, result.Sequences[0].Actions, "r_drawskybox 0")
			}
			if tt.wantCloudsHidden {
				assertHasAction(t, result.Sequences[0].Actions, "mirv_sky clouds draw 0")
			} else {
				assertNoAction(t, result.Sequences[0].Actions, "mirv_sky clouds draw 0")
			}
		})
	}
}

func TestBuild_KillFeedLifetimeFollowsOption(t *testing.T) {
	result, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		KillFeedLifetime:  7,
	})
	if err != nil {
		t.Fatalf("Build lifetime=7: %v", err)
	}
	assertHasAction(t, result.Sequences[0].Actions, "mirv_deathmsg lifetime 7")
}

func TestBuild_BlockKillFeedEmitsFilterOnlyWhenEnabled(t *testing.T) {
	enabled, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		BlockKillFeed:     true,
		KillFeedLifetime:  4,
	})
	if err != nil {
		t.Fatalf("Build block on: %v", err)
	}
	assertHasAction(t, enabled.Sequences[0].Actions, "mirv_deathmsg filter add block 1")

	disabled, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		BlockKillFeed:     false,
		KillFeedLifetime:  4,
	})
	if err != nil {
		t.Fatalf("Build block off: %v", err)
	}
	assertNoAction(t, disabled.Sequences[0].Actions, "mirv_deathmsg filter add block 1")
}

func TestBuild_VoiceIndicesValueFollowsEnableSwitch(t *testing.T) {
	enabled, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		ActionSettings: ActionSettings{
			EnableVoiceIndices:  true,
			VoiceIndicesValue:   0,
			EnableVoiceIndicesH: true,
			VoiceIndicesHValue:  0,
		},
	})
	if err != nil {
		t.Fatalf("Build enabled voice: %v", err)
	}
	assertHasAction(t, enabled.Sequences[0].Actions, "tv_listen_voice_indices -1")
	assertHasAction(t, enabled.Sequences[0].Actions, "tv_listen_voice_indices_h -1")

	disabled, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		ActionSettings: ActionSettings{
			EnableVoiceIndices:  false,
			VoiceIndicesValue:   -1,
			EnableVoiceIndicesH: true,
			VoiceIndicesHValue:  -1,
		},
	})
	if err != nil {
		t.Fatalf("Build disabled voice: %v", err)
	}
	assertHasAction(t, disabled.Sequences[0].Actions, "tv_listen_voice_indices 0")
	assertHasAction(t, disabled.Sequences[0].Actions, "tv_listen_voice_indices_h 0")
}

func TestBuild_AddsRecordStartEndAndTakeMetadata(t *testing.T) {
	result, err := Build([]Item{
		{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11}, IncludeVictim: true},
		{Kill: demo.ClipKill{ID: "k2", Tick: 280, KillerSlot: 8, VictimSlot: 12}, IncludeVictim: true},
	}, BuildOptions{
		TickRate:           64,
		KillerPreSeconds:   1,
		KillerPostSeconds:  1,
		VictimPreSeconds:   1,
		VictimPostSeconds:  1,
		RecordFPS:          60,
		VideoPreset:        "c1",
		RecordOutputDir:    `D:/clips/output`,
		RecordBatchName:    "20260422_131500",
		EnableSpecShowXray: true,
		ActionSettings:     ActionSettings{VoiceIndicesValue: 0, VoiceIndicesHValue: 0},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var starts []Action
	var ends []Action
	for _, seq := range result.Sequences[1:] {
		for _, action := range seq.Actions {
			switch action.Cmd {
			case "mirv_streams record start":
				starts = append(starts, action)
			case "mirv_streams record end":
				ends = append(ends, action)
			}
		}
	}
	if len(starts) == 0 || len(starts) != len(ends) {
		t.Fatalf("start/end mismatch: %d/%d", len(starts), len(ends))
	}

	for idx := range starts {
		expectedTake := idx + 1
		expectedName := takeName(expectedTake)
		if starts[idx].Metadata == nil || ends[idx].Metadata == nil {
			t.Fatalf("missing metadata for take %d", expectedTake)
		}
		if starts[idx].Metadata.TakeIndex != expectedTake || ends[idx].Metadata.TakeIndex != expectedTake {
			t.Fatalf("take index mismatch for %d: start=%+v end=%+v", expectedTake, starts[idx].Metadata, ends[idx].Metadata)
		}
		if starts[idx].Metadata.TakeName != expectedName || ends[idx].Metadata.TakeName != expectedName {
			t.Fatalf("take name mismatch for %d: start=%+v end=%+v", expectedTake, starts[idx].Metadata, ends[idx].Metadata)
		}
		if starts[idx].Metadata.RecordPhase != "start" || ends[idx].Metadata.RecordPhase != "end" {
			t.Fatalf("phase mismatch for %d: start=%+v end=%+v", expectedTake, starts[idx].Metadata, ends[idx].Metadata)
		}
		if starts[idx].Tick > ends[idx].Tick {
			t.Fatalf("record start tick should <= end tick: %d > %d", starts[idx].Tick, ends[idx].Tick)
		}
	}
}

func TestBuild_SeeksBeforeSegmentStartWithoutChangingRecordingWindow(t *testing.T) {
	result, err := Build([]Item{
		{Kill: demo.ClipKill{ID: "k1", Tick: 500, KillerSlot: 7, VictimSlot: 11}, IncludeVictim: false},
	}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Sequences) < 2 {
		t.Fatalf("sequence len=%d want at least 2", len(result.Sequences))
	}
	actions := result.Sequences[1].Actions
	assertHasAction(t, actions, "demo_gototick 308")

	recordStart := mustFindAction(t, actions, "mirv_streams record start")
	if recordStart.Tick != 446 {
		t.Fatalf("record start tick=%d want 446", recordStart.Tick)
	}
	if len(result.TakePlans) != 1 {
		t.Fatalf("take plans len=%d want 1", len(result.TakePlans))
	}
	if result.TakePlans[0].StartTick != 436 {
		t.Fatalf("take plan start tick=%d want 436", result.TakePlans[0].StartTick)
	}
}

func TestBuild_SeekLeadDoesNotJumpBehindPreviousPassEnd(t *testing.T) {
	result, err := Build([]Item{
		{Kill: demo.ClipKill{ID: "k1", Tick: 500, KillerSlot: 7, VictimSlot: 11}, IncludeVictim: false},
		{Kill: demo.ClipKill{ID: "k2", Tick: 650, KillerSlot: 9, VictimSlot: 12}, IncludeVictim: false},
	}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Sequences) < 2 {
		t.Fatalf("sequence len=%d want at least 2", len(result.Sequences))
	}

	actions := result.Sequences[1].Actions
	assertHasAction(t, actions, "demo_gototick 308")
	assertHasAction(t, actions, "demo_gototick 565")
	assertNoAction(t, actions, "demo_gototick 458")
}

func TestBuild_TakePlansContainMergedKillIDs(t *testing.T) {
	result, err := Build([]Item{
		{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7}, IncludeVictim: false},
		{Kill: demo.ClipKill{ID: "k2", Tick: 220, KillerSlot: 7}, IncludeVictim: false},
	}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		ActionSettings:    ActionSettings{VoiceIndicesValue: 0, VoiceIndicesHValue: 0},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.TakePlans) != 1 {
		t.Fatalf("take plan len=%d want 1", len(result.TakePlans))
	}
	plan := result.TakePlans[0]
	if plan.View != "killer" {
		t.Fatalf("take view=%q want killer", plan.View)
	}
	if plan.SpecMode != 1 {
		t.Fatalf("take spec mode=%d want 1", plan.SpecMode)
	}
	if plan.TakeIndex != 1 || plan.TakeName != "take0000" {
		t.Fatalf("unexpected take identity: %+v", plan)
	}
	if len(plan.KillIDs) != 2 || plan.KillIDs[0] != "k1" || plan.KillIDs[1] != "k2" {
		t.Fatalf("unexpected kill ids: %+v", plan.KillIDs)
	}
}

func TestBuild_DifferentPerClipParamsDoNotMergeKillerSegments(t *testing.T) {
	result, err := Build([]Item{
		{
			Kill:               demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
			IncludeVictim:      false,
			EnableVoice:        true,
			EnableSpecShowXray: true,
		},
		{
			Kill:               demo.ClipKill{ID: "k2", Tick: 220, KillerSlot: 7},
			IncludeVictim:      false,
			EnableVoice:        false,
			EnableSpecShowXray: true,
			HasVoiceOverride:   true,
		},
	}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		ActionSettings:    ActionSettings{VoiceIndicesValue: 0, VoiceIndicesHValue: 0},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.TakePlans) != 2 {
		t.Fatalf("take plan len=%d want 2", len(result.TakePlans))
	}
}

func TestBuild_InjectsPerPassVoiceAndXrayCommandsWhenForced(t *testing.T) {
	result, err := Build([]Item{
		{
			Kill:                    demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
			IncludeVictim:           false,
			EnableVoice:             false,
			EnableSpecShowXray:      false,
			HasVoiceOverride:        true,
			HasSpecShowXrayOverride: true,
		},
		{
			Kill:               demo.ClipKill{ID: "k2", Tick: 400, KillerSlot: 8},
			IncludeVictim:      false,
			EnableVoice:        true,
			EnableSpecShowXray: true,
		},
	}, BuildOptions{
		TickRate:                  64,
		KillerPreSeconds:          1,
		KillerPostSeconds:         1,
		RecordFPS:                 60,
		VideoPreset:               "n1",
		RecordOutputDir:           `D:/clips/output`,
		EnableSpecShowXray:        true,
		ForcePerPassVoiceCommands: true,
		ForcePerPassXrayCommands:  true,
		ActionSettings: ActionSettings{
			EnableVoiceIndices:  true,
			EnableVoiceIndicesH: true,
			VoiceIndicesValue:   -1,
			VoiceIndicesHValue:  -1,
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Sequences) != 2 {
		t.Fatalf("sequence len=%d want 2", len(result.Sequences))
	}
	actions := result.Sequences[1].Actions
	assertHasAction(t, actions, "tv_listen_voice_indices 0")
	assertHasAction(t, actions, "tv_listen_voice_indices_h 0")
	assertHasAction(t, actions, "spec_show_xray -1")
	assertHasAction(t, actions, "tv_listen_voice_indices -1")
	assertHasAction(t, actions, "tv_listen_voice_indices_h -1")
	assertHasAction(t, actions, "spec_show_xray 0")
}

func TestBuild_TakePlansContainVictimView(t *testing.T) {
	result, err := Build([]Item{
		{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11}, IncludeVictim: true},
	}, BuildOptions{
		TickRate:           64,
		KillerPreSeconds:   1,
		KillerPostSeconds:  1,
		VictimPreSeconds:   1,
		VictimPostSeconds:  1,
		RecordFPS:          60,
		VideoPreset:        "n1",
		RecordOutputDir:    `D:/clips/output`,
		EnableSpecShowXray: true,
		ActionSettings:     ActionSettings{VoiceIndicesValue: 0, VoiceIndicesHValue: 0},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.TakePlans) != 2 {
		t.Fatalf("take plan len=%d want 2", len(result.TakePlans))
	}
	if result.TakePlans[0].View != "killer" || result.TakePlans[1].View != "victim" {
		t.Fatalf("unexpected views: %+v", result.TakePlans)
	}
	if result.TakePlans[0].SpecMode != 1 || result.TakePlans[1].SpecMode != 1 {
		t.Fatalf("unexpected spec modes: %+v", result.TakePlans)
	}
}

func TestBuild_LastSequenceEndsWithDisconnect(t *testing.T) {
	result, err := Build([]Item{
		{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11}, IncludeVictim: true},
	}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		VictimPreSeconds:  1,
		VictimPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		ActionSettings:    ActionSettings{VoiceIndicesValue: 0, VoiceIndicesHValue: 0},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Sequences) < 2 {
		t.Fatalf("sequence len=%d want >=2", len(result.Sequences))
	}
	last := result.Sequences[len(result.Sequences)-1].Actions
	assertHasAction(t, last, "disconnect")
	assertNoAction(t, last, "quit")
}

func TestBuild_InvalidPresetReturnsError(t *testing.T) {
	_, err := Build([]Item{{
		Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "bad",
		RecordOutputDir:   `D:/clips/output`,
	})
	if err == nil || !strings.Contains(err.Error(), "video_preset") {
		t.Fatalf("expected invalid preset error, got: %v", err)
	}
}

func TestBuild_SupportsAMDPreset(t *testing.T) {
	result, err := Build([]Item{{
		Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "a1",
		RecordOutputDir:   `D:/clips/output`,
	})
	if err != nil {
		t.Fatalf("Build with a1: %v", err)
	}
	bootstrap := result.Sequences[0].Actions
	assertHasPrefixAction(t, bootstrap, "mirv_streams settings add ffmpeg a1 ")
}

func TestBuild_AppliesRecordQualityToHardwarePreset(t *testing.T) {
	result, err := Build([]Item{{
		Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordQuality:     "ultra",
		RecordOutputDir:   `D:/clips/output`,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	bootstrap := result.Sequences[0].Actions
	assertHasPrefixAction(t, bootstrap, "mirv_streams settings add ffmpeg n1 ")
	assertHasActionContaining(t, bootstrap, "-c:v hevc_nvenc")
	assertHasActionContaining(t, bootstrap, "-qp 10")
}

func TestBuild_AddsDisplayAspectForFourThreeLaunchResolution(t *testing.T) {
	tests := []struct {
		name             string
		launchResolution string
		wantAspect       bool
	}{
		{name: "4:3 1440x1080", launchResolution: "4:3", wantAspect: true},
		{name: "4:3 1280x960", launchResolution: "4:3_1280x960", wantAspect: true},
		{name: "16:9", launchResolution: "16:9", wantAspect: false},
		{name: "empty", launchResolution: "", wantAspect: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Build([]Item{{
				Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
			}}, BuildOptions{
				TickRate:          64,
				KillerPreSeconds:  1,
				KillerPostSeconds: 1,
				RecordFPS:         60,
				VideoPreset:       "c1",
				RecordOutputDir:   `D:/clips/output`,
				LaunchResolution:  tt.launchResolution,
			})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			bootstrap := result.Sequences[0].Actions
			if tt.wantAspect {
				assertHasActionContaining(t, bootstrap, "-aspect 16:9")
				return
			}
			assertNoActionContaining(t, bootstrap, "-aspect 16:9")
		})
	}
}

func assertHasAction(t *testing.T, actions []Action, cmd string) {
	t.Helper()
	for _, action := range actions {
		if action.Cmd == cmd {
			return
		}
	}
	t.Fatalf("action %q not found in %#v", cmd, actions)
}

func assertNoAction(t *testing.T, actions []Action, cmd string) {
	t.Helper()
	for _, action := range actions {
		if action.Cmd == cmd {
			t.Fatalf("unexpected action %q in %#v", cmd, actions)
		}
	}
}

func assertHasPrefixAction(t *testing.T, actions []Action, prefix string) {
	t.Helper()
	for _, action := range actions {
		if strings.HasPrefix(action.Cmd, prefix) {
			return
		}
	}
	t.Fatalf("action prefix %q not found in %#v", prefix, actions)
}

func assertNoPrefixAction(t *testing.T, actions []Action, prefix string) {
	t.Helper()
	for _, action := range actions {
		if strings.HasPrefix(action.Cmd, prefix) {
			t.Fatalf("unexpected action prefix %q in %#v", prefix, actions)
		}
	}
}

func assertHasActionContaining(t *testing.T, actions []Action, needle string) {
	t.Helper()
	for _, action := range actions {
		if strings.Contains(action.Cmd, needle) {
			return
		}
	}
	t.Fatalf("action containing %q not found in %#v", needle, actions)
}

func assertNoActionContaining(t *testing.T, actions []Action, needle string) {
	t.Helper()
	for _, action := range actions {
		if strings.Contains(action.Cmd, needle) {
			t.Fatalf("unexpected action containing %q found: %#v", needle, action)
		}
	}
}

func mustFindAction(t *testing.T, actions []Action, cmd string) Action {
	t.Helper()
	for _, action := range actions {
		if action.Cmd == cmd {
			return action
		}
	}
	t.Fatalf("action %q not found in %#v", cmd, actions)
	return Action{}
}

func takeName(index int) string {
	return fmt.Sprintf("take%04d", index-1)
}
