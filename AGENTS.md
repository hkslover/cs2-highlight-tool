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
| `internal/config/` | 配置读写、默认值、兼容处理与路径规范化 |
| `internal/release/`、`internal/endpoints/`、`internal/download/` | Release 快照、下载地址策略、下载与解压 |
| `internal/demo/` | Demo 解析、击杀事实、整局 POV 计划 |
| `internal/wanmei/`、`internal/fivee/` | 对战平台查询与 Demo 下载 |
| `internal/plugingen/`、`internal/clipsjson/` | 片段过滤、历史键与插件 JSON 构建 |
| `internal/producews/` | 制作 WebSocket、队列、take 状态与诊断 |
| `internal/producegame/`、`internal/producemerge/` | 游戏环境准备与恢复、录制产物合并 |
| `internal/ffmpegprofile/`、`internal/procutil/` | 编码能力探测与回退、平台进程工具 |
| `internal/logging/`、`internal/changelog/` | 结构化日志与脱敏、内嵌版本更新说明 |
| `frontend/src/app/` | 应用壳、导航、hash 路由 |
| `frontend/src/features/` | `workspace-init`、`startup`、`import`、`clips`、`produce`、`edit`、`settings`、`ads`、`changelog` |
| `frontend/src/domains/` | 跨页面领域状态与无头业务逻辑（`clip-selection`、`demo`、`edit`、`production`、`settings`；仅 TypeScript，不放 Vue SFC） |
| `frontend/src/shared/` | 通用类型、状态、视角映射、事件常量、i18n |
| `frontend/src/shared/backend/` | 强类型 Wails 调用适配层、后端方法契约与统一边界错误 |
| `frontend/src/shared/ui/` | 跨 feature 复用的展示组件；业务页面组件仍归属对应 `features/` |
| `frontend/tests/` | Node 原生领域单元测试与 TypeScript 契约检查；运行时产物写入前端缓存目录 |
| `frontend/wailsjs/` | Wails 自动生成绑定 |
| `.github/workflows/release-windows.yml` | Windows 发布构建 |

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
- `GetStartupState`、`RunStartupChecks`、`RetryStartupComponent`、`ReinstallStartupComponent`、`CancelStartupDownload`、`OpenManualDownload`、`ImportManualDownload`、`PickCS2Path`、`EnterMainApp`、`OpenExternalURL`、`ExportStartupLogs` 的入口为 `internal/app/app_startup.go`。
- 状态源为 `GetStartupState` 和 `startup_state_changed`，模型见 `internal/envsetup/state.go`。`mode` 为 `workspace_init|startup|main`；`phase` 为 `detecting_source|waiting_source|running_tasks|ready`；二者不可混用。
- 组件 ID：`hlae`、`plugin`、`ffmpeg`、`cs2`。组件/启动状态：`pending`、`checking`、`downloading`、`installing`、`ready`、`warning`、`failed`、`needs_action`。
- 启动时实时 GeoIP 检测，统一更新源固定为 `github`，地区结果不持久化。先获取统一 Release 快照，再检查软件自身更新；确认 `self_update.available=true` 时必须先更新软件，不启动组件检查/安装。自更新检查失败保持非致命语义。
- 组件下载策略：CN 或 GeoIP 失败/为空时，`mirror_url` 与 `url` 并行竞速，先成功完成的链路胜出并取消另一条；非 CN 仅 `github_url`。竞速全部失败才报错，不恢复多源自动回退或 gh-proxy 终极兜底。统一源失败可使用本地已安装组件并标记 warning，不伪造远端成功。
- HLAE、插件本地版本均来自各自安装目录 `changelog.xml` 首个 `<version>`，不以配置持久化版本为真值。
- `StartupState.ads[]` 仅包含有效的 `main_steps_top_banner` Sponsored Card，展示字段为 `click_url/sponsor/title/rich_html/image_url/image_alt`；点击走外部浏览器，广告失败不得阻断启动。

### Demo 导入与解析

- `PickDemoFiles` 返回受管控路径 `<dataDir>/demo/raw/...`，不直接返回原始选择路径。
- `ListWanmeiRecentMatches(page)` 将小于 1 的页码归一为 1；状态为 `client_not_running|client_not_logged_in|ready`，战绩包含 `download_match_id/k4/k5/rating`。`ImportWanmeiMatch` 接受 `PVP@...`，产物为 `<dataDir>/demo/wanmei/<matchID>/<matchID>.dem`。
- `GetFiveEPlayerName` 读取持久化 `fivee_player_name`；`ListFiveERecentMatches(playerName, page)` 接受 ID 或含 `domain=<id>` 的分享链接，先规范化并保存 domain ID，页码最小为 1，战绩包含 `match_id/download_match_id/rating`。`ImportFiveEMatch` 接受 ID/URL/zip 名，产物为 `<dataDir>/demo/5e/<matchID>/<matchID>.dem`。
- `ParseDemoFile` 的 `players[]` 包含 `name/steam_id/steam_id_text/kills/deaths/assists`，不包含 `team`；`clip_players[]` 按玩家、回合、击杀组织。前端请求必须使用字符串 SteamID，不能将 JS number 的 `steam_id` 转回字符串使用。

### 设置与插件计划

设置入口为 `internal/app/clip_settings.go`，默认值与持久化在 `internal/config/config.go`，命令生成在 `internal/clipsjson/builder.go`。前端 `src/domains/settings/` 的共享草稿在工作目录切换进入 `workspace_init` 时必须调用 `resetForWorkspace`；离开该阶段后重新 `init`，旧 generation 的在途读写不得回写当前状态。

| 字段 | 稳定语义 |
| --- | --- |
| `record_quality`、`edit_quality` | `standard|high|ultra`，默认 `high`；软件编码映射 CRF，硬件编码映射 QP / `q:v` |
| `edit_fps` | `24..240`，默认 `60` |
| `video_preset` | `auto|c1|n1|a1|i1`，默认 `auto`，由后端 FFmpeg 能力探测选择 |
| `launch_resolution` | `16:9|4:3|4:3_1280x960`，默认 `4:3`；两种 4:3 为 `1440x1080`、`1280x960`，录制通过 FFmpeg `-aspect 16:9` 标记拉伸播放 |
| `record_output_dir` | Get/Save 设置时固定为 `<dataDir>/outputs` |
| `hide_all_ui` | 默认 false；开启写入 `cl_draw_only_deathnotices 1` |
| `hide_player_avatars` | 默认 false；开启写入 `cl_teamcounter_playercount_instead_of_avatars true` |
| `use_shoulder_camera` | 默认 false；开启在 `r_show_build_info 0` 前写入越肩命令 |
| `sky_blackout` | 默认 true；开启仅写入 `r_drawskybox 0` |
| `disable_clouds` | 默认 false；开启仅写入 `mirv_sky clouds draw 0`，与天空开关独立 |
| `pov_radar_enabled` | 默认 false；开启仅写入 `csdm_radar_pov 1`，与 `pov_hud_enabled` 的 VPK/gameinfo 生命周期独立 |

上述命令开关关闭时不写入对应命令或反向重置命令。FFmpeg 探测缓存字段为 `ffmpeg_detected_preset/ffmpeg_detected_encoders/ffmpeg_detected_at`，供自动选择和编码回退使用。`GetClipSettings` / `SaveClipSettings` 响应中的 `effective_video_preset/effective_video_encoder/effective_video_status` 是只读的实际编码结果，不改变持久化的 `video_preset=auto`；前端应在探测完成后展示这些字段。`GetClipActionSettings` / `SaveClipActionSettings` 的语音配置必须与 ClipSettings 语义一致。

- 生成入口为 `GeneratePluginJSON`、`GeneratePluginJSONBatch`、`GeneratePluginJSONBatchAndLaunchHLAE`；批量请求通过 `jobs[]` 承载单 Demo 请求。请求/结果结构见 `internal/app/plugin_generate.go`。
- 新调用使用 `selected_items[]`；`selected_kills` 仅作兼容。`include_killer` 缺省为 true，`include_victim` 控制被害者录制。
- `primary_view=killer|victim` 表示选中玩家在击杀事件中的角色，缺省按 killer。UI 的“主视角/对方视角”须据此映射；死亡模式不能把主视角固定理解为击杀者。字段缺省的兼容和窗口映射见 `clip_settings.go`、`plugin_generate.go`、`frontend/src/shared/clip-views.ts`。
- 单片段 `clip_overrides` 支持 `killer_pre_seconds/killer_post_seconds/victim_pre_seconds/victim_post_seconds/enable_voice/enable_spec_show_xray_zero`，缺省继承全局设置。
- 整局 POV 使用独立的 `full_round_pov.player_steam_id`。每回合一个 take，早于 victim clip takes；victim-only 片段须传 `include_killer=false`。
- `PreviewFullRoundPOV` 仅解析预览，不生成文件；只保留目标至少有一次有效击杀的回合，无击杀时 segments 为空。回合起点取 `RoundStart`，有效击杀/死亡以 `RoundFreezetimeEnd` 后为准；生成录制终点为目标死亡后 1 秒，存活时为下一回合开始前 1 秒，无下一回合则回退本回合结束。预览原始 tick 与生成时补边分别见 `internal/demo/full_round_pov.go`、`internal/app/full_round_pov.go`。
- `take_plans[]` 的整局 POV 使用 `view=full_round_pov` 和稳定 `source_id`，附带 `round/player_name/player_steam_id/start_tick/end_tick/end_reason`。
- 击杀 take plan/history 可附带 `tick_rate`、`record_start_tick`、`record_end_tick` 和 `kill_offsets_seconds`；这些字段只描述实际录制观测与元数据，不改变原有的 Demo 事件窗口或插件命令时序。
- 批量启动的 `debug.keep_intermediate_files` 仅本会话有效，默认 false；true 时收尾只清理 `*.mux.tmp.mp4`，保留 take 视频/音频中间产物。
- `GetDebugPluginDLLOverride` / `PickDebugPluginDLLOverride` / `ClearDebugPluginDLLOverride` 返回 `active/path`；仅 debug UI 使用，会话级生效，不写配置、不参与组件版本检测，注入目标仍为 CS2 `game/csgo/plugin/bin/server.dll`。

### 制作、剪辑与清理

- `GetWorkActivity` 返回 `produce_busy/storage_busy`。制作忙碌覆盖启动、录制、合成、收尾；目录清理还须避让导入、解析、剪辑合成、导出和失败后保留的制作环境。
- 活跃制作会话禁止再次生成或启动；已结束会话才允许重试失败收尾，成功前不得重置片段状态或生成 JSON。后端须原子检查并保留文件使用权，不能仅依赖前端禁用按钮。
- HLAE 已启动而 CS2 PID 未知时保留启动器句柄和回滚状态；确认启动器退出且 CS2 枚举为空后才恢复环境。枚举失败或仍有 CS2 时保留备份供重试，不关闭无法确认归属的游戏进程。
- `CSDM_WS_PORT` 始终为当前 producews 会话固定的 loopback 端口；`CSDM_LOG_PATH` 指向 `<dataDir>/logs/cs2-server-plugin.log`，插件打不开时按协议回退 `csdm.log`。
- 仅制作队列成功结束后发送 `end_produce_session`（`payload.request_id`），插件以 `session_exit_ack` 确认并在游戏线程排入 quit。对应连接在有限窗口内断开属于正常收尾，不写 WS 错误或 incident；确认超时再回退 PID 关闭。跨仓库插件行为须另行核验。
- 状态读取：`GetProduceWSState`、`GetProduceQueueState`、`GetProduceTakeSnapshot`、`GetProduceTakeFiles`、`GetProduceHistorySnapshot`。历史的 `history_type=produce_clip|edited_video` 和 `source_label` 为可选字段，缺省按录制片段处理。
- 剪辑使用 `ProbeClipDuration` / `ConcatEditClips`，后者返回输出路径并发射 `compose_progress`。转场时以后端 ffprobe 探测的时长和 SAR/DAR 为准；入口见 `internal/app/app_edit.go`、`internal/app/edit_ffmpeg.go`。
- “对方视角快节奏剪辑”默认关闭，只由 `frontend/src/domains/edit/fastEdit.ts` 对有完整可靠录制标记的 victim take 计算可选裁剪范围；仅同 Demo、回合、击杀者且在用户序列中相邻的镜头连续，组内使用硬切，killer、full_round_pov、旧素材或缺标记素材保持整段。导出请求冻结范围并在编辑期间禁用修改；录制成功后可在视频旁写入 `.fastedit.json` 元数据，旁车失败不得使录制失败。
- `OpenProducedClipInFolder` / `ExportProduceHistoryVideos` 负责定位和导出产物。`GetOutputsStorageStats` / `OpenOutputsDirectory` / `ClearOutputsDirectory` 与 Demo 对应方法管理受管控目录；清理仅删除目标目录的直接子项，保留目录本身；统计大小包含所有文件，数量分别为 `video_count/demo_count`。
- `GetGameInfoHealth` / `RepairGameInfo` 是 gameinfo 健康状态来源，状态为 `ok|needs_repair|unknown`。修复独立成行的 `Game\tcsgo/plugin` 或 `Game csgo/plugin` 残留，不依赖会话备份。
- `ExportProduceWSLogs` 导出单个脱敏诊断文件，包含 WS/队列/take 快照、事件环、滚动 host 日志、incident 和插件日志尾部；无 Wails context 时写入 `<dataDir>/logs/producews-export-<timestamp>.txt`。

### 事件与其他入口

| 事件 | 来源 / 消费语义 |
| --- | --- |
| `startup_state_changed`、`download_progress`、`log` | 启动快照、下载进度、脱敏日志 |
| `produce_ws_state_changed`、`produce_queue_state_changed`、`produce_take_status_changed` | `internal/producews/service.go` 的制作状态 |
| `produce_take_file_changed`、`produce_history_changed` | `internal/app/produce_takefile.go` 的文件与历史快照 |
| `compose_progress` | `{active, percent, current_step, elapsed_ms, error}` |

前端本地事件 `clip-settings-saved`、`open-produce-history` 定义于 `frontend/src/shared/events.ts`，与 Wails 事件分开维护。平台进程检查入口为 `CheckPlatformClients` / `RequestClosePlatformClient`；版本说明入口为 `GetPendingChangelog` / `AckChangelog`，首装标记逻辑见 `internal/app/app_changelog.go` 和 `internal/config/config.go`。

## 文档维护

调整目录职责、公开方法/字段、事件、状态/阶段/组件 ID、生成机制、构建命令、下载策略或日志脱敏策略时，更新本文件及受影响的子规则。长期语义按所属章节归并，避免追加“新增/更新”流水账；详细实现引用源码入口，删除已失效的路径与接口说明。
