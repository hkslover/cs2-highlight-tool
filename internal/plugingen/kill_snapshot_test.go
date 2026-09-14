package plugingen

import (
	"testing"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/demo"
)

func TestCollectFullRoundPOVKillsUsesOnlyRequestedRounds(t *testing.T) {
	meta := &demo.Metadata{ClipPlayers: []demo.ClipPlayer{{
		SteamID: "765",
		Rounds: []demo.ClipRound{
			{Round: 1, Kills: []demo.ClipKill{{ID: "r1"}}},
			{Round: 2, Kills: []demo.ClipKill{{ID: "r2"}}},
		},
	}}}
	kills := CollectFullRoundPOVKills(meta, []TakePlan{{
		View:          "full_round_pov",
		Round:         2,
		PlayerSteamID: "765",
	}})
	if len(kills) != 1 {
		t.Fatalf("kills=%d want 1: %+v", len(kills), kills)
	}
	if _, ok := kills["r2"]; !ok {
		t.Fatalf("round 2 kill missing: %+v", kills)
	}
}

func TestRegisterProduceKillSnapshotAddsPlanAndPOVKills(t *testing.T) {
	store := make(map[string]map[string]demo.ClipKill)
	RegisterProduceKillSnapshot(store,
		[]TakePlan{{DemoPath: "demo.dem", View: "full_round_pov", Round: 2, PlayerSteamID: "765"}},
		[]clipsjson.Item{{Kill: demo.ClipKill{ID: "selected"}}},
		map[string]map[string]demo.ClipKill{"demo.dem": {"pov": {ID: "pov"}}},
		"")
	if len(store["demo.dem"]) != 2 {
		t.Fatalf("snapshot=%+v want selected and pov kills", store)
	}
}
