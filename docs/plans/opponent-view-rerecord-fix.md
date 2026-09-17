# 补录缺失视角修复实施计划

## 0. 交接状态（实施者每阶段必须更新）

- 调研基线：`101c571`；开始调研时工作区干净。
- 当前阶段：**代码实现、自动化验证与主会话审查已完成；独立 review 工作流因超时未完成，Vue 交互与 Windows 录制仍未验证**。
- 本轮修改了前端完成索引、待制作投影、片段页状态展示、制作按钮初始化保护、前后端回归测试及协作规则说明；未修改后端生产代码。
- 自动化测试/构建已执行；真实 Vue 交互、Wails 事件桥、Windows 录制验证：**尚未执行**，不可视为通过。
- 目标：同一次软件启动中，先录 killer 后补 victim（以及反向补录）可正常进入制作；只录缺失视角，保留已有视频与历史。

| 阶段 | 状态 | 完成证据 / 后续填写 |
| --- | --- | --- |
| P0 代码链路与根因调研 | 完成 | 见第 1 节 |
| P1 按角色拆分完成记录与待制作投影 | 完成 | `frontend/src/domains/production/recordedViews.ts`；`frontend/tests/production-selectors.test.mjs` |
| P2 制作页和片段页一致性接入 | 代码完成，交互未验证 | production composable、制作按钮初始化保护与 ClipsPage/三个 clip 组件均复用角色状态；无截图/实机证据 |
| P3 前后端回归测试 | 完成 | `cd frontend && npm test`、`npm run build`、`env -u CS2_DEBUG_STARTUP_ADS go test ./...`、`go vet ./...` 均通过；新增前后端规划/过滤回归测试 |
| P4 Windows 实机验收 | 未验证 | 当前环境非 Windows；无录制产物、take 计划或历史实机证据 |
| P5 review 与文档收尾 | 主会话完成，独立 review 阻塞 | 主会话完成 diff/LSP/测试审查；最终两个独立 review 子任务随 2,700,000ms 工作流超时停止，未取得最终 reviewer verdict |

更新约定：只能按实际证据标记完成；无法执行则标为“阻塞/未验证”，写明原因。每次交接更新本节与末尾执行记录，禁止用“构建通过”替代交互/录制验证。

## 1. 已核实的原因与源码入口

### 1.1 主要阻断发生在前端，而不是后端不支持补录

`frontend/src/domains/production/selectors.ts`：

- `producedKillIDsByDemo` 将历史里的所有 `kill_ids` 放进 Demo 级集合，丢弃了 `view`。
- `pendingSelectionsByDemo` 只要发现 kill ID 已出现，就过滤掉整个素材，不看 `include_killer/include_victim`。

`frontend/src/features/produce/composables/useProducePage.ts`：

- 上述结果同时驱动 `displayDemos`、`hasPendingMaterials`、待制作分组和 `buildPendingBatchJobs`。
- 所以 killer 历史已存在时，即使重新勾选 victim，该素材也无法进入发送给后端的 `selected_items`。

### 1.2 片段页的标签也会误导，但并非全部都是禁用逻辑

`frontend/src/features/clips/pages/ClipsPage.vue` 重复构建不区分视角的 `producedKillIDsByDemo`，通过 `isKillAlreadyProduced` 提供状态。

消费者：

- `frontend/src/features/clips/components/ClipSelectionPanel.vue`
- `frontend/src/features/clips/components/MaterialListPanel.vue`
- `frontend/src/features/clips/components/MaterialSelectionList.vue`

素材列表用整体“已制作”标签表达部分视角已完成；对方视角复选框实际仅在自杀事件上禁用。`addableKills` 目前仅排除已选素材，并未按历史禁止重新添加。不要误把添加逻辑改成“历史素材禁止选择”。

### 1.3 后端已存在按视角过滤机制

- `internal/plugingen/helpers.go`：`BuildProduceHistoryKeyWithSourceID` 包含 Demo、view、spec mode、排序后的整组 kill IDs，以及可选 source ID。
- `internal/plugingen/filter.go`：`FilterItemsByHistory` 分别计算 killer/victim 保留集合，能将双视角输入裁成 victim-only。
- `internal/app/plugin_generate_launch.go`：`GeneratePluginJSONBatchAndLaunchHLAE` 先构建预览，读取历史键，过滤后重新生成。
- `internal/plugingen/filter_test.go`：已有 `TestFilterItemsByHistory_SeparatesKillerAndVictimViews`。
- `internal/app/produce_merge.go`：合并成功后才调用 `addProduceHistoryEntry`；take 的 `recorded` 或 WS 的 completed 不等于产物成功。
- `internal/app/produce_takefile.go`：历史条目已有 `view/spec_mode/kill_ids`，历史为当前进程内状态；本次无需新增数据库或改持久化。

**结论：优先修正前端历史投影与状态展示，后端保持防重兜底；不要清空历史绕过问题。**

## 2. 需求边界和决策

1. 完成情况按 `Demo + kill ID + 录制角色 + spec mode` 判断；主/对方仅是 UI 映射，不可把“主视角”当固定 killer。
2. 用户选择状态保持不变；待制作列表是其派生副本，只把已经成功的角色裁掉。不要原地改写 `getMaterialSelections` 返回的共享对象。
3. 只有成功制作历史能构成完成事实；失败、等待文件、处理中、仅生成 JSON 不构成成功历史。
4. `history_type` 缺省仍按 `produce_clip`；显式非录制历史（包括 edited_video）不参与。`full_round_pov` 不参与普通击杀视角完成索引。
5. 当前 `jobs.ts` 固定发送两个 spec mode 为 1，索引判断按该实际请求模式匹配，不能把其他模式历史当已完成。缺省模式可按 1 兼容；未知/空 view 不推断成“双视角完成”，保守地不阻断制作。
6. 自杀仍按已有规则不允许开启对方视角；SteamID 继续用字符串；不新增状态机阶段。
7. 不增加“强制重录已完成视角”功能，不自动清空历史，不根据视频路径做磁盘扫描，不修改录制窗口、插件协议或 HLAE 生命周期。
8. 本次不修复整局 POV 待制作显示的其他潜在问题，也不重定义“已生成 N”统计：它仍统计成功 take，而不是 kill 数或视角数。

### 后端键粒度的注意事项

前端投影描述“某个击杀角色已被成功视频覆盖”；后端防重键描述“某个完整 take（可能覆盖多个击杀）”。二者不能宣称完全等价。

本次沿用前端按击杀剔除已覆盖素材的既有产品语义，仅细分到角色。历史 killer take 包含 k1/k2 时，补录两者 victim 应只发 victim；只补 k2 也应只发 victim。不要为追求键一致而在前端重写窗口合并算法，也不要顺手把后端整组键改成单 kill 键。对直接后端调用时“选择集合变化导致 take 键变化”的行为，本次不扩大修复范围。

## 3. 实施步骤

### P1：统一领域完成索引与待制作投影

建议在 `frontend/src/domains/production/` 增加独立纯 TypeScript helper（例如 `recordedViews.ts`），或明确组织在现有 `selectors.ts` 中；要求片段页和制作页复用同一实现，不再复制集合算法。

建议提供以下能力（函数命名可调整，语义不可改变）：

- 从 `ProduceHistoryItem[]` 构建按 Demo/kill/role/spec mode 索引的完成事实。
- 查询某素材 killer/victim 各自是否完成。
- `pendingSelectionsByDemo` 对每个素材返回派生副本：
  - `pendingKiller = (include_killer !== false) && !killerCompleted`
  - `pendingVictim = Boolean(include_victim) && !victimCompleted`
  - 两者都 false 才丢弃素材。
  - 返回副本显式写入两个布尔值，尤其 victim-only 必须 `include_killer=false`。
  - 保留 `primary_view`、`kill`、`clip_overrides` 及其他元数据；不能把 primary_view 改成当前待录角色，否则窗口映射会变化。
- 索引只接受明确 killer/victim；去重历史、多 kill take、跨 Demo 同名 ID 均有确定行为。
- 不使用一个布尔值混合表达“任一视角完成”与“当前请求全部完成”。

如果新增运行时相对导入，按仓库 Node ESM 测试要求使用可解析的 `.js` 路径，并检查测试编译入口。避免引入 Vue 依赖到纯 helper。

### P2：接入页面与准确展示

**制作页**

- 修改 `useProducePage.ts` 使用新完成索引；清理或重命名旧的 `producedKillIDsByDemo` 暴露项及消费者。
- `pendingSelectionsForDemo`、按钮可用性、计数、分组和 `buildPendingBatchJobs` 必须共同使用同一份裁剪结果。
- `frontend/src/domains/production/jobs.ts` 保留字段透传；核实不会把显式 false 恢复为 true。
- `frontend/src/features/produce/pages/ProducePage.vue` 已按 `isPrimaryIncluded/isOpponentIncluded` 显示标签，裁剪后应自然只显示缺失视角；核对死亡模式。
- `captureCurrentKillSnapshot` 继续为剩余 take 保存击杀元数据。
- 不改活动会话/失败收尾保护，也不为刷新显示提前清除活跃 batch/take 状态。结束后返回片段再进入制作沿用现有 reset 生命周期。

**片段页**

- `ClipsPage.vue` 移除重复的 kill-only 历史聚合，复用完成索引。
- 将上述三个组件的历史状态 props 改为能表达分角色完成的类型，不继续用歧义 `isKillAlreadyProduced`。
- 推荐标签明确显示“击杀者视角已制作”“被害者视角已制作”（都完成可显示两个标签），与现有主/对方选择标签区分；避免“已制作”被理解为所有视角完成。
- 保留已完成素材的编辑/重新添加能力，否则无法开启尚未录制的对方视角。
- 仅修改 `frontend/src/shared/i18n/zh-CN.json`；不修改英文翻译或生成绑定。
- `producedTakeCountByDemo` 继续包括 full_round_pov 的成功 take。

**后端**

默认不修改生产代码。先补充测试确认已有过滤正确；如发现与上述判断不同的真实失败，先记录复现与原因再最小修复，不能借机重写历史键/状态机。

### P3：自动化回归与规则更新

- 新增 `frontend/tests/production-selectors.test.mjs`（或等价独立文件）覆盖 helper 与 pending 投影。
- 扩展 `frontend/tests/production-jobs.test.mjs`，验证“历史 → pending → jobs”组合链路，不能只分别测两个孤立函数。
- 参考现有 Node 测试加载方式；新 helper 有运行时依赖时不能直接沿用仅适合无依赖模块的 data URL transpile。必要时接入现有运行时编译配置，并加相应类型测试入口。
- 后端扩展 `internal/plugingen/filter_test.go`、`internal/app/plugin_generate_test.go` 的适当测试，验证 `BuildPlan → history filter → BuildPlan`；用纯规划/已有 seam，不真实启动 HLAE。
- 在根 `AGENTS.md` 和 `frontend/AGENTS.md` 的相关章节简述普通片段完成判断按角色区分；仅在实际改变后端契约时更新 `internal/AGENTS.md`，不追加流水账。

## 4. 必须覆盖的测试矩阵

| 输入/历史 | 预期 |
| --- | --- |
| 无历史，选择 killer | 仅 killer |
| 无历史，选择双视角 | 双视角 |
| killer 已成功，选择双视角 | 仅 victim，include_killer 显式 false |
| victim 已成功，选择双视角 | 仅 killer |
| killer 已成功，仅选择 killer | 无普通待制作素材 |
| killer/victim 均成功，选择双视角 | 无普通待制作素材 |
| killer 成功，victim 失败/处理中 | victim 仍可在会话空闲后重试 |
| primary_view=victim，victim 完成后开启对方 | 补 killer；primary_view 保持 victim |
| 历史 killer take 覆盖 k1/k2，选择二者双视角 | 两项均仅保留 victim |
| 上一行仅重新选择 k2 | 仅 k2 victim |
| 相同 kill ID，不同 Demo | 不串记录 |
| edited_video/full_round_pov/未知 view 历史 | 不吞掉普通待录角色 |
| spec mode 不匹配 | 不标为当前模式已完成 |
| 缺省 history_type、兼容 include_killer 缺省 | 延续旧字段默认语义 |
| clip_overrides + primary_view + kill 元数据 | 投影/组 job 后不丢失、不改写原选择 |
| 重复历史条目与连续重复计算 | 结果幂等，不原地修改输入 |
| 只生成 JSON 后再启动 | 不因生成配置被当成已录制 |
| 补录成功后历史更新 | 新角色不再待制作，旧历史/视频仍保留 |

额外后端断言：第二次规划 take 的 view 只有缺失角色；已存在 killer 的双视角输入不会产生 killer take；双角色历史齐全时过滤为空；多 kill 合并 take 情形不破坏 source ID/full-round 既有测试。

验证命令（记录实际输出，不预填通过）：

```sh
go test ./...
cd frontend && npm test
cd frontend && npm run build
# 返回仓库根目录
git diff --check
```

如修改后端规划实现，另执行 `go vet ./...`。新增/修改 TS 文件应进行可用的主动 LSP 诊断；无服务时记录未覆盖，不代替上述必需命令。

## 5. 交互与 Windows 实机验收

1. 导入含有效击杀的 Demo，选择一条普通击杀，仅录主视角（killer）。
2. 等待视频合并成功及制作收尾完成；记录历史 view、video_path、take plans。
3. 返回片段页，保留原素材，开启对方视角；应明确看到 killer 已完成、victim 尚缺。
4. 进入制作页：该素材存在，只显示待录 victim；按钮在工作状态确认空闲后可用。
5. 启动补录：生成计划及实际插件录制均只有 victim；旧 killer 文件不被覆盖/删除，历史保留两种视角。
6. 再次进入制作：该普通素材无待录角色。
7. 反向验证死亡模式先 victim 后 killer；验证删除选择后重新添加，以及同一合并 take 的多击杀/子集补录。
8. 失败路径：victim 制作失败不形成成功历史，收尾完成后仍可重试；收尾失败时必须维持忙碌/重试保护。
9. 核对现有页面导航、工作目录初始化/切换、状态未加载或查询失败时按钮禁用，以及整局 POV 和历史抽屉无回归。

非 Windows 环境只能完成领域测试/构建及部分 UI 验证，不能声称完成 CS2/HLAE 注入录制与恢复。真实 Wails 事件桥亦需另验。

## 6. Review 检查点与完成标准

- [x] 不再按 kill ID 把双角色整项过滤；未知 view 不产生错误完成事实。
- [x] 只发缺失视角，victim-only 显式 false，死亡模式不反转 primary_view。
- [x] 状态展示和请求来自同一投影，无另一套片段页历史算法。
- [x] 成功历史是依据，不能用 WS completed 或 take recorded 代替。
- [x] 旧素材仍可添加/配置，所有生成入口都被核查。
- [x] 没有改历史键语义、插件协议、窗口算法、busy/cleanup 防护。
- [x] 有不修改原选择、合并 take、反向补录的测试。
- [x] 自动化结果及未验证项已写入进度；Windows 验收仍未完成。
- [x] 本计划和适用 AGENTS 与最终实现一致。

只有实现、自动化与实机验收都具备证据才标记“全部完成”；可先标记“代码完成，等待 Windows 验收”。

## 7. 执行记录（追加，保持状态表同步）

### 初始调研交接

- 已阅读根/前后端协作规则、制作 selectors/jobs/composable、片段页历史标签与选择逻辑、视角映射，以及后端规划、历史过滤与合并成功登记链路。
- 已确认前端丢失 view 是阻断原因；后端已有分角色过滤能力。
- 尚未实现修复、未执行测试/构建、未做真实 UI/Wails/Windows 验证。

### 本轮实施记录

- 基线/提交：`101c571`（未提交）
- 本轮完成阶段：P1、P2 代码接入、P3 自动化验证、P5 主会话审查；P4 未验证，独立 review 工作流阻塞。
- 改动文件与关键决策：
  - 新增 `frontend/src/domains/production/recordedViews.ts`，以 Demo + kill ID + 角色 + spec mode 建立成功 `produce_clip` 索引；要求非空 `video_path`，忽略编辑/整局/未知视角和不匹配模式。
  - `pendingSelectionsByDemo` 对 killer/victim 分别裁剪并返回副本；victim-only 明确写入 `include_killer=false`，保持 `primary_view`、kill 元数据和 overrides。
  - production composable、ClipsPage 与三个片段组件统一使用该状态；制作按钮在历史快照未成功初始化时保持禁用；保留可重新配置/添加素材、self-kill、full-round 统计和后端历史键/协议。
  - 新增 `internal/plugingen/filter_test.go` 与 `internal/app/plugin_generate_test.go` 的 `BuildPlan → history filter → BuildPlan` 角色补录测试；未修改后端生产代码或生成 Wails 绑定；仅新增中文角色状态标签。
- 测试命令、结果、证据位置：
  - `cd frontend && npm test`：通过（59 tests）。
  - `cd frontend && npm run build`：通过（`vue-tsc --noEmit` 与 Vite build，保留既有大 chunk warning）。
  - `env -u CS2_DEBUG_STARTUP_ADS go test ./...`：通过。
  - `go test ./...`：按要求执行但因继承环境 `CS2_DEBUG_STARTUP_ADS=1` 失败；`internal/envsetup/TestRunStartupChecks_PopulatesSupportedAdsIntoState` 预期 2 条广告、实际读到 5 条；取消该环境变量后通过。
  - `env -u CS2_DEBUG_STARTUP_ADS go vet ./...`：通过；`env -u CS2_DEBUG_STARTUP_ADS go test ./internal/plugingen ./internal/app`：通过。
  - `git diff --check` 与未跟踪计划文件的 `git diff --no-index --check`：通过。
  - 主动 LSP 错误诊断：0 errors；部分 TypeScript server 为 push-only，未能对全部文件确认 clean，剩余为既有/风格提示而非错误。
- UI/Windows 验证：未执行；当前环境非 Windows，未声称 Wails/CS2/HLAE 注入、录制、恢复或真实交互通过。
- review 发现与处理：只读调研报告确认后端生产过滤已按角色工作；主会话审查补充了“非空成功视频才计入历史”、后端规划/过滤回归测试和历史初始化失败时禁用制作按钮。最终两个独立 review 子任务未完成，所属工作流在 `2,700,000ms` 超时后停止，未将其结果作为通过证据。
- 阻塞/剩余风险/下一步：需在 Windows 完成两次视角补录、反向补录及失败重试验收，并在条件允许时完成独立 review；真实 Vue/Wails 事件桥、CS2/HLAE 注入录制和环境恢复仍未验证。
