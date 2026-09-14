package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/clipsjson"
	"cs2-highlight-tool-v2/internal/demo"
	"cs2-highlight-tool-v2/internal/plugingen"
)

func (a *App) GeneratePluginJSONBatchAndLaunchHLAE(req GeneratePluginJSONBatchRequest) (*GeneratePluginJSONBatchResult, error) {
	// Hold the launch mutex for the entire pipeline (take-file reset, queue
	// check, old-session stop, environment preparation, HLAE launch, queue
	// start and runtime install). This is the session reservation: two
	// concurrent requests (e.g. a fast double-click) are serialized instead of
	// interleaving stop/prepare/restore steps against each other's
	// environment.
	a.produceLaunchMu.Lock()
	defer a.produceLaunchMu.Unlock()
	releaseWorkspace, _, workspaceErr := a.beginManagedWorkspaceUse()
	if workspaceErr != nil {
		return nil, workspaceErr
	}
	defer releaseWorkspace()
	if len(req.Jobs) == 0 {
		return nil, fmt.Errorf("没有可生成的 demo 任务")
	}
	if err := a.produceBusyError(); err != nil {
		return &GeneratePluginJSONBatchResult{Results: []GeneratePluginJSONBatchItemResult{}, LaunchError: err.Error()}, nil
	}
	// Retry only an already-finished session's failed teardown, before any
	// JSON writes, history filtering or shared take-state reset.
	if err := a.stopProduceSessionWorker(); err != nil {
		return &GeneratePluginJSONBatchResult{Results: []GeneratePluginJSONBatchItemResult{}, LaunchError: err.Error()}, nil
	}
	jobs, err := normalizeGeneratePluginBatchJobs(req.Jobs)
	if err != nil {
		return nil, err
	}
	generation, err := a.captureGenerationSnapshot()
	if err != nil {
		return nil, err
	}

	batchTimestamp := time.Now().Format("20060102_150405")
	demoPaths1 := make([]string, len(jobs))
	for i, j := range jobs {
		demoPaths1[i] = j.DemoPath
	}
	recordSubDirs := plugingen.BuildBatchRecordSubDirs(demoPaths1)
	results := make([]GeneratePluginJSONBatchItemResult, len(jobs))
	contexts := make([]*launchJobContext, len(jobs))
	failureCount := 0

	for idx, job := range jobs {
		item := GeneratePluginJSONBatchItemResult{DemoPath: strings.TrimSpace(job.DemoPath)}
		job.BatchTimestamp = batchTimestamp
		preview, normalizedItems, err := a.generatePluginJSONInternalWithSnapshot(job, generatePluginJSONInternalOptions{
			WriteJSON:    false,
			RecordSubDir: recordSubDirs[idx],
		}, generation)
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
		index                int
		job                  GeneratePluginJSONRequest
		items                []clipsjson.Item
		fullRoundPOVSegments []clipsjson.FullRoundPOVSegment
		recordSubDir         string
	}
	runnables := make([]runnableJob, 0, len(req.Jobs))
	for idx, ctx := range contexts {
		if ctx == nil {
			continue
		}
		filteredItems := filterItemsByHistory(ctx.allItems.Items, ctx.plans, historyKeys)
		filteredFullRoundSegments := filterFullRoundPOVSegmentsByHistory(
			ctx.allItems.FullRoundPOVSegments,
			ctx.plans,
			historyKeys,
			ctx.job.DemoPath,
		)
		if len(filteredItems) == 0 && len(filteredFullRoundSegments) == 0 {
			item := ctx.baseItem
			item.TakePlans = nil
			item.GeneratedTakeCnt = 0
			item.SkippedByHistory = true
			item.SkippedReason = "本 DEM 选中片段已在本次会话制作完成"
			results[idx] = item
			continue
		}
		filteredPreview, _, err := a.generatePluginJSONInternalWithSnapshot(ctx.job, generatePluginJSONInternalOptions{
			ItemsOverride:                   filteredItems,
			UseItemsOverride:                true,
			FullRoundPOVSegmentsOverride:    filteredFullRoundSegments,
			UseFullRoundPOVSegmentsOverride: true,
			WriteJSON:                       false,
			RecordSubDir:                    recordSubDirs[idx],
		}, generation)
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
			index:                idx,
			job:                  ctx.job,
			items:                filteredItems,
			fullRoundPOVSegments: filteredFullRoundSegments,
			recordSubDir:         recordSubDirs[idx],
		})
	}

	successfulDemos := make([]string, 0, len(runnables))
	demoSubDirByDemoPath := make(map[string]string, len(runnables))
	killSnapshotByDemoPath := make(map[string]map[string]demo.ClipKill, len(runnables))
	successCount := 0
	for _, run := range runnables {
		item := results[run.index]
		generated, _, err := a.generatePluginJSONInternalWithSnapshot(run.job, generatePluginJSONInternalOptions{
			ItemsOverride:                   run.items,
			UseItemsOverride:                true,
			FullRoundPOVSegmentsOverride:    run.fullRoundPOVSegments,
			UseFullRoundPOVSegmentsOverride: true,
			WriteJSON:                       true,
			RecordSubDir:                    run.recordSubDir,
		}, generation)
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

	a.resetProduceTakeFiles(result.Results)
	envEpoch := a.beginProduceEnvironmentPrep()

	if err := a.prepareGameInfoForProduce(); err != nil {
		result.LaunchStarted = false
		result.LaunchError = err.Error()
		result.LaunchedDemoPath = launchDemoPath
		if rollbackErr := a.rollbackProduceLaunchEnvironment(0, envEpoch); rollbackErr != nil {
			result.LaunchError = fmt.Sprintf("%s; 回滚制作环境失败: %v", result.LaunchError, rollbackErr)
		}
		return result, nil
	}
	if err := a.preparePluginDLLForProduce(); err != nil {
		result.LaunchStarted = false
		result.LaunchError = err.Error()
		result.LaunchedDemoPath = launchDemoPath
		if rollbackErr := a.rollbackProduceLaunchEnvironment(0, envEpoch); rollbackErr != nil {
			result.LaunchError = fmt.Sprintf("%s; 回滚制作环境失败: %v", result.LaunchError, rollbackErr)
		}
		return result, nil
	}
	if err := a.preparePovForProduce(); err != nil {
		result.LaunchStarted = false
		result.LaunchError = err.Error()
		result.LaunchedDemoPath = launchDemoPath
		if rollbackErr := a.rollbackProduceLaunchEnvironment(0, envEpoch); rollbackErr != nil {
			result.LaunchError = fmt.Sprintf("%s; 回滚制作环境失败: %v", result.LaunchError, rollbackErr)
		}
		return result, nil
	}

	cs2PID, err := a.launchHLAEGame()
	if err != nil {
		result.LaunchStarted = false
		result.LaunchError = err.Error()
		result.LaunchedDemoPath = launchDemoPath
		var launchErr *hlaeLaunchError
		var pending *pendingHLAELaunch
		if errors.As(err, &launchErr) {
			pending = launchErr.pending
		}
		if rollbackErr := a.rollbackProduceLaunchEnvironment(0, envEpoch, pending); rollbackErr != nil {
			result.LaunchError = fmt.Sprintf("%s; 回滚制作环境失败: %v", result.LaunchError, rollbackErr)
		}
		return result, nil
	}
	if a.produceW == nil {
		result.LaunchStarted = false
		result.LaunchError = "制作 websocket 服务未初始化"
		result.LaunchedDemoPath = launchDemoPath
		if rollbackErr := a.rollbackProduceLaunchEnvironment(cs2PID, envEpoch); rollbackErr != nil {
			result.LaunchError = fmt.Sprintf("%s; 回滚制作环境失败: %v", result.LaunchError, rollbackErr)
		}
		return result, nil
	}
	if err := a.produceW.StartQueue(successfulDemos); err != nil {
		result.LaunchStarted = false
		result.LaunchError = err.Error()
		result.LaunchedDemoPath = launchDemoPath
		if rollbackErr := a.rollbackProduceLaunchEnvironment(cs2PID, envEpoch); rollbackErr != nil {
			result.LaunchError = fmt.Sprintf("%s; 回滚制作环境失败: %v", result.LaunchError, rollbackErr)
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
		envEpoch,
	)

	result.LaunchStarted = true
	result.LaunchedDemoPath = launchDemoPath
	return result, nil
}
