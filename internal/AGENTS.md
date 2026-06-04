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

## 日志字段规范
- Must：启动链路日志后端统一使用 `internal/logging`（`log/slog` 适配层），业务侧通过统一 logger API 记录结构化字段。
- Must：`slog.HandlerOptions.ReplaceAttr` 脱敏规则必须启用，避免敏感字段进入内存 ring buffer 与 `log` 事件流。
- Must：确保关键字段可追踪：`component` `stage` `action` `source` `attempt` `error` `elapsed_ms`。
- Must：导出日志路径继续遵守脱敏规则（URL 参数、认证信息、home path 前缀）。
- Must Not：在日志中输出明文 token、密钥、认证头或用户真实 home 目录。

## CS2 gameinfo 保护约束
- Must：长期原版备份路径固定为 `<dataDir>/backups/gameinfo/gameinfo.gi`。
- Must：只有当前 `gameinfo.gi` 不包含 `csgo/plugin` 时，才允许创建或刷新长期原版备份。
- Must：`RepairGameInfoFromBackup` 必须拒绝使用包含 `csgo/plugin` 的备份文件恢复游戏目录。
- Must：保留录制会话级 `.cs2ht_produce.bak` 备份/恢复链路，长期原版备份只用于异常退出后的手动修复。
- Must Not：用硬编码模板重建 Valve 默认 `gameinfo.gi`。

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
