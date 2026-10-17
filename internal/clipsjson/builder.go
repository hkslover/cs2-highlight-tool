package clipsjson

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"cs2-highlight-tool-v2/internal/demo"
	"cs2-highlight-tool-v2/internal/ffmpegprofile"
)

const (
	DefaultActionTick      = 96
	DefaultTickRate        = 64.0
	defaultSpecMode        = 1
	segmentSpecModeDelay   = 3
	segmentSpecPlayerDelay = 8
	segmentRecordDelay     = 2
	segmentSeekSettleSec   = 2.0
	matchEndSafetySec      = 0.25

	// POV radar per-pass warm-up. povRadarWarmupSeconds is Demo time: when
	// `pov_radar_enabled` is on, every recording pass switches the camera, enables
	// the radar and then plays at least this much Demo time before
	// `mirv_streams record start`. povRadarSpecPlayerToRadar keeps the radar
	// command after the camera switch; povRadarSpecModeToSpecPlayer mirrors the
	// legacy spec_mode -> spec_player spacing so the switch order stays stable.
	povRadarWarmupSeconds        = 2.0
	povRadarSpecPlayerToRadar    = 2
	povRadarSpecModeToSpecPlayer = segmentSpecPlayerDelay - segmentSpecModeDelay
)

const shoulderCameraCommand = "cam_command 1;cam_idealdist 30;cam_idealyaw 0;cam_idealpitch 0;c_thirdpersonshoulder 1;c_thirdpersonshoulderaimdist 300;c_thirdpersonshoulderdist 40;c_thirdpersonshoulderheight 2;c_thirdpersonshoulderoffset 20;"

type ActionMetadata struct {
	TakeIndex   int    `json:"take_index,omitempty"`
	TakeName    string `json:"take_name,omitempty"`
	RecordPhase string `json:"record_phase,omitempty"`
}

type Action struct {
	Cmd      string          `json:"cmd"`
	Tick     int             `json:"tick"`
	Metadata *ActionMetadata `json:"metadata,omitempty"`
}

type Sequence struct {
	Actions []Action `json:"actions"`
}

type ActionSettings struct {
	EnableVoiceIndices  bool
	VoiceIndicesValue   int
	EnableVoiceIndicesH bool
	VoiceIndicesHValue  int
}

type Item struct {
	Kill                    demo.ClipKill
	IncludeKiller           *bool
	IncludeVictim           bool
	KillerSpecMode          int
	VictimSpecMode          int
	KillerPreSeconds        float64
	KillerPostSeconds       float64
	VictimPreSeconds        float64
	VictimPostSeconds       float64
	EnableVoice             bool
	EnableSpecShowXray      bool
	HasVoiceOverride        bool
	HasSpecShowXrayOverride bool
}

type BuildOptions struct {
	TickRate          float64
	KillerPreSeconds  float64
	KillerPostSeconds float64
	VictimPreSeconds  float64
	VictimPostSeconds float64
	// MatchEndTick is the tick of the post-match win panel (final scoreboard),
	// as parsed from the demo. When set, killer/victim recording windows are
	// clamped so they never extend into it. Zero disables the clamp.
	MatchEndTick              int
	FullRoundPOVSegments      []FullRoundPOVSegment
	ExtraCommands             []string
	ActionSettings            ActionSettings
	RecordFPS                 int
	RecordQuality             string
	VideoPreset               string
	RecordOutputDir           string
	RecordBatchName           string
	EnableSpecShowXray        bool
	HideAllUI                 bool
	HidePlayerAvatars         bool
	HideChatMessages          bool
	UseShoulderCamera         bool
	PovRadarEnabled           bool
	SkyBlackout               bool
	DisableClouds             bool
	KillFeedLifetime          int
	BlockKillFeed             bool
	NoFlashStrength           float64
	ForcePerPassVoiceCommands bool
	ForcePerPassXrayCommands  bool
	LaunchResolution          string
}

type BuildResult struct {
	Sequences    []Sequence
	SegmentCount int
	TakePlans    []TakePlan
}

type TakePlan struct {
	TakeIndex     int      `json:"take_index"`
	TakeName      string   `json:"take_name"`
	View          string   `json:"view"`
	SpecMode      int      `json:"spec_mode"`
	KillIDs       []string `json:"kill_ids"`
	SourceID      string   `json:"source_id,omitempty"`
	Round         int      `json:"round,omitempty"`
	PlayerName    string   `json:"player_name,omitempty"`
	PlayerSteamID string   `json:"player_steam_id,omitempty"`
	StartTick     int      `json:"start_tick,omitempty"`
	EndTick       int      `json:"end_tick,omitempty"`
	EndReason     string   `json:"end_reason,omitempty"`
	// TickRate and the record command ticks describe the generated recording
	// window. StartTick/EndTick remain the source event window and are kept
	// separate because spec_player and the recorder commands add a small delay.
	TickRate           float64   `json:"tick_rate,omitempty"`
	RecordStartTick    int       `json:"record_start_tick,omitempty"`
	RecordEndTick      int       `json:"record_end_tick,omitempty"`
	KillOffsetsSeconds []float64 `json:"kill_offsets_seconds,omitempty"`
}

type killSegment struct {
	StartTick          int
	EndTick            int
	Round              int
	Target             string
	SpecMode           int
	KillIDs            []string
	KillTicks          []int
	PreTicks           int
	PostTicks          int
	EnableVoice        bool
	EnableSpecShowXray bool
}

type clipPass struct {
	StartTick          int
	EndTick            int
	Target             string
	SpecMode           int
	View               string
	KillIDs            []string
	KillTicks          []int
	SourceID           string
	Round              int
	PlayerName         string
	PlayerSteamID      string
	EndReason          string
	EnableVoice        bool
	EnableSpecShowXray bool
}

type FullRoundPOVSegment struct {
	Round              int
	StartTick          int
	EndTick            int
	Target             string
	SpecMode           int
	SourceID           string
	PlayerName         string
	PlayerSteamID      string
	EndReason          string
	EnableVoice        bool
	EnableSpecShowXray bool
}

type bootstrapOptions struct {
	ActionSettings     ActionSettings
	ExtraCommands      []string
	EnableSpecShowXray bool
	HideAllUI          bool
	HidePlayerAvatars  bool
	HideChatMessages   bool
	UseShoulderCamera  bool
	SkyBlackout        bool
	DisableClouds      bool
	KillFeedLifetime   int
	BlockKillFeed      bool
	NoFlashStrength    float64
	RecordFPS          int
	VideoPreset        string
	FFmpegParams       string
	RecordName         string
}

type normalizedSelectedKill struct {
	Kill                    demo.ClipKill
	IncludeKiller           bool
	IncludeVictim           bool
	KillerSpecMode          int
	VictimSpecMode          int
	KillerPreSeconds        float64
	KillerPostSeconds       float64
	VictimPreSeconds        float64
	VictimPostSeconds       float64
	EnableVoice             bool
	EnableSpecShowXray      bool
	HasVoiceOverride        bool
	HasSpecShowXrayOverride bool
}

func Build(items []Item, opts BuildOptions) (*BuildResult, error) {
	normalized, err := normalizeItems(items)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 && len(opts.FullRoundPOVSegments) == 0 {
		return nil, fmt.Errorf("没有可导出的录制片段")
	}

	tickRate := opts.TickRate
	if tickRate <= 0 {
		tickRate = DefaultTickRate
	}
	seekSettleTicks := int(math.Round(segmentSeekSettleSec * tickRate))
	preTicks := int(math.Round(opts.KillerPreSeconds * tickRate))
	postTicks := int(math.Round(opts.KillerPostSeconds * tickRate))
	victimPreTicks := int(math.Round(opts.VictimPreSeconds * tickRate))
	victimPostTicks := int(math.Round(opts.VictimPostSeconds * tickRate))
	if preTicks < 0 {
		preTicks = 0
	}
	if postTicks < 0 {
		postTicks = 0
	}
	if victimPreTicks < 0 {
		victimPreTicks = 0
	}
	if victimPostTicks < 0 {
		victimPostTicks = 0
	}
	matchEndBoundTick := 0
	if opts.MatchEndTick > 0 {
		matchEndBoundTick = opts.MatchEndTick - int(math.Round(matchEndSafetySec*tickRate))
		if matchEndBoundTick < 0 {
			matchEndBoundTick = 0
		}
	}
	recordFPS := opts.RecordFPS
	if recordFPS <= 0 {
		recordFPS = 60
	}
	videoPreset, ffmpegParams, err := BuildRecordEncoding(opts.VideoPreset, opts.RecordQuality, opts.LaunchResolution)
	if err != nil {
		return nil, err
	}
	recordOutputDir := strings.TrimSpace(opts.RecordOutputDir)
	if recordOutputDir == "" {
		recordOutputDir = "outputs"
	}
	recordBatchName := strings.TrimSpace(opts.RecordBatchName)
	recordName := normalizePathForCommand(recordOutputDir)
	if recordBatchName != "" {
		recordName = normalizePathForCommand(recordOutputDir + "/" + recordBatchName)
	}

	killerSegments := buildKillerSegments(normalized, tickRate, preTicks, postTicks, matchEndBoundTick)
	victimSegments := buildVictimSegments(normalized, tickRate, victimPreTicks, victimPostTicks, matchEndBoundTick)
	sequences, takePlans, err := buildMaterialSequences(opts.FullRoundPOVSegments, killerSegments, victimSegments, "disconnect", seekSettleTicks, tickRate, buildPassCommandOptions{
		ForceVoice:      opts.ForcePerPassVoiceCommands,
		ForceXray:       opts.ForcePerPassXrayCommands,
		PovRadarEnabled: opts.PovRadarEnabled,
	})
	if err != nil {
		return nil, err
	}

	if bootstrap := buildBootstrapSequence(bootstrapOptions{
		ActionSettings:     opts.ActionSettings,
		ExtraCommands:      opts.ExtraCommands,
		EnableSpecShowXray: opts.EnableSpecShowXray,
		HideAllUI:          opts.HideAllUI,
		HidePlayerAvatars:  opts.HidePlayerAvatars,
		HideChatMessages:   opts.HideChatMessages,
		UseShoulderCamera:  opts.UseShoulderCamera,
		SkyBlackout:        opts.SkyBlackout,
		DisableClouds:      opts.DisableClouds,
		KillFeedLifetime:   opts.KillFeedLifetime,
		BlockKillFeed:      opts.BlockKillFeed,
		NoFlashStrength:    opts.NoFlashStrength,
		RecordFPS:          recordFPS,
		VideoPreset:        videoPreset,
		FFmpegParams:       ffmpegParams,
		RecordName:         recordName,
	}); len(bootstrap.Actions) > 0 {
		sequences = append([]Sequence{bootstrap}, sequences...)
	}

	if len(sequences) == 0 {
		return nil, fmt.Errorf("没有生成有效动作")
	}

	return &BuildResult{
		Sequences:    sequences,
		SegmentCount: len(opts.FullRoundPOVSegments) + len(killerSegments) + len(victimSegments),
		TakePlans:    takePlans,
	}, nil
}

func normalizeItems(items []Item) ([]normalizedSelectedKill, error) {
	if len(items) == 0 {
		return nil, nil
	}

	seen := make(map[string]normalizedSelectedKill, len(items))
	for _, raw := range items {
		kill := raw.Kill
		killID := strings.TrimSpace(kill.ID)
		if killID == "" || kill.Tick < 0 {
			continue
		}
		kill.ID = killID
		includeKiller := true
		if raw.IncludeKiller != nil {
			includeKiller = *raw.IncludeKiller
		}
		if current, ok := seen[killID]; ok {
			current.IncludeKiller = current.IncludeKiller || includeKiller
			current.IncludeVictim = current.IncludeVictim || raw.IncludeVictim
			current.KillerSpecMode = normalizeSpecMode(raw.KillerSpecMode)
			current.VictimSpecMode = normalizeSpecMode(raw.VictimSpecMode)
			current.KillerPreSeconds = raw.KillerPreSeconds
			current.KillerPostSeconds = raw.KillerPostSeconds
			current.VictimPreSeconds = raw.VictimPreSeconds
			current.VictimPostSeconds = raw.VictimPostSeconds
			current.EnableVoice = raw.EnableVoice
			current.EnableSpecShowXray = raw.EnableSpecShowXray
			current.HasVoiceOverride = current.HasVoiceOverride || raw.HasVoiceOverride
			current.HasSpecShowXrayOverride = current.HasSpecShowXrayOverride || raw.HasSpecShowXrayOverride
			seen[killID] = current
			continue
		}
		seen[killID] = normalizedSelectedKill{
			Kill:                    kill,
			IncludeKiller:           includeKiller,
			IncludeVictim:           raw.IncludeVictim,
			KillerSpecMode:          normalizeSpecMode(raw.KillerSpecMode),
			VictimSpecMode:          normalizeSpecMode(raw.VictimSpecMode),
			KillerPreSeconds:        raw.KillerPreSeconds,
			KillerPostSeconds:       raw.KillerPostSeconds,
			VictimPreSeconds:        raw.VictimPreSeconds,
			VictimPostSeconds:       raw.VictimPostSeconds,
			EnableVoice:             raw.EnableVoice,
			EnableSpecShowXray:      raw.EnableSpecShowXray,
			HasVoiceOverride:        raw.HasVoiceOverride,
			HasSpecShowXrayOverride: raw.HasSpecShowXrayOverride,
		}
	}

	result := make([]normalizedSelectedKill, 0, len(seen))
	for _, item := range seen {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kill.Tick == result[j].Kill.Tick {
			return result[i].Kill.ID < result[j].Kill.ID
		}
		return result[i].Kill.Tick < result[j].Kill.Tick
	})
	if len(result) == 0 {
		return nil, fmt.Errorf("没有可导出的击杀片段")
	}
	return result, nil
}

func normalizeSpecMode(_mode int) int {
	return defaultSpecMode
}

func killWindow(kill demo.ClipKill, preTicks int, postTicks int, matchEndBoundTick int) (int, int) {
	startTick := kill.Tick - preTicks
	if startTick < 0 {
		startTick = 0
	}
	endTick := kill.Tick + postTicks
	if matchEndBoundTick > 0 && endTick > matchEndBoundTick {
		endTick = matchEndBoundTick
		if endTick < kill.Tick {
			// The kill itself happens too close to (or after) the win panel;
			// always show the kill rather than cut it off entirely.
			endTick = kill.Tick
		}
	}
	if endTick < startTick {
		endTick = startTick
	}
	return startTick, endTick
}

func buildKillerSegments(items []normalizedSelectedKill, tickRate float64, defaultPreTicks int, defaultPostTicks int, matchEndBoundTick int) []killSegment {
	if len(items) == 0 {
		return nil
	}
	if tickRate <= 0 {
		tickRate = DefaultTickRate
	}
	segments := make([]killSegment, 0, len(items))
	for _, item := range items {
		if !item.IncludeKiller {
			continue
		}
		target := resolveSpecTarget(item.Kill)
		if target == "0" {
			continue
		}
		preTicks := defaultPreTicks
		if item.KillerPreSeconds > 0 {
			preTicks = int(math.Round(item.KillerPreSeconds * tickRate))
		}
		postTicks := defaultPostTicks
		if item.KillerPostSeconds > 0 {
			postTicks = int(math.Round(item.KillerPostSeconds * tickRate))
		}
		if preTicks <= 0 {
			preTicks = defaultPreTicks
		}
		if postTicks <= 0 {
			postTicks = defaultPostTicks
		}
		startTick, endTick := killWindow(item.Kill, preTicks, postTicks, matchEndBoundTick)
		if len(segments) == 0 {
			segments = append(segments, killSegment{
				StartTick:          startTick,
				EndTick:            endTick,
				Round:              item.Kill.Round,
				Target:             target,
				SpecMode:           item.KillerSpecMode,
				KillIDs:            []string{item.Kill.ID},
				KillTicks:          []int{item.Kill.Tick},
				PreTicks:           preTicks,
				PostTicks:          postTicks,
				EnableVoice:        item.EnableVoice,
				EnableSpecShowXray: item.EnableSpecShowXray,
			})
			continue
		}
		last := &segments[len(segments)-1]
		if last.Target == target &&
			last.SpecMode == item.KillerSpecMode &&
			last.PreTicks == preTicks &&
			last.PostTicks == postTicks &&
			last.EnableVoice == item.EnableVoice &&
			last.EnableSpecShowXray == item.EnableSpecShowXray &&
			startTick <= last.EndTick {
			if endTick > last.EndTick {
				last.EndTick = endTick
			}
			last.KillIDs = append(last.KillIDs, item.Kill.ID)
			last.KillTicks = append(last.KillTicks, item.Kill.Tick)
			if last.Round != item.Kill.Round {
				// A merged killer take may span round boundaries. Keep the
				// history round unset rather than attributing the whole take to
				// one of the source kills.
				last.Round = 0
			}
			continue
		}
		segments = append(segments, killSegment{
			StartTick:          startTick,
			EndTick:            endTick,
			Round:              item.Kill.Round,
			Target:             target,
			SpecMode:           item.KillerSpecMode,
			KillIDs:            []string{item.Kill.ID},
			KillTicks:          []int{item.Kill.Tick},
			PreTicks:           preTicks,
			PostTicks:          postTicks,
			EnableVoice:        item.EnableVoice,
			EnableSpecShowXray: item.EnableSpecShowXray,
		})
	}
	return segments
}

func buildVictimSegments(items []normalizedSelectedKill, tickRate float64, defaultPreTicks int, defaultPostTicks int, matchEndBoundTick int) []killSegment {
	if tickRate <= 0 {
		tickRate = DefaultTickRate
	}
	segments := make([]killSegment, 0, len(items))
	for _, item := range items {
		if !item.IncludeVictim {
			continue
		}
		target := resolveVictimSpecTarget(item.Kill)
		if target == "0" {
			continue
		}
		preTicks := defaultPreTicks
		if item.VictimPreSeconds > 0 {
			preTicks = int(math.Round(item.VictimPreSeconds * tickRate))
		}
		postTicks := defaultPostTicks
		if item.VictimPostSeconds > 0 {
			postTicks = int(math.Round(item.VictimPostSeconds * tickRate))
		}
		if preTicks <= 0 {
			preTicks = defaultPreTicks
		}
		if postTicks <= 0 {
			postTicks = defaultPostTicks
		}
		startTick, endTick := killWindow(item.Kill, preTicks, postTicks, matchEndBoundTick)
		segments = append(segments, killSegment{
			StartTick:          startTick,
			EndTick:            endTick,
			Round:              item.Kill.Round,
			Target:             target,
			SpecMode:           item.VictimSpecMode,
			KillIDs:            []string{item.Kill.ID},
			KillTicks:          []int{item.Kill.Tick},
			PreTicks:           preTicks,
			PostTicks:          postTicks,
			EnableVoice:        item.EnableVoice,
			EnableSpecShowXray: item.EnableSpecShowXray,
		})
	}
	return segments
}

type buildPassCommandOptions struct {
	ForceVoice      bool
	ForceXray       bool
	PovRadarEnabled bool
}

func buildMaterialSequences(
	fullRoundSegments []FullRoundPOVSegment,
	killerSegments []killSegment,
	victimSegments []killSegment,
	terminalCommand string,
	seekSettleTicks int,
	tickRate float64,
	passCommandOptions buildPassCommandOptions,
) ([]Sequence, []TakePlan, error) {
	sequences := make([]Sequence, 0, 2+len(victimSegments))
	takePlans := make([]TakePlan, 0, len(fullRoundSegments)+len(killerSegments)+len(victimSegments))
	takeCounter := 0
	terminalAction := strings.TrimSpace(terminalCommand)
	if terminalAction == "" {
		terminalAction = "disconnect"
	}

	fullRoundPasses := make([]clipPass, 0, len(fullRoundSegments))
	for _, seg := range fullRoundSegments {
		target := strings.TrimSpace(seg.Target)
		if target == "" || target == "0" || seg.StartTick < 0 || seg.EndTick < seg.StartTick {
			continue
		}
		specMode := normalizeSpecMode(seg.SpecMode)
		sourceID := strings.TrimSpace(seg.SourceID)
		if sourceID == "" {
			sourceID = strings.TrimSpace(segSourceID(seg))
		}
		fullRoundPasses = append(fullRoundPasses, clipPass{
			StartTick:          seg.StartTick,
			EndTick:            seg.EndTick,
			Target:             target,
			SpecMode:           specMode,
			View:               "full_round_pov",
			SourceID:           sourceID,
			Round:              seg.Round,
			PlayerName:         strings.TrimSpace(seg.PlayerName),
			PlayerSteamID:      strings.TrimSpace(seg.PlayerSteamID),
			EndReason:          strings.TrimSpace(seg.EndReason),
			EnableVoice:        seg.EnableVoice,
			EnableSpecShowXray: seg.EnableSpecShowXray,
		})
	}

	killerPasses := make([]clipPass, 0, len(killerSegments))
	for _, seg := range killerSegments {
		killerPasses = append(killerPasses, clipPass{
			StartTick:          seg.StartTick,
			EndTick:            seg.EndTick,
			Target:             seg.Target,
			SpecMode:           seg.SpecMode,
			View:               "killer",
			KillIDs:            append([]string(nil), seg.KillIDs...),
			KillTicks:          append([]int(nil), seg.KillTicks...),
			Round:              seg.Round,
			EnableVoice:        seg.EnableVoice,
			EnableSpecShowXray: seg.EnableSpecShowXray,
		})
	}

	victimPasses := make([]clipPass, 0, len(victimSegments))
	for _, seg := range victimSegments {
		victimPasses = append(victimPasses, clipPass{
			StartTick:          seg.StartTick,
			EndTick:            seg.EndTick,
			Target:             seg.Target,
			SpecMode:           seg.SpecMode,
			View:               "victim",
			KillIDs:            append([]string(nil), seg.KillIDs...),
			KillTicks:          append([]int(nil), seg.KillTicks...),
			Round:              seg.Round,
			EnableVoice:        seg.EnableVoice,
			EnableSpecShowXray: seg.EnableSpecShowXray,
		})
	}

	groups := buildMaterialPassGroups(fullRoundPasses, killerPasses, victimPasses, passCommandOptions.PovRadarEnabled)
	for idx, passes := range groups {
		finalCommand := terminalAction
		if idx < len(groups)-1 {
			finalCommand = "go_to_next_sequence"
		}
		actions, plans, err := buildActionsFromPasses(passes, finalCommand, &takeCounter, seekSettleTicks, tickRate, passCommandOptions)
		if err != nil {
			return nil, nil, err
		}
		if len(actions) > 0 {
			sequences = append(sequences, Sequence{Actions: actions})
			takePlans = append(takePlans, plans...)
		}
	}

	return sequences, takePlans, nil
}

// buildMaterialPassGroups assembles the material passes into sequence groups in
// the fixed full-round -> killer -> victim order. Without the POV radar the
// legacy grouping is kept (all full-round passes share one sequence, all killer
// passes share another, each victim pass is its own sequence). With the radar
// enabled every pass gets its own sequence so a later pass can seek back and
// warm the radar up without switching players during the previous recording.
func buildMaterialPassGroups(fullRoundPasses, killerPasses, victimPasses []clipPass, radarPerPass bool) [][]clipPass {
	groups := make([][]clipPass, 0, 2+len(victimPasses))
	if radarPerPass {
		for _, pass := range fullRoundPasses {
			groups = append(groups, []clipPass{pass})
		}
		for _, pass := range killerPasses {
			groups = append(groups, []clipPass{pass})
		}
		for _, pass := range victimPasses {
			groups = append(groups, []clipPass{pass})
		}
		return groups
	}
	if len(fullRoundPasses) > 0 {
		groups = append(groups, fullRoundPasses)
	}
	if len(killerPasses) > 0 {
		groups = append(groups, killerPasses)
	}
	for _, pass := range victimPasses {
		groups = append(groups, []clipPass{pass})
	}
	return groups
}

func segSourceID(seg FullRoundPOVSegment) string {
	steamID := strings.TrimSpace(seg.PlayerSteamID)
	if steamID == "" {
		steamID = "unknown"
	}
	return fmt.Sprintf("full_round_pov:r%d:p%s", seg.Round, steamID)
}

func buildBootstrapSequence(opts bootstrapOptions) Sequence {
	actions := make([]Action, 0, 16+len(opts.ExtraCommands))
	actionTick := DefaultActionTick
	voiceEnabled := opts.ActionSettings.EnableVoiceIndices && opts.ActionSettings.EnableVoiceIndicesH
	appendCommand := func(cmd string) {
		actions = append(actions, Action{Cmd: cmd, Tick: actionTick})
	}

	if opts.UseShoulderCamera {
		for _, cmd := range ShoulderCameraCommands(true) {
			appendCommand(cmd)
		}
	}
	appendCommand("r_show_build_info 0")
	appendCommand("cl_trueview_show_status 0")
	appendCommand("engine_no_focus_sleep 0")
	appendCommand("cl_demo_predict 0")
	appendCommand("cl_spec_show_bindings 0")
	if opts.DisableClouds {
		appendCommand(CloudsCommand(true))
	}
	if opts.SkyBlackout {
		appendCommand(SkyBlackoutCommand(true))
	}
	// Zero strength keeps the historical output: the command is not written at
	// all, so the demo keeps the engine's normal flash effect.
	if opts.NoFlashStrength > 0 {
		appendCommand(NoFlashCommand(opts.NoFlashStrength))
	}
	appendCommand(KillFeedLifetimeCommand(opts.KillFeedLifetime))
	if opts.BlockKillFeed {
		for _, cmd := range BlockKillFeedCommands(true) {
			appendCommand(cmd)
		}
	}

	appendCommand(SpecShowXrayCommand(opts.EnableSpecShowXray))
	if opts.HideAllUI {
		appendCommand(HideAllUICommand(true))
	}
	appendCommand(PlayerAvatarModeCommand(opts.HidePlayerAvatars))
	if opts.HideChatMessages {
		appendCommand(HideChatCommand(true))
	}
	for _, cmd := range VoiceIndicesCommands(voiceEnabled) {
		appendCommand(cmd)
	}
	for _, cmd := range RecordInitCommands(RecordInitOptions{
		RecordFPS:    opts.RecordFPS,
		VideoPreset:  opts.VideoPreset,
		FFmpegParams: opts.FFmpegParams,
		RecordName:   opts.RecordName,
	}) {
		appendCommand(cmd)
	}
	for _, cmd := range opts.ExtraCommands {
		command := strings.TrimSpace(cmd)
		if command == "" {
			continue
		}
		appendCommand(command)
	}
	if len(actions) == 0 {
		return Sequence{}
	}
	actions = append(actions, Action{Cmd: "go_to_next_sequence", Tick: actionTick + 1})
	sort.SliceStable(actions, func(i, j int) bool {
		return actions[i].Tick < actions[j].Tick
	})
	return Sequence{Actions: actions}
}

func voiceIndicesValue(enabled bool) int {
	if enabled {
		return -1
	}
	return 0
}

func buildActionsFromPasses(
	passes []clipPass,
	finalCommand string,
	takeCounter *int,
	seekSettleTicks int,
	tickRate float64,
	passCommandOptions buildPassCommandOptions,
) ([]Action, []TakePlan, error) {
	if len(passes) == 0 {
		return nil, nil, nil
	}
	command := strings.TrimSpace(finalCommand)
	if command == "" {
		command = "disconnect"
	}
	if tickRate <= 0 {
		tickRate = DefaultTickRate
	}
	minSpecModeTick := DefaultActionTick + segmentSpecModeDelay

	actions := make([]Action, 0, len(passes)*6+1)
	takePlans := make([]TakePlan, 0, len(passes))
	lastRecordEndTick := 0
	for idx, pass := range passes {
		jumpTick := DefaultActionTick
		if idx > 0 {
			jumpTick = passes[idx-1].EndTick + 1
			if jumpTick < DefaultActionTick {
				jumpTick = DefaultActionTick
			}
		}

		// The recorder window always comes from the legacy timing so the
		// exported video never changes when the POV radar is enabled.
		specModeTick := pass.StartTick + segmentSpecModeDelay
		if specModeTick < minSpecModeTick {
			specModeTick = minSpecModeTick
		}
		specPlayerTick := pass.StartTick + segmentSpecPlayerDelay
		if specPlayerTick < DefaultActionTick+segmentSpecPlayerDelay {
			specPlayerTick = DefaultActionTick + segmentSpecPlayerDelay
		}
		startTick := specPlayerTick + segmentRecordDelay
		if startTick < DefaultActionTick+segmentSpecPlayerDelay+segmentRecordDelay {
			startTick = DefaultActionTick + segmentSpecPlayerDelay + segmentRecordDelay
		}
		endTick := pass.EndTick + 1
		if endTick < startTick {
			endTick = startTick
		}
		lastRecordEndTick = endTick

		radarTick := 0
		if passCommandOptions.PovRadarEnabled {
			// Switch the camera first, enable the radar, then let the Demo play
			// at least povRadarWarmupSeconds of Demo time before the recorder
			// starts. Move the whole preparation earlier instead of delaying the
			// recording, otherwise short clips would lose their kill.
			warmupTicks := int(math.Ceil(povRadarWarmupSeconds * tickRate))
			radarTick = startTick - warmupTicks
			specPlayerTick = radarTick - povRadarSpecPlayerToRadar
			specModeTick = specPlayerTick - povRadarSpecModeToSpecPlayer
			if specModeTick < minSpecModeTick {
				return nil, nil, fmt.Errorf(
					"Demo 起点不足以完成 POV 雷达两秒预热：%s start_tick=%d spec_mode_tick=%d 低于安全下限 %d；请减少录制前置时间（pre_seconds）或关闭 POV 雷达",
					describePass(pass), pass.StartTick, specModeTick, minSpecModeTick)
			}
		}

		seekTargetTick := pass.StartTick - seekSettleTicks
		if passCommandOptions.PovRadarEnabled {
			seekTargetTick = specModeTick - seekSettleTicks
		}
		minSeekTargetTick := 0
		if idx > 0 {
			minSeekTargetTick = jumpTick
		}
		if seekTargetTick < minSeekTargetTick {
			seekTargetTick = minSeekTargetTick
		}
		// Radar passes each start in a fresh sequence at Demo tick 0. A seek
		// back to (or onto) its own trigger tick would be consumed again when
		// playback crosses that tick, since the plugin tracks tick crossings,
		// not consumed actions. Let these early passes play from 0 instead.
		if !passCommandOptions.PovRadarEnabled || seekTargetTick > jumpTick {
			actions = append(actions, Action{Cmd: fmt.Sprintf("demo_gototick %d", seekTargetTick), Tick: jumpTick})
		}

		if passCommandOptions.ForceVoice {
			voiceValue := voiceIndicesValue(pass.EnableVoice)
			actions = append(actions, Action{Cmd: fmt.Sprintf("tv_listen_voice_indices %d", voiceValue), Tick: specModeTick})
			actions = append(actions, Action{Cmd: fmt.Sprintf("tv_listen_voice_indices_h %d", voiceValue), Tick: specModeTick})
		}
		if passCommandOptions.ForceXray {
			actions = append(actions, Action{Cmd: fmt.Sprintf("spec_show_xray %d", xrayCommandValue(pass.EnableSpecShowXray)), Tick: specModeTick})
		}
		actions = append(actions, Action{Cmd: fmt.Sprintf("spec_mode %d", normalizeSpecMode(pass.SpecMode)), Tick: specModeTick})
		actions = append(actions, Action{Cmd: "spec_player " + pass.Target, Tick: specPlayerTick})
		if radarTick > 0 {
			actions = append(actions, Action{Cmd: PovRadarCommand(true), Tick: radarTick})
		}
		if takeCounter != nil {
			*takeCounter = *takeCounter + 1
			takeName := fmt.Sprintf("take%04d", *takeCounter-1)
			takePlans = append(takePlans, TakePlan{
				TakeIndex:          *takeCounter,
				TakeName:           takeName,
				View:               strings.TrimSpace(pass.View),
				SpecMode:           normalizeSpecMode(pass.SpecMode),
				KillIDs:            append([]string(nil), pass.KillIDs...),
				SourceID:           strings.TrimSpace(pass.SourceID),
				Round:              pass.Round,
				PlayerName:         strings.TrimSpace(pass.PlayerName),
				PlayerSteamID:      strings.TrimSpace(pass.PlayerSteamID),
				StartTick:          pass.StartTick,
				EndTick:            pass.EndTick,
				EndReason:          strings.TrimSpace(pass.EndReason),
				TickRate:           tickRate,
				RecordStartTick:    startTick,
				RecordEndTick:      endTick,
				KillOffsetsSeconds: killOffsetsFromTicks(pass.KillTicks, startTick, tickRate),
			})
			actions = append(actions, Action{
				Cmd:  "mirv_streams record start",
				Tick: startTick,
				Metadata: &ActionMetadata{
					TakeIndex:   *takeCounter,
					TakeName:    takeName,
					RecordPhase: "start",
				},
			})
			actions = append(actions, Action{
				Cmd:  "mirv_streams record end",
				Tick: endTick,
				Metadata: &ActionMetadata{
					TakeIndex:   *takeCounter,
					TakeName:    takeName,
					RecordPhase: "end",
				},
			})
		} else {
			actions = append(actions, Action{Cmd: "mirv_streams record start", Tick: startTick})
			actions = append(actions, Action{Cmd: "mirv_streams record end", Tick: endTick})
		}
	}

	// Never cut the sequence before the recorder has stopped: a very short
	// clip can clamp the record end past the source window end.
	disconnectTick := passes[len(passes)-1].EndTick + 1
	if disconnectTick < lastRecordEndTick {
		disconnectTick = lastRecordEndTick
	}
	actions = append(actions, Action{Cmd: command, Tick: disconnectTick})

	sort.SliceStable(actions, func(i, j int) bool {
		return actions[i].Tick < actions[j].Tick
	})
	return actions, takePlans, nil
}

// describePass identifies one material pass in generation errors without
// leaking any App or filesystem concerns into this package.
func describePass(pass clipPass) string {
	parts := make([]string, 0, 4)
	if view := strings.TrimSpace(pass.View); view != "" {
		parts = append(parts, "view="+view)
	}
	if sourceID := strings.TrimSpace(pass.SourceID); sourceID != "" {
		parts = append(parts, "source_id="+sourceID)
	}
	if len(pass.KillIDs) > 0 {
		parts = append(parts, "kill_ids="+strings.Join(pass.KillIDs, ","))
	}
	if target := strings.TrimSpace(pass.Target); target != "" {
		parts = append(parts, "target="+target)
	}
	if len(parts) == 0 {
		return "pass"
	}
	return "pass[" + strings.Join(parts, " ") + "]"
}

func xrayCommandValue(enableSpecShowXray bool) int {
	if enableSpecShowXray {
		return 0
	}
	return -1
}

func killOffsetsFromTicks(killTicks []int, recordStartTick int, tickRate float64) []float64 {
	if len(killTicks) == 0 || recordStartTick <= 0 || tickRate <= 0 {
		return nil
	}
	offsets := make([]float64, 0, len(killTicks))
	for _, tick := range killTicks {
		if tick < 0 {
			continue
		}
		offsets = append(offsets, float64(tick-recordStartTick)/tickRate)
	}
	if len(offsets) == 0 {
		return nil
	}
	return offsets
}

// BuildRecordEncoding resolves the HLAE ffmpeg profile and its encode
// parameters for one already-resolved preset. The two 4:3 launch resolutions
// keep the real-pixel stretch. Callers resolve `auto` to an effective preset
// before calling this helper.
func BuildRecordEncoding(preset string, quality string, launchResolution string) (string, string, error) {
	videoPreset, _, err := ffmpegprofile.HLAEProfileByID(preset)
	if err != nil {
		return "", "", err
	}
	params, err := ffmpegprofile.BuildRecordingEncodeArgs(videoPreset, quality)
	if err != nil {
		return "", "", err
	}
	if shouldStretchFourThreeRecording(launchResolution) {
		// 真实像素拉伸替代 -aspect 元数据：忽略 SAR 的播放器不再出侧边黑边；取偶防 HEVC 奇数宽失败。
		params += " -vf scale=trunc(ih*16/9/2)*2:ih"
	}
	return videoPreset, params, nil
}

func shouldStretchFourThreeRecording(launchResolution string) bool {
	switch strings.TrimSpace(launchResolution) {
	case "4:3", "4:3_1280x960":
		return true
	default:
		return false
	}
}

func normalizePathForCommand(path string) string {
	path = strings.TrimSpace(path)
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimSuffix(path, "/")
	return path
}

func resolveSpecTarget(kill demo.ClipKill) string {
	if kill.KillerSlot > 0 {
		return strconv.Itoa(kill.KillerSlot)
	}
	if kill.KillerEntityID > 0 {
		return strconv.Itoa(kill.KillerEntityID)
	}
	name := strings.TrimSpace(kill.KillerName)
	if name == "" {
		return "0"
	}
	name = strings.ReplaceAll(name, `"`, "")
	return fmt.Sprintf("%q", name)
}

func resolveVictimSpecTarget(kill demo.ClipKill) string {
	if kill.VictimSlot > 0 {
		return strconv.Itoa(kill.VictimSlot)
	}
	if kill.VictimEntityID > 0 {
		return strconv.Itoa(kill.VictimEntityID)
	}
	name := strings.TrimSpace(kill.VictimName)
	if name == "" {
		return "0"
	}
	name = strings.ReplaceAll(name, `"`, "")
	return fmt.Sprintf("%q", name)
}
