# 02 启动会话代码审查

审查日期：2026-10-11。基线：`10f3052af12acd5284504aa641016dbf7a675118`（`feat(debug): add independent debug session and HLAE launch`），对比其父提交。审查依据为 `02-launch-session.md`、计划 README、根目录及前后端 AGENTS 规则，以及制作启动、环境恢复、工作目录生命周期和前端消费者。

结论：发现 2 项 P1、3 项 P2，当前实现尚不能完整验收 02。本次只审查，没有修改产品源码。审查开始时工作区仅有未跟踪的 `docs/plans/debug-workbench/`，原文件均保留。

## 1. [P1] Shutdown 在 debug 进程关闭失败后仍恢复游戏环境

定位：[internal/app/app.go:267](../../../internal/app/app.go#L267)，关联同文件 272–277 行及 `debug_session.go:412–420`。

`stopDebugSessionWorker()` 返回关闭失败或等待超时后，Shutdown 只记录日志并继续执行。如果没有正式制作 runtime，`stopProduceSessionWorker()` 返回 nil，随后进入 `forceRestoreProduceEnvironmentForProduce()`。这个 epoch=0 的恢复入口会处理 debug 使用的同一份 gameinfo、DLL、POV 和品牌状态，绕过 debug teardown 必须先确认进程退出的保护。

触发：debug CS2 无法退出或退出状态无法确认时关闭桌面程序。结果是在游戏可能仍运行时恢复 gameinfo/替换 DLL/删除本会话 VPK，备份也可能被消耗，违反 02 第 8、9 项及根规则的恢复顺序。

证据：临时 overlay 用例 `TestReviewShutdownRetainsDebugEnvironment` 注入 CS2 关闭错误，Shutdown 后 gameinfo 已恢复，用例按保护要求断言失败。

建议：debug 停止失败后禁止无条件恢复共享环境；只允许已确认关闭的所属 runtime 执行恢复，并保留失败状态、文件占用和备份。

## 2. [P1] 收尾失败的会话可被另一种启动模式抢占

定位：[internal/app/debug_session.go:82](../../../internal/app/debug_session.go#L82) 及 [internal/app/debug_session.go:184](../../../internal/app/debug_session.go#L184)。关联 `work_activity.go:45–68`、`plugin_generate_launch.go:31–38`、`produce_gameconfig.go:541–546`。

两个方向都有缺口：

- debug 的 `done` 已关闭但 `teardownErr` 非空时，`debugBusyError()` 返回 nil。正式制作入口只重试正式制作 worker，没有停止或重试这个 debug runtime，随后可以开始新一轮环境准备。
- 正式制作的 `done` 已关闭但收尾失败时，`produceBusyError()` 同样放行。`LaunchDebugGame()` 只调用 `stopDebugSessionWorker()`，没有调用 `stopProduceSessionWorker()`，随后直接递增环境 epoch、准备资源并启动 HLAE。

触发：旧会话因为 CS2 关闭失败或环境恢复失败而进入可重试状态，用户切换页面，启动另一种模式。`GetWorkActivity()` 虽会显示忙碌，但启动入口未强制遵守保留占用的事实。旧游戏可以仍在运行，新 epoch 还会使旧 runtime 的后续恢复变成空操作；共享备份及状态可能被新准备覆盖。

证据：临时 overlay 的 `TestReviewRetainedDebugAdmission` 证明保留 CS2 的 failed debug 被正式制作准入放行；`TestReviewDebugRejectsFailedProduce` 证明 failed 正式制作仍持有旧 PID 时，debug 实际进入 HLAE 启动并返回成功。

建议：两个启动入口在新环境准备和 JSON/状态写入前统一处理两种已结束 runtime 的收尾。活跃会话拒绝抢占；保留资源的失败会话必须先成功恢复，否则拒绝本次启动。

## 3. [P2] 环境准备没有使用启动时冻结的配置路径快照

定位：[internal/app/debug_session.go:215](../../../internal/app/debug_session.go#L215)。关联 `produce_gameconfig.go:68–77`、`290–303`、`348–357`、`produce_brand.go:31–44`。

`LaunchDebugGame()` 读取 `cfg`，但这份快照仅传给 `launchHLAEGameWithOptions()`。gameinfo、插件 DLL、POV 和品牌 helper 仍各自调用 `a.loadConfig()`；runtime 也没有持有冻结的配置路径。`produceLaunchMu` 不覆盖配置 Store 的所有写入入口，例如 `PickCS2Path()`。

触发：启动准备期间另一个允许的调用修改 CS2 路径。环境 helper 可按新路径准备安装 B，而 HLAE 仍按首次 `cfg` 启动安装 A；多个 helper 之间也可能使用不同路径。结果可能是启动成功但资源未加载，或修改了另一个安装目录。即使文件操作各自正确，仍违反 02 第 1 项的同一快照要求。

证据：源码调用链确认；本次未进行并发路径变更的运行复现。

建议：一次读取配置并解析相关路径，把同一配置/路径快照显式传给全部准备及启动 helper，正式制作包装入口保持原有语义。

## 4. [P2] 插件连接/断开不会更新 debug 页的连接状态

定位：[internal/app/debug_session.go:158](../../../internal/app/debug_session.go#L158)，关联同文件 `366–409`、`frontend/src/domains/debug/debugSessionCore.ts:123–129` 和 `internal/producews/service.go:2022–2023`。

`fillDebugPluginState()` 只在读取 debug 快照时查询 WS 状态。debug 的事件仅在启动、停止、失败和退出时发射，watcher 的存活轮询没有发射插件状态变化。WS 连接/断开发出的事件是 `produce_ws_state_changed`，debug domain 仅订阅 `debug_session_state_changed`，初始化后也不刷新快照。

触发：HLAE 返回 PID 后插件才连接，或运行中插件断开。页面会一直显示启动返回时的“等待插件连接”或旧的“插件已连接”。切出再进入页面也不会必然刷新，因为 domain 初始化有去重。这影响 02 已要求展示的 transport 连接状态，不属于 03 尚未实现的协议 ready。

证据：检索全部 debug 发射点、WS 状态发射点和前端订阅路径确认；未运行真实 Wails 桥验证。

建议：将 WS 状态变化转换为 debug 快照事件，并更新其排序时间戳；或由 debug domain 明确订阅并刷新。连接成功仍须保持 `plugin_ready=false`，直到 03 握手完成。

## 5. [P2] launch/stop 的迟到响应可污染新工作目录状态

定位：[frontend/src/domains/debug/debugSessionCore.ts:166](../../../frontend/src/domains/debug/debugSessionCore.ts#L166)，关联同文件 `182–195`、`199–207`。

`init()` 和事件回调检查 generation，但 `launch()`、`stop()` 没有。`dispose()` 清空状态并增加 generation 后，旧启动/关闭请求仍会调用 `apply()`，失败回调也会写入旧错误，finally 还可能清除新请求的 loading 标记。

触发：旧操作后端已完成并释放资源，但前端 Promise 尚未返回，期间工作目录切换调用 reset/dispose；旧响应随后覆盖新会话状态。此时本地时间戳已重置为 0，旧快照会被接受，造成错误的运行状态、PID 和按钮策略。违反 02 第 10 项“清空旧请求结果”。

证据：对 `npm test` 生成的真实 core 执行最小调用：创建 pending launch → dispose → 完成旧 launch。最终输出 `session_id=old-workspace, status=running`，说明已清空状态被旧结果重新填入。

建议：launch/stop 捕获调用时 generation，所有状态、错误和 finally 写入均先检查 generation；页面侧应用返回值也应忽略已经失效的生命周期。补充迟到成功、迟到失败和新请求并发的测试。

## 验证与验收边界

| 检查 | 实际结果 |
| --- | --- |
| `go test ./...` | 全部通过，含 app、envsetup、release、日志等包 |
| `cd frontend && npm test` | 类型契约检查通过，65 个测试通过 |
| `cd frontend && npm run build` | 类型检查与 Vite 构建通过；存在 bundle 大小提示 |
| 临时 Go overlay 回归断言 | 3 个用例均失败，分别复现 Shutdown 恢复和双向跨模式准入缺口；未加入仓库测试源码 |
| 前端迟到请求最小复现 | 成功复现 dispose 后旧 launch 回写 |
| Windows HLAE/CS2、资源组合和环境恢复 | 当前 macOS 环境未验证 |
| 真实 Wails 事件桥、Vue 页面/路由交互 | 未验证 |

已有实现覆盖了两模式 HLAE Hook、独立 debug DTO、显式资源开关、无插件搜索路径清理、活跃会话互斥、未知 PID 保留、关闭重试，以及协议未就绪时保持 ready=false。上述通过项和源码检查不能替代 Windows 验收，也不能覆盖本报告指出的失败路径。

本次不把 03–06 尚未实现的自定义命令、播放、实时设置、手动录制列为 02 缺陷。建议先修复两项 P1，再补齐三项 P2 和对应回归验证后进行 Windows 验收。
