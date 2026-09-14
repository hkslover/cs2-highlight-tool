package plugingen

import (
	"reflect"
	"strings"
	"testing"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/demo"
)

func testGenerationSettings() GenerationSettings {
	return GenerationSettings{
		KillerPreSeconds:     1,
		KillerPostSeconds:    1,
		VictimPreSeconds:     1,
		VictimPostSeconds:    1,
		RecordFPS:            60,
		RecordQuality:        "high",
		EffectiveVideoPreset: "c1",
		RecordOutputDir:      "/tmp/outputs",
		LaunchResolution:     "16:9",
	}
}

func TestBuildPlan_IsDeterministicAndUsesInjectedBatchName(t *testing.T) {
	input := PlanInput{
		DemoPath: "/demo/match.dem",
		Items: []clipsjson.Item{{
			Kill:              demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7},
			IncludeKiller:     boolPtr(true),
			IncludeVictim:     false,
			KillerSpecMode:    1,
			KillerPreSeconds:  1,
			KillerPostSeconds: 1,
		}},
		TickRate:       64,
		BatchTimestamp: "20260914_120000",
		RecordSubDir:   "demo_one",
		Settings:       testGenerationSettings(),
	}
	first, err := BuildPlan(input)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	second, err := BuildPlan(input)
	if err != nil {
		t.Fatalf("BuildPlan second call: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same explicit input produced different plans:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if len(first.TakePlans) != 1 || first.TakePlans[0].TakeName != "take0000" {
		t.Fatalf("unexpected take plans: %+v", first.TakePlans)
	}
	wantHistoryKey := BuildProduceHistoryKey("/demo/match.dem", "killer", 1, []string{"k1"})
	if len(first.HistoryKeys) != 1 || first.HistoryKeys[0] != wantHistoryKey {
		t.Fatalf("history keys=%v want [%s]", first.HistoryKeys, wantHistoryKey)
	}
	if len(first.Sequences) == 0 || !strings.Contains(first.Sequences[0].Actions[len(first.Sequences[0].Actions)-2].Cmd, "20260914_120000/demo_one") {
		t.Fatalf("record name did not use injected batch name: %+v", first.Sequences[0].Actions)
	}
}

func TestBuildPlan_FullRoundPassPrecedesVictimPassAndKeepsSourceID(t *testing.T) {
	input := PlanInput{
		Items: []clipsjson.Item{{
			Kill:          demo.ClipKill{ID: "k1", Tick: 400, KillerSlot: 7, VictimSlot: 11},
			IncludeKiller: boolPtr(false),
			IncludeVictim: true,
		}},
		FullRoundPOVSegments: []clipsjson.FullRoundPOVSegment{{
			Round:         2,
			StartTick:     100,
			EndTick:       300,
			Target:        "7",
			SpecMode:      1,
			SourceID:      "full_round_pov:r2:p765",
			PlayerName:    "player",
			PlayerSteamID: "765",
		}},
		TickRate: 64,
		Settings: testGenerationSettings(),
	}
	plan, err := BuildPlan(input)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.TakePlans) != 2 {
		t.Fatalf("take plans=%d want 2: %+v", len(plan.TakePlans), plan.TakePlans)
	}
	if plan.TakePlans[0].View != "full_round_pov" || plan.TakePlans[0].SourceID != "full_round_pov:r2:p765" {
		t.Fatalf("full-round take not first/stable: %+v", plan.TakePlans)
	}
	if plan.TakePlans[1].View != "victim" {
		t.Fatalf("victim take mismatch: %+v", plan.TakePlans)
	}
}
