package plugingen

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/ffmpegprofile"
)

// BuildProduceHistoryKey constructs a stable, order-independent key for a produce take plan.
func BuildProduceHistoryKey(demoPath string, view string, specMode int, killIDs []string) string {
	return BuildProduceHistoryKeyWithSourceID(demoPath, view, specMode, killIDs, "")
}

func BuildProduceHistoryKeyWithSourceID(demoPath string, view string, specMode int, killIDs []string, sourceID string) string {
	normalized := make([]string, 0, len(killIDs))
	for _, killID := range killIDs {
		id := strings.TrimSpace(killID)
		if id != "" {
			normalized = append(normalized, id)
		}
	}
	sort.Strings(normalized)
	key := strings.TrimSpace(demoPath) + "#" +
		strings.ToLower(strings.TrimSpace(view)) + "#" +
		fmt.Sprintf("%d", specMode) + "#" +
		strings.Join(normalized, "|")
	source := strings.ToLower(strings.TrimSpace(sourceID))
	if source != "" {
		key += "#" + source
	}
	return key
}

// SanitizeDemoSubDirName converts a demo file path into a safe subdirectory name.
func SanitizeDemoSubDirName(demoPath string) string {
	base := strings.TrimSpace(filepath.Base(strings.TrimSpace(demoPath)))
	if base == "" || base == "." {
		base = "demo"
	}
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	if base == "" {
		base = "demo"
	}
	var b strings.Builder
	b.Grow(len(base))
	lastUnderscore := false
	for _, r := range base {
		allowed := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-'
		if allowed {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	name := strings.Trim(b.String(), "._-")
	if name == "" {
		return "demo"
	}
	return name
}

// BuildBatchRecordSubDirs derives a unique subdirectory name for each demo path in a batch.
// Duplicate demo names receive an incremental suffix (_2, _3, …).
func BuildBatchRecordSubDirs(demoPaths []string) []string {
	result := make([]string, len(demoPaths))
	seen := make(map[string]int, len(demoPaths))
	for i, demoPath := range demoPaths {
		base := SanitizeDemoSubDirName(demoPath)
		key := strings.ToLower(base)
		seen[key]++
		if seen[key] > 1 {
			base = fmt.Sprintf("%s_%d", base, seen[key])
		}
		result[i] = base
	}
	return result
}

// ResolvePluginVideoPreset resolves the effective video preset to use.
// If userPreset is "auto", it derives the best preset from cfg's detected capabilities.
func ResolvePluginVideoPreset(userPreset string, cfg *config.Config) string {
	var detectedPreset string
	var detectedEncoders []string
	if cfg != nil {
		detectedPreset = cfg.FFmpegDetectedPreset
		detectedEncoders = cfg.FFmpegDetectedEncoders
	}
	resolved := ffmpegprofile.ResolveVideoPreset(userPreset, detectedPreset, detectedEncoders)
	if strings.TrimSpace(resolved.EffectivePreset) == "" {
		return ffmpegprofile.UserPresetC1
	}
	return resolved.EffectivePreset
}
