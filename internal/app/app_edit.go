package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/ffmpegprofile"
)

type EditConcatClip struct {
	VideoPath string  `json:"video_path"`
	Duration  float64 `json:"duration"`
}

type EditConcatTransition struct {
	Type       string  `json:"type"`
	Duration   float64 `json:"duration"`
	AfterIndex int     `json:"after_index,omitempty"`
}

type EditConcatRequest struct {
	Clips       []EditConcatClip       `json:"clips"`
	Transitions []EditConcatTransition `json:"transitions"`
}

type resolvedEditClip struct {
	VideoPath          string
	Duration           float64
	Width              int
	Height             int
	SampleAspectRatio  string
	DisplayAspectRatio string
}

type editEncodeSettings struct {
	FPS         int
	Quality     string
	VideoPreset string
	Caps        ffmpegprofile.Capabilities
}

const (
	defaultEditTransitionDuration = 0.3
	minEditTransitionDuration     = 0.05
	maxEditTransitionDuration     = 5.0
)

// ConcatEditClips merges edit clips. The transition path uses one filter graph
// and one encode; request clip durations are retained for UI compatibility but
// are not trusted for transition timing.
func (a *App) ConcatEditClips(request EditConcatRequest) (string, error) {
	transitionByIndex, err := normalizeEditTransitions(len(request.Clips), request.Transitions)
	if err != nil {
		return "", err
	}

	resolvedClips, err := a.resolveEditClips(request.Clips, len(transitionByIndex) > 0)
	if err != nil {
		return "", err
	}

	outputDir, ffmpegExe, encode := a.resolveEditOutputPaths()
	if ffmpegExe == "" {
		return "", fmt.Errorf("ffmpeg not found")
	}
	if _, err := os.Stat(ffmpegExe); err != nil {
		return "", fmt.Errorf("ffmpeg not found at %s", ffmpegExe)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory failed: %w", err)
	}

	outputPath := filepath.Join(outputDir, fmt.Sprintf("edit_%s.mp4", time.Now().Format("20060102_150405")))
	tracker := newComposeProgressTracker(a, editComposeStageCount(len(resolvedClips), len(transitionByIndex) > 0))

	if len(transitionByIndex) == 0 {
		if _, err := concatSimple(ffmpegExe, resolvedClips, outputPath, encode, tracker); err != nil {
			tracker.fail(err)
			return "", err
		}
	} else {
		if _, err := concatWithTransitions(ffmpegExe, resolvedClips, transitionByIndex, outputPath, encode, tracker); err != nil {
			tracker.fail(err)
			return "", err
		}
	}

	if _, statErr := os.Stat(outputPath); statErr != nil {
		tracker.fail(statErr)
		return "", fmt.Errorf("output video not created: %w", statErr)
	}

	a.addEditedHistoryEntry(outputPath, "edit_timeline")
	tracker.complete()
	return outputPath, nil
}

func (a *App) resolveEditClips(input []EditConcatClip, forceProbe ...bool) ([]resolvedEditClip, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("no clips provided")
	}

	probeForTransitions := len(forceProbe) > 0 && forceProbe[0]
	ffprobeExe := a.resolveFFprobeExe()
	if probeForTransitions {
		if ffprobeExe == "" {
			return nil, fmt.Errorf("ffprobe not found")
		}
		if _, err := os.Stat(ffprobeExe); err != nil {
			return nil, fmt.Errorf("ffprobe not found at %s", ffprobeExe)
		}
	}

	resolved := make([]resolvedEditClip, 0, len(input))
	for i, clip := range input {
		p := strings.TrimSpace(clip.VideoPath)
		if p == "" {
			return nil, fmt.Errorf("clip %d video path is empty", i+1)
		}
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("clip %d video file not found: %s", i+1, p)
		}

		duration := clip.Duration
		width := 0
		height := 0
		sampleAspectRatio := ""
		displayAspectRatio := ""
		if probeForTransitions {
			info, err := probeVideoStreamInfo(ffprobeExe, p)
			if err != nil {
				return nil, fmt.Errorf("clip %d probe video stream failed: %w", i+1, err)
			}
			duration = info.Duration
			width = info.Width
			height = info.Height
			sampleAspectRatio = info.SampleAspectRatio
			displayAspectRatio = info.DisplayAspectRatio
		} else if duration <= 0 {
			if ffprobeExe == "" {
				return nil, fmt.Errorf("clip %d duration is invalid and ffprobe not found", i+1)
			}
			if _, err := os.Stat(ffprobeExe); err != nil {
				return nil, fmt.Errorf("ffprobe not found at %s", ffprobeExe)
			}
			probed, err := probeDurationByFFprobe(ffprobeExe, p)
			if err != nil {
				return nil, fmt.Errorf("clip %d probe duration failed: %w", i+1, err)
			}
			duration = probed
		}
		if duration <= 0 {
			return nil, fmt.Errorf("clip %d duration must be > 0", i+1)
		}

		resolvedDuration := duration
		if !probeForTransitions {
			resolvedDuration = math.Round(duration*1000) / 1000
		}
		resolved = append(resolved, resolvedEditClip{
			VideoPath:          p,
			Duration:           resolvedDuration,
			Width:              width,
			Height:             height,
			SampleAspectRatio:  sampleAspectRatio,
			DisplayAspectRatio: displayAspectRatio,
		})
	}
	return resolved, nil
}

func normalizeEditTransitions(clipCount int, input []EditConcatTransition) (map[int]EditConcatTransition, error) {
	result := make(map[int]EditConcatTransition)
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

func normalizeTransition(input EditConcatTransition) (EditConcatTransition, error) {
	transition := input
	transition.Type = strings.ToLower(strings.TrimSpace(transition.Type))
	if transition.Type == "" {
		transition.Type = "fade"
	}
	if transition.Type != "fade" {
		return EditConcatTransition{}, fmt.Errorf("unsupported transition type: %s", transition.Type)
	}

	d := transition.Duration
	if d <= 0 {
		d = defaultEditTransitionDuration
	}
	if d < minEditTransitionDuration || d > maxEditTransitionDuration {
		return EditConcatTransition{}, fmt.Errorf("transition duration out of range: %.3f", d)
	}
	transition.Duration = math.Round(d*1000) / 1000
	return transition, nil
}

func concatSimple(
	ffmpegExe string,
	clips []resolvedEditClip,
	outputPath string,
	encode editEncodeSettings,
	tracker *composeProgressTracker,
) ([]byte, error) {
	listPath := outputPath + ".concat.txt"
	defer os.Remove(listPath)

	var lines []string
	for _, clip := range clips {
		absPath, err := filepath.Abs(strings.TrimSpace(clip.VideoPath))
		if err != nil {
			return nil, fmt.Errorf("resolve clip path failed: %w", err)
		}
		lines = append(lines, fmt.Sprintf("file '%s'", strings.ReplaceAll(absPath, "'", "\\'")))
	}
	if err := os.WriteFile(listPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return nil, fmt.Errorf("write concat list failed: %w", err)
	}

	if tracker != nil {
		tracker.stageStart("合成输出")
	}
	stageDuration := totalClipDuration(clips)
	profiles := buildEditRetryProfiles(encode)
	var lastOut []byte
	var lastErr error
	for _, profile := range profiles {
		videoArgs, err := ffmpegprofile.BuildEditEncodeArgs(profile.ID, encode.Quality)
		if err != nil {
			lastErr = err
			continue
		}
		args := []string{
			"-f", "concat",
			"-safe", "0",
			"-i", listPath,
			"-vf", fmt.Sprintf("settb=AVTB,setpts=PTS-STARTPTS,fps=%d,format=yuv420p", encode.FPS),
			"-af", "asetpts=PTS-STARTPTS,aformat=sample_rates=48000:channel_layouts=stereo",
		}
		args = append(args, videoArgs...)
		args = append(args,
			"-c:a", "aac",
			"-b:a", "192k",
			"-movflags", "+faststart",
			"-y",
			outputPath,
		)
		cmd := ffmpegCommand(ffmpegExe, withFFmpegProgressArgs(args)...)
		configureNoWindowProcess(cmd)
		out, err := runFFmpegCommandWithProgress(cmd, stageDuration, tracker)
		if err == nil {
			if tracker != nil {
				tracker.stageDone()
			}
			return out, nil
		}
		lastOut = out
		lastErr = fmt.Errorf("[%s] %w", profile.ID, err)
	}
	return lastOut, fmt.Errorf("ffmpeg concat failed: %w: %s", lastErr, strings.TrimSpace(string(lastOut)))
}

func concatWithTransitions(
	ffmpegExe string,
	clips []resolvedEditClip,
	transitionByIndex map[int]EditConcatTransition,
	outputPath string,
	encode editEncodeSettings,
	tracker *composeProgressTracker,
) ([]byte, error) {
	if len(clips) < 2 {
		return nil, fmt.Errorf("at least 2 clips are required for transitions")
	}

	plan, err := buildTransitionFilterGraph(clips, transitionByIndex, encode.FPS)
	if err != nil {
		return nil, err
	}

	if tracker != nil {
		tracker.stageStart("合成输出")
	}
	profiles := buildEditRetryProfiles(encode)
	var lastOut []byte
	var lastErr error
	for _, profile := range profiles {
		videoArgs, buildErr := ffmpegprofile.BuildEditEncodeArgs(profile.ID, encode.Quality)
		if buildErr != nil {
			lastErr = buildErr
			continue
		}

		args := []string{"-y"}
		for _, clip := range clips {
			args = append(args, "-i", clip.VideoPath)
		}
		args = append(args,
			"-filter_complex", plan.Filter,
			"-map", "[v]",
			"-map", "[a]",
		)
		args = append(args, videoArgs...)
		args = append(args,
			"-c:a", "aac",
			"-b:a", "192k",
			"-movflags", "+faststart",
			outputPath,
		)

		cmd := ffmpegCommand(ffmpegExe, withFFmpegProgressArgs(args)...)
		configureNoWindowProcess(cmd)
		out, runErr := runFFmpegCommandWithProgress(cmd, plan.TotalDuration, tracker)
		if runErr == nil {
			if tracker != nil {
				tracker.stageDone()
			}
			return out, nil
		}
		lastOut = out
		lastErr = fmt.Errorf("[%s] %w", profile.ID, runErr)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable ffmpeg encoding profile")
	}
	return lastOut, fmt.Errorf("ffmpeg transition failed: %w: %s", lastErr, strings.TrimSpace(string(lastOut)))
}

type transitionGraphPlan struct {
	Filter        string
	TotalDuration float64
}

func buildTransitionFilterGraph(
	clips []resolvedEditClip,
	transitionByIndex map[int]EditConcatTransition,
	fps int,
) (transitionGraphPlan, error) {
	if len(clips) < 2 {
		return transitionGraphPlan{}, fmt.Errorf("at least 2 clips are required for transitions")
	}
	if fps <= 0 {
		return transitionGraphPlan{}, fmt.Errorf("edit fps must be > 0")
	}
	for gapIndex := range transitionByIndex {
		if gapIndex < 0 || gapIndex >= len(clips)-1 {
			return transitionGraphPlan{}, fmt.Errorf("transition gap index out of range: %d", gapIndex)
		}
	}

	frameDuration := 1.0 / float64(fps)
	durations := make([]float64, len(clips))
	for i, clip := range clips {
		if clip.Width <= 0 || clip.Height <= 0 {
			return transitionGraphPlan{}, fmt.Errorf("clip %d has invalid resolution: %dx%d", i, clip.Width, clip.Height)
		}
		durations[i] = alignToFrameGrid(clip.Duration, fps)
		if durations[i] < 2*frameDuration {
			return transitionGraphPlan{}, fmt.Errorf("clip %d is too short after frame alignment: %.6f seconds", i, durations[i])
		}
	}

	width := clips[0].Width
	height := clips[0].Height
	targetSampleAspectRatio := editClipSampleAspectRatio(clips[0])
	filters := make([]string, 0, len(clips)*2+len(clips)-1)
	for i, duration := range durations {
		filters = append(filters, fmt.Sprintf(
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=%s,fps=%d,settb=AVTB,setpts=PTS-STARTPTS,format=yuv420p,trim=duration=%.6f,setpts=PTS-STARTPTS[v%d]",
			i,
			width,
			height,
			width,
			height,
			targetSampleAspectRatio,
			fps,
			duration,
			i,
		))
		filters = append(filters, fmt.Sprintf(
			"[%d:a]aresample=async=1:first_pts=0,aformat=sample_rates=48000:channel_layouts=stereo,apad,atrim=0:%.6f,asetpts=PTS-STARTPTS[a%d]",
			i,
			duration,
			i,
		))
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
				return transitionGraphPlan{}, fmt.Errorf("unsupported transition type at gap %d: %s", gapIndex, transition.Type)
			}
			duration := alignToFrameGrid(transition.Duration, fps)
			if duration < frameDuration {
				return transitionGraphPlan{}, fmt.Errorf("transition duration at gap %d is too short after frame alignment: %.6f seconds", gapIndex, duration)
			}
			if duration >= currentDuration || duration >= durations[gapIndex+1] {
				return transitionGraphPlan{}, fmt.Errorf(
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

	return transitionGraphPlan{
		Filter:        strings.Join(filters, ";"),
		TotalDuration: currentDuration,
	}, nil
}

func editClipSampleAspectRatio(clip resolvedEditClip) string {
	if ratio, ok := parseEditAspectRatio(clip.SampleAspectRatio); ok {
		return ratio.String()
	}

	displayRatio, ok := parseEditAspectRatio(clip.DisplayAspectRatio)
	if !ok || clip.Width <= 0 || clip.Height <= 0 {
		return "1/1"
	}

	return reduceEditAspectRatio(
		displayRatio.num*int64(clip.Height),
		displayRatio.den*int64(clip.Width),
	)
}

type editAspectRatio struct {
	num int64
	den int64
}

func (ratio editAspectRatio) String() string {
	return fmt.Sprintf("%d/%d", ratio.num, ratio.den)
}

func parseEditAspectRatio(raw string) (editAspectRatio, bool) {
	ratio := strings.TrimSpace(raw)
	if ratio == "" || strings.EqualFold(ratio, "N/A") {
		return editAspectRatio{}, false
	}

	separator := ":"
	if !strings.Contains(ratio, separator) {
		separator = "/"
	}
	parts := strings.Split(ratio, separator)
	if len(parts) != 2 {
		return editAspectRatio{}, false
	}
	numerator, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || numerator <= 0 {
		return editAspectRatio{}, false
	}
	denominator, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil || denominator <= 0 {
		return editAspectRatio{}, false
	}

	return reduceEditAspectRatioParts(numerator, denominator), true
}

func reduceEditAspectRatio(numerator, denominator int64) string {
	if numerator <= 0 || denominator <= 0 {
		return "1/1"
	}
	reduced := reduceEditAspectRatioParts(numerator, denominator)
	return reduced.String()
}

func reduceEditAspectRatioParts(numerator, denominator int64) editAspectRatio {
	common := editAspectRatioGCD(numerator, denominator)
	return editAspectRatio{num: numerator / common, den: denominator / common}
}

func editAspectRatioGCD(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func alignToFrameGrid(seconds float64, fps int) float64 {
	if fps <= 0 {
		return seconds
	}
	return math.Round(seconds*float64(fps)) / float64(fps)
}

func editComposeStageCount(clipCount int, withTransitions bool) int {
	if clipCount <= 0 {
		return 1
	}
	if !withTransitions {
		return 1
	}
	return 1
}

func totalClipDuration(clips []resolvedEditClip) float64 {
	total := 0.0
	for _, clip := range clips {
		total += clip.Duration
	}
	return total
}
