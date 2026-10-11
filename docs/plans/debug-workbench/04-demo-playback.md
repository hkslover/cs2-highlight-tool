# 04：播放导入页当前 Demo

前置：03。范围包含手动播放，不包含片段规划和手动录制。

## 实施

1. 新增 `PlayDebugDemo({demo_path})`，前端点击时冻结导入页 `selectedEntry.file_path`，后端校验当前工作目录的受管控 Demo 文件，复用现有 managed path/文件使用权逻辑。不要求击杀列表、选中玩家或已生成 JSON。
2. 新增消息 `debug_play_demo`，payload 为 `{request_id,demo_path}`。插件 debug 分支在游戏线程清空旧 sequences、设置当前 Demo、执行带引号的 `playdemo`，发送 `debug_command_result`。
3. 这个分支绝不调用 `LoadSequencesFile`，不读取/写入/删除 `<demo>.json`。在 debug 模式下，启动参数自动播放以及其他可能触发加载序列的入口也不能恢复自动制作行为；检索全部 `LoadSequencesFile` 调用点后处理。
4. 路径引号/换行等由统一命令参数处理保证，按项目现有规则拒绝不能安全组成播放命令的路径，不构造裸 `playdemo path`。
5. 当前 `demo_started` 是 WS 线程排命令后就发送，不能证明 engine 实际播放。debug 模式根据游戏线程观测的 `isPlayingDemo` 变化更新状态，使用独立 `debug_demo_state`：`{session_id,demo_path,playing,tick}`。只在关键状态变化发送，首版不增加高频 tick 推送。
6. 播放时应用当前调试草稿中的游戏快捷命令：必须在实际 Demo 可执行对应设置的时机应用一次，避免主菜单先发然后 Demo 加载覆盖。不要使用 tick 自动序列；雷达可先初始化，其他依赖 Demo 的命令从游戏线程的播放状态变化触发。
7. 提供暂停/继续按钮可复用 `debug_execute` 的 `demo_pause/demo_resume`；可选不另建请求 DTO。重复播放由显式点击发起，不使用生产队列的重连自动派发。
8. 无插件模式按钮按总计划禁用。运行中换 Demo 在 06 接入后需拒绝录制/停止/合并中的切换，先完成当前段。回到导入页改变选择不自动发送播放命令。

## 验收

- 一个 Demo 带有效旧 `.json`，debug 播放不跳 tick、不自动录制、不 disconnect/quit；旁车字节保持不变。
- 零击杀/仅有导入文件的 Demo 可播放；无选择或文件失效时报错。
- 含空格与中文路径可用；UI 分别显示“播放命令已执行”和实际播放状态。
- 播放结束不自动结束 debug 游戏；断连重连不重复播放。
- 正常制作仍读取 JSON 并按原队列运行。
- 两仓库有针对性的测试/Windows 验证，桌面端按三项检查执行，更新新消息文档。
