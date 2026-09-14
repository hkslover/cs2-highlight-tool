package plugingen

import (
	"fmt"
	"strings"

	"cs2-highlight-tool-v2/internal/clipsjson"
)

// GenerationSettings is the explicit, immutable settings snapshot consumed by
// the generation planner. App captures it once after workspace admission and
// can then reuse it for every job in a batch.
type GenerationSettings struct {
	KillerPreSeconds     float64
	KillerPostSeconds    float64
	VictimPreSeconds     float64
	VictimPostSeconds    float64
	EnableVoice          bool
	RecordFPS            int
	RecordQuality        string
	VideoPreset          string
	EffectiveVideoPreset string
	RecordOutputDir      string
	EnableSpecShowXray   bool
	HideAllUI            bool
	HidePlayerAvatars    bool
	UseShoulderCamera    bool
	PovRadarEnabled      bool
	SkyBlackout          bool
	DisableClouds        bool
	KillFeedLifetime     int
	BlockKillFeed        bool
	LaunchResolution     string
}

// PlanInput contains all data needed to produce plugin actions and take
// metadata. It intentionally has no App, Wails, lock, process, clock, or
// filesystem dependency.
type PlanInput struct {
	DemoPath             string
	Items                []clipsjson.Item
	FullRoundPOVSegments []clipsjson.FullRoundPOVSegment
	TickRate             float64
	MatchEndTick         int
	ExtraCommands        []string
	BatchTimestamp       string
	RecordSubDir         string
	Settings             GenerationSettings
}

// GenerationPlan is the pure planner output. The App may serialize its
// sequences and adapt its take plans to the public Wails DTO, but it should not
// need to recalculate selection windows, ordering, IDs, or command timing.
type GenerationPlan struct {
	Sequences               []clipsjson.Sequence
	SegmentCount            int
	TakePlans               []clipsjson.TakePlan
	HistoryKeys             []string
	Items                   []clipsjson.Item
	FullRoundPOVSegments    []clipsjson.FullRoundPOVSegment
	HasVoiceOverride        bool
	HasSpecShowXrayOverride bool
}

// BuildPlan constructs a deterministic plugin generation plan from an
// explicit settings and selection snapshot. BatchTimestamp and RecordSubDir
// must be supplied by the caller when stable naming is required; this function
// never reads the system clock.
func BuildPlan(input PlanInput) (*GenerationPlan, error) {
	items := cloneItems(input.Items)
	fullRoundSegments := cloneFullRoundSegments(input.FullRoundPOVSegments)
	if len(items) == 0 && len(fullRoundSegments) == 0 {
		return nil, fmt.Errorf("没有可导出的录制片段")
	}

	settings := input.Settings
	videoPreset := strings.TrimSpace(settings.EffectiveVideoPreset)
	if videoPreset == "" {
		videoPreset = ResolvePluginVideoPreset(settings.VideoPreset, nil)
	}
	recordBatchName := strings.TrimSpace(input.BatchTimestamp)
	if subDir := strings.TrimSpace(input.RecordSubDir); subDir != "" {
		if recordBatchName != "" {
			recordBatchName += "/" + subDir
		} else {
			recordBatchName = subDir
		}
	}

	buildResult, err := clipsjson.Build(items, clipsjson.BuildOptions{
		TickRate:             input.TickRate,
		MatchEndTick:         input.MatchEndTick,
		KillerPreSeconds:     settings.KillerPreSeconds,
		KillerPostSeconds:    settings.KillerPostSeconds,
		VictimPreSeconds:     settings.VictimPreSeconds,
		VictimPostSeconds:    settings.VictimPostSeconds,
		FullRoundPOVSegments: fullRoundSegments,
		ExtraCommands:        cloneStrings(input.ExtraCommands),
		ActionSettings: clipsjson.ActionSettings{
			EnableVoiceIndices:  settings.EnableVoice,
			VoiceIndicesValue:   0,
			EnableVoiceIndicesH: settings.EnableVoice,
			VoiceIndicesHValue:  0,
		},
		RecordFPS:                 settings.RecordFPS,
		RecordQuality:             settings.RecordQuality,
		VideoPreset:               videoPreset,
		RecordOutputDir:           settings.RecordOutputDir,
		RecordBatchName:           recordBatchName,
		EnableSpecShowXray:        settings.EnableSpecShowXray,
		HideAllUI:                 settings.HideAllUI,
		HidePlayerAvatars:         settings.HidePlayerAvatars,
		UseShoulderCamera:         settings.UseShoulderCamera,
		PovRadarEnabled:           settings.PovRadarEnabled,
		SkyBlackout:               settings.SkyBlackout,
		DisableClouds:             settings.DisableClouds,
		KillFeedLifetime:          settings.KillFeedLifetime,
		BlockKillFeed:             settings.BlockKillFeed,
		ForcePerPassVoiceCommands: hasVoiceOverride(items),
		ForcePerPassXrayCommands:  hasSpecShowXrayOverride(items),
		LaunchResolution:          settings.LaunchResolution,
	})
	if err != nil {
		return nil, err
	}

	return &GenerationPlan{
		Sequences:               cloneSequences(buildResult.Sequences),
		SegmentCount:            buildResult.SegmentCount,
		TakePlans:               cloneTakePlans(buildResult.TakePlans),
		HistoryKeys:             buildHistoryKeys(input.DemoPath, buildResult.TakePlans),
		Items:                   items,
		FullRoundPOVSegments:    fullRoundSegments,
		HasVoiceOverride:        hasVoiceOverride(items),
		HasSpecShowXrayOverride: hasSpecShowXrayOverride(items),
	}, nil
}

func buildHistoryKeys(demoPath string, plans []clipsjson.TakePlan) []string {
	demoPath = strings.TrimSpace(demoPath)
	if demoPath == "" || len(plans) == 0 {
		return nil
	}
	keys := make([]string, 0, len(plans))
	for _, plan := range plans {
		keys = append(keys, BuildProduceHistoryKeyWithSourceID(
			demoPath,
			plan.View,
			plan.SpecMode,
			plan.KillIDs,
			plan.SourceID,
		))
	}
	return keys
}

func hasVoiceOverride(items []clipsjson.Item) bool {
	for _, item := range items {
		if item.HasVoiceOverride {
			return true
		}
	}
	return false
}

func hasSpecShowXrayOverride(items []clipsjson.Item) bool {
	for _, item := range items {
		if item.HasSpecShowXrayOverride {
			return true
		}
	}
	return false
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneItems(values []clipsjson.Item) []clipsjson.Item {
	if len(values) == 0 {
		return nil
	}
	result := make([]clipsjson.Item, len(values))
	for i, value := range values {
		result[i] = value
		result[i].IncludeKiller = cloneBoolPointer(value.IncludeKiller)
	}
	return result
}

func cloneFullRoundSegments(values []clipsjson.FullRoundPOVSegment) []clipsjson.FullRoundPOVSegment {
	if len(values) == 0 {
		return nil
	}
	return append([]clipsjson.FullRoundPOVSegment(nil), values...)
}

func cloneSequences(values []clipsjson.Sequence) []clipsjson.Sequence {
	if len(values) == 0 {
		return nil
	}
	result := make([]clipsjson.Sequence, len(values))
	for i, sequence := range values {
		result[i] = sequence
		if len(sequence.Actions) > 0 {
			result[i].Actions = append([]clipsjson.Action(nil), sequence.Actions...)
			for j, action := range result[i].Actions {
				if action.Metadata != nil {
					metadata := *action.Metadata
					result[i].Actions[j].Metadata = &metadata
				}
			}
		}
	}
	return result
}

func cloneTakePlans(values []clipsjson.TakePlan) []clipsjson.TakePlan {
	if len(values) == 0 {
		return nil
	}
	result := make([]clipsjson.TakePlan, len(values))
	for i, value := range values {
		result[i] = value
		result[i].KillIDs = append([]string(nil), value.KillIDs...)
		result[i].KillOffsetsSeconds = append([]float64(nil), value.KillOffsetsSeconds...)
	}
	return result
}
