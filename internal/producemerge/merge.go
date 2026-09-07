// Package producemerge contains the pure business logic for merging recorded
// take files (video + audio) into final output clips using ffmpeg.
package producemerge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/procutil"
)

// FFmpegCommandContext is the function used to create context-aware ffmpeg
// exec.Cmd instances. It is a package-level variable so tests can substitute
// a fake.
var FFmpegCommandContext = exec.CommandContext

// MergeTimeout bounds how long a single ffmpeg mux may run before it is
// killed and reported as a timeout. It is a package-level variable so tests
// can shorten it.
var MergeTimeout = 10 * time.Minute

// MergeTakeVideoAudio combines a recorded video file and a WAV audio file using
// ffmpeg, writing the result to a new uniquely-named mp4 alongside the source
// video. Returns the path to the merged output file.
//
// It is a thin wrapper around MergeTakeVideoAudioContext using a background
// context; callers that have a session context (so cancellation propagates to
// the ffmpeg process) should use MergeTakeVideoAudioContext directly.
func MergeTakeVideoAudio(ffmpegExe string, videoPath string, audioPath string) (string, error) {
	return MergeTakeVideoAudioContext(context.Background(), ffmpegExe, videoPath, audioPath)
}

// MergeTakeVideoAudioContext combines a recorded video file and a WAV audio
// file using ffmpeg, writing the result to a new uniquely-named mp4 alongside
// the source video. Returns the path to the merged output file.
//
// The mux is bounded by MergeTimeout on top of ctx: a session cancellation
// kills ffmpeg immediately, and a hung ffmpeg (file lock, antivirus, network
// drive) is killed after MergeTimeout. On every exit path other than a
// successful rename, the partially written "<final>.mux.tmp.mp4" file is
// removed so no orphaned temporary mux file is left behind. If that removal
// itself fails (e.g. antivirus or a network drive still holds the file), the
// failure is joined into the returned error instead of being silently
// swallowed.
func MergeTakeVideoAudioContext(ctx context.Context, ffmpegExe string, videoPath string, audioPath string) (finalVideoPath string, retErr error) {
	if strings.TrimSpace(videoPath) == "" {
		return "", fmt.Errorf("视频文件路径为空")
	}
	if _, err := os.Stat(videoPath); err != nil {
		return "", fmt.Errorf("视频文件不存在: %s", videoPath)
	}
	if _, err := os.Stat(audioPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("音频文件不存在: %s", audioPath)
		}
		return "", fmt.Errorf("读取音频文件失败: %w", err)
	}
	exe := strings.TrimSpace(ffmpegExe)
	if exe == "" {
		return "", fmt.Errorf("ffmpeg 路径为空")
	}
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("ffmpeg 不存在: %s", exe)
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("合成任务已取消: %w", err)
	}

	finalVideoPath, err := NextMergedVideoPath(filepath.Dir(videoPath), time.Now())
	if err != nil {
		return "", fmt.Errorf("生成最终视频名失败: %w", err)
	}

	muxCtx, cancel := context.WithTimeout(ctx, MergeTimeout)
	defer cancel()

	tmpOutput := finalVideoPath + ".mux.tmp.mp4"
	_ = os.Remove(tmpOutput)
	renamed := false
	defer func() {
		if renamed {
			return
		}
		if rmErr := os.Remove(tmpOutput); rmErr != nil && !os.IsNotExist(rmErr) {
			retErr = errors.Join(retErr, fmt.Errorf("清理临时合成文件失败: %w", rmErr))
		}
	}()

	cmd := FFmpegCommandContext(
		muxCtx,
		exe,
		"-y",
		"-i", videoPath,
		"-i", audioPath,
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-c:v", "copy",
		"-c:a", "aac",
		"-b:a", "192k",
		"-shortest",
		tmpOutput,
	)
	procutil.ConfigureNoWindowProcess(cmd)

	out, err := cmd.CombinedOutput()
	if err != nil {
		switch {
		case ctx.Err() != nil:
			return "", fmt.Errorf("合成任务已取消: %w", ctx.Err())
		case muxCtx.Err() == context.DeadlineExceeded:
			return "", fmt.Errorf("ffmpeg 合成超时（超过 %s）: %w", MergeTimeout, muxCtx.Err())
		default:
			return "", fmt.Errorf("ffmpeg 合成失败: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	if err := os.Rename(tmpOutput, finalVideoPath); err != nil {
		return "", fmt.Errorf("写入合成后视频失败: %w", err)
	}
	renamed = true
	return finalVideoPath, nil
}

// NextMergedVideoPath returns the next available output path in dir for a
// merged video file. The base name is derived from now (HHMMSS) and a numeric
// suffix is appended when a conflict exists.
func NextMergedVideoPath(dir string, now time.Time) (string, error) {
	base := now.Format("150405")
	for i := 0; i < 10000; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s_%02d", base, i)
		}
		candidate := filepath.Join(dir, name+".mp4")
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				return candidate, nil
			}
			return "", err
		}
	}
	return "", fmt.Errorf("生成最终视频文件名失败")
}

// WaitForTakeFilesReady polls until both video and audio files exist, have
// stable (non-changing) sizes, and are confirmed readable by ffmpeg probe.
// Returns an error if the deadline is exceeded or ctx is cancelled.
func WaitForTakeFilesReady(
	ctx context.Context,
	ffmpegExe string,
	videoPath string,
	audioPath string,
	timeout time.Duration,
	interval time.Duration,
) error {
	deadline := time.Now().Add(timeout)
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	var (
		lastVideoSize int64 = -1
		lastAudioSize int64 = -1
		stableCount   int
		lastProbeErr  string
	)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("合成任务已取消")
		default:
		}

		videoInfo, videoErr := os.Stat(videoPath)
		audioInfo, audioErr := os.Stat(audioPath)
		if videoErr == nil && audioErr == nil {
			videoSize := videoInfo.Size()
			audioSize := audioInfo.Size()
			if videoSize > 0 && audioSize > 0 && videoSize == lastVideoSize && audioSize == lastAudioSize {
				stableCount++
				if stableCount >= 2 {
					if err := probeTakeFilesReadable(ctx, ffmpegExe, videoPath, audioPath); err == nil {
						return nil
					} else {
						lastProbeErr = err.Error()
					}
				}
			} else {
				stableCount = 0
				lastVideoSize = videoSize
				lastAudioSize = audioSize
			}
		} else {
			stableCount = 0
			lastVideoSize = -1
			lastAudioSize = -1
		}

		if time.Now().After(deadline) {
			if strings.TrimSpace(lastProbeErr) != "" {
				return fmt.Errorf("等待录制文件落地超时（%ds）: %s", int(timeout/time.Second), lastProbeErr)
			}
			return fmt.Errorf("等待录制文件落地超时（%ds）", int(timeout/time.Second))
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("合成任务已取消")
		case <-time.After(interval):
		}
	}
}

// probeTakeFilesReadable uses ffmpeg to verify that both the video and audio
// files can be opened and decoded. Returns an error if probing fails.
func probeTakeFilesReadable(ctx context.Context, ffmpegExe string, videoPath string, audioPath string) error {
	exe := strings.TrimSpace(ffmpegExe)
	if exe == "" {
		return fmt.Errorf("ffmpeg 路径为空")
	}
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("ffmpeg 不存在: %s", exe)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := FFmpegCommandContext(
		probeCtx,
		exe,
		"-v", "error",
		"-i", videoPath,
		"-i", audioPath,
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-t", "0",
		"-f", "null",
		"-",
	)
	procutil.ConfigureNoWindowProcess(cmd)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("ffmpeg probe 已取消: %w", ctx.Err())
	}
	if probeCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("ffmpeg probe 超时")
	}
	if err != nil {
		return fmt.Errorf("ffmpeg probe 失败: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
