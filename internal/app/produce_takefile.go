package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/demo"
	"cs2-highlight-tool-v2/internal/plugingen"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	produceHistoryTypeProduce = "produce_clip"
	produceHistoryTypeEdited  = "edited_video"
)

var openDirectoryDialog = wailsruntime.OpenDirectoryDialog

type ProduceTakeFile struct {
	DemoPath    string `json:"demo_path"`
	TakeIndex   int    `json:"take_index"`
	TakeName    string `json:"take_name,omitempty"`
	View        string `json:"view"`
	VideoPath   string `json:"video_path,omitempty"`
	AudioPath   string `json:"audio_path,omitempty"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	UpdatedAtMs int64  `json:"updated_at_ms"`
}

type ProduceTakeFileSnapshot struct {
	Items       []ProduceTakeFile `json:"items"`
	UpdatedAtMs int64             `json:"updated_at_ms"`
}

type ProduceHistoryItem struct {
	DemoPath      string          `json:"demo_path"`
	TakeIndex     int             `json:"take_index"`
	TakeName      string          `json:"take_name,omitempty"`
	View          string          `json:"view"`
	SpecMode      int             `json:"spec_mode"`
	KillIDs       []string        `json:"kill_ids"`
	Kills         []demo.ClipKill `json:"kills,omitempty"`
	SourceID      string          `json:"source_id,omitempty"`
	Round         int             `json:"round,omitempty"`
	PlayerName    string          `json:"player_name,omitempty"`
	PlayerSteamID string          `json:"player_steam_id,omitempty"`
	StartTick     int             `json:"start_tick,omitempty"`
	EndTick       int             `json:"end_tick,omitempty"`
	EndReason     string          `json:"end_reason,omitempty"`
	VideoPath     string          `json:"video_path"`
	HistoryType   string          `json:"history_type,omitempty"`
	SourceLabel   string          `json:"source_label,omitempty"`
	CompletedAtMs int64           `json:"completed_at_ms"`
}

type ProduceHistorySnapshot struct {
	Items       []ProduceHistoryItem `json:"items"`
	UpdatedAtMs int64                `json:"updated_at_ms"`
}

type ExportProduceHistoryResult struct {
	Cancelled bool     `json:"cancelled"`
	TargetDir string   `json:"target_dir,omitempty"`
	Total     int      `json:"total"`
	Moved     int      `json:"moved"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors,omitempty"`
}

func (a *App) GetProduceTakeFiles() ProduceTakeFileSnapshot {
	a.produceStateMu.Lock()
	defer a.produceStateMu.Unlock()
	return a.takeFileSnapshotLocked()
}

func (a *App) getProduceHistorySnapshot() ProduceHistorySnapshot {
	a.produceStateMu.Lock()
	defer a.produceStateMu.Unlock()
	return a.produceHistorySnapshotLocked()
}

func (a *App) OpenProducedClipInFolder(videoPath string) error {
	target := strings.TrimSpace(videoPath)
	if target == "" {
		return fmt.Errorf("视频路径为空")
	}
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("视频文件不存在: %s", target)
	}
	if goruntime.GOOS != "windows" {
		return fmt.Errorf("当前系统暂不支持打开文件定位")
	}
	cmd := exec.Command("explorer.exe", "/select,", target)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开文件目录失败: %w", err)
	}
	return nil
}

func (a *App) ExportProduceHistoryVideos() (*ExportProduceHistoryResult, error) {
	result := &ExportProduceHistoryResult{}

	selected, err := openDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:                "选择导出文件夹",
		CanCreateDirectories: true,
	})
	if err != nil {
		return nil, err
	}
	targetDir := strings.TrimSpace(selected)
	if targetDir == "" {
		result.Cancelled = true
		return result, nil
	}
	targetDir = filepath.Clean(targetDir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("创建导出目录失败: %w", err)
	}
	result.TargetDir = targetDir

	a.produceStateMu.Lock()
	historySnapshot := a.produceHistorySnapshotLocked()
	a.produceStateMu.Unlock()

	result.Total = len(historySnapshot.Items)
	if result.Total == 0 {
		return result, nil
	}

	for _, item := range historySnapshot.Items {
		sourcePath := strings.TrimSpace(item.VideoPath)
		if sourcePath == "" {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("历史条目缺少视频路径: demo=%s take=%d", item.DemoPath, item.TakeIndex))
			continue
		}
		destPath := filepath.Join(targetDir, filepath.Base(sourcePath))
		if err := copyFileWithReplace(sourcePath, destPath); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("复制视频失败: %s -> %s: %v", sourcePath, destPath, err))
			continue
		}
		result.Moved++
	}
	return result, nil
}

func (a *App) resetProduceTakeFiles(results []GeneratePluginJSONBatchItemResult) {
	plans := collectTakePlans(results, false)
	a.produceStateMu.Lock()
	if a.produceState.takeFiles == nil {
		a.produceState.takeFiles = make(map[string]ProduceTakeFile, len(plans))
	} else {
		for key := range a.produceState.takeFiles {
			delete(a.produceState.takeFiles, key)
		}
	}
	a.produceState.takeFileOrder = a.produceState.takeFileOrder[:0]
	for _, plan := range plans {
		key := takeFileKey(plan.DemoPath, plan.TakeIndex, plan.View)
		if _, exists := a.produceState.takeFiles[key]; exists {
			continue
		}
		a.produceState.takeFiles[key] = ProduceTakeFile{
			DemoPath:    plan.DemoPath,
			TakeIndex:   plan.TakeIndex,
			TakeName:    plan.TakeName,
			View:        strings.TrimSpace(plan.View),
			Status:      "pending",
			UpdatedAtMs: nowMs(),
		}
		a.produceState.takeFileOrder = append(a.produceState.takeFileOrder, key)
	}
	snapshot := a.takeFileSnapshotLocked()
	a.produceStateMu.Unlock()
	a.emitTakeFilesSnapshot(snapshot)
}

func (a *App) updateTakeFileEntry(plan ProduceTakePlan, mutate func(file *ProduceTakeFile)) {
	if mutate == nil {
		return
	}
	key := takeFileKey(plan.DemoPath, plan.TakeIndex, plan.View)
	a.produceStateMu.Lock()
	file, exists := a.produceState.takeFiles[key]
	if !exists {
		file = ProduceTakeFile{
			DemoPath:  plan.DemoPath,
			TakeIndex: plan.TakeIndex,
			TakeName:  plan.TakeName,
			View:      plan.View,
			Status:    "pending",
		}
		a.produceState.takeFileOrder = append(a.produceState.takeFileOrder, key)
	}
	mutate(&file)
	a.produceState.takeFiles[key] = file
	snapshot := a.takeFileSnapshotLocked()
	a.produceStateMu.Unlock()
	a.emitTakeFilesSnapshot(snapshot)
}

func (a *App) takeFileSnapshotLocked() ProduceTakeFileSnapshot {
	snapshot := ProduceTakeFileSnapshot{
		Items:       make([]ProduceTakeFile, 0, len(a.produceState.takeFileOrder)),
		UpdatedAtMs: nowMs(),
	}
	for _, key := range a.produceState.takeFileOrder {
		item, ok := a.produceState.takeFiles[key]
		if !ok {
			continue
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	return snapshot
}

func (a *App) emitTakeFilesSnapshot(snapshot ProduceTakeFileSnapshot) {
	if a.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "produce_take_file_changed", snapshot)
}

func (a *App) produceHistorySnapshotLocked() ProduceHistorySnapshot {
	snapshot := ProduceHistorySnapshot{
		Items:       make([]ProduceHistoryItem, 0, len(a.produceState.historyOrder)),
		UpdatedAtMs: nowMs(),
	}
	for _, key := range a.produceState.historyOrder {
		item, ok := a.produceState.historyItems[key]
		if !ok {
			continue
		}
		cloned := item
		cloned.KillIDs = append([]string(nil), item.KillIDs...)
		if len(item.Kills) > 0 {
			cloned.Kills = append([]demo.ClipKill(nil), item.Kills...)
		}
		snapshot.Items = append(snapshot.Items, cloned)
	}
	return snapshot
}

func (a *App) emitProduceHistorySnapshot(snapshot ProduceHistorySnapshot) {
	if a.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "produce_history_changed", snapshot)
}

func (a *App) addProduceHistoryEntry(state *produceSessionRuntime, plan ProduceTakePlan, videoPath string) {
	if strings.TrimSpace(videoPath) == "" {
		return
	}
	key := plugingen.BuildProduceHistoryKeyWithSourceID(plan.DemoPath, plan.View, plan.SpecMode, plan.KillIDs, plan.SourceID)
	kills := resolveHistoryKills(state, plan)
	item := ProduceHistoryItem{
		DemoPath:      strings.TrimSpace(plan.DemoPath),
		TakeIndex:     plan.TakeIndex,
		TakeName:      strings.TrimSpace(plan.TakeName),
		View:          strings.TrimSpace(plan.View),
		SpecMode:      plan.SpecMode,
		KillIDs:       append([]string(nil), plan.KillIDs...),
		Kills:         append([]demo.ClipKill(nil), kills...),
		SourceID:      strings.TrimSpace(plan.SourceID),
		Round:         plan.Round,
		PlayerName:    strings.TrimSpace(plan.PlayerName),
		PlayerSteamID: strings.TrimSpace(plan.PlayerSteamID),
		StartTick:     plan.StartTick,
		EndTick:       plan.EndTick,
		EndReason:     strings.TrimSpace(plan.EndReason),
		VideoPath:     strings.TrimSpace(videoPath),
		HistoryType:   produceHistoryTypeProduce,
		SourceLabel:   "record_take",
		CompletedAtMs: nowMs(),
	}

	a.appendProduceHistoryItem(key, item)
}

func (a *App) addEditedHistoryEntry(videoPath string, sourceLabel string) {
	path := strings.TrimSpace(videoPath)
	if path == "" {
		return
	}

	completedAt := nowMs()
	item := ProduceHistoryItem{
		DemoPath:      "",
		TakeIndex:     0,
		TakeName:      "",
		View:          "",
		SpecMode:      0,
		KillIDs:       nil,
		Kills:         nil,
		VideoPath:     path,
		HistoryType:   produceHistoryTypeEdited,
		SourceLabel:   strings.TrimSpace(sourceLabel),
		CompletedAtMs: completedAt,
	}
	key := buildEditedHistoryKey(path, completedAt)
	a.appendProduceHistoryItem(key, item)
}

func buildEditedHistoryKey(videoPath string, completedAtMs int64) string {
	return "edited#" + strconv.FormatInt(completedAtMs, 10) + "#" + strings.ToLower(strings.TrimSpace(videoPath))
}

func (a *App) appendProduceHistoryItem(key string, item ProduceHistoryItem) {
	a.produceStateMu.Lock()
	if a.produceState.historyItems == nil {
		a.produceState.historyItems = make(map[string]ProduceHistoryItem)
	}
	if a.produceState.historyKeyIndex == nil {
		a.produceState.historyKeyIndex = make(map[string]struct{})
	}
	if item.HistoryType == produceHistoryTypeEdited || strings.HasPrefix(key, "edited#") {
		for {
			if _, exists := a.produceState.historyItems[key]; !exists {
				break
			}
			item.CompletedAtMs++
			key = buildEditedHistoryKey(item.VideoPath, item.CompletedAtMs)
		}
	}
	if _, exists := a.produceState.historyItems[key]; !exists {
		a.produceState.historyOrder = append(a.produceState.historyOrder, key)
	}
	a.produceState.historyItems[key] = item
	a.produceState.historyKeyIndex[key] = struct{}{}
	snapshot := a.produceHistorySnapshotLocked()
	a.produceStateMu.Unlock()
	a.emitProduceHistorySnapshot(snapshot)
}

func resolveHistoryKills(state *produceSessionRuntime, plan ProduceTakePlan) []demo.ClipKill {
	if state == nil {
		return nil
	}
	demoPath := strings.TrimSpace(plan.DemoPath)
	if demoPath == "" {
		return nil
	}
	killByID := state.killsByDemo[demoPath]
	if len(killByID) == 0 {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(plan.View)) == "full_round_pov" {
		playerSteamID := strings.TrimSpace(plan.PlayerSteamID)
		if playerSteamID == "" || plan.Round <= 0 {
			return nil
		}
		kills := make([]demo.ClipKill, 0, 4)
		for _, kill := range killByID {
			if kill.Round != plan.Round {
				continue
			}
			if strings.TrimSpace(kill.KillerSteamID) != playerSteamID {
				continue
			}
			kills = append(kills, kill)
		}
		sort.Slice(kills, func(i, j int) bool {
			if kills[i].Tick == kills[j].Tick {
				return kills[i].ID < kills[j].ID
			}
			return kills[i].Tick < kills[j].Tick
		})
		return kills
	}
	if len(plan.KillIDs) == 0 {
		return nil
	}
	kills := make([]demo.ClipKill, 0, len(plan.KillIDs))
	for _, killID := range plan.KillIDs {
		id := strings.TrimSpace(killID)
		if id == "" {
			continue
		}
		kill, exists := killByID[id]
		if !exists {
			continue
		}
		kills = append(kills, kill)
	}
	return kills
}

func normalizeProduceKillSnapshots(input map[string]map[string]demo.ClipKill) map[string]map[string]demo.ClipKill {
	if len(input) == 0 {
		return map[string]map[string]demo.ClipKill{}
	}
	normalized := make(map[string]map[string]demo.ClipKill, len(input))
	for demoPath, killByID := range input {
		dp := strings.TrimSpace(demoPath)
		if dp == "" || len(killByID) == 0 {
			continue
		}
		dst := make(map[string]demo.ClipKill, len(killByID))
		for killID, kill := range killByID {
			id := strings.TrimSpace(killID)
			if id == "" {
				continue
			}
			clone := kill
			clone.ID = strings.TrimSpace(clone.ID)
			if clone.ID == "" {
				clone.ID = id
			}
			dst[id] = clone
		}
		if len(dst) > 0 {
			normalized[dp] = dst
		}
	}
	return normalized
}

// ---- helper functions moved from produce_session.go ----

func collectTakePlans(results []GeneratePluginJSONBatchItemResult, onlySuccess bool) []ProduceTakePlan {
	plans := make([]ProduceTakePlan, 0)
	for _, item := range results {
		if onlySuccess && item.Error != "" {
			continue
		}
		for _, plan := range item.TakePlans {
			plans = append(plans, ProduceTakePlan{
				DemoPath:      strings.TrimSpace(plan.DemoPath),
				TakeIndex:     plan.TakeIndex,
				TakeName:      strings.TrimSpace(plan.TakeName),
				View:          strings.TrimSpace(plan.View),
				SpecMode:      plan.SpecMode,
				KillIDs:       append([]string(nil), plan.KillIDs...),
				SourceID:      strings.TrimSpace(plan.SourceID),
				Round:         plan.Round,
				PlayerName:    strings.TrimSpace(plan.PlayerName),
				PlayerSteamID: strings.TrimSpace(plan.PlayerSteamID),
				StartTick:     plan.StartTick,
				EndTick:       plan.EndTick,
				EndReason:     strings.TrimSpace(plan.EndReason),
			})
		}
	}
	return plans
}

func (a *App) resolveProduceSessionPaths(batchTimestamp string) (string, string) {
	cfg, err := a.loadConfig()
	if err != nil {
		return "", ""
	}
	recordOutputDir := config.CleanPath(cfg.RecordOutputDir)
	batchDir := recordOutputDir
	ts := strings.TrimSpace(batchTimestamp)
	if ts != "" {
		batchDir = filepath.Join(recordOutputDir, ts)
	}
	ffmpegExe := config.JoinExe(config.CleanPath(cfg.FFmpegDir), "ffmpeg.exe")
	return batchDir, ffmpegExe
}

func takePlanKey(demoPath string, takeIndex int) string {
	return strings.TrimSpace(demoPath) + "#" + strconv.Itoa(takeIndex)
}

func takeFileKey(demoPath string, takeIndex int, view string) string {
	return strings.TrimSpace(demoPath) + "#" + strconv.Itoa(takeIndex) + "#" + strings.TrimSpace(view)
}

func nowMs() int64 {
	return time.Now().UnixMilli()
}
