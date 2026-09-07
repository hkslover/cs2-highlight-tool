<template>
  <n-config-provider :theme="darkTheme" :theme-overrides="themeOverrides">
    <n-message-provider>
      <n-dialog-provider>
        <n-notification-provider>
          <div class="app-shell">
            <TopBar />
            <div class="app-content" :class="{ 'main-mode': state.mode === 'main' }">
              <template v-if="state.mode === 'main'">
                <MainApp :ads="state.ads" />
              </template>
              <template v-else>
                <StartupWizard :state="state" :progress-map="progressMap" />
                <WorkspaceInitModal v-if="state.mode === 'workspace_init'" />
              </template>
            </div>
            <ChangelogModal :pending="pendingChangelog" @close="ackChangelog" />
          </div>
        </n-notification-provider>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { onMounted, reactive, watch } from "vue";
import { darkTheme, type GlobalThemeOverrides } from "naive-ui";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import MainApp from "@/app/components/MainApp.vue";
import TopBar from "@/app/components/TopBar.vue";
import StartupWizard from "@/features/startup/components/StartupWizard.vue";
import WorkspaceInitModal from "@/features/workspace-init/components/WorkspaceInitModal.vue";
import ChangelogModal from "@/features/changelog/components/ChangelogModal.vue";
import { useChangelog } from "@/features/changelog/composables/useChangelog";
import { useI18n } from "@/shared/i18n";
import type { ProgressMessage, StartupState } from "@/shared/types";

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: "#2fb572",
    primaryColorHover: "#39cb80",
    primaryColorPressed: "#24955b",
    primaryColorSuppl: "#2fb572",
    bodyColor: "#0e1110",
    cardColor: "#151816",
    modalColor: "#171b18",
    popoverColor: "#1a201c",
    borderColor: "#2a332d",
    dividerColor: "#252c27",
    textColorBase: "#edf1ee",
    textColor1: "#edf1ee",
    textColor2: "#c6d1c8",
    textColor3: "#88958b",
    successColor: "#2fb572",
    successColorHover: "#39cb80",
    successColorPressed: "#24955b",
    warningColor: "#e5a93c",
    warningColorHover: "#f0b64d",
    errorColor: "#e05353",
    errorColorHover: "#ec6969",
    infoColor: "#4d82db",
    infoColorHover: "#6294e8",
    fontFamily:
      '"Barlow Semi Condensed","Noto Sans SC","PingFang SC","Microsoft YaHei",sans-serif',
    borderRadius: "10px",
  },
  Card: {
    borderColor: "#2a332d",
    color: "#151816",
  },
  Alert: {
    borderRadius: "10px",
  },
  Tag: {
    borderRadius: "999px",
  },
  Button: {
    borderRadiusMedium: "8px",
    borderRadiusSmall: "6px",
    borderRadiusTiny: "5px",
  },
};

const { t } = useI18n();

const state = reactive<StartupState>({
  mode: "startup",
  phase: "detecting_source",
  running: false,
  source_step: {
    status: "pending",
    source: "custom",
    country_code: "",
    message: t("app.source_waiting"),
    error: "",
  },
  fatal_error: "",
  entry_notice: "",
  ads: [],
  self_update: {
    status: "pending",
    available: false,
    current: "0.0.0",
    latest: "",
    url: "",
    error: "",
  },
  steps: [],
  can_enter_main: false,
  config: {},
});

const progressMap = reactive<Record<string, ProgressMessage>>({});

const { pending: pendingChangelog, checkPending, ack: ackChangelog } = useChangelog();

watch(
  () => state.mode,
  (mode) => {
    if (mode === "main") {
      void checkPending();
    }
  },
);

function isActiveStatus(status: string | undefined): boolean {
  return ["checking", "downloading", "installing"].includes(status || "");
}

function statusForProgressKey(key: string): string {
  if (key === "self_update") {
    return state.self_update.status || "";
  }
  const step = state.steps.find((item) => item.id === key);
  return step?.status || "";
}

function applyState(next: StartupState) {
  const wasRunning = state.running;
  Object.assign(state, next);

  if (!wasRunning && state.running) {
    for (const key of Object.keys(progressMap)) {
      delete progressMap[key];
    }
  }

  for (const [key, progress] of Object.entries(progressMap)) {
    if (!isActiveStatus(statusForProgressKey(key)) && !progress.active) {
      delete progressMap[key];
    }
  }
}

async function callBackend(method: string, ...args: unknown[]) {
  const api = window.go?.app?.App as Record<string, (...args: unknown[]) => Promise<unknown>> | undefined;
  const fn = api?.[method];
  if (!fn) {
    throw new Error(`Wails API not loaded: ${method}`);
  }
  return fn(...args);
}

onMounted(async () => {
  EventsOn("startup_state_changed", (next: StartupState) => {
    applyState(next);
  });

  EventsOn("download_progress", (next: ProgressMessage) => {
    if (!next.component_id) return;
    const safePercent = Number.isFinite(next.percent)
      ? Math.max(0, Math.min(100, next.percent))
      : 0;
    progressMap[next.component_id] = {
      ...next,
      percent: safePercent,
    };
  });

  try {
    const initial = (await callBackend("GetStartupState")) as StartupState;
    applyState(initial);
    // Skip RunStartupChecks while user is still initializing the workspace directory.
    // SetWorkspaceDir on the backend will trigger startup checks itself; we just react
    // to the resulting startup_state_changed events.
    if (state.mode !== "workspace_init") {
      await callBackend("RunStartupChecks");
    }
  } catch {
    // startup errors are surfaced through backend state/log events
  }
});
</script>

<style scoped>
.app-shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
}

.app-content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.app-content.main-mode {
  overflow: hidden;
}
</style>
