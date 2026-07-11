package app

import (
	"strings"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/demo"
	"cs2-highlight-tool-v2/internal/ffmpegprofile"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type ClipSettings struct {
	KillerPreSeconds   float64 `json:"killer_pre_seconds"`
	KillerPostSeconds  float64 `json:"killer_post_seconds"`
	VictimPreSeconds   float64 `json:"victim_pre_seconds"`
	VictimPostSeconds  float64 `json:"victim_post_seconds"`
	AutoAddVictimView  bool    `json:"auto_add_victim_view"`
	EnableVoice        bool    `json:"enable_voice"`
	RecordFPS          int     `json:"record_fps"`
	RecordQuality      string  `json:"record_quality"`
	EditFPS            int     `json:"edit_fps"`
	EditQuality        string  `json:"edit_quality"`
	VideoPreset        string  `json:"video_preset"`
	LaunchResolution   string  `json:"launch_resolution"`
	RecordOutputDir    string  `json:"record_output_dir"`
	EnableSpecShowXray bool    `json:"enable_spec_show_xray_zero"`
	HideAllUI          bool    `json:"hide_all_ui"`
	UseShoulderCamera  bool    `json:"use_shoulder_camera"`
	PovHudEnabled      bool    `json:"pov_hud_enabled"`
	SkyBlackout        bool    `json:"sky_blackout"`
	DisableClouds      bool    `json:"disable_clouds"`
	KillFeedLifetime   int     `json:"kill_feed_lifetime"`
	BlockKillFeed      bool    `json:"block_kill_feed"`
}

type ClipActionSettings struct {
	EnableVoiceIndices  bool `json:"enable_voice_indices"`
	VoiceIndicesValue   int  `json:"voice_indices_value"`
	EnableVoiceIndicesH bool `json:"enable_voice_indices_h"`
	VoiceIndicesHValue  int  `json:"voice_indices_h_value"`
}

type SelectedClipItem struct {
	Kill           demo.ClipKill      `json:"kill"`
	IncludeKiller  *bool              `json:"include_killer,omitempty"`
	IncludeVictim  bool               `json:"include_victim"`
	KillerSpecMode int                `json:"killer_spec_mode"`
	VictimSpecMode int                `json:"victim_spec_mode"`
	ClipOverrides  *ClipItemOverrides `json:"clip_overrides,omitempty"`
}

type ClipItemOverrides struct {
	KillerPreSeconds   *float64 `json:"killer_pre_seconds,omitempty"`
	KillerPostSeconds  *float64 `json:"killer_post_seconds,omitempty"`
	VictimPreSeconds   *float64 `json:"victim_pre_seconds,omitempty"`
	VictimPostSeconds  *float64 `json:"victim_post_seconds,omitempty"`
	EnableVoice        *bool    `json:"enable_voice,omitempty"`
	EnableSpecShowXray *bool    `json:"enable_spec_show_xray_zero,omitempty"`
}

func (a *App) GetClipSettings() (*ClipSettings, error) {
	cfg, err := config.LoadOrCreate(a.configPath(), a.dataRoot())
	if err != nil {
		return nil, err
	}
	settings := normalizeClipSettings(ClipSettings{
		KillerPreSeconds:   cfg.KillerPreSeconds,
		KillerPostSeconds:  cfg.KillerPostSeconds,
		VictimPreSeconds:   cfg.VictimPreSeconds,
		VictimPostSeconds:  cfg.VictimPostSeconds,
		AutoAddVictimView:  cfg.AutoAddVictimView,
		RecordFPS:          cfg.RecordFPS,
		RecordQuality:      cfg.RecordQuality,
		EditFPS:            cfg.EditFPS,
		EditQuality:        cfg.EditQuality,
		VideoPreset:        cfg.VideoPreset,
		LaunchResolution:   cfg.LaunchResolution,
		RecordOutputDir:    a.fixedRecordOutputDir(),
		EnableSpecShowXray: cfg.EnableSpecShowXray,
		HideAllUI:          cfg.HideAllUI,
		UseShoulderCamera:  cfg.UseShoulderCamera,
		PovHudEnabled:      cfg.PovHudEnabled,
		SkyBlackout:        cfg.SkyBlackout,
		DisableClouds:      cfg.DisableClouds,
		KillFeedLifetime:   cfg.KillFeedLifetime,
		BlockKillFeed:      cfg.BlockKillFeed,
	})
	actionSettings := config.ResolveClipActionSettings(cfg)
	settings.EnableVoice = actionSettings.EnableVoiceIndices && actionSettings.EnableVoiceIndicesH
	return &settings, nil
}

func (a *App) SaveClipSettings(input ClipSettings) (*ClipSettings, error) {
	settings := normalizeClipSettings(input)
	settings.RecordOutputDir = a.fixedRecordOutputDir()
	path := a.configPath()
	cfg, err := config.LoadOrCreate(path, a.dataRoot())
	if err != nil {
		return nil, err
	}
	cfg.KillerPreSeconds = settings.KillerPreSeconds
	cfg.KillerPostSeconds = settings.KillerPostSeconds
	cfg.VictimPreSeconds = settings.VictimPreSeconds
	cfg.VictimPostSeconds = settings.VictimPostSeconds
	cfg.AutoAddVictimView = settings.AutoAddVictimView
	cfg.RecordFPS = settings.RecordFPS
	cfg.RecordQuality = settings.RecordQuality
	cfg.EditFPS = settings.EditFPS
	cfg.EditQuality = settings.EditQuality
	cfg.VideoPreset = settings.VideoPreset
	cfg.LaunchResolution = settings.LaunchResolution
	cfg.RecordOutputDir = settings.RecordOutputDir
	cfg.EnableSpecShowXray = settings.EnableSpecShowXray
	cfg.HideAllUI = settings.HideAllUI
	cfg.UseShoulderCamera = settings.UseShoulderCamera
	cfg.PovHudEnabled = settings.PovHudEnabled
	cfg.SkyBlackout = settings.SkyBlackout
	cfg.DisableClouds = settings.DisableClouds
	cfg.KillFeedLifetime = settings.KillFeedLifetime
	cfg.BlockKillFeed = settings.BlockKillFeed
	actionSettings := config.ResolveClipActionSettings(cfg)
	actionSettings.EnableVoiceIndices = settings.EnableVoice
	actionSettings.EnableVoiceIndicesH = settings.EnableVoice
	if settings.EnableVoice {
		actionSettings.VoiceIndicesValue = -1
		actionSettings.VoiceIndicesHValue = -1
	} else {
		actionSettings.VoiceIndicesValue = 0
		actionSettings.VoiceIndicesHValue = 0
	}
	config.SetClipActionSettings(cfg, actionSettings)
	if err := config.Save(path, cfg); err != nil {
		return nil, err
	}
	return &settings, nil
}

func (a *App) PickRecordOutputDir() (string, error) {
	selected, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:                "选择录制输出目录",
		CanCreateDirectories: true,
	})
	if err != nil {
		return "", err
	}
	if selected == "" {
		return "", nil
	}
	return config.CleanPath(selected), nil
}

func (a *App) GetClipActionSettings() (*ClipActionSettings, error) {
	cfg, err := config.LoadOrCreate(a.configPath(), a.dataRoot())
	if err != nil {
		return nil, err
	}
	resolved := config.ResolveClipActionSettings(cfg)
	settings := ClipActionSettings{
		EnableVoiceIndices:  resolved.EnableVoiceIndices,
		VoiceIndicesValue:   resolved.VoiceIndicesValue,
		EnableVoiceIndicesH: resolved.EnableVoiceIndicesH,
		VoiceIndicesHValue:  resolved.VoiceIndicesHValue,
	}
	return &settings, nil
}

func (a *App) SaveClipActionSettings(input ClipActionSettings) (*ClipActionSettings, error) {
	settings := normalizeClipActionSettings(input)
	path := a.configPath()
	cfg, err := config.LoadOrCreate(path, a.dataRoot())
	if err != nil {
		return nil, err
	}
	config.SetClipActionSettings(cfg, config.ClipActionSettings{
		EnableVoiceIndices:  settings.EnableVoiceIndices,
		VoiceIndicesValue:   settings.VoiceIndicesValue,
		EnableVoiceIndicesH: settings.EnableVoiceIndicesH,
		VoiceIndicesHValue:  settings.VoiceIndicesHValue,
	})
	if err := config.Save(path, cfg); err != nil {
		return nil, err
	}
	return &settings, nil
}

func (a *App) fixedRecordOutputDir() string {
	return config.CleanPath(a.dataPath("outputs"))
}

func normalizeClipSettings(input ClipSettings) ClipSettings {
	settings := input
	settings.KillerPreSeconds = config.NormalizeClipWindowSeconds(settings.KillerPreSeconds, config.DefaultKillerPreSeconds)
	settings.KillerPostSeconds = config.NormalizeClipWindowSeconds(settings.KillerPostSeconds, config.DefaultKillerPostSeconds)
	settings.VictimPreSeconds = config.NormalizeClipWindowSeconds(settings.VictimPreSeconds, config.DefaultVictimPreSeconds)
	settings.VictimPostSeconds = config.NormalizeClipWindowSeconds(settings.VictimPostSeconds, config.DefaultVictimPostSeconds)
	if settings.RecordFPS <= 0 {
		settings.RecordFPS = config.DefaultRecordFPS
	}
	if settings.RecordFPS < 1 {
		settings.RecordFPS = 1
	}
	if settings.RecordFPS > 240 {
		settings.RecordFPS = 240
	}
	settings.RecordQuality = ffmpegprofile.NormalizeEditQuality(settings.RecordQuality)
	if settings.EditFPS <= 0 {
		settings.EditFPS = config.DefaultEditFPS
	}
	if settings.EditFPS < config.MinEditFPS {
		settings.EditFPS = config.MinEditFPS
	}
	if settings.EditFPS > config.MaxEditFPS {
		settings.EditFPS = config.MaxEditFPS
	}
	settings.EditQuality = strings.ToLower(strings.TrimSpace(settings.EditQuality))
	if settings.EditQuality != "standard" && settings.EditQuality != "high" && settings.EditQuality != "ultra" {
		settings.EditQuality = config.DefaultEditQuality
	}
	settings.VideoPreset = ffmpegprofile.NormalizeUserPreset(settings.VideoPreset)
	settings.LaunchResolution = strings.TrimSpace(settings.LaunchResolution)
	if !config.IsSupportedLaunchResolution(settings.LaunchResolution) {
		settings.LaunchResolution = config.DefaultLaunchResolution
	}
	settings.RecordOutputDir = config.CleanPath(settings.RecordOutputDir)
	if settings.KillFeedLifetime <= 0 {
		settings.KillFeedLifetime = config.DefaultKillFeedLifetime
	}
	if settings.KillFeedLifetime < config.MinKillFeedLifetime {
		settings.KillFeedLifetime = config.MinKillFeedLifetime
	}
	if settings.KillFeedLifetime > config.MaxKillFeedLifetime {
		settings.KillFeedLifetime = config.MaxKillFeedLifetime
	}
	return settings
}

func normalizeClipActionSettings(input ClipActionSettings) ClipActionSettings {
	enabled := input.EnableVoiceIndices && input.EnableVoiceIndicesH
	input.EnableVoiceIndices = enabled
	input.EnableVoiceIndicesH = enabled
	return input
}

func boolPtr(value bool) *bool {
	v := value
	return &v
}
