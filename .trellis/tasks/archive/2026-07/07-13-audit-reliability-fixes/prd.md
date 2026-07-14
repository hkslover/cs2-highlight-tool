# 审查并修复高优先级并发与可靠性缺陷

## Goal

核验并修复制作会话、WebSocket 写入、配置持久化、组件下载和制作环境恢复路径中的高风险可靠性问题，避免 goroutine 泄漏、慢网误失败、配置丢失/损坏、UI 全局卡死、失败素材丢失和路径/环境状态错乱。

## Audit findings

本轮只读核验基于当前 checkout，目标包测试基线为：`go test ./internal/app ./internal/config ./internal/download ./internal/producews` 通过。

* **P1-1 confirmed**：`internal/app/produce_session.go:149-157` 的 defer 按 LIFO 实际执行为恢复环境 → `workerWG.Wait()` → `close(taskCh)` → `close(done)`。自然结束路径在 `canStopProduceSession` 返回后没有调用 `state.cancel()`，merge worker 继续等待 `ctx.Done()` 或 task channel，runner 会在 `Wait` 永久等待；显式停止路径才会通过 `stopProduceRuntime` 取消。
* **P1-2 confirmed**：`internal/download/file.go:34` 使用 `http.Client{Timeout: 3 * time.Minute}`，覆盖整个 body 读取；EOF 后未校验 `Content-Length`，截断响应可能被视为成功。当前没有连接/响应头/读取空闲停滞的分层超时。
* **P1-3 confirmed**：`config.LoadOrCreate` 每次成功读取后都 `Save`，`config.Save` 使用截断式 `os.WriteFile`。`internal/app` 多个 Wails 方法直接执行 load-modify-save，`App` 没有统一配置互斥；现有 `envsetup.Service.configMu` 只覆盖 envsetup 自身路径。
* **P1-4 confirmed**：`internal/producews/service.go:525-543` 和 `774-806` 在持有 `s.mu` 时执行 `gameConn.WriteJSON`，且没有写截止时间。阻塞写可以阻塞状态查询和消息处理；WebSocket 写也需要独立串行化。
* **P2-1 confirmed**：`internal/app/produce_cleanup.go:21-69` 按全部 `plansByTake` 无条件删除 raw video/audio，不检查 `produceState.takeFiles` 的失败状态；merge 失败后的原始素材会被删除。
* **P2-2 confirmed**：`prepareGameInfoForProduce` 在发现所有注入路径已存在时直接记录 `modified=false`。若崩溃残留 `.cs2ht_produce.bak`，下一次会命中该分支，恢复函数不会处理残留备份。
* **P2-3 confirmed**：批量入口在 `plugin_generate.go:150/197` 保留原始 `job.DemoPath`，而 `generatePluginJSONInternal` 生成的 take plan 使用 `filepath.Abs` 后的路径；后续 `demoSubDirByDemoPath`、kill snapshot 和 `StartQueue` 使用的 key 因此可能不一致。

## Requirements

* P1-1：让自然结束和取消结束都统一取消 session context；确保 task channel 在 worker 等待前关闭，`done` 最终可观察地关闭；添加无需调用 Stop 的自然结束回归测试。
* P1-4：锁内只读取连接/队列快照；使用独立写锁保证 gorilla/websocket 单写者；写操作设置有限的 write deadline；写失败时在锁外回写队列失败状态，不阻塞 `GetWSState`/`GetQueueState`。
* P1-3：`config.Save` 使用同目录临时文件写入并原子 rename；`LoadOrCreate` 仅在创建、默认值/迁移或规范化确实改变内容时回写；App 的所有配置 load-modify-save 通过统一 `configMu` 保护，读路径也使用统一 helper。
* P1-2：取消整个 HTTP client 的总时限，保留连接、TLS 握手和响应头超时；为 body 读取提供可测试的空闲停滞超时；EOF 后若 `Content-Length > 0` 且实际字节数不一致，返回明确的下载不完整错误并清理失败文件。
* P2-1：清理前读取 take file 状态，`failed` take 保留 raw video/audio；成功或无状态的 take 维持现有清理行为。
* P2-2：准备 gameinfo 时优先恢复同路径残留 `.cs2ht_produce.bak`，清理备份，再基于恢复后的内容执行本次注入；补充崩溃残留回归测试。
* P2-3：两个批量入口在生成 record subdir、内部生成、结果、kill snapshot、demo subdir map 和 `StartQueue` 前统一将 demo 路径规范化为绝对路径。

## Acceptance Criteria

* [ ] 每个确认的缺陷都有针对性回归测试或可验证的单元覆盖。
* [ ] 自然完成制作会话在不调用 Stop 的情况下关闭 `done`，worker 不残留。
* [ ] WebSocket 写阻塞/失败时状态查询不会被 `s.mu` 卡住，且并发写不会破坏连接。
* [ ] 配置并发更新不产生交错丢失写；写入中断不会留下半截正式 config 文件。
* [ ] 慢速但持续有数据的下载可完成，截断响应返回“下载不完整”类错误。
* [ ] 失败 take raw 素材保留；成功 take 仍清理；崩溃残留 gameinfo 备份会恢复。
* [ ] 批量相对路径与绝对路径产生一致的 take/session key。
* [ ] `go test ./...` 通过；本任务无前端源代码变更时不强制构建前端，但交付时说明结果。

## Definition of Done

* 逐批更新实时执行计划并记录测试结果。
* 运行后端 Required Checks：`go test ./...`；必要时运行 `go vet ./...`、相关 `-race` 测试。
* 使用 `trellis-check` 规范完成质量复核。
* 判断是否需要更新 `.trellis/spec/`；不改变稳定状态/事件/Wails 方法契约。
* 不触碰用户已有的 `frontend/wailsjs/go/models.ts` 改动，不手工编辑自动生成文件。

## Technical Approach

按风险与耦合度分批：

1. P1-1 + P1-4：生命周期收尾和 WebSocket 写路径，先加入回归测试。
2. P1-3：配置原子写、按需迁移和 App 配置 helper/互斥。
3. P1-2：分层 HTTP 超时、空闲读取控制与长度校验。
4. P2-1 + P2-2 + P2-3：局部清理、崩溃恢复和批量路径规范化。

## Decision (ADR-lite)

**Context**：问题集中在现有生命周期/持久化边界，改变公共 Wails 契约会扩大风险。

**Decision**：使用标准库 context、net/http、文件临时替换和现有状态结构做最小行为保持修复；不增加前端设置或新状态枚举。下载空闲阈值做成包内可测试变量，生产默认值足以覆盖普通慢网但能终止真正停滞连接。

**Consequences**：配置 helper 会增加 App 内部调用约束；WebSocket 写错误需要在锁外重新进入状态锁；失败 take 会占用更多磁盘，但保留了用户可恢复素材。

## Out of Scope

* 不修改前端 UI、Wails 自动生成绑定或稳定接口/事件/状态枚举。
* 不重构整个 producews 状态机，不改变队列协议和 ack 语义。
* 不实现断点续传、下载重试策略或新的下载源策略。
* 不自动清理用户手动创建的非本次会话素材。

## Technical Notes

* 主要文件：`internal/app/produce_session.go`、`produce_cleanup.go`、`produce_gameconfig.go`、`plugin_generate.go`、`app.go` 及配置调用点；`internal/producews/service.go`；`internal/download/file.go`；`internal/config/config.go`。
* 规则：根 `AGENTS.md`、`internal/AGENTS.md`、`.trellis/spec/backend/*` 与 `.trellis/spec/guides/*` 已读取。
