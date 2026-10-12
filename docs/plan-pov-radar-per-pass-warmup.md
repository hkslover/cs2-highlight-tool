# POV 雷达逐片段开启与录制前预热计划

状态：待实施。本文只提供源码分析和实施计划，不表示已修改或实机验证。

## 1. 需求与实施边界

正式制作开启 `pov_radar_enabled` 时，每个实际录制 pass 都按以下顺序执行：

```text
跳转到准备位置
→ spec_mode <mode>
→ spec_player <target>
→ csdm_radar_pov 1
→ Demo 正常播放至少 2 秒
→ mirv_streams record start
→ mirv_streams record end
```

雷达命令不再由初始化序列自动发出。每次生成 `spec_mode/spec_player` 后都生成一次开启命令，包括同一玩家的再次录制；不得因上一段已开启或玩家相同而去重。覆盖 killer、victim、整局 POV 三类 pass，不额外限定 `spec_mode=1`，沿用用户选择的实际视角。

保留现有设置字段、默认值、持久化和前端控件。关闭该设置时，不生成雷达开启/关闭命令，不新增预热时间，保持原动作时序和 sequence 分组。雷达与 POV HUD VPK、品牌 VPK 的生命周期继续独立。

本计划只调整正式制作 JSON 的自动命令。调试页的手动快捷命令、`GetDebugConsolePlan` 和调试录制不纳入这次自动时序改造。`ExtraCommands` 中的用户自定义命令继续原样处理，不引入命令过滤器；“初始化不再开启雷达”指设置自动生成的命令。

## 2. 当前源码证据

| 文件 / 入口 | 当前行为及关联 |
| --- | --- |
| `internal/clipsjson/builder.go`：`Build` | 接收 `BuildOptions.PovRadarEnabled`，目前传入 `bootstrapOptions`，没有传入逐 pass 命令选项 |
| 同文件：`buildBootstrapSequence` | `PovRadarEnabled=true` 时，在 `DefaultActionTick=96` 写入一次 `PovRadarCommand(true)` |
| 同文件：`buildActionsFromPasses` | 每个 pass 生成 seek、视角切换、录制边界和 take 元数据；这是统一插入雷达命令的入口 |
| 同文件：时序常量 | `segmentSpecModeDelay=3`、`segmentSpecPlayerDelay=8`、`segmentRecordDelay=2`，单位均为 tick；最后的 2 不是 2 秒 |
| 同文件：`segmentSeekSettleSec=2.0` | seek 到 `pass.StartTick` 之前约 2 秒，视角切换仍在 `pass.StartTick` 之后，因此这段时间不是雷达开启后的预热 |
| 同文件：`buildMaterialSequences` | 整局 POV passes 共用一组 sequence，killer passes 共用一组，victim 每段一组；同组后续 seek 被限制为不能早于上一段结束 |
| `internal/clipsjson/gamesettings.go`：`PovRadarCommand` | 生成插件特殊命令 `csdm_radar_pov 1/0`，不是游戏注册的 cvar，可直接复用 |
| `internal/plugingen/plan.go`：`BuildPlan` | 已将冻结设置快照中的 `PovRadarEnabled` 传入 `clipsjson.Build`，无需增加配置链路 |
| `internal/plugingen/full_round.go` | 将整局事实转为 `FullRoundPOVSegment`，固定第一视角，已有结束补边策略；不要为了预热修改这些事实窗口 |
| `internal/clipsjson/builder_test.go` | `TestBuild_PovRadarCommandOnlyWhenEnabled` 当前断言初始化序列包含雷达开启命令，需要改写 |
| 同文件的录制窗口测试 | 已断言 `RecordStartTick/RecordEndTick/KillOffsetsSeconds`，以及 seek 不改变录制窗口；这些语义应继续保留 |

另行只读核对了插件源码：

```text
/Users/snow/Desktop/my_workspace/cs2-highlight-tool-work/cs2-server-plugin/
  cs2-server-plugin/main.cpp
  cs2-server-plugin/radar_pov.cpp
```

`TryHandlePluginSpecialCommand` 在 sequence 执行路径拦截雷达命令，hook 可以经命令开启；已经 installed 且 enabled 时会记录 `already active, ignoring duplicate command` 并直接返回。`go_to_next_sequence` 会弹出当前 sequence，并执行 `demo_gototick 0`、重置 `currentTick=-1`。这些源码支持运行中逐段发命令，但不证明实际视角已生效、雷达已渲染正确。

重复发送 `1` 是用户要求的防护动作，现有插件并不会因此强制刷新缓存。首版保留它的幂等行为，不擅自插入 `0→1` 或重装 hook；若实机仍发生切人后失效，再另立插件刷新方案。

## 3. 推荐方案：提前准备，保留原录制窗口

### 3.1 不直接延后 record start

如果在原来的 `spec_player` 后等待两秒再录，短片段可能漏掉击杀，甚至出现 start 晚于 end。当前代码会把 end 提升到 start，但这不能修复已经错过的内容，且终止命令仍按原 end 排列。

因此先计算旧逻辑得到的录制起止 tick，再向前安排视角切换与雷达预热。保留用户 pre/post 设置、source window、原录制起止和击杀相对录制起点的偏移。

### 3.2 明确 tick 公式

对每个 pass，使用已经归一化的 Demo `tickRate`，不使用录制 FPS：

```text
S = pass.StartTick
E = pass.EndTick
R = 旧逻辑计算的 record start
  = max(S + segmentSpecPlayerDelay, 96 + segmentSpecPlayerDelay)
    + segmentRecordDelay
Q = 旧逻辑计算的 record end = max(E + 1, R)

W = ceil(2.0 × tickRate)                   // 至少两秒 Demo 时间
radarTick      = R - W
specPlayerTick = radarTick - 2             // 沿用两 tick 的命令间隔
specModeTick   = specPlayerTick - 5        // 沿用原 mode→player 间隔
seekTarget    = max(0, specModeTick - seekSettleTicks)
```

建议将“两秒雷达预热”和“视角切换后两 tick 命令间隔”命名为独立常量，避免与 seek settle 混淆。`csdm_radar_pov 1` 使用独立、更晚的 tick，不能仅在 actions 数组中排在 `spec_player` 后却使用相同 tick。两 tick 是否足以让引擎切换视角需要实机验证，必要时只调整这个命令间隔并相应提前准备位置。

为保留原 seek 后的播放稳定时间，将 seek 目标同步提前到新 `specModeTick` 之前；不能继续使用 `S-seekSettleTicks`，否则可能直接跳过提前后的动作。

开启雷达时，只有 `seekTarget > DefaultActionTick` 才生成准备 seek。目标不晚于触发 tick=96 时省略 seek，让当前独立 sequence 从 tick=0 正常播放；插件按 tick 跨越消费动作，没有一次性消费标记，回跳到触发 tick 或之前会重复执行 seek，无法进入录制。省略 seek 不改变视角切换、雷达开启和录制边界 tick。

以现有测试场景为例：64 tick/s，击杀 tick=500，pre/post 各 1 秒：

| 动作 / 字段 | 旧值 | 开启雷达后的推荐值 |
| --- | --- | --- |
| source StartTick / EndTick | 436 / 564 | 436 / 564 |
| demo_gototick 目标 | 308 | 183 |
| spec_mode | 439 | 311 |
| spec_player | 444 | 316 |
| csdm_radar_pov 1 | 初始化 tick 96 | 318 |
| record start / end | 446 / 565 | 446 / 565 |

318→446 为 128 tick，即两秒；预热不写入输出视频。

### 3.3 开启雷达时，每个 pass 独立 sequence

推荐首版在 `PovRadarEnabled=true` 时，将每个已经形成的 pass 放入独立 sequence，统一沿用现有 victim 的 sequence 切换机制。关闭时继续旧分组。

原因是相邻 killer/整局 pass 的准备位置可能落入上一段录制窗口。继续放在同一 sequence 会提前切换上一段视角；现有“seek 不能早于上一段结束”限制也会导致预热不足。简单按 tick 排序无法解决该冲突。

独立 sequence 使上一段先结束，再回到 0 并 seek 到下一段准备位置；允许跨 sequence 的准备窗口重叠或向后跳，不在同一段录制期间切换玩家。会增加 sequence 切换与 seek 的开销，这是保留完整素材并保证逐段预热的明确取舍。

实施时：

1. 继续先按原逻辑构建/合并 killer segments，再转换为 passes；不要把同一已合并 take 拆成多个 take。
2. 保持顺序为整局 POV → killer → victim，以及各类别内部原排序。
3. takeCounter 在所有素材 sequence 之间连续递增；take 名称、kill IDs、source ID、view、spec mode、历史去重键和 segment count 不因拆 sequence 改变。
4. 中间 sequence 的末尾为 `go_to_next_sequence`，最后一个为既有 terminal command。同 tick 时必须先 `record end` 再切 sequence/断开，保留稳定排序及插入顺序。
5. 初始化 sequence 仍只运行一次，编码、输出目录和其他初始化命令保持原语义。
6. 需要 seek 的素材 sequence 仍在既有 `DefaultActionTick` 触发，且目标必须严格晚于触发 tick；较早目标省略 seek。核对每条 sequence 都能播放到录制结束，不能只验证数组顺序。

### 3.4 Demo 起点不足的处理

当提前后的 `specModeTick` 低于当前首段动作的安全下限（现有为 `DefaultActionTick+3`），不能同时满足原录制起点和完整两秒预热。不得用钳位掩盖问题，也不得静默延后录制、缩短预热、丢弃 take 或生成零时长录制。

首版建议在启用雷达的生成路径返回明确错误，指出具体 pass/source 和“Demo 起点不足以完成 POV 雷达两秒预热”，建议用户减少录制前置时间（`pre_seconds`）或关闭雷达重试。生成函数应在 JSON 落盘/历史登记前返回失败。接近 Demo 起点的用例必须单独测试；普通真实回合有足够准备 tick 时保持原窗口。未来若要容许降级，应另行定义用户可见的策略。

`seekTarget` 可以按现有规则钳位到 0，但钳位后的早期目标不生成 seek，改为从 0 正常播放；这可能缩短最初的 seek settle，但不得缩短雷达开启到录制之间的两秒。不要为该需求改变 Demo 结束补边、有效击杀或 match-end 规则。

### 3.5 两秒的含义和限制

本计划的两秒是 Demo 时间，在正常 `demo_timescale=1` 下对应约两秒运行时间。tick 计划不证明渲染已经完成；用户自定义 timescale、卡顿或引擎切换延迟会影响实际结果。首版不加游戏线程 sleep，不使用 `pause_playback` 的阻塞等待，因为暂停/阻塞不等于正常渲染预热。若必须保证墙钟时间或等待明确雷达就绪信号，需扩展插件调度协议，另立任务。

## 4. 分步实施任务

### 步骤 1：贯通逐 pass 的雷达选项

- 修改 `internal/clipsjson/builder.go`，将 `BuildOptions.PovRadarEnabled` 传到素材/pass 构建路径，可扩展现有私有 `buildPassCommandOptions`。
- 移除 `bootstrapOptions` 中仅为正式初始化雷达使用的字段及调用参数，删除 `buildBootstrapSequence` 的自动雷达分支。
- 复用 `PovRadarCommand(true)`；不改变 `gamesettings.go` 的调试开关输出，也不增加公开 DTO/配置字段。
- 搜索所有私有 helper 调用方和直接测试调用，同步参数语义。

### 步骤 2：实现可验证的准备时序

- 在无副作用的 clipsjson 层计算原录制窗口和雷达准备 tick，必要时提取小型私有时序 helper，避免在 App 层补命令。
- 关闭设置直接沿用旧路径。开启设置执行前述公式与起点校验，再生成严格有序的 mode、player、radar、start/end 动作。
- voice/xray 的逐 pass override 命令跟随新 `specModeTick`，仍在录制前执行，并覆盖继承和显式 override 场景。
- 若校验需要返回错误，逐级传回 `Build`/`BuildPlan`，保持失败时不写 JSON、不登记 take/history。
- `TakePlan.RecordStartTick/RecordEndTick` 必须来自实际生成的边界动作；source StartTick/EndTick 保留原值，kill offsets 按实际 start 计算。

### 步骤 3：按 pass 隔离开启雷达的素材 sequence

- 调整 `buildMaterialSequences` 或抽取已有 pass→sequence 组装逻辑，开启雷达时每 pass 一组，关闭时保留原分组。
- 保持 takeCounter、终止命令、类别顺序与已合并片段身份。
- 核对 `internal/plugingen/plan.go`、`internal/app/plugin_generate*.go`、`internal/producews` 对 sequences 和 take plans 的消费者；不得假定“killer 恒为一条 sequence”或“sequence 数等于素材数”。
- 前端若只是展示片段/take 数，不应因内部 sequence 数增加改变这些数量或完成投影。

### 步骤 4：测试、契约说明与交付

- 修改/补充 `builder_test.go`、`full_round_test.go` 以及必要的 plugingen/App 集成测试。
- 更新根 `AGENTS.md` 中 `pov_radar_enabled` 的稳定语义：正式制作逐 pass 切视角后开启，录制前至少两秒预热，关闭不发命令；按需要同步 `internal/AGENTS.md`。
- 不手工改 Wails 生成绑定；本方案没有新增接口，正常情况下无需前端或绑定修改。
- 完成下列验证后，提交实施结果、修改文件、测试结果，以及尚未通过的实机验收项。

## 5. 必需测试矩阵

| 场景 | 必须断言 |
| --- | --- |
| 默认/显式关闭雷达 | 所有 sequence 无自动 `csdm_radar_pov`；原窗口、动作 tick 和分组保持不变 |
| 单 killer 开启 | bootstrap 无自动雷达；素材中 mode < player < radar < start；start−radar 至少为两秒 tick |
| 同玩家多 pass | 每 pass 恰好一条自动开启命令，不因相同 target 去重 |
| 换玩家相邻 killer | 每 pass 独立 sequence；下一段准备不进入上一段录制 sequence；窗口和 take 身份不变 |
| 相邻/重叠整局 pass | 同样可独立准备，不靠“上一段结束”钳位缩短预热 |
| victim-only 与混合素材 | 三类都覆盖；整局→killer→victim 顺序、takeCounter 和最后 terminal 正确 |
| 视角 1/其他模式 | 不额外过滤 mode；生成实际 mode 后再开启雷达 |
| 同玩家合并击杀 | 原本合并的 segment 仍一个 take、一个 pass、一次开启；kill IDs/offsets 保留 |
| tickRate 64、128、有效非整数 | 用 Demo tickRate 换算并向上取整，不依赖录制 FPS；无效缺省按既有归一化处理 |
| 起点不足 / 恰好下限 | 不足明确失败，恰好可行生成成功；无负 tick，无静默缩短等待或漏 kill |
| seek 目标为 0、小于 96、等于 96、刚超过 96 | 前三类省略 seek，后一类正常前跳；按插件 tick 跨越规则模拟，包括 sequence 重置和 seek 后回放，所有段均只录制一次并完成 |
| 很短片段 / pre=0 / match-end 附近 | 有准备空间时窗口仍不变，end/terminal 在实际 start 之后；原结束钳位规则保持 |
| voice/xray override | 命令位于新准备时段、录制前，继承和显式值正确 |
| take/history/fast-edit 元数据 | 边界动作与 RecordStart/End 一致，KillOffsetsSeconds、source ID、历史键不因 sequence 拆分变化 |
| BuildPlan 与生成入口 | 冻结配置开关实际传到每 pass；起点错误向调用方传播，失败无生成副作用 |

关键断言应遍历所有素材 sequence 和所有 pass，不能只搜索“存在某条命令”。也不能只检查排序后的数组索引，要检查 tick 差与 sequence 归属。关闭路径的现有精确 tick 测试继续保留。

建议命令：

```sh
go test ./internal/clipsjson ./internal/plugingen ./internal/app ./internal/producews
go test ./...
go vet ./...
git diff --check
```

如果实施实际触及前端或公开接口，再按仓库要求运行 `frontend` 的 `npm test` 和 `npm run build`；不要仅为文档或私有 helper 调整生成绑定。

## 6. Windows / HLAE / CS2 实机验收

1. 保留一份开启、一份关闭雷达的生成 JSON，逐段对照命令 tick 与 take plan。
2. 使用当前插件，录制单第一视角片段，确认初次开启发生在实际切到目标后，输出第一帧雷达已正常。
3. 连续录不同玩家、同玩家重录、相邻短间隔片段；检查插件逐段收到 `csdm_radar_pov 1`，包括 duplicate 日志，以及切人后雷达内容对应新玩家。
4. 覆盖 killer、victim、整局 POV 与非第一视角模式，确认玩家视角和录制范围没有错位。
5. 验证每 pass 独立 sequence 时 `go_to_next_sequence→gototick 0→准备 seek` 正常推进，take 名不重置，无漏录/重复录制/队列卡住。
6. 对照开启与关闭输出，核对击杀仍在原位置、短片段没有丢失事件，最终合成与历史状态正确。
7. 分开测试普通 1x 播放与自定义 timescale；如果 1x 下两 tick 仍不足以完成视角切换，调整切换后命令间隔并复跑。两秒预热不足则记录实测，再调整独立常量。

源码与 Go 测试通过只建立开发门槛；雷达渲染、视角生效、sequence 跳转与 HLAE take 连续编号必须通过实机验收。若逐段重复 `1` 后仍失效，明确报告插件的 duplicate no-op 行为，后续再设计刷新动作，不能宣称此方案已修复所有 hook 状态问题。

## 7. 本次文档交付范围

本次仅创建此计划。没有修改业务代码、插件、配置或绑定，也没有运行应用测试/构建或 Windows 实机录制。实施模型应以当前 checkout 的源码和目录规则为准，按步骤完成后独立核对上述时序与验收条件。
