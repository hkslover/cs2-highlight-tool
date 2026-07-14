<template>
  <n-space vertical :size="14">
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
                    {{ t(item.hintKey) }}
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

    <n-alert v-if="errorMessage" type="error" :bordered="false">{{ errorMessage }}</n-alert>
    <n-alert v-if="successMessage" type="success" :bordered="false">{{ successMessage }}</n-alert>
  </n-space>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from "vue";
import { useDialog, useMessage } from "naive-ui";
import { t } from "@/shared/i18n";
import { CLIP_SETTINGS_SAVED_EVENT } from "@/shared/events";
import { useDebugSettings } from "@/shared/state/useDebugSettings";
import type { ClipSettings, DebugPluginDLLOverrideState, DemoStorageStats, OutputsStorageStats } from "@/shared/types";
import StorageDirectoryCard from "./StorageDirectoryCard.vue";

const props = withDefaults(
  defineProps<{
    active?: boolean;
  }>(),
  {
    active: true,
  },
);

const AUTO_SAVE_DELAY_MS = 500;
const saving = ref(false);
const outputsLoading = ref(false);
const demoLoading = ref(false);
const openingOutputsDir = ref(false);
const openingDemoDir = ref(false);
const clearingOutputs = ref(false);
const clearingDemo = ref(false);
const pickingDebugPluginDLL = ref(false);
const clearingDebugPluginDLL = ref(false);
const errorMessage = ref("");
const successMessage = ref("");
const syncingSettings = ref(false);
const hasPendingSave = ref(false);
const settingSearchQuery = ref("");
const settingPage = ref(1);
let autoSaveTimer: ReturnType<typeof setTimeout> | null = null;
const SETTING_PAGE_SIZE = 8;
const dialog = useDialog();
const message = useMessage();
const { debugEnabled, keepProduceIntermediates } = useDebugSettings();
const settings = reactive<ClipSettings>({
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
  use_shoulder_camera: false,
  pov_hud_enabled: true,
  pov_radar_enabled: false,
  sky_blackout: true,
  disable_clouds: false,
  kill_feed_lifetime: 4,
  block_kill_feed: false,
});
const outputsStats = reactive<OutputsStorageStats>({
  output_dir: "",
  video_count: 0,
  total_size_bytes: 0,
});
const demoStats = reactive<DemoStorageStats>({
  demo_dir: "",
  demo_count: 0,
  total_size_bytes: 0,
});
const debugPluginDLL = reactive<DebugPluginDLLOverrideState>({
  active: false,
  path: "",
});
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
type SearchableSwitchSettingKey =
  | "enable_voice"
  | "enable_spec_show_xray_zero"
  | "hide_all_ui"
  | "use_shoulder_camera"
  | "pov_hud_enabled"
  | "pov_radar_enabled"
  | "sky_blackout"
  | "disable_clouds"
  | "block_kill_feed";
type SearchableNumberSettingKey = "kill_feed_lifetime";
type SearchableClipSettingItem =
  | {
      key: SearchableSwitchSettingKey;
      labelKey: string;
      kind: "switch";
      aliases: string[];
      hintKey?: string;
    }
  | {
      key: SearchableNumberSettingKey;
      labelKey: string;
      kind: "number";
      aliases: string[];
      hintKey?: string;
      min: number;
      max: number;
      step: number;
      precision: number;
    };
const searchableClipSettings: SearchableClipSettingItem[] = [
  {
    key: "enable_voice",
    labelKey: "main.settings.enable_voice",
    kind: "switch",
    aliases: ["启用队伍语音", "队伍语音", "team voice", "voice"],
  },
  {
    key: "enable_spec_show_xray_zero",
    labelKey: "main.settings.enable_spec_show_xray_zero",
    kind: "switch",
    aliases: ["关闭x光", "关闭 x 光", "xray", "x-ray", "x光", "spec_show_xray"],
  },
  {
    key: "hide_all_ui",
    labelKey: "main.settings.hide_all_ui",
    kind: "switch",
    aliases: ["隐藏所有ui", "hidden ui", "hide ui"],
  },
  {
    key: "use_shoulder_camera",
    labelKey: "main.settings.use_shoulder_camera",
    kind: "switch",
    aliases: ["越肩视角", "shoulder camera", "camera"],
  },
  {
    key: "pov_hud_enabled",
    labelKey: "main.settings.pov_hud_enabled",
    kind: "switch",
    aliases: ["pov hud", "hud"],
  },
  {
    key: "pov_radar_enabled",
    labelKey: "main.settings.pov_radar_enabled",
    kind: "switch",
    aliases: ["pov雷达", "pov radar", "radar"],
    hintKey: "main.settings.pov_radar_hint",
  },
  {
    key: "sky_blackout",
    labelKey: "main.settings.sky_blackout",
    kind: "switch",
    aliases: ["天空变黑", "sky", "blackout", "drawskybox"],
  },
  {
    key: "disable_clouds",
    labelKey: "main.settings.disable_clouds",
    kind: "switch",
    aliases: ["关闭云层", "cloud", "clouds"],
  },
  {
    key: "kill_feed_lifetime",
    labelKey: "main.settings.kill_feed_lifetime",
    kind: "number",
    aliases: ["击杀信息留存", "kill feed", "death notice", "deathnotice"],
    min: 1,
    max: 10,
    step: 1,
    precision: 0,
  },
  {
    key: "block_kill_feed",
    labelKey: "main.settings.block_kill_feed",
    kind: "switch",
    aliases: ["屏蔽击杀信息", "block kill feed", "kill feed"],
  },
];
const filteredSearchableClipSettings = computed(() => {
  const query = normalizeSettingSearch(settingSearchQuery.value);
  if (!query) return searchableClipSettings;
  return searchableClipSettings.filter((item) => settingMatchesSearch(item, query));
});
const pagedSearchableClipSettings = computed(() => {
  const start = (settingPage.value - 1) * SETTING_PAGE_SIZE;
  return filteredSearchableClipSettings.value.slice(start, start + SETTING_PAGE_SIZE);
});

watch(
  () => props.active,
  (active) => {
    if (!active) {
      clearAutoSaveTimer();
      return;
    }
    void loadSettings();
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

watch(
  settings,
  () => {
    scheduleAutoSave();
  },
  { deep: true },
);

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
  clearAutoSaveTimer();
});

async function callBackend<T>(method: string, ...args: unknown[]): Promise<T> {
  const api = (window as any).go?.app?.App as Record<string, (...a: unknown[]) => Promise<unknown>> | undefined;
  const fn = api?.[method];
  if (!fn) throw new Error(`Wails API not loaded: ${method}`);
  return fn(...args) as Promise<T>;
}

async function loadSettings() {
  clearAutoSaveTimer();
  errorMessage.value = "";
  successMessage.value = "";
  try {
    const next = await callBackend<ClipSettings>("GetClipSettings");
    await applySettingsFromBackend(next);
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  }
}

async function loadOutputsStats() {
  if (!props.active || outputsLoading.value) {
    return;
  }
  outputsLoading.value = true;
  errorMessage.value = "";
  try {
    const next = await callBackend<OutputsStorageStats>("GetOutputsStorageStats");
    Object.assign(outputsStats, next);
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    outputsLoading.value = false;
  }
}

async function loadDemoStats() {
  if (!props.active || demoLoading.value) {
    return;
  }
  demoLoading.value = true;
  errorMessage.value = "";
  try {
    const next = await callBackend<DemoStorageStats>("GetDemoStorageStats");
    Object.assign(demoStats, next);
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    demoLoading.value = false;
  }
}

async function loadDebugPluginDLLOverride() {
  if (!props.active || !debugEnabled.value) {
    return;
  }
  errorMessage.value = "";
  try {
    const next = await callBackend<DebugPluginDLLOverrideState>("GetDebugPluginDLLOverride");
    Object.assign(debugPluginDLL, next);
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  }
}

async function pickDebugPluginDLL() {
  if (pickingDebugPluginDLL.value) {
    return;
  }
  pickingDebugPluginDLL.value = true;
  errorMessage.value = "";
  successMessage.value = "";
  try {
    const next = await callBackend<DebugPluginDLLOverrideState>("PickDebugPluginDLLOverride");
    Object.assign(debugPluginDLL, next);
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    pickingDebugPluginDLL.value = false;
  }
}

async function clearDebugPluginDLL() {
  if (clearingDebugPluginDLL.value) {
    return;
  }
  clearingDebugPluginDLL.value = true;
  errorMessage.value = "";
  successMessage.value = "";
  try {
    const next = await callBackend<DebugPluginDLLOverrideState>("ClearDebugPluginDLLOverride");
    Object.assign(debugPluginDLL, next);
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    clearingDebugPluginDLL.value = false;
  }
}

async function openOutputsDirectory() {
  if (openingOutputsDir.value) {
    return;
  }
  openingOutputsDir.value = true;
  errorMessage.value = "";
  try {
    await callBackend<void>("OpenOutputsDirectory");
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    openingOutputsDir.value = false;
  }
}

async function openDemoDirectory() {
  if (openingDemoDir.value) {
    return;
  }
  openingDemoDir.value = true;
  errorMessage.value = "";
  try {
    await callBackend<void>("OpenDemoDirectory");
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    openingDemoDir.value = false;
  }
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
  if (clearingOutputs.value) {
    return;
  }
  clearingOutputs.value = true;
  errorMessage.value = "";
  successMessage.value = "";
  try {
    const next = await callBackend<OutputsStorageStats>("ClearOutputsDirectory");
    Object.assign(outputsStats, next);
    message.success(t("main.settings.outputs_clear_success"));
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    clearingOutputs.value = false;
  }
}

async function clearDemoDirectory() {
  if (clearingDemo.value) {
    return;
  }
  clearingDemo.value = true;
  errorMessage.value = "";
  successMessage.value = "";
  try {
    const next = await callBackend<DemoStorageStats>("ClearDemoDirectory");
    Object.assign(demoStats, next);
    message.success(t("main.settings.demo_clear_success"));
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    clearingDemo.value = false;
  }
}

async function applySettingsFromBackend(next: ClipSettings) {
  syncingSettings.value = true;
  Object.assign(settings, next);
  await nextTick();
  syncingSettings.value = false;
}

function normalizeSettingSearch(value: string): string {
  return value.trim().toLowerCase().replace(/\s+/g, "");
}

function settingMatchesSearch(item: SearchableClipSettingItem, query: string): boolean {
  const haystack = normalizeSettingSearch([t(item.labelKey), item.key, ...item.aliases].join(" "));
  if (haystack.includes(query)) {
    return true;
  }
  let queryIndex = 0;
  for (const char of haystack) {
    if (char === query[queryIndex]) {
      queryIndex++;
    }
    if (queryIndex === query.length) {
      return true;
    }
  }
  return false;
}

function switchSettingValue(item: SearchableClipSettingItem): boolean {
  if (item.kind !== "switch") {
    return false;
  }
  return settings[item.key];
}

function numberSettingValue(item: SearchableClipSettingItem): number {
  if (item.kind !== "number") {
    return 0;
  }
  return settings[item.key];
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

function clearAutoSaveTimer() {
  if (autoSaveTimer == null) {
    return;
  }
  clearTimeout(autoSaveTimer);
  autoSaveTimer = null;
}

function scheduleAutoSave() {
  if (!props.active || syncingSettings.value) {
    return;
  }
  clearAutoSaveTimer();
  errorMessage.value = "";
  successMessage.value = "";
  autoSaveTimer = setTimeout(() => {
    autoSaveTimer = null;
    void saveSettings();
  }, AUTO_SAVE_DELAY_MS);
}

async function saveSettings() {
  if (!props.active || syncingSettings.value) {
    return;
  }
  if (saving.value) {
    hasPendingSave.value = true;
    return;
  }
  saving.value = true;
  errorMessage.value = "";
  try {
    const saved = await callBackend<ClipSettings>("SaveClipSettings", settings);
    await applySettingsFromBackend(saved);
    window.dispatchEvent(new CustomEvent(CLIP_SETTINGS_SAVED_EVENT));
    successMessage.value = t("main.settings.saved");
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    saving.value = false;
    if (hasPendingSave.value) {
      hasPendingSave.value = false;
      void saveSettings();
    }
  }
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
