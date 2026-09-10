package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func (a *App) prepareRawDemoFiles(paths []string) ([]string, error) {
	releaseFiles, dataDir, fileErr := a.beginManagedWorkspaceUse()
	if fileErr != nil {
		return nil, fileErr
	}
	defer releaseFiles()

	if len(paths) == 0 {
		return nil, nil
	}
	rawRoot := filepath.Join(dataDir, "demo", "raw")
	if err := os.MkdirAll(rawRoot, 0755); err != nil {
		return nil, fmt.Errorf("创建 demo raw 目录失败: %w", err)
	}

	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		sourcePath, err := normalizeSourcePath(path)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[sourcePath]; ok {
			continue
		}
		seen[sourcePath] = struct{}{}

		targetPath, err := copyDemoToRaw(sourcePath, rawRoot)
		if err != nil {
			return nil, err
		}
		result = append(result, targetPath)
	}

	return result, nil
}

func normalizeSourcePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("无效的 demo 文件路径")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析 demo 绝对路径失败: %w", err)
	}
	return filepath.Clean(absPath), nil
}

func copyDemoToRaw(sourcePath string, rawRoot string) (string, error) {
	targetDir := filepath.Join(rawRoot, hashSourcePath(sourcePath))
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("创建 demo 目标目录失败: %w", err)
	}

	targetPath := filepath.Join(targetDir, filepath.Base(sourcePath))
	if samePath(sourcePath, targetPath) {
		return targetPath, nil
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("打开 demo 源文件失败: %w", err)
	}
	defer sourceFile.Close()

	tempFile, err := os.CreateTemp(targetDir, "demo-copy-*.tmp")
	if err != nil {
		return "", fmt.Errorf("创建 demo 临时文件失败: %w", err)
	}
	tempPath := tempFile.Name()
	copyOK := false
	defer func() {
		if !copyOK {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := io.Copy(tempFile, sourceFile); err != nil {
		_ = tempFile.Close()
		return "", fmt.Errorf("复制 demo 文件失败: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("写入 demo 临时文件失败: %w", err)
	}

	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("覆盖旧 demo 文件失败: %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return "", fmt.Errorf("落盘 demo 文件失败: %w", err)
	}
	copyOK = true
	return targetPath, nil
}

func (a *App) cleanupLegacyRawDemoCopy(sourcePath string) {
	a.cleanupLegacyRawDemoCopyAt(sourcePath, a.dataRoot())
}

func (a *App) cleanupLegacyRawDemoCopyAt(sourcePath string, dataDir string) {
	if a == nil || strings.TrimSpace(sourcePath) == "" || strings.TrimSpace(dataDir) == "" {
		return
	}
	normalizedPath, err := normalizeSourcePath(sourcePath)
	if err != nil {
		return
	}
	rawRoot := filepath.Join(strings.TrimSpace(dataDir), "demo", "raw")
	legacyPath := filepath.Join(rawRoot, hashSourcePath(normalizedPath), filepath.Base(normalizedPath))
	if samePath(normalizedPath, legacyPath) {
		return
	}
	if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
		return
	}
	removeEmptyDirsUpTo(filepath.Dir(legacyPath), rawRoot)
}

func removeEmptyDirsUpTo(path string, stopPath string) {
	current := filepath.Clean(path)
	stop := filepath.Clean(stopPath)
	for {
		if samePath(current, stop) {
			return
		}
		if err := os.Remove(current); err != nil {
			return
		}
		parent := filepath.Dir(current)
		if samePath(parent, current) {
			return
		}
		current = parent
	}
}

func hashSourcePath(sourcePath string) string {
	normalized := filepath.Clean(strings.TrimSpace(sourcePath))
	if runtime.GOOS == "windows" {
		normalized = strings.ToLower(normalized)
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])[:16]
}

func samePath(a string, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
