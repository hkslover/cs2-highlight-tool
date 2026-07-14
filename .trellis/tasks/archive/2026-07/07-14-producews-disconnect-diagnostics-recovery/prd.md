# Stabilize produce WebSocket diagnostics and reconnect

## Goal

联合修复 `cs2-highlight-tool` 与 `cs2-server-plugin` 的 WebSocket 可观测性、
线程安全和瞬断恢复问题：让每一次断连都能被准确分类和导出，让插件端不再并发
访问非线程安全的 `easywsclient`，并让录制队列在短暂断连后可在同一动态端口上恢复，
而不是立即折叠成同一句 `game websocket disconnected`。

## What I already know

* 主程序已经安全地绑定 `127.0.0.1:0` 并通过 `CSDM_WS_PORT` 将端口传给 HLAE/CS2。
* 主程序丢弃 `ReadJSON` 的真实错误，任何当前连接的读循环退出都会立即失败运行中队列。
* 插件已经每 2 秒自动重连；无需新建第二套重连线程。
* 插件内置 `easywsclient` 会自动回复 ping，但该库本身不是线程安全的。
* 当前插件在 WebSocket 线程与 CS2 engine 线程之间共享和删除裸 `ws` 指针，并并发修改
  `easywsclient` 发送缓冲；这是明确的数据竞争和潜在帧损坏来源。
* 主程序 supervisor 在动态监听模式下可能在 `Serve` 异常后换到新随机端口，而已启动插件
  仍只知道旧端口；录制会话期间端口必须保持固定。
* 插件断线期间会直接丢弃出站事件；仅给主程序增加宽限期可能把“立即失败”变成“永远等不到
  `demo_done`”。
* produce 页已有“队列运行但插件未连接”的 warning 展示，可复用现有状态，不需要新增用户设置
  或新的公开状态枚举。
* 当前仓库以 `AGENTS.md` 和 `.trellis/spec/` 为稳定契约来源；不以 `CLAUDE.md` 维护新的公共方法。
* `frontend/wailsjs/**` 是生成文件，不能手改；当前工作树已有用户的
  `frontend/wailsjs/go/models.ts` 改动，实施时必须保留。

## Assumptions

* 重连宽限期使用内部常量，建议默认 15 秒；插件 2 秒重试周期可获得约 7 次机会，不新增设置 UI。
* 心跳建议 15 秒 ping、45 秒 pong/read deadline；该值只用于发现死连接，不把正常 demo 加载卡顿
  误判为应用消息超时。
* 主程序日志采用 2 MiB 单文件、3 个备份的大小轮转；内存事件环建议保留最近 500 条。
* 插件日志通过新增的可选 `CSDM_LOG_PATH` 定位到 `<dataDir>/logs/cs2-server-plugin.log`；
  未设置时继续回退相对路径 `csdm.log`，保持独立使用兼容性。
* 插件可靠发送的 MVP 使用“单线程发送队列 + 有界近期事件重放”，只重放幂等事件
  `record_status`、`demo_started`、`demo_done`；不重放无 demo 身份的通用 `status` ack。

## Scope and delivery boundaries

本计划包含两个仓库，但必须分别提交、测试和发布：

1. `cs2-server-plugin`：先完成 WebSocket 单线程所有权、事件队列/重放和确定性日志路径，生成新的
   Windows DLL/release 资产。
2. `cs2-highlight-tool`：完成诊断、端口固定、心跳、宽限恢复、导出 UI 和契约更新。
3. 发布时先发布插件 tag/资产并确认统一 Release 源可解析新版本，再发布主程序；不能只发布主程序行为
   修复却继续向用户分发旧 DLL。

## Requirements

### R1. Preserve and classify host read/close causes

* 将 `ReadJSON` 拆为 `ReadMessage` + `json.Unmarshal`：
  * `ReadMessage` 成功但 envelope JSON 无效：记录 message type、长度、经转义/截断/脱敏的最多
    512 字节预览，然后继续读下一帧；
  * envelope 有效但业务 payload 无效：按消息名记录 warning 并忽略该消息，不关闭连接；
  * 未知 `name`：记录 warning，不改变连接或队列；
  * `ReadMessage` 返回错误：连接已不可继续使用，分类后退出并把错误传给关闭处理。
* 分类至少包含：正常 close、异常 close（close code/text）、EOF/transport、协议错误、服务主动停止、
  新连接顶替旧连接。
* `WSState.LastError` 和最终 queue error 使用简短、脱敏、可行动的中文文案，并区分：
  * 录制中连接断开；
  * demo 切换/派发时连接不可用；
  * 重连宽限期耗尽；
  * `playdemo` 写失败或 ack 超时。
* 被新连接顶替的旧 `connID` 退出只能记为 superseded，不得清空新连接或失败队列。

### R2. Add bounded, always-on host diagnostics

* 为 `producews.Service` 注入独立诊断/Logger 依赖，由 `internal/app.NewApp` 使用 `dataDir` 接线，
  component 固定为 `producews`。
* 日志写到 `<dataDir>/logs/producews.log`，按 2 MiB × 3 backups 轮转；写盘失败不得阻塞或
  终止录制，只能降级到内存环并记录 drop/write failure 计数。
* 记录：服务 listen/serve/supervisor、端口、连接建立/顶替/关闭、远端地址、连接持续时间、ping/pong、
  收发消息名/大小/安全摘要、JSON/payload 错误、未知消息、队列状态迁移、ack/reconnect timer、写失败、
  fail/finish 原因。
* 不在 `Service.mu` 内做文件 I/O、Wails event emit 或可能阻塞的日志调用；先复制状态/日志数据，解锁后
  再写入或交给诊断 worker。
* 对 read error、queue fail、Serve 意外退出生成 incident report：包含原因、连接元数据、脱敏后的
  `WSState` / `QueueState` / `TakeStatusSnapshot`、最近约 500 条事件和插件日志尾部（若存在）。
* 同一 incident 只生成一份报告；文件名包含毫秒或序号，避免同秒覆盖；报告使用临时文件 + rename。

### R3. Share export redaction instead of duplicating it

* 将 home 路径、URL、credential/token、任意文本中路径的导出脱敏能力从
  `envsetup/service_logs_export.go` 抽到 `internal/logging` 的公共 helper。
* `ExportStartupLogs` 迁移为调用公共 helper，输出行为保持不变并保留现有回归测试。
* producews 文件日志、incident report 和手动导出均使用同一脱敏规则；原始 demo 路径不得出现在导出文件。
* 结构化日志动态值放入 fields/meta，不把 payload/path 拼进固定 message；只有受限 raw preview 例外，
  且必须先转义、截断、脱敏。

### R4. Make plugin WebSocket ownership thread-safe

* `easywsclient::WebSocket` 的创建、`poll`、`dispatch`、`send`、`close` 和 delete 全部只允许在
  `wsConnectionThread` 执行。
* 移除跨线程共享裸 `ws` 指针作为连接真值；用原子 connected 状态供状态输出，Shutdown 只设置
  `isQuitting` 并等待连接线程自清理。
* `SendMsg` 只负责序列化并写入互斥保护的有界出站队列，不直接调用 `ws->send`。
* 队列必须有明确上限和溢出策略：保留 `demo_done`/录制边界事件优先于低价值事件，并将 drop 计数写日志；
  禁止无界增长。
* WebSocket 线程连接成功后按顺序 drain；断连后保留/重放近期幂等 durable 事件
  `record_status`、`demo_started`、`demo_done`，但不重放通用 `status` ack，避免给下一条 demo 错误确认。
* `HandleWebSocketMessage` 对 host 发来的无效 JSON/字段类型使用异常保护并记录，不能让异常逃出线程入口。
* 插件日志写入增加互斥；读取可选 `CSDM_LOG_PATH`，失败时回退 `csdm.log` 并保留现有控制台日志。

### R5. Keep the session port stable

* 首次成功绑定 `127.0.0.1:0` 后，将实际 `listener.Addr()` 固定为该 Service 生命周期的重监听地址。
* `Serve` 意外退出后的 supervisor 必须尝试重绑同一实际端口，不得再次用 `:0` 静默换端口。
* 同一端口重绑失败时按现有 backoff/重试预算处理；耗尽后明确报告“插件仍指向旧端口，需要重启制作
  会话”，不能宣称在新端口恢复成功。
* `Port()` 在一次 Start/Stop 生命周期内保持稳定；Stop 后再次 Start 才允许获取新动态端口。

### R6. Tighten connection admission and add heartbeat

* 只有 `process=game` 才允许升级并接管 game connection；空参数和其他值在 upgrade 前返回 400。
* 监听仍保持 loopback-only，`CheckOrigin` 的放宽不得扩大为非 loopback 监听。
* 建立连接后设置 read limit；服务端每 15 秒通过当前连接发送 ping，并记录 pong/RTT/最近 pong 时间。
* pong handler 延长 45 秒 read deadline。写 ping 失败或 deadline 到期走统一关闭分类，不另造第二条
  queue fail 路径。
* heartbeat goroutine/timer 必须绑定 connID 和 Service Stop 生命周期；顶替连接、关闭、Stop 后无泄漏。

### R7. Add coordinated reconnect grace

* queue Running 时当前连接断开：停止/暂停 ack timer，保留 queue/take 状态并启动一次 15 秒总体宽限计时；
  不立即调用 `failQueueLocked`。
* 宽限期内新 `process=game` 连接到达：取消 reconnect timer。
  * 若 `PendingAck=false`，保持当前 demo 继续运行，依赖插件重放 durable 事件；
  * 若 `PendingAck=true`，先给插件短暂重放窗口；若仍 pending，再向新 conn 重发同一路径的
    `playdemo` 并重启 ack timer；不得推进 `Completed` 或切到下一 demo。
* `dispatchNextDemo` 发现 `gameConn == nil` 时复用同一个 reconnect grace，不直接失败。
* 多次短断连共享同一总体 deadline或明确重置策略；推荐不无限续期，单次 incident 最长 15 秒。
* deadline 到期后只失败一次，错误包含等待秒数和 incident report 文件名/标识。
* Stop、queue finish/fail、新 queue start 必须取消遗留 ack/reconnect/heartbeat timers。
* 不新增用户配置字段或设置页选项；前端沿用 `running && !connected` 的现有 warning 状态。

### R8. Export and frontend behavior

* 新增 Wails 方法 `ExportProduceWSLogs() (string, error)`，使用 SaveFileDialog；无 Wails context 的测试路径
  默认写入 `<dataDir>/logs/producews-export-<timestamp>.txt`。
* 导出文件合并：当前状态快照、内存事件环、轮转 host 日志、incident report 列表/内容、确定性插件日志尾部；
  所有内容在最终写出前再次脱敏。
* produce 页在任一 WS/queue/launch error 出现时展示“导出制作日志”按钮；成功/取消/失败反馈遵循现有
  message 与 i18n 模式。
* i18n 只改 `frontend/src/shared/i18n/zh-CN.json`。
* 新 Wails 方法写入根 `AGENTS.md` 和 `.trellis/spec/backend/wails-bindings.md`；producews、logging、
  directory responsibility 的变化同步到对应 `.trellis/spec/backend/*`。不把 `CLAUDE.md` 当稳定契约更新。
* 通过 Wails 生成命令刷新 bindings，禁止手改 `frontend/wailsjs/**`，并保留当前用户已有生成文件改动。

## Implementation sequence

### Batch A — Instrumentation foundation (`cs2-highlight-tool`)

1. 抽取共享 export redaction 并先用现有 envsetup 测试锁定无行为变化。
2. 新建 producews diagnostics/rotation/ring/report 组件和单元测试。
3. 接入连接、消息、队列和 supervisor 日志；完成 `ReadMessage` 分类，但暂不改变断连即失败策略。
4. 接入 `ExportProduceWSLogs` 与前端按钮。

### Batch B — Transport ownership (`cs2-server-plugin`)

1. WebSocket 单线程所有权 + 有界出站队列。
2. durable 事件近期重放、JSON exception guard、连接/重试/drop 日志。
3. `CSDM_LOG_PATH` 与日志互斥。
4. Windows Release x64 构建并做断开/重连 smoke test；发布新的 plugin tag/资产。

### Batch C — Host health and recovery (`cs2-highlight-tool`)

1. 固定 supervisor 重绑端口。
2. 严格 `process=game`、read limit、ping/pong deadline。
3. ack timer 与 reconnect grace 协调、pending ack 重发、超时单次失败。
4. Go integration/race tests 和新版 DLL 联调。

### Batch D — Release gate

1. 确认主程序统一 Release 快照能解析并安装 Batch B 的插件版本/changelog。
2. 在 Windows 上完成至少一次两 demo 队列联调：录制中强制断 socket，插件 2 秒重连后队列继续并最终完成。
3. 验证导出文件同时包含 host/plugin 时间线且不含真实用户 home/demo 路径。
4. 再发布主程序；保留回退到上一版 host 和上一版 plugin 资产的路径。

## Acceptance Criteria

### Host automated checks

* [ ] 无效 envelope JSON 后再发送有效消息，连接仍存活且有效消息被处理。
* [ ] payload JSON 无效或未知 `name` 只记录 warning，不断连、不推进队列。
* [ ] normal close、1006/unexpected EOF、协议错误、superseded 分别出现在结构化日志/报告中。
* [ ] 旧 conn 的 read-loop 退出不能清除新 conn 或失败当前队列。
* [ ] 首次动态端口在 supervisor 重监听后保持不变；同端口重绑耗尽不会切换到另一个随机端口。
* [ ] ping 收到 pong 会延长 deadline；无 pong 会在约 45 秒进入统一断连恢复路径。
* [ ] ack 前断连并在 15 秒内重连：同一 demo 至多按设计重发一次，ack 后继续。
* [ ] ack 后录制中断连并在 15 秒内重连：不推进 demo index，重放的 `record_status`/`demo_done` 可完成队列。
* [ ] 15 秒内未重连：队列只 fail 一次，所有 timer/goroutine 被清理并生成一份 report。
* [ ] 文件轮转、环形上限、report 同秒命名、日志目录不可写降级均有测试。
* [ ] startup export 迁移后现有脱敏输出不回归；produce export 不包含测试 home、token、demo 原始路径。
* [ ] `go test -count=1 ./...`、`go vet ./...` 通过。
* [ ] `go test -race ./internal/producews ./internal/app ./internal/logging ./internal/envsetup` 通过。
* [ ] `cd frontend && npm run build` 通过。

### Plugin automated/manual checks

* [ ] 静态结构/测试证明只有连接线程调用 `easywsclient` 实例方法和 delete。
* [ ] engine 线程并发产生 record events 时，队列无数据竞争、无 use-after-free、顺序保持。
* [ ] host 暂停 2–10 秒后恢复同一端口，插件自动重连并重放 durable events；generic `status` 不跨连接误重放。
* [ ] 队列达到上限时按优先级丢弃并记录 drop count，不发生无界内存增长。
* [ ] host 发来 malformed/unknown command 不会终止 WebSocket 线程或 CS2 进程。
* [ ] 设置 `CSDM_LOG_PATH` 后日志写入指定文件；未设置时仍写 `csdm.log`。
* [ ] Windows `Release|x64` 构建成功并上传 `server.dll` artifact。

### End-to-end checks

* [ ] 新版 host + 新版 plugin 完成正常单 demo、两 demo 队列，无重连时行为与现状一致。
* [ ] demo 切换的 800 ms 窗口强制断连，插件在宽限期内重连，队列不失败且不会跳 demo/重复计数。
* [ ] 录制中强制断连后恢复，最终 `demo_done` 到达并完成队列。
* [ ] 真正关闭 CS2/插件后 15 秒，队列明确失败而不是永久等待。
* [ ] 导出的单个文本报告足以区分：非法 JSON、WebSocket close code、心跳超时、端口重绑失败、
  插件重连、插件事件队列 drop。

## Definition of Done

* 两个仓库分别保持小而可回滚的提交，不能把插件二进制发布与主程序代码混成一个不可拆提交。
* 所有新增 goroutine/timer/file handle 有 Stop/Shutdown 测试，状态锁内无阻塞 I/O。
* 公共 Wails/日志/producews 契约和 Trellis specs 已更新。
* 生成绑定由工具生成且用户原有 dirty changes 被保留。
* 插件 release 先可用，主程序 release 后启用完整恢复链路。
* 实施结束按 Trellis 流程执行 `trellis-check`、spec update、分别提交，再 finish/archive。

## Decision (ADR-lite)

**Context**：现有主程序把 JSON 解码失败、协议断开和真实进程退出压成同一错误；与此同时插件虽然会
重连，却用非线程安全方式跨线程操作 WebSocket 并在断线时丢事件。只改 host 日志不能消除明确的数据竞争，
只加 host 宽限期又可能掩盖丢事件并造成永久等待。

**Decision**：先建立两端可关联的诊断时间线并将插件 WebSocket 收敛为单线程所有权，再在保持动态端口
不变的前提下加入 heartbeat 与 15 秒 host grace。恢复协议保持向后兼容，不改变现有 `playdemo` string
payload；插件只重放可由 host 幂等处理的 durable 事件。

**Consequences**：改动跨两个仓库并要求插件先发布；实现量大于单纯记录 `ReadJSON` error，但能同时覆盖
最可信的数据竞争根因、端口恢复缺陷和瞬断恢复。若新版插件尚未分发，host 仍可依靠日志准确失败，但不得
宣称 mid-demo reliable recovery 已可用。

## Out of Scope

* 不新增持久化设置、重连秒数 UI 或远程监听地址。
* 不把 WebSocket 暴露到 loopback 之外，不新增认证协议。
* 不重写 `easywsclient` 或引入新的 WebSocket C++ 依赖；先通过单线程 owner 隔离其线程安全限制。
* 不改变 demo/take JSON 业务格式、录制命令语义或 FFmpeg 合并流程。
* 不在本任务清理无关的 `frontend/wailsjs/go/models.ts` 用户改动。
* 不保证旧版 host + 新版 plugin 具备完整重连恢复；兼容目标是正常连接/录制不回归。

## Research References

* [`research/joint-websocket-lifecycle.md`](research/joint-websocket-lifecycle.md) — 两端生命周期、ping/pong、
  插件线程竞争、动态端口 supervisor 和对原分析的校正。

## Technical Notes

* Host core: `internal/producews/service.go`, `internal/producews/*_test.go`.
* Host wiring/export: `internal/app/app.go`, `internal/app/hlae_launch.go`, new produce-log binding,
  `internal/logging`, `internal/envsetup/service_logs_export.go`.
* Frontend: `frontend/src/features/produce/pages/ProducePage.vue`, its composable, and only `zh-CN.json`.
* Plugin core: `cs2-server-plugin/main.cpp`, bundled `easywsclient`, Windows solution/workflow and changelog asset.
* Release dependency: host component key `cs2-server-plugin` resolves repository `hkslover/cs2-server-plugin`;
  the plugin package must contain both `server.dll` and matching `changelog.xml`.
