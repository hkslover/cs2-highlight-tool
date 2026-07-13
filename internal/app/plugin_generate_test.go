package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeGeneratePluginBatchJobsUsesAbsoluteDemoPaths(t *testing.T) {
	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "demo.dem")
	relativePath, err := filepath.Rel(mustGetwd(t), targetPath)
	if err != nil {
		t.Fatalf("relative path: %v", err)
	}

	jobs, err := normalizeGeneratePluginBatchJobs([]GeneratePluginJSONRequest{{DemoPath: relativePath}})
	if err != nil {
		t.Fatalf("normalize batch jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs=%d want 1", len(jobs))
	}
	abs, err := filepath.Abs(relativePath)
	if err != nil {
		t.Fatalf("absolute path: %v", err)
	}
	if jobs[0].DemoPath != abs {
		t.Fatalf("demo path=%q want %q", jobs[0].DemoPath, abs)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}
