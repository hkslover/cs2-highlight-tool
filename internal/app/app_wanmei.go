package app

import (
	"strings"

	"cs2-highlight-tool-v2/internal/envsetup"
	"cs2-highlight-tool-v2/internal/wanmei"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) ListWanmeiRecentMatches(page int) (*wanmei.WanmeiMatchListResult, error) {
	return wanmei.ListRecentMatches(page)
}

func (a *App) ImportWanmeiMatch(matchID string) ([]string, error) {
	releaseFiles, fileErr := a.beginManagedFileUse()
	if fileErr != nil {
		return nil, fileErr
	}
	defer releaseFiles()

	downloadMatchID, err := wanmei.ExtractNumericMatchID(matchID)
	if err != nil {
		return nil, err
	}
	cacheRoot := a.dataPath("demo", "wanmei", downloadMatchID)
	progressID := wanmei.ProgressComponentID(downloadMatchID)

	stablePath, err := wanmei.ImportDemo(downloadMatchID, cacheRoot, func(active bool, percent float64, indeterminate bool) {
		a.emitWanmeiDownloadProgress(progressID, active, percent, indeterminate)
	})
	if err != nil {
		return nil, err
	}
	a.cleanupLegacyRawDemoCopy(stablePath)
	return []string{stablePath}, nil
}

func (a *App) emitWanmeiDownloadProgress(componentID string, active bool, percent float64, indeterminate bool) {
	if a == nil || a.ctx == nil || strings.TrimSpace(componentID) == "" {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "download_progress", envsetup.ProgressMessage{
		ComponentID:   componentID,
		Active:        active,
		Percent:       percent,
		Indeterminate: indeterminate,
	})
}
