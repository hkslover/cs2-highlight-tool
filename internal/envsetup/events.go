package envsetup

import "github.com/wailsapp/wails/v2/pkg/runtime"

func (s *Service) emitState() {
	if s == nil || s.isStopped() || s.ctx == nil {
		return
	}
	runtime.EventsEmit(s.ctx, "startup_state_changed", s.GetStartupState())
}

func (s *Service) emitProgress(componentID string, active bool, percent float64, indeterminate bool) {
	if s == nil || s.isStopped() || s.ctx == nil {
		return
	}
	runtime.EventsEmit(s.ctx, "download_progress", ProgressMessage{
		ComponentID:   componentID,
		Active:        active,
		Percent:       percent,
		Indeterminate: indeterminate,
	})
}
