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
	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "demo.dem")
	relativePath, err := filepath.Rel(mustGetwd(t), targetPath)
	if err != nil {
		t.Fatalf("relative path: %v", err)
	}

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
	app := &App{exeDir: exeDir}
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
	app := &App{exeDir: exeDir}
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

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}
