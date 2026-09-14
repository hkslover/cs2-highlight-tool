package edit

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// BuildTransitionFilterGraph creates the existing hard-cut/fade graph from
// normalized media facts. It does not probe files or change transition
// behavior; the implementation is deliberately a pure planner.
func BuildTransitionFilterGraph(clips []ResolvedClip, transitionByIndex map[int]Transition, fps int) (TransitionGraphPlan, error) {
	if len(clips) == 0 {
		return TransitionGraphPlan{}, fmt.Errorf("at least 1 clip is required")
	}
	if fps <= 0 {
		return TransitionGraphPlan{}, fmt.Errorf("edit fps must be > 0")
	}
	for gapIndex := range transitionByIndex {
		if gapIndex < 0 || gapIndex >= len(clips)-1 {
			return TransitionGraphPlan{}, fmt.Errorf("transition gap index out of range: %d", gapIndex)
		}
	}

	frameDuration := 1.0 / float64(fps)
	durations := make([]float64, len(clips))
	for i, clip := range clips {
		if clip.Width <= 0 || clip.Height <= 0 {
			return TransitionGraphPlan{}, fmt.Errorf("clip %d has invalid resolution: %dx%d", i, clip.Width, clip.Height)
		}
		start := 0.0
		end := clip.Duration
		if clip.HasTrim {
			start = clip.TrimStart
			end = clip.TrimEnd
			if math.IsNaN(start) || math.IsInf(start, 0) || math.IsNaN(end) || math.IsInf(end, 0) || start < 0 || end <= start || end > clip.Duration {
				return TransitionGraphPlan{}, fmt.Errorf("clip %d trim range is invalid: %.6f..%.6f (duration %.6f)", i, start, end, clip.Duration)
			}
		}
		durations[i] = AlignToFrameGrid(end-start, fps)
		if durations[i] < 2*frameDuration {
			return TransitionGraphPlan{}, fmt.Errorf("clip %d is too short after frame alignment: %.6f seconds", i, durations[i])
		}
	}

	width := clips[0].Width
	height := clips[0].Height
	targetSampleAspectRatio := ClipSampleAspectRatio(clips[0])
	filters := make([]string, 0, len(clips)*2+len(clips)-1)
	for i, duration := range durations {
		start := 0.0
		if clips[i].HasTrim {
			start = clips[i].TrimStart
		}
		videoTrim := fmt.Sprintf("trim=duration=%.6f", duration)
		if clips[i].HasTrim {
			videoTrim = fmt.Sprintf("trim=start=%.6f:duration=%.6f", start, duration)
		}
		filters = append(filters, fmt.Sprintf(
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=%s,fps=%d,settb=AVTB,setpts=PTS-STARTPTS,format=yuv420p,%s,setpts=PTS-STARTPTS[v%d]",
			i,
			width,
			height,
			width,
			height,
			targetSampleAspectRatio,
			fps,
			videoTrim,
			i,
		))
		if !clips[i].AudioKnown || clips[i].HasAudio {
			audioTrim := fmt.Sprintf("atrim=0:%.6f", duration)
			if clips[i].HasTrim {
				audioTrim = fmt.Sprintf("atrim=start=%.6f:duration=%.6f", start, duration)
			}
			filters = append(filters, fmt.Sprintf(
				"[%d:a]aresample=async=1:first_pts=0,aformat=sample_rates=48000:channel_layouts=stereo,apad,%s,asetpts=PTS-STARTPTS[a%d]",
				i,
				audioTrim,
				i,
			))
		} else {
			filters = append(filters, fmt.Sprintf(
				"anullsrc=channel_layout=stereo:sample_rate=48000,atrim=duration=%.6f,asetpts=PTS-STARTPTS[a%d]",
				duration,
				i,
			))
		}
	}

	currentVideo := "[v0]"
	currentAudio := "[a0]"
	currentDuration := durations[0]
	for gapIndex := 0; gapIndex < len(clips)-1; gapIndex++ {
		nextVideo := fmt.Sprintf("[v%d]", gapIndex+1)
		nextAudio := fmt.Sprintf("[a%d]", gapIndex+1)
		lastGap := gapIndex == len(clips)-2
		outputVideo := fmt.Sprintf("[vx%d]", gapIndex)
		outputAudio := fmt.Sprintf("[ax%d]", gapIndex)
		if lastGap {
			outputVideo = "[v]"
			outputAudio = "[a]"
		}

		if transition, ok := transitionByIndex[gapIndex]; ok {
			if strings.ToLower(strings.TrimSpace(transition.Type)) != "fade" {
				return TransitionGraphPlan{}, fmt.Errorf("unsupported transition type at gap %d: %s", gapIndex, transition.Type)
			}
			duration := AlignToFrameGrid(transition.Duration, fps)
			if duration < frameDuration {
				return TransitionGraphPlan{}, fmt.Errorf("transition duration at gap %d is too short after frame alignment: %.6f seconds", gapIndex, duration)
			}
			if duration >= currentDuration || duration >= durations[gapIndex+1] {
				return TransitionGraphPlan{}, fmt.Errorf(
					"transition duration %.6f exceeds clip durations at gap %d (left=%.6f right=%.6f)",
					duration,
					gapIndex,
					currentDuration,
					durations[gapIndex+1],
				)
			}
			offset := currentDuration - duration
			filters = append(filters,
				fmt.Sprintf("%s%sxfade=transition=fade:duration=%.6f:offset=%.6f%s", currentVideo, nextVideo, duration, offset, outputVideo),
				fmt.Sprintf("%s%sacrossfade=d=%.6f:c1=tri:c2=tri%s", currentAudio, nextAudio, duration, outputAudio),
			)
			currentDuration += durations[gapIndex+1] - duration
		} else {
			filters = append(filters, fmt.Sprintf(
				"%s%s%s%sconcat=n=2:v=1:a=1%s%s",
				currentVideo,
				currentAudio,
				nextVideo,
				nextAudio,
				outputVideo,
				outputAudio,
			))
			currentDuration += durations[gapIndex+1]
		}
		currentVideo = outputVideo
		currentAudio = outputAudio
	}
	if len(clips) == 1 {
		filters = append(filters, "[v0]null[v]", "[a0]anull[a]")
	}

	return TransitionGraphPlan{
		Filter:        strings.Join(filters, ";"),
		TotalDuration: currentDuration,
	}, nil
}

// ClipSampleAspectRatio preserves the source sample aspect ratio. If only a
// display ratio is available, it derives the corresponding SAR from the
// probed dimensions; malformed metadata falls back to square pixels.
func ClipSampleAspectRatio(clip ResolvedClip) string {
	if ratio, ok := parseAspectRatio(clip.SampleAspectRatio); ok {
		return ratio.String()
	}

	displayRatio, ok := parseAspectRatio(clip.DisplayAspectRatio)
	if !ok || clip.Width <= 0 || clip.Height <= 0 {
		return "1/1"
	}

	return reduceAspectRatio(
		displayRatio.num*int64(clip.Height),
		displayRatio.den*int64(clip.Width),
	)
}

type aspectRatio struct {
	num int64
	den int64
}

func (ratio aspectRatio) String() string {
	return fmt.Sprintf("%d/%d", ratio.num, ratio.den)
}

func parseAspectRatio(raw string) (aspectRatio, bool) {
	ratio := strings.TrimSpace(raw)
	if ratio == "" || strings.EqualFold(ratio, "N/A") {
		return aspectRatio{}, false
	}

	separator := ":"
	if !strings.Contains(ratio, separator) {
		separator = "/"
	}
	parts := strings.Split(ratio, separator)
	if len(parts) != 2 {
		return aspectRatio{}, false
	}
	numerator, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || numerator <= 0 {
		return aspectRatio{}, false
	}
	denominator, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil || denominator <= 0 {
		return aspectRatio{}, false
	}

	return reduceAspectRatioParts(numerator, denominator), true
}

func reduceAspectRatio(numerator, denominator int64) string {
	if numerator <= 0 || denominator <= 0 {
		return "1/1"
	}
	reduced := reduceAspectRatioParts(numerator, denominator)
	return reduced.String()
}

func reduceAspectRatioParts(numerator, denominator int64) aspectRatio {
	common := aspectRatioGCD(numerator, denominator)
	return aspectRatio{num: numerator / common, den: denominator / common}
}

func aspectRatioGCD(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}
