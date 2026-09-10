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

	"cs2-highlight-tool-v2/internal/download"
	"cs2-highlight-tool-v2/internal/wanmei"
)

func TestImportWanmeiMatch_CleansUpLegacyRawDemo(t *testing.T) {
	oldReq := wanmei.HTTPRequestFn
	oldResolve := wanmei.OSSResolveHTTPDoFn
	oldDownload := wanmei.DownloadFileFn
	oldUnzip := wanmei.UnzipFn
	oldFind := wanmei.FindFirstByExtFn
	oldCopy := wanmei.CopyFileFn

	token := "demo_token"
	steamID := "76561198051245123"
	serverTime := "1711111111"
	encodedToken := encodeWanmeiLogForTest(token + "_" + serverTime + "_" + steamID)

	httpCalls := 0
	wanmei.HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		httpCalls++
		switch httpCalls {
		case 1:
			return stubWanmeiTestResponse(http.StatusOK, fmt.Sprintf(`{"nickname":"alice","token":"%s"}`, encodedToken)), nil
		case 2:
			return stubWanmeiTestResponse(http.StatusOK, "1.2.3.4"), nil
		default:
			return nil, fmt.Errorf("unexpected HTTP request: %s", req.URL.String())
		}
	}
	wanmei.OSSResolveHTTPDoFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"https://oss.example.com/demo.zip"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	}
	wanmei.DownloadFileFn = func(url string, targetPath string, emitProgress download.ProgressFunc) error {
		return os.WriteFile(targetPath, []byte("zip"), 0644)
	}
	wanmei.UnzipFn = func(ctx context.Context, archivePath string, destDir string) error {
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(destDir, "inner.dem"), []byte("dem-content"), 0644)
	}
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

	matchID := "9208138716569380236"
	app := &App{exeDir: t.TempDir()}
	stableSourcePath := filepath.Join(app.exeDir, "demo", "wanmei", matchID, matchID+".dem")
	legacyRawPath := filepath.Join(app.exeDir, "demo", "raw", hashSourcePath(stableSourcePath), filepath.Base(stableSourcePath))

	if err := os.MkdirAll(filepath.Dir(legacyRawPath), 0755); err != nil {
		t.Fatalf("mkdir legacy raw dir: %v", err)
	}
	if err := os.WriteFile(legacyRawPath, []byte("legacy"), 0644); err != nil {
		t.Fatalf("write legacy raw demo: %v", err)
	}

	got, err := app.ImportWanmeiMatch("PVP@" + matchID)
	if err != nil {
		t.Fatalf("ImportWanmeiMatch() error: %v", err)
	}
	if len(got) != 1 || got[0] != stableSourcePath {
		t.Fatalf("result = %v, want [%q]", got, stableSourcePath)
	}
	if _, err := os.Stat(legacyRawPath); err == nil || !os.IsNotExist(err) {
		t.Fatalf("legacy raw path should be removed, stat err=%v", err)
	}
}

func stubWanmeiTestResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func encodeWanmeiLogForTest(raw string) string {
	encoded := make([]byte, 0, len(raw)*2)
	for i, b := range []byte(raw) {
		x := b ^ byte((42+3*i)%255)
		encoded = append(encoded, fmt.Sprintf("%02x", x)...)
	}
	return "^" + string(encoded) + "$"
}
