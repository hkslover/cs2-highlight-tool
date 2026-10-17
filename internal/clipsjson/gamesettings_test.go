package clipsjson

import (
	"reflect"
	"strings"
	"testing"

	"cs2-highlight-tool-v2/internal/demo"
)

func findDebugSettingPlan(t *testing.T, plans []DebugGameSettingPlan, key string) DebugGameSettingPlan {
	t.Helper()
	for _, plan := range plans {
		if plan.Key == key {
			return plan
		}
	}
	t.Fatalf("debug setting plan %q not found in %#v", key, plans)
	return DebugGameSettingPlan{}
}

func TestDebugGameSettingPlanSwitchVariants(t *testing.T) {
	plans, _ := BuildDebugGameSettingPlan(GameSettings{
		SkyBlackout:        false,
		DisableClouds:      true,
		HideAllUI:          true,
		HidePlayerAvatars:  false,
		HideChatMessages:   false,
		EnableSpecShowXray: false,
		EnableVoice:        true,
		UseShoulderCamera:  false,
		PovRadarEnabled:    true,
		BlockKillFeed:      false,
		KillFeedLifetime:   6,
		NoFlashStrength:    0.35,
	})

	cases := []struct {
		key  string
		on   []string
		off  []string
		want []string
	}{
		{
			key:  GameSettingKeySkyBlackout,
			on:   []string{"r_drawskybox 0"},
			off:  []string{"r_drawskybox 1"},
			want: []string{"r_drawskybox 1"},
		},
		{
			key:  GameSettingKeyDisableClouds,
			on:   []string{"mirv_sky clouds draw 0"},
			off:  []string{"mirv_sky clouds draw 1"},
			want: []string{"mirv_sky clouds draw 0"},
		},
		{
			key:  GameSettingKeyPovRadarEnabled,
			on:   []string{"csdm_radar_pov 1"},
			off:  []string{"csdm_radar_pov 0"},
			want: []string{"csdm_radar_pov 1"},
		},
		{
			key:  GameSettingKeyHideAllUI,
			on:   []string{"cl_draw_only_deathnotices 1"},
			off:  []string{"cl_draw_only_deathnotices 0"},
			want: []string{"cl_draw_only_deathnotices 1"},
		},
		{
			key:  GameSettingKeyHidePlayerAvatars,
			on:   []string{"cl_teamcounter_playercount_instead_of_avatars true"},
			off:  []string{"cl_teamcounter_playercount_instead_of_avatars false"},
			want: []string{"cl_teamcounter_playercount_instead_of_avatars false"},
		},
		{
			key:  GameSettingKeyHideChatMessages,
			on:   []string{"hidehud 128"},
			off:  []string{"hidehud 0"},
			want: []string{"hidehud 0"},
		},
		{
			key:  GameSettingKeySpecShowXray,
			on:   []string{"spec_show_xray 0"},
			off:  []string{"spec_show_xray -1"},
			want: []string{"spec_show_xray -1"},
		},
		{
			key:  GameSettingKeyNoFlashStrength,
			on:   []string{"mirv_noflash 0.35"},
			off:  []string{"mirv_noflash 0.00"},
			want: []string{"mirv_noflash 0.35"},
		},
		{
			key:  GameSettingKeyEnableVoice,
			on:   []string{"tv_listen_voice_indices -1", "tv_listen_voice_indices_h -1"},
			off:  []string{"tv_listen_voice_indices 0", "tv_listen_voice_indices_h 0"},
			want: []string{"tv_listen_voice_indices -1", "tv_listen_voice_indices_h -1"},
		},
		{
			key:  GameSettingKeyBlockKillFeed,
			on:   []string{"mirv_deathmsg filter add block 1"},
			off:  []string{"mirv_deathmsg filter clear"},
			want: []string{"mirv_deathmsg filter clear"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.key, func(t *testing.T) {
			plan := findDebugSettingPlan(t, plans, tt.key)
			if !reflect.DeepEqual(plan.On, tt.on) {
				t.Fatalf("on commands=%#v want %#v", plan.On, tt.on)
			}
			if !reflect.DeepEqual(plan.Off, tt.off) {
				t.Fatalf("off commands=%#v want %#v", plan.Off, tt.off)
			}
			if !reflect.DeepEqual(plan.Commands, tt.want) {
				t.Fatalf("current commands=%#v want %#v", plan.Commands, tt.want)
			}
		})
	}

	shoulder := findDebugSettingPlan(t, plans, GameSettingKeyUseShoulderCamera)
	if !reflect.DeepEqual(shoulder.On, []string{shoulderCameraCommand}) {
		t.Fatalf("shoulder on commands=%#v want the produce shoulder setup", shoulder.On)
	}
	if len(shoulder.Off) != 1 ||
		!strings.Contains(shoulder.Off[0], "c_thirdpersonshoulder 0") ||
		!strings.Contains(shoulder.Off[0], "cam_command 2") {
		t.Fatalf("shoulder restore must clear the shoulder flag and switch the camera mode, got %#v", shoulder.Off)
	}

	lifetime := findDebugSettingPlan(t, plans, GameSettingKeyKillFeedLifetime)
	if !reflect.DeepEqual(lifetime.Commands, []string{"mirv_deathmsg lifetime 6"}) {
		t.Fatalf("lifetime commands=%#v", lifetime.Commands)
	}
	if lifetime.On != nil || lifetime.Off != nil {
		t.Fatalf("lifetime must not expose toggle variants: %#v", lifetime)
	}
}

func TestDebugGameSettingPlanKeepsSkyAndCloudsIndependent(t *testing.T) {
	plans, applyAll := BuildDebugGameSettingPlan(GameSettings{
		SkyBlackout:      true,
		DisableClouds:    false,
		KillFeedLifetime: 4,
	})

	sky := findDebugSettingPlan(t, plans, GameSettingKeySkyBlackout)
	clouds := findDebugSettingPlan(t, plans, GameSettingKeyDisableClouds)
	if !reflect.DeepEqual(sky.Commands, []string{"r_drawskybox 0"}) {
		t.Fatalf("sky commands=%#v want sky blackout", sky.Commands)
	}
	if !reflect.DeepEqual(clouds.Commands, []string{"mirv_sky clouds draw 1"}) {
		t.Fatalf("cloud commands=%#v want the independent cloud restore", clouds.Commands)
	}
	for _, cmd := range append(append([]string(nil), sky.Commands...), clouds.Commands...) {
		if strings.Contains(cmd, "mirv_sky") && strings.Contains(cmd, "r_drawskybox") {
			t.Fatalf("unexpected coupled command %q", cmd)
		}
	}

	// Apply-all must respect item order and include each item's current
	// variant, not both variants.
	wantApplyAll := make([]string, 0, sumPlanCommands(plans))
	for _, plan := range plans {
		wantApplyAll = append(wantApplyAll, plan.Commands...)
	}
	if !reflect.DeepEqual(applyAll, wantApplyAll) {
		t.Fatalf("apply all=%#v want %#v", applyAll, wantApplyAll)
	}
	order := make([]string, 0, len(plans))
	for _, plan := range plans {
		order = append(order, plan.Key)
	}
	wantOrder := []string{
		GameSettingKeyUseShoulderCamera,
		GameSettingKeyDisableClouds,
		GameSettingKeySkyBlackout,
		GameSettingKeyPovRadarEnabled,
		GameSettingKeyKillFeedLifetime,
		GameSettingKeyNoFlashStrength,
		GameSettingKeyBlockKillFeed,
		GameSettingKeySpecShowXray,
		GameSettingKeyHideAllUI,
		GameSettingKeyHidePlayerAvatars,
		GameSettingKeyHideChatMessages,
		GameSettingKeyEnableVoice,
	}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("plan order=%#v want %#v", order, wantOrder)
	}
}

func sumPlanCommands(plans []DebugGameSettingPlan) int {
	total := 0
	for _, plan := range plans {
		total += len(plan.Commands)
	}
	return total
}

func TestNoFlashCommandFormatsStrengthAndKeepsRestoreExplicit(t *testing.T) {
	cases := []struct {
		strength float64
		want     string
	}{
		{strength: 0, want: "mirv_noflash 0.00"},
		{strength: 0.3, want: "mirv_noflash 0.30"},
		{strength: 0.355, want: "mirv_noflash 0.36"},
		{strength: 1, want: "mirv_noflash 1.00"},
		{strength: 2, want: "mirv_noflash 1.00"},
		{strength: -1, want: "mirv_noflash 0.00"},
	}
	for _, tt := range cases {
		if got := NoFlashCommand(tt.strength); got != tt.want {
			t.Fatalf("NoFlashCommand(%v)=%q want %q", tt.strength, got, tt.want)
		}
	}
}

func TestDebugGameSettingPlanZeroNoFlashStrengthKeepsRestoreOnly(t *testing.T) {
	plans, applyAll := BuildDebugGameSettingPlan(GameSettings{KillFeedLifetime: 4})

	noFlash := findDebugSettingPlan(t, plans, GameSettingKeyNoFlashStrength)
	if !reflect.DeepEqual(noFlash.Commands, []string{"mirv_noflash 0.00"}) {
		t.Fatalf("current commands=%#v want the explicit zero restore", noFlash.Commands)
	}
	if !reflect.DeepEqual(noFlash.Off, noFlash.On) {
		t.Fatalf("a zero strength must not invent a different on variant: %#v", noFlash)
	}
	if !reflect.DeepEqual(applyAll, concatPlanCommands(plans)) {
		t.Fatalf("apply all=%#v want plan order", applyAll)
	}
}

func concatPlanCommands(plans []DebugGameSettingPlan) []string {
	commands := make([]string, 0, sumPlanCommands(plans))
	for _, plan := range plans {
		commands = append(commands, plan.Commands...)
	}
	return commands
}

func TestKillFeedLifetimeCommandFallsBackForUnsetValue(t *testing.T) {
	if got := KillFeedLifetimeCommand(0); got != "mirv_deathmsg lifetime 4" {
		t.Fatalf("unset lifetime=%q want the default lifetime", got)
	}
	if got := KillFeedLifetimeCommand(3); got != "mirv_deathmsg lifetime 3" {
		t.Fatalf("lifetime=%q", got)
	}
}

func TestRecordInitCommandsMatchProduceBootstrap(t *testing.T) {
	const recordName = `D:/clips/output/20260422_131500`
	init := RecordInitCommands(RecordInitOptions{
		RecordFPS:    60,
		VideoPreset:  "n1",
		FFmpegParams: "-c:v hevc_nvenc",
		RecordName:   recordName,
	})
	want := []string{
		"mirv_endofmatch 0",
		"mirv_streams record screen enabled 1",
		"mirv_streams record fps 60",
		"mirv_streams record startMovieWav 1",
		`mirv_streams settings add ffmpeg n1 "-c:v hevc_nvenc {QUOTE}{AFX_STREAM_PATH}.mp4{QUOTE}"`,
		"mirv_streams record screen settings n1",
		`mirv_streams record name "D:/clips/output/20260422_131500"`,
	}
	if !reflect.DeepEqual(init, want) {
		t.Fatalf("record init commands=%#v want %#v", init, want)
	}
}

func TestRecordInitCommandsUseExplicitSettingsName(t *testing.T) {
	init := RecordInitCommands(RecordInitOptions{
		RecordFPS:    60,
		VideoPreset:  "c1",
		FFmpegParams: "-crf 4",
		RecordName:   "D:/clips/output/debug",
		SettingsName: "debug_c1_ultra_1a2b3c4d",
	})
	want := []string{
		"mirv_endofmatch 0",
		"mirv_streams record screen enabled 1",
		"mirv_streams record fps 60",
		"mirv_streams record startMovieWav 1",
		`mirv_streams settings add ffmpeg debug_c1_ultra_1a2b3c4d "-crf 4 {QUOTE}{AFX_STREAM_PATH}.mp4{QUOTE}"`,
		"mirv_streams record screen settings debug_c1_ultra_1a2b3c4d",
		`mirv_streams record name "D:/clips/output/debug"`,
	}
	if !reflect.DeepEqual(init, want) {
		t.Fatalf("record init commands=%#v want %#v", init, want)
	}
}

func TestDebugRecordSettingsNameTracksEncodeParameters(t *testing.T) {
	base := DebugRecordSettingsName("c1", "high", "-crf 4")
	if !strings.HasPrefix(base, "debug_c1_high_") {
		t.Fatalf("settings name=%q want a readable debug prefix", base)
	}
	if strings.HasPrefix(base, "afx") || strings.ContainsAny(base, " \t") {
		t.Fatalf("settings name=%q must be one safe console token", base)
	}
	if got := DebugRecordSettingsName("c1", "high", "-crf 4"); got != base {
		t.Fatalf("identical parameters must reuse the name: %q vs %q", got, base)
	}
	if got := DebugRecordSettingsName("c1", "ultra", "-crf 2"); got == base {
		t.Fatalf("changed parameters must produce a new name, got %q", got)
	}
	if got := DebugRecordSettingsName("n1", "high", "-crf 4"); got == base {
		t.Fatalf("changed preset must produce a new name, got %q", got)
	}
	if got := DebugRecordSettingsName("", "", ""); !strings.HasPrefix(got, "debug_preset_preset_") {
		t.Fatalf("blank values must fall back to a safe token, got %q", got)
	}
}

// The produce bootstrap must keep emitting exactly the shared recording init
// order: recording settings and boundaries stay separate.
func TestBuildBootstrapUsesSharedRecordInitOrder(t *testing.T) {
	item := Item{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7}}
	result, err := Build([]Item{item}, BuildOptions{
		TickRate:          64,
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordFPS:         60,
		VideoPreset:       "n1",
		RecordOutputDir:   "D:/clips/output",
		RecordBatchName:   "20260422_131500",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Sequences) == 0 {
		t.Fatal("Build returned no sequences")
	}

	want := RecordInitCommands(RecordInitOptions{
		RecordFPS:    60,
		VideoPreset:  "n1",
		FFmpegParams: mustRecordEncodeParams(t, "n1", "high", ""),
		RecordName:   "D:/clips/output/20260422_131500",
	})
	var got []string
	for _, action := range result.Sequences[0].Actions {
		for _, cmd := range want {
			if action.Cmd == cmd {
				got = append(got, action.Cmd)
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bootstrap record init=%#v want %#v", got, want)
	}
}

func mustRecordEncodeParams(t *testing.T, preset string, quality string, resolution string) string {
	t.Helper()
	_, params, err := BuildRecordEncoding(preset, quality, resolution)
	if err != nil {
		t.Fatalf("BuildRecordEncoding: %v", err)
	}
	return params
}
