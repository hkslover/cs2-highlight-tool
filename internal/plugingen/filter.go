package plugingen

import (
	"strings"

	"cs2-highlight-tool-v2/internal/clipsjson"
)

// TakePlan describes a single recording take — the demo file, camera view,
// spec mode, and the set of kill IDs included in that take.
// This mirrors the app-layer ProduceTakePlan but is defined here so that
// pure filtering logic has no dependency on the Wails binding layer.
type TakePlan struct {
	DemoPath      string
	View          string
	SpecMode      int
	KillIDs       []string
	SourceID      string
	Round         int
	PlayerSteamID string
}

// FilterFullRoundPOVSegmentsByHistory returns only full-round segments whose
// stable source IDs are not already present in history. The history set is a
// caller-provided snapshot; this function never consults app state.
func FilterFullRoundPOVSegmentsByHistory(
	segments []clipsjson.FullRoundPOVSegment,
	plans []TakePlan,
	historyKeys map[string]struct{},
	fallbackDemoPath string,
) []clipsjson.FullRoundPOVSegment {
	if len(segments) == 0 {
		return nil
	}
	keepBySourceID := make(map[string]bool, len(segments))
	for _, plan := range plans {
		if strings.ToLower(strings.TrimSpace(plan.View)) != "full_round_pov" {
			continue
		}
		sourceID := strings.TrimSpace(plan.SourceID)
		if sourceID == "" {
			continue
		}
		demoPath := strings.TrimSpace(plan.DemoPath)
		if demoPath == "" {
			demoPath = strings.TrimSpace(fallbackDemoPath)
		}
		key := BuildProduceHistoryKeyWithSourceID(demoPath, plan.View, plan.SpecMode, plan.KillIDs, sourceID)
		if _, exists := historyKeys[key]; exists {
			continue
		}
		keepBySourceID[sourceID] = true
	}
	filtered := make([]clipsjson.FullRoundPOVSegment, 0, len(segments))
	for _, segment := range segments {
		sourceID := strings.TrimSpace(segment.SourceID)
		if sourceID == "" {
			sourceID = BuildFullRoundPOVSourceID(segment.Round, segment.PlayerSteamID)
			segment.SourceID = sourceID
		}
		if keepBySourceID[sourceID] {
			filtered = append(filtered, segment)
		}
	}
	return filtered
}

// FilterItemsByHistory returns the subset of items whose corresponding take
// plans are NOT already present in historyKeys. Items are also adjusted so
// that only the needed killer/victim perspectives are included.
//
// historyKeys is a set of keys produced by BuildProduceHistoryKey.
func FilterItemsByHistory(
	items []clipsjson.Item,
	plans []TakePlan,
	historyKeys map[string]struct{},
) []clipsjson.Item {
	if len(items) == 0 || len(plans) == 0 {
		return nil
	}
	killerKeepByKillID := make(map[string]bool)
	victimKeepByKillID := make(map[string]bool)
	for _, plan := range plans {
		if _, exists := historyKeys[BuildProduceHistoryKeyWithSourceID(plan.DemoPath, plan.View, plan.SpecMode, plan.KillIDs, plan.SourceID)]; exists {
			continue
		}
		view := strings.ToLower(strings.TrimSpace(plan.View))
		for _, killID := range plan.KillIDs {
			id := strings.TrimSpace(killID)
			if id == "" {
				continue
			}
			if view == "victim" {
				victimKeepByKillID[id] = true
			} else {
				killerKeepByKillID[id] = true
			}
		}
	}

	filtered := make([]clipsjson.Item, 0, len(items))
	for _, item := range items {
		killID := strings.TrimSpace(item.Kill.ID)
		if killID == "" {
			continue
		}
		keepKiller := killerKeepByKillID[killID]
		keepVictim := victimKeepByKillID[killID]
		if !keepKiller && !keepVictim {
			continue
		}
		next := item
		v := keepKiller
		next.IncludeKiller = &v
		next.IncludeVictim = keepVictim
		filtered = append(filtered, next)
	}
	return filtered
}
