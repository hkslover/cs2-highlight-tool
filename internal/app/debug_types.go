package app

// Debug session status values. They are deliberately separate from the produce
// queue and startup enums: a debug session is an interactive workbench, not a
// recording queue.
const (
	DebugStatusIdle     = "idle"
	DebugStatusStarting = "starting"
	DebugStatusRunning  = "running"
	DebugStatusStopping = "stopping"
	DebugStatusFailed   = "failed"
)

// DebugLaunchRequest freezes the launch options of one debug session. The
// frontend initializes it from the clip settings draft, but the values are
// session-scoped and never written back to the persisted config.
type DebugLaunchRequest struct {
	InjectPlugin     bool   `json:"inject_plugin"`
	PovHudEnabled    bool   `json:"pov_hud_enabled"`
	BrandEnabled     bool   `json:"brand_enabled"`
	PovRadarEnabled  bool   `json:"pov_radar_enabled"`
	LaunchResolution string `json:"launch_resolution"`
	// Nil keeps legacy defaults; an explicit empty string removes them.
	LaunchArguments *string  `json:"launch_arguments,omitempty"`
	StartupCommands []string `json:"startup_commands,omitempty"`
}

// Debug protocol readiness and result statuses. `executed` means the plugin
// handed the commands to the game thread; it is not a guarantee that a
// particular console command had an observable effect. Version 2 replaced the
// playback observation's current tick with engine pause reporting, so an older
// DLL is rejected with an update prompt instead of showing a pause button whose
// state can never be confirmed.
const (
	DebugProtocolVersion = 2

	DebugCommandStatusExecuted = "executed"
	DebugCommandStatusFailed   = "failed"
)

// Plugin transport/readiness classification of the debug snapshot. It is
// derived from the host's own connection and handshake facts so the page never
// has to guess a state from the localized PluginMessage text. It is not a
// session lifecycle status and never claims readiness from a connection alone.
const (
	DebugPluginStateDisabled          = "disabled"
	DebugPluginStateWaitingConnection = "waiting_connection"
	DebugPluginStateWaitingReady      = "waiting_ready"
	DebugPluginStateReady             = "ready"
	DebugPluginStateError             = "error"
)

// DebugCommandRequest is the public request of SendDebugConsoleCommand. Each
// element is one console command string; a custom input keeps its semicolons
// and quotes inside a single element, while quick settings may send several.
type DebugCommandRequest struct {
	Commands []string `json:"commands"`
}

// DebugCommandResponse is the public execution receipt. Status is `executed`
// or `failed`; Error carries the plugin-reported failure when available.
type DebugCommandResponse struct {
	RequestID string   `json:"request_id"`
	Status    string   `json:"status"`
	Error     string   `json:"error,omitempty"`
	Commands  []string `json:"commands"`
}

// DebugPlayDemoRequest plays one managed Demo in the running debug session. It
// never loads a produce sequence file: the plugin clears any queued sequences
// and issues a quoted `playdemo` on the game thread. Commands are optional
// debug-draft settings applied when the Demo can actually accept them; plugin
// special actions (for example the POV radar) are initialized immediately.
type DebugPlayDemoRequest struct {
	DemoPath string   `json:"demo_path"`
	Commands []string `json:"commands,omitempty"`
}

// DebugPlayDemoResponse is the receipt for PlayDebugDemo. `executed` only
// proves the plugin handed `playdemo` to the engine; the actual playback state
// is reported separately through DebugSessionState.DemoPlaying.
type DebugPlayDemoResponse struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	DemoPath  string `json:"demo_path"`
}

// DebugDemoState is the plugin-observed playback state. It is only emitted on a
// playback start/stop/pause transition (or an observed path change), so the
// desktop never mistakes a queued `playdemo` for proof that the engine is
// actually playing the Demo. Paused is an engine fact of the loaded Demo; it is
// never inferred from tick growth.
type DebugDemoState struct {
	SessionID string `json:"session_id"`
	DemoPath  string `json:"demo_path"`
	Playing   bool   `json:"playing"`
	Paused    bool   `json:"paused"`
}

// Manual debug recording lifecycle statuses. They are separate from the
// produce take status and never enter the production queue/history pipeline.
const (
	DebugRecordingStatusStarting  = "starting"
	DebugRecordingStatusRecording = "recording"
	DebugRecordingStatusStopping  = "stopping"
	DebugRecordingStatusMerging   = "merging"
	DebugRecordingStatusCompleted = "completed"
	DebugRecordingStatusFailed    = "failed"
)

// DebugRecordingRequest starts one manual recording segment. The workbench
// freezes these values for the segment; an empty quality/preset falls back to
// the persisted clip settings, and `auto` is resolved to the actual probed
// encoder before the commands are built. `keep_intermediate_files` reuses the
// debug session option so a successful merge can drop only this segment's raw
// video/audio.
type DebugRecordingRequest struct {
	RecordFPS             int    `json:"record_fps"`
	RecordQuality         string `json:"record_quality"`
	VideoPreset           string `json:"video_preset"`
	MotionBlurEnabled     bool   `json:"motion_blur_enabled"`
	KeepIntermediateFiles bool   `json:"keep_intermediate_files"`
}

// DebugRecordingStatus is one manual recording segment in the workbench
// snapshot. It is deliberately not a ProduceTakePlan: it carries no kill,
// view or take-plan metadata and is never registered in the produce history.
// Directory/VideoPath/AudioPath are the known expected paths; FinalPath is
// only set after a successful mux. A failed merge keeps both source paths so
// RetryDebugRecordingMerge can re-run only this segment.
type DebugRecordingStatus struct {
	RecordingID string `json:"recording_id"`
	DemoPath    string `json:"demo_path"`
	TakeName    string `json:"take_name"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`

	Directory string `json:"directory"`
	VideoPath string `json:"video_path,omitempty"`
	AudioPath string `json:"audio_path,omitempty"`
	FinalPath string `json:"final_path,omitempty"`

	StartTick   int   `json:"start_tick,omitempty"`
	EndTick     int   `json:"end_tick,omitempty"`
	StartedAtMs int64 `json:"started_at_ms"`
	UpdatedAtMs int64 `json:"updated_at_ms"`
}

// DebugSessionState is the full public snapshot of the debug workbench. Process
// start and plugin readiness are reported separately: a running CS2 PID does
// not prove that the plugin transport is connected, and a connected transport
// does not prove that the debug protocol is available.
type DebugSessionState struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`

	InjectPlugin     bool   `json:"inject_plugin"`
	PovHudEnabled    bool   `json:"pov_hud_enabled"`
	BrandEnabled     bool   `json:"brand_enabled"`
	PovRadarEnabled  bool   `json:"pov_radar_enabled"`
	LaunchResolution string `json:"launch_resolution"`

	LaunchArguments      string                `json:"launch_arguments"`
	StartupCommands      []string              `json:"startup_commands"`
	StartupCommandResult *DebugCommandResponse `json:"startup_command_result,omitempty"`

	ProcessStarted  bool   `json:"process_started"`
	CS2PID          int    `json:"cs2_pid"`
	PluginEnabled   bool   `json:"plugin_enabled"`
	PluginConnected bool   `json:"plugin_connected"`
	PluginReady     bool   `json:"plugin_ready"`
	PluginMessage   string `json:"plugin_message"`
	// PluginState is one of disabled|waiting_connection|waiting_ready|ready|error.
	PluginState string `json:"plugin_state"`

	// DemoPath is the last Demo a play request targeted; DemoPlaying and
	// DemoPaused are the plugin-observed engine playback facts, never inferred
	// from a successful play receipt. DemoPlayPending is true while a sent play
	// request has not yet been confirmed by an engine observation of the
	// requested Demo: pause/resume must stay disabled in that window because the
	// last observation may still describe the previous Demo.
	DemoPath        string `json:"demo_path"`
	DemoPlaying     bool   `json:"demo_playing"`
	DemoPaused      bool   `json:"demo_paused"`
	DemoPlayPending bool   `json:"demo_play_pending"`

	// Recording is the active manual recording segment (starting/recording/
	// stopping/merging), or null when none is active. It is always present so
	// the page never has to distinguish "missing" from "no active segment".
	// Recordings is the result list of this workspace (active segment included),
	// newest last. Both are populated even after the game is closed, so a failed
	// merge stays visible and retryable.
	Recording  *DebugRecordingStatus  `json:"recording"`
	Recordings []DebugRecordingStatus `json:"recordings"`

	Error       string `json:"error"`
	UpdatedAtMs int64  `json:"updated_at_ms"`
}
