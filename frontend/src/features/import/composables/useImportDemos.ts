import { computed, ref } from "vue";
import type {
  ClipParameterOverrides,
  ClipPrimaryView,
  DemoClipKill,
  DemoListEntry,
  DemoMaterialSelection,
  DemoMetadata,
  GeneratePluginJSONRequest,
} from "@/shared/types";
import {
  selectedPlayerByDemo,
  materialByDemo,
  fullRoundPovByDemo,
  fullRoundPlanByDemo,
  fullRoundPlanErrorByDemo,
  clipSelectModeByDemo,
  getClipPlayers,
  getDeathPlayers,
  getClipSelectMode,
  setClipSelectMode,
  getFullRoundPlayers,
  getSelectedPlayerSteamID,
  setSelectedPlayerSteamID,
  getFullRoundPlayerSteamID,
  getClipRounds,
  getDeathRounds,
  getFullRoundPOVSelection,
  setFullRoundPOVEnabled,
  syncFullRoundPOVPlayer,
  getDemoMaterials,
  setDemoMaterials,
  syncDefaultPlayer,
  syncDefaultDeathPlayer,
  syncDefaultFullRoundPlayer,
  fetchFullRoundPOVPlan,
  getFullRoundPOVTrackingLabel,
  formatDuration,
  callBackend,
} from "./useDemoData";
import { normalizeClipOverrides, basename } from "./demo-helpers";

const demoList = ref<DemoListEntry[]>([]);
const selectedIndex = ref<number | null>(null);
const detailCollapsed = ref(true);
const autoAddVictimView = ref(true);

let keyCounter = 0;

const selectedEntry = computed<DemoListEntry | null>(() => {
  const idx = selectedIndex.value;
  if (idx == null || idx < 0 || idx >= demoList.value.length) return null;
  return demoList.value[idx];
});

const selectedDemo = computed<DemoMetadata | null>(() => selectedEntry.value?.meta ?? null);
const canSelectPrev = computed(() => selectedIndex.value != null && selectedIndex.value > 0);
const canSelectNext = computed(
  () => selectedIndex.value != null && selectedIndex.value < demoList.value.length - 1,
);
const clipReadyDemos = computed(() =>
  demoList.value.filter(
    (entry) => (entry.meta?.clip_players?.length ?? 0) > 0 || (entry.meta?.players?.length ?? 0) > 0,
  ),
);

function onDemosSelected(paths: string[]) {
  const newEntries: DemoListEntry[] = [];
  for (const p of paths) {
    if (demoList.value.find((d) => d.file_path === p)) continue;
    newEntries.push({
      key: `demo-${++keyCounter}`,
      file_path: p,
      file_name: basename(p),
      loading: true,
    });
  }
  if (newEntries.length === 0) return;

  const firstNewIndex = demoList.value.length;
  demoList.value.push(...newEntries);
  if (selectedIndex.value == null) {
    selectedIndex.value = firstNewIndex;
  }

  for (const entry of newEntries) {
    callBackend("ParseDemoFile", entry.file_path)
      .then((meta) => {
        const idx = demoList.value.findIndex((d) => d.key === entry.key);
        if (idx < 0) return;
        const nextEntry = {
          ...entry,
          loading: false,
          meta: (meta as DemoMetadata) ?? undefined,
        };
        demoList.value[idx] = nextEntry;
        syncDefaultPlayer(nextEntry);
      })
      .catch((err: unknown) => {
        const idx = demoList.value.findIndex((d) => d.key === entry.key);
        if (idx < 0) return;
        const message = err instanceof Error ? err.message : String(err);
        demoList.value[idx] = { ...entry, loading: false, error: message };
      });
  }
}

function removeDemoAt(index: number) {
  const removed = demoList.value[index];
  demoList.value.splice(index, 1);

  if (removed) {
    const nextPlayers = { ...selectedPlayerByDemo.value };
    delete nextPlayers[removed.key];
    selectedPlayerByDemo.value = nextPlayers;

    const nextMaterials = { ...materialByDemo.value };
    delete nextMaterials[removed.key];
    materialByDemo.value = nextMaterials;

    const nextFullRound = { ...fullRoundPovByDemo.value };
    delete nextFullRound[removed.key];
    fullRoundPovByDemo.value = nextFullRound;

    const nextFullRoundPlan = { ...fullRoundPlanByDemo.value };
    delete nextFullRoundPlan[removed.key];
    fullRoundPlanByDemo.value = nextFullRoundPlan;

    const nextFullRoundPlanError = { ...fullRoundPlanErrorByDemo.value };
    delete nextFullRoundPlanError[removed.key];
    fullRoundPlanErrorByDemo.value = nextFullRoundPlanError;

    const nextClipSelectMode = { ...clipSelectModeByDemo.value };
    delete nextClipSelectMode[removed.key];
    clipSelectModeByDemo.value = nextClipSelectMode;
  }

  if (selectedIndex.value === index) {
    selectedIndex.value =
      demoList.value.length > 0 ? Math.min(index, demoList.value.length - 1) : null;
  } else if (selectedIndex.value != null && selectedIndex.value > index) {
    selectedIndex.value--;
  }
}

function toggleSelected(index: number) {
  selectedIndex.value = selectedIndex.value === index ? null : index;
}

function selectPrevDemo() {
  if (!canSelectPrev.value || selectedIndex.value == null) return;
  selectedIndex.value--;
}

function selectNextDemo() {
  if (!canSelectNext.value || selectedIndex.value == null) return;
  selectedIndex.value++;
}

function toggleDetailCollapsed() {
  detailCollapsed.value = !detailCollapsed.value;
}

function selectDemoByKey(key: string) {
  const idx = demoList.value.findIndex((entry) => entry.key === key);
  if (idx >= 0) {
    selectedIndex.value = idx;
  }
}

function syncDefaultPlayerForMode(entry: DemoListEntry) {
  if (getClipSelectMode(entry) === "deaths" && (entry.meta?.death_players?.length ?? 0) > 0) {
    syncDefaultDeathPlayer(entry);
    return;
  }
  if ((entry.meta?.clip_players?.length ?? 0) > 0) {
    syncDefaultPlayer(entry);
    return;
  }
  syncDefaultFullRoundPlayer(entry);
}

function ensureClipDemoSelected(): DemoListEntry | null {
  const current = selectedEntry.value;
  if (current && ((current.meta?.clip_players?.length ?? 0) > 0 || (current.meta?.players?.length ?? 0) > 0)) {
    syncDefaultPlayerForMode(current);
    return current;
  }
  const fallback = clipReadyDemos.value[0] ?? null;
  if (!fallback) return null;
  selectDemoByKey(fallback.key);
  syncDefaultPlayerForMode(fallback);
  return fallback;
}

function addMaterialSelection(
  entry: DemoListEntry | null,
  kill: DemoClipKill,
  includeVictim: boolean,
  includeKiller = true,
  primaryView: ClipPrimaryView = "killer",
) {
  if (!entry || !kill?.id) return;
  const current = getDemoMaterials(entry);
  const existingIndex = current.findIndex((item) => item.kill.id === kill.id);
  if (existingIndex >= 0) {
    // The same kill event can be reached from both directions (as the killer's
    // kill and as the victim's death). Merging the passes is right, but the
    // primary_view already recorded stays — it is what the existing per-item
    // window overrides were entered against.
    const next = current.slice();
    next[existingIndex] = {
      ...next[existingIndex],
      include_victim: next[existingIndex].include_victim || includeVictim,
      include_killer: next[existingIndex].include_killer !== false || includeKiller,
      killer_spec_mode: 1,
      victim_spec_mode: 1,
    };
    setDemoMaterials(entry, next);
    return;
  }
  const next = current.concat({
    kill,
    include_killer: includeKiller,
    include_victim: includeVictim,
    killer_spec_mode: 1,
    victim_spec_mode: 1,
    primary_view: primaryView,
  });
  setDemoMaterials(entry, next);
}

function updateMaterialSpecModes(
  entry: DemoListEntry | null,
  killID: string,
  _patch: Partial<Pick<DemoMaterialSelection, "killer_spec_mode" | "victim_spec_mode">>,
) {
  if (!entry || !killID) return;
  const current = getDemoMaterials(entry);
  const idx = current.findIndex((item) => item.kill.id === killID);
  if (idx < 0) return;
  const next = current.slice();
  next[idx] = {
    ...next[idx],
    killer_spec_mode: 1,
    victim_spec_mode: 1,
  };
  setDemoMaterials(entry, next);
}

function updateMaterialClipOverrides(
  entry: DemoListEntry | null,
  killID: string,
  patch: Partial<ClipParameterOverrides>,
) {
  if (!entry || !killID) return;
  const current = getDemoMaterials(entry);
  const idx = current.findIndex((item) => item.kill.id === killID);
  if (idx < 0) return;
  const next = current.slice();
  const merged = {
    ...(next[idx].clip_overrides || {}),
    ...patch,
  };
  next[idx] = {
    ...next[idx],
    clip_overrides: normalizeClipOverrides(merged),
  };
  setDemoMaterials(entry, next);
}

function updateMaterialIncludeVictim(
  entry: DemoListEntry | null,
  killID: string,
  includeVictim: boolean,
) {
  if (!entry || !killID) return;
  const current = getDemoMaterials(entry);
  const idx = current.findIndex((item) => item.kill.id === killID);
  if (idx < 0) return;
  const next = current.slice();
  next[idx] = {
    ...next[idx],
    include_victim: includeVictim,
  };
  setDemoMaterials(entry, next);
}

function updateMaterialIncludeKiller(
  entry: DemoListEntry | null,
  killID: string,
  includeKiller: boolean,
) {
  if (!entry || !killID) return;
  const current = getDemoMaterials(entry);
  const idx = current.findIndex((item) => item.kill.id === killID);
  if (idx < 0) return;
  const next = current.slice();
  next[idx] = {
    ...next[idx],
    include_killer: includeKiller,
  };
  setDemoMaterials(entry, next);
}

function removeMaterialSelection(entry: DemoListEntry | null, killID: string) {
  if (!entry || !killID) return;
  const current = getDemoMaterials(entry);
  const next = current.filter((item) => item.kill.id !== killID);
  setDemoMaterials(entry, next);
}

function isKillSelectedInDemo(entry: DemoListEntry | null, killID: string): boolean {
  if (!entry || !killID) return false;
  const current = getDemoMaterials(entry);
  return current.some((item) => item.kill.id === killID);
}

function getMaterialSelections(entry: DemoListEntry | null): DemoMaterialSelection[] {
  return getDemoMaterials(entry);
}

function getMaterialSelectionCount(entry: DemoListEntry | null): number {
  return getDemoMaterials(entry).length;
}

function clearMaterialSelections(entry: DemoListEntry | null) {
  if (!entry) return;
  setDemoMaterials(entry, []);
}

function buildBatchJobs(): GeneratePluginJSONRequest[] {
  return clipReadyDemos.value
    .map((entry) => {
      const fullRound = getFullRoundPOVSelection(entry);
      const plan = fullRoundPlanByDemo.value[entry.key];
      const hasPOVSegments = !!(
        fullRound.enabled &&
        fullRound.player_steam_id &&
        plan &&
        (plan.segments?.length ?? 0) > 0
      );
      return {
        demo_path: entry.file_path,
        tick_rate: entry.meta?.tick_rate ?? 64,
        selected_items: getDemoMaterials(entry).map((item) => ({
          kill: item.kill,
          include_killer: item.include_killer,
          include_victim: item.include_victim,
          killer_spec_mode: 1,
          victim_spec_mode: 1,
          primary_view: item.primary_view ?? "killer",
          clip_overrides: item.clip_overrides,
        })),
        full_round_pov: hasPOVSegments
          ? { player_steam_id: fullRound.player_steam_id }
          : undefined,
      };
    })
    .filter((job) => job.selected_items.length > 0 || !!job.full_round_pov);
}

export function useImportDemos() {
  return {
    demoList,
    selectedIndex,
    detailCollapsed,
    selectedEntry,
    selectedDemo,
    canSelectPrev,
    canSelectNext,
    clipReadyDemos,
    autoAddVictimView,
    onDemosSelected,
    removeDemoAt,
    toggleSelected,
    formatDuration,
    selectPrevDemo,
    selectNextDemo,
    toggleDetailCollapsed,
    selectDemoByKey,
    ensureClipDemoSelected,
    getClipPlayers,
    getDeathPlayers,
    getClipSelectMode,
    setClipSelectMode,
    getFullRoundPlayers,
    getSelectedPlayerSteamID,
    setSelectedPlayerSteamID,
    getFullRoundPlayerSteamID,
    getClipRounds,
    getDeathRounds,
    getFullRoundPOVSelection,
    setFullRoundPOVEnabled,
    syncFullRoundPOVPlayer,
    fullRoundPlanByDemo,
    fullRoundPlanErrorByDemo,
    fetchFullRoundPOVPlan,
    getFullRoundPOVTrackingLabel,
    getMaterialSelections,
    getMaterialSelectionCount,
    addMaterialSelection,
    updateMaterialSpecModes,
    updateMaterialClipOverrides,
    updateMaterialIncludeVictim,
    updateMaterialIncludeKiller,
    removeMaterialSelection,
    isKillSelectedInDemo,
    clearMaterialSelections,
    buildBatchJobs,
    syncDefaultFullRoundPlayer,
  };
}
