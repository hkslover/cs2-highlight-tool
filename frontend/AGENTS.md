# AGENTS.md（前端）

作用域：`frontend/**`。与根级 `AGENTS.md` 同时生效；跨层字段、事件和默认值统一见根文件。本文件补充前端组织、状态消费与交互约束。

## 目录与路由

- `src/app/`：应用壳、导航和路由；`src/features/`：按业务域组织页面、组件和 composables；`src/domains/`：跨页面复用的领域状态与无头业务逻辑（仅 TypeScript，不放 Vue SFC）；`src/shared/`：通用类型、状态、i18n 和工具。
- `src/shared/backend/` 是唯一的强类型 Wails 方法调用适配层，`src/shared/ui/` 放跨 feature 复用的展示组件；业务页面组件放在对应 `src/features/*/components/`。
- `tests/` 包含 Node 原生领域单元测试和 TypeScript 契约检查；不把测试运行时产物提交到仓库。
- 路由入口为 `src/app/router.ts`，使用 hash 模式。导入、片段、制作三步骤为 `/import`、`/clips`、`/produce`；另有 `/edit`、`/settings`。
- `/import` 是导入方式入口，子路由为 `/import/wanmei`、`/import/5e`；本地文件选择由入口按钮触发，不增加文件导入子路由。`ImportPage.vue` 通过嵌套 router-view 渲染子页面。
- 工作目录初始化与启动向导由应用状态控制，不能擅自改成与后端 mode 冲突的本地阶段。
- 优先使用 `@/` 引用 `src/**`，减少深层相对路径。

## 类型、绑定与 i18n

- 后端调用边界保持为 `window.go.app.App.*`，由 `src/shared/backend/` 的强类型适配层统一访问；保留 `window.go` 尚未加载时的防御逻辑和显式错误，避免静默失败。新增或重构的业务调用不得自定义 `callBackend` 或使用 `any` 绕过契约。
- 手写类型入口为 `src/shared/types/` 与 `src/global.d.ts`；与后端模型保持语义一致，不用本地自定义枚举替代后端状态。
- 不得手工编辑 `wailsjs/**`、`src/auto-imports.d.ts`、`src/components.d.ts`；调整生成源并通过项目生成流程更新。
- Vue API（ref、computed、watch、nextTick、onMounted 等）须在 script setup 中从 `vue` 显式导入，不依赖自动导入的全局声明，避免 Windows 构建 TS2304。
- strict TypeScript 下，模板回调使用具名且显式声明参数类型的函数，或在表达式中显式标注类型，避免 TS7006。
- Node 原生 ESM 直接加载的领域模块，其相对导入须匹配编译后的 `.js` 路径；仅由 Vite 解析的模块可按现有 alias/扩展名规范书写，不为表面一致性机械改动。
- i18n 默认只修改 `src/shared/i18n/zh-CN.json`，不自行生成或修改 `en-US.json`；用户明确要求同步英文时按该要求执行。

## 启动与展示状态

- 启动状态以 `GetStartupState` 加事件流为单一事实来源，持续正确消费 `startup_state_changed`、`download_progress`，保持 `StartupState/ProgressMessage` 与后端一致。
- `workspace_init` 消费工作目录初始化接口；已初始化与否由后端返回，目录合法性由 `ValidateWorkspaceDir` 校验。
- 状态文本、tag、进度及按钮可用性须匹配根文件枚举和 `running/self_update/can_enter_main` 语义；只在 checking/downloading/installing 等活跃状态展示进度条。
- `self_update` 归一化逻辑须可解释且兼容后端状态；改变状态展示时同步文档和验证步骤。
- 广告只渲染 `main_steps_top_banner`，保留现有 HTML 净化流程，点击走外部浏览器，不在导入方式卡片区混入广告入口。
- gameinfo 健康状态以 `GetGameInfoHealth` / `RepairGameInfo` 为来源，不自行读文件或推断状态。

## 片段与制作

- `src/domains/clip-selection/` 维护普通片段选择、筛选器和整局 POV 的独立状态；`src/domains/demo/` 维护 Demo 列表/解析状态，页面组件不得把整局 POV 塞回 Demo 元数据列表。
- `src/domains/production/` 先建立事件订阅再读取初始快照，并按 `updated_at_ms` 与来源优先级合并；事件不能被延迟快照回滚。初始化/重试/销毁必须沿用其生命周期 API。
- `src/domains/settings/` 是跨设置入口共享的草稿与确认状态，保存防抖、`flush` 和 `dispose` 必须等待在途读写；读取失败不得把前端占位默认值写回后端。工作目录切换进入 `workspace_init` 时调用 `resetForWorkspace`，离开该阶段后由 `init` 读取新工作目录；旧 generation 的在途读写不得回写当前状态。
- `src/domains/edit/` 管理剪辑序列和合成进度；路由/应用生命周期切换时必须调用 `init`/`dispose`，旧 epoch 的导出完成或事件不得污染新实例。
- ClipSettings 的允许值、默认值、命令开关语义遵循根文件“设置与插件计划”；前端不得自行决定编码能力或绕过后端归一化。
- 精确 SteamID 使用 `steam_id_text` 或对应字符串字段；不得将 number 形式的 `steam_id` 作为请求值。
- 主视角/对方视角通过 `primary_view` 映射，复用 `src/shared/clip-views.ts`；死亡模式的主视角是 victim，录制角色仍是 killer/victim。
- 整局 POV 是 Demo 级独立状态，不伪装成普通击杀片段。请求通过 `full_round_pov.player_steam_id` 传递，普通片段通过 `selected_items[]` 传递；victim-only 项传 `include_killer=false`。
- `PreviewFullRoundPOV` 的 segments 为空时显示空态并阻止生成，不能为零击杀玩家伪造可录制回合。
- 制作状态消费对应快照和事件，名称见根文件；历史 `history_type/source_label` 保持向后兼容，缺省按 `produce_clip` 处理。
- 制作按钮、设置页清理按钮使用 `src/shared/state/useWorkActivity.ts` 和 `GetWorkActivity` 的状态；未加载或查询失败时禁用。制作忙碌包含收尾，不能仅看录制是否结束。
- debug DLL override 仅在 debug 设置组展示，以 Get/Pick/Clear 接口返回值为事实来源，不写前端持久化状态；`debug.keep_intermediate_files` 仅随本次批量启动请求传递。
- 剪辑通过 `ProbeClipDuration` / `ConcatEditClips` 和 `compose_progress` 协作；不得继续使用源码已移除的工程保存、素材自动匹配等接口。
- 前端本地事件复用 `src/shared/events.ts` 常量；不要与后端 Wails 事件混淆。订阅与组件/composable 生命周期配对清理。

## 验证与维护

- 前端代码改动执行 `cd frontend && npm run build`；领域状态或后端适配层改动同时执行 `cd frontend && npm test`；前后端契约变更还须执行 `go test ./...`。
- `npm test` 目前主要覆盖可注入 runtime 的领域 core/helper、适配层和类型契约；未覆盖真实 `productionStore`/`EventsOn` wrapper、Vue 页面/路由交互或 selection-state/materials 的完整页面链路，不能把它们当作已验证项。
- 状态或按钮策略改动，核对相关路径：工作目录初始化 → 自动启动检查 → 组件状态变化 → 重试/导入按钮 → 进入主页面。
- 路由或视图改动，核对三步骤导航、导入子页返回、受影响的剪辑/设置入口；制作或清理按钮改动，核对忙碌、未加载和查询失败状态。
- 无法在当前环境完成 UI 或 Windows 实机核对时，明确记录未验证项；不以构建通过代替交互验证。
- 新增/重命名契约、状态映射或目录职责时同步根文件和适用的子规则；仅文档改动按根文件的文档检查执行。
