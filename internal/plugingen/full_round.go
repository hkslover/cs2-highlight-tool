package plugingen

import (
	"math"
	"strconv"
	"strings"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/demo"
)

const fullRoundPOVEndPaddingSeconds = 1.0

// FullRoundPOVSettings is the generation subset of clip settings applied to
// every full-round pass.
type FullRoundPOVSettings struct {
	EnableVoice        bool
	EnableSpecShowXray bool
}

// BuildFullRoundPOVSegments converts parsed demo facts into the clipsjson
// segment type. The conversion is deterministic and performs no filesystem or
// process work; ParseFullRoundPOVPlan remains an app/demo responsibility.
func BuildFullRoundPOVSegments(
	plan *demo.FullRoundPOVPlan,
	settings FullRoundPOVSettings,
	tickRate float64,
) []clipsjson.FullRoundPOVSegment {
	if plan == nil || len(plan.Segments) == 0 {
		return nil
	}
	if tickRate <= 0 {
		tickRate = 64
	}
	segments := make([]clipsjson.FullRoundPOVSegment, 0, len(plan.Segments))
	for _, segment := range plan.Segments {
		if segment.RecordStartTick < 0 || segment.RecordEndTick < segment.RecordStartTick || segment.TargetSlot <= 0 {
			continue
		}
		endTick := FullRoundPOVRecordEndTick(segment, tickRate)
		segments = append(segments, clipsjson.FullRoundPOVSegment{
			Round:              segment.Round,
			StartTick:          segment.RecordStartTick,
			EndTick:            endTick,
			Target:             strconv.Itoa(segment.TargetSlot),
			SpecMode:           1,
			SourceID:           BuildFullRoundPOVSourceID(segment.Round, plan.PlayerSteamID),
			PlayerName:         strings.TrimSpace(plan.PlayerName),
			PlayerSteamID:      strings.TrimSpace(plan.PlayerSteamID),
			EndReason:          strings.TrimSpace(segment.EndReason),
			EnableVoice:        settings.EnableVoice,
			EnableSpecShowXray: settings.EnableSpecShowXray,
		})
	}
	return segments
}

// FullRoundPOVRecordEndTick applies the one-second recording-end padding
// policy to an already parsed segment.
func FullRoundPOVRecordEndTick(segment demo.FullRoundPOVSegment, tickRate float64) int {
	endTick := segment.RecordEndTick
	if tickRate <= 0 {
		return endTick
	}
	paddingTicks := int(math.Round(fullRoundPOVEndPaddingSeconds * tickRate))
	if paddingTicks <= 0 {
		return endTick
	}
	if strings.TrimSpace(segment.EndReason) == demo.FullRoundPOVEndTargetDeath {
		return endTick + paddingTicks
	}
	if segment.NextRoundStartTick > 0 {
		nextRoundEndTick := segment.NextRoundStartTick - paddingTicks
		if nextRoundEndTick >= segment.RecordStartTick {
			return nextRoundEndTick
		}
	}
	return endTick
}

// BuildFullRoundPOVSourceID returns the stable source identity used by take
// history and by the clipsjson builder.
func BuildFullRoundPOVSourceID(round int, playerSteamID string) string {
	steamID := strings.TrimSpace(playerSteamID)
	if steamID == "" {
		steamID = "unknown"
	}
	return "full_round_pov:r" + strconv.Itoa(round) + ":p" + steamID
}
