package app

import (
	"context"
	"path/filepath"
	"strings"

	"cs2-highlight-tool-v2/internal/envsetup"
	"cs2-highlight-tool-v2/internal/wanmei"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) ListWanmeiRecentMatches(page int) (*wanmei.WanmeiMatchListResult, error) {
	return wanmei.ListRecentMatches(page)
}

func (a *App) ImportWanmeiMatch(matchID string) ([]string, error) {
	// See ImportFiveEMatch: the workspace reservation precedes normalization so
	// a directory clear cannot be bypassed, and it covers the whole shared task.
	workCtx, releaseFiles, dataDir, fileErr := a.beginManagedWorkspaceTaskUse()
	if fileErr != nil {
		return nil, fileErr
	}
	defer releaseFiles()

	downloadMatchID, err := wanmei.ExtractNumericMatchID(matchID)
	if err != nil {
		return nil, err
	}
	cacheRoot := filepath.Join(dataDir, "demo", "wanmei", downloadMatchID)
	progressID := wanmei.ProgressComponentID(downloadMatchID)

	stablePath, ranImport, err := a.importCoordinator().do(
		workCtx,
		workCtx,
		platformImportKey{platform: platformImportWanmei, matchID: downloadMatchID},
		func(runCtx context.Context) (string, error) {
			return wanmei.ImportDemoContext(runCtx, downloadMatchID, cacheRoot, func(active bool, percent float64, indeterminate bool) {
				a.emitWanmeiDownloadProgress(progressID, active, percent, indeterminate)
			})
		},
	)
	if err != nil {
		return nil, err
	}
	if ranImport {
		a.cleanupLegacyRawDemoCopyAt(stablePath, dataDir)
	}
	return []string{stablePath}, nil
}

func (a *App) emitWanmeiDownloadProgress(componentID string, active bool, percent float64, indeterminate bool) {
	if a == nil || a.ctx == nil || strings.TrimSpace(componentID) == "" {
		return
	}
	if session := a.workspaceSnapshot().session; session != nil && session.isClosed() {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "download_progress", envsetup.ProgressMessage{
		ComponentID:   componentID,
		Active:        active,
		Percent:       percent,
		Indeterminate: indeterminate,
	})
}
