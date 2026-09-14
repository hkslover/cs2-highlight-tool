package edit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

const defaultProbeTimeout = 30

func (r Runner) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	factory := r.CommandContext
	if factory == nil {
		factory = exec.CommandContext
	}
	return factory(ctx, name, args...)
}

func (r Runner) configure(cmd *exec.Cmd) {
	if r.ConfigureProcess != nil {
		r.ConfigureProcess(cmd)
	}
}

func (r Runner) probeTimeout() string {
	if r.ProbeTimeout > 0 {
		return r.ProbeTimeout.String()
	}
	return fmt.Sprintf("%ds", defaultProbeTimeout)
}

// ProbeDuration invokes ffprobe with the caller's context and returns the
// format duration rounded to milliseconds for the public duration API.
func (r Runner) ProbeDuration(ctx context.Context, ffprobeExe, videoPath string) (float64, error) {
	if ctx == nil {
		return 0, fmt.Errorf("edit context is required")
	}
	if err := ctx.Err(); err != nil {
		return 0, probeFailure(ctx, err, "", r.probeTimeout())
	}
	cmd := r.command(ctx, ffprobeExe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)
	r.configure(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, probeFailure(ctx, err, string(out), r.probeTimeout())
	}
	raw := strings.TrimSpace(string(out))
	duration, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, NewError(ErrorProbe, fmt.Errorf("parse ffprobe duration failed: %s", raw))
	}
	if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return 0, NewError(ErrorProbe, fmt.Errorf("invalid ffprobe duration: %s", raw))
	}
	return math.Round(duration*1000) / 1000, nil
}

// ProbeVideoStreamInfo obtains the authoritative video-stream duration and
// audio/SAR/DAR facts for transition or trim planning.
func (r Runner) ProbeVideoStreamInfo(ctx context.Context, ffprobeExe, videoPath string) (ProbedVideoInfo, error) {
	if ctx == nil {
		return ProbedVideoInfo{}, fmt.Errorf("edit context is required")
	}
	if err := ctx.Err(); err != nil {
		return ProbedVideoInfo{}, probeFailure(ctx, err, "", r.probeTimeout())
	}
	cmd := r.command(ctx, ffprobeExe,
		"-v", "error",
		"-show_entries", "stream=codec_type,duration,width,height,sample_aspect_ratio,display_aspect_ratio",
		"-show_entries", "format=duration",
		"-of", "json",
		videoPath,
	)
	r.configure(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ProbedVideoInfo{}, probeFailure(ctx, err, string(out), r.probeTimeout())
	}

	var payload struct {
		Streams []struct {
			CodecType          string          `json:"codec_type"`
			Duration           json.RawMessage `json:"duration"`
			Width              int             `json:"width"`
			Height             int             `json:"height"`
			SampleAspectRatio  string          `json:"sample_aspect_ratio"`
			DisplayAspectRatio string          `json:"display_aspect_ratio"`
		} `json:"streams"`
		Format struct {
			Duration json.RawMessage `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return ProbedVideoInfo{}, NewError(ErrorProbe, fmt.Errorf("parse ffprobe video stream failed: %w", err))
	}
	if len(payload.Streams) == 0 {
		return ProbedVideoInfo{}, NewError(ErrorProbe, fmt.Errorf("ffprobe returned no video stream"))
	}

	videoIndex := -1
	hasAudio := false
	hasCodecType := false
	for index, candidate := range payload.Streams {
		codecType := strings.ToLower(strings.TrimSpace(candidate.CodecType))
		if codecType != "" {
			hasCodecType = true
		}
		if codecType == "audio" {
			hasAudio = true
		}
		if videoIndex < 0 && (codecType == "video" || codecType == "") {
			videoIndex = index
		}
	}
	if videoIndex < 0 {
		return ProbedVideoInfo{}, NewError(ErrorProbe, fmt.Errorf("ffprobe returned no video stream"))
	}
	stream := payload.Streams[videoIndex]
	duration, ok := parseProbeDurationValue(stream.Duration)
	if !ok {
		duration, ok = parseProbeDurationValue(payload.Format.Duration)
	}
	if !ok {
		return ProbedVideoInfo{}, NewError(ErrorProbe, fmt.Errorf("ffprobe returned invalid video duration"))
	}
	if stream.Width <= 0 || stream.Height <= 0 {
		return ProbedVideoInfo{}, NewError(ErrorProbe, fmt.Errorf("ffprobe returned invalid video resolution: %dx%d", stream.Width, stream.Height))
	}

	return ProbedVideoInfo{
		Duration:           duration,
		Width:              stream.Width,
		Height:             stream.Height,
		SampleAspectRatio:  strings.TrimSpace(stream.SampleAspectRatio),
		DisplayAspectRatio: strings.TrimSpace(stream.DisplayAspectRatio),
		HasAudio:           hasAudio,
		AudioKnown:         hasCodecType,
	}, nil
}

func parseProbeDurationValue(raw json.RawMessage) (float64, bool) {
	value := strings.TrimSpace(string(raw))
	value = strings.Trim(value, `"`)
	if value == "" || strings.EqualFold(value, "N/A") {
		return 0, false
	}
	duration, err := strconv.ParseFloat(value, 64)
	if err != nil || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return 0, false
	}
	return duration, true
}

func probeFailure(ctx context.Context, cmdErr error, output, timeout string) error {
	if ctx != nil {
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return NewError(ErrorProbe, fmt.Errorf("ffprobe 探测超时（超过 %s）: %w", timeout, ctx.Err()))
		case ctx.Err() != nil:
			return NewError(ErrorCanceled, fmt.Errorf("工作目录正在关闭，探测已取消: %w", ctx.Err()))
		}
	}
	return NewError(ErrorProbe, fmt.Errorf("ffprobe failed: %w: %s", cmdErr, strings.TrimSpace(output)))
}
