<template>
  <n-space vertical :size="14">
    <n-card size="small" :bordered="true" class="section-card">
      <template #header>
        <span class="section-title">{{ t("main.settings.clip_title") }}</span>
      </template>

      <n-space vertical :size="12">
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.killer_pre_seconds") }}</span>
          <n-input-number v-model:value="settings.killer_pre_seconds" :min="1" :max="5" :step="0.5" :precision="1" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.killer_post_seconds") }}</span>
          <n-input-number v-model:value="settings.killer_post_seconds" :min="1" :max="5" :step="0.5" :precision="1" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.victim_pre_seconds") }}</span>
          <n-input-number v-model:value="settings.victim_pre_seconds" :min="1" :max="2" :step="0.5" :precision="1" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.victim_post_seconds") }}</span>
          <n-input-number v-model:value="settings.victim_post_seconds" :min="1" :max="2" :step="0.5" :precision="1" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.auto_add_victim") }}</span>
          <n-switch v-model:value="settings.auto_add_victim_view" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.enable_voice") }}</span>
          <n-switch v-model:value="settings.enable_voice" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.enable_spec_show_xray_zero") }}</span>
          <n-switch v-model:value="settings.enable_spec_show_xray_zero" />
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.hide_all_ui") }}</span>
          <n-switch v-model:value="settings.hide_all_ui" />
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
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.pov_hud_enabled") }}</span>
          <n-switch v-model:value="settings.pov_hud_enabled" />
        </div>
        <n-alert v-if="settings.pov_hud_enabled" type="warning" :bordered="false" style="white-space: pre-line; margin-top: 12px;">
          {{ t("main.settings.pov_hud_warning") }}
        </n-alert>
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

    <n-card size="small" :bordered="true" class="section-card">
      <template #header>
        <span class="section-title">{{ t("main.settings.log_title") }}</span>
      </template>
      <n-space vertical :size="8">
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.log_count") }}</span>
          <span>{{ logStats.log_count }}</span>
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.log_total_size") }}</span>
          <span>{{ formatBytes(logStats.total_size_bytes) }}</span>
        </div>
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.outputs_dir") }}</span>
          <span class="setting-path">{{ logStats.log_dir || "—" }}</span>
        </div>
        <n-button size="small" :loading="clearingLogs" @click="confirmClearLogs">{{ t("main.settings.log_clear") }}</n-button>
      </n-space>
    </n-card>

    <n-card v-if="debugEnabled" size="small" :bordered="true" class="section-card">
      <template #header>
        <span class="section-title">{{ t("main.settings.debug_title") }}</span>
      </template>

      <n-space vertical :size="12">
        <div class="setting-row">
          <span class="setting-label">{{ t("main.settings.keep_produce_intermediates") }}</span>
          <n-switch v-model:value="keepProduceIntermediates" />
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
import type { ClipSettings, DemoStorageStats, LogStorageStats, OutputsStorageStats } from "@/shared/types";
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
const errorMessage = ref("");
const successMessage = ref("");
const syncingSettings = ref(false);
const hasPendingSave = ref(false);
let autoSaveTimer: ReturnType<typeof setTimeout> | null = null;
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
  pov_hud_enabled: false,
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
const logStats = reactive<LogStorageStats>({
  log_dir: "",
  log_count: 0,
  total_size_bytes: 0,
});
const clearingLogs = ref(false);
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
    void loadLogStats();
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

async function loadLogStats() {
  if (!props.active) {
    return;
  }
  errorMessage.value = "";
  try {
    const next = await callBackend<LogStorageStats>("GetLogStorageStats");
    Object.assign(logStats, next);
  } catch (err: unknown) {
    // 日志统计加载失败不报错（目录可能不存在）
  }
}

function confirmClearLogs() {
  dialog.warning({
    title: t("main.settings.log_clear_confirm_title"),
    content: t("main.settings.log_clear_confirm_content", {
      count: logStats.log_count,
      size: formatBytes(logStats.total_size_bytes),
    }),
    positiveText: t("main.settings.outputs_clear_confirm_positive"),
    negativeText: t("main.settings.outputs_clear_confirm_negative"),
    onPositiveClick: () => {
      void clearLogs();
    },
  });
}

async function clearLogs() {
  if (clearingLogs.value) {
    return;
  }
  clearingLogs.value = true;
  errorMessage.value = "";
  successMessage.value = "";
  try {
    const next = await callBackend<LogStorageStats>("ClearLogs");
    Object.assign(logStats, next);
    message.success(t("main.settings.log_clear_success"));
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    clearingLogs.value = false;
  }
}

async function applySettingsFromBackend(next: ClipSettings) {
  syncingSettings.value = true;
  Object.assign(settings, next);
  await nextTick();
  syncingSettings.value = false;
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

.preset-select {
  width: 220px;
}

</style>
