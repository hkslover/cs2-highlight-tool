package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CopyFile copies src to dst using a same-directory temporary file and a
// commit rename.  A failed read, close, or rename never exposes a partial
// destination.  Existing destinations are moved aside while the commit is
// attempted so that a failed replacement can restore the previous file (this
// also works on Windows, where Rename does not replace an existing path).
func CopyFile(src, dst string) error {
	return CopyFileAtomicWithContext(context.Background(), src, dst)
}

// CopyFileAtomic is the explicit name for the transactional CopyFile
// contract.  CopyFile remains the compatibility entry point used by existing
// callers.
func CopyFileAtomic(src, dst string) error {
	return CopyFileAtomicWithContext(context.Background(), src, dst)
}

// CopyFileAtomicWithContext is CopyFileAtomic with cancellation checks before
// and during the copy and immediately before the commit rename.
func CopyFileAtomicWithContext(ctx context.Context, src, dst string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	source := filepath.Clean(strings.TrimSpace(src))
	target := filepath.Clean(strings.TrimSpace(dst))
	if source == "." || strings.TrimSpace(src) == "" {
		return fmt.Errorf("源文件路径为空")
	}
	if target == "." || strings.TrimSpace(dst) == "" {
		return fmt.Errorf("目标文件路径为空")
	}
	if sameFilePath(source, target) {
		return nil
	}
	if err := contextErr(ctx); err != nil {
		return err
	}

	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("源路径不是普通文件: %s", source)
	}

	return copyReaderAtomicWithOps(ctx, in, target, info.Mode().Perm(), defaultAtomicCopyOps())
}

// CopyReaderAtomic commits bytes supplied by reader to dst atomically.  It is
// intentionally small so callers that already own a reader do not need to
// materialize it before committing.  The reader is not closed by this helper.
func CopyReaderAtomic(ctx context.Context, reader io.Reader, dst string) error {
	return copyReaderAtomicWithOps(ctx, reader, dst, 0644, defaultAtomicCopyOps())
}

// IsLikelyDemoFile performs the deliberately limited validation used for old
// demo cache entries.  It rejects missing/non-regular/too-short files and
// checks only the eight-byte file stamp.  A positive result is not proof that
// the demo is complete; tail truncation still requires the normal parser to
// detect it.
func IsLikelyDemoFile(path string) (bool, error) {
	filePath := filepath.Clean(strings.TrimSpace(path))
	if filePath == "." || strings.TrimSpace(path) == "" {
		return false, nil
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	const fileStampSize = 8
	if !info.Mode().IsRegular() || info.Size() < fileStampSize {
		return false, nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	stamp := make([]byte, fileStampSize)
	if _, err := io.ReadFull(f, stamp); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return false, nil
		}
		return false, err
	}
	// PBDEMS2 is the CS2 demo stamp. HL2DEMO is accepted as a basic legacy
	// shape check; the parser remains responsible for deciding whether that
	// older format is usable by the application. Demo stamps are C strings, so
	// the eighth byte is normally a NUL terminator.
	stampText := strings.TrimRight(string(stamp), "\x00")
	return stampText == "PBDEMS2" || stampText == "HL2DEMO", nil
}

type atomicCopyTemp interface {
	io.Writer
	Close() error
}

type atomicCopyOps struct {
	stat       func(string) (os.FileInfo, error)
	mkdirAll   func(string, os.FileMode) error
	createTemp func(string, string) (string, atomicCopyTemp, error)
	rename     func(string, string) error
	remove     func(string) error
	chmod      func(string, os.FileMode) error
}

func defaultAtomicCopyOps() atomicCopyOps {
	return atomicCopyOps{
		stat:       os.Stat,
		mkdirAll:   os.MkdirAll,
		createTemp: createAtomicCopyTemp,
		rename:     os.Rename,
		remove:     os.Remove,
		chmod:      os.Chmod,
	}
}

func (ops atomicCopyOps) withDefaults() atomicCopyOps {
	defaults := defaultAtomicCopyOps()
	if ops.stat == nil {
		ops.stat = defaults.stat
	}
	if ops.mkdirAll == nil {
		ops.mkdirAll = defaults.mkdirAll
	}
	if ops.createTemp == nil {
		ops.createTemp = defaults.createTemp
	}
	if ops.rename == nil {
		ops.rename = defaults.rename
	}
	if ops.remove == nil {
		ops.remove = defaults.remove
	}
	if ops.chmod == nil {
		ops.chmod = defaults.chmod
	}
	return ops
}

func createAtomicCopyTemp(dir, pattern string) (string, atomicCopyTemp, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", nil, err
	}
	return f.Name(), f, nil
}

func copyReaderAtomicWithOps(ctx context.Context, reader io.Reader, dst string, mode os.FileMode, ops atomicCopyOps) error {
	ops = ops.withDefaults()
	if reader == nil {
		return fmt.Errorf("源读取器为空")
	}
	if err := contextErr(ctx); err != nil {
		return err
	}

	target := filepath.Clean(strings.TrimSpace(dst))
	if target == "." || strings.TrimSpace(dst) == "" {
		return fmt.Errorf("目标文件路径为空")
	}
	parent := filepath.Dir(target)
	if err := ops.mkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	tmpPath, tmp, err := ops.createTemp(parent, "."+filepath.Base(target)+".copy-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	removeTemp := func() error {
		if removeErr := ops.remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("清理临时文件 %q 失败: %w", tmpPath, removeErr)
		}
		return nil
	}

	_, copyErr := io.Copy(tmp, contextReader{ctx: ctx, reader: reader})
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		cleanupErr := removeTemp()
		return errors.Join(
			wrapCopyError("复制到临时文件失败", copyErr),
			wrapCopyError("关闭临时文件失败", closeErr),
			cleanupErr,
		)
	}
	if err := contextErr(ctx); err != nil {
		return errors.Join(err, removeTemp())
	}
	if mode == 0 {
		mode = 0644
	}
	if err := ops.chmod(tmpPath, mode); err != nil {
		return errors.Join(fmt.Errorf("设置临时文件权限失败: %w", err), removeTemp())
	}

	// Re-check at commit time so a destination created while the copy was in
	// progress is protected by the same restore path as an existing cache.
	_, statErr := ops.stat(target)
	targetExists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return errors.Join(fmt.Errorf("检查目标文件失败: %w", statErr), removeTemp())
	}

	var backupPath string
	backupMoved := false
	if targetExists {
		backupPath, err = moveTargetToBackup(ops, parent, target)
		if err != nil {
			return errors.Join(err, removeTemp())
		}
		backupMoved = true
	}

	if err := contextErr(ctx); err != nil {
		return restoreAfterCommitFailure(ops, target, backupPath, backupMoved, err, removeTemp)
	}
	if err := ops.rename(tmpPath, target); err != nil {
		return restoreAfterCommitFailure(ops, target, backupPath, backupMoved,
			fmt.Errorf("提交目标文件失败: %w", err), removeTemp)
	}

	// The new target is committed.  A stale backup is never allowed to turn a
	// successful cache import into a failure; leave it for a later cleanup if a
	// filesystem refuses removal.
	if backupMoved {
		_ = ops.remove(backupPath)
	}
	return nil
}

func moveTargetToBackup(ops atomicCopyOps, parent, target string) (string, error) {
	backupPath, backup, err := ops.createTemp(parent, "."+filepath.Base(target)+".backup-*.tmp")
	if err != nil {
		return "", fmt.Errorf("创建缓存备份临时文件失败: %w", err)
	}
	if err := backup.Close(); err != nil {
		_ = ops.remove(backupPath)
		return "", fmt.Errorf("关闭缓存备份临时文件失败: %w", err)
	}
	if err := ops.remove(backupPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("准备缓存备份路径失败: %w", err)
	}
	if err := ops.rename(target, backupPath); err != nil {
		return "", fmt.Errorf("备份旧缓存失败: %w", err)
	}
	return backupPath, nil
}

func restoreAfterCommitFailure(
	ops atomicCopyOps,
	target, backupPath string,
	backupMoved bool,
	primary error,
	removeTemp func() error,
) error {
	cleanupErr := removeTemp()
	if !backupMoved {
		return errors.Join(primary, cleanupErr)
	}
	restoreErr := ops.rename(backupPath, target)
	if restoreErr != nil {
		return errors.Join(primary, fmt.Errorf("恢复旧缓存失败: %w", restoreErr), cleanupErr)
	}
	return errors.Join(primary, cleanupErr)
}

func wrapCopyError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := contextErr(r.ctx); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func sameFilePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
