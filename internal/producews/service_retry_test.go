package producews

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// withListenFn swaps the package-level listenFn for the duration of a test.
// It restores the original on cleanup.
func withListenFn(t *testing.T, fn func(network, address string) (net.Listener, error)) {
	t.Helper()
	orig := listenFn
	listenFn = fn
	t.Cleanup(func() { listenFn = orig })
}

// pickFreeAddr returns a TCP address known to be free at call time. The
// listener is closed before returning, so the address may race with other
// processes — only use when the test will immediately bind to the address.
func pickFreeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick free addr: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

// TestService_RetriesListenUntilSuccess — AC1:
// First Listen fails (port "occupied"), supervisor retries and succeeds on
// attempt 2 once the occupancy clears.
func TestService_RetriesListenUntilSuccess(t *testing.T) {
	var calls atomic.Int32
	releaseAt := time.Now().Add(800 * time.Millisecond)

	withListenFn(t, func(network, address string) (net.Listener, error) {
		calls.Add(1)
		if time.Now().Before(releaseAt) {
			return nil, errors.New("simulated bind failure")
		}
		return net.Listen(network, address)
	})

	svc := New("127.0.0.1:0", nil)
	err := svc.Start()
	if err == nil {
		t.Fatalf("Start() should fail on first attempt")
	}
	defer svc.Stop()

	// Wait up to ~3s for the supervisor's second attempt to succeed.
	deadline := time.Now().Add(3 * time.Second)
	var addr string
	for time.Now().Before(deadline) {
		addr = svc.Address()
		if addr != "" && svc.GetWSState().LastError == "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if addr == "" {
		t.Fatalf("supervisor never established a listener; calls=%d state=%+v", calls.Load(), svc.GetWSState())
	}
	if svc.GetWSState().LastError != "" {
		t.Fatalf("LastError should be cleared after success, got %q", svc.GetWSState().LastError)
	}

	// Connect a fake plugin client to confirm the server is actually serving.
	conn := mustConnectGameClient(t, addr)
	defer conn.Close()
	waitFor(t, 2*time.Second, func() bool {
		return svc.GetWSState().Connected
	})
}

// TestService_RetryListenExhausts — AC2:
// Port is permanently occupied. After 5 attempts, supervisor writes the
// exhausted message and stops retrying.
func TestService_RetryListenExhausts(t *testing.T) {
	// Speed up: replace the backoff sequence with much shorter durations so
	// the test runs in <1s. Restore on cleanup.
	origSeq := backoffSequence
	backoffSequence = []time.Duration{
		10 * time.Millisecond,
		10 * time.Millisecond,
		10 * time.Millisecond,
		10 * time.Millisecond,
		10 * time.Millisecond,
	}
	t.Cleanup(func() { backoffSequence = origSeq })

	var calls atomic.Int32
	withListenFn(t, func(network, address string) (net.Listener, error) {
		calls.Add(1)
		return nil, errors.New("simulated permanent bind failure")
	})

	svc := New("127.0.0.1:0", nil)
	if err := svc.Start(); err == nil {
		t.Fatalf("Start() should fail when port is permanently occupied")
	}
	defer svc.Stop()

	// Wait for the supervisor to exhaust retries.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(svc.GetWSState().LastError, "重试 5 次失败") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	state := svc.GetWSState()
	if !strings.Contains(state.LastError, "重试 5 次失败") {
		t.Fatalf("LastError should contain exhaustion message, got %q (calls=%d)", state.LastError, calls.Load())
	}
	// Total listen calls should be 1 (Start) + maxRetryAttempts (5 supervisor) = 6.
	if got := calls.Load(); int(got) < maxRetryAttempts+1 {
		t.Fatalf("expected at least %d listen attempts, got %d", maxRetryAttempts+1, got)
	}

	// Confirm supervisor really stopped: wait a bit and ensure no further
	// listen calls are made.
	before := calls.Load()
	time.Sleep(150 * time.Millisecond)
	if after := calls.Load(); after != before {
		t.Fatalf("supervisor kept retrying after exhaustion: before=%d after=%d", before, after)
	}
}

// TestService_ServeExitTriggersRestart — AC3:
// Once Serve is running, forcibly closing the listener triggers a restart on
// the same port. A plugin client can re-dial successfully.
func TestService_ServeExitTriggersRestart(t *testing.T) {
	// Speed up backoff so the test doesn't stall on the 500ms post-Serve sleep.
	origSeq := backoffSequence
	backoffSequence = []time.Duration{
		20 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
	}
	t.Cleanup(func() { backoffSequence = origSeq })

	addr := pickFreeAddr(t)
	svc := New(addr, nil)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	// Verify initial Serve is up by dialing.
	firstConn := mustConnectGameClient(t, addr)
	waitFor(t, 2*time.Second, func() bool { return svc.GetWSState().Connected })
	_ = firstConn.Close()

	// Yank the listener out from under Serve. http.Server is now wedged on a
	// closed listener — Serve returns immediately.
	svc.mu.Lock()
	listener := svc.listener
	svc.mu.Unlock()
	if listener == nil {
		t.Fatalf("expected listener to be set")
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	// Wait for supervisor to re-listen and serve again. We detect success by
	// dialing and observing Connected become true.
	var reconnectErr error
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, _, err := websocket.DefaultDialer.Dial((&url.URL{
			Scheme:   "ws",
			Host:     addr,
			Path:     "/",
			RawQuery: "process=game",
		}).String(), nil)
		if err == nil {
			defer conn.Close()
			waitFor(t, 2*time.Second, func() bool { return svc.GetWSState().Connected })
			return
		}
		reconnectErr = err
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("supervisor did not restart Serve within 3s; last dial error: %v", reconnectErr)
}

// TestService_StopAbortsRetry — AC4:
// Supervisor is stuck in backoff after a failed Listen; Stop() must abort
// quickly without waiting for the backoff sleep to complete.
func TestService_StopAbortsRetry(t *testing.T) {
	// Use a long backoff to ensure the supervisor is genuinely sleeping when
	// Stop() is called.
	origSeq := backoffSequence
	backoffSequence = []time.Duration{
		3 * time.Second,
		3 * time.Second,
		3 * time.Second,
		3 * time.Second,
		3 * time.Second,
	}
	t.Cleanup(func() { backoffSequence = origSeq })

	withListenFn(t, func(network, address string) (net.Listener, error) {
		return nil, errors.New("simulated bind failure")
	})

	svc := New("127.0.0.1:0", nil)
	if err := svc.Start(); err == nil {
		t.Fatalf("Start() should fail")
	}

	// Give the supervisor a moment to enter the backoff sleep after its first
	// retry attempt.
	time.Sleep(80 * time.Millisecond)

	start := time.Now()
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Fatalf("Stop took too long: %s (expected <200ms; supervisor blocked on backoff?)", elapsed)
	}
}

func TestService_DefaultUsesOSAssignedPort(t *testing.T) {
	occupied, occupiedErr := net.Listen("tcp", "127.0.0.1:4574")
	if occupiedErr == nil {
		defer occupied.Close()
	}

	svc := NewDefault(nil)
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	port, err := svc.Port()
	if err != nil {
		t.Fatalf("Port: %v", err)
	}
	if port <= 0 {
		t.Fatalf("Port = %d, want a positive OS-assigned port", port)
	}
	if occupiedErr == nil && port == 4574 {
		t.Fatalf("Port = %d, want a port different from occupied legacy port", port)
	}

	addr := svc.Address()
	host, addressPort, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	if host != "127.0.0.1" || addressPort != fmt.Sprintf("%d", port) {
		t.Fatalf("Address = %q, want 127.0.0.1:%d", addr, port)
	}
}

func TestService_PortBeforeStartReturnsError(t *testing.T) {
	svc := NewDefault(nil)
	if _, err := svc.Port(); err == nil {
		t.Fatal("Port should fail before Start")
	}
}
