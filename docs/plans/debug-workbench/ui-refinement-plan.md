# Debug workbench 细节调整计划

日期：2026-10-11。状态：待其他模型实施，实施后由原审查模型 review。

本计划基于当前两仓库源码及用户已完成的 Windows 上机反馈。用户确认现有功能正常，本次调整交互与展示，并同步必要的插件协议；当前回合只创建计划，不修改代码。既往实测不代表本计划实施后的验收已完成。

## 范围与优先级

- 桌面端：`/Users/snow/Desktop/my_workspace/cs2-highlight-tool`。
- 插件端：`/Users/snow/Desktop/my_workspace/cs2-highlight-tool-work/cs2-server-plugin`。
- 仅实施下述 8 项；不自动推进 `06-manual-recording.md`、不扩大到完整设置工作台、录制功能或制作系统重构。
- 本计划在“Demo 必须由导入页当前选择决定”“暂停/继续两个按钮”“快捷设置与自定义命令分卡”“展示当前 tick”“启动后显示重启提示”等交互上取代既有计划的对应要求。其他会话、协议、制作隔离及环境恢复约束继续有效。
- 开始实施前读取两仓库适用规则、检查各自 `git status`，保留无关 WIP。本次只读检查时两仓库均未报告已有改动。

## 需求与确定的交互

| 项目 | 实施结果 | 验收重点 |
| --- | --- | --- |
| 1. 插件启动 | “插件模式”改为“启动插件”，下拉框改为开关 | 开关直接映射 `inject_plugin`，开为 true、关为 false；维持现有默认行为 |
| 2. 品牌文案 | 调试页“品牌 VPK”改为“标识水印” | 仅改展示文案，继续映射 `brand_enabled` 和原 VPK 生命周期 |
| 3. 橙色提示 | 删除启动选项右侧的“重启后生效”橙色标签 | 游戏启动后仍按现有规则锁定启动资源及分辨率，不因删除标签开放编辑 |
| 4. 会话状态 | “连接与会话状态”卡片标题改为“会话状态”；插件连接用与会话状态一致的标签组件 | 状态有文字、有颜色；已连接和协议就绪分开表达 |
| 5. Demo 与 tick | 调试页提供已导入 Demo 下拉框；删除当前 tick 功能及跨端字段 | 无需返回导入页改选；仍保留实际播放状态与重播确认 |
| 6. 打开路径 | Demo 文件路径右侧添加图标按钮“打开所在文件夹” | 打开该文件的父目录，支持嵌套平台目录、中文与空格路径 |
| 7. 暂停/继续 | 两个按钮合并为一个 | 未暂停显示“暂停”及 warning 色；已暂停显示“继续”及 success 色；状态以引擎观测为准 |
| 8. 命令卡片 | 快捷设置与自定义命令合并为“控制台命令”卡片 | 预设下拉框填入可编辑输入框，再统一点击“发送”；选择预设本身不发送 |

### 启动与会话展示（1–4）

1. 将页面 `launch.pluginMode` 的字符串模式草稿改为布尔开关，或通过布尔适配绑定；移除不再使用的选项数组。请求、会话回填、无插件雷达禁用逻辑全部核对，避免只换控件未换消费者。
2. 删除启动插件、POV HUD、标识水印、分辨率行内的 `restart_required` 标签。保留必要的禁用原因及真实错误；不删除 `status=starting/stopping` 等状态本身使用的 warning 色。
3. 插件标签从现有 `plugin_enabled/plugin_connected/plugin_ready/plugin_message` 推导，不新增会话生命周期状态或把连接冒充 ready：

| 条件 | 标签 | 颜色 |
| --- | --- | --- |
| 会话未启动 | 未连接 | default |
| 活跃会话未启用插件 | 未启用插件 | default |
| 已启用、尚未连接 | 等待连接 | warning |
| 已连接、尚未 ready | 已连接，等待就绪 | warning |
| 当前连接已完成 ready 握手 | 已就绪 | success |
| 后端明确报告超时、协议失败或异常断连 | 连接异常/就绪失败 | error |

错误分类使用已有后端事实与错误状态；如当前字段无法可靠区分，先核对后端映射，不通过中文消息关键词猜状态。较长的 `plugin_message` 可在标签下方展示或作为提示，保留排障信息。无插件模式不标红。

### Demo 下拉选择与目录定位（5–6）

1. 列表复用 `frontend/src/domains/demo/demo-state.ts` 的 `demoList`，不扫描磁盘、不再创建第二套导入列表、不要求重新解析文件才允许播放。
2. 使用 Demo 稳定 `key` 作为选项 value，名称为主标签，同名文件用父目录/平台信息区分；完整路径可查看。
3. 调试页独立持有待播放 Demo key，存放在 debug domain/store，随工作目录重置清空；首次进入可优先使用导入页已选项，否则选列表首项。只在首次初始化或原选择失效时执行默认选择。
4. 在调试页改选不改变导入页、片段页的选择；返回导入页改选也不强制覆盖调试选择。列表为空时清空选择并提示先导入；选中条目被移除时回退至首项或空。
5. 切换下拉框只改变待播放对象，点击“播放”才冻结路径和命令并发送 `PlayDebugDemo`。实际播放对象继续来自会话 `demo_path/demo_playing`，切换选项不能把旧 Demo 状态伪装成新 Demo 已播放。
6. 解析 loading/error 不直接等同于文件不可播放：保留零击杀、仅已导入文件也可播放的能力；后端继续验证受管控路径、文件存在、扩展名及命令引用安全。
7. 文件路径行对应当前下拉选择。新增窄用途 Wails 方法 `OpenDemoInFolder(demoPath)`，打开经校验的该受管控 `.dem` 父目录。现有 `OpenDemoDirectory()` 只打开 Demo 根目录，不能代替本需求；`OpenProducedClipInFolder()` 也不能作为 Demo 接口复用。
8. 复用现有受管控 Demo 校验和目录打开工具；进入工作目录使用权边界后校验并执行，拒绝外部路径、越界链接、非 `.dem` 或缺失文件。无选择禁用按钮，失败显示可见错误。
9. 同步 Wails 适配层、方法契约和生成绑定；使用项目生成流程，禁止手工改 `frontend/wailsjs/**`。不使用 `OpenExternalURL` 打开文件系统路径。

### 删除当前 tick 与单按钮暂停（5、7）

当前插件只在播放确认、启停或路径改变时发送 `debug_demo_state`，tick 不持续更新；桌面端也不因 tick 单独变化发射状态。因此本次删除该展示能力，不修补实时 tick 推送。

1. 删除页面当前 Tick 行及相关中文文案；删除桌面 `DebugDemoState.tick`、`DebugSessionState.demo_tick`、runtime 的 `debugDemoTick`、快照/重置及前端 domain 默认值、共享类型和测试中的对应字段。
2. 插件 `BuildDebugDemoStateMessage` 及调用去掉对外 `tick` 参数/字段，README 和协议断言同步；保留 `session_id/demo_path/playing` 播放观测消息。
3. **保留插件内部 tick 观测及 `DebugPlaybackGate` 的 tick 回退判断。** 该判断用于同文件重播确认和延迟应用播放命令，不属于用户要删除的“当前 tick”功能。禁止全局删除 `GetDemoTick/currentTick` 或正式制作 tick 序列。
4. 为单按钮新增最小暂停事实：插件 `debug_demo_state.paused`、桌面 `DebugDemoState.paused` 和 `DebugSessionState.demo_paused`，沿现有连接/会话绑定链路同步到前端。它是播放属性，不新增会话 `status` 枚举。
5. 插件在游戏线程读取引擎真实暂停状态，核对当前 SDK/引擎接口后实现；启动、停止、路径切换或暂停状态改变才发状态事件，不增加逐帧 tick 消息。不要用 tick 不增长推断暂停，也不要解析用户命令字符串推断暂停。
6. 本次只读检查尚未确认可直接使用的暂停接口。实施者需先核对当前 SDK、插件已有 engine/demo 接口与实际运行支持；若不能可靠观测，明确列为阻塞项并反馈，不用按钮本地翻转假装完成。
7. `demo_paused` 仅在实际 Demo 已加载时有效；播放结束、会话结束、连接断开/替换、工作目录切换时清 false。旧连接、异会话、迟到消息不得修改当前状态。
8. 单按钮仅在运行、plugin ready、实际 Demo 播放中且无相关在途请求时启用。按 `demo_paused` 发送 `demo_pause` 或 `demo_resume`，继续复用 `SendDebugConsoleCommand`，无需新增暂停请求 DTO。
9. 发送期间禁用防连点；失败/超时保留原观测状态并显示错误。成功回执只说明命令已执行，不直接翻转暂停事实；按现有有限请求策略处理未收到观测的情况，不能永久 loading。通过自定义命令或游戏内操作暂停后，按钮也能同步。
10. 修改后的 host 忽略旧消息多余 tick，不要求消息必须有 tick；暂停字段缺失不能被当成已验证的可切换状态。实施时明确旧 DLL 行为，必要时沿现有 `protocol_version` 校验能力并提示更新，不静默显示错误按钮。

### 合并控制台命令（8）

1. 卡片内上方为“快捷指令”下拉框，下方为共用可编辑命令输入框、一个发送按钮、一份 loading/错误/最近回执。
2. 保留现有四项预设：关闭天空 `r_drawskybox 0`、开启天空 `r_drawskybox 1`、隐藏 UI `cl_draw_only_deathnotices 1`、显示 UI `cl_draw_only_deathnotices 0`。本次不扩展完整设置列表。
3. 选择预设将其命令填入输入框，可修改后发送；不要选择一次发送一次、按钮再发送一次。支持重复选择同一预设恢复输入，手工编辑时避免下拉显示与输入含义冲突。
4. 统一复用 debug domain 的 `sendDebugConsoleCommand`；自定义输入整体仍为一个命令字符串，保留分号与引号，不新增白名单、不改成制作 sequence。
5. 未运行、无插件、未 ready 或发送中沿现有规则禁用发送；准备输入仍可用。不会调用 `SaveClipSettings` 或更改正式设置草稿。

## 实施入口

| 层 | 主要入口 |
| --- | --- |
| 页面与中文 | `frontend/src/features/debug/pages/DebugPage.vue`、`frontend/src/shared/i18n/zh-CN.json` |
| Debug 状态与隔离 | `frontend/src/domains/debug/debugSessionCore.ts`、`debugSessionStore.ts`、`index.ts` |
| Demo 列表 | `frontend/src/domains/demo/demo-state.ts`、`frontend/src/features/import/pages/ImportPage.vue` |
| 类型与 Wails 边界 | `frontend/src/shared/types/debug.ts`、`frontend/src/shared/backend/adapter.ts`、`types.ts`、`frontend/wailsjs/`（生成） |
| 会话/播放 | `internal/app/debug_types.go`、`debug_session.go`、`debug_playback.go`、`debug_command.go` |
| 目录定位参考 | `internal/app/outputs_storage.go`、`produce_takefile.go` 及原平台目录打开工具 |
| 桌面测试 | `internal/app/debug_playback_test.go`、现有 debug 会话/命令测试、`frontend/tests/` 中 debug core 与后端契约测试 |
| 插件 | `cs2-server-plugin/main.cpp`、`record_status_bridge.cpp/.h`、`debug_playback.h`、`tests/debug_playback_test.cpp`、`README.md`（路径相对插件仓库） |

实施前使用 `rg` 确认字段全部消费者及新增方法的注册/契约要求，不局限于表内文件。默认只改中文 i18n，英文由用户维护。

## 建议执行顺序

1. 先确认真实暂停状态的引擎接口与旧 DLL 兼容策略，完成插件/host 消息契约及有意义的状态测试。
2. 完成 Demo 本地选择状态、重置与文件夹打开边界，生成 Wails 绑定并同步适配层。
3. 实施页面 1–8 项、精简无用样式/中文 key，保持其他功能和锁定策略。
4. 更新根/前端/后端适用 `AGENTS.md` 的长期语义、插件 README，以及旧 `04-demo-playback.md`、`05-live-settings.md` 和计划 README 中与本计划冲突的描述。不要把实施流水账加入 AGENTS。
5. 执行下述检查，交付两仓库 diff、验证证据和未验证项，等待 review；不自动提交、推送或进入手动录制开发。

## 验证与 review 清单

### 自动检查

- 桌面端：`go test ./...`、`cd frontend && npm test`、`cd frontend && npm run build`、`git diff --check`。
- 有意义的测试：Demo 选择与导入选择隔离、所选条目移除与工作目录重置、迟到消息隔离、暂停属性清理与状态同步、目录打开越界/不存在/中文路径、消息不再携带 tick。
- 保留并运行 `DebugPlaybackGate` 同文件重播、旧 Demo 正常 tick 增长不确认新播放、路径切换及空命令请求等既有测试；插件按实际项目构建方式编译相关 C++ 测试和 Windows DLL，报告精确命令及平台。
- 搜索 `demo_tick/debugDemoTick/BuildDebugDemoStateMessage` 核对删除链路与生成产物；区分内部重播判断、Demo 解析元数据和正式制作 tick，不以零 tick 字符串匹配为验收目标。
- 新增方法/字段须覆盖消费者和契约，不只靠 Vue 构建通过。纯文案和样式不编写镜像实现的测试。

### Windows 交互验收

- 开关启用/关闭插件均可启动，雷达禁用策略正常；显示“标识水印”，启动后无行内橙色重启提示且启动项仍锁定。
- 插件标签覆盖未启动、无插件、等待连接、已连接未 ready、就绪、断连/失败，错误可见且颜色符合事实。
- 导入两个 Demo 后直接在 debug 下拉切换并播放，无需返回导入页；同名不同目录可辨识。切换待播放对象不改变当前引擎播放事实。
- 文件夹按钮进入所选 Demo 的真实父目录，含中文/空格和 wanmei/5e 子目录；移除/失效文件不会打开无关路径。
- 页面无当前 tick，插件状态消息无 tick；同文件重播和播放草稿延迟应用仍正常。
- 一个按钮完成暂停/继续，文字颜色切换正确；自定义命令和游戏内暂停也能同步。播放结束、重播、断连、命令失败及连点不产生错误状态。
- 快捷指令填入后经一次发送执行，自定义分号/引号保留，共用一份回执，正式设置不被改写。
- debug 仍不读取/执行/修改 Demo 旁车制作 JSON；正式制作仍按原队列执行；停止及环境恢复不受影响。

### 交付要求

实施模型报告：两仓库改动文件与 Git 状态、各项需求完成情况、自动检查结果、Windows 实测结果及未验证项，特别说明暂停观测来源和协议兼容策略。本机 Node/Go 检查或插件纯 C++ 测试不能替代 Windows 注入、实际引擎暂停和目录交互验证。

review 重点：用户 8 项是否完整、是否仅删除 tick 对外能力而误伤重播/制作、暂停状态是否真实、Demo 选择是否独立且 generation 安全、目录打开是否在工作目录边界内、生成绑定/适配层/规则是否同步。
