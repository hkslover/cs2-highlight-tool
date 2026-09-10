package envsetup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallFFmpegFromArchiveValidatesLayoutBeforeReplacingExistingInstall
// covers the regression where an archive whose ffmpeg.exe is not directly
// inside a bin/ directory passed the extraction check, replaced the working
// installation, and only then failed the post-commit existence check.
func TestInstallFFmpegFromArchiveValidatesLayoutBeforeReplacingExistingInstall(t *testing.T) {
	tests := []struct {
		name    string
		entries map[string][]byte
	}{
		{
			name: "ffmpeg exe nested outside bin",
			entries: map[string][]byte{
				"release/tools/ffmpeg.exe": []byte("new-ffmpeg"),
			},
		},
		{
			name: "ffmpeg exe flat at archive root",
			entries: map[string][]byte{
				"ffmpeg.exe": []byte("new-ffmpeg"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataDir := t.TempDir()
			svc := NewWithDataDir(t.TempDir(), dataDir, "test")
			svc.Startup(nil)

			targetDir := filepath.Join(dataDir, "ffmpeg")
			oldExe := filepath.Join(targetDir, "bin", "ffmpeg.exe")
			if err := os.MkdirAll(filepath.Dir(oldExe), 0755); err != nil {
				t.Fatalf("create old ffmpeg directory: %v", err)
			}
			if err := os.WriteFile(oldExe, []byte("known-good-old-ffmpeg"), 0644); err != nil {
				t.Fatalf("write old ffmpeg.exe: %v", err)
			}
			markerPath := filepath.Join(targetDir, "preserve.txt")
			if err := os.WriteFile(markerPath, []byte("known-good-old-install"), 0644); err != nil {
				t.Fatalf("write old ffmpeg marker: %v", err)
			}

			archivePath := createZipForTest(t, tt.entries)
			err := svc.installFFmpegFromArchive(archivePath)
			if err == nil {
				t.Fatal("installFFmpegFromArchive succeeded for an unexpected layout")
			}
			if !strings.Contains(err.Error(), "bin/ffmpeg.exe") {
				t.Fatalf("error = %q, want substring %q", err, "bin/ffmpeg.exe")
			}
			data, readErr := os.ReadFile(oldExe)
			if readErr != nil {
				t.Fatalf("old ffmpeg.exe was removed: %v", readErr)
			}
			if string(data) != "known-good-old-ffmpeg" {
				t.Fatalf("old ffmpeg.exe = %q, want known-good-old-ffmpeg", data)
			}
			marker, markerErr := os.ReadFile(markerPath)
			if markerErr != nil {
				t.Fatalf("old ffmpeg install was replaced: %v", markerErr)
			}
			if string(marker) != "known-good-old-install" {
				t.Fatalf("old ffmpeg marker = %q, want known-good-old-install", marker)
			}
			assertNoReplaceDirResidue(t, dataDir, "ffmpeg")
		})
	}
}

func TestInstallFFmpegFromArchiveAcceptsBinLayoutAndReplacesOldVersion(t *testing.T) {
	dataDir := t.TempDir()
	svc := NewWithDataDir(t.TempDir(), dataDir, "test")
	svc.Startup(nil)

	targetDir := filepath.Join(dataDir, "ffmpeg")
	oldExe := filepath.Join(targetDir, "bin", "ffmpeg.exe")
	if err := os.MkdirAll(filepath.Dir(oldExe), 0755); err != nil {
		t.Fatalf("create old ffmpeg directory: %v", err)
	}
	if err := os.WriteFile(oldExe, []byte("old-ffmpeg"), 0644); err != nil {
		t.Fatalf("write old ffmpeg.exe: %v", err)
	}

	archivePath := createZipForTest(t, map[string][]byte{
		"package/bin/ffmpeg.exe": []byte("new-ffmpeg"),
		"package/readme.txt":     []byte("doc"),
	})
	if err := svc.installFFmpegFromArchive(archivePath); err != nil {
		t.Fatalf("installFFmpegFromArchive error: %v", err)
	}

	installedExe := filepath.Join(dataDir, "ffmpeg", "bin", "ffmpeg.exe")
	data, err := os.ReadFile(installedExe)
	if err != nil {
		t.Fatalf("read installed ffmpeg.exe: %v", err)
	}
	if string(data) != "new-ffmpeg" {
		t.Fatalf("installed ffmpeg.exe = %q, want new-ffmpeg", data)
	}
	assertNoReplaceDirResidue(t, dataDir, "ffmpeg")
}

func assertNoReplaceDirResidue(t *testing.T, parent, base string) {
	t.Helper()
	for _, pattern := range []string{"." + base + ".staging-*", "." + base + ".backup-*"} {
		matches, err := filepath.Glob(filepath.Join(parent, pattern))
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		if len(matches) != 0 {
			t.Fatalf("directory replacement residue remains for %s: %v", pattern, matches)
		}
	}
}
