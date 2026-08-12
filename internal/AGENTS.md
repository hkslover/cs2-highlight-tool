# AGENTS.md（internal）

本文件作用域：`internal/**`。  
与根级 `AGENTS.md` 同时生效；如冲突，以本文件（更具体作用域）为准。

## envsetup 状态机约束
- Must：保持以下组件 ID 不变：`hlae` `plugin` `ffmpeg` `cs2`。
- Must：保持以下状态值语义一致：`pending` `checking` `downloading` `installing` `ready` `warning` `failed` `needs_action`。
- Must：保持以下阶段值语义一致：`detecting_source` `waiting_source` `running_tasks` `ready`。
- Must：`StartupState` 字段语义保持稳定，新增字段需保证前端兼容并更新规则文档。
- Must：统一 Release 快照获取完成后先检查软件自身更新；若 `SelfUpdate.Available=true` 且状态为 `needs_action`，不得启动组件检查/下载/安装任务，组件步骤应保持未启动语义。
- Must：软件更新检查失败保持非致命语义，可继续组件环境检查；只有确认发现新版本时才阻断组件流程。
- Must：`StartupState.ads[]` 仅承载 `placement=main_steps_top_banner` 的有效 Sponsored Card 广告数据（`click_url/sponsor/title/rich_html/image_url/image_alt`），广告解析失败不得阻塞启动主流程。
- Must：HLAE 的 `LocalVersion` 必须来自安装目录 `changelog.xml` 的首个 `<version>`；不得以配置文件持久化版本号作为真值来源。
- Must：插件 DLL 的 `LocalVersion` 必须来自安装目录 `changelog.xml` 的首个 `<version>`；不得以配置文件持久化版本号作为真值来源。
- Must Not：在未同步前端映射与测试前，重命名状态枚举、阶段枚举或组件 ID。

## 并发与锁
- Must：涉及 `Service.state`、`Service.logs`、`Service.config` 的读写遵循现有锁策略（`mu` / `configMu`）。
- Must：避免锁内执行可能阻塞的外部调用（I/O、网络、runtime 事件发射）。
- Must：遵循既有模式，状态更新后通过 `emitState()` 通知前端。
- Must Not：引入新的锁顺序反转，避免死锁风险。

## Debug 注入约束
- Must：debug 插件 DLL override 只允许作为 `internal/app.App` 的会话级状态，不写入 `config.json`。
- Must：录制前注入 CS2 `game/csgo/plugin/bin/server.dll` 时，debug override 只覆盖源 DLL 选择；目标目录、备份、恢复逻辑继续复用 `preparePluginDLLForProduce` / `forceRestorePluginDLLForProduce`。
- Must Not：让 debug override 参与启动阶段插件版本检测；插件本地版本真值仍来自安装目录 `changelog.xml`。

## Clip 设置与插件动作约束
- Must：`pov_radar_enabled` 通过 `GetClipSettings` / `SaveClipSettings` 持久化，默认值为 `false`。
- Must：仅当 `pov_radar_enabled=true` 时，插件 JSON bootstrap 才写入 `csdm_radar_pov 1`；关闭时不得写入该命令或重置命令。
- Must Not：将 `pov_radar_enabled` 与现有 `pov_hud_enabled` 的 VPK/gameinfo 生命周期联动。
- Must：`hide_player_avatars` 通过 `GetClipSettings` / `SaveClipSettings` 持久化，默认值为 `false`；仅当启用时插件 JSON bootstrap 才写入 `cl_teamcounter_playercount_instead_of_avatars true`。

## 日志字段规范
- Must：启动链路日志后端统一使用 `internal/logging`（`log/slog` 适配层），业务侧通过统一 logger API 记录结构化字段。
- Must：`slog.HandlerOptions.ReplaceAttr` 脱敏规则必须启用，避免敏感字段进入内存 ring buffer 与 `log` 事件流。
- Must：确保关键字段可追踪：`component` `stage` `action` `source` `attempt` `error` `elapsed_ms`。
- Must：导出日志路径继续遵守脱敏规则（URL 参数、认证信息、home path 前缀）。
- Must Not：在日志中输出明文 token、密钥、认证头或用户真实 home 目录。

## 统一更新源约束
- Must：启动阶段优先请求统一 Release API 快照，再由各组件消费快照。
- Must：每次启动都需实时进行 GeoIP 检测并获取 `country_code`；统一更新源固定为 `github`；不得依赖 `config.json` 持久化地区结果。
- Must：组件下载回退顺序固定为：`country_code=CN` 时 `url -> mirror_url`，非 CN 时仅 `github_url`，GeoIP 检测失败或 `country_code` 为空时默认 `url -> mirror_url`；失败后直接报错。
- Must：统一源请求失败时允许回退到本地已安装组件（warning 语义），但不得伪造远端版本成功状态。
- Must Not：恢复多源自动回退（`executeWithSourceFallback` / `orderedRetrySources`）旧流程，或恢复 gh-proxy 终极兜底尝试。

## 后端变更必测
- Required：执行 `go test ./...`
- Required（触及 envsetup/release）：重点确认以下包通过：
- `go test ./internal/envsetup ./internal/release`
- Required（修改状态字段/事件契约）：补充或更新对应测试，至少覆盖：
- 状态迁移正确性
- 回退流程与持久化行为
- 日志字段完整性与脱敏行为
