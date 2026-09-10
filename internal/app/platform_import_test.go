package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/download"
	"cs2-highlight-tool-v2/internal/envsetup"
	"cs2-highlight-tool-v2/internal/fivee"
	"cs2-highlight-tool-v2/internal/wanmei"
)

const (
	testImportFiveEMatchID   = "g161-20260427162329954189731"
	testImportFiveEMatchIDB  = "g161-20260427162329954189732"
	testImportWanmeiMatchID  = "9208138716569380236"
	testImportWanmeiMatchIDB = "9208138716569380237"
	testImportDemoContent    = "PBDEMS2\x00shared-import"
)

type platformImportCase struct {
	name       string
	platform   platformImportPlatform
	cacheDir   string
	matchA     string
	matchB     string
	importCall func(a *App, matchID string) ([]string, error)
}

func platformImportCases() []platformImportCase {
	return []platformImportCase{
		{
			name:     "fivee",
			platform: platformImportFiveE,
			cacheDir: "5e",
			matchA:   testImportFiveEMatchID,
			matchB:   testImportFiveEMatchIDB,
			importCall: func(a *App, matchID string) ([]string, error) {
				return a.ImportFiveEMatch(matchID)
			},
		},
		{
			name:     "wanmei",
			platform: platformImportWanmei,
			cacheDir: "wanmei",
			matchA:   testImportWanmeiMatchID,
			matchB:   testImportWanmeiMatchIDB,
			importCall: func(a *App, matchID string) ([]string, error) {
				return a.ImportWanmeiMatch("PVP@" + matchID)
			},
		},
	}
}

func (tc platformImportCase) cachePath(dataDir string, matchID string) string {
	return filepath.Join(dataDir, "demo", tc.cacheDir, matchID, matchID+".dem")
}

// stubPlatformImportSeams installs deterministic platform seams for one test
// and restores every package seam on cleanup. archiveURL is the URL handed to
// the transfer layer; when empty each request returns a URL that embeds the
// requested match ID so a stub can tell parallel matches apart. A nil
// downloadFn selects the built-in download (used by the cancellation test).
func stubPlatformImportSeams(
	t *testing.T,
	platform platformImportPlatform,
	archiveURL string,
	downloadFn func(url string, targetPath string, emitProgress download.ProgressFunc) error,
	demContent []byte,
) {
	t.Helper()
	unzipFn := func(ctx context.Context, archivePath string, destDir string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(destDir, "inner.dem"), demContent, 0o644)
	}

	switch platform {
	case platformImportFiveE:
		oldReq := fivee.HTTPRequestFn
		oldDownload := fivee.DownloadFileFn
		oldUnzip := fivee.UnzipFn
		oldFind := fivee.FindFirstByExtFn
		oldCopy := fivee.CopyFileFn

		fivee.HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
			demoURL := strings.TrimSpace(archiveURL)
			if demoURL == "" {
				demoURL = "https://example.com/" + path.Base(req.URL.Path) + "_de_dust2.zip"
			}
			return stubFiveETestResponse(http.StatusOK, `{"data":{"main":{"demo_url":"`+demoURL+`"}}}`), nil
		}
		fivee.DownloadFileFn = downloadFn
		fivee.UnzipFn = unzipFn
		fivee.FindFirstByExtFn = download.FindFirstByExt
		fivee.CopyFileFn = download.CopyFile
		t.Cleanup(func() {
			fivee.HTTPRequestFn = oldReq
			fivee.DownloadFileFn = oldDownload
			fivee.UnzipFn = oldUnzip
			fivee.FindFirstByExtFn = oldFind
			fivee.CopyFileFn = oldCopy
		})
	case platformImportWanmei:
		oldReq := wanmei.HTTPRequestFn
		oldResolve := wanmei.OSSResolveHTTPDoFn
		oldDownload := wanmei.DownloadFileFn
		oldUnzip := wanmei.UnzipFn
		oldFind := wanmei.FindFirstByExtFn
		oldCopy := wanmei.CopyFileFn

		encodedToken := encodeWanmeiLogForTest("demo_token_1711111111_76561198051245123")
		wanmei.HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
			switch req.URL.String() {
			case "http://127.0.0.1:55555/":
				return stubWanmeiTestResponse(http.StatusOK, fmt.Sprintf(`{"nickname":"alice","token":"%s"}`, encodedToken)), nil
			case "https://api-ipv4.ip.sb/ip":
				return stubWanmeiTestResponse(http.StatusOK, "1.2.3.4"), nil
			default:
				return nil, fmt.Errorf("unexpected wanmei HTTP request: %s", req.URL.String())
			}
		}
		wanmei.OSSResolveHTTPDoFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
			location := strings.TrimSpace(archiveURL)
			if location == "" {
				location = "https://oss.example.com/" + req.URL.Query().Get("match_id") + "_0.zip"
			}
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{location}},
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}
		wanmei.DownloadFileFn = downloadFn
		wanmei.UnzipFn = unzipFn
		wanmei.FindFirstByExtFn = download.FindFirstByExt
		wanmei.CopyFileFn = download.CopyFile
		t.Cleanup(func() {
			wanmei.HTTPRequestFn = oldReq
			wanmei.OSSResolveHTTPDoFn = oldResolve
			wanmei.DownloadFileFn = oldDownload
			wanmei.UnzipFn = oldUnzip
			wanmei.FindFirstByExtFn = oldFind
			wanmei.CopyFileFn = oldCopy
		})
	default:
		t.Fatalf("unknown platform %q", platform)
	}
}

func assertNoImportStagingLeftovers(t *testing.T, cacheRoot string) {
	t.Helper()
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		t.Fatalf("read cache root %q: %v", cacheRoot, err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".5e-import-") || strings.HasPrefix(entry.Name(), ".wanmei-import-") {
			t.Fatalf("import staging directory was not cleaned up: %s", entry.Name())
		}
	}
}

func TestImportSameMatchSharesOneDownload(t *testing.T) {
	for _, tc := range platformImportCases() {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			a := newWorkspaceLifecycleTestApp(t, dataDir)

			var downloadCalls int32
			downloadStarted := make(chan struct{})
			releaseDownload := make(chan struct{})
			stubPlatformImportSeams(t, tc.platform, "", func(url string, targetPath string, emitProgress download.ProgressFunc) error {
				if atomic.AddInt32(&downloadCalls, 1) == 1 {
					close(downloadStarted)
				}
				<-releaseDownload
				return os.WriteFile(targetPath, []byte("zip"), 0o644)
			}, []byte(testImportDemoContent))

			type outcome struct {
				paths []string
				err   error
			}
			results := make(chan outcome, 2)
			startImport := func() {
				go func() {
					paths, err := tc.importCall(a, tc.matchA)
					results <- outcome{paths: paths, err: err}
				}()
			}

			startImport()
			select {
			case <-downloadStarted:
			case <-time.After(5 * time.Second):
				t.Fatal("first import never started its download")
			}
			startImport()
			// The shared task is already inside the download, so the second
			// caller can only be waiting for the same result.
			waitForImportWaiters(t, a.importCoordinator(), platformImportKey{platform: tc.platform, matchID: tc.matchA}, 1)
			close(releaseDownload)

			wantPath := tc.cachePath(dataDir, tc.matchA)
			for i := 0; i < 2; i++ {
				select {
				case result := <-results:
					if result.err != nil {
						t.Fatalf("import error: %v", result.err)
					}
					if len(result.paths) != 1 || result.paths[0] != wantPath {
						t.Fatalf("import paths = %v, want [%q]", result.paths, wantPath)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("import did not return")
				}
			}
			if got := atomic.LoadInt32(&downloadCalls); got != 1 {
				t.Fatalf("download calls = %d, want 1 shared download", got)
			}
			content, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("read cached demo: %v", err)
			}
			if string(content) != testImportDemoContent {
				t.Fatalf("cached demo content = %q, want %q", string(content), testImportDemoContent)
			}
			assertNoImportStagingLeftovers(t, filepath.Dir(wantPath))
		})
	}
}

func TestImportFailureCanBeRetried(t *testing.T) {
	for _, tc := range platformImportCases() {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			a := newWorkspaceLifecycleTestApp(t, dataDir)

			var downloadCalls int32
			boom := errors.New("injected download failure")
			stubPlatformImportSeams(t, tc.platform, "", func(url string, targetPath string, emitProgress download.ProgressFunc) error {
				if atomic.AddInt32(&downloadCalls, 1) == 1 {
					return boom
				}
				return os.WriteFile(targetPath, []byte("zip"), 0o644)
			}, []byte(testImportDemoContent))

			if _, err := tc.importCall(a, tc.matchA); err == nil {
				t.Fatal("first import should fail")
			}
			wantPath := tc.cachePath(dataDir, tc.matchA)
			if _, err := os.Stat(wantPath); !os.IsNotExist(err) {
				t.Fatalf("failed import left a cache file, stat err=%v", err)
			}
			assertNoImportStagingLeftovers(t, filepath.Dir(wantPath))

			paths, err := tc.importCall(a, tc.matchA)
			if err != nil {
				t.Fatalf("retry import error: %v", err)
			}
			if len(paths) != 1 || paths[0] != wantPath {
				t.Fatalf("retry paths = %v, want [%q]", paths, wantPath)
			}
			if got := atomic.LoadInt32(&downloadCalls); got != 2 {
				t.Fatalf("download calls = %d, want 2 (failures must not be cached)", got)
			}
			content, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("read retried demo: %v", err)
			}
			if string(content) != testImportDemoContent {
				t.Fatalf("retried demo content = %q, want %q", string(content), testImportDemoContent)
			}
		})
	}
}

func TestImportDifferentMatchesRunInParallel(t *testing.T) {
	for _, tc := range platformImportCases() {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			a := newWorkspaceLifecycleTestApp(t, dataDir)

			entered := make(chan string, 2)
			release := make(chan struct{})
			stubPlatformImportSeams(t, tc.platform, "", func(url string, targetPath string, emitProgress download.ProgressFunc) error {
				entered <- filepath.Dir(targetPath)
				<-release
				return os.WriteFile(targetPath, []byte("zip"), 0o644)
			}, []byte(testImportDemoContent))

			type outcome struct {
				matchID string
				paths   []string
				err     error
			}
			results := make(chan outcome, 2)
			for _, matchID := range []string{tc.matchA, tc.matchB} {
				matchID := matchID
				go func() {
					paths, err := tc.importCall(a, matchID)
					results <- outcome{matchID: matchID, paths: paths, err: err}
				}()
			}

			stagingDirs := make(map[string]struct{}, 2)
			for i := 0; i < 2; i++ {
				select {
				case dir := <-entered:
					stagingDirs[dir] = struct{}{}
				case <-time.After(5 * time.Second):
					t.Fatalf("different matches did not download in parallel, saw %v", stagingDirs)
				}
			}
			if len(stagingDirs) != 2 {
				t.Fatalf("parallel imports reused one archive/extract directory: %v", stagingDirs)
			}
			for dir := range stagingDirs {
				if _, err := os.Stat(dir); err != nil {
					t.Fatalf("staging directory %q missing while both imports run: %v", dir, err)
				}
			}
			close(release)

			for i := 0; i < 2; i++ {
				select {
				case result := <-results:
					if result.err != nil {
						t.Fatalf("import %s error: %v", result.matchID, result.err)
					}
					wantPath := tc.cachePath(dataDir, result.matchID)
					if len(result.paths) != 1 || result.paths[0] != wantPath {
						t.Fatalf("import %s paths = %v, want [%q]", result.matchID, result.paths, wantPath)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("parallel import did not return")
				}
			}
			for _, matchID := range []string{tc.matchA, tc.matchB} {
				wantPath := tc.cachePath(dataDir, matchID)
				content, err := os.ReadFile(wantPath)
				if err != nil {
					t.Fatalf("read parallel demo %s: %v", matchID, err)
				}
				if string(content) != testImportDemoContent {
					t.Fatalf("parallel demo %s content = %q", matchID, string(content))
				}
				assertNoImportStagingLeftovers(t, filepath.Dir(wantPath))
			}
		})
	}
}

func TestImportFailureDoesNotDisturbParallelImport(t *testing.T) {
	for _, tc := range platformImportCases() {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			a := newWorkspaceLifecycleTestApp(t, dataDir)

			entered := make(chan string, 2)
			release := make(chan struct{})
			boom := errors.New("injected download failure")
			stubPlatformImportSeams(t, tc.platform, "", func(url string, targetPath string, emitProgress download.ProgressFunc) error {
				entered <- filepath.Dir(targetPath)
				<-release
				if strings.Contains(filepath.Base(targetPath), tc.matchA) {
					return boom
				}
				return os.WriteFile(targetPath, []byte("zip"), 0o644)
			}, []byte(testImportDemoContent))

			type outcome struct {
				matchID string
				err     error
			}
			results := make(chan outcome, 2)
			for _, matchID := range []string{tc.matchA, tc.matchB} {
				matchID := matchID
				go func() {
					_, err := tc.importCall(a, matchID)
					results <- outcome{matchID: matchID, err: err}
				}()
			}
			for i := 0; i < 2; i++ {
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					t.Fatal("parallel downloads did not both start")
				}
			}
			close(release)

			for i := 0; i < 2; i++ {
				select {
				case result := <-results:
					if result.matchID == tc.matchA && result.err == nil {
						t.Fatal("failing import unexpectedly succeeded")
					}
					if result.matchID == tc.matchB && result.err != nil {
						t.Fatalf("sibling import failed because of the other match: %v", result.err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("parallel import did not return")
				}
			}

			failedPath := tc.cachePath(dataDir, tc.matchA)
			if _, err := os.Stat(failedPath); !os.IsNotExist(err) {
				t.Fatalf("failed import left a cache file, stat err=%v", err)
			}
			assertNoImportStagingLeftovers(t, filepath.Dir(failedPath))
			siblingPath := tc.cachePath(dataDir, tc.matchB)
			content, err := os.ReadFile(siblingPath)
			if err != nil {
				t.Fatalf("sibling demo was disturbed: %v", err)
			}
			if string(content) != testImportDemoContent {
				t.Fatalf("sibling demo content = %q, want %q", string(content), testImportDemoContent)
			}
			assertNoImportStagingLeftovers(t, filepath.Dir(siblingPath))
		})
	}
}

func TestImportAbortsWhenWorkspaceCloses(t *testing.T) {
	for _, tc := range platformImportCases() {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			a := newWorkspaceLifecycleTestApp(t, dataDir)

			var startOnce sync.Once
			downloadStarted := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "1048576")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(bytes.Repeat([]byte("d"), 4096))
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
				startOnce.Do(func() { close(downloadStarted) })
				<-r.Context().Done()
			}))
			defer server.Close()

			// nil selects the built-in context-aware download so the workspace
			// cancellation really reaches the transfer.
			stubPlatformImportSeams(t, tc.platform, server.URL+"/demo.zip", nil, []byte(testImportDemoContent))

			errCh := make(chan error, 1)
			go func() {
				_, err := tc.importCall(a, tc.matchA)
				errCh <- err
			}()
			select {
			case <-downloadStarted:
			case <-time.After(5 * time.Second):
				t.Fatal("download never started")
			}

			session := a.workspaceSnapshot().session
			if session == nil {
				t.Fatal("test app has no workspace session")
			}
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := session.close(stopCtx); err != nil {
				t.Fatalf("session.close did not wait for the import task: %v", err)
			}

			select {
			case err := <-errCh:
				if err == nil {
					t.Fatal("import succeeded after workspace close")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("import task did not stop after workspace close")
			}
			wantPath := tc.cachePath(dataDir, tc.matchA)
			if _, err := os.Stat(wantPath); !os.IsNotExist(err) {
				t.Fatalf("canceled import committed a cache file, stat err=%v", err)
			}
			assertNoImportStagingLeftovers(t, filepath.Dir(wantPath))
		})
	}
}

func TestImportAbortsWhenWorkspaceClosesDuringExtraction(t *testing.T) {
	for _, tc := range platformImportCases() {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			a := newWorkspaceLifecycleTestApp(t, dataDir)

			var startOnce sync.Once
			extractionStarted := make(chan struct{})
			blockingExtraction := func(ctx context.Context, archivePath string, destDir string) error {
				if err := os.MkdirAll(destDir, 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(destDir, "partial.dem"), []byte("partial"), 0o644); err != nil {
					return err
				}
				startOnce.Do(func() { close(extractionStarted) })
				<-ctx.Done()
				return ctx.Err()
			}

			stubPlatformImportSeams(t, tc.platform, "", func(url string, targetPath string, emitProgress download.ProgressFunc) error {
				return os.WriteFile(targetPath, []byte("zip"), 0o644)
			}, []byte(testImportDemoContent))
			// Override the extraction seam for this test only; the helper's
			// cleanup still restores the package default.
			switch tc.platform {
			case platformImportFiveE:
				fivee.UnzipFn = blockingExtraction
			case platformImportWanmei:
				wanmei.UnzipFn = blockingExtraction
			default:
				t.Fatalf("unknown platform %q", tc.platform)
			}

			errCh := make(chan error, 1)
			go func() {
				_, err := tc.importCall(a, tc.matchA)
				errCh <- err
			}()
			select {
			case <-extractionStarted:
			case <-time.After(5 * time.Second):
				t.Fatal("extraction never started")
			}

			session := a.workspaceSnapshot().session
			if session == nil {
				t.Fatal("test app has no workspace session")
			}
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := session.close(stopCtx); err != nil {
				t.Fatalf("session.close did not interrupt the extraction: %v", err)
			}

			select {
			case err := <-errCh:
				if err == nil {
					t.Fatal("import succeeded after workspace close during extraction")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("import task did not stop after workspace close")
			}
			wantPath := tc.cachePath(dataDir, tc.matchA)
			if _, err := os.Stat(wantPath); !os.IsNotExist(err) {
				t.Fatalf("canceled extraction committed a cache file, stat err=%v", err)
			}
			assertNoImportStagingLeftovers(t, filepath.Dir(wantPath))
		})
	}
}

func TestWorkspaceInstallReplacesImportCoordinator(t *testing.T) {
	dataDir := t.TempDir()
	a := newWorkspaceLifecycleTestApp(t, dataDir)
	first := a.importCoordinator()

	newRoot := t.TempDir()
	svc := envsetup.NewWithDataDir(t.TempDir(), newRoot, "test")
	a.serviceMu.Lock()
	a.installWorkspaceLocked(newRoot, svc)
	a.serviceMu.Unlock()

	if a.importCoordinator() == first {
		t.Fatal("a new workspace identity reused the previous import coordinator")
	}
}
