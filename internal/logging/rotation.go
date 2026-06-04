package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// DefaultLogMaxAgeDays is the default number of days to retain log files.
	DefaultLogMaxAgeDays = 7
	// DefaultLogMaxTotalSize is the default max total size of all log files in bytes (50MB).
	DefaultLogMaxTotalSize = 50 * 1024 * 1024
)

// RotatingWriter implements io.Writer, writing to date-stamped log files.
// It automatically rotates to a new file when the date changes.
type RotatingWriter struct {
	dir     string
	mu      sync.Mutex
	file    *os.File
	curDate string
}

// NewRotatingWriter creates a new rotating writer that writes log files
// to the specified directory. Files are named "cs2ht-YYYY-MM-DD.log".
func NewRotatingWriter(dir string) (*RotatingWriter, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("解析日志目录路径失败: %w", err)
	}
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	w := &RotatingWriter{dir: absDir}
	if err := w.rotate(time.Now()); err != nil {
		return nil, err
	}
	return w, nil
}

// Write writes p to the current log file, rotating if the date has changed.
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	today := now.Format("2006-01-02")
	if today != w.curDate {
		if err := w.rotate(now); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	if err != nil {
		return n, fmt.Errorf("写入日志文件失败: %w", err)
	}
	return n, nil
}

// Close closes the current log file.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
	}
	return nil
}

// LogDir returns the directory path used by this writer.
func (w *RotatingWriter) LogDir() string {
	return w.dir
}

func (w *RotatingWriter) rotate(now time.Time) error {
	if w.file != nil {
		_ = w.file.Close()
	}
	date := now.Format("2006-01-02")
	filename := fmt.Sprintf("cs2ht-%s.log", date)
	path := filepath.Join(w.dir, filename)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败 %s: %w", path, err)
	}
	w.file = f
	w.curDate = date
	return nil
}
