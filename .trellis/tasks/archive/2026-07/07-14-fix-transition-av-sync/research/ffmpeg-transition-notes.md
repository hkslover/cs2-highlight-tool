# FFmpeg 转场链路技术背景（实施必读）

实施者注意：本文解释"为什么这样设计"，与 `plan.md` 配套。

## 现有实现的缺陷根因

1. **xfade vs acrossfade 定位机制不同**：`xfade` 的转场点由显式 `offset`（相对第一路视频时间轴）决定；`acrossfade` 没有 offset 参数，固定重叠**第一路音频实际结束前的最后 d 秒**。因此只要某片段音频长度 ≠ 视频长度，两个转场点就错开。
2. **AAC 粒度**：AAC 帧 = 1024 samples（48kHz ≈ 21.3ms），编码后音频时长向上对齐到帧边界；另有 priming（编码器延迟，通常 2048 samples ≈ 42.6ms），在 MP4 中靠 edit list 表达。
3. **concat demuxer + `-c copy` 拼 AAC**：concat demuxer 忽略第二段的 edit list → 每个拼接点引入 ~20–45ms 音频错位；且要求两输入视频流编码参数完全一致，retry chain（hevc_nvenc→h264_nvenc→libx264）可能让相邻文件参数不同 → "成功"但花屏/坏文件，不触发 fallback。
4. **级联误差累积**：旧实现 `currentDuration = cur + next - d` 纯算术推进、从不 re-probe 中间产物；实际时长受 fps 帧对齐、AAC 帧对齐、`-shortest` 截断影响，每级偏差带入下一级 offset。
5. **多代有损编码**：N 片段全转场时第 1 个片段被 AAC 192k / 视频有损重编码 N 次。

## 目标方案（单条 filter_complex，一次编码）要点

### 时长与网格对齐

- 用 ffprobe 取每个输入**视频流**时长（`-select_streams v:0 -show_entries stream=duration`），不要用 `format=duration`（容器时长含音频尾部 padding，普遍偏大）。部分容器 stream duration 为 `N/A`，需回退 format duration。
- 将每个片段时长对齐到帧网格：`Di = round(rawDi * fps) / fps`。转场时长 d 同样对齐。这样所有 offset 都是整数帧，且 48000/fps 在 60/120fps 下是整数采样（800/400），音频可采样级对齐。

### 每路输入预处理（graph 内，无中间文件）

视频（第 i 路）：
```
[i:v]scale=W:H:force_original_aspect_ratio=decrease,
     pad=W:H:(ow-iw)/2:(oh-ih)/2,setsar=1,
     fps=FPS,settb=AVTB,setpts=PTS-STARTPTS,format=yuv420p,
     trim=duration=Di,setpts=PTS-STARTPTS[vi]
```
W×H 取第一个片段的分辨率（probe width/height）。xfade 要求两输入分辨率、fps、timebase 完全一致，缺 scale 时混入异分辨率素材会直接报错。

音频（第 i 路）：
```
[i:a]aresample=async=1:first_pts=0,
     aformat=sample_rates=48000:channel_layouts=stereo,
     apad,atrim=0:Di,asetpts=PTS-STARTPTS[ai]
```
- `aresample=async=1:first_pts=0`：源音频起始 PTS 非 0 时**补静音而不是平移**，保住原始 A/V 相对偏移（旧实现 `asetpts=PTS-STARTPTS` 双边各自归零会破坏它）。
- `apad + atrim=0:Di`：强制音频长度精确等于 Di（旧实现 `-shortest` 只能"截到较短流"，不保证等长）。

预处理后每路满足：视频长 = 音频长 = Di（帧/采样级），这是 xfade 与 acrossfade 转场点重合的充分条件。

### gap 串联（纯算术，无累积误差）

维护 `cur` 标签与 `curDur`（初值 D0）：
- 有转场（fade）：
  ```
  [curV][v_{i+1}]xfade=transition=fade:duration=d:offset=(curDur-d)[vxN];
  [curA][a_{i+1}]acrossfade=d=d:c1=tri:c2=tri[axN]
  ```
  `curDur += D_{i+1} - d`
- 硬切：
  ```
  [curV][curA][v_{i+1}][a_{i+1}]concat=n=2:v=1:a=1[vxN][axN]
  ```
  `curDur += D_{i+1}`

xfade 与 concat filter 可在同一张图内混用。由于所有输入都在图内预处理成精确 Di，算术推导的 offset 与实际流完全一致，无级联误差。

### 输出（仅此一次编码）

```
-map [v] -map [a] <BuildEditEncodeArgs(profile, quality)> -c:a aac -b:a 192k -movflags +faststart -y out.mp4
```
retry chain 对**整条命令**逐 profile 重试（与现有 concatSimple 的重试模式一致）。qsv profile 的 `-pix_fmt nv12` 会在 graph 输出 yuv420p 后再转一次，concatSimple 现状即如此，可接受。

### 校验规则（用对齐后的 Di）

- acrossfade/xfade 要求参与转场的两路时长均 > d：沿用现有检查，但左侧用**累计 curDur**、右侧用 D_{i+1}，且都基于 probe+对齐后的值，不信任前端传入 Duration。
- 片段极短（Di < 2 帧）直接报错，避免 fps/trim 产出空流。

### 进度

单条命令 → 单 stage。`-progress pipe:1` 的 out_time 对照最终 `curDur`（总输出时长）换算百分比，复用 `runFFmpegCommandWithProgress`，`editComposeStageCount` 带转场时返回 1。

## 已知边界

- 片段数很多（>10）时 filter graph 大、内存占用高：本次不处理，未来可按组分批（PRD 方案 C）为扩展点。
- 视频流起始 PTS 非 0 而音频为 0 的素材：`setpts=PTS-STARTPTS` 仍会平移视频，极端素材可能残留固定偏移；本工具产物视频从 0 开始，接受此限制。
