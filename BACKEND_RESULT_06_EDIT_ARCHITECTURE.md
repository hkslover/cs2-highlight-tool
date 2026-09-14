# Step 06 完成记录：剪辑规划与执行分离

## 迁移结果

- 新增 `internal/edit/` 领域包：`model.go` 定义显式媒体事实、编码选择、进度事件与少量可判断错误类别；`plan.go` 负责裁剪范围、转场归一化、事实归一化和编码 profile 选择；`filtergraph.go` 负责现有硬切/淡入淡出、帧对齐、SAR/DAR 与混合音轨 filter graph；`probe.go` 负责 context-aware ffprobe；`runner.go` 负责 context-aware FFmpeg、进度读取和原有编码回退。
- `internal/app/app_edit.go` 保留 `EditConcatRequest`、`ProbeClipDuration`/`ConcatEditClips` 的 Wails 边界、工作目录准入、输出预留/提交、history 与最终 `compose_progress` 映射；仅通过窄转换适配调用领域包。
- `internal/app/edit_ffmpeg.go` 保留配置/可执行文件路径解析；命令 factory 由 `App` 实例持有，生产默认使用 `exec.CommandContext`，不再通过包级可变 seam 注入。`internal/app/edit_task.go` 继续拥有 Step 04 的单任务准入、任务临时目录、`O_EXCL` 唯一输出、取消检查、身份校验和提交清理。
- 根 `AGENTS.md` 与 `internal/AGENTS.md` 已补充 `internal/edit` 职责和 App/领域边界。未修改前端契约、Wails 生成绑定、UI、编码策略、转场效果或 fastEdit 规则。

## 保持的语义

- 旧 sequential transitions 与显式 `after_index` 均保持；转场长度仍按现有范围和帧网格校正。
- trim 仅接受有限、非空、在探测时长内的范围，保留一毫秒 probe rounding 容差；有转场或 trim 时优先使用视频流事实，request duration 不覆盖 probe 结果。
- filter graph 继续按首段输出规格处理，保留源 SAR/DAR 推导；缺少音轨时生成静音轨，有音轨/无音轨混排保持兼容；fastEdit 仍由前端决定裁剪范围，后端只验证并执行传入范围。
- 取消在 FFmpeg 成功后、重试前、验证前和提交前仍具有终止优先级；编码失败才进入原有 profile fallback。领域 runner 不写 history、不发 Wails 事件、不发布最终产物。
- 进度事件仍映射为 `active/percent/current_step/elapsed_ms/error`；App tracker 保存已发出的最大百分比，因此编码器回退时不会出现进度倒退。App→领域→产物提交→history 的集成路径由既有 App 测试保留。

## 测试与验证

| 命令/场景 | 实际结果 | 边界 |
| --- | --- | --- |
| `go test ./... -count=1` | 全部包通过 | macOS，未替代 Windows 行为验证 |
| `go test -race ./internal/app ./internal/edit -count=1` | 通过 | 只覆盖测试执行路径 |
| `go vet ./...` | 通过 | — |
| `go test ./internal/edit` | 纯规划、取消、进度 helper，以及首个 encoder 失败后 fallback 成功的回归测试通过 | 领域测试不等同于真实 Wails 事件 |
| App 进度/命令注入回归 | 通过；tracker 验证 `0→80→80→100`，安全测试按 App 实例注入 command factory | 未声称覆盖真实 Wails 事件桥 |
| 真实 FFmpeg/ffprobe 短合成 | 输出 duration `2.202s`，视频 `640x480`、SAR `1:1`、DAR `4:3`，存在双声道音轨 | macOS 工具链；未执行 Windows FFmpeg/HLAE/CS2 |
| `git diff --check` | 通过 | 未把未跟踪计划文件当作业务改动处理 |

真实 Windows FFmpeg 进程树取消、硬件编码回退、Wails 页面交互、CS2/HLAE 注入和完整录制链路仍需在 Windows 环境验证。
