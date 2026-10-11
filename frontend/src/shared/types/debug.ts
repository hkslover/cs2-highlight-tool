import type { ClipSettings } from "./clips";

/** Debug session status values, separate from produce queue and startup enums. */
export type DebugSessionStatus =
  | "idle"
  | "starting"
  | "running"
  | "stopping"
  | "failed";

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

  error: string;
  updated_at_ms: number;
}
