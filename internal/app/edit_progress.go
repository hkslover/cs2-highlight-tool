package app

import (
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type composeProgressPayload struct {
	Active      bool    `json:"active"`
	Percent     float64 `json:"percent"`
	CurrentStep string  `json:"current_step,omitempty"`
	ElapsedMS   int64   `json:"elapsed_ms,omitempty"`
	Error       string  `json:"error,omitempty"`
}

type composeProgressTracker struct {
	mu sync.Mutex

	app         *App
	startedAt   time.Time
	totalStages int
	finished    int
	// maxPercent spans encoder retries: each FFmpeg attempt reports progress
	// from its own zero, while the public compose stream must remain monotonic.
	maxPercent   float64
	currentStage string
	emitHook     func(composeProgressPayload)
}

func newComposeProgressTracker(app *App, totalStages int) *composeProgressTracker {
	if totalStages <= 0 {
		totalStages = 1
	}
	return &composeProgressTracker{
		app:         app,
		startedAt:   time.Now(),
		totalStages: totalStages,
	}
}

func (t *composeProgressTracker) stageStart(label string) {
	t.mu.Lock()
	t.currentStage = strings.TrimSpace(label)
	percent := t.currentPercentLocked(0)
	payload := t.buildPayloadLocked(true, percent, t.currentStage, "")
	t.mu.Unlock()
	t.emit(payload)
}

func (t *composeProgressTracker) stageProgress(ratio float64) {
	t.mu.Lock()
	percent := t.currentPercentLocked(ratio)
	payload := t.buildPayloadLocked(true, percent, t.currentStage, "")
	t.mu.Unlock()
	t.emit(payload)
}

func (t *composeProgressTracker) stageDone() {
	t.mu.Lock()
	if t.finished < t.totalStages {
		t.finished++
	}
	percent := t.currentPercentLocked(0)
	payload := t.buildPayloadLocked(true, percent, t.currentStage, "")
	t.mu.Unlock()
	t.emit(payload)
}

func (t *composeProgressTracker) fail(err error) {
	t.mu.Lock()
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	percent := t.currentPercentLocked(0)
	payload := t.buildPayloadLocked(false, percent, t.currentStage, errText)
	t.mu.Unlock()
	t.emit(payload)
}

func (t *composeProgressTracker) complete() {
	t.mu.Lock()
	t.finished = t.totalStages
	payload := t.buildPayloadLocked(false, 100, t.currentStage, "")
	t.mu.Unlock()
	t.emit(payload)
}

func (t *composeProgressTracker) buildPayloadLocked(active bool, percent float64, step string, errText string) composeProgressPayload {
	percent = clampProgressPercent(percent)
	if percent < t.maxPercent {
		percent = t.maxPercent
	} else {
		t.maxPercent = percent
	}
	return composeProgressPayload{
		Active:      active,
		Percent:     percent,
		CurrentStep: strings.TrimSpace(step),
		ElapsedMS:   time.Since(t.startedAt).Milliseconds(),
		Error:       strings.TrimSpace(errText),
	}
}

func (t *composeProgressTracker) currentPercentLocked(stageRatio float64) float64 {
	if t.totalStages <= 0 {
		return 0
	}
	if stageRatio < 0 {
		stageRatio = 0
	}
	if stageRatio > 1 {
		stageRatio = 1
	}
	base := float64(t.finished) * 100 / float64(t.totalStages)
	span := 100 / float64(t.totalStages)
	return base + stageRatio*span
}

func (t *composeProgressTracker) emit(payload composeProgressPayload) {
	if t != nil && t.emitHook != nil {
		t.emitHook(payload)
	}
	if t == nil || t.app == nil || t.app.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(t.app.ctx, "compose_progress", payload)
}

func clampProgressPercent(percent float64) float64 {
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}
