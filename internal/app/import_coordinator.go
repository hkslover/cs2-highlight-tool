package app

import (
	"context"
	"fmt"
	"sync"
)

// platformImportPlatform identifies the platform client that owns a real demo
// download. It participates in the coordination key so a 5E match ID can never
// merge with a Wanmei ID that happens to have the same text.
type platformImportPlatform string

const (
	platformImportFiveE  platformImportPlatform = "5e"
	platformImportWanmei platformImportPlatform = "wanmei"
)

// platformImportKey is the normalized identity of one platform import. Callers
// pass canonicalized match IDs (fivee.ExtractMatchID /
// wanmei.ExtractNumericMatchID output), never a raw user URL, so equivalent
// inputs share one task while different matches keep running in parallel.
type platformImportKey struct {
	platform platformImportPlatform
	matchID  string
}

type platformImportResult struct {
	path string
	err  error
}

// platformImportTask is one in-flight real import. The owning caller writes
// result before closing done; every waiter reads it only after <-done, so the
// channel close publishes one frozen result to all of them.
type platformImportTask struct {
	done    chan struct{}
	result  platformImportResult
	waiters int
}

// platformImportCoordinator deduplicates concurrent imports of the same
// (platform, normalized match ID) inside one workspace. Same-key callers share
// a single download/extract/commit; different keys stay independent.
//
// The coordinator never holds its mutex across network or file work.
// Registering a task, publishing the frozen result and removing the in-flight
// entry are the only critical sections.
type platformImportCoordinator struct {
	mu    sync.Mutex
	tasks map[platformImportKey]*platformImportTask
}

func newPlatformImportCoordinator() *platformImportCoordinator {
	return &platformImportCoordinator{tasks: make(map[platformImportKey]*platformImportTask)}
}

// do runs fn for key and returns its result to every caller that arrives while
// the task is in flight. The first caller performs the real work under runCtx;
// later callers only wait. waitCtx bounds one waiter's own wait and never the
// shared task: canceling it removes that waiter alone.
//
// Errors are not cached. The in-flight entry is removed before waiters wake,
// so a caller that retries after a failure starts a new real import instead of
// observing the finished one.
//
// The second return value reports whether this caller performed the real
// import, which lets the caller run one-time post-processing exactly once.
func (c *platformImportCoordinator) do(
	runCtx context.Context,
	waitCtx context.Context,
	key platformImportKey,
	fn func(context.Context) (string, error),
) (path string, ranImport bool, err error) {
	if runCtx == nil {
		runCtx = context.Background()
	}
	if waitCtx == nil {
		waitCtx = context.Background()
	}
	if c == nil {
		path, err = fn(runCtx)
		return path, true, err
	}

	c.mu.Lock()
	if c.tasks == nil {
		c.tasks = make(map[platformImportKey]*platformImportTask)
	}
	if task, ok := c.tasks[key]; ok {
		task.waiters++
		c.mu.Unlock()

		path, err = task.wait(waitCtx)

		c.mu.Lock()
		if task.waiters > 0 {
			task.waiters--
		}
		c.mu.Unlock()
		return path, false, err
	}
	task := &platformImportTask{done: make(chan struct{})}
	c.tasks[key] = task
	c.mu.Unlock()

	// Publish the result and wake waiters even if fn panics: leaking an
	// in-flight entry would block every later caller for this key forever.
	func() {
		defer func() {
			recovered := recover()
			if recovered != nil {
				err = fmt.Errorf("平台导入任务异常终止: %v", recovered)
			}
			c.mu.Lock()
			task.result = platformImportResult{path: path, err: err}
			delete(c.tasks, key)
			c.mu.Unlock()
			close(task.done)
			if recovered != nil {
				panic(recovered)
			}
		}()
		path, err = fn(runCtx)
	}()

	return path, true, err
}

// waiterCount reports how many callers currently wait for the in-flight task
// of key. Proving that a second caller joined the shared task (instead of
// starting another download) is the main use; an owner-only import reports 0.
func (c *platformImportCoordinator) waiterCount(key platformImportKey) int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	task := c.tasks[key]
	if task == nil {
		return 0
	}
	return task.waiters
}

func (t *platformImportTask) wait(ctx context.Context) (string, error) {
	select {
	case <-t.done:
		return t.result.path, t.result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// importCoordinator returns the import coordinator owned by the installed
// workspace identity. Production construction installs it with the identity;
// manually constructed App fixtures that never installed one get a lazily
// created coordinator so focused tests exercise the same coordination.
func (a *App) importCoordinator() *platformImportCoordinator {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.platformImports == nil {
		a.platformImports = newPlatformImportCoordinator()
	}
	return a.platformImports
}
