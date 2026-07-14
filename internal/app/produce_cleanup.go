package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"cs2-highlight-tool-v2/internal/plugingen"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) cleanupProduceTemporaryFiles(state *produceSessionRuntime) {
	if state == nil || strings.TrimSpace(state.batchDir) == "" {
		return
	}

	takePaths := make(map[string]struct{}, len(state.plansByTake)*2)
	takeDirs := make(map[string]struct{}, len(state.plansByTake))
	demoDirs := make(map[string]struct{}, len(state.demoSubDirs))
	failedTakes := make(map[string]struct{})
	a.produceStateMu.Lock()
	for key, file := range a.produceState.takeFiles {
		if strings.EqualFold(strings.TrimSpace(file.Status), "failed") {
			failedTakes[key] = struct{}{}
		}
	}
	a.produceStateMu.Unlock()

	for _, plan := range state.plansByTake {
		if _, failed := failedTakes[takeFileKey(plan.DemoPath, plan.TakeIndex, plan.View)]; failed {
			continue
		}
		videoPath, audioPath := expectedTakePaths(state, plan)
		if strings.TrimSpace(videoPath) != "" {
			takePaths[videoPath] = struct{}{}
			demoDirs[filepath.Dir(videoPath)] = struct{}{}
		}
		if strings.TrimSpace(audioPath) != "" {
			takePaths[audioPath] = struct{}{}
			takeDirs[filepath.Dir(audioPath)] = struct{}{}
		}
	}

	for demoPath, subDir := range state.demoSubDirs {
		dp := strings.TrimSpace(demoPath)
		if dp == "" {
			continue
		}
		normalizedSubDir := strings.TrimSpace(subDir)
		if normalizedSubDir == "" {
			normalizedSubDir = plugingen.SanitizeDemoSubDirName(dp)
		}
		demoDir := filepath.Join(state.batchDir, normalizedSubDir)
		demoDirs[demoDir] = struct{}{}
	}

	if state.keepIntermediateFiles {
		for demoDir := range demoDirs {
			if err := removeMuxTmpFiles(demoDir); err != nil {
				a.logProduceCleanupError(demoDir, err)
			}
		}
		return
	}

	for path := range takePaths {
		if err := removeFileIfExists(path); err != nil {
			a.logProduceCleanupError(path, err)
		}
	}

	for dir := range takeDirs {
		if err := removeEmptyDirUpward(dir, state.batchDir); err != nil {
			a.logProduceCleanupError(dir, err)
		}
	}

	for demoDir := range demoDirs {
		if err := removeMuxTmpFiles(demoDir); err != nil {
			a.logProduceCleanupError(demoDir, err)
		}
		if err := removeEmptyDirUpward(demoDir, state.batchDir); err != nil {
			a.logProduceCleanupError(demoDir, err)
		}
	}
}

func (a *App) logProduceCleanupError(path string, err error) {
	if err == nil || a.ctx == nil {
		return
	}
	wailsruntime.LogWarning(a.ctx, fmt.Sprintf("cleanup temp file failed: path=%s err=%v", path, err))
}

func removeFileIfExists(path string) error {
	target := strings.TrimSpace(path)
	if target == "" {
		return nil
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func removeMuxTmpFiles(dir string) error {
	targetDir := strings.TrimSpace(dir)
	if targetDir == "" {
		return nil
	}
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".mux.tmp.mp4") {
			continue
		}
		if err := removeFileIfExists(filepath.Join(targetDir, name)); err != nil {
			return err
		}
	}
	return nil
}

func removeEmptyDirUpward(dir string, stopDir string) error {
	current := filepath.Clean(strings.TrimSpace(dir))
	stop := filepath.Clean(strings.TrimSpace(stopDir))
	if current == "" {
		return nil
	}

	for {
		if current == "." || current == string(filepath.Separator) {
			return nil
		}
		if err := os.Remove(current); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			if isDirNotEmpty(err) {
				return nil
			}
			return err
		}
		if stop != "" && samePath(current, stop) {
			return nil
		}
		next := filepath.Dir(current)
		if next == current {
			return nil
		}
		current = next
	}
}

func isDirNotEmpty(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ENOTEMPTY) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "directory not empty")
}

func (a *App) requestCloseCS2Process(state *produceSessionRuntime) {
	if state == nil || state.closeRequested {
		return
	}
	state.closeRequested = true

	if state.cs2PID <= 0 {
		state.closeDone = true
		return
	}

	if state.queueSucceeded && a.produceW != nil {
		if err := a.produceW.RequestGracefulExit(); err == nil {
			state.gracefulExit = true
			state.gracefulExitAt = time.Now().Add(produceGracefulExitWait)
			return
		} else if a.ctx != nil {
			wailsruntime.LogWarning(a.ctx, fmt.Sprintf("request graceful cs2 exit failed, falling back to pid close: %v", err))
		}
	}
	a.closeCS2ProcessByPID(state)
}

func (a *App) advanceCloseCS2Process(state *produceSessionRuntime, now time.Time) {
	if state == nil || !state.closeRequested || state.closeDone || !state.gracefulExit {
		return
	}
	if a.produceW != nil && a.produceW.GracefulExitStatus().Completed {
		state.closeDone = true
		return
	}
	if now.Before(state.gracefulExitAt) {
		return
	}
	if a.produceW != nil {
		a.produceW.ExpectGracefulExitFallback()
	}
	state.gracefulExit = false
	a.closeCS2ProcessByPID(state)
}

func (a *App) closeCS2ProcessByPID(state *produceSessionRuntime) {
	if state == nil || state.closeDone {
		return
	}
	defer func() { state.closeDone = true }()
	if state.cs2PID <= 0 {
		return
	}
	if err := closeCS2ProcessByPIDFn(state.cs2PID); err != nil && a.ctx != nil {
		wailsruntime.LogError(a.ctx, fmt.Sprintf("close cs2 process failed (pid=%d): %v", state.cs2PID, err))
	}
}

// ---- file utility functions moved from produce_session.go ----

func copyFileWithReplace(src string, dst string) error {
	source := filepath.Clean(strings.TrimSpace(src))
	target := filepath.Clean(strings.TrimSpace(dst))
	if source == "" {
		return fmt.Errorf("源路径为空")
	}
	if target == "" {
		return fmt.Errorf("目标路径为空")
	}
	if samePath(source, target) {
		return nil
	}
	if _, err := os.Stat(source); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("源文件不存在: %s", source)
		}
		return fmt.Errorf("读取源文件失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("覆盖目标文件失败: %w", err)
	}
	if err := copyFileStreamAtomic(source, target); err != nil {
		return fmt.Errorf("复制文件失败: %w", err)
	}
	return nil
}

func copyFileStreamAtomic(src string, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	info, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".copy-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	writeOK := false
	defer func() {
		if !writeOK {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmpFile, sourceFile); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, info.Mode()); err != nil {
		return err
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return err
	}
	writeOK = true
	return nil
}

func copyFile(src string, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

func removeDirIfEmpty(path string) error {
	target := strings.TrimSpace(path)
	if target == "" {
		return nil
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(entries) > 0 {
		return nil
	}
	return os.Remove(target)
}
