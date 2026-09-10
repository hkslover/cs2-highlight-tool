package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testPlatformImportKey(matchID string) platformImportKey {
	return platformImportKey{platform: platformImportFiveE, matchID: matchID}
}

// waitForImportWaiters blocks until the in-flight task of key has at least
// want waiters. Callers use it only after the owning import is already inside
// the real task, so a later caller can only have joined the shared task; the
// deadline turns a broken coordinator into a test failure instead of a hang.
func waitForImportWaiters(t *testing.T, c *platformImportCoordinator, key platformImportKey, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if got := c.waiterCount(key); got >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d import waiter(s), got %d", want, c.waiterCount(key))
		}
		time.Sleep(time.Millisecond)
	}
}

func TestImportCoordinatorSharesSameKeyTask(t *testing.T) {
	c := newPlatformImportCoordinator()
	key := testPlatformImportKey("g161-20260427162329954189731")

	var runs int32
	entered := make(chan struct{})
	release := make(chan struct{})
	fn := func(context.Context) (string, error) {
		atomic.AddInt32(&runs, 1)
		close(entered)
		<-release
		return "shared.dem", nil
	}

	type outcome struct {
		path      string
		ranImport bool
		err       error
	}
	const callers = 4
	results := make(chan outcome, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			path, ranImport, err := c.do(context.Background(), context.Background(), key, fn)
			results <- outcome{path: path, ranImport: ranImport, err: err}
		}()
	}
	close(start)
	<-entered
	// Every other caller must join the in-flight task before the real import
	// can finish, so exactly one download is proven rather than assumed.
	waitForImportWaiters(t, c, key, callers-1)
	close(release)
	wg.Wait()
	close(results)

	if got := atomic.LoadInt32(&runs); got != 1 {
		t.Fatalf("real import runs = %d, want 1", got)
	}
	leaders := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("shared import returned error: %v", result.err)
		}
		if result.path != "shared.dem" {
			t.Fatalf("shared import path = %q, want %q", result.path, "shared.dem")
		}
		if result.ranImport {
			leaders++
		}
	}
	if leaders != 1 {
		t.Fatalf("callers that ran the real import = %d, want 1", leaders)
	}
	if got := c.waiterCount(key); got != 0 {
		t.Fatalf("waiters after completion = %d, want 0", got)
	}
}

func TestImportCoordinatorRunsIndependentKeysInParallel(t *testing.T) {
	cases := []struct {
		name string
		keys []platformImportKey
	}{
		{
			name: "different match IDs",
			keys: []platformImportKey{
				{platform: platformImportFiveE, matchID: "g161-20260427162329954189731"},
				{platform: platformImportFiveE, matchID: "g161-20260427162329954189732"},
			},
		},
		{
			name: "same ID on different platforms",
			keys: []platformImportKey{
				{platform: platformImportFiveE, matchID: "123"},
				{platform: platformImportWanmei, matchID: "123"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newPlatformImportCoordinator()
			entered := make(chan platformImportKey, len(tc.keys))
			release := make(chan struct{})

			type outcome struct {
				key  platformImportKey
				path string
				err  error
			}
			results := make(chan outcome, len(tc.keys))
			for _, key := range tc.keys {
				key := key
				fn := func(context.Context) (string, error) {
					entered <- key
					<-release
					return key.matchID + ".dem", nil
				}
				go func() {
					path, _, err := c.do(context.Background(), context.Background(), key, fn)
					results <- outcome{key: key, path: path, err: err}
				}()
			}

			// Both keys must reach the real task before either can finish.
			seen := make(map[platformImportKey]bool, len(tc.keys))
			for i := 0; i < len(tc.keys); i++ {
				select {
				case key := <-entered:
					seen[key] = true
				case <-time.After(2 * time.Second):
					t.Fatalf("independent keys did not run in parallel, saw %v", seen)
				}
			}
			close(release)

			for i := 0; i < len(tc.keys); i++ {
				select {
				case result := <-results:
					if result.err != nil {
						t.Fatalf("independent import %v failed: %v", result.key, result.err)
					}
					if result.path != result.key.matchID+".dem" {
						t.Fatalf("independent import %v path = %q", result.key, result.path)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("independent import did not return")
				}
			}
		})
	}
}

func TestImportCoordinatorWaiterCancelKeepsSharedTask(t *testing.T) {
	c := newPlatformImportCoordinator()
	key := testPlatformImportKey("g161-20260427162329954189731")

	var runs int32
	entered := make(chan struct{})
	release := make(chan struct{})
	fn := func(context.Context) (string, error) {
		atomic.AddInt32(&runs, 1)
		close(entered)
		<-release
		return "shared.dem", nil
	}

	type outcome struct {
		path string
		err  error
	}
	ownerResult := make(chan outcome, 1)
	go func() {
		path, _, err := c.do(context.Background(), context.Background(), key, fn)
		ownerResult <- outcome{path: path, err: err}
	}()
	<-entered

	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	waiterErr := make(chan error, 1)
	go func() {
		_, ranImport, err := c.do(context.Background(), waiterCtx, key, fn)
		if ranImport {
			waiterErr <- errors.New("cancelled waiter performed the real import")
			return
		}
		waiterErr <- err
	}()
	waitForImportWaiters(t, c, key, 1)
	cancelWaiter()
	select {
	case err := <-waiterErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled waiter error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled waiter did not return")
	}

	// The shared task is still running and a later waiter still receives its
	// result; canceling one waiter only removed that waiter.
	survivor := make(chan outcome, 1)
	go func() {
		path, _, err := c.do(context.Background(), context.Background(), key, fn)
		survivor <- outcome{path: path, err: err}
	}()
	waitForImportWaiters(t, c, key, 1)
	close(release)

	for name, ch := range map[string]chan outcome{"surviving waiter": survivor, "real import": ownerResult} {
		select {
		case result := <-ch:
			if result.err != nil || result.path != "shared.dem" {
				t.Fatalf("%s = (%q, %v), want (%q, nil)", name, result.path, result.err, "shared.dem")
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s did not receive the shared result", name)
		}
	}
	if got := atomic.LoadInt32(&runs); got != 1 {
		t.Fatalf("real import runs = %d, want 1", got)
	}
}

func TestImportCoordinatorSharesFrozenFailure(t *testing.T) {
	c := newPlatformImportCoordinator()
	key := testPlatformImportKey("g161-20260427162329954189731")
	boom := errors.New("download failed")

	entered := make(chan struct{})
	release := make(chan struct{})
	fn := func(context.Context) (string, error) {
		close(entered)
		<-release
		return "", boom
	}

	errs := make(chan error, 2)
	go func() {
		_, _, err := c.do(context.Background(), context.Background(), key, fn)
		errs <- err
	}()
	<-entered
	go func() {
		_, _, err := c.do(context.Background(), context.Background(), key, fn)
		errs <- err
	}()
	waitForImportWaiters(t, c, key, 1)
	close(release)

	for i := 0; i < 2; i++ {
		select {
		case err := <-errs:
			if !errors.Is(err, boom) {
				t.Fatalf("shared failure = %v, want %v", err, boom)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("caller did not receive the shared failure")
		}
	}
}

func TestImportCoordinatorDoesNotCacheFailure(t *testing.T) {
	c := newPlatformImportCoordinator()
	key := testPlatformImportKey("g161-20260427162329954189731")
	boom := errors.New("download failed")

	var runs int32
	fn := func(context.Context) (string, error) {
		if atomic.AddInt32(&runs, 1) == 1 {
			return "", boom
		}
		return "retry.dem", nil
	}

	path, ranImport, err := c.do(context.Background(), context.Background(), key, fn)
	if !errors.Is(err, boom) || path != "" || !ranImport {
		t.Fatalf("first call = (%q, %v, %v), want the real import failure", path, ranImport, err)
	}
	path, ranImport, err = c.do(context.Background(), context.Background(), key, fn)
	if err != nil || path != "retry.dem" || !ranImport {
		t.Fatalf("retry call = (%q, %v, %v), want a new real import", path, ranImport, err)
	}
	if got := atomic.LoadInt32(&runs); got != 2 {
		t.Fatalf("real import runs = %d, want 2", got)
	}
}

func TestImportCoordinatorWakesWaitersWhenTaskPanics(t *testing.T) {
	c := newPlatformImportCoordinator()
	key := testPlatformImportKey("g161-20260427162329954189731")

	entered := make(chan struct{})
	release := make(chan struct{})
	fn := func(context.Context) (string, error) {
		close(entered)
		<-release
		panic("import exploded")
	}

	ownerPanicked := make(chan any, 1)
	go func() {
		defer func() { ownerPanicked <- recover() }()
		_, _, _ = c.do(context.Background(), context.Background(), key, fn)
	}()
	<-entered
	waiterErr := make(chan error, 1)
	go func() {
		_, _, err := c.do(context.Background(), context.Background(), key, fn)
		waiterErr <- err
	}()
	waitForImportWaiters(t, c, key, 1)
	close(release)

	if recovered := <-ownerPanicked; recovered == nil {
		t.Fatal("panic did not propagate to the owning caller")
	}
	select {
	case err := <-waiterErr:
		if err == nil || !strings.Contains(err.Error(), "异常终止") {
			t.Fatalf("waiter error = %v, want a reported task failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiter hung after the shared task panicked")
	}
}
