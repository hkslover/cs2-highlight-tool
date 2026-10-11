# 02：独立调试会话与两种 HLAE 启动

前置：01。核心复用入口：`hlae_launch.go`、`produce_gameconfig.go`、`produce_brand.go`、`produce_session.go`、`produce_cleanup.go`、`work_activity.go`、`workspace_session.go`。

## 实施

1. 定义 `DebugLaunchRequest`：`inject_plugin/pov_hud_enabled/brand_enabled/pov_radar_enabled/launch_resolution`。调试草稿从 `GetClipSettings` 初始化，品牌默认 true；不写 config。会话冻结 config 路径与请求选项，后续 helper 不重新读取不同快照。
2. 将 HLAE 启动实现拆出接收快照/明确选项的内部 helper，正式制作旧入口作为薄包装。两模式都注入 `AfxHookSource2.dll`；无插件模式无需 WS/插件 DLL 可用，也不设置本次插件连接环境变量。
3. 将游戏环境准备参数化为显式挂载路径集合。正式制作集合保持 plugin+brand+可选 POV；debug 集合按选项选取。复用备份、epoch、已有 VPK 不覆盖、本次写入才清理的语义。
4. 无插件模式须保证本项目插件搜索路径不会因为之前的残留而意外加载：启动前检查已知 `Game csgo/plugin`；必要时在本会话的 gameinfo 备份范围内临时去掉，退出恢复原内容。品牌/POV 的关闭同样按有效搜索路径处理，不删除用户既有 VPK/DLL。不处理未知第三方插件体系。
5. 插件模式复用已安装 DLL 或 `GetDebugPluginDLLOverride` 对应的选择，设置 `CSDM_WS_PORT`、`CSDM_LOG_PATH`；增加 `CSDM_DEBUG_SESSION=<session_id>`，供 03 的新版插件进入手动调试模式。
6. 新增独立 debug runtime，持有工作目录 context、环境 epoch、CS2 PID/待确认 HLAE 句柄、文件使用权和状态。复用现有 `produceLaunchMu` 做启动准入；将 debug 占用接入 `produceBusyError/GetWorkActivity`，正常制作、重新启动和目录清理不能抢占活跃调试会话。
7. 实现 `GetDebugSessionState/LaunchDebugGame/StopDebugGame` 与 `debug_session_state_changed`。进程启动成功和插件就绪分开展示；无插件模式显示“未启用插件”，不是连接失败。
8. 退出复用确认 PID 已退出后恢复环境的顺序；未知 PID 使用待确认 launcher 处理逻辑，恢复失败保留占用与备份供 `StopDebugGame` 重试。检测用户手动退出/自定义 quit 后进行同样收尾。
9. 接入 `App.Shutdown` 和工作目录关闭路径：先停止调试任务并等待，再停止 WS/关闭 Store。切路由不触发 Stop。此步仅接好收尾扩展点，录制中的有序停止在 06 补齐。
10. 前端接入启动、关闭、状态快照与事件；状态先订阅后读取，按 session/时间戳合并；工作目录切换清空本地会话草稿与旧请求结果。

## 必需验证

- 启动参数：无插件仍有 HLAE Hook，但不要求 produce WS；插件模式携带固定端口和 debug session。
- 正常制作仍强制品牌；debug 可关闭品牌，POV HUD 与雷达独立。
- 原本就有的 VPK 保留；两模式成功/失败/关闭恢复 gameinfo；无插件不意外挂载已有本项目 plugin 路径。
- 重复启动、正式制作与 debug 冲突、Reset/清理冲突复用后端准入；未知 PID 不提前恢复环境。
- 执行 `go test ./...`、`cd frontend && npm test`、`npm run build`；更新绑定与三份相关规则。
- Windows 验证两种启动和资源组合。本阶段新版插件协议未完成时，只能报告 PID 成功，不能报告调试插件已就绪。
