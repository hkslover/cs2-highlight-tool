# Produce WebSocket Port Contract

## Scenario: Dynamic port propagation to the server plugin

### 1. Scope / Trigger

This contract applies whenever `internal/app` launches HLAE/CS2 and the CS2
server plugin connects back to `internal/producews`. The server plugin reads
`CSDM_WS_PORT`; the tool owns the listener and must pass the listener's actual
port to HLAE before HLAE starts CS2.

### 2. Signatures

```go
func NewDefault(emit EventEmitter) *Service
func (s *Service) Start() error
func (s *Service) Stop() error
func (s *Service) Port() (int, error)
func (a *App) launchHLAEGame() (int, error)
```

`NewDefault` uses `127.0.0.1:0`. `Service.Port` is a locked read of the
first successful concrete listener address; it is not a port probe that opens
and closes a second socket.

### 3. Contracts

- Listener address: `127.0.0.1:0`; the OS assigns the concrete TCP port.
- Listener lifetime: the listener returned by `net.Listen` remains owned by
  `Service` until `Stop` or a serve failure; the successful listener is never
  released merely to test availability.
- The first successful `127.0.0.1:0` address becomes `relistenAddr` for the
  whole Start/Stop lifecycle. A supervisor restart must bind that exact
  concrete address, never another `:0` port.
- `Service.Port()` returns the positive port from `relistenAddr`, including
  during a short Serve/rebind gap. It fails only before a successful Start or
  after Stop.
- HLAE command environment contains `CSDM_WS_PORT=<Service.Port()>` and
  `CSDM_LOG_PATH=<dataDir>/logs/cs2-server-plugin.log`.
- HLAE/customLoader is expected to inherit this environment when it starts
  CS2, allowing `server.dll` to connect to `ws://localhost:<port>/?process=game`.
  The slash before `?` is required by `easywsclient`; without it the client
  sends `GET /` and silently loses the process query.
- The updated plugin remains backward-compatible on its side: if the variable
  is absent, it may still fall back to 4574. The tool does not use 4574 as its
  default listener.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Service started with a TCP listener | `Port()` returns a positive port |
| Service not started | `Port()` returns `produce websocket server is not started` |
| Serve is restarting after initial success | `Port()` returns the original concrete port while supervisor rebinds it |
| Concrete port cannot be rebound after retry budget | `WSState.LastError` says the plugin still targets the old port and the user must restart the produce session |
| Listener address has no usable port | `Port()` returns `produce websocket port is unavailable` |
| HLAE launch has no `produceW` service | `launchHLAEGame` returns a descriptive error and does not start HLAE |
| HLAE launch cannot read an active port | `launchHLAEGame` returns a wrapped port error and does not start HLAE |

### 5. Good / Base / Bad Cases

- Good: `4574` is occupied, `producews` binds an OS-assigned loopback port,
  and HLAE receives that exact port in `CSDM_WS_PORT`.
- Base: `4574` is free; the tool still binds `127.0.0.1:0` and passes the
  assigned port rather than relying on the legacy default.
- Bad: call `net.Listen("tcp", "127.0.0.1:0")`, close it, then start a second
  listener and pass its old port; this creates a race and can send the plugin
  to a port another process acquired.

### 6. Tests Required

- `internal/producews`: default service binds a positive loopback port even
  while 4574 is occupied; `Address()` and `Port()` agree.
- `internal/producews`: `Port()` fails before `Start`.
- `internal/app`: fake HLAE command records `CSDM_WS_PORT`; assert it equals
  the running `producews.Service.Port()`.
- Run `go test ./...` and `go vet ./...`; use `go test -race` for the touched
  `producews` and `app` packages when changing listener synchronization.

### 7. Wrong vs Correct

#### Wrong

```go
probe, _ := net.Listen("tcp", "127.0.0.1:0")
port := probe.Addr().(*net.TCPAddr).Port
probe.Close()
// Another process can claim port before the real server starts.
```

#### Correct

```go
listener, err := net.Listen("tcp", "127.0.0.1:0")
if err != nil {
    return err
}
// Keep the concrete address in Service; Port() reads relistenAddr while holding mu.
cmd.Env = append(os.Environ(), fmt.Sprintf("CSDM_WS_PORT=%d", port))
```

## Scenario: Non-blocking serialized websocket writes

### 1. Scope / Trigger

This applies to every outbound game websocket command. A peer can stop
reading while the TCP send buffer is full; state locks must remain available
for UI polling and inbound message handling during that write.

### 2. Signatures

```go
func (s *Service) SendCommand(name string, payload any) error
func (s *Service) dispatchNextDemo()
func (s *Service) writeOutgoing(conn *websocket.Conn, message outgoingMessage, timeout time.Duration) error
```

### 3. Contracts

- `s.mu` is used only to validate state and snapshot the active connection,
  connection ID, and write timeout.
- `s.writeMu` serializes all gorilla/websocket writes; no two goroutines may
  call `WriteJSON` concurrently on the same connection.
- The actual write runs outside `s.mu` and sets a finite write deadline.
- `dispatchNextDemo` re-enters `s.mu` after the write and mutates queue state
  only if the same queue item and connection are still current.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Service not started/no connection | Return the existing descriptive error |
| Write blocks | Deadline bounds the write; state getters remain callable |
| Write fails for current queue item | Close that stale connection and use the single reconnect-grace path; do not create a second immediate queue failure |
| Queue/connection changed while write is in flight | Do not overwrite the newer queue state |

### 5. Good / Base / Bad Cases

- Good: a blocked command write does not block `GetWSState` or `GetQueueState`.
- Base: normal `playdemo` writes start the ack timer only after success.
- Bad: hold `s.mu` around `gameConn.WriteJSON`; one stalled peer freezes the
  entire production page.

### 6. Tests Required

- Inject a blocked writer and assert state getters return before releasing it.
- Exercise normal queue dispatch and existing websocket integration tests.
- Run `go test -race` for `internal/producews` and `internal/app`.

### 7. Wrong vs Correct

#### Wrong

```go
s.mu.Lock()
defer s.mu.Unlock()
return s.gameConn.WriteJSON(message)
```

#### Correct

```go
s.mu.Lock()
conn := s.gameConn
s.mu.Unlock()
return s.writeOutgoing(conn, message, timeout)
```

## Scenario: Produce WebSocket diagnostics, admission, and bounded recovery

### 1. Scope / Trigger

- Trigger: changing `internal/producews`, HLAE/plugin launch environment, or
  the produce-page recovery UX.
- Scope: loopback listener supervision, `process=game` connection ownership,
  heartbeat, queue ack/reconnect timers, `Diagnostics`, and the standalone
  `cs2-server-plugin` transport contract.

### 2. Signatures

```go
func (s *Service) SetDiagnostics(diagnostics *Diagnostics)
func (s *Service) ExportDiagnostics() string
func (s *Service) SetReconnectGrace(timeout time.Duration)
func (s *Service) SetReplayWindow(delay time.Duration)
func (s *Service) RequestGracefulExit() error
func (s *Service) ExpectGracefulExitFallback() bool
func (s *Service) GracefulExitStatus() GracefulExitStatus
func (a *App) ExportProduceWSLogs() (string, error)
```

Plugin environment keys: `CSDM_WS_PORT` and `CSDM_LOG_PATH`.

### 3. Contracts

- `Service.Start` rejects non-loopback listen addresses, so the WebSocket
  endpoint is reachable only from the local machine. Query `process=game` is
  the current plugin contract; non-empty values other than `game` return HTTP
  400 before upgrade. An empty value is temporarily accepted and recorded as
  `legacy_process_missing` because released `easywsclient` DLLs constructed
  `ws://localhost:<port>?process=game`, whose parser silently sends `GET /`.
  The corrected plugin URL is `ws://localhost:<port>/?process=game`.
- One current `connID` owns the game connection. An old read loop may only log
  `superseded`; it must never clear a newer connection or fail its queue.
- Every connection sets a 1 MiB read limit, server ping every 15 seconds, and
  pong/read deadline of 45 seconds. A ping write failure or expired deadline
  closes the connection and flows through the same classified close handler.
- A running queue disconnect stops its ack timer but preserves queue/take
  state for one 15-second total reconnect grace. A new connection cancels the
  grace timer. Pending ack waits a short durable-plugin replay window, then
  resends the same `playdemo` once and restarts the ack timer; it never advances
  `Completed` or changes demo index.
- A post-ack reconnect waits for plugin replay of durable `record_status`,
  `demo_started`, and `demo_done`; generic `status` acknowledgement is never
  replayed. Grace expiry fails the queue once with actionable Chinese text and
  the incident report filename.
- After a successful queue (`Completed == Total` and no queue error), the app
  waits its normal process-close delay, then sends
  `{"name":"end_produce_session","payload":{"request_id":"..."}}`.
  The plugin responds once with `session_exit_ack` using the same request ID,
  drains that response from its single WebSocket-owner thread, and only then
  queues engine command `quit`. The plugin must not call `ExitProcess` or
  accept arbitrary console commands from WebSocket.
- `session_exit_ack` is deliberately non-durable. Replaying an acknowledgement
  after reconnect could incorrectly confirm a later session. The host marks a
  disconnect as expected only after the matching acknowledgement, or directly
  before its PID fallback; the flag is tied to one `connID` and a finite
  deadline. Expected disconnect clears `WSState.LastError`, emits an info
  diagnostic, and does not write an incident. A disconnect before confirmation
  remains a transport/protocol failure.
- If the acknowledgement or game exit does not complete promptly, the app
  calls `ExpectGracefulExitFallback` and uses the existing WM_CLOSE/PID
  fallback. This retains compatibility with old DLLs that ignore the new
  command while keeping their known fallback disconnect non-fatal.
- Diagnostics have a 500-event in-memory ring and asynchronous 2 MiB × 3
  file rotation at `<dataDir>/logs/producews.log`. Filesystem failures increment
  counters but never block `Service.mu` or recording state.
- Reports and manual export use `internal/logging` redaction. Raw demo paths,
  home prefixes, credentials, URL secrets, and plugin-log paths must not appear.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Invalid JSON/payload or unknown message name | Record bounded warning with escaped summary, keep connection alive |
| Read error / close / EOF | Classify normal, abnormal, transport, protocol, superseded, or service stop; capture one incident |
| Queue disconnect within grace | Queue remains `Running`; `Connected=false`; no ack timeout fires |
| New conn while `PendingAck=true` | Cancel grace, wait replay window, resend same demo only if still pending |
| New conn while `PendingAck=false` | Do not resend active demo; accept durable completion or dispatch an already-completed next item |
| Grace expires | Fail once, reset recording takes to pending, include report basename |
| Log path cannot be written | Retain memory ring; increment `WriteFailures`; continue service |
| Matching `session_exit_ack`, then socket closes | Mark graceful exit complete; clear WS error; no incident |
| Socket closes before matching exit acknowledgement | Preserve normal classified disconnect error and incident behavior |
| Exit acknowledgement/game shutdown timeout | Mark the pending connection as expected, then use PID close fallback |

### 5. Good / Base / Bad Cases

- Good: listener restarts and rebinds its original dynamic port; the already
  launched plugin reconnects without a changed environment.
- Good: connection drops before ack, plugin reconnects, and one replayed
  `playdemo` for the identical path receives `status`.
- Base: normal connected two-demo queue behaves as before and sends one
  `playdemo` per demo.
- Good: after a successful queue, the plugin sends its one-shot exit
  acknowledgement, queues `quit` on the engine thread, and the host records a
  normal session completion rather than a transport error.
- Base: an older DLL ignores `end_produce_session`; after the bounded wait the
  app closes the known CS2 PID and the host classifies that close as expected.
- Bad: construct `ws://localhost:<port>?process=game` in the plugin. The
  `easywsclient` URL parser drops the query and the host can only identify it
  as a legacy connection.
- Bad: replay `session_exit_ack` after reconnect or treat every later socket
  close as expected. This can hide a genuine failed recording session.
- Bad: call Wails emit, disk I/O, or a network write while holding `Service.mu`.

### 6. Tests Required

- `internal/producews`: invalid envelope/payload remains connected; current
  `process=game`, legacy empty query, and non-game process admission; a
  superseded connection cannot clear current state.
- `internal/producews`: dynamic rebind holds one port, including rebind
  exhaustion; heartbeat pong keeps a peer alive and absent pong closes it.
- `internal/producews`: disconnect before ack resends the same path once;
  disconnect after ack accepts durable `demo_done`; grace expiry fails once.
- `internal/producews`: ring limit, rotation, report uniqueness, unwritable
  disk degradation, and demo/token/path redaction.
- `internal/producews`: matching exit acknowledgement followed by disconnect
  clears `LastError`; unacknowledged disconnect remains an error.
- `internal/app`: successful session sends `end_produce_session` before PID
  fallback; acknowledged close skips fallback, timeout invokes it once.
- `internal/app`: HLAE receives both environment keys and headless diagnostic
  export writes beneath `<dataDir>/logs`.
- Run `go test -race ./internal/producews ./internal/app ./internal/logging ./internal/envsetup`.

### 7. Wrong vs Correct

#### Wrong

```go
if s.queueState.Running {
    s.failQueueLocked("websocket disconnected")
}
```

This loses the plugin's reconnect/replay opportunity and makes a normal
transient plugin reconnect look like a completed recording failure.

#### Correct

```go
s.stopAckTimerLocked()
s.startReconnectTimerLocked() // one bounded, queue-scoped grace window
// Keep queue/take snapshots intact. On reconnect, only replay a still-pending
// playdemo after the durable-event window; otherwise consume replayed events.
```

#### Wrong

```cpp
// Runs from the WebSocket thread and bypasses CS2/plugin teardown.
TerminateProcess(GetCurrentProcess(), 0);
```

#### Correct

```cpp
SendMsg(BuildSessionExitAckMessage(requestID)); // one-shot, not replayed
exitAfterWebSocketAck = true;
// The WebSocket-owner loop drains the acknowledgement first, then:
QueueEngineCommand("quit"); // executed only by the game thread
```
