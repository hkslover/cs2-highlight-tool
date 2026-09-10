# AGENTS.md（后端）

作用域：`internal/**`。与根级 `AGENTS.md` 同时生效；本文件补充后端实现约束。跨层字段、事件、下载策略和制作生命周期统一见根文件，修改时须同时检查前端消费者。

## 分层与定位

- `app/` 承担 Wails 边界和跨模块编排，公开请求/响应在此定义；不要让低层包依赖 UI。
- 启动状态模型与通知入口：`envsetup/state.go`、`envsetup/events.go`；检查/动作/状态逻辑按 `service_*.go` 分工。
- 配置默认值、归一化和兼容处理集中在 `config/config.go`；同一工作目录的读改写事务由 `config.Store` 统一串行化并由 App 注入 `envsetup.Service`；应用层设置映射在 `app/clip_settings.go`。
- Demo 事实与整局 POV 解析在 `demo/`，片段归一化与生成编排在 `app/plugin_generate.go`，插件命令构建在 `clipsjson/`。
- 平台导入协调（同键去重、等待者、结果固定与清理）在 `app/import_coordinator.go`，由工作目录身份持有并在切换/重置时替换；`fivee`、`wanmei` 只负责来源解析、下载解压与缓存提交，不互相依赖。
- 会话、清理和文件占用分别从 `app/produce_session.go`、`app/produce_cleanup.go`、`app/work_activity.go` 查起；WebSocket 状态与协议在 `producews/`。
- Windows 专用行为与其他平台实现通过现有平台文件分开维护；非 Windows 测试不能替代 Windows 运行验证。
- `download.ReplaceDirWithContents` 只承担目录文件系统提交，不包含 HLAE/FFmpeg 组件判断。实现必须使用本次实例创建且可确认归属的唯一 staging/backup；准备或备份失败保持旧 target，提交失败先尝试恢复，恢复失败保留两条恢复路径并返回主/恢复双重错误。提交成功但旧 backup 清理失败通过 `ReplaceDirWithContentsWithReport` 暴露为告警，不得回报为安装失败；不触碰既有 `.old` 或未知事务残留。文件系统故障注入使用实例依赖，不增加包级可写 seam。
- `download.CopyFile` 是兼容入口，但使用同目录临时文件完成 Copy、Close 检查和提交 rename；缓存导入通过 `IsLikelyDemoFile` 只做基本文件戳校验，不能把命中结果当作完整性证明。目标已存在但不是普通文件（尤其是目录）时必须在移动目标前拒绝，不得把目录移入备份路径后用文件覆盖。原子复制的故障注入通过实例 `atomicCopyOps`，不得引入包级可写 seam。
- `download.UnzipWithContext` 在打开归档、每个条目和每次复制之间检查 ctx，长解压可被工作目录关闭打断；`download.Unzip` 是无取消的兼容入口，故障注入通过实例 `unzipOps`。平台导入解压 seam 因此接收 ctx，组件安装等旧调用者继续使用兼容入口。

## 工作目录与启动状态机

- 初始化遵循 `app/app_workspace.go`、`appdata/validate.go`、`appdata/registry_windows.go`。保留 service 未初始化时的防御逻辑，不把 `workspace_init` 当作组件检查失败。
- `StartupState` 的 mode、phase、状态值和组件 ID 遵循根文件；新增字段须保持前端兼容，不得只改后端枚举。
- 统一 Release 快照后先检查自身更新。确认发现更新并进入 `needs_action` 时，组件保持未启动语义；自更新检查失败可继续组件检查。
- 广告解析失败不得阻断启动，只向前端暴露有效的 Sponsored Card。
- HLAE/插件版本读安装目录的 `changelog.xml`，不得改为配置版本号。
- 下载顺序遵循根文件；统一源请求失败可回退本地已安装组件并使用 warning，但不得伪造远端版本成功。不得恢复 `executeWithSourceFallback` / `orderedRetrySources` 旧流程或 gh-proxy 终极兜底。

## 并发、进程与收尾

- `Service.state`、`Service.logs`、已提交的 `Service.config` 快照由 `Service.mu` 保护；配置文件的最新读取、归一化、mutate 和保存只走工作目录共享的 `config.Store`，不再为 App/Service 各自维护文件锁。工作目录准入后再进入 Store，保存成功后才更新 Service 快照，事件必须在 Store 解锁后发射；应用 service 的访问遵循 `serviceMu`。
- `app/workspace_session.go` 的私有 `workspaceSession` 持有不可变 root/generation/service 与生命周期 context；任务须在启动前登记。切换/退出先关闭准入，再取消并等待，不得在 service/state 锁内等待、做 I/O 或发射事件。`envsetup.Service` 的 `BindLifecycleContext`、`CloseIfIdle`、`Stop` 仅由 App 的 session 生命周期调用，旧实例关闭后不得重新接收任务。
- 避免锁内执行阻塞 I/O、网络或 runtime 事件发射，不引入锁顺序反转。启动状态更新后通过 `emitState()` 通知前端；制作事件复用现有队列。
- `GetWorkActivity` 的前端禁用策略不代替后端互斥；文件读写/导出/清理复用现有文件使用权机制。多个调用者共享的长任务（如平台导入）必须用 `beginManagedWorkspaceTaskUse` 的工作目录 context 运行并持有所属准入直到真实任务退出；等待者取消不得取消共享任务，进度与提交等副作用只由真实任务执行。
- 制作生命周期遵循根文件“制作、剪辑与清理”：活跃会话禁止重复生成，失败收尾保留重试所需状态；未确认进程退出、环境恢复前不得提前释放备份和占用。
- 剪辑合成一次只允许一个任务：`app/edit_task.go` 的准入拒绝第二个活跃请求而不排队；任务持有工作目录文件使用权、任务专属临时目录和 `O_EXCL` 预留的唯一输出名，只有验证临时视频、确认任务未取消且预留路径仍保有本次文件身份后才 rename 提交并登记 history。失败、超时或取消只清理本任务登记的路径，不得删除已成功提交、被替换对象或其他任务的视频；工作目录取消不得被包装成编码器不可用，也不得触发编码器回退重试，提交成功后不再删除已发布文件。
- HLAE 启动成功但 CS2 PID 未知时，保留启动器句柄并核对进程枚举；不能因 PID 为空直接恢复环境，也不能关闭归属不明的游戏进程。
- 涉及会话退出、取消和 FFmpeg 合并时，检查等待、探测及子进程是否遵循现有取消/超时机制，避免收尾后旧任务继续写文件。

## 设置、生成与游戏环境

- 新设置核对配置默认值/兼容处理、Get/Save DTO、生成逻辑及前端类型/控件；关闭命令开关时不生成命令或重置命令。编码设置的 `effective_video_preset`、`effective_video_encoder`、`effective_video_status` 仅作为 Get/Save 响应的只读探测结果，不能覆盖持久化的 `video_preset` 用户策略。
- `pov_radar_enabled` 不得与 `pov_hud_enabled` 的 VPK/gameinfo 生命周期联动；`sky_blackout` 不得联动关闭云层。
- `primary_view`、单片段覆盖和整局 POV 的语义遵循根文件。主视角是选中玩家视角，不能固定为 killer；`include_killer` 缺省 true 的兼容性须保留。
- 录制 take plan/history 的 `tick_rate`、`record_start_tick`、`record_end_tick`、`kill_offsets_seconds` 只作为后续剪辑的可选观测元数据；插件动作和原有 source window 保持不变。最终视频旁的 `.fastedit.json` 只能 best-effort 写入，不能让成功录制失败。
- debug DLL override 只作为 `App` 会话状态，不写入配置，也不参与启动插件版本检测。
- debug override 仅改变注入源 DLL，目标、备份和恢复复用 `preparePluginDLLForProduce` / `forceRestorePluginDLLForProduce`。
- 修改生成计划时核对 take 命名、稳定 source ID、历史去重键与前端选择状态，不能仅验证 JSON 能序列化。

## 日志与诊断

- 启动链路统一使用 `internal/logging` 的 `log/slog` 适配层，通过统一 logger API 记录结构化字段。
- 保留 `slog.HandlerOptions.ReplaceAttr` 脱敏，敏感信息不得进入内存 ring buffer 或 `log` 事件流。
- 关键字段保持可追踪：`component/stage/action/source/attempt/error/elapsed_ms`。
- 导出和错误路径同样脱敏 URL 参数、认证信息及 home path；禁止记录明文 token、密钥、认证头或真实 home 目录。
- 制作诊断应保留 WS、队列、take 和退出阶段信息；正常有序断开不能误记为故障。导出内容范围见根文件。

## 验证与维护

- 后端代码改动执行 `go test ./...`；envsetup/release 改动确认这两个包通过，可用 `go test ./internal/envsetup ./internal/release` 定位失败。
- 状态、回退或日志契约变更，按涉及范围补充状态迁移、持久化/回退、字段与脱敏测试。
- 制作生命周期变更应覆盖重复启动、文件清理互斥、失败收尾重试、进程归属与取消传播等实际受影响路径。
- 剪辑裁剪范围须校验有限值、边界和非空结果；缺少可靠 victim 标记时保持整段，不按当前设置推断死亡位置。可选裁剪与无音轨合成沿用 `app_edit.go` / `edit_ffmpeg.go` 的单次处理路径，并覆盖混合整段/裁剪输入。编辑相关改动还须覆盖单任务准入、唯一输出预留、失败/取消不写 history 以及不删除既有产物。
- 跨层契约变更还须执行前端构建并更新根文件与前端规则；仅文档改动按根文件的文档检查执行。
