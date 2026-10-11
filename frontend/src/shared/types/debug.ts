import type { ClipSettings } from "./clips";

/** Debug session status values, separate from produce queue and startup enums. */
export type DebugSessionStatus =
  | "idle"
  | "starting"
  | "running"
  | "stopping"
  | "failed";

/**
 * Structured plugin transport/readiness classification of the session
 * snapshot. The empty value describes an idle snapshot before any session
 * exists; everything else is derived by the backend from its own connection
 * and handshake facts.
 */
export type DebugPluginState =
  | ""
  | "disabled"
  | "waiting_connection"
  | "waiting_ready"
  | "ready"
  | "error";

export type DebugLaunchResolution = ClipSettings["launch_resolution"];

/** Session-scoped launch options; never written back to the persisted config. */
export interface DebugLaunchRequest {
  inject_plugin: boolean;
  pov_hud_enabled: boolean;
  brand_enabled: boolean;
  pov_radar_enabled: boolean;
  launch_resolution: DebugLaunchResolution;
}

/** Console execution status reported by the plugin's game thread. */
export type DebugCommandStatus = "executed" | "failed";

/**
 * One debug console request. A custom input stays a single element so the
 * engine receives the original string, semicolons and quotes included.
 */
export interface DebugCommandRequest {
  commands: string[];
}

/** The execution receipt for one `SendDebugConsoleCommand` call. */
export interface DebugCommandResponse {
  request_id: string;
  status: DebugCommandStatus;
  error?: string;
  commands: string[];
}

/**
 * Play one managed Demo in the running debug session. `commands` are optional
 * debug-draft settings the plugin applies once the Demo can accept them; it
 * never loads a produce sequence file.
 */
export interface DebugPlayDemoRequest {
  demo_path: string;
  commands?: string[];
}

/**
 * Receipt for `PlayDebugDemo`. `executed` only proves the plugin handed
 * `playdemo` to the engine; the real playback state is `demo_playing`.
 */
export interface DebugPlayDemoResponse {
  request_id: string;
  status: DebugCommandStatus;
  error?: string;
  demo_path: string;
}

/**
 * Session-scoped debug workbench draft of the hot-updatable clip settings.
 * It is initialized from `GetClipSettings`, never saved back, and is only used
 * to request the backend-built console command plan.
 */
export interface DebugSettingsDraft {
  pov_radar_enabled: boolean;
  sky_blackout: boolean;
  disable_clouds: boolean;
  hide_all_ui: boolean;
  hide_player_avatars: boolean;
  hide_chat_messages: boolean;
  enable_spec_show_xray_zero: boolean;
  enable_voice: boolean;
  use_shoulder_camera: boolean;
  kill_feed_lifetime: number;
  block_kill_feed: boolean;
  record_fps: number;
  record_quality: ClipSettings["record_quality"];
  video_preset: ClipSettings["video_preset"];
  launch_resolution: DebugLaunchResolution;
}

/**
 * One row of the backend-built console commands. `commands` describes the
 * draft's current value; switches also carry their on/off variants so a toggle
 * click can send the exact command without rebuilding the plan.
 */
export interface DebugSettingPlan {
  key: string;
  commands: string[];
  on?: string[];
  off?: string[];
}

/**
 * The debug workbench's built command plan. `apply_all` is the deterministic
 * order behind "apply current game settings"; `recording` is the
 * next-recording initialization. The effective video fields are read-only
 * probe results for `video_preset=auto`.
 */
export interface DebugConsolePlan {
  settings: DebugSettingPlan[];
  apply_all: string[];
  recording: string[];
  effective_video_preset: string;
  effective_video_encoder: string;
  effective_video_status: string;
  record_output_dir: string;
}

/**
 * Full debug workbench snapshot. Process start, transport connection and
 * debug-protocol readiness are reported separately and must not be conflated.
 */
export interface DebugSessionState {
  session_id: string;
  status: DebugSessionStatus;

  inject_plugin: boolean;
  pov_hud_enabled: boolean;
  brand_enabled: boolean;
  pov_radar_enabled: boolean;
  launch_resolution: DebugLaunchResolution;

  process_started: boolean;
  cs2_pid: number;
  plugin_enabled: boolean;
  plugin_connected: boolean;
  plugin_ready: boolean;
  plugin_message: string;
  plugin_state: DebugPluginState;

  /** Last Demo a play request targeted, and the plugin-observed engine facts. */
  demo_path: string;
  demo_playing: boolean;
  demo_paused: boolean;
  /** A sent play request has not yet been confirmed by an engine observation. */
  demo_play_pending: boolean;

  error: string;
  updated_at_ms: number;
}
