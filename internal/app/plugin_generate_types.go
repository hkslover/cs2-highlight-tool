package app

import (
	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/demo"
)

type GeneratePluginJSONRequest struct {
	DemoPath string  `json:"demo_path"`
	TickRate float64 `json:"tick_rate"`
	// MatchEndTick is the demo tick of the post-match win panel (final
	// scoreboard), as reported by demo.Metadata.MatchEndTick from the earlier
	// parse. Used to keep recordings from bleeding into the settlement screen.
	MatchEndTick   int                `json:"match_end_tick,omitempty"`
	SelectedItems  []SelectedClipItem `json:"selected_items,omitempty"`
	FullRoundPOV   *FullRoundPOVItem  `json:"full_round_pov,omitempty"`
	ExtraCommands  []string           `json:"extra_commands,omitempty"`
	BatchTimestamp string             `json:"batch_timestamp,omitempty"`

	// Deprecated compatibility input. New callers should use SelectedItems.
	SelectedKills []demo.ClipKill `json:"selected_kills,omitempty"`
}

type FullRoundPOVItem struct {
	PlayerSteamID string `json:"player_steam_id"`
}

type GeneratePluginJSONResult struct {
	JSONPath      string            `json:"json_path"`
	SequenceCount int               `json:"sequence_count"`
	SegmentCount  int               `json:"segment_count"`
	ActionCount   int               `json:"action_count"`
	TakePlans     []ProduceTakePlan `json:"take_plans,omitempty"`
}

type GeneratePluginJSONBatchRequest struct {
	Jobs  []GeneratePluginJSONRequest   `json:"jobs"`
	Debug *GeneratePluginJSONBatchDebug `json:"debug,omitempty"`
}

type GeneratePluginJSONBatchDebug struct {
	KeepIntermediateFiles bool `json:"keep_intermediate_files"`
}

type GeneratePluginJSONBatchItemResult struct {
	DemoPath         string            `json:"demo_path"`
	JSONPath         string            `json:"json_path,omitempty"`
	SequenceCount    int               `json:"sequence_count,omitempty"`
	SegmentCount     int               `json:"segment_count,omitempty"`
	ActionCount      int               `json:"action_count,omitempty"`
	TakePlans        []ProduceTakePlan `json:"take_plans,omitempty"`
	GeneratedTakeCnt int               `json:"generated_take_count,omitempty"`
	SkippedByHistory bool              `json:"skipped_by_history,omitempty"`
	SkippedReason    string            `json:"skipped_reason,omitempty"`
	Error            string            `json:"error,omitempty"`
}

type GeneratePluginJSONBatchResult struct {
	Results          []GeneratePluginJSONBatchItemResult `json:"results"`
	SuccessCount     int                                 `json:"success_count"`
	FailureCount     int                                 `json:"failure_count"`
	BatchTimestamp   string                              `json:"batch_timestamp,omitempty"`
	LaunchStarted    bool                                `json:"launch_started,omitempty"`
	LaunchedDemoPath string                              `json:"launched_demo_path,omitempty"`
	LaunchError      string                              `json:"launch_error,omitempty"`
}

type ProduceTakePlan struct {
	DemoPath           string    `json:"demo_path"`
	TakeIndex          int       `json:"take_index"`
	TakeName           string    `json:"take_name,omitempty"`
	View               string    `json:"view"`
	SpecMode           int       `json:"spec_mode"`
	KillIDs            []string  `json:"kill_ids"`
	SourceID           string    `json:"source_id,omitempty"`
	Round              int       `json:"round,omitempty"`
	PlayerName         string    `json:"player_name,omitempty"`
	PlayerSteamID      string    `json:"player_steam_id,omitempty"`
	StartTick          int       `json:"start_tick,omitempty"`
	EndTick            int       `json:"end_tick,omitempty"`
	EndReason          string    `json:"end_reason,omitempty"`
	TickRate           float64   `json:"tick_rate,omitempty"`
	RecordStartTick    int       `json:"record_start_tick,omitempty"`
	RecordEndTick      int       `json:"record_end_tick,omitempty"`
	KillOffsetsSeconds []float64 `json:"kill_offsets_seconds,omitempty"`
}

type pluginAction = clipsjson.Action
type pluginSequence = clipsjson.Sequence

type generatePluginJSONInternalOptions struct {
	ItemsOverride                   []clipsjson.Item
	UseItemsOverride                bool
	FullRoundPOVSegmentsOverride    []clipsjson.FullRoundPOVSegment
	UseFullRoundPOVSegmentsOverride bool
	WriteJSON                       bool
	RecordSubDir                    string
}

type normalizedSelectedItems struct {
	Items                   []clipsjson.Item
	FullRoundPOVSegments    []clipsjson.FullRoundPOVSegment
	HasVoiceOverride        bool
	HasSpecShowXrayOverride bool
}
