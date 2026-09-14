package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Package-level seams in the same style as the produce pipeline: tests shorten
// timeouts or force an output-name collision without running a real FFmpeg.

// editProbeTimeout bounds one read-only ffprobe command. It is deliberately
// generous: it protects against a hung probe, not against big files, and it is
// reported separately from a workspace cancellation.
var editProbeTimeout = 30 * time.Second

// editComposeTimeout bounds one ffmpeg invocation of a compose task. A compose
// may legitimately run for minutes, so this only kills a hung process; the
// resulting error is classified as a timeout, not as a cancellation.
var editComposeTimeout = 30 * time.Minute

// editOutputNameSuffix returns the random suffix of one reserved output name.
var editOutputNameSuffix = newEditOutputNameSuffix

const (
	editTaskTempDirPrefix     = ".edit-task-"
	editOutputReserveAttempts = 8
)

func newEditOutputNameSuffix() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(buf[:])
}

// editComposeTask is one admitted ConcatEditClips run. It owns the task
// lifecycle context, a private temp directory for the intermediate video and
// the atomically reserved final output path.
type editComposeTask struct {
	ctx    context.Context
	cancel context.CancelFunc

	workspaceRoot string

	tempDir        string
	tempOutput     string
	finalOutput    string
	reservedInfo   os.FileInfo
	reservedOutput bool
	committed      bool
}

// editTaskAdmission enforces one active compose at a time. It rejects instead
// of queueing on purpose: the desktop UI has a single compose task, and a
// queued second task would mix progress events and file ownership. The mutex
// is only ever held for field updates, never while running FFmpeg or waiting.
type editTaskAdmission struct {
	mu     sync.Mutex
	active *editComposeTask
}

// errEditComposeBusy rejects a second compose before it touches the disk.
var errEditComposeBusy = errors.New("已有剪辑合成任务正在进行，请等待完成后再试")

// beginEditComposeTask admits one compose task and derives its cancelable
// context from the workspace lifecycle context. The caller must already hold a
// managed workspace use; a busy slot releases only that reservation.
func (a *App) beginEditComposeTask(workCtx context.Context, workspaceRoot string) (*editComposeTask, error) {
	if a == nil {
		return nil, workspaceNotInitializedErr()
	}
	if workspaceRoot == "" {
		return nil, workspaceNotInitializedErr()
	}
	if workCtx == nil {
		workCtx = context.Background()
	}

	a.editTasks.mu.Lock()
	if a.editTasks.active != nil {
		a.editTasks.mu.Unlock()
		return nil, errEditComposeBusy
	}
	// Reserve the slot before any I/O so a concurrent caller is rejected even
	// while this task is still preparing its directories.
	taskCtx, cancel := context.WithCancel(workCtx)
	task := &editComposeTask{
		ctx:           taskCtx,
		cancel:        cancel,
		workspaceRoot: workspaceRoot,
	}
	a.editTasks.active = task
	a.editTasks.mu.Unlock()
	return task, nil
}

// finishEditComposeTask releases the single-compose slot. It must run after
// the task-owned cleanup so no new task can observe a half-removed temp dir.
func (a *App) finishEditComposeTask(task *editComposeTask) {
	if task == nil {
		return
	}
	if task.cancel != nil {
		task.cancel()
	}
	a.editTasks.mu.Lock()
	if a.editTasks.active == task {
		a.editTasks.active = nil
	}
	a.editTasks.mu.Unlock()
}

// prepare freezes the task-owned paths: the private temp directory the
// intermediate video is written to and the atomically reserved final path.
func (t *editComposeTask) prepare(outputDir string) error {
	if t == nil {
		return fmt.Errorf("剪辑任务未初始化")
	}
	if outputDir == "" {
		return fmt.Errorf("剪辑输出目录为空")
	}
	// Task-owned temp files and reservations stay inside the workspace this
	// task was admitted for, so a workspace reset never races an outside path.
	if !editPathWithin(t.workspaceRoot, outputDir) {
		return fmt.Errorf("剪辑输出目录不在当前工作目录内: %s", outputDir)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory failed: %w", err)
	}
	tempDir, err := os.MkdirTemp(outputDir, editTaskTempDirPrefix)
	if err != nil {
		return fmt.Errorf("create edit task directory failed: %w", err)
	}
	t.tempDir = tempDir
	// Keep a recognizable container suffix: ffmpeg infers the muxer from it.
	t.tempOutput = filepath.Join(tempDir, "edit.mp4")
	t.reservedOutput = false

	finalOutput, reservedInfo, err := reserveEditOutputPath(outputDir, time.Now())
	if err != nil {
		return err
	}
	t.finalOutput = finalOutput
	t.reservedInfo = reservedInfo
	t.reservedOutput = true
	return nil
}

// ownsReservedOutput reports whether the reserved path still refers to the
// very file this task created. Any replacement (regular file, symlink,
// directory) fails the identity check, so neither commit nor cleanup may touch
// it. The check narrows but cannot remove the final check-to-rename window;
// that would need a platform no-replace rename or a private output directory.
func (t *editComposeTask) ownsReservedOutput() (os.FileInfo, bool) {
	if t == nil || !t.reservedOutput || t.finalOutput == "" || t.reservedInfo == nil {
		return nil, false
	}
	info, err := os.Lstat(t.finalOutput)
	if err != nil {
		return nil, false
	}
	return info, os.SameFile(t.reservedInfo, info)
}

// commit publishes the verified intermediate video at the reserved path. The
// rename never crosses a volume (both paths live under outputDir) and replaces
// only this task's own placeholder, so a previous video is never overwritten.
func (t *editComposeTask) commit() error {
	if t == nil || !t.reservedOutput || t.finalOutput == "" || t.tempOutput == "" {
		return fmt.Errorf("剪辑输出尚未预留")
	}
	// Enforcement lives here, not only in the RPC: a canceled task never
	// publishes, and the check stays valid if the caller's flow changes.
	if cancelErr := editTaskCanceledError(t.ctx); cancelErr != nil {
		return cancelErr
	}
	if _, ok := t.ownsReservedOutput(); !ok {
		return fmt.Errorf("预留的剪辑输出路径已被替换，已保留原对象: %s", t.finalOutput)
	}
	if err := os.Rename(t.tempOutput, t.finalOutput); err != nil {
		return fmt.Errorf("提交剪辑产物失败: %w", err)
	}
	t.committed = true
	return nil
}

// cleanup removes only the paths this task owns: always its private temp
// directory, and the reserved placeholder while nothing was committed. A
// committed video and any pre-existing video are never removed.
func (t *editComposeTask) cleanup() {
	if t == nil {
		return
	}
	if t.tempDir != "" {
		_ = os.RemoveAll(t.tempDir)
		t.tempDir = ""
	}
	if !t.committed && t.reservedOutput && t.finalOutput != "" {
		// Only the exact regular file this task reserved is removed. A
		// replaced object keeps its own identity and is left alone.
		if info, ok := t.ownsReservedOutput(); ok && info.Mode().IsRegular() {
			_ = os.Remove(t.finalOutput)
		}
		t.reservedOutput = false
	}
}

// reserveEditOutputPath atomically reserves one unique final video path. The
// name keeps the historical second-resolution timestamp and adds a random
// suffix, and O_CREATE|O_EXCL means a pre-existing file is never reused or
// overwritten: there is no check-then-create window. The returned FileInfo is
// the identity later commit/cleanup must still find at that path.
func reserveEditOutputPath(outputDir string, now time.Time) (string, os.FileInfo, error) {
	if outputDir == "" {
		return "", nil, fmt.Errorf("剪辑输出目录为空")
	}
	for attempt := 0; attempt < editOutputReserveAttempts; attempt++ {
		name := fmt.Sprintf("edit_%s_%s.mp4", now.Format("20060102_150405"), editOutputNameSuffix())
		path := filepath.Join(outputDir, name)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if !errors.Is(err, fs.ErrExist) {
				return "", nil, fmt.Errorf("预留剪辑输出路径失败: %w", err)
			}
			continue
		}
		info, statErr := file.Stat()
		closeErr := file.Close()
		if statErr != nil || closeErr != nil {
			_ = os.Remove(path)
			if statErr != nil {
				return "", nil, fmt.Errorf("预留剪辑输出路径失败: %w", statErr)
			}
			return "", nil, fmt.Errorf("预留剪辑输出路径失败: %w", closeErr)
		}
		return path, info, nil
	}
	return "", nil, fmt.Errorf("预留剪辑输出路径失败: 同名文件已存在")
}

// editPathWithin reports whether path is inside root. Both sides are cleaned;
// the check keeps every task-owned path inside the admitted workspace.
func editPathWithin(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// verifyEditTempOutput validates the intermediate video before it may be
// published; a missing or empty file must never become a history entry.
func verifyEditTempOutput(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("output video not created: %w", err)
	}
	if info.IsDir() || info.Size() <= 0 {
		return fmt.Errorf("output video is empty: %s", path)
	}
	return nil
}

// editTaskCanceledError reports a canceled workspace/task context. Cancellation
// must terminate the task with its own reason and must never be reported as an
// unavailable encoder or trigger the next retry profile.
func editTaskCanceledError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("剪辑任务已取消: %w", err)
	}
	return nil
}

// editProbeFailure classifies one failed ffprobe invocation. The command has
// already exited when this is called, so no child process is left behind.
func editProbeFailure(ctx context.Context, cmdErr error, output string) error {
	if ctx != nil {
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return fmt.Errorf("ffprobe 探测超时（超过 %s）: %w", editProbeTimeout, ctx.Err())
		case ctx.Err() != nil:
			return fmt.Errorf("工作目录正在关闭，探测已取消: %w", ctx.Err())
		}
	}
	return fmt.Errorf("ffprobe failed: %w: %s", cmdErr, strings.TrimSpace(output))
}
