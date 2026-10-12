package clipsjson

import (
	"testing"

	"cs2-highlight-tool-v2/internal/demo"
)

func TestBuild_FullRoundPOVBeforeVictimClip(t *testing.T) {
	result, err := Build([]Item{
		{
			Kill:           demo.ClipKill{ID: "k1", Tick: 500, KillerSlot: 12, VictimSlot: 7},
			IncludeKiller:  boolPtr(false),
			IncludeVictim:  true,
			VictimSpecMode: 1,
		},
	}, BuildOptions{
		TickRate:          64,
		VictimPreSeconds:  1,
		VictimPostSeconds: 1,
		RecordFPS:         60,
		RecordQuality:     "high",
		VideoPreset:       "c1",
		RecordOutputDir:   "outputs",
		FullRoundPOVSegments: []FullRoundPOVSegment{
			{
				Round:         1,
				StartTick:     1000,
				EndTick:       1800,
				Target:        "12",
				PlayerName:    "target",
				PlayerSteamID: "76561190000000001",
				EndReason:     "round_end",
			},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(result.TakePlans) != 2 {
		t.Fatalf("take plans len=%d want 2", len(result.TakePlans))
	}
	first := result.TakePlans[0]
	if first.View != "full_round_pov" || first.Round != 1 || first.SourceID == "" {
		t.Fatalf("unexpected first take plan: %+v", first)
	}
	second := result.TakePlans[1]
	if second.View != "victim" || len(second.KillIDs) != 1 || second.KillIDs[0] != "k1" {
		t.Fatalf("unexpected second take plan: %+v", second)
	}
	if len(result.Sequences) < 3 {
		t.Fatalf("sequence len=%d want at least 3", len(result.Sequences))
	}
	assertAction(t, result.Sequences[1].Actions, "demo_gototick 872")
	assertAction(t, result.Sequences[1].Actions, "spec_player 12")
	assertAction(t, result.Sequences[2].Actions, "spec_player 7")
}

func boolPtr(v bool) *bool {
	return &v
}

func assertAction(t *testing.T, actions []Action, cmd string) {
	t.Helper()
	for _, action := range actions {
		if action.Cmd == cmd {
			return
		}
	}
	t.Fatalf("action %q not found in %#v", cmd, actions)
}

func TestBuild_PovRadarFullRoundPassesEachGetOwnSequenceAndWarmup(t *testing.T) {
	result, err := Build(nil, BuildOptions{
		TickRate:        64,
		RecordFPS:       60,
		RecordQuality:   "high",
		VideoPreset:     "c1",
		RecordOutputDir: "outputs",
		PovRadarEnabled: true,
		FullRoundPOVSegments: []FullRoundPOVSegment{
			{Round: 1, StartTick: 1000, EndTick: 1500, Target: "12", SpecMode: 1, PlayerSteamID: "76561190000000001"},
			{Round: 1, StartTick: 1200, EndTick: 1800, Target: "12", SpecMode: 1, PlayerSteamID: "76561190000000001"},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Sequences) != 3 {
		t.Fatalf("sequence len=%d want 3 (bootstrap + one per full-round pass)", len(result.Sequences))
	}
	if len(result.TakePlans) != 2 {
		t.Fatalf("take plans len=%d want 2", len(result.TakePlans))
	}
	// The second pass overlaps the first source window; it must still prepare in
	// its own sequence instead of relying on the previous pass end clamp.
	for i, seq := range result.Sequences[1:] {
		radar := mustFindAction(t, seq.Actions, "csdm_radar_pov 1")
		start := mustFindAction(t, seq.Actions, "mirv_streams record start")
		if warmup := start.Tick - radar.Tick; warmup < 128 {
			t.Fatalf("full-round pass %d warmup=%d ticks want >= 128", i, warmup)
		}
	}
	assertAction(t, result.Sequences[1].Actions, "go_to_next_sequence")
	assertAction(t, result.Sequences[2].Actions, "disconnect")
}
