package plugingen

import (
	"strings"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/demo"
)

// RegisterProduceKillSnapshot merges the selected clip facts and any
// full-round POV facts into a per-demo snapshot. The caller owns the map and
// supplies a stable history/plan snapshot; no app state is consulted.
func RegisterProduceKillSnapshot(
	store map[string]map[string]demo.ClipKill,
	plans []TakePlan,
	items []clipsjson.Item,
	povKillsByDemo map[string]map[string]demo.ClipKill,
	fallbackDemoPath string,
) {
	if store == nil {
		return
	}
	killByID := make(map[string]demo.ClipKill, len(items))
	for _, item := range items {
		killID := strings.TrimSpace(item.Kill.ID)
		if killID == "" {
			continue
		}
		kill := item.Kill
		kill.ID = killID
		if _, exists := killByID[killID]; !exists {
			killByID[killID] = kill
		}
	}
	for _, povKills := range povKillsByDemo {
		for killID, kill := range povKills {
			id := strings.TrimSpace(killID)
			if id == "" {
				continue
			}
			if _, exists := killByID[id]; exists {
				continue
			}
			kill.ID = id
			killByID[id] = kill
		}
	}
	if len(killByID) == 0 {
		return
	}

	demoPaths := make(map[string]struct{}, len(plans)+len(povKillsByDemo)+1)
	for _, plan := range plans {
		if demoPath := strings.TrimSpace(plan.DemoPath); demoPath != "" {
			demoPaths[demoPath] = struct{}{}
		}
	}
	for demoPath := range povKillsByDemo {
		if demoPath = strings.TrimSpace(demoPath); demoPath != "" {
			demoPaths[demoPath] = struct{}{}
		}
	}
	if demoPath := strings.TrimSpace(fallbackDemoPath); demoPath != "" {
		demoPaths[demoPath] = struct{}{}
	}
	for demoPath := range demoPaths {
		current := store[demoPath]
		if current == nil {
			current = make(map[string]demo.ClipKill, len(killByID))
			store[demoPath] = current
		}
		for killID, kill := range killByID {
			if _, exists := current[killID]; exists {
				continue
			}
			current[killID] = kill
		}
	}
}

// CollectFullRoundPOVKills extracts the tracked player's kills for the rounds
// represented by full_round_pov plans. Parsing the demo is deliberately left
// to the app; this function only transforms an already parsed metadata
// snapshot and is therefore deterministic and easy to test.
func CollectFullRoundPOVKills(meta *demo.Metadata, plans []TakePlan) map[string]demo.ClipKill {
	result := make(map[string]demo.ClipKill)
	if meta == nil {
		return result
	}
	type tracker struct {
		playerSteamID string
		rounds        map[int]struct{}
	}
	trackers := make([]tracker, 0)
	for _, plan := range plans {
		if strings.ToLower(strings.TrimSpace(plan.View)) != "full_round_pov" {
			continue
		}
		playerSteamID := strings.TrimSpace(plan.PlayerSteamID)
		if playerSteamID == "" || plan.Round <= 0 {
			continue
		}
		var current *tracker
		for i := range trackers {
			if trackers[i].playerSteamID == playerSteamID {
				current = &trackers[i]
				break
			}
		}
		if current == nil {
			trackers = append(trackers, tracker{playerSteamID: playerSteamID, rounds: make(map[int]struct{})})
			current = &trackers[len(trackers)-1]
		}
		current.rounds[plan.Round] = struct{}{}
	}
	for _, wanted := range trackers {
		for _, player := range meta.ClipPlayers {
			if strings.TrimSpace(player.SteamID) != wanted.playerSteamID {
				continue
			}
			for _, round := range player.Rounds {
				if _, ok := wanted.rounds[round.Round]; !ok {
					continue
				}
				for _, kill := range round.Kills {
					id := strings.TrimSpace(kill.ID)
					if id == "" {
						continue
					}
					if _, exists := result[id]; exists {
						continue
					}
					kill.ID = id
					result[id] = kill
				}
			}
			break
		}
	}
	return result
}
