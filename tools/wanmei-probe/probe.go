package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"cs2-highlight-tool-v2/internal/download"
	"cs2-highlight-tool-v2/internal/wanmei"
	"github.com/bodgit/sevenzip"
)

const (
	defaultLocalLoginURL  = "http://127.0.0.1:55555/"
	defaultAllowedOrigin  = "https://esports.wanmei.com"
	defaultAllowedReferer = "https://esports.wanmei.com/"
	defaultMatchListURL   = "https://api.wmpvp.com/api/csgo/home/match/list"
	defaultAppVersion     = "4.2.2"
	defaultPlatform       = "ios"
	defaultAppTheme       = "0"
	defaultGameType       = "1,2"
	defaultGameTypeStr    = "1,2"
	defaultPVPType        = -1
	defaultDataSource     = 3
	defaultSeasonID       = "recent"
	defaultPageSize       = 11
	defaultPublicIPAPI    = "https://api-ipv4.ip.sb/ip"
	defaultDemoBaseURL    = "https://pwaweblogin.wmpvp.com"
	defaultDemoAppID      = "20000"
	defaultDemoSecret     = "969c1bcfdc527c319157cc48f83b1d106ebdeca3e8d9763f1ae6b88dde9b3ea9"
	defaultCupID          = "0"
	defaultPWAReferer     = "https://client.wmpvp.com"

	maxTraceBodyBytes = 2 << 20 // 2 MiB
	maxArchiveEntries = 100
	downloadUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
)

var (
	ipv4Regex    = regexp.MustCompile(`^(?:\d{1,3}\.){3}\d{1,3}$`)
	httpURLRegex = regexp.MustCompile(`https?://[^\s"'<>\\]+`)
)

// DefaultsResponse exposes the current internal/wanmei constants and environment info.
type DefaultsResponse struct {
	LocalLoginURL     string   `json:"local_login_url"`
	AllowedOrigin     string   `json:"allowed_origin"`
	AllowedReferer    string   `json:"allowed_referer"`
	MatchListURL      string   `json:"match_list_url"`
	AppVersion        string   `json:"app_version"`
	AppVersionPresets []string `json:"app_version_presets"`
	Platform          string   `json:"platform"`
	AppTheme          string   `json:"app_theme"`
	GameType          string   `json:"game_type"`
	GameTypeStr       string   `json:"game_type_str"`
	PVPType           int      `json:"pvp_type"`
	DataSource        int      `json:"data_source"`
	CSGOSeasonID      string   `json:"csgo_season_id"`
	Page              int      `json:"page"`
	PageSize          int      `json:"page_size"`
	PublicIPAPI       string   `json:"public_ip_api"`
	DemoBaseURL       string   `json:"demo_base_url"`
	DemoAppID         string   `json:"demo_app_id"`
	DemoSecret        string   `json:"demo_secret"`
	CupID             string   `json:"cup_id"`
	PWAReferer        string   `json:"pwa_referer"`
	WorkDir           string   `json:"work_dir"`
	GOOS              string   `json:"goos"`
}

func defaultConfigResponse(workDir string) DefaultsResponse {
	return DefaultsResponse{
		LocalLoginURL:     defaultLocalLoginURL,
		AllowedOrigin:     defaultAllowedOrigin,
		AllowedReferer:    defaultAllowedReferer,
		MatchListURL:      defaultMatchListURL,
		AppVersion:        defaultAppVersion,
		AppVersionPresets: []string{"4.2.2", "3.7.7", "1.0.26111211"},
		Platform:          defaultPlatform,
		AppTheme:          defaultAppTheme,
		GameType:          defaultGameType,
		GameTypeStr:       defaultGameTypeStr,
		PVPType:           defaultPVPType,
		DataSource:        defaultDataSource,
		CSGOSeasonID:      defaultSeasonID,
		Page:              1,
		PageSize:          defaultPageSize,
		PublicIPAPI:       defaultPublicIPAPI,
		DemoBaseURL:       defaultDemoBaseURL,
		DemoAppID:         defaultDemoAppID,
		DemoSecret:        defaultDemoSecret,
		CupID:             defaultCupID,
		PWAReferer:        defaultPWAReferer,
		WorkDir:           workDir,
		GOOS:              runtime.GOOS,
	}
}

// HTTPTraceRequest records the outgoing request sent by a diagnostic step.
type HTTPTraceRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body,omitempty"`
}

// HTTPTraceResponse records raw response status, headers, and body.
type HTTPTraceResponse struct {
	StatusCode int                 `json:"status_code"`
	Status     string              `json:"status,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	RawBody    string              `json:"raw_body,omitempty"`
	BodySize   int                 `json:"body_size"`
	Truncated  bool                `json:"truncated,omitempty"`
	ElapsedMs  int64               `json:"elapsed_ms"`
	Error      string              `json:"error,omitempty"`
}

func doTraceRequest(ctx context.Context, req *http.Request, timeout time.Duration, followRedirects bool) (HTTPTraceRequest, HTTPTraceResponse) {
	reqHeaders := make(map[string]string, len(req.Header))
	for k, vals := range req.Header {
		reqHeaders[k] = strings.Join(vals, ", ")
	}
	var reqBodyCopy string
	if req.GetBody != nil {
		if rc, err := req.GetBody(); err == nil {
			b, _ := io.ReadAll(io.LimitReader(rc, maxTraceBodyBytes))
			reqBodyCopy = string(b)
			_ = rc.Close()
		}
	}
	traceReq := HTTPTraceRequest{
		Method:  req.Method,
		URL:     req.URL.String(),
		Headers: reqHeaders,
		Body:    reqBodyCopy,
	}

	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	if !followRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	started := time.Now()
	resp, err := client.Do(req.WithContext(ctx))
	elapsed := time.Since(started).Milliseconds()
	if err != nil {
		return traceReq, HTTPTraceResponse{
			ElapsedMs: elapsed,
			Error:     err.Error(),
		}
	}
	defer resp.Body.Close()

	bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, maxTraceBodyBytes+1))
	truncated := false
	if len(bodyBytes) > maxTraceBodyBytes {
		bodyBytes = bodyBytes[:maxTraceBodyBytes]
		truncated = true
	}

	respHeaders := make(map[string][]string, len(resp.Header))
	for k, v := range resp.Header {
		copied := make([]string, len(v))
		copy(copied, v)
		respHeaders[k] = copied
	}

	traceResp := HTTPTraceResponse{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Headers:    respHeaders,
		RawBody:    string(bodyBytes),
		BodySize:   len(bodyBytes),
		Truncated:  truncated,
		ElapsedMs:  elapsed,
	}
	if readErr != nil {
		traceResp.Error = fmt.Sprintf("读取响应体失败: %v", readErr)
	}
	return traceReq, traceResp
}

func parseCustomHeaders(raw string) (http.Header, error) {
	headers := make(http.Header)
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		colon := strings.Index(trimmed, ":")
		if colon <= 0 {
			return nil, fmt.Errorf("自定义请求头第 %d 行格式无效（应为 Key: Value）: %q", idx+1, trimmed)
		}
		key := strings.TrimSpace(trimmed[:colon])
		val := strings.TrimSpace(trimmed[colon+1:])
		if key == "" {
			return nil, fmt.Errorf("自定义请求头第 %d 行名称为空", idx+1)
		}
		headers.Add(key, val)
	}
	return headers, nil
}

func applyCustomHeaders(req *http.Request, custom http.Header) {
	for k, vals := range custom {
		req.Header.Del(k)
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
}

// Step1Request configures Step 1 local login probe.
type Step1Request struct {
	URL           string `json:"url"`
	Origin        string `json:"origin"`
	Referer       string `json:"referer"`
	CustomHeaders string `json:"custom_headers"`
	TimeoutSec    int    `json:"timeout_sec"`
}

// Step1ParsedLogin holds parsed JSON fields from 127.0.0.1:55555.
type Step1ParsedLogin struct {
	Nickname     string `json:"nickname"`
	Avatar       string `json:"avatar"`
	EncodedToken string `json:"encoded_token"`
}

// Step1Result is the Step 1 diagnostic result.
type Step1Result struct {
	OK           bool              `json:"ok"`
	ClientStatus string            `json:"client_status"`
	Summary      string            `json:"summary"`
	Error        string            `json:"error,omitempty"`
	Request      HTTPTraceRequest  `json:"request"`
	Response     HTTPTraceResponse `json:"response"`
	Parsed       Step1ParsedLogin  `json:"parsed"`
	Step2Preview *Step2Result      `json:"step2_preview,omitempty"`
}

// RunStep1 executes Step 1: local login probe against the Wanmei desktop client.
func RunStep1(ctx context.Context, req Step1Request) Step1Result {
	targetURL := strings.TrimSpace(req.URL)
	if targetURL == "" {
		targetURL = defaultLocalLoginURL
	}
	origin := strings.TrimSpace(req.Origin)
	referer := strings.TrimSpace(req.Referer)
	timeout := time.Duration(req.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	customHeaders, err := parseCustomHeaders(req.CustomHeaders)
	if err != nil {
		return Step1Result{
			ClientStatus: "invalid_input",
			Summary:      "请求参数无效",
			Error:        err.Error(),
		}
	}

	httpReq, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return Step1Result{
			ClientStatus: "invalid_input",
			Summary:      "创建本地登录请求失败",
			Error:        err.Error(),
		}
	}
	if origin != "" {
		httpReq.Header.Set("Origin", origin)
	}
	if referer != "" {
		httpReq.Header.Set("Referer", referer)
	}
	applyCustomHeaders(httpReq, customHeaders)

	traceReq, traceResp := doTraceRequest(ctx, httpReq, timeout, false)
	result := Step1Result{
		Request:  traceReq,
		Response: traceResp,
	}

	if traceResp.Error != "" && traceResp.StatusCode == 0 {
		result.ClientStatus = string(wanmei.ClientNotRunning)
		result.Summary = "无法连接完美客户端本地服务（未运行或端口变更）"
		result.Error = traceResp.Error
		return result
	}
	if traceResp.Error != "" {
		result.ClientStatus = "read_error"
		result.Summary = "读取本地登录响应失败"
		result.Error = traceResp.Error
		return result
	}

	trimmedBody := strings.TrimSpace(traceResp.RawBody)
	if traceResp.StatusCode == http.StatusCreated || trimmedBody == "{}" {
		result.ClientStatus = string(wanmei.ClientNotLogged)
		result.Summary = fmt.Sprintf("客户端已响应（HTTP %d），但当前未登录", traceResp.StatusCode)
		return result
	}
	if traceResp.StatusCode != http.StatusOK {
		result.ClientStatus = "http_error"
		result.Summary = fmt.Sprintf("本地接口返回非 200 状态码: %d（现行代码会判为未登录）", traceResp.StatusCode)
		result.Error = fmt.Sprintf("HTTP %d: %s", traceResp.StatusCode, truncatePreview(trimmedBody, 300))
		return result
	}

	var payload struct {
		Nickname string `json:"nickname"`
		Avatar   string `json:"avatar"`
		Token    string `json:"token"`
	}
	if err := json.Unmarshal([]byte(traceResp.RawBody), &payload); err != nil {
		result.ClientStatus = "parse_error"
		result.Summary = "本地接口返回 200，但 JSON 解析失败"
		result.Error = err.Error()
		return result
	}

	result.Parsed = Step1ParsedLogin{
		Nickname:     strings.TrimSpace(payload.Nickname),
		Avatar:       strings.TrimSpace(payload.Avatar),
		EncodedToken: strings.TrimSpace(payload.Token),
	}
	if result.Parsed.EncodedToken == "" {
		result.ClientStatus = string(wanmei.ClientNotLogged)
		result.Summary = "JSON 中 token 字段为空（未登录或字段名变化）"
		return result
	}

	preview := RunStep2(Step2Request{EncodedToken: result.Parsed.EncodedToken})
	result.Step2Preview = &preview
	result.OK = true
	result.ClientStatus = string(wanmei.ClientReady)
	if result.Parsed.Nickname != "" {
		result.Summary = fmt.Sprintf("本地登录态正常：%s（已获取编码 token）", result.Parsed.Nickname)
	} else {
		result.Summary = "本地登录态正常（已获取编码 token）"
	}
	return result
}

// Step2Request provides the encoded token for Step 2 decoding.
type Step2Request struct {
	EncodedToken string `json:"encoded_token"`
}

// Step2Result exposes every intermediate value of decodeEncodeLog and parseDecodedToken.
type Step2Result struct {
	OK                bool   `json:"ok"`
	Summary           string `json:"summary"`
	Error             string `json:"error,omitempty"`
	InputLength       int    `json:"input_length"`
	HasCaret          bool   `json:"has_caret"`
	HasTrailingDollar bool   `json:"has_trailing_dollar"`
	Prefix            string `json:"prefix"`
	HexLength         int    `json:"hex_length"`
	HexValid          bool   `json:"hex_valid"`
	DecodedRaw        string `json:"decoded_raw"`
	PartsCount        int    `json:"parts_count"`
	Token             string `json:"token"`
	ServerTime        string `json:"server_time"`
	ServerTimeHuman   string `json:"server_time_human,omitempty"`
	SteamID           string `json:"steam_id"`
}

// RunStep2 executes Step 2: token XOR decode and token_serverTime_steamID parsing.
func RunStep2(req Step2Request) Step2Result {
	encoded := strings.TrimSpace(req.EncodedToken)
	res := Step2Result{
		InputLength: len(encoded),
	}
	if encoded == "" {
		res.Summary = "encoded_token 为空"
		res.Error = "请先通过 Step 1 获取或手动输入 encoded_token"
		return res
	}

	decoded, meta := inspectDecodeEncodeLog(encoded)
	res.HasCaret = meta.hasCaret
	res.HasTrailingDollar = meta.hasTrailingDollar
	res.Prefix = meta.prefix
	res.HexLength = meta.hexLength
	res.HexValid = meta.hexValid
	res.DecodedRaw = decoded

	parts := strings.Split(res.DecodedRaw, "_")
	res.PartsCount = len(parts)
	token, steamID, serverTime, err := parseDecodedToken(res.DecodedRaw)
	if err != nil {
		res.Summary = "Token 解码或分段校验失败"
		res.Error = err.Error()
		return res
	}

	res.OK = true
	res.Token = token
	res.SteamID = steamID
	res.ServerTime = serverTime
	res.ServerTimeHuman = formatServerTimeHuman(serverTime)
	res.Summary = fmt.Sprintf("解码成功：SteamID=%s，serverTime=%s", steamID, serverTime)
	return res
}

type decodeMeta struct {
	hasCaret          bool
	hasTrailingDollar bool
	prefix            string
	hexLength         int
	hexValid          bool
}

func inspectDecodeEncodeLog(encoded string) (string, decodeMeta) {
	caret := strings.Index(encoded, "^")
	dollar := strings.LastIndex(encoded, "$")
	meta := decodeMeta{
		hasCaret:          caret >= 0,
		hasTrailingDollar: dollar == len(encoded)-1 && dollar > caret,
	}
	if caret == -1 || dollar != len(encoded)-1 || caret >= dollar {
		return encoded, meta
	}
	meta.prefix = encoded[:caret]
	hexPart := encoded[caret+1 : dollar]
	meta.hexLength = len(hexPart)
	if len(hexPart)%2 != 0 {
		return encoded, meta
	}

	raw, err := hex.DecodeString(hexPart)
	if err != nil {
		return encoded, meta
	}
	meta.hexValid = true
	for i := range raw {
		raw[i] ^= byte((42 + 3*i) % 255)
	}
	return meta.prefix + string(raw), meta
}

func parseDecodedToken(decoded string) (string, string, string, error) {
	parts := strings.Split(decoded, "_")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("解码格式异常（期望 token_serverTime_steamID，当前仅 %d 段）", len(parts))
	}

	token := strings.TrimSpace(strings.Join(parts[:len(parts)-2], "_"))
	serverTime := strings.TrimSpace(parts[len(parts)-2])
	steamID := strings.TrimSpace(parts[len(parts)-1])
	if token == "" || serverTime == "" || steamID == "" {
		return "", "", "", fmt.Errorf("token、server_time 或 steam_id 为空")
	}
	if _, err := strconv.ParseInt(steamID, 10, 64); err != nil {
		return "", "", "", fmt.Errorf("steam_id 无效: %q", steamID)
	}
	if _, err := strconv.ParseInt(serverTime, 10, 64); err != nil {
		return "", "", "", fmt.Errorf("server_time 无效: %q", serverTime)
	}
	return token, steamID, serverTime, nil
}

func formatServerTimeHuman(raw string) string {
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return ""
	}
	switch {
	case v >= 1_000_000_000_000:
		return time.UnixMilli(v).Local().Format("2006-01-02 15:04:05 MST") + " (毫秒)"
	case v >= 1_000_000_000:
		return time.Unix(v, 0).Local().Format("2006-01-02 15:04:05 MST") + " (秒)"
	default:
		return ""
	}
}

// Step3Request configures Step 3 match list query.
type Step3Request struct {
	URL           string `json:"url"`
	Token         string `json:"token"`
	SteamID       string `json:"steam_id"`
	ToSteamID     string `json:"to_steam_id"`
	Page          int    `json:"page"`
	PageSize      int    `json:"page_size"`
	AppVersion    string `json:"app_version"`
	Platform      string `json:"platform"`
	AppTheme      string `json:"app_theme"`
	GameType      string `json:"game_type"`
	GameTypeStr   string `json:"game_type_str"`
	PVPType       *int   `json:"pvp_type"`
	DataSource    *int   `json:"data_source"`
	CSGOSeasonID  string `json:"csgo_season_id"`
	Timestamp     string `json:"timestamp"`
	CustomHeaders string `json:"custom_headers"`
	TimeoutSec    int    `json:"timeout_sec"`
}

// SkippedMatchItem records match entries ignored because ExtractNumericMatchID failed.
type SkippedMatchItem struct {
	RawMatchID string `json:"raw_match_id"`
	MapName    string `json:"map_name,omitempty"`
	EndTime    string `json:"end_time,omitempty"`
	Reason     string `json:"reason"`
}

// Step3Result is the Step 3 diagnostic result.
type Step3Result struct {
	OK            bool                     `json:"ok"`
	Summary       string                   `json:"summary"`
	Error         string                   `json:"error,omitempty"`
	MySteamID     string                   `json:"my_steam_id,omitempty"`
	ToSteamID     string                   `json:"to_steam_id,omitempty"`
	Request       HTTPTraceRequest         `json:"request"`
	Response      HTTPTraceResponse        `json:"response"`
	APIStatusCode int                      `json:"api_status_code"`
	APICode       int                      `json:"api_code"`
	APIMessage    string                   `json:"api_message,omitempty"`
	DataShape     string                   `json:"data_shape,omitempty"`
	RawItemCount  int                      `json:"raw_item_count"`
	SkippedItems  []SkippedMatchItem       `json:"skipped_items"`
	Matches       []wanmei.WanmeiMatchItem `json:"matches"`
}

type matchListAPIResponse struct {
	StatusCode   int             `json:"statusCode"`
	Code         int             `json:"code"`
	ErrorMessage string          `json:"errorMessage"`
	Msg          string          `json:"msg"`
	Data         json.RawMessage `json:"data"`
}

type matchRaw struct {
	MatchID    string `json:"matchId"`
	MatchIDAlt string `json:"matchID"`
	MapName    string `json:"mapName"`
	Score1     int    `json:"score1"`
	Score2     int    `json:"score2"`
	Kill       int    `json:"kill"`
	Death      int    `json:"death"`
	Assist     int    `json:"assist"`
	K4         any    `json:"k4"`
	K5         any    `json:"k5"`
	Rating     any    `json:"rating"`
	EndTime    string `json:"endTime"`
}

// RunStep3 executes Step 3: Wanmei match list POST request.
func RunStep3(ctx context.Context, req Step3Request) Step3Result {
	targetURL := strings.TrimSpace(req.URL)
	if targetURL == "" {
		targetURL = defaultMatchListURL
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return Step3Result{Summary: "token 为空", Error: "请先完成 Step 2 或手动填入 token", SkippedItems: []SkippedMatchItem{}, Matches: []wanmei.WanmeiMatchItem{}}
	}
	steamIDStr := strings.TrimSpace(req.SteamID)
	steamIDInt, err := strconv.ParseInt(steamIDStr, 10, 64)
	if err != nil {
		return Step3Result{Summary: "mySteamId 无效", Error: "mySteamId (steam_id) 必须是有效的 64 位整数", SkippedItems: []SkippedMatchItem{}, Matches: []wanmei.WanmeiMatchItem{}}
	}
	toSteamIDStr := strings.TrimSpace(req.ToSteamID)
	toSteamIDInt := steamIDInt
	if toSteamIDStr == "" {
		toSteamIDStr = steamIDStr
	} else {
		parsedTo, err := strconv.ParseInt(toSteamIDStr, 10, 64)
		if err != nil {
			return Step3Result{Summary: "toSteamId 无效", Error: "toSteamId 必须是有效的 64 位整数", SkippedItems: []SkippedMatchItem{}, Matches: []wanmei.WanmeiMatchItem{}}
		}
		toSteamIDInt = parsedTo
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	appVersion := strings.TrimSpace(req.AppVersion)
	if appVersion == "" {
		appVersion = defaultAppVersion
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = defaultPlatform
	}
	appTheme := strings.TrimSpace(req.AppTheme)
	if appTheme == "" {
		appTheme = defaultAppTheme
	}
	gameType := strings.TrimSpace(req.GameType)
	if gameType == "" {
		gameType = defaultGameType
	}
	gameTypeStr := strings.TrimSpace(req.GameTypeStr)
	if gameTypeStr == "" {
		gameTypeStr = gameType
	}
	pvpType := defaultPVPType
	if req.PVPType != nil {
		pvpType = *req.PVPType
	}
	dataSource := defaultDataSource
	if req.DataSource != nil {
		dataSource = *req.DataSource
	}
	seasonID := strings.TrimSpace(req.CSGOSeasonID)
	if seasonID == "" {
		seasonID = defaultSeasonID
	}
	ts := strings.TrimSpace(req.Timestamp)
	if ts == "" {
		ts = strconv.FormatInt(time.Now().Unix(), 10)
	}
	timeout := time.Duration(req.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	customHeaders, err := parseCustomHeaders(req.CustomHeaders)
	if err != nil {
		return Step3Result{Summary: "自定义请求头无效", Error: err.Error(), SkippedItems: []SkippedMatchItem{}, Matches: []wanmei.WanmeiMatchItem{}}
	}

	payload := map[string]any{
		"pvpType":      pvpType,
		"dataSource":   dataSource,
		"page":         page,
		"csgoSeasonId": seasonID,
		"mySteamId":    steamIDInt,
		"toSteamId":    toSteamIDInt,
		"pageSize":     pageSize,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return Step3Result{Summary: "构建请求体失败", Error: err.Error(), SkippedItems: []SkippedMatchItem{}, Matches: []wanmei.WanmeiMatchItem{}}
	}

	httpReq, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return Step3Result{Summary: "创建请求失败", Error: err.Error(), SkippedItems: []SkippedMatchItem{}, Matches: []wanmei.WanmeiMatchItem{}}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "*/*")
	httpReq.Header.Set("token", token)
	httpReq.Header.Set("t", ts)
	httpReq.Header.Set("appversion", appVersion)
	httpReq.Header.Set("platform", platform)
	httpReq.Header.Set("appTheme", appTheme)
	httpReq.Header.Set("gameType", gameType)
	httpReq.Header.Set("gameTypeStr", gameTypeStr)
	applyCustomHeaders(httpReq, customHeaders)

	traceReq, traceResp := doTraceRequest(ctx, httpReq, timeout, true)
	result := Step3Result{
		MySteamID:    strconv.FormatInt(steamIDInt, 10),
		ToSteamID:    strconv.FormatInt(toSteamIDInt, 10),
		Request:      traceReq,
		Response:     traceResp,
		SkippedItems: []SkippedMatchItem{},
		Matches:      []wanmei.WanmeiMatchItem{},
	}

	if traceResp.Error != "" {
		result.Summary = "请求完美战绩接口失败"
		result.Error = traceResp.Error
		return result
	}
	if traceResp.StatusCode < 200 || traceResp.StatusCode >= 300 {
		result.Summary = fmt.Sprintf("战绩接口返回 HTTP %d（appversion=%s）", traceResp.StatusCode, appVersion)
		result.Error = fmt.Sprintf("HTTP %d: %s", traceResp.StatusCode, truncatePreview(traceResp.RawBody, 300))
		return result
	}

	var apiResp matchListAPIResponse
	if err := json.Unmarshal([]byte(traceResp.RawBody), &apiResp); err != nil {
		result.Summary = "解析战绩 JSON 失败"
		result.Error = err.Error()
		return result
	}

	result.APIStatusCode = apiResp.StatusCode
	result.APICode = apiResp.Code
	msg := strings.TrimSpace(apiResp.ErrorMessage)
	if msg == "" {
		msg = strings.TrimSpace(apiResp.Msg)
	}
	result.APIMessage = msg

	if apiResp.StatusCode != 0 || apiResp.Code != 0 {
		if msg == "" {
			msg = "未知业务错误"
		}
		result.Summary = fmt.Sprintf("战绩接口业务错误（statusCode=%d, code=%d, appversion=%s）", apiResp.StatusCode, apiResp.Code, appVersion)
		result.Error = msg
		return result
	}

	matches, skipped, rawCount, shape := parseMatchListDiagnostic(apiResp.Data)
	result.DataShape = shape
	result.RawItemCount = rawCount
	result.SkippedItems = skipped
	result.Matches = matches
	result.OK = true
	result.Summary = fmt.Sprintf("战绩列表正常（appversion=%s）：共 %d 条原始记录，成功解析 %d 条", appVersion, rawCount, len(matches))
	if len(skipped) > 0 {
		result.Summary += fmt.Sprintf("，跳过 %d 条", len(skipped))
	}
	return result
}

func parseMatchListDiagnostic(raw json.RawMessage) ([]wanmei.WanmeiMatchItem, []SkippedMatchItem, int, string) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return []wanmei.WanmeiMatchItem{}, []SkippedMatchItem{}, 0, "empty/null"
	}

	var wrapper struct {
		MatchList []matchRaw `json:"matchList"`
		List      []matchRaw `json:"list"`
	}
	var parsed []matchRaw
	shape := "unknown"
	if err := json.Unmarshal(raw, &wrapper); err == nil {
		if len(wrapper.MatchList) > 0 {
			parsed = wrapper.MatchList
			shape = "data.matchList"
		} else if len(wrapper.List) > 0 {
			parsed = wrapper.List
			shape = "data.list"
		}
	}
	if len(parsed) == 0 {
		var direct []matchRaw
		if err := json.Unmarshal(raw, &direct); err == nil {
			parsed = direct
			shape = "data[]"
		}
	}
	if len(parsed) == 0 {
		return []wanmei.WanmeiMatchItem{}, []SkippedMatchItem{}, 0, shape
	}

	matches := make([]wanmei.WanmeiMatchItem, 0, len(parsed))
	skipped := make([]SkippedMatchItem, 0)
	for _, item := range parsed {
		rawMatchID := strings.TrimSpace(item.MatchID)
		if rawMatchID == "" {
			rawMatchID = strings.TrimSpace(item.MatchIDAlt)
		}
		downloadMatchID, err := wanmei.ExtractNumericMatchID(rawMatchID)
		if err != nil {
			skipped = append(skipped, SkippedMatchItem{
				RawMatchID: rawMatchID,
				MapName:    strings.TrimSpace(item.MapName),
				EndTime:    strings.TrimSpace(item.EndTime),
				Reason:     err.Error(),
			})
			continue
		}
		matches = append(matches, wanmei.WanmeiMatchItem{
			MatchID:         rawMatchID,
			DownloadMatchID: downloadMatchID,
			MapName:         strings.TrimSpace(item.MapName),
			Score1:          item.Score1,
			Score2:          item.Score2,
			Kill:            item.Kill,
			Death:           item.Death,
			Assist:          item.Assist,
			K4:              parseFlexibleInt(item.K4),
			K5:              parseFlexibleInt(item.K5),
			Rating:          parseFlexibleFloat(item.Rating),
			EndTime:         strings.TrimSpace(item.EndTime),
		})
	}
	return matches, skipped, len(parsed), shape
}

func parseFlexibleInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
		if f, err := v.Float64(); err == nil {
			return int(f)
		}
	case string:
		s := strings.TrimSpace(v)
		if i, err := strconv.Atoi(s); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int(f)
		}
	}
	return 0
}

func parseFlexibleFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f
		}
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return 0
}

// Step4Request configures Step 4 public IP lookup.
type Step4Request struct {
	URL        string `json:"url"`
	TimeoutSec int    `json:"timeout_sec"`
}

// Step4Result is the Step 4 diagnostic result.
type Step4Result struct {
	OK        bool              `json:"ok"`
	Summary   string            `json:"summary"`
	Error     string            `json:"error,omitempty"`
	Request   HTTPTraceRequest  `json:"request"`
	Response  HTTPTraceResponse `json:"response"`
	IP        string            `json:"ip"`
	ValidIPv4 bool              `json:"valid_ipv4"`
}

// RunStep4 executes Step 4: fetches public IPv4 used by X-PWA-Signature.
func RunStep4(ctx context.Context, req Step4Request) Step4Result {
	targetURL := strings.TrimSpace(req.URL)
	if targetURL == "" {
		targetURL = defaultPublicIPAPI
	}
	timeout := time.Duration(req.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	httpReq, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return Step4Result{Summary: "创建公网 IP 请求失败", Error: err.Error()}
	}
	traceReq, traceResp := doTraceRequest(ctx, httpReq, timeout, true)
	result := Step4Result{
		Request:  traceReq,
		Response: traceResp,
	}

	if traceResp.Error != "" {
		result.Summary = "请求公网 IP 接口失败"
		result.Error = traceResp.Error
		return result
	}
	if traceResp.StatusCode != http.StatusOK {
		result.Summary = fmt.Sprintf("公网 IP 接口返回状态码 %d", traceResp.StatusCode)
		result.Error = fmt.Sprintf("HTTP %d: %s", traceResp.StatusCode, truncatePreview(traceResp.RawBody, 200))
		return result
	}

	ip := strings.TrimSpace(traceResp.RawBody)
	result.IP = ip
	result.ValidIPv4 = ipv4Regex.MatchString(ip)
	if !result.ValidIPv4 {
		result.Summary = "返回内容不是合法 IPv4"
		result.Error = fmt.Sprintf("无效的公网 IP 格式: %q", ip)
		return result
	}

	result.OK = true
	result.Summary = fmt.Sprintf("公网 IPv4 获取成功：%s", ip)
	return result
}

// Step5Request configures Step 5 signed demo URL generation.
type Step5Request struct {
	DemoBaseURL      string `json:"demo_base_url"`
	MatchID          string `json:"match_id"`
	CupID            string `json:"cup_id"`
	AccessToken      string `json:"access_token"`
	AppID            string `json:"app_id"`
	Secret           string `json:"secret"`
	Randnum          string `json:"randnum"`
	Timestamp        string `json:"timestamp"`
	SkipNumericCheck bool   `json:"skip_numeric_check"`
}

// Step5Result exposes every crypto intermediate of buildSignedDemoURL.
type Step5Result struct {
	OK              bool   `json:"ok"`
	Summary         string `json:"summary"`
	Error           string `json:"error,omitempty"`
	RawMatchID      string `json:"raw_match_id"`
	DownloadMatchID string `json:"download_match_id"`
	Path            string `json:"path"`
	QueryString     string `json:"query_string"`
	Randnum         string `json:"randnum"`
	Timestamp       string `json:"timestamp"`
	MD5Input        string `json:"md5_input"`
	MD5Hex          string `json:"md5_hex"`
	SHA1Input       string `json:"sha1_input"`
	SHA1Hex         string `json:"sha1_hex"`
	SignedURL       string `json:"signed_url"`
}

// RunStep5 executes Step 5: deterministic/traceable signed demo URL generation.
func RunStep5(req Step5Request) Step5Result {
	rawMatchID := strings.TrimSpace(req.MatchID)
	res := Step5Result{RawMatchID: rawMatchID}
	if rawMatchID == "" {
		res.Summary = "match_id 为空"
		res.Error = "请先在 Step 3 选择战绩，或手动输入 match_id / PVP@..."
		return res
	}
	accessToken := strings.TrimSpace(req.AccessToken)
	if accessToken == "" {
		res.Summary = "access_token 为空"
		res.Error = "请先完成 Step 2 或手动填入 access_token"
		return res
	}

	downloadMatchID := rawMatchID
	if !req.SkipNumericCheck {
		normalized, err := wanmei.ExtractNumericMatchID(rawMatchID)
		if err != nil {
			res.Summary = "match_id 格式无效"
			res.Error = err.Error()
			return res
		}
		downloadMatchID = normalized
	}

	baseURL := strings.TrimSpace(req.DemoBaseURL)
	if baseURL == "" {
		baseURL = defaultDemoBaseURL
	}
	cupID := strings.TrimSpace(req.CupID)
	if cupID == "" {
		cupID = defaultCupID
	}
	appID := strings.TrimSpace(req.AppID)
	if appID == "" {
		appID = defaultDemoAppID
	}
	secret := strings.TrimSpace(req.Secret)
	if secret == "" {
		secret = defaultDemoSecret
	}
	randnum := strings.TrimSpace(req.Randnum)
	if randnum == "" {
		randnum = strconv.Itoa(100000 + int(time.Now().UnixNano())%900000)
	}
	ts := strings.TrimSpace(req.Timestamp)
	if ts == "" {
		ts = strconv.FormatInt(time.Now().Unix(), 10)
	}

	params := map[string]string{
		"access_token": accessToken,
		"cup_id":       cupID,
		"match_id":     downloadMatchID,
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	queryParts := make([]string, 0, len(keys))
	for _, k := range keys {
		queryParts = append(queryParts, k+"="+params[k])
	}
	queryString := strings.Join(queryParts, "&")

	md5Input := randnum + ts + queryString
	md5Sum := md5.Sum([]byte(md5Input))
	md5Hex := hex.EncodeToString(md5Sum[:])

	sha1Input := appID + md5Hex + secret
	sha1Sum := sha1.Sum([]byte(sha1Input))
	sha1Hex := hex.EncodeToString(sha1Sum[:])

	path := "/csgo/demo/" + downloadMatchID + "_" + cupID + ".dem"
	root := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	signedQS := "a=" + appID + "&r=" + randnum + "&s=" + sha1Hex + "&t=" + ts
	if queryString != "" {
		signedQS += "&" + queryString
	}
	signedURL := root + "?" + signedQS

	res.OK = true
	res.DownloadMatchID = downloadMatchID
	res.Path = path
	res.QueryString = queryString
	res.Randnum = randnum
	res.Timestamp = ts
	res.MD5Input = md5Input
	res.MD5Hex = md5Hex
	res.SHA1Input = sha1Input
	res.SHA1Hex = sha1Hex
	res.SignedURL = signedURL
	res.Summary = fmt.Sprintf("已生成签名 URL（match_id=%s, cup_id=%s, appid=%s）", downloadMatchID, cupID, appID)
	return res
}

// Step6Request configures Step 6 OSS URL resolution.
type Step6Request struct {
	SignedURL       string `json:"signed_url"`
	SteamID         string `json:"steam_id"`
	IPAddr          string `json:"ip_addr"`
	ServerTime      string `json:"server_time"`
	Referer         string `json:"referer"`
	ManualSignature string `json:"manual_signature"`
	CustomHeaders   string `json:"custom_headers"`
	FollowRedirects bool   `json:"follow_redirects"`
	TimeoutSec      int    `json:"timeout_sec"`
}

// Step6CryptoTrace records AES-128-CBC key/IV derivation for X-PWA-Signature.
type Step6CryptoTrace struct {
	SteamID    string `json:"steam_id"`
	IPAddr     string `json:"ip_addr"`
	ServerTime string `json:"server_time"`
	AESKey     string `json:"aes_key"`
	AESIV      string `json:"aes_iv"`
	Signature  string `json:"signature"`
}

// Step6Result is the Step 6 diagnostic result.
type Step6Result struct {
	OK           bool              `json:"ok"`
	Summary      string            `json:"summary"`
	Error        string            `json:"error,omitempty"`
	Crypto       Step6CryptoTrace  `json:"crypto"`
	Request      HTTPTraceRequest  `json:"request"`
	Response     HTTPTraceResponse `json:"response"`
	Location     string            `json:"location,omitempty"`
	OSSURL       string            `json:"oss_url,omitempty"`
	PrettyJSON   string            `json:"pretty_json,omitempty"`
	DetectedURLs []string          `json:"detected_urls,omitempty"`
}

// RunStep6 executes Step 6: sends X-PWA-* headers and inspects 302 Location vs 200 JSON.
func RunStep6(ctx context.Context, req Step6Request) Step6Result {
	signedURL := strings.TrimSpace(req.SignedURL)
	if signedURL == "" {
		return Step6Result{Summary: "signed_url 为空", Error: "请先生成 Step 5 签名 URL 或手动输入"}
	}
	steamID := strings.TrimSpace(req.SteamID)
	ipAddr := strings.TrimSpace(req.IPAddr)
	serverTime := strings.TrimSpace(req.ServerTime)
	referer := strings.TrimSpace(req.Referer)
	if referer == "" {
		referer = defaultPWAReferer
	}
	timeout := time.Duration(req.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	customHeaders, err := parseCustomHeaders(req.CustomHeaders)
	if err != nil {
		return Step6Result{Summary: "自定义请求头无效", Error: err.Error()}
	}

	var cryptoTrace Step6CryptoTrace
	sig := strings.TrimSpace(req.ManualSignature)
	if sig != "" {
		cryptoTrace = Step6CryptoTrace{
			SteamID:    steamID,
			IPAddr:     ipAddr,
			ServerTime: serverTime,
			Signature:  sig,
		}
	} else {
		if steamID == "" || ipAddr == "" || serverTime == "" {
			return Step6Result{
				Summary: "缺少签名参数",
				Error:   "需要 steam_id、ip_addr、server_time（或填写 manual_signature）",
			}
		}
		var err error
		cryptoTrace, err = buildXPWASignatureTrace(steamID, ipAddr, serverTime)
		if err != nil {
			return Step6Result{
				Summary: "构建 X-PWA-Signature 失败",
				Error:   err.Error(),
				Crypto:  cryptoTrace,
			}
		}
		sig = cryptoTrace.Signature
	}

	httpReq, err := http.NewRequest(http.MethodGet, signedURL, nil)
	if err != nil {
		return Step6Result{Summary: "创建请求失败", Error: err.Error(), Crypto: cryptoTrace}
	}
	if steamID != "" {
		httpReq.Header.Set("X-PWA-SteamId", steamID)
		httpReq.Header.Set("PwaSteamId", steamID)
	}
	httpReq.Header.Set("X-PWA-Signature", sig)
	httpReq.Header.Set("Referer", referer)
	applyCustomHeaders(httpReq, customHeaders)

	traceReq, traceResp := doTraceRequest(ctx, httpReq, timeout, req.FollowRedirects)
	result := Step6Result{
		Crypto:   cryptoTrace,
		Request:  traceReq,
		Response: traceResp,
	}

	if traceResp.Error != "" {
		result.Summary = "请求签名 URL 失败"
		result.Error = traceResp.Error
		return result
	}

	if pretty := prettyJSONIfValid(traceResp.RawBody); pretty != "" {
		result.PrettyJSON = pretty
	}
	result.DetectedURLs = detectURLsInResponse(traceResp.RawBody)

	if traceResp.StatusCode >= 300 && traceResp.StatusCode < 400 {
		rawLoc := firstHeaderValue(traceResp.Headers, "Location")
		result.Location = rawLoc
		if rawLoc == "" {
			result.Summary = fmt.Sprintf("收到 HTTP %d 重定向，但缺少 Location 头", traceResp.StatusCode)
			result.Error = "重定向响应缺少 Location 头"
			return result
		}
		resolved := rawLoc
		if parsedLoc, parseErr := url.Parse(rawLoc); parseErr == nil {
			if baseParsed, baseErr := url.Parse(signedURL); baseErr == nil {
				resolved = baseParsed.ResolveReference(parsedLoc).String()
			}
		}
		result.OK = true
		result.OSSURL = resolved
		result.Summary = fmt.Sprintf("收到 HTTP %d 重定向，成功获取 OSS 直链", traceResp.StatusCode)
		return result
	}

	result.Summary = fmt.Sprintf("未收到 3xx 重定向，实际返回 HTTP %d（现行代码会报错：未收到重定向响应，状态码: %d）", traceResp.StatusCode, traceResp.StatusCode)
	result.Error = fmt.Sprintf("未收到重定向响应，状态码: %d", traceResp.StatusCode)
	if len(result.DetectedURLs) > 0 {
		result.Summary += fmt.Sprintf("；但在响应体中发现 %d 个候选 URL", len(result.DetectedURLs))
	}
	return result
}

func buildXPWASignatureTrace(steamID, ipAddr, serverTime string) (Step6CryptoTrace, error) {
	trace := Step6CryptoTrace{
		SteamID:    steamID,
		IPAddr:     ipAddr,
		ServerTime: serverTime,
	}
	if len(steamID) < 16 {
		return trace, fmt.Errorf("steam_id 长度不足 16 位")
	}

	ts := serverTime
	keyOffset := len(ts) - 16
	if keyOffset < 0 {
		keyOffset = len(steamID) + keyOffset
	}
	if keyOffset < 0 || keyOffset > len(steamID) {
		return trace, fmt.Errorf("无法从 server_time(len=%d) 与 steam_id(len=%d) 计算 AES key 偏移", len(ts), len(steamID))
	}
	keyText := ts + steamID[keyOffset:]
	ivText := steamID[len(steamID)-16:]
	trace.AESKey = keyText
	trace.AESIV = ivText

	key := []byte(keyText)
	iv := []byte(ivText)
	if len(key) != 16 {
		return trace, fmt.Errorf("无效的 AES key 长度: %d（期望 16）", len(key))
	}
	if len(iv) != 16 {
		return trace, fmt.Errorf("无效的 AES IV 长度: %d（期望 16）", len(iv))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return trace, fmt.Errorf("创建 AES cipher 失败: %w", err)
	}

	padded := pkcs7Pad([]byte(ipAddr), aes.BlockSize)
	encrypted := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(encrypted, padded)

	trace.Signature = ts + "-" + hex.EncodeToString(encrypted)
	return trace, nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	pad := bytes.Repeat([]byte{byte(padLen)}, padLen)
	return append(data, pad...)
}

func firstHeaderValue(headers map[string][]string, name string) string {
	for k, vals := range headers {
		if strings.EqualFold(k, name) && len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}

func prettyJSONIfValid(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || (!strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[")) {
		return ""
	}
	var anyVal any
	if err := json.Unmarshal([]byte(trimmed), &anyVal); err != nil {
		return ""
	}
	formatted, err := json.MarshalIndent(anyVal, "", "  ")
	if err != nil {
		return ""
	}
	return string(formatted)
}

func detectURLsInResponse(raw string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	add := func(candidate string) {
		c := strings.TrimSpace(candidate)
		if c == "" {
			return
		}
		if _, ok := seen[c]; ok {
			return
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}

	var decoded any
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &decoded) == nil {
		var walk func(any)
		walk = func(node any) {
			switch v := node.(type) {
			case map[string]any:
				keys := make([]string, 0, len(v))
				for k := range v {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					walk(v[k])
				}
			case []any:
				for _, item := range v {
					walk(item)
				}
			case string:
				if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
					add(v)
				}
			}
		}
		walk(decoded)
	}

	for _, match := range httpURLRegex.FindAllString(raw, 10) {
		add(match)
	}
	return out
}

// FileSniffResult describes the magic-byte classification of a downloaded file or head sample.
type FileSniffResult struct {
	Format        string `json:"format"`
	Description   string `json:"description"`
	IsExpectedZip bool   `json:"is_expected_zip"`
	HexPreview    string `json:"hex_preview"`
	ASCIIPreview  string `json:"ascii_preview"`
	TextPreview   string `json:"text_preview,omitempty"`
}

// SniffBytes classifies content by magic bytes / structural prefix.
func SniffBytes(data []byte) FileSniffResult {
	sample := data
	if len(sample) > 32 {
		sample = sample[:32]
	}
	res := FileSniffResult{
		HexPreview:   formatHexSpaced(sample),
		ASCIIPreview: formatASCIIPreview(sample),
	}

	switch {
	case len(data) >= 4 && (bytes.HasPrefix(data, []byte{0x50, 0x4b, 0x03, 0x04}) ||
		bytes.HasPrefix(data, []byte{0x50, 0x4b, 0x05, 0x06}) ||
		bytes.HasPrefix(data, []byte{0x50, 0x4b, 0x01, 0x02})):
		res.Format = "ZIP"
		res.Description = "ZIP 压缩包（与现行 internal/wanmei UnzipWithContext 一致）"
		res.IsExpectedZip = true
	case len(data) >= 6 && bytes.HasPrefix(data, []byte{0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c}):
		res.Format = "7Z"
		res.Description = "7z 压缩包（注意：internal/wanmei 使用 UnzipWithContext，遇到 7z 会失败）"
	case len(data) >= 7 && (bytes.HasPrefix(data, []byte("PBDEMS2")) || bytes.HasPrefix(data, []byte("HL2DEMO"))):
		res.Format = "RAW_DEM"
		res.Description = "直接返回了未压缩的 .dem 文件（注意：internal/wanmei 调用 UnzipWithContext 会失败）"
	case len(data) >= 2 && bytes.HasPrefix(data, []byte{0x1f, 0x8b}):
		res.Format = "GZIP"
		res.Description = "GZIP 压缩文件"
	case len(data) >= 3 && bytes.HasPrefix(data, []byte("BZh")):
		res.Format = "BZIP2"
		res.Description = "BZIP2 压缩文件"
	case len(data) >= 6 && bytes.HasPrefix(data, []byte{0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00}):
		res.Format = "XZ"
		res.Description = "XZ 压缩文件"
	case len(data) >= 6 && bytes.HasPrefix(data, []byte("Rar!\x1a\x07")):
		res.Format = "RAR"
		res.Description = "RAR 压缩包"
	case len(data) >= 4 && bytes.HasPrefix(data, []byte{0x28, 0xb5, 0x2f, 0xfd}):
		res.Format = "ZSTD"
		res.Description = "Zstandard 压缩文件"
	default:
		trimmed := bytes.TrimSpace(data)
		switch {
		case len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && utf8.Valid(trimmed):
			res.Format = "JSON"
			res.Description = "返回了 JSON 文本（可能是鉴权或错误信息）"
			res.TextPreview = truncatePreview(string(data), 2048)
		case len(trimmed) > 0 && trimmed[0] == '<' && utf8.Valid(trimmed):
			res.Format = "XML_OR_HTML"
			res.Description = "返回了 XML / HTML 页面（常见于 OSS 签名过期或 403/404 错误）"
			res.TextPreview = truncatePreview(string(data), 2048)
		case len(data) == 0:
			res.Format = "EMPTY"
			res.Description = "文件为空"
		case utf8.Valid(data):
			res.Format = "TEXT"
			res.Description = "纯文本响应"
			res.TextPreview = truncatePreview(string(data), 2048)
		default:
			res.Format = "UNKNOWN"
			res.Description = "未知二进制格式"
		}
	}
	return res
}

func formatHexSpaced(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	parts := make([]string, len(data))
	for i, b := range data {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	return strings.Join(parts, " ")
}

func formatASCIIPreview(data []byte) string {
	var b strings.Builder
	for _, ch := range data {
		if ch >= 32 && ch <= 126 {
			b.WriteByte(ch)
		} else {
			b.WriteByte('.')
		}
	}
	return b.String()
}

func truncatePreview(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}

// Step7HeadRequest probes OSS response headers and initial bytes without saving the whole file.
type Step7HeadRequest struct {
	OSSURL     string `json:"oss_url"`
	TimeoutSec int    `json:"timeout_sec"`
}

// Step7HeadResult is the quick peek probe result for Step 7.
type Step7HeadResult struct {
	OK                 bool                `json:"ok"`
	Summary            string              `json:"summary"`
	Error              string              `json:"error,omitempty"`
	Request            HTTPTraceRequest    `json:"request"`
	StatusCode         int                 `json:"status_code"`
	Status             string              `json:"status,omitempty"`
	Headers            map[string][]string `json:"headers,omitempty"`
	ContentLength      int64               `json:"content_length"`
	ContentType        string              `json:"content_type,omitempty"`
	ContentDisposition string              `json:"content_disposition,omitempty"`
	PeekBytes          int                 `json:"peek_bytes"`
	ElapsedMs          int64               `json:"elapsed_ms"`
	Sniff              FileSniffResult     `json:"sniff"`
}

// RunStep7ProbeHead fetches response headers and up to 4 KiB using the same GET headers as download.FileWithContext.
func RunStep7ProbeHead(ctx context.Context, req Step7HeadRequest) Step7HeadResult {
	ossURL := strings.TrimSpace(req.OSSURL)
	if ossURL == "" {
		return Step7HeadResult{Summary: "oss_url 为空", Error: "请先通过 Step 6 获取或手动输入 OSS 下载地址"}
	}
	timeout := time.Duration(req.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, ossURL, nil)
	if err != nil {
		return Step7HeadResult{Summary: "创建下载探测请求失败", Error: err.Error()}
	}
	httpReq.Header.Set("User-Agent", downloadUserAgent)

	traceReq := HTTPTraceRequest{
		Method:  httpReq.Method,
		URL:     httpReq.URL.String(),
		Headers: map[string]string{"User-Agent": downloadUserAgent},
	}

	client := &http.Client{}
	started := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		return Step7HeadResult{
			Request:   traceReq,
			ElapsedMs: time.Since(started).Milliseconds(),
			Summary:   "请求 OSS 直链失败",
			Error:     err.Error(),
		}
	}
	defer resp.Body.Close()

	sample, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	// Cancel immediately after reading the leading 4 KiB so multi-hundred-MB demos stop transferring.
	cancel()
	elapsed := time.Since(started).Milliseconds()

	headers := make(map[string][]string, len(resp.Header))
	for k, v := range resp.Header {
		copied := make([]string, len(v))
		copy(copied, v)
		headers[k] = copied
	}

	sniff := SniffBytes(sample)
	result := Step7HeadResult{
		Request:            traceReq,
		StatusCode:         resp.StatusCode,
		Status:             resp.Status,
		Headers:            headers,
		ContentLength:      resp.ContentLength,
		ContentType:        resp.Header.Get("Content-Type"),
		ContentDisposition: resp.Header.Get("Content-Disposition"),
		PeekBytes:          len(sample),
		ElapsedMs:          elapsed,
		Sniff:              sniff,
	}

	if readErr != nil && !errors.Is(readErr, context.Canceled) {
		result.Summary = "读取响应头/前 4KB 失败"
		result.Error = readErr.Error()
		return result
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Summary = fmt.Sprintf("OSS 返回 HTTP %d（格式: %s）", resp.StatusCode, sniff.Format)
		result.Error = fmt.Sprintf("下载失败，状态码: %d", resp.StatusCode)
		return result
	}

	result.OK = true
	result.Summary = fmt.Sprintf("OSS HTTP %d，总大小 %s，魔数识别: %s（%s）",
		resp.StatusCode, formatByteSize(resp.ContentLength), sniff.Format, sniff.Description)
	return result
}

// ArchiveEntry summarizes one entry inside a zip/7z archive.
type ArchiveEntry struct {
	Name             string `json:"name"`
	UncompressedSize uint64 `json:"uncompressed_size"`
	IsDir            bool   `json:"is_dir"`
}

// Step7WanmeiPipelineResult captures the exact internal/wanmei Step 7 execution stages.
type Step7WanmeiPipelineResult struct {
	DownloadOK       bool           `json:"download_ok"`
	DownloadError    string         `json:"download_error,omitempty"`
	DownloadMs       int64          `json:"download_ms"`
	UnzipOK          bool           `json:"unzip_ok"`
	UnzipError       string         `json:"unzip_error,omitempty"`
	UnzipMs          int64          `json:"unzip_ms"`
	ArchiveEntries   []ArchiveEntry `json:"archive_entries,omitempty"`
	FindDemOK        bool           `json:"find_dem_ok"`
	FindDemError     string         `json:"find_dem_error,omitempty"`
	DemPath          string         `json:"dem_path,omitempty"`
	DemSize          int64          `json:"dem_size"`
	LikelyDemoOK     bool           `json:"likely_demo_ok"`
	LikelyDemoError  string         `json:"likely_demo_error,omitempty"`
	LikelyDemoStamp  string         `json:"likely_demo_stamp,omitempty"`
	CommitOK         bool           `json:"commit_ok"`
	CommitError      string         `json:"commit_error,omitempty"`
	StableCommitPath string         `json:"stable_commit_path,omitempty"`
}

// Step7FallbackDiagnostic reports whether 7z or raw .dem handling would succeed when UnzipWithContext fails.
type Step7FallbackDiagnostic struct {
	Attempted      bool           `json:"attempted"`
	FormatDetected string         `json:"format_detected,omitempty"`
	Success        bool           `json:"success"`
	Message        string         `json:"message,omitempty"`
	Entries        []ArchiveEntry `json:"entries,omitempty"`
	DemPath        string         `json:"dem_path,omitempty"`
	DemSize        int64          `json:"dem_size"`
	LikelyDemoOK   bool           `json:"likely_demo_ok"`
	Error          string         `json:"error,omitempty"`
}

// Step7Status describes a full Step 7 download & extraction task.
type Step7Status struct {
	TaskID           string                    `json:"task_id"`
	Kind             string                    `json:"kind"` // "step7" | "native_import"
	Running          bool                      `json:"running"`
	OK               bool                      `json:"ok"`
	Phase            string                    `json:"phase"`
	Summary          string                    `json:"summary"`
	Error            string                    `json:"error,omitempty"`
	ProgressPercent  float64                   `json:"progress_percent"`
	Indeterminate    bool                      `json:"indeterminate"`
	DownloadedBytes  int64                     `json:"downloaded_bytes"`
	ElapsedMs        int64                     `json:"elapsed_ms"`
	WorkSubDir       string                    `json:"work_subdir,omitempty"`
	ArchivePath      string                    `json:"archive_path,omitempty"`
	ArchiveSize      int64                     `json:"archive_size"`
	Sniff            *FileSniffResult          `json:"sniff,omitempty"`
	WanmeiPipeline   Step7WanmeiPipelineResult `json:"wanmei_pipeline"`
	FallbackDiag     Step7FallbackDiagnostic   `json:"fallback_diag"`
	NativeImportPath string                    `json:"native_import_path,omitempty"`
}

// Step7StartRequest configures a full Step 7 download and extraction task.
type Step7StartRequest struct {
	OSSURL          string `json:"oss_url"`
	MatchID         string `json:"match_id"`
	RunFallbackDiag bool   `json:"run_fallback_diag"`
}

// NativeImportRequest runs unmodified wanmei.ImportDemoContext into an isolated temp directory.
type NativeImportRequest struct {
	MatchID string `json:"match_id"`
}

// Step7Manager manages active download/extraction tasks inside an isolated temp workDir.
type Step7Manager struct {
	workDir string

	mu     sync.Mutex
	seq    int64
	cancel context.CancelFunc
	doneCh chan struct{}
	status Step7Status
}

// NewStep7Manager creates a task manager rooted at workDir.
func NewStep7Manager(workDir string) *Step7Manager {
	return &Step7Manager{
		workDir: workDir,
		status: Step7Status{
			Phase:   "idle",
			Summary: "尚未开始 Step 7 下载测试",
		},
	}
}

// StartStep7 cancels any previous task, cleans old temp artifacts, and runs Step 7 asynchronously.
func (m *Step7Manager) StartStep7(req Step7StartRequest) (Step7Status, error) {
	ossURL := strings.TrimSpace(req.OSSURL)
	if ossURL == "" {
		return Step7Status{}, fmt.Errorf("oss_url 不能为空")
	}
	matchID := sanitizeFileStem(req.MatchID)
	if matchID == "" {
		matchID = "probe"
	}

	taskID, runDir, ctx, done := m.beginTask("step7", "正在下载 OSS 文件...")

	go func() {
		defer close(done)
		m.runStep7Task(ctx, taskID, runDir, ossURL, matchID, req.RunFallbackDiag)
	}()

	return m.Status(taskID), nil
}

// StartNativeImport runs unmodified wanmei.ImportDemoContext into a fresh temp subdir.
func (m *Step7Manager) StartNativeImport(req NativeImportRequest) (Step7Status, error) {
	numericID, err := wanmei.ExtractNumericMatchID(req.MatchID)
	if err != nil {
		return Step7Status{}, err
	}

	taskID, runDir, ctx, done := m.beginTask("native_import", "正在执行原生 wanmei.ImportDemoContext...")

	go func() {
		defer close(done)
		started := time.Now()
		cacheRoot := filepath.Join(runDir, numericID)
		path, importErr := wanmei.ImportDemoContext(ctx, numericID, cacheRoot, func(active bool, percent float64, indeterminate bool) {
			m.update(taskID, func(st *Step7Status) {
				if active {
					st.ProgressPercent = percent
					st.Indeterminate = indeterminate
				}
				st.ElapsedMs = time.Since(started).Milliseconds()
			})
		})

		m.update(taskID, func(st *Step7Status) {
			st.Running = false
			st.ElapsedMs = time.Since(started).Milliseconds()
			if importErr != nil {
				if errors.Is(importErr, context.Canceled) || errors.Is(importErr, download.ErrCanceled) {
					st.Phase = "canceled"
					st.Summary = "原生 ImportDemoContext 已取消"
				} else {
					st.Phase = "failed"
					st.Summary = "原生 ImportDemoContext 失败"
				}
				st.Error = importErr.Error()
				return
			}
			var size int64
			if info, statErr := os.Stat(path); statErr == nil {
				size = info.Size()
			}
			st.OK = true
			st.Phase = "done"
			st.ProgressPercent = 100
			st.Indeterminate = false
			st.NativeImportPath = path
			st.WanmeiPipeline.DemPath = path
			st.WanmeiPipeline.DemSize = size
			st.Summary = fmt.Sprintf("原生 wanmei.ImportDemoContext 全链路通过！输出: %s (%s)", filepath.Base(path), formatByteSize(size))
		})
	}()

	return m.Status(taskID), nil
}

func (m *Step7Manager) beginTask(kind string, summary string) (string, string, context.Context, chan struct{}) {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	prevDone := m.doneCh
	m.mu.Unlock()

	if prevDone != nil {
		<-prevDone
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cleanupWorkDirChildren(m.workDir)

	m.seq++
	taskID := fmt.Sprintf("%s-%d", kind, m.seq)
	runDir, err := os.MkdirTemp(m.workDir, taskID+"-*")
	if err != nil {
		runDir = filepath.Join(m.workDir, taskID)
		_ = os.MkdirAll(runDir, 0755)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	m.cancel = cancel
	m.doneCh = done
	m.status = Step7Status{
		TaskID:        taskID,
		Kind:          kind,
		Running:       true,
		Phase:         "downloading",
		Summary:       summary,
		Indeterminate: true,
		WorkSubDir:    runDir,
	}
	return taskID, runDir, ctx, done
}

func (m *Step7Manager) update(taskID string, fn func(*Step7Status)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.TaskID != taskID {
		return
	}
	fn(&m.status)
}

func (m *Step7Manager) runStep7Task(ctx context.Context, taskID, runDir, ossURL, matchID string, runFallback bool) {
	started := time.Now()
	archivePath := filepath.Join(runDir, matchID+"_0.zip")
	extractDir := filepath.Join(runDir, "extract")
	stablePath := filepath.Join(runDir, matchID+".dem")

	m.update(taskID, func(st *Step7Status) {
		st.ArchivePath = archivePath
		st.Phase = "downloading"
		st.Summary = "正在使用 download.FileWithContext 下载..."
	})

	dlStart := time.Now()
	dlErr := download.FileWithContext(ctx, ossURL, archivePath, func(active bool, percent float64, indeterminate bool) {
		m.update(taskID, func(st *Step7Status) {
			if active {
				st.ProgressPercent = percent
				st.Indeterminate = indeterminate
			}
			st.ElapsedMs = time.Since(started).Milliseconds()
		})
	})
	dlMs := time.Since(dlStart).Milliseconds()

	if dlErr != nil {
		m.update(taskID, func(st *Step7Status) {
			st.Running = false
			st.ElapsedMs = time.Since(started).Milliseconds()
			st.WanmeiPipeline.DownloadMs = dlMs
			st.WanmeiPipeline.DownloadError = dlErr.Error()
			if errors.Is(dlErr, context.Canceled) || errors.Is(dlErr, download.ErrCanceled) {
				st.Phase = "canceled"
				st.Summary = "Step 7 下载已取消"
			} else {
				st.Phase = "failed"
				st.Summary = "Step 7 下载失败"
			}
			st.Error = dlErr.Error()
		})
		return
	}

	var archiveSize int64
	if info, err := os.Stat(archivePath); err == nil {
		archiveSize = info.Size()
	}

	sniff := sniffFileHead(archivePath)
	m.update(taskID, func(st *Step7Status) {
		st.WanmeiPipeline.DownloadOK = true
		st.WanmeiPipeline.DownloadMs = dlMs
		st.ArchiveSize = archiveSize
		st.DownloadedBytes = archiveSize
		st.ProgressPercent = 100
		st.Indeterminate = false
		st.Sniff = &sniff
		st.Phase = "unzipping"
		st.Summary = fmt.Sprintf("下载完成 (%s, 识别格式: %s)，正在使用 download.UnzipWithContext 解压...",
			formatByteSize(archiveSize), sniff.Format)
		st.ElapsedMs = time.Since(started).Milliseconds()
	})

	_ = os.MkdirAll(extractDir, 0755)
	entries, _ := listZipEntries(archivePath)

	unzipStart := time.Now()
	unzipErr := download.UnzipWithContext(ctx, archivePath, extractDir)
	unzipMs := time.Since(unzipStart).Milliseconds()

	if unzipErr != nil {
		fallback := Step7FallbackDiagnostic{}
		if runFallback {
			fallback = runFallbackArchiveCheck(ctx, runDir, archivePath, sniff.Format)
		}
		m.update(taskID, func(st *Step7Status) {
			st.Running = false
			st.ElapsedMs = time.Since(started).Milliseconds()
			st.WanmeiPipeline.UnzipMs = unzipMs
			st.WanmeiPipeline.UnzipError = unzipErr.Error()
			st.WanmeiPipeline.ArchiveEntries = entries
			st.FallbackDiag = fallback
			if errors.Is(unzipErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				st.Phase = "canceled"
				st.Summary = "Step 7 解压已取消"
			} else {
				st.Phase = "failed"
				st.Summary = fmt.Sprintf("download.UnzipWithContext 解压失败（文件格式识别为 %s）", sniff.Format)
				if fallback.Success {
					st.Summary += "；" + fallback.Message
				}
			}
			st.Error = unzipErr.Error()
		})
		return
	}

	m.update(taskID, func(st *Step7Status) {
		st.WanmeiPipeline.UnzipOK = true
		st.WanmeiPipeline.UnzipMs = unzipMs
		st.WanmeiPipeline.ArchiveEntries = entries
		st.Phase = "finding_dem"
		st.Summary = "解压成功，正在查找 .dem 文件..."
		st.ElapsedMs = time.Since(started).Milliseconds()
	})

	demPath, findErr := download.FindFirstByExt(extractDir, ".dem")
	if findErr != nil {
		m.update(taskID, func(st *Step7Status) {
			st.Running = false
			st.Phase = "failed"
			st.ElapsedMs = time.Since(started).Milliseconds()
			st.WanmeiPipeline.FindDemError = findErr.Error()
			st.Summary = "ZIP 解压成功，但未在压缩包内找到 .dem 文件"
			st.Error = findErr.Error()
		})
		return
	}

	var demSize int64
	if info, err := os.Stat(demPath); err == nil {
		demSize = info.Size()
	}

	likely, likelyErr := download.IsLikelyDemoFile(demPath)
	stamp := readDemoStamp(demPath)

	m.update(taskID, func(st *Step7Status) {
		st.WanmeiPipeline.FindDemOK = true
		st.WanmeiPipeline.DemPath = demPath
		st.WanmeiPipeline.DemSize = demSize
		st.WanmeiPipeline.LikelyDemoOK = likely
		st.WanmeiPipeline.LikelyDemoStamp = stamp
		if likelyErr != nil {
			st.WanmeiPipeline.LikelyDemoError = likelyErr.Error()
		}
		st.Phase = "verifying_dem"
		st.ElapsedMs = time.Since(started).Milliseconds()
	})

	if !likely {
		m.update(taskID, func(st *Step7Status) {
			st.Running = false
			st.Phase = "failed"
			st.ElapsedMs = time.Since(started).Milliseconds()
			st.Summary = fmt.Sprintf("已提取 %s (%s)，但文件头校验未通过（stamp=%q）",
				filepath.Base(demPath), formatByteSize(demSize), stamp)
			if likelyErr != nil {
				st.Error = likelyErr.Error()
			} else {
				st.Error = fmt.Sprintf("未检测到 PBDEMS2/HL2DEMO 文件头（实际前缀: %q）", stamp)
			}
		})
		return
	}

	if err := download.CopyFileAtomicWithContext(ctx, demPath, stablePath); err != nil {
		m.update(taskID, func(st *Step7Status) {
			st.Running = false
			st.Phase = "failed"
			st.ElapsedMs = time.Since(started).Milliseconds()
			st.WanmeiPipeline.CommitError = err.Error()
			st.Summary = "已找到合法 .dem，但原子提交复制失败"
			st.Error = err.Error()
		})
		return
	}

	m.update(taskID, func(st *Step7Status) {
		st.Running = false
		st.OK = true
		st.Phase = "done"
		st.ElapsedMs = time.Since(started).Milliseconds()
		st.WanmeiPipeline.CommitOK = true
		st.WanmeiPipeline.StableCommitPath = stablePath
		st.Summary = fmt.Sprintf("Step 7 全流程验证通过！.dem 文件 %s（%s，文件头: %s）",
			filepath.Base(demPath), formatByteSize(demSize), stamp)
	})
}

func runFallbackArchiveCheck(ctx context.Context, runDir, archivePath, format string) Step7FallbackDiagnostic {
	diag := Step7FallbackDiagnostic{
		FormatDetected: format,
	}
	switch format {
	case "7Z":
		diag.Attempted = true
		fallbackDir := filepath.Join(runDir, "extract-7z-diag")
		_ = os.MkdirAll(fallbackDir, 0755)
		entries, _ := list7zEntries(archivePath)
		diag.Entries = entries
		if err := download.Extract7z(archivePath, fallbackDir); err != nil {
			diag.Error = err.Error()
			diag.Message = fmt.Sprintf("诊断尝试 download.Extract7z 也失败: %v", err)
			return diag
		}
		demPath, err := download.FindFirstByExt(fallbackDir, ".dem")
		if err != nil {
			diag.Error = err.Error()
			diag.Message = "download.Extract7z 解压成功，但未找到 .dem 文件"
			return diag
		}
		var size int64
		if info, err := os.Stat(demPath); err == nil {
			size = info.Size()
		}
		likely, _ := download.IsLikelyDemoFile(demPath)
		diag.Success = likely
		diag.DemPath = demPath
		diag.DemSize = size
		diag.LikelyDemoOK = likely
		if likely {
			diag.Message = fmt.Sprintf("诊断确认：OSS 已改为 7z 格式！使用 download.Extract7z 成功提取 %s (%s)",
				filepath.Base(demPath), formatByteSize(size))
		} else {
			diag.Message = "download.Extract7z 提取出 .dem，但文件头校验未通过"
		}
		return diag

	case "RAW_DEM":
		diag.Attempted = true
		var size int64
		if info, err := os.Stat(archivePath); err == nil {
			size = info.Size()
		}
		likely, err := download.IsLikelyDemoFile(archivePath)
		diag.DemPath = archivePath
		diag.DemSize = size
		diag.LikelyDemoOK = likely
		if err != nil {
			diag.Error = err.Error()
		}
		diag.Success = likely
		if likely {
			diag.Message = fmt.Sprintf("诊断确认：OSS 直接返回了未压缩的 .dem 文件（%s，文件头 %s）",
				formatByteSize(size), readDemoStamp(archivePath))
		}
		return diag

	default:
		_ = ctx
		return diag
	}
}

// Status returns the current Step 7 task status, refreshing live disk bytes while downloading.
func (m *Step7Manager) Status(taskID string) Step7Status {
	m.mu.Lock()
	st := m.status
	m.mu.Unlock()

	if taskID != "" && st.TaskID != "" && st.TaskID != taskID {
		return Step7Status{
			TaskID:  taskID,
			Phase:   "not_found",
			Summary: "任务已被新任务替换或已清理",
		}
	}
	if st.Running && st.Phase == "downloading" && st.ArchivePath != "" {
		if info, err := os.Stat(st.ArchivePath); err == nil {
			st.DownloadedBytes = info.Size()
		}
	}
	return st
}

// Cancel cancels the currently active Step 7 or native import task.
func (m *Step7Manager) Cancel() Step7Status {
	m.mu.Lock()
	cancel := m.cancel
	done := m.doneCh
	st := m.status
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return m.Status(st.TaskID)
}

// Cleanup cancels any active task and deletes temporary artifacts inside workDir.
func (m *Step7Manager) Cleanup() (int64, error) {
	m.Cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	removed := cleanupWorkDirChildren(m.workDir)
	m.status = Step7Status{
		Phase:   "idle",
		Summary: "已清理临时下载与解压目录",
	}
	return removed, nil
}

func cleanupWorkDirChildren(workDir string) int64 {
	if strings.TrimSpace(workDir) == "" {
		return 0
	}
	var total int64
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		p := filepath.Join(workDir, entry.Name())
		total += dirByteSize(p)
		_ = os.RemoveAll(p)
	}
	return total
}

func dirByteSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, infoErr := d.Info(); infoErr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func sniffFileHead(path string) FileSniffResult {
	f, err := os.Open(path)
	if err != nil {
		return FileSniffResult{Format: "ERROR", Description: err.Error()}
	}
	defer f.Close()

	buf := make([]byte, 4096)
	n, _ := io.ReadFull(f, buf)
	if n < 0 {
		n = 0
	}
	return SniffBytes(buf[:n])
}

func readDemoStamp(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 8)
	n, _ := io.ReadFull(f, buf)
	if n <= 0 {
		return ""
	}
	return strings.TrimRight(string(buf[:n]), "\x00")
}

func listZipEntries(path string) ([]ArchiveEntry, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	entries := make([]ArchiveEntry, 0, minInt(len(r.File), maxArchiveEntries))
	for i, f := range r.File {
		if i >= maxArchiveEntries {
			break
		}
		entries = append(entries, ArchiveEntry{
			Name:             f.Name,
			UncompressedSize: f.UncompressedSize64,
			IsDir:            f.FileInfo().IsDir(),
		})
	}
	return entries, nil
}

func list7zEntries(path string) ([]ArchiveEntry, error) {
	r, err := sevenzip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	entries := make([]ArchiveEntry, 0, minInt(len(r.File), maxArchiveEntries))
	for i, f := range r.File {
		if i >= maxArchiveEntries {
			break
		}
		entries = append(entries, ArchiveEntry{
			Name:             f.Name,
			UncompressedSize: f.UncompressedSize,
			IsDir:            f.FileInfo().IsDir(),
		})
	}
	return entries, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func sanitizeFileStem(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if numeric, err := wanmei.ExtractNumericMatchID(trimmed); err == nil {
		return numeric
	}
	var b strings.Builder
	for _, r := range trimmed {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func formatByteSize(n int64) string {
	if n < 0 {
		return "未知大小"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 4; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(n)/float64(div), "KMGT"[exp])
}
