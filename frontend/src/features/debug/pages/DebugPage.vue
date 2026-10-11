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
                <n-switch v-model:value="launch.povRadar" :disabled="radarDisabled" />
                <span v-if="radarDisabled" class="setting-hint">
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
            <div class="action-row">
              <n-button type="primary" disabled>{{ t("main.debug.btn_start_record") }}</n-button>
              <n-button secondary disabled>{{ t("main.debug.btn_stop_record") }}</n-button>
              <span class="setting-hint">{{ t("main.debug.not_wired") }}</span>
            </div>
            <div class="result-head">
              <span class="setting-label">{{ t("main.debug.result_title") }}</span>
            </div>
            <n-empty
              v-if="recordingResults.length === 0"
              size="small"
              :description="t('main.debug.record_empty')"
            />
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
import { useSettingsStore } from "@/domains/settings/settings-state";
import {
  buildDebugDemoOptions,
  canToggleDemoPause,
  debugPauseCommand,
  useDebugDemoSelection,
  useDebugSession,
} from "@/domains/debug";
import { demoList, selectedEntry } from "@/domains/demo/demo-state";
import type {
  DebugCommandResponse,
  DebugLaunchRequest,
  DebugLaunchResolution,
  DebugPlayDemoResponse,
  DebugPluginState,
  DebugSessionState,
} from "@/shared/types";

type LaunchResolution = DebugLaunchResolution;

interface DebugRecordingResult {
  id: string;
  name: string;
  status: string;
}

const router = useRouter();
const settingsStore = useSettingsStore();
const {
  sessionState,
  initialized,
  launching,
  stopping,
  sending,
  playing: playingRequest,
  lastCommandResult,
  lastPlayResult,
  errorMessage,
  initDebugSession,
  launchDebugGame,
  stopDebugGame,
  sendDebugConsoleCommand,
  playDebugDemo,
} = useDebugSession();
const { debugDemoKey, ensureDebugDemoSelection, selectDebugDemo } =
  useDebugDemoSelection();

const launch = reactive({
  injectPlugin: true,
  povHud: true,
  brandVpk: true,
  povRadar: false,
  resolution: "4:3" as LaunchResolution,
});

const commandText = ref("");
const presetKey = ref<string | null>(null);
const commandError = ref("");
const playError = ref("");
const folderError = ref("");
const pauseError = ref("");
const pauseRequesting = ref(false);
const recordingResults = ref<DebugRecordingResult[]>([]);
// The Demo-list watcher must not preempt the import-page default before the
// first mount-time selection ran.
let demoSelectionInitialized = false;

// Settings load asynchronously and the launch controls stay editable meanwhile.
// Each field records whether the user touched it during that window, so the
// late settings response cannot overwrite a choice the user just made.
const loadingDefaults = ref(true);
const launchTouched = { povHud: false, povRadar: false, resolution: false };
watch(
  () => launch.povHud,
  () => {
    if (loadingDefaults.value) launchTouched.povHud = true;
  },
  { flush: "sync" },
);
watch(
  () => launch.povRadar,
  () => {
    if (loadingDefaults.value) launchTouched.povRadar = true;
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

const sessionStatus = computed(() => sessionState.value.status);
const launchLocked = computed(
  () =>
    sessionStatus.value === "starting" ||
    sessionStatus.value === "running" ||
    sessionStatus.value === "stopping",
);
const canLaunch = computed(() => !launchLocked.value && !launching.value);
const canStop = computed(() => sessionStatus.value !== "idle" && !stopping.value);
const radarDisabled = computed(() => !launch.injectPlugin);
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

const resolutionOptions = computed(() => [
  { label: t("main.settings.resolution_16_9"), value: "16:9" },
  { label: t("main.settings.resolution_4_3"), value: "4:3" },
  { label: t("main.settings.resolution_4_3_1280x960"), value: "4:3_1280x960" },
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

const canPlay = computed(
  () =>
    canSend.value &&
    !playingRequest.value &&
    !sessionState.value.demo_play_pending &&
    !!selectedDebugDemo.value &&
    !!selectedDebugDemo.value.file_path,
);
const playDisabledHint = computed(() => {
  if (!selectedDebugDemo.value) return t("main.debug.demo_empty");
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

onMounted(async () => {
  await Promise.allSettled([initDebugSession(), settingsStore.init()]);
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

function applySettingsDefaults() {
  const settings = settingsStore.draftSettings;
  // Stop tracking before applying defaults: these assignments are not user edits.
  loadingDefaults.value = false;
  if (!launchTouched.povHud) launch.povHud = settings.pov_hud_enabled;
  if (!launchTouched.povRadar) launch.povRadar = settings.pov_radar_enabled;
  if (!launchTouched.resolution) launch.resolution = settings.launch_resolution;
}

function applySessionToDraft(state: DebugSessionState) {
  launch.injectPlugin = state.inject_plugin;
  launch.povHud = state.pov_hud_enabled;
  launch.brandVpk = state.brand_enabled;
  launch.povRadar = state.pov_radar_enabled;
  launch.resolution = state.launch_resolution;
}

function buildLaunchRequest(): DebugLaunchRequest {
  return {
    inject_plugin: launch.injectPlugin,
    pov_hud_enabled: launch.povHud,
    brand_enabled: launch.brandVpk,
    pov_radar_enabled: launch.povRadar,
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

/**
 * Debug-draft game commands applied when the Demo can accept them. The launch
 * card currently only owns the POV radar toggle; 05 expands this from the
 * dedicated debug settings draft.
 */
function buildPlaybackCommands(): string[] {
  const commands: string[] = [];
  if (launch.povRadar) commands.push("csdm_radar_pov 1");
  return commands;
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

.control-select {
  width: 180px;
}

.control-select--wide {
  width: 100%;
  max-width: 360px;
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
