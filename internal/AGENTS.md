# AGENTS.md（后端）

作用域：`internal/**`。与根级 `AGENTS.md` 同时生效；本文件补充后端实现约束。跨层字段、事件、下载策略和制作生命周期统一见根文件，修改时须同时检查前端消费者。

## 分层与定位

- `app/` 承担 Wails 边界和跨模块编排，公开请求/响应在此定义；不要让低层包依赖 UI。
- 启动状态模型与通知入口：`envsetup/state.go`、`envsetup/events.go`；检查/动作/状态逻辑按 `service_*.go` 分工。
- 配置默认值、归一化和兼容处理集中在 `config/config.go`；同一工作目录的读改写事务由 `config.Store` 统一串行化并由 App 注入 `envsetup.Service`；应用层设置映射在 `app/clip_settings.go`。
- Demo 事实与整局 POV 解析在 `demo/`；`plugingen/` 的 `NormalizeSelectedItems`、`BuildPlan`、整局 POV 转换与历史过滤只接收显式快照并保持无副作用；`app/plugin_generate*.go` 负责 Wails DTO 适配、工作目录/配置准入、JSON/take/history 写入和 HLAE 启动编排；插件动作仍由 `clipsjson/` 构建。
- 平台导入协调（同键去重、等待者、结果固定与清理）在 `app/import_coordinator.go`，由工作目录身份持有并在切换/重置时替换；`fivee`、`wanmei` 只负责来源解析、下载解压与缓存提交，不互相依赖。
- 会话、清理和文件占用分别从 `app/produce_session.go`、`app/produce_cleanup.go`、`app/work_activity.go` 查起；WebSocket 状态与协议在 `producews/`。
- `edit/` 承担不依赖 App/Wails 的剪辑模型、裁剪/转场/filter graph 规划、ffprobe 解析和 context-aware FFmpeg runner；它只接收显式媒体事实、编码选择、命令工厂与进度回调，不读取配置、切换 workspace、发 Wails 事件或登记 history。`app/app_edit.go`、`app/edit_ffmpeg.go` 仅负责 DTO/配置/准入/路径预留与提交/history/事件适配。
- Windows 专用行为与其他平台实现通过现有平台文件分开维护；非 Windows 测试不能替代 Windows 运行验证。
- `download.ReplaceDirWithContents` 只承担目录文件系统提交，不包含 HLAE/FFmpeg 组件判断。实现必须使用本次实例创建且可确认归属的唯一 staging/backup；准备或备份失败保持旧 target，提交失败先尝试恢复，恢复失败保留两条恢复路径并返回主/恢复双重错误。提交成功但旧 backup 清理失败通过 `ReplaceDirWithContentsWithReport` 暴露为告警，不得回报为安装失败；不触碰既有 `.old` 或未知事务残留。文件系统故障注入使用实例依赖，不增加包级可写 seam。
- `download.CopyFile` 是兼容入口，但使用同目录临时文件完成 Copy、Close 检查和提交 rename；缓存导入通过 `IsLikelyDemoFile` 只做基本文件戳校验，不能把命中结果当作完整性证明。目标已存在但不是普通文件（尤其是目录）时必须在移动目标前拒绝，不得把目录移入备份路径后用文件覆盖。原子复制的故障注入通过实例 `atomicCopyOps`，不得引入包级可写 seam。
- `download.UnzipWithContext` 在打开归档、每个条目和每次复制之间检查 ctx，长解压可被工作目录关闭打断；`download.Unzip` 是无取消的兼容入口，故障注入通过实例 `unzipOps`。平台导入解压 seam 因此接收 ctx，组件安装等旧调用者继续使用兼容入口。

## 工作目录与启动状态机

- 初始化遵循 `app/app_workspace.go`、`appdata/validate.go`、`appdata/registry_windows.go`。保留 service 未初始化时的防御逻辑，不把 `workspace_init` 当作组件检查失败。
- `StartupState` 的 mode、phase、状态值和组件 ID 遵循根文件；新增字段须保持前端兼容，不得只改后端枚举。
- 统一 Release 快照后先检查自身更新。确认发现更新并进入 `needs_action` 时，组件保持未启动语义；自更新检查失败可继续组件检查。
- 广告解析失败不得阻断启动，只向前端暴露有效的 Sponsored Card。广告是纯图模型：`validateAndNormalizeAd` 只要求 `placement/click_url/image_url`，manifest 中多余的 `sponsor/title/rich_html` 必须被忽略而不是触发丢弃。
- HLAE/插件版本读安装目录的 `changelog.xml`，不得改为配置版本号。
- HLAE 通道语义见根文件。通道 ID 由 `release.IsValidChannelID` 限制为单个安全路径段（拒绝 `.` 开头，从而与 `ReplaceDirWithContents` 的 `.<name>.staging-`/`.backup-` 事务目录区分）；读取各通道安装版本属于磁盘 I/O，须在 `Service.mu` 外完成后再 `updateStep`。
- 下载顺序遵循根文件；统一源请求失败可回退本地已安装组件并使用 warning，但不得伪造远端版本成功。不得恢复 `executeWithSourceFallback` / `orderedRetrySources` 旧流程或 gh-proxy 终极兜底。

## 并发、进程与收尾

- `Service.state`、`Service.logs`、已提交的 `Service.config` 快照由 `Service.mu` 保护；配置文件的最新读取、归一化、mutate 和保存只走工作目录共享的 `config.Store`，不再为 App/Service 各自维护文件锁。工作目录准入后再进入 Store，保存成功后才更新 Service 快照，事件必须在 Store 解锁后发射；应用 service 的访问遵循 `serviceMu`。
- `app/workspace_session.go` 的私有 `workspaceSession` 持有不可变 root/generation/service 与生命周期 context；任务须在启动前登记。切换/退出先关闭准入，再取消并等待，不得在 service/state 锁内等待、做 I/O 或发射事件。`envsetup.Service` 的 `BindLifecycleContext`、`CloseIfIdle`、`Stop` 仅由 App 的 session 生命周期调用，旧实例关闭后不得重新接收任务。
- 避免锁内执行阻塞 I/O、网络或 runtime 事件发射，不引入锁顺序反转。启动状态更新后通过 `emitState()` 通知前端；制作事件复用现有队列。
- 调试快照的会话身份、录制列表与版本必须一致：读取期间身份或单调版本变化时重新构建；安装/清空 runtime、登记/清空录制列表须在对应状态锁内提交版本，不得将旧 runtime 或旧录制数据标为新版本。快照读取不持有状态锁发射事件。
- `GetWorkActivity` 的前端禁用策略不代替后端互斥；文件读写/导出/清理复用现有文件使用权机制。多个调用者共享的长任务（如平台导入）必须用 `beginManagedWorkspaceTaskUse` 的工作目录 context 运行并持有所属准入直到真实任务退出；等待者取消不得取消共享任务，进度与提交等副作用只由真实任务执行。
- 制作生命周期遵循根文件“制作、剪辑与清理”：活跃会话禁止重复生成，失败收尾保留重试所需状态；未确认进程退出、环境恢复前不得提前释放备份和占用。
- 调试会话（`app/debug_session.go`、`app/debug_types.go`）是独立 runtime：持有工作目录 context 与文件使用权、环境 epoch、CS2 PID/待确认 HLAE 句柄和 `idle|starting|running|stopping|failed` 状态，复用 `produceLaunchMu` 准入并把占用接入 `produceBusyError`/`GetWorkActivity`，不得只依赖前端禁用按钮。环境准备按显式选项参数化（`gameInfoPrepOptions`、`preparePovForProduceWithOptions`、`prepareBrandForProduceWithOptions`），正式制作保持 plugin+brand+可选 POV 的旧默认；无插件模式必须在会话备份内临时移除残留 `csgo/plugin`，不得删除用户既有 VPK/DLL。未知 PID 保留启动器句柄与备份，确认退出后才恢复环境；失败收尾保留 runtime 供 `StopDebugGame` 重试。两种启动入口在准备新环境前必须统一重试已结束 runtime 的收尾，任一保留会话未释放时拒绝启动；`App.Shutdown` 先停止调试会话再停 WS/关闭 Store，停止失败时不得执行 epoch=0 的共享环境恢复。环境准备使用启动时冻结的同一配置快照（`prepareGameInfoWithConfig`、`preparePluginDLLWithConfig`、`preparePovWithConfig`、`prepareBrandWithConfig`），正式制作包装入口保持原语义；切换路由不得触发停止。
- 调试启动增强（`app/debug_startup.go`）在环境准备前校验 `launch_arguments/startup_commands`，归一化复制参数指针与指令切片，避免调用方修改已冻结请求。参数仅覆盖调试的 CS2 `-cmdLine`，正式制作仍用默认值；调试的渲染分辨率由 `launchResolutionFromArguments` 按双引号/反斜杠边界从参数里的 `-w/-h` 推导，忽略引号内的命令文本（客户端 `launch_resolution` 被忽略），因此调试启动不追加分辨率参数，正式制作仍按 `launch_resolution` 追加。会话 watcher 等待运行与插件 ready 后只尝试一次，在同一 runtime/连接上发送 `debug_execute`，停止准入后拒绝发送；结果复用 `DebugCommandResponse` 写入快照 `startup_command_result` 并推进版本，超时/断连/失败均不自动重试。不得把自动执行放在页面挂载或重连回调中。
- 调试命令通道（`app/debug_command.go`）复用 `producews` 的 `debug_session_ready`/`debug_execute`/`debug_command_result` 消息与当前连接：`producews` 只把 debug incoming 消息转给注册的 handler，不进入队列 ACK/完成处理；就绪要求插件针对本会话 `session_id` 握手且连接仍是当前连接，重连或断开使就绪与在途请求失败。在途请求按发送连接绑定：握手或传输刷新发现连接切换时立即失败旧连接请求，回执只接受同一连接上的匹配 `request_id`，不能只比 request ID。`SendDebugConsoleCommand` 仅 ready 时可调用，命令按序交给游戏线程，超时/断连不自动重发，等待在 `rt.done`、回执或超时三者之一结束时释放；首次 ready 由主动定时器到期更新并发射快照，不得只依赖后续快照读取判断超时；快照的 `plugin_state` 只由这些连接/握手事实推导，就绪/断开事实变化时重发快照；不得在 producews 状态锁内调用 Wails 或执行长等待。
- 调试播放（`app/debug_playback.go`）复用同一 ready/连接绑定与 `sendDebugRequest` 回执路径：`resolveManagedDebugDemoPath` 只允许当前工作目录 `demo` 子目录内存在、非目录、`.dem` 且不含引号/换行/控制字符的路径，拒绝未受管控路径时不得向插件发送消息；`PlayDebugDemo` 的准入、发送与回执提交绑定同一 runtime/连接；只有 `executed` 且所属会话/连接仍有效、没有更晚引擎观测、没有正在播放的旧 Demo 时才记录请求 `demo_path`，不得据此置 `demo_playing` 或用旧回执污染新会话。`debug_demo_state` 与握手/回执一样在 runtime 锁内核对当前连接与本会话 `session_id`，旧连接或外部 session 不得改写播放状态；传输断开或被替换时清 `demo_playing`/`demo_paused`/`demo_play_pending`，重连不重发播放命令。消息为 `{session_id,demo_path,playing,paused}`：`paused` 是插件游戏线程读到的引擎暂停事实，播放为假时强制清 false，不由 tick 增长或命令字符串推断；协议版本 2 拒绝只发 `tick` 的旧 DLL。发送器在同一准入锁内、发送前按 wire request_id 登记 `demo_play_pending` 和确认定时器，成功回执不重新置位；引擎观测到目标 Demo 正在播放（路径完全匹配）时清除并停止定时器。`debugPlayConfirmTimeout` 到期主动清理、更新时间并发射可见错误快照，不依赖 getter 轮询；发送/回执失败、停止或断连/替换清理对应请求，旧定时器/失败回执不得影响重试。等待期间拒绝另一个播放请求。`OpenDemoInFolder` 复用同一受管控 `.dem` 校验（拆分为 `resolveManagedDebugDemoPathIn`）后只打开文件父目录：除字面包含外还要求 `EvalSymlinks` 的真实目标仍在真实 Demo 根内（拒绝符号链接/junction 越界，无法解析则拒绝），扫描路径与链接测试均需覆盖；`openManagedDirectory` 的平台启动走 `startDirectoryOpenFn` 接缝供测试断言目标目录。插件侧 `LoadSequencesFile` 在 `CSDM_DEBUG_SESSION` 非空时不得加载或删除 `<demo>.json`，debug 播放只清 sequences、执行带引号的 `playdemo`。
- 调试手动录制（`app/debug_recording.go`）一次只允许一段，并由单一工作台操作锁串行化 `StartDebugRecording`/`StopDebugRecording`/`RetryDebugRecordingMerge` 与 `PlayDebugDemo`、`StopDebugGame` 收尾：插件事件处理路径（握手、回执、`debug_record_status`、断连）不得获取该锁，否则正在等待的回执永远无法送达。发送 start 前必须重核任务仍为 `starting`；`StopDebugRecording` 在准备期间必须排队至 start 结束再结束同一 recording_id，而不是在插件录制尚不存在时失败。`StopDebugGame`/`Shutdown` 在收尾前把会话置为 `stopping`，之后公开操作改走 `debugPublicRuntime`（要求 `running`）拒绝新录制/换 Demo/控制台命令，旧段合并完成后到 teardown 之间的窗口也不得接纳新工作；仅内部 `sendDebugRecordStop` 保留 ready 通道。段目录按随机 ID 分配，用 `DebugRecordSettingsName` 避免 HLAE 同名 settings 冲突；`debug_record_status` 与 stop 回执均须核对当前连接、会话与 recording_id 后才推进状态，迟到/外部消息不得改写。`debugRecordingTakePaths` 是唯一的输出路径规则（`<dir>/take0000.mp4`、`<dir>/take0000/audio.wav`），不得改成扫描 outputs。session 与录制变更共用严格递增的 `nextDebugStamp` 时钟，快照版本取实际反映数据的最大戳（列表清空额外 bump floor，不能回退；释放并清空 `debugRuntime` 也必须 bump floor，否则最终空闲快照会回退到比上一个会话快照更旧的版本，前端拒绝后继续显示已关闭的会话与 PID）；不得回退到只用 `rt.updatedAtMs` 或用墙钟毫秒直接当版本。合并必须在工作目录任务 context 下用 `producemerge.WaitForTakeFilesReady`/`MergeTakeVideoAudioContext`（超时/间隔在 launch 时快照给 goroutine，不在后台读包级接缝），`StopDebugRecording` 不得阻塞到 ffmpeg 完成；重试与新录制共享同一单段准入（`launchDebugRecordingMerge` 拒绝其他活跃段），每次合并有独立 attempt 代次，取消后未退出的旧 goroutine 不得提交结果或清掉新取消句柄；取消/超时后必须等待旧任务退出（或由代次隔离）才能重试。成功且未保留中间文件时只能删除本段登记的原始视频/音频，合并失败与取消必须保留源文件供 `RetryDebugRecordingMerge`（重试前确认源文件可读）。结果列表为 App 级并随工作目录安装/重置清空；录制/合并接入 `produceBusyError`/`GetWorkActivity`，合并期间拒绝新录制与切换 Demo；断连、游戏退出、会话取消把未结束段标为失败且不重发 start/stop，`StopDebugGame` 先结束有界合并再退游戏。测试若可能留下后台合并，返回前必须 `waitForDebugRecordingIdle` 再让 cleanup 恢复 ffmpeg/超时接缝。
- 剪辑合成一次只允许一个任务：`app/edit_task.go` 的准入拒绝第二个活跃请求而不排队；任务持有工作目录文件使用权、任务专属临时目录和 `O_EXCL` 预留的唯一输出名，只有验证临时视频、确认任务未取消且预留路径仍保有本次文件身份后才 rename 提交并登记 history。失败、超时或取消只清理本任务登记的路径，不得删除已成功提交、被替换对象或其他任务的视频；工作目录取消不得被包装成编码器不可用，也不得触发编码器回退重试，提交成功后不再删除已发布文件。
- HLAE 启动成功但 CS2 PID 未知时，保留启动器句柄并核对进程枚举；不能因 PID 为空直接恢复环境，也不能关闭归属不明的游戏进程。
- 涉及会话退出、取消和 FFmpeg 合并时，检查等待、探测及子进程是否遵循现有取消/超时机制，避免收尾后旧任务继续写文件。

## 设置、生成与游戏环境

- 新设置核对配置默认值/兼容处理、Get/Save DTO、生成逻辑及前端类型/控件；除 `hide_player_avatars` 始终写入 `cl_teamcounter_playercount_instead_of_avatars true|false` 外，关闭命令开关时不生成命令或重置命令。运行时画面/声音与录制初始化的无副作用命令构建集中在 `clipsjson/gamesettings.go`（包括调试专用的显式恢复命令与 `RecordInitCommands`），`app/debug_settings.go` 的 `GetDebugConsolePlan` 只负责草稿归一化、`auto` 编码解析与 DTO 装配，不得把命令拼接放回 App 或前端；`RecordInitCommands` 无条件包含 `mirv_endofmatch 0`（隐藏比赛结束动画/结算画面，HLAE ≥ 2.181.1，无配置开关，正式 Build、调试录制与调试计划共用），不得为它新增设置字段或关闭分支；`motion_blur_enabled` 开启时同一个 helper 把 `record fps` 改为 `MotionBlurCaptureFPS` 派生的采集帧率，并按 `MotionBlurCommands` 创建参数名派生的 sampler（内层引用本次 ffmpeg settings 名）后由 `record screen settings` 使用；sampler 的 `strength` 不是模糊强度（`1` = 不跨输出帧泄漏），拖影长度由 `exposure` 决定，采集率派生与上限不得下沉到前端；调试草稿不得写回配置，调试录制输出固定为 `<dataDir>/outputs/debug`，并使用按编码参数派生的 `DebugRecordSettingsName`（正式 Build 继续用实际 preset 名），因为同一游戏进程内 HLAE 不覆盖同名 settings 条目；`GetDebugConsolePlan` 必须可重复调用且同一参数派生同一名称。编码设置的 `effective_video_preset`、`effective_video_encoder`、`effective_video_status` 仅作为 Get/Save 响应的只读探测结果，不能覆盖持久化的 `video_preset` 用户策略。`noflash_strength` 是强度型设置（`0.0–1.0`，两位小数，`config.NormalizeNoFlashStrength` 归一化），启用判定为 `> 0`：正式 Build 为 0 时不写命令，调试行为的显式恢复值是 `mirv_noflash 0.00`；不得把它改回布尔开关或另造默认强度。
- POV HUD 内嵌资源由 `tools/hud-pov-vpk/build_vpk.py` 从模板与 CSS 构建；顶部头像血条按 `HUD--team--ct/terrorist` 当前视角阵营仅显示己方，整条敌方容器（含红色背景）透明，未知阵营默认隐藏双方。更新资源后执行 `python3 -m unittest discover -s tools/hud-pov-vpk -v`，并另行验证 CT/T、切视角和换边的游戏内显示；仍不得覆盖用户已有 `pov.vpk`。
- `pov_radar_enabled` 不得与 `pov_hud_enabled` 的 VPK/gameinfo 生命周期联动（雷达开关不控制 POV VPK 的部署/卸载）；`sky_blackout` 不得联动关闭云层。开启时 `clipsjson` 逐 pass 在 `spec_mode`/`spec_player` 后、`mirv_streams record start` 前发一次 `csdm_radar_pov 1`，录制起点前至少两秒 Demo 预热；关闭时保持原序列分组与 tick 时序。
- `block_kill_feed` 与 `pov_kill_feed_only` 改写同一 HLAE 过滤列表：正式 Build 中 block 优先，开启时不发逐 pass 过滤，避免 `mirv_deathmsg filter clear` 清掉初始化序列的 block 规则。POV 过滤必须逐 pass 在 `spec_player` 后用该 pass 的显式 Steam64 发送（`clipsjson.PovKillFeedCommands`），因为 HLAE 在命令执行时解析 `xTrace`/ID 而不是每条提示到达时；缺少有效 XUID 时仍清空规则列表，避免沿用上一 pass 的过滤。调试工作台使用 `PovKillFeedTraceCommands` 的 `xTrace` 变体，切换视角后需重发；block 开启时该计划行不输出 current 命令，避免 `apply_all` 用 `filter clear` 撤销 block 规则。前端互斥只在 UI 层，后端始终保留 block 优先级。
- `producegame/assets/brand.vpk` 为每次录制必需的内嵌品牌资源，无配置或 UI 开关；`app/produce_brand.go` 部署至 CS2 的 `csgo/brand.vpk`，与可选 POV 同样保留用户既有 VPK，仅清理本会话创建的文件。挂载、环境 epoch 回滚、失败收尾重试和 gameinfo 残留修复须同时覆盖品牌路径。
- `hide_chat_messages` 默认开启，旧配置缺少字段时继承默认值，显式 false 保持关闭；开启时每个 Demo 的初始化序列写入 `hidehud 128`，在片段录制前执行，关闭时不写入或重置 `hidehud`。
- `primary_view`、单片段覆盖和整局 POV 的语义遵循根文件。主视角是选中玩家视角，不能固定为 killer；`include_killer` 缺省 true 的兼容性须保留。
- 录制 take plan/history 的 `tick_rate`、`record_start_tick`、`record_end_tick`、`kill_offsets_seconds` 只作为后续剪辑的可选观测元数据；插件动作和原有 source window 保持不变。这些字段只保留在内存录制历史与 `GetProduceHistorySnapshot` 快照中，不在视频旁写入 `.fastedit.json` 或任何衍生文件。
- debug DLL override 只作为 `App` 会话状态，不写入配置，也不参与启动插件版本检测。
- debug override 仅改变注入源 DLL，目标、备份和恢复复用 `preparePluginDLLForProduce` / `forceRestorePluginDLLForProduce`。
- 修改生成计划时核对 take 命名、稳定 source ID、历史去重键与前端选择状态，不能仅验证 JSON 能序列化。
- 批量生成在准入并停止旧会话后只读取一次 `config.Store` 快照；后续 job 不得重新读取配置而产生窗口、编码或命令设置漂移。规划函数不得读取时钟，批次时间戳由 App 注入。

- 调试握手与回执在 runtime 锁内检查消息 `ConnID` 等于 `producews.GameConnID()`；旧 readLoop 延迟处理的有效握手、不兼容协议版本与回执都须忽略，不能覆盖新连接 ready 或失败新连接请求。保持 runtime → producews 状态锁顺序，producews 调用 handler 前须解锁。

## 日志与诊断

- 启动链路统一使用 `internal/logging` 的 `log/slog` 适配层，通过统一 logger API 记录结构化字段。
- 保留 `slog.HandlerOptions.ReplaceAttr` 脱敏，敏感信息不得进入内存 ring buffer 或 `log` 事件流。
- 关键字段保持可追踪：`component/stage/action/source/attempt/error/elapsed_ms`。
- 导出和错误路径同样脱敏 URL 参数、认证信息及 home path；禁止记录明文 token、密钥、认证头或真实 home 目录。
- 制作诊断应保留 WS、队列、take 和退出阶段信息；正常有序断开不能误记为故障。导出内容范围见根文件。

## 验证与维护

- 后端代码改动执行 `go test ./...`；envsetup/release 改动确认这两个包通过，可用 `go test ./internal/envsetup ./internal/release` 定位失败。
- 生成规划改动需覆盖 `primary_view`/兼容输入、override 继承、full-round 顺序与 source ID、历史过滤、take 元数据和批次快照冻结；`go vet ./...` 作为提取完成后的静态检查。
- 状态、回退或日志契约变更，按涉及范围补充状态迁移、持久化/回退、字段与脱敏测试。
- 制作生命周期变更应覆盖重复启动、文件清理互斥、失败收尾重试、进程归属与取消传播等实际受影响路径。
- 剪辑裁剪范围须校验有限值、边界和非空结果；缺少可靠 victim 标记时保持整段，不按当前设置推断死亡位置。可选裁剪与无音轨合成由 `internal/edit` 的单次处理路径执行，并覆盖混合整段/裁剪输入。编辑相关改动还须覆盖单任务准入、唯一输出预留、失败/取消不写 history 以及不删除既有产物。
- 跨层契约变更还须执行前端构建并更新根文件与前端规则；仅文档改动按根文件的文档检查执行。
