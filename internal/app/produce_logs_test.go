package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"cs2-highlight-tool-v2/internal/producews"
)

func TestExportProduceWSLogs_HeadlessWritesManagedLogFile(t *testing.T) {
	dataDir := t.TempDir()
	service := producews.NewDefault(nil)
	diagnostics := producews.NewDiagnostics(dataDir)
	diagnostics.Record(producews.DiagnosticEvent{
		Level:   "error",
		Stage:   "reconnect",
		Action:  "grace_expired",
		Message: "等待游戏插件 WebSocket 重连超时",
	})
	service.SetDiagnostics(diagnostics)
	app := &App{dataDir: dataDir, exeDir: t.TempDir(), produceW: service}

	path, err := app.ExportProduceWSLogs()
	if err != nil {
		t.Fatalf("ExportProduceWSLogs: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(dataDir, "logs") {
		t.Fatalf("export dir = %q", filepath.Dir(path))
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if !bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("export is missing UTF-8 BOM: % x", content[:min(len(content), 3)])
	}
	if !utf8.Valid(content[len(utf8BOM):]) {
		t.Fatal("export content after BOM is not valid UTF-8")
	}
	text := string(content[len(utf8BOM):])
	if !strings.Contains(text, "Produce WebSocket Diagnostics") || !strings.Contains(text, "等待游戏插件 WebSocket 重连超时") {
		t.Fatalf("unexpected export content: %s", content)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
