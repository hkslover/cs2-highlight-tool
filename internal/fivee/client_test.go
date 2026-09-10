package fivee

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/download"
)

func TestExtractMatchID(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "raw id", input: "g161-20260427162329954189731", want: "g161-20260427162329954189731"},
		{name: "match detail url", input: "https://gate.5eplay.com/crane/http/api/data/match/g161-20260427162329954189731", want: "g161-20260427162329954189731"},
		{name: "demo zip url", input: "https://gz-t-demo.5eplaycdn.com/pug/20260427/g161-20260427162329954189731_de_dust2.zip", want: "g161-20260427162329954189731"},
		{name: "invalid", input: "hello", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractMatchID(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("extractMatchID(%q) expected error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("extractMatchID(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("extractMatchID(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestListRecentMatches_ParsesList(t *testing.T) {
	oldReq := HTTPRequestFn
	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		if req.URL.Host != "ya-api-app.5eplay.com" {
			t.Fatalf("unexpected host: %s", req.URL.Host)
		}
		if req.URL.Query().Get("domain") != "molim" {
			t.Fatalf("domain query = %q, want %q", req.URL.Query().Get("domain"), "molim")
		}
		if req.URL.Query().Get("page") != "3" {
			t.Fatalf("page query = %q, want %q", req.URL.Query().Get("page"), "3")
		}
		return stubFiveEResponse(http.StatusOK, `{
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
						"rating": "1.61",
						"end_time": "1777280204"
					}
				]
			}
		}`), nil
	}
	t.Cleanup(func() { HTTPRequestFn = oldReq })

	matches, err := ListRecentMatches("molim", 3)
	if err != nil {
		t.Fatalf("ListRecentMatches() error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches len = %d, want 1", len(matches))
	}
	match := matches[0]
	if match.DownloadMatchID != "g161-20260427162329954189731" {
		t.Fatalf("download_match_id = %q", match.DownloadMatchID)
	}
	if match.Score1 != 13 || match.Score2 != 5 {
		t.Fatalf("score = %d:%d, want 13:5", match.Score1, match.Score2)
	}
	if match.Kill != 21 || match.Death != 13 || match.Assist != 5 {
		t.Fatalf("kda = %d/%d/%d, want 21/13/5", match.Kill, match.Death, match.Assist)
	}
	if math.Abs(match.Rating-1.61) > 1e-6 {
		t.Fatalf("rating = %f, want 1.61", match.Rating)
	}
	wantEndTime := time.Unix(1777280204, 0).Format("2006-01-02 15:04:05")
	if match.EndTime != wantEndTime {
		t.Fatalf("end_time = %q, want %q", match.EndTime, wantEndTime)
	}
}

func TestListRecentMatches_ParsesGzipBody(t *testing.T) {
	oldReq := HTTPRequestFn
	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return stubFiveEGzipResponse(http.StatusOK, `{
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
		}`)
	}
	t.Cleanup(func() { HTTPRequestFn = oldReq })

	matches, err := ListRecentMatches("molim", 1)
	if err != nil {
		t.Fatalf("ListRecentMatches() error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches len = %d, want 1", len(matches))
	}
	if matches[0].DownloadMatchID != "g161-20260427162329954189731" {
		t.Fatalf("download_match_id = %q", matches[0].DownloadMatchID)
	}
}

func TestListRecentMatches_EmptyPageReturnsNoMatches(t *testing.T) {
	oldReq := HTTPRequestFn
	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		if req.URL.Query().Get("page") != "2" {
			t.Fatalf("page query = %q, want %q", req.URL.Query().Get("page"), "2")
		}
		return stubFiveEResponse(http.StatusOK, `{
			"success": true,
			"errcode": 0,
			"message": "",
			"data": {"match_list": []}
		}`), nil
	}
	t.Cleanup(func() { HTTPRequestFn = oldReq })

	matches, err := ListRecentMatches("molim", 2)
	if err != nil {
		t.Fatalf("ListRecentMatches() error: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches len = %d, want 0", len(matches))
	}
}

func TestListRecentMatches_NormalizesPageBelowOne(t *testing.T) {
	oldReq := HTTPRequestFn
	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		if req.URL.Query().Get("page") != "1" {
			t.Fatalf("page query = %q, want %q", req.URL.Query().Get("page"), "1")
		}
		return stubFiveEResponse(http.StatusOK, `{
			"success": true,
			"errcode": 0,
			"message": "",
			"data": {"match_list": []}
		}`), nil
	}
	t.Cleanup(func() { HTTPRequestFn = oldReq })

	if _, err := ListRecentMatches("molim", 0); err != nil {
		t.Fatalf("ListRecentMatches() error: %v", err)
	}
}

func TestNormalizePlayerDomainInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "raw domain id",
			input: "  12139xi22eza  ",
			want:  "12139xi22eza",
		},
		{
			name:  "share text with profile url",
			input: "【5E对战平台：被窝以外皆他乡OVO的个人主页】https://csgo.5eplay.com/app/share_loding_type7?domain=12139xi22eza&tab=77&uuid=338955c1-d7bb-11f0-a93a-0c42a164bc3c",
			want:  "12139xi22eza",
		},
		{
			name:  "query string only",
			input: "domain=12139xi22eza&tab=77",
			want:  "12139xi22eza",
		},
		{
			name:  "empty",
			input: "   ",
			want:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizePlayerDomainInput(tc.input); got != tc.want {
				t.Fatalf("NormalizePlayerDomainInput(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSafeArchiveNameStripsURLQueryAndUnsafeCharacters(t *testing.T) {
	got := safeArchiveName("https://example.com/path/demo.zip?signature=secret&expires=123", "g161-20260427162329954189731")
	if got != "demo.zip" {
		t.Fatalf("safeArchiveName() = %q, want %q", got, "demo.zip")
	}

	got = safeArchiveName("https://example.com/download?name=demo.zip", "g161-20260427162329954189731")
	if strings.ContainsAny(got, `?\/:*<>|"`) {
		t.Fatalf("safeArchiveName() returned unsafe name %q", got)
	}
	if got == "" {
		t.Fatal("safeArchiveName() returned empty name")
	}
}

func TestImportDemo_ExpiredDemoURL(t *testing.T) {
	oldReq := HTTPRequestFn
	oldDownload := DownloadFileFn

	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return stubFiveEResponse(http.StatusOK, `{"data":{"main":{"demo_url":""}}}`), nil
	}
	downloadCalls := 0
	DownloadFileFn = func(url string, targetPath string, emitProgress download.ProgressFunc) error {
		downloadCalls++
		return nil
	}
	t.Cleanup(func() {
		HTTPRequestFn = oldReq
		DownloadFileFn = oldDownload
	})

	_, err := ImportDemo("g161-20260427162329954189731", t.TempDir(), nil)
	if err == nil {
		t.Fatal("ImportDemo expected error")
	}
	if !strings.Contains(err.Error(), "已过期") {
		t.Fatalf("error = %v, want expired message", err)
	}
	if downloadCalls != 0 {
		t.Fatalf("download should not be called when demo url expired, got %d", downloadCalls)
	}
}

func TestImportDemo_ReuseCachedDemoWithoutRedownload(t *testing.T) {
	oldReq := HTTPRequestFn
	oldDownload := DownloadFileFn
	oldUnzip := UnzipFn
	oldFind := FindFirstByExtFn
	oldCopy := CopyFileFn

	detailCalls := 0
	downloadCalls := 0
	unzipCalls := 0
	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		detailCalls++
		return stubFiveEResponse(http.StatusOK, `{"data":{"main":{"demo_url":"https://example.com/g161-20260427162329954189731_de_dust2.zip"}}}`), nil
	}
	DownloadFileFn = func(url string, targetPath string, emitProgress download.ProgressFunc) error {
		downloadCalls++
		return os.WriteFile(targetPath, []byte("zip"), 0644)
	}
	UnzipFn = func(ctx context.Context, archivePath string, destDir string) error {
		unzipCalls++
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(destDir, "inner.dem"), []byte("PBDEMS2\x00fivee-dem"), 0644)
	}
	FindFirstByExtFn = download.FindFirstByExt
	CopyFileFn = download.CopyFile

	t.Cleanup(func() {
		HTTPRequestFn = oldReq
		DownloadFileFn = oldDownload
		UnzipFn = oldUnzip
		FindFirstByExtFn = oldFind
		CopyFileFn = oldCopy
	})

	matchID := "g161-20260427162329954189731"
	cacheRoot := filepath.Join(t.TempDir(), "demo", "5e", matchID)
	stableSourcePath := filepath.Join(cacheRoot, matchID+".dem")
	archivePath := filepath.Join(cacheRoot, matchID+"_de_dust2.zip")
	extractDir := filepath.Join(cacheRoot, "extract")

	first, err := ImportDemo(matchID, cacheRoot, nil)
	if err != nil {
		t.Fatalf("first ImportDemo() error: %v", err)
	}
	second, err := ImportDemo(matchID, cacheRoot, nil)
	if err != nil {
		t.Fatalf("second ImportDemo() error: %v", err)
	}
	if first != stableSourcePath {
		t.Fatalf("first path = %q, want %q", first, stableSourcePath)
	}
	if first != second {
		t.Fatalf("expected same path, got first=%q second=%q", first, second)
	}
	if detailCalls != 1 {
		t.Fatalf("detail calls = %d, want 1", detailCalls)
	}
	if downloadCalls != 1 {
		t.Fatalf("download calls = %d, want 1", downloadCalls)
	}
	if unzipCalls != 1 {
		t.Fatalf("unzip calls = %d, want 1", unzipCalls)
	}
	if _, err := os.Stat(extractDir); err == nil || !os.IsNotExist(err) {
		t.Fatalf("extract dir should be cleaned, stat err=%v", err)
	}
	if _, err := os.Stat(archivePath); err == nil || !os.IsNotExist(err) {
		t.Fatalf("archive should be cleaned, stat err=%v", err)
	}

	content, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("read managed demo failed: %v", err)
	}
	if string(content) != "PBDEMS2\x00fivee-dem" {
		t.Fatalf("managed demo content = %q, want %q", string(content), "PBDEMS2\\x00fivee-dem")
	}
}

func stubFiveEResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func stubFiveEGzipResponse(statusCode int, body string) (*http.Response, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write([]byte(body)); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	header := make(http.Header)
	header.Set("Content-Encoding", "gzip")
	return &http.Response{
		StatusCode: statusCode,
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(buf.Bytes())),
	}, nil
}
