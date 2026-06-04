package app

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

type OutputsStorageStats struct {
	OutputDir      string `json:"output_dir"`
	VideoCount     int    `json:"video_count"`
	TotalSizeBytes int64  `json:"total_size_bytes"`
}

type DemoStorageStats struct {
	DemoDir        string `json:"demo_dir"`
	DemoCount      int    `json:"demo_count"`
	TotalSizeBytes int64  `json:"total_size_bytes"`
}

func (a *App) GetOutputsStorageStats() (*OutputsStorageStats, error) {
	return a.outputsStorageStats()
}

func (a *App) ClearOutputsDirectory() (*OutputsStorageStats, error) {
	outputDir := a.fixedRecordOutputDir()
	if err := clearManagedDirectory(outputDir, "输出目录"); err != nil {
		return nil, err
	}
	return a.outputsStorageStats()
}

func (a *App) OpenOutputsDirectory() error {
	outputDir := a.fixedRecordOutputDir()
	return openManagedDirectory(outputDir, "输出目录")
}

func (a *App) GetDemoStorageStats() (*DemoStorageStats, error) {
	return a.demoStorageStats()
}

func (a *App) ClearDemoDirectory() (*DemoStorageStats, error) {
	demoDir := a.demoStorageDir()
	if err := clearManagedDirectory(demoDir, "Dem 目录"); err != nil {
		return nil, err
	}
	return a.demoStorageStats()
}

func (a *App) OpenDemoDirectory() error {
	demoDir := a.demoStorageDir()
	return openManagedDirectory(demoDir, "Dem 目录")
}

func (a *App) outputsStorageStats() (*OutputsStorageStats, error) {
	outputDir := a.fixedRecordOutputDir()
	stats := &OutputsStorageStats{OutputDir: outputDir}
	if err := collectManagedDirectoryStats(outputDir, "输出目录", func(path string) {
		if isOutputVideoFile(path) {
			stats.VideoCount++
		}
	}, &stats.TotalSizeBytes); err != nil {
		return nil, err
	}
	return stats, nil
}

func (a *App) demoStorageStats() (*DemoStorageStats, error) {
	demoDir := a.demoStorageDir()
	stats := &DemoStorageStats{DemoDir: demoDir}
	if err := collectManagedDirectoryStats(demoDir, "Dem 目录", func(path string) {
		if isDemoFile(path) {
			stats.DemoCount++
		}
	}, &stats.TotalSizeBytes); err != nil {
		return nil, err
	}
	return stats, nil
}

func (a *App) demoStorageDir() string {
	return a.dataPath("demo")
}

func clearManagedDirectory(dir string, label string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建%s失败: %w", label, err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("读取%s失败: %w", label, err)
	}
	for _, entry := range entries {
		childPath := filepath.Join(dir, entry.Name())
		if err := os.RemoveAll(childPath); err != nil {
			return fmt.Errorf("清理%s失败: %w", label, err)
		}
	}
	return nil
}

func openManagedDirectory(dir string, label string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建%s失败: %w", label, err)
	}

	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开%s失败: %w", label, err)
	}
	return nil
}

func collectManagedDirectoryStats(
	dir string,
	label string,
	countFile func(path string),
	totalSizeBytes *int64,
) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建%s失败: %w", label, err)
	}

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		*totalSizeBytes += info.Size()
		countFile(path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("统计%s失败: %w", label, err)
	}
	return nil
}

func isOutputVideoFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".mov", ".mkv", ".avi":
		return true
	default:
		return false
	}
}

func isDemoFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".dem")
}

// LogStorageStats describes the current log storage state.
type LogStorageStats struct {
	LogDir         string `json:"log_dir"`
	LogCount       int    `json:"log_count"`
	TotalSizeBytes int64  `json:"total_size_bytes"`
}

// GetLogStorageStats returns statistics about the logs directory.
func (a *App) GetLogStorageStats() (*LogStorageStats, error) {
	logDir := a.dataPath("logs")
	return collectLogStorageStats(logDir)
}

// ClearLogs removes all log files from the logs directory.
func (a *App) ClearLogs() (*LogStorageStats, error) {
	logDir := a.dataPath("logs")
	if err := clearManagedDirectory(logDir, "日志目录"); err != nil {
		return nil, err
	}
	return collectLogStorageStats(logDir)
}

func collectLogStorageStats(dir string) (*LogStorageStats, error) {
	stats := &LogStorageStats{LogDir: dir}
	err := collectManagedDirectoryStats(dir, "日志目录", func(path string) {
		if isLogFile(path) {
			stats.LogCount++
		}
	}, &stats.TotalSizeBytes)
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func isLogFile(path string) bool {
	return strings.HasPrefix(filepath.Base(path), "cs2ht-") && strings.EqualFold(filepath.Ext(path), ".log")
}
