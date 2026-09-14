package plugingen

import (
	"strings"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/demo"
)

// ClipItemOverrides contains the per-clip settings that are allowed to
// override the generation defaults. It deliberately mirrors the wire shape
// without depending on the app/Wails DTOs.
type ClipItemOverrides struct {
	KillerPreSeconds   *float64
	KillerPostSeconds  *float64
	VictimPreSeconds   *float64
	VictimPostSeconds  *float64
	EnableVoice        *bool
	EnableSpecShowXray *bool
}

// SelectedItem is the domain representation of one selected kill. App-layer
// DTOs are adapted to this type before entering the planner.
type SelectedItem struct {
	Kill           demo.ClipKill
	IncludeKiller  *bool
	IncludeVictim  bool
	KillerSpecMode int
	VictimSpecMode int
	PrimaryView    string
	ClipOverrides  *ClipItemOverrides
}

// SelectionSettings is the small, explicit settings snapshot needed to
// normalize selected clips. It is intentionally independent of config.Config
// and the app's public settings DTO.
type SelectionSettings struct {
	KillerPreSeconds   float64
	KillerPostSeconds  float64
	VictimPreSeconds   float64
	VictimPostSeconds  float64
	EnableVoice        bool
	EnableSpecShowXray bool
}

// NormalizedSelection is the planner-ready selection. The slices are owned by
// the result and can safely be retained while an App executes a plan.
type NormalizedSelection struct {
	Items                   []clipsjson.Item
	FullRoundPOVSegments    []clipsjson.FullRoundPOVSegment
	HasVoiceOverride        bool
	HasSpecShowXrayOverride bool
}

// IsVictimPrimaryView reports whether the selected player is the victim side
// of a kill. The comparison is intentionally tolerant of casing/whitespace so
// it matches the public request compatibility behavior.
func IsVictimPrimaryView(primaryView string) bool {
	return strings.EqualFold(strings.TrimSpace(primaryView), "victim")
}

// NormalizeSelectedItems applies compatibility defaults, primary-view window
// mapping, per-clip overrides, and the clipsjson input shape. It performs no
// I/O, reads no clock, and does not mutate the caller's slices.
//
// selectedKills is the legacy compatibility input. It is used only when the
// new selectedItems slice is empty, and preserves the historical default of
// recording the victim pass for each legacy kill.
func NormalizeSelectedItems(
	selectedItems []SelectedItem,
	selectedKills []demo.ClipKill,
	defaults SelectionSettings,
) (*NormalizedSelection, error) {
	items := make([]SelectedItem, 0, len(selectedItems)+len(selectedKills))
	items = append(items, selectedItems...)
	if len(items) == 0 && len(selectedKills) > 0 {
		for _, kill := range selectedKills {
			items = append(items, SelectedItem{Kill: kill, IncludeVictim: true})
		}
	}
	if len(items) == 0 {
		return &NormalizedSelection{}, nil
	}

	normalized := make([]clipsjson.Item, 0, len(items))
	result := &NormalizedSelection{}
	for _, item := range items {
		// Killer*/Victim* are positional windows: they describe the selected
		// player's pass and the opponent pass. Death-mode selections therefore
		// swap the defaults before applying explicit per-clip overrides.
		killerPreDefault := defaults.KillerPreSeconds
		killerPostDefault := defaults.KillerPostSeconds
		victimPreDefault := defaults.VictimPreSeconds
		victimPostDefault := defaults.VictimPostSeconds
		if IsVictimPrimaryView(item.PrimaryView) {
			killerPreDefault, victimPreDefault = victimPreDefault, killerPreDefault
			killerPostDefault, victimPostDefault = victimPostDefault, killerPostDefault
		}

		killerPreSeconds := killerPreDefault
		killerPostSeconds := killerPostDefault
		victimPreSeconds := victimPreDefault
		victimPostSeconds := victimPostDefault
		enableVoice := defaults.EnableVoice
		enableSpecShowXray := defaults.EnableSpecShowXray
		itemHasVoiceOverride := false
		itemHasSpecShowXrayOverride := false

		if override := item.ClipOverrides; override != nil {
			if override.KillerPreSeconds != nil {
				killerPreSeconds = *override.KillerPreSeconds
			}
			if override.KillerPostSeconds != nil {
				killerPostSeconds = *override.KillerPostSeconds
			}
			if override.VictimPreSeconds != nil {
				victimPreSeconds = *override.VictimPreSeconds
			}
			if override.VictimPostSeconds != nil {
				victimPostSeconds = *override.VictimPostSeconds
			}
			if override.EnableVoice != nil {
				enableVoice = *override.EnableVoice
				itemHasVoiceOverride = true
				result.HasVoiceOverride = true
			}
			if override.EnableSpecShowXray != nil {
				enableSpecShowXray = *override.EnableSpecShowXray
				itemHasSpecShowXrayOverride = true
				result.HasSpecShowXrayOverride = true
			}
		}

		killerPreSeconds = config.NormalizeClipWindowSeconds(killerPreSeconds, killerPreDefault)
		killerPostSeconds = config.NormalizeClipWindowSeconds(killerPostSeconds, killerPostDefault)
		victimPreSeconds = config.NormalizeClipWindowSeconds(victimPreSeconds, victimPreDefault)
		victimPostSeconds = config.NormalizeClipWindowSeconds(victimPostSeconds, victimPostDefault)

		normalized = append(normalized, clipsjson.Item{
			Kill:                    item.Kill,
			IncludeKiller:           cloneBoolPointer(item.IncludeKiller),
			IncludeVictim:           item.IncludeVictim,
			KillerSpecMode:          1,
			VictimSpecMode:          1,
			KillerPreSeconds:        killerPreSeconds,
			KillerPostSeconds:       killerPostSeconds,
			VictimPreSeconds:        victimPreSeconds,
			VictimPostSeconds:       victimPostSeconds,
			EnableVoice:             enableVoice,
			EnableSpecShowXray:      enableSpecShowXray,
			HasVoiceOverride:        itemHasVoiceOverride,
			HasSpecShowXrayOverride: itemHasSpecShowXrayOverride,
		})
	}
	result.Items = normalized
	return result, nil
}

func cloneBoolPointer(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
