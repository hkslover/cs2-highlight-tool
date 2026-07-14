## Bug Analysis: Produce WebSocket disconnects lost queue progress

### 1. Root Cause Category

- **Category**: B - Cross-Layer Contract, D - Test Coverage Gap, E - Implicit Assumption.
- **Specific Cause**: The host assumed a `ReadMessage` failure must immediately
  fail the queue, while the plugin already retried every two seconds but used a
  non-thread-safe shared `easywsclient` instance and dropped events while
  disconnected. A host supervisor also rebound `127.0.0.1:0`, which could
  silently change the port after the plugin inherited its launch environment.
  During the recovery change, strict `process=game` admission exposed a second
  transport-contract defect: the plugin URL omitted `/` before `?process=game`.
  `easywsclient` then sent `GET /`, lost the query, and received HTTP 400.

### 2. Why Fixes Failed (if applicable)

1. **Read-error-only visibility**: Splitting `ReadJSON` into envelope and
   payload diagnostics makes malformed messages observable, but cannot recover
   from an EOF/protocol failure and does not preserve dropped plugin events.
2. **Host-only reconnect grace**: A grace timer without plugin transport
   ownership/replay can wait for `demo_done` that was lost during the outage.
3. **Restarting with `:0`**: A listener restart can look successful locally
   while the launched plugin still connects to the old port.

### 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Only the plugin connection thread owns `easywsclient`; game threads enqueue bounded messages | DONE |
| P0 | Contract | Persist first concrete loopback address for the full produce session and pass it as `CSDM_WS_PORT` | DONE |
| P0 | Runtime | Ping/pong deadline plus a single bounded reconnect grace routes all close/write failures through one recovery path | DONE |
| P1 | Privacy | Shared export redaction hides arbitrary demo paths, credentials, and home prefixes in every support artifact | DONE |
| P1 | Tests | Cover strict admission, stable rebind, heartbeat, pending-ack replay, post-ack durable completion, and grace expiry | DONE |
| P1 | Compatibility | Use `/?process=game` in the plugin and accept/query-log empty process only for legacy loopback DLLs | DONE |
| P1 | Shutdown protocol | Use one-shot `end_produce_session` / `session_exit_ack`, engine-thread `quit`, and conn-scoped expected-close fallback | DONE |
| P2 | Release validation | Build/sign Windows `Release|x64` DLL and run real HLAE/CS2 reconnect smoke tests | TODO |

### 4. Systematic Expansion

- **Similar Issues**: Any future plugin command channel must avoid sharing a
  transport object between engine and network threads; any child process that
  receives an ephemeral endpoint must keep that endpoint stable for its
  lifetime.
- **Design Improvement**: Treat reconnect as a cross-repository protocol:
  host idempotency, plugin durable-event selection, fixed endpoint, and timer
  ownership must be changed and released together.
- **Shutdown Improvement**: A successful recording session must not use an
  unclassified PID close as its primary exit path. The host sends a typed exit
  request; the plugin confirms it once, queues `quit` on the game thread, and
  the host treats only that request's bounded close window as normal. Exit
  acknowledgements are non-durable so reconnect cannot confirm a later run.
- **Process Improvement**: Require a static transport-ownership scan and a
  Windows DLL smoke test whenever `easywsclient` lifecycle code changes.

### 5. Knowledge Capture

- [x] Updated `backend/producews.md` with fixed-port, heartbeat, diagnostics,
  admission, and reconnect contract.
- [x] Updated `backend/logging-guidelines.md` with shared export redaction.
- [x] Updated `backend/wails-bindings.md`, `directory-structure.md`, and the
  backend index for the Wails export and managed logs ownership.
- [x] Added the cross-layer reconnect checklist.
- [ ] Build and smoke test the Windows plugin release before publishing it.
