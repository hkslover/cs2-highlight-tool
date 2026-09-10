package envsetup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallHLAEFromArchiveValidatesBeforeReplacingExistingInstall(t *testing.T) {
	tests := []struct {
		name     string
		entries  map[string][]byte
		wantPart string
	}{
		{
			name: "missing hook dll",
			entries: map[string][]byte{
				"release/hlae.exe":      []byte("new-hlae"),
				"release/changelog.xml": []byte("<changelog><version>2.0.0</version></changelog>"),
			},
			wantPart: "AfxHookSource2.dll",
		},
		{
			name: "missing version",
			entries: map[string][]byte{
				"release/hlae.exe":               []byte("new-hlae"),
				"release/x64/AfxHookSource2.dll": []byte("new-hook"),
				"release/changelog.xml":          []byte("<changelog></changelog>"),
			},
			wantPart: "changelog.xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exeDir := t.TempDir()
			dataDir := t.TempDir()
			svc := NewWithDataDir(exeDir, dataDir, "test")
			svc.Startup(nil)

			targetDir := filepath.Join(dataDir, "hlae")
			markerPath := filepath.Join(targetDir, "preserve.txt")
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				t.Fatalf("create old HLAE directory: %v", err)
			}
			if err := os.WriteFile(markerPath, []byte("known-good-old-install"), 0644); err != nil {
				t.Fatalf("write old HLAE marker: %v", err)
			}

			archivePath := createZipForTest(t, tt.entries)
			err := svc.installHLAEFromArchive(archivePath)
			if err == nil {
				t.Fatal("installHLAEFromArchive succeeded for an invalid archive")
			}
			if !strings.Contains(err.Error(), tt.wantPart) {
				t.Fatalf("error = %q, want substring %q", err, tt.wantPart)
			}
			data, readErr := os.ReadFile(markerPath)
			if readErr != nil {
				t.Fatalf("old HLAE marker was removed: %v", readErr)
			}
			if string(data) != "known-good-old-install" {
				t.Fatalf("old HLAE marker = %q, want known-good-old-install", data)
			}
			if _, statErr := os.Stat(filepath.Join(targetDir, "HLAE.exe")); !os.IsNotExist(statErr) {
				t.Fatalf("invalid archive touched target HLAE.exe, stat err = %v", statErr)
			}
		})
	}
}
