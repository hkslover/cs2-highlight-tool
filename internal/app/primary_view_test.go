package app

import (
	"testing"

	"cs2-highlight-tool-v2/internal/demo"
)

func primaryViewTestDefaults() ClipSettings {
	return ClipSettings{
		KillerPreSeconds:  5,
		KillerPostSeconds: 4,
		VictimPreSeconds:  2,
		VictimPostSeconds: 1,
	}
}

func normalizeOneItem(t *testing.T, item SelectedClipItem) (pre, post, oppPre, oppPost float64) {
	t.Helper()
	normalized, err := normalizeSelectedItems(
		GeneratePluginJSONRequest{SelectedItems: []SelectedClipItem{item}},
		primaryViewTestDefaults(),
	)
	if err != nil {
		t.Fatalf("normalizeSelectedItems: %v", err)
	}
	if len(normalized.Items) != 1 {
		t.Fatalf("items=%d want 1", len(normalized.Items))
	}
	got := normalized.Items[0]
	return got.KillerPreSeconds, got.KillerPostSeconds, got.VictimPreSeconds, got.VictimPostSeconds
}

// A kill clip: the selected player is the killer, so the killer pass gets the
// primary (long) window and the victim pass the opponent (short) one.
func TestNormalizeSelectedItems_KillerPrimaryKeepsWindows(t *testing.T) {
	killerPre, killerPost, victimPre, victimPost := normalizeOneItem(t, SelectedClipItem{
		Kill:          demo.ClipKill{ID: "k1", Tick: 1000},
		IncludeVictim: true,
	})
	if killerPre != 5 || killerPost != 4 {
		t.Fatalf("killer window = %v/%v, want 5/4", killerPre, killerPost)
	}
	if victimPre != 2 || victimPost != 1 {
		t.Fatalf("victim window = %v/%v, want 2/1", victimPre, victimPost)
	}
}

// A death clip: the selected player is the victim, so the victim pass is the
// primary one and must receive the long window instead.
func TestNormalizeSelectedItems_VictimPrimarySwapsWindows(t *testing.T) {
	killerPre, killerPost, victimPre, victimPost := normalizeOneItem(t, SelectedClipItem{
		Kill:          demo.ClipKill{ID: "k1", Tick: 1000},
		IncludeVictim: true,
		PrimaryView:   PrimaryViewVictim,
	})
	if victimPre != 5 || victimPost != 4 {
		t.Fatalf("victim window = %v/%v, want 5/4", victimPre, victimPost)
	}
	if killerPre != 2 || killerPost != 1 {
		t.Fatalf("killer window = %v/%v, want 2/1", killerPre, killerPost)
	}
}

// Per-item overrides stay keyed by role, so they land on their pass unchanged
// and are not affected by the primary/opponent swap.
func TestNormalizeSelectedItems_VictimPrimaryHonoursRoleKeyedOverrides(t *testing.T) {
	override := 9.5
	killerPre, _, victimPre, _ := normalizeOneItem(t, SelectedClipItem{
		Kill:          demo.ClipKill{ID: "k1", Tick: 1000},
		IncludeVictim: true,
		PrimaryView:   PrimaryViewVictim,
		ClipOverrides: &ClipItemOverrides{VictimPreSeconds: &override},
	})
	if victimPre != override {
		t.Fatalf("victim pre = %v, want %v", victimPre, override)
	}
	if killerPre != 2 {
		t.Fatalf("killer pre = %v, want swapped default 2", killerPre)
	}
}

func TestIsVictimPrimaryView(t *testing.T) {
	for _, value := range []string{"victim", "VICTIM", " victim "} {
		if !isVictimPrimaryView(value) {
			t.Fatalf("%q should be a victim primary view", value)
		}
	}
	for _, value := range []string{"", "killer", "other"} {
		if isVictimPrimaryView(value) {
			t.Fatalf("%q should not be a victim primary view", value)
		}
	}
}
