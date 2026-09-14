// Package edit contains the side-effect-free planning and context-aware
// execution primitives used by the application's edit boundary.
//
// The package deliberately does not know about Wails, App, workspace
// sessions, history, or UI DTOs. Callers provide already-probed media facts,
// an executable command factory, and a progress sink; publication and
// history remain the responsibility of the caller that owns the task.
package edit

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"cs2-highlight-tool-v2/internal/ffmpegprofile"
)

// Clip is the explicit input to the edit planner. Duration is the request
// value until a caller supplies an authoritative probe result.
type Clip struct {
	VideoPath    string
	Duration     float64
	StartSeconds *float64
	EndSeconds   *float64
}

// Transition describes the gap after AfterIndex. A zero AfterIndex remains
// compatible with the historical sequential transition form.
type Transition struct {
	Type       string
	Duration   float64
	AfterIndex int
}

// ProbedVideoInfo is the media fact set used by the planner. It is produced by
// ProbeVideoStreamInfo and is intentionally passed in rather than discovered
// by pure planning functions.
type ProbedVideoInfo struct {
	Duration           float64
	Width              int
	Height             int
	SampleAspectRatio  string
	DisplayAspectRatio string
	HasAudio           bool
	AudioKnown         bool
}

// ResolvedClip is a normalized clip ready for filter-graph or concat-command
// planning. Duration is authoritative when a probe was supplied.
type ResolvedClip struct {
	VideoPath          string
	Duration           float64
	TrimStart          float64
	TrimEnd            float64
	HasTrim            bool
	Width              int
	Height             int
	SampleAspectRatio  string
	DisplayAspectRatio string
	HasAudio           bool
	AudioKnown         bool
}

// EncodeSettings is the immutable encoding choice for one compose task.
type EncodeSettings struct {
	FPS         int
	Quality     string
	VideoPreset string
	Caps        ffmpegprofile.Capabilities
}

// EncodeLimits lets the application provide its configuration-domain bounds
// without making this package read configuration or global state.
type EncodeLimits struct {
	DefaultFPS int
	MinFPS     int
	MaxFPS     int
}

// TransitionGraphPlan is the pure output of filter-graph planning.
type TransitionGraphPlan struct {
	Filter        string
	TotalDuration float64
}

// CommandFactory is the only process-construction seam needed by the runner
// and probe helpers. It allows tests to inject deterministic helper commands.
type CommandFactory func(context.Context, string, ...string) *exec.Cmd

// ConfigureProcess is an optional platform hook (for example, hiding a
// console window on Windows). The edit package never chooses process flags on
// behalf of the application.
type ConfigureProcess func(*exec.Cmd)

// ProgressEventKind identifies the small internal progress vocabulary. App
// maps these events to the existing compose_progress payload.
type ProgressEventKind string

const (
	ProgressStageStart    ProgressEventKind = "stage_start"
	ProgressStageProgress ProgressEventKind = "stage_progress"
	ProgressStageDone     ProgressEventKind = "stage_done"
)

// ProgressEvent is emitted by Runner while a compose command is active.
type ProgressEvent struct {
	Kind  ProgressEventKind
	Label string
	Ratio float64
}

// ProgressSink receives internal events. It is nil-safe and never called
// while the runner holds any package lock.
type ProgressSink func(ProgressEvent)

// ErrorKind is intentionally small: callers can branch on lifecycle stages
// without parsing FFmpeg/ffprobe stderr text.
type ErrorKind string

const (
	ErrorCanceled ErrorKind = "canceled"
	ErrorProbe    ErrorKind = "probe"
	ErrorEncode   ErrorKind = "encode"
	ErrorPublish  ErrorKind = "publish"
)

// Error preserves the human-readable message while exposing a stable domain
// category to adapters that need to choose a UI/logging path.
type Error struct {
	Kind ErrorKind
	Err  error
}

func (e *Error) Error() string {
	if e == nil || e.Err == nil {
		return string(e.Kind)
	}
	return e.Err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewError(kind ErrorKind, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Err: err}
}

func KindOf(err error) ErrorKind {
	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr.Kind
	}
	return ""
}

// Runner executes probe/compose commands with the caller's context. Public
// probe/compose methods reject a nil context rather than creating an
// unmanaged background context. It owns neither output reservation nor final
// publication; those remain at App's task boundary.
type Runner struct {
	CommandContext   CommandFactory
	ConfigureProcess ConfigureProcess
	ProbeTimeout     time.Duration
	ComposeTimeout   time.Duration
	Progress         ProgressSink
}
