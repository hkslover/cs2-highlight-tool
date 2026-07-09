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
currently held listener and is not a port probe that opens and closes a second
socket.

### 3. Contracts

- Listener address: `127.0.0.1:0`; the OS assigns the concrete TCP port.
- Listener lifetime: the listener returned by `net.Listen` remains owned by
  `Service` until `Stop` or a serve failure; the successful listener is never
  released merely to test availability.
- `Service.Port()` returns the positive `*net.TCPAddr.Port` of the active
  listener.
- HLAE command environment contains `CSDM_WS_PORT=<Service.Port()>`.
- HLAE/customLoader is expected to inherit this environment when it starts
  CS2, allowing `server.dll` to connect to `ws://localhost:<port>?process=game`.
- The updated plugin remains backward-compatible on its side: if the variable
  is absent, it may still fall back to 4574. The tool does not use 4574 as its
  default listener.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Service started with a TCP listener | `Port()` returns a positive port |
| Service not started | `Port()` returns `produce websocket server is not started` |
| Listener is nil or no longer active during serve restart | `Port()` returns the same unavailable/not-started error |
| Listener address is not `*net.TCPAddr` or port is non-positive | `Port()` returns `produce websocket port is unavailable` |
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
// Keep listener in Service; Port() reads listener.Addr() while holding mu.
cmd.Env = append(os.Environ(), fmt.Sprintf("CSDM_WS_PORT=%d", port))
```
