package app

import (
	"context"
	"errors"
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
	produceGracefulExitWait   = 3 * time.Second
)

// produceSessionStopWait bounds how long stopProduceRuntime waits for a
// cancelled session to fully exit (workers drained and environment restored)
// before giving up and reporting an error. Package-level so tests can shorten
// it.
var produceSessionStopWait = 10 * time.Second

type produceSessionRuntime struct {
	cancel        context.CancelFunc
	done          chan struct{}
	pendingLaunch *pendingHLAELaunch

	// envEpoch is the produce-environment generation this session was
	// prepared for (see App.beginProduceEnvironmentPrep). The session's
	// deferred environment restore is a no-op once a newer session has taken
	// over the environment, so a late-running old session can never restore
	// over (or delete the backups of) a newer session's gameinfo/plugin DLL.
	envEpoch uint64

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

	cs2PID              int
	queueStopped        bool
	queueSucceeded      bool
	queueStopAt         time.Time
	closeAt             time.Time
	closeRequested      bool
	closeDone           bool
	closeErr            error
	processExitVerified bool
	gracefulExit        bool
	gracefulExitAt      time.Time
	compositionPhase    bool

	keepIntermediateFiles bool
	teardownErr           error
}

type produceSessionState struct {
	runtime         *produceSessionRuntime
	envEpoch        uint64
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
	envEpoch uint64,
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
		envEpoch:              envEpoch,
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

	a.produceStateMu.Lock()
	a.produceState.runtime = next
	a.produceStateMu.Unlock()
	// GeneratePluginJSONBatchAndLaunchHLAE holds produceLaunchMu and fully
	// drains the previous runtime before this install, so no old teardown can
	// still be pending after the pointer changes.

	go a.runProduceSessionWorker(ctx, next)
}

// stopProduceSessionWorker stops the current runtime and waits for the
// session goroutine to fully exit (merge workers drained AND the environment
// restore completed). It returns an error when the session does not exit
// within produceSessionStopWait; callers that are about to prepare a new
// environment (e.g. the launch pipeline) must abort in that case, because
// proceeding would leave the old session's environment state and backups in
// the global produce state while a new session overwrites them.
//
// The runtime pointer is cleared ONLY after a successful stop. On timeout the
// pointer is kept, so an immediate retry waits on the same still-alive session
// again instead of seeing nil and preparing a new environment while the old
// session is still running (whose late restore would then be epoch-guarded to
// a no-op, leaving the old backups to be overwritten by the new prepare).
func (a *App) stopProduceSessionWorker() error {
	a.produceStateMu.Lock()
	old := a.produceState.runtime
	a.produceStateMu.Unlock()
	waitErr := stopProduceRuntime(old)
	if waitErr != nil && !produceRuntimeDone(old) {
		return waitErr
	}
	// state.done is closed only after the session goroutine attempted its
	// teardown. Re-run the safety-critical parts synchronously before releasing
	// ownership: successful operations are idempotent, while a transient PID or
	// file-lock failure gets one more chance and is propagated if it persists.
	if err := a.retryProduceRuntimeTeardown(old); err != nil {
		if waitErr != nil {
			return errors.Join(waitErr, err)
		}
		return err
	}
	a.produceStateMu.Lock()
	if a.produceState.runtime == old {
		a.produceState.runtime = nil
	}
	a.produceStateMu.Unlock()
	return nil
}

// stopProduceRuntime cancels the session and waits until it has fully exited
// (state.done closed, which happens after merge workers are drained and the
// deferred environment restore ran). A nil state is a no-op.
func stopProduceRuntime(state *produceSessionRuntime) error {
	if state == nil {
		return nil
	}
	state.cancel()
	select {
	case <-state.done:
		if state.teardownErr != nil {
			return fmt.Errorf("上一个制作会话收尾失败: %w", state.teardownErr)
		}
		return nil
	case <-time.After(produceSessionStopWait):
		return fmt.Errorf("上一个制作会话在 %s 内未完全退出，已取消本次启动，请稍后重试", produceSessionStopWait)
	}
}

func produceRuntimeDone(state *produceSessionRuntime) bool {
	if state == nil {
		return true
	}
	select {
	case <-state.done:
		return true
	default:
		return false
	}
}

func (a *App) retryProduceRuntimeTeardown(state *produceSessionRuntime) error {
	if state == nil {
		return nil
	}
	if err := a.forceCloseCS2ProcessForTeardown(state); err != nil {
		return fmt.Errorf("关闭旧 CS2 进程失败: %w", err)
	}
	if err := a.forceRestoreProduceEnvironmentForEpoch(state.envEpoch); err != nil {
		return fmt.Errorf("恢复旧制作环境失败: %w", err)
	}
	return nil
}

// rollbackProduceLaunchEnvironment tears down a launch that failed after the
// produce environment was prepared but before a normal session runtime was
// installed. If closing CS2 or restoring files fails, it retains a completed
// runtime record so the next launch/shutdown retries the same teardown instead
// of overwriting the still-active environment and its backups.
func (a *App) rollbackProduceLaunchEnvironment(cs2PID int, envEpoch uint64, pending ...*pendingHLAELaunch) error {
	done := make(chan struct{})
	close(done)
	state := &produceSessionRuntime{
		cancel:   func() {},
		done:     done,
		envEpoch: envEpoch,
		cs2PID:   cs2PID,
	}
	if len(pending) > 0 {
		state.pendingLaunch = pending[0]
	}

	var rollbackErr error
	if err := a.forceCloseCS2ProcessForTeardown(state); err != nil {
		rollbackErr = errors.Join(rollbackErr, fmt.Errorf("关闭 CS2 进程失败: %w", err))
	} else if err := a.forceRestoreProduceEnvironmentForEpoch(envEpoch); err != nil {
		rollbackErr = errors.Join(rollbackErr, fmt.Errorf("恢复制作环境失败: %w", err))
	}
	if rollbackErr == nil {
		return nil
	}

	state.teardownErr = rollbackErr
	a.produceStateMu.Lock()
	if a.produceState.runtime == nil {
		a.produceState.runtime = state
	} else {
		rollbackErr = errors.Join(rollbackErr, fmt.Errorf("无法保留失败的启动收尾状态：已有制作会话"))
	}
	a.produceStateMu.Unlock()
	return rollbackErr
}

func (a *App) runProduceSessionWorker(ctx context.Context, state *produceSessionRuntime) {
	defer close(state.done)
	// Sweep orphaned ".mux.tmp.mp4" files on every exit path (including
	// cancellation) after the workers have exited, so a killed ffmpeg's
	// partial output cannot linger. Registered right after close(state.done)
	// so it runs just before done closes: a waiter on done can then rely on
	// the sweep having finished.
	defer a.cleanupMuxTmpFilesForSession(state)
	defer func() {
		var teardownErr error
		// A graceful plugin acknowledgement means quit was queued, not that the
		// process has already exited. Verify the owned PID is gone before touching
		// gameinfo/plugin files; the PID closer returns immediately when it is.
		if err := a.forceCloseCS2ProcessForTeardown(state); err != nil {
			teardownErr = errors.Join(teardownErr, fmt.Errorf("关闭 CS2 进程失败: %w", err))
		}
		// Restore only the environment this session prepared. If a newer
		// session already took over the environment (e.g. this session was
		// replaced while its ffmpeg was still being torn down), this restore
		// becomes a no-op so it can never delete or overwrite the newer
		// session's gameinfo / plugin DLL / POV files.
		// Do not restore files while CS2 may still have them loaded. The retained
		// runtime lets the next stop/retry close the PID and retry restoration.
		if teardownErr == nil {
			if err := a.forceRestoreProduceEnvironmentForEpoch(state.envEpoch); err != nil {
				teardownErr = errors.Join(teardownErr, fmt.Errorf("恢复制作环境失败: %w", err))
			}
		}
		state.teardownErr = teardownErr
		if teardownErr != nil && a.ctx != nil {
			wailsruntime.LogError(a.ctx, fmt.Sprintf("produce session teardown failed: %v", teardownErr))
		}
	}()
	// Shutdown must happen in this order: cancel merge work, close the task
	// channel, wait for the worker to exit, then restore the game environment.
	// Defers run in LIFO order, so keep the final done notification first.
	defer state.workerWG.Wait()
	defer close(state.taskCh)
	defer state.cancel()

	ticker := time.NewTicker(state.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// A replacement/shutdown can cancel the runtime before the normal
			// delayed close path runs. Close the owned CS2 process synchronously so
			// environment restoration never races a still-running game process.
			_ = a.forceCloseCS2ProcessForTeardown(state)
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
					state.queueSucceeded = queue.Completed == queue.Total && strings.TrimSpace(queue.LastError) == ""
					state.queueStopAt = now
					state.closeAt = now.Add(produceProcessCloseDelay)
					state.compositionPhase = true
				}
			}

			if state.queueStopped && !state.closeRequested && !now.Before(state.closeAt) {
				a.requestCloseCS2Process(state)
			}

			if state.closeRequested && !state.closeDone {
				a.advanceCloseCS2Process(state, now)
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
