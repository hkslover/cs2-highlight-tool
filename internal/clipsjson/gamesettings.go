package clipsjson

import (
	"fmt"
	"hash/fnv"
	"math"
	"strconv"
	"strings"
)

// Game setting keys used by the debug workbench console plan. They mirror the
// ClipSettings JSON field names so the frontend can map plan rows to its draft
// without repeating any command knowledge.
const (
	GameSettingKeyPovRadarEnabled   = "pov_radar_enabled"
	GameSettingKeySkyBlackout       = "sky_blackout"
	GameSettingKeyDisableClouds     = "disable_clouds"
	GameSettingKeyHideAllUI         = "hide_all_ui"
	GameSettingKeyHidePlayerAvatars = "hide_player_avatars"
	GameSettingKeyHideChatMessages  = "hide_chat_messages"
	GameSettingKeySpecShowXray      = "enable_spec_show_xray_zero"
	GameSettingKeyEnableVoice       = "enable_voice"
	GameSettingKeyUseShoulderCamera = "use_shoulder_camera"
	GameSettingKeyKillFeedLifetime  = "kill_feed_lifetime"
	GameSettingKeyPovKillFeedOnly   = "pov_kill_feed_only"
	GameSettingKeyBlockKillFeed     = "block_kill_feed"
	GameSettingKeyNoFlashStrength   = "noflash_strength"

	// noFlashStrengthDecimals mirrors the persisted precision (0..1 with two
	// decimals) so the command never carries more digits than the config
	// round-trips.
	noFlashStrengthDecimals = 2

	// shoulderCameraRestoreCommand returns the third-person camera to first
	// person. `cam_command 2` is CAM_COMMAND_TOFIRSTPERSON (hl2sdk
	// input.h:203-205) and `c_thirdpersonshoulder 0` clears the shoulder flag,
	// so the camera mode is restored instead of only flipping a UI boolean.
	// Windows in-game confirmation is still required; this plan only reuses the
	// documented engine values.
	shoulderCameraRestoreCommand = "c_thirdpersonshoulder 0;cam_command 2"

	// deathMsgFilterClearCommand is HLAE's only way to undo a death notice
	// filter without knowing its list index: `mirv_deathmsg filter clear`
	// clears the whole filter list (MirvDeathMsgFilter.cpp). The UI must state
	// that it also drops the user's own filters.
	deathMsgFilterClearCommand = "mirv_deathmsg filter clear"

	// deathMsgClearCommand removes death notices that are already on screen.
	// `filter clear` only edits the rule list, so a POV switch also needs this
	// command when the previous perspective's notice must not leak into the
	// next take.
	deathMsgClearCommand = "mirv_deathmsg clear"
)

// GameSettings carries the hot-updatable runtime settings shared by the
// produce bootstrap and the debug workbench. The helpers below are pure: they
// never read config, probe encoders, use ticks or touch the filesystem.
type GameSettings struct {
	PovRadarEnabled    bool
	SkyBlackout        bool
	DisableClouds      bool
	HideAllUI          bool
	HidePlayerAvatars  bool
	HideChatMessages   bool
	EnableSpecShowXray bool
	EnableVoice        bool
	UseShoulderCamera  bool
	KillFeedLifetime   int
	PovKillFeedOnly    bool
	BlockKillFeed      bool
	// NoFlashStrength is the `mirv_noflash` strength in [0, 1]. Zero means the
	// command is never written, so the demo keeps the normal flash effect.
	NoFlashStrength float64
}

// ShoulderCameraStartCommand is the produce-bootstrap shoulder camera setup.
func ShoulderCameraStartCommand() string {
	return shoulderCameraCommand
}

// ShoulderCameraRestoreCommand returns the debug-only restore for the shoulder
// camera. The produce bootstrap never writes it.
func ShoulderCameraRestoreCommand() string {
	return shoulderCameraRestoreCommand
}

// ShoulderCameraCommands returns the command for the requested shoulder camera
// state: the produce setup when enabled, the explicit first-person restore
// when disabled.
func ShoulderCameraCommands(enabled bool) []string {
	if enabled {
		return []string{shoulderCameraCommand}
	}
	return []string{shoulderCameraRestoreCommand}
}

// SkyBlackoutCommand returns the sky command for the requested state. The
// debug restore writes `r_drawskybox 1`; clouds are never coupled to it.
func SkyBlackoutCommand(enabled bool) string {
	if enabled {
		return "r_drawskybox 0"
	}
	return "r_drawskybox 1"
}

// CloudsCommand returns the cloud drawing command for the requested state
// (HLAE `mirv_sky clouds draw 0|1`), independent of the sky switch.
func CloudsCommand(enabled bool) string {
	if enabled {
		return "mirv_sky clouds draw 0"
	}
	return "mirv_sky clouds draw 1"
}

// PovRadarCommand returns the plugin special action for the POV radar. The
// plugin intercepts `csdm_radar_pov`; it is not a registered console cvar.
func PovRadarCommand(enabled bool) string {
	if enabled {
		return "csdm_radar_pov 1"
	}
	return "csdm_radar_pov 0"
}

// HideAllUICommand returns the `cl_draw_only_deathnotices` command for the
// requested state.
func HideAllUICommand(enabled bool) string {
	if enabled {
		return "cl_draw_only_deathnotices 1"
	}
	return "cl_draw_only_deathnotices 0"
}

// PlayerAvatarModeCommand is written for both values, matching the produce
// bootstrap which always emits the boolean.
func PlayerAvatarModeCommand(enabled bool) string {
	return fmt.Sprintf("cl_teamcounter_playercount_instead_of_avatars %t", enabled)
}

// HideChatCommand returns the hidehud command for the requested state. The
// restore (`hidehud 0`) clears every hidehud bit, so the debug UI must state
// that impact; the produce bootstrap never writes the restore.
func HideChatCommand(enabled bool) string {
	if enabled {
		return "hidehud 128"
	}
	return "hidehud 0"
}

// SpecShowXrayCommand reuses the produce polarity helper instead of guessing
// from the field name.
func SpecShowXrayCommand(enabled bool) string {
	return fmt.Sprintf("spec_show_xray %d", xrayCommandValue(enabled))
}

// KillFeedLifetimeCommand returns the HLAE death notice lifetime command. The
// caller keeps the normalized 1..10 range (or the default for an unset value).
func KillFeedLifetimeCommand(seconds int) string {
	if seconds <= 0 {
		seconds = 4
	}
	return fmt.Sprintf("mirv_deathmsg lifetime %d", seconds)
}

// NoFlashCommand returns the `mirv_noflash` command with a float strength.
// HLAE >= 2.176.0 accepts the 0.0-1.0 strength; the value is clamped and
// formatted with two decimals so the command and the persisted setting agree.
// The caller decides whether a zero strength is written at all: the produce
// bootstrap skips it, while the debug workbench uses `mirv_noflash 0.00` as
// its explicit restore command.
func NoFlashCommand(strength float64) string {
	return fmt.Sprintf("mirv_noflash %.*f", noFlashStrengthDecimals, clampNoFlashStrength(strength))
}

func clampNoFlashStrength(strength float64) float64 {
	if strength != strength { // NaN
		return 0
	}
	if strength < 0 {
		return 0
	}
	if strength > 1 {
		strength = 1
	}
	scale := math.Pow10(noFlashStrengthDecimals)
	return math.Round(strength*scale) / scale
}

// BlockKillFeedCommands returns the command for the requested state. Enabling
// reuses the produce filter; disabling clears the HLAE filter list.
func BlockKillFeedCommands(enabled bool) []string {
	if enabled {
		return []string{"mirv_deathmsg filter add block 1"}
	}
	return []string{deathMsgFilterClearCommand}
}

// PovKillFeedCommands returns the death notice commands that keep only the
// notices the given player took part in (as killer, victim or assister). HLAE
// resolves `xTrace` when the command is parsed, not when each notice is
// created, so the produce planner must repeat this per recording pass with the
// pass's explicit Steam64 instead of relying on a single bootstrap filter.
//
// A missing or unusable SteamID still clears the rule list: leaving the
// previous pass's rule active would hide the new perspective's notices.
func PovKillFeedCommands(steamID string) []string {
	commands := []string{deathMsgClearCommand, deathMsgFilterClearCommand}
	xuid, ok := normalizeDeathMsgXuid(steamID)
	if !ok {
		return commands
	}
	match := "!x" + xuid
	return append(commands, fmt.Sprintf(
		"mirv_deathmsg filter add attackerMatch=%s victimMatch=%s assisterMatch=%s block=1 lastRule=1",
		match, match, match,
	))
}

// PovKillFeedTraceCommands is the debug workbench variant. `xTrace` resolves to
// the player that is spectated when the command runs, so the interactive
// session has to re-send it after switching perspective. The produce pipeline
// never uses this variant because its passes must stay deterministic.
func PovKillFeedTraceCommands(enabled bool) []string {
	if !enabled {
		return []string{deathMsgFilterClearCommand}
	}
	return []string{
		deathMsgClearCommand,
		deathMsgFilterClearCommand,
		"mirv_deathmsg filter add attackerMatch=!xTrace victimMatch=!xTrace assisterMatch=!xTrace block=1 lastRule=1",
	}
}

// normalizeDeathMsgXuid keeps only a real, non-zero Steam64. The value is
// round-tripped through ParseUint so the console command never carries signs,
// quotes or leading zeros coming from a malformed demo field.
func normalizeDeathMsgXuid(steamID string) (string, bool) {
	value, err := strconv.ParseUint(strings.TrimSpace(steamID), 10, 64)
	if err != nil || value == 0 {
		return "", false
	}
	return strconv.FormatUint(value, 10), true
}

// VoiceIndicesCommands returns both voice index commands for the requested
// state so the pair can never drift apart.
func VoiceIndicesCommands(enabled bool) []string {
	value := voiceIndicesValue(enabled)
	return []string{
		fmt.Sprintf("tv_listen_voice_indices %d", value),
		fmt.Sprintf("tv_listen_voice_indices_h %d", value),
	}
}

// RecordInitOptions carries the already resolved recording settings. The
// caller resolves `auto` to an effective preset and builds the encode
// parameters before calling this helper.
type RecordInitOptions struct {
	RecordFPS    int
	VideoPreset  string
	FFmpegParams string
	RecordName   string
	// SettingsName is the HLAE ffmpeg settings entry name. An empty value keeps
	// the produce behavior (the effective preset id); the debug workbench
	// passes DebugRecordSettingsName because HLAE refuses to overwrite an
	// existing settings entry inside one game process.
	SettingsName string
}

// RecordInitCommands returns the HLAE recording initialization commands shared
// by the produce bootstrap and the debug workbench. It never starts or ends a
// recording: take boundaries, ticks, sequence control and the final
// `mirv_streams record start` stay with the caller.
//
// `mirv_endofmatch 0` is unconditional (HLAE >= 2.181.1): it hides the
// end-of-match animation and scoreboard scene so a demo that reaches match end
// does not capture it. There is no settings switch; older HLAE builds only
// report an unknown command and keep the scene visible.
func RecordInitCommands(opts RecordInitOptions) []string {
	fps := opts.RecordFPS
	if fps <= 0 {
		fps = 60
	}
	settingsName := strings.TrimSpace(opts.SettingsName)
	if settingsName == "" {
		settingsName = opts.VideoPreset
	}
	return []string{
		"mirv_endofmatch 0",
		"mirv_streams record screen enabled 1",
		fmt.Sprintf("mirv_streams record fps %d", fps),
		"mirv_streams record startMovieWav 1",
		fmt.Sprintf(`mirv_streams settings add ffmpeg %s "%s {QUOTE}{AFX_STREAM_PATH}.mp4{QUOTE}"`, settingsName, opts.FFmpegParams),
		fmt.Sprintf("mirv_streams record screen settings %s", settingsName),
		fmt.Sprintf(`mirv_streams record name "%s"`, normalizePathForCommand(opts.RecordName)),
	}
}

// DebugRecordSettingsName returns the settings entry name the debug workbench
// uses for one recording configuration. `mirv_streams settings add ffmpeg`
// fails with AFXERROR when the name already exists, so the name changes with
// the encode parameters; re-applying identical parameters reuses the identical
// entry. The produce bootstrap keeps using the plain effective preset id.
func DebugRecordSettingsName(preset string, quality string, ffmpegParams string) string {
	sanitize := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		var b strings.Builder
		for _, r := range value {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
				b.WriteRune(r)
			}
		}
		if b.Len() == 0 {
			return "preset"
		}
		return b.String()
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(ffmpegParams))
	return fmt.Sprintf("debug_%s_%s_%08x", sanitize(preset), sanitize(quality), hash.Sum32())
}

// DebugGameSettingPlan is one row of the debug workbench's built console
// commands. Commands describes the current draft value; switches additionally
// carry both variants so a toggle click can send the exact command without
// rebuilding the plan. Non-switch rows have no On/Off.
type DebugGameSettingPlan struct {
	Key      string   `json:"key"`
	Commands []string `json:"commands"`
	On       []string `json:"on,omitempty"`
	Off      []string `json:"off,omitempty"`
}

// BuildDebugGameSettingPlan returns the per-setting commands for the debug
// workbench in the same order as the produce bootstrap, plus the ordered
// apply-all command list. Every switch carries an explicit restore command:
// the interactive debug session must be able to switch back, while the
// produce bootstrap keeps writing only its enabled commands.
func BuildDebugGameSettingPlan(settings GameSettings) ([]DebugGameSettingPlan, []string) {
	toggle := func(key string, commands func(bool) []string, enabled bool) DebugGameSettingPlan {
		on := commands(true)
		off := commands(false)
		current := off
		if enabled {
			current = on
		}
		return DebugGameSettingPlan{
			Key:      key,
			Commands: append([]string(nil), current...),
			On:       on,
			Off:      off,
		}
	}

	// The POV filter and the block-all filter edit the same HLAE rule list, so
	// the block row comes first and a settled block-all suppresses the POV
	// row's current command entirely. Emitting `filter clear` for the POV row
	// would undo the block rule during apply-all.
	blockKillFeed := toggle(GameSettingKeyBlockKillFeed, BlockKillFeedCommands, settings.BlockKillFeed)
	povKillFeedOnly := toggle(GameSettingKeyPovKillFeedOnly, PovKillFeedTraceCommands, settings.PovKillFeedOnly)
	if settings.BlockKillFeed {
		povKillFeedOnly.Commands = []string{}
	}

	items := []DebugGameSettingPlan{
		toggle(GameSettingKeyUseShoulderCamera, ShoulderCameraCommands, settings.UseShoulderCamera),
		toggle(GameSettingKeyDisableClouds, func(enabled bool) []string {
			return []string{CloudsCommand(enabled)}
		}, settings.DisableClouds),
		toggle(GameSettingKeySkyBlackout, func(enabled bool) []string {
			return []string{SkyBlackoutCommand(enabled)}
		}, settings.SkyBlackout),
		toggle(GameSettingKeyPovRadarEnabled, func(enabled bool) []string {
			return []string{PovRadarCommand(enabled)}
		}, settings.PovRadarEnabled),
		{
			Key:      GameSettingKeyKillFeedLifetime,
			Commands: []string{KillFeedLifetimeCommand(settings.KillFeedLifetime)},
		},
		// `mirv_noflash` takes a float strength, so this row is not a boolean
		// switch: `on` applies the configured strength and `off` restores the
		// normal flash effect with an explicit zero. A zero draft strength makes
		// both variants identical on purpose instead of inventing a value.
		{
			Key:      GameSettingKeyNoFlashStrength,
			Commands: []string{NoFlashCommand(settings.NoFlashStrength)},
			On:       []string{NoFlashCommand(settings.NoFlashStrength)},
			Off:      []string{NoFlashCommand(0)},
		},
		blockKillFeed,
		povKillFeedOnly,
		toggle(GameSettingKeySpecShowXray, func(enabled bool) []string {
			return []string{SpecShowXrayCommand(enabled)}
		}, settings.EnableSpecShowXray),
		toggle(GameSettingKeyHideAllUI, func(enabled bool) []string {
			return []string{HideAllUICommand(enabled)}
		}, settings.HideAllUI),
		toggle(GameSettingKeyHidePlayerAvatars, func(enabled bool) []string {
			return []string{PlayerAvatarModeCommand(enabled)}
		}, settings.HidePlayerAvatars),
		toggle(GameSettingKeyHideChatMessages, func(enabled bool) []string {
			return []string{HideChatCommand(enabled)}
		}, settings.HideChatMessages),
		toggle(GameSettingKeyEnableVoice, VoiceIndicesCommands, settings.EnableVoice),
	}

	applyAll := make([]string, 0, len(items))
	for _, item := range items {
		applyAll = append(applyAll, item.Commands...)
	}
	return items, applyAll
}
