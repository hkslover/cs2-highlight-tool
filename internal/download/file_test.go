package download

import (
	"bytes"
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

func TestCopyReaderAtomicReadFailurePreservesExistingTarget(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, "match.dem")
	if err := os.WriteFile(targetPath, []byte("previous-demo"), 0644); err != nil {
		t.Fatalf("write existing target: %v", err)
	}

	reader := &failAfterBytesReader{data: []byte("partial-demo"), err: errors.New("injected source read failure")}
	err := CopyReaderAtomic(context.Background(), reader, targetPath)
	if err == nil || !errors.Is(err, reader.err) {
		t.Fatalf("CopyReaderAtomic error = %v, want source read failure", err)
	}
	assertFileContent(t, targetPath, "previous-demo")
	assertNoAtomicCopyTemps(t, root, "match.dem")
}

func TestCopyReaderAtomicReadFailureLeavesNewTargetAbsent(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, "match.dem")
	reader := &failAfterBytesReader{data: []byte("partial-demo"), err: errors.New("injected source read failure")}

	err := CopyReaderAtomic(context.Background(), reader, targetPath)
	if err == nil || !errors.Is(err, reader.err) {
		t.Fatalf("CopyReaderAtomic error = %v, want source read failure", err)
	}
	if _, statErr := os.Stat(targetPath); !os.IsNotExist(statErr) {
		t.Fatalf("failed new target exists, stat err=%v", statErr)
	}
	assertNoAtomicCopyTemps(t, root, "match.dem")
}

func TestCopyReaderAtomicCloseFailurePreservesExistingTarget(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, "match.dem")
	if err := os.WriteFile(targetPath, []byte("previous-demo"), 0644); err != nil {
		t.Fatalf("write existing target: %v", err)
	}
	closeErr := errors.New("injected temp close failure")
	ops := defaultAtomicCopyOps()
	ops.createTemp = func(dir, pattern string) (string, atomicCopyTemp, error) {
		file, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return "", nil, err
		}
		return file.Name(), &closeErrorTemp{File: file, err: closeErr}, nil
	}

	err := copyReaderAtomicWithOps(context.Background(), bytes.NewReader([]byte("complete-demo")), targetPath, 0644, ops)
	if err == nil || !errors.Is(err, closeErr) {
		t.Fatalf("copyReaderAtomicWithOps error = %v, want close failure", err)
	}
	assertFileContent(t, targetPath, "previous-demo")
	assertNoAtomicCopyTemps(t, root, "match.dem")
}

func TestCopyReaderAtomicCommitFailureRestoresExistingTarget(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, "match.dem")
	if err := os.WriteFile(targetPath, []byte("previous-demo"), 0644); err != nil {
		t.Fatalf("write existing target: %v", err)
	}
	commitErr := errors.New("injected commit rename failure")
	ops := defaultAtomicCopyOps()
	renameCalls := 0
	ops.rename = func(old, new string) error {
		renameCalls++
		if renameCalls == 2 {
			return commitErr
		}
		return os.Rename(old, new)
	}

	err := copyReaderAtomicWithOps(context.Background(), bytes.NewReader([]byte("new-demo")), targetPath, 0644, ops)
	if err == nil || !errors.Is(err, commitErr) {
		t.Fatalf("copyReaderAtomicWithOps error = %v, want commit failure", err)
	}
	assertFileContent(t, targetPath, "previous-demo")
	assertNoAtomicCopyTemps(t, root, "match.dem")
}

func TestCopyReaderAtomicCommitFailureLeavesNewTargetAbsent(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, "match.dem")
	commitErr := errors.New("injected commit rename failure")
	ops := defaultAtomicCopyOps()
	ops.rename = func(old, new string) error { return commitErr }

	err := copyReaderAtomicWithOps(context.Background(), bytes.NewReader([]byte("new-demo")), targetPath, 0644, ops)
	if err == nil || !errors.Is(err, commitErr) {
		t.Fatalf("copyReaderAtomicWithOps error = %v, want commit failure", err)
	}
	if _, statErr := os.Stat(targetPath); !os.IsNotExist(statErr) {
		t.Fatalf("failed new target exists, stat err=%v", statErr)
	}
	assertNoAtomicCopyTemps(t, root, "match.dem")
}

func TestIsLikelyDemoFileRejectsObviousCorruption(t *testing.T) {
	root := t.TempDir()
	validPath := filepath.Join(root, "valid.dem")
	if err := os.WriteFile(validPath, []byte("PBDEMS2\x00truncated-tail-is-unknown"), 0644); err != nil {
		t.Fatalf("write valid-shaped demo: %v", err)
	}
	valid, err := IsLikelyDemoFile(validPath)
	if err != nil || !valid {
		t.Fatalf("IsLikelyDemoFile(valid) = %v, %v; want true", valid, err)
	}

	invalidPath := filepath.Join(root, "invalid.dem")
	if err := os.WriteFile(invalidPath, []byte("partial"), 0644); err != nil {
		t.Fatalf("write invalid demo: %v", err)
	}
	valid, err = IsLikelyDemoFile(invalidPath)
	if err != nil || valid {
		t.Fatalf("IsLikelyDemoFile(invalid) = %v, %v; want false", valid, err)
	}
}

type failAfterBytesReader struct {
	data []byte
	err  error
}

func (r *failAfterBytesReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

type closeErrorTemp struct {
	*os.File
	err error
}

func (f *closeErrorTemp) Close() error {
	closeErr := f.File.Close()
	if closeErr != nil {
		return closeErr
	}
	return f.err
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s = %q, want %q", path, string(data), want)
	}
}

func assertNoAtomicCopyTemps(t *testing.T, dir, base string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "."+base+".copy-*.tmp"))
	if err != nil {
		t.Fatalf("glob atomic copy temps: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("atomic copy temp files remain: %v", matches)
	}
}
