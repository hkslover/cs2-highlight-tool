package download

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileWithContextCancelRemovesPartialFile(t *testing.T) {
	chunkWritten := make(chan struct{})
	releaseHandler := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("partial"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		close(chunkWritten)
		<-releaseHandler
	}))
	defer server.Close()
	defer close(releaseHandler)

	targetPath := filepath.Join(t.TempDir(), "download.tmp")
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- FileWithContext(ctx, server.URL, targetPath, nil)
	}()

	<-chunkWritten
	cancel()
	err := <-errCh
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("FileWithContext error = %v, want ErrCanceled", err)
	}
	if _, statErr := os.Stat(targetPath); !os.IsNotExist(statErr) {
		t.Fatalf("partial target exists after cancel, stat err = %v", statErr)
	}
}

func TestFileWithContextAllowsSlowContinuousDownload(t *testing.T) {
	originalIdleTimeout := downloadIdleTimeout
	downloadIdleTimeout = 50 * time.Millisecond
	t.Cleanup(func() { downloadIdleTimeout = originalIdleTimeout })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "6")
		flusher, _ := w.(http.Flusher)
		for _, chunk := range []string{"a", "b", "c", "d", "e", "f"} {
			_, _ = w.Write([]byte(chunk))
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer server.Close()

	targetPath := filepath.Join(t.TempDir(), "slow-download.tmp")
	if err := FileWithContext(context.Background(), server.URL, targetPath, nil); err != nil {
		t.Fatalf("FileWithContext slow download: %v", err)
	}
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != "abcdef" {
		t.Fatalf("downloaded data = %q, want %q", data, "abcdef")
	}
}

func TestFileWithContextRejectsTruncatedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		_, _ = w.Write([]byte("short"))
	}))
	defer server.Close()

	targetPath := filepath.Join(t.TempDir(), "truncated-download.tmp")
	err := FileWithContext(context.Background(), server.URL, targetPath, nil)
	if err == nil || !strings.Contains(err.Error(), "下载不完整") {
		t.Fatalf("FileWithContext error = %v, want incomplete download error", err)
	}
	if _, statErr := os.Stat(targetPath); !os.IsNotExist(statErr) {
		t.Fatalf("truncated target exists after failure, stat err = %v", statErr)
	}
}

func TestFileWithContextRejectsIdleResponse(t *testing.T) {
	originalIdleTimeout := downloadIdleTimeout
	downloadIdleTimeout = 20 * time.Millisecond
	t.Cleanup(func() { downloadIdleTimeout = originalIdleTimeout })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if flusher, ok := w.(http.Flusher); ok {
			_, _ = w.Write([]byte("first"))
			flusher.Flush()
		}
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	targetPath := filepath.Join(t.TempDir(), "stalled-download.tmp")
	err := FileWithContext(context.Background(), server.URL, targetPath, nil)
	if err == nil || !strings.Contains(err.Error(), "下载读取超时") {
		t.Fatalf("FileWithContext error = %v, want idle timeout error", err)
	}
	if _, statErr := os.Stat(targetPath); !os.IsNotExist(statErr) {
		t.Fatalf("stalled target exists after failure, stat err = %v", statErr)
	}
}
