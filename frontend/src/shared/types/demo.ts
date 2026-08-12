export interface DemoMetadata {
  file_path: string;
  file_name: string;
  map_name: string;
  server_name: string;
  duration: number;
  tick_rate: number;
  total_rounds: number;
  overtime_count: number;
  score_ct: number;
  score_t: number;
  clan_name_ct: string;
  clan_name_t: string;
  players: DemoPlayerInfo[];
  clip_players: DemoClipPlayer[];
  /** Same kills as clip_players, grouped by victim instead of by killer. */
  death_players?: DemoClipPlayer[];
  match_end_tick?: number;
}

export interface DemoPlayerInfo {
  name: string;
  steam_id: number;
  steam_id_text?: string;
  kills: number;
  deaths: number;
  assists: number;
}

export interface DemoClipPlayer {
  name: string;
  steam_id: string;
  /** Clip count for clip_players entries. */
  total_kills: number;
  /** Clip count for death_players entries. */
  total_deaths?: number;
  rounds: DemoClipRound[];
}

export interface DemoClipRound {
  round: number;
  kills: DemoClipKill[];
}

export interface DemoClipKill {
  id: string;
  round: number;
  tick: number;
  map_name: string;
  killer_name: string;
  killer_steam_id: string;
  killer_slot: number;
  killer_entity_id: number;
  killer_side: string;
  victim_name: string;
  victim_steam_id: string;
  victim_slot: number;
  victim_entity_id: number;
  victim_side: string;
  weapon_name: string;
  is_headshot: boolean;
  is_wallbang: boolean;
}

export interface DemoListEntry {
  key: string;
  file_path: string;
  file_name: string;
  loading: boolean;
  error?: string;
  meta?: DemoMetadata;
}

import type { ClipParameterOverrides } from "./clips";

/**
 * Which side of the kill event is the player the user picked on the clip page.
 * "killer" for a kill clip, "victim" for a death clip. It decides which pass is
 * shown as 主视角 and which as 对方视角, and which one gets the longer window —
 * the passes themselves stay bound to their roles.
 */
export type ClipPrimaryView = "killer" | "victim";

export interface DemoMaterialSelection {
  kill: DemoClipKill;
  include_killer?: boolean;
  include_victim: boolean;
  killer_spec_mode: number;
  victim_spec_mode: number;
  primary_view?: ClipPrimaryView;
  clip_overrides?: ClipParameterOverrides;
}
