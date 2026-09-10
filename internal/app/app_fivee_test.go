package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/download"
	"cs2-highlight-tool-v2/internal/fivee"
)

func TestListFiveERecentMatches_PersistsPlayerName(t *testing.T) {
	oldReq := fivee.HTTPRequestFn
	fivee.HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return stubFiveETestResponse(http.StatusOK, `{
			"success": true,
			"errcode": 0,
			"message": "",
			"data": {
				"match_list": [
					{
						"match_id": "g161-20260427162329954189731",
						"map_name": "Dust2",
						"group1_all_score": "13",
						"group2_all_score": "5",
						"kill": "21",
						"death": "13",
						"assist": "5",
						"end_time": "1777280204"
					}
				]
			}
		}`), nil
	}
	t.Cleanup(func() { fivee.HTTPRequestFn = oldReq })

	app := &App{exeDir: t.TempDir()}
	result, err := app.ListFiveERecentMatches("  molim  ", 3)
	if err != nil {
		t.Fatalf("ListFiveERecentMatches() error: %v", err)
	}
	if result.PlayerName != "molim" {
		t.Fatalf("player_name = %q, want %q", result.PlayerName, "molim")
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches len = %d, want 1", len(result.Matches))
	}

	cfg, err := config.LoadOrCreate(filepath.Join(app.exeDir, "config.json"), app.exeDir)
	if err != nil {
		t.Fatalf("LoadOrCreate config failed: %v", err)
	}
	if cfg.FiveEPlayerName != "molim" {
		t.Fatalf("fivee_player_name = %q, want %q", cfg.FiveEPlayerName, "molim")
	}
}

func TestListFiveERecentMatches_ExtractsDomainFromShareLinkAndPersists(t *testing.T) {
	oldReq := fivee.HTTPRequestFn
	fivee.HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		if got := req.URL.Query().Get("domain"); got != "12139xi22eza" {
			t.Fatalf("domain query = %q, want %q", got, "12139xi22eza")
		}
		return stubFiveETestResponse(http.StatusOK, `{
			"success": true,
			"errcode": 0,
			"message": "",
			"data": {"match_list": []}
		}`), nil
	}
	t.Cleanup(func() { fivee.HTTPRequestFn = oldReq })

	app := &App{exeDir: t.TempDir()}
	input := "【5E对战平台：被窝以外皆他乡OVO的个人主页】https://csgo.5eplay.com/app/share_loding_type7?domain=12139xi22eza&tab=77&uuid=338955c1-d7bb-11f0-a93a-0c42a164bc3c"
	result, err := app.ListFiveERecentMatches(input, 1)
	if err != nil {
		t.Fatalf("ListFiveERecentMatches() error: %v", err)
	}
	if result.PlayerName != "12139xi22eza" {
		t.Fatalf("player_name = %q, want %q", result.PlayerName, "12139xi22eza")
	}

	cfg, err := config.LoadOrCreate(filepath.Join(app.exeDir, "config.json"), app.exeDir)
	if err != nil {
		t.Fatalf("LoadOrCreate config failed: %v", err)
	}
	if cfg.FiveEPlayerName != "12139xi22eza" {
		t.Fatalf("fivee_player_name = %q, want %q", cfg.FiveEPlayerName, "12139xi22eza")
	}
}

func TestListFiveERecentMatches_EmptyPlayerNameSkipsRemoteCall(t *testing.T) {
	oldReq := fivee.HTTPRequestFn
	calls := 0
	fivee.HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		calls++
		return nil, fmt.Errorf("unexpected remote call")
	}
	t.Cleanup(func() { fivee.HTTPRequestFn = oldReq })

	app := &App{exeDir: t.TempDir()}
	result, err := app.ListFiveERecentMatches("   ", 1)
	if err != nil {
		t.Fatalf("ListFiveERecentMatches() error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("remote calls = %d, want 0", calls)
	}
	if result.PlayerName != "" {
		t.Fatalf("player_name = %q, want empty", result.PlayerName)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("matches len = %d, want 0", len(result.Matches))
	}
}

func TestImportFiveEMatch_CleansUpLegacyRawDemo(t *testing.T) {
	oldReq := fivee.HTTPRequestFn
	oldDownload := fivee.DownloadFileFn
	oldUnzip := fivee.UnzipFn
	oldFind := fivee.FindFirstByExtFn
	oldCopy := fivee.CopyFileFn

	fivee.HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return stubFiveETestResponse(http.StatusOK, `{"data":{"main":{"demo_url":"https://example.com/g161-20260427162329954189731_de_dust2.zip"}}}`), nil
	}
	fivee.DownloadFileFn = func(url string, targetPath string, emitProgress download.ProgressFunc) error {
		return os.WriteFile(targetPath, []byte("zip"), 0644)
	}
	fivee.UnzipFn = func(ctx context.Context, archivePath string, destDir string) error {
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(destDir, "inner.dem"), []byte("fivee-dem"), 0644)
	}
	fivee.FindFirstByExtFn = download.FindFirstByExt
	fivee.CopyFileFn = download.CopyFile

	t.Cleanup(func() {
		fivee.HTTPRequestFn = oldReq
		fivee.DownloadFileFn = oldDownload
		fivee.UnzipFn = oldUnzip
		fivee.FindFirstByExtFn = oldFind
		fivee.CopyFileFn = oldCopy
	})

	matchID := "g161-20260427162329954189731"
	app := &App{exeDir: t.TempDir()}
	stableSourcePath := filepath.Join(app.exeDir, "demo", "5e", matchID, matchID+".dem")
	legacyRawPath := filepath.Join(app.exeDir, "demo", "raw", hashSourcePath(stableSourcePath), filepath.Base(stableSourcePath))

	if err := os.MkdirAll(filepath.Dir(legacyRawPath), 0755); err != nil {
		t.Fatalf("mkdir legacy raw dir: %v", err)
	}
	if err := os.WriteFile(legacyRawPath, []byte("legacy"), 0644); err != nil {
		t.Fatalf("write legacy raw demo: %v", err)
	}

	got, err := app.ImportFiveEMatch(matchID)
	if err != nil {
		t.Fatalf("ImportFiveEMatch() error: %v", err)
	}
	if len(got) != 1 || got[0] != stableSourcePath {
		t.Fatalf("result = %v, want [%q]", got, stableSourcePath)
	}
	if _, err := os.Stat(legacyRawPath); err == nil || !os.IsNotExist(err) {
		t.Fatalf("legacy raw path should be removed, stat err=%v", err)
	}
}

func stubFiveETestResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
