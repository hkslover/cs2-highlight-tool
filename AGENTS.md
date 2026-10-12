# AGENTS.md

本文件是仓库 AI 协作规则的主入口。`internal/AGENTS.md` 与 `frontend/AGENTS.md` 分别补充后端、前端实现约束；更具体的目录规则优先。本文件记录长期约束和源码入口，不作为变更日志或完整 API 手册。

## 项目与代码地图

CS2 Demo 导入、片段选择、自动录制与后期拼接的 Windows 桌面工具。主流程为：工作目录初始化 → 启动环境准备 → 导入 → 选择片段 → 制作 → 剪辑。

技术栈：Wails v2、Go（版本要求见 `go.mod`）、Vue 3、TypeScript、Naive UI、vue-router 4。构建配置见 `wails.json` 和 `frontend/package.json`。

| 入口 | 职责 |
| --- | --- |
| `main.go` | Wails 入口与应用绑定 |
| `internal/app/` | Wails 暴露边界、业务流程编排、制作会话与文件使用权管理 |
| `internal/appdata/` | 工作目录校验、Windows 注册表读写、旧数据清理 |
| `internal/envsetup/` | 启动状态机、自更新、组件检查安装、启动日志导出 |
| `internal/config/` | 配置读写、默认值、兼容处理、路径规范化与工作目录级 `Store` 事务 |
| `internal/release/`、`internal/endpoints/`、`internal/download/` | Release 快照、下载地址策略、下载与解压 |
| `internal/demo/` | Demo 解析、击杀事实、整局 POV 计划 |
| `internal/wanmei/`、`internal/fivee/` | 对战平台查询与 Demo 下载 |
| `internal/plugingen/`、`internal/clipsjson/` | 无副作用的片段归一化/历史过滤/生成规划与插件 JSON 动作构建 |
| `internal/producews/` | 制作 WebSocket、队列、take 状态与诊断 |
| `internal/producegame/`、`internal/producemerge/` | 游戏环境准备与恢复、录制产物合并 |
| `internal/edit/` | 无 Wails 副作用的剪辑事实归一化、转场/filter graph 规划、ffprobe 与 FFmpeg 执行器 |
| `internal/ffmpegprofile/`、`internal/procutil/` | 编码能力探测与回退、平台进程工具 |
| `internal/logging/`、`internal/changelog/` | 结构化日志与脱敏、内嵌版本更新说明 |
| `frontend/src/app/` | 应用壳、导航、hash 路由 |
| `frontend/src/features/` | `workspace-init`、`startup`、`import`、`clips`、`produce`、`edit`、`settings`、`debug`、`ads`、`changelog` |
| `frontend/src/domains/` | 跨页面领域状态与无头业务逻辑（`clip-selection`、`demo`、`edit`、`production`、`settings`；仅 TypeScript，不放 Vue SFC） |
| `frontend/src/shared/` | 通用类型、状态、视角映射、事件常量、i18n |
| `frontend/src/shared/backend/` | 强类型 Wails 调用适配层、后端方法契约与统一边界错误 |
| `frontend/src/shared/ui/` | 跨 feature 复用的展示组件；业务页面组件仍归属对应 `features/` |
| `frontend/tests/` | Node 原生领域单元测试与 TypeScript 契约检查；运行时产物写入前端缓存目录 |
| `frontend/wailsjs/` | Wails 自动生成绑定 |
| `.github/workflows/release-windows.yml` | Windows 发布构建 |
| `.github/workflows/ci.yml` | PR/主干 CI，windows-latest 上的后端 Go 测试与前端测试/构建（与发布工作流同平台，无 Linux 矩阵） |

## 开始工作与改动边界

1. 阅读本文件和目标目录的 `AGENTS.md`，检查 `git status`，保留已有的无关改动与生成产物。
2. 从上表定位源码，确认改动是否触及配置、Wails 方法、事件、状态或插件协议；同时检查对应前后端消费者。
3. `internal/app` 负责 Wails 边界与流程编排，不把 UI 逻辑下沉到低层包；可复用逻辑放在对应领域包。
4. 不得手工修改 `frontend/wailsjs/**`、`frontend/src/auto-imports.d.ts`、`frontend/src/components.d.ts`；需要改变绑定时修改生成源，使用项目生成流程。
5. i18n 默认只修改 `frontend/src/shared/i18n/zh-CN.json`，`en-US.json` 由用户维护；用户明确要求同步英文时按该要求执行。
6. 不得擅自引入与现有状态机冲突的状态、阶段或组件 ID。不得恢复 `PROJECT_STRUCTURE.md`，也不得另行维护 `CLAUDE.md` 作为主规则文件。

## 运行与验证

| 改动或任务 | 必需命令 / 核对 |
| --- | --- |
| 后端代码 | `go test ./...` |
| 前端代码 | `cd frontend && npm run build`（类型检查和 Vite 构建） |
| 前端领域/适配层测试 | `cd frontend && npm test`（类型检查与 Node 原生单元测试） |
| 同时涉及前后端或接口契约 | 执行 `go test ./...`、`cd frontend && npm test` 与 `cd frontend && npm run build` |
| 启动状态机、下载回退、日志脱敏 | 全量后端测试中确认 `internal/envsetup`、`internal/release` 及相关日志测试通过 |
| 仅文档 | `git diff --check`，核对文档提及的路径、方法、字段与源码；无需仅为文档变更运行应用构建 |
| 本地开发 / 产物构建 | `wails dev` / `wails build` |

前端依赖缺失时可在 `frontend` 执行 `npm ci`。按改动补充有意义的测试；涉及 UI 交互时按前端规则核对相关路径。汇报实际执行结果和未验证项；Node 单元测试和前端构建不能证明真实 Wails 事件桥、Vue 页面交互或 Windows 上 HLAE/CS2 注入、录制和环境恢复成功。

## 跨层契约

以下保留易出错的语义。完整方法签名和 JSON 字段以 `internal/app/` 的公开方法、请求/响应结构及 `frontend/src/shared/types/` 为核对入口；变更契约时须同步实现、消费者、测试与适用的 AGENTS 文档。

### 工作目录与启动

- `GetWorkspaceState`、`PickWorkspaceDir`、`ValidateWorkspaceDir`、`SetWorkspaceDir`、`ResetWorkspace`、`ExitApp` 位于 `internal/app/app_workspace.go`。
- Windows 从 `HKCU\Software\CS2HighlightTool` 的 `DataDir` 读取工作目录；未配置或目录不可用时进入 `workspace_init`。选择父目录时自动追加 `cs2HighLightTool`；目录校验由 `internal/appdata/validate.go` 负责。
- `<dataDir>` 是已选工作目录，配置、组件、Demo、输出、临时文件和日志均以此为根；不可再假设 Windows 固定使用 `%LOCALAPPDATA%/CS2 Highlight Tool`。非 Windows 开发回退见 `fallbackDataDirForDev`。
- `<exeDir>` 与 `<dataDir>` 必须区分：前者定位程序和自更新替换目标。`ResetWorkspace` 会删除当前整个工作目录并清除注册表记录，不等同于清理 Demo 或 outputs。
- `internal/app/workspace_session.go` 的私有工作目录实例固定 root、generation、envsetup service 与生命周期 context；后台任务须先登记，Reset/Shutdown 先关闭准入并按既有制作/文件占用规则取消、等待或拒绝，旧实例不得在新工作目录提交后写文件或发射启动事件。
- 每个工作目录实例只创建一个 `internal/config.Store`，由 App 与 `envsetup.Service` 共享；Store 锁覆盖最新配置读取、默认/兼容归一化、mutate 和原子保存，网络、探测与事件不得进入 Store 锁。工作目录关闭时 Store 永久关闭，旧实例不得重新落盘；锁顺序保持为工作目录准入 → Store，事件在 Store 解锁后发射。
- `GetStartupState`、`RunStartupChecks`、`RetryStartupComponent`、`ReinstallStartupComponent`、`CancelStartupDownload`、`OpenManualDownload`、`ImportManualDownload`、`PickCS2Path`、`EnterMainApp`、`OpenExternalURL`、`ExportStartupLogs` 的入口为 `internal/app/app_startup.go`。
- 状态源为 `GetStartupState` 和 `startup_state_changed`，模型见 `internal/envsetup/state.go`。`mode` 为 `workspace_init|startup|main`；`phase` 为 `detecting_source|waiting_source|running_tasks|ready`；二者不可混用。
- 组件 ID：`hlae`、`plugin`、`ffmpeg`、`cs2`。组件/启动状态：`pending`、`checking`、`downloading`、`installing`、`ready`、`warning`、`failed`、`needs_action`。
- 启动时实时 GeoIP 检测，统一更新源固定为 `github`，地区结果不持久化。先获取统一 Release 快照，再检查软件自身更新；确认 `self_update.available=true` 时必须先更新软件，不启动组件检查/安装。自更新检查失败保持非致命语义。
- 组件下载策略：CN 或 GeoIP 失败/为空时，`mirror_url` 与 `url` 并行竞速，先成功完成的链路胜出并取消另一条；非 CN 仅 `github_url`。竞速全部失败才报错，不恢复多源自动回退或 gh-proxy 终极兜底。统一源失败可使用本地已安装组件并标记 warning，不伪造远端成功。
- HLAE、插件本地版本均来自各自安装目录 `changelog.xml` 首个 `<version>`，不以配置持久化版本为真值。
- HLAE/FFmpeg 目录安装使用 `internal/download.ReplaceDirWithContents` 的提交事务：先在目标同卷创建本次唯一 staging 并完成复制，再将旧目标移到本次唯一 backup，最后 rename 提交；备份 rename、提交或恢复失败不得主动删除旧版本，恢复失败须保留 staging/backup 并返回双重错误及路径。提交成功后的 backup 清理失败只作为告警并保留 backup，不得被当成安装失败触发破坏性重试；历史遗留 `.old` 或事务目录不属于本次所有权，不得无条件删除。组件必需文件由 envsetup 在提交前验证，`ReplaceDirWithContentsWithReport` 的清理告警由调用方记录。该事务不宣称覆盖进程在两次 rename 之间崩溃的自动恢复。
- `StartupState.ads[]` 仅包含有效的 Sponsored Card（支持 `main_steps_top_banner` 顶部横幅与 `main_entry_popup` 进入主界面弹窗），且为纯图广告：下发字段只有 `click_url/image_url/image_alt`，`image_url` 支持远程 `http(s)` 与 `data:image/...`。Release manifest 继续返回的 `sponsor/title/rich_html` 由客户端忽略，既不作必填也不因此丢弃该条，服务端无需改字段。点击走外部浏览器，广告失败不得阻断启动。

### Demo 导入与解析

- `PickDemoFiles` 返回受管控路径 `<dataDir>/demo/raw/...`，不直接返回原始选择路径。
- `ListWanmeiRecentMatches(page)` 将小于 1 的页码归一为 1；状态为 `client_not_running|client_not_logged_in|ready`，战绩包含 `download_match_id/k4/k5/rating`。`ImportWanmeiMatch` 接受 `PVP@...`，产物为 `<dataDir>/demo/wanmei/<matchID>/<matchID>.dem`。
- `GetFiveEPlayerName` 读取持久化 `fivee_player_name`；`ListFiveERecentMatches(playerName, page)` 接受 ID 或含 `domain=<id>` 的分享链接，先规范化并保存 domain ID，页码最小为 1，战绩包含 `match_id/download_match_id/rating`。`ImportFiveEMatch` 接受 ID/URL/zip 名，产物为 `<dataDir>/demo/5e/<matchID>/<matchID>.dem`。
- 平台导入仅在解压文件经 `internal/download.CopyFile` 的同目录临时文件、Copy/Close 检查和 rename 提交完成后返回成功；失败只清理本次临时文件并保留旧目标。旧缓存命中前仅做 regular、长度和 `PBDEMS2`/`HL2DEMO` 文件戳的基本校验，不能据此证明尾部未截断；校验失败会重新获取，不默认引入版本化提交标记或强制全量重下。
- 同一工作目录内按 `(平台, 规范化 matchID)` 共享在途导入任务（`internal/app/import_coordinator.go`）：同键并发只执行一次真实下载/解压/提交并返回同一路径，不同键继续并行，5e 与 wanmei 的同文本 ID 不得合并。失败不缓存，重试会重新执行真实任务；等待者退出只退出自己的等待。共享任务运行在工作目录生命周期 context 下并持有文件使用权直到真实任务退出，工作目录关闭会取消下载与解压（`download.UnzipWithContext` 在遍历和复制过程中检查 ctx）且不提交缓存；`download_progress` 只由真实任务发出。
- `ParseDemoFile` 的 `players[]` 包含 `name/steam_id/steam_id_text/kills/deaths/assists`，不包含 `team`；`clip_players[]` 按玩家、回合、击杀组织。前端请求必须使用字符串 SteamID，不能将 JS number 的 `steam_id` 转回字符串使用。

### 设置与插件计划

设置入口为 `internal/app/clip_settings.go`，默认值与持久化在 `internal/config/config.go`，命令生成在 `internal/clipsjson/builder.go`。前端 `src/domains/settings/` 的共享草稿在工作目录切换进入 `workspace_init` 时必须调用 `resetForWorkspace`；离开该阶段后重新 `init`，旧 generation 的在途读写不得回写当前状态。

| 字段 | 稳定语义 |
| --- | --- |
| `record_quality`、`edit_quality` | `standard | high | ultra`，默认`high`；软件编码映射 CRF，硬件编码映射 QP /`q:v` |
| `edit_fps` | `24..240`，默认 `60` |
| `video_preset` | `auto | c1 | n1 | a1 | i1`，默认`auto`，由后端 FFmpeg 能力探测选择 |
| `launch_resolution` | `16:9 | 4:3 | 4:3_1280x960`，默认`4:3`；两种 4:3 为`1440x1080`、`1280x960`，录制通过 FFmpeg `-vf scale=trunc(ih*16/9/2)*2:ih` 真实拉伸为方像素 16:9（替代旧的 `-aspect 16:9` 元数据标签，兼容忽略 SAR 的播放器与异常捕获高度） |
| `record_output_dir` | Get/Save 设置时固定为 `<dataDir>/outputs` |
| `hide_all_ui` | 默认 false；开启写入 `cl_draw_only_deathnotices 1` |
| `hide_player_avatars` | 默认 false；开启写入 `cl_teamcounter_playercount_instead_of_avatars true`，关闭写入 `cl_teamcounter_playercount_instead_of_avatars false` |
| `hide_chat_messages` | 默认 true，旧配置缺少字段时按开启处理，显式 false 保持关闭；开启在每个 Demo 的初始化序列中写入 `hidehud 128`，在片段录制前执行；关闭不写命令或反向重置 |
| `use_shoulder_camera` | 默认 false；开启在 `r_show_build_info 0` 前写入越肩命令 |
| `sky_blackout` | 默认 true；开启仅写入 `r_drawskybox 0` |
| `disable_clouds` | 默认 false；开启仅写入 `mirv_sky clouds draw 0`，与天空开关独立 |
| `pov_radar_enabled` | 默认 false；开启时每个正式录制 pass 在 `spec_mode`/`spec_player` 后、`mirv_streams record start` 前写入一次 `csdm_radar_pov 1`（同目标重复录制也各写一次，不去重），录制起点前保证至少两秒 Demo 播放预热，并把视角切换与 seek 整体前移而不推迟录制窗口；Demo 起点不足以完成预热时生成失败且不写 JSON/不登记 take。关闭时不写命令、不新增预热；与 `pov_hud_enabled` 的 VPK/gameinfo 生命周期独立 |

除 `hide_player_avatars` 始终写入对应布尔值外，上述命令开关关闭时不写入对应命令或反向重置命令。`pov_radar_enabled` 开启时初始化序列不再写入雷达命令，改为逐 pass 在切视角后开启并按 Demo tickRate 预留至少两秒预热（`povRadarWarmupSeconds`），每个 pass 独立 sequence；关闭时保持原动作时序与 killer/整局分组。调试工作台的 `GetDebugConsolePlan` 复用同一批无副作用命令 helper 为这些设置构建显式开关/恢复命令（`r_drawskybox 1`、`mirv_sky clouds draw 1`、`hidehud 0`、`c_thirdpersonshoulder 0;cam_command 2`、`mirv_deathmsg filter clear`），恢复命令只服务调试，不改变正式 Build 的关闭行为。调试录制使用按编码参数派生的 HLAE settings 名（`DebugRecordSettingsName`；正式 Build 继续用实际 preset 名），因为同一游戏进程内 `mirv_streams settings add ffmpeg` 不覆盖同名条目。FFmpeg 探测缓存字段为 `ffmpeg_detected_preset/ffmpeg_detected_encoders/ffmpeg_detected_at`，供自动选择和编码回退使用。`GetClipSettings` / `SaveClipSettings` 响应中的 `effective_video_preset/effective_video_encoder/effective_video_status` 是只读的实际编码结果，不改变持久化的 `video_preset=auto`；前端应在探测完成后展示这些字段。`GetClipActionSettings` / `SaveClipActionSettings` 的语音配置必须与 ClipSettings 语义一致。

- 生成入口为 `GeneratePluginJSON`、`GeneratePluginJSONBatch`、`GeneratePluginJSONBatchAndLaunchHLAE`；批量请求通过 `jobs[]` 承载单 Demo 请求。请求/结果结构见 `internal/app/plugin_generate_types.go`，编排见 `plugin_generate.go` 与 `plugin_generate_launch.go`。
- 生成规划由 `internal/plugingen` 的显式输入/输出承担：`NormalizeSelectedItems`、`BuildPlan`、整局 POV 转换与历史过滤不得访问 App、Wails、磁盘配置或进程；App 在工作目录准入后冻结一次配置快照，再负责 JSON 写入、take/history 登记和可选启动编排。
- 新调用使用 `selected_items[]`；`selected_kills` 仅作兼容。`include_killer` 缺省为 true，`include_victim` 控制被害者录制。
- 普通击杀片段的前端完成投影按 Demo、kill ID、明确录制角色和实际 spec mode 索引；仅成功的 `produce_clip` killer/victim 历史参与判断，`edited_video`、整局 POV 和未知视角不吞掉待录角色，补录副本不得改写原选择。
- `primary_view=killer|victim` 表示选中玩家在击杀事件中的角色，缺省按 killer。UI 的“主视角/对方视角”须据此映射；死亡模式不能把主视角固定理解为击杀者。请求字段见 `internal/app/clip_settings.go`，兼容缺省与窗口映射见 `internal/app/plugin_generate_adapters.go`、`internal/plugingen/selection.go`、`frontend/src/shared/clip-views.ts`。
- 单片段 `clip_overrides` 支持 `killer_pre_seconds/killer_post_seconds/victim_pre_seconds/victim_post_seconds/enable_voice/enable_spec_show_xray_zero`，缺省继承全局设置。
- 整局 POV 使用独立的 `full_round_pov.player_steam_id`。每回合一个 take，早于 victim clip takes；victim-only 片段须传 `include_killer=false`。
- `PreviewFullRoundPOV` 仅解析预览，不生成文件；只保留目标至少有一次有效击杀的回合，无击杀时 segments 为空。回合起点取 `RoundStart`，有效击杀/死亡以 `RoundFreezetimeEnd` 后为准；生成录制终点为目标死亡后 1 秒，存活时为下一回合开始前 1 秒，无下一回合则回退本回合结束。预览原始 tick 与生成时补边分别见 `internal/demo/full_round_pov.go`、`internal/plugingen/full_round.go`；App 兼容委托位于 `internal/app/full_round_pov.go`。
- `take_plans[]` 的整局 POV 使用 `view=full_round_pov` 和稳定 `source_id`，附带 `round/player_name/player_steam_id/start_tick/end_tick/end_reason`。
- 击杀 take plan/history 可附带 `tick_rate`、`record_start_tick`、`record_end_tick` 和 `kill_offsets_seconds`；这些字段只描述实际录制观测与元数据，不改变原有的 Demo 事件窗口或插件命令时序。
- 批量启动的 `debug.keep_intermediate_files` 仅本会话有效，默认 false；true 时收尾只清理 `*.mux.tmp.mp4`，保留 take 视频/音频中间产物。
- `GetDebugPluginDLLOverride` / `PickDebugPluginDLLOverride` / `ClearDebugPluginDLLOverride` 返回 `active/path`；仅 debug UI 使用，会话级生效，不写配置、不参与组件版本检测，注入目标仍为 CS2 `game/csgo/plugin/bin/server.dll`。

### 制作、剪辑与清理

- `GetWorkActivity` 返回 `produce_busy/storage_busy`。制作忙碌覆盖启动、录制、合成、收尾；目录清理还须避让导入、解析、剪辑合成、导出和失败后保留的制作环境。
- 活跃制作会话禁止再次生成或启动；已结束会话才允许重试失败收尾，成功前不得重置片段状态或生成 JSON。后端须原子检查并保留文件使用权，不能仅依赖前端禁用按钮。
- HLAE 已启动而 CS2 PID 未知时保留启动器句柄和回滚状态；确认启动器退出且 CS2 枚举为空后才恢复环境。枚举失败或仍有 CS2 时保留备份供重试，不关闭无法确认归属的游戏进程。
- 录制始终部署并挂载内嵌 `internal/producegame/assets/brand.vpk`，不受设置开关控制；游戏目标为 `game/csgo/brand.vpk`，搜索路径为 `Game\tcsgo/brand.vpk`，与插件路径一起注入，先于可选 POV 路径。沿用 POV 文件生命周期：已有普通 VPK 不覆盖，仅删除本会话创建的文件；启动失败与录制收尾按环境 epoch 回滚，失败保留重试状态。入口为 `internal/app/produce_brand.go` 与 `plugin_generate_launch.go`。
- `CSDM_WS_PORT` 始终为当前 producews 会话固定的 loopback 端口；`CSDM_LOG_PATH` 指向 `<dataDir>/logs/cs2-server-plugin.log`，插件打不开时按协议回退 `csdm.log`。
- 仅制作队列成功结束后发送 `end_produce_session`（`payload.request_id`），插件以 `session_exit_ack` 确认并在游戏线程排入 quit。对应连接在有限窗口内断开属于正常收尾，不写 WS 错误或 incident；确认超时再回退 PID 关闭。跨仓库插件行为须另行核验。
- 调试会话（`internal/app/debug_session.go`、`debug_types.go`）与制作会话互斥但独立：`LaunchDebugGame` 冻结 `DebugLaunchRequest`（`inject_plugin/pov_hud_enabled/brand_enabled/pov_radar_enabled/launch_resolution`；调试页启动固定发送 `pov_radar_enabled=false`，雷达改由运行时控制台快捷指令控制，不再作为启动选项），以显式选项准备 gameinfo/plugin DLL/POV/品牌并启动 HLAE；两种模式都注入 `AfxHookSource2.dll`，`inject_plugin=false` 不要求 produce WS、不设置插件连接环境变量，并在本会话的 gameinfo 备份内临时移除残留 `csgo/plugin` 搜索路径。插件模式复用固定 `CSDM_WS_PORT`/`CSDM_LOG_PATH` 并新增 `CSDM_DEBUG_SESSION=<session_id>`；启动选项不写回配置。`GetDebugSessionState`/`StopDebugGame` 使用 `idle|starting|running|stopping|failed`，进程启动、插件连接与调试协议就绪分开报告；`plugin_ready` 只由插件针对本会话 `session_id` 的 `debug_session_ready` 握手、且该握手所属连接仍是当前连接时为真，不得把已连接报告为 ready；快照的 `plugin_state`（`disabled|waiting_connection|waiting_ready|ready|error`）是同一批后端连接/握手事实的结构化分类，供页面标签使用，不由前端按 `plugin_message` 文案猜状态。`SendDebugConsoleCommand` 复用 `producews` 传输并绑定当前连接：仅 ready 时可调用，自定义输入整体作为一个命令字符串（保留分号与引号）按序交给游戏线程，回执在游戏线程处理完整请求后回传；在途请求按发送连接绑定，连接切换立即失败旧连接请求，回执须与发送连接一致；未 ready、旧 DLL、断连与超时返回可见错误，首次 ready 超时由主动定时器更新并发射快照，超时或断连不自动重发，`debug_session_ready`/`debug_execute`/`debug_command_result` 不进入制作队列 ACK 路径。`PlayDebugDemo`（`app/debug_playback.go`）只接受当前工作目录 `demo` 子目录内、存在且可安全引用的受管控 `.dem` 文件，复用同一 ready/连接绑定发送 `debug_play_demo`；插件在游戏线程清空旧 sequences、执行带引号的 `playdemo`，绝不读取/写入/删除 `<demo>.json`，debug 模式下 `playdemo` 与 `+playdemo` 两条入口都不再加载制作序列。播放回执只表示命令已交给引擎，实际播放状态由独立 `debug_demo_state`（`{session_id,demo_path,playing,paused}`，仅启动/停止/暂停变化或观测路径变化时发送）更新 `DebugSessionState.demo_path/demo_playing/demo_paused`；`paused` 是引擎事实（插件游戏线程读 `IDemoFile::IsDemoPaused`），不得由 tick 增长或命令字符串推断，暂停属性只在播放中有效并在播放结束、会话结束、断连或连接替换时清 false；`demo_play_pending` 在发送前按 wire request_id 在准入锁内登记，成功回执不重新置位；引擎确认清除等待。确认超时由绑定 runtime/连接/request_id 的主动定时器清理并发射带可见错误的新快照；确认、发送/回执失败、断连、连接替换或停止取消对应定时器，旧回执与旧定时器不得清除新请求。等待期间拒绝再次播放，前端禁用播放及暂停/继续；协议版本 2 拒绝只报告 `tick` 的旧 DLL 并提示更新插件，不静默露出无法确认的暂停按钮。回执不置 `demo_playing`，请求校验、发送与结果提交始终绑定同一 runtime/连接，迟到回执不得写入新会话、重命名仍在播放的旧 Demo 或覆盖更新的引擎观测；传输断开或连接被替换时清为未播放，重连不重发播放命令，播放结束也不结束 debug 游戏。`OpenDemoInFolder`（`app/debug_playback.go`）复用同一受管控 `.dem` 校验（`resolveManagedDebugDemoPathIn`），除字面包含关系外还要求 `EvalSymlinks` 后的真实目标仍在真实 Demo 根内（拒绝符号链接/junction 越界，无法解析则拒绝），只打开该文件父目录，不做通用文件系统打开。调试草稿的播放命令随 `debug_play_demo` 下发：插件先初始化插件特殊命令（如 `csdm_radar_pov`），其余依赖 Demo 的命令须等引擎报告目标完整路径正在播放后应用一次；同文件重播须观测到停止或 tick 回退，旧 Demo 的正常 tick 增长不确认新播放，无快捷命令的请求也须确认；debug 播放分支不执行制作 tick 序列。`GetDebugConsolePlan`（`app/debug_settings.go`）从会话级 `DebugSettingsDraft` 构建运行时画面/声音与录制初始化命令：开关同时返回 on/off 变体，显式恢复命令（`r_drawskybox 1`、`mirv_sky clouds draw 1`、`hidehud 0`、`c_thirdpersonshoulder 0;cam_command 2`、`mirv_deathmsg filter clear`）只服务调试，正式 Build 的关闭语义不变；`apply_all` 为固定顺序，录制初始化命令复用 `RecordInitCommands`，调试输出固定为 `<dataDir>/outputs/debug`，运行中冻结会话启动分辨率，`auto` 预设由后端实际探测结果解析；草稿只读自 `GetClipSettings`，绝不写回配置，路由切换保留草稿、只有显式重载才覆盖。调试页把计划转换为可搜索、按画面/声音分组的控制台快捷指令（`frontend/src/domains/debug/debugQuickCommands.ts`）：选择只填入共用编辑框，不自动发送、不改草稿、不写正式设置；多条命令按顺序以分号连接成一个完整命令字符串交给 `SendDebugConsoleCommand`，保留原分号与引号，不做白名单。播放请求携带空快捷指令列表，播放不会用隐藏草稿值覆盖用户刚发送的雷达或其他运行时命令；雷达实际状态不另存前端，也不由自定义命令反推；调试页文字支持选中复制（仅 Debug 页局部覆盖全局 `user-select: none`）。显式应用刷新先取消预览防抖，显式应用与设置重载共享准入并串行请求，后台预览与后到的显式请求不得使等待中的应用返回过期结果。会话活跃时 `produceBusyError`/`GetWorkActivity`、目录清理、正式制作与再次启动互斥；未知 PID 保留启动器句柄与备份，确认退出后才恢复环境。两种启动入口在准备新环境前统一重试已结束 runtime 的收尾，任一保留会话未释放时拒绝本次启动；`App.Shutdown` 先停止调试会话再停 WS/Store，停止失败时不得执行共享环境的无条件恢复，切换路由不触发停止。环境准备使用启动时冻结的同一配置快照，正式制作包装入口保持原语义。
- 调试连接身份在 `internal/app/debug_command.go` 的 runtime 锁内按 `producews.GameConnID()` 核对：旧连接延迟到达的握手、协议错误与回执均不得修改当前会话状态或完成请求。插件 debug 初始化不等待 `ClientFullyConnect`，ready 必须由游戏线程命令消费路径确认；插件实现与 Windows 实机验证在外部插件仓库完成。
- 调试手动录制（`internal/app/debug_recording.go`、`debug_types.go`）首版一次一段：`StartDebugRecording`、`StopDebugRecording`、`RetryDebugRecordingMerge`、`PlayDebugDemo` 与 `StopDebugGame` 的录制收尾共用同一工作台操作锁串行执行（插件事件路径不得获取它，否则会阻塞正在等待的回执），使“准备中点击停止”“准备中切换 Demo”“多段并发重试”不能各自通过检查后再作用于过期状态；发送 start 前还须重核任务仍为 `starting`，否则不发命令；会话进入 `stopping`（`StopDebugGame`/`Shutdown` 在收尾前设置）后所有公开操作改走 `debugPublicRuntime` 封闭，连旧段合并完成后的窗口也拒绝新录制/换 Demo/控制台命令，仅内部停止通道保留。`StartDebugRecording` 要求插件 ready 且 Demo 正在播放，冻结会话启动分辨率与本段 FPS/`record_quality`/实际 preset（`auto` 由探测缓存解析），分配唯一 `<outputs>/debug/<session-id>/<recording-id>/`，复用 `RecordInitCommands` 与 `DebugRecordSettingsName` 生成初始化命令和最后的 `mirv_streams record start`；编码复用 `ffmpegprofile`/`BuildRecordEncoding`，4:3 保留真实像素拉伸。插件 `debug_record_start`/`debug_record_stop` 只在游戏线程执行，`debug_record_status`（`{session_id,recording_id,demo_path,take_name,record_phase,tick,ts_ms}`）在真正调用录制边界命令后发出，不进入正式 `record_status` take/history；桌面端仅在该事件（或插件已执行边界后的成功回执补偿）推进 `starting→recording`、`stopping→merging`，并核对当前连接、会话与 recording_id。输出路径规则固定在 `debugRecordingTakePaths`（HLAE `record name` → `<dir>/take0000.mp4` 与 `<dir>/take0000/audio.wav`，需 Windows 实机确认），不扫描 outputs 猜最新文件。合并复用 `producemerge.WaitForTakeFilesReady`/`MergeTakeVideoAudioContext` 与调试工作目录任务 context，并与新录制共用单段准入：同一时刻只能有一个录制/合并段，失败段的重试在另一段活跃时被拒绝。`StopDebugRecording` 快速返回并异步合并，失败保留源文件供 `RetryDebugRecordingMerge`（仅源文件可读时）；每次合并有独立代次，取消后仍存活的旧 goroutine 不得写入新一次重试的状态或清掉新取消句柄；成功且未保留中间文件时只清理本段已登记的原始视频/音频，绝不删除最终视频、不扫描其他会话。录制结果列表为 App 级（`DebugSessionState.recording/recordings`），随工作目录安装/重置清空，以便游戏关闭后仍能定位或重试失败段；session 与录制变更共用严格递增的 `nextDebugStamp` 时钟，快照 `updated_at_ms` 取实际反映的数据的最大戳（列表清空另有 floor 戳），使前端按 `updated_at_ms` 合并时不会因同毫秒时间戳丢弃录制事件或乱序回退；释放并清空 `debugRuntime`（`clearDebugRuntime`）时同样推进 floor，否则最终空闲快照会退回只取录制列表/旧 floor 的更旧版本，前端将拒绝该事件并继续显示已关闭的会话与 PID；录制/合并计入 `produceBusyError`/`GetWorkActivity`，合并期间拒绝再次录制与切换 Demo，普通画面命令仍可执行。`StopDebugGame` 先请求 end 并等待有界合并（超时则取消并等待旧任务退出后再允许重试）再退出游戏；断连、游戏退出、会话取消把未结束段标为失败并保留已知文件，重连不重发 start/stop；插件回调与 Windows 实机录制须另行核验。
- 状态读取：`GetProduceWSState`、`GetProduceQueueState`、`GetProduceTakeSnapshot`、`GetProduceTakeFiles`、`GetProduceHistorySnapshot`。历史的 `history_type=produce_clip|edited_video` 和 `source_label` 为可选字段，缺省按录制片段处理。
- 剪辑使用 `ProbeClipDuration` / `ConcatEditClips`，后者返回输出路径并发射 `compose_progress`；一次只允许一个合成任务，第二个活跃请求返回忙碌错误而不排队。转场时以后端 ffprobe 探测的时长和 SAR/DAR 为准；入口见 `internal/app/app_edit.go`、`internal/app/edit_ffmpeg.go`、`internal/app/edit_task.go`。
- 每次合成在 `<outputs>/edit` 下分配带随机后缀的唯一输出名（`O_EXCL` 原子预留）和任务专属临时目录：先写临时视频，验证后才 rename 提交并登记 history；提交前确认任务未取消，并校验预留路径仍是本任务创建的文件，被替换时保留原对象并拒绝覆盖。失败、超时或取消只清理本任务登记的路径，不覆盖既有视频；提交成功后即按成功登记 history，不删除已发布文件。FFprobe/FFmpeg 统一使用任务 context（探测为有限超时，合成超时与工作目录取消分别归类），取消后不进入编码器回退，并等待子进程与输出 reader 结束后才释放文件使用权。
- “对方视角快节奏剪辑”默认关闭，只由 `frontend/src/domains/edit/fastEdit.ts` 对有完整可靠录制标记的 victim take 计算可选裁剪范围；仅同 Demo、回合、击杀者且在用户序列中相邻的镜头连续，组内使用硬切，killer、full_round_pov、旧素材或缺标记素材保持整段。导出请求冻结范围并在编辑期间禁用修改；录制成功后可在视频旁写入 `.fastedit.json` 元数据，旁车失败不得使录制失败。
- `OpenProducedClipInFolder` / `ExportProduceHistoryVideos` 负责定位和导出产物。`GetOutputsStorageStats` / `OpenOutputsDirectory` / `ClearOutputsDirectory` 与 Demo 对应方法管理受管控目录；清理仅删除目标目录的直接子项，保留目录本身；统计大小包含所有文件，数量分别为 `video_count/demo_count`。
- `GetGameInfoHealth` / `RepairGameInfo` 是 gameinfo 健康状态来源，状态为 `ok|needs_repair|unknown`。修复独立成行、以制表符或空格分隔的 `Game csgo/plugin`、`Game csgo/pov.vpk` 与 `Game csgo/brand.vpk` 残留，不依赖会话备份。
- `ExportProduceWSLogs` 导出单个脱敏诊断文件，包含 WS/队列/take 快照、事件环、滚动 host 日志、incident 和插件日志尾部；无 Wails context 时写入 `<dataDir>/logs/producews-export-<timestamp>.txt`。

### 事件与其他入口

| 事件 | 来源 / 消费语义 |
| --- | --- |
| `startup_state_changed`、`download_progress`、`log` | 启动快照、下载进度、脱敏日志 |
| `produce_ws_state_changed`、`produce_queue_state_changed`、`produce_take_status_changed` | `internal/producews/service.go` 的制作状态 |
| `produce_take_file_changed`、`produce_history_changed` | `internal/app/produce_takefile.go` 的文件与历史快照 |
| `compose_progress` | `{active, percent, current_step, elapsed_ms, error}` |
| `debug_session_state_changed` | `internal/app/debug_session.go` 的调试会话快照（含 `session_id/updated_at_ms`），不进入制作队列状态 |

前端本地事件 `clip-settings-saved`、`open-produce-history` 定义于 `frontend/src/shared/events.ts`，与 Wails 事件分开维护。平台进程检查入口为 `CheckPlatformClients` / `RequestClosePlatformClient`；版本说明入口为 `GetPendingChangelog` / `AckChangelog`，首装标记逻辑见 `internal/app/app_changelog.go` 和 `internal/config/config.go`。

## 文档维护

调整目录职责、公开方法/字段、事件、状态/阶段/组件 ID、生成机制、构建命令、下载策略或日志脱敏策略时，更新本文件及受影响的子规则。长期语义按所属章节归并，避免追加“新增/更新”流水账；详细实现引用源码入口，删除已失效的路径与接口说明。
