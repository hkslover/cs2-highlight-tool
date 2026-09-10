package envsetup

import (
	"strings"

	"cs2-highlight-tool-v2/internal/logging"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const startupLogBufferLimit = 12000

type logFields = logging.Fields

type stepToken = logging.StepToken

func (s *Service) emitLog(level, message string) {
	s.emitLogWithFields(level, message, logFields{})
}

func (s *Service) emitLogWithFields(level, message string, fields logFields) {
	if s.logger == nil {
		return
	}
	switch normalizeLogLevel(level) {
	case logging.LevelWarn:
		s.logger.Warn(message, fields)
	case logging.LevelError:
		s.logger.Error(message, fields)
	default:
		s.logger.Info(message, fields)
	}
}

func normalizeLogLevel(level string) logging.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "warn", "warning":
		return logging.LevelWarn
	case "err", "error":
		return logging.LevelError
	default:
		return logging.LevelInfo
	}
}

func (s *Service) logStepStart(component, stage, action, source string, attempt int, meta map[string]string) stepToken {
	if s.logger == nil {
		return stepToken{}
	}
	return s.logger.StepStart(component, stage, action, source, attempt, meta)
}

func (s *Service) logStepDone(component, stage, action, source string, attempt int, started stepToken, meta map[string]string) {
	if s.logger == nil {
		return
	}
	if strings.TrimSpace(started.Component) == "" {
		started.Component = component
	}
	if strings.TrimSpace(started.Stage) == "" {
		started.Stage = stage
	}
	if strings.TrimSpace(started.Action) == "" {
		started.Action = action
	}
	if strings.TrimSpace(started.Source) == "" {
		started.Source = source
	}
	if started.Attempt == 0 {
		started.Attempt = attempt
	}
	s.logger.StepDone(started, meta)
}

func (s *Service) logStepFail(component, stage, action, source string, attempt int, started stepToken, err error, meta map[string]string) {
	if s.logger == nil {
		return
	}
	if strings.TrimSpace(started.Component) == "" {
		started.Component = component
	}
	if strings.TrimSpace(started.Stage) == "" {
		started.Stage = stage
	}
	if strings.TrimSpace(started.Action) == "" {
		started.Action = action
	}
	if strings.TrimSpace(started.Source) == "" {
		started.Source = source
	}
	if started.Attempt == 0 {
		started.Attempt = attempt
	}
	s.logger.StepFail(started, err, meta)
}

func (s *Service) appendLogEntry(entry logging.Entry) {
	if s == nil || s.isStopped() {
		return
	}
	logEntry := LogMessage{
		Level:     strings.TrimSpace(entry.Level),
		Message:   strings.TrimSpace(entry.Message),
		Time:      strings.TrimSpace(entry.Time),
		Component: strings.TrimSpace(entry.Component),
		Stage:     strings.TrimSpace(entry.Stage),
		Action:    strings.TrimSpace(entry.Action),
		Source:    strings.ToLower(strings.TrimSpace(entry.Source)),
		Attempt:   entry.Attempt,
		ElapsedMS: entry.ElapsedMS,
		Error:     strings.TrimSpace(entry.Error),
	}
	if len(entry.Meta) > 0 {
		logEntry.Meta = copyMeta(entry.Meta)
	}

	s.mu.Lock()
	s.logs = append(s.logs, logEntry)
	if len(s.logs) > startupLogBufferLimit {
		s.logs = append([]LogMessage(nil), s.logs[len(s.logs)-startupLogBufferLimit:]...)
	}
	s.mu.Unlock()

	if s.ctx == nil {
		return
	}
	runtime.EventsEmit(s.ctx, "log", logEntry)
}

func copyMeta(meta map[string]string) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]string, len(meta))
	for key, value := range meta {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
