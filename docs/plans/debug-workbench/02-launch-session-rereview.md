# 02 启动会话修复复审

审查日期：2026-10-11。基线：`0eaa165f0a8aa95013601bed1bf1cd7c4af95525`（`fix(debug): guard shutdown restore and cross-mode session teardown`），对比父提交 `10f3052af12acd5284504aa641016dbf7a675118`。依据为原报告 `02-launch-session-review.md`、02 计划、根目录与前后端 AGENTS 规则。

结论：原报告的 2 项 P1、3 项 P2 已在源码层面修复，本次没有发现新的阻断缺陷。自动验证通过，可以进入 Windows 与真实 Wails 验收；不能据此宣布 02 已完成全部验收。仅新增本复审文档，未修改产品源码、测试或原报告，未提交 Git。

## 原问题逐项核对

| 原问题 | 复审结果与证据 | 验证边界 |
| --- | --- | --- |
| P1：Shutdown 在 debug 关闭失败后恢复环境 | `internal/app/app.go:273` 保存两种 stop 的错误，`restoreSharedEnvironmentAfterShutdown` 仅在两者均成功时执行共享恢复。关闭失败时保留 gameinfo 与备份，后续成功重试恢复原文件。 | 新增 `TestShutdownKeepsDebugEnvironmentWhenStopFails` 验证 stop 与恢复 helper 的组合，不直接执行完整 `App.Shutdown`。完整退出流程仍需实机验证。 |
| P1：失败收尾被另一模式抢占 | `internal/app/debug_session.go:211` 与 `internal/app/plugin_generate_launch.go:38` 均先处理两种 runtime 的收尾；任何失败立即退出，发生在新环境 epoch、JSON 生成与 take 状态重置之前。活跃会话仍由原准入拒绝。 | 双向保留 runtime 回归测试通过；debug → produce 还断言 gameinfo 与备份未被新启动消费。 |
| P2：准备阶段重新读取配置 | `internal/app/debug_session.go:241` 读取一次 `cfg`，随后 gameinfo、DLL、POV、品牌以及 HLAE 启动接收同一快照。新 `WithConfig` helper 不再调用 `loadConfig`；恢复沿用会话记录的路径。正式制作入口保留原包装语义。 | 本项为调用链审查；新增测试未覆盖启动中并发修改 CS2 路径的场景，本次也未运行该场景。 |
| P2：插件连接变化不更新 debug 状态 | `internal/app/app.go:232` 在 `produce_ws_state_changed` 时调用 `refreshDebugPluginState`，更新当前活跃插件会话的时间戳并重发 debug 快照。连接仍从 WS 状态读取，`plugin_ready` 保持 false。producews emitter 由独立事件 worker 调用，没有在 Service 状态锁内重入 `GetWSState`。 | 新测试验证刷新时间戳与无插件模式不刷新，未建立真实插件连接/断开并验证 Wails 页面。 |
| P2：迟到 launch/stop 污染新工作目录 | `frontend/src/domains/debug/debugSessionCore.ts:163`、`:186` 对成功、错误和 finally 的写入增加 generation 守卫；旧成功响应返回 null。`DebugPage.vue:387` 忽略 null，不回填启动草稿。 | 仓库新增测试覆盖 launch 的成功、失败与旧请求不清除新 loading。本次另以编译后的 core 做 stop 成功/失败迟到复现，两种情况均保持新状态、错误与 stopping 不受污染。 |

此外，`internal/app/work_activity.go:263` 增加 failed debug 收尾的显式清理准入保护，保留环境的 runtime 不能被目录清理或工作目录切换绕过；相应回归测试通过。

## 非阻断的文档一致性问题

`frontend/AGENTS.md` 的“调试工作台”写明“迟到的成功或失败响应返回 null”，实际实现仅对旧成功响应返回 null；旧失败响应仍向调用者抛错，但不写当前 domain 的错误或 loading。`frontend/tests/debug-session.test.mjs` 也明确断言旧失败继续 reject。

当前页面捕获并忽略该错误，因此本次没有复现新工作目录状态污染。建议将规则改为“旧成功响应返回 null，旧失败仍 reject，但不得回写当前状态”，或统一实现和测试的返回语义。此项是文档与实现不一致，不将原 P2 重新判为未修复。

## 验证结果

| 检查 | 实际结果 |
| --- | --- |
| `go test ./...` | 全部通过，包含 app、envsetup、release 与日志相关包 |
| `go test -race ./internal/app -run 'Test(ShutdownKeepsDebug\|RetainedDebug\|RetainedProduce\|RefreshDebug\|DebugRetained\|LaunchDebug\|DebugSession\|DebugUnknown)' -count=1` | 通过；仅针对该匹配集合，不代表全仓 race 验证 |
| `cd frontend && npm test` | 类型契约检查与 Node 原生单元测试通过 |
| `cd frontend && npm run build` | vue-tsc 与 Vite 构建通过；仍有超过 500 KiB 的 bundle 提示 |
| 不落盘的 Node stop 迟到结果复现 | 成功/失败两种情形均通过状态隔离断言；未加入仓库测试源码 |
| Windows HLAE/CS2 启动、资源组合、未知 PID 与恢复 | 当前 macOS 环境未验证 |
| 真实 Wails 事件桥、Vue 页面与路由交互 | 未验证 |

建议后续补充并发配置路径变更、真实 WS 连接/断开到 debug 事件的集成回归，并将本次 stop 隔离断言纳入长期测试。这些是测试证据的补强，不是本次已确认的产品缺陷。03–06 未实现的协议、播放、实时设置与手动录制不纳入本次 02 复审缺陷。

## 工作区

开始时 `git status --short` 仅有 `?? docs/plans/debug-workbench/`。自动验证后仍仅有该未跟踪目录；本次复审文档也位于其中。原有计划和报告均保留。
