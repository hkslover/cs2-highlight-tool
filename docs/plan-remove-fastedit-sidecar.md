# 移除录制视频的 `.fastedit.json` 副产物

## 目标与范围

停止在正式录制视频旁生成 `<videoPath>.fastedit.json`。当前程序没有重新导入视频并从该文件恢复击杀时间点的功能，因此移除这项磁盘写入，同时保留现有录制历史、剪辑序列和受害者视角快节奏剪辑功能。

本次交付仅为实施计划，不修改业务代码。实施时不增加配置开关，不引入元数据重新导入功能，不修改插件协议、Wails 方法或生成绑定。

本计划中的“删除”指移除生成逻辑及其专属代码、测试和规则描述。既有输出目录中的 `.fastedit.json` 不自动扫描或删除；历史文件清理不纳入本次实施，避免额外文件操作和所有权判断。

## 已核实的现状

| 位置 | 当前行为 | 实施要求 |
| --- | --- | --- |
| `internal/app/produce_takefile.go` 的 `addProduceHistoryEntry` | 构建 `ProduceHistoryItem`，尝试写旁车，再登记内存历史；写入不区分 killer、victim 或整局 POV | 仅移除旁车调用，保留历史构建和登记 |
| 同文件的 `produceTakeMetadataSuffix`、`produceTakeMetadataPath`、`writeProduceTakeMetadataSidecar` | 将完整历史项序列化为 JSON，并写入 `<videoPath>.fastedit.json` | 删除旁车专属常量和 helper，清理失去用途的 import |
| `internal/app/app_edit_test.go` 的 `TestWriteProduceTakeMetadataSidecar_PreservesTimingFields` | 验证旁车保存录制时序字段 | 移除被取消功能的测试，保留或补充历史元数据验证 |
| `frontend/src/domains/edit/fastEdit.ts` | 从 `EditSequenceItem.historyItem` 读取时序和击杀信息，仅处理符合条件的 victim 素材 | 保持算法和输入契约 |
| `frontend/src/domains/production/productionStore.ts` | 通过 `GetProduceHistorySnapshot` 获取录制历史 | 保持历史数据来源 |
| 根 `AGENTS.md`、`internal/AGENTS.md` | 描述录制成功后的旁车写入及 best-effort 约束 | 更新为当前历史元数据用途，删除已失效的旁车生成描述 |

当前源码中的旁车读取只出现在专属写入测试中，生产路径没有旁车读取入口。实施前应再次检索确认，避免后续源码变化引入新消费者。

## 实施步骤

### 1. 确认基线与消费者

- 阅读根目录和目标目录的 `AGENTS.md`，检查 `git status`，保留无关改动。
- 全仓检索 `.fastedit.json`、`produceTakeMetadataSuffix`、`produceTakeMetadataPath`、`writeProduceTakeMetadataSidecar`，确认写入入口与消费者仍符合上表。
- 核对 `addProduceHistoryEntry`、`GetProduceHistorySnapshot` 与前端 `historyItem` 的数据流，确认快速剪辑所需字段来自内存历史。

### 2. 移除磁盘副产物生成

- 删除 `addProduceHistoryEntry` 中的旁车说明注释和 `writeProduceTakeMetadataSidecar` 调用。
- 删除旁车专属常量及两个 helper。
- 按实际剩余用途清理 import，并执行 `gofmt`。
- 保留 `ProduceHistoryItem` 的全部原有字段，以及 `resolveHistoryKills`、击杀偏移计算、`appendProduceHistoryItem` 和历史事件发射。
- 不修改正式录制、合并、视频命名、源文件清理、编辑成片登记和错误处理流程。

### 3. 调整测试与规则文档

- 删除 `TestWriteProduceTakeMetadataSidecar_PreservesTimingFields`，清理仅为该测试使用的 import。
- 检查已有历史测试是否覆盖 `tick_rate`、`record_start_tick`、`record_end_tick`、`kill_offsets_seconds` 与击杀事实；如果没有，补充调用真实 `addProduceHistoryEntry` 并从历史快照断言这些字段的测试，确保移除落盘不会误删剪辑输入。
- 测试使用临时目录和有效历史输入，不启动游戏或 FFmpeg；沿用现有 App/runtime 测试初始化方式。
- 在该真实历史登记测试中确认对应 `<videoPath>.fastedit.json` 未生成。若已有测试覆盖历史字段，优先扩展现有测试，不新增重复测试。
- 更新根 `AGENTS.md` 和 `internal/AGENTS.md` 中与旁车生成有关的描述，继续保留录制时序元数据和快节奏剪辑约束。
- 前端功能与规则无需为此改动；不手工修改自动生成文件或 i18n。

### 4. 验证与交付

- 执行 `go test ./...`，确认后端测试通过。
- 执行 `cd frontend && npm test`，作为历史元数据消费者的回归检查，重点确认 `edit-fast.test.mjs` 中 victim 裁剪、killer 保持整段和缺标记回退用例通过。
- 执行 `git diff --check`，核对最终 diff 仅包含旁车移除、相关测试与规则文档。
- 再次检索旁车 helper，确认无遗留调用；`.fastedit.json` 可以保留在计划、否定性测试或历史说明中，不要求字符串完全消失。
- 汇报修改文件、实际验证结果与未验证项。若实施范围扩展到前端源码，按仓库规则另行执行 `npm run build`；本计划不要求为纯后端移除修改前端源码。

## 验收条件

1. 新完成的正式录制视频不再产生 `.fastedit.json`，单视角与多视角录制使用相同规则。
2. 录制视频仍按原流程合并并登记成功历史，输出路径和内容不因本次变更改变。
3. 内存历史及快照保留原有击杀和录制时间字段，前端快节奏剪辑继续消费这些字段。
4. 受害者视角快节奏剪辑默认关闭；开启后的裁剪条件、分组和硬切规则保持原行为，killer、整局 POV 与缺标记素材保持整段。
5. 既有 `.fastedit.json` 和其他输出文件不被本次实施自动删除。
6. 后端全量测试、前端领域测试与 diff 检查通过；规则文档不再声明当前程序会生成旁车。

## 验证边界

源码与单元测试可确认磁盘写入入口已移除、历史数据契约保留和剪辑规划回归通过，不能证明 Windows 上 HLAE/CS2 的真实录制成功。Windows 实机可补充一次正式录制检查：视频生成且可播放、目录不新增旁车、历史可加入剪辑序列；使用符合条件的 victim 素材确认快节奏剪辑仍能导出。未执行实机检查时须在交付说明中明确记录。

当前方案不新增历史持久化或重启恢复能力。已有的内存历史生命周期保持原样。
