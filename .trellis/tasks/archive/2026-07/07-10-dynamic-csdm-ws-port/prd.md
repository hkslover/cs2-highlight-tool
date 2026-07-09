# Sync producews dynamic websocket port with server plugin

## Goal

让当前工具与已更新的 server plugin 对齐：`producews` 默认绑定
`127.0.0.1:0`，由操作系统分配并由服务持续持有真实端口；启动 HLAE
时将该端口作为 `CSDM_WS_PORT` 注入子进程环境，使 HLAE 启动的 CS2/server.dll
连接到同一个 WebSocket 服务，同时保留旧插件未设置环境变量时使用 4574 的兼容性。

## Requirements

* 将 `producews` 默认监听地址从 `127.0.0.1:4574` 改为 `127.0.0.1:0`。
* 监听成功后保持现有 listener 生命周期，不执行“探测后释放再监听”。
* 在 `producews.Service` 提供线程安全的端口读取方法；服务未启动、listener 不可用或地址不是 TCP 地址时返回描述性错误。
* 更新 `launchHLAEGame`，从当前 `producews` 读取真实端口，并将 `CSDM_WS_PORT=<port>` 传给 HLAE。
* 不修改 Wails 方法名、事件名、前端类型或自动生成文件。
* 更新/新增后端测试，覆盖动态端口分配、端口读取和 HLAE 环境变量传递。

## Acceptance Criteria

* [x] 默认启动的 `producews` 地址为回环地址上的动态端口，并且该端口可连接。
* [x] `Service.Port()` 在服务运行时返回 `listener.Addr()` 的 TCP 端口，在服务未启动时返回错误。
* [x] HLAE 命令接收到的环境包含当前 `producews` 的真实端口，而不是固定 4574。
* [x] 原有 WebSocket、队列和启动相关测试继续通过。
* [x] `go test ./...` 通过。

## Definition of Done

* 代码遵循 `internal` 锁和错误处理规范。
* 相关测试覆盖成功路径与服务未启动边界。
* 运行后端 Required Check，并检查工作区只包含本次任务的预期改动。

## Technical Approach

* `producews.NewDefault` 使用 `127.0.0.1:0`；`Start` 直接调用现有 `listenFn`，
  监听成功后由 `adoptListenerLocked` 保存 listener 和真实地址。
* 新增 `Service.Port()`，在锁内检查 `s.started`、`s.listener`，再读取
  `*net.TCPAddr.Port`。
* `launchHLAEGame` 在创建命令前读取端口，并设置 `cmd.Env`；环境变量写入使用
  单一键值，避免父环境中已有同名变量造成歧义。
* 动态监听后重试机制仍保留，主要用于 listener/Serve 异常；失败提示不再声称
  固定端口 4574。

## Decision (ADR-lite)

**Context**: server plugin 已支持通过 `CSDM_WS_PORT` 选择 WebSocket 端口，而工具
原先固定监听 4574，无法规避端口冲突。

**Decision**: 直接绑定 `127.0.0.1:0` 并持有 listener，读取操作系统分配的端口后
通过 HLAE 的继承环境传递给 CS2/server.dll。

**Consequences**: 消除固定端口冲突和“探测后释放”的竞态；保留自定义地址构造函数
能力供测试/内部调用。需要依赖 HLAE/customLoader 正常继承环境变量，真实 Windows
联调仍需用插件日志确认。

## Out of Scope

* 不修改 server plugin/DLL。
* 不改变旧插件的 4574 默认回退行为。
* 不新增端口配置 UI、配置文件字段或远程端口暴露。
* 不在本任务中做 Windows HLAE 实机联调。

## Technical Notes

* 主要文件：`internal/producews/service.go`、`internal/producews/*_test.go`、
  `internal/app/hlae_launch.go`、`internal/app/*_test.go`。
* 相关项目规范：`.trellis/spec/backend/error-handling.md`、
  `.trellis/spec/backend/quality-guidelines.md`、
  `.trellis/spec/backend/directory-structure.md`。
