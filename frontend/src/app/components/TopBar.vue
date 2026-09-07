<template>
  <div>
    <header class="topbar">
      <div class="topbar-inner">
        <div class="topbar-left">
          <button class="brand-btn" :title="t('topbar.brand')" @click="onBrandClick">
            <span :class="['brand', { 'brand--debug': debugEnabled }]">{{ t("topbar.brand") }}</span>
          </button>
        </div>

        <div class="topbar-center">
          <button class="donate-btn" :title="t('topbar.donate_title')" @click="donateVisible = true">
            <svg class="donate-heart" width="12" height="12" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z"/>
            </svg>
            <span class="donate-label">{{ t('topbar.donate') }}</span>
          </button>
        </div>

        <div class="topbar-right">
          <n-select
            :value="locale"
            :options="localeOptions"
            size="tiny"
            :consistent-menu-width="false"
            class="locale-select"
            @update:value="setLocale"
          />
          <n-popover
            v-model:show="gameInfoPopoverVisible"
            trigger="click"
            placement="bottom-end"
            :show-arrow="true"
          >
            <template #trigger>
              <button class="health-btn" :title="t('topbar.gameinfo_health')" @click="onGameInfoHealthClick">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path
                    d="M14.7 6.3a4.2 4.2 0 0 0-5.2 5.2L3.8 17.2a1.8 1.8 0 0 0 0 2.6l.4.4a1.8 1.8 0 0 0 2.6 0l5.7-5.7a4.2 4.2 0 0 0 5.2-5.2l-2.8 2.8-2.9-.8-.8-2.9 3.5-2.1z"
                    stroke="currentColor"
                    stroke-width="1.5"
                    stroke-linejoin="round"
                  />
                </svg>
                <span :class="['health-dot', healthDotClass]" />
              </button>
            </template>
            <div class="health-popover">
              <div class="health-popover-header">
                <span class="health-title">{{ t("topbar.gameinfo_health") }}</span>
                <span :class="['health-status', healthStatusClass]">{{ gameInfoStatusText }}</span>
              </div>
              <p class="health-message">{{ gameInfoMessage }}</p>
              <!-- <p v-if="gameInfoPath" class="health-path">{{ gameInfoPath }}</p> -->
              <p v-if="gameInfoError" class="health-error">{{ gameInfoError }}</p>
              <div class="health-actions">
                <n-button
                  v-if="needsRepair"
                  size="tiny"
                  type="error"
                  :loading="repairing"
                  @click="repairGameInfo"
                >
                  {{ t("topbar.gameinfo_repair") }}
                </n-button>
                <n-button size="tiny" :loading="loading" @click="refreshGameInfoHealth">
                  {{ t("topbar.gameinfo_refresh") }}
                </n-button>
              </div>
            </div>
          </n-popover>
          <button class="history-btn" :title="t('topbar.history')" @click="openHistoryDrawer">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <circle cx="12" cy="12" r="8.5" stroke="currentColor" stroke-width="1.5" />
              <path d="M12 7.8v4.5l3 1.8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
            </svg>
          </button>
          <button class="settings-btn" :title="t('topbar.settings')" @click="openSettings">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                d="M10.2 2.6a1 1 0 0 1 1.6 0l1.2 1.6a1 1 0 0 0 1.1.37l1.9-.55a1 1 0 0 1 1.4.81l.18 1.98a1 1 0 0 0 .73.9l1.93.5a1 1 0 0 1 .5 1.52l-1.1 1.66a1 1 0 0 0 0 1.1l1.1 1.66a1 1 0 0 1-.5 1.52l-1.93.5a1 1 0 0 0-.73.9l-.17 1.98a1 1 0 0 1-1.42.81l-1.88-.55a1 1 0 0 0-1.12.37l-1.2 1.6a1 1 0 0 1-1.6 0l-1.2-1.6a1 1 0 0 0-1.12-.37l-1.88.55a1 1 0 0 1-1.42-.81l-.17-1.98a1 1 0 0 0-.73-.9l-1.93-.5a1 1 0 0 1-.5-1.52l1.1-1.66a1 1 0 0 0 0-1.1l-1.1-1.66a1 1 0 0 1 .5-1.52l1.93-.5a1 1 0 0 0 .73-.9l.18-1.98a1 1 0 0 1 1.4-.81l1.9.55a1 1 0 0 0 1.1-.37z"
                stroke="currentColor"
                stroke-width="1.5"
              />
              <circle cx="12" cy="12" r="3" stroke="currentColor" stroke-width="1.5" />
            </svg>
          </button>

          <div class="win-controls">
            <button
              class="win-btn"
              :title="t('topbar.minimize')"
              @click="WindowMinimise"
            >
              <svg width="10" height="1" viewBox="0 0 10 1"><rect width="10" height="1" fill="currentColor" /></svg>
            </button>
            <button
              class="win-btn"
              :title="t('topbar.maximize')"
              @click="WindowToggleMaximise"
            >
              <svg width="10" height="10" viewBox="0 0 10 10"><rect x="0.5" y="0.5" width="9" height="9" rx="1.5" fill="none" stroke="currentColor" stroke-width="1" /></svg>
            </button>
            <button
              class="win-btn win-btn--close"
              :title="t('topbar.close')"
              @click="Quit"
            >
              <svg width="10" height="10" viewBox="0 0 10 10"><path d="M1 1l8 8M9 1l-8 8" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" /></svg>
            </button>
          </div>
        </div>
      </div>
    </header>

    <n-drawer v-model:show="historyVisible" placement="right" :width="560">
      <ProduceHistoryDropdown
        ref="historyDropdownRef"
        :history-snapshot="historySnapshot"
        @export="onHistoryExported"
      />
    </n-drawer>

    <n-drawer v-model:show="settingsVisible" placement="right" :width="560">
      <n-drawer-content :title="t('main.settings.title')" closable>
        <SettingsPanel :active="settingsVisible" />
      </n-drawer-content>
    </n-drawer>

    <n-modal v-model:show="donateVisible" :mask-closable="true">
      <div class="donate-card">
        <button class="donate-card-close" :title="t('topbar.donate_modal_close')" @click="donateVisible = false">
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
            <path d="M1 1l12 12M13 1L1 13" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
          </svg>
        </button>
        <div class="donate-card-header">
          <svg class="donate-card-heart" width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z"/>
          </svg>
          <span class="donate-card-title">{{ t('topbar.donate_modal_title') }}</span>
        </div>
        <div class="donate-card-body">
          <div class="donate-qr-frame">
            <img class="donate-qr-img" src="/donate-qrcode.jpg" :alt="t('topbar.donate_title')" />
          </div>
          <p class="donate-card-subtitle">{{ t('topbar.donate_modal_subtitle') }}</p>
          <!-- <p class="donate-card-hint">{{ t('topbar.donate_modal_hint') }}</p> -->
        </div>
      </div>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { useMessage } from "naive-ui";
import { WindowMinimise, WindowToggleMaximise, Quit } from "../../../wailsjs/runtime/runtime";
import { useI18n } from "@/shared/i18n";
import { useProduceHistory } from "@/features/produce/composables/useProduceHistory";
import { useDebugSettings } from "@/shared/state/useDebugSettings";
import { useGameInfoHealth } from "@/app/composables/useGameInfoHealth";
import SettingsPanel from "@/features/settings/components/SettingsPanel.vue";
import ProduceHistoryDropdown from "@/app/components/ProduceHistoryDropdown.vue";
import { OPEN_PRODUCE_HISTORY_EVENT } from "@/shared/events";

const { locale, setLocale, t } = useI18n();
const message = useMessage();
const { debugEnabled, activateDebugByBrandClick } = useDebugSettings();
const { historySnapshot } = useProduceHistory();
const {
  health: gameInfoHealth,
  loading,
  repairing,
  lastError,
  needsRepair,
  isHealthy,
  isUnknown,
  refresh,
  repair,
} = useGameInfoHealth();
const historyVisible = ref(false);
const settingsVisible = ref(false);
const donateVisible = ref(false);
const gameInfoPopoverVisible = ref(false);
const autoOpenedGameInfoPopover = ref(false);
const historyDropdownRef = ref<InstanceType<typeof ProduceHistoryDropdown> | null>(null);

const onOpenProduceHistory = () => {
  openHistoryDrawer();
};

const localeOptions = computed(() => [
  { label: t("common.locale.zh"), value: "zh-CN" },
  { label: t("common.locale.en"), value: "en-US" },
]);

const healthDotClass = computed(() => {
  if (needsRepair.value) return "health-dot--error";
  if (isHealthy.value) return "health-dot--ok";
  return "health-dot--unknown";
});

const healthStatusClass = computed(() => {
  if (needsRepair.value) return "health-status--error";
  if (isHealthy.value) return "health-status--ok";
  return "health-status--unknown";
});

const gameInfoStatusText = computed(() => {
  if (needsRepair.value) return t("topbar.gameinfo_status_needs_repair");
  if (isHealthy.value) return t("topbar.gameinfo_status_ok");
  return t("topbar.gameinfo_status_unknown");
});

const gameInfoMessage = computed(() => {
  if (gameInfoHealth.value.message) return gameInfoHealth.value.message;
  if (needsRepair.value) return t("topbar.gameinfo_message_needs_repair");
  if (isHealthy.value) return t("topbar.gameinfo_message_ok");
  return t("topbar.gameinfo_message_unknown");
});

const gameInfoPath = computed(() => gameInfoHealth.value.gameinfo_path || "");
const gameInfoError = computed(() => lastError.value || gameInfoHealth.value.error || "");

onMounted(async () => {
  window.addEventListener(OPEN_PRODUCE_HISTORY_EVENT, onOpenProduceHistory);
  const next = await refresh();
  if (next.needs_repair && !autoOpenedGameInfoPopover.value) {
    autoOpenedGameInfoPopover.value = true;
    gameInfoPopoverVisible.value = true;
  }
});

onBeforeUnmount(() => {
  window.removeEventListener(OPEN_PRODUCE_HISTORY_EVENT, onOpenProduceHistory);
});

function openSettings() {
  settingsVisible.value = true;
}

async function onGameInfoHealthClick() {
  if (isUnknown.value || gameInfoError.value) {
    await refreshGameInfoHealth();
  }
}

async function refreshGameInfoHealth() {
  await refresh();
}

async function repairGameInfo() {
  try {
    const next = await repair();
    if (!next.needs_repair) {
      gameInfoPopoverVisible.value = false;
      message.success(t("topbar.gameinfo_repair_success"));
    }
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : String(error);
    message.error(t("topbar.gameinfo_repair_failed", { error: errorMessage }));
  }
}

async function openHistoryDrawer() {
  historyVisible.value = true;
  // Allow history dropdown to init after drawer opens
  await nextTick();
  historyDropdownRef.value?.ensureInit();
}

function onBrandClick() {
  activateDebugByBrandClick();
}

function onHistoryExported() {
  // Refresh handled by composable, no additional work needed
}
</script>

<style scoped>
.topbar {
  background: rgba(17, 19, 18, 0.98);
  border-bottom: 1px solid #303732;
  position: sticky;
  top: 0;
  z-index: 70;
  user-select: none;
  cursor: default;
  --wails-draggable: drag;
}

.topbar-inner {
  align-items: center;
  display: flex;
  justify-content: space-between;
  height: 38px;
  padding: 0 0 0 12px;
  position: relative;
}

.topbar-left {
  align-items: center;
  display: flex;
  gap: 8px;
  min-width: 0;
}

.brand-btn {
  background: transparent;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  padding: 2px 4px;
  --wails-draggable: no-drag;
}

.brand {
  color: #edf1ee;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.03em;
  white-space: nowrap;
}

.brand--debug {
  color: #e3923d;
}

.topbar-right {
  align-items: center;
  display: flex;
  gap: 8px;
  --wails-draggable: no-drag;
  pointer-events: auto;
}

.locale-select {
  width: 74px;
}

.health-btn,
.history-btn,
.settings-btn {
  align-items: center;
  background: transparent;
  border: none;
  border-radius: 6px;
  color: #aeb8b0;
  cursor: pointer;
  display: inline-flex;
  height: 26px;
  justify-content: center;
  padding: 0;
  transition: background 0.15s, color 0.15s;
  width: 28px;
}

.health-btn {
  position: relative;
}

.health-dot {
  border: 1px solid #111312;
  border-radius: 50%;
  bottom: 4px;
  height: 7px;
  position: absolute;
  right: 5px;
  width: 7px;
}

.health-dot--ok {
  background: #2f9462;
}

.health-dot--error {
  background: #c95d5d;
  box-shadow: 0 0 8px rgba(201, 93, 93, 0.5);
}

.health-dot--unknown {
  background: #777f7a;
}

.health-popover {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-width: 320px;
  min-width: 260px;
}

.health-popover-header {
  align-items: center;
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.health-title {
  color: #edf1ee;
  font-size: 13px;
  font-weight: 600;
}

.health-status {
  border-radius: 999px;
  font-size: 11px;
  line-height: 1;
  padding: 4px 7px;
  white-space: nowrap;
}

.health-status--ok {
  background: rgba(47, 148, 98, 0.16);
  color: #6fd69f;
}

.health-status--error {
  background: rgba(201, 93, 93, 0.16);
  color: #f09797;
}

.health-status--unknown {
  background: rgba(141, 152, 144, 0.16);
  color: #aeb8b0;
}

.health-message {
  color: #c9d3cb;
  font-size: 12px;
  line-height: 1.5;
  margin: 0;
}

.health-path {
  background: rgba(0, 0, 0, 0.18);
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 6px;
  color: #8d9890;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
  line-height: 1.45;
  margin: 0;
  max-width: 300px;
  overflow-wrap: anywhere;
  padding: 6px 7px;
}

.health-error {
  color: #f09797;
  font-size: 12px;
  line-height: 1.45;
  margin: 0;
  overflow-wrap: anywhere;
}

.health-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}

.health-btn:hover,
.history-btn:hover,
.settings-btn:hover {
  background: rgba(255, 255, 255, 0.08);
  color: #edf1ee;
}

.health-btn:active,
.history-btn:active,
.settings-btn:active {
  background: rgba(255, 255, 255, 0.12);
}

.win-controls {
  display: flex;
  margin-left: 4px;
}

.win-btn {
  align-items: center;
  background: transparent;
  border: none;
  color: #aeb8b0;
  cursor: pointer;
  display: inline-flex;
  height: 26px;
  justify-content: center;
  padding: 0;
  transition: background 0.15s, color 0.15s;
  width: 38px;
}

.win-btn:hover {
  background: rgba(255, 255, 255, 0.08);
  color: #edf1ee;
}

.win-btn--close:hover {
  background: rgba(230, 80, 80, 0.9);
  color: #fff;
}

/* ── Donate button (topbar center) ──────────────────────── */

.topbar-center {
  left: 50%;
  pointer-events: auto;
  position: absolute;
  top: 50%;
  transform: translate(-50%, -50%);
  --wails-draggable: no-drag;
}

.donate-btn {
  align-items: center;
  background: rgba(245, 166, 35, 0.08);
  border: 1px solid rgba(245, 166, 35, 0.28);
  border-radius: 20px;
  color: #dfaa60;
  cursor: pointer;
  display: inline-flex;
  font-size: 11px;
  gap: 5px;
  height: 24px;
  letter-spacing: 0.03em;
  padding: 0 10px 0 8px;
  transition: all 0.2s ease;
  white-space: nowrap;
}

.donate-btn:hover {
  background: rgba(245, 166, 35, 0.16);
  border-color: rgba(245, 166, 35, 0.6);
  box-shadow: 0 0 12px rgba(245, 166, 35, 0.25);
  color: #f7cb8b;
}

.donate-btn:active {
  background: rgba(245, 166, 35, 0.22);
}

@keyframes heartbeat {
  0%, 70%, 100% { transform: scale(1); }
  20% { transform: scale(1.22); }
  40% { transform: scale(1.05); }
}

.donate-heart {
  animation: heartbeat 2.8s ease-in-out infinite;
  color: #f5a623;
  flex-shrink: 0;
}

.donate-label {
  font-weight: 500;
}

/* ── Donate modal card ───────────────────────────────────── */

.donate-card {
  background: #141816;
  border: 1px solid rgba(245, 166, 35, 0.3);
  border-radius: 18px;
  box-shadow: 0 32px 80px rgba(0, 0, 0, 0.85), 0 0 0 1px rgba(255, 255, 255, 0.04);
  overflow: hidden;
  position: relative;
  width: 296px;
}

.donate-card-close {
  align-items: center;
  background: transparent;
  border: none;
  border-radius: 50%;
  color: rgba(174, 184, 176, 0.5);
  cursor: pointer;
  display: inline-flex;
  height: 28px;
  justify-content: center;
  padding: 0;
  position: absolute;
  right: 14px;
  top: 14px;
  transition: background 0.15s, color 0.15s;
  width: 28px;
  z-index: 1;
}

.donate-card-close:hover {
  background: rgba(255, 255, 255, 0.08);
  color: #edf1ee;
}

.donate-card-header {
  align-items: center;
  border-bottom: 1px solid rgba(245, 166, 35, 0.16);
  display: flex;
  gap: 7px;
  padding: 16px 48px 14px 20px;
}

.donate-card-heart {
  color: #f5a623;
  flex-shrink: 0;
}

.donate-card-title {
  color: #e8f0ea;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.03em;
}

.donate-card-body {
  align-items: center;
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 22px 24px 24px;
}

.donate-qr-frame {
  border-radius: 12px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.4);
  line-height: 0;
  overflow: hidden;
  padding: 10px;
}

.donate-qr-img {
  border-radius: 4px;
  display: block;
  height: 220px;
  object-fit: contain;
  width: 220px;
}

.donate-card-subtitle {
  color: #b8c8bb;
  font-size: 13px;
  letter-spacing: 0.02em;
  margin: 0;
  text-align: center;
}

.donate-card-hint {
  color: rgba(174, 184, 176, 0.55);
  font-size: 11px;
  letter-spacing: 0.04em;
  margin: 0;
  text-align: center;
}
</style>
