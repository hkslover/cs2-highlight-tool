# 07 阶段性交付报告：两仓库联调与验收

报告日期：2026-10-12。本报告根据源码核对、已有自动检查记录及用户补充的 Windows 测试反馈整理。

当前结论：01–06 的实现已提交并推送，用户已在 Windows 环境测试并确认功能正常。plan07 已完成本地协议核对与部分展示收尾；完整逐项验收、Windows 构建记录及实机产物信息尚待补齐。记录缺口不代表功能测试失败，整体正常反馈也不等于全部异常场景均已通过。

用户提出部分功能和前端展示较繁杂，可能另列功能删减与界面精简计划；该工作暂不开展，不列为本步功能缺陷或验收阻塞项。

## 1. 仓库基线与本步变更

| 仓库 | 分支 | 本步开始前已推送提交 |
| --- | --- | --- |
| 桌面端 `/Users/snow/Desktop/my_workspace/cs2-highlight-tool` | `feat/debug-workbench-navigation` | `40ab811` feat(debug): add manual recording and isolated audio-video merge |
| 插件端 `/Users/snow/Desktop/my_workspace/cs2-highlight-tool-work/cs2-server-plugin` | `feat/debug-plugin-command-channel` | `35515a5` feat(debug): add game-thread manual recording protocol |

桌面端 01–06 已提交：`0459189`、`10f3052`、`0eaa165`、`1c551ad`、`60c4876`、`d21e771`、`8ed87c0`、`40ab811`。
插件端调试协议已提交：`943bbfd`、`deb26f1`、`e636b34`、`7262f77`、`35515a5`。

以上为本地核对基线。用户尚未提供 Windows 测试实际使用的桌面端提交、插件提交及 DLL 路径，因此暂不将实测结果绑定到未经确认的二进制版本。

### 本步桌面端变更

前端展示改动尚未提交。用户授权提交重写后的报告，本次文档提交包含本报告及验收条件修正，不包含下表的前端代码；不执行推送、tag 或 release。

| 文件 | 变更 |
| --- | --- |
| `frontend/src/features/debug/pages/DebugPage.vue` | 录制结果行补上「创建时间」「源文件目录」，最终视频行不再回退成源目录；失败段显示合并重试说明；新增本地 `formatCreatedAt` |
| `frontend/src/shared/i18n/zh-CN.json` | 删除骨架期占位键 `not_wired`（接口待接入）与冗余 `record_starting_recording`；其余 4 个录制键改为真实使用 |
| `docs/plans/debug-workbench/07-delivery-report.md` | 整理交付状态、用户实测反馈及待补证据 |
| `docs/plans/debug-workbench/07-integration-acceptance.md` | 将 Demo 选择验收条件同步为已实施的调试页独立下拉选择 |

### 本步插件端变更

无。插件工作树仅保留原有未跟踪 `.vscode/`。

## 2. 源码与协议核对结论

桌面端 DTO、`shared/types`、`backend/types.ts` + `adapter.ts`、生成绑定（`frontend/wailsjs/go/**`）、debug 事件与插件消息名/字段已核对并对齐。以下属于源码核对结论，不代替游戏内验收：

- 消息名双向一致：`debug_session_ready`、`debug_execute`、`debug_play_demo`、`debug_record_start`、`debug_record_stop`、`debug_command_result`、`debug_demo_state`、`debug_record_status`。
- 字段一致：握手 `{session_id, protocol_version}`；命令 `{request_id, commands}`；播放 `{request_id, demo_path, commands?}`；录制起停 `{request_id, recording_id, demo_path, take_name, commands}` / `{request_id, recording_id}`；回执 `{request_id, status, error?}`；播放观测 `{session_id, demo_path, playing, paused}`；录制边界 `{session_id, recording_id, demo_path, take_name, record_phase, tick, ts_ms}`。
- 协议版本双方均为 2；桌面端拒绝旧 DLL 并提示更新，不静默露出无法确认的暂停按钮。
- 归属清晰：`producews.handleIncomingMessage` 把 4 个 debug incoming 消息转给注册的 debug handler 后立即返回，不进入队列 ACK/完成处理；前端只有 `src/domains/debug/debugSessionCore.ts` 订阅 `debug_session_state_changed`，produce/take/history 域不消费 debug 事件，debug 域也不订阅 `produce_ws_state_changed`。
- Wails 方法齐备：`GetDebugSessionState`、`LaunchDebugGame`、`StopDebugGame`、`SendDebugConsoleCommand`、`PlayDebugDemo`、`StartDebugRecording`、`StopDebugRecording`、`RetryDebugRecordingMerge`、`OpenDebugRecordingInFolder`、`GetDebugConsolePlan` 均有 Go 实现、生成绑定与强类型适配层条目。
- Demo 选择以已实施的 `ui-refinement-plan.md` 为准：调试页从已导入列表独立选择，不要求返回导入页改选；只有点击播放才切换游戏中的 Demo。

## 3. 自动检查记录（macOS）

以下沿用此前实施与本步联调的执行记录。本次报告重写只做文档核对，没有重新运行应用测试或构建。

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 后端全量测试 | `go test ./...` | 通过 |
| 前端测试 | `cd frontend && npm test` | 通过，`fail 0` |
| 前端构建 | `cd frontend && npm run build` | 通过（`vue-tsc --noEmit` + Vite） |
| 插件便携回归 | `clang++ -std=c++14 -Wall -Wextra -Werror tests/debug_playback_test.cpp -o /tmp/csdm-debug-playback-test && /tmp/csdm-debug-playback-test` | 通过；在插件仓库执行 |
| 插件协议测试 | `clang++ -std=c++14 -Wall -Wextra -Werror -Ics2-server-plugin/deps/json/include tests/debug_protocol_test.cpp cs2-server-plugin/record_status_bridge.cpp -o /tmp/csdm-debug-protocol-test && /tmp/csdm-debug-protocol-test` | 通过；在插件仓库执行 |

快照与录制相关 race 回归、延迟读取的关闭/替换回归在 01–06 提交前已通过。上述自动检查不能替代 Windows DLL 构建、真实 Wails 页面交互、HLAE 注入或实际录制产物质量验证。

## 4. Windows 测试与插件构建记录

用户补充反馈：已在 Windows 环境测试功能，确认功能正常；同时认为部分功能和前端展示较繁杂，拟后续另列精简计划。

这条反馈已作为实际使用结果记录。尚未提供逐项测试范围、构建日志、实际 DLL 路径及对应提交，因此不能据此补写具体 MSBuild 成功记录或宣称完整验收表全部通过。执行本地核对的代理没有 Windows/MSVC 环境。

| 项目 | 当前记录 |
| --- | --- |
| Windows 功能整体测试 | 用户反馈通过 |
| Windows 测试使用的桌面端/插件提交 | 待补 |
| 插件 Release x64 构建结果及日志 | 待补记录；本地代理未执行 |
| 本次实测加载的插件 DLL 路径 | 待补 |
| 本步尚未提交的前端展示改动 | 尚无针对这批改动的 Windows 实测反馈 |

- 标准构建命令：`msbuild /m /p:Configuration=Release /p:Platform=x64 cs2-server-plugin.sln`
- 预期产物：`static/cs2/server.dll`（`OutDir=$(SolutionDir)static\cs2\`，`TargetName=server`；`*.dll` 被 `.gitignore` 忽略，不会入库）
- macOS 本地仓库中找到的历史插件 `server.dll`：`build/windows/cs2-server-plugin-windows-2b4e6ae6e9fa19c48bea79313644f7ff70a60a47/server.dll`。其目录名对应提交 `2b4e6ae6`，二进制不含本次核对的 debug 协议字符串，不能用它证明新版调试功能通过。这一结论仅描述本地历史产物，不代表用户在 Windows 使用了该 DLL。SDK 和游戏 DLL 不属于本项目插件产物。

## 5. Windows 逐项验收记录

已记录整体正常反馈；用户未逐条列明下列子项的覆盖情况。“待补逐项记录”表示证据尚未细分，不表示用户尚未进行任何 Windows 测试。

| 场景 | 状态 |
| --- | --- |
| 隐藏入口（10 次品牌点击出第五步、路由与窄窗口） | 待补逐项记录；代码路径已核对 |
| 无插件启动（HLAE Hook、未加载本项目 server.dll、显示未启用插件） | 待补逐项记录 |
| 插件启动（新 DLL ready 与 session_id 一致、PID 与连接分别可见） | 待补逐项记录 |
| 资源对比（POV/品牌组合与退出恢复） | 待补逐项记录 |
| 雷达（启动勾选、播放前启用、运行中 1/0、与 POV HUD 独立） | 待补逐项记录 |
| 任意命令（普通/分号/自定义未知命令、无伪造 cvar 成功） | 待补逐项记录 |
| 设置热更（天空/云层/UI/头像/聊天/X-ray/语音/越肩/击杀信息） | 待补逐项记录 |
| 旧 JSON（旁车计划不执行、原文件不变） | 待补逐项记录 |
| 当前选择（调试页独立下拉选择已导入 Demo，点击播放才切换，无片段选择也可用） | 待补逐项记录 |
| 单段录制（任意起停、声画正常、不关游戏） | 待补逐项记录 |
| 连续录制（两段路径不同、无历史污染、保留中间文件开关有效） | 待补逐项记录 |
| 停止/失败（断连、合并失败、手动关游戏、应用退出） | 待补逐项记录 |
| 互斥（debug 活跃时制作/再启动/清理受约束、退出后释放） | 实机子项待补；Go 单测已覆盖准入路径 |
| 正式流程回归（品牌、JSON、历史、队列结束退出与恢复） | 待补逐项记录 |
| 正式配置（debug 设置不落盘、显式 false 与默认迁移） | 实机子项待补；草稿不写回已有自动测试 |

目前没有用户报告的功能失败项。异常、环境恢复和正式流程回归尚不能仅凭整体正常反馈标为全部通过。

## 6. 实机产物信息

| 信息 | 当前记录 |
| --- | --- |
| 实际加载的新版插件 DLL | 用户尚未提供路径和版本 |
| 演示 Demo | 用户尚未提供路径 |
| 第一段合并视频 | 用户尚未提供路径 |
| 第二段合并视频 | 用户尚未提供路径 |
| 原始视频/音频路径及保留开关结果 | 用户尚未提供逐段记录 |

源码中的输出规则为 `<dataDir>/outputs/debug/<session-id>/<recording-id>/`；原始文件由 `debugRecordingTakePaths` 定位为 `<dir>/take0000.mp4` 与 `<dir>/take0000/audio.wav`，最终视频以结果中的 `final_path` 为准。这里记录的是代码规则，不将“尚未提供产物路径”写成“用户没有生成产物”。

## 7. 当前交付状态与后续事项

- **01–06：实现与本地自动验证完成，已提交推送；Windows 正常功能已获用户整体确认。**
- **07：本地协议核对和展示收尾已完成；完整逐项验收与产物记录待补，暂不标记为全部验收完成。**
- 补记 Windows 测试使用的提交、DLL 路径和构建结果；若现有测试已使用新版 DLL，可沿用并补记，无需仅因本地代理没有 Windows 环境而重复构建。
- 将第 5 节已测试的场景补记为通过，未覆盖的异常、环境恢复和正式流程回归继续验证；实际发现失败时再回所属步骤修复。
- 补记演示 Demo 和至少两段合并视频路径，以及真实录制路径、保留中间文件和声画结果。
- 功能删减与前端精简留待用户另行制定和授权计划，本步不实施。

首版非目标（无插件远程控制、cvar 读取、控制台全文流、调试录像进入正式 history）继续保持总计划边界。
