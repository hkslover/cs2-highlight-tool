package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cs2-highlight-tool-v2/internal/endpoints"
)

type Config struct {
	CS2Dir                 string              `json:"cs2_dir"`
	CS2Exe                 string              `json:"cs2_exe"`
	HLAEExe                string              `json:"hlae_exe"`
	PluginDLL              string              `json:"plugin_dll"`
	FFmpegDir              string              `json:"ffmpeg_dir"`
	FiveEPlayerName        string              `json:"fivee_player_name"`
	DownloadSource         string              `json:"download_source"`
	CountryCode            string              `json:"country_code"`
	SourceCheckedAt        string              `json:"source_checked_at"`
	KillerPreSeconds       float64             `json:"killer_pre_seconds"`
	KillerPostSeconds      float64             `json:"killer_post_seconds"`
	VictimPreSeconds       float64             `json:"victim_pre_seconds"`
	VictimPostSeconds      float64             `json:"victim_post_seconds"`
	AutoAddVictimView      bool                `json:"auto_add_victim_view"`
	RecordFPS              int                 `json:"record_fps"`
	RecordQuality          string              `json:"record_quality"`
	EditFPS                int                 `json:"edit_fps"`
	EditQuality            string              `json:"edit_quality"`
	VideoPreset            string              `json:"video_preset"`
	FFmpegDetectedPreset   string              `json:"ffmpeg_detected_preset,omitempty"`
	FFmpegDetectedEncoders []string            `json:"ffmpeg_detected_encoders,omitempty"`
	FFmpegDetectedAt       string              `json:"ffmpeg_detected_at,omitempty"`
	LaunchResolution       string              `json:"launch_resolution"`
	RecordOutputDir        string              `json:"record_output_dir"`
	EnableSpecShowXray     bool                `json:"enable_spec_show_xray_zero"`
	HideAllUI              bool                `json:"hide_all_ui"`
	PovHudEnabled          bool                `json:"pov_hud_enabled"`
	ClipActionSettings     *ClipActionSettings `json:"clip_action_settings,omitempty"`
}

type ClipActionSettings struct {
	EnableVoiceIndices  bool `json:"enable_voice_indices"`
	VoiceIndicesValue   int  `json:"voice_indices_value"`
	EnableVoiceIndicesH bool `json:"enable_voice_indices_h"`
	VoiceIndicesHValue  int  `json:"voice_indices_h_value"`
}

const (
	DefaultKillerPreSeconds  = 4.0
	DefaultKillerPostSeconds = 4.0
	DefaultVictimPreSeconds  = 2.0
	DefaultVictimPostSeconds = 2.0
	DefaultRecordFPS         = 60
	DefaultRecordQuality     = "high"
	DefaultEditFPS           = 60
	MinEditFPS               = 24
	MaxEditFPS               = 240
	DefaultEditQuality       = "high"
	DefaultVideoPreset       = "auto"
	DefaultLaunchResolution  = "4:3"
	LaunchResolution16x9     = "16:9"
	LaunchResolution4x3      = "4:3"
	LaunchResolution4x3Low   = "4:3_1280x960"
)

func Default(dataDir string) *Config {
	return &Config{
		HLAEExe:            filepath.Join(dataDir, "hlae", "HLAE.exe"),
		PluginDLL:          filepath.Join(dataDir, "plugin", "server.dll"),
		FFmpegDir:          filepath.Join(dataDir, "ffmpeg", "bin"),
		DownloadSource:     defaultDownloadSource(),
		KillerPreSeconds:   DefaultKillerPreSeconds,
		KillerPostSeconds:  DefaultKillerPostSeconds,
		VictimPreSeconds:   DefaultVictimPreSeconds,
		VictimPostSeconds:  DefaultVictimPostSeconds,
		AutoAddVictimView:  true,
		RecordFPS:          DefaultRecordFPS,
		RecordQuality:      DefaultRecordQuality,
		EditFPS:            DefaultEditFPS,
		EditQuality:        DefaultEditQuality,
		VideoPreset:        DefaultVideoPreset,
		LaunchResolution:   DefaultLaunchResolution,
		RecordOutputDir:    filepath.Join(dataDir, "outputs"),
		EnableSpecShowXray: true,
		HideAllUI:          false,
		PovHudEnabled:      false,
		ClipActionSettings: Ptr(DefaultClipActionSettings()),
	}
}

func LoadOrCreate(path, dataDir string) (*Config, error) {
	cfg := Default(dataDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := Save(path, cfg); err != nil {
				return nil, err
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	if !strings.Contains(string(data), `"enable_spec_show_xray_zero"`) {
		cfg.EnableSpecShowXray = true
	}
	ApplyDefaults(cfg, dataDir)
	if err := Save(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func ApplyDefaults(cfg *Config, dataDir string) {
	base := Default(dataDir)
	cfg.CS2Dir = CleanPath(cfg.CS2Dir)
	cfg.CS2Exe = CleanPath(cfg.CS2Exe)
	cfg.FiveEPlayerName = strings.TrimSpace(cfg.FiveEPlayerName)
	if cfg.HLAEExe == "" {
		cfg.HLAEExe = base.HLAEExe
	} else {
		cfg.HLAEExe = CleanPath(cfg.HLAEExe)
	}
	if cfg.PluginDLL == "" {
		cfg.PluginDLL = base.PluginDLL
	} else {
		cfg.PluginDLL = CleanPath(cfg.PluginDLL)
	}
	if cfg.FFmpegDir == "" {
		cfg.FFmpegDir = base.FFmpegDir
	} else {
		cfg.FFmpegDir = CleanPath(cfg.FFmpegDir)
	}
	if !isSupportedDownloadSource(cfg.DownloadSource) {
		cfg.DownloadSource = defaultDownloadSource()
	}
	if cfg.KillerPreSeconds <= 0 {
		cfg.KillerPreSeconds = DefaultKillerPreSeconds
	}
	if cfg.KillerPostSeconds <= 0 {
		cfg.KillerPostSeconds = DefaultKillerPostSeconds
	}
	if cfg.VictimPreSeconds <= 0 {
		cfg.VictimPreSeconds = DefaultVictimPreSeconds
	}
	if cfg.VictimPostSeconds <= 0 {
		cfg.VictimPostSeconds = DefaultVictimPostSeconds
	}
	if cfg.RecordFPS <= 0 {
		cfg.RecordFPS = DefaultRecordFPS
	}
	cfg.RecordQuality = strings.ToLower(strings.TrimSpace(cfg.RecordQuality))
	if !isSupportedEditQuality(cfg.RecordQuality) {
		cfg.RecordQuality = DefaultRecordQuality
	}
	if cfg.EditFPS <= 0 {
		cfg.EditFPS = DefaultEditFPS
	}
	if cfg.EditFPS < MinEditFPS {
		cfg.EditFPS = MinEditFPS
	}
	if cfg.EditFPS > MaxEditFPS {
		cfg.EditFPS = MaxEditFPS
	}
	cfg.EditQuality = strings.ToLower(strings.TrimSpace(cfg.EditQuality))
	if !isSupportedEditQuality(cfg.EditQuality) {
		cfg.EditQuality = DefaultEditQuality
	}
	cfg.VideoPreset = strings.ToLower(strings.TrimSpace(cfg.VideoPreset))
	if !isSupportedVideoPreset(cfg.VideoPreset) {
		cfg.VideoPreset = DefaultVideoPreset
	}
	cfg.FFmpegDetectedPreset = strings.ToLower(strings.TrimSpace(cfg.FFmpegDetectedPreset))
	if !isSupportedDetectedPreset(cfg.FFmpegDetectedPreset) {
		cfg.FFmpegDetectedPreset = ""
	}
	cfg.FFmpegDetectedEncoders = normalizeEncoderList(cfg.FFmpegDetectedEncoders)
	cfg.FFmpegDetectedAt = strings.TrimSpace(cfg.FFmpegDetectedAt)
	cfg.LaunchResolution = strings.TrimSpace(cfg.LaunchResolution)
	if !IsSupportedLaunchResolution(cfg.LaunchResolution) {
		cfg.LaunchResolution = DefaultLaunchResolution
	}
	cfg.RecordOutputDir = base.RecordOutputDir
	if cfg.ClipActionSettings == nil {
		cfg.ClipActionSettings = Ptr(DefaultClipActionSettings())
	}
}

func DefaultClipActionSettings() ClipActionSettings {
	return ClipActionSettings{
		EnableVoiceIndices:  true,
		VoiceIndicesValue:   -1,
		EnableVoiceIndicesH: true,
		VoiceIndicesHValue:  -1,
	}
}

func ResolveClipActionSettings(cfg *Config) ClipActionSettings {
	if cfg == nil || cfg.ClipActionSettings == nil {
		return DefaultClipActionSettings()
	}
	settings := *cfg.ClipActionSettings
	enabled := settings.EnableVoiceIndices && settings.EnableVoiceIndicesH
	settings.EnableVoiceIndices = enabled
	settings.EnableVoiceIndicesH = enabled
	return settings
}

func SetClipActionSettings(cfg *Config, settings ClipActionSettings) {
	if cfg == nil {
		return
	}
	enabled := settings.EnableVoiceIndices && settings.EnableVoiceIndicesH
	settings.EnableVoiceIndices = enabled
	settings.EnableVoiceIndicesH = enabled
	cfg.ClipActionSettings = Ptr(settings)
}

func defaultDownloadSource() string {
	supported := endpoints.SupportedReleaseSources()
	if len(supported) > 0 {
		return supported[0]
	}
	return "custom"
}

func isSupportedDownloadSource(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	for _, supported := range endpoints.SupportedReleaseSources() {
		if source == supported {
			return true
		}
	}
	return false
}

func isSupportedEditQuality(quality string) bool {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "standard", "high", "ultra":
		return true
	default:
		return false
	}
}

func IsSupportedLaunchResolution(launchResolution string) bool {
	switch strings.TrimSpace(launchResolution) {
	case LaunchResolution16x9, LaunchResolution4x3, LaunchResolution4x3Low:
		return true
	default:
		return false
	}
}

func isSupportedVideoPreset(preset string) bool {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "auto", "c1", "n1", "a1", "i1":
		return true
	default:
		return false
	}
}

func isSupportedDetectedPreset(preset string) bool {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "", "c1", "n1", "a1", "i1", "n1_h264", "a1_h264", "i1_h264":
		return true
	default:
		return false
	}
}

func normalizeEncoderList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		normalized := strings.ToLower(strings.TrimSpace(raw))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func CleanPath(path string) string {
	path = strings.TrimSpace(strings.Trim(path, "\"'"))
	if path == "" {
		return ""
	}
	return filepath.Clean(filepath.FromSlash(path))
}

func JoinExe(dir, name string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, name)
}

func Ptr[T any](v T) *T {
	return &v
}
