# 改动计划：转场合并重写为单条 filter_complex 一次编码

> 实施前先读 `research/ffmpeg-transition-notes.md`（设计依据）与 `prd.md`（范围/验收）。
> 硬约束：`ConcatEditClips` / `ProbeClipDuration` 是 Wails 稳定公开方法，签名不可变；`compose_progress` 事件名不可变；`EditConcatRequest` JSON 结构不可变（`Duration` 字段保留但后端不再信任其值）。

## 改动 1：ffprobe 视频流信息（`internal/app/edit_ffmpeg.go`）

新增：

```go
type probedVideoInfo struct {
    Duration float64 // 秒，视频流实际时长
    Width    int
    Height   int
}

func probeVideoStreamInfo(ffprobeExe, videoPath string) (probedVideoInfo, error)
```

- 一次 ffprobe 调用取全：`-v error -select_streams v:0 -show_entries stream=duration,width,height -show_entries format=duration -of json <path>`，用 `encoding/json` 解析。
- Duration 优先取 `streams[0].duration`；为空或 `"N/A"` 时回退 `format.duration`。两者都无效则报错。
- Width/Height ≤ 0 报错。
- 复用 `ffmpegCommand` 变量 + `configureNoWindowProcess`（保持现有 mock 注入方式，便于测试）。
- 现有 `probeDurationByFFprobe` 保留（`ProbeClipDuration` 公开方法仍用它，前端展示用途，不动）。

## 改动 2：`resolveEditClips` 强制 probe（`internal/app/app_edit.go`）

- `resolvedEditClip` 增加 `Width`、`Height` 字段。
- 不再信任 `clip.Duration`：对每个片段一律调 `probeVideoStreamInfo` 取实际视频流时长与分辨率（前端传入的 Duration 仅作为 probe 失败时的日志参考，不参与 offset 计算）。
- ffprobe 不存在时直接报错（原来仅 Duration≤0 时才要求 ffprobe；新逻辑转场路径必须有 ffprobe）。注意：`concatSimple` 路径不需要分辨率，但统一 probe 无害且顺带修正其时长——**本次仅在转场路径生效即可**，实现上可让 `ConcatEditClips` 在 `len(transitionByIndex) > 0` 时才走强制 probe，避免扩大 concatSimple 行为变化（concatSimple 属 Out of Scope）。

## 改动 3：帧网格对齐工具（`internal/app/app_edit.go`）

```go
func alignToFrameGrid(seconds float64, fps int) float64 // round(seconds*fps)/fps
```

- 片段时长 `Di` 与转场时长 `d` 都过一遍对齐；`d` 对齐后若 < 1/fps 视为无效（沿用现有 min/max 校验之后做）。

## 改动 4：核心重写 `concatWithTransitions`（`internal/app/app_edit.go`）

删除函数：`normalizeTransitionInputClip`、`applyFadeTransition`、`concatHardCutPair`（及其调用与 workDir 中间文件逻辑）。

新增纯函数（便于单测，不做 I/O）：

```go
type transitionGraphPlan struct {
    Filter        string   // 完整 filter_complex 字符串
    TotalDuration float64  // 输出总时长（进度分母）
}

func buildTransitionFilterGraph(
    clips []resolvedEditClip,          // 含 probe 后 Duration/Width/Height
    transitionByIndex map[int]EditConcatTransition,
    fps int,
) (transitionGraphPlan, error)
```

构图规则（细节见 research 笔记）：

1. 目标分辨率 W×H = 第一个片段的 Width×Height。
2. 每路输入 i 生成：
   - `[i:v]scale=W:H:force_original_aspect_ratio=decrease,pad=W:H:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=FPS,settb=AVTB,setpts=PTS-STARTPTS,format=yuv420p,trim=duration=%.6f,setpts=PTS-STARTPTS[v{i}]`
   - `[i:a]aresample=async=1:first_pts=0,aformat=sample_rates=48000:channel_layouts=stereo,apad,atrim=0:%.6f[a{i}]`
   - 时长值用对齐后的 `Di`，格式化用 `%.6f`（帧网格对齐后 6 位小数无损）。
3. 逐 gap 串联（`cur` 标签 + `curDur` 算术推进）：
   - fade gap：`[curV][v{i+1}]xfade=transition=fade:duration=%.6f:offset=%.6f[vx{n}]` + `[curA][a{i+1}]acrossfade=d=%.6f:c1=tri:c2=tri[ax{n}]`，offset = curDur - d，之后 `curDur += D_{i+1} - d`。
   - 硬切 gap：`[curV][curA][v{i+1}][a{i+1}]concat=n=2:v=1:a=1[vx{n}][ax{n}]`，之后 `curDur += D_{i+1}`。
   - 最终标签固定为 `[v]` / `[a]`。
4. 校验（构图前）：每个 fade gap 要求 `d < curDur（左侧累计）` 且 `d < D_{i+1}`；`Di < 2.0/fps` 报错。错误信息保持现有风格（`fmt.Errorf` + gap index + 具体数值）。

`concatWithTransitions` 重写为：

```go
plan := buildTransitionFilterGraph(...)
tracker.stageStart("合成输出")
for _, profile := range buildEditRetryProfiles(encode) {
    videoArgs := ffmpegprofile.BuildEditEncodeArgs(profile.ID, encode.Quality)
    args := ["-y", "-i", c0, "-i", c1, ..., "-filter_complex", plan.Filter,
             "-map", "[v]", "-map", "[a]", videoArgs...,
             "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", outputPath]
    // ffmpegCommand + configureNoWindowProcess + runFFmpegCommandWithProgress(cmd, plan.TotalDuration, tracker)
    // 成功 → stageDone 返回；失败 → 记录 lastOut/lastErr 换下一个 profile（与 concatSimple 现有重试模式一致）
}
```

- 不再有 workDir / 中间文件 / 最后的 `-c copy` finalize 步骤。
- 错误包装沿用现有格式：`fmt.Errorf("ffmpeg transition failed: %w: %s", lastErr, strings.TrimSpace(string(lastOut)))`。

## 改动 5：进度阶段数（`internal/app/app_edit.go`）

- `editComposeStageCount`：带转场时返回 `1`（原 `clipCount + (clipCount-1) + 1`）。签名不变。
- 检查 `internal/app/edit_progress.go` 中 tracker 对 stage 数的假设（`newComposeProgressTracker(a, 1)`），确认单 stage 下 `stageProgress/stageDone/complete` 百分比正确。

## 改动 6：测试（`internal/app/app_edit_test.go`）

删除针对已删函数的用例，新增：

1. `buildTransitionFilterGraph` 纯函数测试（重点，无需真 ffmpeg）：
   - 2 片段 + 1 个 fade：filter 串包含正确的 `xfade=...offset=`（offset = D0 - d，帧对齐值）与 `acrossfade`；
   - 3 片段混合（gap0 fade、gap1 硬切）：xfade 与 concat 混排、`curDur` 推进正确（TotalDuration = D0+D1-d+D2）；
   - 全硬切；
   - d ≥ 某侧时长 → 报错；片段过短 → 报错；
   - 分辨率不同的输入 → 每路都有 scale/pad 且目标为第一片段分辨率。
2. `probeVideoStreamInfo`：mock `ffmpegCommand` 返回 JSON，验证 stream duration 优先、`N/A` 回退 format duration、异常报错。
3. `resolveEditClips`：传入 Duration>0 时仍走 probe（转场路径）。
4. 现有 `normalizeEditTransitions` 测试不受影响，保留。

## 改动 7：文档/注释

- `ConcatEditClips` 方法注释更新：说明转场路径为单命令一次编码、Duration 字段仅作展示参考。
- 前端不改（API 兼容）。`EditPage.vue` 的 `durationCache` 只影响 UI 展示时长，后端已不依赖 → 在 PR 描述中说明即可。

## 实施顺序（建议 3 个提交）

1. **PR1**：改动 1 + 2 + 3（probe 基础设施 + 对齐工具）+ 对应单测。行为变化：转场路径时长来源变为 probe。
2. **PR2**：改动 4 + 5（核心重写 + 进度）+ 改动 6 单测。删除三个旧函数。
3. **PR3**：改动 7 + 真机验证（见下）+ 收尾。

## 验证（实施后必做）

- `go test ./...`
- 真机（Windows + 实际 ffmpeg）：合并 3+ 片段（混合 fade/硬切），然后：
  - `ffprobe -show_entries stream=duration` 检查输出音视频流时长差 < 50ms；
  - 目测转场点画面淡化与声音淡化同步、结尾无冻结；
  - 混入一个不同分辨率的素材验证 scale/pad 生效不报错；
  - `compose_progress` 进度条从 0 平滑到 100。
