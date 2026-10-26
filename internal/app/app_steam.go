package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/envsetup"
	"cs2-highlight-tool-v2/internal/steam"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// SteamImportRequest identifies the official match to import. Either field can
// be provided: the match ID comes from the recent match list, while a pasted
// share code also works for matches that are not in that list (the Game
// Coordinator resolves the share code into a match).
type SteamImportRequest struct {
	MatchID   string `json:"match_id"`
	ShareCode string `json:"share_code"`
}

// steamMatchSnapshot remembers the demo URL of the last match list per match,
// so importing a listed match does not need another Game Coordinator round trip
// (the coordinator is unusable while CS2 is running, and its links are only
// valid for a while anyway).
type steamMatchSnapshot struct {
	mu   sync.Mutex
	urls map[string]string
}

func newSteamMatchSnapshot() *steamMatchSnapshot {
	return &steamMatchSnapshot{urls: make(map[string]string)}
}

// replace swaps the snapshot for the matches of a fresh list response.
func (s *steamMatchSnapshot) replace(matches []steam.MatchItem) {
	urls := make(map[string]string, len(matches))
	for _, match := range matches {
		if url := strings.TrimSpace(match.DemoURL()); url != "" {
			urls[match.MatchID] = url
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.urls = urls
}

// remember adds one resolved match (a share code lookup) to the snapshot.
func (s *steamMatchSnapshot) remember(match steam.MatchItem) {
	url := strings.TrimSpace(match.DemoURL())
	if url == "" {
		return
	}
	s.rememberURL(match.MatchID, url)
}

func (s *steamMatchSnapshot) rememberURL(matchID, demoURL string) {
	matchID = strings.TrimSpace(matchID)
	demoURL = strings.TrimSpace(demoURL)
	if matchID == "" || demoURL == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.urls == nil {
		s.urls = make(map[string]string)
	}
	s.urls[matchID] = demoURL
}

func (s *steamMatchSnapshot) lookup(matchID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	url, ok := s.urls[strings.TrimSpace(matchID)]
	return url, ok
}

// steamSnapshot returns the snapshot of the installed workspace, creating it on
// first use. It is replaced with the workspace identity, so a removed workspace
// can never contribute match URLs to the next one.
func (a *App) steamSnapshot() *steamMatchSnapshot {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.steamMatches == nil {
		a.steamMatches = newSteamMatchSnapshot()
	}
	return a.steamMatches
}

// ListSteamRecentMatches returns the official matchmaking matches of the
// signed-in Steam account.
//
// A running CS2 owns the Game Coordinator connection of the account, so the
// request is not even attempted: it would only time out. The list is read-only
// (no managed files are touched), which keeps directory clearing unblocked.
func (a *App) ListSteamRecentMatches() (*steam.MatchListResult, error) {
	if pids, err := listCS2PIDs(); err == nil && len(pids) > 0 {
		return &steam.MatchListResult{Status: steam.ClientCS2Running, Matches: make([]steam.MatchItem, 0)}, nil
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	result, err := steam.ListRecentMatches(context.Background(), steam.ListOptions{
		SteamAPIDirs: steamAPIDirsFromConfig(cfg),
	})
	if err != nil {
		return nil, err
	}
	if result != nil && result.Status == steam.ClientReady {
		a.steamSnapshot().replace(result.Matches)
	}
	return result, nil
}

// ImportSteamMatch downloads one official match demo and returns its managed
// path. The demo URL comes from the last list response when available;
// otherwise a share code is resolved through the Game Coordinator.
func (a *App) ImportSteamMatch(request SteamImportRequest) ([]string, error) {
	raw := strings.TrimSpace(request.ShareCode)
	if raw == "" {
		raw = strings.TrimSpace(request.MatchID)
	}
	ref, err := steam.ParseMatchRef(raw)
	if err != nil {
		return nil, err
	}

	// The workspace reservation precedes validation of the download target so a
	// directory clear cannot be bypassed, and it covers the whole shared task.
	workCtx, releaseFiles, dataDir, fileErr := a.beginManagedWorkspaceTaskUse()
	if fileErr != nil {
		return nil, fileErr
	}
	defer releaseFiles()

	demoURL, err := a.resolveSteamDemoURL(workCtx, ref)
	if err != nil {
		return nil, err
	}

	cacheRoot := filepath.Join(dataDir, "demo", "steam", ref.MatchID)
	progressID := steam.ProgressComponentID(ref.MatchID)

	stablePath, _, err := a.importCoordinator().do(
		workCtx,
		workCtx,
		platformImportKey{platform: platformImportSteam, matchID: ref.MatchID},
		func(runCtx context.Context) (string, error) {
			return steam.ImportDemoContext(runCtx, demoURL, ref.MatchID, cacheRoot,
				func(active bool, percent float64, indeterminate bool) {
					a.emitSteamDownloadProgress(progressID, active, percent, indeterminate)
				})
		},
	)
	if err != nil {
		return nil, err
	}
	return []string{stablePath}, nil
}

// resolveSteamDemoURL prefers the demo URL of the last list response and falls
// back to a share code lookup.
func (a *App) resolveSteamDemoURL(ctx context.Context, ref steam.MatchRef) (string, error) {
	snapshot := a.steamSnapshot()
	if demoURL, ok := snapshot.lookup(ref.MatchID); ok {
		return demoURL, nil
	}
	if ref.ShareCode == nil {
		return "", fmt.Errorf("未找到该比赛，请先刷新列表，或粘贴比赛分享码后重试")
	}
	// Resolving a share code needs the Game Coordinator, which CS2 owns while
	// it runs. A snapshot hit above needs no coordinator at all.
	if pids, err := listCS2PIDs(); err == nil && len(pids) > 0 {
		return "", fmt.Errorf("CS2 正在运行，无法向 Game Coordinator 查询分享码，请退出 CS2 后重试")
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return "", err
	}
	match, err := steam.FetchMatch(ctx, steam.ListOptions{SteamAPIDirs: steamAPIDirsFromConfig(cfg)}, *ref.ShareCode)
	if err != nil {
		return "", err
	}
	snapshot.remember(*match)
	return match.DemoURL(), nil
}

// steamAPIDirsFromConfig lists the directories searched for the user's local
// steam_api64.dll. CS2 ships it next to cs2.exe, so the configured executable
// directory is the only candidate: no Valve binary is bundled with this tool.
func steamAPIDirsFromConfig(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	cs2Exe := config.CleanPath(cfg.CS2Exe)
	if cs2Exe == "" {
		return nil
	}
	return []string{filepath.Dir(cs2Exe)}
}

func (a *App) emitSteamDownloadProgress(componentID string, active bool, percent float64, indeterminate bool) {
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
