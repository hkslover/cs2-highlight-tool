# 仅显示当前视角击杀信息（POV kill feed filter）

## 目标

在设置中增加 `pov_kill_feed_only`（仅显示当前视角下的击杀信息）：正式录制时，每个录制 pass 只保留该 pass 视角玩家参与（击杀、被击杀、助攻）的 HLAE 击杀提示，其余玩家的提示被屏蔽；调试工作台提供等价的运行时快捷指令。

## HLAE 能力与约束（调研结论）

来源：HLAE wiki `Source2:mirv_deathmsg`、`Source:mirv_deathmsg`、`AfxHookSource2/DeathMsg.cpp`、`shared/MirvDeathMsgFilter.h`。

- CS2 支持 `mirv_deathmsg clear`、`filter clear/add`、`localPlayer`、`lifetime`、`lifetimeMod`、`colors`、`debug`、`help`；`fake` 仅 CS:GO 支持。
- 过滤规则在 `filter add` 时解析为静态的 `DeathMsgId`。`xTrace` 在**命令执行时**解析为当前被观战玩家，之后不会再随视角变化。
- 规则匹配语义：`attackerMatch`/`victimMatch`/`assisterMatch` 三个条件同时满足才命中；`!` 表示“不是该玩家”；命中后应用 `block=1`，`lastRule=1` 停止后续规则评估；未命中则不过滤。
- 因此“只保留本人参与”的稳定写法是逐 pass 用显式 Steam64：

  ```text
  mirv_deathmsg clear
  mirv_deathmsg filter clear
  mirv_deathmsg filter add attackerMatch=!x<ID> victimMatch=!x<ID> assisterMatch=!x<ID> block=1 lastRule=1
  ```

- `filter clear` 清空整张过滤列表，所以不能把 `block_kill_feed`（`filter add block 1`）与逐 pass POV 过滤混用：逐 pass 的 clear 会撤销 block。后端约定 block 优先。
- `mirv_deathmsg clear` 清掉屏幕上已存在的提示，避免上一个 pass/上一个视角的提示进入本段录制。

## 实现

| 层 | 文件 | 内容 |
| --- | --- | --- |
| 命令构建 | `internal/clipsjson/gamesettings.go` | `PovKillFeedCommands(steamID)`（正式、显式 XUID）、`PovKillFeedTraceCommands(enabled)`（调试、`xTrace`）、`GameSettingKeyPovKillFeedOnly` |
| 生成规划 | `internal/clipsjson/builder.go` | `killSegment/clipPass` 携带 `PlayerSteamID`；`Build` 计算 `PovKillFeedOnly && !BlockKillFeed`；逐 pass 在 `spec_player` 同 tick 写入过滤命令 |
| 设置 | `internal/config/config.go`、`internal/app/clip_settings.go` | `pov_kill_feed_only` 默认 false，Get/Save 往返 |
| 快照 | `internal/plugingen/plan.go`、`internal/app/plugin_generation_plan.go` | 生成快照透传 |
| 调试计划 | `internal/app/debug_settings.go`、`internal/clipsjson/gamesettings.go` | 计划行 `pov_kill_feed_only`，`on` 使用 `xTrace`，`off` 为 `filter clear` |
| 前端 | `settings-schema.ts`、`settings-state.ts`、`DebugSettingsCore`、`debugQuickCommands.ts`、`SettingsPanel.vue` | 开关、默认值、调试快捷指令；设置页互斥：开启一个关闭另一个 |
| 文案 | `frontend/src/shared/i18n/zh-CN.json` | 设置标签/hint、调试快捷指令标签 |

## 语义

- 正式 Build：每 pass 在 `spec_player` 后、`mirv_streams record start` 前发送 clear + filter clear + 该玩家显式 XUID 规则；整体序列分组与原有 tick 时序不变。
- 整局 POV 使用 `full_round_pov.player_steam_id`；killer pass 用 `killer_steam_id`，victim pass 用 `victim_steam_id`。
- 无有效 Steam64（空、`0`、非数字）时只发送 clear + `filter clear`，避免沿用上一个 pass 的规则而隐藏当前视角提示。
- `block_kill_feed` 与 `pov_kill_feed_only` 同时为 true 时 block 优先，不发送逐 pass 过滤；设置页也做互斥，但不改写后端已持久化的冲突值。
- 调试工作台的 `xTrace` 规则在发送时解析当前观战玩家，切换视角后需要重新发送快捷指令；`off` 与 block 的 off 都是 `mirv_deathmsg filter clear`，会清空整张过滤列表，UI 须保留该提示。block 开启时调试计划不输出该行的 current 命令，避免 `apply_all` 的 `filter clear` 撤销 block 规则。
- 调试 `xTrace` 在没有当前观战玩家时解析为 0，规则会屏蔽全部提示；此时应切回玩家视角重发，或发送“击杀信息：显示全部”恢复。

## 验证

- `go test ./internal/clipsjson/`：逐 pass XUID、整局 POV、block 优先级、无 XUID 回退、关闭不写命令、调试计划的 on/off 与 apply-all 顺序。
- `go test ./internal/app/`：设置保存/读取往返、GeneratePluginJSON 全链路、block 抑制 POV 过滤。
- `cd frontend && npm test`、`npm run build`。
- Windows 实机未验证：需要确认 CS2 当前版本下 `mirv_deathmsg filter add ... lastRule=1` 的实际屏蔽效果、`mirv_deathmsg clear` 的时序、助攻提示是否计入，以及 `spec_player` 同 tick 发送过滤命令是否稳定生效。
