package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ExportProduceWSLogs writes one sanitized diagnostic artifact for the current
// produce WebSocket session. In tests and headless development it falls back
// to the managed logs directory when no Wails context is available.
func (a *App) ExportProduceWSLogs() (string, error) {
	if a == nil || a.produceW == nil {
		return "", fmt.Errorf("制作 WebSocket 服务未初始化")
	}
	defaultDir := a.dataPath("logs")
	if err := os.MkdirAll(defaultDir, 0o755); err != nil {
		return "", fmt.Errorf("创建制作日志目录失败: %w", err)
	}
	filename := fmt.Sprintf("producews-export-%s.txt", time.Now().Format("20060102-150405.000"))
	targetPath := filepath.Join(defaultDir, filename)
	if a.ctx != nil {
		selected, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
			Title:           "导出制作日志",
			DefaultFilename: filename,
			Filters: []wruntime.FileFilter{
				{DisplayName: "Text File (*.txt)", Pattern: "*.txt"},
			},
		})
		if err != nil {
			return "", fmt.Errorf("打开制作日志保存对话框失败: %w", err)
		}
		if strings.TrimSpace(selected) == "" {
			return "", nil
		}
		targetPath = selected
	}
	if err := os.WriteFile(targetPath, []byte(a.produceW.ExportDiagnostics()), 0o644); err != nil {
		return "", fmt.Errorf("写入制作日志文件失败: %w", err)
	}
	return targetPath, nil
}
