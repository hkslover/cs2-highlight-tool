package plugingen

import (
	"testing"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/demo"
)

func boolPtr(b bool) *bool { return &b }

func TestFilterItemsByHistory_ReturnsNilWhenNoItems(t *testing.T) {
	result := FilterItemsByHistory(nil, []TakePlan{{DemoPath: "a", View: "killer", SpecMode: 1, KillIDs: []string{"k1"}}}, nil)
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestFilterItemsByHistory_ReturnsNilWhenNoPlans(t *testing.T) {
	items := []clipsjson.Item{
		{Kill: demo.ClipKill{ID: "k1"}, IncludeKiller: boolPtr(true)},
	}
	result := FilterItemsByHistory(items, nil, nil)
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestFilterItemsByHistory_ExcludesItemsAlreadyInHistory(t *testing.T) {
	items := []clipsjson.Item{
		{Kill: demo.ClipKill{ID: "k1"}, IncludeKiller: boolPtr(true), IncludeVictim: false},
	}
	plans := []TakePlan{
		{DemoPath: "demo.dem", View: "killer", SpecMode: 1, KillIDs: []string{"k1"}},
	}
	historyKey := BuildProduceHistoryKey("demo.dem", "killer", 1, []string{"k1"})
	historyKeys := map[string]struct{}{historyKey: {}}

	result := FilterItemsByHistory(items, plans, historyKeys)
	if len(result) != 0 {
		t.Fatalf("expected empty result when all items are in history, got %d", len(result))
	}
}

func TestFilterItemsByHistory_IncludesItemsNotInHistory(t *testing.T) {
	items := []clipsjson.Item{
		{Kill: demo.ClipKill{ID: "k1"}, IncludeKiller: boolPtr(true), IncludeVictim: false},
		{Kill: demo.ClipKill{ID: "k2"}, IncludeKiller: boolPtr(true), IncludeVictim: false},
	}
	plans := []TakePlan{
		{DemoPath: "demo.dem", View: "killer", SpecMode: 1, KillIDs: []string{"k1"}},
		{DemoPath: "demo.dem", View: "killer", SpecMode: 1, KillIDs: []string{"k2"}},
	}
	// Only k1's plan is in history
	historyKey := BuildProduceHistoryKey("demo.dem", "killer", 1, []string{"k1"})
	historyKeys := map[string]struct{}{historyKey: {}}

	result := FilterItemsByHistory(items, plans, historyKeys)
	if len(result) != 1 {
		t.Fatalf("expected 1 item (k2 only), got %d", len(result))
	}
	if result[0].Kill.ID != "k2" {
		t.Fatalf("expected k2, got %q", result[0].Kill.ID)
	}
}

func TestFilterItemsByHistory_SeparatesKillerAndVictimViews(t *testing.T) {
	items := []clipsjson.Item{
		{Kill: demo.ClipKill{ID: "k1"}, IncludeKiller: boolPtr(true), IncludeVictim: true},
	}
	plans := []TakePlan{
		{DemoPath: "demo.dem", View: "killer", SpecMode: 1, KillIDs: []string{"k1"}},
		{DemoPath: "demo.dem", View: "victim", SpecMode: 1, KillIDs: []string{"k1"}},
	}
	// Only killer plan is in history
	killerKey := BuildProduceHistoryKey("demo.dem", "killer", 1, []string{"k1"})
	historyKeys := map[string]struct{}{killerKey: {}}

	result := FilterItemsByHistory(items, plans, historyKeys)
	if len(result) != 1 {
		t.Fatalf("expected 1 item (victim view only), got %d", len(result))
	}
	if result[0].IncludeKiller == nil || *result[0].IncludeKiller {
		t.Fatalf("expected IncludeKiller=false for victim-only item, got %v", result[0].IncludeKiller)
	}
	if !result[0].IncludeVictim {
		t.Fatal("expected IncludeVictim=true for victim-only item")
	}
}

func TestFilterItemsByHistory_RebuildsMissingVictimPlan(t *testing.T) {
	items := []clipsjson.Item{
		{
			Kill:           demo.ClipKill{ID: "k1", Tick: 200, KillerSlot: 7, VictimSlot: 11},
			IncludeKiller:  boolPtr(true),
			IncludeVictim:  true,
			KillerSpecMode: 1,
			VictimSpecMode: 1,
		},
		{
			Kill:           demo.ClipKill{ID: "k2", Tick: 240, KillerSlot: 7, VictimSlot: 11},
			IncludeKiller:  boolPtr(true),
			IncludeVictim:  true,
			KillerSpecMode: 1,
			VictimSpecMode: 1,
		},
	}
	initial, err := BuildPlan(PlanInput{
		DemoPath:       "demo.dem",
		Items:          items,
		TickRate:       64,
		BatchTimestamp: "batch",
		Settings:       testGenerationSettings(),
	})
	if err != nil {
		t.Fatalf("BuildPlan initial: %v", err)
	}

	plans := make([]TakePlan, len(initial.TakePlans))
	historyKeys := make(map[string]struct{})
	killerPlans := 0
	for i, plan := range initial.TakePlans {
		plans[i] = TakePlan{
			DemoPath: "demo.dem",
			View:     plan.View,
			SpecMode: plan.SpecMode,
			KillIDs:  append([]string(nil), plan.KillIDs...),
			SourceID: plan.SourceID,
		}
		if plan.View == "killer" {
			killerPlans++
			historyKeys[BuildProduceHistoryKeyWithSourceID("demo.dem", plan.View, plan.SpecMode, plan.KillIDs, plan.SourceID)] = struct{}{}
		}
	}
	if killerPlans == 0 {
		t.Fatalf("initial plan did not contain a killer take: %+v", initial.TakePlans)
	}

	filtered := FilterItemsByHistory(items, plans, historyKeys)
	if len(filtered) != len(items) {
		t.Fatalf("filtered items=%d want %d", len(filtered), len(items))
	}
	for _, item := range filtered {
		if item.IncludeKiller == nil || *item.IncludeKiller {
			t.Fatalf("expected killer=false after history filtering: %+v", item)
		}
		if !item.IncludeVictim {
			t.Fatalf("expected victim=true after history filtering: %+v", item)
		}
	}

	retry, err := BuildPlan(PlanInput{
		DemoPath:       "demo.dem",
		Items:          filtered,
		TickRate:       64,
		BatchTimestamp: "retry",
		Settings:       testGenerationSettings(),
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

func TestFilterItemsByHistory_IgnoresNonMatchingHistoryKeys(t *testing.T) {
	items := []clipsjson.Item{
		{Kill: demo.ClipKill{ID: "k1"}, IncludeKiller: boolPtr(true)},
	}
	plans := []TakePlan{
		{DemoPath: "demo.dem", View: "killer", SpecMode: 1, KillIDs: []string{"k1"}},
	}
	// Different history key (edited video) that should not affect produce filtering
	historyKeys := map[string]struct{}{
		"edited#123456#d:/clips/edit.mp4": {},
	}

	result := FilterItemsByHistory(items, plans, historyKeys)
	if len(result) != 1 {
		t.Fatalf("expected 1 item (not in history), got %d", len(result))
	}
	if result[0].Kill.ID != "k1" {
		t.Fatalf("unexpected item: %+v", result[0])
	}
}

func TestFilterFullRoundPOVSegmentsByHistoryUsesStableSourceID(t *testing.T) {
	segments := []clipsjson.FullRoundPOVSegment{
		{Round: 1, PlayerSteamID: "765", SourceID: "full_round_pov:r1:p765", Target: "7", StartTick: 100, EndTick: 200},
		{Round: 2, PlayerSteamID: "765", SourceID: "full_round_pov:r2:p765", Target: "7", StartTick: 300, EndTick: 400},
	}
	plans := []TakePlan{
		{DemoPath: "demo.dem", View: "full_round_pov", SpecMode: 1, SourceID: segments[0].SourceID},
		{DemoPath: "demo.dem", View: "full_round_pov", SpecMode: 1, SourceID: segments[1].SourceID},
	}
	history := map[string]struct{}{
		BuildProduceHistoryKeyWithSourceID("demo.dem", "full_round_pov", 1, nil, segments[0].SourceID): {},
	}
	filtered := FilterFullRoundPOVSegmentsByHistory(segments, plans, history, "")
	if len(filtered) != 1 || filtered[0].SourceID != segments[1].SourceID {
		t.Fatalf("filtered=%+v want only round 2", filtered)
	}
}
