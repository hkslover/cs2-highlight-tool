# AGENTS.md

本文件是本仓库的唯一 AI 主记忆文件（Single Source of Truth）。

## 项目概览
- 技术栈：`Wails v2 + Go + Vue 3 + TypeScript + Naive UI + vue-router@4`。
- 目标：启动时完成环境准备（统一更新源检查、自更新检测、组件检查/安装），成功后进入主页面。
- 目录分层：
- 根级规则：本文件（全局规则）。
- 子目录规则：`internal/AGENTS.md`、`frontend/AGENTS.md`（更具体，优先级更高）。
- 约束策略：严格执行（Must / Must Not / Required Checks）。

## 代码地图
- `main.go`：Wails 入口；`--apply-update` 时进入 updater 流程。
- `internal/app`：Wails 绑定层（UI 可调用方法入口）。
- `internal/appdata`：应用数据根目录解析与旧版 exe 同目录数据迁移。
- `internal/envsetup`：启动状态机、组件检查、统一更新源快照消费、日志导出（按 `service_*.go` 进行职责拆分）。
- `internal/config`：`config.json` 读写、默认值、路径规范化。
- `internal/release`：Release API 获取与资产选择。
- `internal/endpoints`：下载源与手动下载链接配置。
- `internal/download`：下载与解压/替换目录工具。
- `internal/logging`：启动链路结构化日志适配（`log/slog` + 脱敏）。
- `internal/demo`：Demo 文件解析（demoinfocs-golang）。
- `internal/updater`：自更新应用逻辑。
- `frontend/src/app`：应用壳、顶部栏、主流程容器与路由装配。
- `frontend/src/features`：按业务域组织页面与组件（`startup`、`import`、`clips`、`produce`）。
- `frontend/src/shared`：跨功能通用能力（`i18n`、共享类型）。
- `frontend/wailsjs`：自动生成的 Go 绑定代码。
- `docs/ARCHITECTURE.md`：目录职责、依赖方向与常见改动入口说明。
- `tools`：本地 mock release API 与调试脚本。

## 运行/构建/测试命令
- 后端测试：`go test ./...`
- 前端构建校验：`cd frontend && npm run build`
- 本地开发：`wails dev`
- 产物构建：`wails build`

## 全局硬规则
- Must：开始任何改动前先阅读本文件与目标子目录 `AGENTS.md`。
- Must：遵守分层边界，`internal/app` 是 Wails 暴露边界，不把 UI 逻辑下沉到低层包。
- Must：改动完成后执行对应 Required Checks，并在汇报中给出结果。
- Must：涉及状态字段、事件名、接口签名变更时，同步更新本文件及相关子规则文件。
- Must Not：手工修改自动生成文件：`frontend/wailsjs/**`、`frontend/src/auto-imports.d.ts`、`frontend/src/components.d.ts`。
- Must Not：在未明确需求时引入与现有状态机冲突的新状态名/阶段名/组件 ID。
- Must Not：恢复或重新引入 `PROJECT_STRUCTURE.md`，除非用户明确要求。
- Must Not：在仓库内并行维护 `CLAUDE.md` 作为主规则文件。
- Must：i18n 变更时只需提供 `zh-CN.json`，`en-US.json` 由用户自行维护。

## 稳定契约（Public Interfaces）
- Wails 暴露方法（`internal/app.App`）：
- `GetStartupState`
- `GetStartupState` 返回值语义新增：`ads[]`（仅包含 `placement=main_steps_top_banner` 的有效 Sponsored Card 广告；字段 `click_url/sponsor/title/rich_html/image_url/image_alt`，点击统一走外部浏览器）
- `RunStartupChecks`
- `RetryStartupComponent`
- `ReinstallStartupComponent`
- `OpenManualDownload`
- `OpenExternalURL`
- `ImportManualDownload`
- `PickCS2Path`
- `EnterMainApp`
- `CancelStartupDownload(componentID string)` StartupState
- `ApplySelfUpdate`
- `ExportStartupLogs`
- `PickDemoFiles`
- `PickDemoFiles` 返回值语义：返回位于 `<dataDir>/demo/raw/...` 的受管控 Demo 路径（非原始选择路径）
- `ListWanmeiRecentMatches(page)`
- `ListWanmeiRecentMatches(page)` 返回值语义：`page<1` 自动按 `1` 处理；`status=client_not_running/client_not_logged_in/ready`；`ready` 时返回最近战绩列表（含 `download_match_id`、`k4`、`k5`、`rating`）
- `ImportWanmeiMatch`
- `ImportWanmeiMatch` 返回值语义：输入 `matchID`（支持 `PVP@...`），下载并解压后返回位于 `<dataDir>/demo/wanmei/<matchID>/<matchID>.dem` 的受管控 Demo 路径
- `GetFiveEPlayerName`
- `GetFiveEPlayerName` 返回值语义：返回 `config.json` 中持久化的 5E 查询 ID（`fivee_player_name`，无值时返回空字符串）
- `ListFiveERecentMatches(playerName, page)`
- `ListFiveERecentMatches(playerName, page)` 返回值语义：输入玩家 ID 或包含 `domain=<id>` 的 5E 个人主页分享链接与分页参数，`page<1` 自动按 `1` 处理；先规范化并持久化 `fivee_player_name`（保存提取后的 domain ID），再返回该玩家最近 5E 战绩列表（含 `match_id/download_match_id/rating`）
- `ImportFiveEMatch`
- `ImportFiveEMatch` 返回值语义：输入 `matchID`（支持原始 ID/URL/zip 名），通过 5E match 接口解析 `demo_url` 下载并解压，返回位于 `<dataDir>/demo/5e/<matchID>/<matchID>.dem` 的受管控 Demo 路径
- `ParseDemoFile`
- `ParseDemoFile` 返回的 `players[]` 字段：`name` `steam_id` `kills` `deaths` `assists`（不包含 `team`）
- `ParseDemoFile` 额外返回 `clip_players[]`（按玩家 -> 回合 -> 击杀明细，供 clips/produce 使用）
- `GetClipSettings`
- `SaveClipSettings`
- `GetClipSettings` / `SaveClipSettings` 字段约定新增：`edit_fps`（范围 `24..240`，默认 `60`）与 `edit_quality`（`standard|high|ultra`，默认 `high`）
- `GetClipSettings` / `SaveClipSettings` 字段约定新增：`record_quality`（`standard|high|ultra`，默认 `high`；软件编码映射到 `crf`，硬件编码映射到 `qp` / `q:v`）
- `GetClipSettings` / `SaveClipSettings` 字段约定更新：`video_preset` 取值 `auto|c1|n1|a1|i1`（默认 `auto`；`auto` 表示按 FFmpeg 探测能力自动选择）
- `GetClipSettings` / `SaveClipSettings` 字段约定更新：`launch_resolution` 取值 `16:9|4:3|4:3_1280x960`（默认 `4:3`；`4:3` 表示 `1440x1080`，`4:3_1280x960` 表示 `1280x960`）
- `GetClipSettings` / `SaveClipSettings` 字段约定新增：`hide_all_ui`（默认 `false`；开启时生成插件 JSON bootstrap 写入 `cl_draw_only_deathnotices 1`，关闭时不写入该命令）
- `GeneratePluginJSON`
- `GeneratePluginJSON` 支持可选参数 `record_victim_view`（开启后按片段生成“击杀者视角 -> 被害者视角”连续序列）
- `GeneratePluginJSON` 支持可选参数 `victim_view_mode`：`batch`（先击杀者后逐个被害者）/`interleaved`（击杀者与被害者交替）
- `GeneratePluginJSON` 支持可选参数 `record_material_movies`（开启后每段 pass 用 `mirv_startmovie <name>` / `mirv_endmovie` 包裹，按命名约定 `clip_<order>_<role>` 录出独立素材）
- `GeneratePluginJSON` / `GeneratePluginJSONBatchAndLaunchHLAE` 的 `selected_items[]` 支持可选 `clip_overrides`：`killer_pre_seconds` `killer_post_seconds` `victim_pre_seconds` `victim_post_seconds` `enable_voice` `enable_spec_show_xray_zero`（字段缺省表示继承全局设置）
- `GeneratePluginJSONBatchAndLaunchHLAE` 支持可选参数 `debug.keep_intermediate_files`（`true` 时录制会话结束仅清理 `*.mux.tmp.mp4`，保留 take 视频/音频中间产物；默认 `false`）
- `GenerateMaterialPluginJSON`（等同 `GeneratePluginJSON` + `record_material_movies=true` + 默认 `mode=material`，供素材模式/剪辑模式阶段1 使用）
- `GetClipMode` / `SaveClipMode`（持久化 `clip_mode`，取值 `material` / `edit`）
- `SaveClipProject` / `LoadClipProject`（`<dataDir>/projects/<demo_basename>.clipproject.json` 读写）
- `PickMaterialDirectory`（打开目录选择器）
- `ScanMaterialClips`（扫描 `clip_<order>_<role>.<mp4|mov|mkv|avi>` 素材）
- `AutoMatchMaterials`（按 order+role 回填 `TimelineItem.MaterialKillerFile` / `MaterialVictimFile`）
- `ComposeClipProject` / `CancelCompose`（调用 ffmpeg 合成，异步发射 `compose_progress`）
- `GetOutputsStorageStats`（递归统计 `<dataDir>/outputs`，返回 `output_dir`、视频文件数量 `video_count`、所有文件总字节数 `total_size_bytes`）
- `OpenOutputsDirectory`（确保 `<dataDir>/outputs` 存在并打开目录位置）
- `ClearOutputsDirectory`（删除 `<dataDir>/outputs` 下所有直接子项，保留 outputs 目录本身，并返回清理后的统计）
- `GetDemoStorageStats`（递归统计 `<dataDir>/demo`，返回 `demo_dir`、Demo 文件数量 `demo_count`、所有文件总字节数 `total_size_bytes`）
- `OpenDemoDirectory`（确保 `<dataDir>/demo` 存在并打开目录位置）
- `ClearDemoDirectory`（删除 `<dataDir>/demo` 下所有直接子项，保留 demo 目录本身，并返回清理后的统计）
- `GetGameInfoStatus`（读取当前 CS2 `gameinfo.gi` 状态，返回 `status=normal|abnormal|unavailable`、`clean`、`has_backup`、`can_repair`、`gameinfo_path`、`backup_path`、`message`；`abnormal` 表示当前文件包含 `csgo/plugin`）
- `RepairGameInfoFromBackup`（仅使用 `<dataDir>/backups/gameinfo/gameinfo.gi` 中不包含 `csgo/plugin` 的原版备份恢复当前 CS2 `gameinfo.gi`，恢复后返回最新状态）
- `GetProduceHistorySnapshot` 返回的 `items[]` 新增可选字段：`history_type=produce_clip|edited_video`、`source_label`（用于区分录制片段与剪辑成片来源）
- `config.json` 新增持久化字段：`fivee_player_name`（5E 导入页查询 ID 缓存，保存 5E domain ID）
- `config.json` 新增持久化字段：`record_quality`（录制质量，取值 `standard|high|ultra`，默认 `high`）
- `config.json` 新增持久化字段：`ffmpeg_detected_preset`、`ffmpeg_detected_encoders[]`、`ffmpeg_detected_at`（启动阶段 FFmpeg 能力探测缓存，供 `video_preset=auto` 与编码回退使用）
- `config.json` 新增持久化字段：`hide_all_ui`（隐藏所有 UI，默认 `false`）
- 应用数据根目录约定：Windows 默认 `<dataDir>=%LOCALAPPDATA%/CS2 Highlight Tool`；`config.json`、组件目录、demo、projects、outputs、temp、updates、logs 均位于 `<dataDir>`。`<exeDir>` 仅用于定位当前程序本体与自更新替换目标。
- CS2 `gameinfo.gi` 原版备份约定：工具启动或录制注入前，只在当前 `gameinfo.gi` 不包含 `csgo/plugin` 时写入 `<dataDir>/backups/gameinfo/gameinfo.gi`；不得用包含 `csgo/plugin` 的文件覆盖该长期备份。
- 关键事件名（前后端协作契约）：
- `startup_state_changed`
- `download_progress`
- `log`
- `compose_progress`（剪辑模式 ffmpeg 合成进度：`{active, percent, current_step, elapsed_ms, error}`）
- 关键状态枚举（约定，不可随意破坏）：
- 通用状态：`pending` `checking` `downloading` `installing` `ready` `warning` `failed` `needs_action`
- 阶段：`detecting_source` `waiting_source` `running_tasks` `ready`
- HLAE 本地版本来源约定：
- `steps[].local_version`（`id=hlae`）由安装目录 `changelog.xml` 的首个 `<version>` 解析，不以 `config.json` 持久化字段为准。
- 插件本地版本来源约定：
- `steps[].local_version`（`id=plugin`）由安装目录 `changelog.xml` 的首个 `<version>` 解析，不以 `config.json` 持久化字段为准。
- 统一更新源选择约定：
- 启动时实时 GeoIP 获取 `country_code`；统一更新源固定为 `github`（地区结果不持久化）。
- 自更新优先约定：
- 启动时统一 Release 快照获取完成后，必须先检查软件自身版本；若 `self_update.available=true`，不得启动 HLAE、插件、FFmpeg、CS2 组件检查/安装流程，用户必须先完成软件更新。
- 自更新检查失败不等同于发现新版本，保持非致命语义并继续后续组件检查。
- 组件下载回退顺序：`country_code=CN` 时 `url -> mirror_url`；非 CN 时仅 `github_url`；GeoIP 检测失败或 `country_code` 为空时默认 `url -> mirror_url`；失败后直接报错（不再做 gh-proxy 终极兜底）。

## 变更前检查
- 确认改动范围属于 `internal/`、`frontend/` 或两者。
- 确认是否触及稳定契约（方法名/事件名/状态枚举/阶段枚举）。
- 确认是否涉及自动生成文件；如涉及，改生成源，不改生成结果。
- 确认需要执行的最低验证命令集合（见下节）。

## 变更后验证
- Required（后端相关改动）：`go test ./...`
- Required（前端相关改动）：`cd frontend && npm run build`
- Required（同时改前后端）：两条命令都执行。
- Required（涉及状态机/回退/日志脱敏）：至少关注 `internal/envsetup` 与 `internal/release` 测试是否通过。

## 维护触发器
以下场景发生时，必须同步更新 AGENTS 文档（根或子规则）：
- 目录结构或模块职责调整。
- 新增/重命名状态字段、状态枚举、阶段枚举、组件 ID。
- 新增/重命名事件名或 Wails 暴露方法名。
- 运行/构建/测试命令变化。
- 自动生成文件路径或生成机制变化。
- 下载源策略、回退策略、日志脱敏策略变化。
- 日志后端实现约束变化（例如 `slog` 适配层或字段脱敏规则调整）。

维护验收演练（模板，提交前自检一次）：
- 假设新增状态字段 `source_step.latency_ms`：
- 更新后端 `StartupState` 定义与赋值逻辑。
- 更新前端类型定义与消费逻辑（如展示或忽略策略）。
- 检查是否需要补充测试断言。
- 更新 `AGENTS.md` 中“稳定契约”与触发器说明（如果字段属于长期契约）。
- 假设新增事件名 `startup_health_changed`：
- 后端发射事件与前端订阅逻辑同步落地。
- 更新 `frontend/AGENTS.md` 的“状态来源约束”。
- 运行 Required Checks 并在交付说明中列出事件契约变更。

## 参考规范
- OpenAI Codex AGENTS 指南：`https://developers.openai.com/codex/guides/agents-md`
- Claude 记忆实践：`https://code.claude.com/docs/en/memory`
- GitHub Copilot 仓库/路径级指令：`https://docs.github.com/en/copilot/how-tos/configure-custom-instructions/add-repository-instructions`
- AGENTS.md 开放格式：`https://agents.md/index`
