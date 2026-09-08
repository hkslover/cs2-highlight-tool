/**
 * Resolve the textual identity used by full-round POV selection.
 *
 * The parser also exposes a numeric `steam_id` for the scoreboard. That value
 * is unsafe as a JavaScript number for a 64-bit SteamID, so this boundary must
 * only accept the lossless text field.
 */
export interface FullRoundPlayerIdentity {
  steam_id_text?: string | null;
}

export function fullRoundPlayerSteamID(player: FullRoundPlayerIdentity): string {
  return typeof player.steam_id_text === "string" ? player.steam_id_text.trim() : "";
}

/**
 * Keep a still-valid current POV player across ordinary roster changes. A
 * preferred ID is used only during an explicit selection/defaulting action;
 * otherwise the current POV identity wins and a valid first player is the
 * final fallback.
 */
export function resolveFullRoundPlayerSteamID(
  players: readonly FullRoundPlayerIdentity[],
  preferredPlayerSteamID?: string,
  currentPlayerSteamID?: string,
): string {
  const playerIDs = players
    .map(fullRoundPlayerSteamID)
    .filter((steamID): steamID is string => !!steamID);
  const preferred = preferredPlayerSteamID?.trim() || "";
  if (preferred && playerIDs.includes(preferred)) return preferred;

  const current = currentPlayerSteamID?.trim() || "";
  if (current && playerIDs.includes(current)) return current;

  return playerIDs[0] || "";
}
