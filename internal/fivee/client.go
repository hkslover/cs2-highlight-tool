package fivee

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/download"
)

type FiveEMatchItem struct {
	MatchID         string  `json:"match_id"`
	DownloadMatchID string  `json:"download_match_id"`
	MapName         string  `json:"map_name"`
	Score1          int     `json:"score1"`
	Score2          int     `json:"score2"`
	Kill            int     `json:"kill"`
	Death           int     `json:"death"`
	Assist          int     `json:"assist"`
	Rating          float64 `json:"rating"`
	EndTime         string  `json:"end_time"`
}

type FiveEMatchListResult struct {
	PlayerName string           `json:"player_name"`
	Matches    []FiveEMatchItem `json:"matches"`
}

type matchListAPIResponse struct {
	Success bool             `json:"success"`
	ErrCode any              `json:"errcode"`
	Message string           `json:"message"`
	Data    matchListPayload `json:"data"`
}

type matchListPayload struct {
	MatchList []matchRaw `json:"match_list"`
}

type matchRaw struct {
	MatchID        string `json:"match_id"`
	MapName        string `json:"map_name"`
	Map            string `json:"map"`
	Group1AllScore any    `json:"group1_all_score"`
	Group2AllScore any    `json:"group2_all_score"`
	Kill           any    `json:"kill"`
	Death          any    `json:"death"`
	Assist         any    `json:"assist"`
	Rating         any    `json:"rating"`
	EndTime        any    `json:"end_time"`
}

type matchDetailResponse struct {
	Data struct {
		Main struct {
			DemoURL string `json:"demo_url"`
		} `json:"main"`
	} `json:"data"`
}

const (
	matchListURL       = "https://ya-api-app.5eplay.com/v0/mars/api/csgo/match_data/match_list"
	matchDetailBaseURL = "https://gate.5eplay.com/crane/http/api/data/match"
	ProgressPrefix     = "fivee_import_"
)

var (
	HTTPRequestFn    = defaultHTTPRequest
	DownloadFileFn   = download.File
	UnzipFn          = download.Unzip
	FindFirstByExtFn = download.FindFirstByExt
	CopyFileFn       = download.CopyFile
	matchIDPattern   = regexp.MustCompile(`(?i)g\d+(?:-[a-z0-9]+)+`)
	playerDomainRE   = regexp.MustCompile(`(?i)(?:[?&]|^)domain=([^&#\s]+)`)
	ErrDemoExpired   = errors.New("5E DEM 已过期，无法下载")
)

func defaultHTTPRequest(req *http.Request, timeout time.Duration) (*http.Response, error) {
	client := &http.Client{Timeout: timeout}
	return client.Do(req)
}

// ProgressComponentID returns the progress event component ID for a match.
func ProgressComponentID(matchID string) string {
	return ProgressPrefix + strings.TrimSpace(matchID)
}

// ListRecentMatches fetches recent 5E matches for the given player name.
func ListRecentMatches(playerName string, page int) ([]FiveEMatchItem, error) {
	if page < 1 {
		page = 1
	}
	return fetchRecentMatches(playerName, page)
}

// NormalizePlayerDomainInput extracts the 5E profile domain from share links.
func NormalizePlayerDomainInput(raw string) string {
	original := strings.TrimSpace(raw)
	if original == "" {
		return ""
	}

	matched := playerDomainRE.FindStringSubmatch(original)
	if len(matched) >= 2 {
		domain, err := url.QueryUnescape(strings.TrimSpace(matched[1]))
		if err == nil {
			domain = strings.TrimSpace(domain)
			if domain != "" {
				return domain
			}
		}
	}
	return original
}

// FetchDemoURL resolves the download URL for a 5E match demo.
func FetchDemoURL(matchID string) (string, error) {
	return fetchDemoURL(matchID)
}

// ExtractMatchID parses a raw 5E match ID string into its canonical form.
func ExtractMatchID(raw string) (string, error) {
	return extractMatchID(raw)
}

// ImportDemo downloads and extracts a 5E demo into cacheRoot.
// Returns the path to the stable .dem file on success.
func ImportDemo(downloadMatchID, cacheRoot string, onProgress func(active bool, percent float64, indeterminate bool)) (string, error) {
	if err := os.MkdirAll(cacheRoot, 0755); err != nil {
		return "", fmt.Errorf("创建 5E DEM 缓存目录失败: %w", err)
	}
	stableSourcePath := filepath.Join(cacheRoot, downloadMatchID+".dem")
	if valid, err := download.IsLikelyDemoFile(stableSourcePath); err != nil {
		return "", fmt.Errorf("检查 5E DEM 缓存文件失败: %w", err)
	} else if valid {
		return stableSourcePath, nil
	}

	demoURL, err := fetchDemoURL(downloadMatchID)
	if err != nil {
		if errors.Is(err, ErrDemoExpired) {
			return "", err
		}
		return "", fmt.Errorf("获取 5E DEM 下载地址失败: %w", err)
	}

	archiveName := safeArchiveName(demoURL, downloadMatchID)
	archivePath := filepath.Join(cacheRoot, archiveName)
	extractDir := filepath.Join(cacheRoot, "extract")
	if err := os.RemoveAll(extractDir); err != nil {
		return "", fmt.Errorf("清理 5E DEM 解压目录失败: %w", err)
	}
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return "", fmt.Errorf("创建 5E DEM 解压目录失败: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(extractDir)
		_ = os.Remove(archivePath)
	}()

	if err := DownloadFileFn(demoURL, archivePath, func(active bool, percent float64, indeterminate bool) {
		if onProgress != nil {
			onProgress(active, percent, indeterminate)
		}
	}); err != nil {
		return "", fmt.Errorf("下载 5E DEM 失败: %w", err)
	}
	if err := UnzipFn(archivePath, extractDir); err != nil {
		return "", fmt.Errorf("解压 5E DEM 失败: %w", err)
	}

	extractedDemPath, err := FindFirstByExtFn(extractDir, ".dem")
	if err != nil {
		return "", fmt.Errorf("未在 5E DEM 压缩包中找到 .dem 文件: %w", err)
	}

	if extractedDemPath != stableSourcePath {
		if err := CopyFileFn(extractedDemPath, stableSourcePath); err != nil {
			return "", fmt.Errorf("写入 5E DEM 缓存文件失败: %w", err)
		}
	}
	return stableSourcePath, nil
}

func safeArchiveName(rawURL, fallbackID string) string {
	archiveName := ""
	if parsed, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
		archivePath := strings.TrimSpace(parsed.Path)
		if decoded, decodeErr := url.PathUnescape(archivePath); decodeErr == nil {
			archivePath = decoded
		}
		archiveName = path.Base(archivePath)
	}
	archiveName = strings.TrimSpace(archiveName)
	if archiveName == "" || archiveName == "." || archiveName == "/" || archiveName == `\` {
		archiveName = strings.TrimSpace(fallbackID) + ".zip"
	}
	// URL query parameters are intentionally excluded above.  Keep the local
	// name conservative for Windows and reject path traversal even if a server
	// returns an unusual path segment.
	archiveName = strings.NewReplacer("<", "_", ">", "_", ":", "_", `"`, "_", "/", "_", `\`, "_", "|", "_", "?", "_", "*", "_").Replace(archiveName)
	archiveName = strings.Trim(archiveName, " .")
	if archiveName == "" || strings.EqualFold(archiveName, strings.TrimSpace(fallbackID)+".dem") {
		archiveName = strings.TrimSpace(fallbackID) + ".zip"
	}
	return archiveName
}

func fetchRecentMatches(playerName string, page int) ([]FiveEMatchItem, error) {
	endpoint, err := url.Parse(matchListURL)
	if err != nil {
		return nil, fmt.Errorf("构建 5E 战绩请求地址失败: %w", err)
	}
	query := endpoint.Query()
	query.Set("date_time", "0")
	query.Set("match_type", "-1")
	query.Set("map_name", "")
	query.Set("domain", strings.TrimSpace(playerName))
	query.Set("time", "2")
	query.Set("page", strconv.Itoa(page))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("创建 5E 战绩请求失败: %w", err)
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh-Hans;q=0.9")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/json")

	resp, err := HTTPRequestFn(req, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("请求 5E 战绩失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("5E 战绩接口返回状态码: %d", resp.StatusCode)
	}

	body, err := readHTTPBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取 5E 战绩响应失败: %w", err)
	}

	var apiResp matchListAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("解析 5E 战绩响应失败: %w", err)
	}

	errCode := parseInt(apiResp.ErrCode)
	if !apiResp.Success || errCode != 0 {
		message := strings.TrimSpace(apiResp.Message)
		if message == "" {
			message = fmt.Sprintf("errcode=%d", errCode)
		}
		return nil, fmt.Errorf("5E 战绩接口错误: %s", message)
	}

	return parseMatchList(apiResp.Data.MatchList), nil
}

func parseMatchList(raw []matchRaw) []FiveEMatchItem {
	if len(raw) == 0 {
		return []FiveEMatchItem{}
	}

	result := make([]FiveEMatchItem, 0, len(raw))
	for _, item := range raw {
		rawMatchID := strings.TrimSpace(item.MatchID)
		downloadMatchID, err := extractMatchID(rawMatchID)
		if err != nil {
			continue
		}
		mapName := strings.TrimSpace(item.MapName)
		if mapName == "" {
			mapName = strings.TrimSpace(item.Map)
		}
		result = append(result, FiveEMatchItem{
			MatchID:         rawMatchID,
			DownloadMatchID: downloadMatchID,
			MapName:         mapName,
			Score1:          parseInt(item.Group1AllScore),
			Score2:          parseInt(item.Group2AllScore),
			Kill:            parseInt(item.Kill),
			Death:           parseInt(item.Death),
			Assist:          parseInt(item.Assist),
			Rating:          parseFloat(item.Rating),
			EndTime:         formatEndTime(item.EndTime),
		})
	}
	return result
}

func fetchDemoURL(matchID string) (string, error) {
	requestURL := strings.TrimRight(matchDetailBaseURL, "/") + "/" + url.PathEscape(strings.TrimSpace(matchID))
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建 5E 下载地址请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := HTTPRequestFn(req, 15*time.Second)
	if err != nil {
		return "", fmt.Errorf("请求 5E 下载地址失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("5E 下载地址接口返回状态码: %d", resp.StatusCode)
	}

	body, err := readHTTPBody(resp)
	if err != nil {
		return "", fmt.Errorf("读取 5E 下载地址响应失败: %w", err)
	}

	var payload matchDetailResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("解析 5E 下载地址响应失败: %w", err)
	}

	demoURL := strings.TrimSpace(payload.Data.Main.DemoURL)
	if demoURL == "" {
		return "", ErrDemoExpired
	}
	return demoURL, nil
}

func extractMatchID(raw string) (string, error) {
	original := strings.TrimSpace(raw)
	if original == "" {
		return "", fmt.Errorf("matchID 不能为空")
	}

	candidate := original
	if parsed, err := url.Parse(original); err == nil {
		if parsed.Path != "" {
			candidate = path.Base(parsed.Path)
		}
	} else if strings.Contains(candidate, "/") {
		candidate = path.Base(candidate)
	}

	candidate = strings.TrimSpace(candidate)
	if idx := strings.Index(candidate, "?"); idx >= 0 {
		candidate = candidate[:idx]
	}
	candidate = trimCaseInsensitiveSuffix(candidate, ".zip")
	candidate = trimCaseInsensitiveSuffix(candidate, ".dem")

	lowerCandidate := strings.ToLower(candidate)
	if idx := strings.Index(lowerCandidate, "_de_"); idx > 0 {
		candidate = candidate[:idx]
	}
	if idx := strings.Index(candidate, "_"); idx > 0 && strings.HasPrefix(strings.ToLower(candidate), "g") {
		candidate = candidate[:idx]
	}

	if matched := matchIDPattern.FindString(candidate); matched != "" {
		return strings.ToLower(strings.TrimSpace(matched)), nil
	}
	if matched := matchIDPattern.FindString(original); matched != "" {
		return strings.ToLower(strings.TrimSpace(matched)), nil
	}
	return "", fmt.Errorf("matchID 格式无效")
}

func trimCaseInsensitiveSuffix(value string, suffix string) string {
	if len(value) < len(suffix) {
		return value
	}
	if strings.EqualFold(value[len(value)-len(suffix):], suffix) {
		return value[:len(value)-len(suffix)]
	}
	return value
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

func parseInt64(value any) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case float32:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return i
		}
		f, err := v.Float64()
		if err == nil {
			return int64(f)
		}
		return 0
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0
		}
		i, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			return i
		}
		f, err := strconv.ParseFloat(s, 64)
		if err == nil {
			return int64(f)
		}
		return 0
	default:
		return 0
	}
}

func formatEndTime(value any) string {
	if ts := parseInt64(value); ts > 0 {
		return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
	}
	raw := strings.TrimSpace(fmt.Sprintf("%v", value))
	if raw == "" || raw == "<nil>" {
		return ""
	}
	return raw
}

func readHTTPBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, fmt.Errorf("响应体为空")
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	encoding := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	if strings.Contains(encoding, "gzip") || looksLikeGzip(raw) {
		decoded, err := decodeGzipBytes(raw)
		if err != nil {
			return nil, err
		}
		return decoded, nil
	}
	return raw, nil
}

func decodeGzipBytes(raw []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("解压 gzip 响应失败: %w", err)
	}
	defer reader.Close()

	decoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("读取 gzip 响应失败: %w", err)
	}
	return decoded, nil
}

func looksLikeGzip(raw []byte) bool {
	return len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b
}
