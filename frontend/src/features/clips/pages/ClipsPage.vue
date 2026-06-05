<template>
  <div class="clips-page">
    <div class="clips-layout">
      <n-card
        class="left-card"
        :bordered="true"
        content-style="height: 100%; overflow: hidden; padding: 0;"
        content-class="left-card-content"
      >
        <div class="panel-head">
          <span class="panel-title">{{ t("main.clips.material_list_title") }}</span>
        </div>
        <div class="card-body">
          <n-empty v-if="!clipReadyDemos.length" :description="t('main.clips.no_demo')" />

          <n-collapse
            v-else
            accordion
            v-model:expanded-names="expandedDemoNames"
            @update:expanded-names="handleExpandedChange"
          >
          <n-collapse-item
            v-for="entry in clipReadyDemos"
            :key="entry.key"
            :name="entry.key"
            :title="entry.file_name"
          >
            <template #header-extra>
              <n-space align="center" size="small">
                <n-tag size="small">{{ getMaterialSelectionCount(entry) }}</n-tag>
                <n-tag
                  v-if="producedCountForDemo(entry) > 0"
                  size="small"
                  type="warning"
                  :bordered="false"
                >
                  {{ t("main.clips.produced_count", { count: producedCountForDemo(entry) }) }}
                </n-tag>
              </n-space>
            </template>

            <n-empty
              v-if="!getMaterialSelections(entry).length"
              :description="t('main.clips.no_materials_for_demo')"
              size="small"
            />

            <n-collapse
              v-else
              :expanded-names="getMaterialRoundExpandedNames(entry)"
              @update:expanded-names="handleMaterialRoundExpandedChange(entry, $event)"
            >
              <n-collapse-item
                v-for="group in getMaterialRoundGroups(entry)"
                :key="`${entry.key}-round-${group.round}`"
                :name="String(group.round)"
                :title="t('main.clips.round_title', { round: group.round, kills: group.items.length })"
              >
                <n-space vertical :size="8">
                  <div
                    v-for="item in group.items"
                    :key="item.kill.id"
                    class="material-row"
                    @dblclick="removeMaterialSelection(entry, item.kill.id)"
                  >
                    <div class="material-head">
                      <div class="material-tags-row">
                        <n-space align="center" size="small" class="view-tags">
                          <n-tag v-if="item.clip_overrides?.record_full_round" size="small" type="info" :bordered="false">
                            {{ t("main.clips.round_record_tag") }}
                          </n-tag>
                          <n-tag v-else size="small" type="success" :bordered="false">
                            {{ t("main.clips.killer_view") }}
                          </n-tag>
                          <n-tag v-if="item.include_victim" size="small" type="warning" :bordered="false">
                            {{ t("main.clips.victim_view") }}
                          </n-tag>
                        </n-space>
                        <n-tag
                          v-if="isKillAlreadyProduced(entry.file_path, item.kill.id)"
                          size="small"
                          type="warning"
                          :bordered="false"
                        >
                          {{ t("main.clips.already_produced") }}
                        </n-tag>
                        <n-button
                          text
                          size="small"
                          class="expand-btn"
                          @click.stop="toggleMaterialSettings(entry, item.kill.id)"
                          @dblclick.stop
                        >
                          {{ isMaterialSettingsExpanded(entry, item.kill.id) ? t("main.clips.collapse") : t("main.clips.expand") }}
                          {{ isMaterialSettingsExpanded(entry, item.kill.id) ? "▾" : "▸" }}
                        </n-button>
                      </div>
                      <div class="material-meta">
                        <DeathNoticeLine :kill="item.kill" compact />
                      </div>
                    </div>
                    <div
                      v-if="isMaterialSettingsExpanded(entry, item.kill.id)"
                      class="material-settings"
                      @dblclick.stop
                    >
                      <div class="setting-row">
                        <n-checkbox
                          :checked="item.include_victim"
                          @update:checked="handleVictimEnabledChange(entry, item.kill.id, !!$event)"
                        >
                          {{ t("main.clips.victim_enabled") }}
                        </n-checkbox>
                      </div>
                      <div class="setting-row">
                        <span class="setting-label">{{ t("main.settings.killer_pre_seconds") }}</span>
                        <n-input-number
                          :value="effectiveNumberValue(item, 'killer_pre_seconds')"
                          :min="1"
                          :max="5"
                          :step="0.5"
                          :precision="1"
                          @update:value="handleKillerPreValueChange(entry, item.kill.id, $event)"
                        />
                      </div>
                      <div class="setting-row">
                        <span class="setting-label">{{ t("main.settings.killer_post_seconds") }}</span>
                        <n-input-number
                          :value="effectiveNumberValue(item, 'killer_post_seconds')"
                          :min="1"
                          :max="5"
                          :step="0.5"
                          :precision="1"
                          @update:value="handleKillerPostValueChange(entry, item.kill.id, $event)"
                        />
                      </div>
                      <template v-if="item.include_victim">
                        <div class="setting-row">
                          <span class="setting-label">{{ t("main.settings.victim_pre_seconds") }}</span>
                          <n-input-number
                            :value="effectiveNumberValue(item, 'victim_pre_seconds')"
                            :min="1"
                            :max="2"
                            :step="0.5"
                            :precision="1"
                            @update:value="handleVictimPreValueChange(entry, item.kill.id, $event)"
                          />
                        </div>
                        <div class="setting-row">
                          <span class="setting-label">{{ t("main.settings.victim_post_seconds") }}</span>
                          <n-input-number
                            :value="effectiveNumberValue(item, 'victim_post_seconds')"
                            :min="1"
                            :max="2"
                            :step="0.5"
                            :precision="1"
                            @update:value="handleVictimPostValueChange(entry, item.kill.id, $event)"
                          />
                        </div>
                      </template>
                      <div class="setting-row">
                        <span class="setting-label">{{ t("main.settings.enable_voice") }}</span>
                        <n-switch
                          :value="effectiveBooleanValue(item, 'enable_voice')"
                          @update:value="handleVoiceEnabledChange(entry, item.kill.id, !!$event)"
                        />
                      </div>
                      <div class="setting-row">
                        <span class="setting-label">{{ t("main.settings.enable_spec_show_xray_zero") }}</span>
                        <n-switch
                          :value="effectiveBooleanValue(item, 'enable_spec_show_xray_zero')"
                          @update:value="handleXrayEnabledChange(entry, item.kill.id, !!$event)"
                        />
                      </div>
                    </div>
                  </div>
                </n-space>
              </n-collapse-item>
            </n-collapse>
          </n-collapse-item>
          </n-collapse>
        </div>
      </n-card>

      <n-card
        class="right-card"
        :bordered="true"
        content-style="height: 100%; overflow: hidden; padding: 0;"
        content-class="right-card-content"
      >
        <div class="panel-head">
          <span class="panel-title">{{ t("main.clips.select_title") }}</span>
        </div>
        <div class="right-card-body">
          <n-empty v-if="!activeDemoEntry" class="right-empty" :description="t('main.clips.no_demo')" />

          <template v-else>
            <div class="select-toolbar">
              <n-grid :cols="24" :x-gap="12" :y-gap="8">
                <n-gi :span="14">
                  <n-select
                    :value="selectedPlayerSteamID"
                    :options="playerOptions"
                    :placeholder="t('main.clips.player_placeholder')"
                    @update:value="handlePlayerChange"
                  />
                </n-gi>
                <n-gi :span="10">
                  <div class="summary-box">
                    <n-text depth="3">
                      {{ t("main.clips.material_summary", { count: getMaterialSelectionCount(activeDemoEntry) }) }}
                    </n-text>
                  </div>
                </n-gi>
              </n-grid>
            </div>

            <n-scrollbar class="select-scroll" trigger="none">
              <n-empty v-if="!currentRounds.length" :description="t('main.clips.no_round_kills')" />

              <n-collapse v-else v-model:expanded-names="expandedRounds">
                <n-collapse-item
                  v-for="round in currentRounds"
                  :key="round.round"
                  :name="String(round.round)"
                  :title="t('main.clips.round_title', { round: round.round, kills: round.kills.length })"
                >
                  <template #header-extra>
                    <n-button
                      size="tiny"
                      secondary
                      @click.stop="addRoundKills(round)"
                    >
                      {{ t("main.clips.record_round") }}
                    </n-button>
                  </template>
                  <n-space vertical :size="8">
                    <div
                      v-for="kill in round.kills"
                      :key="kill.id"
                      class="kill-row"
                      :class="{ selected: isKillSelectedInDemo(activeDemoEntry, kill.id) }"
                      @dblclick="toggleKillSelection(kill)"
                    >
                      <div class="kill-line">
                        <DeathNoticeLine :kill="kill" />
                      </div>
                      <n-tag
                        v-if="isKillAlreadyProduced(activeDemoEntry.file_path, kill.id)"
                        size="small"
                        type="warning"
                        :bordered="false"
                      >
                        {{ t("main.clips.already_produced") }}
                      </n-tag>
                    </div>
                  </n-space>
                </n-collapse-item>
              </n-collapse>
            </n-scrollbar>
          </template>
        </div>
      </n-card>
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
  type SelectOption,
} from "naive-ui";
import { t } from "@/shared/i18n";
import { CLIP_SETTINGS_SAVED_EVENT } from "@/shared/events";
import type { ClipSettings, DemoClipKill, DemoListEntry, DemoMaterialSelection } from "@/shared/types";
import { useImportDemos } from "@/features/import/composables/useImportDemos";
import DeathNoticeLine from "@/features/clips/components/DeathNoticeLine.vue";
import { ensureProduceHistoryInitialized, useProduceHistory } from "@/features/produce/composables/useProduceHistory";

const {
  selectedEntry,
  clipReadyDemos,
  selectDemoByKey,
  ensureClipDemoSelected,
  autoAddVictimView,
  getClipPlayers,
  getSelectedPlayerSteamID,
  setSelectedPlayerSteamID,
  getClipRounds,
  getMaterialSelections,
  getMaterialSelectionCount,
  addMaterialSelection,
  updateMaterialClipOverrides,
  updateMaterialIncludeVictim,
  removeMaterialSelection,
  isKillSelectedInDemo,
} = useImportDemos();
const { historySnapshot } = useProduceHistory();

const expandedRounds = ref<string[]>([]);
const expandedDemoNames = ref<string[]>([]);
const materialExpandedRoundsByDemo = ref<Record<string, string[]>>({});
const materialSettingsExpandedByDemo = ref<Record<string, string[]>>({});
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
  pov_hud_enabled: false,
});

type ClipOverrideNumberKey =
  | "killer_pre_seconds"
  | "killer_post_seconds"
  | "victim_pre_seconds"
  | "victim_post_seconds";
type ClipOverrideBooleanKey = "enable_voice" | "enable_spec_show_xray_zero";

const activeDemoEntry = computed<DemoListEntry | null>(() => {
  const current = selectedEntry.value;
  if (current && (current.meta?.clip_players?.length ?? 0) > 0) {
    return current;
  }
  return clipReadyDemos.value[0] ?? null;
});

const selectedPlayerSteamID = computed(() => getSelectedPlayerSteamID(activeDemoEntry.value));
const players = computed(() => getClipPlayers(activeDemoEntry.value));

const playerOptions = computed<SelectOption[]>(() =>
  players.value.map((player) => ({
    label: `${player.name} (${player.total_kills})`,
    value: player.steam_id,
  })),
);

const currentRounds = computed(() => getClipRounds(activeDemoEntry.value, selectedPlayerSteamID.value));

watch(
  () => [activeDemoEntry.value?.key, selectedPlayerSteamID.value, currentRounds.value.length],
  () => {
    expandedRounds.value = currentRounds.value.map((round) => String(round.round));
  },
  { immediate: true },
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
  ensureClipDemoSelected();
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

async function callBackend<T>(method: string, ...args: unknown[]): Promise<T> {
  const api = (window as any).go?.app?.App as Record<string, (...a: unknown[]) => Promise<unknown>> | undefined;
  const fn = api?.[method];
  if (!fn) throw new Error(`Wails API not loaded: ${method}`);
  return fn(...args) as Promise<T>;
}

async function loadClipSettings() {
  try {
    const settings = await callBackend<ClipSettings>("GetClipSettings");
    clipSettings.value = settings;
    autoAddVictimView.value = !!settings.auto_add_victim_view;
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

function handlePlayerChange(next: string | number | null) {
  if (next == null) {
    return;
  }
  setSelectedPlayerSteamID(activeDemoEntry.value, String(next));
}

function addKill(kill: DemoClipKill) {
  addMaterialSelection(activeDemoEntry.value, kill, autoAddVictimView.value);
}

function addRoundKills(round: { round: number; kills: DemoClipKill[] }) {
  const entry = activeDemoEntry.value;
  if (!entry || !round.kills.length) return;
  // 整回合一刀不剪：仅用第一个击杀锚定回合，不切割
  const firstKill = round.kills[0];
  if (!isKillSelectedInDemo(entry, firstKill.id)) {
    addMaterialSelection(entry, firstKill, false);
  }
  updateMaterialClipOverrides(entry, firstKill.id, { record_full_round: true });
}

function toggleKillSelection(kill: DemoClipKill) {
  if (isKillSelectedInDemo(activeDemoEntry.value, kill.id)) {
    removeMaterialSelection(activeDemoEntry.value, kill.id);
    return;
  }
  addKill(kill);
}

function isKillAlreadyProduced(demoPath: string, killID: string): boolean {
  if (!demoPath || !killID) return false;
  const set = producedKillIDsByDemo.value.get(demoPath);
  return !!set?.has(killID);
}

function producedCountForDemo(entry: DemoListEntry): number {
  return producedKillIDsByDemo.value.get(entry.file_path)?.size || 0;
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

function handleVictimEnabledChange(entry: DemoListEntry, killID: string, checked: boolean) {
  updateMaterialIncludeVictim(entry, killID, !!checked);
}

function effectiveNumberValue(item: DemoMaterialSelection, key: ClipOverrideNumberKey): number {
  const overrideValue = item.clip_overrides?.[key];
  if (typeof overrideValue === "number" && Number.isFinite(overrideValue)) {
    return overrideValue;
  }
  return clipSettings.value[key];
}

function effectiveBooleanValue(item: DemoMaterialSelection, key: ClipOverrideBooleanKey): boolean {
  const overrideValue = item.clip_overrides?.[key];
  if (typeof overrideValue === "boolean") {
    return overrideValue;
  }
  return !!clipSettings.value[key];
}

function setNumberOverride(entry: DemoListEntry, killID: string, key: ClipOverrideNumberKey, value: number) {
  updateMaterialClipOverrides(entry, killID, {
    [key]: value,
  });
}

function handleNumberValueChange(entry: DemoListEntry, killID: string, key: ClipOverrideNumberKey, value: number | null) {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return;
  }
  setNumberOverride(entry, killID, key, value);
}

function handleKillerPreValueChange(entry: DemoListEntry, killID: string, value: number | null) {
  handleNumberValueChange(entry, killID, "killer_pre_seconds", value);
}

function handleKillerPostValueChange(entry: DemoListEntry, killID: string, value: number | null) {
  handleNumberValueChange(entry, killID, "killer_post_seconds", value);
}

function handleVictimPreValueChange(entry: DemoListEntry, killID: string, value: number | null) {
  handleNumberValueChange(entry, killID, "victim_pre_seconds", value);
}

function handleVictimPostValueChange(entry: DemoListEntry, killID: string, value: number | null) {
  handleNumberValueChange(entry, killID, "victim_post_seconds", value);
}

function handleVoiceEnabledChange(entry: DemoListEntry, killID: string, checked: boolean) {
  updateMaterialClipOverrides(entry, killID, { enable_voice: checked });
}

function handleXrayEnabledChange(entry: DemoListEntry, killID: string, checked: boolean) {
  updateMaterialClipOverrides(entry, killID, { enable_spec_show_xray_zero: checked });
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
  display: grid;
  grid-template-columns: minmax(320px, 38fr) minmax(520px, 62fr);
  gap: 10px;
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
  overflow: hidden;
  display: flex;
  flex-direction: column;
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

.right-empty {
  margin-top: 8px;
}

.select-toolbar {
  flex: 0 0 auto;
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
  padding: 8px;
  border: 1px solid #2f3631;
  border-radius: 8px;
  cursor: pointer;
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

.expand-btn {
  margin-left: auto;
  flex: 0 0 auto;
  font-size: 12px;
}

.material-settings {
  border: 1px solid #3a423d;
  border-radius: 8px;
  padding: 8px;
  background: rgba(47, 54, 49, 0.25);
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
  gap: 8px;
  padding: 8px;
  border: 1px solid #2f3631;
  border-radius: 8px;
  cursor: pointer;
}

.kill-row.selected {
  border-color: #2f9462;
  background: rgba(47, 148, 98, 0.15);
}

.kill-line {
  flex: 1;
  min-width: 0;
}
</style>
