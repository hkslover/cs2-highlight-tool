package envsetup

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/release"
)

func seedHLAEReleaseSnapshotForTest(svc *Service, asset release.Asset) {
	svc.mu.Lock()
	svc.releaseSnapshot = &release.UnifiedLatest{
		Components: map[string]map[string][]release.ComponentCandidate{
			release.ComponentKeyHLAE: {
				release.SourceGitHub: {
					{
						Repo:   "advancedfx/advancedfx",
						Source: release.SourceGitHub,
						OK:     true,
						Info: &release.Info{
							TagName: "v2.0.0",
							Repo:    "advancedfx/advancedfx",
							Source:  release.SourceGitHub,
							Assets:  []release.Asset{asset},
						},
					},
				},
			},
		},
	}
	svc.state.SourceStep.Source = string(defaultDownloadSource())
	svc.mu.Unlock()
}

func TestCollectReleaseAssetCandidates_CNUsesMirrorThenURL(t *testing.T) {
	svc := New(t.TempDir(), "1.0.0")
	svc.Startup(nil)
	seedHLAEReleaseSnapshotForTest(svc, release.Asset{
		Name:        "hlae_2_0_0.zip",
		DownloadURL: "https://files.example.com/url.zip",
		URL:         "https://files.example.com/url.zip",
		GitHubURL:   "https://files.example.com/github.zip",
		MirrorURL:   "https://files.example.com/mirror.zip",
	})
	svc.mu.Lock()
	svc.state.SourceStep.CountryCode = "CN"
	svc.mu.Unlock()

	candidates, err := svc.collectReleaseAssetCandidates(componentHLAE, defaultDownloadSource(), release.SelectHLAEAsset)
	if err != nil {
		t.Fatalf("collectReleaseAssetCandidates error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}
	if candidates[0].URLKind != urlKindMirror || candidates[0].AssetURL != "https://files.example.com/mirror.zip" {
		t.Fatalf("candidate[0] = %#v", candidates[0])
	}
	if candidates[1].URLKind != urlKindDirect || candidates[1].AssetURL != "https://files.example.com/url.zip" {
		t.Fatalf("candidate[1] = %#v", candidates[1])
	}
}

func TestCollectReleaseAssetCandidates_UnknownCountryUsesMirrorThenURL(t *testing.T) {
	svc := New(t.TempDir(), "1.0.0")
	svc.Startup(nil)
	seedHLAEReleaseSnapshotForTest(svc, release.Asset{
		Name:        "hlae_2_0_0.zip",
		DownloadURL: "https://files.example.com/url.zip",
		URL:         "https://files.example.com/url.zip",
		GitHubURL:   "https://files.example.com/github.zip",
		MirrorURL:   "https://files.example.com/mirror.zip",
	})
	svc.mu.Lock()
	svc.state.SourceStep.CountryCode = ""
	svc.mu.Unlock()

	candidates, err := svc.collectReleaseAssetCandidates(componentHLAE, defaultDownloadSource(), release.SelectHLAEAsset)
	if err != nil {
		t.Fatalf("collectReleaseAssetCandidates error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}
	if candidates[0].URLKind != urlKindMirror || candidates[0].AssetURL != "https://files.example.com/mirror.zip" {
		t.Fatalf("candidate[0] = %#v", candidates[0])
	}
	if candidates[1].URLKind != urlKindDirect || candidates[1].AssetURL != "https://files.example.com/url.zip" {
		t.Fatalf("candidate[1] = %#v", candidates[1])
	}
}

func TestCollectReleaseAssetCandidates_OutsideCNUsesGitHubOnly(t *testing.T) {
	svc := New(t.TempDir(), "1.0.0")
	svc.Startup(nil)
	seedHLAEReleaseSnapshotForTest(svc, release.Asset{
		Name:        "hlae_2_0_0.zip",
		DownloadURL: "https://files.example.com/url.zip",
		URL:         "https://files.example.com/url.zip",
		GitHubURL:   "https://files.example.com/github.zip",
		MirrorURL:   "https://files.example.com/mirror.zip",
	})
	svc.mu.Lock()
	svc.state.SourceStep.CountryCode = "US"
	svc.mu.Unlock()

	candidates, err := svc.collectReleaseAssetCandidates(componentHLAE, defaultDownloadSource(), release.SelectHLAEAsset)
	if err != nil {
		t.Fatalf("collectReleaseAssetCandidates error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(candidates))
	}
	if candidates[0].URLKind != urlKindGitHub || candidates[0].AssetURL != "https://files.example.com/github.zip" {
		t.Fatalf("candidate = %#v", candidates[0])
	}
}

func TestCollectReleaseAssetCandidates_OutsideCNMissingGitHubURLFails(t *testing.T) {
	svc := New(t.TempDir(), "1.0.0")
	svc.Startup(nil)
	seedHLAEReleaseSnapshotForTest(svc, release.Asset{
		Name:        "hlae_2_0_0.zip",
		DownloadURL: "https://files.example.com/url.zip",
		URL:         "https://files.example.com/url.zip",
		MirrorURL:   "https://files.example.com/mirror.zip",
	})
	svc.mu.Lock()
	svc.state.SourceStep.CountryCode = "US"
	svc.mu.Unlock()

	_, err := svc.collectReleaseAssetCandidates(componentHLAE, defaultDownloadSource(), release.SelectHLAEAsset)
	if err == nil {
		t.Fatal("collectReleaseAssetCandidates error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "github_url") {
		t.Fatalf("error = %v, want github_url hint", err)
	}
}

func TestDownloadAndInstallWithFallback_CNDoesNotAttemptGitHubURL(t *testing.T) {
	urlHits := 0
	mirrorHits := 0
	githubHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/url.zip":
			urlHits++
		case "/mirror.zip":
			mirrorHits++
		case "/github.zip":
			githubHits++
		default:
			http.NotFound(w, r)
			return
		}
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer server.Close()

	svc := New(t.TempDir(), "1.0.0")
	svc.Startup(nil)
	seedHLAEReleaseSnapshotForTest(svc, release.Asset{
		Name:        "hlae_2_0_0.zip",
		DownloadURL: server.URL + "/url.zip",
		URL:         server.URL + "/url.zip",
		GitHubURL:   server.URL + "/github.zip",
		MirrorURL:   server.URL + "/mirror.zip",
	})
	svc.mu.Lock()
	svc.state.SourceStep.CountryCode = "CN"
	svc.mu.Unlock()

	candidates, err := svc.collectReleaseAssetCandidates(componentHLAE, defaultDownloadSource(), release.SelectHLAEAsset)
	if err != nil {
		t.Fatalf("collectReleaseAssetCandidates error: %v", err)
	}

	err = svc.downloadAndInstallWithFallback(componentHLAE, "v2.0.0", candidates, func(path string) error {
		return nil
	})
	if err == nil {
		t.Fatal("downloadAndInstallWithFallback error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "(url)") || !strings.Contains(err.Error(), "(mirror_url)") {
		t.Fatalf("error = %v, want url kind tags", err)
	}
	if strings.Contains(err.Error(), "github_url") {
		t.Fatalf("error = %v, should not include github_url attempt", err)
	}
	if urlHits != 1 || mirrorHits != 1 || githubHits != 0 {
		t.Fatalf("hits url=%d mirror=%d github=%d", urlHits, mirrorHits, githubHits)
	}
}

func TestDownloadAndInstallWithFallback_RaceUsesFasterCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow.zip":
			// 声明一个远大于实际写入量的长度，确保慢链路不会因为提前 EOF 而胜出。
			w.Header().Set("Content-Length", "1048576")
			_, _ = w.Write([]byte("slow"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-r.Context().Done():
			case <-time.After(3 * time.Second):
			}
		case "/fast.zip":
			_, _ = w.Write([]byte("fast-payload"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := New(t.TempDir(), "1.0.0")
	svc.Startup(nil)
	candidates := []releaseAssetCandidate{
		{
			Source:   DownloadSourceGitHub,
			Asset:    release.Asset{Name: "hlae_2_0_0.zip"},
			AssetURL: server.URL + "/slow.zip",
			URLKind:  urlKindMirror,
		},
		{
			Source:   DownloadSourceGitHub,
			Asset:    release.Asset{Name: "hlae_2_0_0.zip"},
			AssetURL: server.URL + "/fast.zip",
			URLKind:  urlKindDirect,
		},
	}

	var installedPath string
	var installedContent string
	err := svc.downloadAndInstallWithFallback(componentHLAE, "v2.0.0", candidates, func(path string) error {
		installedPath = path
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		installedContent = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("downloadAndInstallWithFallback error: %v", err)
	}
	if !strings.HasSuffix(installedPath, "_"+urlKindDirect+".zip") {
		t.Fatalf("installed path = %q, want fast direct candidate", installedPath)
	}
	if installedContent != "fast-payload" {
		t.Fatalf("installed content = %q, want fast-payload", installedContent)
	}

	entries, err := os.ReadDir(filepath.Join(svc.dataDir, "temp"))
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(installedPath) {
		t.Fatalf("temp entries = %#v, want only winner %s", entries, filepath.Base(installedPath))
	}
}

func TestDownloadAndInstallWithFallback_RaceFallsBackWhenWinnerInstallFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mirror.zip":
			_, _ = w.Write([]byte("mirror-payload"))
		case "/url.zip":
			_, _ = w.Write([]byte("url-payload"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := New(t.TempDir(), "1.0.0")
	svc.Startup(nil)
	candidates := []releaseAssetCandidate{
		{
			Source:   DownloadSourceGitHub,
			Asset:    release.Asset{Name: "hlae_2_0_0.zip"},
			AssetURL: server.URL + "/mirror.zip",
			URLKind:  urlKindMirror,
		},
		{
			Source:   DownloadSourceGitHub,
			Asset:    release.Asset{Name: "hlae_2_0_0.zip"},
			AssetURL: server.URL + "/url.zip",
			URLKind:  urlKindDirect,
		},
	}

	installPaths := make([]string, 0, 2)
	installedContent := ""
	err := svc.downloadAndInstallWithFallback(componentHLAE, "v2.0.0", candidates, func(path string) error {
		installPaths = append(installPaths, path)
		if len(installPaths) == 1 {
			return errors.New("模拟安装失败")
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		installedContent = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("downloadAndInstallWithFallback error: %v", err)
	}
	if len(installPaths) != 2 {
		t.Fatalf("install calls = %d, want 2", len(installPaths))
	}
	if base := filepath.Base(installPaths[1]); base != "hlae_v2.0.0.zip" {
		t.Fatalf("fallback install path = %q, want unsuffixed temp path", installPaths[1])
	}
	if installedContent != "mirror-payload" && installedContent != "url-payload" {
		t.Fatalf("installed content = %q, want one of the candidate payloads", installedContent)
	}
}

func TestAwaitRaceOutcome_UserCancelWinsOverQueuedSuccess(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	results := make(chan downloadRaceResult, 1)
	results <- downloadRaceResult{index: 0}
	cancel(errDownloadCanceledByUser)

	outcome := awaitRaceOutcome(ctx, results, []releaseAssetCandidate{
		{Source: DownloadSourceGitHub, URLKind: urlKindDirect},
	})
	if !outcome.canceled {
		t.Fatalf("outcome = %#v, want canceled", outcome)
	}
	if outcome.winner != -1 {
		t.Fatalf("winner = %d, want -1", outcome.winner)
	}
}

func TestAwaitRaceOutcome_ReturnsQueuedSuccess(t *testing.T) {
	ctx := context.Background()
	results := make(chan downloadRaceResult, 1)
	results <- downloadRaceResult{index: 1}

	outcome := awaitRaceOutcome(ctx, results, []releaseAssetCandidate{
		{Source: DownloadSourceGitHub, URLKind: urlKindDirect},
		{Source: DownloadSourceGitHub, URLKind: urlKindMirror},
	})
	if outcome.canceled {
		t.Fatalf("outcome = %#v, want not canceled", outcome)
	}
	if outcome.winner != 1 {
		t.Fatalf("winner = %d, want 1", outcome.winner)
	}
}
