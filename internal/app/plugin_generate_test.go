package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/demo"
	"cs2-highlight-tool-v2/internal/plugingen"
)

func TestNormalizeGeneratePluginBatchJobsUsesAbsoluteDemoPaths(t *testing.T) {
	// 只需要一个相对路径：不依赖临时目录，因为 Windows 的 %TEMP% 常在与
	// 工作目录不同的卷上，那时 filepath.Rel 无法表达相对路径。
	relativePath := filepath.Join("testdata", "demo.dem")

	jobs, err := normalizeGeneratePluginBatchJobs([]GeneratePluginJSONRequest{{DemoPath: relativePath}})
	if err != nil {
		t.Fatalf("normalize batch jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs=%d want 1", len(jobs))
	}
	abs, err := filepath.Abs(relativePath)
	if err != nil {
		t.Fatalf("absolute path: %v", err)
	}
	if jobs[0].DemoPath != abs {
		t.Fatalf("demo path=%q want %q", jobs[0].DemoPath, abs)
	}
}

func TestGenerationSnapshotFreezesSettingsAcrossJobs(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	if _, err := app.SaveClipSettings(ClipSettings{
		KillerPreSeconds:  1,
		KillerPostSeconds: 1,
		RecordQuality:     "standard",
		VideoPreset:       "c1",
	}); err != nil {
		t.Fatalf("SaveClipSettings initial: %v", err)
	}
	snapshot, err := app.captureGenerationSnapshot()
	if err != nil {
		t.Fatalf("captureGenerationSnapshot: %v", err)
	}
	firstDemo := writeDemoFile(t)
	secondDemo := writeDemoFile(t)
	request := func(path string) GeneratePluginJSONRequest {
		return GeneratePluginJSONRequest{
			DemoPath:       path,
			TickRate:       64,
			BatchTimestamp: "fixed_batch",
			SelectedItems: []SelectedClipItem{{
				Kill:          demo.ClipKill{ID: "k1", Tick: 320, KillerSlot: 7},
				IncludeVictim: false,
			}},
		}
	}
	if _, _, err := app.generatePluginJSONInternalWithSnapshot(request(firstDemo), generatePluginJSONInternalOptions{WriteJSON: true}, snapshot); err != nil {
		t.Fatalf("first job: %v", err)
	}
	if _, err := app.SaveClipSettings(ClipSettings{
		KillerPreSeconds:  8,
		KillerPostSeconds: 8,
		RecordQuality:     "ultra",
		VideoPreset:       "n1",
	}); err != nil {
		t.Fatalf("SaveClipSettings changed: %v", err)
	}
	if _, _, err := app.generatePluginJSONInternalWithSnapshot(request(secondDemo), generatePluginJSONInternalOptions{WriteJSON: true}, snapshot); err != nil {
		t.Fatalf("second job: %v", err)
	}
	firstJSON, err := os.ReadFile(firstDemo + ".json")
	if err != nil {
		t.Fatalf("read first JSON: %v", err)
	}
	secondJSON, err := os.ReadFile(secondDemo + ".json")
	if err != nil {
		t.Fatalf("read second JSON: %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("jobs using one snapshot produced different plans after settings update")
	}
}

func TestGenerationHistoryFilterKeepsFullRoundWhenItemsAreEmpty(t *testing.T) {
	exeDir := t.TempDir()
	app := newTestApp(t, exeDir)
	snapshot, err := app.captureGenerationSnapshot()
	if err != nil {
		t.Fatalf("captureGenerationSnapshot: %v", err)
	}

	demoPath := writeDemoFile(t)
	items := []clipsjson.Item{{
		Kill:           demo.ClipKill{ID: "k1", Tick: 320, KillerSlot: 7, VictimSlot: 11},
		IncludeKiller:  boolPtr(true),
		IncludeVictim:  false,
		KillerSpecMode: 1,
	}}
	fullRoundSourceID := plugingen.BuildFullRoundPOVSourceID(1, "76561190000000001")
	plans := []ProduceTakePlan{
		{
			DemoPath: demoPath,
			View:     "killer",
			SpecMode: 1,
			KillIDs:  []string{"k1"},
		},
		{
			DemoPath:      demoPath,
			View:          "full_round_pov",
			SpecMode:      1,
			SourceID:      fullRoundSourceID,
			Round:         1,
			PlayerSteamID: "76561190000000001",
		},
	}
	historyKeys := map[string]struct{}{
		plugingen.BuildProduceHistoryKeyWithSourceID(demoPath, "killer", 1, []string{"k1"}, ""): {},
	}
	filteredItems := filterItemsByHistory(items, plans, historyKeys)
	if len(filteredItems) != 0 {
		t.Fatalf("historical killer item should be filtered, got %d items", len(filteredItems))
	}

	fullRoundSegments := []clipsjson.FullRoundPOVSegment{{
		Round:         1,
		StartTick:     100,
		EndTick:       200,
		Target:        "7",
		SpecMode:      1,
		SourceID:      fullRoundSourceID,
		PlayerName:    "target",
		PlayerSteamID: "76561190000000001",
		EndReason:     "round_end",
	}}
	filteredFullRoundSegments := filterFullRoundPOVSegmentsByHistory(fullRoundSegments, plans, historyKeys, demoPath)
	if len(filteredFullRoundSegments) != 1 {
		t.Fatalf("unrecorded full-round POV should remain, got %d segments", len(filteredFullRoundSegments))
	}

	preview, _, err := app.generatePluginJSONInternalWithSnapshot(GeneratePluginJSONRequest{
		DemoPath: demoPath,
		TickRate: 64,
		SelectedItems: []SelectedClipItem{{
			Kill:          demo.ClipKill{ID: "k1", Tick: 320, KillerSlot: 7, VictimSlot: 11},
			IncludeVictim: false,
		}},
	}, generatePluginJSONInternalOptions{
		ItemsOverride:                   filteredItems,
		UseItemsOverride:                true,
		FullRoundPOVSegmentsOverride:    filteredFullRoundSegments,
		UseFullRoundPOVSegmentsOverride: true,
		WriteJSON:                       false,
	}, snapshot)
	if err != nil {
		t.Fatalf("generate filtered preview: %v", err)
	}
	if len(preview.TakePlans) != 1 || preview.TakePlans[0].View != "full_round_pov" {
		t.Fatalf("filtered generation reintroduced historical clip: %+v", preview.TakePlans)
	}
}

func TestGenerationHistoryFilterRebuildsVictimOnlyPlan(t *testing.T) {
	demoPath := filepath.Join(t.TempDir(), "match.dem")
	items := []clipsjson.Item{{
		Kill:           demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11},
		IncludeKiller:  boolPtr(true),
		IncludeVictim:  true,
		KillerSpecMode: 1,
		VictimSpecMode: 1,
	}}
	settings := plugingen.GenerationSettings{
		KillerPreSeconds:     1,
		KillerPostSeconds:    1,
		VictimPreSeconds:     1,
		VictimPostSeconds:    1,
		RecordFPS:            60,
		RecordQuality:        "high",
		EffectiveVideoPreset: "c1",
		LaunchResolution:     "16:9",
	}
	initial, err := plugingen.BuildPlan(plugingen.PlanInput{
		DemoPath:       demoPath,
		Items:          items,
		TickRate:       64,
		BatchTimestamp: "initial",
		Settings:       settings,
	})
	if err != nil {
		t.Fatalf("BuildPlan initial: %v", err)
	}

	plans := make([]ProduceTakePlan, len(initial.TakePlans))
	historyKeys := make(map[string]struct{})
	for i, plan := range initial.TakePlans {
		plans[i] = ProduceTakePlan{
			DemoPath: demoPath,
			View:     plan.View,
			SpecMode: plan.SpecMode,
			KillIDs:  append([]string(nil), plan.KillIDs...),
			SourceID: plan.SourceID,
		}
		if plan.View == "killer" {
			historyKeys[plugingen.BuildProduceHistoryKeyWithSourceID(demoPath, plan.View, plan.SpecMode, plan.KillIDs, plan.SourceID)] = struct{}{}
		}
	}

	filtered := filterItemsByHistory(items, plans, historyKeys)
	if len(filtered) != 1 {
		t.Fatalf("filtered items=%d want 1", len(filtered))
	}
	if filtered[0].IncludeKiller == nil || *filtered[0].IncludeKiller {
		t.Fatalf("expected killer=false after history filtering: %+v", filtered[0].IncludeKiller)
	}
	if !filtered[0].IncludeVictim {
		t.Fatal("expected victim=true after history filtering")
	}

	retry, err := plugingen.BuildPlan(plugingen.PlanInput{
		DemoPath:       demoPath,
		Items:          filtered,
		TickRate:       64,
		BatchTimestamp: "retry",
		Settings:       settings,
	})
	if err != nil {
		t.Fatalf("BuildPlan retry: %v", err)
	}
	if len(retry.TakePlans) == 0 {
		t.Fatalf("retry plan has no take plans")
	}
	for _, plan := range retry.TakePlans {
		if plan.View != "victim" {
			t.Fatalf("retry plan reintroduced killer take: %+v", retry.TakePlans)
		}
	}
}
