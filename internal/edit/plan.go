package edit

import (
	"fmt"
	"math"
	"strings"

	"cs2-highlight-tool-v2/internal/ffmpegprofile"
)

const (
	DefaultTransitionDuration    = 0.3
	MinTransitionDuration        = 0.05
	MaxTransitionDuration        = 5.0
	TrimBoundaryToleranceSeconds = 0.001
)

// NormalizeTransitions accepts both the historical sequential form and the
// explicit after_index form, returning one normalized transition per gap.
func NormalizeTransitions(clipCount int, input []Transition) (map[int]Transition, error) {
	result := make(map[int]Transition)
	if clipCount <= 1 {
		if len(input) > 0 {
			return nil, fmt.Errorf("transitions require at least 2 clips")
		}
		return result, nil
	}
	if len(input) == 0 {
		return result, nil
	}

	hasNonZeroAfter := false
	for _, transition := range input {
		if transition.AfterIndex > 0 {
			hasNonZeroAfter = true
			break
		}
	}
	legacySequential := len(input) == clipCount-1 && !hasNonZeroAfter

	if legacySequential {
		for i, transition := range input {
			normalized, err := normalizeTransition(transition)
			if err != nil {
				return nil, fmt.Errorf("transition %d invalid: %w", i+1, err)
			}
			normalized.AfterIndex = i
			result[i] = normalized
		}
		return result, nil
	}

	for i, transition := range input {
		normalized, err := normalizeTransition(transition)
		if err != nil {
			return nil, fmt.Errorf("transition %d invalid: %w", i+1, err)
		}
		if normalized.AfterIndex < 0 || normalized.AfterIndex >= clipCount-1 {
			return nil, fmt.Errorf("transition %d after_index out of range: %d", i+1, normalized.AfterIndex)
		}
		if _, exists := result[normalized.AfterIndex]; exists {
			return nil, fmt.Errorf("duplicate transition for gap index %d", normalized.AfterIndex)
		}
		result[normalized.AfterIndex] = normalized
	}

	return result, nil
}

func normalizeTransition(input Transition) (Transition, error) {
	transition := input
	transition.Type = strings.ToLower(strings.TrimSpace(transition.Type))
	if transition.Type == "" {
		transition.Type = "fade"
	}
	if transition.Type != "fade" {
		return Transition{}, fmt.Errorf("unsupported transition type: %s", transition.Type)
	}

	d := transition.Duration
	if math.IsNaN(d) || math.IsInf(d, 0) {
		return Transition{}, fmt.Errorf("transition duration must be finite")
	}
	if d <= 0 {
		d = DefaultTransitionDuration
	}
	if d < MinTransitionDuration || d > MaxTransitionDuration {
		return Transition{}, fmt.Errorf("transition duration out of range: %.3f", d)
	}
	transition.Duration = math.Round(d*1000) / 1000
	return transition, nil
}

// ResolveTrimRange validates and clamps the optional trim range against the
// authoritative media duration. The one-millisecond tolerance covers the
// existing format-duration rounding boundary.
func ResolveTrimRange(clip Clip, duration float64, index int) (float64, float64, bool, error) {
	if clip.StartSeconds == nil && clip.EndSeconds == nil {
		return 0, duration, false, nil
	}
	start := 0.0
	if clip.StartSeconds != nil {
		start = *clip.StartSeconds
	}
	end := duration
	if clip.EndSeconds != nil {
		end = *clip.EndSeconds
	}
	if math.IsNaN(start) || math.IsInf(start, 0) || math.IsNaN(end) || math.IsInf(end, 0) {
		return 0, 0, false, fmt.Errorf("clip %d trim range must be finite", index+1)
	}
	if start < 0 || end < 0 {
		return 0, 0, false, fmt.Errorf("clip %d trim range is invalid: %.6f..%.6f (duration %.6f)", index+1, start, end, duration)
	}
	if start > duration {
		if start-duration > TrimBoundaryToleranceSeconds {
			return 0, 0, false, fmt.Errorf("clip %d trim range is invalid: %.6f..%.6f (duration %.6f)", index+1, start, end, duration)
		}
		start = duration
	}
	if end > duration {
		if end-duration > TrimBoundaryToleranceSeconds {
			return 0, 0, false, fmt.Errorf("clip %d trim range is invalid: %.6f..%.6f (duration %.6f)", index+1, start, end, duration)
		}
		end = duration
	}
	if start >= duration || end <= start {
		return 0, 0, false, fmt.Errorf("clip %d trim range is invalid: %.6f..%.6f (duration %.6f)", index+1, start, end, duration)
	}
	return start, end, true, nil
}

// ResolveClip maps a request and optional probe fact into a normalized clip.
// No filesystem or process access happens here.
func ResolveClip(input Clip, duration float64, info *ProbedVideoInfo, index int, authoritative bool) (ResolvedClip, error) {
	if info != nil {
		duration = info.Duration
	}
	if duration <= 0 {
		return ResolvedClip{}, fmt.Errorf("clip %d duration must be > 0", index+1)
	}
	if math.IsNaN(duration) || math.IsInf(duration, 0) {
		return ResolvedClip{}, fmt.Errorf("clip %d duration must be finite", index+1)
	}
	trimStart, trimEnd, hasTrim, err := ResolveTrimRange(input, duration, index)
	if err != nil {
		return ResolvedClip{}, err
	}
	resolvedDuration := duration
	if !authoritative {
		resolvedDuration = math.Round(duration*1000) / 1000
	}
	resolved := ResolvedClip{
		VideoPath: strings.TrimSpace(input.VideoPath),
		Duration:  resolvedDuration,
		TrimStart: trimStart,
		TrimEnd:   trimEnd,
		HasTrim:   hasTrim,
	}
	if info != nil {
		resolved.Width = info.Width
		resolved.Height = info.Height
		resolved.SampleAspectRatio = info.SampleAspectRatio
		resolved.DisplayAspectRatio = info.DisplayAspectRatio
		resolved.HasAudio = info.HasAudio
		resolved.AudioKnown = info.AudioKnown
	}
	return resolved, nil
}

func HasTrim(clips []ResolvedClip) bool {
	for _, clip := range clips {
		if clip.HasTrim {
			return true
		}
	}
	return false
}

func AlignToFrameGrid(seconds float64, fps int) float64 {
	if fps <= 0 {
		return seconds
	}
	return math.Round(seconds*float64(fps)) / float64(fps)
}

func StageCount(clipCount int, withTransitions bool) int {
	if clipCount <= 0 || !withTransitions {
		return 1
	}
	return 1
}

func TotalClipDuration(clips []ResolvedClip) float64 {
	total := 0.0
	for _, clip := range clips {
		total += clip.Duration
	}
	return total
}

// NormalizeEncodeSettings validates the application-provided encoding choice
// without loading configuration or consulting process/global state.
func NormalizeEncodeSettings(fps int, quality, videoPreset string, detectedEncoders []string, limits EncodeLimits) EncodeSettings {
	nextFPS := fps
	if nextFPS <= 0 {
		nextFPS = limits.DefaultFPS
	}
	if limits.MinFPS > 0 && nextFPS < limits.MinFPS {
		nextFPS = limits.MinFPS
	}
	if limits.MaxFPS > 0 && nextFPS > limits.MaxFPS {
		nextFPS = limits.MaxFPS
	}
	return EncodeSettings{
		FPS:         nextFPS,
		Quality:     ffmpegprofile.NormalizeEditQuality(quality),
		VideoPreset: ffmpegprofile.NormalizeUserPreset(videoPreset),
		Caps:        ffmpegprofile.CapabilitiesFromEncoders(detectedEncoders),
	}
}

func BuildRetryProfiles(encode EncodeSettings) []ffmpegprofile.Profile {
	return ffmpegprofile.BuildRetryChain(encode.VideoPreset, encode.Caps)
}
