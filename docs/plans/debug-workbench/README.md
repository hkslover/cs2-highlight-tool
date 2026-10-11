# Debug 调试工作台实施计划

状态：待实现。编写日期：2026-10-11。本次只新增计划文档，尚未实现功能。

## 目标与执行方式

保留品牌点击 10 次开启 debug 的入口。在导入、片段、制作、剪辑四个步骤后追加第五个「调试」步骤，提供 HLAE 启动、可选插件加载、启动前资源选择、插件连接状态、快捷设置与自定义控制台命令、当前导入 Demo 播放、手动开始/停止录制及音视频合并。

后续模型按下面顺序执行，一次只做被指定的步骤。每步完成后报告改动文件、自动验证、Windows 实测及未完成项。不要一次性重写制作系统，也不要为了调试功能建立通用插件框架。

| 步骤 | 文件 | 可验收结果 |
| --- | --- | --- |
| 01 | [01-navigation.md](01-navigation.md) | debug 开启后出现第五步和独立页面 |
| 02 | [02-launch-session.md](02-launch-session.md) | 两种 HLAE 启动、资源选择、关闭与恢复 |
| 03 | [03-plugin-command-channel.md](03-plugin-command-channel.md) | 插件接收自定义命令，游戏线程执行，桌面端收到回执 |
| 04 | [04-demo-playback.md](04-demo-playback.md) | 播放导入页当前 Demo，不执行旧制作 JSON |
| 05 | [05-live-settings.md](05-live-settings.md) | 设置快捷命令、调试草稿和重启项语义完整 |
| 06 | [06-manual-recording.md](06-manual-recording.md) | 任意时刻录制、停止、合并、连续录多段 |
| 07 | [07-integration-acceptance.md](07-integration-acceptance.md) | 两仓库联调、回归与 Windows 验收 |

依赖顺序：01 → 02 → 03 → 04 → 05 → 06 → 07。02 可以先验证进程与环境，03 完成后才验证插件就绪和启动雷达。

## 本计划确定的产品语义

1. 调试页面是主界面的一个路由，保持启动 `mode=main`，不新增 startup mode/phase。切换页面不关闭调试游戏。
2. 两种模式都由 HLAE 加载 `AfxHookSource2.dll`；「无插件」仅指不加载本项目的 `server.dll`。不能把它实现成直接运行 `cs2.exe`。
3. 插件模式提供页面内远程命令、播放和录制控制。无插件模式先用于 HLAE/资源对比，相关远程按钮禁用并说明需要插件；可在游戏原生控制台操作。首版不另建 RCON、键盘模拟或第二套 HLAE 控制通道。
4. 启动前可选择 POV HUD、品牌 VPK、分辨率、插件加载。品牌默认开启但可在调试中关闭；正常制作仍始终加载品牌。这些选择只对本调试会话生效。
5. POV 雷达依赖插件，和 POV HUD/VPK 独立。启动前可勾选；实际由插件游戏线程就绪后、播放 Demo 前启用。无插件模式禁用此选项。运行中允许开启/关闭。
6. 调试草稿从已有设置初始化，操作不调用 `SaveClipSettings`，不自动改变正式制作配置。设置页原有 debug DLL override 与保留中间文件选项继续复用。
7. 录制不选择击杀、不生成 take 计划、不自动跳 tick、不自动退出游戏。停止一段录制立即进入文件等待与合并，完成后继续留在游戏中，可开始下一段。
8. 首版一次只录制/合并一段；合并完成后开始下一段，避免新增并发合并调度。
9. 录制结果先放调试页独立列表和 `<dataDir>/outputs/debug/...`，提供定位文件与重试合并。首版不写正式制作 history，不伪造 killer/victim/kill ID，也不扩展 history_type；正式制作历史与去重保持原语义。
10. 自定义命令允许普通控制台命令与分号组合，快捷设置只是便捷入口。首版不做任意 cvar 查询、控制台全文流或确认所有命令实际生效的系统。

第 3 条是首版范围决定：如果后续要求无插件模式也能从页面远程控制，应单独补充传输方案，不能假装不存在的插件连接可以执行命令。

## 当前代码核对基线

桌面端：`/Users/snow/Desktop/my_workspace/cs2-highlight-tool`，`main`，HEAD `63a60ff2f869e79de5d14736c978c3a7d83d2e41`。开始阅读时工作树干净。

插件端：用户指定的 `/Users/snow/Desktop/my_workspace/cs2-highlight-tool-work/cs2-server-plugin`，`main`，HEAD `2a398e2fbbf343775edbdb315ff3d2b3b6c156a7`。存在未跟踪 `.pi/`、`.vscode/`、`cs2-server-plugin.log`、`tools/__pycache__/`，后续保留。不是旁边 `/Users/snow/Desktop/my_workspace/cs2-server-plugin` 的镜像仓库；后者当前只有发布脚本与 README，没有实现源码。

上述提交仅用于定位阅读基线，后续实现前重新检查两仓库状态与规则，不假设 HEAD 未改变。

| 能力 | 已有入口 | 复用方法与缺口 |
| --- | --- | --- |
| 隐藏 debug 入口 | `frontend/src/shared/state/useDebugSettings.ts` | 直接复用 `debugEnabled`、点击计数、`keepProduceIntermediates` |
| 工作流导航 | `frontend/src/app/components/MainApp.vue`、`frontend/src/app/router.ts` | 当前四步；追加 `/debug`，仅 debug 可见 |
| 当前 Demo | `frontend/src/domains/demo/demo-state.ts` | 复用 `selectedEntry`/`selectedDemo`；播放以 entry 的受管控 `file_path` 为准，不要求有击杀 |
| 设置 | `internal/app/clip_settings.go`、`frontend/src/domains/settings/` | 读取正式设置为调试初值；独立临时草稿，避免共享保存防抖把调试操作落盘 |
| HLAE | `internal/app/hlae_launch.go` | 路径解析、分辨率、启动参数、PID 检测可复用；当前强依赖 produce WS，需支持显式启动选项 |
| 环境 | `internal/app/produce_gameconfig.go`、`produce_brand.go`、`internal/producegame/` | gameinfo、DLL、POV/品牌投放、epoch 恢复可复用；目前 plugin/brand 强制挂载，需参数化 |
| 游戏关闭 | `internal/app/produce_cleanup.go`、`produce_session.go` | PID 归属、确认退出、失败后保留备份可复用；不能直接使用自动队列结束/退出 worker |
| WS 发送 | `internal/producews/service.go` | `SendCommand`、loopback 端口、串行写入、事件队列可复用；无通用控制台协议和 debug 状态 |
| 插件消息 | 插件 `cs2-server-plugin/main.cpp` | `HandleWebSocketMessage` 当前只处理 `playdemo`、`capture-player-view`、`end_produce_session` |
| 游戏线程命令 | 插件 `QueueEngineCommand`、`NewFrameStageNotify` | 已有线程安全队列与 `ExecuteClientCmd`，扩展队列项即可，不在 WS 线程调用 engine |
| 雷达 | 插件 `TryHandleRadarPovSequenceCmd`、`radar_pov.cpp` | `csdm_radar_pov` 是 JSON 特殊动作，当前不是普通注册控制台命令；调试必须共享处理入口 |
| 播放 | 插件 `LoadSequencesFile` 与 `playdemo` 分支 | 旧分支读取 `<demo>.json`，调试必须显式绕开，不覆写或删除旁车 |
| 录制配置 | `internal/clipsjson/builder.go`、`internal/ffmpegprofile/` | 复用录制命令、编码选择与 4:3 实像素拉伸；不得调用完整 Build 去伪造一个片段 |
| 录制通知 | 插件 `record_status_bridge.h/.cpp` | 当前仅自动 sequence 执行边界时发消息，普通 queued command 不回传录制状态 |
| 合并 | `internal/producemerge/merge.go` | 复用 `WaitForTakeFilesReady`、`MergeTakeVideoAudioContext`；debug 单段任务接入 |
| 文件互斥 | `internal/app/work_activity.go`、`workspace_session.go` | debug 纳入现有制作互斥和工作目录任务准入，不能只禁用前端按钮 |

## 最小结构与契约方向

桌面端新增 `internal/app/app_debug.go`（Wails 方法）、`debug_types.go`（DTO）、`debug_session.go`（调试会话）、`debug_recording.go`（单段录制合并）；需要共享的无副作用命令构建放 `internal/clipsjson/` 或其他已有合适领域包。不要预先创建泛化 manager/interface 层。

前端新增 `frontend/src/features/debug/`，页面与 composable 放该 feature；只有跨路由需要持续持有的会话状态才放 `frontend/src/domains/debug/`。调用统一经过 `shared/backend/`，类型放 `shared/types/debug.ts`。

拟新增 Wails 方法（属于计划，不是当前已有 API）：

| 方法 | 作用 |
| --- | --- |
| `GetDebugSessionState()` | 会话、启动选项、插件就绪、当前 Demo、录制状态、结果列表 |
| `LaunchDebugGame(request)` | 冻结启动配置、准备环境、启动 HLAE，返回当前状态 |
| `StopDebugGame()` | 若录制中先结束并处理文件，再退出游戏、恢复环境；失败时可重试 |
| `SendDebugConsoleCommand(request)` | 发送自定义或快捷命令，返回带 request_id 的执行回执 |
| `PlayDebugDemo(request)` | 调试播放受管控 Demo，不加载制作序列 |
| `StartDebugRecording(request)` | 冻结本段编码/FPS，分配唯一目录，配置并开始 |
| `StopDebugRecording()` | 停止后启动异步合并，返回状态，不等完整合并才响应 |
| `RetryDebugRecordingMerge(recordingID)` | 重用失败段已知路径重试合并，不重录 |
| `OpenDebugRecordingInFolder(recordingID)` | 定位本会话已完成结果，避免复用只允许正式 history 路径的入口 |

新增独立事件 `debug_session_state_changed`，载荷与 Get 状态一致，包含 `session_id/updated_at_ms`。不要把 debug 状态写入 `produce_queue_state_changed`。内部调试协议详见 03；公开 DTO 在对应实现步骤确定并同步消费者。

会话状态建议 `idle|starting|running|stopping|failed`；录制状态建议 `idle|starting|recording|stopping|merging|completed|failed`，与现有制作/启动枚举隔离。`failed` 必须带错误，若尚持有游戏环境则保持忙碌，可重试关闭；不能按文字状态直接推断已释放资源。

## 约束与非目标

- 保留现有进程归属、文件使用权、环境 epoch 与关闭恢复约束；不另写 crash recovery、复杂审计、无限重试、全量能力协商或持久任务数据库。
- 不在调试启动中调用 `GeneratePluginJSONBatchAndLaunchHLAE`、`StartQueue` 或自动制作 worker；它们会跳 tick、消费历史、结束后关游戏。
- 启动选项从配置快照派生，不临时修改全局 config 再改回来。正常制作调用参数化 helper 时显式保留原有默认行为。
- 任意控制台命令可能自行 `quit`、`disconnect` 或改录制路径。允许发送，但不承诺自动合并用户绕过录制按钮创建的文件；页面录制按钮创建的任务才是受管理产物。进程退出须被会话检测并恢复环境。
- 只在重要路径补测试：两种启动、资源恢复、特殊雷达命令、无 JSON 播放、单段 start/end 与合并、互斥。不要为每个展示字段写重复实现的测试。
- 不手改 Wails 生成绑定、auto-imports/components 声明；修改生成源后运行 `wails generate module` 与项目构建流程。i18n 默认只改中文。
- 每步变更公开 API/事件/目录职责时更新适用的 `AGENTS.md`，不要把本计划的拟议接口描述为已经实现。

## 验证边界

本计划来自两仓库源码阅读，未执行游戏或 DLL 构建。实施后按仓库规则运行 Go 全量测试、前端测试与构建；插件使用 Windows MSBuild。非 Windows 自动检查不能证明 HLAE 注入、雷达 Hook、真实游戏命令或录制成功。最终实机验收见 07。
