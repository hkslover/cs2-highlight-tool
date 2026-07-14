package envsetup

import (
	"cs2-highlight-tool-v2/internal/logging"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (s *Service) ExportStartupLogs() (string, error) {
	s.emitLogWithFields("info", "用户触发导出启动日志", logFields{
		Component: "startup",
		Action:    "export_logs",
	})
	state := s.GetStartupState()
	logs := s.logsSnapshot()

	now := time.Now()
	filename := fmt.Sprintf("startup-log-%s.txt", now.Format("20060102-150405"))
	defaultDir := filepath.Join(s.dataDir, "logs")
	if err := os.MkdirAll(defaultDir, 0755); err != nil {
		return "", fmt.Errorf("创建日志目录失败: %w", err)
	}

	targetPath := ""
	if s.ctx != nil {
		selected, err := runtime.SaveFileDialog(s.ctx, runtime.SaveDialogOptions{
			Title:           "导出环境准备日志",
			DefaultFilename: filename,
			Filters: []runtime.FileFilter{
				{DisplayName: "Text File (*.txt)", Pattern: "*.txt"},
			},
		})
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(selected) == "" {
			s.emitLogWithFields("warning", "导出启动日志已取消", logFields{
				Component: "startup",
				Action:    "export_logs",
			})
			return "", nil
		}
		targetPath = selected
	} else {
		targetPath = filepath.Join(defaultDir, filename)
	}

	content := buildStartupLogReport(now, state, logs)
	if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入日志文件失败: %w", err)
	}
	s.emitLogWithFields("info", fmt.Sprintf("环境准备日志已导出: %s", targetPath), logFields{
		Component: "startup",
		Action:    "export_logs",
		Meta: map[string]string{
			"target_path": targetPath,
		},
	})
	return targetPath, nil
}

func buildStartupLogReport(exportTime time.Time, state StartupState, logs []LogMessage) string {
	state = sanitizeStartupStateForExport(state)
	logs = sanitizeLogsForExport(logs)
	logs = sortLogsByTime(logs)

	var builder strings.Builder
	writeLine := func(format string, args ...any) {
		builder.WriteString(fmt.Sprintf(format, args...))
		builder.WriteString("\n")
	}

	writeLine("CS2 Highlight Tool - Startup Log Export")
	writeLine("Exported At: %s", exportTime.Format(time.RFC3339))
	writeLine("")
	writeLine("[Startup State]")
	writeLine("Mode: %s", state.Mode)
	writeLine("Phase: %s", state.Phase)
	writeLine("Running: %v", state.Running)
	writeLine("CanEnterMain: %v", state.CanEnterMain)
	writeLine("FatalError: %s", state.FatalError)
	writeLine("EntryNotice: %s", state.EntryNotice)
	writeLine("")
	writeLine("[Source Step]")
	writeLine("Status: %s", state.SourceStep.Status)
	writeLine("Source: %s", state.SourceStep.Source)
	writeLine("CountryCode: %s", state.SourceStep.CountryCode)
	writeLine("Message: %s", state.SourceStep.Message)
	writeLine("Error: %s", state.SourceStep.Error)
	writeLine("")
	writeLine("[Self Update]")
	writeLine("Status: %s", state.SelfUpdate.Status)
	writeLine("Available: %v", state.SelfUpdate.Available)
	writeLine("Current: %s", state.SelfUpdate.Current)
	writeLine("Latest: %s", state.SelfUpdate.Latest)
	writeLine("URL: %s", state.SelfUpdate.URL)
	writeLine("Error: %s", state.SelfUpdate.Error)
	writeLine("")
	writeLine("[Components]")
	for _, step := range state.Steps {
		writeLine("- %s (%s)", step.Name, step.ID)
		writeLine("  Status: %s", step.Status)
		writeLine("  LocalVersion: %s", step.LocalVersion)
		writeLine("  RemoteVersion: %s", step.RemoteVersion)
		writeLine("  Path: %s", step.Path)
		writeLine("  ManualURL: %s", step.ManualURL)
		writeLine("  Error: %s", step.Error)
	}

	writeLine("")
	writeLine("[Timeline]")
	if len(logs) == 0 {
		writeLine("(no logs)")
	} else {
		for i, entry := range logs {
			writeLine("%04d %s [%s] component=%s stage=%s action=%s source=%s attempt=%d elapsed=%dms message=%s",
				i+1,
				entry.Time,
				strings.ToUpper(entry.Level),
				emptyFallback(entry.Component, "-"),
				emptyFallback(entry.Stage, "-"),
				emptyFallback(entry.Action, "-"),
				emptyFallback(entry.Source, "-"),
				entry.Attempt,
				entry.ElapsedMS,
				emptyFallback(entry.Message, "-"),
			)
			if strings.TrimSpace(entry.Error) != "" {
				writeLine("  error: %s", entry.Error)
			}
			if len(entry.Meta) > 0 {
				writeLine("  meta: %s", formatMeta(entry.Meta))
			}
		}
	}

	writeLine("")
	writeLine("[Failure Summary]")
	failures := collectFailureEntries(logs)
	if len(failures) == 0 {
		writeLine("(no failures)")
	} else {
		for i, failure := range failures {
			writeLine("%d) %s component=%s stage=%s action=%s source=%s attempt=%d error=%s",
				i+1,
				failure.Time,
				emptyFallback(failure.Component, "-"),
				emptyFallback(failure.Stage, "-"),
				emptyFallback(failure.Action, "-"),
				emptyFallback(failure.Source, "-"),
				failure.Attempt,
				emptyFallback(failure.Error, "-"),
			)
		}
	}

	writeLine("")
	writeLine("[Performance Summary]")
	perf := summarizePerformance(logs)
	if len(perf) == 0 {
		writeLine("(no timing data)")
	} else {
		components := make([]string, 0, len(perf))
		for component := range perf {
			components = append(components, component)
		}
		sort.Strings(components)
		for _, component := range components {
			stat := perf[component]
			writeLine("- %s: total=%dms count=%d longest=%s (%dms)",
				component,
				stat.TotalMS,
				stat.Count,
				emptyFallback(stat.LongestStage, "-"),
				stat.LongestMS,
			)
		}
	}

	return builder.String()
}

type failureEntry struct {
	Time      string
	Component string
	Stage     string
	Action    string
	Source    string
	Attempt   int
	Error     string
}

type performanceStat struct {
	TotalMS      int64
	Count        int
	LongestMS    int64
	LongestStage string
}

func sanitizeStartupStateForExport(state StartupState) StartupState {
	state.FatalError = sanitizeErrorForExport(state.FatalError)
	state.EntryNotice = sanitizeErrorForExport(state.EntryNotice)
	state.SourceStep.Message = sanitizeTextForExport(state.SourceStep.Message)
	state.SourceStep.Error = sanitizeErrorForExport(state.SourceStep.Error)
	state.SelfUpdate.URL = sanitizeURLForExport(state.SelfUpdate.URL)
	state.SelfUpdate.Error = sanitizeErrorForExport(state.SelfUpdate.Error)
	for i := range state.Steps {
		state.Steps[i].Path = sanitizePathForExport(state.Steps[i].Path)
		state.Steps[i].ManualURL = sanitizeURLForExport(state.Steps[i].ManualURL)
		state.Steps[i].Error = sanitizeErrorForExport(state.Steps[i].Error)
	}
	for i := range state.Ads {
		state.Ads[i].ClickURL = sanitizeURLForExport(state.Ads[i].ClickURL)
		state.Ads[i].Sponsor = sanitizeTextForExport(state.Ads[i].Sponsor)
		state.Ads[i].Title = sanitizeTextForExport(state.Ads[i].Title)
		state.Ads[i].RichHTML = sanitizeTextForExport(state.Ads[i].RichHTML)
		state.Ads[i].ImageURL = sanitizeURLForExport(state.Ads[i].ImageURL)
		state.Ads[i].ImageAlt = sanitizeTextForExport(state.Ads[i].ImageAlt)
	}
	return state
}

func sanitizeLogsForExport(logs []LogMessage) []LogMessage {
	if len(logs) == 0 {
		return nil
	}
	out := make([]LogMessage, 0, len(logs))
	for _, entry := range logs {
		next := entry
		next.Message = sanitizeTextForExport(next.Message)
		next.Error = sanitizeErrorForExport(next.Error)
		next.Source = strings.ToLower(strings.TrimSpace(next.Source))
		if len(next.Meta) > 0 {
			meta := make(map[string]string, len(next.Meta))
			for key, value := range next.Meta {
				meta[key] = sanitizeMetaValueForExport(key, value)
			}
			next.Meta = meta
		}
		out = append(out, next)
	}
	return out
}

func sanitizeMetaValueForExport(key, value string) string {
	return logging.SanitizeExportMetaValue(key, value)
}

func sortLogsByTime(logs []LogMessage) []LogMessage {
	if len(logs) <= 1 {
		return logs
	}
	ordered := append([]LogMessage(nil), logs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		ti, oi := parseLogTime(ordered[i].Time)
		tj, oj := parseLogTime(ordered[j].Time)
		if oi && oj {
			return ti.Before(tj)
		}
		if oi != oj {
			return oi
		}
		return false
	})
	return ordered
}

func parseLogTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed, true
	}
	parsed, err = time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed, true
	}
	return time.Time{}, false
}

func collectFailureEntries(logs []LogMessage) []failureEntry {
	failures := make([]failureEntry, 0, len(logs))
	for _, entry := range logs {
		errText := strings.TrimSpace(entry.Error)
		if errText == "" && strings.EqualFold(entry.Level, "error") {
			errText = strings.TrimSpace(entry.Message)
		}
		if errText == "" {
			continue
		}
		failures = append(failures, failureEntry{
			Time:      entry.Time,
			Component: entry.Component,
			Stage:     entry.Stage,
			Action:    entry.Action,
			Source:    entry.Source,
			Attempt:   entry.Attempt,
			Error:     errText,
		})
	}
	return failures
}

func summarizePerformance(logs []LogMessage) map[string]performanceStat {
	stats := make(map[string]performanceStat)
	for _, entry := range logs {
		if entry.ElapsedMS <= 0 {
			continue
		}
		component := strings.TrimSpace(entry.Component)
		if component == "" {
			component = "startup"
		}
		stat := stats[component]
		stat.Count++
		stat.TotalMS += entry.ElapsedMS
		if entry.ElapsedMS > stat.LongestMS {
			stat.LongestMS = entry.ElapsedMS
			stat.LongestStage = stageLabel(entry.Stage, entry.Action)
		}
		stats[component] = stat
	}
	return stats
}

func stageLabel(stage, action string) string {
	stage = strings.TrimSpace(stage)
	action = strings.TrimSpace(action)
	if stage == "" && action == "" {
		return ""
	}
	if stage == "" {
		return action
	}
	if action == "" {
		return stage
	}
	return stage + "/" + action
}

func formatMeta(meta map[string]string) string {
	if len(meta) == 0 {
		return ""
	}
	keys := make([]string, 0, len(meta))
	for key := range meta {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, meta[key]))
	}
	return strings.Join(parts, ", ")
}

func emptyFallback(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func sanitizeTextForExport(value string) string {
	return logging.SanitizeExportText(value)
}

func sanitizeErrorForExport(value string) string {
	return sanitizeTextForExport(value)
}

func sanitizePathForExport(value string) string {
	return logging.SanitizeExportPath(value)
}

func sanitizeURLForExport(rawURL string) string {
	return logging.SanitizeExportURL(rawURL)
}
