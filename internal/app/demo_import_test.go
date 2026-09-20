package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareRawDemoFiles_DedupAndOverwrite(t *testing.T) {
	exeDir := t.TempDir()
	dataDir := t.TempDir()
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "match.dem")
	if err := os.WriteFile(sourcePath, []byte("v1"), 0644); err != nil {
		t.Fatalf("write source demo: %v", err)
	}

	app := &App{exeDir: exeDir, dataDir: dataDir}
	first, err := app.prepareRawDemoFiles([]string{sourcePath, sourcePath})
	if err != nil {
		t.Fatalf("prepareRawDemoFiles first call: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first result len = %d, want 1", len(first))
	}
	firstPath := first[0]
	if filepath.Base(firstPath) != "match.dem" {
		t.Fatalf("first target basename = %q, want %q", filepath.Base(firstPath), "match.dem")
	}
	if gotRoot := filepath.Join(dataDir, "demo", "raw"); filepath.Dir(filepath.Dir(firstPath)) != gotRoot {
		t.Fatalf("first target root = %q, want under %q", filepath.Dir(filepath.Dir(firstPath)), gotRoot)
	}
	if _, err := os.Stat(filepath.Join(exeDir, "demo", "raw")); !os.IsNotExist(err) {
		t.Fatalf("legacy exeDir raw directory should not exist, stat err=%v", err)
	}
	content, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read first target file: %v", err)
	}
	if string(content) != "v1" {
		t.Fatalf("first target content = %q, want %q", string(content), "v1")
	}

	if err := os.WriteFile(sourcePath, []byte("v2"), 0644); err != nil {
		t.Fatalf("rewrite source demo: %v", err)
	}
	second, err := app.prepareRawDemoFiles([]string{sourcePath})
	if err != nil {
		t.Fatalf("prepareRawDemoFiles second call: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second result len = %d, want 1", len(second))
	}
	if second[0] != firstPath {
		t.Fatalf("second target path = %q, want %q", second[0], firstPath)
	}
	content, err = os.ReadFile(second[0])
	if err != nil {
		t.Fatalf("read second target file: %v", err)
	}
	if string(content) != "v2" {
		t.Fatalf("second target content = %q, want %q", string(content), "v2")
	}
}

func TestPrepareRawDemoFiles_SameNameDifferentSources(t *testing.T) {
	exeDir := t.TempDir()
	sourceRoot := t.TempDir()
	sourceA := filepath.Join(sourceRoot, "a", "match.dem")
	sourceB := filepath.Join(sourceRoot, "b", "match.dem")
	if err := os.MkdirAll(filepath.Dir(sourceA), 0755); err != nil {
		t.Fatalf("mkdir sourceA: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(sourceB), 0755); err != nil {
		t.Fatalf("mkdir sourceB: %v", err)
	}
	if err := os.WriteFile(sourceA, []byte("A"), 0644); err != nil {
		t.Fatalf("write sourceA: %v", err)
	}
	if err := os.WriteFile(sourceB, []byte("B"), 0644); err != nil {
		t.Fatalf("write sourceB: %v", err)
	}

	app := newTestApp(t, exeDir)
	got, err := app.prepareRawDemoFiles([]string{sourceA, sourceB})
	if err != nil {
		t.Fatalf("prepareRawDemoFiles: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("result len = %d, want 2", len(got))
	}
	if got[0] == got[1] {
		t.Fatalf("two different source demos should not map to same target path: %q", got[0])
	}
	if filepath.Base(got[0]) != "match.dem" || filepath.Base(got[1]) != "match.dem" {
		t.Fatalf("target basenames should both be match.dem, got %q and %q", filepath.Base(got[0]), filepath.Base(got[1]))
	}
	contentA, err := os.ReadFile(got[0])
	if err != nil {
		t.Fatalf("read targetA: %v", err)
	}
	contentB, err := os.ReadFile(got[1])
	if err != nil {
		t.Fatalf("read targetB: %v", err)
	}
	if string(contentA) == string(contentB) {
		t.Fatalf("expected different copied contents, got both %q", string(contentA))
	}
}
