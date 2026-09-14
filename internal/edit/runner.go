package edit

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"cs2-highlight-tool-v2/internal/ffmpegprofile"
)

const defaultComposeTimeout = 30 * time.Minute

func (r Runner) composeTimeout() time.Duration {
	if r.ComposeTimeout > 0 {
		return r.ComposeTimeout
	}
	return defaultComposeTimeout
}

func (r Runner) emit(event ProgressEvent) {
	if r.Progress != nil {
		r.Progress(event)
	}
}

func canceledError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return NewError(ErrorCanceled, fmt.Errorf("剪辑任务已取消: %w", err))
	}
	return nil
}

// ComposeSimple runs the no-transition concat path. The caller owns listPath
// and outputPath; this method only writes/removes its task-local list file and
// invokes FFmpeg with the provided context.
func (r Runner) ComposeSimple(ctx context.Context, ffmpegExe string, clips []ResolvedClip, listPath, outputPath string, encode EncodeSettings) ([]byte, error) {
	if ctx == nil {
		return nil, fmt.Errorf("edit context is required")
	}
	if cancelErr := canceledError(ctx); cancelErr != nil {
		return nil, cancelErr
	}
	if len(clips) == 0 {
		return nil, fmt.Errorf("at least 1 clip is required")
	}
	defer os.Remove(listPath)

	lines := make([]string, 0, len(clips))
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

	r.emit(ProgressEvent{Kind: ProgressStageStart, Label: "合成输出"})
	stageDuration := TotalClipDuration(clips)
	profiles := BuildRetryProfiles(encode)
	var lastOut []byte
	var lastErr error
	for _, profile := range profiles {
		if cancelErr := canceledError(ctx); cancelErr != nil {
			return lastOut, cancelErr
		}
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
		attemptCtx, cancelAttempt := context.WithTimeout(ctx, r.composeTimeout())
		cmd := r.command(attemptCtx, ffmpegExe, WithProgressArgs(args)...)
		r.configure(cmd)
		out, err := r.runCommandWithProgress(cmd, stageDuration)
		timedOut := errors.Is(attemptCtx.Err(), context.DeadlineExceeded)
		cancelAttempt()
		if err == nil {
			if cancelErr := canceledError(ctx); cancelErr != nil {
				return out, cancelErr
			}
			r.emit(ProgressEvent{Kind: ProgressStageDone})
			return out, nil
		}
		lastOut = out
		if cancelErr := canceledError(ctx); cancelErr != nil {
			return lastOut, cancelErr
		}
		if timedOut {
			lastErr = fmt.Errorf("[%s] 剪辑合成超时（超过 %s）", profile.ID, r.composeTimeout())
			continue
		}
		lastErr = fmt.Errorf("[%s] %w", profile.ID, err)
	}
	return lastOut, NewError(ErrorEncode, fmt.Errorf("ffmpeg concat failed: %w: %s", lastErr, strings.TrimSpace(string(lastOut))))
}

// ComposeWithTransitions runs the filter-graph path and retains the existing
// retry/fallback behavior. It returns command output only; publication is
// deliberately outside this package.
func (r Runner) ComposeWithTransitions(ctx context.Context, ffmpegExe string, clips []ResolvedClip, transitionByIndex map[int]Transition, outputPath string, encode EncodeSettings) ([]byte, error) {
	if ctx == nil {
		return nil, fmt.Errorf("edit context is required")
	}
	if cancelErr := canceledError(ctx); cancelErr != nil {
		return nil, cancelErr
	}
	if len(clips) == 0 {
		return nil, fmt.Errorf("at least 1 clip is required")
	}
	plan, err := BuildTransitionFilterGraph(clips, transitionByIndex, encode.FPS)
	if err != nil {
		return nil, err
	}

	r.emit(ProgressEvent{Kind: ProgressStageStart, Label: "合成输出"})
	profiles := BuildRetryProfiles(encode)
	var lastOut []byte
	var lastErr error
	for _, profile := range profiles {
		if cancelErr := canceledError(ctx); cancelErr != nil {
			return lastOut, cancelErr
		}
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

		attemptCtx, cancelAttempt := context.WithTimeout(ctx, r.composeTimeout())
		cmd := r.command(attemptCtx, ffmpegExe, WithProgressArgs(args)...)
		r.configure(cmd)
		out, runErr := r.runCommandWithProgress(cmd, plan.TotalDuration)
		timedOut := errors.Is(attemptCtx.Err(), context.DeadlineExceeded)
		cancelAttempt()
		if runErr == nil {
			if cancelErr := canceledError(ctx); cancelErr != nil {
				return out, cancelErr
			}
			r.emit(ProgressEvent{Kind: ProgressStageDone})
			return out, nil
		}
		lastOut = out
		if cancelErr := canceledError(ctx); cancelErr != nil {
			return lastOut, cancelErr
		}
		if timedOut {
			lastErr = fmt.Errorf("[%s] 剪辑合成超时（超过 %s）", profile.ID, r.composeTimeout())
			continue
		}
		lastErr = fmt.Errorf("[%s] %w", profile.ID, runErr)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable ffmpeg encoding profile")
	}
	return lastOut, NewError(ErrorEncode, fmt.Errorf("ffmpeg transition failed: %w: %s", lastErr, strings.TrimSpace(string(lastOut))))
}

// WithProgressArgs asks FFmpeg for machine-readable progress on stdout while
// keeping the output path as the final argument.
func WithProgressArgs(args []string) []string {
	if len(args) == 0 {
		return []string{"-progress", "pipe:1", "-nostats"}
	}
	last := args[len(args)-1]
	rebuilt := make([]string, 0, len(args)+3)
	rebuilt = append(rebuilt, args[:len(args)-1]...)
	rebuilt = append(rebuilt, "-progress", "pipe:1", "-nostats", last)
	return rebuilt
}

// RunCommandWithProgress executes an already-created command. It is exported
// for the App adapter's focused progress tests and does not know about Wails.
func RunCommandWithProgress(cmd *exec.Cmd, expectedDurationSeconds float64, sink ProgressSink) ([]byte, error) {
	return runCommandWithProgress(cmd, expectedDurationSeconds, sink)
}

func (r Runner) runCommandWithProgress(cmd *exec.Cmd, expectedDurationSeconds float64) ([]byte, error) {
	return runCommandWithProgress(cmd, expectedDurationSeconds, func(event ProgressEvent) { r.emit(event) })
}

func runCommandWithProgress(cmd *exec.Cmd, expectedDurationSeconds float64, sink ProgressSink) ([]byte, error) {
	if sink == nil {
		return cmd.CombinedOutput()
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var readWG sync.WaitGroup

	readWG.Add(1)
	go func() {
		defer readWG.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			stdoutBuf.WriteString(line)
			stdoutBuf.WriteByte('\n')
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key == "progress" && value == "end" {
				sink(ProgressEvent{Kind: ProgressStageProgress, Ratio: 1})
				continue
			}
			outSeconds, parsed := ParseProgressSeconds(key, value)
			if !parsed || expectedDurationSeconds <= 0 {
				continue
			}
			sink(ProgressEvent{Kind: ProgressStageProgress, Ratio: outSeconds / expectedDurationSeconds})
		}
	}()

	readWG.Add(1)
	go func() {
		defer readWG.Done()
		_, _ = io.Copy(&stderrBuf, stderrPipe)
	}()

	waitErr := cmd.Wait()
	readWG.Wait()

	combined := append([]byte{}, stdoutBuf.Bytes()...)
	combined = append(combined, stderrBuf.Bytes()...)
	if waitErr != nil {
		return combined, waitErr
	}
	return combined, nil
}

// ParseProgressSeconds parses the FFmpeg progress keys used by the current
// compose event contract.
func ParseProgressSeconds(key, value string) (float64, bool) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	switch key {
	case "out_time_us", "out_time_ms":
		raw, err := strconv.ParseInt(value, 10, 64)
		if err != nil || raw < 0 {
			return 0, false
		}
		return float64(raw) / 1_000_000, true
	case "out_time":
		parts := strings.Split(value, ":")
		if len(parts) != 3 {
			return 0, false
		}
		hours, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, false
		}
		minutes, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, false
		}
		seconds, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return 0, false
		}
		return hours*3600 + minutes*60 + seconds, true
	default:
		return 0, false
	}
}
