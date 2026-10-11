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

	Error       string `json:"error"`
	UpdatedAtMs int64  `json:"updated_at_ms"`
}
