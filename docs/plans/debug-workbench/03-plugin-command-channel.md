# 03：插件通用调试命令通道

前置：02。这是用户明确提出的插件改进：接收其他指令，并交给游戏控制台执行。

插件仓库必须使用 `/Users/snow/Desktop/my_workspace/cs2-highlight-tool-work/cs2-server-plugin`。改动点为 `cs2-server-plugin/main.cpp`、`record_status_bridge.h/.cpp` 或一个必要的小型 debug protocol 源文件；新增编译源时同步 `.vcxproj/.filters` 和 Makefile 的现有源列表。

## 当前实现与必须复用的路径

- Go `producews.Service.SendCommand(name,payload)` 已发送 `{name,payload}`，通过既有 WS 写锁串行化，不需新服务器。
- 插件 `HandleWebSocketMessage` 未识别通用命令；现有 `pendingCommands` 存字符串，`NewFrameStageNotify` 在游戏线程消费并调用 `ExecuteClientCmd`。
- `TryHandleRadarPovSequenceCmd` 对 `csdm_radar_pov 0/1` 做特殊处理，不能直接把该字符串交给 engine。
- easywsclient 由 WS 线程独占；其他线程通过 `SendMsg` 入队。保留此线程归属。

## 拟议最小协议

仅当 `CSDM_DEBUG_SESSION` 非空时开启 debug 分支；正式制作行为保持现状。以下都是新增消息，不是现有协议。

| 方向/name | payload | 时机与语义 |
| --- | --- | --- |
| 插件 → `debug_session_ready` | `{session_id, protocol_version:1}` | 游戏线程 hook 已解析 engine 并进入命令消费路径、且连接建立后发送；重连时重发；不代表 Demo 已播放 |
| 桌面 → `debug_execute` | `{request_id, commands:string[]}` | 自定义字符串作为一个数组元素，保留分号；快捷设置可多元素按序执行 |
| 插件 → `debug_command_result` | `{request_id, status:"executed"|"failed", error?}` | 游戏线程处理全部命令之后回传；不是 WS 收包后立即 ACK |

04 增加 `debug_play_demo`，06 增加 `debug_record_start/debug_record_stop/debug_record_status`。共用 request_id 回执，不增加泛化 RPC 框架。Go 仅维护当前会话少量待回执请求，有限超时；连接断开使待请求失败，不自动重发命令或录制开始。

## 实施

1. 扩展现有队列项，保留普通字符串入队的薄包装，同时允许携带 debug request_id 和后续录制 metadata。网络线程只解析和入队；游戏状态变更、录制动作及特殊雷达命令全部由游戏线程处理。
2. 抽出/改名共享的插件特殊动作处理入口。自动 JSON sequence 和 queued debug 命令都调用它，再由未处理命令走 `ExecuteClientCmd`。不要尝试把 `csdm_radar_pov` 当普通 cvar 注册后另写一套逻辑。
3. 注意现有雷达处理会调用 `RadarPov_QueueEngineSetup` 再入队；当前消费 pendingCommands 时持有 pendingCommandsMutex，直接在锁内调用雷达 helper 会死锁。先在锁内取出队列项，释放锁后执行，保留命令顺序。
4. 自定义命令支持空白 trim、空输入报错、顺序执行。普通命令字符串整体交给 engine，保留分号与引号；对插件特殊指令至少支持单独发送 `csdm_radar_pov 0/1`。不需要实现完整控制台语法解析器；组合特殊命令时让 UI 分开发送。
5. 快捷设置和自定义输入共用 `SendDebugConsoleCommand`；只有当前插件调试会话 ready 时可调用。后端按当前 debug session 持有的连接与 ready session_id 判断，不接受其他遗留游戏连接为本次 ready。
6. 把 debug incoming 消息接入 `producews` 的现有分发和事件通路，交给 debug 会话处理；不得调用队列 ACK/完成处理函数。不要在锁内调用 Wails 或执行长等待。
7. 回执表示已调用 engine/插件特殊处理。普通 `ExecuteClientCmd` 无通用成功返回，UI 文案用“已交给游戏执行”。雷达 hook 安装失败可从特殊处理入口返回明确错误，别沿用当前仅写日志的成功外观。
8. 首次 debug ready 超时显示“插件未连接或不支持调试协议”；不需要复杂能力协商。新版 DLL 可用现有 override 选择，先联调，不自动发布 release。
   debug 会话在独立初始化线程每 100 ms 重试 client hook 安装，不等待 `ClientFullyConnect`；安装线程不执行 engine 命令，ready 由游戏线程消费路径确认，退出前 join 初始化线程。桌面端在修改 ready、协议错误或接收回执前确认消息连接仍为当前连接，拒绝旧 readLoop 延迟处理的握手和回执。
9. 接入自定义输入按钮、请求错误与最近命令结果。日志走既有脱敏规则，不增加永久命令历史、控制台全文抓取或无限缓冲。

## 验收

- 普通命令、分号组合按顺序到达游戏线程；未知控制台命令不被桌面端白名单挡住。
- 自定义发送 `csdm_radar_pov 1`/`0` 能开关，首次安装不会死锁。
- 回执时点是游戏线程执行后；不将 WS 发送 nil error 当执行成功。
- 未 ready、旧 DLL、断连、超时均有可见错误；不自动重发有副作用的命令。
- JSON 自动制作的雷达与普通命令路径保持原行为。
- 桌面端三项检查；插件 Windows `msbuild /m /p:Configuration=Release /p:Platform=x64 cs2-server-plugin.sln`，产物 `static/cs2/server.dll`；源码静态检查不能代替该构建与实机验证。
- 更新插件 README 的 debug 环境变量/消息语义，和桌面端跨层规则。
