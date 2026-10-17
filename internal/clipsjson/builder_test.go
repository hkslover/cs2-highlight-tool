package clipsjson

import (
	"fmt"
	"math"
	"strconv"
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
	// Suppressing the end-of-match scene is unconditional: there is no settings
	// switch, so every demo bootstrap must carry the command.
	assertHasAction(t, bootstrap, "mirv_endofmatch 0")
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

func TestBuild_ClampsRecordingWindowsToMatchEndTick(t *testing.T) {
	// killer post window (200+4*64=456) and victim post window (200+2*64=328)
	// both overshoot a win panel at tick 220 without the clamp.
	result, err := Build([]Item{{
		Kill:              demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11},
		IncludeVictim:     true,
		KillerPostSeconds: 4,
		VictimPostSeconds: 2,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 4,
		VictimPreSeconds:  1,
		VictimPostSeconds: 2,
		MatchEndTick:      220,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.TakePlans) != 2 {
		t.Fatalf("take plans len=%d want 2", len(result.TakePlans))
	}
	// matchEndSafetySec=0.25 * 64 ticks/s = 16 ticks of safety margin before tick 220.
	const wantBoundTick = 220 - 16
	for _, plan := range result.TakePlans {
		if plan.EndTick > wantBoundTick {
			t.Fatalf("view=%s end tick=%d want <= %d (match end bound)", plan.View, plan.EndTick, wantBoundTick)
		}
		if plan.EndTick >= 220 {
			t.Fatalf("view=%s end tick=%d bled into match end tick 220", plan.View, plan.EndTick)
		}
	}
}

func TestBuild_MatchEndClampNeverCutsOffTheKillItself(t *testing.T) {
	// The kill happens only 2 ticks before the match end tick, well inside the
	// safety margin; the clip must still include the kill moment rather than
	// end before it.
	result, err := Build([]Item{{
		Kill:              demo.ClipKill{ID: "k1", Tick: 500, KillerSlot: 7, VictimSlot: 11},
		IncludeVictim:     true,
		KillerPostSeconds: 4,
		VictimPostSeconds: 2,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 4,
		VictimPreSeconds:  1,
		VictimPostSeconds: 2,
		MatchEndTick:      502,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, plan := range result.TakePlans {
		if plan.EndTick < 500 {
			t.Fatalf("view=%s end tick=%d cut off before the kill at tick 500", plan.View, plan.EndTick)
		}
	}
}

func TestBuild_NoMatchEndTickLeavesRecordingWindowUnclamped(t *testing.T) {
	result, err := Build([]Item{{
		Kill:              demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
		IncludeVictim:     false,
		KillerPostSeconds: 4,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 4,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.TakePlans) != 1 {
		t.Fatalf("take plans len=%d want 1", len(result.TakePlans))
	}
	if want := 200 + int(4*64); result.TakePlans[0].EndTick != want {
		t.Fatalf("end tick=%d want %d", result.TakePlans[0].EndTick, want)
	}
}

func TestBuild_HidePlayerAvatarsCommandReflectsSetting(t *testing.T) {
	baseOptions := BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
	}
	items := []Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
		IncludeVictim: false,
	}}

	enabledOptions := baseOptions
	enabledOptions.HidePlayerAvatars = true
	enabled, err := Build(items, enabledOptions)
	if err != nil {
		t.Fatalf("Build enabled hide_player_avatars: %v", err)
	}
	assertHasAction(t, enabled.Sequences[0].Actions, "cl_teamcounter_playercount_instead_of_avatars true")
	assertNoAction(t, enabled.Sequences[0].Actions, "cl_teamcounter_playercount_instead_of_avatars false")

	disabled, err := Build(items, baseOptions)
	if err != nil {
		t.Fatalf("Build disabled hide_player_avatars: %v", err)
	}
	assertHasAction(t, disabled.Sequences[0].Actions, "cl_teamcounter_playercount_instead_of_avatars false")
	assertNoAction(t, disabled.Sequences[0].Actions, "cl_teamcounter_playercount_instead_of_avatars true")
}

func TestBuild_FullRoundPOVHideChatMessagesOnlyInBootstrap(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		result, err := Build(nil, BuildOptions{
			HideChatMessages: enabled,
			VideoPreset:      "c1",
			FullRoundPOVSegments: []FullRoundPOVSegment{
				{Round: 1, StartTick: 200, EndTick: 600, Target: "7", SpecMode: 1, PlayerSteamID: "123"},
			},
		})
		if err != nil {
			t.Fatalf("Build enabled=%v: %v", enabled, err)
		}
		count := 0
		for sequenceIndex, sequence := range result.Sequences {
			for _, action := range sequence.Actions {
				if !strings.HasPrefix(action.Cmd, "hidehud") {
					continue
				}
				count++
				if !enabled || sequenceIndex != 0 || action.Tick != DefaultActionTick || action.Cmd != "hidehud 128" {
					t.Fatalf("unexpected hidehud action in sequence %d: %+v", sequenceIndex, action)
				}
			}
		}
		wantCount := 0
		if enabled {
			wantCount = 1
		}
		if count != wantCount {
			t.Fatalf("hidehud count=%d want %d", count, wantCount)
		}
	}
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

func TestBuild_PovRadarEnabledOnlyInMaterialPassAfterCameraSwitch(t *testing.T) {
	result, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 500, KillerSlot: 7},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		PovRadarEnabled:   true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Sequences) != 2 {
		t.Fatalf("sequence len=%d want 2", len(result.Sequences))
	}
	// The bootstrap must never enable the radar now; each pass does it itself.
	assertNoPrefixAction(t, result.Sequences[0].Actions, "csdm_radar_pov")

	actions := result.Sequences[1].Actions
	specMode := mustFindAction(t, actions, "spec_mode 1")
	specPlayer := mustFindAction(t, actions, "spec_player 7")
	radar := mustFindAction(t, actions, "csdm_radar_pov 1")
	start := mustFindAction(t, actions, "mirv_streams record start")
	end := mustFindAction(t, actions, "mirv_streams record end")
	if !(specMode.Tick < specPlayer.Tick && specPlayer.Tick < radar.Tick && radar.Tick < start.Tick && start.Tick <= end.Tick) {
		t.Fatalf("unexpected ordering: mode=%d player=%d radar=%d start=%d end=%d", specMode.Tick, specPlayer.Tick, radar.Tick, start.Tick, end.Tick)
	}
	// 64 tick/s, kill=500, pre/post=1s: S=436, E=564.
	if specMode.Tick != 311 || specPlayer.Tick != 316 || radar.Tick != 318 || start.Tick != 446 || end.Tick != 565 {
		t.Fatalf("unexpected ticks: mode=%d player=%d radar=%d start=%d end=%d", specMode.Tick, specPlayer.Tick, radar.Tick, start.Tick, end.Tick)
	}
	assertHasAction(t, actions, "demo_gototick 183")
	if warmup := start.Tick - radar.Tick; warmup < int(math.Ceil(povRadarWarmupSeconds*64)) {
		t.Fatalf("radar warmup=%d ticks want >= %d", warmup, int(math.Ceil(povRadarWarmupSeconds*64)))
	}

	if len(result.TakePlans) != 1 {
		t.Fatalf("take plans len=%d want 1", len(result.TakePlans))
	}
	plan := result.TakePlans[0]
	if plan.StartTick != 436 || plan.EndTick != 564 {
		t.Fatalf("source window changed: %+v", plan)
	}
	if plan.RecordStartTick != start.Tick || plan.RecordEndTick != end.Tick {
		t.Fatalf("take plan boundaries must match the record actions: plan=%+v start=%d end=%d", plan, start.Tick, end.Tick)
	}
}

func TestBuild_PovRadarDisabledKeepsLegacyTimingAndGrouping(t *testing.T) {
	items := []Item{
		{Kill: demo.ClipKill{ID: "k1", Tick: 500, KillerSlot: 7}, IncludeVictim: false},
		{Kill: demo.ClipKill{ID: "k2", Tick: 650, KillerSlot: 9}, IncludeVictim: false},
	}
	baseOptions := BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
	}

	off, err := Build(items, baseOptions)
	if err != nil {
		t.Fatalf("Build radar off: %v", err)
	}
	if len(off.Sequences) != 2 {
		t.Fatalf("radar off sequence len=%d want 2 (bootstrap + merged killer group)", len(off.Sequences))
	}
	for _, seq := range off.Sequences {
		assertNoPrefixAction(t, seq.Actions, "csdm_radar_pov")
	}
	assertHasAction(t, off.Sequences[1].Actions, "demo_gototick 308")
	assertHasAction(t, off.Sequences[1].Actions, "demo_gototick 565")

	onOptions := baseOptions
	onOptions.PovRadarEnabled = true
	on, err := Build(items, onOptions)
	if err != nil {
		t.Fatalf("Build radar on: %v", err)
	}
	if len(on.Sequences) != 3 {
		t.Fatalf("radar on sequence len=%d want 3 (bootstrap + one sequence per pass)", len(on.Sequences))
	}
	if len(on.TakePlans) != len(off.TakePlans) {
		t.Fatalf("radar changed take count: off=%d on=%d", len(off.TakePlans), len(on.TakePlans))
	}
	for i := range off.TakePlans {
		if off.TakePlans[i].StartTick != on.TakePlans[i].StartTick || off.TakePlans[i].EndTick != on.TakePlans[i].EndTick {
			t.Fatalf("radar changed take %d source window: off=%+v on=%+v", i, off.TakePlans[i], on.TakePlans[i])
		}
		if off.TakePlans[i].RecordStartTick != on.TakePlans[i].RecordStartTick || off.TakePlans[i].RecordEndTick != on.TakePlans[i].RecordEndTick {
			t.Fatalf("radar changed take %d recorder window: off=%+v on=%+v", i, off.TakePlans[i], on.TakePlans[i])
		}
		if off.TakePlans[i].TakeName != on.TakePlans[i].TakeName {
			t.Fatalf("radar changed take %d identity: off=%q on=%q", i, off.TakePlans[i].TakeName, on.TakePlans[i].TakeName)
		}
	}
}

func TestBuild_PovRadarEmitsOneCommandPerPassEvenForSameTarget(t *testing.T) {
	result, err := Build([]Item{
		{Kill: demo.ClipKill{ID: "k1", Tick: 500, KillerSlot: 7}, IncludeVictim: false},
		{Kill: demo.ClipKill{ID: "k2", Tick: 900, KillerSlot: 7}, IncludeVictim: false},
	}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		PovRadarEnabled:   true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Sequences) != 3 {
		t.Fatalf("sequence len=%d want 3", len(result.Sequences))
	}
	for i, seq := range result.Sequences[1:] {
		count := 0
		for _, action := range seq.Actions {
			if action.Cmd == "csdm_radar_pov 1" {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("pass %d radar command count=%d want 1: %+v", i, count, seq.Actions)
		}
	}
	if len(result.TakePlans) != 2 || result.TakePlans[0].TakeName != "take0000" || result.TakePlans[1].TakeName != "take0001" {
		t.Fatalf("unexpected take identity: %+v", result.TakePlans)
	}
}

func TestBuild_PovRadarSeparatesAdjacentPassesAcrossSequences(t *testing.T) {
	result, err := Build([]Item{
		{Kill: demo.ClipKill{ID: "k1", Tick: 500, KillerSlot: 7}, IncludeVictim: false},
		{Kill: demo.ClipKill{ID: "k2", Tick: 560, KillerSlot: 9}, IncludeVictim: false},
	}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		PovRadarEnabled:   true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Two overlapping windows would be merged into one group without the radar;
	// enabling it must not merge or reorder them.
	if len(result.Sequences) != 3 {
		t.Fatalf("sequence len=%d want 3", len(result.Sequences))
	}
	for _, seq := range result.Sequences[1:] {
		assertHasAction(t, seq.Actions, "csdm_radar_pov 1")
		assertHasAction(t, seq.Actions, "mirv_streams record start")
	}
	assertHasAction(t, result.Sequences[1].Actions, "go_to_next_sequence")
	assertHasAction(t, result.Sequences[2].Actions, "disconnect")
	if len(result.TakePlans) != 2 {
		t.Fatalf("take plans len=%d want 2: %+v", len(result.TakePlans), result.TakePlans)
	}
	for i, seq := range result.Sequences[1:] {
		startAction := mustFindAction(t, seq.Actions, "mirv_streams record start")
		radarAction := mustFindAction(t, seq.Actions, "csdm_radar_pov 1")
		if warmup := startAction.Tick - radarAction.Tick; warmup < int(math.Ceil(povRadarWarmupSeconds*64)) {
			t.Fatalf("pass %d warmup=%d ticks want >= %d", i, warmup, int(math.Ceil(povRadarWarmupSeconds*64)))
		}
	}
}

func TestBuild_PovRadarCoversFullRoundKillerAndVictimPasses(t *testing.T) {
	result, err := Build([]Item{
		{Kill: demo.ClipKill{ID: "k1", Tick: 600, KillerSlot: 7, VictimSlot: 11}, IncludeVictim: true},
	}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		VictimPreSeconds:  1,
		VictimPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		PovRadarEnabled:   true,
		FullRoundPOVSegments: []FullRoundPOVSegment{
			{Round: 1, StartTick: 1000, EndTick: 1800, Target: "12", SpecMode: 1, PlayerSteamID: "76561190000000001"},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Sequences) != 4 {
		t.Fatalf("sequence len=%d want 4 (bootstrap + three passes)", len(result.Sequences))
	}
	views := []string{"full_round_pov", "killer", "victim"}
	for i, wantView := range views {
		seq := result.Sequences[i+1]
		assertHasAction(t, seq.Actions, "csdm_radar_pov 1")
		if result.TakePlans[i].View != wantView {
			t.Fatalf("take %d view=%q want %q", i, result.TakePlans[i].View, wantView)
		}
	}
	for _, seq := range result.Sequences[1:3] {
		assertHasAction(t, seq.Actions, "go_to_next_sequence")
	}
	assertHasAction(t, result.Sequences[3].Actions, "disconnect")
}

func TestBuild_PovRadarMergedKillStaysOnePassAndOneCommand(t *testing.T) {
	result, err := Build([]Item{
		{Kill: demo.ClipKill{ID: "k1", Tick: 500, KillerSlot: 7}, IncludeVictim: false},
		{Kill: demo.ClipKill{ID: "k2", Tick: 520, KillerSlot: 7}, IncludeVictim: false},
	}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		PovRadarEnabled:   true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Sequences) != 2 {
		t.Fatalf("sequence len=%d want 2 (merged pass kept together)", len(result.Sequences))
	}
	count := 0
	for _, action := range result.Sequences[1].Actions {
		if action.Cmd == "csdm_radar_pov 1" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("radar command count=%d want 1", count)
	}
	if len(result.TakePlans) != 1 {
		t.Fatalf("take plans len=%d want 1", len(result.TakePlans))
	}
	if ids := result.TakePlans[0].KillIDs; len(ids) != 2 || ids[0] != "k1" || ids[1] != "k2" {
		t.Fatalf("merged kill ids changed: %+v", ids)
	}
}

func TestBuild_PovRadarWarmupUsesDemoTickRate(t *testing.T) {
	tests := []struct {
		name     string
		tickRate float64
		killTick int
	}{
		{name: "64 tick", tickRate: 64, killTick: 500},
		{name: "128 tick", tickRate: 128, killTick: 1000},
		{name: "non-integer tick", tickRate: 70.5, killTick: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Build([]Item{{
				Kill:          demo.ClipKill{ID: "k1", Tick: tt.killTick, KillerSlot: 7},
				IncludeVictim: false,
			}}, BuildOptions{
				TickRate:          tt.tickRate,
				KillerPreSeconds:  1,
				KillerPostSeconds: 1,
				RecordFPS:         60,
				VideoPreset:       "n1",
				RecordOutputDir:   `D:/clips/output`,
				PovRadarEnabled:   true,
			})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			actions := result.Sequences[1].Actions
			radar := mustFindAction(t, actions, "csdm_radar_pov 1")
			start := mustFindAction(t, actions, "mirv_streams record start")
			want := int(math.Ceil(povRadarWarmupSeconds * tt.tickRate))
			if warmup := start.Tick - radar.Tick; warmup < want {
				t.Fatalf("warmup=%d ticks want >= %d (tickRate=%v)", warmup, want, tt.tickRate)
			}
			assertNoPrefixAction(t, result.Sequences[0].Actions, "csdm_radar_pov")
		})
	}
}

func TestBuild_PovRadarInsufficientDemoStartReturnsError(t *testing.T) {
	// 64 tick/s, pre=1s: StartTick=223 -> spec_mode tick 98, one tick below the
	// DefaultActionTick+segmentSpecModeDelay safety floor.
	_, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 287, KillerSlot: 7},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		PovRadarEnabled:   true,
	})
	if err == nil {
		t.Fatal("expected insufficient demo start error")
	}
	if !strings.Contains(err.Error(), "POV 雷达") || !strings.Contains(err.Error(), "kill_ids=k1") {
		t.Fatalf("error must name the radar warm-up and the pass: %v", err)
	}
}

func TestBuild_PovRadarBoundaryExactlyFeasibleSucceeds(t *testing.T) {
	// 64 tick/s, pre=1s: StartTick=224 -> spec_mode tick 99, exactly at the floor.
	result, err := Build([]Item{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 288, KillerSlot: 7},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		PovRadarEnabled:   true,
	})
	if err != nil {
		t.Fatalf("Build at boundary: %v", err)
	}
	actions := result.Sequences[1].Actions
	specMode := mustFindAction(t, actions, "spec_mode 1")
	if specMode.Tick != DefaultActionTick+segmentSpecModeDelay {
		t.Fatalf("spec_mode tick=%d want %d", specMode.Tick, DefaultActionTick+segmentSpecModeDelay)
	}
	assertNoPrefixAction(t, actions, "demo_gototick")
	radar := mustFindAction(t, actions, "csdm_radar_pov 1")
	start := mustFindAction(t, actions, "mirv_streams record start")
	if warmup := start.Tick - radar.Tick; warmup < 128 {
		t.Fatalf("warmup=%d ticks want >= 128", warmup)
	}
}

func TestBuild_PovRadarEarlySeekDoesNotRepeatDuringPlayback(t *testing.T) {
	for _, sourceStart := range []int{224, 300, 349, 350} {
		t.Run(strconv.Itoa(sourceStart), func(t *testing.T) {
			result, err := Build(nil, BuildOptions{
				TickRate:        64,
				VideoPreset:     "c1",
				PovRadarEnabled: true,
				FullRoundPOVSegments: []FullRoundPOVSegment{
					{StartTick: 1000, EndTick: 1100, Target: "7", SpecMode: 1},
					{StartTick: sourceStart, EndTick: sourceStart + 100, Target: "9", SpecMode: 1},
				},
			})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			// At S=349 the proposed seek target is exactly its trigger tick
			// (96); S=350 is the first case that can safely seek forward.
			earlyActions := result.Sequences[2].Actions
			if sourceStart <= 349 {
				assertNoPrefixAction(t, earlyActions, "demo_gototick")
			} else {
				assertHasAction(t, earlyActions, "demo_gototick 97")
			}

			// Model the plugin's tick-crossing consumer, including asynchronous
			// seeks and the reset to 0 between sequences. Do not mark actions as
			// consumed: a backward seek must expose repeat execution here.
			sequenceIndex, previousTick, tick := 0, -1, 0
			starts, ends, radars := 0, 0, 0
			for frame := 0; frame < 5000 && sequenceIndex < len(result.Sequences); frame++ {
				nextTick := tick + 1
				nextSequence := false
				for _, action := range result.Sequences[sequenceIndex].Actions {
					if action.Tick > tick || action.Tick <= previousTick {
						continue
					}
					switch {
					case strings.HasPrefix(action.Cmd, "demo_gototick "):
						var parseErr error
						nextTick, parseErr = strconv.Atoi(strings.TrimPrefix(action.Cmd, "demo_gototick "))
						if parseErr != nil {
							t.Fatal(parseErr)
						}
					case action.Cmd == "csdm_radar_pov 1":
						radars++
					case action.Cmd == "mirv_streams record start":
						starts++
					case action.Cmd == "mirv_streams record end":
						ends++
					case action.Cmd == "go_to_next_sequence" || action.Cmd == "disconnect":
						nextSequence = true
					}
					if nextSequence {
						break
					}
				}
				previousTick, tick = tick, nextTick
				if nextSequence {
					sequenceIndex++
					previousTick, tick = -1, 0
				}
			}
			if sequenceIndex != len(result.Sequences) || starts != 2 || ends != 2 || radars != 2 {
				t.Fatalf("playback did not complete exactly once per pass: sequence=%d/%d starts=%d ends=%d radars=%d",
					sequenceIndex, len(result.Sequences), starts, ends, radars)
			}
			plan := result.TakePlans[1]
			if plan.RecordStartTick != sourceStart+10 || plan.RecordEndTick != sourceStart+101 {
				t.Fatalf("early pass recording window changed: %+v", plan)
			}
			start := mustFindAction(t, earlyActions, "mirv_streams record start")
			radar := mustFindAction(t, earlyActions, "csdm_radar_pov 1")
			if start.Tick-radar.Tick != 128 {
				t.Fatalf("early pass warmup=%d want 128", start.Tick-radar.Tick)
			}
		})
	}
}

func TestBuild_PovRadarKeepsVoiceAndXrayOverridesBeforeRecording(t *testing.T) {
	result, err := Build([]Item{
		{
			Kill:                    demo.ClipKill{ID: "k1", Tick: 500, KillerSlot: 7},
			IncludeVictim:           false,
			EnableVoice:             false,
			EnableSpecShowXray:      false,
			HasVoiceOverride:        true,
			HasSpecShowXrayOverride: true,
		},
		{
			Kill:          demo.ClipKill{ID: "k2", Tick: 900, KillerSlot: 9},
			IncludeVictim: false,
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
		PovRadarEnabled:           true,
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
	for i, seq := range result.Sequences[1:] {
		start := mustFindAction(t, seq.Actions, "mirv_streams record start")
		voice := mustFindAction(t, seq.Actions, "tv_listen_voice_indices 0")
		xray := mustFindAction(t, seq.Actions, "spec_show_xray -1")
		if voice.Tick >= start.Tick || xray.Tick >= start.Tick {
			t.Fatalf("pass %d override commands must precede recording: voice=%d xray=%d start=%d", i, voice.Tick, xray.Tick, start.Tick)
		}
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

func TestBuild_NoFlashEmitsStrengthOnlyWhenEnabled(t *testing.T) {
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
		NoFlashStrength:   0.35,
		KillFeedLifetime:  4,
	})
	if err != nil {
		t.Fatalf("Build noflash=0.35: %v", err)
	}
	assertHasAction(t, enabled.Sequences[0].Actions, "mirv_noflash 0.35")

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
		NoFlashStrength:   0,
		KillFeedLifetime:  4,
	})
	if err != nil {
		t.Fatalf("Build noflash=0: %v", err)
	}
	assertNoPrefixAction(t, disabled.Sequences[0].Actions, "mirv_noflash")
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

func TestBuild_PovKillFeedOnlyFiltersEachPassBySpectatedPlayer(t *testing.T) {
	result, err := Build([]Item{
		{
			Kill: demo.ClipKill{
				ID: "k1", Tick: 200,
				KillerSlot: 7, KillerSteamID: "76561190000000001",
				VictimSlot: 11, VictimSteamID: "76561190000000002",
			},
			IncludeVictim: true,
		},
		{
			Kill: demo.ClipKill{
				ID: "k2", Tick: 400,
				KillerSlot: 9, KillerSteamID: "76561190000000003",
				VictimSlot: 12, VictimSteamID: "76561190000000004",
			},
			IncludeVictim: false,
		},
	}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		VictimPreSeconds:  1,
		VictimPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		PovKillFeedOnly:   true,
		KillFeedLifetime:  4,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Sequences) != 3 {
		t.Fatalf("sequence len=%d want bootstrap + killer group + one victim sequence", len(result.Sequences))
	}

	bootstrap := result.Sequences[0].Actions
	assertNoPrefixAction(t, bootstrap, "mirv_deathmsg clear")
	assertNoPrefixAction(t, bootstrap, "mirv_deathmsg filter add attackerMatch=")

	killerActions := result.Sequences[1].Actions
	assertHasAction(t, killerActions, "mirv_deathmsg filter add attackerMatch=!x76561190000000001 victimMatch=!x76561190000000001 assisterMatch=!x76561190000000001 block=1 lastRule=1")
	assertHasAction(t, killerActions, "mirv_deathmsg filter add attackerMatch=!x76561190000000003 victimMatch=!x76561190000000003 assisterMatch=!x76561190000000003 block=1 lastRule=1")
	assertNoActionContaining(t, killerActions, "!x76561190000000002")
	assertNoActionContaining(t, killerActions, "!x76561190000000004")
	if clears := countActions(killerActions, "mirv_deathmsg filter clear"); clears != 2 {
		t.Fatalf("killer group filter clears=%d want one per pass", clears)
	}

	victimActions := result.Sequences[2].Actions
	assertHasAction(t, victimActions, "mirv_deathmsg filter add attackerMatch=!x76561190000000002 victimMatch=!x76561190000000002 assisterMatch=!x76561190000000002 block=1 lastRule=1")

	// The filter has to be in place before the recorder starts and after the
	// camera switch, otherwise the take can still show other players' notices
	// or keep the previous pass's rule.
	spec := mustFindAction(t, killerActions, "spec_player 7")
	filter := mustFindAction(t, killerActions, "mirv_deathmsg filter add attackerMatch=!x76561190000000001 victimMatch=!x76561190000000001 assisterMatch=!x76561190000000001 block=1 lastRule=1")
	if filter.Tick != spec.Tick {
		t.Fatalf("filter tick=%d want spec_player tick=%d", filter.Tick, spec.Tick)
	}
	if actionIndex(killerActions, "spec_player 7") > actionIndex(killerActions, "mirv_deathmsg filter clear") {
		t.Fatalf("filter must be applied after spec_player: %#v", killerActions)
	}
	start := mustFindAction(t, killerActions, "mirv_streams record start")
	if filter.Tick >= start.Tick {
		t.Fatalf("filter tick=%d must precede record start=%d", filter.Tick, start.Tick)
	}
}

func TestBuild_PovKillFeedOnlyUsesFullRoundPOVPlayer(t *testing.T) {
	result, err := Build(nil, BuildOptions{
		TickRate:         64,
		RecordFPS:        60,
		VideoPreset:      "n1",
		RecordOutputDir:  `D:/clips/output`,
		PovKillFeedOnly:  true,
		KillFeedLifetime: 4,
		FullRoundPOVSegments: []FullRoundPOVSegment{{
			Round:         1,
			StartTick:     200,
			EndTick:       400,
			Target:        "12",
			SpecMode:      1,
			SourceID:      "full_round_pov:r1:p76561190000000009",
			PlayerSteamID: "76561190000000009",
		}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Sequences) != 2 {
		t.Fatalf("sequence len=%d want bootstrap + full-round POV", len(result.Sequences))
	}
	assertHasAction(t, result.Sequences[1].Actions, "mirv_deathmsg filter add attackerMatch=!x76561190000000009 victimMatch=!x76561190000000009 assisterMatch=!x76561190000000009 block=1 lastRule=1")
}

func TestBuild_BlockKillFeedWinsOverPovKillFeedOnly(t *testing.T) {
	result, err := Build([]Item{{
		Kill: demo.ClipKill{
			ID: "k1", Tick: 200,
			KillerSlot: 7, KillerSteamID: "76561190000000001",
			VictimSlot: 11, VictimSteamID: "76561190000000002",
		},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		PovKillFeedOnly:   true,
		BlockKillFeed:     true,
		KillFeedLifetime:  4,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	assertHasAction(t, result.Sequences[0].Actions, "mirv_deathmsg filter add block 1")
	// A per-pass clear would wipe the bootstrap block rule, so the stronger
	// switch must suppress the POV filter entirely.
	for _, sequence := range result.Sequences[1:] {
		assertNoPrefixAction(t, sequence.Actions, "mirv_deathmsg clear")
		assertNoPrefixAction(t, sequence.Actions, "mirv_deathmsg filter ")
	}
}

func TestBuild_PovKillFeedOnlyClearsRulesWithoutAnXuid(t *testing.T) {
	result, err := Build([]Item{{
		Kill: demo.ClipKill{
			ID: "k1", Tick: 200, KillerSlot: 7,
			// A bot/synthetic entity has no Steam64; the pass must not keep the
			// previous rule and hide the new perspective's notices.
			VictimSlot: 11, VictimSteamID: "0",
		},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		PovKillFeedOnly:   true,
		KillFeedLifetime:  4,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	assertHasAction(t, result.Sequences[1].Actions, "mirv_deathmsg filter clear")
	assertNoPrefixAction(t, result.Sequences[1].Actions, "mirv_deathmsg filter add attackerMatch=")
}

func TestBuild_PovKillFeedOnlyOffEmitsNoDeathNoticeCommands(t *testing.T) {
	result, err := Build([]Item{{
		Kill: demo.ClipKill{
			ID: "k1", Tick: 200,
			KillerSlot: 7, KillerSteamID: "76561190000000001",
			VictimSlot: 11, VictimSteamID: "76561190000000002",
		},
		IncludeVictim: false,
	}}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   `D:/clips/output`,
		PovKillFeedOnly:   false,
		KillFeedLifetime:  4,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, sequence := range result.Sequences {
		assertNoPrefixAction(t, sequence.Actions, "mirv_deathmsg clear")
		assertNoPrefixAction(t, sequence.Actions, "mirv_deathmsg filter ")
	}
}

func countActions(actions []Action, cmd string) int {
	count := 0
	for _, action := range actions {
		if action.Cmd == cmd {
			count++
		}
	}
	return count
}

func actionIndex(actions []Action, cmd string) int {
	for i, action := range actions {
		if action.Cmd == cmd {
			return i
		}
	}
	return -1
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

func TestBuild_TakePlanCarriesRecorderOffsetsWithoutChangingWindow(t *testing.T) {
	includeKiller := false
	result, err := Build([]Item{
		{
			Kill:          demo.ClipKill{ID: "victim-kill", Round: 7, Tick: 200, KillerSlot: 7, VictimSlot: 11},
			IncludeKiller: &includeKiller,
			IncludeVictim: true,
		},
	}, BuildOptions{
		TickRate:          64,
		VictimPreSeconds:  1,
		VictimPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "c1",
		RecordOutputDir:   `D:/clips/output`,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.TakePlans) != 1 {
		t.Fatalf("take plans len=%d want 1", len(result.TakePlans))
	}
	plan := result.TakePlans[0]
	if plan.View != "victim" || plan.Round != 7 || plan.StartTick != 136 || plan.EndTick != 264 {
		t.Fatalf("unexpected source event window: %+v", plan)
	}
	if plan.TickRate != 64 || plan.RecordStartTick != 146 || plan.RecordEndTick != 265 {
		t.Fatalf("unexpected recorder metadata: %+v", plan)
	}
	if len(plan.KillOffsetsSeconds) != 1 || math.Abs(plan.KillOffsetsSeconds[0]-0.84375) > 1e-9 {
		t.Fatalf("unexpected kill offsets: %+v", plan.KillOffsetsSeconds)
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

func TestBuild_StretchesFourThreeLaunchResolutionToSquarePixels(t *testing.T) {
	tests := []struct {
		name             string
		launchResolution string
		wantStretch      bool
	}{
		{name: "4:3 1440x1080", launchResolution: "4:3", wantStretch: true},
		{name: "4:3 1280x960", launchResolution: "4:3_1280x960", wantStretch: true},
		{name: "16:9", launchResolution: "16:9", wantStretch: false},
		{name: "empty", launchResolution: "", wantStretch: false},
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
			if tt.wantStretch {
				assertHasActionContaining(t, bootstrap, "-vf scale=trunc(ih*16/9/2)*2:ih")
			} else {
				assertNoActionContaining(t, bootstrap, "-vf scale=trunc(ih*16/9/2)*2:ih")
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

func TestBuild_ShortClipKeepsRecordEndBeforeTerminal(t *testing.T) {
	for _, radarEnabled := range []bool{false, true} {
		result, err := Build([]Item{{
			Kill:          demo.ClipKill{ID: "k1", Tick: 500, KillerSlot: 7},
			IncludeVictim: false,
		}}, BuildOptions{
			TickRate:          64,
			KillerPreSeconds:  0,
			KillerPostSeconds: 0,
			RecordFPS:         60,
			VideoPreset:       "n1",
			RecordOutputDir:   `D:/clips/output`,
			PovRadarEnabled:   radarEnabled,
		})
		if err != nil {
			t.Fatalf("Build radar=%v: %v", radarEnabled, err)
		}
		actions := result.Sequences[len(result.Sequences)-1].Actions
		start := mustFindAction(t, actions, "mirv_streams record start")
		end := mustFindAction(t, actions, "mirv_streams record end")
		terminal := mustFindAction(t, actions, "disconnect")
		if end.Tick < start.Tick {
			t.Fatalf("radar=%v record end=%d before start=%d", radarEnabled, end.Tick, start.Tick)
		}
		if terminal.Tick < end.Tick {
			t.Fatalf("radar=%v terminal=%d before record end=%d", radarEnabled, terminal.Tick, end.Tick)
		}
		endIndex, terminalIndex := -1, -1
		for i, action := range actions {
			switch action.Cmd {
			case "mirv_streams record end":
				endIndex = i
			case "disconnect":
				terminalIndex = i
			}
		}
		if endIndex < 0 || terminalIndex < 0 || endIndex > terminalIndex {
			t.Fatalf("radar=%v record end must precede terminal: %+v", radarEnabled, actions)
		}
		if plan := result.TakePlans[0]; plan.RecordStartTick != start.Tick || plan.RecordEndTick != end.Tick {
			t.Fatalf("radar=%v take plan does not match boundaries: %+v", radarEnabled, plan)
		}
	}
}
