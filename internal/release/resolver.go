package release

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Info struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
	Repo    string  `json:"repo,omitempty"`
	Source  string  `json:"source,omitempty"`
}

type Asset struct {
	Name               string `json:"name"`
	DownloadURL        string `json:"download_url"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url,omitempty"`
	GitHubURL          string `json:"github_url,omitempty"`
	MirrorURL          string `json:"mirror_url,omitempty"`
}

const (
	ComponentKeyApp    = "app"
	ComponentKeyHLAE   = "hlae"
	ComponentKeyPlugin = "cs2-server-plugin"

	SourceGitHub = "github"
)

type ComponentCandidate struct {
	Repo     string
	Source   string
	OK       bool
	Status   int
	Error    string
	Info     *Info
	Channels []Channel
	Versions map[string]*Info
}

// Channel pins a component to one synced version (e.g. a legacy HLAE build
// for an older CS2). Clients persist the channel ID, never the tag, so the
// release console can move a channel without client changes.
type Channel struct {
	ID   string
	Name string
	Note string
	Tag  string
}

type UnifiedLatest struct {
	Project            string
	Components         map[string]map[string][]ComponentCandidate
	Ads                AdsManifest
	AdValidationErrors []string
}

type manifestProjectPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type manifestAssetPayload struct {
	Name               string `json:"name"`
	URL                string `json:"url"`
	GitHubURL          string `json:"github_url"`
	MirrorURL          string `json:"mirror_url"`
	DownloadURL        string `json:"download_url"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type manifestReleasePayload struct {
	TagName string                 `json:"tag_name"`
	Tag     string                 `json:"tag"`
	Assets  []manifestAssetPayload `json:"assets"`
}

type manifestChannelPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Note string `json:"note"`
	Tag  string `json:"tag"`
}

type manifestDependencyPayload struct {
	Name      string                            `json:"name"`
	Repo      string                            `json:"repo"`
	LatestTag string                            `json:"latest_tag"`
	Latest    manifestReleasePayload            `json:"latest"`
	Channels  []manifestChannelPayload          `json:"channels"`
	Versions  map[string]manifestReleasePayload `json:"versions"`
}

type manifestPayload struct {
	Project      manifestProjectPayload               `json:"project"`
	Dependencies map[string]manifestDependencyPayload `json:"dependencies"`
	Ads          manifestAdsPayload                   `json:"ads"`
}

func ComponentKey(componentID string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(componentID)) {
	case "app", "self_update":
		return ComponentKeyApp, true
	case "hlae":
		return ComponentKeyHLAE, true
	case "plugin":
		return ComponentKeyPlugin, true
	default:
		return "", false
	}
}

func (u *UnifiedLatest) ComponentCandidates(componentID string, source string) []ComponentCandidate {
	if u == nil {
		return nil
	}
	componentKey, ok := ComponentKey(componentID)
	if !ok {
		return nil
	}
	bySource, ok := u.Components[componentKey]
	if !ok {
		return nil
	}
	return append([]ComponentCandidate(nil), bySource[normalizeSource(source)]...)
}

func (u *UnifiedLatest) ComponentInfoBySource(componentID string, source string) (*Info, bool) {
	candidates := u.ComponentCandidates(componentID, source)
	return pickBestInfo(candidates)
}

// ComponentChannels returns the channels published for a component, in
// manifest order.
func (u *UnifiedLatest) ComponentChannels(componentID string) []Channel {
	for _, candidate := range u.ComponentCandidates(componentID, SourceGitHub) {
		if len(candidate.Channels) > 0 {
			return append([]Channel(nil), candidate.Channels...)
		}
	}
	return nil
}

// ComponentVersionInfo returns the release info of one synced version.
func (u *UnifiedLatest) ComponentVersionInfo(componentID string, tag string) (*Info, bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, false
	}
	for _, candidate := range u.ComponentCandidates(componentID, SourceGitHub) {
		if info, ok := candidate.Versions[tag]; ok && hasUsableAsset(info) {
			return cloneInfo(info), true
		}
	}
	return nil, false
}

func (u *UnifiedLatest) ComponentSourceError(componentID string, source string) string {
	candidates := u.ComponentCandidates(componentID, source)
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Error) != "" {
			return strings.TrimSpace(candidate.Error)
		}
	}
	return ""
}

func (u *UnifiedLatest) ComponentInfo(componentID string) (*Info, bool) {
	return u.ComponentInfoBySource(componentID, SourceGitHub)
}

func AssetDownloadURL(asset Asset) string {
	if strings.TrimSpace(asset.DownloadURL) != "" {
		return strings.TrimSpace(asset.DownloadURL)
	}
	if strings.TrimSpace(asset.URL) != "" {
		return strings.TrimSpace(asset.URL)
	}
	if strings.TrimSpace(asset.GitHubURL) != "" {
		return strings.TrimSpace(asset.GitHubURL)
	}
	if strings.TrimSpace(asset.MirrorURL) != "" {
		return strings.TrimSpace(asset.MirrorURL)
	}
	return strings.TrimSpace(asset.BrowserDownloadURL)
}

func FetchUnifiedLatest(apiURL string) (*UnifiedLatest, error) {
	if strings.TrimSpace(apiURL) == "" {
		return nil, fmt.Errorf("Release API URL 未配置")
	}

	fetchWithClient := func(client *http.Client) (*UnifiedLatest, error) {
		if client == nil {
			client = &http.Client{Timeout: 30 * time.Second}
		}
		req, err := http.NewRequest(http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "CS2-Highlight-Tool-v2")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("Release API 返回状态码: %d", resp.StatusCode)
		}

		var payload manifestPayload
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, err
		}

		project := firstNonEmpty(payload.Project.Name, payload.Project.ID)
		result := &UnifiedLatest{
			Project:    project,
			Components: make(map[string]map[string][]ComponentCandidate, len(payload.Dependencies)),
		}

		for dependencyKey, dependency := range payload.Dependencies {
			componentKey, ok := dependencyComponentKey(dependencyKey, dependency)
			if !ok {
				continue
			}
			candidate := parseManifestDependency(dependency, dependencyKey)
			if _, exists := result.Components[componentKey]; !exists {
				result.Components[componentKey] = make(map[string][]ComponentCandidate, 1)
			}
			result.Components[componentKey][SourceGitHub] = append(result.Components[componentKey][SourceGitHub], candidate)
		}
		result.Ads, result.AdValidationErrors = parseManifestAds(payload.Ads)
		return result, nil
	}

	info, err := fetchWithClient(&http.Client{Timeout: 30 * time.Second})
	if err == nil {
		return info, nil
	}
	if hasProxyEnv() {
		directClient := &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{Proxy: nil},
		}
		if directInfo, directErr := fetchWithClient(directClient); directErr == nil {
			return directInfo, nil
		}
	}
	return nil, err
}

func dependencyComponentKey(dependencyKey string, dependency manifestDependencyPayload) (string, bool) {
	keys := []string{dependencyKey, dependency.Name, dependency.Repo}
	for _, raw := range keys {
		normalized := strings.ToLower(strings.TrimSpace(raw))
		switch normalized {
		case "advancedfx", "hlae", "advancedfx/advancedfx":
			return ComponentKeyHLAE, true
		case "cs2-server-plugin", "plugin", "hkslover/cs2-server-plugin":
			return ComponentKeyPlugin, true
		case "app", "self_update", "cs2-highlight-tool", "cs2-highlight-tool-v2", "hkslover/cs2-highlight-tool", "hkslover/cs2-highlight-tool-v2":
			return ComponentKeyApp, true
		}
	}
	return "", false
}

func parseManifestDependency(dependency manifestDependencyPayload, dependencyKey string) ComponentCandidate {
	repo := strings.TrimSpace(dependency.Repo)
	tagName := firstNonEmpty(dependency.Latest.TagName, dependency.Latest.Tag, dependency.LatestTag)
	htmlURL := defaultReleasePage(repo, tagName)
	assets := convertManifestAssets(dependency.Latest.Assets)
	channels := convertManifestChannels(dependency.Channels)
	versions := convertManifestVersions(repo, dependency.Versions)

	ok := len(assets) > 0
	errorMessage := ""
	if !ok {
		errorMessage = fmt.Sprintf("manifest 组件 %s 缺少可用资产", strings.TrimSpace(dependencyKey))
	}

	var info *Info
	if tagName != "" || htmlURL != "" || len(assets) > 0 {
		info = &Info{
			TagName: tagName,
			HTMLURL: htmlURL,
			Assets:  assets,
			Repo:    repo,
			Source:  SourceGitHub,
		}
	}

	return ComponentCandidate{
		Repo:     repo,
		Source:   SourceGitHub,
		OK:       ok,
		Error:    errorMessage,
		Info:     info,
		Channels: channels,
		Versions: versions,
	}
}

// convertManifestChannels drops channels whose ID could not be used as a
// directory name or that point at no tag, and keeps the first of duplicates.
func convertManifestChannels(in []manifestChannelPayload) []Channel {
	out := make([]Channel, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, channel := range in {
		id := strings.TrimSpace(channel.ID)
		tag := strings.TrimSpace(channel.Tag)
		if !IsValidChannelID(id) || tag == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, Channel{
			ID:   id,
			Name: firstNonEmpty(channel.Name, id),
			Note: strings.TrimSpace(channel.Note),
			Tag:  tag,
		})
	}
	return out
}

func convertManifestVersions(repo string, in map[string]manifestReleasePayload) map[string]*Info {
	out := make(map[string]*Info, len(in))
	for key, version := range in {
		tagName := firstNonEmpty(version.TagName, version.Tag, key)
		info := &Info{
			TagName: tagName,
			HTMLURL: defaultReleasePage(repo, tagName),
			Assets:  convertManifestAssets(version.Assets),
			Repo:    repo,
			Source:  SourceGitHub,
		}
		out[strings.TrimSpace(key)] = info
		if tagName != strings.TrimSpace(key) {
			if _, exists := out[tagName]; !exists {
				out[tagName] = info
			}
		}
	}
	return out
}

// IsValidChannelID reports whether a channel ID is safe to use as a single
// path segment.
func IsValidChannelID(id string) bool {
	if id == "" || len(id) > 64 || id[0] == '.' {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}

func defaultReleasePage(repo string, tagName string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return ""
	}
	if strings.TrimSpace(tagName) == "" {
		return "https://github.com/" + repo + "/releases"
	}
	return "https://github.com/" + repo + "/releases/tag/" + url.PathEscape(strings.TrimSpace(tagName))
}

func convertManifestAssets(in []manifestAssetPayload) []Asset {
	out := make([]Asset, 0, len(in))
	for _, asset := range in {
		normalizedURL := normalizeDownloadURL(firstNonEmpty(asset.DownloadURL, asset.URL))
		normalizedGitHubURL := normalizeDownloadURL(asset.GitHubURL)
		normalizedMirrorURL := normalizeDownloadURL(asset.MirrorURL)
		normalizedBrowserURL := normalizeDownloadURL(asset.BrowserDownloadURL)
		out = append(out, Asset{
			Name:               strings.TrimSpace(asset.Name),
			DownloadURL:        normalizedURL,
			BrowserDownloadURL: normalizedBrowserURL,
			URL:                normalizedURL,
			GitHubURL:          normalizedGitHubURL,
			MirrorURL:          normalizedMirrorURL,
		})
	}
	return out
}

func normalizeDownloadURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err == nil && strings.TrimSpace(parsed.Scheme) != "" {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	candidate := "https://" + strings.TrimPrefix(raw, "/")
	parsed, err = url.Parse(candidate)
	if err == nil && strings.TrimSpace(parsed.Host) != "" {
		return candidate
	}
	return raw
}

func pickBestInfo(candidates []ComponentCandidate) (*Info, bool) {
	for _, candidate := range candidates {
		if candidate.OK && hasUsableAsset(candidate.Info) {
			return cloneInfo(candidate.Info), true
		}
	}
	for _, candidate := range candidates {
		if hasUsableAsset(candidate.Info) {
			return cloneInfo(candidate.Info), true
		}
	}
	return nil, false
}

func hasUsableAsset(info *Info) bool {
	if info == nil || len(info.Assets) == 0 {
		return false
	}
	for _, asset := range info.Assets {
		if strings.TrimSpace(AssetDownloadURL(asset)) != "" {
			return true
		}
	}
	return false
}

func cloneInfo(info *Info) *Info {
	if info == nil {
		return nil
	}
	cloned := *info
	cloned.Assets = append([]Asset(nil), info.Assets...)
	return &cloned
}

func normalizeSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == SourceGitHub {
		return source
	}
	return SourceGitHub
}

func hasProxyEnv() bool {
	keys := []string{"http_proxy", "https_proxy", "all_proxy", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"}
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func SelectAppExeAsset(release *Info) (Asset, bool) {
	return selectAsset(release, func(name string) int {
		if !strings.HasSuffix(name, ".exe") {
			return 0
		}
		score := 1
		if strings.Contains(name, "cs2-highlight-tool") {
			score += 4
		}
		if strings.Contains(name, "windows") || strings.Contains(name, "win") {
			score += 2
		}
		if strings.Contains(name, "amd64") || strings.Contains(name, "x64") {
			score += 2
		}
		return score
	})
}

func SelectHLAEAsset(release *Info) (Asset, bool) {
	return selectAsset(release, func(name string) int {
		if strings.HasPrefix(name, "hlae_") && strings.HasSuffix(name, ".zip") && !strings.HasSuffix(name, ".zip.asc") {
			return 10
		}
		return 0
	})
}

func SelectPluginAsset(release *Info) (Asset, bool) {
	return selectAsset(release, func(name string) int {
		if strings.HasSuffix(name, ".zip") {
			return 10
		}
		return 0
	})
}

func selectAsset(release *Info, score func(name string) int) (Asset, bool) {
	if release == nil {
		return Asset{}, false
	}
	bestScore := 0
	var best Asset
	for _, asset := range release.Assets {
		name := strings.ToLower(strings.TrimSpace(asset.Name))
		if name == "" {
			name = strings.ToLower(filepath.Base(AssetDownloadURL(asset)))
		}
		if AssetDownloadURL(asset) == "" {
			continue
		}
		if s := score(name); s > bestScore {
			bestScore = s
			best = asset
		}
	}
	return best, bestScore > 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
