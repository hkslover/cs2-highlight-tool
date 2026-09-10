package wanmei

import (
	"encoding/json"
	"fmt"
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

func TestDecodeEncodeLog_Success(t *testing.T) {
	raw := "token_value_1711111111_76561198000000000"
	encoded := encodeLogForTest(raw)
	if got := decodeEncodeLog(encoded); got != raw {
		t.Fatalf("decodeEncodeLog() = %q, want %q", got, raw)
	}
}

func TestDecodeEncodeLog_InvalidFormat(t *testing.T) {
	raw := "invalid-format"
	if got := decodeEncodeLog(raw); got != raw {
		t.Fatalf("decodeEncodeLog() = %q, want %q", got, raw)
	}
}

func TestExtractNumericMatchID(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "raw number", input: "9208138716569380236", want: "9208138716569380236"},
		{name: "prefixed match id", input: "PVP@9208138716569380236", want: "9208138716569380236"},
		{name: "download path", input: "https://pwaweblogin.wmpvp.com/csgo/demo/9208138716569380236_0.dem", want: "9208138716569380236"},
		{name: "invalid", input: "PVP@abc", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractNumericMatchID(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ExtractNumericMatchID(%q) expected error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExtractNumericMatchID(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("ExtractNumericMatchID(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestListRecentMatches_ClientNotRunning(t *testing.T) {
	oldReq := HTTPRequestFn
	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return nil, fmt.Errorf("connection refused")
	}
	t.Cleanup(func() { HTTPRequestFn = oldReq })

	result, err := ListRecentMatches(1)
	if err != nil {
		t.Fatalf("ListRecentMatches() error: %v", err)
	}
	if result.Status != ClientNotRunning {
		t.Fatalf("status = %q, want %q", result.Status, ClientNotRunning)
	}
}

func TestListRecentMatches_ClientNotLogged(t *testing.T) {
	oldReq := HTTPRequestFn
	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return stubResponse(http.StatusCreated, "{}"), nil
	}
	t.Cleanup(func() { HTTPRequestFn = oldReq })

	result, err := ListRecentMatches(1)
	if err != nil {
		t.Fatalf("ListRecentMatches() error: %v", err)
	}
	if result.Status != ClientNotLogged {
		t.Fatalf("status = %q, want %q", result.Status, ClientNotLogged)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("matches len = %d, want 0", len(result.Matches))
	}
}

func TestListRecentMatches_ClientNotLoggedWithEmptyPayload(t *testing.T) {
	oldReq := HTTPRequestFn
	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return stubResponse(http.StatusOK, "{}"), nil
	}
	t.Cleanup(func() { HTTPRequestFn = oldReq })

	result, err := ListRecentMatches(1)
	if err != nil {
		t.Fatalf("ListRecentMatches() error: %v", err)
	}
	if result.Status != ClientNotLogged {
		t.Fatalf("status = %q, want %q", result.Status, ClientNotLogged)
	}
}

func TestListRecentMatches_Ready(t *testing.T) {
	oldReq := HTTPRequestFn
	token := "demo_token"
	steamID := "76561198000000000"
	encodedToken := encodeLogForTest(token + "_1711111111_" + steamID)
	calls := 0

	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			if req.URL.String() != localLoginURL {
				t.Fatalf("unexpected local login url: %s", req.URL.String())
			}
			return stubResponse(http.StatusOK, fmt.Sprintf(`{"nickname":"alice","token":"%s"}`, encodedToken)), nil
		case 2:
			if req.URL.String() != matchListURL {
				t.Fatalf("unexpected match list url: %s", req.URL.String())
			}
			if req.Header.Get("token") != token {
				t.Fatalf("request token header = %q, want %q", req.Header.Get("token"), token)
			}
			requestBody, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read request body failed: %v", err)
			}
			var requestPayload map[string]any
			if err := json.Unmarshal(requestBody, &requestPayload); err != nil {
				t.Fatalf("unmarshal request body failed: %v", err)
			}
			if page := parseInt(requestPayload["page"]); page != 2 {
				t.Fatalf("request page = %d, want 2", page)
			}
			return stubResponse(http.StatusOK, `{
				"statusCode":0,
				"data":{
					"matchList":[
						{"matchId":"PVP@9208138716569380236","mapName":"de_mirage","score1":13,"score2":9,"kill":20,"death":15,"assist":5,"k4":"2","k5":1,"rating":"1.37","endTime":"2026-04-26 12:00:00"},
						{"matchId":"PVP@invalid","mapName":"de_nuke","score1":13,"score2":11,"kill":12,"death":18,"assist":2,"endTime":"2026-04-25 12:00:00"}
					]
				}
			}`), nil
		default:
			return nil, fmt.Errorf("unexpected call: %d", calls)
		}
	}
	t.Cleanup(func() { HTTPRequestFn = oldReq })

	result, err := ListRecentMatches(2)
	if err != nil {
		t.Fatalf("ListRecentMatches() error: %v", err)
	}
	if result.Status != ClientReady {
		t.Fatalf("status = %q, want %q", result.Status, ClientReady)
	}
	if result.Nickname != "alice" {
		t.Fatalf("nickname = %q, want %q", result.Nickname, "alice")
	}
	if result.SteamID != steamID {
		t.Fatalf("steam_id = %q, want %q", result.SteamID, steamID)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches len = %d, want 1", len(result.Matches))
	}
	if result.Matches[0].DownloadMatchID != "9208138716569380236" {
		t.Fatalf("download_match_id = %q", result.Matches[0].DownloadMatchID)
	}
	if result.Matches[0].K4 != 2 || result.Matches[0].K5 != 1 {
		t.Fatalf("k4/k5 = %d/%d, want 2/1", result.Matches[0].K4, result.Matches[0].K5)
	}
	if math.Abs(result.Matches[0].Rating-1.37) > 1e-6 {
		t.Fatalf("rating = %f, want 1.37", result.Matches[0].Rating)
	}
}

func TestImportDemo_ReuseCachedDemoWithoutRedownload(t *testing.T) {
	oldReq := HTTPRequestFn
	oldResolve := OSSResolveHTTPDoFn
	oldDownload := DownloadFileFn
	oldUnzip := UnzipFn
	oldFind := FindFirstByExtFn
	oldCopy := CopyFileFn

	token := "demo_token"
	steamID := "76561198051245123"
	serverTime := "1711111111"
	encodedToken := encodeLogForTest(token + "_" + serverTime + "_" + steamID)

	httpCalls := 0
	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		httpCalls++
		switch httpCalls {
		case 1:
			return stubResponse(http.StatusOK, fmt.Sprintf(`{"nickname":"alice","token":"%s"}`, encodedToken)), nil
		case 2:
			return stubResponse(http.StatusOK, "1.2.3.4"), nil
		default:
			return nil, fmt.Errorf("unexpected HTTP request: %s", req.URL.String())
		}
	}

	OSSResolveHTTPDoFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"https://oss.example.com/demo.zip"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	}

	downloadCalls := 0
	unzipCalls := 0
	DownloadFileFn = func(url string, targetPath string, emitProgress download.ProgressFunc) error {
		downloadCalls++
		if url != "https://oss.example.com/demo.zip" {
			return fmt.Errorf("unexpected url: %s", url)
		}
		return os.WriteFile(targetPath, []byte("zip"), 0644)
	}
	UnzipFn = func(archivePath string, destDir string) error {
		unzipCalls++
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return err
		}
		content := fmt.Sprintf("PBDEMS2\x00dem-%d", unzipCalls)
		return os.WriteFile(filepath.Join(destDir, fmt.Sprintf("inner-%d.dem", unzipCalls)), []byte(content), 0644)
	}
	FindFirstByExtFn = download.FindFirstByExt
	CopyFileFn = download.CopyFile

	t.Cleanup(func() {
		HTTPRequestFn = oldReq
		OSSResolveHTTPDoFn = oldResolve
		DownloadFileFn = oldDownload
		UnzipFn = oldUnzip
		FindFirstByExtFn = oldFind
		CopyFileFn = oldCopy
	})

	matchID := "9208138716569380236"
	cacheRoot := filepath.Join(t.TempDir(), "demo", "wanmei", matchID)
	stableSourcePath := filepath.Join(cacheRoot, matchID+".dem")
	archivePath := filepath.Join(cacheRoot, matchID+"_0.zip")
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
	if string(content) != "PBDEMS2\x00dem-1" {
		t.Fatalf("managed demo content = %q, want %q", string(content), "PBDEMS2\\x00dem-1")
	}
}

func TestImportDemo_EmptyCachedDemoTriggersRedownload(t *testing.T) {
	oldReq := HTTPRequestFn
	oldResolve := OSSResolveHTTPDoFn
	oldDownload := DownloadFileFn
	oldUnzip := UnzipFn
	oldFind := FindFirstByExtFn
	oldCopy := CopyFileFn

	token := "demo_token"
	steamID := "76561198051245123"
	serverTime := "1711111111"
	encodedToken := encodeLogForTest(token + "_" + serverTime + "_" + steamID)

	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		if req.URL.String() == localLoginURL {
			return stubResponse(http.StatusOK, fmt.Sprintf(`{"nickname":"alice","token":"%s"}`, encodedToken)), nil
		}
		if req.URL.String() == publicIPAPI {
			return stubResponse(http.StatusOK, "1.2.3.4"), nil
		}
		return nil, fmt.Errorf("unexpected HTTP request: %s", req.URL.String())
	}

	OSSResolveHTTPDoFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"https://oss.example.com/demo.zip"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	}

	downloadCalls := 0
	unzipCalls := 0
	DownloadFileFn = func(url string, targetPath string, emitProgress download.ProgressFunc) error {
		downloadCalls++
		return os.WriteFile(targetPath, []byte("zip"), 0644)
	}
	UnzipFn = func(archivePath string, destDir string) error {
		unzipCalls++
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(destDir, "inner.dem"), []byte("redownloaded"), 0644)
	}
	FindFirstByExtFn = download.FindFirstByExt
	CopyFileFn = download.CopyFile

	t.Cleanup(func() {
		HTTPRequestFn = oldReq
		OSSResolveHTTPDoFn = oldResolve
		DownloadFileFn = oldDownload
		UnzipFn = oldUnzip
		FindFirstByExtFn = oldFind
		CopyFileFn = oldCopy
	})

	matchID := "9208138716569380236"
	cacheRoot := filepath.Join(t.TempDir(), "demo", "wanmei", matchID)
	if err := os.MkdirAll(cacheRoot, 0755); err != nil {
		t.Fatalf("mkdir cache root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheRoot, matchID+".dem"), []byte{}, 0644); err != nil {
		t.Fatalf("write empty cached demo: %v", err)
	}

	got, err := ImportDemo(matchID, cacheRoot, nil)
	if err != nil {
		t.Fatalf("ImportDemo() error: %v", err)
	}
	expectedPath := filepath.Join(cacheRoot, matchID+".dem")
	if got != expectedPath {
		t.Fatalf("managed path = %q, want %q", got, expectedPath)
	}
	if downloadCalls != 1 {
		t.Fatalf("download calls = %d, want 1", downloadCalls)
	}
	if unzipCalls != 1 {
		t.Fatalf("unzip calls = %d, want 1", unzipCalls)
	}

	content, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("read managed demo failed: %v", err)
	}
	if string(content) != "redownloaded" {
		t.Fatalf("managed demo content = %q, want %q", string(content), "redownloaded")
	}
}

func TestFetchPublicIPv4_Valid(t *testing.T) {
	oldReq := HTTPRequestFn
	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return stubResponse(http.StatusOK, "1.2.3.4"), nil
	}
	t.Cleanup(func() { HTTPRequestFn = oldReq })

	ip, err := fetchPublicIPv4("https://fake-ip-api.example.com/ip", 5*time.Second)
	if err != nil {
		t.Fatalf("fetchPublicIPv4() error: %v", err)
	}
	if ip != "1.2.3.4" {
		t.Fatalf("ip = %q, want %q", ip, "1.2.3.4")
	}
}

func TestFetchPublicIPv4_InvalidFormat(t *testing.T) {
	oldReq := HTTPRequestFn
	HTTPRequestFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return stubResponse(http.StatusOK, "not-an-ip"), nil
	}
	t.Cleanup(func() { HTTPRequestFn = oldReq })

	_, err := fetchPublicIPv4("https://fake-ip-api.example.com/ip", 5*time.Second)
	if err == nil {
		t.Fatal("fetchPublicIPv4() expected error for invalid IP format")
	}
}

func TestPvpSign(t *testing.T) {
	sig := pvpSign("123456", "1711111111", "access_token=abc&cup_id=0&match_id=9208138716569380236", "20000", "969c1bcfdc527c319157cc48f83b1d106ebdeca3e8d9763f1ae6b88dde9b3ea9")
	if sig == "" {
		t.Fatal("pvpSign() returned empty string")
	}
	sig2 := pvpSign("123456", "1711111111", "access_token=abc&cup_id=0&match_id=9208138716569380236", "20000", "969c1bcfdc527c319157cc48f83b1d106ebdeca3e8d9763f1ae6b88dde9b3ea9")
	if sig != sig2 {
		t.Fatalf("pvpSign() not deterministic: %q != %q", sig, sig2)
	}
	sig3 := pvpSign("654321", "1711111111", "access_token=abc&cup_id=0&match_id=9208138716569380236", "20000", "969c1bcfdc527c319157cc48f83b1d106ebdeca3e8d9763f1ae6b88dde9b3ea9")
	if sig == sig3 {
		t.Fatal("pvpSign() different inputs produced same signature")
	}
}

func TestBuildXPWASignature(t *testing.T) {
	steamID := "76561198051245123"
	ipAddr := "1.2.3.4"
	serverTime := "1711111111"

	sig, err := buildXPWASignature(steamID, ipAddr, serverTime)
	if err != nil {
		t.Fatalf("buildXPWASignature() error: %v", err)
	}

	parts := strings.SplitN(sig, "-", 2)
	if len(parts) != 2 {
		t.Fatalf("buildXPWASignature() format invalid: %q", sig)
	}
	if parts[0] != serverTime {
		t.Fatalf("signature prefix = %q, want %q", parts[0], serverTime)
	}
	if len(parts[1])%32 != 0 {
		t.Fatalf("encrypted hex part has odd length: %d", len(parts[1]))
	}

	sig2, _ := buildXPWASignature(steamID, ipAddr, serverTime)
	if sig != sig2 {
		t.Fatal("buildXPWASignature() not deterministic")
	}

	sig3, _ := buildXPWASignature(steamID, "5.6.7.8", serverTime)
	if sig == sig3 {
		t.Fatal("buildXPWASignature() same output for different IPs")
	}
}

func TestBuildXPWASignature_ShortSteamID(t *testing.T) {
	_, err := buildXPWASignature("123", "1.2.3.4", "1711111111")
	if err == nil {
		t.Fatal("buildXPWASignature() expected error for short steam_id")
	}
}

func TestBuildPWAHeaders(t *testing.T) {
	headers, err := buildPWAHeaders("76561198051245123", "1.2.3.4", "1711111111")
	if err != nil {
		t.Fatalf("buildPWAHeaders() error: %v", err)
	}
	if headers["X-PWA-SteamId"] != "76561198051245123" {
		t.Fatalf("X-PWA-SteamId = %q", headers["X-PWA-SteamId"])
	}
	if headers["PwaSteamId"] != "76561198051245123" {
		t.Fatalf("PwaSteamId = %q", headers["PwaSteamId"])
	}
	if headers["Referer"] != pwaReferer {
		t.Fatalf("Referer = %q", headers["Referer"])
	}
	sig := headers["X-PWA-Signature"]
	if !strings.Contains(sig, "1711111111-") {
		t.Fatalf("X-PWA-Signature missing server_time prefix: %q", sig)
	}
}

func TestBuildSignedDemoURL(t *testing.T) {
	url := buildSignedDemoURL("9208138716569380236", "0", "test_token")
	if !strings.HasPrefix(url, demoBaseURL+"/csgo/demo/9208138716569380236_0.dem?") {
		t.Fatalf("unexpected URL prefix: %q", url)
	}
	if !strings.Contains(url, "a="+demoAppID) {
		t.Fatalf("URL missing appid param: %q", url)
	}
	if !strings.Contains(url, "access_token=test_token") {
		t.Fatalf("URL missing access_token param: %q", url)
	}
	if !strings.Contains(url, "match_id=9208138716569380236") {
		t.Fatalf("URL missing match_id param: %q", url)
	}
	if !strings.Contains(url, "cup_id=0") {
		t.Fatalf("URL missing cup_id param: %q", url)
	}
}

func TestResolveOSSURL_Success(t *testing.T) {
	oldResolve := OSSResolveHTTPDoFn
	OSSResolveHTTPDoFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusFound,
			Header: http.Header{
				"Location": []string{"https://oss.example.com/bucket/demo.zip?sign=abc"},
			},
			Body: io.NopCloser(strings.NewReader("")),
		}, nil
	}
	t.Cleanup(func() { OSSResolveHTTPDoFn = oldResolve })

	headers := map[string]string{
		"X-PWA-SteamId":   "76561198051245123",
		"X-PWA-Signature": "1711111111-abcd1234",
		"PwaSteamId":      "76561198051245123",
		"Referer":         pwaReferer,
	}
	location, err := resolveOSSURL("https://pwaweblogin.wmpvp.com/csgo/demo/9208138716569380236_0.dem?a=20000&r=123&s=abc&t=123", headers, 8*time.Second)
	if err != nil {
		t.Fatalf("resolveOSSURL() error: %v", err)
	}
	if location != "https://oss.example.com/bucket/demo.zip?sign=abc" {
		t.Fatalf("location = %q", location)
	}
}

func TestResolveOSSURL_NonRedirect(t *testing.T) {
	oldResolve := OSSResolveHTTPDoFn
	OSSResolveHTTPDoFn = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return stubResponse(http.StatusOK, "not a redirect"), nil
	}
	t.Cleanup(func() { OSSResolveHTTPDoFn = oldResolve })

	headers := map[string]string{"X-PWA-SteamId": "76561198051245123"}
	_, err := resolveOSSURL("https://example.com/signed", headers, 8*time.Second)
	if err == nil {
		t.Fatal("resolveOSSURL() expected error for non-redirect response")
	}
}

func stubResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func encodeLogForTest(raw string) string {
	encoded := make([]byte, 0, len(raw)*2)
	for i, b := range []byte(raw) {
		x := b ^ byte((42+3*i)%255)
		encoded = append(encoded, fmt.Sprintf("%02x", x)...)
	}
	return "^" + string(encoded) + "$"
}
