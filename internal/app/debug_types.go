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
