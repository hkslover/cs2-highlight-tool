# Debug 播放等待状态修复记录

日期：2026-10-11。用户授权在第二次 review 后直接修复两个 P2 问题。本轮只修改桌面端对应实现、测试和规则；已有两个仓库的其他 WIP 全部保留，未提交或推送。

## 修复结果

- 播放发送器在同一 runtime/连接准入锁内，以真实 wire request_id 登记等待状态和定时器，随后才发送消息。成功回执只保留原有路径记录语义，不重新置等待，确认先到不会被后处理的回执覆盖。
- 20 秒确认超时使用主动定时器，清理等待、更新快照时间、通过 `debug_session_state_changed` 通知页面，同时显示“等待 Demo 播放确认超时，请检查游戏播放状态后重试”。快照 getter 直接读取状态，不靠读取时比较时间隐式过期。
- 确认、发送/回执失败、回执超时、停止、协议拒绝和连接失效会清理相应等待并停止定时器。失败回执与定时器按请求 ID 判断所有权，旧请求不能解除新请求等待。
- 等待目标确认期间后端拒绝另一个播放请求，页面同时禁用播放按钮并显示已有等待提示；暂停准入继续使用原来的共享策略。
- 成功回执不表示实际播放或暂停事实。Demo 路径/playing 的观测仍使用现有协议和同一连接/会话守卫，本次没有修改插件消息或版本。

主要实现：`internal/app/debug_command.go`、`debug_playback.go`、`debug_session.go`；页面变更：`frontend/src/features/debug/pages/DebugPage.vue`。根与前后端 AGENTS 已同步发送前登记及主动超时语义。

## 回归测试与验证

新增 `internal/app/debug_playback_pending_test.go`：

1. 使用真实本地 WebSocket 测试传输，先应用目标播放确认、再释放成功回执，断言发送前已登记等待且回执返回后不会重新置位；包含同路径旧播放事实不直接绕过等待的情况。
2. 仅等待状态事件，不轮询 getter，验证超时主动推送等待清除、可见错误和新快照时间。
3. 验证旧请求定时器回调和失败清理不能影响同路径重试，当前请求失败能够清除自己的等待。

实际执行结果：

| 检查 | 结果 |
| --- | --- |
| 相关播放/观测/链接测试 | 通过 |
| `go test -race ./internal/app -run 'TestDebugPlay\|TestOldPlayback\|TestPlayDebug\|TestDebugDemo\|TestManagedDemo' -count=1` | 通过，无竞态报告 |
| `go test ./...` | 通过，含 envsetup |
| `cd frontend && npm test` | 87 项通过，0 失败 |
| `cd frontend && npm run build` | 通过；保留原有 chunk size 提示 |
| `git diff --check` | 通过 |

本轮没有改变 Wails 公开 DTO/方法，因此不需要重新生成绑定。既有工作模型的生成绑定改动仍保留。没有重新构建插件 DLL，也没有在 Windows 实测真实 Wails 事件桥、CS2 加载/暂停或注入；本地传输和事件接缝测试不能代替这些验收。

前两份 review 文件作为当时审查结果保留；其中这两个问题已由本轮处理，最新交付状态以本记录与当前源码为准。
