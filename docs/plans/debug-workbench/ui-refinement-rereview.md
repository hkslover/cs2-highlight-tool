# Debug workbench 修复复审

日期：2026-10-11。范围：上次 review 的两项 P2 修复及其新增状态/测试/消费者；同时核对插件播放消息时序。结论：越界链接问题已修复，暂停按钮增加了在途准入；新增的播放确认等待逻辑仍有 2 项 P2 问题，建议继续修正。本次未修改应用或插件源码。

## 发现

### 1. P2：播放确认先到时，后处理的成功回执会重新置 pending

位置：`internal/app/debug_playback.go:46`、`markDebugPlaybackPending` 的 76–80 行。

成功回执返回后，`PlayDebugDemo` 无条件调用 `markDebugPlaybackPending`。该函数不检查发送前观测版本，也不检查本次请求是否已经获得确认。因此，若目标 Demo 的 `debug_demo_state` 在 Go 调用方从回执等待恢复执行前到达，handler 先更新了实际播放事实，随后回执路径又把 `debugDemoPlayPending` 设为 true。

这不是依赖插件主动逆序发送：即使插件按回执→状态消息发送，host 的 WebSocket 读循环也可能先读完两条消息，等待回执的调用方随后才被调度。目标已在播放且没有其他状态变化时，插件不会重复发送确认，暂停按钮就会保留等待。

验证：通过 Go overlay 加载系统临时目录中的复现测试，先调用现有 `handleDebugDemoState` 应用目标实际播放观测，再走 `markDebugPlaybackPending` 回执路径；测试失败，明确得到“confirmed playback was marked pending again when the receipt path ran”。未往仓库写入该测试源码。

建议：pending 应在发送前按本次 runtime/连接/请求身份登记，并由对应确认清除；失败或超时按同一身份清理。或在回执路径保留足够的观测版本/请求身份，避免覆盖发送后已到达的确认。不要仅根据“当前路径等于目标且 playing=true”跳过等待，那会把同文件重播前的旧播放误认为新请求已确认。补充确认先于回执处理、同文件重播和上一请求迟到消息的测试。

### 2. P2：确认超时没有主动发射状态，页面仍可能一直禁用暂停

位置：`internal/app/debug_session.go:221`；新增测试 `internal/app/debug_playback_test.go:697`。

20 秒截止只在 `debugSessionSnapshot` 被调用时计算 `pending=false`。没有播放确认超时定时器，也没有在截止时更新 `updatedAtMs` 并发射 `debug_session_state_changed`。正常进程 watcher 只检查游戏退出，不定期推送快照；前端 debug core 也不轮询状态。

当页面已收到 `demo_play_pending=true`，而之后没有新的播放变化或连接事件时，20 秒经过也不会更新前端保存的快照，暂停按钮仍保持禁用。这也使发现 1 中的竞态不能通过当前超时机制可靠恢复。

现有 `TestDebugPlayPendingExpiresWithoutConfirmation` 不断调用 `GetDebugSessionState`，所以只证明按需读取的结果变为 false，不能证明真实页面得到解除禁用事件。

建议：建立绑定 runtime、连接及本次请求的有限超时任务，在截止时清理对应 pending、更新快照时间并发射事件；确认/失败/断连/停止/工作目录切换时取消或失效该任务。超时给出可见说明，不靠其他操作偶然刷新页面。测试应只订阅事件等待超时，不通过轮询 getter 驱动更新，并覆盖旧定时器不能清除新请求。

## 上次问题的复核

- 越界链接：`ensureManagedRealPathWithin` 对根与目标执行 `filepath.EvalSymlinks` 后检查真实路径包含关系；文件链接、父目录链接越界都补了拒绝用例，目录打开器不会被调用。上次这一项可关闭。Windows junction 行为仍需实际平台验证。
- 暂停准入：页面使用 `canToggleDemoPause`，涵盖 `playingRequest`、`pauseRequesting`、`sending` 和后端 `demo_play_pending`；处理函数也重新检查准入。原“请求等待回执期间仍可点击暂停”已修复，但新增的确认窗口尚需解决上述两项问题。
- 插件内部 tick/重播 gate 仍保留，本轮没有为修复这些问题扩大到制作队列。

## 验证记录

| 检查 | 本次结果 |
| --- | --- |
| `go test ./...` | 首次 envsetup 的 `TestInstallFFmpegFromArchive_LogsStages` 失败；该用例单独重跑通过，最终全量重跑通过。未证明首次失败原因，也未修改该包 |
| `cd frontend && npm test` | 87 项通过，0 失败 |
| `cd frontend && npm run build` | 通过，仍有 chunk size 提示 |
| 两仓库 `git diff --check` | 通过 |
| 确认先于回执处理的 overlay 复现测试 | 按预期失败，证明发现 1；不计入正常套件通过项 |

另一次 JSON 格式全量复跑因工具输出缓冲区不足提前终止，不作为应用失败或完整检查结果；随后提高输出缓冲区运行普通全量命令，退出码为 0。

overlay 测试文件：`/tmp/cs2-debug-rereview-observation_test.go`，映射文件：`/tmp/cs2-debug-rereview-overlay.json`。命令：`go test -overlay /tmp/cs2-debug-rereview-overlay.json ./internal/app -run '^TestReviewObservationBeforePlayReceiptMustNotBecomePending$' -count=1`。

本次未运行 Windows DLL 构建、真实 Wails 页面、HLAE/CS2 暂停或 Windows junction；本轮插件源码相较上次审查未增加对应修复，未重复运行已通过的独立 C++ 检查。两仓库 WIP 保留，未提交或推送。
