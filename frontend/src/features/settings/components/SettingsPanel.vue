<template>
  <n-space vertical :size="14">
    <n-alert v-if="errorMessage" type="error" :bordered="false" closable @close="dismissError">{{ errorMessage }}</n-alert>
    <n-alert v-if="successMessage" type="success" :bordered="false" closable @close="successMessage = ''">{{ successMessage }}</n-alert>
    <n-card size="small" :bordered="true" class="section-card">
      <template #header>
        <span class="section-title">{{ t("main.settings.clip_title") }}</span>
      </template>

      <n-space vertical :size="12">
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.killer_pre_seconds") }}</span>
          <n-input-number v-model:value="settings.killer_pre_seconds" :min="1" :max="20" :step="0.5" :precision="1" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.killer_post_seconds") }}</span>
          <n-input-number v-model:value="settings.killer_post_seconds" :min="1" :max="20" :step="0.5" :precision="1" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.victim_pre_seconds") }}</span>
          <n-input-number v-model:value="settings.victim_pre_seconds" :min="1" :max="20" :step="0.5" :precision="1" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.victim_post_seconds") }}</span>
          <n-input-number v-model:value="settings.victim_post_seconds" :min="1" :max="20" :step="0.5" :precision="1" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.auto_add_victim") }}</span>
          <n-switch v-model:value="settings.auto_add_victim_view" />
        </div>
        <div class="setting-list-panel">
          <div class="setting-list-head">
            <div class="setting-list-title-row">
              <span class="setting-list-title">{{ t("main.settings.recording_options_list_title") }}</span>
            </div>
            <div class="setting-list-controls">
              <n-input
                v-model:value="settingSearchQuery"
                clearable
                size="small"
                class="setting-search"
                :placeholder="t('main.settings.recording_options_search_placeholder')"
              />
            </div>
          </div>

          <div class="setting-list-body">
            <template v-if="pagedSearchableClipSettings.length">
              <div v-for="item in pagedSearchableClipSettings" :key="item.key" class="setting-row setting-list-row">
                <span class="setting-label-wrap">
                  <span class="setting-label">{{ t(item.labelKey) }}</span>
                  <n-tooltip v-if="item.hintKey" trigger="hover" placement="top">
                    <template #trigger>
                      <span class="hint-dot">?</span>
                    </template>
                    <div class="setting-hint-content">
                      <span>{{ t(item.hintKey) }}</span>
                      <img
                        v-if="item.key === 'pov_radar_enabled'"
                        :src="povRadarDemoImage"
                        alt="POV radar demonstration"
                        class="pov-radar-demo-image"
                      />
                    </div>
                  </n-tooltip>
                </span>
                <n-switch
                  v-if="item.kind === 'switch'"
                  :value="switchSettingValue(item)"
                  @update:value="updateSwitchSetting(item, $event)"
                />
                <n-input-number
                  v-else
                  :value="numberSettingValue(item)"
                  :min="item.min"
                  :max="item.max"
                  :step="item.step"
                  :precision="item.precision"
                  @update:value="updateNumberSetting(item, $event)"
                />
              </div>
            </template>
            <n-empty v-else size="small" :description="t('main.settings.recording_options_empty')" />
          </div>

          <div v-if="filteredSearchableClipSettings.length > SETTING_PAGE_SIZE" class="setting-list-pagination">
            <n-pagination
              v-model:page="settingPage"
              :page-size="SETTING_PAGE_SIZE"
              :item-count="filteredSearchableClipSettings.length"
              size="small"
            />
          </div>
        </div>
      </n-space>
    </n-card>

    <n-card size="small" :bordered="true" class="section-card">
      <template #header>
        <span class="section-title">{{ t("main.settings.recording_title") }}</span>
      </template>

      <n-space vertical :size="12">
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.record_fps") }}</span>
          <n-input-number v-model:value="settings.record_fps" :min="1" :max="240" :step="1" :precision="0" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.record_quality") }}</span>
          <n-select v-model:value="settings.record_quality" :options="recordQualityOptions" class="preset-select" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.video_preset") }}</span>
          <n-select v-model:value="settings.video_preset" :options="presetOptions" class="preset-select" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.launch_resolution") }}</span>
          <n-select v-model:value="settings.launch_resolution" :options="resolutionOptions" class="preset-select" />
        </div>
      </n-space>
    </n-card>

    <n-card size="small" :bordered="true" class="section-card">
      <template #header>
        <span class="section-title">{{ t("main.settings.editing_title") }}</span>
      </template>

      <n-space vertical :size="12">
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.edit_fps") }}</span>
          <n-input-number v-model:value="settings.edit_fps" :min="24" :max="240" :step="1" :precision="0" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.edit_quality") }}</span>
          <n-select v-model:value="settings.edit_quality" :options="editQualityOptions" class="preset-select" />
        </div>
      </n-space>
    </n-card>

    <StorageDirectoryCard
      :title="t('main.settings.outputs_title')"
      :primary-value="outputsStats.video_count"
      :primary-label="t('main.settings.outputs_video_count')"
      :total-size="formatBytes(outputsStats.total_size_bytes)"
      :total-size-label="t('main.settings.outputs_total_size')"
      :path-label="t('main.settings.outputs_dir')"
      :path="outputsStats.output_dir"
      :refresh-label="t('main.settings.outputs_refresh')"
      :open-label="t('main.settings.outputs_open')"
      :clear-label="t('main.settings.outputs_clear')"
      :loading="outputsLoading"
      :opening="openingOutputsDir"
      :clearing="clearingOutputs"
      :clear-disabled="storageBusy"
      @refresh="loadOutputsStats"
      @open="openOutputsDirectory"
      @clear="confirmClearOutputs"
    />

    <StorageDirectoryCard
      :title="t('main.settings.demo_title')"
      :primary-value="demoStats.demo_count"
      :primary-label="t('main.settings.demo_count')"
      :total-size="formatBytes(demoStats.total_size_bytes)"
      :total-size-label="t('main.settings.outputs_total_size')"
      :path-label="t('main.settings.outputs_dir')"
      :path="demoStats.demo_dir"
      :refresh-label="t('main.settings.outputs_refresh')"
      :open-label="t('main.settings.outputs_open')"
      :clear-label="t('main.settings.outputs_clear')"
      :loading="demoLoading"
      :opening="openingDemoDir"
      :clearing="clearingDemo"
      :clear-disabled="storageBusy"
      @refresh="loadDemoStats"
      @open="openDemoDirectory"
      @clear="confirmClearDemo"
    />

    <n-card v-if="debugEnabled" size="small" :bordered="true" class="section-card">
      <template #header>
        <span class="section-title">{{ t("main.settings.debug_title") }}</span>
      </template>

      <n-space vertical :size="12">
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.keep_produce_intermediates") }}</span>
          <n-switch v-model:value="keepProduceIntermediates" />
        </div>
        <div class="setting-row debug-file-row">
          <span class="setting-label">{{ t("main.settings.debug_plugin_dll") }}</span>
          <div class="debug-file-control">
            <n-tag size="small" :bordered="false" :type="debugPluginDLL.active ? 'warning' : 'default'" class="debug-file-path">
              {{ debugPluginDLL.path || t("main.settings.debug_plugin_dll_default") }}
            </n-tag>
            <n-button size="tiny" :loading="pickingDebugPluginDLL" @click="pickDebugPluginDLL">
              {{ t("main.settings.browse") }}
            </n-button>
            <n-button
              v-if="debugPluginDLL.active"
              size="tiny"
              tertiary
              :loading="clearingDebugPluginDLL"
              @click="clearDebugPluginDLL"
            >
              {{ t("main.settings.debug_plugin_dll_clear") }}
            </n-button>
          </div>
        </div>
      </n-space>
    </n-card>
  </n-space>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { useDialog, useMessage } from "naive-ui";
import { t } from "@/shared/i18n";
import { backend } from "@/shared/backend";
import povRadarDemoImage from "@/assets/images/pov-radar-demo.png";
import { useDebugSettings } from "@/shared/state/useDebugSettings";
import { useWorkActivity } from "@/shared/state/useWorkActivity";
import { useSettingsStore } from "@/domains/settings/settings-state";
import {
  SEARCHABLE_CLIP_SETTINGS,
  getSearchableNumberValue,
  getSearchableSwitchValue,
  normalizeSettingSearch,
  settingMatchesSearch,
  type SearchableClipSettingItem,
} from "@/domains/settings/settings-schema";
import { useSettingsDebug } from "@/features/settings/composables/useSettingsDebug";
import { useSettingsStorage } from "@/features/settings/composables/useSettingsStorage";
import StorageDirectoryCard from "./StorageDirectoryCard.vue";

const props = withDefaults(
  defineProps<{
    active?: boolean;
  }>(),
  {
    active: true,
  },
);

const { storageBusy } = useWorkActivity();

const settingsStore = useSettingsStore();
const settings = settingsStore.draftSettings;
const localErrorMessage = ref("");
const successMessage = ref("");
const settingSearchQuery = ref("");
const settingPage = ref(1);
let successTimer: ReturnType<typeof setTimeout> | null = null;

function clearSuccessTimer() {
  if (successTimer != null) {
    clearTimeout(successTimer);
    successTimer = null;
  }
}
const SETTING_PAGE_SIZE = 8;
const dialog = useDialog();
const message = useMessage();
const { debugEnabled, keepProduceIntermediates } = useDebugSettings();
const storage = useSettingsStorage(
  backend,
  () => props.active,
  () => storageBusy.value,
);
const {
  outputsStats,
  demoStats,
  outputsLoading,
  demoLoading,
  openingOutputsDir,
  openingDemoDir,
  clearingOutputs,
  clearingDemo,
  loadOutputsStats,
  loadDemoStats,
  openOutputsDirectory,
  openDemoDirectory,
  clearOutputsDirectory: clearStoredOutputsDirectory,
  clearDemoDirectory: clearStoredDemoDirectory,
} = storage;
const debug = useSettingsDebug(backend, () => props.active, debugEnabled);
const {
  debugPluginDLL,
  pickingDebugPluginDLL,
  clearingDebugPluginDLL,
  loadDebugPluginDLLOverride,
  pickDebugPluginDLL,
  clearDebugPluginDLL,
} = debug;
const errorMessage = computed(
  () => settingsStore.errorMessage.value || storage.errorMessage.value || debug.errorMessage.value || localErrorMessage.value,
);
const presetOptions = computed(() => [
  { label: t("main.settings.video_preset_auto"), value: "auto" },
  { label: t("main.settings.video_preset_c1"), value: "c1" },
  { label: t("main.settings.video_preset_n1"), value: "n1" },
  { label: t("main.settings.video_preset_a1"), value: "a1" },
  { label: t("main.settings.video_preset_i1"), value: "i1" },
]);
const resolutionOptions = computed(() => [
  { label: t("main.settings.resolution_16_9"), value: "16:9" },
  { label: t("main.settings.resolution_4_3"), value: "4:3" },
  { label: t("main.settings.resolution_4_3_1280x960"), value: "4:3_1280x960" },
]);
const recordQualityOptions = computed(() => [
  { label: t("main.settings.edit_quality_standard"), value: "standard" },
  { label: t("main.settings.edit_quality_high"), value: "high" },
  { label: t("main.settings.edit_quality_ultra"), value: "ultra" },
]);
const editQualityOptions = computed(() => [
  { label: t("main.settings.edit_quality_standard"), value: "standard" },
  { label: t("main.settings.edit_quality_high"), value: "high" },
  { label: t("main.settings.edit_quality_ultra"), value: "ultra" },
]);
const filteredSearchableClipSettings = computed(() => {
  const query = normalizeSettingSearch(settingSearchQuery.value);
  if (!query) return SEARCHABLE_CLIP_SETTINGS;
  return SEARCHABLE_CLIP_SETTINGS.filter((item) => settingMatchesSearch(item, query, t(item.labelKey)));
});
const pagedSearchableClipSettings = computed(() => {
  const start = (settingPage.value - 1) * SETTING_PAGE_SIZE;
  return filteredSearchableClipSettings.value.slice(start, start + SETTING_PAGE_SIZE);
});

watch(
  () => props.active,
  (active) => {
    if (!active) {
      void settingsStore.dispose();
      return;
    }
    void settingsStore.init();
    void loadOutputsStats();
    void loadDemoStats();
    if (debugEnabled.value) {
      void loadDebugPluginDLLOverride();
    }
  },
  { immediate: true },
);

watch(
  debugEnabled,
  (enabled) => {
    if (props.active && enabled) {
      void loadDebugPluginDLLOverride();
    }
  },
  { immediate: true },
);

watch(settingsStore.lastSavedVersion, (version, previousVersion) => {
  if (version <= previousVersion) {
    return;
  }
  clearSuccessTimer();
  successMessage.value = t("main.settings.saved");
  successTimer = setTimeout(() => {
    successMessage.value = "";
    successTimer = null;
  }, 3000);
});

watch(settingSearchQuery, () => {
  settingPage.value = 1;
});

watch(
  () => filteredSearchableClipSettings.value.length,
  (total) => {
    const maxPage = Math.max(1, Math.ceil(total / SETTING_PAGE_SIZE));
    if (settingPage.value > maxPage) {
      settingPage.value = maxPage;
    }
    if (settingPage.value < 1) {
      settingPage.value = 1;
    }
  },
);

onBeforeUnmount(() => {
  void settingsStore.dispose();
  clearSuccessTimer();
});

function dismissError(): void {
  localErrorMessage.value = "";
  settingsStore.clearError();
  storage.errorMessage.value = "";
  debug.errorMessage.value = "";
}

function confirmClearOutputs() {
  dialog.warning({
    title: t("main.settings.outputs_clear_confirm_title"),
    content: t("main.settings.outputs_clear_confirm_content", {
      count: outputsStats.video_count,
      size: formatBytes(outputsStats.total_size_bytes),
    }),
    positiveText: t("main.settings.outputs_clear_confirm_positive"),
    negativeText: t("main.settings.outputs_clear_confirm_negative"),
    onPositiveClick: () => {
      void clearOutputsDirectory();
    },
  });
}

function confirmClearDemo() {
  dialog.warning({
    title: t("main.settings.demo_clear_confirm_title"),
    content: t("main.settings.demo_clear_confirm_content", {
      count: demoStats.demo_count,
      size: formatBytes(demoStats.total_size_bytes),
    }),
    positiveText: t("main.settings.outputs_clear_confirm_positive"),
    negativeText: t("main.settings.outputs_clear_confirm_negative"),
    onPositiveClick: () => {
      void clearDemoDirectory();
    },
  });
}

async function clearOutputsDirectory() {
  if (clearingOutputs.value || storageBusy.value) {
    return;
  }
  localErrorMessage.value = "";
  storage.errorMessage.value = "";
  successMessage.value = "";
  await clearStoredOutputsDirectory();
  if (!storage.errorMessage.value) {
    message.success(t("main.settings.outputs_clear_success"));
  }
}

async function clearDemoDirectory() {
  if (clearingDemo.value || storageBusy.value) {
    return;
  }
  localErrorMessage.value = "";
  storage.errorMessage.value = "";
  successMessage.value = "";
  await clearStoredDemoDirectory();
  if (!storage.errorMessage.value) {
    message.success(t("main.settings.demo_clear_success"));
  }
}

function switchSettingValue(item: SearchableClipSettingItem): boolean {
  return getSearchableSwitchValue(settings, item);
}

function numberSettingValue(item: SearchableClipSettingItem): number {
  return getSearchableNumberValue(settings, item);
}

function updateSwitchSetting(item: SearchableClipSettingItem, value: boolean): void {
  if (item.kind !== "switch") {
    return;
  }
  settings[item.key] = value;
}

function updateNumberSetting(item: SearchableClipSettingItem, value: number | null): void {
  if (item.kind !== "number" || typeof value !== "number" || !Number.isFinite(value)) {
    return;
  }
  settings[item.key] = value;
}

function formatBytes(bytes: number): string {
  const safeBytes = Number.isFinite(bytes) && bytes > 0 ? bytes : 0;
  if (safeBytes < 1024) {
    return `${safeBytes} B`;
  }
  const units = ["KB", "MB", "GB", "TB"];
  let value = safeBytes / 1024;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex++;
  }
  return `${value >= 100 ? value.toFixed(0) : value.toFixed(1)} ${units[unitIndex]}`;
}
</script>

<style scoped>
.section-card {
  background: #1a1e1b;
}

.section-title {
  font-size: 13px;
  font-weight: 600;
}

.setting-row {
  align-items: center;
  display: flex;
  gap: 12px;
  justify-content: space-between;
}

.setting-label {
  color: #c9d3cb;
  font-size: 13px;
}

.setting-label-wrap {
  align-items: center;
  display: inline-flex;
  gap: 6px;
  min-width: 0;
}

.setting-hint-content {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.pov-radar-demo-image {
  display: block;
  height: auto;
  max-width: min(240px, 50vw);
  width: 100%;
  border-radius: 10px;
}

.hint-dot {
  align-items: center;
  border: 1px solid #516056;
  border-radius: 50%;
  color: #9cb8a8;
  cursor: help;
  display: inline-flex;
  flex: 0 0 auto;
  font-size: 11px;
  height: 18px;
  justify-content: center;
  user-select: none;
  width: 18px;
}

.preset-select {
  width: 220px;
}

.setting-list-panel {
  background: #161a17;
  border: 1px solid #2f3631;
  border-radius: 8px;
  padding: 12px;
}

.setting-list-head {
  align-items: center;
  display: flex;
  gap: 12px;
  justify-content: space-between;
}

.setting-list-title-row {
  align-items: baseline;
  display: flex;
  gap: 8px;
  min-width: 0;
}

.setting-list-title {
  color: #dbe5dd;
  font-size: 13px;
  font-weight: 600;
}

.setting-list-controls {
  align-items: center;
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}

.setting-search {
  width: 220px;
}

.setting-list-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
}

.setting-list-row {
  background: #1c211d;
  border: 1px solid #303932;
  border-radius: 6px;
  min-height: 38px;
  padding: 8px 10px;
}

.setting-list-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 10px;
}

.debug-file-row {
  align-items: flex-start;
}

.debug-file-control {
  align-items: center;
  display: flex;
  flex: 1;
  gap: 8px;
  justify-content: flex-end;
  min-width: 0;
}

.debug-file-path {
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 720px) {
  .setting-row,
  .debug-file-control {
    align-items: stretch;
    flex-direction: column;
  }

  .setting-list-head,
  .setting-list-controls,
  .setting-list-title-row {
    align-items: stretch;
    flex-direction: column;
  }

  .setting-list-pagination {
    justify-content: center;
  }

  .debug-file-path,
  .preset-select,
  .setting-search {
    max-width: 100%;
    width: 100%;
  }
}

</style>
