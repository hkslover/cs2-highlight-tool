<template>
  <div class="clips-page">
    <div ref="containerRef" class="clips-layout">
      <MaterialListPanel
        :style="leftPanelStyle"
        :clip-ready-demos="clipReadyDemos"
        :expanded-demo-names="expandedDemoNames"
        :clip-settings="clipSettings"
        :material-count="getMaterialSelectionCount"
        :materials="getMaterialSelections"
        :get-p-o-v-selection="getFullRoundPOVSelection"
        :get-p-o-v-plan="getFullRoundPOVPlan"
        :get-p-o-v-error="getFullRoundPOVError"
        :get-p-o-v-tracking-label="getFullRoundPOVTrackingLabel"
        :produced-count="producedCountForDemo"
        :can-clear-materials="canClearMaterials"
        :is-kill-already-produced="isKillAlreadyProducedForEntry"
        :get-p-o-v-expanded="getFullRoundPOVExpanded"
        :get-p-o-v-round-expanded="getPOVRoundExpanded"
        :pov-round-kills="povRoundKills"
        :pov-segment-title="povSegmentTitle"
        :get-material-expanded="getMaterialRoundExpandedNames"
        :get-material-settings-expanded="getMaterialSettingsExpandedNames"
        @update:expanded-demos="handleExpandedChange"
        @clear-materials="handleClearMaterials"
        @update:pov-expanded="handleFullRoundPOVExpanded"
        @update:pov-round-expanded="handlePOVRoundExpanded"
        @update:material-round-expanded="handleMaterialRoundExpandedChange"
        @update:material-settings-expanded="handleMaterialSettingsExpanded"
      />

      <div
        class="clips-splitter"
        :class="{ dragging: isResizing }"
        @mousedown="startResize"
      />

      <ClipSelectionPanel
        :style="rightPanelStyle"
        :active-demo-entry="activeDemoEntry"
        :full-round-p-o-v-enabled="fullRoundPOVEnabled"
        :selected-player-steam-i-d="selectedPlayerSteamID"
        :player-options="playerOptions"
        :kill-filter="killFilter"
        :matched-count="filteredKills.length"
        :total-count="scopedKills.length"
        :addable-count="addableKills.length"
        :selected-count="getMaterialSelectionCount(activeDemoEntry)"
        :max-round="maxRound"
        :max-distance="maxDistance"
        :weapon-groups="weaponGroups"
        :current-rounds="currentRounds"
        :expanded-rounds="expandedRounds"
        :empty-kill-description="emptyKillDescription"
        :is-kill-selected="isKillSelectedForActiveDemo"
        :is-kill-already-produced="isKillAlreadyProduced"
        @pov-toggle="handleFullRoundPOVSwitch"
        @player-change="handlePlayerChange"
        @role-change="handleRoleChange"
        @ignore-player-change="handleIgnorePlayerChange"
        @traits-change="handleTraitsChange"
        @weapons-change="handleWeaponsChange"
        @hit-groups-change="handleHitGroupsChange"
        @sides-change="handleSidesChange"
        @rounds-change="handleRoundsChange"
        @distance-change="handleDistanceChange"
        @apply-preset="handleApplyPreset"
        @clear="handleClearFilter"
        @select-all="handleSelectAllFiltered"
        @rounds-expanded="handleRoundsExpanded"
        @toggle-kill="toggleKillSelection"
      />
    </div>
  </div>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import {
  NButton,
  NCard,
  NCheckbox,
  NCollapse,
  NCollapseItem,
  NEmpty,
  NGi,
  NGrid,
  NInputNumber,
  NSelect,
  NScrollbar,
  NSpace,
  NSwitch,
  NTag,
  NText,
  useMessage,
  type SelectOption,
} from "naive-ui";
import { t } from "@/shared/i18n";
import { CLIP_SETTINGS_SAVED_EVENT } from "@/shared/events";
import type {
  ClipSettings,
  DemoHitGroup,
  DemoClipKill,
  DemoListEntry,
  DemoMaterialSelection,
  DemoPlayerInfo,
  FullRoundPOVSegment,
} from "@/shared/types";
import {
  isOpponentIncluded,
  isPrimaryIncluded,
  isSelfKill,
  roleOfPosition,
  type ViewPosition,
  type WindowEdge,
} from "@/shared/clip-views";
import {
  ALL_PLAYERS_VALUE,
  collectScopedKills,
  groupDemoWeapons,
  isKillFilterActive,
  maxKillDistance,
  resolvePresetPatch,
  resolvePrimaryView,
  sanitizeFilterForScopedKills,
  type KillFilter,
  type KillFilterPreset,
  type KillPlayerRole,
  type KillTrait,
} from "@/shared/kill-filter";
import {
  clipReadyDemos,
  ensureClipDemoSelected,
  selectedEntry,
  selectDemoByKey,
} from "@/domains/demo";
import {
  addMaterialSelection,
  autoAddVictimView,
  clearMaterialSelections,
  fetchFullRoundPOVPlan,
  formatDuration,
  fullRoundPlanByDemo,
  fullRoundPlanErrorByDemo,
  getAllDemoKills,
  getClipPlayers,
  getClipRounds,
  getDeathPlayers,
  getDemoMaterials,
  getFilteredKills,
  getFilteredRounds,
  getFullRoundPlayers,
  getFullRoundPOVSelection,
  getFullRoundPOVTrackingLabel,
  getFullRoundPlayerSteamID,
  getKillFilter,
  getMaterialSelectionCount,
  getMaterialSelections,
  getSelectedPlayerSteamID,
  isKillSelectedInDemo,
  patchKillFilter,
  removeMaterialSelection,
  resetKillFilterConditions,
  setAutoAddVictimView,
  setFullRoundPOVEnabled,
  setKillFilterRole,
  setSelectedPlayerSteamID,
  syncDefaultFullRoundPlayer,
  syncFullRoundPOVPlayer,
  updateMaterialClipOverrides,
  updateMaterialIncludeKiller,
  updateMaterialIncludeVictim,
} from "@/domains/clip-selection";
import ClipSelectionPanel from "@/features/clips/components/ClipSelectionPanel.vue";
import MaterialListPanel from "@/features/clips/components/MaterialListPanel.vue";
import { ensureProduceHistoryInitialized, useProduceHistory } from "@/features/produce/composables/useProduceHistory";
import { useSplitter } from "@/shared/composables/useSplitter";
import { backend } from "@/shared/backend";

const containerRef = ref<HTMLElement | null>(null);
const { isResizing, leftPanelStyle, rightPanelStyle, startResize } = useSplitter(containerRef, {
  initialLeftRatio: 0.38,
  minLeftPx: 300,
  minRightPx: 450,
  splitterWidthPx: 12,
});

const { historySnapshot } = useProduceHistory();
const message = useMessage();

const expandedRounds = ref<string[]>([]);
const expandedDemoNames = ref<string[]>([]);
const materialExpandedRoundsByDemo = ref<Record<string, string[]>>({});
const materialSettingsExpandedByDemo = ref<Record<string, string[]>>({});
const fullRoundPOVExpandedByDemo = ref<Record<string, string[]>>({});
const povRoundExpandedByDemo = ref<Record<string, string[]>>({});
const clipSettings = ref<ClipSettings>({
  killer_pre_seconds: 5,
  killer_post_seconds: 5,
  victim_pre_seconds: 1,
  victim_post_seconds: 1,
  auto_add_victim_view: true,
  enable_voice: true,
  record_fps: 60,
  record_quality: "high",
  edit_fps: 60,
  edit_quality: "high",
  video_preset: "auto",
  launch_resolution: "4:3",
  record_output_dir: "",
  enable_spec_show_xray_zero: true,
  hide_all_ui: false,
  hide_player_avatars: false,
  use_shoulder_camera: false,
  pov_hud_enabled: true,
  pov_radar_enabled: false,
  sky_blackout: true,
  disable_clouds: false,
  kill_feed_lifetime: 4,
  block_kill_feed: false,
});

type ClipOverrideNumberKey =
  | "killer_pre_seconds"
  | "killer_post_seconds"
  | "victim_pre_seconds"
  | "victim_post_seconds";
type ClipOverrideBooleanKey = "enable_voice" | "enable_spec_show_xray_zero";

const activeDemoEntry = computed<DemoListEntry | null>(() => {
  const current = selectedEntry.value;
  if (current && ((current.meta?.clip_players?.length ?? 0) > 0 || (current.meta?.players?.length ?? 0) > 0)) {
    return current;
  }
  return clipReadyDemos.value[0] ?? null;
});

const fullRoundPOVSelection = computed(() => getFullRoundPOVSelection(activeDemoEntry.value));
const fullRoundPOVEnabled = computed(() => fullRoundPOVSelection.value.enabled);
const killFilter = computed(() => getKillFilter(activeDemoEntry.value));
const selectedClipPlayerSteamID = computed(() => getSelectedPlayerSteamID(activeDemoEntry.value));
const selectedPlayerSteamID = computed(() =>
  fullRoundPOVEnabled.value
    ? fullRoundPOVSelection.value.player_steam_id
    : selectedClipPlayerSteamID.value,
);
const clipPlayers = computed(() => getClipPlayers(activeDemoEntry.value));
const deathPlayers = computed(() => getDeathPlayers(activeDemoEntry.value));
const fullRoundPlayers = computed(() => getFullRoundPlayers(activeDemoEntry.value));

const allDemoKills = computed(() => getAllDemoKills(activeDemoEntry.value));
const scopedKills = computed(() =>
  collectScopedKills(allDemoKills.value, killFilter.value, selectedClipPlayerSteamID.value),
);
const filteredKills = computed(() =>
  fullRoundPOVEnabled.value ? [] : getFilteredKills(activeDemoEntry.value),
);
// Bulk add skips whatever is already in the material list, so the button count
// is what would actually be added rather than what is merely on screen.
const addableKills = computed(() =>
  filteredKills.value.filter((kill) => !isKillSelectedInDemo(activeDemoEntry.value, kill.id)),
);
const maxRound = computed(() =>
  Math.max(activeDemoEntry.value?.meta?.total_rounds ?? 0, ...allDemoKills.value.map((k) => k.round), 1),
);
const maxDistance = computed(() => Math.max(maxKillDistance(scopedKills.value), 1));
const weaponGroups = computed(() => groupDemoWeapons(scopedKills.value));

const playerOptions = computed<SelectOption[]>(() => {
  if (fullRoundPOVEnabled.value) {
    return fullRoundPlayers.value.map((player) => ({
      label: fullRoundPlayerLabel(player),
      value: getFullRoundPlayerSteamID(player),
    }));
  }
  // Each role has its own roster and count: killers by frags, victims by deaths.
  const options: SelectOption[] =
    killFilter.value.role === "victim"
      ? deathPlayers.value.map((player) => ({
          label: `${player.name} (${player.total_deaths ?? 0})`,
          value: player.steam_id,
        }))
      : clipPlayers.value.map((player) => ({
          label: `${player.name} (${player.total_kills})`,
          value: player.steam_id,
        }));
  return [{ label: t("main.clips.filter.all_players"), value: ALL_PLAYERS_VALUE }, ...options];
});

const currentRounds = computed(() =>
  fullRoundPOVEnabled.value
    ? getClipRounds(activeDemoEntry.value, selectedPlayerSteamID.value)
    : getFilteredRounds(activeDemoEntry.value),
);
const emptyKillDescription = computed(() => {
  if (fullRoundPOVEnabled.value) return t("main.clips.no_full_round_player_kills");
  if (isKillFilterActive(killFilter.value)) return t("main.clips.filter.no_match");
  if (killFilter.value.role === "victim") {
    return deathPlayers.value.length
      ? t("main.clips.no_round_deaths")
      : t("main.clips.no_death_players");
  }
  return t("main.clips.no_round_kills");
});

watch(
  () => [
    activeDemoEntry.value?.key,
    selectedPlayerSteamID.value,
    killFilter.value,
    currentRounds.value.length,
  ],
  () => {
    expandedRounds.value = currentRounds.value.map((round) => String(round.round));
  },
  { immediate: true, deep: true },
);

watch(
  () => activeDemoEntry.value?.key,
  (key) => {
    if (!key) {
      expandedDemoNames.value = [];
      return;
    }
    if (!expandedDemoNames.value.includes(key)) {
      expandedDemoNames.value = [key];
    }
  },
  { immediate: true },
);

onMounted(() => {
  const entry = ensureClipDemoSelected();
  if (entry) syncDefaultFullRoundPlayer(entry);
  void ensureProduceHistoryInitialized();
  void loadClipSettings();
  window.addEventListener(CLIP_SETTINGS_SAVED_EVENT, onClipSettingsSaved);
});

onBeforeUnmount(() => {
  window.removeEventListener(CLIP_SETTINGS_SAVED_EVENT, onClipSettingsSaved);
});

const producedKillIDsByDemo = computed(() => {
  const byDemo = new Map<string, Set<string>>();
  for (const item of historySnapshot.value.items || []) {
    const demoPath = item.demo_path || "";
    if (!demoPath) continue;
    if ((item.history_type || "produce_clip") === "edited_video") continue;
    if (!byDemo.has(demoPath)) {
      byDemo.set(demoPath, new Set<string>());
    }
    const set = byDemo.get(demoPath)!;
    for (const killID of item.kill_ids || []) {
      if (killID) {
        set.add(killID);
      }
    }
  }
  return byDemo;
});

// producedTakeCountByDemo counts ALL produce_clip history takes for a demo,
// including full_round_pov takes (which carry no kill_ids). This drives the
// "已生成 N" badge so POV recordings are reflected alongside clip kills.
const producedTakeCountByDemo = computed(() => {
  const byDemo = new Map<string, number>();
  for (const item of historySnapshot.value.items || []) {
    const demoPath = item.demo_path || "";
    if (!demoPath) continue;
    if ((item.history_type || "produce_clip") === "edited_video") continue;
    byDemo.set(demoPath, (byDemo.get(demoPath) || 0) + 1);
  }
  return byDemo;
});

type ExpandedNames = string | number | Array<string | number> | null;

interface POVPlanView {
  readonly player_name: string;
  readonly player_steam_id: string;
  readonly segments: readonly Readonly<FullRoundPOVSegment>[];
}

function normalizeExpandedNames(names: ExpandedNames): string[] {
  return (Array.isArray(names) ? names : names != null ? [names] : []).map((name) => String(name));
}

function getFullRoundPOVPlan(entry: DemoListEntry): POVPlanView | undefined {
  return fullRoundPlanByDemo.value[entry.key];
}

function getFullRoundPOVError(entry: DemoListEntry): string | undefined {
  return fullRoundPlanErrorByDemo.value[entry.key];
}

function getMaterialSettingsExpandedNames(entry: DemoListEntry): string[] {
  return materialSettingsExpandedByDemo.value[entry.key] || [];
}

function handleMaterialSettingsExpanded(entry: DemoListEntry, names: string[]): void {
  materialSettingsExpandedByDemo.value = {
    ...materialSettingsExpandedByDemo.value,
    [entry.key]: names,
  };
}

function isKillAlreadyProducedForEntry(entry: DemoListEntry, killID: string): boolean {
  return isKillAlreadyProduced(entry.file_path, killID);
}

function isKillSelectedForActiveDemo(killID: string): boolean {
  return isKillSelectedInDemo(activeDemoEntry.value, killID);
}

function handleTraitsChange(value: KillTrait[]): void {
  patchKillFilter(activeDemoEntry.value, { traits: value });
}

function handleWeaponsChange(value: string[]): void {
  patchKillFilter(activeDemoEntry.value, { weapons: value });
}

function handleHitGroupsChange(value: DemoHitGroup[]): void {
  patchKillFilter(activeDemoEntry.value, { hit_groups: value });
}

function handleSidesChange(value: string[]): void {
  patchKillFilter(activeDemoEntry.value, { sides: value });
}

function handleRoundsChange(value: [number, number] | null): void {
  patchKillFilter(activeDemoEntry.value, { rounds: value });
}

function handleDistanceChange(value: [number, number] | null): void {
  patchKillFilter(activeDemoEntry.value, { distance: value });
}

function handleClearFilter(): void {
  resetKillFilterConditions(activeDemoEntry.value);
}

function handleRoundsExpanded(names: ExpandedNames): void {
  expandedRounds.value = normalizeExpandedNames(names);
}

async function loadClipSettings() {
  try {
    const settings = await backend.GetClipSettings();
    clipSettings.value = settings;
    setAutoAddVictimView(!!settings.auto_add_victim_view);
  } catch {
    // ignore settings load error in clips page
  }
}

function onClipSettingsSaved() {
  void loadClipSettings();
}

function handleExpandedChange(names: string | number | Array<string | number> | null) {
  const list = (Array.isArray(names) ? names : names != null ? [names] : []).map((name) => String(name));
  expandedDemoNames.value = list;
  const next = list[0];
  if (next) {
    selectDemoByKey(next);
  }
}

function sanitizeFilterConditionsForContext(
  entry: DemoListEntry | null,
  overridePlayerSteamID?: string,
  overrideRole?: KillPlayerRole,
  overrideIgnorePlayer?: boolean,
) {
  if (!entry) return;
  const filter = getKillFilter(entry);
  const playerSteamID = overridePlayerSteamID ?? getSelectedPlayerSteamID(entry);
  const role = overrideRole ?? filter.role;
  const ignorePlayer = overrideIgnorePlayer ?? filter.ignore_player;

  const tempFilter: KillFilter = { ...filter, role, ignore_player: ignorePlayer };
  const scoped = collectScopedKills(getAllDemoKills(entry), tempFilter, playerSteamID);
  const patch = sanitizeFilterForScopedKills(filter, scoped);
  if (Object.keys(patch).length > 0) {
    patchKillFilter(entry, patch);
  }
}

async function handlePlayerChange(next: string | number | null) {
  if (next == null) {
    return;
  }
  const playerSteamID = String(next);
  const entry = activeDemoEntry.value;
  if (fullRoundPOVEnabled.value && playerSteamID) {
    syncFullRoundPOVPlayer(entry, playerSteamID);
    await fetchFullRoundPOVPlan(entry, playerSteamID);
    return;
  }
  setSelectedPlayerSteamID(entry, playerSteamID);
  sanitizeFilterConditionsForContext(entry, playerSteamID);
}

function addKill(kill: DemoClipKill) {
  const entry = activeDemoEntry.value;
  // A suicide has no second camera to record — both sides are the same player.
  const autoAddOpponent = autoAddVictimView.value && !isSelfKill(kill);
  if (fullRoundPOVEnabled.value) {
    // The POV pass already covers the tracked player's own camera for the whole
    // round, so picking a kill here only adds the opponent's angle.
    addMaterialSelection(entry, kill, true, false, "killer");
    return;
  }
  if (resolvePrimaryView(killFilter.value) === "victim") {
    // The selected player is the victim: their own camera is the victim pass.
    addMaterialSelection(entry, kill, true, autoAddOpponent, "victim");
    return;
  }
  addMaterialSelection(entry, kill, autoAddOpponent, true, "killer");
}

let lastClickTime = 0;
let lastClickKillId = "";

function toggleKillSelection(kill: DemoClipKill) {
  const now = Date.now();
  if (kill.id === lastClickKillId && now - lastClickTime < 250) {
    return;
  }
  lastClickTime = now;
  lastClickKillId = kill.id;

  if (isKillSelectedInDemo(activeDemoEntry.value, kill.id)) {
    removeMaterialSelection(activeDemoEntry.value, kill.id);
    return;
  }
  addKill(kill);
}

function handleRoleChange(role: KillPlayerRole) {
  setKillFilterRole(activeDemoEntry.value, role);
  sanitizeFilterConditionsForContext(activeDemoEntry.value, undefined, role);
}

function handleIgnorePlayerChange(ignore: boolean) {
  patchKillFilter(activeDemoEntry.value, { ignore_player: ignore });
  if (!ignore) {
    sanitizeFilterConditionsForContext(activeDemoEntry.value, undefined, undefined, false);
  }
}

function handleApplyPreset(preset: KillFilterPreset) {
  // Presets name weapon families; they only become concrete weapon names once
  // there is a candidate list to resolve them against.
  patchKillFilter(activeDemoEntry.value, resolvePresetPatch(preset, scopedKills.value));
}

function handleSelectAllFiltered() {
  const pending = addableKills.value;
  if (!pending.length) {
    message.info(t("main.clips.filter.select_all_none"));
    return;
  }
  for (const kill of pending) {
    addKill(kill);
  }
  message.success(t("main.clips.filter.select_all_done", { count: pending.length }));
}

async function handleFullRoundPOVSwitch(value: boolean) {
  const entry = activeDemoEntry.value;
  if (!entry) return;
  setFullRoundPOVEnabled(entry, value);
  if (value) {
    const playerSteamID = getFullRoundPOVSelection(entry).player_steam_id;
    if (playerSteamID) {
      await fetchFullRoundPOVPlan(entry, playerSteamID);
    }
  }
}

function fullRoundPlayerLabel(player: DemoPlayerInfo): string {
  return player.name || getFullRoundPlayerSteamID(player);
}

function isKillAlreadyProduced(demoPath: string, killID: string): boolean {
  if (!demoPath || !killID) return false;
  const set = producedKillIDsByDemo.value.get(demoPath);
  return !!set?.has(killID);
}

function producedCountForDemo(entry: DemoListEntry): number {
  return producedTakeCountByDemo.value.get(entry.file_path) || 0;
}

// The clear button belongs to the demo currently being worked on, so it only
// shows on the open entry and only while that entry actually has selections.
function canClearMaterials(entry: DemoListEntry): boolean {
  return entry.key === activeDemoEntry.value?.key && getMaterialSelectionCount(entry) > 0;
}

function handleClearMaterials(entry: DemoListEntry) {
  const count = getMaterialSelectionCount(entry);
  if (!count) return;
  clearMaterialSelections(entry);
  // Drop the demo's expand records too: a stale round list would leave the
  // groups collapsed the next time materials are added back.
  const { [entry.key]: _rounds, ...restRounds } = materialExpandedRoundsByDemo.value;
  materialExpandedRoundsByDemo.value = restRounds;
  const { [entry.key]: _settings, ...restSettings } = materialSettingsExpandedByDemo.value;
  materialSettingsExpandedByDemo.value = restSettings;
  message.success(t("main.clips.clear_selected_done", { count }));
}

function getMaterialRoundGroups(entry: DemoListEntry): Array<{ round: number; items: DemoMaterialSelection[] }> {
  const items = getMaterialSelections(entry);
  const grouped = new Map<number, DemoMaterialSelection[]>();
  for (const item of items) {
    const round = item.kill.round;
    if (!grouped.has(round)) {
      grouped.set(round, []);
    }
    grouped.get(round)!.push(item);
  }
  return Array.from(grouped.entries())
    .sort((a, b) => a[0] - b[0])
    .map(([round, roundItems]) => ({ round, items: roundItems }));
}

function getMaterialRoundExpandedNames(entry: DemoListEntry): string[] {
  const allRounds = getMaterialRoundGroups(entry).map((group) => String(group.round));
  if (!Object.prototype.hasOwnProperty.call(materialExpandedRoundsByDemo.value, entry.key)) {
    return allRounds;
  }
  const current = materialExpandedRoundsByDemo.value[entry.key] || [];
  return current.filter((name) => allRounds.includes(name));
}

function handleMaterialRoundExpandedChange(
  entry: DemoListEntry,
  names: string | number | Array<string | number> | null,
) {
  const list = (Array.isArray(names) ? names : names != null ? [names] : []).map((name) => String(name));
  materialExpandedRoundsByDemo.value = {
    ...materialExpandedRoundsByDemo.value,
    [entry.key]: list,
  };
}

function isMaterialSettingsExpanded(entry: DemoListEntry, killID: string): boolean {
  const expanded = materialSettingsExpandedByDemo.value[entry.key] || [];
  return expanded.includes(killID);
}

function toggleMaterialSettings(entry: DemoListEntry, killID: string) {
  const expanded = materialSettingsExpandedByDemo.value[entry.key] || [];
  const next = expanded.includes(killID) ? expanded.filter((id) => id !== killID) : expanded.concat(killID);
  materialSettingsExpandedByDemo.value = {
    ...materialSettingsExpandedByDemo.value,
    [entry.key]: next,
  };
}

function handleOpponentEnabledChange(entry: DemoListEntry, item: DemoMaterialSelection, checked: boolean) {
  if (roleOfPosition(item, "opponent") === "killer") {
    updateMaterialIncludeKiller(entry, item.kill.id, checked);
    return;
  }
  updateMaterialIncludeVictim(entry, item.kill.id, checked);
}

/** Per-item overrides stay keyed by role, so a position resolves to its role. */
function overrideKeyFor(
  item: DemoMaterialSelection,
  position: ViewPosition,
  edge: WindowEdge,
): ClipOverrideNumberKey {
  return `${roleOfPosition(item, position)}_${edge}_seconds` as ClipOverrideNumberKey;
}

/** Global settings are positional: the killer_* pair is the primary window. */
function settingsKeyFor(position: ViewPosition, edge: WindowEdge): ClipOverrideNumberKey {
  return position === "primary"
    ? (`killer_${edge}_seconds` as ClipOverrideNumberKey)
    : (`victim_${edge}_seconds` as ClipOverrideNumberKey);
}

function positionSeconds(item: DemoMaterialSelection, position: ViewPosition, edge: WindowEdge): number {
  const overrideValue = item.clip_overrides?.[overrideKeyFor(item, position, edge)];
  if (typeof overrideValue === "number" && Number.isFinite(overrideValue)) {
    return overrideValue;
  }
  return clipSettings.value[settingsKeyFor(position, edge)];
}

function handleSecondsChange(
  entry: DemoListEntry,
  item: DemoMaterialSelection,
  position: ViewPosition,
  edge: WindowEdge,
  value: number | null,
) {
  if (typeof value !== "number" || !Number.isFinite(value)) return;
  updateMaterialClipOverrides(entry, item.kill.id, {
    [overrideKeyFor(item, position, edge)]: value,
  });
}

function effectiveBooleanValue(item: DemoMaterialSelection, key: ClipOverrideBooleanKey): boolean {
  const overrideValue = item.clip_overrides?.[key];
  if (typeof overrideValue === "boolean") {
    return overrideValue;
  }
  return !!clipSettings.value[key];
}

function handleVoiceEnabledChange(entry: DemoListEntry, killID: string, checked: boolean) {
  updateMaterialClipOverrides(entry, killID, { enable_voice: checked });
}

function handleXrayEnabledChange(entry: DemoListEntry, killID: string, checked: boolean) {
  updateMaterialClipOverrides(entry, killID, { enable_spec_show_xray_zero: checked });
}

function getFullRoundPOVExpanded(entry: DemoListEntry | null): string[] {
  if (!entry?.key) return [];
  return fullRoundPOVExpandedByDemo.value[entry.key] || [];
}

function handleFullRoundPOVExpanded(
  entry: DemoListEntry | null,
  names: string | number | Array<string | number> | null,
) {
  if (!entry) return;
  const list = (Array.isArray(names) ? names : names != null ? [names] : []).map((name) => String(name));
  fullRoundPOVExpandedByDemo.value = {
    ...fullRoundPOVExpandedByDemo.value,
    [entry.key]: list,
  };
}

function getPOVRoundKillCount(entry: DemoListEntry | null, playerSteamID: string, roundNum: number): number {
  if (!entry?.meta?.clip_players) return 0;
  const player = entry.meta.clip_players.find((p) => p.steam_id === playerSteamID);
  if (!player) return 0;
  const round = player.rounds.find((r) => r.round === roundNum);
  return round?.kills?.length ?? 0;
}

function povRoundKills(entry: DemoListEntry | null, roundNum: number): DemoClipKill[] {
  if (!entry?.meta?.clip_players) return [];
  const playerSteamID = getFullRoundPOVSelection(entry).player_steam_id;
  const player = entry.meta.clip_players.find((p) => p.steam_id === playerSteamID);
  if (!player) return [];
  const round = player.rounds.find((r) => r.round === roundNum);
  if (!round?.kills?.length) return [];
  return [...round.kills].sort((a, b) => {
    if (a.tick === b.tick) return String(a.id).localeCompare(String(b.id));
    return a.tick - b.tick;
  });
}

function povSegmentTitle(entry: DemoListEntry | null, segment: FullRoundPOVSegment): string {
  const playerSteamID = getFullRoundPOVSelection(entry).player_steam_id;
  const kills = getPOVRoundKillCount(entry, playerSteamID, segment.round);
  const died = String(segment.end_reason || "").toLowerCase() === "target_death";
  const key = died ? "main.clips.full_round_pov_round_title_died" : "main.clips.full_round_pov_round_title_survived";
  return t(key, { round: segment.round, kills });
}

function getPOVRoundExpanded(entry: DemoListEntry | null): string[] {
  if (!entry?.key) return [];
  return povRoundExpandedByDemo.value[entry.key] || [];
}

function handlePOVRoundExpanded(
  entry: DemoListEntry | null,
  names: string | number | Array<string | number> | null,
) {
  if (!entry) return;
  const list = (Array.isArray(names) ? names : names != null ? [names] : []).map((name) => String(name));
  povRoundExpandedByDemo.value = {
    ...povRoundExpandedByDemo.value,
    [entry.key]: list,
  };
}

</script>

<style scoped>
.clips-page {
  height: 100%;
  min-height: 0;
  overflow-y: hidden;
  overflow-x: auto;
}

.clips-layout {
  display: flex;
  height: 100%;
  min-height: 0;
  min-width: 860px;
  align-items: stretch;
}

.left-card,
.right-card {
  background: #181b19;
  height: 100%;
  max-height: 100%;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.clips-splitter {
  position: relative;
  flex: 0 0 12px;
  cursor: col-resize;
}

.clips-splitter::before {
  content: "";
  position: absolute;
  left: 50%;
  top: 8px;
  bottom: 8px;
  width: 2px;
  border-radius: 999px;
  background: #303732;
  transform: translateX(-50%);
  transition: background-color 0.2s ease;
}

.clips-splitter:hover::before,
.clips-splitter.dragging::before {
  background: #2f9462;
}

.left-card :deep(.left-card-content),
.right-card :deep(.right-card-content) {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.panel-head {
  flex-shrink: 0;
  min-height: 34px;
  padding: 6px 10px;
  border-bottom: 1px solid #303732;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.card-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 8px 10px 10px;
}

.right-card-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  padding: 8px 10px 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.panel-title {
  font-size: 13px;
  font-weight: 600;
}

.panel-actions {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.switch-label {
  color: #aab4ad;
  font-size: 12px;
  white-space: nowrap;
}

.right-empty {
  margin-top: 8px;
}

.select-toolbar {
  flex: 0 0 auto;
}

.mode-switch-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 8px;
}

.switch-hint {
  color: #8d9890;
  font-size: 12px;
}

.setting-hint {
  color: #8d9890;
  font-size: 12px;
}

.select-scroll {
  flex: 1;
  min-height: 0;
}

.summary-box {
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: flex-end;
}

.material-row {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid #28312b;
  border-radius: 8px;
  background: #151816;
  transition: border-color 0.18s ease, background 0.18s ease;
}

.material-row:hover {
  border-color: #38453d;
  background: #181d1a;
}

.material-head {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.material-tags-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  flex-wrap: wrap;
}

.material-meta {
  width: 100%;
  min-width: 0;
}

.view-tags {
  flex: 0 1 auto;
  min-width: 0;
}

.clear-materials-btn {
  font-size: 12px;
  flex: 0 0 auto;
}

.material-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 0 0 auto;
}

.expand-btn {
  flex: 0 0 auto;
  font-size: 12px;
}

.material-delete-btn {
  background: transparent;
  border: none;
  border-radius: 4px;
  color: #7d8881;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  padding: 0;
  transition: background 0.15s ease, color 0.15s ease;
}

.material-delete-btn:hover {
  background: rgba(224, 83, 83, 0.16);
  color: #f28b8b;
}

.material-settings {
  border: 1px solid #323b35;
  border-radius: 8px;
  padding: 10px;
  background: rgba(26, 32, 28, 0.5);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.setting-label {
  font-size: 12px;
  color: #a7b2aa;
}

.kill-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid #28312b;
  border-radius: 8px;
  background: #151816;
  cursor: pointer;
  user-select: none;
  transition: border-color 0.15s ease, background 0.15s ease, transform 0.15s ease;
}

.kill-row:hover {
  border-color: rgba(47, 181, 114, 0.45);
  background: #1a201c;
  transform: translateX(2px);
}

.kill-row.selected {
  border-color: #2fb572;
  background: linear-gradient(90deg, rgba(47, 181, 114, 0.14) 0%, rgba(20, 24, 21, 0.6) 100%);
  box-shadow: 0 0 10px rgba(47, 181, 114, 0.12), inset 0 0 0 1px rgba(47, 181, 114, 0.2);
}

.full-round-pov-section {
  margin-bottom: 10px;
}

.pov-round-kills {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 6px 0 2px;
}

.pov-round-empty {
  font-size: 12px;
  color: #8d9890;
}

.full-round-loading {
  padding: 8px 12px;
  font-size: 12px;
  color: #8d9890;
}

.full-round-error {
  color: #e07f7f;
}

.full-round-player {
  font-size: 12px;
  color: #edf1ee;
}

.kill-line {
  flex: 1;
  min-width: 0;
}
</style>
