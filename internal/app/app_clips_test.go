package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/demo"
	"cs2-highlight-tool-v2/internal/plugingen"
	"cs2-highlight-tool-v2/internal/producews"
)

func TestGeneratePluginJSON_SortsByTickAndMergesByKillerTarget(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	if _, err := app.SaveClipSettings(ClipSettings{KillerPreSeconds: 1, KillerPostSeconds: 1}); err != nil {
		t.Fatalf("SaveClipSettings: %v", err)
	}

	demoPath := writeDemoFile(t)

	result, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{Kill: demo.ClipKill{ID: "k3", Tick: 220, KillerSlot: 9, VictimSlot: 30}, IncludeVictim: false},
			{Kill: demo.ClipKill{ID: "k2", Tick: 210, KillerSlot: 7, VictimSlot: 20}, IncludeVictim: false},
			{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 10}, IncludeVictim: false},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSON: %v", err)
	}
	if result.SequenceCount != 2 {
		t.Fatalf("sequence_count=%d want 2", result.SequenceCount)
	}

	sequences := readGeneratedSequences(t, demoPath+".json")
	if len(sequences) != 2 {
		t.Fatalf("sequence len=%d want 2", len(sequences))
	}
	actions := sequences[1].Actions

	// k1 and k2 should merge (same killer target 7), k3 should stay separate (target 9).
	assertHasAction(t, actions, "demo_gototick 136")
	assertHasAction(t, actions, "demo_gototick 156")
	assertHasAction(t, actions, "spec_player 7")
	assertHasAction(t, actions, "spec_player 9")
}

func TestGeneratePluginJSON_VictimEachAsOwnSequence(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	if _, err := app.SaveClipSettings(ClipSettings{KillerPreSeconds: 1, KillerPostSeconds: 1}); err != nil {
		t.Fatalf("SaveClipSettings: %v", err)
	}

	demoPath := writeDemoFile(t)

	result, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{Kill: demo.ClipKill{ID: "k2", Tick: 300, KillerSlot: 7, VictimSlot: 12}, IncludeVictim: true},
			{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11}, IncludeVictim: true},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSON: %v", err)
	}
	if result.SequenceCount != 4 {
		t.Fatalf("sequence_count=%d want 4", result.SequenceCount)
	}

	sequences := readGeneratedSequences(t, demoPath+".json")
	if len(sequences) != 4 {
		t.Fatalf("sequence len=%d want 4", len(sequences))
	}

	assertHasAction(t, sequences[0].Actions, "go_to_next_sequence")
	assertHasAction(t, sequences[1].Actions, "go_to_next_sequence")
	assertHasAction(t, sequences[2].Actions, "go_to_next_sequence")
	assertHasAction(t, sequences[3].Actions, "disconnect")
	assertHasAction(t, sequences[2].Actions, "spec_player 11")
	assertHasAction(t, sequences[3].Actions, "spec_player 12")
}

func TestGeneratePluginJSON_VictimUsesVictimPrePostSeconds(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	if _, err := app.SaveClipSettings(ClipSettings{
		KillerPreSeconds:  2,
		KillerPostSeconds: 2,
		VictimPreSeconds:  1,
		VictimPostSeconds: 1,
	}); err != nil {
		t.Fatalf("SaveClipSettings: %v", err)
	}

	demoPath := writeDemoFile(t)
	_, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{Kill: demo.ClipKill{ID: "k1", Tick: 320, KillerSlot: 7, VictimSlot: 11}, IncludeVictim: true},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSON: %v", err)
	}

	sequences := readGeneratedSequences(t, demoPath+".json")
	if len(sequences) != 3 {
		t.Fatalf("sequence len=%d want 3", len(sequences))
	}
	assertHasAction(t, sequences[1].Actions, "demo_gototick 192")
	assertHasAction(t, sequences[2].Actions, "demo_gototick 256")
}

func TestGeneratePluginJSON_ClipOverridesUsePerClipWindows(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}
	demoPath := writeDemoFile(t)

	if _, err := app.SaveClipSettings(ClipSettings{
		KillerPreSeconds:  5,
		KillerPostSeconds: 5,
		VictimPreSeconds:  1,
		VictimPostSeconds: 1,
	}); err != nil {
		t.Fatalf("SaveClipSettings: %v", err)
	}

	_, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{
				Kill:          demo.ClipKill{ID: "k1", Tick: 640, KillerSlot: 7, VictimSlot: 11},
				IncludeVictim: true,
				ClipOverrides: &ClipItemOverrides{
					KillerPreSeconds:  float64Ptr(4),
					KillerPostSeconds: float64Ptr(1.5),
					VictimPreSeconds:  float64Ptr(4),
					VictimPostSeconds: float64Ptr(4),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSON: %v", err)
	}

	sequences := readGeneratedSequences(t, demoPath+".json")
	if len(sequences) != 3 {
		t.Fatalf("sequence len=%d want 3", len(sequences))
	}
	assertHasAction(t, sequences[1].Actions, "demo_gototick 384")
	assertHasAction(t, sequences[2].Actions, "demo_gototick 384")
}

func TestGeneratePluginJSON_ClipVictimOverridesIgnoredWhenVictimDisabled(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}
	demoPath := writeDemoFile(t)

	_, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{
				Kill:          demo.ClipKill{ID: "k1", Tick: 320, KillerSlot: 7, VictimSlot: 11},
				IncludeVictim: false,
				ClipOverrides: &ClipItemOverrides{
					VictimPreSeconds:  float64Ptr(2),
					VictimPostSeconds: float64Ptr(2),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSON: %v", err)
	}

	sequences := readGeneratedSequences(t, demoPath+".json")
	if len(sequences) != 2 {
		t.Fatalf("sequence len=%d want 2", len(sequences))
	}
	assertNoAction(t, sequences[1].Actions, "go_to_next_sequence")
}

func TestGeneratePluginJSON_ClipOverridesInjectPerPassVoiceAndXrayCommands(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}
	demoPath := writeDemoFile(t)

	if _, err := app.SaveClipSettings(ClipSettings{
		EnableVoice:        true,
		EnableSpecShowXray: true,
	}); err != nil {
		t.Fatalf("SaveClipSettings: %v", err)
	}

	_, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{
				Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
				IncludeVictim: false,
				ClipOverrides: &ClipItemOverrides{
					EnableVoice:        boolPtr(false),
					EnableSpecShowXray: boolPtr(false),
				},
			},
			{
				Kill:          demo.ClipKill{ID: "k2", Tick: 400, KillerSlot: 8},
				IncludeVictim: false,
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSON: %v", err)
	}

	sequences := readGeneratedSequences(t, demoPath+".json")
	if len(sequences) != 2 {
		t.Fatalf("sequence len=%d want 2", len(sequences))
	}
	actions := sequences[1].Actions
	assertHasAction(t, actions, "tv_listen_voice_indices 0")
	assertHasAction(t, actions, "tv_listen_voice_indices_h 0")
	assertHasAction(t, actions, "spec_show_xray -1")
	assertHasAction(t, actions, "tv_listen_voice_indices -1")
	assertHasAction(t, actions, "tv_listen_voice_indices_h -1")
	assertHasAction(t, actions, "spec_show_xray 0")
}

func TestGeneratePluginJSON_KillerThirdPersonInputIsForcedToSpecMode1(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	demoPath := writeDemoFile(t)
	_, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{
				Kill:           demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11},
				IncludeVictim:  false,
				KillerSpecMode: 2,
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSON: %v", err)
	}

	sequences := readGeneratedSequences(t, demoPath+".json")
	if len(sequences) != 2 {
		t.Fatalf("sequence len=%d want 2", len(sequences))
	}
	assertHasAction(t, sequences[1].Actions, "spec_mode 1")
	assertNoAction(t, sequences[1].Actions, "spec_mode 3")
}

func TestGeneratePluginJSON_VictimThirdPersonInputIsForcedToSpecMode1(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	demoPath := writeDemoFile(t)
	_, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{
				Kill:           demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11},
				IncludeVictim:  true,
				VictimSpecMode: 2,
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSON: %v", err)
	}

	sequences := readGeneratedSequences(t, demoPath+".json")
	if len(sequences) != 3 {
		t.Fatalf("sequence len=%d want 3", len(sequences))
	}
	assertHasAction(t, sequences[2].Actions, "spec_mode 1")
	assertNoAction(t, sequences[2].Actions, "spec_mode 3")
}

func TestGeneratePluginJSON_SameTargetDifferentKillerSpecModeInputsStillMerge(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	if _, err := app.SaveClipSettings(ClipSettings{KillerPreSeconds: 1, KillerPostSeconds: 1}); err != nil {
		t.Fatalf("SaveClipSettings: %v", err)
	}

	demoPath := writeDemoFile(t)
	_, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{
				Kill:           demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11},
				IncludeVictim:  false,
				KillerSpecMode: 1,
			},
			{
				Kill:           demo.ClipKill{ID: "k2", Tick: 205, KillerSlot: 7, VictimSlot: 12},
				IncludeVictim:  false,
				KillerSpecMode: 2,
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSON: %v", err)
	}

	sequences := readGeneratedSequences(t, demoPath+".json")
	if len(sequences) != 2 {
		t.Fatalf("sequence len=%d want 2", len(sequences))
	}
	actions := sequences[1].Actions
	assertHasAction(t, actions, "demo_gototick 136")
	assertHasAction(t, actions, "spec_mode 1")
	assertNoAction(t, actions, "spec_mode 3")
}

func TestGeneratePluginJSON_OldRequestFallsBackToSpecMode1(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	demoPath := writeDemoFile(t)
	_, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath:      demoPath,
		TickRate:      64,
		SelectedKills: []demo.ClipKill{{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11}},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSON: %v", err)
	}

	sequences := readGeneratedSequences(t, demoPath+".json")
	if len(sequences) == 0 {
		t.Fatalf("sequence len=%d want > 0", len(sequences))
	}
	assertHasAction(t, sequences[1].Actions, "spec_mode 1")
}

func TestGeneratePluginJSON_ContainsMirvRecordingCommands(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	demoPath := writeDemoFile(t)
	_, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11}, IncludeVictim: true},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSON: %v", err)
	}

	sequences := readGeneratedSequences(t, demoPath+".json")
	hasRecordStart := false
	hasRecordEnd := false
	hasFfmpeg := false
	for _, seq := range sequences {
		for _, action := range seq.Actions {
			hasRecordStart = hasRecordStart || action.Cmd == "mirv_streams record start"
			hasRecordEnd = hasRecordEnd || action.Cmd == "mirv_streams record end"
			hasFfmpeg = hasFfmpeg || strings.Contains(strings.ToLower(action.Cmd), "mirv_streams settings add ffmpeg")
		}
	}
	if !hasRecordStart || !hasRecordEnd || !hasFfmpeg {
		t.Fatalf("missing expected mirv commands: start=%v end=%v ffmpeg=%v", hasRecordStart, hasRecordEnd, hasFfmpeg)
	}
}

func TestGeneratePluginJSON_EmitsTakePlans(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	demoPath := writeDemoFile(t)
	result, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11}, IncludeVictim: true},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSON: %v", err)
	}
	if len(result.TakePlans) != 2 {
		t.Fatalf("take plans len=%d want 2", len(result.TakePlans))
	}
	if result.TakePlans[0].View != "killer" || result.TakePlans[1].View != "victim" {
		t.Fatalf("unexpected take plan views: %+v", result.TakePlans)
	}
	if result.TakePlans[0].SpecMode != 1 || result.TakePlans[1].SpecMode != 1 {
		t.Fatalf("unexpected take plan spec modes: %+v", result.TakePlans)
	}
	if len(result.TakePlans[0].KillIDs) != 1 || result.TakePlans[0].KillIDs[0] != "k1" {
		t.Fatalf("unexpected killer take plan kill ids: %+v", result.TakePlans[0].KillIDs)
	}
}

func TestGeneratePluginJSONInternal_LastSequenceAlwaysDisconnect(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	demoPath := writeDemoFile(t)
	_, _, err := app.generatePluginJSONInternal(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11}, IncludeVictim: true},
		},
	}, generatePluginJSONInternalOptions{
		WriteJSON: true,
	})
	if err != nil {
		t.Fatalf("generatePluginJSONInternal: %v", err)
	}

	sequences := readGeneratedSequences(t, demoPath+".json")
	if len(sequences) < 2 {
		t.Fatalf("sequence len=%d want >=2", len(sequences))
	}
	last := sequences[len(sequences)-1].Actions
	assertHasAction(t, last, "disconnect")
	assertNoAction(t, last, "quit")
}

func TestGeneratePluginJSONBatchAndLaunchHLAE_AllSkippedByHistory(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{
		exeDir: exeDir,
		produceState: produceSessionState{
			historyKeyIndex: make(map[string]struct{}),
		},
	}

	demoPath := writeDemoFile(t)
	preview, _, err := app.generatePluginJSONInternal(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11}, IncludeVictim: true},
		},
	}, generatePluginJSONInternalOptions{
		WriteJSON: false,
	})
	if err != nil {
		t.Fatalf("generate preview: %v", err)
	}
	for _, plan := range preview.TakePlans {
		key := plugingen.BuildProduceHistoryKey(plan.DemoPath, plan.View, plan.SpecMode, plan.KillIDs)
		app.produceState.historyKeyIndex[key] = struct{}{}
	}

	result, err := app.GeneratePluginJSONBatchAndLaunchHLAE(GeneratePluginJSONBatchRequest{
		Jobs: []GeneratePluginJSONRequest{
			{
				DemoPath: demoPath,
				TickRate: 64,
				SelectedItems: []SelectedClipItem{{
					Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11},
					IncludeVictim: true,
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSONBatchAndLaunchHLAE: %v", err)
	}
	if result.LaunchStarted {
		t.Fatalf("launch should not start when all clips are skipped: %+v", result)
	}
	if !strings.Contains(result.LaunchError, "无需录制") {
		t.Fatalf("unexpected launch error: %q", result.LaunchError)
	}
	if len(result.Results) != 1 {
		t.Fatalf("results len=%d want 1", len(result.Results))
	}
	if !result.Results[0].SkippedByHistory {
		t.Fatalf("expected skipped_by_history=true, got %+v", result.Results[0])
	}
}

func TestFilterItemsByHistory_IgnoreEditedHistoryKeys(t *testing.T) {
	items := []clipsjson.Item{
		{
			Kill:           demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11},
			IncludeKiller:  boolPtr(true),
			IncludeVictim:  true,
			KillerSpecMode: 1,
			VictimSpecMode: 1,
		},
	}
	plans := []ProduceTakePlan{
		{
			DemoPath: "demo.dem",
			View:     "killer",
			SpecMode: 1,
			KillIDs:  []string{"k1"},
		},
	}
	historyKeys := map[string]struct{}{
		buildEditedHistoryKey("D:/clips/edit_20260101.mp4", 123456): {},
	}

	filtered := filterItemsByHistory(items, plans, historyKeys)
	if len(filtered) != 1 {
		t.Fatalf("filtered items=%d want 1", len(filtered))
	}
	if filtered[0].Kill.ID != "k1" {
		t.Fatalf("unexpected filtered item: %+v", filtered[0])
	}
}

func TestGeneratePluginJSON_ExtraCommandsOnlyInBootstrapSequence(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	demoPath := writeDemoFile(t)
	_, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath:      demoPath,
		TickRate:      64,
		ExtraCommands: []string{"echo once"},
		SelectedItems: []SelectedClipItem{
			{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11}, IncludeVictim: true},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSON: %v", err)
	}

	sequences := readGeneratedSequences(t, demoPath+".json")
	if len(sequences) != 3 {
		t.Fatalf("sequence len=%d want 3", len(sequences))
	}
	assertHasAction(t, sequences[0].Actions, "echo once")
	for idx := 1; idx < len(sequences); idx++ {
		assertNoAction(t, sequences[idx].Actions, "echo once")
		assertNoPrefixAction(t, sequences[idx].Actions, "tv_listen_voice_indices")
	}
}

func TestGeneratePluginJSON_VoiceIndicesFollowEnableSwitch(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}
	demoPath := writeDemoFile(t)

	if _, err := app.SaveClipActionSettings(ClipActionSettings{
		EnableVoiceIndices:  true,
		VoiceIndicesValue:   0,
		EnableVoiceIndicesH: true,
		VoiceIndicesHValue:  0,
	}); err != nil {
		t.Fatalf("SaveClipActionSettings enabled: %v", err)
	}
	if _, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7}, IncludeVictim: false},
		},
	}); err != nil {
		t.Fatalf("GeneratePluginJSON enabled: %v", err)
	}
	enabled := readGeneratedSequences(t, demoPath+".json")
	assertHasAction(t, enabled[0].Actions, "tv_listen_voice_indices -1")
	assertHasAction(t, enabled[0].Actions, "tv_listen_voice_indices_h -1")

	if _, err := app.SaveClipActionSettings(ClipActionSettings{
		EnableVoiceIndices:  false,
		VoiceIndicesValue:   -1,
		EnableVoiceIndicesH: false,
		VoiceIndicesHValue:  -1,
	}); err != nil {
		t.Fatalf("SaveClipActionSettings disabled: %v", err)
	}
	if _, err := app.GeneratePluginJSON(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{
			{Kill: demo.ClipKill{ID: "k2", Tick: 260, KillerSlot: 9}, IncludeVictim: false},
		},
	}); err != nil {
		t.Fatalf("GeneratePluginJSON disabled: %v", err)
	}
	disabled := readGeneratedSequences(t, demoPath+".json")
	assertHasAction(t, disabled[0].Actions, "tv_listen_voice_indices 0")
	assertHasAction(t, disabled[0].Actions, "tv_listen_voice_indices_h 0")
}

func TestClipActionSettings_GetAndSave(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	initial, err := app.GetClipActionSettings()
	if err != nil {
		t.Fatalf("GetClipActionSettings: %v", err)
	}
	if !initial.EnableVoiceIndices || initial.VoiceIndicesValue != -1 {
		t.Fatalf("default voice_indices settings mismatch: %+v", initial)
	}
	if !initial.EnableVoiceIndicesH || initial.VoiceIndicesHValue != -1 {
		t.Fatalf("default voice_indices_h settings mismatch: %+v", initial)
	}

	saved, err := app.SaveClipActionSettings(ClipActionSettings{
		EnableVoiceIndices:  false,
		VoiceIndicesValue:   0,
		EnableVoiceIndicesH: true,
		VoiceIndicesHValue:  5,
	})
	if err != nil {
		t.Fatalf("SaveClipActionSettings: %v", err)
	}
	if saved.EnableVoiceIndices {
		t.Fatalf("save did not persist enable_voice_indices=false: %+v", saved)
	}
	if saved.EnableVoiceIndicesH {
		t.Fatalf("save should normalize enable_voice_indices_h=false when only one was enabled: %+v", saved)
	}
	if saved.VoiceIndicesValue != 0 {
		t.Fatalf("save did not persist voice_indices_value=0: %+v", saved)
	}

	loaded, err := app.GetClipActionSettings()
	if err != nil {
		t.Fatalf("GetClipActionSettings after save: %v", err)
	}
	if loaded.EnableVoiceIndices {
		t.Fatalf("load did not persist enable_voice_indices=false: %+v", loaded)
	}
	if loaded.EnableVoiceIndicesH {
		t.Fatalf("load should normalize enable_voice_indices_h=false: %+v", loaded)
	}
	if loaded.VoiceIndicesValue != 0 || loaded.VoiceIndicesHValue != 5 {
		t.Fatalf("load settings mismatch: %+v", loaded)
	}
}

func TestClipSettings_GetAndSave(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	initial, err := app.GetClipSettings()
	if err != nil {
		t.Fatalf("GetClipSettings: %v", err)
	}
	if initial.KillerPreSeconds != config.DefaultKillerPreSeconds || initial.KillerPostSeconds != config.DefaultKillerPostSeconds {
		t.Fatalf("default killer settings mismatch: %+v", initial)
	}
	if initial.VictimPreSeconds != config.DefaultVictimPreSeconds || initial.VictimPostSeconds != config.DefaultVictimPostSeconds {
		t.Fatalf("default victim settings mismatch: %+v", initial)
	}
	if !initial.AutoAddVictimView || !initial.EnableVoice {
		t.Fatalf("default auto/voice settings mismatch: %+v", initial)
	}
	if initial.VideoPreset != "auto" {
		t.Fatalf("default video_preset mismatch: %+v", initial)
	}
	if initial.EditFPS != config.DefaultEditFPS {
		t.Fatalf("default edit_fps mismatch: %+v", initial)
	}
	if initial.EditQuality != config.DefaultEditQuality {
		t.Fatalf("default edit_quality mismatch: %+v", initial)
	}
	if initial.RecordQuality != config.DefaultRecordQuality {
		t.Fatalf("default record_quality mismatch: %+v", initial)
	}
	if initial.LaunchResolution != "4:3" {
		t.Fatalf("default launch_resolution mismatch: %+v", initial)
	}
	if initial.HideAllUI {
		t.Fatalf("hide_all_ui should default to false: %+v", initial)
	}
	if initial.UseShoulderCamera {
		t.Fatalf("use_shoulder_camera should default to false: %+v", initial)
	}
	if initial.DisableClouds {
		t.Fatalf("disable_clouds should default to false: %+v", initial)
	}
	if initial.RecordOutputDir != filepath.Join(exeDir, "outputs") {
		t.Fatalf("default record_output_dir mismatch: %+v", initial)
	}

	saved, err := app.SaveClipSettings(ClipSettings{
		KillerPreSeconds:  20,
		KillerPostSeconds: 0.3,
		VictimPreSeconds:  19.5,
		VictimPostSeconds: 22.9,
		AutoAddVictimView: false,
		EnableVoice:       false,
		EditFPS:           300,
		EditQuality:       "ultra",
		RecordQuality:     "standard",
		VideoPreset:       "n1",
		LaunchResolution:  "16:9",
		HideAllUI:         true,
		UseShoulderCamera: true,
		DisableClouds:     true,
	})
	if err != nil {
		t.Fatalf("SaveClipSettings: %v", err)
	}
	if saved.KillerPreSeconds != 20 || saved.KillerPostSeconds != 1 {
		t.Fatalf("killer settings should be clamped to [1,20] with 0.5 step: %+v", saved)
	}
	if saved.VictimPreSeconds != 19.5 || saved.VictimPostSeconds != 20 {
		t.Fatalf("victim settings should be clamped to [1,20] with 0.5 step: %+v", saved)
	}
	if saved.AutoAddVictimView || saved.EnableVoice {
		t.Fatalf("auto/voice settings should persist false: %+v", saved)
	}
	if saved.VideoPreset != "n1" || saved.LaunchResolution != "16:9" {
		t.Fatalf("video/resolution settings should persist valid values: %+v", saved)
	}
	if saved.EditFPS != config.MaxEditFPS || saved.EditQuality != "ultra" {
		t.Fatalf("edit settings should clamp/persist valid values: %+v", saved)
	}
	if saved.RecordQuality != "standard" {
		t.Fatalf("record_quality should persist valid value: %+v", saved)
	}
	if !saved.HideAllUI {
		t.Fatalf("hide_all_ui should persist true: %+v", saved)
	}
	if !saved.UseShoulderCamera {
		t.Fatalf("use_shoulder_camera should persist true: %+v", saved)
	}
	if !saved.DisableClouds {
		t.Fatalf("disable_clouds should persist true: %+v", saved)
	}
	if saved.RecordOutputDir != filepath.Join(exeDir, "outputs") {
		t.Fatalf("record_output_dir should be fixed under exeDir: %+v", saved)
	}

	loaded, err := app.GetClipSettings()
	if err != nil {
		t.Fatalf("GetClipSettings after save: %v", err)
	}
	if loaded.KillerPreSeconds != 20 || loaded.KillerPostSeconds != 1 || loaded.VictimPreSeconds != 19.5 || loaded.VictimPostSeconds != 20 {
		t.Fatalf("saved clip settings mismatch: %+v", loaded)
	}
	if loaded.AutoAddVictimView || loaded.EnableVoice {
		t.Fatalf("saved auto/voice settings mismatch: %+v", loaded)
	}
	if loaded.VideoPreset != "n1" || loaded.LaunchResolution != "16:9" {
		t.Fatalf("saved video/resolution settings mismatch: %+v", loaded)
	}
	if loaded.EditFPS != config.MaxEditFPS || loaded.EditQuality != "ultra" {
		t.Fatalf("saved edit settings mismatch: %+v", loaded)
	}
	if loaded.RecordQuality != "standard" {
		t.Fatalf("saved record_quality mismatch: %+v", loaded)
	}
	if !loaded.HideAllUI {
		t.Fatalf("saved hide_all_ui mismatch: %+v", loaded)
	}
	if !loaded.UseShoulderCamera {
		t.Fatalf("saved use_shoulder_camera mismatch: %+v", loaded)
	}
	if !loaded.DisableClouds {
		t.Fatalf("saved disable_clouds mismatch: %+v", loaded)
	}
	if loaded.RecordOutputDir != filepath.Join(exeDir, "outputs") {
		t.Fatalf("loaded record_output_dir should be fixed under exeDir: %+v", loaded)
	}

	saved1280, err := app.SaveClipSettings(ClipSettings{
		LaunchResolution: "4:3_1280x960",
	})
	if err != nil {
		t.Fatalf("SaveClipSettings with 1280x960 launch resolution: %v", err)
	}
	if saved1280.LaunchResolution != "4:3_1280x960" {
		t.Fatalf("1280x960 launch_resolution should persist valid value: %+v", saved1280)
	}
	loaded1280, err := app.GetClipSettings()
	if err != nil {
		t.Fatalf("GetClipSettings after 1280x960 save: %v", err)
	}
	if loaded1280.LaunchResolution != "4:3_1280x960" {
		t.Fatalf("loaded 1280x960 launch_resolution mismatch: %+v", loaded1280)
	}

	actionSettings, err := app.GetClipActionSettings()
	if err != nil {
		t.Fatalf("GetClipActionSettings: %v", err)
	}
	if actionSettings.EnableVoiceIndices || actionSettings.EnableVoiceIndicesH {
		t.Fatalf("voice toggles should both be false after saving clip settings: %+v", actionSettings)
	}

	fallback, err := app.SaveClipSettings(ClipSettings{
		EditFPS:          1,
		EditQuality:      "invalid",
		RecordQuality:    "invalid",
		VideoPreset:      "invalid",
		LaunchResolution: "invalid",
	})
	if err != nil {
		t.Fatalf("SaveClipSettings with invalid video/resolution: %v", err)
	}
	if fallback.VideoPreset != "auto" {
		t.Fatalf("invalid video_preset should fall back to auto: %+v", fallback)
	}
	if fallback.LaunchResolution != "4:3" {
		t.Fatalf("invalid launch_resolution should fall back to 4:3: %+v", fallback)
	}
	if fallback.EditFPS != config.MinEditFPS {
		t.Fatalf("invalid edit_fps should clamp to min: %+v", fallback)
	}
	if fallback.EditQuality != config.DefaultEditQuality {
		t.Fatalf("invalid edit_quality should fall back to default: %+v", fallback)
	}
	if fallback.RecordQuality != config.DefaultRecordQuality {
		t.Fatalf("invalid record_quality should fall back to default: %+v", fallback)
	}
}

func TestBuildHLAECommandLine(t *testing.T) {
	line43 := buildHLAECommandLine("4:3")
	if !strings.Contains(line43, "-w 1440 -h 1080") {
		t.Fatalf("4:3 command line should include 1440x1080 args: %q", line43)
	}

	line43Low := buildHLAECommandLine("4:3_1280x960")
	if !strings.Contains(line43Low, "-w 1280 -h 960") {
		t.Fatalf("4:3 1280x960 command line should include 1280x960 args: %q", line43Low)
	}
	if strings.Contains(line43Low, "-w 1440 -h 1080") {
		t.Fatalf("4:3 1280x960 command line should not include 1440x1080 args: %q", line43Low)
	}

	line169 := buildHLAECommandLine("16:9")
	if strings.Contains(line169, "-w 1440 -h 1080") {
		t.Fatalf("16:9 command line should not include 1440x1080 args: %q", line169)
	}
}

func TestGeneratePluginJSONBatch_PartialFailure(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	okDemo := writeDemoFile(t)
	missingDemo := filepath.Join(t.TempDir(), "missing.dem")

	result, err := app.GeneratePluginJSONBatch(GeneratePluginJSONBatchRequest{
		Jobs: []GeneratePluginJSONRequest{
			{
				DemoPath: okDemo,
				TickRate: 64,
				SelectedItems: []SelectedClipItem{{
					Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11},
					IncludeVictim: false,
				}},
			},
			{
				DemoPath: missingDemo,
				TickRate: 64,
				SelectedItems: []SelectedClipItem{{
					Kill:          demo.ClipKill{ID: "k2", Tick: 300, KillerSlot: 8, VictimSlot: 12},
					IncludeVictim: false,
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSONBatch: %v", err)
	}
	if result.SuccessCount != 1 || result.FailureCount != 1 {
		t.Fatalf("counts = %d/%d want 1/1", result.SuccessCount, result.FailureCount)
	}
	if len(result.Results) != 2 {
		t.Fatalf("results len=%d want 2", len(result.Results))
	}
	if result.Results[0].Error != "" {
		t.Fatalf("first result should succeed: %+v", result.Results[0])
	}
	if result.Results[1].Error == "" {
		t.Fatalf("second result should fail: %+v", result.Results[1])
	}
}

func TestGeneratePluginJSONBatch_UsesDemoSubDirsWithinSharedBatchTimestampDir(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	if _, err := app.SaveClipSettings(ClipSettings{
		RecordOutputDir:    t.TempDir(),
		EnableSpecShowXray: true,
	}); err != nil {
		t.Fatalf("SaveClipSettings: %v", err)
	}

	demoA := writeDemoFile(t)
	demoB := writeDemoFileInDir(t, t.TempDir(), "match_b.dem")
	result, err := app.GeneratePluginJSONBatch(GeneratePluginJSONBatchRequest{
		Jobs: []GeneratePluginJSONRequest{
			{
				DemoPath:      demoA,
				TickRate:      64,
				SelectedItems: []SelectedClipItem{{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7}, IncludeVictim: false}},
			},
			{
				DemoPath:      demoB,
				TickRate:      64,
				SelectedItems: []SelectedClipItem{{Kill: demo.ClipKill{ID: "k2", Tick: 260, KillerSlot: 8}, IncludeVictim: false}},
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSONBatch: %v", err)
	}
	if result.SuccessCount != 2 || result.FailureCount != 0 {
		t.Fatalf("batch counts mismatch: %+v", result)
	}

	seqA := readGeneratedSequences(t, demoA+".json")
	seqB := readGeneratedSequences(t, demoB+".json")
	nameA := bootstrapRecordNamePath(t, seqA)
	nameB := bootstrapRecordNamePath(t, seqB)
	if nameA == "" || nameB == "" {
		t.Fatalf("record name command missing: %q / %q", nameA, nameB)
	}
	if nameA == nameB {
		t.Fatalf("record dirs should differ by demo subdir, got same: %q", nameA)
	}
	if !strings.HasSuffix(nameA, "/match") {
		t.Fatalf("demoA record dir mismatch: %q", nameA)
	}
	if !strings.HasSuffix(nameB, "/match_b") {
		t.Fatalf("demoB record dir mismatch: %q", nameB)
	}
	parentA := filepath.Dir(nameA)
	parentB := filepath.Dir(nameB)
	if parentA != parentB {
		t.Fatalf("batch timestamp dir should be shared, got %q vs %q", parentA, parentB)
	}
}

func TestGeneratePluginJSONBatch_UsesIncrementalSuffixForDuplicateDemoNames(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	if _, err := app.SaveClipSettings(ClipSettings{
		RecordOutputDir:    t.TempDir(),
		EnableSpecShowXray: true,
	}); err != nil {
		t.Fatalf("SaveClipSettings: %v", err)
	}

	dirA := filepath.Join(t.TempDir(), "a")
	dirB := filepath.Join(t.TempDir(), "b")
	if err := os.MkdirAll(dirA, 0755); err != nil {
		t.Fatalf("mkdir dirA: %v", err)
	}
	if err := os.MkdirAll(dirB, 0755); err != nil {
		t.Fatalf("mkdir dirB: %v", err)
	}

	demoA := writeDemoFileInDir(t, dirA, "same_name.dem")
	demoB := writeDemoFileInDir(t, dirB, "same_name.dem")
	result, err := app.GeneratePluginJSONBatch(GeneratePluginJSONBatchRequest{
		Jobs: []GeneratePluginJSONRequest{
			{
				DemoPath:      demoA,
				TickRate:      64,
				SelectedItems: []SelectedClipItem{{Kill: demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7}, IncludeVictim: false}},
			},
			{
				DemoPath:      demoB,
				TickRate:      64,
				SelectedItems: []SelectedClipItem{{Kill: demo.ClipKill{ID: "k2", Tick: 260, KillerSlot: 8}, IncludeVictim: false}},
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSONBatch: %v", err)
	}
	if result.SuccessCount != 2 || result.FailureCount != 0 {
		t.Fatalf("batch counts mismatch: %+v", result)
	}

	seqA := readGeneratedSequences(t, demoA+".json")
	seqB := readGeneratedSequences(t, demoB+".json")
	nameA := bootstrapRecordNamePath(t, seqA)
	nameB := bootstrapRecordNamePath(t, seqB)
	if !strings.HasSuffix(nameA, "/same_name") {
		t.Fatalf("first duplicate suffix mismatch: %q", nameA)
	}
	if !strings.HasSuffix(nameB, "/same_name_2") {
		t.Fatalf("second duplicate suffix mismatch: %q", nameB)
	}
}

func TestGeneratePluginJSONBatch_ReturnsBatchTimestampAndTakePlans(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	demoPath := writeDemoFile(t)
	result, err := app.GeneratePluginJSONBatch(GeneratePluginJSONBatchRequest{
		Jobs: []GeneratePluginJSONRequest{
			{
				DemoPath: demoPath,
				TickRate: 64,
				SelectedItems: []SelectedClipItem{{
					Kill:          demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11},
					IncludeVictim: true,
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePluginJSONBatch: %v", err)
	}
	if strings.TrimSpace(result.BatchTimestamp) == "" {
		t.Fatalf("batch timestamp should not be empty")
	}
	if len(result.Results) != 1 {
		t.Fatalf("results len=%d want 1", len(result.Results))
	}
	if len(result.Results[0].TakePlans) != 2 {
		t.Fatalf("take plans len=%d want 2", len(result.Results[0].TakePlans))
	}
}

func TestSaveClipSettings_AlwaysUsesFixedOutputDir(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	saved, err := app.SaveClipSettings(ClipSettings{
		RecordOutputDir:    filepath.Join(t.TempDir(), "输出"),
		EnableSpecShowXray: true,
	})
	if err != nil {
		t.Fatalf("save should not fail when record_output_dir is user-provided: %v", err)
	}
	if saved.RecordOutputDir != filepath.Join(exeDir, "outputs") {
		t.Fatalf("saved record_output_dir should be fixed: %+v", saved)
	}

	loaded, err := app.GetClipSettings()
	if err != nil {
		t.Fatalf("GetClipSettings: %v", err)
	}
	if loaded.RecordOutputDir != filepath.Join(exeDir, "outputs") {
		t.Fatalf("loaded record_output_dir should be fixed: %+v", loaded)
	}
}

func TestGeneratePluginJSONBatchAndLaunchHLAE_LaunchErrorWhenEnvironmentMissing(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	demoPath := writeDemoFile(t)
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
	if result.SuccessCount != 1 || result.FailureCount != 0 {
		t.Fatalf("batch result mismatch: %+v", result)
	}
	if result.LaunchStarted {
		t.Fatalf("launch should fail in test env: %+v", result)
	}
	if result.LaunchError == "" {
		t.Fatalf("expected launch error in test env: %+v", result)
	}
	if result.LaunchedDemoPath != demoPath {
		t.Fatalf("launched demo path mismatch: got %q want %q", result.LaunchedDemoPath, demoPath)
	}
}

func TestGeneratePluginJSONBatchAndLaunchHLAE_PIDDetectFailureStopsLaunch(t *testing.T) {
	exeDir := t.TempDir()
	demoPath, _ := prepareLaunchTestEnvironment(t, exeDir)
	app := &App{exeDir: exeDir}

	oldLaunchCmd := launchHLAECommand
	oldListFn := listCS2PIDsFn
	oldCloseFn := closeCS2ProcessByPIDFn
	launchHLAECommand = helperLaunchCommand
	listCalls := 0
	listCS2PIDsFn = func() ([]int, error) {
		listCalls++
		if listCalls == 1 {
			return []int{1000}, nil
		}
		return nil, fmt.Errorf("mock poll failed")
	}
	closeCS2ProcessByPIDFn = func(pid int) error { return nil }
	t.Cleanup(func() {
		launchHLAECommand = oldLaunchCmd
		listCS2PIDsFn = oldListFn
		closeCS2ProcessByPIDFn = oldCloseFn
	})

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
		t.Fatalf("launch should stop when pid detection fails: %+v", result)
	}
	if !strings.Contains(result.LaunchError, "未识别到新的 cs2.exe 进程") {
		t.Fatalf("unexpected launch error: %q", result.LaunchError)
	}
}

func TestGeneratePluginJSONBatchAndLaunchHLAE_StartQueueFailureClosesPID(t *testing.T) {
	exeDir := t.TempDir()
	demoPath, _ := prepareLaunchTestEnvironment(t, exeDir)
	app := &App{
		exeDir:   exeDir,
		produceW: producews.NewDefault(nil),
	}

	oldLaunchCmd := launchHLAECommand
	oldListFn := listCS2PIDsFn
	oldCloseFn := closeCS2ProcessByPIDFn
	launchHLAECommand = helperLaunchCommand
	listCalls := 0
	listCS2PIDsFn = func() ([]int, error) {
		listCalls++
		if listCalls == 1 {
			return []int{1000}, nil
		}
		return []int{1000, 1001}, nil
	}
	closeCalls := 0
	closedPID := 0
	closeCS2ProcessByPIDFn = func(pid int) error {
		closeCalls++
		closedPID = pid
		return nil
	}
	t.Cleanup(func() {
		launchHLAECommand = oldLaunchCmd
		listCS2PIDsFn = oldListFn
		closeCS2ProcessByPIDFn = oldCloseFn
	})

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
		t.Fatalf("launch should fail when StartQueue fails: %+v", result)
	}
	if !strings.Contains(result.LaunchError, "produce websocket server is not started") {
		t.Fatalf("unexpected launch error: %q", result.LaunchError)
	}
	if closeCalls != 1 || closedPID != 1001 {
		t.Fatalf("close pid mismatch: calls=%d pid=%d", closeCalls, closedPID)
	}
}

func TestGeneratePluginJSONBatchAndLaunchHLAE_NoLaunchWhenAllJobsFail(t *testing.T) {
	exeDir := t.TempDir()
	app := &App{exeDir: exeDir}

	missingDemo := filepath.Join(t.TempDir(), "missing.dem")
	result, err := app.GeneratePluginJSONBatchAndLaunchHLAE(GeneratePluginJSONBatchRequest{
		Jobs: []GeneratePluginJSONRequest{
			{
				DemoPath: missingDemo,
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
	if result.SuccessCount != 0 || result.FailureCount != 1 {
		t.Fatalf("batch result mismatch: %+v", result)
	}
	if result.LaunchStarted {
		t.Fatalf("launch should not start when all jobs fail: %+v", result)
	}
	if !strings.Contains(result.LaunchError, "没有可启动的 demo") {
		t.Fatalf("unexpected launch error: %q", result.LaunchError)
	}
}

func prepareLaunchTestEnvironment(t *testing.T, exeDir string) (string, string) {
	t.Helper()
	demoPath := writeDemoFile(t)

	hlaeExe := filepath.Join(exeDir, "hlae", "HLAE.exe")
	hookDLL := filepath.Join(exeDir, "hlae", "x64", "AfxHookSource2.dll")
	if err := os.MkdirAll(filepath.Dir(hookDLL), 0755); err != nil {
		t.Fatalf("mkdir hook dir: %v", err)
	}
	if err := os.WriteFile(hlaeExe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write hlae exe: %v", err)
	}
	if err := os.WriteFile(hookDLL, []byte("dll"), 0644); err != nil {
		t.Fatalf("write hook dll: %v", err)
	}

	cs2Root := filepath.Join(exeDir, "cs2")
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
	if err := os.WriteFile(gameInfoPath, []byte("FileSystem\n{\n\tSearchPaths\n\t{\n\t\tGame\tcsgo\n\t}\n}\n"), 0644); err != nil {
		t.Fatalf("write gameinfo: %v", err)
	}
	pluginDLL := filepath.Join(exeDir, "plugin", "server.dll")
	if err := os.MkdirAll(filepath.Dir(pluginDLL), 0755); err != nil {
		t.Fatalf("mkdir plugin dll dir: %v", err)
	}
	if err := os.WriteFile(pluginDLL, []byte("plugin"), 0644); err != nil {
		t.Fatalf("write plugin dll: %v", err)
	}

	cfg := config.Default(exeDir)
	cfg.CS2Dir = cs2Root
	cfg.CS2Exe = cs2Exe
	cfg.HLAEExe = hlaeExe
	cfg.PluginDLL = pluginDLL
	if err := config.Save(filepath.Join(exeDir, "config.json"), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return demoPath, cs2Exe
}

func helperLaunchCommand(_ string, _ ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessLaunchHLAE", "--")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS_LAUNCH_HLAE=1")
	return cmd
}

func TestHelperProcessLaunchHLAE(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_LAUNCH_HLAE") != "1" {
		return
	}
	os.Exit(0)
}

func writeDemoFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "match.dem")
	if err := os.WriteFile(path, []byte("demo"), 0644); err != nil {
		t.Fatalf("write demo: %v", err)
	}
	return path
}

func writeDemoFileInDir(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("demo"), 0644); err != nil {
		t.Fatalf("write demo: %v", err)
	}
	return path
}

func readGeneratedSequences(t *testing.T, path string) []pluginSequence {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	var sequences []pluginSequence
	if err := json.Unmarshal(payload, &sequences); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	return sequences
}

func assertHasAction(t *testing.T, actions []pluginAction, cmd string) {
	t.Helper()
	for _, action := range actions {
		if action.Cmd == cmd {
			return
		}
	}
	t.Fatalf("action %q not found in %#v", cmd, actions)
}

func assertNoAction(t *testing.T, actions []pluginAction, cmd string) {
	t.Helper()
	for _, action := range actions {
		if action.Cmd == cmd {
			t.Fatalf("unexpected action %q in %#v", cmd, actions)
		}
	}
}

func assertNoPrefixAction(t *testing.T, actions []pluginAction, prefix string) {
	t.Helper()
	for _, action := range actions {
		if strings.HasPrefix(action.Cmd, prefix) {
			t.Fatalf("unexpected action prefix %q in %#v", prefix, actions)
		}
	}
}

func bootstrapRecordNamePath(t *testing.T, sequences []pluginSequence) string {
	t.Helper()
	if len(sequences) == 0 {
		return ""
	}
	for _, action := range sequences[0].Actions {
		if strings.HasPrefix(action.Cmd, `mirv_streams record name "`) {
			raw := strings.TrimPrefix(action.Cmd, `mirv_streams record name "`)
			raw = strings.TrimSuffix(raw, `"`)
			return raw
		}
	}
	return ""
}

func float64Ptr(value float64) *float64 {
	v := value
	return &v
}
