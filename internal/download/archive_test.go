package download

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, root, name, contents string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestReplaceDirWithContentsCommitsNewContentAndPreservesUnknownResidue(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "component")
	writeTestFile(t, src, "bin/tool.exe", "new")
	writeTestFile(t, dst, "bin/tool.exe", "old")
	stale := filepath.Join(root, "component.old")
	writeTestFile(t, stale, "recovery-marker", "leave-me")

	report, err := replaceDirWithContents(defaultReplaceDirFS(), src, dst)
	if err != nil {
		t.Fatalf("replaceDirWithContents error: %v", err)
	}
	if !report.HadExistingTarget || !report.Committed || report.Recovered {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.BackupCleanupError != nil {
		t.Fatalf("unexpected backup cleanup warning: %v", report.BackupCleanupError)
	}
	if got := readTestFile(t, filepath.Join(dst, "bin/tool.exe")); got != "new" {
		t.Fatalf("target content = %q, want new", got)
	}
	if got := readTestFile(t, filepath.Join(stale, "recovery-marker")); got != "leave-me" {
		t.Fatalf("pre-existing residue content = %q, want leave-me", got)
	}
	if _, err := os.Stat(report.StagingPath); !os.IsNotExist(err) {
		t.Fatalf("staging path still exists after commit: %s (stat err %v)", report.StagingPath, err)
	}
	if _, err := os.Stat(report.BackupPath); !os.IsNotExist(err) {
		t.Fatalf("backup path still exists after cleanup: %s (stat err %v)", report.BackupPath, err)
	}
}

func TestReplaceDirWithContentsCopyFailureLeavesOldTargetUntouched(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "component")
	writeTestFile(t, src, "new.txt", "new")
	oldPath := writeTestFile(t, dst, "old.txt", "old")
	copyErr := errors.New("injected copy failure")

	ops := defaultReplaceDirFS()
	ops.copyDirContents = func(string, string) error { return copyErr }
	report, err := replaceDirWithContents(ops, src, dst)
	if !errors.Is(err, copyErr) {
		t.Fatalf("error = %v, want copy failure", err)
	}
	if report.Phase != ReplaceDirPhasePrepare {
		t.Fatalf("phase = %q, want prepare", report.Phase)
	}
	if got := readTestFile(t, oldPath); got != "old" {
		t.Fatalf("old target content = %q, want old", got)
	}
	if _, statErr := os.Stat(report.StagingPath); !os.IsNotExist(statErr) {
		t.Fatalf("failed staging was not cleaned: %s (stat err %v)", report.StagingPath, statErr)
	}
}

func TestReplaceDirWithContentsBackupRenameFailureDoesNotDeleteTarget(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "component")
	writeTestFile(t, src, "new.txt", "new")
	oldPath := writeTestFile(t, dst, "old.txt", "old")
	renameErr := errors.New("injected backup rename failure")

	ops := defaultReplaceDirFS()
	var renameCalls int
	ops.rename = func(old, new string) error {
		renameCalls++
		if renameCalls == 1 {
			return renameErr
		}
		return os.Rename(old, new)
	}
	report, err := replaceDirWithContents(ops, src, dst)
	if !errors.Is(err, renameErr) {
		t.Fatalf("error = %v, want backup rename failure", err)
	}
	if report.Phase != ReplaceDirPhaseBackup {
		t.Fatalf("phase = %q, want backup", report.Phase)
	}
	if got := readTestFile(t, oldPath); got != "old" {
		t.Fatalf("old target content = %q, want old", got)
	}
	if _, statErr := os.Stat(report.StagingPath); !os.IsNotExist(statErr) {
		t.Fatalf("failed staging was not cleaned: %s (stat err %v)", report.StagingPath, statErr)
	}
}

func TestReplaceDirWithContentsCommitFailureRestoresOldAndRetainsStaging(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "component")
	writeTestFile(t, src, "new.txt", "new")
	oldPath := writeTestFile(t, dst, "old.txt", "old")
	commitErr := errors.New("injected commit rename failure")

	ops := defaultReplaceDirFS()
	var renameCalls int
	ops.rename = func(old, new string) error {
		renameCalls++
		if renameCalls == 2 {
			return commitErr
		}
		return os.Rename(old, new)
	}
	report, err := replaceDirWithContents(ops, src, dst)
	if !errors.Is(err, commitErr) {
		t.Fatalf("error = %v, want commit failure", err)
	}
	if !report.Recovered || report.Phase != ReplaceDirPhaseCommit {
		t.Fatalf("unexpected report after recovery: %+v", report)
	}
	if got := readTestFile(t, oldPath); got != "old" {
		t.Fatalf("restored target content = %q, want old", got)
	}
	if _, statErr := os.Stat(report.StagingPath); statErr != nil {
		t.Fatalf("staging should be retained after commit failure: %s (stat err %v)", report.StagingPath, statErr)
	}
	if _, statErr := os.Stat(report.BackupPath); !os.IsNotExist(statErr) {
		t.Fatalf("restored backup path should be gone: %s (stat err %v)", report.BackupPath, statErr)
	}
	var replaceErr *ReplaceDirError
	if !errors.As(err, &replaceErr) || !replaceErr.Recovered {
		t.Fatalf("error does not report successful recovery: %T %v", err, err)
	}
}

func TestReplaceDirWithContentsRecoveryFailureKeepsBothRecoveryPaths(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "component")
	writeTestFile(t, src, "new.txt", "new")
	writeTestFile(t, dst, "old.txt", "old")
	commitErr := errors.New("injected commit rename failure")
	restoreErr := errors.New("injected restore rename failure")

	ops := defaultReplaceDirFS()
	var renameCalls int
	ops.rename = func(old, new string) error {
		renameCalls++
		switch renameCalls {
		case 2:
			return commitErr
		case 3:
			return restoreErr
		default:
			return os.Rename(old, new)
		}
	}
	report, err := replaceDirWithContents(ops, src, dst)
	if !errors.Is(err, commitErr) || !errors.Is(err, restoreErr) {
		t.Fatalf("error = %v, want both commit and restore failures", err)
	}
	if report.Recovered || report.Phase != ReplaceDirPhaseRestore {
		t.Fatalf("unexpected report after failed recovery: %+v", report)
	}
	if _, statErr := os.Stat(report.StagingPath); statErr != nil {
		t.Fatalf("staging must be retained: %s (stat err %v)", report.StagingPath, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(report.StagingPath, "new.txt")); statErr != nil {
		t.Fatalf("staging content missing: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(report.BackupPath, "old.txt")); statErr != nil {
		t.Fatalf("backup content missing: %v", statErr)
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Fatalf("target should remain absent after failed recovery, stat err = %v", statErr)
	}
	if !strings.Contains(err.Error(), report.StagingPath) || !strings.Contains(err.Error(), report.BackupPath) {
		t.Fatalf("error %q does not include recovery paths", err)
	}
}

func TestReplaceDirWithContentsBackupCleanupFailureIsWarningAfterCommit(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "component")
	writeTestFile(t, src, "new.txt", "new")
	writeTestFile(t, dst, "old.txt", "old")
	cleanupErr := errors.New("injected backup cleanup failure")

	ops := defaultReplaceDirFS()
	var removeCalls int
	ops.removeAll = func(path string) error {
		removeCalls++
		if removeCalls == 2 {
			return cleanupErr
		}
		return os.RemoveAll(path)
	}
	report, err := replaceDirWithContents(ops, src, dst)
	if err != nil {
		t.Fatalf("cleanup warning must not fail committed install: %v", err)
	}
	if !report.Committed || report.BackupCleanupError == nil {
		t.Fatalf("unexpected report: %+v", report)
	}
	if !errors.Is(report.BackupCleanupError, cleanupErr) {
		t.Fatalf("backup cleanup error = %v, want injected error", report.BackupCleanupError)
	}
	if got := readTestFile(t, filepath.Join(dst, "new.txt")); got != "new" {
		t.Fatalf("target content = %q, want new", got)
	}
	if got := readTestFile(t, filepath.Join(report.BackupPath, "old.txt")); got != "old" {
		t.Fatalf("retained backup content = %q, want old", got)
	}
}

func TestReplaceDirWithContentsFirstInstallCommitFailureRetainsStaging(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "component")
	writeTestFile(t, src, "new.txt", "new")
	commitErr := errors.New("injected first-install commit failure")

	ops := defaultReplaceDirFS()
	ops.rename = func(string, string) error { return commitErr }
	report, err := replaceDirWithContents(ops, src, dst)
	if !errors.Is(err, commitErr) {
		t.Fatalf("error = %v, want commit failure", err)
	}
	if report.HadExistingTarget || report.BackupPath != "" {
		t.Fatalf("first install unexpectedly has backup: %+v", report)
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Fatalf("target should remain absent, stat err = %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(report.StagingPath, "new.txt")); statErr != nil {
		t.Fatalf("staging content missing: %v", statErr)
	}
}

func TestReplaceDirWithContentsParentFailureDoesNotTouchTarget(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "component")
	writeTestFile(t, src, "new.txt", "new")
	oldPath := writeTestFile(t, dst, "old.txt", "old")
	parentErr := errors.New("injected target parent not writable")

	ops := defaultReplaceDirFS()
	ops.mkdirAll = func(string, os.FileMode) error { return parentErr }
	report, err := replaceDirWithContents(ops, src, dst)
	if !errors.Is(err, parentErr) {
		t.Fatalf("error = %v, want parent failure", err)
	}
	if report.StagingPath != "" || report.BackupPath != "" {
		t.Fatalf("no transaction paths should be created before parent setup: %+v", report)
	}
	if got := readTestFile(t, oldPath); got != "old" {
		t.Fatalf("old target content = %q, want old", got)
	}
}

func TestReplaceDirErrorUnwrapsPrimaryAndRecovery(t *testing.T) {
	primary := errors.New("primary")
	recovery := errors.New("recovery")
	report := ReplaceDirReport{
		TargetPath:  filepath.Join("tmp", "component"),
		StagingPath: filepath.Join("tmp", "staging"),
		BackupPath:  filepath.Join("tmp", "backup"),
	}
	err := replaceDirError(report, ReplaceDirPhaseRestore, primary, fmt.Errorf("wrapped: %w", recovery))
	if !errors.Is(err, primary) || !errors.Is(err, recovery) {
		t.Fatalf("error = %v, want both causes", err)
	}
	if !strings.Contains(err.Error(), report.StagingPath) || !strings.Contains(err.Error(), report.BackupPath) {
		t.Fatalf("error %q does not include paths", err)
	}
}
