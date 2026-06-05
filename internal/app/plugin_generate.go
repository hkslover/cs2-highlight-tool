package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/demo"
	"cs2-highlight-tool-v2/internal/ffmpegprofile"
	"cs2-highlight-tool-v2/internal/plugingen"
	"cs2-highlight-tool-v2/internal/producews"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type GeneratePluginJSONRequest struct {
	DemoPath       string             `json:"demo_path"`
	TickRate       float64            `json:"tick_rate"`
	SelectedItems  []SelectedClipItem `json:"selected_items,omitempty"`
	ExtraCommands  []string           `json:"extra_commands,omitempty"`
	BatchTimestamp string             `json:"batch_timestamp,omitempty"`

	// Deprecated compatibility input. New callers should use SelectedItems.
	SelectedKills []demo.ClipKill `json:"selected_kills,omitempty"`
}

type GeneratePluginJSONResult struct {
	JSONPath      string            `json:"json_path"`
	SequenceCount int               `json:"sequence_count"`
	SegmentCount  int               `json:"segment_count"`
	ActionCount   int               `json:"action_count"`
	TakePlans     []ProduceTakePlan `json:"take_plans,omitempty"`
}

type GeneratePluginJSONBatchRequest struct {
	Jobs  []GeneratePluginJSONRequest   `json:"jobs"`
	Debug *GeneratePluginJSONBatchDebug `json:"debug,omitempty"`
}

type GeneratePluginJSONBatchDebug struct {
	KeepIntermediateFiles bool `json:"keep_intermediate_files"`
}

type GeneratePluginJSONBatchItemResult struct {
	DemoPath         string            `json:"demo_path"`
	JSONPath         string            `json:"json_path,omitempty"`
	SequenceCount    int               `json:"sequence_count,omitempty"`
	SegmentCount     int               `json:"segment_count,omitempty"`
	ActionCount      int               `json:"action_count,omitempty"`
	TakePlans        []ProduceTakePlan `json:"take_plans,omitempty"`
	GeneratedTakeCnt int               `json:"generated_take_count,omitempty"`
	SkippedByHistory bool              `json:"skipped_by_history,omitempty"`
	SkippedReason    string            `json:"skipped_reason,omitempty"`
	Error            string            `json:"error,omitempty"`
}

type GeneratePluginJSONBatchResult struct {
	Results          []GeneratePluginJSONBatchItemResult `json:"results"`
	SuccessCount     int                                 `json:"success_count"`
	FailureCount     int                                 `json:"failure_count"`
	BatchTimestamp   string                              `json:"batch_timestamp,omitempty"`
	LaunchStarted    bool                                `json:"launch_started,omitempty"`
	LaunchedDemoPath string                              `json:"launched_demo_path,omitempty"`
	LaunchError      string                              `json:"launch_error,omitempty"`
}

type ProduceTakePlan struct {
	DemoPath  string   `json:"demo_path"`
	TakeIndex int      `json:"take_index"`
	TakeName  string   `json:"take_name,omitempty"`
	View      string   `json:"view"`
	SpecMode  int      `json:"spec_mode"`
	KillIDs   []string `json:"kill_ids"`
}

type pluginAction = clipsjson.Action
type pluginSequence = clipsjson.Sequence

type generatePluginJSONInternalOptions struct {
	ItemsOverride []clipsjson.Item
	WriteJSON     bool
	RecordSubDir  string
}

type normalizedSelectedItems struct {
	Items                   []clipsjson.Item
	HasVoiceOverride        bool
	HasSpecShowXrayOverride bool
}

func (a *App) GetProduceWSState() producews.WSState {
	if a.produceW == nil {
		return producews.WSState{}
	}
	return a.produceW.GetWSState()
}

func (a *App) GetProduceQueueState() producews.QueueState {
	if a.produceW == nil {
		return producews.QueueState{CurrentIndex: -1}
	}
	return a.produceW.GetQueueState()
}

func (a *App) GetProduceTakeSnapshot() producews.TakeStatusSnapshot {
	if a.produceW == nil {
		return producews.TakeStatusSnapshot{}
	}
	return a.produceW.GetTakeSnapshot()
}

func (a *App) GetProduceHistorySnapshot() ProduceHistorySnapshot {
	return a.getProduceHistorySnapshot()
}

func (a *App) GeneratePluginJSONBatch(req GeneratePluginJSONBatchRequest) (*GeneratePluginJSONBatchResult, error) {
	if len(req.Jobs) == 0 {
		return nil, fmt.Errorf("没有可生成的 demo 任务")
	}
	results := make([]GeneratePluginJSONBatchItemResult, 0, len(req.Jobs))
	successCount := 0
	failureCount := 0
	batchTimestamp := time.Now().Format("20060102_150405")
	demoPaths0 := make([]string, len(req.Jobs))
	for i, j := range req.Jobs {
		demoPaths0[i] = j.DemoPath
	}
	recordSubDirs := plugingen.BuildBatchRecordSubDirs(demoPaths0)
	for idx, job := range req.Jobs {
		item := GeneratePluginJSONBatchItemResult{DemoPath: strings.TrimSpace(job.DemoPath)}
		job.BatchTimestamp = batchTimestamp
		result, _, err := a.generatePluginJSONInternal(job, generatePluginJSONInternalOptions{
			WriteJSON:    true,
			RecordSubDir: recordSubDirs[idx],
		})
		if err != nil {
			failureCount++
			item.Error = err.Error()
			results = append(results, item)
			continue
		}
		successCount++
		item.JSONPath = result.JSONPath
		item.SequenceCount = result.SequenceCount
		item.SegmentCount = result.SegmentCount
		item.ActionCount = result.ActionCount
		item.TakePlans = append([]ProduceTakePlan(nil), result.TakePlans...)
		item.GeneratedTakeCnt = len(item.TakePlans)
		results = append(results, item)
	}
	batchResult := &GeneratePluginJSONBatchResult{
		Results:        results,
		SuccessCount:   successCount,
		FailureCount:   failureCount,
		BatchTimestamp: batchTimestamp,
	}
	a.resetProduceTakeFiles(batchResult.Results)
	return batchResult, nil
}

func (a *App) GeneratePluginJSONBatchAndLaunchHLAE(req GeneratePluginJSONBatchRequest) (*GeneratePluginJSONBatchResult, error) {
	if len(req.Jobs) == 0 {
		return nil, fmt.Errorf("没有可生成的 demo 任务")
	}

	batchTimestamp := time.Now().Format("20060102_150405")
	demoPaths1 := make([]string, len(req.Jobs))
	for i, j := range req.Jobs {
		demoPaths1[i] = j.DemoPath
	}
	recordSubDirs := plugingen.BuildBatchRecordSubDirs(demoPaths1)
	results := make([]GeneratePluginJSONBatchItemResult, len(req.Jobs))
	contexts := make([]*launchJobContext, len(req.Jobs))
	failureCount := 0

	for idx, job := range req.Jobs {
		item := GeneratePluginJSONBatchItemResult{DemoPath: strings.TrimSpace(job.DemoPath)}
		job.BatchTimestamp = batchTimestamp
		preview, normalizedItems, err := a.generatePluginJSONInternal(job, generatePluginJSONInternalOptions{
			WriteJSON:    false,
			RecordSubDir: recordSubDirs[idx],
		})
		if err != nil {
			item.Error = err.Error()
			results[idx] = item
			failureCount++
			continue
		}
		contexts[idx] = &launchJobContext{
			job:      job,
			baseItem: item,
			allItems: normalizedItems,
			plans:    append([]ProduceTakePlan(nil), preview.TakePlans...),
		}
		results[idx] = item
	}

	historyKeys := a.getProduceHistoryKeySet()
	type runnableJob struct {
		index        int
		job          GeneratePluginJSONRequest
		items        []clipsjson.Item
		recordSubDir string
	}
	runnables := make([]runnableJob, 0, len(req.Jobs))
	for idx, ctx := range contexts {
		if ctx == nil {
			continue
		}
		filteredItems := filterItemsByHistory(ctx.allItems, ctx.plans, historyKeys)
		if len(filteredItems) == 0 {
			item := ctx.baseItem
			item.TakePlans = nil
			item.GeneratedTakeCnt = 0
			item.SkippedByHistory = true
			item.SkippedReason = "本 DEM 选中片段已在本次会话制作完成"
			results[idx] = item
			continue
		}
		filteredPreview, _, err := a.generatePluginJSONInternal(ctx.job, generatePluginJSONInternalOptions{
			ItemsOverride: filteredItems,
			WriteJSON:     false,
			RecordSubDir:  recordSubDirs[idx],
		})
		if err != nil {
			item := ctx.baseItem
			item.TakePlans = nil
			item.GeneratedTakeCnt = 0
			item.Error = err.Error()
			results[idx] = item
			failureCount++
			continue
		}
		if len(filteredPreview.TakePlans) == 0 {
			item := ctx.baseItem
			item.TakePlans = nil
			item.GeneratedTakeCnt = 0
			item.SkippedByHistory = true
			item.SkippedReason = "本 DEM 选中片段已在本次会话制作完成"
			results[idx] = item
			continue
		}
		runnables = append(runnables, runnableJob{
			index:        idx,
			job:          ctx.job,
			items:        filteredItems,
			recordSubDir: recordSubDirs[idx],
		})
	}

	successfulDemos := make([]string, 0, len(runnables))
	demoSubDirByDemoPath := make(map[string]string, len(runnables))
	killSnapshotByDemoPath := make(map[string]map[string]demo.ClipKill, len(runnables))
	successCount := 0
	for _, run := range runnables {
		item := results[run.index]
		generated, _, err := a.generatePluginJSONInternal(run.job, generatePluginJSONInternalOptions{
			ItemsOverride: run.items,
			WriteJSON:     true,
			RecordSubDir:  run.recordSubDir,
		})
		if err != nil {
			item.Error = err.Error()
			results[run.index] = item
			failureCount++
			continue
		}
		item.JSONPath = generated.JSONPath
		item.SequenceCount = generated.SequenceCount
		item.SegmentCount = generated.SegmentCount
		item.ActionCount = generated.ActionCount
		item.TakePlans = append([]ProduceTakePlan(nil), generated.TakePlans...)
		item.GeneratedTakeCnt = len(item.TakePlans)
		item.SkippedByHistory = false
		item.SkippedReason = ""
		results[run.index] = item
		registerProduceKillSnapshot(killSnapshotByDemoPath, item.TakePlans, run.items, run.job.DemoPath)
		successCount++
		demoPath := strings.TrimSpace(item.DemoPath)
		if demoPath != "" {
			successfulDemos = append(successfulDemos, demoPath)
			if _, exists := demoSubDirByDemoPath[demoPath]; !exists {
				demoSubDirByDemoPath[demoPath] = strings.TrimSpace(run.recordSubDir)
			}
		}
	}

	result := &GeneratePluginJSONBatchResult{
		Results:        results,
		SuccessCount:   successCount,
		FailureCount:   failureCount,
		BatchTimestamp: batchTimestamp,
	}
	a.resetProduceTakeFiles(result.Results)

	launchDemoPath := ""
	if len(successfulDemos) > 0 {
		launchDemoPath = successfulDemos[0]
	}
	if launchDemoPath == "" {
		result.LaunchStarted = false
		if failureCount > 0 {
			result.LaunchError = "没有可启动的 demo（JSON 生成全部失败）"
		} else {
			result.LaunchError = "无需录制：所选片段均已在本次会话制作完成"
		}
		return result, nil
	}
	if a.produceW != nil && a.produceW.GetQueueState().Running {
		result.LaunchStarted = false
		result.LaunchError = "当前已有制作队列在运行中"
		result.LaunchedDemoPath = launchDemoPath
		return result, nil
	}

	if err := a.prepareGameInfoForProduce(); err != nil {
		result.LaunchStarted = false
		result.LaunchError = err.Error()
		result.LaunchedDemoPath = launchDemoPath
		return result, nil
	}
	if err := a.preparePluginDLLForProduce(); err != nil {
		result.LaunchStarted = false
		result.LaunchError = err.Error()
		result.LaunchedDemoPath = launchDemoPath
		if restoreErr := a.forceRestoreProduceEnvironmentForProduce(); restoreErr != nil {
			result.LaunchError = fmt.Sprintf("%s; 恢复制作环境失败: %v", result.LaunchError, restoreErr)
		}
		return result, nil
	}
	if err := a.preparePovForProduce(); err != nil {
		result.LaunchStarted = false
		result.LaunchError = err.Error()
		result.LaunchedDemoPath = launchDemoPath
		if restoreErr := a.forceRestoreProduceEnvironmentForProduce(); restoreErr != nil {
			result.LaunchError = fmt.Sprintf("%s; 恢复制作环境失败: %v", result.LaunchError, restoreErr)
		}
		return result, nil
	}

	cs2PID, err := a.launchHLAEGame()
	if err != nil {
		result.LaunchStarted = false
		result.LaunchError = err.Error()
		result.LaunchedDemoPath = launchDemoPath
		if restoreErr := a.forceRestoreProduceEnvironmentForProduce(); restoreErr != nil {
			result.LaunchError = fmt.Sprintf("%s; 恢复制作环境失败: %v", result.LaunchError, restoreErr)
		}
		return result, nil
	}
	if a.produceW == nil {
		result.LaunchStarted = false
		result.LaunchError = "制作 websocket 服务未初始化"
		result.LaunchedDemoPath = launchDemoPath
		if closeErr := closeCS2ProcessByPIDFn(cs2PID); closeErr != nil && a.ctx != nil {
			wailsruntime.LogError(a.ctx, fmt.Sprintf("close cs2 failed after launch error (pid=%d): %v", cs2PID, closeErr))
		}
		if restoreErr := a.forceRestoreProduceEnvironmentForProduce(); restoreErr != nil {
			result.LaunchError = fmt.Sprintf("%s; 恢复制作环境失败: %v", result.LaunchError, restoreErr)
		}
		return result, nil
	}
	if err := a.produceW.StartQueue(successfulDemos); err != nil {
		result.LaunchStarted = false
		result.LaunchError = err.Error()
		result.LaunchedDemoPath = launchDemoPath
		if closeErr := closeCS2ProcessByPIDFn(cs2PID); closeErr != nil {
			result.LaunchError = fmt.Sprintf("%s; 关闭 cs2 失败: %v", result.LaunchError, closeErr)
			if a.ctx != nil {
				wailsruntime.LogError(a.ctx, fmt.Sprintf("close cs2 failed after queue start error (pid=%d): %v", cs2PID, closeErr))
			}
		}
		if restoreErr := a.forceRestoreProduceEnvironmentForProduce(); restoreErr != nil {
			result.LaunchError = fmt.Sprintf("%s; 恢复制作环境失败: %v", result.LaunchError, restoreErr)
		}
		return result, nil
	}

	batchDir, ffmpegExe := a.resolveProduceSessionPaths(result.BatchTimestamp)
	keepIntermediateFiles := req.Debug != nil && req.Debug.KeepIntermediateFiles
	a.startProduceSessionWorker(
		batchDir,
		ffmpegExe,
		demoSubDirByDemoPath,
		killSnapshotByDemoPath,
		result.Results,
		cs2PID,
		keepIntermediateFiles,
	)

	result.LaunchStarted = true
	result.LaunchedDemoPath = launchDemoPath
	return result, nil
}

func (a *App) GeneratePluginJSON(req GeneratePluginJSONRequest) (*GeneratePluginJSONResult, error) {
	result, _, err := a.generatePluginJSONInternal(req, generatePluginJSONInternalOptions{
		WriteJSON: true,
	})
	return result, err
}

func (a *App) generatePluginJSONInternal(
	req GeneratePluginJSONRequest,
	opts generatePluginJSONInternalOptions,
) (*GeneratePluginJSONResult, []clipsjson.Item, error) {
	demoPath := strings.TrimSpace(req.DemoPath)
	if demoPath == "" {
		return nil, nil, fmt.Errorf("demo 路径为空")
	}
	absDemoPath, err := filepath.Abs(demoPath)
	if err != nil {
		return nil, nil, fmt.Errorf("解析 demo 路径失败: %w", err)
	}
	if _, err := os.Stat(absDemoPath); err != nil {
		return nil, nil, fmt.Errorf("demo 文件不存在: %s", absDemoPath)
	}

	cfg, err := config.LoadOrCreate(a.configPath(), a.dataRoot())
	if err != nil {
		return nil, nil, err
	}

	// 如果 FFmpeg 能力检测数据为空，尝试同步快速检测以确保 GPU 加速可用
	if cfg.FFmpegDetectedPreset == "" && len(cfg.FFmpegDetectedEncoders) == 0 {
		ffmpegExe := config.JoinExe(config.CleanPath(cfg.FFmpegDir), "ffmpeg.exe")
		if ffmpegExe != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			caps, detectErr := ffmpegprofile.DetectCapabilities(ctx, ffmpegExe, nil)
			cancel()
			if detectErr == nil && !caps.IsEmpty() {
				resolved := ffmpegprofile.ResolveProfile(ffmpegprofile.UserPresetAuto, caps)
				cfg.FFmpegDetectedPreset = resolved.SelectedProfile.ID
				cfg.FFmpegDetectedEncoders = caps.AvailableEncoders()
				cfg.FFmpegDetectedAt = time.Now().UTC().Format(time.RFC3339)
				_ = config.Save(a.configPath(), cfg)
			}
		}
	}

	actionSettings := config.ResolveClipActionSettings(cfg)
	clipSettings := normalizeClipSettings(ClipSettings{
		KillerPreSeconds:   cfg.KillerPreSeconds,
		KillerPostSeconds:  cfg.KillerPostSeconds,
		VictimPreSeconds:   cfg.VictimPreSeconds,
		VictimPostSeconds:  cfg.VictimPostSeconds,
		AutoAddVictimView:  cfg.AutoAddVictimView,
		EnableVoice:        actionSettings.EnableVoiceIndices && actionSettings.EnableVoiceIndicesH,
		RecordFPS:          cfg.RecordFPS,
		VideoPreset:        cfg.VideoPreset,
		RecordOutputDir:    a.fixedRecordOutputDir(),
		EnableSpecShowXray: cfg.EnableSpecShowXray,
		HideAllUI:          cfg.HideAllUI,
	})

	var items []clipsjson.Item
	hasVoiceOverride := false
	hasSpecShowXrayOverride := false
	if len(opts.ItemsOverride) > 0 {
		items = append([]clipsjson.Item(nil), opts.ItemsOverride...)
		for _, item := range items {
			if item.HasVoiceOverride {
				hasVoiceOverride = true
			}
			if item.HasSpecShowXrayOverride {
				hasSpecShowXrayOverride = true
			}
		}
	} else {
		normalizedItems, normalizeErr := normalizeSelectedItems(req, clipSettings)
		if normalizeErr != nil {
			return nil, nil, normalizeErr
		}
		items = normalizedItems.Items
		hasVoiceOverride = normalizedItems.HasVoiceOverride
		hasSpecShowXrayOverride = normalizedItems.HasSpecShowXrayOverride
	}
	if len(items) == 0 {
		return nil, nil, fmt.Errorf("没有可导出的击杀片段")
	}

	batchTimestamp := strings.TrimSpace(req.BatchTimestamp)
	if batchTimestamp == "" {
		batchTimestamp = time.Now().Format("20060102_150405")
	}
	recordBatchName := batchTimestamp
	recordSubDir := strings.TrimSpace(opts.RecordSubDir)
	if recordSubDir != "" {
		if recordBatchName != "" {
			recordBatchName += "/" + recordSubDir
		} else {
			recordBatchName = recordSubDir
		}
	}
	buildResult, err := clipsjson.Build(items, clipsjson.BuildOptions{
		TickRate:          req.TickRate,
		KillerPreSeconds:  clipSettings.KillerPreSeconds,
		KillerPostSeconds: clipSettings.KillerPostSeconds,
		VictimPreSeconds:  clipSettings.VictimPreSeconds,
		VictimPostSeconds: clipSettings.VictimPostSeconds,
		ExtraCommands:     req.ExtraCommands,
		ActionSettings: clipsjson.ActionSettings{
			EnableVoiceIndices:  clipSettings.EnableVoice,
			VoiceIndicesValue:   0,
			EnableVoiceIndicesH: clipSettings.EnableVoice,
			VoiceIndicesHValue:  0,
		},
		RecordFPS:                 clipSettings.RecordFPS,
		RecordQuality:             clipSettings.RecordQuality,
		VideoPreset:               plugingen.ResolvePluginVideoPreset(clipSettings.VideoPreset, cfg),
		RecordOutputDir:           clipSettings.RecordOutputDir,
		RecordBatchName:           recordBatchName,
		EnableSpecShowXray:        clipSettings.EnableSpecShowXray,
		HideAllUI:                 clipSettings.HideAllUI,
		ForcePerPassVoiceCommands: hasVoiceOverride,
		ForcePerPassXrayCommands:  hasSpecShowXrayOverride,
		LaunchResolution:          cfg.LaunchResolution,
	})
	if err != nil {
		return nil, nil, err
	}

	jsonPath := absDemoPath + ".json"
	if opts.WriteJSON {
		payload, err := json.MarshalIndent(buildResult.Sequences, "", "  ")
		if err != nil {
			return nil, nil, fmt.Errorf("序列化 json 失败: %w", err)
		}
		payload = append(payload, '\n')
		if err := os.WriteFile(jsonPath, payload, 0644); err != nil {
			return nil, nil, fmt.Errorf("写入 json 失败: %w", err)
		}
	}

	actionCount := 0
	for _, sequence := range buildResult.Sequences {
		actionCount += len(sequence.Actions)
	}
	takePlans := make([]ProduceTakePlan, 0, len(buildResult.TakePlans))
	for _, plan := range buildResult.TakePlans {
		takePlans = append(takePlans, ProduceTakePlan{
			DemoPath:  absDemoPath,
			TakeIndex: plan.TakeIndex,
			TakeName:  plan.TakeName,
			View:      strings.TrimSpace(plan.View),
			SpecMode:  plan.SpecMode,
			KillIDs:   append([]string(nil), plan.KillIDs...),
		})
	}

	return &GeneratePluginJSONResult{
		JSONPath:      jsonPath,
		SequenceCount: len(buildResult.Sequences),
		SegmentCount:  buildResult.SegmentCount,
		ActionCount:   actionCount,
		TakePlans:     takePlans,
	}, append([]clipsjson.Item(nil), items...), nil
}

func (a *App) getProduceHistoryKeySet() map[string]struct{} {
	a.produceStateMu.Lock()
	defer a.produceStateMu.Unlock()
	result := make(map[string]struct{}, len(a.produceState.historyKeyIndex))
	for key := range a.produceState.historyKeyIndex {
		result[key] = struct{}{}
	}
	return result
}

// filterItemsByHistory is a thin app-layer wrapper around plugingen.FilterItemsByHistory.
// It converts ProduceTakePlan slices to the plugingen.TakePlan type expected by the
// lower-layer package.
func filterItemsByHistory(
	items []clipsjson.Item,
	plans []ProduceTakePlan,
	historyKeys map[string]struct{},
) []clipsjson.Item {
	return plugingen.FilterItemsByHistory(items, toPlugingenTakePlans(plans), historyKeys)
}

// toPlugingenTakePlans converts app-layer ProduceTakePlan slice to plugingen.TakePlan.
func toPlugingenTakePlans(plans []ProduceTakePlan) []plugingen.TakePlan {
	out := make([]plugingen.TakePlan, len(plans))
	for i, p := range plans {
		out[i] = plugingen.TakePlan{
			DemoPath: p.DemoPath,
			View:     p.View,
			SpecMode: p.SpecMode,
			KillIDs:  p.KillIDs,
		}
	}
	return out
}

func registerProduceKillSnapshot(
	store map[string]map[string]demo.ClipKill,
	plans []ProduceTakePlan,
	items []clipsjson.Item,
	fallbackDemoPath string,
) {
	if len(items) == 0 || store == nil {
		return
	}

	killByID := make(map[string]demo.ClipKill, len(items))
	for _, item := range items {
		killID := strings.TrimSpace(item.Kill.ID)
		if killID == "" {
			continue
		}
		kill := item.Kill
		kill.ID = killID
		if _, exists := killByID[killID]; !exists {
			killByID[killID] = kill
		}
	}
	if len(killByID) == 0 {
		return
	}

	demoPaths := make(map[string]struct{}, len(plans)+1)
	for _, plan := range plans {
		demoPath := strings.TrimSpace(plan.DemoPath)
		if demoPath != "" {
			demoPaths[demoPath] = struct{}{}
		}
	}
	if demoPath := strings.TrimSpace(fallbackDemoPath); demoPath != "" {
		demoPaths[demoPath] = struct{}{}
	}

	for demoPath := range demoPaths {
		if strings.TrimSpace(demoPath) == "" {
			continue
		}
		current := store[demoPath]
		if current == nil {
			current = make(map[string]demo.ClipKill, len(killByID))
			store[demoPath] = current
		}
		for killID, kill := range killByID {
			if _, exists := current[killID]; exists {
				continue
			}
			current[killID] = kill
		}
	}
}

func normalizeSelectedItems(req GeneratePluginJSONRequest, defaults ClipSettings) (*normalizedSelectedItems, error) {
	items := make([]SelectedClipItem, 0, len(req.SelectedItems)+len(req.SelectedKills))
	items = append(items, req.SelectedItems...)
	if len(items) == 0 && len(req.SelectedKills) > 0 {
		for _, kill := range req.SelectedKills {
			items = append(items, SelectedClipItem{Kill: kill, IncludeVictim: true})
		}
	}
	if len(items) == 0 {
		return &normalizedSelectedItems{}, nil
	}

	normalized := make([]clipsjson.Item, 0, len(items))
	hasVoiceOverride := false
	hasSpecShowXrayOverride := false
	for _, item := range items {
		killerPreSeconds := defaults.KillerPreSeconds
		killerPostSeconds := defaults.KillerPostSeconds
		victimPreSeconds := defaults.VictimPreSeconds
		victimPostSeconds := defaults.VictimPostSeconds
		enableVoice := defaults.EnableVoice
		enableSpecShowXray := defaults.EnableSpecShowXray
		itemHasVoiceOverride := false
		itemHasSpecShowXrayOverride := false

		if item.ClipOverrides != nil {
			if item.ClipOverrides.KillerPreSeconds != nil {
				killerPreSeconds = *item.ClipOverrides.KillerPreSeconds
			}
			if item.ClipOverrides.KillerPostSeconds != nil {
				killerPostSeconds = *item.ClipOverrides.KillerPostSeconds
			}
			if item.ClipOverrides.VictimPreSeconds != nil {
				victimPreSeconds = *item.ClipOverrides.VictimPreSeconds
			}
			if item.ClipOverrides.VictimPostSeconds != nil {
				victimPostSeconds = *item.ClipOverrides.VictimPostSeconds
			}
			if item.ClipOverrides.EnableVoice != nil {
				enableVoice = *item.ClipOverrides.EnableVoice
				itemHasVoiceOverride = true
				hasVoiceOverride = true
			}
			if item.ClipOverrides.EnableSpecShowXray != nil {
				enableSpecShowXray = *item.ClipOverrides.EnableSpecShowXray
				itemHasSpecShowXrayOverride = true
				hasSpecShowXrayOverride = true
			}
		}

		killerPreSeconds = normalizeSeconds(killerPreSeconds, defaults.KillerPreSeconds, 1, 5)
		killerPostSeconds = normalizeSeconds(killerPostSeconds, defaults.KillerPostSeconds, 1, 5)
		victimPreSeconds = normalizeSeconds(victimPreSeconds, defaults.VictimPreSeconds, 1, 2)
		victimPostSeconds = normalizeSeconds(victimPostSeconds, defaults.VictimPostSeconds, 1, 2)

		normalized = append(normalized, clipsjson.Item{
			Kill:                    item.Kill,
			IncludeVictim:           item.IncludeVictim,
			KillerSpecMode:          1,
			VictimSpecMode:          1,
			KillerPreSeconds:        killerPreSeconds,
			KillerPostSeconds:       killerPostSeconds,
			VictimPreSeconds:        victimPreSeconds,
			VictimPostSeconds:       victimPostSeconds,
			EnableVoice:             enableVoice,
			EnableSpecShowXray:      enableSpecShowXray,
			HasVoiceOverride:        itemHasVoiceOverride,
			HasSpecShowXrayOverride: itemHasSpecShowXrayOverride,
		})
	}
	return &normalizedSelectedItems{
		Items:                   normalized,
		HasVoiceOverride:        hasVoiceOverride,
		HasSpecShowXrayOverride: hasSpecShowXrayOverride,
	}, nil
}
