package app

import (
	"fmt"
	"strconv"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/demo"
	"cs2-highlight-tool-v2/internal/plugingen"
)

func (a *App) PreviewFullRoundPOV(demoPath, playerSteamID string) (*demo.FullRoundPOVPlan, error) {
	releaseFiles, fileErr := a.beginManagedFileUse()
	if fileErr != nil {
		return nil, fileErr
	}
	defer releaseFiles()

	steamID, err := strconv.ParseUint(playerSteamID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("无效的 SteamID: %w", err)
	}
	return demo.ParseFullRoundPOVPlan(demoPath, steamID)
}

func buildFullRoundPOVSegmentsForPlugin(plan *demo.FullRoundPOVPlan, settings ClipSettings, tickRate float64) []clipsjson.FullRoundPOVSegment {
	return plugingen.BuildFullRoundPOVSegments(plan, plugingen.FullRoundPOVSettings{
		EnableVoice:        settings.EnableVoice,
		EnableSpecShowXray: settings.EnableSpecShowXray,
	}, tickRate)
}

func fullRoundPOVRecordEndTick(segment demo.FullRoundPOVSegment, _ ClipSettings, tickRate float64) int {
	return plugingen.FullRoundPOVRecordEndTick(segment, tickRate)
}

func buildFullRoundPOVSourceID(round int, playerSteamID string) string {
	return plugingen.BuildFullRoundPOVSourceID(round, playerSteamID)
}
