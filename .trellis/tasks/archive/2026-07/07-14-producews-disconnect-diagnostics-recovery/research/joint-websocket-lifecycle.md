# Joint WebSocket lifecycle research

## Repositories inspected

* Host app: `cs2-highlight-tool/internal/producews/service.go`, app startup/HLAE wiring,
  logging/export infrastructure, produce-page state and errors.
* Game plugin: `cs2-server-plugin/cs2-server-plugin/main.cpp` and bundled
  `deps/easywsclient/easywsclient.{hpp,cpp}`.

## Confirmed host behavior

* The host binds `127.0.0.1:0`, keeps the listener, and injects the resulting
  port as `CSDM_WS_PORT` when HLAE starts.
* `readLoop` currently discards the exact `ReadJSON` error. The active
  connection is then cleared and a running queue fails immediately with the
  generic text `game websocket disconnected`.
* `handleWebSocket` accepts both `process=game` and an empty `process`, and any
  accepted connection replaces the current game connection.
* A replaced connection is protected by `gameConnID`: its later read-loop exit
  cannot clear the newer connection or fail the queue.
* There is no ping/pong health policy and no reconnect grace. The queue's ack
  timer is independent of connection recovery.
* If `Serve` exits unexpectedly, the supervisor calls `Listen` again with the
  original `127.0.0.1:0`. That can allocate a different port even though the
  already-running plugin still has the old `CSDM_WS_PORT`. A successful rebind
  to a new port would therefore be unusable for that recording session.
* Incoming envelope/payload decode failures and unknown message names are
  silently ignored. Only the envelope-level `ReadJSON` failure closes the
  connection.
* The produce page already renders a warning while the queue is running and
  `wsState.connected` is false, so an internal reconnect grace does not require
  a new public queue-state enum.

## Confirmed plugin behavior

* The plugin has had an automatic reconnect loop since its initial standalone
  import. After a disconnect or failed connect, it retries every two seconds.
* The bundled `easywsclient` recognizes WebSocket ping frames and automatically
  queues a matching pong. A host read deadline based on pong is compatible with
  the current plugin implementation, subject to an integration/smoke test.
* The plugin always connects using `?process=game`, so rejecting an empty
  `process` is compatible with the supported plugin.
* Plugin-to-host JSON is produced by `nlohmann::json::dump()`. Ordinary plugin
  code therefore does not naturally emit malformed JSON. A malformed/protocol
  frame remains possible if transport buffers are corrupted.
* `easywsclient` is not thread-safe. The global raw `ws` pointer and its
  internal transmit buffer are accessed from the WebSocket connection thread
  (`poll`, `dispatch`, delete) and the CS2 engine thread (`SendMsg`). Shutdown
  also calls `close` from another thread. This creates data-race/use-after-free
  and frame-corruption risk and is a stronger candidate than an ordinary JSON
  serialization bug.
* Messages emitted while `ws == NULL` are dropped. Messages already handed to
  `easywsclient::send` can also be lost when its connection-local transmit
  buffer is destroyed. A host-only reconnect grace can therefore hang later if
  `record_status` or `demo_done` was lost during the outage.
* The plugin writes `csdm.log` relative to the process working directory and
  deletes it at startup. The host cannot reliably locate this file for export.

## Corrections to the supplied analysis

1. Reconnect support is already present in the plugin. The missing part is the
   host-side grace/recovery state and preservation/replay of plugin events.
2. Pong compatibility can be established from the bundled source: ping is
   handled and pong is sent automatically. It is still necessary to verify the
   packaged Windows DLL, but a no-timeout observation phase is not required by
   the source implementation.
3. Splitting `ReadJSON` is useful for visibility and tolerance, but it cannot
   recover from a Gorilla WebSocket protocol/read error; after a read error the
   connection must be discarded. Only a successful `ReadMessage` followed by a
   JSON unmarshal error may be logged and skipped.
4. Updating `CLAUDE.md` is not the current contract. Root `AGENTS.md` is the
   repository's source of truth and must document new Wails methods/events;
   `.trellis/spec/` must be updated when logging or producews contracts change.

## Recommended convergence

Use two independently releasable implementation batches:

1. Observability and plugin transport safety: preserve exact close/read causes,
   add bounded rolling diagnostics and sanitized export, make the plugin
   connection thread the sole owner of `easywsclient`, and give the plugin a
   deterministic log path through an environment variable.
2. Health and recovery: pin the dynamically allocated port across supervisor
   restarts, add ping/pong deadlines, pause queue timers during a bounded grace,
   recover on the plugin's existing two-second reconnect loop, and replay only
   idempotent durable plugin events so a short outage cannot lose `demo_done`.

Do not ship host reconnect grace without the plugin send-ownership/replay work;
that combination can turn an immediate visible failure into a queue that waits
forever for an event that was dropped during disconnection.
