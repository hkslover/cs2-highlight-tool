# Debug workbench 细节调整 review

日期：2026-10-11。审查范围：桌面端和插件端当前未提交 diff，含新增未跟踪源码/测试。结论：8 项需求的主要实现已覆盖，但有以下 2 个 P2 问题，建议修正后再验收。本次未修改应用或插件代码。

## 发现

### 1. P2：新增目录打开入口未拒绝越界链接

位置：`internal/app/debug_playback.go:143`，校验实现位于同文件 107–127 行；引用的 `internal/app/edit_task.go:254` 仅进行路径字符串的相对位置检查。

`OpenDemoInFolder` 复用的校验先按字面路径判断是否位于 `<dataDir>/demo`，再执行跟随符号链接的 `os.Stat`。没有解析真实目标路径或拒绝路径上的链接。因此，若 `demo/linked` 指向工作目录外一个包含 `.dem` 的目录，`demo/linked/match.dem` 会通过校验，文件管理器会进入工作目录外的实际目录。这与计划明确要求拒绝越界链接不一致。

路径校验 helper 本来就存在此限制；本次新增的公开目录打开接口沿用了它，也继承了此问题。现有新增测试只覆盖字面上的外部路径、缺失文件、非 Demo 和空值，没有覆盖链接目录。

建议：在已登记的工作目录使用权内，核对 Demo 根与目标的真实路径，确认真实目标仍在真实 Demo 根内，或按项目策略拒绝链接路径；核对 Windows junction/reparse 路径行为。补充文件链接与父目录链接越界用例，断言拒绝且不调用目录打开器。保留普通中文/空格/嵌套平台目录的成功路径。

### 2. P2：播放请求在途时，暂停/继续按钮仍可发送命令

位置：`frontend/src/features/debug/pages/DebugPage.vue:522`，事件处理位于 648–658 行。

`canTogglePause` 只检查 `canSend` 和 `demo_playing`，未检查 `playingRequest`。`playDebugDemo` 使用独立的 `playing` 标记，不会设置 `sending`。

触发方式：Demo A 已播放，选择 B 并点击播放；在播放请求等待回执、引擎仍报告 A 的播放状态时，暂停按钮仍启用。此时发送的暂停/继续基于 A 的 `demo_paused`，却可能在切换过程或 B 开始后执行，造成新 Demo 被意外暂停，或命令方向与新 Demo 状态不符。同文件重播也有相同边界。

建议：暂停按钮禁用策略至少包含 `!playingRequest && !pauseRequesting`，处理函数也检查准入条件。对于播放回执返回但目标尚未被引擎确认的加载窗口，继续维持暂停禁用直到目标播放已确认或有限超时/失败，不把“命令已执行”当成目标加载完成。补充播放切换/重播在途时暂停不可用的行为验证。

## 已核对的完成项

- 启动插件使用布尔开关，并正确映射 `inject_plugin`；标识水印文案已更新。
- 行内橙色重启提示已删除，启动资源和分辨率锁定仍保留。
- 会话标题和插件状态标签已更新，使用结构化 `plugin_state`，不根据中文消息猜状态，不将连接视为 ready。
- Demo 列表复用导入 domain，debug 单独保存选中 key；列表变化与工作目录重置有测试。
- 页面与 host/public DTO、插件消息已去掉 tick；插件内部 `GetDemoTick` 和重播 gate 保留，正式制作序列未改动。
- 暂停事实来自插件现有 `IDemoFile::IsDemoPaused()`；host 仅在 playing 时保留 paused，暂停变化触发状态更新。debug 协议升为 v2，既有版本检查会拒绝旧 DLL。
- 快捷预设填入统一命令输入框，选择本身不发送；共享发送和回执，保留自定义分号/引号。
- 新方法的适配层、类型及 Wails 绑定内容一致；仅凭 diff 无法证明绑定通过生成命令产出，交付者仍需提供生成证据。
- 适用 AGENTS 和旧计划描述已同步。未扩展到手动录制实现。

## 本次实际验证

| 检查 | 结果 |
| --- | --- |
| 桌面 `go test ./...` | 通过 |
| 前端 `npm test` | 82 项通过，0 失败 |
| 前端 `npm run build` | 通过；Vite 仍有 chunk size 提示 |
| 两仓库 `git diff --check` | 通过 |
| 插件 `tests/debug_playback_test.cpp` | macOS clang++ 编译与运行通过 |
| 插件 `tests/debug_protocol_test.cpp` + `record_status_bridge.cpp` | macOS clang++ 编译与运行通过 |

插件独立测试使用 `clang++ -std=c++17 -Wall -Wextra -Werror`；协议测试额外包含 `-Ics2-server-plugin/deps/json/include` 并链接 `cs2-server-plugin/record_status_bridge.cpp`，测试二进制输出在系统临时目录。

本次没有构建 Windows DLL，也没有运行真实 Wails 页面、HLAE/CS2 或验证 `IsDemoPaused` 当前游戏版本的 vtable 行为。Windows 上应重点验收暂停/继续、自定义命令/游戏内暂停同步、播放切换与重播、连接恢复及目录打开；自动测试通过不能替代这些验收。

## Git 状态

两仓库的实施改动均仍处于未提交工作区。保留已有改动与新增文件；本次只新增本 review 文档，没有替工作模型修复、提交或推送代码。
