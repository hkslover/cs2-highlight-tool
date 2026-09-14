package app

import (
	"testing"
)

func TestComposeProgressTrackerIsMonotonicAcrossEncoderRetries(t *testing.T) {
	tracker := newComposeProgressTracker(nil, 1)
	percentages := make([]float64, 0, 4)
	tracker.emitHook = func(payload composeProgressPayload) {
		percentages = append(percentages, payload.Percent)
	}

	tracker.stageStart("合成输出")
	tracker.stageProgress(0.8)
	// A fallback encoder starts reporting from its own zero. The public
	// compose_progress stream must not move backwards to that local progress.
	tracker.stageProgress(0.1)
	tracker.stageProgress(1)

	if len(percentages) != 4 {
		t.Fatalf("progress events=%v, want stage start plus three updates", percentages)
	}
	for i := 1; i < len(percentages); i++ {
		if percentages[i] < percentages[i-1] {
			t.Fatalf("progress regressed at %d: %v", i, percentages)
		}
	}
	if percentages[1] != 80 || percentages[2] != 80 || percentages[3] != 100 {
		t.Fatalf("progress=%v, want 0,80,80,100", percentages)
	}
}
