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
}

// Debug protocol readiness and result statuses. `executed` means the plugin
// handed the commands to the game thread; it is not a guarantee that a
// particular console command had an observable effect.
const (
	DebugProtocolVersion = 1

	DebugCommandStatusExecuted = "executed"
	DebugCommandStatusFailed   = "failed"
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
// playback start/stop transition, so the desktop never mistakes a queued
// `playdemo` for proof that the engine is actually playing the Demo.
type DebugDemoState struct {
	SessionID string `json:"session_id"`
	DemoPath  string `json:"demo_path"`
	Playing   bool   `json:"playing"`
	Tick      int    `json:"tick"`
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

	ProcessStarted  bool   `json:"process_started"`
	CS2PID          int    `json:"cs2_pid"`
	PluginEnabled   bool   `json:"plugin_enabled"`
	PluginConnected bool   `json:"plugin_connected"`
	PluginReady     bool   `json:"plugin_ready"`
	PluginMessage   string `json:"plugin_message"`

	// DemoPath is the last Demo a play request targeted; DemoPlaying is the
	// plugin-observed engine playback state, never inferred from a successful
	// play receipt.
	DemoPath    string `json:"demo_path"`
	DemoPlaying bool   `json:"demo_playing"`
	DemoTick    int    `json:"demo_tick"`

	Error       string `json:"error"`
	UpdatedAtMs int64  `json:"updated_at_ms"`
}
