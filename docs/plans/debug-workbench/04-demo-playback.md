# 04：播放导入页当前 Demo

前置：03。范围包含手动播放，不包含片段规划和手动录制。

## 实施

1. 新增 `PlayDebugDemo({demo_path})`，前端点击时冻结调试页下拉当前选中的 Demo 路径（`debugDemoKey` 对应的受管控文件，首次默认可取自导入页选择），后端校验当前工作目录的受管控 Demo 文件，复用现有 managed path/文件使用权逻辑。不要求击杀列表、选中玩家或已生成 JSON。切换下拉只改变待播放对象，不发送命令；调度与导入页选择互不影响。
2. 新增消息 `debug_play_demo`，payload 为 `{request_id,demo_path}`。插件 debug 分支在游戏线程清空旧 sequences、设置当前 Demo、执行带引号的 `playdemo`，发送 `debug_command_result`。
3. 这个分支绝不调用 `LoadSequencesFile`，不读取/写入/删除 `<demo>.json`。在 debug 模式下，启动参数自动播放以及其他可能触发加载序列的入口也不能恢复自动制作行为；检索全部 `LoadSequencesFile` 调用点后处理。
4. 路径引号/换行等由统一命令参数处理保证，按项目现有规则拒绝不能安全组成播放命令的路径，不构造裸 `playdemo path`。
5. 当前 `demo_started` 是 WS 线程排命令后就发送，不能证明 engine 实际播放。debug 模式根据游戏线程观测的 `isPlayingDemo` 变化更新状态，使用独立 `debug_demo_state`：`{session_id,demo_path,playing,paused}`（ui-refinement-plan 取代本项：删除对外 tick，新增引擎暂停事实与协议版本 2）。只在启动、停止、暂停变化或观测路径变化时发送，不增加高频 tick 推送，也不在页面上展示当前 tick。
6. 播放时应用当前调试草稿中的游戏快捷命令：必须在实际 Demo 可执行对应设置的时机应用一次，避免主菜单先发然后 Demo 加载覆盖。不要使用 tick 自动序列；雷达可先初始化，其他依赖 Demo 的命令从游戏线程的播放状态变化触发。
7. 提供单个暂停/继续按钮，复用 `debug_execute` 的 `demo_pause/demo_resume`；可选不另建请求 DTO。按钮只由引擎观测的 `demo_paused` 决定文字与颜色，回执不直接翻转，发送期间禁用防连点（ui-refinement-plan 取代原“两个按钮”实现）。播放回执后 `demo_play_pending` 保持禁用直到引擎确认目标 Demo（或有限超时/失败/断连），此时切换与同文件重播都不得发出暂停/继续命令（review 修正）。重复播放由显式点击发起，不使用生产队列的重连自动派发。
8. 未启用插件时按钮按总计划禁用。运行中换 Demo 在 06 接入后需拒绝录制/停止/合并中的切换，先完成当前段。在调试页改选或返回导入页改选都不会自动发送播放命令，也不会互相覆盖选择。
9. 调试页 Demo 下拉旁提供“打开所在文件夹”按钮，调用窄用途 `OpenDemoInFolder` 打开受管控 `.dem` 的父目录；不复用 `OpenDemoDirectory`（只开 Demo 根目录）或 `OpenProducedClipInFolder`。字面位于 `demo` 下不够：校验还须解析真实路径，拒绝指向工作目录外的符号链接/junction（不确定时拒绝），并覆盖文件链接、父目录链接越界用例（review 修正）。

## 验收

- 一个 Demo 带有效旧 `.json`，debug 播放不跳 tick、不自动录制、不 disconnect/quit；旁车字节保持不变。
- 零击杀/仅有导入文件的 Demo 可播放；无选择或文件失效时报错。
- 含空格与中文路径可用；UI 分别显示“播放命令已执行”和实际播放状态，实际播放路径单独展示且不因下拉切换而变。
- 播放结束不自动结束 debug 游戏；断连重连不重复播放。
- 正常制作仍读取 JSON 并按原队列运行。
- 两仓库有针对性的测试/Windows 验证，桌面端按三项检查执行，更新新消息文档。
