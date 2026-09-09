package envsetup

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"cs2-highlight-tool-v2/internal/download"
	"cs2-highlight-tool-v2/internal/endpoints"
	"cs2-highlight-tool-v2/internal/release"
)

const (
	urlKindDirect = "url"
	urlKindMirror = "mirror_url"
	urlKindGitHub = "github_url"
)

type releaseAssetCandidate struct {
	Source   DownloadSource
	Info     *release.Info
	Asset    release.Asset
	AssetURL string
	URLKind  string
}

func infoManualURL(componentID string, fallbackSource DownloadSource, info *release.Info) string {
	if info == nil {
		return endpoints.ManualURLFor(componentID, string(fallbackSource))
	}
	return firstNonEmpty(
		info.HTMLURL,
		endpoints.ReleasePageURL(info.Repo, info.Source),
		endpoints.ManualURLFor(componentID, string(fallbackSource)),
	)
}

func (s *Service) collectReleaseAssetCandidates(componentID string, primarySource DownloadSource, selectAsset func(*release.Info) (release.Asset, bool)) ([]releaseAssetCandidate, error) {
	primarySource = normalizeDownloadSource(string(primarySource))
	info, err := s.componentReleaseInfo(primarySource, componentID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.ToUpper(string(primarySource)), err)
	}

	asset, ok := selectAsset(info)
	if !ok {
		return nil, fmt.Errorf("%s: Release 中未找到可用资产", strings.ToUpper(string(primarySource)))
	}

	countryCode := s.currentCountryCode()
	orderedURLs := orderedAssetURLsByCountry(asset, countryCode)
	candidates := make([]releaseAssetCandidate, 0, len(orderedURLs))
	seenURL := make(map[string]struct{}, len(orderedURLs))
	for _, item := range orderedURLs {
		assetURL := strings.TrimSpace(item.url)
		if assetURL == "" {
			continue
		}
		if _, exists := seenURL[assetURL]; exists {
			continue
		}
		seenURL[assetURL] = struct{}{}
		candidates = append(candidates, releaseAssetCandidate{
			Source:   primarySource,
			Info:     info,
			Asset:    asset,
			AssetURL: assetURL,
			URLKind:  item.kind,
		})
	}

	if len(candidates) == 0 {
		if preferDirectAndMirror(countryCode) {
			return nil, fmt.Errorf("%s: 资产下载链接为空（期望字段: url 或 mirror_url）", strings.ToUpper(string(primarySource)))
		}
		return nil, fmt.Errorf("%s: 资产下载链接为空（期望字段: github_url）", strings.ToUpper(string(primarySource)))
	}
	return candidates, nil
}

type urlCandidate struct {
	kind string
	url  string
}

func orderedAssetURLsByCountry(asset release.Asset, countryCode string) []urlCandidate {
	if preferDirectAndMirror(countryCode) {
		return []urlCandidate{
			{kind: urlKindMirror, url: strings.TrimSpace(asset.MirrorURL)},
			{kind: urlKindDirect, url: firstNonEmpty(strings.TrimSpace(asset.URL), strings.TrimSpace(asset.DownloadURL))},
		}
	}
	return []urlCandidate{{kind: urlKindGitHub, url: strings.TrimSpace(asset.GitHubURL)}}
}

func preferDirectAndMirror(countryCode string) bool {
	countryCode = strings.ToUpper(strings.TrimSpace(countryCode))
	return countryCode == "" || countryCode == "CN"
}

func (s *Service) downloadAndInstallWithFallback(componentID string, latest string, candidates []releaseAssetCandidate, install func(path string) error) error {
	if len(candidates) == 0 {
		return fmt.Errorf("没有可用下载候选")
	}
	// 单候选保持原有顺序语义；多候选（CN / GeoIP 为空时的 mirror + url）并行竞速，
	// 先下载完成的链路胜出，避免“慢但不断流”的链路拖住整个下载。
	if len(candidates) == 1 {
		return s.downloadAndInstallSequential(componentID, latest, candidates, install)
	}

	winner, winnerPath, raceFailures, err := s.raceDownloadCandidates(componentID, latest, candidates)
	if err != nil {
		return err
	}
	if installErr := install(winnerPath); installErr != nil {
		failures := append([]string(nil), raceFailures...)
		failures = append(failures, fmt.Sprintf("%s(%s) 安装失败: %v", strings.ToUpper(string(winner.Source)), winner.URLKind, installErr))
		remaining := make([]releaseAssetCandidate, 0, len(candidates)-1)
		for _, candidate := range candidates {
			if candidate.AssetURL == winner.AssetURL {
				continue
			}
			remaining = append(remaining, candidate)
		}
		if seqErr := s.downloadAndInstallSequential(componentID, latest, remaining, install); seqErr != nil {
			if errors.Is(seqErr, download.ErrCanceled) {
				return seqErr
			}
			failures = append(failures, seqErr.Error())
			return fmt.Errorf("%s", strings.Join(failures, " | "))
		}
		return nil
	}
	return nil
}

func (s *Service) downloadAndInstallSequential(componentID string, latest string, candidates []releaseAssetCandidate, install func(path string) error) error {
	failures := make([]string, 0, len(candidates))
	attempt := 0
	for _, candidate := range candidates {
		attempt++
		s.emitLogWithFields("info", "开始尝试下载组件资产", logFields{
			Component: componentID,
			Stage:     "download_fallback",
			Action:    "attempt",
			Source:    string(candidate.Source),
			Attempt:   attempt,
			Meta: map[string]string{
				"url":      candidate.AssetURL,
				"url_kind": candidate.URLKind,
			},
		})
		targetPath := tempAssetPath(s.dataDir, componentID, latest, candidate.Asset)
		if err := s.downloadFile(componentID, candidate.AssetURL, targetPath); err != nil {
			if errors.Is(err, download.ErrCanceled) {
				return err
			}
			failures = append(failures, fmt.Sprintf("%d/%s(%s) 下载失败: %v", attempt, strings.ToUpper(string(candidate.Source)), candidate.URLKind, err))
			continue
		}
		if err := install(targetPath); err != nil {
			failures = append(failures, fmt.Sprintf("%d/%s(%s) 安装失败: %v", attempt, strings.ToUpper(string(candidate.Source)), candidate.URLKind, err))
			continue
		}
		return nil
	}

	if len(failures) == 0 {
		return fmt.Errorf("所有下载回退均失败")
	}
	return fmt.Errorf("%s", strings.Join(failures, " | "))
}

type downloadRaceResult struct {
	index int
	err   error
}

// raceDownloadCandidates 并行下载所有候选链接，第一个成功完成的胜出，其余立即取消并清理。
// 只有全部候选都失败时才返回错误；用户取消返回 download.ErrCanceled。
func (s *Service) raceDownloadCandidates(componentID string, latest string, candidates []releaseAssetCandidate) (releaseAssetCandidate, string, []string, error) {
	active, ctx, cancelCause := s.beginDownloadGroup(componentID)
	defer s.endDownloadGroup(componentID, active)
	defer cancelCause(errDownloadRaceFinished)
	defer s.emitProgress(componentID, false, 0, false)

	targetPaths := make([]string, len(candidates))
	for i, candidate := range candidates {
		targetPaths[i] = tempAssetPathForCandidate(s.dataDir, componentID, latest, candidate.Asset, candidate.URLKind)
	}

	progress := &raceProgressTracker{service: s, componentID: componentID}
	results := make(chan downloadRaceResult, len(candidates))
	var wg sync.WaitGroup
	for i, candidate := range candidates {
		i, candidate := i, candidate
		s.emitLogWithFields("info", "开始竞速下载组件资产", logFields{
			Component: componentID,
			Stage:     "download_fallback",
			Action:    "race_start",
			Source:    string(candidate.Source),
			Attempt:   i + 1,
			Meta: map[string]string{
				"url":      candidate.AssetURL,
				"url_kind": candidate.URLKind,
			},
		})
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := download.FileWithContext(ctx, candidate.AssetURL, targetPaths[i], progress.report)
			results <- downloadRaceResult{index: i, err: err}
		}()
	}

	failures := make([]string, 0, len(candidates))
	winner := -1
	for remaining := len(candidates); remaining > 0; remaining-- {
		result := <-results
		if result.err == nil {
			winner = result.index
			cancelCause(errDownloadRaceFinished)
			break
		}
		if errors.Is(result.err, download.ErrCanceled) {
			if errors.Is(context.Cause(ctx), errDownloadCanceledByUser) {
				cancelCause(errDownloadRaceFinished)
				wg.Wait()
				removeDownloadTempFiles(targetPaths)
				return releaseAssetCandidate{}, "", nil, download.ErrCanceled
			}
			// 竞速内部取消（另一条链路已胜出）不算失败。
			continue
		}
		failures = append(failures, fmt.Sprintf("%d/%s(%s) 下载失败: %v", result.index+1, strings.ToUpper(string(candidates[result.index].Source)), candidates[result.index].URLKind, result.err))
	}
	wg.Wait()

	if winner < 0 {
		removeDownloadTempFiles(targetPaths)
		if len(failures) == 0 {
			return releaseAssetCandidate{}, "", nil, fmt.Errorf("所有下载回退均失败")
		}
		return releaseAssetCandidate{}, "", failures, fmt.Errorf("%s", strings.Join(failures, " | "))
	}

	// 胜者保留，落败者（包括在胜出瞬间已完成的）全部清理，避免遗留临时文件。
	for i, path := range targetPaths {
		if i != winner {
			_ = os.Remove(path)
		}
	}
	if _, err := os.Stat(targetPaths[winner]); err != nil {
		return releaseAssetCandidate{}, "", failures, fmt.Errorf("竞速下载完成但文件不存在: %w", err)
	}

	selected := candidates[winner]
	s.emitLogWithFields("info", "竞速下载完成，采用先完成的链路", logFields{
		Component: componentID,
		Stage:     "download_fallback",
		Action:    "race_winner",
		Source:    string(selected.Source),
		Attempt:   winner + 1,
		Meta: map[string]string{
			"url":      selected.AssetURL,
			"url_kind": selected.URLKind,
		},
	})
	return selected, targetPaths[winner], failures, nil
}

// raceProgressTracker 只上报当前进度最高的链路，避免两条下载的百分比来回跳动。
type raceProgressTracker struct {
	service     *Service
	componentID string

	mu          sync.Mutex
	started     bool
	bestPercent float64
}

func (t *raceProgressTracker) report(active bool, percent float64, indeterminate bool) {
	if !active {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if indeterminate {
		if !t.started {
			t.started = true
			t.service.emitProgress(t.componentID, true, 0, true)
		}
		return
	}
	if !t.started || percent > t.bestPercent {
		t.started = true
		if percent > t.bestPercent {
			t.bestPercent = percent
		}
		t.service.emitProgress(t.componentID, true, t.bestPercent, false)
	}
}

func removeDownloadTempFiles(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func tempAssetPathForCandidate(dataDir, componentID, latest string, asset release.Asset, urlKind string) string {
	basePath := tempAssetPath(dataDir, componentID, latest, asset)
	urlKind = strings.TrimSpace(urlKind)
	if urlKind == "" {
		return basePath
	}
	ext := filepath.Ext(basePath)
	if ext == "" {
		return basePath + "_" + sanitizeFileName(urlKind)
	}
	return strings.TrimSuffix(basePath, ext) + "_" + sanitizeFileName(urlKind) + ext
}

func tempAssetPath(dataDir, componentID, latest string, asset release.Asset) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(asset.Name)))
	if ext == "" {
		u, err := url.Parse(strings.TrimSpace(release.AssetDownloadURL(asset)))
		if err == nil {
			ext = strings.ToLower(filepath.Ext(u.Path))
		}
	}
	if ext == "" {
		ext = ".zip"
	}
	return filepath.Join(dataDir, "temp", componentID+"_"+sanitizeFileName(latest)+ext)
}
