export type GameInfoHealthStatus = "ok" | "needs_repair" | "unknown";

export interface GameInfoHealth {
  status: GameInfoHealthStatus;
  needs_repair: boolean;
  gameinfo_path: string;
  message: string;
  error: string;
}

export interface DebugPluginDLLOverrideState {
  active: boolean;
  path: string;
}
