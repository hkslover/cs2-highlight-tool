import { readonly, ref } from "vue";
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
import { backend } from "@/shared/backend";
import {
  fullRoundPlayerSteamID,
  resolveFullRoundPlayerSteamID,
} from "./selection-logic";

/**
 * Selection mode controls the source roster for ordinary clips. Full-round
 * POV has its own player state below and never reuses this map.
 */
export type ClipSelectMode = "kills" | "deaths";

export interface DemoFullRoundPOVSelection {
  enabled: boolean;
  player_steam_id: string;
}

const selectedPlayerByDemoState = ref<Record<string, string>>({});
const materialByDemoState = ref<Record<string, DemoMaterialSelection[]>>({});
const fullRoundPovByDemoState = ref<Record<string, DemoFullRoundPOVSelection>>({});
const fullRoundPlanByDemoState = ref<Record<string, FullRoundPOVPlan>>({});
const fullRoundPlanErrorByDemoState = ref<Record<string, string>>({});
const clipSelectModeByDemoState = ref<Record<string, ClipSelectMode>>({});
const killFilterByDemoState = ref<Record<string, KillFilter>>({});
const autoAddVictimViewState = ref(true);

/** Read-only state exports. Mutations go through the actions in this module. */
export const selectedPlayerByDemo = readonly(selectedPlayerByDemoState);
export const materialByDemo = readonly(materialByDemoState);
export const fullRoundPovByDemo = readonly(fullRoundPovByDemoState);
export const fullRoundPlanByDemo = readonly(fullRoundPlanByDemoState);
export const fullRoundPlanErrorByDemo = readonly(fullRoundPlanErrorByDemoState);
export const clipSelectModeByDemo = readonly(clipSelectModeByDemoState);
export const killFilterByDemo = readonly(killFilterByDemoState);
export const autoAddVictimView = readonly(autoAddVictimViewState);

export function setAutoAddVictimView(enabled: boolean): void {
  autoAddVictimViewState.value = enabled;
}

export function getClipPlayers(entry: DemoListEntry | null): DemoClipPlayer[] {
  return entry?.meta?.clip_players?.slice() ?? [];
}

export function getDeathPlayers(entry: DemoListEntry | null): DemoClipPlayer[] {
  return entry?.meta?.death_players?.slice() ?? [];
}

export function getFullRoundPlayers(entry: DemoListEntry | null): DemoPlayerInfo[] {
  return entry?.meta?.players?.slice() ?? [];
}

export function getClipSelectMode(entry: DemoListEntry | null): ClipSelectMode {
  if (!entry) return "kills";
  return clipSelectModeByDemoState.value[entry.key] ?? "kills";
}

/**
 * Switching mode is an action because it may need to choose a valid player
 * from the newly active roster. The corresponding getter is deliberately pure.
 */
export function setClipSelectMode(entry: DemoListEntry | null, mode: ClipSelectMode): void {
  if (!entry) return;
  clipSelectModeByDemoState.value = {
    ...clipSelectModeByDemoState.value,
    [entry.key]: mode,
  };
  if (mode === "deaths") {
    syncDefaultDeathPlayer(entry);
  } else {
    syncDefaultPlayer(entry);
  }
}

export function getKillFilter(entry: DemoListEntry | null): KillFilter {
  if (!entry) return createDefaultKillFilter();
  const filter = killFilterByDemoState.value[entry.key] ?? createDefaultKillFilter();
  return {
    ...filter,
    weapons: filter.weapons.slice(),
    traits: filter.traits.slice(),
    hit_groups: filter.hit_groups.slice(),
    rounds: filter.rounds ? [filter.rounds[0], filter.rounds[1]] : null,
    distance: filter.distance ? [filter.distance[0], filter.distance[1]] : null,
  };
}

export function patchKillFilter(entry: DemoListEntry | null, patch: Partial<KillFilter>): void {
  if (!entry) return;
  killFilterByDemoState.value = {
    ...killFilterByDemoState.value,
    [entry.key]: { ...getKillFilter(entry), ...patch },
  };
}

export function setKillFilterRole(entry: DemoListEntry | null, role: KillPlayerRole): void {
  if (!entry) return;
  patchKillFilter(entry, { role });
  setClipSelectMode(entry, role === "victim" ? "deaths" : "kills");
}

export function resetKillFilterConditions(entry: DemoListEntry | null): void {
  if (!entry) return;
  patchKillFilter(entry, clearKillFilterConditions(getKillFilter(entry)));
}

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

/** Pure getter for the ordinary clip/filter player. */
export function getSelectedPlayerSteamID(entry: DemoListEntry | null): string {
  if (!entry) return "";
  return selectedPlayerByDemoState.value[entry.key] ?? "";
}

/** Explicit defaulting action for ordinary clip/filter selection. */
export function syncDefaultPlayerForRole(entry: DemoListEntry | null): void {
  if (!entry) return;
  if (getKillFilter(entry).role === "victim") {
    syncDefaultDeathPlayer(entry);
    return;
  }
  syncDefaultPlayer(entry);
}

export function setSelectedPlayerSteamID(entry: DemoListEntry | null, steamID: string): void {
  if (!entry) return;
  selectedPlayerByDemoState.value = {
    ...selectedPlayerByDemoState.value,
    [entry.key]: steamID.trim(),
  };
}

/**
 * `steam_id` is a JSON number and cannot safely represent a 64-bit SteamID in
 * JavaScript. Only the backend-provided text field is accepted here.
 */
export function getFullRoundPlayerSteamID(player: DemoPlayerInfo): string {
  return fullRoundPlayerSteamID(player);
}

export function getClipRounds(entry: DemoListEntry | null, playerSteamID: string): DemoClipRound[] {
  if (!entry || !playerSteamID) return [];
  return getClipPlayers(entry).find((player) => player.steam_id === playerSteamID)?.rounds ?? [];
}

export function getDeathRounds(entry: DemoListEntry | null, playerSteamID: string): DemoClipRound[] {
  if (!entry || !playerSteamID) return [];
  return getDeathPlayers(entry).find((player) => player.steam_id === playerSteamID)?.rounds ?? [];
}

export function getFullRoundPOVSelection(entry: DemoListEntry | null): DemoFullRoundPOVSelection {
  if (!entry) return { enabled: false, player_steam_id: "" };
  const selection = fullRoundPovByDemoState.value[entry.key];
  return selection
    ? { enabled: selection.enabled, player_steam_id: selection.player_steam_id }
    : { enabled: false, player_steam_id: "" };
}

/**
 * Full-round POV owns a separate player selection. Enabling it clears ordinary
 * material selections, but intentionally leaves the filter role/player alone.
 */
export function setFullRoundPOVEnabled(entry: DemoListEntry | null, enabled: boolean): void {
  if (!entry) return;
  clearFullRoundPOVPlanState(entry);
  if (!enabled) {
    fullRoundPovByDemoState.value = {
      ...fullRoundPovByDemoState.value,
      [entry.key]: { enabled: false, player_steam_id: "" },
    };
    return;
  }

  syncDefaultFullRoundPlayer(entry, getSelectedPlayerSteamID(entry));
  const playerSteamID = fullRoundPovByDemoState.value[entry.key]?.player_steam_id ?? "";
  setDemoMaterials(entry, []);
  fullRoundPovByDemoState.value = {
    ...fullRoundPovByDemoState.value,
    [entry.key]: { enabled: true, player_steam_id: playerSteamID },
  };
}

export function syncFullRoundPOVPlayer(entry: DemoListEntry | null, playerSteamID: string): void {
  if (!entry || !getFullRoundPOVSelection(entry).enabled) return;
  fullRoundPovByDemoState.value = {
    ...fullRoundPovByDemoState.value,
    [entry.key]: { enabled: true, player_steam_id: playerSteamID.trim() },
  };
  clearFullRoundPOVPlanState(entry);
}

export function clearFullRoundPOVPlanState(entry: DemoListEntry | null): void {
  if (!entry) return;
  const nextPlanCache = { ...fullRoundPlanByDemoState.value };
  delete nextPlanCache[entry.key];
  fullRoundPlanByDemoState.value = nextPlanCache;

  const nextErrorCache = { ...fullRoundPlanErrorByDemoState.value };
  delete nextErrorCache[entry.key];
  fullRoundPlanErrorByDemoState.value = nextErrorCache;
}

export function getDemoMaterials(entry: DemoListEntry | null): DemoMaterialSelection[] {
  if (!entry) return [];
  return (materialByDemoState.value[entry.key] ?? []).map((item) => ({
    ...item,
    clip_overrides: item.clip_overrides ? { ...item.clip_overrides } : undefined,
  }));
}

export function setDemoMaterials(entry: DemoListEntry | null, next: DemoMaterialSelection[]): void {
  if (!entry) return;
  const sorted = next.slice().sort((a, b) =>
    a.kill.tick === b.kill.tick ? a.kill.id.localeCompare(b.kill.id) : a.kill.tick - b.kill.tick,
  );
  materialByDemoState.value = {
    ...materialByDemoState.value,
    [entry.key]: sorted,
  };
}

export function syncDefaultPlayer(entry: DemoListEntry | null): void {
  if (!entry?.meta?.clip_players?.length) return;
  const players = entry.meta.clip_players;
  const current = getSelectedPlayerSteamID(entry);
  if (players.some((player) => player.steam_id === current)) return;
  selectedPlayerByDemoState.value = {
    ...selectedPlayerByDemoState.value,
    [entry.key]: players[0].steam_id,
  };
}

export function syncDefaultDeathPlayer(entry: DemoListEntry | null): void {
  if (!entry?.meta?.death_players?.length) return;
  const players = entry.meta.death_players;
  const current = getSelectedPlayerSteamID(entry);
  if (players.some((player) => player.steam_id === current)) return;
  selectedPlayerByDemoState.value = {
    ...selectedPlayerByDemoState.value,
    [entry.key]: players[0].steam_id,
  };
}

export function syncDefaultFullRoundPlayer(
  entry: DemoListEntry | null,
  preferredPlayerSteamID?: string,
): void {
  if (!entry?.meta?.players?.length) return;
  const current = getFullRoundPOVSelection(entry).player_steam_id;
  const playerSteamID = resolveFullRoundPlayerSteamID(
    entry.meta.players,
    preferredPlayerSteamID,
    current,
  );
  if (!playerSteamID || playerSteamID === current) return;
  fullRoundPovByDemoState.value = {
    ...fullRoundPovByDemoState.value,
    [entry.key]: {
      enabled: getFullRoundPOVSelection(entry).enabled,
      player_steam_id: playerSteamID,
    },
  };
}

/** Remove every clip/filter/POV cache for a demo, including stale plan errors. */
export function clearDemoSelectionState(demoKey: string): void {
  const nextSelected = { ...selectedPlayerByDemoState.value };
  delete nextSelected[demoKey];
  selectedPlayerByDemoState.value = nextSelected;

  const nextMaterials = { ...materialByDemoState.value };
  delete nextMaterials[demoKey];
  materialByDemoState.value = nextMaterials;

  const nextPOV = { ...fullRoundPovByDemoState.value };
  delete nextPOV[demoKey];
  fullRoundPovByDemoState.value = nextPOV;

  const nextPlans = { ...fullRoundPlanByDemoState.value };
  delete nextPlans[demoKey];
  fullRoundPlanByDemoState.value = nextPlans;

  const nextPlanErrors = { ...fullRoundPlanErrorByDemoState.value };
  delete nextPlanErrors[demoKey];
  fullRoundPlanErrorByDemoState.value = nextPlanErrors;

  const nextModes = { ...clipSelectModeByDemoState.value };
  delete nextModes[demoKey];
  clipSelectModeByDemoState.value = nextModes;

  const nextFilters = { ...killFilterByDemoState.value };
  delete nextFilters[demoKey];
  killFilterByDemoState.value = nextFilters;
}

/**
 * Requests are guarded by the current POV state. Removing a demo or changing
 * its tracked player therefore makes an in-flight response a no-op.
 */
export async function fetchFullRoundPOVPlan(
  entry: DemoListEntry | null,
  playerSteamID: string,
): Promise<void> {
  const requestedPlayerSteamID = playerSteamID.trim();
  if (!entry || !requestedPlayerSteamID) return;
  clearFullRoundPOVPlanState(entry);
  const entryKey = entry.key;
  const isCurrentRequest = (): boolean => {
    const current = getFullRoundPOVSelection(entry);
    return current.enabled && current.player_steam_id === requestedPlayerSteamID;
  };

  try {
    const plan = await backend.PreviewFullRoundPOV(entry.file_path, requestedPlayerSteamID);
    if (!isCurrentRequest()) return;
    fullRoundPlanByDemoState.value = {
      ...fullRoundPlanByDemoState.value,
      [entryKey]: plan,
    };
  } catch (err: unknown) {
    if (!isCurrentRequest()) return;
    const message = err instanceof Error ? err.message : String(err || "");
    fullRoundPlanErrorByDemoState.value = {
      ...fullRoundPlanErrorByDemoState.value,
      [entryKey]: message || t("main.clips.full_round_pov_load_failed_unknown"),
    };
  }
}

export function getFullRoundPOVTrackingLabel(entry: DemoListEntry | null): string {
  if (!entry) return "";
  const selection = getFullRoundPOVSelection(entry);
  if (!selection.enabled || !selection.player_steam_id) return "";
  const player = getFullRoundPlayers(entry).find(
    (item) => getFullRoundPlayerSteamID(item) === selection.player_steam_id,
  );
  return player?.name || selection.player_steam_id;
}

export function formatDuration(seconds: number): string {
  if (!seconds || seconds <= 0) return "-";
  const minutes = Math.floor(seconds / 60);
  const remainder = Math.floor(seconds % 60);
  return t("main.import.duration_fmt", { minutes, seconds: remainder });
}
