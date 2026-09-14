# Backend Result：Step 04 剪辑任务准入、输出唯一性与取消

- 实施前 HEAD：`1dc8ef05b22fb76c23cb5fef05f4591e98c1626c`（Merge pull request #57 from hkslover/codex/step-03-install-commit）
- 实施后提交：未提交（保留可审查 diff；工作树同时包含 Step 03-C 的未提交记录与本次改动）
- 本次范围：Step 04 —— B5 剪辑正确性与取消（单任务准入、唯一输出与原子发布、任务 context/探测保护）
- 状态：实现完成但有待验证项（Windows 真实 FFmpeg 进程、真实硬件回退与连续导出未验证）

前置核对：Step 01 的工作目录实例与受管文件使用权、Step 03 的原子提交约定在当前 HEAD 已生效。计划证据中
“`ProbeClipDuration` 未获取文件使用权”只对审查基线 `201cd67` 成立：该入口在 Step 01（`0e32f68`）已加上
`beginManagedFileUse`，本步补齐的是有限探测超时与任务 context 传播。

## 变更与证据

| 目标或 bug ID | 实际修改位置 | 原因与结果 | 对应测试 |
| --- | --- | --- | --- |
| B5：第二个合成不被拒绝，两个任务共用输出与进度 | `internal/app/edit_task.go`（新增 `editTaskAdmission`、`beginEditComposeTask`、`finishEditComposeTask`）；`internal/app/app_edit.go` | 取得工作目录使用权后立即尝试非阻塞准入；已有活跃任务时返回忙碌错误，不排队、不写盘，并立即释放工作目录使用权。准入锁只在字段更新时持有，不含 I/O/等待/事件 | `TestConcatEditClipsRejectsSecondTaskWhileFirstIsRunning` |
| B5：秒级输出名导致同秒覆盖 | `edit_task.go` 的 `reserveEditOutputPath`、`editComposeTask.prepare` | 输出名改为 `edit_<时间戳>_<12 位随机>.mp4`，用 `O_CREATE|O_EXCL` 原子预留；同名既有文件只会触发换名重试，不存在“检查不存在再创建”的 TOCTOU。每次任务另有 `.edit-task-*` 专属临时目录 | `TestConcatEditClipsSuccessiveTasksKeepDistinctOutputs`、`TestReserveEditOutputPathNeverReusesExistingFile` |
| 直接写最终视频、失败留半成品 | `app_edit.go` `ConcatEditClips`、`edit_task.go` `commit/cleanup/verifyEditTempOutput` | 先写 `<tempDir>/edit.mp4`，验证存在且非空后 rename 提交到预留路径，成功才 `addEditedHistoryEntry`；失败、取消、超时只删除本任务临时目录与本次占位文件 | `TestConcatEditClipsFailureLeavesNoArtifactOrHistory`、`TestConcatEditClipsMissingIntermediateIsNotPublished`、`TestEditComposeTaskCommitFailureDoesNotPublishOrDeleteForeignPath` |
| `.concat.txt` 派生自输出名，任务间互删 | `app_edit.go` `concatSimple` 改为接收 `listPath`；调用方传入任务临时目录 | 列表文件不再与输出同名，也不再出现在最终输出目录；转场路径同事务 | `TestConcatEditClipsTransitionPathPublishesOnce`（断言输出目录只剩已提交视频） |
| 探测/剪辑缺少取消传播 | `internal/app/edit_ffmpeg.go`、`app_edit.go` | 删除无 ctx 的 `ffmpegCommand`，统一走 `newFFmpegCommandContext`（`ffmpegCommandContext = exec.CommandContext`）；`probeDurationByFFprobe`/`probeVideoStreamInfo`/`resolveEditClips`/`concatSimple`/`concatWithTransitions` 全部接收任务 ctx | `TestConcatEditClipsWorkspaceCloseCancelsRunningFFmpeg`、`TestProbeHelpersDoNotStartCommandWithCanceledContext` |
| 探测无超时、取消与编码失败混淆 | `edit_ffmpeg.go` `ProbeClipDuration`、`resolveEditClips`；`edit_task.go` `editProbeFailure`、`editTaskCanceledError` | 探测使用 `editProbeTimeout`（30s）与工作目录 ctx，超时/取消分别归类为探测失败；`ctx` 已取消时不启动命令。`ProbeClipDuration` 仍只登记共享文件使用权、不占合成槽 | `TestProbeClipDurationTimeoutIsClassifiedAsProbeFailure`、`TestProbeClipDurationBlocksDirectoryClearAndReleasesUse` |
| 取消触发新一轮编码器回退 | `app_edit.go` 两个 concat 函数 | 每个 profile 尝试前与失败后检查任务 ctx：取消立即以“剪辑任务已取消”返回，绝不进入下一 profile；每次尝试独立的 `editComposeTimeout`（30min）按超时分类后继续既有编码回退顺序 | `TestConcatEditClipsCancelDoesNotRetryNextEncoderProfile`（断言仅 1 次 FFmpeg 调用） |
| 阻塞探测/合成期间的清理互斥 | 复用 `beginManagedWorkspaceTaskUse` | 合成与阻塞探测都持有受管文件使用权，`ClearOutputsDirectory` 在期间被拒绝，结束后恢复 | 准入测试与探测测试中的 `ClearOutputsDirectory` 断言 |
| 旧任务事件越过新任务 | `ConcatEditClips` 的 LIFO defer 顺序 | 合成同步执行；返回前 `cmd.Wait` 与输出 reader 均结束，然后依次执行任务清理 → 准入释放 → 文件使用权释放，最终 `compose_progress` 一定早于准入释放。payload 与事件名未改，未新增 job ID | 全部合成测试（成功/失败/取消/超时分支） |
| 所有权收紧 | `edit_task.go` `editPathWithin`、`cleanup` | 输出目录必须位于本次准入的工作目录内；清理只删除自己创建的正规占位文件，路径被替换为目录/符号链接时保留不动 | `TestEditComposeTaskCommitFailureDoesNotPublishOrDeleteForeignPath` |

### 旧行为的确定性反证（HEAD worktree）

在 `1dc8ef0` 的临时 worktree 中放入同一场景的测试（`b5_head_test.go`，未入库，worktree 已删除）：

```text
=== RUN   TestHeadSecondConcatEditClipsIsNotRejected
    b5_head_test.go:90: HEAD (no admission): the second active compose was accepted, want busy rejection
--- FAIL: TestHeadSecondConcatEditClipsIsNotRejected (0.06s)

=== RUN   TestHeadSameSecondConcatReusesOutputPath
    b5_head_test.go:126: HEAD reuses one second-resolution output path ".../outputs/edit/edit_20260910_224126.mp4"; the second task overwrites the first
--- FAIL: TestHeadSameSecondConcatReusesOutputPath (0.80s)

=== RUN   TestHeadWorkspaceCloseCannotCancelRunningFFmpeg
    b5_head_test.go:177: HEAD: session.close reported 停止工作目录任务超时: context deadline exceeded while ffmpeg is still blocked (no cancellation reaches the process)
--- PASS: TestHeadWorkspaceCloseCannotCancelRunningFFmpeg (5.04s)
```

前两条证明 HEAD 确实接受第二个活跃合成并复用同一输出路径；第三条证明 HEAD 的阻塞 FFmpeg 不接收工作目录取消
（`session.close` 只能等到超时）。修复后对应的 `TestConcatEditClipsRejectsSecondTaskWhileFirstIsRunning`、
`TestConcatEditClipsSuccessiveTasksKeepDistinctOutputs`、`TestConcatEditClipsWorkspaceCloseCancelsRunningFFmpeg`
分别断言忙碌拒绝、路径不同且旧文件保留、`session.close` 立即返回且任务以取消错误结束。

### 审查跟进：取消发布边界与预留文件身份（P2 × 2）

审查指出两处 P2，均在同一 diff 内修复：

1. **FFmpeg 成功退出后、发布前取消仍会发布**。原先两个 concat 函数的成功分支与 `ConcatEditClips` 的验证/提交路径不再检查 ctx。现在：
   - `concatSimple`/`concatWithTransitions` 在 runner 返回 nil 且 reader 收尾完成后立即检查任务 ctx，已取消则以取消错误返回，不进入验证/提交；
   - `ConcatEditClips` 在 rename 之前设置唯一“取消胜出”边界，已取消则发失败事件并返回；
   - `editComposeTask.commit()` 自身拒绝已取消任务，作为与调用流无关的强制检查。
   提交成功之后工作目录才关闭时，按成功登记 history 并返回成功，不删除已发布文件、也不把结果伪装成未提交。
   测试：`TestConcatEditClipsCancelAfterFFmpegSuccessDoesNotPublish`（无转场/转场两个子用例；假 FFmpeg 刻意不绑定 ctx，
   先取消工作目录再让它成功退出）、`TestEditComposeTaskCommitRefusesCanceledTask`。

2. **预留路径不保持文件身份**。原先 `cleanup` 只按 IsRegular 删除、`commit` 无条件 rename，占位文件被替换后会删除/覆盖替换对象。现在
   `reserveEditOutputPath` 返回创建时的 `os.FileInfo`，`editComposeTask` 在提交与清理时用 `os.SameFile` 校验路径仍是本次文件：
   身份不符时提交返回“预留的剪辑输出路径已被替换，已保留原对象”，清理跳过删除；目录、普通文件、符号链接替换均被覆盖。
   测试：`TestEditComposeTaskCommitRefusesReplacedRegularFile`、`TestEditComposeTaskCommitRefusesReplacedSymlink`、
   `TestEditComposeTaskCleanupPreservesReplacedRegularFile`、`TestReserveEditOutputPathNeverReusesExistingFile`（新增身份断言）。

仍存在的边界（不宣称消除）：身份校验与 rename 之间仍有理论上的检查—重命名窗口；彻底消除需要平台级 no-replace rename
（`renameat2(RENAME_NOREPLACE)` / `renamex_np(RENAME_EXCL)` / 不带 `MOVEFILE_REPLACE_EXISTING` 的 `MoveFileEx`）或私有任务子目录方案。
本步按审查建议保留扁平输出并做身份校验，未引入平台适配层，已列入剩余项。

## 验证

| 命令/场景 | 平台 | 实际结果 | 未覆盖范围 |
| --- | --- | --- | --- |
| `go test ./... -count=1` | macOS（darwin/arm64，go1.24.0） | 全部包 ok（含 envsetup/release/producews；审查修复后重跑） | Windows 分支未执行 |
| `go test -race ./internal/app -count=1` | macOS | ok（32.1s，审查修复后重跑） | race 只覆盖实际执行路径 |
| `go vet ./...` | macOS | 通过 | — |
| `gofmt -l`（本次改动文件） | — | 无输出 | 仓库既有 `app_workspace_test.go`、`platform_client.go`、`demo/killfacts_test.go` 未格式化，未在本次触碰 |
| `git diff --check` | — | 通过 | — |
| HEAD worktree 反证（见上） | macOS | 旧代码接受第二次合成、复用路径、取消不达进程 | 一次性证据，测试文件已删除 |
| 前端 | — | 未执行 `npm test`/`npm run build` | 未改公开方法、JSON 字段、事件名与前端类型；`frontend/wailsjs` 未改 |

本机沙箱禁止写默认 `GOCACHE`（`~/Library/Caches/go-build`），上述 Go 命令使用 `GOCACHE=$TMPDIR/dsh-go-build` 执行，不影响测试语义。

已确认的既有 flake（与本次改动无关）：全量并行运行时偶发 `internal/envsetup` 的
`TestInstallFFmpegFromArchive_LogsStages` 失败，失败信息为 `TempDir RemoveAll cleanup: directory not empty`，
原因是异步 FFmpeg 编码能力探测在 `t.TempDir` 清理开始后仍向该目录写入。在 HEAD `1dc8ef0` 的临时 worktree 中连跑
5 次全量测试复现 2 次；本步改动后的全量运行也在该包出现过同一失败，单包/单测运行稳定通过。该包不在 Step 04 范围内，未修改。

## 契约、所有权与失败恢复

- 公开契约变化：无。`ConcatEditClips` 仍返回最终视频路径，`compose_progress` 的 `{active, percent, current_step, elapsed_ms, error}` 与事件名不变，未新增 job ID 或取消方法；输出目录与 history 字段不变。输出文件名新增随机后缀，但没有消费者解析该名称（前端只使用返回路径与 history）。
- 新增/改变的内部接口和调用者：新增 `editComposeTask`/`editTaskAdmission`/`reserveEditOutputPath`/`verifyEditTempOutput`/`editPathWithin`/`editProbeFailure`/`editTaskCanceledError`/`editAttemptTimeout`；`resolveEditClips`、`concatSimple`、`concatWithTransitions`、`probeDurationByFFprobe`、`probeVideoStreamInfo` 增加 ctx 参数，调用者仅本包与白盒测试。删除包级 `ffmpegCommand`，命令 seam 收敛为 `ffmpegCommandContext`。
- 锁顺序：工作目录准入（`managedFilesMu` + session task gate）→ `editTasks.mu`；`editTasks.mu` 只在准入/释放的短临界区持有，内部不做 I/O、不等待子进程、不发事件，无锁顺序反转。
- 资源释放：`ConcatEditClips` 的 defer 为 LIFO —— 任务清理 → 准入释放 → 文件使用权释放；`cleanup`、`finishEditComposeTask` 与使用权释放均幂等。工作目录关闭先取消任务 ctx，`exec.CommandContext` 终止 FFmpeg，`cmd.Wait` 与 stdout/stderr reader 结束后任务才返回，会话 `close` 等到任务真正退出。
- 失败后磁盘/内存状态：未提交的任务只删除 `.edit-task-*` 临时目录与本次 `O_EXCL` 占位文件，不写 history；其他任务与既有成功视频不受影响。取消错误不进入编码回退，探测/合成超时与取消分别归类。

## 相对计划的调整

1. 计划建议任务持有 `done` 通道；实现为同步执行 + LIFO defer。合成在 RPC goroutine 内完成，`cmd.Wait` 与 reader 已结束后才返回，准入释放顺序已可表达，无需额外 done/回调。
2. 输出预留采用计划允许的“扁平输出 + 独占 reservation”：`outputs/edit` 下时间戳 + 随机后缀 + `O_EXCL` 占位；临时目录放在同一目录以保证提交 rename 不跨卷。未采用任务子目录，避免改变既有产物发现与用户浏览习惯。
3. 新增每次 FFmpeg 尝试的 `editComposeTimeout`（30 分钟）。计划要求“合成超时与时长/用户取消分开，不用固定很短时限”；超时按 profile 归类后继续既有编码回退，只有工作目录/应用取消是终止性的。
4. `verifyEditTempOutput` 在原有 `Stat` 之上要求非空；中间产物缺失或为空都不提交、不写 history。
5. 额外增加 `editPathWithin` 输出目录包含校验与“只删除正规占位文件”的保护，属于所有权收紧，不改变正常路径行为。
6. 计划第 4 条前半（`ProbeClipDuration` 未登记文件使用权）在当前源码已由 Step 01 修复，本步只实现有限超时与 ctx 传播。

## 剩余项和交接

- 本阶段未完成项：
  - 未提供用户手动“取消剪辑”公开方法或按钮（按计划不做，仅接 App shutdown 与工作目录关闭）。
  - 预留路径身份校验与 rename 之间仍有理论上的检查—重命名窗口（见“审查跟进”）；彻底消除需平台 no-replace rename 或私有输出子目录，本步未做。
  - `runFFmpegCommandWithProgress` 仍是先 `cmd.Wait` 后 `readWG.Wait`（既有实现）：取消时子进程退出使 reader 立即 EOF，未改结构；若后续要在意进度输出尾部，可在 Step 06 一并整理。
  - 硬件编码超时后仍按既有顺序回退下一个 profile，真实硬件失败耗时与是否值得缩短未验证。
  - 触及路径中仍使用非 context 命令的只剩打开目录/定位文件（`outputs_storage.go`、`produce_takefile.go` 的 `explorer/open/xdg-open`），不属于探测或剪辑执行路径。
- Windows/真实进程验证待办：
  - 真实 FFmpeg 长任务取消：`exec.CommandContext` 只终止直接子进程，未验证 Windows 上 ffmpeg 的进程树/句柄释放。
  - 真实连续导出、硬件编码失败回退、输出文件被占用导致 rename 提交失败的恢复表现。
  - 真实 Wails 并发 RPC（前端 `exporting` 本身禁用按钮，后端忙碌错误是安全网）。
- 下一阶段（Step 06 剪辑架构提取）可依赖：
  - `editComposeTask` 的生命周期与 LIFO 释放顺序、`ffmpegCommandContext` 唯一命令 seam、`editProbeTimeout`/`editComposeTimeout` 常量与“取消终止、超时分类”的语义。
  - `beginManagedWorkspaceTaskUse` 的“工作目录 ctx + 直到任务退出才释放的准入”语义；剪辑任务不复用导入协调器。
