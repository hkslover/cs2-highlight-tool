import { ref } from "vue";
import { t } from "@/shared/i18n";
import type {
  DemoClipKill,
  DemoClipPlayer,
  DemoClipRound,
  DemoListEntry,
  DemoMaterialSelection,
  DemoMetadata,
  DemoPlayerInfo,
  FullRoundPOVPlan,
} from "@/shared/types";
import {
  clearKillFilterConditions,
  collectDemoKills,
  createDefaultKillFilter,
  filterKills,
  groupKillsByRound,
  type KillFilter,
  type KillPlayerRole,
} from "@/shared/kill-filter";

export const selectedPlayerByDemo = ref<Record<string, string>>({});
export const materialByDemo = ref<Record<string, DemoMaterialSelection[]>>({});
export const fullRoundPovByDemo = ref<Record<string, DemoFullRoundPOVSelection>>({});
export const fullRoundPlanByDemo = ref<Record<string, FullRoundPOVPlan>>({});
export const fullRoundPlanErrorByDemo = ref<Record<string, string>>({});
export const clipSelectModeByDemo = ref<Record<string, ClipSelectMode>>({});
export const killFilterByDemo = ref<Record<string, KillFilter>>({});

/**
 * Which set of clips the selection list offers for the selected player:
 * "kills" (the player killed someone) or "deaths" (the player got killed).
 * Both record the selected player's own first-person view — death mode just
 * makes them the victim of the event instead of the killer.
 */
export type ClipSelectMode = "kills" | "deaths";

export interface DemoFullRoundPOVSelection {
  enabled: boolean;
  player_steam_id: string;
}

export function useDemoData() {
  return {
    selectedPlayerByDemo,
    materialByDemo,
    fullRoundPovByDemo,
  };
}

export function getClipPlayers(entry: DemoListEntry | null): DemoClipPlayer[] {
  if (!entry?.meta?.clip_players) return [];
  return entry.meta.clip_players;
}

export function getDeathPlayers(entry: DemoListEntry | null): DemoClipPlayer[] {
  if (!entry?.meta?.death_players) return [];
  return entry.meta.death_players;
}

export function getClipSelectMode(entry: DemoListEntry | null): ClipSelectMode {
  if (!entry) return "kills";
  return clipSelectModeByDemo.value[entry.key] ?? "kills";
}

export function setClipSelectMode(entry: DemoListEntry | null, mode: ClipSelectMode) {
  if (!entry) return;
  clipSelectModeByDemo.value = {
    ...clipSelectModeByDemo.value,
    [entry.key]: mode,
  };
  // The two modes offer different player sets (killers vs. victims), so the
  // currently selected player may not exist in the one we just switched to.
  if (mode === "deaths") {
    syncDefaultDeathPlayer(entry);
  } else {
    syncDefaultPlayer(entry);
  }
}

export function getKillFilter(entry: DemoListEntry | null): KillFilter {
  if (!entry) return createDefaultKillFilter();
  return killFilterByDemo.value[entry.key] ?? createDefaultKillFilter();
}

export function patchKillFilter(entry: DemoListEntry | null, patch: Partial<KillFilter>) {
  if (!entry) return;
  killFilterByDemo.value = {
    ...killFilterByDemo.value,
    [entry.key]: { ...getKillFilter(entry), ...patch },
  };
}

/**
 * Switches which side of a kill the selected player must be on.
 *
 * This is what the death-mode switch used to do, so ClipSelectMode is kept
 * mirroring the role: it still drives the default-player sync and the primary
 * view of newly added clips.
 */
export function setKillFilterRole(entry: DemoListEntry | null, role: KillPlayerRole) {
  if (!entry) return;
  patchKillFilter(entry, { role });
  setClipSelectMode(entry, role === "victim" ? "deaths" : "kills");
}

export function resetKillFilterConditions(entry: DemoListEntry | null) {
  if (!entry) return;
  patchKillFilter(entry, clearKillFilterConditions(getKillFilter(entry)));
}

/** Every kill in the demo, before filtering — the denominator of "23 / 97". */
export function getAllDemoKills(entry: DemoListEntry | null): DemoClipKill[] {
  return collectDemoKills(entry?.meta);
}

export function getFilteredKills(entry: DemoListEntry | null): DemoClipKill[] {
  if (!entry) return [];
  return filterKills(getAllDemoKills(entry), getKillFilter(entry), getSelectedPlayerSteamID(entry));
}

export function getFilteredRounds(entry: DemoListEntry | null): DemoClipRound[] {
  return groupKillsByRound(getFilteredKills(entry));
}

export function getFullRoundPlayers(entry: DemoListEntry | null): DemoPlayerInfo[] {
  if (!entry?.meta?.players) return [];
  return entry.meta.players;
}

export function getSelectedPlayerSteamID(entry: DemoListEntry | null): string {
  if (!entry) return "";
  syncDefaultPlayerForRole(entry);
  return selectedPlayerByDemo.value[entry.key] ?? "";
}

/**
 * Re-points the selection at a valid player whenever the current one is absent
 * from the roster the active role offers: killers for "killer", victims for
 * "victim".
 */
export function syncDefaultPlayerForRole(entry: DemoListEntry | null): void {
  if (!entry) return;
  if (getKillFilter(entry).role === "victim") {
    syncDefaultDeathPlayer(entry);
    return;
  }
  syncDefaultPlayer(entry);
}

export function setSelectedPlayerSteamID(entry: DemoListEntry | null, steamID: string) {
  if (!entry) return;
  selectedPlayerByDemo.value = {
    ...selectedPlayerByDemo.value,
    [entry.key]: steamID,
  };
}

export function getFullRoundPlayerSteamID(player: DemoPlayerInfo): string {
  const explicit = String(player.steam_id_text || "").trim();
  if (explicit) return explicit;
  return String(player.steam_id || "").trim();
}

export function getClipRounds(entry: DemoListEntry | null, playerSteamID: string): DemoClipRound[] {
  if (!entry || !playerSteamID) return [];
  const players = getClipPlayers(entry);
  const player = players.find((item) => item.steam_id === playerSteamID);
  return player?.rounds ?? [];
}

export function getDeathRounds(entry: DemoListEntry | null, playerSteamID: string): DemoClipRound[] {
  if (!entry || !playerSteamID) return [];
  const player = getDeathPlayers(entry).find((item) => item.steam_id === playerSteamID);
  return player?.rounds ?? [];
}

export function getFullRoundPOVSelection(entry: DemoListEntry | null): DemoFullRoundPOVSelection {
  if (!entry) return { enabled: false, player_steam_id: "" };
  return fullRoundPovByDemo.value[entry.key] ?? { enabled: false, player_steam_id: "" };
}

export function setFullRoundPOVEnabled(entry: DemoListEntry | null, enabled: boolean) {
  if (!entry) return;
  clearFullRoundPOVPlanState(entry);
  if (enabled) {
    // Full-round POV already covers the tracked player's whole round including
    // the moment they die, so death mode has nothing left to add. Reset the
    // flag without re-defaulting the player — syncDefaultFullRoundPlayer below
    // picks from the wider meta.players list and may well keep the current one.
    // The filter role is reset alongside it: it now drives the same default
    // sync, so leaving it on "victim" would validate the tracked player against
    // the victim list instead of the scoreboard.
    clipSelectModeByDemo.value = {
      ...clipSelectModeByDemo.value,
      [entry.key]: "kills",
    };
    patchKillFilter(entry, { role: "killer" });
    syncDefaultFullRoundPlayer(entry);
    const playerSteamID = selectedPlayerByDemo.value[entry.key] || "";
    setDemoMaterials(entry, []);
    fullRoundPovByDemo.value = {
      ...fullRoundPovByDemo.value,
      [entry.key]: { enabled: true, player_steam_id: playerSteamID },
    };
    return;
  }
  fullRoundPovByDemo.value = {
    ...fullRoundPovByDemo.value,
    [entry.key]: { enabled: false, player_steam_id: "" },
  };
}

export function syncFullRoundPOVPlayer(entry: DemoListEntry | null, playerSteamID: string) {
  if (!entry) return;
  const current = getFullRoundPOVSelection(entry);
  if (!current.enabled) return;
  fullRoundPovByDemo.value = {
    ...fullRoundPovByDemo.value,
    [entry.key]: { enabled: true, player_steam_id: String(playerSteamID || "").trim() },
  };
  clearFullRoundPOVPlanState(entry);
}

export function clearFullRoundPOVPlanState(entry: DemoListEntry | null) {
  if (!entry) return;
  const nextPlanCache = { ...fullRoundPlanByDemo.value };
  delete nextPlanCache[entry.key];
  fullRoundPlanByDemo.value = nextPlanCache;

  const nextErrorCache = { ...fullRoundPlanErrorByDemo.value };
  delete nextErrorCache[entry.key];
  fullRoundPlanErrorByDemo.value = nextErrorCache;
}

export function getDemoMaterials(entry: DemoListEntry | null): DemoMaterialSelection[] {
  if (!entry) return [];
  const current = materialByDemo.value[entry.key];
  if (current) return current;
  materialByDemo.value = {
    ...materialByDemo.value,
    [entry.key]: [],
  };
  return materialByDemo.value[entry.key] || [];
}

export function setDemoMaterials(entry: DemoListEntry | null, next: DemoMaterialSelection[]) {
  if (!entry) return;
  const sorted = next
    .slice()
    .sort((a, b) =>
      a.kill.tick === b.kill.tick ? a.kill.id.localeCompare(b.kill.id) : a.kill.tick - b.kill.tick,
    );
  materialByDemo.value = {
    ...materialByDemo.value,
    [entry.key]: sorted,
  };
}

export function syncDefaultPlayer(entry: DemoListEntry | null): void {
  if (!entry?.meta?.clip_players?.length) return;
  const players = entry.meta.clip_players;
  const current = selectedPlayerByDemo.value[entry.key];
  const exists = players.some((player) => player.steam_id === current);
  if (exists) return;
  selectedPlayerByDemo.value = {
    ...selectedPlayerByDemo.value,
    [entry.key]: players[0].steam_id,
  };
}

export function syncDefaultDeathPlayer(entry: DemoListEntry | null): void {
  if (!entry?.meta?.death_players?.length) return;
  const players = entry.meta.death_players;
  const current = selectedPlayerByDemo.value[entry.key];
  if (players.some((player) => player.steam_id === current)) return;
  selectedPlayerByDemo.value = {
    ...selectedPlayerByDemo.value,
    [entry.key]: players[0].steam_id,
  };
}

export async function fetchFullRoundPOVPlan(entry: DemoListEntry | null, playerSteamID: string): Promise<void> {
  const requestedPlayerSteamID = String(playerSteamID || "").trim();
  if (!entry || !requestedPlayerSteamID) return;
  clearFullRoundPOVPlanState(entry);
  const entryKey = entry.key;
  const isCurrentRequest = () => {
    const current = getFullRoundPOVSelection(entry);
    return current.enabled && current.player_steam_id === requestedPlayerSteamID;
  };
  try {
    const plan = await callBackend<FullRoundPOVPlan>(
      "PreviewFullRoundPOV",
      entry.file_path,
      requestedPlayerSteamID,
    );
    if (!isCurrentRequest()) return;
    fullRoundPlanByDemo.value = {
      ...fullRoundPlanByDemo.value,
      [entryKey]: plan,
    };
  } catch (err: unknown) {
    if (!isCurrentRequest()) return;
    const message = err instanceof Error ? err.message : String(err || "");
    fullRoundPlanErrorByDemo.value = {
      ...fullRoundPlanErrorByDemo.value,
      [entryKey]: message || t("main.clips.full_round_pov_load_failed_unknown"),
    };
  }
}

export function getFullRoundPOVTrackingLabel(entry: DemoListEntry | null): string {
  if (!entry) return "";
  const sel = getFullRoundPOVSelection(entry);
  if (!sel.enabled || !sel.player_steam_id) return "";
  const players = getFullRoundPlayers(entry);
  const player = players.find((p) => getFullRoundPlayerSteamID(p) === sel.player_steam_id);
  return player?.name || sel.player_steam_id;
}

export function syncDefaultFullRoundPlayer(entry: DemoListEntry | null): void {
  if (!entry?.meta?.players?.length) return;
  const players = entry.meta.players;
  const current = selectedPlayerByDemo.value[entry.key];
  const exists = players.some((player) => getFullRoundPlayerSteamID(player) === current);
  if (exists) return;
  selectedPlayerByDemo.value = {
    ...selectedPlayerByDemo.value,
    [entry.key]: getFullRoundPlayerSteamID(players[0]),
  };
}

export function formatDuration(seconds: number): string {
  if (!seconds || seconds <= 0) return "-";
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return t("main.import.duration_fmt", { minutes: m, seconds: s });
}

export async function callBackend<T>(method: string, ...args: unknown[]): Promise<T> {
  const api = (window as any).go?.app?.App as
    | Record<string, (...a: unknown[]) => Promise<unknown>>
    | undefined;
  const fn = api?.[method];
  if (!fn) throw new Error(`Wails API not loaded: ${method}`);
  return fn(...args) as Promise<T>;
}
