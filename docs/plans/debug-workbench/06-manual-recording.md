# 06：手动录制、停止与音视频合并

前置：05。首版一次一段，合并结束再开始下一段；停止录制不退出游戏。

## 插件协议补齐

| 消息 | payload/行为 |
| --- | --- |
| `debug_record_start` | `{request_id,recording_id,demo_path,take_name,commands:string[]}`；commands 含录制初始化与最后的 start，游戏线程顺序执行 |
| `debug_record_stop` | `{request_id,recording_id}`；结束当前段，不执行 quit/disconnect |
| `debug_record_status` | `{session_id,recording_id,demo_path,take_name,record_phase:"start"|"end",tick,ts_ms}`；实际调用录制边界命令后由游戏线程发出 |

操作复用 03 的 `debug_command_result` 回执。`debug_record_status` 必须与当前 recording_id 对齐，不进入正式 `record_status` 的 take/history 状态。边界回执只证明录制命令已交给 HLAE，完整产物以文件等待和 ffprobe 为准。

插件对当前 debug recording 保存一个小型活动记录；start/stop 必须在游戏线程更新，不依赖 Demo tick 变化，暂停播放时按钮仍可用。不为首版建立任务队列数据库。

## 桌面端实施

1. `StartDebugRecording` 要求插件 ready 且 Demo 正在播放，当前没有录制/停止/合并任务。冻结 Demo、实际启动分辨率、本段 FPS、record_quality、实际编码 preset。受管控路径与 FFmpeg 探测复用已有机制。
2. 分配唯一 `<outputs>/debug/<session-id>/<recording-id>/`，本段 record name 指向该目录下的唯一前缀。目录命名用现有唯一 ID 方式，不靠秒级时间戳防重名。
3. 复用 05 提取的录制初始化 helper，命令包括 screen enabled、fps、startMovieWav、ffmpeg settings、screen settings、record name，最后 `mirv_streams record start`。使用唯一 settings 名避免多段重复 add 冲突。
4. 编码参数复用 `ffmpegprofile.HLAEProfileByID/BuildRecordingEncodeArgs` 和 auto 选择。4:3 两种分辨率保留 `scale=trunc(ih*16/9/2)*2:ih` 的真实像素拉伸，不能退回 `-aspect` 元数据。
5. 插件 start 事件到达才切换 recording；Stop 先切换 stopping，收到对应 end 事件后切换 merging，启动异步单段文件任务。操作 API 快速返回状态，前端不阻塞到 FFmpeg 完成。
6. 参考 `expectedTakePaths`：现有生产模式期望 `takeName.mp4` 与 `takeName/audio.wav`。手动模式 record name 与 HLAE take 编号必须 Windows 验证；把实际输出目录规则固定在一个 helper 中，不硬猜“record name 就等于最终文件名”，也不扫描整个 outputs 找最新文件。
7. 调用 `producemerge.WaitForTakeFilesReady` → `MergeTakeVideoAudioContext`，全部继承当前调试工作目录任务 context。等文件可读后合并，成功展示最终路径；合并失败保留视频/音频供 `RetryDebugRecordingMerge` 重试。
8. 保留/清理中间文件复用 `keepProduceIntermediates` 作为本调试会话选项。成功且关闭保留时仅清理本段已登记的原始视频/音频；不要误删 finalVideoPath，不扫描清理其他会话文件。失败不清理可重试源文件。
9. 独立 debug 结果列表包含 recording_id、Demo、时间、状态、错误、源文件/最终路径；提供定位结果和失败合并重试。不调用 `addProduceHistoryEntry`，不添加 kill metadata/fastedit 旁车。
10. 补齐 StopDebugGame：录制中先请求 end、等待边界并完成或明确记录合并失败，再退出游戏；merging 时等待当前有界任务。App/工作目录强制关闭可取消合并并等待子进程退出，保留源文件，确认游戏退出后恢复环境。
11. 插件断连/游戏异常退出时停止把任务显示为 recording，标明失败并保留已知文件；仅在源文件确实可读时允许合并重试。恢复连接不自动 start/stop 重发。不扩展生产队列的 reconnect replay。
12. 自定义命令绕过按钮开启的录制不纳入本任务列表；UI 给一句说明。若已管理录制期间自定义命令导致退出，按异常结束处理，不伪造停止成功。无需命令黑名单。

## 验收

- 播放 → 任意时刻开始 → 任意时刻停止 → MP4 含视频音频；游戏仍运行。
- 连续两段不会覆盖，暂停 Demo 时可停止；零击杀 Demo 也可录制。
- 合并期间禁用新录制和换 Demo，普通画面命令仍可执行。
- 4:3 真实导出宽高、SAR/DAR 和声音正确；auto 使用后端编码选择。
- 重复 start/stop 被后端状态约束；结束事件不能误关联另一 recording_id。
- 缺音频、损坏文件或 FFmpeg 失败可见且保留源文件；重试合并只处理本段。
- 关闭录制中游戏、应用退出、工作目录关闭能停止进程/合并并恢复环境；失败保持可重试占用。
- `go test ./...`、前端测试/构建、插件 Windows 构建及实机录制全部报告。纯录制边界 mock 不能证明生成了文件。
