package download

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
)

func Unzip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	destClean := filepath.Clean(destDir) + string(os.PathSeparator)
	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(target)+pathSuffix(f.FileInfo().IsDir()), destClean) {
			return fmt.Errorf("压缩包包含非法路径: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func Extract7z(archivePath, destDir string) error {
	r, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	destClean := filepath.Clean(destDir) + string(os.PathSeparator)
	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(target)+pathSuffix(f.FileInfo().IsDir()), destClean) {
			return fmt.Errorf("压缩包包含非法路径: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func pathSuffix(isDir bool) string {
	if isDir {
		return string(os.PathSeparator)
	}
	return ""
}

func CopyDirContents(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return CopyFile(path, target)
	})
}

// ReplaceDirPhase identifies the point at which a directory replacement
// stopped.  The phases intentionally mirror the install commit table: the
// source is prepared first, the old target is backed up, the new target is
// committed, and cleanup is last.
type ReplaceDirPhase string

const (
	ReplaceDirPhasePrepare ReplaceDirPhase = "prepare"
	ReplaceDirPhaseBackup  ReplaceDirPhase = "backup"
	ReplaceDirPhaseCommit  ReplaceDirPhase = "commit"
	ReplaceDirPhaseRestore ReplaceDirPhase = "restore"
	ReplaceDirPhaseCleanup ReplaceDirPhase = "cleanup"
)

// ReplaceDirReport describes the paths owned by one replacement attempt.
// StagingPath and BackupPath are retained in the report even after a
// successful cleanup so callers can include the paths in diagnostics.
// BackupCleanupError is a warning: the new target has already been committed
// and the backup is deliberately left in place for recovery.
type ReplaceDirReport struct {
	TargetPath         string
	StagingPath        string
	BackupPath         string
	HadExistingTarget  bool
	Committed          bool
	Recovered          bool
	Phase              ReplaceDirPhase
	BackupCleanupError error
}

// ReplaceDirError is returned when the new directory could not be committed
// (or the old directory could not be prepared for the commit).  Primary and
// Recovery are kept separate so callers and tests can inspect both failures.
// The staging/backup paths identify the only paths created by this attempt;
// pre-existing recovery directories are never removed by this helper.
type ReplaceDirError struct {
	Phase       ReplaceDirPhase
	TargetPath  string
	StagingPath string
	BackupPath  string
	Primary     error
	Recovery    error
	Recovered   bool
}

func (e *ReplaceDirError) Error() string {
	if e == nil {
		return "目录替换失败"
	}
	message := fmt.Sprintf("目录替换在 %s 阶段失败", e.Phase)
	if e.Primary != nil {
		message += ": " + e.Primary.Error()
	}
	if e.Recovery != nil {
		message += "; 恢复/清理失败: " + e.Recovery.Error()
	}
	if e.Recovered {
		message += "; 旧版本已恢复"
	}
	if e.StagingPath != "" {
		message += "; staging=" + e.StagingPath
	}
	if e.BackupPath != "" {
		message += "; backup=" + e.BackupPath
	}
	return message
}

// Unwrap exposes both the operation error and a possible recovery/cleanup
// error to errors.Is/errors.As without losing the primary failure.
func (e *ReplaceDirError) Unwrap() []error {
	if e == nil {
		return nil
	}
	result := make([]error, 0, 2)
	if e.Primary != nil {
		result = append(result, e.Primary)
	}
	if e.Recovery != nil {
		result = append(result, e.Recovery)
	}
	return result
}

// replaceDirFS is an instance dependency for the filesystem transaction.
// Keeping the seam on an instance avoids package-global mutable hooks while
// still allowing deterministic rename/copy/cleanup fault injection in tests.
type replaceDirFS struct {
	stat            func(string) (os.FileInfo, error)
	mkdirAll        func(string, os.FileMode) error
	mkdirTemp       func(string, string) (string, error)
	rename          func(string, string) error
	removeAll       func(string) error
	copyDirContents func(string, string) error
}

func defaultReplaceDirFS() replaceDirFS {
	return replaceDirFS{
		stat:            os.Lstat,
		mkdirAll:        os.MkdirAll,
		mkdirTemp:       os.MkdirTemp,
		rename:          os.Rename,
		removeAll:       os.RemoveAll,
		copyDirContents: CopyDirContents,
	}
}

func (ops replaceDirFS) withDefaults() replaceDirFS {
	defaults := defaultReplaceDirFS()
	if ops.stat == nil {
		ops.stat = defaults.stat
	}
	if ops.mkdirAll == nil {
		ops.mkdirAll = defaults.mkdirAll
	}
	if ops.mkdirTemp == nil {
		ops.mkdirTemp = defaults.mkdirTemp
	}
	if ops.rename == nil {
		ops.rename = defaults.rename
	}
	if ops.removeAll == nil {
		ops.removeAll = defaults.removeAll
	}
	if ops.copyDirContents == nil {
		ops.copyDirContents = defaults.copyDirContents
	}
	return ops
}

func replaceDirError(report ReplaceDirReport, phase ReplaceDirPhase, primary, recovery error) error {
	return &ReplaceDirError{
		Phase:       phase,
		TargetPath:  report.TargetPath,
		StagingPath: report.StagingPath,
		BackupPath:  report.BackupPath,
		Primary:     primary,
		Recovery:    recovery,
		Recovered:   report.Recovered,
	}
}

// ReplaceDirWithContentsWithReport runs the directory replacement and
// returns a report suitable for structured logging.  A backup cleanup error
// is reported in the result but does not turn a committed installation into a
// failed installation; the backup remains available for manual recovery.
func ReplaceDirWithContentsWithReport(src, dst string) (ReplaceDirReport, error) {
	return replaceDirWithContents(defaultReplaceDirFS(), src, dst)
}

func ReplaceDirWithContents(src, dst string) error {
	_, err := ReplaceDirWithContentsWithReport(src, dst)
	return err
}

func replaceDirWithContents(ops replaceDirFS, src, dst string) (ReplaceDirReport, error) {
	ops = ops.withDefaults()
	report := ReplaceDirReport{
		TargetPath: filepath.Clean(dst),
		Phase:      ReplaceDirPhasePrepare,
	}

	if src == "" {
		return report, replaceDirError(report, ReplaceDirPhasePrepare, fmt.Errorf("源目录为空"), nil)
	}
	if dst == "" {
		return report, replaceDirError(report, ReplaceDirPhasePrepare, fmt.Errorf("目标目录为空"), nil)
	}
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	report.TargetPath = dst

	sourceInfo, err := ops.stat(src)
	if err != nil {
		return report, replaceDirError(report, ReplaceDirPhasePrepare, fmt.Errorf("读取源目录失败: %w", err), nil)
	}
	if !sourceInfo.IsDir() {
		return report, replaceDirError(report, ReplaceDirPhasePrepare, fmt.Errorf("源路径不是目录: %s", src), nil)
	}

	parent := filepath.Dir(dst)
	if err := ops.mkdirAll(parent, 0755); err != nil {
		return report, replaceDirError(report, ReplaceDirPhasePrepare, fmt.Errorf("创建目标父目录失败: %w", err), nil)
	}

	staging, err := ops.mkdirTemp(parent, "."+filepath.Base(dst)+".staging-")
	if err != nil {
		return report, replaceDirError(report, ReplaceDirPhasePrepare, fmt.Errorf("创建 staging 目录失败: %w", err), nil)
	}
	report.StagingPath = staging

	cleanupStaging := func(phase ReplaceDirPhase, primary error) (ReplaceDirReport, error) {
		if cleanupErr := ops.removeAll(staging); cleanupErr != nil {
			return report, replaceDirError(report, phase, primary,
				fmt.Errorf("清理 staging %q 失败: %w", staging, cleanupErr))
		}
		return report, replaceDirError(report, phase, primary, nil)
	}

	if err := ops.copyDirContents(src, staging); err != nil {
		return cleanupStaging(ReplaceDirPhasePrepare, fmt.Errorf("准备新内容失败: %w", err))
	}
	stagingInfo, err := ops.stat(staging)
	if err != nil {
		return cleanupStaging(ReplaceDirPhasePrepare, fmt.Errorf("验证 staging 目录失败: %w", err))
	}
	if !stagingInfo.IsDir() {
		return cleanupStaging(ReplaceDirPhasePrepare, fmt.Errorf("staging 路径不是目录: %s", staging))
	}

	report.Phase = ReplaceDirPhaseBackup
	_, err = ops.stat(dst)
	hasTarget := true
	if err != nil {
		if os.IsNotExist(err) {
			hasTarget = false
		} else {
			return cleanupStaging(ReplaceDirPhaseBackup, fmt.Errorf("读取旧目标失败: %w", err))
		}
	}
	report.HadExistingTarget = hasTarget

	if !hasTarget {
		report.Phase = ReplaceDirPhaseCommit
		if err := ops.rename(staging, dst); err != nil {
			// There is no old target to restore. Keep the staging directory for
			// diagnostics/retry and never remove an unrelated path.
			return report, replaceDirError(report, ReplaceDirPhaseCommit,
				fmt.Errorf("提交新目录失败: %w", err), nil)
		}
		report.Committed = true
		report.Phase = ReplaceDirPhaseCleanup
		return report, nil
	}

	// Create a unique, currently empty backup name. The placeholder is ours;
	// any pre-existing .old or transaction directory is intentionally left
	// untouched because it may be the only recovery source from an older run.
	backup, err := ops.mkdirTemp(parent, "."+filepath.Base(dst)+".backup-")
	if err != nil {
		return cleanupStaging(ReplaceDirPhaseBackup, fmt.Errorf("创建 backup 目录失败: %w", err))
	}
	report.BackupPath = backup
	if err := ops.removeAll(backup); err != nil {
		// The backup placeholder is still ours, so it is deliberately retained
		// when its cleanup fails. Do not touch the old target.
		return cleanupStaging(ReplaceDirPhaseBackup, fmt.Errorf("准备 backup 目录失败: %w", err))
	}
	if err := ops.rename(dst, backup); err != nil {
		// A failed backup rename leaves dst as the valid old version. The old
		// implementation removed dst here; that is explicitly forbidden.
		return cleanupStaging(ReplaceDirPhaseBackup, fmt.Errorf("备份旧目录失败: %w", err))
	}

	report.Phase = ReplaceDirPhaseCommit
	if err := ops.rename(staging, dst); err != nil {
		commitErr := fmt.Errorf("提交新目录失败: %w", err)
		report.Phase = ReplaceDirPhaseRestore
		restoreErr := ops.rename(backup, dst)
		if restoreErr == nil {
			report.Recovered = true
			report.Phase = ReplaceDirPhaseCommit
			return report, replaceDirError(report, ReplaceDirPhaseCommit, commitErr, nil)
		}
		// Both paths remain in place. In particular, do not remove staging or
		// backup after a failed recovery: one of them may be needed for manual
		// repair or the next startup.
		return report, replaceDirError(report, ReplaceDirPhaseRestore, commitErr,
			fmt.Errorf("恢复旧目录失败: %w", restoreErr))
	}

	report.Committed = true
	report.Phase = ReplaceDirPhaseCleanup
	if err := ops.removeAll(backup); err != nil {
		// The new target is already live. Keep backup and report it as a warning
		// instead of returning an installation error that could trigger a
		// destructive retry.
		report.BackupCleanupError = fmt.Errorf("清理旧版本 backup %q 失败: %w", backup, err)
		return report, nil
	}
	return report, nil
}

func FindFile(root, nameLower string) (string, error) {
	var found string
	errFound := fmt.Errorf("found")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(d.Name()) == nameLower {
			found = path
			return errFound
		}
		return nil
	})
	if err != nil && err != errFound {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("未找到 %s", nameLower)
	}
	return found, nil
}

func FindFirstByExt(root, ext string) (string, error) {
	ext = strings.ToLower(ext)
	var found string
	errFound := fmt.Errorf("found")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) == ext {
			found = path
			return errFound
		}
		return nil
	})
	if err != nil && err != errFound {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("未找到 %s 文件", ext)
	}
	return found, nil
}
