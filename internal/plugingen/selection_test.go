package plugingen

import (
	"testing"

	"cs2-highlight-tool-v2/internal/demo"
)

func TestNormalizeSelectedItems_PrimaryViewAndLegacyDefaults(t *testing.T) {
	killer := demo.ClipKill{ID: "killer", Tick: 640, KillerSlot: 7, VictimSlot: 11}
	victim := demo.ClipKill{ID: "victim", Tick: 640, KillerSlot: 7, VictimSlot: 11}
	selection, err := NormalizeSelectedItems([]SelectedItem{
		{
			Kill:          killer,
			IncludeVictim: false,
		},
		{
			Kill:          victim,
			PrimaryView:   "victim",
			IncludeVictim: true,
		},
	}, nil, SelectionSettings{
		KillerPreSeconds:  5,
		KillerPostSeconds: 6,
		VictimPreSeconds:  1,
		VictimPostSeconds: 2,
	})
	if err != nil {
		t.Fatalf("NormalizeSelectedItems: %v", err)
	}
	if len(selection.Items) != 2 {
		t.Fatalf("items=%d want 2", len(selection.Items))
	}
	if got := selection.Items[0].KillerPreSeconds; got != 5 {
		t.Fatalf("killer primary pre=%v want 5", got)
	}
	if got := selection.Items[1].KillerPreSeconds; got != 1 {
		t.Fatalf("victim primary killer pre=%v want swapped 1", got)
	}
	if got := selection.Items[1].VictimPostSeconds; got != 6 {
		t.Fatalf("victim primary victim post=%v want swapped 6", got)
	}

	legacy, err := NormalizeSelectedItems(nil, []demo.ClipKill{{ID: "legacy", Tick: 200, KillerSlot: 7, VictimSlot: 11}}, SelectionSettings{})
	if err != nil {
		t.Fatalf("legacy NormalizeSelectedItems: %v", err)
	}
	if len(legacy.Items) != 1 || !legacy.Items[0].IncludeVictim {
		t.Fatalf("legacy selection should include victim: %+v", legacy.Items)
	}
	if legacy.Items[0].IncludeKiller != nil {
		t.Fatalf("legacy include_killer should preserve nil compatibility default, got %v", legacy.Items[0].IncludeKiller)
	}
}

func TestNormalizeSelectedItems_OverrideFlagsAreExplicit(t *testing.T) {
	voice := false
	xray := false
	selection, err := NormalizeSelectedItems([]SelectedItem{{
		Kill:          demo.ClipKill{ID: "k1", Tick: 100, KillerSlot: 1},
		ClipOverrides: &ClipItemOverrides{EnableVoice: &voice, EnableSpecShowXray: &xray},
	}}, nil, SelectionSettings{EnableVoice: true, EnableSpecShowXray: true})
	if err != nil {
		t.Fatalf("NormalizeSelectedItems: %v", err)
	}
	if !selection.HasVoiceOverride || !selection.HasSpecShowXrayOverride {
		t.Fatalf("override flags not retained: %+v", selection)
	}
	if selection.Items[0].EnableVoice || selection.Items[0].EnableSpecShowXray {
		t.Fatalf("explicit false overrides lost: %+v", selection.Items[0])
	}
}
