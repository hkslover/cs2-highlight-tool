package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	editdomain "cs2-highlight-tool-v2/internal/edit"
)

// These DTOs remain at the Wails boundary. The edit domain receives explicit
// copies through the narrow adapters below and never imports App or Wails.
type EditConcatClip struct {
	VideoPath    string   `json:"video_path"`
	Duration     float64  `json:"duration"`
	StartSeconds *float64 `json:"start_seconds,omitempty"`
	EndSeconds   *float64 `json:"end_seconds,omitempty"`
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

// Aliases keep existing App boundary tests and call sites source-compatible;
// the implementation types now live in internal/edit.
type resolvedEditClip = editdomain.ResolvedClip
type editEncodeSettings = editdomain.EncodeSettings
type transitionGraphPlan = editdomain.TransitionGraphPlan
type probedVideoInfo = editdomain.ProbedVideoInfo

// ConcatEditClips owns the Wails/workspace boundary and publication lifecycle.
// Planning and command execution are delegated to internal/edit; only the
// final rename and history entry stay here.
func (a *App) ConcatEditClips(request EditConcatRequest) (string, error) {
	workCtx, releaseFiles, workspaceRoot, fileErr := a.beginManagedWorkspaceTaskUse()
	if fileErr != nil {
		return "", fileErr
	}
	defer releaseFiles()

	task, err := a.beginEditComposeTask(workCtx, workspaceRoot)
	if err != nil {
		return "", err
	}
	// Deferred order is LIFO: task cleanup runs first, then the slot is
	// released, and only then is the workspace file use released. The old task
	// therefore finishes its final events and files before a new task or a
	// directory clear can enter.
	defer a.finishEditComposeTask(task)
	defer task.cleanup()

	transitionByIndex, err := normalizeEditTransitions(len(request.Clips), request.Transitions)
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
	if err := task.prepare(outputDir); err != nil {
		return "", err
	}

	needsTrimProbe := false
	for _, clip := range request.Clips {
		if clip.StartSeconds != nil || clip.EndSeconds != nil {
			needsTrimProbe = true
			break
		}
	}
	resolvedClips, err := a.resolveEditClips(task.ctx, request.Clips, len(transitionByIndex) > 0 || needsTrimProbe)
	if err != nil {
		return "", err
	}

	tracker := newComposeProgressTracker(a, editComposeStageCount(len(resolvedClips), len(transitionByIndex) > 0))
	if len(transitionByIndex) == 0 && !hasEditTrim(resolvedClips) {
		if _, err := concatSimple(a, task.ctx, ffmpegExe, resolvedClips, filepath.Join(task.tempDir, "concat.txt"), task.tempOutput, encode, tracker); err != nil {
			tracker.fail(err)
			return "", err
		}
	} else {
		if _, err := concatWithTransitions(a, task.ctx, ffmpegExe, resolvedClips, transitionByIndex, task.tempOutput, encode, tracker); err != nil {
			tracker.fail(err)
			return "", err
		}
	}

	if err := verifyEditTempOutput(task.tempOutput); err != nil {
		tracker.fail(err)
		return "", err
	}
	// Cancellation wins until this boundary. Once the rename succeeds the
	// artifact is published and history records that committed fact.
	if cancelErr := editTaskCanceledError(task.ctx); cancelErr != nil {
		tracker.fail(cancelErr)
		return "", cancelErr
	}
	if err := task.commit(); err != nil {
		tracker.fail(err)
		return "", err
	}

	a.addEditedHistoryEntry(task.finalOutput, "edit_timeline")
	tracker.complete()
	return task.finalOutput, nil
}

// resolveEditClips performs only boundary validation/configured probing. The
// probe implementation and clip normalization are domain operations.
func (a *App) resolveEditClips(ctx context.Context, input []EditConcatClip, forceProbe ...bool) ([]resolvedEditClip, error) {
	if ctx == nil {
		ctx = context.Background()
	}
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
	runner := a.newEditRunner(nil)
	resolved := make([]resolvedEditClip, 0, len(input))
	for i, clip := range input {
		if cancelErr := editTaskCanceledError(ctx); cancelErr != nil {
			return nil, cancelErr
		}
		p := strings.TrimSpace(clip.VideoPath)
		if p == "" {
			return nil, fmt.Errorf("clip %d video path is empty", i+1)
		}
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("clip %d video file not found: %s", i+1, p)
		}

		var info *editdomain.ProbedVideoInfo
		duration := clip.Duration
		if probeForTransitions {
			probeCtx, cancelProbe := context.WithTimeout(ctx, editProbeTimeout)
			probed, err := runner.ProbeVideoStreamInfo(probeCtx, ffprobeExe, p)
			cancelProbe()
			if err != nil {
				return nil, fmt.Errorf("clip %d probe video stream failed: %w", i+1, err)
			}
			info = &probed
		} else if duration <= 0 {
			if ffprobeExe == "" {
				return nil, fmt.Errorf("clip %d duration is invalid and ffprobe not found", i+1)
			}
			if _, err := os.Stat(ffprobeExe); err != nil {
				return nil, fmt.Errorf("ffprobe not found at %s", ffprobeExe)
			}
			probeCtx, cancelProbe := context.WithTimeout(ctx, editProbeTimeout)
			probed, err := runner.ProbeDuration(probeCtx, ffprobeExe, p)
			cancelProbe()
			if err != nil {
				return nil, fmt.Errorf("clip %d probe duration failed: %w", i+1, err)
			}
			duration = probed
		}

		resolvedClip, err := editdomain.ResolveClip(toDomainClip(clip), duration, info, i, info != nil)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, resolvedClip)
	}
	return resolved, nil
}

func toDomainClip(clip EditConcatClip) editdomain.Clip {
	return editdomain.Clip{
		VideoPath:    clip.VideoPath,
		Duration:     clip.Duration,
		StartSeconds: clip.StartSeconds,
		EndSeconds:   clip.EndSeconds,
	}
}

func toDomainTransitions(input []EditConcatTransition) []editdomain.Transition {
	result := make([]editdomain.Transition, 0, len(input))
	for _, transition := range input {
		result = append(result, editdomain.Transition{
			Type:       transition.Type,
			Duration:   transition.Duration,
			AfterIndex: transition.AfterIndex,
		})
	}
	return result
}

func fromDomainTransition(transition editdomain.Transition) EditConcatTransition {
	return EditConcatTransition{Type: transition.Type, Duration: transition.Duration, AfterIndex: transition.AfterIndex}
}

func normalizeEditTransitions(clipCount int, input []EditConcatTransition) (map[int]EditConcatTransition, error) {
	normalized, err := editdomain.NormalizeTransitions(clipCount, toDomainTransitions(input))
	if err != nil {
		return nil, err
	}
	result := make(map[int]EditConcatTransition, len(normalized))
	for index, transition := range normalized {
		result[index] = fromDomainTransition(transition)
	}
	return result, nil
}

func resolveEditTrimRange(clip EditConcatClip, duration float64, index int) (float64, float64, bool, error) {
	return editdomain.ResolveTrimRange(toDomainClip(clip), duration, index)
}

func hasEditTrim(clips []resolvedEditClip) bool {
	return editdomain.HasTrim(clips)
}

func editComposeStageCount(clipCount int, withTransitions bool) int {
	return editdomain.StageCount(clipCount, withTransitions)
}

func totalClipDuration(clips []resolvedEditClip) float64 {
	return editdomain.TotalClipDuration(clips)
}

func buildTransitionFilterGraph(clips []resolvedEditClip, transitionByIndex map[int]EditConcatTransition, fps int) (transitionGraphPlan, error) {
	transitions := make(map[int]editdomain.Transition, len(transitionByIndex))
	for index, transition := range transitionByIndex {
		transitions[index] = editdomain.Transition{Type: transition.Type, Duration: transition.Duration, AfterIndex: transition.AfterIndex}
	}
	return editdomain.BuildTransitionFilterGraph(clips, transitions, fps)
}

func editClipSampleAspectRatio(clip resolvedEditClip) string {
	return editdomain.ClipSampleAspectRatio(clip)
}

func alignToFrameGrid(seconds float64, fps int) float64 {
	return editdomain.AlignToFrameGrid(seconds, fps)
}

func concatSimple(app *App, ctx context.Context, ffmpegExe string, clips []resolvedEditClip, listPath, outputPath string, encode editEncodeSettings, tracker *composeProgressTracker) ([]byte, error) {
	return app.newEditRunner(tracker).ComposeSimple(ctx, ffmpegExe, clips, listPath, outputPath, encode)
}

func concatWithTransitions(app *App, ctx context.Context, ffmpegExe string, clips []resolvedEditClip, transitionByIndex map[int]EditConcatTransition, outputPath string, encode editEncodeSettings, tracker *composeProgressTracker) ([]byte, error) {
	transitions := make(map[int]editdomain.Transition, len(transitionByIndex))
	for index, transition := range transitionByIndex {
		transitions[index] = editdomain.Transition{Type: transition.Type, Duration: transition.Duration, AfterIndex: transition.AfterIndex}
	}
	return app.newEditRunner(tracker).ComposeWithTransitions(ctx, ffmpegExe, clips, transitions, outputPath, encode)
}
