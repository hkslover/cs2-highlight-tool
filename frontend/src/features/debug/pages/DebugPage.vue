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
              <span class="setting-label">{{ t("main.debug.plugin_mode") }}</span>
              <div class="setting-control">
                <n-select
                  v-model:value="launch.pluginMode"
                  :options="pluginModeOptions"
                  :disabled="launchLocked"
                  class="control-select"
                />
                <n-tag v-if="launchLocked" size="small" :bordered="false" type="warning">
                  {{ t("main.debug.restart_required") }}
                </n-tag>
              </div>
            </div>

            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.pov_hud") }}</span>
              <div class="setting-control">
                <n-switch v-model:value="launch.povHud" :disabled="launchLocked" />
                <n-tag v-if="launchLocked" size="small" :bordered="false" type="warning">
                  {{ t("main.debug.restart_required") }}
                </n-tag>
              </div>
            </div>

            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.brand_vpk") }}</span>
              <div class="setting-control">
                <n-switch v-model:value="launch.brandVpk" :disabled="launchLocked" />
                <n-tag v-if="launchLocked" size="small" :bordered="false" type="warning">
                  {{ t("main.debug.restart_required") }}
                </n-tag>
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
                <n-tag v-if="launchLocked" size="small" :bordered="false" type="warning">
                  {{ t("main.debug.restart_required") }}
                </n-tag>
              </div>
            </div>

            <div class="action-row">
              <n-button type="primary" disabled>{{ t("main.debug.btn_launch") }}</n-button>
              <n-button secondary disabled>{{ t("main.debug.btn_stop") }}</n-button>
              <span class="setting-hint">{{ t("main.debug.not_wired") }}</span>
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
              <n-tag size="small" :bordered="false">{{ t("main.debug.session_idle") }}</n-tag>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.plugin_state") }}</span>
              <n-tag size="small" :bordered="false">{{ t("main.debug.not_wired") }}</n-tag>
            </div>
            <span class="setting-hint">{{ t("main.debug.session_hint") }}</span>
          </n-space>
        </n-card>

        <n-card size="small" :bordered="true" class="debug-card">
          <template #header>
            <span class="card-title">{{ t("main.debug.demo_title") }}</span>
          </template>

          <div v-if="selectedEntry" class="demo-info">
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.demo_name") }}</span>
              <span class="setting-value">{{ selectedEntry.file_name }}</span>
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.debug.demo_path") }}</span>
              <span class="setting-value setting-value--path">{{ selectedEntry.file_path }}</span>
            </div>
          </div>
          <n-empty v-else size="small" :description="t('main.debug.demo_empty')" />

          <div class="action-row">
            <n-button size="small" secondary @click="goToImport">
              {{ t("main.debug.btn_back_import") }}
            </n-button>
            <n-button size="small" type="primary" disabled>
              {{ t("main.debug.btn_play_demo") }}
            </n-button>
          </div>
        </n-card>

        <n-card size="small" :bordered="true" class="debug-card">
          <template #header>
            <span class="card-title">{{ t("main.debug.quick_title") }}</span>
          </template>

          <n-space vertical :size="10">
            <span class="setting-hint">{{ t("main.debug.quick_hint") }}</span>
            <div class="quick-grid">
              <n-button
                v-for="item in quickCommands"
                :key="item.key"
                size="small"
                secondary
                disabled
              >
                {{ t(item.labelKey) }}
              </n-button>
            </div>
          </n-space>
        </n-card>

        <n-card size="small" :bordered="true" class="debug-card debug-card--wide">
          <template #header>
            <span class="card-title">{{ t("main.debug.cmd_title") }}</span>
          </template>

          <n-space vertical :size="10">
            <n-input
              v-model:value="commandText"
              type="textarea"
              :autosize="{ minRows: 3, maxRows: 6 }"
              :placeholder="t('main.debug.cmd_placeholder')"
            />
            <div class="action-row">
              <n-button type="primary" disabled>{{ t("main.debug.btn_send") }}</n-button>
              <span class="setting-hint">{{ t("main.debug.not_wired") }}</span>
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
import { useSettingsStore } from "@/domains/settings/settings-state";
import { selectedEntry } from "@/domains/demo/demo-state";
import type { ClipSettings } from "@/shared/types/clips";

type DebugPluginMode = "plugin" | "none";
type LaunchResolution = ClipSettings["launch_resolution"];

interface DebugRecordingResult {
  id: string;
  name: string;
  status: string;
}

const router = useRouter();
const settingsStore = useSettingsStore();

// Temporary UI draft. Step 02 replaces this local flag with the backend debug
// session state; until then no launch/recording API exists and every action
// stays disabled, so nothing here claims a running game or a live connection.
const sessionRunning = ref(false);

const launch = reactive({
  pluginMode: "plugin" as DebugPluginMode,
  povHud: true,
  brandVpk: true,
  povRadar: false,
  resolution: "4:3" as LaunchResolution,
});

const commandText = ref("");
const recordingResults = ref<DebugRecordingResult[]>([]);

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

const launchLocked = computed(() => sessionRunning.value);
const radarDisabled = computed(() => launch.pluginMode === "none");

const pluginModeOptions = computed(() => [
  { label: t("main.debug.plugin_mode_plugin"), value: "plugin" },
  { label: t("main.debug.plugin_mode_none"), value: "none" },
]);

const resolutionOptions = computed(() => [
  { label: t("main.settings.resolution_16_9"), value: "16:9" },
  { label: t("main.settings.resolution_4_3"), value: "4:3" },
  { label: t("main.settings.resolution_4_3_1280x960"), value: "4:3_1280x960" },
]);

const quickCommands = [
  { key: "sky-off", labelKey: "main.debug.quick_sky_off" },
  { key: "sky-on", labelKey: "main.debug.quick_sky_on" },
  { key: "ui-hide", labelKey: "main.debug.quick_ui_hide" },
  { key: "ui-show", labelKey: "main.debug.quick_ui_show" },
];

onMounted(async () => {
  // The debug draft starts from the confirmed settings, but never writes back
  // to them: debug actions must not change the production configuration.
  try {
    await settingsStore.init();
  } catch {
    // Keep the placeholder draft; the debug page does not persist settings.
  }
  const settings = settingsStore.draftSettings;
  // Stop tracking before applying defaults: these assignments are not user edits.
  loadingDefaults.value = false;
  if (!launchTouched.povHud) launch.povHud = settings.pov_hud_enabled;
  if (!launchTouched.povRadar) launch.povRadar = settings.pov_radar_enabled;
  if (!launchTouched.resolution) launch.resolution = settings.launch_resolution;
});

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
  margin-bottom: 12px;
}

.quick-grid {
  display: grid;
  gap: 8px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.result-head {
  border-top: 1px solid #252c27;
  padding-top: 8px;
}

@media (max-width: 760px) {
  .debug-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
