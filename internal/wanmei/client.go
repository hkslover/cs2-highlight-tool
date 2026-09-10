package wanmei

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/download"
)

type ClientStatus string

const (
	ClientNotRunning ClientStatus = "client_not_running"
	ClientNotLogged  ClientStatus = "client_not_logged_in"
	ClientReady      ClientStatus = "ready"
)

type WanmeiMatchItem struct {
	MatchID         string  `json:"match_id"`
	DownloadMatchID string  `json:"download_match_id"`
	MapName         string  `json:"map_name"`
	Score1          int     `json:"score1"`
	Score2          int     `json:"score2"`
	Kill            int     `json:"kill"`
	Death           int     `json:"death"`
	Assist          int     `json:"assist"`
	K4              int     `json:"k4"`
	K5              int     `json:"k5"`
	Rating          float64 `json:"rating"`
	EndTime         string  `json:"end_time"`
}

type WanmeiMatchListResult struct {
	Status   ClientStatus      `json:"status"`
	Nickname string            `json:"nickname,omitempty"`
	SteamID  string            `json:"steam_id,omitempty"`
	Matches  []WanmeiMatchItem `json:"matches"`
}

type localLoginResponse struct {
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Token    string `json:"token"`
}

type localLoginInfo struct {
	Nickname   string
	Token      string
	SteamID    string
	ServerTime string
}

type matchListAPIResponse struct {
	StatusCode   int             `json:"statusCode"`
	Code         int             `json:"code"`
	ErrorMessage string          `json:"errorMessage"`
	Msg          string          `json:"msg"`
	Data         json.RawMessage `json:"data"`
}

type matchListPayload struct {
	MatchList []matchRaw `json:"matchList"`
	List      []matchRaw `json:"list"`
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

const (
	localLoginURL  = "http://127.0.0.1:55555/"
	allowedOrigin  = "https://esports.wanmei.com"
	matchListURL   = "https://api.wmpvp.com/api/csgo/home/match/list"
	matchPageSize  = 11
	ProgressPrefix = "wanmei_import_"

	demoAppID      = "20000"
	demoSecret     = "969c1bcfdc527c319157cc48f83b1d106ebdeca3e8d9763f1ae6b88dde9b3ea9"
	publicIPAPI    = "https://api-ipv4.ip.sb/ip"
	pwaReferer     = "https://client.wmpvp.com"
	demoBaseURL    = "https://pwaweblogin.wmpvp.com"
	defaultTimeout = 8
)

var (
	HTTPRequestFn      = defaultHTTPRequest
	OSSResolveHTTPDoFn = defaultOSSResolveHTTPDo
	DownloadFileFn     = download.File
	UnzipFn            = download.Unzip
	FindFirstByExtFn   = download.FindFirstByExt
	CopyFileFn         = download.CopyFile
)

func defaultOSSResolveHTTPDo(req *http.Request, timeout time.Duration) (*http.Response, error) {
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return client.Do(req)
}

func defaultHTTPRequest(req *http.Request, timeout time.Duration) (*http.Response, error) {
	client := &http.Client{Timeout: timeout}
	return client.Do(req)
}

// ProgressComponentID returns the progress event component ID for a match.
func ProgressComponentID(matchID string) string {
	return ProgressPrefix + strings.TrimSpace(matchID)
}

// ListRecentMatches fetches recent Wanmei matches for the logged-in player.
func ListRecentMatches(page int) (*WanmeiMatchListResult, error) {
	if page < 1 {
		page = 1
	}

	result := &WanmeiMatchListResult{Matches: make([]WanmeiMatchItem, 0)}

	loginInfo, status, err := fetchLocalLoginInfo()
	if err != nil {
		return nil, err
	}
	result.Status = status
	if status != ClientReady {
		return result, nil
	}

	result.Nickname = loginInfo.Nickname
	result.SteamID = loginInfo.SteamID
	matches, err := fetchRecentMatches(loginInfo.Token, loginInfo.SteamID, page)
	if err != nil {
		return nil, err
	}
	result.Matches = matches
	return result, nil
}

// ImportDemo downloads and extracts a Wanmei demo into cacheRoot.
// Returns the path to the stable .dem file on success.
func ImportDemo(downloadMatchID, cacheRoot string, onProgress func(active bool, percent float64, indeterminate bool)) (string, error) {
	if err := os.MkdirAll(cacheRoot, 0755); err != nil {
		return "", fmt.Errorf("创建完美 DEM 缓存目录失败: %w", err)
	}
	stableSourcePath := filepath.Join(cacheRoot, downloadMatchID+".dem")
	if valid, err := download.IsLikelyDemoFile(stableSourcePath); err != nil {
		return "", fmt.Errorf("检查完美 DEM 缓存文件失败: %w", err)
	} else if valid {
		return stableSourcePath, nil
	}

	archivePath := filepath.Join(cacheRoot, downloadMatchID+"_0.zip")
	extractDir := filepath.Join(cacheRoot, "extract")
	if err := os.RemoveAll(extractDir); err != nil {
		return "", fmt.Errorf("清理完美 DEM 解压目录失败: %w", err)
	}
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return "", fmt.Errorf("创建完美 DEM 解压目录失败: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(extractDir)
		_ = os.Remove(archivePath)
	}()

	loginInfo, status, err := fetchLocalLoginInfo()
	if err != nil {
		return "", err
	}
	if status != ClientReady {
		return "", fmt.Errorf("完美客户端未登录，无法下载 DEM")
	}

	ipAddr, ipErr := fetchPublicIPv4(publicIPAPI, time.Duration(defaultTimeout)*time.Second)
	if ipErr != nil {
		return "", fmt.Errorf("获取公网 IP 失败: %w", ipErr)
	}

	signedURL := buildSignedDemoURL(downloadMatchID, "0", loginInfo.Token)

	pwaHeaders, err := buildPWAHeaders(loginInfo.SteamID, ipAddr, loginInfo.ServerTime)
	if err != nil {
		return "", fmt.Errorf("构建 PWA 请求头失败: %w", err)
	}

	ossURL, err := resolveOSSURL(signedURL, pwaHeaders, time.Duration(defaultTimeout)*time.Second)
	if err != nil {
		return "", fmt.Errorf("解析 DEM 下载地址失败: %w", err)
	}

	if err := DownloadFileFn(ossURL, archivePath, func(active bool, percent float64, indeterminate bool) {
		if onProgress != nil {
			onProgress(active, percent, indeterminate)
		}
	}); err != nil {
		return "", fmt.Errorf("下载完美 DEM 失败: %w", err)
	}
	if err := UnzipFn(archivePath, extractDir); err != nil {
		return "", fmt.Errorf("解压完美 DEM 失败: %w", err)
	}

	extractedDemPath, err := FindFirstByExtFn(extractDir, ".dem")
	if err != nil {
		return "", fmt.Errorf("未在完美 DEM 压缩包中找到 .dem 文件: %w", err)
	}

	if extractedDemPath != stableSourcePath {
		if err := CopyFileFn(extractedDemPath, stableSourcePath); err != nil {
			return "", fmt.Errorf("写入完美 DEM 缓存文件失败: %w", err)
		}
	}
	return stableSourcePath, nil
}

// ExtractNumericMatchID parses a raw Wanmei match ID string into its numeric form.
func ExtractNumericMatchID(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("matchID 不能为空")
	}

	if idx := strings.LastIndex(value, "/"); idx >= 0 && idx < len(value)-1 {
		value = value[idx+1:]
	}
	value = strings.TrimSuffix(value, ".zip")
	value = strings.TrimSuffix(value, ".dem")
	if idx := strings.Index(value, "_"); idx >= 0 {
		value = value[:idx]
	}
	if idx := strings.LastIndex(value, "@"); idx >= 0 && idx < len(value)-1 {
		value = value[idx+1:]
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("matchID 不能为空")
	}

	for _, r := range value {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("matchID 格式无效")
		}
	}
	return value, nil
}

func fetchLocalLoginInfo() (*localLoginInfo, ClientStatus, error) {
	req, err := http.NewRequest(http.MethodGet, localLoginURL, nil)
	if err != nil {
		return nil, ClientNotRunning, fmt.Errorf("创建完美本地登录请求失败: %w", err)
	}
	req.Header.Set("Origin", allowedOrigin)
	req.Header.Set("Referer", allowedOrigin+"/")

	resp, err := HTTPRequestFn(req, 5*time.Second)
	if err != nil {
		return nil, ClientNotRunning, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ClientNotRunning, fmt.Errorf("读取完美本地登录响应失败: %w", err)
	}
	trimmedBody := strings.TrimSpace(string(body))

	if resp.StatusCode == http.StatusCreated || trimmedBody == "{}" {
		return nil, ClientNotLogged, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, ClientNotLogged, nil
	}

	var payload localLoginResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, ClientNotLogged, fmt.Errorf("解析完美本地登录响应失败: %w", err)
	}

	encodedToken := strings.TrimSpace(payload.Token)
	if encodedToken == "" {
		return nil, ClientNotLogged, nil
	}

	decodedToken := decodeEncodeLog(encodedToken)
	token, steamID, serverTime, err := parseDecodedToken(decodedToken)
	if err != nil {
		return nil, ClientNotLogged, fmt.Errorf("解析完美登录 token 失败: %w", err)
	}

	return &localLoginInfo{
		Nickname:   strings.TrimSpace(payload.Nickname),
		Token:      token,
		SteamID:    steamID,
		ServerTime: serverTime,
	}, ClientReady, nil
}

func fetchRecentMatches(token string, steamID string, page int) ([]WanmeiMatchItem, error) {
	if page < 1 {
		page = 1
	}

	steamIDInt, err := strconv.ParseInt(strings.TrimSpace(steamID), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("无效的完美 SteamID")
	}

	payload := map[string]any{
		"pvpType":      -1,
		"dataSource":   3,
		"page":         page,
		"csgoSeasonId": "recent",
		"mySteamId":    steamIDInt,
		"toSteamId":    steamIDInt,
		"pageSize":     matchPageSize,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("构建完美战绩请求失败: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, matchListURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建完美战绩请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("token", token)
	req.Header.Set("t", strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set("appversion", "3.7.7")
	req.Header.Set("platform", "ios")
	req.Header.Set("appTheme", "0")
	req.Header.Set("gameType", "1,2")
	req.Header.Set("gameTypeStr", "1,2")

	resp, err := HTTPRequestFn(req, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("请求完美战绩失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("完美战绩接口返回状态码: %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取完美战绩响应失败: %w", err)
	}

	var apiResp matchListAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析完美战绩响应失败: %w", err)
	}

	if code := resolveAPIStatusCode(apiResp); code != 0 {
		message := strings.TrimSpace(apiResp.ErrorMessage)
		if message == "" {
			message = strings.TrimSpace(apiResp.Msg)
		}
		if message == "" {
			message = "未知错误"
		}
		return nil, fmt.Errorf("完美战绩接口错误: %s", message)
	}

	matches, err := parseMatchList(apiResp.Data)
	if err != nil {
		return nil, err
	}
	return matches, nil
}

func resolveAPIStatusCode(resp matchListAPIResponse) int {
	if resp.StatusCode != 0 {
		return resp.StatusCode
	}
	if resp.Code != 0 {
		return resp.Code
	}
	return 0
}

func parseMatchList(raw json.RawMessage) ([]WanmeiMatchItem, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return []WanmeiMatchItem{}, nil
	}

	parsed := make([]matchRaw, 0)
	var wrapper matchListPayload
	if err := json.Unmarshal(raw, &wrapper); err == nil {
		if len(wrapper.MatchList) > 0 {
			parsed = wrapper.MatchList
		} else if len(wrapper.List) > 0 {
			parsed = wrapper.List
		}
	}
	if len(parsed) == 0 {
		var direct []matchRaw
		if err := json.Unmarshal(raw, &direct); err == nil {
			parsed = direct
		}
	}
	if len(parsed) == 0 {
		return []WanmeiMatchItem{}, nil
	}

	result := make([]WanmeiMatchItem, 0, len(parsed))
	for _, item := range parsed {
		rawMatchID := strings.TrimSpace(item.MatchID)
		if rawMatchID == "" {
			rawMatchID = strings.TrimSpace(item.MatchIDAlt)
		}
		downloadMatchID, err := ExtractNumericMatchID(rawMatchID)
		if err != nil {
			continue
		}
		result = append(result, WanmeiMatchItem{
			MatchID:         rawMatchID,
			DownloadMatchID: downloadMatchID,
			MapName:         strings.TrimSpace(item.MapName),
			Score1:          item.Score1,
			Score2:          item.Score2,
			Kill:            item.Kill,
			Death:           item.Death,
			Assist:          item.Assist,
			K4:              parseInt(item.K4),
			K5:              parseInt(item.K5),
			Rating:          parseFloat(item.Rating),
			EndTime:         strings.TrimSpace(item.EndTime),
		})
	}
	return result, nil
}

func parseInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return int(i)
		}
		f, err := v.Float64()
		if err == nil {
			return int(f)
		}
		return 0
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0
		}
		i, err := strconv.Atoi(s)
		if err == nil {
			return i
		}
		f, err := strconv.ParseFloat(s, 64)
		if err == nil {
			return int(f)
		}
		return 0
	default:
		return 0
	}
}

func parseFloat(value any) float64 {
	switch v := value.(type) {
	case float32:
		return float64(v)
	case float64:
		return v
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, err := v.Float64()
		if err == nil {
			return f
		}
		i, err := v.Int64()
		if err == nil {
			return float64(i)
		}
		return 0
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0
		}
		f, err := strconv.ParseFloat(s, 64)
		if err == nil {
			return f
		}
		return 0
	default:
		return 0
	}
}

func decodeEncodeLog(encoded string) string {
	caret := strings.Index(encoded, "^")
	dollar := strings.LastIndex(encoded, "$")
	if caret == -1 || dollar != len(encoded)-1 || caret >= dollar {
		return encoded
	}

	hexPart := encoded[caret+1 : dollar]
	if len(hexPart)%2 != 0 {
		return encoded
	}

	raw, err := hex.DecodeString(hexPart)
	if err != nil {
		return encoded
	}
	for i := range raw {
		raw[i] ^= byte((42 + 3*i) % 255)
	}
	return encoded[:caret] + string(raw)
}

func parseDecodedToken(decoded string) (string, string, string, error) {
	parts := strings.Split(decoded, "_")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("解码格式异常")
	}

	token := strings.TrimSpace(strings.Join(parts[:len(parts)-2], "_"))
	serverTime := strings.TrimSpace(parts[len(parts)-2])
	steamID := strings.TrimSpace(parts[len(parts)-1])
	if token == "" || serverTime == "" || steamID == "" {
		return "", "", "", fmt.Errorf("token、server_time 或 steam_id 为空")
	}
	if _, err := strconv.ParseInt(steamID, 10, 64); err != nil {
		return "", "", "", fmt.Errorf("steam_id 无效")
	}
	if _, err := strconv.ParseInt(serverTime, 10, 64); err != nil {
		return "", "", "", fmt.Errorf("server_time 无效")
	}
	return token, steamID, serverTime, nil
}

func fetchPublicIPv4(ipAPI string, timeout time.Duration) (string, error) {
	req, err := http.NewRequest(http.MethodGet, ipAPI, nil)
	if err != nil {
		return "", fmt.Errorf("创建 IP 查询请求失败: %w", err)
	}

	resp, err := HTTPRequestFn(req, timeout)
	if err != nil {
		return "", fmt.Errorf("请求公网 IP 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("公网 IP 接口返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取公网 IP 响应失败: %w", err)
	}

	ip := strings.TrimSpace(string(body))
	if !regexp.MustCompile(`^(?:\d{1,3}\.){3}\d{1,3}$`).MatchString(ip) {
		return "", fmt.Errorf("无效的公网 IP 格式: %s", ip)
	}
	return ip, nil
}

func buildXPWASignature(steamID, ipAddr, serverTime string) (string, error) {
	if len(steamID) < 16 {
		return "", fmt.Errorf("steam_id 长度不足 16 位")
	}

	ts := serverTime
	keyOffset := len(ts) - 16
	if keyOffset < 0 {
		keyOffset = len(steamID) + keyOffset
	}
	keyText := ts + steamID[keyOffset:]
	ivText := steamID[len(steamID)-16:]
	key := []byte(keyText)
	iv := []byte(ivText)

	if len(key) != 16 {
		return "", fmt.Errorf("无效的 AES key 长度: %d", len(key))
	}
	if len(iv) != 16 {
		return "", fmt.Errorf("无效的 AES IV 长度: %d", len(iv))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建 AES cipher 失败: %w", err)
	}

	padded := pkcs7Pad([]byte(ipAddr), aes.BlockSize)
	encrypted := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(encrypted, padded)

	return ts + "-" + hex.EncodeToString(encrypted), nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	pad := bytes.Repeat([]byte{byte(padLen)}, padLen)
	return append(data, pad...)
}

func resolveOSSURL(signedURL string, headers map[string]string, timeout time.Duration) (string, error) {
	req, err := http.NewRequest(http.MethodGet, signedURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建签名请求失败: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := OSSResolveHTTPDoFn(req, timeout)
	if err != nil {
		return "", fmt.Errorf("请求签名 URL 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		if location == "" {
			return "", fmt.Errorf("重定向响应缺少 Location 头")
		}
		return location, nil
	}

	return "", fmt.Errorf("未收到重定向响应，状态码: %d", resp.StatusCode)
}

func buildPWAHeaders(steamID, ipAddr, serverTime string) (map[string]string, error) {
	sig, err := buildXPWASignature(steamID, ipAddr, serverTime)
	if err != nil {
		return nil, fmt.Errorf("构建 X-PWA-Signature 失败: %w", err)
	}
	return map[string]string{
		"X-PWA-SteamId":   steamID,
		"X-PWA-Signature": sig,
		"PwaSteamId":      steamID,
		"Referer":         pwaReferer,
	}, nil
}

func buildSignedDemoURL(downloadMatchID, cupID, accessToken string) string {
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
	var queryParts []string
	for _, k := range keys {
		queryParts = append(queryParts, k+"="+params[k])
	}
	queryString := strings.Join(queryParts, "&")

	randnum := strconv.Itoa(100000 + int(time.Now().UnixNano())%900000)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	signature := pvpSign(randnum, ts, queryString, demoAppID, demoSecret)

	path := "/csgo/demo/" + downloadMatchID + "_" + cupID + ".dem"
	root := strings.TrimRight(demoBaseURL, "/") + "/" + strings.TrimLeft(path, "/")

	signedQS := "a=" + demoAppID + "&r=" + randnum + "&s=" + signature + "&t=" + ts
	if queryString != "" {
		signedQS += "&" + queryString
	}
	return root + "?" + signedQS
}

func pvpSign(randnum, ts, data, appid, secret string) string {
	md5Sum := md5.Sum([]byte(randnum + ts + data))
	md5Hex := hex.EncodeToString(md5Sum[:])
	sha1Sum := sha1.Sum([]byte(appid + md5Hex + secret))
	return hex.EncodeToString(sha1Sum[:])
}
