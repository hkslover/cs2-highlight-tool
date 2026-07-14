package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cs2-highlight-tool-v2/internal/producews"
)

func TestExportProduceWSLogs_HeadlessWritesManagedLogFile(t *testing.T) {
	dataDir := t.TempDir()
	service := producews.NewDefault(nil)
	service.SetDiagnostics(producews.NewDiagnostics(dataDir))
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
	if !strings.Contains(string(content), "Produce WebSocket Diagnostics") {
		t.Fatalf("unexpected export content: %s", content)
	}
}
