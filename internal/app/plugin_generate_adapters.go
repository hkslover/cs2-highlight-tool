package app

import (
	"fmt"
	"strconv"
	"strings"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/demo"
	"cs2-highlight-tool-v2/internal/plugingen"
)

func filterItemsByHistory(
	items []clipsjson.Item,
	plans []ProduceTakePlan,
	historyKeys map[string]struct{},
) []clipsjson.Item {
	return plugingen.FilterItemsByHistory(items, toPlugingenTakePlans(plans), historyKeys)
}

func filterFullRoundPOVSegmentsByHistory(
	segments []clipsjson.FullRoundPOVSegment,
	plans []ProduceTakePlan,
	historyKeys map[string]struct{},
	fallbackDemoPath string,
) []clipsjson.FullRoundPOVSegment {
	return plugingen.FilterFullRoundPOVSegmentsByHistory(segments, toPlugingenTakePlans(plans), historyKeys, fallbackDemoPath)
}

// toPlugingenTakePlans converts app-layer ProduceTakePlan slice to plugingen.TakePlan.
func toPlugingenTakePlans(plans []ProduceTakePlan) []plugingen.TakePlan {
	out := make([]plugingen.TakePlan, len(plans))
	for i, p := range plans {
		out[i] = plugingen.TakePlan{
			DemoPath:      p.DemoPath,
			View:          p.View,
			SpecMode:      p.SpecMode,
			KillIDs:       append([]string(nil), p.KillIDs...),
			SourceID:      p.SourceID,
			Round:         p.Round,
			PlayerSteamID: p.PlayerSteamID,
		}
	}
	return out
}

func registerProduceKillSnapshot(
	store map[string]map[string]demo.ClipKill,
	plans []ProduceTakePlan,
	items []clipsjson.Item,
	fallbackDemoPath string,
) {
	povKillsByDemo := collectFullRoundPOVKills(plans)
	plugingen.RegisterProduceKillSnapshot(store, toPlugingenTakePlans(plans), items, povKillsByDemo, fallbackDemoPath)
}

// collectFullRoundPOVKills resolves the tracked player's per-round kills for
// every full_round_pov take plan by parsing the underlying demo. The result is
// keyed by demo path so the caller can fold the kills into the kill snapshot.
// Errors are non-fatal: a parse failure simply yields no extra kills.
func collectFullRoundPOVKills(plans []ProduceTakePlan) map[string]map[string]demo.ClipKill {
	byDemo := make(map[string]map[string]demo.ClipKill)
	plansByDemo := make(map[string][]ProduceTakePlan)
	for _, plan := range plans {
		if strings.ToLower(strings.TrimSpace(plan.View)) != "full_round_pov" {
			continue
		}
		playerSteamID := strings.TrimSpace(plan.PlayerSteamID)
		demoPath := strings.TrimSpace(plan.DemoPath)
		if playerSteamID == "" || demoPath == "" {
			continue
		}
		round := plan.Round
		if round <= 0 {
			continue
		}
		plansByDemo[demoPath] = append(plansByDemo[demoPath], plan)
	}

	for demoPath, demoPlans := range plansByDemo {
		meta, err := demo.ParseMetadata(demoPath)
		if err != nil || meta == nil {
			continue
		}
		converted := make([]plugingen.TakePlan, 0, len(demoPlans))
		for _, plan := range demoPlans {
			converted = append(converted, plugingen.TakePlan{
				DemoPath:      plan.DemoPath,
				View:          plan.View,
				SpecMode:      plan.SpecMode,
				KillIDs:       append([]string(nil), plan.KillIDs...),
				SourceID:      plan.SourceID,
				Round:         plan.Round,
				PlayerSteamID: plan.PlayerSteamID,
			})
		}
		byDemo[demoPath] = plugingen.CollectFullRoundPOVKills(meta, converted)
	}
	return byDemo
}

func normalizeSelectedItems(req GeneratePluginJSONRequest, defaults ClipSettings) (*normalizedSelectedItems, error) {
	fullRoundPOVSegments, err := normalizeFullRoundPOVSelection(req, defaults)
	if err != nil {
		return nil, err
	}
	selection, err := normalizeSelectedClipItems(req, defaults)
	if err != nil {
		return nil, err
	}
	return &normalizedSelectedItems{
		Items:                   selection.Items,
		FullRoundPOVSegments:    fullRoundPOVSegments,
		HasVoiceOverride:        selection.HasVoiceOverride,
		HasSpecShowXrayOverride: selection.HasSpecShowXrayOverride,
	}, nil
}

// normalizeSelectedClipItems adapts the public App request to the pure
// plugingen selection normalizer. Full-round parsing is intentionally kept out
// of this helper so launch filtering can reuse a frozen selection without
// re-reading the demo.
func normalizeSelectedClipItems(req GeneratePluginJSONRequest, defaults ClipSettings) (*plugingen.NormalizedSelection, error) {
	items := make([]plugingen.SelectedItem, 0, len(req.SelectedItems))
	for _, item := range req.SelectedItems {
		var overrides *plugingen.ClipItemOverrides
		if item.ClipOverrides != nil {
			overrides = &plugingen.ClipItemOverrides{
				KillerPreSeconds:   item.ClipOverrides.KillerPreSeconds,
				KillerPostSeconds:  item.ClipOverrides.KillerPostSeconds,
				VictimPreSeconds:   item.ClipOverrides.VictimPreSeconds,
				VictimPostSeconds:  item.ClipOverrides.VictimPostSeconds,
				EnableVoice:        item.ClipOverrides.EnableVoice,
				EnableSpecShowXray: item.ClipOverrides.EnableSpecShowXray,
			}
		}
		items = append(items, plugingen.SelectedItem{
			Kill:           item.Kill,
			IncludeKiller:  item.IncludeKiller,
			IncludeVictim:  item.IncludeVictim,
			KillerSpecMode: item.KillerSpecMode,
			VictimSpecMode: item.VictimSpecMode,
			PrimaryView:    item.PrimaryView,
			ClipOverrides:  overrides,
		})
	}
	return plugingen.NormalizeSelectedItems(items, req.SelectedKills, plugingen.SelectionSettings{
		KillerPreSeconds:   defaults.KillerPreSeconds,
		KillerPostSeconds:  defaults.KillerPostSeconds,
		VictimPreSeconds:   defaults.VictimPreSeconds,
		VictimPostSeconds:  defaults.VictimPostSeconds,
		EnableVoice:        defaults.EnableVoice,
		EnableSpecShowXray: defaults.EnableSpecShowXray,
	})
}

func normalizeFullRoundPOVSelection(req GeneratePluginJSONRequest, defaults ClipSettings) ([]clipsjson.FullRoundPOVSegment, error) {
	if req.FullRoundPOV == nil {
		return nil, nil
	}
	steamIDText := strings.TrimSpace(req.FullRoundPOV.PlayerSteamID)
	if steamIDText == "" {
		return nil, fmt.Errorf("整局 POV 跟踪玩家为空")
	}
	steamID, err := strconv.ParseUint(steamIDText, 10, 64)
	if err != nil || steamID == 0 {
		return nil, fmt.Errorf("整局 POV 跟踪玩家 SteamID 无效")
	}
	plan, err := demo.ParseFullRoundPOVPlan(req.DemoPath, steamID)
	if err != nil {
		return nil, err
	}
	segments := plugingen.BuildFullRoundPOVSegments(plan, plugingen.FullRoundPOVSettings{
		EnableVoice:        defaults.EnableVoice,
		EnableSpecShowXray: defaults.EnableSpecShowXray,
	}, req.TickRate)
	if len(segments) == 0 {
		return nil, fmt.Errorf("没有可导出的整局 POV 回合片段")
	}
	return segments, nil
}
