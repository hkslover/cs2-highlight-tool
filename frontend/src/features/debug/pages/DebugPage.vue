<template>
  <div class="debug-page">
    <div class="debug-scroll">
      <div class="debug-grid">
        <n-card size="small" :bordered="true" class="debug-card">
          <template #header>
            <span class="card-title">{{ t("main.debug.launch_title") }}</span>
          </template>

          <n-space vertical :size="12">
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.inject_plugin") }}</span>
              <div class="setting-control">
                <n-switch v-model:value="launch.injectPlugin" :disabled="launchLocked" />
              </div>
            </div>

            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.pov_hud") }}</span>
              <div class="setting-control">
                <n-switch v-model:value="launch.povHud" :disabled="launchLocked" />
              </div>
            </div>

            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.brand_watermark") }}</span>
              <div class="setting-control">
                <n-switch v-model:value="launch.brandVpk" :disabled="launchLocked" />
              </div>
            </div>

            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.pov_radar") }}</span>
              <div class="setting-control">
                <n-switch
                  :value="settingsDraft.pov_radar_enabled"
                  :disabled="radarDisabled"
                  @update:value="handleLaunchRadarChange"
                />
                <span v-if="radarRequiresPlugin" class="setting-hint">
                  {{ t("main.debug.pov_radar_requires_plugin") }}
                </span>
              </div>
            </div>

            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.resolution") }}</span>
              <div class="setting-control">
                <n-select
                  v-model:value="launch.resolution"
                  :options="resolutionOptions"
                  :disabled="launchLocked"
                  class="control-select"
                />
              </div>
            </div>

            <div class="action-row">
              <n-button
                type="primary"
                :loading="launching"
                :disabled="!canLaunch"
                @click="handleLaunch"
              >
                {{ t("main.debug.btn_launch") }}
              </n-button>
              <n-button
                secondary
                :loading="stopping"
                :disabled="!canStop"
                @click="handleStop"
              >
                {{ t("main.debug.btn_stop") }}
              </n-button>
            </div>
          </n-space>
        </n-card>

        <n-card size="small" :bordered="true" class="debug-card">
          <template #header>
            <span class="card-title">{{ t("main.debug.session_title") }}</span>
          </template>

          <n-space vertical :size="12">
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.session_state") }}</span>
              <n-tag size="small" :bordered="false" :type="statusTagType">
                {{ t(statusLabelKey) }}
              </n-tag>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.process_state") }}</span>
              <n-tag size="small" :bordered="false">
                {{ sessionState.process_started ? t("main.debug.process_started") : t("main.debug.process_stopped") }}
              </n-tag>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.cs2_pid") }}</span>
              <span class="setting-value">{{ sessionState.cs2_pid > 0 ? sessionState.cs2_pid : t("main.debug.value_none") }}</span>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.plugin_state") }}</span>
              <n-tag size="small" :bordered="false" :type="pluginTagType">
                {{ pluginTagLabel }}
              </n-tag>
            </div>
            <span v-if="pluginDetail" class="setting-hint">{{ pluginDetail }}</span>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.session_id") }}</span>
              <span class="setting-value setting-value--path">{{ sessionState.session_id || t("main.debug.value_none") }}</span>
            </div>
            <n-alert v-if="sessionError" type="error" :bordered="false" size="small">
              {{ sessionError }}
            </n-alert>
            <span v-if="!initialized && !sessionError" class="setting-hint">{{ t("main.debug.session_hint") }}</span>
          </n-space>
        </n-card>

        <n-card size="small" :bordered="true" class="debug-card">
          <template #header>
            <span class="card-title">{{ t("main.debug.demo_title") }}</span>
          </template>

          <n-space vertical :size="10">
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.demo_select") }}</span>
              <n-select
                :value="debugDemoKey"
                :options="demoOptions"
                :disabled="demoOptions.length === 0"
                :placeholder="t('main.debug.demo_empty')"
                class="control-select control-select--wide"
                @update:value="selectDebugDemo"
              />
            </div>

            <div v-if="selectedDebugDemo" class="demo-info">
              <div class="setting-row">
                <span class="setting-label">{{ t("main.debug.demo_name") }}</span>
                <span class="setting-value">{{ selectedDebugDemo.file_name }}</span>
              </div>
              <div class="setting-row">
                <span class="setting-label">{{ t("main.debug.demo_path") }}</span>
                <span class="setting-value setting-value--path" :title="selectedDebugDemo.file_path">
                  {{ selectedDebugDemo.file_path }}
                </span>
                <n-button
                  size="tiny"
                  secondary
                  :title="t('main.debug.btn_open_folder')"
                  :disabled="!selectedDebugDemo.file_path"
                  @click="handleOpenDemoFolder"
                >
                  {{ t("main.debug.btn_open_folder") }}
                </n-button>
              </div>
            </div>
            <n-empty v-else size="small" :description="t('main.debug.demo_empty')" />

            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.demo_playback_state") }}</span>
              <n-tag size="small" :bordered="false" :type="playbackTagType">
                {{ playbackLabel }}
              </n-tag>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.demo_playing_path") }}</span>
              <span class="setting-value setting-value--path" :title="sessionState.demo_path">
                {{ sessionState.demo_path || t("main.debug.value_none") }}
              </span>
            </div>

            <div class="action-row">
              <n-button size="small" secondary @click="goToImport">
                {{ t("main.debug.btn_back_import") }}
              </n-button>
              <n-button
                size="small"
                type="primary"
                :loading="playingRequest"
                :disabled="!canPlay"
                @click="handlePlayDemo"
              >
                {{ t("main.debug.btn_play_demo") }}
              </n-button>
              <n-button
                size="small"
                :type="pauseButtonType"
                :loading="pauseRequesting"
                :disabled="!canTogglePause"
                @click="handleTogglePause"
              >
                {{ pauseButtonLabel }}
              </n-button>
            </div>
            <span v-if="!canPlay && playDisabledHint" class="setting-hint">{{ playDisabledHint }}</span>
            <span v-if="!canTogglePause && pauseDisabledHint" class="setting-hint">{{ pauseDisabledHint }}</span>
            <n-alert v-if="playError" type="error" :bordered="false" size="small">
              {{ playError }}
            </n-alert>
            <n-alert v-if="folderError" type="error" :bordered="false" size="small">
              {{ folderError }}
            </n-alert>
            <n-alert v-if="pauseError" type="error" :bordered="false" size="small">
              {{ pauseError }}
            </n-alert>
            <div v-if="lastPlayResult" class="command-result">
              <n-tag
                size="small"
                :bordered="false"
                :type="lastPlayResult.status === 'executed' ? 'success' : 'error'"
              >
                {{ lastPlayResult.status === "executed" ? t("main.debug.play_executed") : t("main.debug.play_failed") }}
              </n-tag>
              <span v-if="lastPlayResult.error" class="setting-hint">
                {{ lastPlayResult.error }}
              </span>
            </div>
          </n-space>
        </n-card>

        <n-card size="small" :bordered="true" class="debug-card debug-card--wide">
          <template #header>
            <span class="card-title">{{ t("main.debug.live_settings_title") }}</span>
          </template>
          <template #header-extra>
            <n-space :size="8" align="center">
              <n-button
                size="tiny"
                secondary
                :loading="settingsLoading"
                :disabled="settingsLoading || applyingSettings"
                @click="handleReloadSettings"
              >
                {{ t("main.debug.live_settings_reload") }}
              </n-button>
              <n-button
                size="tiny"
                type="primary"
                :loading="applyingAll"
                :disabled="!canApplyAll"
                @click="handleApplyAll"
              >
                {{ t("main.debug.live_settings_apply_all") }}
              </n-button>
            </n-space>
          </template>

          <n-space vertical :size="10">
            <span class="setting-hint">{{ t("main.debug.live_settings_hint") }}</span>

            <n-alert v-if="settingsReadError" type="error" :bordered="false" size="small">
              {{ settingsReadError }}
            </n-alert>
            <n-alert v-if="settingsPlanError" type="warning" :bordered="false" size="small">
              {{ settingsPlanError }}
            </n-alert>
            <n-alert v-if="applyError" type="error" :bordered="false" size="small">
              {{ applyError }}
            </n-alert>

            <div class="setting-list">
              <div v-for="row in liveSettingRows" :key="row.key" class="setting-field">
                <div class="setting-row">
                  <span class="setting-label">{{ t(row.labelKey) }}</span>
                  <div class="setting-control">
                    <n-switch
                      :value="settingsDraft[row.key]"
                      :disabled="!canToggleSetting"
                      @update:value="(value: boolean) => handleSettingToggle(row.key, value)"
                    />
                  </div>
                </div>
                <span v-if="row.hintKey" class="setting-hint">{{ t(row.hintKey) }}</span>
                <span v-if="row.key === 'pov_radar_enabled' && sessionRunningWithoutPlugin" class="setting-hint">
                  {{ t("main.debug.pov_radar_requires_plugin") }}
                </span>
                <span
                  class="setting-command"
                  :class="{ 'setting-command--empty': !rowPreview(row.key) }"
                >
                  {{ rowPreview(row.key) || t("main.debug.value_none") }}
                </span>
              </div>

              <div class="setting-field">
                <div class="setting-row">
                  <span class="setting-label">{{ t("main.settings.kill_feed_lifetime") }}</span>
                  <div class="setting-control">
                    <n-input-number
                      :value="settingsDraft.kill_feed_lifetime"
                      :min="1"
                      :max="10"
                      :step="1"
                      :precision="0"
                      :disabled="!settingsLoaded"
                      class="control-number"
                      @update:value="handleLifetimeChange"
                    />
                    <n-button
                      size="tiny"
                      secondary
                      :disabled="!canToggleSetting"
                      @click="handleApplyLifetime"
                    >
                      {{ t("main.debug.apply") }}
                    </n-button>
                  </div>
                </div>
                <span class="setting-command">
                  {{ rowPreview("kill_feed_lifetime") || t("main.debug.value_none") }}
                </span>
              </div>
            </div>

            <span v-if="!canSend" class="setting-hint">{{ commandDisabledHint }}</span>
          </n-space>
        </n-card>

        <n-card size="small" :bordered="true" class="debug-card">
          <template #header>
            <span class="card-title">{{ t("main.debug.recording_settings_title") }}</span>
          </template>

          <n-space vertical :size="10">
            <span class="setting-hint">{{ t("main.debug.recording_settings_hint") }}</span>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.settings.record_fps") }}</span>
              <div class="setting-control">
                <n-input-number
                  :value="settingsDraft.record_fps"
                  :min="1"
                  :max="240"
                  :step="1"
                  :precision="0"
                  :disabled="!settingsLoaded"
                  class="control-number"
                  @update:value="handleRecordFPSChange"
                />
              </div>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.settings.record_quality") }}</span>
              <div class="setting-control">
                <n-select
                  :value="settingsDraft.record_quality"
                  :options="recordQualityOptions"
                  :disabled="!settingsLoaded"
                  class="control-select"
                  @update:value="handleRecordQualityChange"
                />
              </div>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.settings.video_preset") }}</span>
              <div class="setting-control">
                <n-select
                  :value="settingsDraft.video_preset"
                  :options="videoPresetOptions"
                  :disabled="!settingsLoaded"
                  class="control-select"
                  @update:value="handleVideoPresetChange"
                />
              </div>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.effective_preset") }}</span>
              <span class="setting-value">{{ effectivePresetLabel }}</span>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.recording_output_dir") }}</span>
              <span class="setting-value setting-value--path" :title="settingsPlan?.record_output_dir">
                {{ settingsPlan?.record_output_dir || t("main.debug.value_none") }}
              </span>
            </div>
            <div class="action-row">
              <n-button
                size="small"
                type="primary"
                :loading="applyingRecording"
                :disabled="!canApplyRecording"
                @click="handleApplyRecording"
              >
                {{ t("main.debug.recording_apply") }}
              </n-button>
              <span v-if="!canApplyRecording" class="setting-hint">{{ commandDisabledHint }}</span>
            </div>
            <n-alert v-if="recordingApplyError" type="error" :bordered="false" size="small">
              {{ recordingApplyError }}
            </n-alert>
            <div v-if="settingsPlan && settingsPlan.recording.length > 0" class="command-preview">
              <span class="setting-label">{{ t("main.debug.command_preview") }}</span>
              <div class="command-preview__list">
                <span v-for="command in settingsPlan.recording" :key="command" class="setting-command">
                  {{ command }}
                </span>
              </div>
            </div>
          </n-space>
        </n-card>

        <n-card size="small" :bordered="true" class="debug-card">
          <template #header>
            <span class="card-title">{{ t("main.debug.formal_title") }}</span>
          </template>

          <n-space vertical :size="10">
            <span class="setting-hint">{{ t("main.debug.formal_hint") }}</span>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.settings.edit_fps") }}</span>
              <span class="setting-value">{{ formalSettings?.edit_fps ?? t("main.debug.value_none") }}</span>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.settings.edit_quality") }}</span>
              <span class="setting-value">{{ qualityLabel(formalSettings?.edit_quality) }}</span>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.settings.killer_pre_seconds") }}</span>
              <span class="setting-value">{{ formalSettings?.killer_pre_seconds ?? t("main.debug.value_none") }}</span>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.settings.killer_post_seconds") }}</span>
              <span class="setting-value">{{ formalSettings?.killer_post_seconds ?? t("main.debug.value_none") }}</span>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.settings.victim_pre_seconds") }}</span>
              <span class="setting-value">{{ formalSettings?.victim_pre_seconds ?? t("main.debug.value_none") }}</span>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.settings.victim_post_seconds") }}</span>
              <span class="setting-value">{{ formalSettings?.victim_post_seconds ?? t("main.debug.value_none") }}</span>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.settings.auto_add_victim") }}</span>
              <span class="setting-value">
                {{ formalSettings?.auto_add_victim_view ? t("main.debug.value_enabled") : t("main.debug.value_disabled") }}
              </span>
            </div>
            <span class="setting-hint">{{ t("main.debug.formal_clip_hint") }}</span>
          </n-space>
        </n-card>

        <n-card size="small" :bordered="true" class="debug-card debug-card--wide">
          <template #header>
            <span class="card-title">{{ t("main.debug.cmd_title") }}</span>
          </template>

          <n-space vertical :size="10">
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.quick_title") }}</span>
              <n-select
                :value="presetKey"
                :options="presetOptions"
                :placeholder="t('main.debug.quick_placeholder')"
                class="control-select control-select--wide"
                @update:value="applyPreset"
              />
            </div>
            <n-input
              v-model:value="commandText"
              type="textarea"
              :autosize="{ minRows: 3, maxRows: 6 }"
              :placeholder="t('main.debug.cmd_placeholder')"
            />
            <div class="action-row">
              <n-button
                type="primary"
                :loading="sending"
                :disabled="!canSend"
                @click="handleSendCommand"
              >
                {{ t("main.debug.btn_send") }}
              </n-button>
              <span v-if="!canSend" class="setting-hint">{{ commandDisabledHint }}</span>
            </div>
            <n-alert v-if="commandError" type="error" :bordered="false" size="small">
              {{ commandError }}
            </n-alert>
            <div v-if="lastCommandResult" class="command-result">
              <n-tag
                size="small"
                :bordered="false"
                :type="lastCommandResult.status === 'executed' ? 'success' : 'error'"
              >
                {{ lastCommandResult.status === "executed" ? t("main.debug.cmd_executed") : t("main.debug.cmd_failed") }}
              </n-tag>
              <span class="setting-value setting-value--wrap">
                {{ lastCommandResult.commands.join(" ; ") }}
              </span>
              <span v-if="lastCommandResult.error" class="setting-hint">
                {{ lastCommandResult.error }}
              </span>
            </div>
          </n-space>
        </n-card>

        <n-card size="small" :bordered="true" class="debug-card debug-card--wide">
          <template #header>
            <span class="card-title">{{ t("main.debug.record_title") }}</span>
          </template>

          <n-space vertical :size="10">
            <span class="setting-hint">{{ t("main.debug.record_hint") }}</span>

            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.record_status") }}</span>
              <n-tag size="small" :bordered="false" :type="activeRecordingTagType">
                {{ activeRecordingLabel }}
              </n-tag>
            </div>
            <div v-if="sessionState.recording" class="setting-row">
              <span class="setting-label">{{ t("main.debug.record_current_demo") }}</span>
              <span
                class="setting-value setting-value--path"
                :title="sessionState.recording.demo_path"
              >
                {{ sessionState.recording.demo_path || t("main.debug.value_none") }}
              </span>
            </div>

            <div class="action-row">
              <n-button
                type="primary"
                :loading="startingRecording"
                :disabled="!canStartRecording"
                @click="handleStartRecording"
              >
                {{ t("main.debug.btn_start_record") }}
              </n-button>
              <n-button
                secondary
                :loading="stoppingRecording"
                :disabled="!canStopRecording"
                @click="handleStopRecording"
              >
                {{ t("main.debug.btn_stop_record") }}
              </n-button>
              <span v-if="!canStartRecording && recordDisabledHint" class="setting-hint">
                {{ recordDisabledHint }}
              </span>
            </div>
            <span class="setting-hint">{{ t("main.debug.record_custom_note") }}</span>
            <n-alert v-if="recordError" type="error" :bordered="false" size="small">
              {{ recordError }}
            </n-alert>

            <div class="result-head">
              <span class="setting-label">{{ t("main.debug.result_title") }}</span>
            </div>
            <n-empty
              v-if="recordingResults.length === 0"
              size="small"
              :description="t('main.debug.record_empty')"
            />
            <div v-else class="recording-list">
              <div
                v-for="item in recordingResults"
                :key="item.recording_id"
                class="recording-item"
              >
                <div class="setting-row">
                  <span class="setting-label setting-label--id">{{ item.recording_id }}</span>
                  <div class="setting-control">
                    <n-tag size="small" :bordered="false" :type="recordingTagType(item.status)">
                      {{ recordingStatusLabel(item.status) }}
                    </n-tag>
                    <n-button size="tiny" secondary @click="handleOpenRecordingFolder(item.recording_id)">
                      {{ t("main.debug.btn_open_result") }}
                    </n-button>
                    <n-button
                      v-if="item.status === 'failed'"
                      size="tiny"
                      type="primary"
                      :loading="retryingRecording === item.recording_id"
                      :disabled="recordingBusy"
                      @click="handleRetryMerge(item.recording_id)"
                    >
                      {{ t("main.debug.btn_retry_merge") }}
                    </n-button>
                  </div>
                </div>
                <div class="setting-row">
                  <span class="setting-label">{{ t("main.debug.record_demo") }}</span>
                  <span class="setting-value setting-value--path" :title="item.demo_path">
                    {{ item.demo_path || t("main.debug.value_none") }}
                  </span>
                </div>
                <div class="setting-row">
                  <span class="setting-label">{{ t("main.debug.record_final_path") }}</span>
                  <span
                    class="setting-value setting-value--path"
                    :title="item.final_path || item.directory"
                  >
                    {{ item.final_path || item.directory || t("main.debug.value_none") }}
                  </span>
                </div>
                <span v-if="item.error" class="setting-hint">{{ item.error }}</span>
              </div>
            </div>
            <n-alert v-if="recordFolderError" type="error" :bordered="false" size="small">
              {{ recordFolderError }}
            </n-alert>
          </n-space>
        </n-card>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { t } from "@/shared/i18n";
import { backend } from "@/shared/backend";
import {
  buildDebugDemoOptions,
  canToggleDemoPause,
  debugPauseCommand,
  debugSettingPreview,
  useDebugDemoSelection,
  useDebugSession,
  useDebugSettingsDraft,
  type DebugSwitchKey,
} from "@/domains/debug";
import { demoList, selectedEntry } from "@/domains/demo/demo-state";
import { useDebugSettings } from "@/shared/state/useDebugSettings";
import type {
  ClipSettings,
  DebugCommandResponse,
  DebugLaunchRequest,
  DebugLaunchResolution,
  DebugPlayDemoResponse,
  DebugPluginState,
  DebugRecordingStatus,
  DebugRecordingStatusValue,
  DebugSessionState,
} from "@/shared/types";

type LaunchResolution = DebugLaunchResolution;

interface LiveSettingRow {
  key: DebugSwitchKey;
  labelKey: string;
  hintKey?: string;
}

const router = useRouter();
const {
  sessionState,
  initialized,
  launching,
  stopping,
  sending,
  playing: playingRequest,
  startingRecording,
  stoppingRecording,
  retryingRecording,
  lastCommandResult,
  lastPlayResult,
  errorMessage,
  initDebugSession,
  launchDebugGame,
  stopDebugGame,
  sendDebugConsoleCommand,
  playDebugDemo,
  startDebugRecording,
  stopDebugRecording,
  retryDebugRecordingMerge,
  openDebugRecordingInFolder,
} = useDebugSession();
const { keepProduceIntermediates } = useDebugSettings();
const { debugDemoKey, ensureDebugDemoSelection, selectDebugDemo } =
  useDebugDemoSelection();
const {
  draft: settingsDraft,
  sourceSettings,
  plan: settingsPlan,
  loaded: settingsLoaded,
  loading: settingsLoading,
  errorMessage: settingsReadError,
  planErrorMessage: settingsPlanError,
  loadDebugSettings,
  reloadDebugSettings,
  applyDebugPlan,
  patchDraft,
  planItem,
  toggleCommands,
  setDebugSwitch,
} = useDebugSettingsDraft();

const launch = reactive({
  injectPlugin: true,
  povHud: true,
  brandVpk: true,
  resolution: "4:3" as LaunchResolution,
});

const commandText = ref("");
const presetKey = ref<string | null>(null);
const commandError = ref("");
const playError = ref("");
const folderError = ref("");
const pauseError = ref("");
const pauseRequesting = ref(false);
const applyError = ref("");
const recordingApplyError = ref("");
const applyingAll = ref(false);
const applyingRecording = ref(false);
const applyingLifetime = ref(false);
const recordError = ref("");
const recordFolderError = ref("");
// The Demo-list watcher must not preempt the import-page default before the
// first mount-time selection ran.
let demoSelectionInitialized = false;

// Settings load asynchronously and the launch controls stay editable meanwhile.
// Each field records whether the user touched it during that window, so the
// late settings response cannot overwrite a choice the user just made. The
// radar has no separate launch field: its only state is the debug draft.
const loadingDefaults = ref(true);
const launchTouched = { povHud: false, resolution: false };
const pendingRadarChoice = ref<boolean | null>(null);
watch(
  () => launch.povHud,
  () => {
    if (loadingDefaults.value) launchTouched.povHud = true;
  },
  { flush: "sync" },
);
watch(
  () => launch.resolution,
  () => {
    if (loadingDefaults.value) launchTouched.resolution = true;
  },
  { flush: "sync" },
);

// The console plan must be built from the resolution the session actually
// uses, not only from the settings snapshot the draft started with.
watch(
  () => launch.resolution,
  (value) => {
    if (settingsDraft.value.launch_resolution !== value) {
      patchDraft({ launch_resolution: value });
    }
  },
);

const sessionStatus = computed(() => sessionState.value.status);
const launchLocked = computed(
  () =>
    sessionStatus.value === "starting" ||
    sessionStatus.value === "running" ||
    sessionStatus.value === "stopping",
);
const canLaunch = computed(() => !launchLocked.value && !launching.value);
const canStop = computed(() => sessionStatus.value !== "idle" && !stopping.value);
// Radar is a hot-updatable setting, but its hot-update home is the runtime
// row: while a session is starting/running/stopping the launch card only
// mirrors the draft, so every editable radar control sends a real command.
const radarDisabled = computed(() => !launch.injectPlugin || launchLocked.value);
const radarRequiresPlugin = computed(() => !launch.injectPlugin);
const sessionError = computed(() => errorMessage.value || sessionState.value.error);
const sessionRunningWithoutPlugin = computed(
  () => sessionStatus.value === "running" && !sessionState.value.plugin_enabled,
);

const statusLabelKey = computed(() => `main.debug.status_${sessionStatus.value}`);
const statusTagType = computed(() => {
  switch (sessionStatus.value) {
    case "running":
      return "success";
    case "starting":
    case "stopping":
      return "warning";
    case "failed":
      return "error";
    default:
      return "default";
  }
});

/**
 * Plugin tag derived from the backend's structured transport/readiness state.
 * A connected transport is never shown as ready, and an idle snapshot without a
 * session is shown as "not connected" rather than an error.
 */
const pluginTag = computed<{ label: string; type: "default" | "warning" | "success" | "error" }>(
  () => {
    const state: DebugPluginState = sessionState.value.plugin_state;
    switch (state) {
      case "ready":
        return { label: t("main.debug.plugin_tag_ready"), type: "success" };
      case "waiting_ready":
        return { label: t("main.debug.plugin_tag_waiting_ready"), type: "warning" };
      case "waiting_connection":
        return { label: t("main.debug.plugin_tag_waiting_connection"), type: "warning" };
      case "disabled":
        return { label: t("main.debug.plugin_tag_disabled"), type: "default" };
      case "error":
        return { label: t("main.debug.plugin_tag_error"), type: "error" };
      default:
        return { label: t("main.debug.plugin_tag_disconnected"), type: "default" };
    }
  },
);
const pluginTagLabel = computed(() => pluginTag.value.label);
const pluginTagType = computed(() => pluginTag.value.type);
// The backend message keeps the troubleshooting detail behind the tag.
const pluginDetail = computed(() => sessionState.value.plugin_message);

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

/** Quick presets only prepare the shared input; sending stays explicit. */
const commandPresets = [
  { key: "sky-off", labelKey: "main.debug.quick_sky_off", command: "r_drawskybox 0" },
  { key: "sky-on", labelKey: "main.debug.quick_sky_on", command: "r_drawskybox 1" },
  { key: "ui-hide", labelKey: "main.debug.quick_ui_hide", command: "cl_draw_only_deathnotices 1" },
  { key: "ui-show", labelKey: "main.debug.quick_ui_show", command: "cl_draw_only_deathnotices 0" },
];
const presetOptions = computed(() =>
  commandPresets.map((preset) => ({ label: t(preset.labelKey), value: preset.key })),
);

function applyPreset(key: string | null) {
  const preset = commandPresets.find((item) => item.key === key);
  if (!preset) {
    presetKey.value = null;
    return;
  }
  commandText.value = preset.command;
  presetKey.value = preset.key;
}

// A manual edit no longer matches the preset it started from, so the dropdown
// clears. Selecting the same preset again then restores its command instead of
// being swallowed as "no change".
watch(commandText, (text) => {
  const preset = commandPresets.find((item) => item.key === presetKey.value);
  if (preset && text !== preset.command) presetKey.value = null;
});

/** Rows in the runtime group. Commands stay backend-built; this is only layout. */
const liveSettingRows: LiveSettingRow[] = [
  { key: "sky_blackout", labelKey: "main.settings.sky_blackout" },
  { key: "disable_clouds", labelKey: "main.settings.disable_clouds" },
  { key: "hide_all_ui", labelKey: "main.settings.hide_all_ui" },
  { key: "hide_player_avatars", labelKey: "main.settings.hide_player_avatars" },
  {
    key: "hide_chat_messages",
    labelKey: "main.settings.hide_chat_messages",
    hintKey: "main.debug.hint_hide_chat_restore",
  },
  { key: "enable_spec_show_xray_zero", labelKey: "main.settings.enable_spec_show_xray_zero" },
  { key: "enable_voice", labelKey: "main.settings.enable_voice" },
  {
    key: "use_shoulder_camera",
    labelKey: "main.settings.use_shoulder_camera",
    hintKey: "main.debug.hint_shoulder_restore",
  },
  { key: "pov_radar_enabled", labelKey: "main.debug.pov_radar" },
  {
    key: "block_kill_feed",
    labelKey: "main.settings.block_kill_feed",
    hintKey: "main.debug.hint_kill_feed_clear",
  },
];

function rowPreview(key: string): string {
  return debugSettingPreview(planItem(key));
}

const effectivePresetLabel = computed(() => {
  const plan = settingsPlan.value;
  if (!plan || plan.effective_video_status !== "ready") {
    return t("main.settings.video_preset_auto");
  }
  const preset = (plan.effective_video_preset || "").trim().replace(/_h264$/i, "");
  const encoder = (plan.effective_video_encoder || "").trim();
  if (preset && encoder) {
    return `${preset} (${encoder})`;
  }
  return encoder || preset || t("main.settings.video_preset_auto");
});
const videoPresetOptions = computed(() => [
  { label: effectivePresetLabel.value, value: "auto" },
  { label: t("main.settings.video_preset_c1"), value: "c1" },
  { label: t("main.settings.video_preset_n1"), value: "n1" },
  { label: t("main.settings.video_preset_a1"), value: "a1" },
  { label: t("main.settings.video_preset_i1"), value: "i1" },
]);

const demoOptions = computed(() => buildDebugDemoOptions(demoList.value));
const selectedDebugDemo = computed(
  () => demoList.value.find((entry) => entry.key === debugDemoKey.value) ?? null,
);

const canSend = computed(
  () => sessionStatus.value === "running" && sessionState.value.plugin_ready && !sending.value,
);
const commandDisabledHint = computed(() => {
  if (sessionStatus.value !== "running") return t("main.debug.cmd_need_session");
  if (!sessionState.value.plugin_enabled) return t("main.debug.cmd_need_plugin");
  if (!sessionState.value.plugin_ready) {
    return sessionState.value.plugin_message || t("main.debug.cmd_need_ready");
  }
  return "";
});

const canToggleSetting = computed(
  () =>
    canSend.value &&
    settingsLoaded.value &&
    !settingsLoading.value &&
    settingsPlan.value !== null &&
    !applyingAll.value &&
    !applyingRecording.value &&
    !applyingLifetime.value,
);
const canApplyAll = computed(
  () =>
    canSend.value &&
    settingsLoaded.value &&
    !settingsLoading.value &&
    settingsPlan.value !== null &&
    !applyingAll.value &&
    !applyingRecording.value &&
    !applyingLifetime.value,
);
const canApplyRecording = computed(
  () =>
    canSend.value &&
    settingsLoaded.value &&
    !settingsLoading.value &&
    settingsPlan.value !== null &&
    !applyingAll.value &&
    !applyingRecording.value &&
    !applyingLifetime.value &&
    // The next-segment recording settings are frozen while a segment is active;
    // applying them mid-recording would change the running configuration.
    !recordingBusy.value,
);

// Apply and reload share admission: a reload must not steal the plan request
// an apply is waiting for (and vice versa).
const applyingSettings = computed(
  () => applyingAll.value || applyingRecording.value || applyingLifetime.value,
);

const canPlay = computed(
  () =>
    canSend.value &&
    !playingRequest.value &&
    !sessionState.value.demo_play_pending &&
    // A recording or its merge owns the current Demo; switching it now would
    // make the segment's Demo metadata lie.
    !recordingBusy.value &&
    !!selectedDebugDemo.value &&
    !!selectedDebugDemo.value.file_path,
);
const playDisabledHint = computed(() => {
  if (!selectedDebugDemo.value) return t("main.debug.demo_empty");
  if (recordingBusy.value) return t("main.debug.record_busy_demo_hint");
  if (sessionState.value.demo_play_pending) return t("main.debug.pause_pending_hint");
  return commandDisabledHint.value;
});
const playbackLabel = computed(() =>
  sessionState.value.demo_playing ? t("main.debug.demo_playing") : t("main.debug.demo_stopped"),
);
const playbackTagType = computed(() => (sessionState.value.demo_playing ? "success" : "default"));

// The single pause/resume button follows the engine observation only, and it
// stays disabled while a play request is in flight or still waiting for the
// engine to confirm the requested Demo: pressing it then would evaluate the
// command direction against the previous Demo's pause state.
const canTogglePause = computed(() =>
  canToggleDemoPause({
    sessionRunning: sessionStatus.value === "running",
    pluginReady: sessionState.value.plugin_ready,
    commandInFlight: sending.value,
    playRequestInFlight: playingRequest.value || pauseRequesting.value,
    demoPlaying: sessionState.value.demo_playing,
    demoPlayPending: sessionState.value.demo_play_pending,
  }),
);
const pauseDisabledHint = computed(() =>
  sessionState.value.demo_play_pending ? t("main.debug.pause_pending_hint") : "",
);
const pauseButtonLabel = computed(() =>
  sessionState.value.demo_paused ? t("main.debug.btn_resume_demo") : t("main.debug.btn_pause_demo"),
);
const pauseButtonType = computed(() =>
  sessionState.value.demo_paused ? "success" : "warning",
);

// ---- Manual recording ----

/** The backend-owned active segment; null when the workbench is between takes. */
const activeRecording = computed<DebugRecordingStatus | null>(
  () => sessionState.value.recording ?? null,
);
const recordingBusy = computed(() => activeRecording.value !== null);
const recordingResults = computed(() => [...sessionState.value.recordings].reverse());

const recordingStatusLabels: Record<DebugRecordingStatusValue, string> = {
  starting: "main.debug.record_status_starting",
  recording: "main.debug.record_status_recording",
  stopping: "main.debug.record_status_stopping",
  merging: "main.debug.record_status_merging",
  completed: "main.debug.record_status_completed",
  failed: "main.debug.record_status_failed",
};

function recordingStatusLabel(status: DebugRecordingStatusValue): string {
  return t(recordingStatusLabels[status] ?? "main.debug.record_status_failed");
}

function recordingTagType(
  status: DebugRecordingStatusValue,
): "default" | "warning" | "success" | "error" {
  switch (status) {
    case "recording":
      return "success";
    case "starting":
    case "stopping":
    case "merging":
      return "warning";
    case "failed":
      return "error";
    default:
      return "default";
  }
}

const activeRecordingLabel = computed(() => {
  const active = activeRecording.value;
  if (!active) return t("main.debug.record_no_active");
  return recordingStatusLabel(active.status);
});
const activeRecordingTagType = computed<"default" | "warning" | "success" | "error">(() =>
  activeRecording.value ? recordingTagType(activeRecording.value.status) : "default",
);

// A start needs a confirmed playing Demo, a ready plugin and no active
// segment; the backend re-checks the same admission atomically.
const canStartRecording = computed(
  () =>
    canSend.value &&
    sessionState.value.demo_playing &&
    !sessionState.value.demo_play_pending &&
    !recordingBusy.value &&
    !startingRecording.value,
);
const canStopRecording = computed(() => {
  const active = activeRecording.value;
  return (
    canSend.value &&
    !!active &&
    (active.status === "starting" || active.status === "recording") &&
    !stoppingRecording.value
  );
});
const recordDisabledHint = computed(() => {
  if (!sessionState.value.demo_playing) return t("main.debug.record_need_play");
  if (recordingBusy.value) return t("main.debug.record_active_hint");
  return commandDisabledHint.value || t("main.debug.record_need_session");
});

async function handleStartRecording() {
  if (!canStartRecording.value) return;
  recordError.value = "";
  try {
    await startDebugRecording({
      record_fps: settingsDraft.value.record_fps,
      record_quality: settingsDraft.value.record_quality,
      video_preset: settingsDraft.value.video_preset,
      keep_intermediate_files: keepProduceIntermediates.value,
    });
  } catch (error: unknown) {
    recordError.value = describeCommandError(error);
  }
}

async function handleStopRecording() {
  if (!canStopRecording.value) return;
  recordError.value = "";
  try {
    await stopDebugRecording();
  } catch (error: unknown) {
    recordError.value = describeCommandError(error);
  }
}

async function handleRetryMerge(recordingID: string) {
  recordError.value = "";
  try {
    await retryDebugRecordingMerge(recordingID);
  } catch (error: unknown) {
    recordError.value = describeCommandError(error);
  }
}

async function handleOpenRecordingFolder(recordingID: string) {
  recordFolderError.value = "";
  try {
    await openDebugRecordingInFolder(recordingID);
  } catch (error: unknown) {
    recordFolderError.value = describeCommandError(error);
  }
}

const formalSettings = computed<ClipSettings | null>(() => sourceSettings.value);

function qualityLabel(value: ClipSettings["edit_quality"] | undefined): string {
  switch (value) {
    case "standard":
      return t("main.settings.edit_quality_standard");
    case "ultra":
      return t("main.settings.edit_quality_ultra");
    case "high":
      return t("main.settings.edit_quality_high");
    default:
      return t("main.debug.value_none");
  }
}

onMounted(async () => {
  await Promise.allSettled([initDebugSession(), loadDebugSettings()]);
  // The debug page keeps its own pending Demo. The import selection is only a
  // first-time default; later import-page changes never override a choice made
  // here.
  ensureDebugDemoSelection(demoList.value, selectedEntry.value?.key ?? null);
  demoSelectionInitialized = true;
  // A session that is already running owns the frozen launch options; otherwise
  // the draft starts from the confirmed settings without ever writing back.
  if (sessionStatus.value !== "idle") {
    applySessionToDraft(sessionState.value);
    loadingDefaults.value = false;
    return;
  }
  applySettingsDefaults();
});

// Keep the debug selection valid when the shared list changes: an entry removed
// on the import page falls back to the first import (or clears the selection).
watch(demoList, (entries) => {
  if (!demoSelectionInitialized) return;
  ensureDebugDemoSelection(entries);
});

function applySettingsDefaults(force = false) {
  const settings = sourceSettings.value;
  // Stop tracking before applying defaults: these assignments are not user edits.
  loadingDefaults.value = false;
  if (!settings) return;
  if (force) {
    // An explicit "reload from formal settings" also drops the pre-launch
    // choices so the launch options follow the reloaded draft again.
    launchTouched.povHud = false;
    launchTouched.resolution = false;
    pendingRadarChoice.value = null;
  }
  if (!launchTouched.povHud) launch.povHud = settings.pov_hud_enabled;
  // Resolution lives in the session debug draft, so a route switch keeps the
  // user's pre-launch choice instead of resetting it from persisted settings.
  // The draft was initialized from the same settings.
  if (!launchTouched.resolution) launch.resolution = settingsDraft.value.launch_resolution;
  // Radar belongs to the draft alone; only a choice made while the initial
  // read was in flight needs to be pushed back after the late assignment.
  if (pendingRadarChoice.value !== null) {
    patchDraft({ pov_radar_enabled: pendingRadarChoice.value });
    pendingRadarChoice.value = null;
  }
}

/**
 * The launch-card radar and the runtime row are one debug-draft field; the
 * play-time command is always rebuilt from that draft. Before the settings
 * read completes the draft is still the placeholder, so remember the choice
 * and re-apply it after the late assignment.
 */
function handleLaunchRadarChange(value: boolean) {
  if (radarDisabled.value) return;
  if (loadingDefaults.value) pendingRadarChoice.value = value;
  patchDraft({ pov_radar_enabled: value });
}

function applySessionToDraft(state: DebugSessionState) {
  launch.injectPlugin = state.inject_plugin;
  launch.povHud = state.pov_hud_enabled;
  launch.brandVpk = state.brand_enabled;
  // Radar is deliberately not taken from the frozen launch request: the draft
  // owns the latest runtime state and the locked launch switch displays it.
  launch.resolution = state.launch_resolution;
}

function buildLaunchRequest(): DebugLaunchRequest {
  return {
    inject_plugin: launch.injectPlugin,
    pov_hud_enabled: launch.povHud,
    brand_enabled: launch.brandVpk,
    pov_radar_enabled: settingsDraft.value.pov_radar_enabled,
    launch_resolution: launch.resolution,
  };
}

async function handleLaunch() {
  try {
    const state = await launchDebugGame(buildLaunchRequest());
    // A null result means a workspace switch disposed the store while the
    // request was in flight; the draft must not adopt the stale options.
    if (state) applySessionToDraft(state);
  } catch {
    // The core keeps the backend error in `errorMessage` for the session card.
  }
}

async function handleStop() {
  try {
    await stopDebugGame();
  } catch {
    // Keep the failed state visible; the button retries the teardown.
  }
}

/** Reload the debug draft from the persisted settings, not from the page. */
async function handleReloadSettings() {
  if (settingsLoading.value || applyingSettings.value) return;
  const ok = await reloadDebugSettings();
  if (ok && sessionStatus.value === "idle") {
    applySettingsDefaults(true);
  }
}

function describeCommandError(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

/** Send one console request and report the receipt into the owning card. */
async function sendCommands(commands: string[], errorSink: { value: string }) {
  errorSink.value = "";
  try {
    const result: DebugCommandResponse | null = await sendDebugConsoleCommand(commands);
    if (result && result.status !== "executed") {
      errorSink.value = result.error || t("main.debug.cmd_failed");
    }
  } catch (error: unknown) {
    errorSink.value = describeCommandError(error);
  }
}

/** A switch click sends exactly one variant from the backend-built plan. */
async function handleSettingToggle(key: DebugSwitchKey, value: boolean) {
  setDebugSwitch(key, value);
  if (!canToggleSetting.value) return;
  const commands = toggleCommands(key, value);
  if (commands.length === 0) {
    applyError.value = t("main.debug.plan_required");
    return;
  }
  await sendCommands(commands, applyError);
}

function handleLifetimeChange(value: number | null) {
  if (typeof value !== "number" || !Number.isFinite(value)) return;
  const clamped = Math.min(10, Math.max(1, Math.round(value)));
  patchDraft({ kill_feed_lifetime: clamped });
}

async function handleApplyLifetime() {
  if (!canToggleSetting.value || applyingLifetime.value) return;
  applyingLifetime.value = true;
  try {
    const plan = await applyDebugPlan();
    const item = plan?.settings.find((entry) => entry.key === "kill_feed_lifetime");
    if (!item || item.commands.length === 0) {
      applyError.value = t("main.debug.plan_required");
      return;
    }
    await sendCommands([...item.commands], applyError);
  } finally {
    applyingLifetime.value = false;
  }
}

/** Apply-all always rebuilds the plan so the sent order matches the draft. */
async function handleApplyAll() {
  if (!canApplyAll.value || applyingAll.value) return;
  applyingAll.value = true;
  try {
    const plan = await applyDebugPlan();
    const commands = plan?.apply_all ?? [];
    if (commands.length === 0) {
      applyError.value = t("main.debug.plan_required");
      return;
    }
    await sendCommands([...commands], applyError);
  } finally {
    applyingAll.value = false;
  }
}

async function handleApplyRecording() {
  if (!canApplyRecording.value || applyingRecording.value) return;
  applyingRecording.value = true;
  try {
    const plan = await applyDebugPlan();
    const commands = plan?.recording ?? [];
    if (commands.length === 0) {
      recordingApplyError.value = t("main.debug.plan_required");
      return;
    }
    await sendCommands([...commands], recordingApplyError);
  } finally {
    applyingRecording.value = false;
  }
}

function handleRecordFPSChange(value: number | null) {
  if (typeof value !== "number" || !Number.isFinite(value)) return;
  patchDraft({ record_fps: Math.min(240, Math.max(1, Math.round(value))) });
}

function handleRecordQualityChange(value: string | number | null) {
  if (value !== "standard" && value !== "high" && value !== "ultra") return;
  patchDraft({ record_quality: value });
}

function handleVideoPresetChange(value: string | number | null) {
  if (value !== "auto" && value !== "c1" && value !== "n1" && value !== "a1" && value !== "i1") {
    return;
  }
  patchDraft({ video_preset: value });
}

/**
 * Playback applies the session debug draft's radar state through the
 * backend-built plan (both on and off), so a runtime toggle is never
 * overridden by a separate launch-card copy and no command is assembled here.
 */
function buildPlaybackCommands(): string[] {
  return toggleCommands("pov_radar_enabled", settingsDraft.value.pov_radar_enabled);
}

async function handlePlayDemo() {
  const entry = selectedDebugDemo.value;
  if (!entry || !entry.file_path) {
    playError.value = t("main.debug.demo_empty");
    return;
  }
  playError.value = "";
  try {
    const result: DebugPlayDemoResponse | null = await playDebugDemo(
      entry.file_path,
      buildPlaybackCommands(),
    );
    if (result && result.status !== "executed") {
      playError.value = result.error || t("main.debug.play_failed");
    }
  } catch (error: unknown) {
    playError.value = describeCommandError(error);
  }
}

async function handleTogglePause() {
  // Re-check admission at call time: the button state is computed, but a click
  // may race a play request that just started waiting for engine confirmation.
  if (!canTogglePause.value) return;
  // A paused Demo is still loaded, so the command is chosen from the engine
  // observation. The receipt never flips the button: only the plugin's next
  // playback observation may change `demo_paused`.
  const command = debugPauseCommand(sessionState.value.demo_paused);
  pauseRequesting.value = true;
  try {
    await sendCommands([command], pauseError);
  } finally {
    pauseRequesting.value = false;
  }
}

async function handleOpenDemoFolder() {
  const entry = selectedDebugDemo.value;
  if (!entry || !entry.file_path) return;
  folderError.value = "";
  try {
    await backend.OpenDemoInFolder(entry.file_path);
  } catch (error: unknown) {
    folderError.value = describeCommandError(error);
  }
}

async function handleSendCommand() {
  // The custom input stays one element so the engine receives the original
  // string, including semicolons and quotes.
  const raw = commandText.value.trim();
  if (!raw) {
    commandError.value = t("main.debug.cmd_empty");
    return;
  }
  await sendCommands([raw], commandError);
}

function goToImport() {
  void router.push("/import");
}
</script>

<style scoped>
.debug-page {
  height: 100%;
  min-height: 0;
}

.debug-scroll {
  height: 100%;
  overflow-y: auto;
  padding: 10px 12px 16px;
}

.debug-grid {
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.debug-card {
  background: #181b19;
}

.debug-card--wide {
  grid-column: 1 / -1;
}

.card-title {
  font-size: 13px;
  font-weight: 600;
}

.setting-row {
  align-items: center;
  display: flex;
  gap: 12px;
  justify-content: space-between;
}

.setting-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.setting-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.setting-label {
  color: #c6d1c8;
  font-size: 13px;
  flex: 0 0 auto;
}

.setting-value {
  color: #edf1ee;
  font-size: 13px;
  min-width: 0;
  overflow: hidden;
  text-align: right;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.setting-value--path {
  color: #97a49c;
  font-size: 12px;
}

.setting-value--wrap {
  color: #c6d1c8;
  white-space: normal;
}

.setting-control {
  align-items: center;
  display: flex;
  gap: 8px;
  min-width: 0;
}

.setting-hint {
  color: #88958b;
  font-size: 12px;
  line-height: 1.5;
}

.setting-command {
  color: #b9c6bd;
  font-family: Consolas, "Courier New", monospace;
  font-size: 12px;
  line-height: 1.5;
  white-space: normal;
  word-break: break-all;
}

.setting-command--empty {
  color: #5f6a63;
}

.command-preview {
  border-top: 1px solid #252c27;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-top: 8px;
}

.command-preview__list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.control-select {
  width: 180px;
}

.control-select--wide {
  width: 100%;
  max-width: 360px;
}

.control-number {
  width: 120px;
}

.action-row {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.demo-info {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 2px;
}

.result-head {
  border-top: 1px solid #252c27;
  padding-top: 8px;
}

.recording-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.recording-item {
  border: 1px solid #252c27;
  border-radius: 6px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 10px;
}

.setting-label--id {
  color: #97a49c;
  font-family: Consolas, "Courier New", monospace;
  font-size: 12px;
}

.command-result {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

@media (max-width: 760px) {
  .debug-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
