export interface ClipSettings {
  killer_pre_seconds: number;
  killer_post_seconds: number;
  victim_pre_seconds: number;
  victim_post_seconds: number;
  auto_add_victim_view: boolean;
  enable_voice: boolean;
  record_fps: number;
  record_quality: "standard" | "high" | "ultra";
  edit_fps: number;
  edit_quality: "standard" | "high" | "ultra";
  video_preset: "auto" | "c1" | "n1" | "a1" | "i1";
  launch_resolution: "16:9" | "4:3" | "4:3_1280x960";
  record_output_dir: string;
  enable_spec_show_xray_zero: boolean;
  hide_all_ui: boolean;
  use_shoulder_camera: boolean;
  pov_hud_enabled: boolean;
  sky_blackout: boolean;
  disable_clouds: boolean;
  kill_feed_lifetime: number;
  block_kill_feed: boolean;
}

export interface OutputsStorageStats {
  output_dir: string;
  video_count: number;
  total_size_bytes: number;
}

export interface DemoStorageStats {
  demo_dir: string;
  demo_count: number;
  total_size_bytes: number;
}

export interface ClipParameterOverrides {
  killer_pre_seconds?: number;
  killer_post_seconds?: number;
  victim_pre_seconds?: number;
  victim_post_seconds?: number;
  enable_voice?: boolean;
  enable_spec_show_xray_zero?: boolean;
}

export interface GeneratePluginSelectedItem {
  kill: DemoClipKill;
  include_killer?: boolean;
  include_victim: boolean;
  killer_spec_mode: number;
  victim_spec_mode: number;
  clip_overrides?: ClipParameterOverrides;
}

export interface GeneratePluginJSONRequest {
  demo_path: string;
  tick_rate: number;
  selected_items: GeneratePluginSelectedItem[];
  full_round_pov?: {
    player_steam_id: string;
  };
  extra_commands?: string[];
}

export interface GeneratePluginJSONResult {
  json_path: string;
  sequence_count: number;
  segment_count: number;
  action_count: number;
  take_plans?: ProduceTakePlan[];
}

export interface GeneratePluginJSONBatchRequest {
  jobs: GeneratePluginJSONRequest[];
  debug?: {
    keep_intermediate_files?: boolean;
  };
}

export interface GeneratePluginJSONBatchItemResult {
  demo_path: string;
  json_path?: string;
  sequence_count?: number;
  segment_count?: number;
  action_count?: number;
  take_plans?: ProduceTakePlan[];
  generated_take_count?: number;
  skipped_by_history?: boolean;
  skipped_reason?: string;
  error?: string;
}

export interface GeneratePluginJSONBatchResult {
  results: GeneratePluginJSONBatchItemResult[];
  success_count: number;
  failure_count: number;
  batch_timestamp?: string;
  launch_started?: boolean;
  launched_demo_path?: string;
  launch_error?: string;
}

export interface FullRoundPOVSegment {
  round: number;
  start_tick: number;
  freeze_end_tick: number;
  round_end_tick: number;
  official_end_tick?: number;
  next_round_start_tick?: number;
  death_tick?: number;
  record_start_tick: number;
  record_end_tick: number;
  target_slot: number;
  end_reason: string;
}

export interface FullRoundPOVPlan {
  player_name: string;
  player_steam_id: string;
  segments: FullRoundPOVSegment[];
}

export interface ProduceTakePlan {
  demo_path: string;
  take_index: number;
  take_name?: string;
  view: string;
  spec_mode: number;
  kill_ids: string[];
  source_id?: string;
  round?: number;
  player_name?: string;
  player_steam_id?: string;
  start_tick?: number;
  end_tick?: number;
  end_reason?: string;
}

export interface ProduceTakeFile {
  demo_path: string;
  take_index: number;
  take_name?: string;
  view: string;
  video_path?: string;
  audio_path?: string;
  status: string;
  error?: string;
  updated_at_ms: number;
}

export interface ProduceTakeFileSnapshot {
  items: ProduceTakeFile[];
  updated_at_ms: number;
}

export interface ProduceQueueState {
  running: boolean;
  total: number;
  completed: number;
  current_index: number;
  current_demo_path?: string;
  pending_ack: boolean;
  last_error?: string;
  demos?: string[];
  updated_at_ms: number;
}

export interface ProduceWSState {
  address: string;
  connected: boolean;
  last_error?: string;
  updated_at_ms: number;
}

export interface ProduceTakeStatus {
  demo_path?: string;
  take_index?: number;
  take_name?: string;
  record_phase?: string;
  status: string;
  tick?: number;
  cmd?: string;
  ts_ms: number;
}

export interface PlatformClientStatus {
  exe_name: string;
  display_name: string;
  running: boolean;
  pid: number;
}

export interface PlatformClientCloseResult {
  exe_name: string;
  closed: boolean;
  error?: string;
}

import type { DemoClipKill } from "./demo";

export interface ProduceTakeStatusSnapshot {
  items: ProduceTakeStatus[];
  total_takes: number;
  started_takes: number;
  completed_takes: number;
  last_event?: ProduceTakeStatus;
  updated_at_ms: number;
}
