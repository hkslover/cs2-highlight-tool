package producews

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnostics_KeepsBoundedRingAndSanitizesExport(t *testing.T) {
	diagnostics := NewDiagnostics(t.TempDir())
	diagnostics.Start()
	for index := 0; index < diagnosticRingLimit+7; index++ {
		diagnostics.Record(DiagnosticEvent{
			Level:     "info",
			Component: "producews",
			Stage:     "test",
			Action:    "event",
			Message:   "test event",
			Meta: map[string]string{
				"token": "must-not-leak",
			},
		})
	}
	diagnostics.Stop()

	snapshot := diagnostics.Snapshot()
	if len(snapshot.Events) != diagnosticRingLimit {
		t.Fatalf("ring size = %d, want %d", len(snapshot.Events), diagnosticRingLimit)
	}
	report := diagnostics.ExportText(WSState{}, QueueState{}, TakeStatusSnapshot{})
	if strings.Contains(report, "must-not-leak") {
		t.Fatalf("diagnostic export leaked credential: %s", report)
	}
	if !strings.Contains(report, "\"token\":\"***\"") {
		t.Fatalf("diagnostic export did not preserve redacted structured field: %s", report)
	}
}

func TestDiagnostics_RotatesAndWritesUniqueIncidentReports(t *testing.T) {
	diagnostics := NewDiagnostics(t.TempDir())
	if err := os.MkdirAll(diagnostics.logDir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(diagnostics.logPath, bytes.Repeat([]byte("x"), diagnosticLogMaxBytes), 0o644); err != nil {
		t.Fatalf("seed diagnostic log: %v", err)
	}
	diagnostics.Start()
	diagnostics.Record(DiagnosticEvent{Level: "info", Component: "producews", Stage: "test", Action: "rotate", Message: "rotate"})
	diagnostics.Stop()
	if _, err := os.Stat(diagnostics.logPath + ".1"); err != nil {
		t.Fatalf("rotated file missing: %v", err)
	}

	first, err := diagnostics.WriteIncident("token=secret", WSState{}, QueueState{}, TakeStatusSnapshot{})
	if err != nil {
		t.Fatalf("first incident: %v", err)
	}
	second, err := diagnostics.WriteIncident("token=secret", WSState{}, QueueState{}, TakeStatusSnapshot{})
	if err != nil {
		t.Fatalf("second incident: %v", err)
	}
	if first == second {
		t.Fatalf("incident paths collide: %q", first)
	}
	content, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read incident: %v", err)
	}
	if strings.Contains(string(content), "secret") {
		t.Fatalf("incident leaked secret: %s", content)
	}
	if filepath.Ext(first) != ".txt" {
		t.Fatalf("incident extension = %q", filepath.Ext(first))
	}
}

func TestDiagnostics_ExportRedactsAllDemoPaths(t *testing.T) {
	diagnostics := NewDiagnostics(t.TempDir())
	pluginLog := "playdemo /private/imports/secret-match.dem\n"
	if err := os.MkdirAll(diagnostics.logDir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(diagnostics.pluginLogPath, []byte(pluginLog), 0o644); err != nil {
		t.Fatalf("write plugin log: %v", err)
	}

	report := diagnostics.ExportText(
		WSState{},
		QueueState{
			CurrentDemoPath: "/private/imports/secret-match.dem",
			Demos:           []string{"/private/imports/secret-match.dem"},
			LastError:       "等待确认超时: /private/imports/secret-match.dem",
		},
		TakeStatusSnapshot{
			Items:     []TakeStatus{{DemoPath: "/private/imports/secret-match.dem"}},
			LastEvent: &TakeStatus{DemoPath: "/private/imports/secret-match.dem"},
		},
	)
	if strings.Contains(report, "/private/imports/secret-match.dem") || strings.Contains(report, "secret-match.dem") {
		t.Fatalf("diagnostic export leaked demo path: %s", report)
	}
	if !strings.Contains(report, "<redacted-demo>") {
		t.Fatalf("diagnostic export omitted redaction marker: %s", report)
	}
}
