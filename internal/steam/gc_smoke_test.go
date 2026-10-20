package steam

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// gCSmokeTimeout bounds one smoke test run: the Game Coordinator request itself
// retries for up to ~30s, so leave headroom for initialisation.
const gCSmokeTimeout = 90 * time.Second

// The smoke tests below talk to the real Steam client through the local
// steam_api64.dll. They are skipped unless CS2_GC_SMOKE=1 is set, because they
// need:
//
//   - a running Steam client signed in with an account that owns CS2,
//   - CS2 itself NOT running (the Game Coordinator accepts one connection per
//     account, so a running game makes the request time out),
//   - the CS2 installation directory so the local steam_api64.dll is found.
//
// Run them on Windows with:
//
//	$env:CS2_GC_SMOKE="1"
//	$env:CS2_DIR="C:\Program Files (x86)\Steam\steamapps\common\Counter-Strike Global Offensive\game\bin\win64"
//	go test ./internal/steam -run TestSmoke -v
//
// CS2_DIR may point at the directory holding steam_api64.dll or directly at
// cs2.exe.

func smokeSteamAPIDirs(t *testing.T) []string {
	t.Helper()

	candidates := []string{os.Getenv("CS2_DIR"), os.Getenv("CS2_EXE")}
	dirs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		trimmed := filepath.Clean(candidate)
		if candidate == "" || trimmed == "." {
			continue
		}
		if info, err := os.Stat(trimmed); err == nil && !info.IsDir() {
			// A file was given (typically cs2.exe): search its directory.
			trimmed = filepath.Dir(trimmed)
		}
		dirs = append(dirs, trimmed)
	}
	if len(dirs) == 0 {
		t.Fatalf("set CS2_DIR (the directory holding %s) or CS2_EXE (the cs2.exe path)", steamAPIFileName)
	}
	return dirs
}

func TestSmokeRecentMatches(t *testing.T) {
	if os.Getenv("CS2_GC_SMOKE") == "" {
		t.Skip("CS2_GC_SMOKE is not set")
	}
	dirs := smokeSteamAPIDirs(t)
	t.Logf("searched directories: %v", dirs)
	if candidates := steamAPICandidatePaths(dirs); len(candidates) == 0 {
		t.Fatalf("no %s found in %v; pass the directory that really contains it", steamAPIFileName, dirs)
	} else {
		t.Logf("candidate library: %v", candidates)
	}

	ctx, cancel := context.WithTimeout(context.Background(), gCSmokeTimeout)
	defer cancel()

	result, err := ListRecentMatches(ctx, ListOptions{SteamAPIDirs: dirs})
	if err != nil {
		t.Fatalf("ListRecentMatches returned error: %v", err)
	}
	t.Logf("status=%s steam_id=%s matches=%d (raw=%d skipped=%d downloadable=%d)",
		result.Status, result.SteamID, len(result.Matches),
		result.RawMatchCount, result.SkippedMatchCount, result.DownloadableMatchCount)
	for _, match := range result.Matches {
		t.Logf("  %s | %s %s | %s | %d:%d %s | kda %d/%d/%d | %ds | ends %s | demo=%v | %s",
			match.MatchID, match.GameMode, match.MapName, match.ShareCode,
			match.Score1, match.Score2, match.Result(),
			match.Kill, match.Death, match.Assist,
			match.DurationSeconds, match.EndTime, match.DemoAvailable, match.DemoName())
	}

	switch result.Status {
	case ClientReady:
		switch {
		case result.RawMatchCount == 0:
			t.Log("the Game Coordinator returned an empty match list: this account has no recent official matchmaking game")
		case len(result.Matches) == 0:
			t.Logf("the Game Coordinator returned %d matches but none was usable; report the raw counts above", result.RawMatchCount)
		}
	case ClientNotRunning, ClientNotLoggedIn, ClientAPIUnavailable, ClientCS2Running, ClientGCUnavailable:
		t.Fatalf("Game Coordinator not reachable: status=%s (quit CS2 and make sure Steam is running and signed in)", result.Status)
	default:
		t.Fatalf("unexpected status: %s", result.Status)
	}
}

// TestSmokeShareCodeImport resolves one match from a share code, which is the
// path used when the user pastes a code instead of picking a list entry. Set
// CS2_GC_SHARE_CODE to a share code of an account that owns CS2.
func TestSmokeShareCodeImport(t *testing.T) {
	if os.Getenv("CS2_GC_SMOKE") == "" {
		t.Skip("CS2_GC_SMOKE is not set")
	}
	raw := os.Getenv("CS2_GC_SHARE_CODE")
	if raw == "" {
		t.Skip("CS2_GC_SHARE_CODE is not set")
	}
	dirs := smokeSteamAPIDirs(t)

	shareCode, err := ParseShareCode(raw)
	if err != nil {
		t.Fatalf("ParseShareCode(%q) returned error: %v", raw, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), gCSmokeTimeout)
	defer cancel()

	item, err := FetchMatch(ctx, ListOptions{SteamAPIDirs: dirs}, shareCode)
	if err != nil {
		t.Fatalf("FetchMatch returned error: %v", err)
	}
	t.Logf("match=%s sharecode=%s demo=%s map=%s available=%v url=%s",
		item.MatchID, item.ShareCode, item.DemoName(), item.MapName, item.DemoAvailable, item.DemoURL())
}
