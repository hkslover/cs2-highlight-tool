package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/plugingen"
	"cs2-highlight-tool-v2/internal/producews"
)

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
	snapshot := a.workspaceSnapshot()
	if snapshot.pendingReset || snapshot.resetComplete || (snapshot.session != nil && snapshot.session.isClosed()) {
		return producews.TakeStatusSnapshot{}
	}
	return a.produceW.GetTakeSnapshot()
}

func (a *App) GetProduceHistorySnapshot() ProduceHistorySnapshot {
	return a.getProduceHistorySnapshot()
}

func (a *App) GeneratePluginJSONBatch(req GeneratePluginJSONBatchRequest) (*GeneratePluginJSONBatchResult, error) {
	// Serialize with the launch pipeline: both reset the shared take-file
	// state during setup.
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
		return nil, err
	}
	if err := a.stopProduceSessionWorker(); err != nil {
		return nil, err
	}
	jobs, err := normalizeGeneratePluginBatchJobs(req.Jobs)
	if err != nil {
		return nil, err
	}
	generation, err := a.captureGenerationSnapshot()
	if err != nil {
		return nil, err
	}
	results := make([]GeneratePluginJSONBatchItemResult, 0, len(req.Jobs))
	successCount := 0
	failureCount := 0
	batchTimestamp := time.Now().Format("20060102_150405")
	demoPaths0 := make([]string, len(jobs))
	for i, j := range jobs {
		demoPaths0[i] = j.DemoPath
	}
	recordSubDirs := plugingen.BuildBatchRecordSubDirs(demoPaths0)
	for idx, job := range jobs {
		item := GeneratePluginJSONBatchItemResult{DemoPath: strings.TrimSpace(job.DemoPath)}
		job.BatchTimestamp = batchTimestamp
		result, _, err := a.generatePluginJSONInternalWithSnapshot(job, generatePluginJSONInternalOptions{
			WriteJSON:    true,
			RecordSubDir: recordSubDirs[idx],
		}, generation)
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

func normalizeGeneratePluginBatchJobs(input []GeneratePluginJSONRequest) ([]GeneratePluginJSONRequest, error) {
	jobs := append([]GeneratePluginJSONRequest(nil), input...)
	for idx := range jobs {
		demoPath := strings.TrimSpace(jobs[idx].DemoPath)
		if demoPath == "" {
			continue
		}
		absDemoPath, err := filepath.Abs(demoPath)
		if err != nil {
			return nil, fmt.Errorf("解析 demo 路径失败: %w", err)
		}
		jobs[idx].DemoPath = absDemoPath
	}
	return jobs, nil
}

func (a *App) GeneratePluginJSON(req GeneratePluginJSONRequest) (*GeneratePluginJSONResult, error) {
	a.produceLaunchMu.Lock()
	defer a.produceLaunchMu.Unlock()
	releaseWorkspace, _, workspaceErr := a.beginManagedWorkspaceUse()
	if workspaceErr != nil {
		return nil, workspaceErr
	}
	defer releaseWorkspace()
	if err := a.produceBusyError(); err != nil {
		return nil, err
	}
	if err := a.stopProduceSessionWorker(); err != nil {
		return nil, err
	}

	result, _, err := a.generatePluginJSONInternal(req, generatePluginJSONInternalOptions{
		WriteJSON: true,
	})
	return result, err
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
