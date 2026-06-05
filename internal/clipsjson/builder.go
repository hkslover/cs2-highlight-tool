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
)

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
	TickRate                  float64
	KillerPreSeconds          float64
	KillerPostSeconds         float64
	VictimPreSeconds          float64
	VictimPostSeconds         float64
	ExtraCommands             []string
	ActionSettings            ActionSettings
	RecordFPS                 int
	RecordQuality             string
	VideoPreset               string
	RecordOutputDir           string
	RecordBatchName           string
	EnableSpecShowXray        bool
	HideAllUI                 bool
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
	TakeIndex int      `json:"take_index"`
	TakeName  string   `json:"take_name"`
	View      string   `json:"view"`
	SpecMode  int      `json:"spec_mode"`
	KillIDs   []string `json:"kill_ids"`
}

type killSegment struct {
	StartTick          int
	EndTick            int
	Target             string
	SpecMode           int
	KillIDs            []string
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
	EnableVoice        bool
	EnableSpecShowXray bool
}

type bootstrapOptions struct {
	ActionSettings     ActionSettings
	ExtraCommands      []string
	EnableSpecShowXray bool
	HideAllUI          bool
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
	if len(normalized) == 0 {
		return nil, fmt.Errorf("没有可导出的击杀片段")
	}

	tickRate := opts.TickRate
	if tickRate <= 0 {
		tickRate = DefaultTickRate
	}
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
	recordFPS := opts.RecordFPS
	if recordFPS <= 0 {
		recordFPS = 60
	}
	videoPreset, ffmpegParams, err := buildFFmpegParams(opts.VideoPreset, opts.RecordQuality, opts.LaunchResolution)
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

	killerSegments := buildKillerSegments(normalized, tickRate, preTicks, postTicks)
	victimSegments := buildVictimSegments(normalized, tickRate, victimPreTicks, victimPostTicks)
	sequences, takePlans := buildMaterialSequences(killerSegments, victimSegments, "disconnect", buildPassCommandOptions{
		ForceVoice: opts.ForcePerPassVoiceCommands,
		ForceXray:  opts.ForcePerPassXrayCommands,
	})

	if bootstrap := buildBootstrapSequence(bootstrapOptions{
		ActionSettings:     opts.ActionSettings,
		ExtraCommands:      opts.ExtraCommands,
		EnableSpecShowXray: opts.EnableSpecShowXray,
		HideAllUI:          opts.HideAllUI,
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
		SegmentCount: len(killerSegments) + len(victimSegments),
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

func killWindow(kill demo.ClipKill, preTicks int, postTicks int) (int, int) {
	startTick := kill.Tick - preTicks
	if startTick < 0 {
		startTick = 0
	}
	endTick := kill.Tick + postTicks
	if endTick < startTick {
		endTick = startTick
	}
	return startTick, endTick
}

func buildKillerSegments(items []normalizedSelectedKill, tickRate float64, defaultPreTicks int, defaultPostTicks int) []killSegment {
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
		startTick, endTick := killWindow(item.Kill, preTicks, postTicks)
		if len(segments) == 0 {
			segments = append(segments, killSegment{
				StartTick:          startTick,
				EndTick:            endTick,
				Target:             target,
				SpecMode:           item.KillerSpecMode,
				KillIDs:            []string{item.Kill.ID},
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
			continue
		}
		segments = append(segments, killSegment{
			StartTick:          startTick,
			EndTick:            endTick,
			Target:             target,
			SpecMode:           item.KillerSpecMode,
			KillIDs:            []string{item.Kill.ID},
			PreTicks:           preTicks,
			PostTicks:          postTicks,
			EnableVoice:        item.EnableVoice,
			EnableSpecShowXray: item.EnableSpecShowXray,
		})
	}
	return segments
}

func buildVictimSegments(items []normalizedSelectedKill, tickRate float64, defaultPreTicks int, defaultPostTicks int) []killSegment {
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
		startTick, endTick := killWindow(item.Kill, preTicks, postTicks)
		segments = append(segments, killSegment{
			StartTick:          startTick,
			EndTick:            endTick,
			Target:             target,
			SpecMode:           item.VictimSpecMode,
			KillIDs:            []string{item.Kill.ID},
			PreTicks:           preTicks,
			PostTicks:          postTicks,
			EnableVoice:        item.EnableVoice,
			EnableSpecShowXray: item.EnableSpecShowXray,
		})
	}
	return segments
}

type buildPassCommandOptions struct {
	ForceVoice bool
	ForceXray  bool
}

func buildMaterialSequences(
	killerSegments []killSegment,
	victimSegments []killSegment,
	terminalCommand string,
	passCommandOptions buildPassCommandOptions,
) ([]Sequence, []TakePlan) {
	sequences := make([]Sequence, 0, 1+len(victimSegments))
	takePlans := make([]TakePlan, 0, len(killerSegments)+len(victimSegments))
	takeCounter := 0
	terminalAction := strings.TrimSpace(terminalCommand)
	if terminalAction == "" {
		terminalAction = "disconnect"
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
			EnableVoice:        seg.EnableVoice,
			EnableSpecShowXray: seg.EnableSpecShowXray,
		})
	}
	if len(killerPasses) > 0 {
		command := terminalAction
		if len(victimSegments) > 0 {
			command = "go_to_next_sequence"
		}
		actions, plans := buildActionsFromPasses(killerPasses, command, &takeCounter, passCommandOptions)
		if len(actions) > 0 {
			sequences = append(sequences, Sequence{Actions: actions})
			takePlans = append(takePlans, plans...)
		}
	}

	for idx, seg := range victimSegments {
		finalCommand := "go_to_next_sequence"
		if idx == len(victimSegments)-1 {
			finalCommand = terminalAction
		}
		actions, plans := buildActionsFromPasses([]clipPass{{
			StartTick:          seg.StartTick,
			EndTick:            seg.EndTick,
			Target:             seg.Target,
			SpecMode:           seg.SpecMode,
			View:               "victim",
			KillIDs:            append([]string(nil), seg.KillIDs...),
			EnableVoice:        seg.EnableVoice,
			EnableSpecShowXray: seg.EnableSpecShowXray,
		}}, finalCommand, &takeCounter, passCommandOptions)
		if len(actions) > 0 {
			sequences = append(sequences, Sequence{Actions: actions})
			takePlans = append(takePlans, plans...)
		}
	}

	return sequences, takePlans
}

func buildBootstrapSequence(opts bootstrapOptions) Sequence {
	actions := make([]Action, 0, 16+len(opts.ExtraCommands))
	actionTick := DefaultActionTick
	voiceEnabled := opts.ActionSettings.EnableVoiceIndices && opts.ActionSettings.EnableVoiceIndicesH
	voiceValue := voiceIndicesValue(voiceEnabled)

	actions = append(actions, Action{Cmd: "r_show_build_info 0", Tick: actionTick})
	actions = append(actions, Action{Cmd: "cl_trueview_show_status 0", Tick: actionTick})
	actions = append(actions, Action{Cmd: "engine_no_focus_sleep 0", Tick: actionTick})
	actions = append(actions, Action{Cmd: "cl_demo_predict 0", Tick: actionTick})
	actions = append(actions, Action{Cmd: fmt.Sprintf("spec_show_xray %d", xrayCommandValue(opts.EnableSpecShowXray)), Tick: actionTick})
	if opts.HideAllUI {
		actions = append(actions, Action{Cmd: "cl_draw_only_deathnotices 1", Tick: actionTick})
	}
	actions = append(actions, Action{Cmd: fmt.Sprintf("tv_listen_voice_indices %d", voiceValue), Tick: actionTick})
	actions = append(actions, Action{Cmd: fmt.Sprintf("tv_listen_voice_indices_h %d", voiceValue), Tick: actionTick})
	actions = append(actions, Action{Cmd: "mirv_streams record screen enabled 1", Tick: actionTick})
	actions = append(actions, Action{Cmd: fmt.Sprintf("mirv_streams record fps %d", opts.RecordFPS), Tick: actionTick})
	actions = append(actions, Action{Cmd: "mirv_streams record startMovieWav 1", Tick: actionTick})
	actions = append(actions, Action{
		Cmd:  fmt.Sprintf(`mirv_streams settings add ffmpeg %s "%s {QUOTE}{AFX_STREAM_PATH}.mp4{QUOTE}"`, opts.VideoPreset, opts.FFmpegParams),
		Tick: actionTick,
	})
	actions = append(actions, Action{Cmd: fmt.Sprintf("mirv_streams record screen settings %s", opts.VideoPreset), Tick: actionTick})
	actions = append(actions, Action{Cmd: fmt.Sprintf(`mirv_streams record name "%s"`, opts.RecordName), Tick: actionTick})
	for _, cmd := range opts.ExtraCommands {
		command := strings.TrimSpace(cmd)
		if command == "" {
			continue
		}
		actions = append(actions, Action{Cmd: command, Tick: actionTick})
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
	passCommandOptions buildPassCommandOptions,
) ([]Action, []TakePlan) {
	if len(passes) == 0 {
		return nil, nil
	}
	command := strings.TrimSpace(finalCommand)
	if command == "" {
		command = "disconnect"
	}

	actions := make([]Action, 0, len(passes)*5+1)
	takePlans := make([]TakePlan, 0, len(passes))
	for idx, pass := range passes {
		jumpTick := DefaultActionTick
		if idx > 0 {
			jumpTick = passes[idx-1].EndTick + 1
			if jumpTick < DefaultActionTick {
				jumpTick = DefaultActionTick
			}
		}
		actions = append(actions, Action{Cmd: fmt.Sprintf("demo_gototick %d", pass.StartTick), Tick: jumpTick})

		specModeTick := pass.StartTick + segmentSpecModeDelay
		if specModeTick < DefaultActionTick+segmentSpecModeDelay {
			specModeTick = DefaultActionTick + segmentSpecModeDelay
		}
		if passCommandOptions.ForceVoice {
			voiceValue := voiceIndicesValue(pass.EnableVoice)
			actions = append(actions, Action{Cmd: fmt.Sprintf("tv_listen_voice_indices %d", voiceValue), Tick: specModeTick})
			actions = append(actions, Action{Cmd: fmt.Sprintf("tv_listen_voice_indices_h %d", voiceValue), Tick: specModeTick})
		}
		if passCommandOptions.ForceXray {
			actions = append(actions, Action{Cmd: fmt.Sprintf("spec_show_xray %d", xrayCommandValue(pass.EnableSpecShowXray)), Tick: specModeTick})
		}
		specPlayerTick := pass.StartTick + segmentSpecPlayerDelay
		if specPlayerTick < DefaultActionTick+segmentSpecPlayerDelay {
			specPlayerTick = DefaultActionTick + segmentSpecPlayerDelay
		}
		actions = append(actions, Action{Cmd: fmt.Sprintf("spec_mode %d", normalizeSpecMode(pass.SpecMode)), Tick: specModeTick})
		actions = append(actions, Action{Cmd: "spec_player " + pass.Target, Tick: specPlayerTick})
		startTick := specPlayerTick + segmentRecordDelay
		if startTick < DefaultActionTick+segmentSpecPlayerDelay+segmentRecordDelay {
			startTick = DefaultActionTick + segmentSpecPlayerDelay + segmentRecordDelay
		}
		endTick := pass.EndTick + 1
		if endTick < startTick {
			endTick = startTick
		}
		if takeCounter != nil {
			*takeCounter = *takeCounter + 1
			takeName := fmt.Sprintf("take%04d", *takeCounter-1)
			takePlans = append(takePlans, TakePlan{
				TakeIndex: *takeCounter,
				TakeName:  takeName,
				View:      strings.TrimSpace(pass.View),
				SpecMode:  normalizeSpecMode(pass.SpecMode),
				KillIDs:   append([]string(nil), pass.KillIDs...),
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

	disconnectTick := passes[len(passes)-1].EndTick + 1
	actions = append(actions, Action{Cmd: command, Tick: disconnectTick})

	sort.SliceStable(actions, func(i, j int) bool {
		return actions[i].Tick < actions[j].Tick
	})
	return actions, takePlans
}

func xrayCommandValue(enableSpecShowXray bool) int {
	if enableSpecShowXray {
		return 0
	}
	return -1
}

func buildFFmpegParams(preset string, quality string, launchResolution string) (string, string, error) {
	videoPreset, _, err := ffmpegprofile.HLAEProfileByID(preset)
	if err != nil {
		return "", "", err
	}
	params, err := ffmpegprofile.BuildRecordingEncodeArgs(videoPreset, quality)
	if err != nil {
		return "", "", err
	}
	if launchResolution == "4:3" || launchResolution == "4:3_1280x960" {
		params += " -aspect 16:9"
	}
	return videoPreset, params, nil
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
