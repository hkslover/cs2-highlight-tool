# AGENTS.md（frontend）

本文件作用域：`frontend/**`。  
与根级 `AGENTS.md` 同时生效；如冲突，以本文件（更具体作用域）为准。

## 前端目录约定
- `src/app/`：应用壳、主流程容器、顶部栏、路由装配。
- `src/features/`：按业务域组织页面与组件（`startup`、`import`、`clips`、`produce`）。
- `src/shared/`：通用类型与 i18n。

## 路由约定
- 主界面三步骤路由：`/import`、`/clips`、`/produce`。
- 导入页子路由：`/import`（三按钮入口）、`/import/wanmei`、`/import/5e`；文件导入在 `/import` 入口按钮直接触发，不占用子路由。
- 路由入口：`src/app/router.ts`（hash 模式）。
- `MainApp.vue` 通过 `n-steps` 展示三步骤导航，通过 `<router-view />` 渲染当前页面。
- `ImportPage.vue` 内嵌第二层 `<router-view />` 用于导入子页面切换。

## i18n 约定
- Must：变更 i18n 时只需提供 `zh-CN.json`，`en-US.json` 由用户自行维护。
- Must Not：自行修改或生成 `en-US.json`。

## 状态来源约束
- Must：前端启动状态以 `GetStartupState` + 事件流为单一事实来源（Single Source of Truth）。
- Must：持续监听并正确消费事件：
- `startup_state_changed`
- `download_progress`
- Must：保持 `StartupState` / `ProgressMessage` 字段与后端模型语义一致。
- Must：`StartupState.ads[]` 仅渲染 `placement=main_steps_top_banner` Sponsored Card 广告位（使用 `click_url/sponsor/title/rich_html/image_url/image_alt`），不得在导入方式卡片区混入广告入口。
- Must：消费 `GetProduceHistorySnapshot` 时保持 `ProduceHistoryItem.history_type`（`produce_clip|edited_video`）与 `source_label` 的向后兼容（缺省按 `produce_clip` 处理）。
- Must：`gameinfo.gi` 健康状态以 `GetGameInfoHealth` / `RepairGameInfo` 返回值为来源；前端不得自行读取或推断 `gameinfo.gi` 文件内容。
- Must：`ClipSettings.video_preset` 需与后端保持一致，允许值 `auto|c1|n1|a1|i1`（`auto` 代表使用后端探测到的 FFmpeg 能力自动选择编码）。
- Must：`ClipSettings.record_quality` 需与后端保持一致，允许值 `standard|high|ultra`（默认 `high`；软件编码映射到 CRF，硬件编码映射到 QP / `q:v`）。
- Must：`ClipSettings.launch_resolution` 需与后端保持一致，允许值 `16:9|4:3|4:3_1280x960`（`4:3` 代表 `1440x1080`，`4:3_1280x960` 代表 `1280x960`；两种 4:3 录制输出均由后端 FFmpeg 参数标记为 16:9 stretched playback）。
- Must：`ClipSettings.hide_all_ui` 需与后端保持一致，默认 `false`；开启时生成插件 JSON bootstrap 写入 `cl_draw_only_deathnotices 1`，关闭时不写入该命令。
- Must：`ClipSettings.use_shoulder_camera` 需与后端保持一致，默认 `false`；开启时生成插件 JSON bootstrap 在 `r_show_build_info 0` 前写入越肩视角命令，关闭时不写入该命令。
- Must：`ClipSettings.sky_blackout` 需与后端保持一致，默认 `true`；开启时仅写入 `r_drawskybox 0`，不得联动关闭云层。
- Must：`ClipSettings.disable_clouds` 需与后端保持一致，默认 `false`；开启时仅写入 `mirv_sky clouds draw 0`，关闭时不写入该命令。
- Must：`ClipSettings.pov_radar_enabled` 需与后端保持一致，默认 `false`；开启时生成插件 JSON bootstrap 写入 `csdm_radar_pov 1`，关闭时不写入该命令。
- Must：`ClipSettings.hide_player_avatars` 需与后端保持一致，默认 `false`；开启时生成插件 JSON bootstrap 写入 `cl_teamcounter_playercount_instead_of_avatars true`，关闭时不写入该命令。
- Must：整局 POV 录制状态必须作为 demo 级独立状态维护，不得伪装成普通击杀片段；生成请求通过 `full_round_pov.player_steam_id` 传递，普通 victim clip 仍通过 `selected_items[]` 传递。
- Must：全玩家选择器需要精确 SteamID 时使用 `DemoPlayerInfo.steam_id_text`，不得使用 JS number 形式的 `steam_id` 作为后端请求值。
- Must：整局 POV 模式下新增的 victim-only `selected_items[]` 必须传 `include_killer=false`，避免额外生成击杀者片段。
- Must：调用 `GeneratePluginJSONBatchAndLaunchHLAE` 时允许传递可选 `debug.keep_intermediate_files`，用于控制是否保留录制中间产物（仅会话级生效）。
- Must：debug 插件 DLL override 以 `GetDebugPluginDLLOverride` / `PickDebugPluginDLLOverride` / `ClearDebugPluginDLLOverride` 为单一事实来源；前端只在 debug 设置分组中展示，不写入本地持久化状态，也不得混入普通设置分组。
- Must Not：在前端新增与后端冲突的“本地自定义状态枚举”替代后端状态。

## UI 状态映射
- Must：状态文本/tag/progress 映射与状态枚举保持一致：
- `pending` `checking` `downloading` `installing` `ready` `warning` `failed` `needs_action`
- Must：`self_update` 的状态归一化逻辑保持可解释且与后端状态兼容。
- Must：仅在活跃状态（如 checking/downloading/installing）展示进度条。
- Must Not：变更状态映射而不同时更新文档与验证步骤。

## 交互行为约束
- Must：后端调用边界保持在 `window.go.app.App.*`（由 Wails 生成绑定定义）。
- Must：涉及按钮可用性逻辑时，保持与 `running/self_update/can_enter_main` 语义一致。
- Must：保留 `window.go` 未加载时的防御逻辑（避免静默失败）。
- Must：优先使用 `@/` 别名引用 `src/**` 模块，减少深层相对路径。
- Must：在 `strict` TypeScript 模式下，Vue 模板事件回调不得依赖匿名参数隐式推断（如 `@update:xxx="(v) => ..."`）；应使用具名处理函数并声明参数类型，或在表达式中显式标注类型，避免 `TS7006`（Windows 构建常见触发）。
- Must：Vue API（`ref`, `computed`, `watch`, `nextTick`, `onMounted` 等）必须在 `<script setup>` 中通过 `import { ... } from "vue"` 显式导入，不得依赖 `unplugin-auto-import` 的全局声明——Windows 构建可能无法解析全局声明导致 `TS2304`（如 `Cannot find name 'nextTick'`）。
- Must Not：手工编辑自动生成文件：`frontend/wailsjs/**`。
- Must Not：重命名关键事件名或方法名而不联动后端与文档。

## 前端变更必测
- Required：执行 `cd frontend && npm run build`
- Required（涉及前后端契约变更）：同时执行 `go test ./...` 并确认前端类型/调用无断裂。
- Required（涉及状态展示与按钮策略）：至少手动核对一次启动向导关键路径：
- 自动启动检查 -> 组件状态变化 -> 可重试/可导入按钮显示 -> 进入主页面 gating
- Required（涉及主界面路由/视图变更）：确认三步骤导航可点击切换、导入子页面返回按钮正常工作。
