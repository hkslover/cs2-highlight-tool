package app

import (
	"strings"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/ffmpegprofile"
)

// DebugSettingsDraft is the session-scoped copy of the hot-updatable clip
// settings used by the debug workbench. The page initializes it from
// GetClipSettings and may reload it, but it is only ever sent back to
// GetDebugConsolePlan: it is never written to the persisted config.
type DebugSettingsDraft struct {
	PovRadarEnabled    bool    `json:"pov_radar_enabled"`
	SkyBlackout        bool    `json:"sky_blackout"`
	DisableClouds      bool    `json:"disable_clouds"`
	HideAllUI          bool    `json:"hide_all_ui"`
	HidePlayerAvatars  bool    `json:"hide_player_avatars"`
	HideChatMessages   bool    `json:"hide_chat_messages"`
	EnableSpecShowXray bool    `json:"enable_spec_show_xray_zero"`
	EnableVoice        bool    `json:"enable_voice"`
	UseShoulderCamera  bool    `json:"use_shoulder_camera"`
	KillFeedLifetime   int     `json:"kill_feed_lifetime"`
	PovKillFeedOnly    bool    `json:"pov_kill_feed_only"`
	BlockKillFeed      bool    `json:"block_kill_feed"`
	NoFlashStrength    float64 `json:"noflash_strength"`
	RecordFPS          int     `json:"record_fps"`
	RecordQuality      string  `json:"record_quality"`
	MotionBlurEnabled  bool    `json:"motion_blur_enabled"`
	VideoPreset        string  `json:"video_preset"`
	LaunchResolution   string  `json:"launch_resolution"`
}

// DebugSettingPlan is one row of the built console commands. Commands always
// describes the draft's current value; switches additionally carry their
// on/off variants so a toggle click sends the exact command without rebuilding
// the plan. Non-switch rows have no variants.
type DebugSettingPlan struct {
	Key      string   `json:"key"`
	Commands []string `json:"commands"`
	On       []string `json:"on,omitempty"`
	Off      []string `json:"off,omitempty"`
}

// DebugConsolePlan is the debug workbench's built console command plan.
// ApplyAll is the deterministic order behind "apply current game settings";
// Recording describes the next-recording initialization, which is applied when
// a recording starts and frozen while one is running. The effective video
// fields are read-only probe results for `video_preset=auto`.
type DebugConsolePlan struct {
	Settings              []DebugSettingPlan `json:"settings"`
	ApplyAll              []string           `json:"apply_all"`
	Recording             []string           `json:"recording"`
	EffectiveVideoPreset  string             `json:"effective_video_preset"`
	EffectiveVideoEncoder string             `json:"effective_video_encoder"`
	EffectiveVideoStatus  string             `json:"effective_video_status"`
	RecordOutputDir       string             `json:"record_output_dir"`
}

// GetDebugConsolePlan builds the debug workbench's console commands from the
// session-scoped draft. It never writes the draft to the persisted config and
// never starts a recording: ticks, sequence control and record boundaries stay
// with the produce pipeline and the manual recording step.
func (a *App) GetDebugConsolePlan(draft DebugSettingsDraft) (*DebugConsolePlan, error) {
	release, err := a.beginManagedFileUse()
	if err != nil {
		return nil, err
	}
	defer release()

	normalized := normalizeDebugSettingsDraft(draft)
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, err
	}

	// The recording settings must match the session that is already running:
	// its launch resolution is frozen at launch and the draft cannot change it
	// from under a live recording.
	resolution := normalized.LaunchResolution
	if rt := a.debugRuntimeForAdmission(); rt != nil && !debugRuntimeDone(rt) {
		rt.mu.Lock()
		if frozen := rt.request.LaunchResolution; frozen != "" {
			resolution = frozen
		}
		rt.mu.Unlock()
	}

	resolved := ffmpegprofile.ResolveVideoPreset(
		normalized.VideoPreset,
		cfg.FFmpegDetectedPreset,
		cfg.FFmpegDetectedEncoders,
	)
	videoPreset, ffmpegParams, err := clipsjson.BuildRecordEncoding(
		resolved.EffectivePreset,
		normalized.RecordQuality,
		resolution,
	)
	if err != nil {
		return nil, err
	}

	settings := clipsjson.GameSettings{
		PovRadarEnabled:    normalized.PovRadarEnabled,
		SkyBlackout:        normalized.SkyBlackout,
		DisableClouds:      normalized.DisableClouds,
		HideAllUI:          normalized.HideAllUI,
		HidePlayerAvatars:  normalized.HidePlayerAvatars,
		HideChatMessages:   normalized.HideChatMessages,
		EnableSpecShowXray: normalized.EnableSpecShowXray,
		EnableVoice:        normalized.EnableVoice,
		UseShoulderCamera:  normalized.UseShoulderCamera,
		KillFeedLifetime:   normalized.KillFeedLifetime,
		PovKillFeedOnly:    normalized.PovKillFeedOnly,
		BlockKillFeed:      normalized.BlockKillFeed,
		NoFlashStrength:    normalized.NoFlashStrength,
	}
	settingsPlan, applyAll := clipsjson.BuildDebugGameSettingPlan(settings)
	planItems := make([]DebugSettingPlan, 0, len(settingsPlan))
	for _, item := range settingsPlan {
		planItems = append(planItems, DebugSettingPlan{
			Key:      item.Key,
			Commands: item.Commands,
			On:       item.On,
			Off:      item.Off,
		})
	}

	outputDir := a.debugRecordOutputDir()
	return &DebugConsolePlan{
		Settings: planItems,
		ApplyAll: applyAll,
		Recording: clipsjson.RecordInitCommands(clipsjson.RecordInitOptions{
			RecordFPS:    normalized.RecordFPS,
			VideoPreset:  videoPreset,
			FFmpegParams: ffmpegParams,
			RecordName:   outputDir,
			MotionBlur:   normalized.MotionBlurEnabled,
			// A repeat apply in the same game must create a new HLAE settings
			// entry, otherwise the old encode parameters stay in effect.
			SettingsName: clipsjson.DebugRecordSettingsName(videoPreset, normalized.RecordQuality, ffmpegParams),
		}),
		EffectiveVideoPreset:  resolved.EffectivePreset,
		EffectiveVideoEncoder: resolved.Encoder,
		EffectiveVideoStatus:  resolved.Status,
		RecordOutputDir:       outputDir,
	}, nil
}

// debugRecordOutputDir is the fixed, managed base directory for debug
// recordings. The draft can never choose an arbitrary output path; each
// recording gets its own unique subdirectory below it.
func (a *App) debugRecordOutputDir() string {
	return config.CleanPath(a.dataPath("outputs", "debug"))
}

// normalizeDebugSettingsDraft clamps the draft exactly like the persisted clip
// settings would be clamped, so a plan built from a hand-edited draft cannot
// produce out-of-range commands.
func normalizeDebugSettingsDraft(draft DebugSettingsDraft) DebugSettingsDraft {
	normalized := draft
	if normalized.RecordFPS <= 0 {
		normalized.RecordFPS = config.DefaultRecordFPS
	}
	if normalized.RecordFPS < 1 {
		normalized.RecordFPS = 1
	}
	if normalized.RecordFPS > 240 {
		normalized.RecordFPS = 240
	}
	normalized.RecordQuality = ffmpegprofile.NormalizeEditQuality(normalized.RecordQuality)
	normalized.VideoPreset = ffmpegprofile.NormalizeUserPreset(normalized.VideoPreset)
	normalized.LaunchResolution = normalizeLaunchResolution(normalized.LaunchResolution)
	if normalized.KillFeedLifetime <= 0 {
		normalized.KillFeedLifetime = config.DefaultKillFeedLifetime
	}
	if normalized.KillFeedLifetime < config.MinKillFeedLifetime {
		normalized.KillFeedLifetime = config.MinKillFeedLifetime
	}
	if normalized.KillFeedLifetime > config.MaxKillFeedLifetime {
		normalized.KillFeedLifetime = config.MaxKillFeedLifetime
	}
	normalized.NoFlashStrength = config.NormalizeNoFlashStrength(normalized.NoFlashStrength)
	return normalized
}

func normalizeLaunchResolution(resolution string) string {
	trimmed := strings.TrimSpace(resolution)
	if !config.IsSupportedLaunchResolution(trimmed) {
		return config.DefaultLaunchResolution
	}
	return trimmed
}
