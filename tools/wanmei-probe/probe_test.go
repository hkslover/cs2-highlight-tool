package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func encodeTokenForTest(raw string) string {
	encoded := make([]byte, 0, len(raw)*2)
	for i, b := range []byte(raw) {
		x := b ^ byte((42+3*i)%255)
		encoded = append(encoded, fmt.Sprintf("%02x", x)...)
	}
	return "^" + string(encoded) + "$"
}

func pkcs7UnpadForTest(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid padded len: %d", len(data))
	}
	pad := int(data[len(data)-1])
	if pad <= 0 || pad > aes.BlockSize || pad > len(data) {
		return nil, fmt.Errorf("invalid padding: %d", pad)
	}
	return data[:len(data)-pad], nil
}

func buildZipBytesForTest(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestStep1AndStep2_SuccessAndErrors(t *testing.T) {
	rawToken := "my_tok_1711111111_76561198051245123"
	encoded := encodeTokenForTest(rawToken)

	var seenOrigin, seenReferer, seenNonce string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenOrigin = r.Header.Get("Origin")
		seenReferer = r.Header.Get("Referer")
		seenNonce = r.Header.Get("X-Nonce")
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"nickname":"alice","token":"%s"}`, encoded)
		case "/not-logged":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"err":"cors rejected"}`))
		}
	}))
	defer srv.Close()

	res := RunStep1(context.Background(), Step1Request{
		URL:           srv.URL + "/ok",
		Origin:        defaultAllowedOrigin,
		Referer:       defaultAllowedReferer,
		CustomHeaders: "X-Nonce: abc123",
	})
	if !res.OK || res.ClientStatus != "ready" {
		t.Fatalf("RunStep1(/ok) failed: %+v", res)
	}
	if seenOrigin != defaultAllowedOrigin || seenReferer != defaultAllowedReferer || seenNonce != "abc123" {
		t.Fatalf("unexpected headers: origin=%q referer=%q nonce=%q", seenOrigin, seenReferer, seenNonce)
	}
	if res.Step2Preview == nil || !res.Step2Preview.OK {
		t.Fatalf("Step2Preview failed: %+v", res.Step2Preview)
	}
	if res.Step2Preview.Token != "my_tok" || res.Step2Preview.ServerTime != "1711111111" || res.Step2Preview.SteamID != "76561198051245123" {
		t.Fatalf("unexpected decoded token parts: %+v", res.Step2Preview)
	}

	resNotLogged := RunStep1(context.Background(), Step1Request{URL: srv.URL + "/not-logged"})
	if resNotLogged.OK || resNotLogged.ClientStatus != "client_not_logged_in" {
		t.Fatalf("expected client_not_logged_in, got %+v", resNotLogged)
	}

	resForbidden := RunStep1(context.Background(), Step1Request{URL: srv.URL + "/forbidden"})
	if resForbidden.OK || resForbidden.ClientStatus != "http_error" || resForbidden.Response.StatusCode != 403 {
		t.Fatalf("expected http_error 403, got %+v", resForbidden)
	}
}

func TestStep3Matches_AppVersionOverrideAndSkipped(t *testing.T) {
	var gotVersion, gotToken string
	var gotMySteamID, gotToSteamID int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get("appversion")
		gotToken = r.Header.Get("token")
		var body struct {
			MySteamID int64 `json:"mySteamId"`
			ToSteamID int64 `json:"toSteamId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotMySteamID = body.MySteamID
		gotToSteamID = body.ToSteamID
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"statusCode": 0,
			"data": {
				"matchList": [
					{"matchId":"PVP@9208138716569380236","mapName":"de_mirage","score1":13,"score2":9,"kill":20,"death":15,"assist":5,"k4":"2","k5":1,"rating":"1.37","endTime":"2026-11-14 10:00:00"},
					{"matchId":"PVP@bad_id","mapName":"de_nuke"}
				]
			}
		}`))
	}))
	defer srv.Close()

	// Default appversion (4.2.2) and empty to_steam_id (falls back to mySteamId).
	resDefault := RunStep3(context.Background(), Step3Request{
		URL:     srv.URL,
		Token:   "tok123",
		SteamID: "76561198051245123",
		Page:    1,
	})
	if !resDefault.OK {
		t.Fatalf("RunStep3(default) failed: %+v", resDefault)
	}
	if gotVersion != "4.2.2" || gotToken != "tok123" {
		t.Fatalf("headers mismatch: appversion=%q token=%q", gotVersion, gotToken)
	}
	if gotMySteamID != 76561198051245123 || gotToSteamID != 76561198051245123 {
		t.Fatalf("steam id mismatch: my=%d to=%d", gotMySteamID, gotToSteamID)
	}
	if len(resDefault.Matches) != 1 || resDefault.Matches[0].DownloadMatchID != "9208138716569380236" {
		t.Fatalf("unexpected matches: %+v", resDefault.Matches)
	}
	if len(resDefault.SkippedItems) != 1 || resDefault.SkippedItems[0].RawMatchID != "PVP@bad_id" {
		t.Fatalf("unexpected skipped_items: %+v", resDefault.SkippedItems)
	}

	// Custom appversion and distinct to_steam_id (viewing another player's matches).
	resOther := RunStep3(context.Background(), Step3Request{
		URL:        srv.URL,
		Token:      "tok123",
		SteamID:    "76561198051245123",
		ToSteamID:  "76561198099999999",
		AppVersion: "1.0.26111211",
		Page:       1,
		PageSize:   11,
	})
	if !resOther.OK {
		t.Fatalf("RunStep3(other) failed: %+v", resOther)
	}
	if gotVersion != "1.0.26111211" {
		t.Fatalf("appversion = %q, want 1.0.26111211", gotVersion)
	}
	if gotMySteamID != 76561198051245123 || gotToSteamID != 76561198099999999 {
		t.Fatalf("custom toSteamId mismatch: my=%d to=%d", gotMySteamID, gotToSteamID)
	}
	if resOther.MySteamID != "76561198051245123" || resOther.ToSteamID != "76561198099999999" {
		t.Fatalf("result steam ids mismatch: my=%q to=%q", resOther.MySteamID, resOther.ToSteamID)
	}

	// Invalid to_steam_id should fail cleanly before making HTTP call.
	resInvalidTo := RunStep3(context.Background(), Step3Request{
		URL:       srv.URL,
		Token:     "tok123",
		SteamID:   "76561198051245123",
		ToSteamID: "not-a-steamid",
	})
	if resInvalidTo.OK || !strings.Contains(resInvalidTo.Summary, "toSteamId") {
		t.Fatalf("expected invalid toSteamId error, got %+v", resInvalidTo)
	}
}

func TestStep4PublicIP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/good" {
			_, _ = w.Write([]byte("203.0.113.42\n"))
			return
		}
		_, _ = w.Write([]byte("not-an-ip"))
	}))
	defer srv.Close()

	okRes := RunStep4(context.Background(), Step4Request{URL: srv.URL + "/good"})
	if !okRes.OK || okRes.IP != "203.0.113.42" || !okRes.ValidIPv4 {
		t.Fatalf("unexpected ok RunStep4: %+v", okRes)
	}

	badRes := RunStep4(context.Background(), Step4Request{URL: srv.URL + "/bad"})
	if badRes.OK || badRes.ValidIPv4 {
		t.Fatalf("expected invalid ipv4 failure, got %+v", badRes)
	}
}

func TestStep5AndStep6_SignatureAnd200JSONDiagnostic(t *testing.T) {
	s5 := RunStep5(Step5Request{
		DemoBaseURL: defaultDemoBaseURL,
		MatchID:     "PVP@9208138716569380236",
		CupID:       "0",
		AccessToken: "tok_abc",
		AppID:       defaultDemoAppID,
		Secret:      defaultDemoSecret,
		Randnum:     "123456",
		Timestamp:   "1711111111",
	})
	if !s5.OK || s5.DownloadMatchID != "9208138716569380236" {
		t.Fatalf("RunStep5 failed: %+v", s5)
	}
	if !strings.HasPrefix(s5.SignedURL, defaultDemoBaseURL+"/csgo/demo/9208138716569380236_0.dem?") {
		t.Fatalf("unexpected signed url: %q", s5.SignedURL)
	}

	steamID := "76561198051245123"
	ipAddr := "203.0.113.42"
	serverTime := "1711111111"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			sig := r.Header.Get("X-PWA-Signature")
			parts := strings.SplitN(sig, "-", 2)
			if len(parts) != 2 || parts[0] != serverTime {
				t.Fatalf("bad signature header: %q", sig)
			}
			cipherBytes, err := hex.DecodeString(parts[1])
			if err != nil {
				t.Fatalf("decode signature hex: %v", err)
			}
			key := []byte(serverTime + steamID[len(steamID)-(16-len(serverTime)):])
			iv := []byte(steamID[len(steamID)-16:])
			block, err := aes.NewCipher(key)
			if err != nil {
				t.Fatalf("new cipher: %v", err)
			}
			plain := make([]byte, len(cipherBytes))
			cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, cipherBytes)
			unpadded, err := pkcs7UnpadForTest(plain)
			if err != nil || string(unpadded) != ipAddr {
				t.Fatalf("decrypted ip = %q (err=%v), want %q", string(unpadded), err, ipAddr)
			}

			w.Header().Set("Location", "https://oss.example.com/demo/1.zip")
			w.WriteHeader(http.StatusFound)
		case "/json200":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"code":0,"data":{"downloadUrl":"https://oss.example.com/new-format.7z"}}`))
		}
	}))
	defer srv.Close()

	res302 := RunStep6(context.Background(), Step6Request{
		SignedURL:  srv.URL + "/redirect",
		SteamID:    steamID,
		IPAddr:     ipAddr,
		ServerTime: serverTime,
	})
	if !res302.OK || res302.OSSURL != "https://oss.example.com/demo/1.zip" {
		t.Fatalf("expected 302 success, got %+v", res302)
	}

	res200 := RunStep6(context.Background(), Step6Request{
		SignedURL:  srv.URL + "/json200",
		SteamID:    steamID,
		IPAddr:     ipAddr,
		ServerTime: serverTime,
	})
	if res200.OK {
		t.Fatalf("expected non-3xx failure for 200 JSON")
	}
	if !strings.Contains(res200.Error, "状态码: 200") {
		t.Fatalf("expected status 200 error, got %q", res200.Error)
	}
	if len(res200.DetectedURLs) != 1 || res200.DetectedURLs[0] != "https://oss.example.com/new-format.7z" {
		t.Fatalf("expected detected url in 200 JSON, got %+v", res200.DetectedURLs)
	}
}

func TestStep7_ZipSuccessAndRawDemDiagnostic(t *testing.T) {
	demBytes := append([]byte("PBDEMS2\x00"), bytes.Repeat([]byte("A"), 128)...)
	zipBytes := buildZipBytesForTest(t, map[string][]byte{
		"9208138716569380236_0.dem": demBytes,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/demo.zip":
			w.Header().Set("Content-Type", "application/zip")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(zipBytes)
		case "/raw.dem":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(demBytes)
		}
	}))
	defer srv.Close()

	headZip := RunStep7ProbeHead(context.Background(), Step7HeadRequest{OSSURL: srv.URL + "/demo.zip"})
	if !headZip.OK || headZip.Sniff.Format != "ZIP" || !headZip.Sniff.IsExpectedZip {
		t.Fatalf("head probe failed: %+v", headZip)
	}

	workDir := t.TempDir()
	mgr := NewStep7Manager(workDir)

	st, err := mgr.StartStep7(Step7StartRequest{
		OSSURL:          srv.URL + "/demo.zip",
		MatchID:         "9208138716569380236",
		RunFallbackDiag: true,
	})
	if err != nil {
		t.Fatalf("StartStep7 failed: %v", err)
	}
	st = waitForStep7Done(t, mgr, st.TaskID)
	if !st.OK || !st.WanmeiPipeline.CommitOK || !st.WanmeiPipeline.LikelyDemoOK {
		t.Fatalf("expected Step 7 zip pipeline OK, got %+v", st)
	}
	if _, statErr := os.Stat(st.WanmeiPipeline.StableCommitPath); statErr != nil {
		t.Fatalf("committed dem missing: %v", statErr)
	}

	// Verify raw .dem diagnostic when UnzipWithContext fails.
	stRaw, err := mgr.StartStep7(Step7StartRequest{
		OSSURL:          srv.URL + "/raw.dem",
		MatchID:         "9208138716569380236",
		RunFallbackDiag: true,
	})
	if err != nil {
		t.Fatalf("StartStep7(raw) failed: %v", err)
	}
	stRaw = waitForStep7Done(t, mgr, stRaw.TaskID)
	if stRaw.OK {
		t.Fatalf("expected raw .dem to fail UnzipWithContext pipeline")
	}
	if stRaw.Sniff == nil || stRaw.Sniff.Format != "RAW_DEM" {
		t.Fatalf("expected RAW_DEM sniff, got %+v", stRaw.Sniff)
	}
	if !stRaw.FallbackDiag.Attempted || !stRaw.FallbackDiag.Success {
		t.Fatalf("expected fallback diagnostic to confirm RAW_DEM, got %+v", stRaw.FallbackDiag)
	}

	if _, cleanErr := mgr.Cleanup(); cleanErr != nil {
		t.Fatalf("Cleanup error: %v", cleanErr)
	}
	entries, _ := os.ReadDir(workDir)
	if len(entries) != 0 {
		t.Fatalf("expected workDir cleaned, remaining entries: %d", len(entries))
	}
}

func waitForStep7Done(t *testing.T, mgr *Step7Manager, taskID string) Step7Status {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		st := mgr.Status(taskID)
		if !st.Running {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("step 7 task %s timed out", taskID)
	return Step7Status{}
}

func TestSniffBytes(t *testing.T) {
	if got := SniffBytes([]byte{0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c, 0x00}); got.Format != "7Z" || got.IsExpectedZip {
		t.Fatalf("unexpected 7z sniff: %+v", got)
	}
	if got := SniffBytes([]byte(`{"code":403,"msg":"denied"}`)); got.Format != "JSON" {
		t.Fatalf("unexpected json sniff: %+v", got)
	}
	if got := SniffBytes([]byte(`<Error><Code>AccessDenied</Code></Error>`)); got.Format != "XML_OR_HTML" {
		t.Fatalf("unexpected xml sniff: %+v", got)
	}
}

func TestServerEndpoints(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "probe")
	_ = os.MkdirAll(workDir, 0755)
	mgr := NewStep7Manager(workDir)
	handler, err := NewHandler(defaultConfigResponse(workDir), mgr)
	if err != nil {
		t.Fatalf("NewHandler error: %v", err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "完美平台 Demo 全链路诊断") {
		t.Fatalf("unexpected GET / response: status=%d", resp.StatusCode)
	}

	respDef, err := http.Get(srv.URL + "/api/defaults")
	if err != nil {
		t.Fatalf("GET /api/defaults failed: %v", err)
	}
	var def DefaultsResponse
	_ = json.NewDecoder(respDef.Body).Decode(&def)
	respDef.Body.Close()
	if def.AppVersion != "4.2.2" || def.DemoAppID != "20000" {
		t.Fatalf("unexpected defaults: %+v", def)
	}

	respBad, err := http.Post(srv.URL+"/api/step2/decode", "application/json", strings.NewReader("{bad"))
	if err != nil {
		t.Fatalf("POST bad json failed: %v", err)
	}
	respBad.Body.Close()
	if respBad.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad json, got %d", respBad.StatusCode)
	}
}
