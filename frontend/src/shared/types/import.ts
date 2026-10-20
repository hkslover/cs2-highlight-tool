export type WanmeiClientStatus = "client_not_running" | "client_not_logged_in" | "ready";

export interface WanmeiMatchItem {
  match_id: string;
  download_match_id: string;
  map_name: string;
  score1: number;
  score2: number;
  kill: number;
  death: number;
  assist: number;
  k4: number;
  k5: number;
  rating: number;
  end_time: string;
}

export interface WanmeiMatchListResult {
  status: WanmeiClientStatus;
  nickname?: string;
  steam_id?: string;
  matches: WanmeiMatchItem[];
}

export interface FiveEMatchItem {
  match_id: string;
  download_match_id: string;
  map_name: string;
  score1: number;
  score2: number;
  kill: number;
  death: number;
  assist: number;
  rating: number;
  end_time: string;
}

export interface FiveEMatchListResult {
  player_name: string;
  matches: FiveEMatchItem[];
}

/**
 * Why the Steam official match list is (not) available. Mirrors
 * internal/steam.ClientStatus; never infer this from message text.
 */
export type SteamClientStatus =
  | "ready"
  | "steam_not_running"
  | "steam_not_logged_in"
  | "cs2_running"
  | "gc_unavailable"
  | "steam_api_unavailable"
  | "unsupported_platform";

export interface SteamMatchItem {
  match_id: string;
  reservation_id: string;
  share_code: string;
  map_name: string;
  game_mode: string;
  score1: number;
  score2: number;
  kill: number;
  death: number;
  assist: number;
  rating: number;
  end_time: string;
  duration_seconds: number;
  demo_available: boolean;
  player_count: number;
}

export interface SteamMatchListResult {
  status: SteamClientStatus;
  steam_id?: string;
  matches: SteamMatchItem[];
  /** Entries the Game Coordinator returned. */
  raw_match_count: number;
  /** Entries that could not be decoded. */
  skipped_match_count: number;
  /** Entries whose demo link is still available. */
  downloadable_match_count: number;
}

/** Matches ImportSteamMatchRequest in internal/app/app_steam.go. */
export interface SteamImportRequest {
  match_id?: string;
  share_code?: string;
}
