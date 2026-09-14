package plugingen

import (
	"testing"

	"cs2-highlight-tool-v2/internal/demo"
)

func TestBuildFullRoundPOVSegments_UsesStableSourceAndEndPadding(t *testing.T) {
	plan := &demo.FullRoundPOVPlan{
		PlayerName:    "Player",
		PlayerSteamID: "765",
		Segments: []demo.FullRoundPOVSegment{
			{
				Round:           3,
				RecordStartTick: 100,
				RecordEndTick:   300,
				TargetSlot:      7,
				EndReason:       demo.FullRoundPOVEndTargetDeath,
			},
		},
	}
	segments := BuildFullRoundPOVSegments(plan, FullRoundPOVSettings{EnableVoice: true, EnableSpecShowXray: true}, 64)
	if len(segments) != 1 {
		t.Fatalf("segments=%d want 1", len(segments))
	}
	got := segments[0]
	if got.SourceID != "full_round_pov:r3:p765" || got.Target != "7" {
		t.Fatalf("stable identity mismatch: %+v", got)
	}
	if got.EndTick != 364 {
		t.Fatalf("end tick=%d want 364", got.EndTick)
	}
	if !got.EnableVoice || !got.EnableSpecShowXray {
		t.Fatalf("settings not carried: %+v", got)
	}
}
