package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/plugingen"
)

// generationSnapshot is captured after workspace admission and reused for all
// jobs in one generation request. The Config pointer is a Store snapshot, not
// the mutable App configuration.
type generationSnapshot struct {
	cfg          *config.Config
	clipSettings ClipSettings
	planner      plugingen.GenerationSettings
}

// generatePluginJSONInternal captures one config snapshot for the standalone
// API. Batch callers use generatePluginJSONInternalWithSnapshot directly so
// every job observes the same settings.
func (a *App) generatePluginJSONInternal(
	req GeneratePluginJSONRequest,
	opts generatePluginJSONInternalOptions,
) (*GeneratePluginJSONResult, *normalizedSelectedItems, error) {
	snapshot, err := a.captureGenerationSnapshot()
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(req.BatchTimestamp) == "" {
		req.BatchTimestamp = time.Now().Format("20060102_150405")
	}
	return a.generatePluginJSONInternalWithSnapshot(req, opts, snapshot)
}

func (a *App) captureGenerationSnapshot() (*generationSnapshot, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("配置为空")
	}
	clipSettings := clipSettingsFromConfig(cfg, a.fixedRecordOutputDir())
	planner := plugingen.GenerationSettings{
		KillerPreSeconds:     clipSettings.KillerPreSeconds,
		KillerPostSeconds:    clipSettings.KillerPostSeconds,
		VictimPreSeconds:     clipSettings.VictimPreSeconds,
		VictimPostSeconds:    clipSettings.VictimPostSeconds,
		EnableVoice:          clipSettings.EnableVoice,
		RecordFPS:            clipSettings.RecordFPS,
		RecordQuality:        clipSettings.RecordQuality,
		VideoPreset:          clipSettings.VideoPreset,
		EffectiveVideoPreset: plugingen.ResolvePluginVideoPreset(clipSettings.VideoPreset, cfg),
		RecordOutputDir:      clipSettings.RecordOutputDir,
		EnableSpecShowXray:   clipSettings.EnableSpecShowXray,
		HideAllUI:            clipSettings.HideAllUI,
		HidePlayerAvatars:    clipSettings.HidePlayerAvatars,
		UseShoulderCamera:    clipSettings.UseShoulderCamera,
		PovRadarEnabled:      clipSettings.PovRadarEnabled,
		SkyBlackout:          clipSettings.SkyBlackout,
		DisableClouds:        clipSettings.DisableClouds,
		KillFeedLifetime:     clipSettings.KillFeedLifetime,
		BlockKillFeed:        clipSettings.BlockKillFeed,
		LaunchResolution:     clipSettings.LaunchResolution,
	}
	return &generationSnapshot{cfg: cfg, clipSettings: clipSettings, planner: planner}, nil
}

func clipSettingsFromConfig(cfg *config.Config, recordOutputDir string) ClipSettings {
	if cfg == nil {
		return normalizeClipSettings(ClipSettings{RecordOutputDir: recordOutputDir})
	}
	actionSettings := config.ResolveClipActionSettings(cfg)
	return normalizeClipSettings(ClipSettings{
		KillerPreSeconds:   cfg.KillerPreSeconds,
		KillerPostSeconds:  cfg.KillerPostSeconds,
		VictimPreSeconds:   cfg.VictimPreSeconds,
		VictimPostSeconds:  cfg.VictimPostSeconds,
		AutoAddVictimView:  cfg.AutoAddVictimView,
		EnableVoice:        actionSettings.EnableVoiceIndices && actionSettings.EnableVoiceIndicesH,
		RecordFPS:          cfg.RecordFPS,
		RecordQuality:      cfg.RecordQuality,
		EditFPS:            cfg.EditFPS,
		EditQuality:        cfg.EditQuality,
		VideoPreset:        cfg.VideoPreset,
		LaunchResolution:   cfg.LaunchResolution,
		RecordOutputDir:    recordOutputDir,
		EnableSpecShowXray: cfg.EnableSpecShowXray,
		HideAllUI:          cfg.HideAllUI,
		HidePlayerAvatars:  cfg.HidePlayerAvatars,
		UseShoulderCamera:  cfg.UseShoulderCamera,
		PovHudEnabled:      cfg.PovHudEnabled,
		PovRadarEnabled:    cfg.PovRadarEnabled,
		SkyBlackout:        cfg.SkyBlackout,
		DisableClouds:      cfg.DisableClouds,
		KillFeedLifetime:   cfg.KillFeedLifetime,
		BlockKillFeed:      cfg.BlockKillFeed,
	})
}

// generatePluginJSONInternalWithSnapshot is the App execution shell around
// plugingen.BuildPlan. It validates paths and optionally writes JSON, while all
// selection windows, ordering, command timing, and take metadata come from the
// pure planner output.
func (a *App) generatePluginJSONInternalWithSnapshot(
	req GeneratePluginJSONRequest,
	opts generatePluginJSONInternalOptions,
	snapshot *generationSnapshot,
) (*GeneratePluginJSONResult, *normalizedSelectedItems, error) {
	if snapshot == nil || snapshot.cfg == nil {
		return nil, nil, fmt.Errorf("配置快照为空")
	}
	demoPath := strings.TrimSpace(req.DemoPath)
	if demoPath == "" {
		return nil, nil, fmt.Errorf("demo 路径为空")
	}
	absDemoPath, err := filepath.Abs(demoPath)
	if err != nil {
		return nil, nil, fmt.Errorf("解析 demo 路径失败: %w", err)
	}
	if _, err := os.Stat(absDemoPath); err != nil {
		return nil, nil, fmt.Errorf("demo 文件不存在: %s", absDemoPath)
	}

	selection := &normalizedSelectedItems{}
	if opts.UseItemsOverride {
		items := append([]clipsjson.Item(nil), opts.ItemsOverride...)
		selection.Items = items
		selection.HasVoiceOverride = itemsHaveVoiceOverride(items)
		selection.HasSpecShowXrayOverride = itemsHaveSpecShowXrayOverride(items)
	} else {
		selected, normalizeErr := normalizeSelectedClipItems(req, snapshot.clipSettings)
		if normalizeErr != nil {
			return nil, nil, normalizeErr
		}
		selection.Items = selected.Items
		selection.HasVoiceOverride = selected.HasVoiceOverride
		selection.HasSpecShowXrayOverride = selected.HasSpecShowXrayOverride
	}
	if opts.UseFullRoundPOVSegmentsOverride {
		selection.FullRoundPOVSegments = append([]clipsjson.FullRoundPOVSegment(nil), opts.FullRoundPOVSegmentsOverride...)
	} else {
		fullRoundSegments, fullRoundErr := normalizeFullRoundPOVSelection(req, snapshot.clipSettings)
		if fullRoundErr != nil {
			return nil, nil, fullRoundErr
		}
		selection.FullRoundPOVSegments = fullRoundSegments
	}
	if len(selection.Items) == 0 && len(selection.FullRoundPOVSegments) == 0 {
		return nil, nil, fmt.Errorf("没有可导出的录制片段")
	}

	plan, err := plugingen.BuildPlan(plugingen.PlanInput{
		DemoPath:             absDemoPath,
		Items:                selection.Items,
		FullRoundPOVSegments: selection.FullRoundPOVSegments,
		TickRate:             req.TickRate,
		MatchEndTick:         req.MatchEndTick,
		ExtraCommands:        req.ExtraCommands,
		BatchTimestamp:       req.BatchTimestamp,
		RecordSubDir:         opts.RecordSubDir,
		Settings:             snapshot.planner,
	})
	if err != nil {
		return nil, nil, err
	}

	jsonPath := absDemoPath + ".json"
	if opts.WriteJSON {
		payload, marshalErr := json.MarshalIndent(plan.Sequences, "", "  ")
		if marshalErr != nil {
			return nil, nil, fmt.Errorf("序列化 json 失败: %w", marshalErr)
		}
		payload = append(payload, '\n')
		if writeErr := os.WriteFile(jsonPath, payload, 0644); writeErr != nil {
			return nil, nil, fmt.Errorf("写入 json 失败: %w", writeErr)
		}
	}

	return &GeneratePluginJSONResult{
		JSONPath:      jsonPath,
		SequenceCount: len(plan.Sequences),
		SegmentCount:  plan.SegmentCount,
		ActionCount:   countPlanActions(plan.Sequences),
		TakePlans:     adaptTakePlans(absDemoPath, plan.TakePlans),
	}, selection, nil
}

func itemsHaveVoiceOverride(items []clipsjson.Item) bool {
	for _, item := range items {
		if item.HasVoiceOverride {
			return true
		}
	}
	return false
}

func itemsHaveSpecShowXrayOverride(items []clipsjson.Item) bool {
	for _, item := range items {
		if item.HasSpecShowXrayOverride {
			return true
		}
	}
	return false
}

func countPlanActions(sequences []clipsjson.Sequence) int {
	count := 0
	for _, sequence := range sequences {
		count += len(sequence.Actions)
	}
	return count
}

func adaptTakePlans(demoPath string, plans []clipsjson.TakePlan) []ProduceTakePlan {
	if len(plans) == 0 {
		return nil
	}
	result := make([]ProduceTakePlan, 0, len(plans))
	for _, plan := range plans {
		result = append(result, ProduceTakePlan{
			DemoPath:           demoPath,
			TakeIndex:          plan.TakeIndex,
			TakeName:           plan.TakeName,
			View:               strings.TrimSpace(plan.View),
			SpecMode:           plan.SpecMode,
			KillIDs:            append([]string(nil), plan.KillIDs...),
			SourceID:           strings.TrimSpace(plan.SourceID),
			Round:              plan.Round,
			PlayerName:         strings.TrimSpace(plan.PlayerName),
			PlayerSteamID:      strings.TrimSpace(plan.PlayerSteamID),
			StartTick:          plan.StartTick,
			EndTick:            plan.EndTick,
			EndReason:          strings.TrimSpace(plan.EndReason),
			TickRate:           plan.TickRate,
			RecordStartTick:    plan.RecordStartTick,
			RecordEndTick:      plan.RecordEndTick,
			KillOffsetsSeconds: append([]float64(nil), plan.KillOffsetsSeconds...),
		})
	}
	return result
}
