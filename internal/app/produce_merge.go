package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/plugingen"
	"cs2-highlight-tool-v2/internal/producemerge"
	"cs2-highlight-tool-v2/internal/producews"
)

type pendingCompletedTake struct {
	plan ProduceTakePlan
}

type mergeTask struct {
	plan      ProduceTakePlan
	videoPath string
	audioPath string
}

func (a *App) enqueueCompletedTakes(state *produceSessionRuntime, snapshot producews.TakeStatusSnapshot) {
	for _, item := range snapshot.Items {
		if strings.TrimSpace(item.Status) != "completed" {
			continue
		}
		demoPath := strings.TrimSpace(item.DemoPath)
		takeIndex := item.TakeIndex
		if demoPath == "" || takeIndex <= 0 {
			continue
		}
		key := takePlanKey(demoPath, takeIndex)
		if _, exists := state.seenCompleted[key]; exists {
			continue
		}
		state.seenCompleted[key] = struct{}{}

		plan, ok := state.plansByTake[key]
		if !ok {
			plan = ProduceTakePlan{
				DemoPath:  demoPath,
				TakeIndex: takeIndex,
				TakeName:  strings.TrimSpace(item.TakeName),
				View:      "",
			}
		}
		plan = mergeObservedTakeStatus(plan, item)
		state.endedQueue = append(state.endedQueue, pendingCompletedTake{plan: plan})

		a.updateTakeFileEntry(plan, func(file *ProduceTakeFile) {
			file.Status = "recorded"
			file.Error = ""
			file.UpdatedAtMs = nowMs()
		})
	}
}

// mergeObservedTakeStatus replaces only recorder timing metadata. The source
// event window (StartTick/EndTick) remains the plan's demo-event baseline, so
// a missing or partial WS observation never makes the editor guess a new clip
// window.
func mergeObservedTakeStatus(plan ProduceTakePlan, status producews.TakeStatus) ProduceTakePlan {
	if status.RecordStartTick > 0 {
		plan.RecordStartTick = status.RecordStartTick
	}
	if status.RecordEndTick > 0 {
		plan.RecordEndTick = status.RecordEndTick
	}
	return plan
}

func (a *App) dispatchMergeTasks(state *produceSessionRuntime) {
	for len(state.endedQueue) > 0 {
		completed := state.endedQueue[0]

		videoPath, audioPath := expectedTakePaths(state, completed.plan)
		task := mergeTask{plan: completed.plan, videoPath: videoPath, audioPath: audioPath}

		state.pendingTaskCnt.Add(1)
		select {
		case state.taskCh <- task:
			state.endedQueue = state.endedQueue[1:]
			a.updateTakeFileEntry(completed.plan, func(file *ProduceTakeFile) {
				file.VideoPath = videoPath
				file.AudioPath = audioPath
				if file.Status != "processing" && file.Status != "completed" && file.Status != "failed" {
					file.Status = "waiting_files"
					file.Error = ""
				}
				file.UpdatedAtMs = nowMs()
			})
		default:
			state.pendingTaskCnt.Add(-1)
			return
		}
	}
}

func (a *App) mergeWorker(ctx context.Context, state *produceSessionRuntime) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-state.taskCh:
			if !ok {
				return
			}
			func() {
				// Decrement inside defer so no error path (including a panic
				// or a hung ffmpeg that gets cancelled) can leave
				// pendingTaskCnt non-zero and block session shutdown.
				defer state.pendingTaskCnt.Add(-1)
				a.handleMergeTask(ctx, state, task)
			}()
		}
	}
}

func (a *App) handleMergeTask(ctx context.Context, state *produceSessionRuntime, task mergeTask) {
	if err := producemerge.WaitForTakeFilesReady(
		ctx,
		state.ffmpegExe,
		task.videoPath,
		task.audioPath,
		state.fileReadyTimeout,
		state.stableInterval,
	); err != nil {
		a.updateTakeFileEntry(task.plan, func(file *ProduceTakeFile) {
			file.Status = "failed"
			file.Error = err.Error()
			file.UpdatedAtMs = nowMs()
		})
		return
	}

	a.updateTakeFileEntry(task.plan, func(file *ProduceTakeFile) {
		file.Status = "processing"
		file.Error = ""
		file.UpdatedAtMs = nowMs()
	})

	// Merge under the session ctx: session cancellation kills the ffmpeg
	// process immediately (instead of blocking the worker forever), and
	// producemerge additionally bounds the mux with its own timeout.
	finalVideoPath, err := producemerge.MergeTakeVideoAudioContext(ctx, state.ffmpegExe, task.videoPath, task.audioPath)
	if err != nil {
		a.updateTakeFileEntry(task.plan, func(file *ProduceTakeFile) {
			file.Status = "failed"
			file.Error = err.Error()
			file.UpdatedAtMs = nowMs()
		})
		return
	}

	a.updateTakeFileEntry(task.plan, func(file *ProduceTakeFile) {
		file.Status = "completed"
		file.Error = ""
		file.AudioPath = ""
		file.VideoPath = finalVideoPath
		file.UpdatedAtMs = nowMs()
	})
	a.addProduceHistoryEntry(state, task.plan, finalVideoPath)
}

// waitForTakeFilesReady is a thin wrapper around producemerge.WaitForTakeFilesReady
// retained for backward compatibility with internal callers and white-box tests.
func waitForTakeFilesReady(
	ctx context.Context,
	ffmpegExe string,
	videoPath string,
	audioPath string,
	timeout time.Duration,
	interval time.Duration,
) error {
	return producemerge.WaitForTakeFilesReady(ctx, ffmpegExe, videoPath, audioPath, timeout, interval)
}

func expectedTakePaths(state *produceSessionRuntime, plan ProduceTakePlan) (string, string) {
	demoPath := strings.TrimSpace(plan.DemoPath)
	demoSubDir := strings.TrimSpace(state.demoSubDirs[demoPath])
	if demoSubDir == "" {
		demoSubDir = plugingen.SanitizeDemoSubDirName(demoPath)
	}
	takeName := strings.TrimSpace(plan.TakeName)
	if takeName == "" && plan.TakeIndex > 0 {
		takeName = fmt.Sprintf("take%04d", plan.TakeIndex-1)
	}
	if takeName == "" {
		takeName = "take"
	}
	videoPath := filepath.Join(state.batchDir, demoSubDir, takeName+".mp4")
	audioPath := filepath.Join(state.batchDir, demoSubDir, takeName, "audio.wav")
	return videoPath, audioPath
}

// nextMergedVideoPath is a thin wrapper around producemerge.NextMergedVideoPath
// retained for backward compatibility with internal callers and white-box tests.
func nextMergedVideoPath(dir string, now time.Time) (string, error) {
	return producemerge.NextMergedVideoPath(dir, now)
}

// mergeTakeVideoAudio is a thin wrapper around producemerge.MergeTakeVideoAudio
// retained for backward compatibility with internal callers and white-box tests.
func mergeTakeVideoAudio(ffmpegExe string, videoPath string, audioPath string) (string, error) {
	return producemerge.MergeTakeVideoAudio(ffmpegExe, videoPath, audioPath)
}
