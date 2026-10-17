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

            <span class="setting-label">{{ t("main.debug.launch_arguments") }}</span>
            <n-input v-model:value="launchInput.arguments" :disabled="launchLocked" />
            <n-button size="tiny" :disabled="launchLocked" @click="resetLaunchArguments">
              {{ t("main.debug.launch_arguments_reset") }}
            </n-button>
            <span class="setting-label">{{ t("main.debug.startup_commands") }}</span>
            <n-input
              v-model:value="launchInput.commands"
              type="textarea"
              :autosize="{ minRows: 3, maxRows: 8 }"
              :disabled="launchLocked || !launch.injectPlugin"
              :placeholder="t('main.debug.startup_commands_placeholder')"
            />
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
            <n-alert v-if="sessionState.startup_command_result" :type="sessionState.startup_command_result.status === 'executed' ? 'success' : 'error'" :bordered="false" size="small">
              {{ sessionState.startup_command_result.status === "executed" ? t("main.debug.startup_commands_executed") : sessionState.startup_command_result.error }}
            </n-alert>
            <span v-else-if="sessionState.startup_commands?.length && launchLocked" class="setting-hint">{{ t("main.debug.startup_commands_waiting") }}</span>
            <n-alert v-if="sessionError" type="error" :bordered="false" size="small">
              {{ sessionError }}
            </n-alert>
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

        <n-card size="small" :bordered="true" class="debug-card">
          <template #header>
            <span class="card-title">{{ t("main.debug.recording_settings_title") }}</span>
          </template>
          <template #header-extra>
            <n-button
              size="tiny"
              secondary
              :loading="settingsLoading"
              :disabled="settingsLoading || applyingRecording"
              @click="handleReloadSettings"
            >
              {{ t("main.debug.recording_reload") }}
            </n-button>
          </template>

          <n-space vertical :size="10">
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
              <span class="setting-label">{{ t("main.settings.motion_blur_enabled") }}</span>
              <div class="setting-control">
                <n-switch
                  :value="settingsDraft.motion_blur_enabled"
                  :disabled="!settingsLoaded"
                  @update:value="handleMotionBlurChange"
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
            </div>
            <n-alert v-if="recordingApplyError" type="error" :bordered="false" size="small">
              {{ recordingApplyError }}
            </n-alert>
            <n-collapse
              v-if="settingsPlan && settingsPlan.recording.length > 0"
              class="command-collapse"
            >
              <n-collapse-item :title="t('main.debug.command_preview')" name="recording">
                <div class="command-preview__list">
                  <span
                    v-for="command in settingsPlan.recording"
                    :key="command"
                    class="setting-command"
                  >
                    {{ command }}
                  </span>
                </div>
              </n-collapse-item>
            </n-collapse>
          </n-space>
        </n-card>

        <n-card size="small" :bordered="true" class="debug-card debug-card--wide">
          <template #header>
            <span class="card-title">{{ t("main.debug.cmd_title") }}</span>
          </template>

          <n-space vertical :size="10">
            <n-alert v-if="settingsReadError" type="error" :bordered="false" size="small">
              {{ settingsReadError }}
            </n-alert>
            <n-alert v-if="settingsPlanError" type="warning" :bordered="false" size="small">
              {{ settingsPlanError }}
            </n-alert>

            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.quick_title") }}</span>
              <n-select
                :value="quickCommandKey"
                :options="quickCommandSelectOptions"
                :loading="settingsLoading || planLoading"
                filterable
                :placeholder="t('main.debug.quick_placeholder')"
                class="control-select control-select--wide"
                @update:value="handleQuickCommandSelect"
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
            </div>
            <n-alert v-if="recordError" type="error" :bordered="false" size="small">
              {{ recordError }}
            </n-alert>

            <template v-if="recordingResults.length > 0">
              <div class="result-head">
                <span class="setting-label">{{ t("main.debug.result_title") }}</span>
              </div>
              <div class="recording-list">
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
                    <span class="setting-label">{{ t("main.debug.record_created_at") }}</span>
                    <span class="setting-value">{{ formatCreatedAt(item.started_at_ms) }}</span>
                  </div>
                  <div class="setting-row">
                    <span class="setting-label">{{ t("main.debug.record_source_dir") }}</span>
                    <span class="setting-value setting-value--path" :title="item.directory">
                      {{ item.directory || t("main.debug.value_none") }}
                    </span>
                  </div>
                  <div class="setting-row">
                    <span class="setting-label">{{ t("main.debug.record_final_path") }}</span>
                    <span class="setting-value setting-value--path" :title="item.final_path">
                      {{ item.final_path || t("main.debug.value_none") }}
                    </span>
                  </div>
                  <span v-if="item.status === 'failed'" class="setting-hint">
                    {{ t("main.debug.record_merge_retry_hint") }}
                  </span>
                  <span v-if="item.error" class="setting-hint">{{ item.error }}</span>
                </div>
              </div>
            </template>
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
import { t } from "@/shared/i18n";
import { backend } from "@/shared/backend";
import {
  buildDebugDemoOptions,
  buildDebugQuickCommandOptions,
  DEBUG_QUICK_COMMAND_GROUPS,
  canToggleDemoPause,
  debugPauseCommand,
  useDebugDemoSelection,
  useDebugSession,
  useDebugSettingsDraft,
} from "@/domains/debug";
import { useDebugLaunchInput } from "@/domains/debug/debugSessionStore";
import {
  DEFAULT_DEBUG_LAUNCH_ARGUMENTS,
  parseDebugStartupCommands,
  parseLaunchResolutionFromArguments,
} from "@/domains/debug/debugSessionCore";
import { demoList, selectedEntry } from "@/domains/demo/demo-state";
import { useDebugSettings } from "@/shared/state/useDebugSettings";
import type {
  DebugCommandResponse,
  DebugLaunchRequest,
  DebugPlayDemoResponse,
  DebugPluginState,
  DebugRecordingStatus,
  DebugRecordingStatusValue,
  DebugSessionState,
} from "@/shared/types";

const {
  sessionState,
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
  planLoading,
  errorMessage: settingsReadError,
  planErrorMessage: settingsPlanError,
  loadDebugSettings,
  reloadDebugSettings,
  applyDebugPlan,
  patchDraft,
} = useDebugSettingsDraft();

const launchInput = useDebugLaunchInput();
function resetLaunchArguments() { launchInput.value.arguments = DEFAULT_DEBUG_LAUNCH_ARGUMENTS; }

const launch = reactive({
  injectPlugin: true,
  povHud: true,
  brandVpk: true,
});

const commandText = ref("");
const quickCommandKey = ref<string | null>(null);
const commandError = ref("");
const playError = ref("");
const folderError = ref("");
const pauseError = ref("");
const pauseRequesting = ref(false);
const recordingApplyError = ref("");
const applyingRecording = ref(false);
const recordError = ref("");
const recordFolderError = ref("");
// The Demo-list watcher must not preempt the import-page default before the
// first mount-time selection ran.
let demoSelectionInitialized = false;

// Settings load asynchronously and the launch controls stay editable meanwhile.
// Each field records whether the user touched it during that window, so the
// late settings response cannot overwrite a choice the user just made.
const loadingDefaults = ref(true);
const launchTouched = { povHud: false };
watch(
  () => launch.povHud,
  () => {
    if (loadingDefaults.value) launchTouched.povHud = true;
  },
  { flush: "sync" },
);

// The launch arguments own the CS2 render resolution, so the console plan must
// be built from the backbuffer the user actually typed, not from the persisted
// formal setting. The backend derives the same profile from these arguments.
watch(
  () => launchInput.value.arguments,
  () => {
    syncLaunchResolutionDraft();
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
const sessionError = computed(() => errorMessage.value || sessionState.value.error);

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

const recordQualityOptions = computed(() => [
  { label: t("main.settings.edit_quality_standard"), value: "standard" },
  { label: t("main.settings.edit_quality_high"), value: "high" },
  { label: t("main.settings.edit_quality_ultra"), value: "ultra" },
]);

/**
 * Console quick commands built from the backend plan. Selecting one only fills
 * the shared input; the page never assembles a command string or edits the
 * debug draft from these options.
 */
const quickCommandOptions = computed(() => buildDebugQuickCommandOptions(settingsPlan.value));
const quickCommandSelectOptions = computed(() =>
  DEBUG_QUICK_COMMAND_GROUPS.map((group) => ({
    type: "group" as const,
    label: t(`main.debug.quick_group_${group}`),
    key: group,
    children: quickCommandOptions.value
      .filter((option) => option.group === group)
      .map((option) => ({ label: t(option.labelKey), value: option.key })),
  })).filter((group) => group.children.length > 0),
);

function handleQuickCommandSelect(key: string | null) {
  const option = quickCommandOptions.value.find((entry) => entry.key === key);
  if (!option) {
    quickCommandKey.value = null;
    return;
  }
  commandText.value = option.command;
  quickCommandKey.value = option.key;
}

// A manual edit no longer matches the selected option, so the dropdown clears.
// Selecting the same option again then restores its command instead of being
// swallowed as "no change". A plan refresh that changes the option's command
// also clears the selection rather than silently keeping a stale command; the
// shared input keeps whatever the user was editing.
watch(commandText, (text) => {
  const option = quickCommandOptions.value.find((entry) => entry.key === quickCommandKey.value);
  if (option && text !== option.command) quickCommandKey.value = null;
});
watch(quickCommandOptions, (options) => {
  if (!quickCommandKey.value) return;
  const option = options.find((entry) => entry.key === quickCommandKey.value);
  if (!option || option.command !== commandText.value) quickCommandKey.value = null;
});

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

const canApplyRecording = computed(
  () =>
    canSend.value &&
    settingsLoaded.value &&
    !settingsLoading.value &&
    settingsPlan.value !== null &&
    !applyingRecording.value &&
    // The next-segment recording settings are frozen while a segment is active;
    // applying them mid-recording would change the running configuration.
    !recordingBusy.value,
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

/** Result rows show the segment's local creation time, not only H:M:S. */
function formatCreatedAt(timestampMs: number): string {
  if (!timestampMs) return t("main.debug.value_none");
  const date = new Date(timestampMs);
  if (Number.isNaN(date.getTime())) return t("main.debug.value_none");
  return date.toLocaleString();
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

async function handleStartRecording() {
  if (!canStartRecording.value) return;
  recordError.value = "";
  try {
    await startDebugRecording({
      record_fps: settingsDraft.value.record_fps,
      record_quality: settingsDraft.value.record_quality,
      video_preset: settingsDraft.value.video_preset,
      motion_blur_enabled: settingsDraft.value.motion_blur_enabled,
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
  }
  if (!launchTouched.povHud) launch.povHud = settings.pov_hud_enabled;
  // The launch arguments own the render resolution, so a reload must not pull
  // it back from the persisted setting.
  syncLaunchResolutionDraft();
}

/**
 * Keep the console plan preview on the resolution the launch arguments carry.
 * The page has no resolution selector; the backend derives the same profile
 * from the same arguments and its result is what a live session reports.
 */
function syncLaunchResolutionDraft() {
  const resolution = parseLaunchResolutionFromArguments(launchInput.value.arguments);
  if (settingsDraft.value.launch_resolution !== resolution) {
    patchDraft({ launch_resolution: resolution });
  }
}

function applySessionToDraft(state: DebugSessionState) {
  launch.injectPlugin = state.inject_plugin;
  launch.povHud = state.pov_hud_enabled;
  launch.brandVpk = state.brand_enabled;
  launchInput.value.arguments = state.launch_arguments ?? DEFAULT_DEBUG_LAUNCH_ARGUMENTS;
  launchInput.value.commands = (state.startup_commands ?? []).join("\n");
  // The session's frozen resolution is the backend's own derivation from those
  // arguments; adopting it keeps the console plan preview aligned with the live
  // session even when the arguments text did not change.
  if (settingsDraft.value.launch_resolution !== state.launch_resolution) {
    patchDraft({ launch_resolution: state.launch_resolution });
  }
}

function buildLaunchRequest(): DebugLaunchRequest {
  // The launch request never enables the POV radar: radar is a runtime action
  // sent through the console quick commands, not a launch option.
  return {
    inject_plugin: launch.injectPlugin,
    pov_hud_enabled: launch.povHud,
    brand_enabled: launch.brandVpk,
    pov_radar_enabled: false,
    // Derived from the arguments the user typed; the backend re-derives it from
    // the same string, so a stale field can never pick the wrong stretch.
    launch_resolution: parseLaunchResolutionFromArguments(launchInput.value.arguments),
    launch_arguments: launchInput.value.arguments,
    startup_commands: launch.injectPlugin ? parseDebugStartupCommands(launchInput.value.commands) : [],
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
  if (settingsLoading.value || applyingRecording.value) return;
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

function handleMotionBlurChange(value: boolean) {
  patchDraft({ motion_blur_enabled: value });
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

async function handlePlayDemo() {
  const entry = selectedDebugDemo.value;
  if (!entry || !entry.file_path) {
    playError.value = t("main.debug.demo_empty");
    return;
  }
  playError.value = "";
  try {
    // Playback carries no quick commands: the radar and every other runtime
    // setting are sent explicitly from the console card and must never be
    // overwritten by a hidden draft value at play time.
    const result: DebugPlayDemoResponse | null = await playDebugDemo(entry.file_path, []);
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
</script>

<style scoped>
.debug-page {
  height: 100%;
  min-height: 0;
  /* The debug workbench is a read-out surface: titles, labels, paths, commands
     and errors must be selectable/copyable. The global app-wide `user-select:
     none` is overridden only here. */
  user-select: text;
  -webkit-user-select: text;
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

.command-preview__list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.command-collapse {
  border-top: 1px solid #252c27;
  padding-top: 8px;
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
