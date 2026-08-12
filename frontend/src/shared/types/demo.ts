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
  /**
   * Flat, chronological list of every kill in the demo, and the source the clip
   * filter runs over — a filter can constrain both sides of a kill at once,
   * which the pre-grouped views below cannot express. Optional because demos
   * parsed by an older build have no such field.
   */
  kills?: DemoClipKill[];
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

  /*
   * Filter facts. All optional: clips carried over from a demo parsed by an
   * older build lack them, and a missing field must read as "not special"
   * rather than break the predicate.
   */

  /** Weapon family. Snipers and machine guns are their own class, not rifles/heavies. */
  weapon_class?: DemoWeaponClass;
  /** Surfaces penetrated — the count behind is_wallbang. */
  penetrated_objects?: number;
  /** Killer-to-victim distance, in metres. */
  distance?: number;
  /** Fatal hit group; 0 when the damage carried none (grenades, fire). */
  hit_group?: DemoHitGroup;

  is_noscope?: boolean;
  is_through_smoke?: boolean;
  is_attacker_blind?: boolean;
  is_assisted_flash?: boolean;
  /** Covers suicides too, since those also land on the killer's own team. */
  is_team_kill?: boolean;
  has_assist?: boolean;
  assister_name?: string;
  assister_steam_id?: string;

  /** Killer and victim state at the tick the kill landed. */
  killer_health?: number;
  killer_armor?: number;
  killer_airborne?: boolean;
  killer_ducking?: boolean;
  killer_scoped?: boolean;
  victim_blinded?: boolean;

  /** First kill of the round, by any player. */
  is_opening_kill?: boolean;
  /**
   * Frags the killer finished this round with, so a 3K filter keeps all three
   * of its kills. Team kills do not count and carry 0.
   */
  killer_round_kills?: number;
  /** Kill made with no living teammate left, including the one that closes it out. */
  is_clutch_kill?: boolean;
  /** Whether the bomb was down when the kill landed. */
  bomb_planted?: boolean;
}

/** Mirrors the WeaponClass* constants in internal/demo/killfacts.go. */
export type DemoWeaponClass =
  | "pistol"
  | "smg"
  | "rifle"
  | "sniper"
  | "shotgun"
  | "machinegun"
  | "grenade"
  | "knife"
  | "zeus"
  | "other";

/** Mirrors demoinfocs events.HitGroup. */
export type DemoHitGroup = 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8;

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
