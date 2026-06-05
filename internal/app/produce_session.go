package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cs2-highlight-tool-v2/internal/demo"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	produceMergeWorkers       = 1
	produceFileReadyTimeout   = 30 * time.Second
	produceWorkerPollInterval = 300 * time.Millisecond
	produceFileStableInterval = 250 * time.Millisecond
	produceProcessCloseDelay  = 5 * time.Second
)

type produceSessionRuntime struct {
	cancel context.CancelFunc
	done   chan struct{}

	batchDir       string
	ffmpegExe      string
	demoSubDirs    map[string]string
	killsByDemo    map[string]map[string]demo.ClipKill
	plansByTake    map[string]ProduceTakePlan
	seenCompleted  map[string]struct{}
	endedQueue     []pendingCompletedTake
	taskCh         chan mergeTask
	pendingTaskCnt atomic.Int32
	workerWG       sync.WaitGroup

	fileReadyTimeout time.Duration
	pollInterval     time.Duration
	stableInterval   time.Duration

	cs2PID           int
	queueStopped     bool
	queueStopAt      time.Time
	closeAt          time.Time
	closeRequested   bool
	closeDone        bool
	compositionPhase bool

	keepIntermediateFiles bool
}

type produceSessionState struct {
	runtime         *produceSessionRuntime
	gameInfo        gameInfoSessionState
	pluginDLL       pluginDLLSessionState
	pov             povSessionState
	takeFiles       map[string]ProduceTakeFile
	takeFileOrder   []string
	historyItems    map[string]ProduceHistoryItem
	historyOrder    []string
	historyKeyIndex map[string]struct{}
}

func (a *App) startProduceSessionWorker(
	batchDir string,
	ffmpegExe string,
	demoSubDirs map[string]string,
	killSnapshotByDemo map[string]map[string]demo.ClipKill,
	results []GeneratePluginJSONBatchItemResult,
	cs2PID int,
	keepIntermediateFiles bool,
) {
	plans := collectTakePlans(results, true)
	plansByTake := make(map[string]ProduceTakePlan, len(plans))
	for _, plan := range plans {
		demoPath := strings.TrimSpace(plan.DemoPath)
		if demoPath == "" || plan.TakeIndex <= 0 {
			continue
		}
		plansByTake[takePlanKey(demoPath, plan.TakeIndex)] = plan
	}

	ctx, cancel := context.WithCancel(context.Background())
	normalizedSubDirs := make(map[string]string, len(demoSubDirs))
	for demoPath, subDir := range demoSubDirs {
		dp := strings.TrimSpace(demoPath)
		if dp == "" {
			continue
		}
		normalizedSubDirs[dp] = strings.TrimSpace(subDir)
	}
	normalizedKillSnapshots := normalizeProduceKillSnapshots(killSnapshotByDemo)
	next := &produceSessionRuntime{
		cancel:                cancel,
		done:                  make(chan struct{}),
		batchDir:              strings.TrimSpace(batchDir),
		ffmpegExe:             strings.TrimSpace(ffmpegExe),
		demoSubDirs:           normalizedSubDirs,
		killsByDemo:           normalizedKillSnapshots,
		plansByTake:           plansByTake,
		seenCompleted:         make(map[string]struct{}),
		taskCh:                make(chan mergeTask, 32),
		fileReadyTimeout:      produceFileReadyTimeout,
		pollInterval:          produceWorkerPollInterval,
		stableInterval:        produceFileStableInterval,
		cs2PID:                cs2PID,
		keepIntermediateFiles: keepIntermediateFiles,
	}

	for i := 0; i < produceMergeWorkers; i++ {
		next.workerWG.Add(1)
		go func() {
			defer next.workerWG.Done()
			a.mergeWorker(ctx, next)
		}()
	}

	var old *produceSessionRuntime
	a.produceStateMu.Lock()
	old = a.produceState.runtime
	a.produceState.runtime = next
	a.produceStateMu.Unlock()
	stopProduceRuntime(old)

	go a.runProduceSessionWorker(ctx, next)
}

func (a *App) stopProduceSessionWorker() {
	a.produceStateMu.Lock()
	old := a.produceState.runtime
	a.produceState.runtime = nil
	a.produceStateMu.Unlock()
	stopProduceRuntime(old)
}

func stopProduceRuntime(state *produceSessionRuntime) {
	if state == nil {
		return
	}
	state.cancel()
	select {
	case <-state.done:
	case <-time.After(3 * time.Second):
	}
}

func (a *App) runProduceSessionWorker(ctx context.Context, state *produceSessionRuntime) {
	defer close(state.done)
	defer close(state.taskCh)
	defer state.workerWG.Wait()
	defer func() {
		if err := a.forceRestoreProduceEnvironmentForProduce(); err != nil && a.ctx != nil {
			wailsruntime.LogError(a.ctx, fmt.Sprintf("restore produce environment failed: %v", err))
		}
	}()

	ticker := time.NewTicker(state.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.produceW == nil {
				continue
			}
			queue := a.produceW.GetQueueState()
			snapshot := a.produceW.GetTakeSnapshot()

			now := time.Now()
			a.enqueueCompletedTakes(state, snapshot)

			if state.compositionPhase {
				a.dispatchMergeTasks(state)
			}

			if !queue.Running && queue.Total > 0 {
				if !state.queueStopped {
					state.queueStopped = true
					state.queueStopAt = now
					state.closeAt = now.Add(produceProcessCloseDelay)
					state.compositionPhase = true
				}
			}

			if state.queueStopped && !state.closeRequested && !now.Before(state.closeAt) {
				a.requestCloseCS2Process(state)
			}

			if a.canStopProduceSession(state) {
				a.cleanupProduceTemporaryFiles(state)
				return
			}
		}
	}
}

func (a *App) isSessionWorkDrained(state *produceSessionRuntime) bool {
	if len(state.endedQueue) > 0 {
		return false
	}
	if state.pendingTaskCnt.Load() > 0 {
		return false
	}
	return true
}

func (a *App) canStopProduceSession(state *produceSessionRuntime) bool {
	if state == nil || !state.queueStopped || !state.closeDone {
		return false
	}
	return a.isSessionWorkDrained(state)
}
