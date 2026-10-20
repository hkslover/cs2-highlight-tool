package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"cs2-highlight-tool-v2/internal/config"
)

// steamDemoFixture is the bz2 fixture shared with the steam package tests: a
// tiny compressed payload whose first eight bytes are the CS2 demo stamp.
func steamDemoFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "steam", "testdata", "sample.dem.bz2"))
	if err != nil {
		t.Fatalf("read steam demo fixture: %v", err)
	}
	return raw
}

func TestImportSteamMatchUsesSnapshotDemoURL(t *testing.T) {
	fixture := steamDemoFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(fixture)))
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	app := newTestAppInTempDir(t)
	const matchID = "3844358149997854864"
	// A listed match: the demo URL is already known, so no Game Coordinator
	// round trip (and therefore no running Steam client) is needed.
	app.steamSnapshot().rememberURL(matchID, server.URL+"/730/match.dem.bz2")

	paths, err := app.ImportSteamMatch(SteamImportRequest{MatchID: matchID})
	if err != nil {
		t.Fatalf("ImportSteamMatch returned error: %v", err)
	}
	want := filepath.Join(app.exeDir, "demo", "steam", matchID, matchID+".dem")
	if len(paths) != 1 || paths[0] != want {
		t.Fatalf("result = %v, want [%q]", paths, want)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat imported demo: %v", err)
	}
	if info.Size() != 8+4096 {
		t.Errorf("imported demo size = %d, want %d", info.Size(), 8+4096)
	}

	// A second import must reuse the cached demo instead of downloading again.
	server.Close()
	again, err := app.ImportSteamMatch(SteamImportRequest{MatchID: matchID})
	if err != nil {
		t.Fatalf("cached ImportSteamMatch returned error: %v", err)
	}
	if len(again) != 1 || again[0] != want {
		t.Fatalf("cached result = %v, want [%q]", again, want)
	}
}

func TestImportSteamMatchAcceptsShareCodeInMatchIDField(t *testing.T) {
	// Without a cached URL the import needs the Game Coordinator, which is not
	// available in a unit test, so this only checks the input normalization: an
	// unknown match with a share code must fail with the lookup error rather
	// than the "refresh the list" error.
	app := newTestAppInTempDir(t)
	_, err := app.ImportSteamMatch(SteamImportRequest{MatchID: "CSGO-3MD4K-ShNFS-p3t6a-dxQJH-f9hWA"})
	if err == nil {
		t.Fatal("ImportSteamMatch accepted a share code without a Game Coordinator")
	}
	if err.Error() == "未找到该比赛，请先刷新列表，或粘贴比赛分享码后重试" {
		t.Fatalf("share code input was not recognized as a share code: %v", err)
	}
}

func TestImportSteamMatchRejectsUnknownMatchWithoutShareCode(t *testing.T) {
	app := newTestAppInTempDir(t)
	_, err := app.ImportSteamMatch(SteamImportRequest{MatchID: "123"})
	if err == nil {
		t.Fatal("ImportSteamMatch accepted a match that was never listed")
	}
	if got := err.Error(); got != "未找到该比赛，请先刷新列表，或粘贴比赛分享码后重试" {
		t.Fatalf("error = %q", got)
	}
}

func TestImportSteamMatchRejectsEmptyRequest(t *testing.T) {
	app := newTestAppInTempDir(t)
	if _, err := app.ImportSteamMatch(SteamImportRequest{}); err == nil {
		t.Fatal("ImportSteamMatch accepted an empty request")
	}
}

func TestSteamMatchSnapshotReplaceAndLookup(t *testing.T) {
	snapshot := newSteamMatchSnapshot()
	snapshot.rememberURL("1", "http://example.invalid/1.dem.bz2")
	if url, ok := snapshot.lookup("1"); !ok || url != "http://example.invalid/1.dem.bz2" {
		t.Fatalf("lookup = %q, %v", url, ok)
	}
	if _, ok := snapshot.lookup("2"); ok {
		t.Fatal("lookup found an unknown match")
	}

	// A fresh list replaces the snapshot: URLs of matches that are no longer
	// listed must not linger.
	snapshot.replace(nil)
	if _, ok := snapshot.lookup("1"); ok {
		t.Fatal("replace kept a stale demo URL")
	}

	// Empty URLs are never cached.
	snapshot.rememberURL("3", "   ")
	if _, ok := snapshot.lookup("3"); ok {
		t.Fatal("rememberURL cached an empty URL")
	}
}

func TestSteamAPIDirsFromConfig(t *testing.T) {
	if dirs := steamAPIDirsFromConfig(nil); len(dirs) != 0 {
		t.Fatalf("nil config produced %v", dirs)
	}
	if dirs := steamAPIDirsFromConfig(&config.Config{}); len(dirs) != 0 {
		t.Fatalf("empty config produced %v", dirs)
	}

	cfg := &config.Config{CS2Exe: filepath.Join("D:", "Steam", "cs2", "game", "bin", "win64", "cs2.exe")}
	dirs := steamAPIDirsFromConfig(cfg)
	want := filepath.Join("D:", "Steam", "cs2", "game", "bin", "win64")
	if len(dirs) != 1 || dirs[0] != want {
		t.Fatalf("dirs = %v, want [%q]", dirs, want)
	}
}
