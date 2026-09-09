export interface ComposeProgressMessage {
  active: boolean;
  percent: number;
  current_step?: string;
  elapsed_ms?: number;
  error?: string;
}

export interface ProduceHistoryItem {
  demo_path: string;
  take_index: number;
  take_name?: string;
  view: string;
  spec_mode: number;
  kill_ids: string[];
  kills?: DemoClipKill[];
  source_id?: string;
  round?: number;
  player_name?: string;
  player_steam_id?: string;
  start_tick?: number;
  end_tick?: number;
  end_reason?: string;
  tick_rate?: number;
  record_start_tick?: number;
  record_end_tick?: number;
  kill_offsets_seconds?: number[];
  video_path: string;
  history_type?: "produce_clip" | "edited_video";
  source_label?: string;
  completed_at_ms: number;
}

export interface ProduceHistorySnapshot {
  items: ProduceHistoryItem[];
  updated_at_ms: number;
}

import type { DemoClipKill } from "./demo";

export interface ProduceHistoryExportResult {
  cancelled: boolean;
  target_dir?: string;
  total: number;
  moved: number;
  failed: number;
  errors?: string[];
}
