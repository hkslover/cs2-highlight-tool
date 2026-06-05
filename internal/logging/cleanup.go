package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CleanOldLogs scans the log directory and removes old log files.
// Step 1: Remove files older than maxAgeDays.
// Step 2: If total size still exceeds maxTotalSize, remove oldest first.
// Returns the number of files removed, total bytes freed, and any error.
func CleanOldLogs(dir string, maxAgeDays int, maxTotalSize int64) (removed int, removedSize int64, err error) {
	if maxAgeDays <= 0 {
		maxAgeDays = DefaultLogMaxAgeDays
	}
	if maxTotalSize <= 0 {
		maxTotalSize = DefaultLogMaxTotalSize
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("读取日志目录失败: %w", err)
	}

	type logFile struct {
		name    string
		path    string
		size    int64
		modTime time.Time
	}

	var files []logFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "cs2ht-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, logFile{
			name:    name,
			path:    filepath.Join(dir, name),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}

	if len(files) == 0 {
		return 0, 0, nil
	}

	// Sort by name (older dates first — date is in the filename)
	sort.Slice(files, func(i, j int) bool {
		return files[i].name < files[j].name
	})

	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)

	// Step 1: Remove files older than maxAgeDays (based on filename date prefix)
	for _, f := range files {
		// Try to parse date from filename "cs2ht-YYYY-MM-DD.log"
		datePart := strings.TrimSuffix(strings.TrimPrefix(f.name, "cs2ht-"), ".log")
		fileDate, parseErr := time.Parse("2006-01-02", datePart)
		if parseErr != nil {
			// If we can't parse the date, use modTime
			fileDate = f.modTime
		}
		if fileDate.Before(cutoff) {
			if rmErr := os.Remove(f.path); rmErr == nil {
				removed++
				removedSize += f.size
			}
		}
	}

	// Step 2: If total size still exceeds maxTotalSize, remove oldest first
	var remaining []logFile
	var totalSize int64
	entries2, _ := os.ReadDir(dir)
	for _, entry := range entries2 {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "cs2ht-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		remaining = append(remaining, logFile{
			name: name,
			path: filepath.Join(dir, name),
			size: info.Size(),
		})
		totalSize += info.Size()
	}

	// Sort by name ascending (oldest first)
	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].name < remaining[j].name
	})

	// Delete oldest files until under limit (keep at least 1 file)
	for totalSize > maxTotalSize && len(remaining) > 1 {
		oldest := remaining[0]
		if rmErr := os.Remove(oldest.path); rmErr == nil {
			removed++
			removedSize += oldest.size
			totalSize -= oldest.size
		}
		remaining = remaining[1:]
	}

	return removed, removedSize, nil
}
