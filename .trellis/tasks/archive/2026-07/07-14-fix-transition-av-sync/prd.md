# 修复剪辑转场链路音画不同步与级联重编码音质劣化

## Goal

`ConcatEditClips`（`internal/app/app_edit.go`）带转场合并多片段时采用"两两串联 + 逐级重编码"，导致：xfade offset 账面时长误差逐级累积、xfade 与 acrossfade 转场点定位机制不一致、`-shortest` 不保证音画等长、硬切 `-c copy` 拼 AAC 引入 priming 错位与跨编码器坏文件风险、N 片段首片被有损重编码 N 次。将转场路径重写为**单条 filter_complex 一次编码**，消除不同步与多代劣化。

## Requirements

* 转场路径改为单条 ffmpeg 命令：所有片段在 filter graph 内预处理（scale/pad 到第一片段分辨率、fps 对齐、音频 apad+atrim 精确等长），xfade/acrossfade/concat 全部图内完成，音视频各仅编码一次。
* 后端不再信任前端传入的 `Duration`：转场路径一律用 ffprobe 视频流时长（stream=duration，N/A 回退 format），并对齐到帧网格后计算 offset。
* 硬切 gap 改用图内 `concat` filter，废弃 concat demuxer `-c copy` 拼接。
* retry chain 语义保留：整条命令按 profile 逐个重试。
* Wails 公开方法签名（`ConcatEditClips`、`ProbeClipDuration`）、`EditConcatRequest` JSON 结构、`compose_progress` 事件名不变。

## Acceptance Criteria

* [ ] 合并 3+ 片段（混合 fade/硬切）后，ffprobe 显示输出音视频流时长差 < 50ms；转场点画面与声音淡化同步，结尾无画面冻结。
* [ ] 音频全链路仅经历一次 AAC 编码；视频仅一次有损编码。
* [ ] 混入不同分辨率素材时正常合并（scale/pad 到首片段分辨率），不报 xfade 错误。
* [ ] `buildTransitionFilterGraph` 有纯函数单测覆盖（offset 算术、混排、校验错误路径）；`go test ./...` 通过。
* [ ] `compose_progress` 单阶段进度 0→100 正常上报。

## Definition of Done

* Tests added/updated（图构造纯函数单测 + probe mock 单测）
* `go test ./...` 通过（前端无改动，无需 npm build）
* PR 描述说明 Duration 字段语义变化（仅展示参考）

## Technical Approach

单条 filter_complex：每路输入 `scale/pad+setsar+fps+settb+setpts+format+trim`（视频）与 `aresample=async=1:first_pts=0+aformat+apad+atrim`（音频）强制音画精确等长 Di（帧网格对齐）；gap 依类型串 `xfade=fade:offset=curDur-d` + `acrossfade` 或 `concat=n=2:v=1:a=1`，`curDur` 纯算术推进（预处理保证与实际流一致，无累积误差）；`-map [v] -map [a]` 一次编码输出。
详细设计：`research/ffmpeg-transition-notes.md`；文件级改动步骤：`plan.md`。

## Decision (ADR-lite)

**Context**：级联架构是不同步与音质劣化的根因，需在"重写为单链一次编码"与"保留级联逐点修正"之间取舍。
**Decision**：采用 Approach A（单条 filter_complex，一次编码）；配套纳入分辨率/帧率归一与后端强制 re-probe。
**Consequences**：进度由多阶段退化为单阶段（按输出总时长上报百分比）；片段数很多时单命令资源占用高——分批合成（原方案 C）留作未来扩展点。

## Out of Scope

* 本任务不实施代码（计划由其他 AI 实施）。
* `concatSimple`（无转场路径）维持现状。
* 前端 `durationCache` 陈旧问题（后端已不依赖该值，仅影响 UI 展示）。
* 新转场类型（当前仅 fade）；超长时间线分批合成。

## Technical Notes

* 关键文件：`internal/app/app_edit.go`、`internal/app/edit_ffmpeg.go`、`internal/app/edit_progress.go`、`internal/app/app_edit_test.go`
* 编码参数来源：`internal/ffmpegprofile/profiles.go`（`BuildEditEncodeArgs` + retry chain，不改）
* 硬约束来自 CLAUDE.md：Wails 公开方法与事件名为稳定契约
* 时长来源现状：前端 `EditPage.vue` `durationCache` → request.Duration；后端 `resolveEditClips` 信任 >0 值（本次废除该信任）

## Research References

* [`research/ffmpeg-transition-notes.md`](research/ffmpeg-transition-notes.md) — xfade/acrossfade 语义差异、AAC 帧/priming 粒度、帧网格对齐与图内预处理设计依据
* [`plan.md`](plan.md) — 文件级改动步骤、测试清单、实施顺序与验证方法
