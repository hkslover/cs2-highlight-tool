<template>
  <main class="startup-shell" :class="{ 'has-ready-bar': state.can_enter_main }">
    <n-space vertical :size="18">
      <n-card class="hero-card" :bordered="false">
        <n-space vertical :size="6">
          <n-h1 class="hero-title">{{ t("startup.title") }}</n-h1>
        </n-space>
      </n-card>

      <n-alert v-if="state.fatal_error" type="error" :show-icon="true">
        {{ state.fatal_error }}
      </n-alert>

      <n-card class="tasks-card" :bordered="false">
        <template #header>
          <span class="tasks-head-title">{{ t("startup.tasks_title") }}</span>
        </template>
        <template #header-extra>
          <n-space :size="8" :wrap="false">
            <n-button size="small" :disabled="busy" @click="exportLogs">
              {{ t("startup.export_logs") }}
            </n-button>
            <n-button
              size="small"
              type="error"
              :disabled="!canReset"
              @click="confirmReset"
            >
              {{ t("workspace.reset.button") }}
            </n-button>
          </n-space>
        </template>

        <n-list class="task-list" bordered :show-divider="false">
          <n-list-item
            v-for="task in tasks"
            :key="task.id"
            class="task-item"
            :class="task.status"
          >
            <div class="task-top">
              <div class="task-title-wrap">
                <n-space align="center" :size="10">
                  <n-text strong class="task-name">{{ task.name }}</n-text>
                  <n-tag
                    :type="statusTagType(task.status)"
                    size="small"
                    round
                    :bordered="false"
                  >
                    {{ statusText(task.status) }}
                  </n-tag>
                </n-space>
              </div>

              <n-space
                v-if="showActions(task)"
                class="task-actions"
                :size="8"
                :wrap="true"
                justify="end"
              >
                <n-button
                  v-if="task.kind === 'self_update' && state.self_update.available"
                  type="primary"
                  size="small"
                  :disabled="busy"
                  @click="openSelfUpdateDownload"
                >
                  {{ t("startup.actions.download_self_update") }}
                </n-button>

                <template v-else-if="task.kind === 'component' && task.component">
                  <n-button
                    v-if="showReinstall(task.component)"
                    size="small"
                    :disabled="busy"
                    @click="reinstall(task.component.id)"
                  >
                    {{ t("startup.actions.reinstall") }}
                  </n-button>
                  <n-button
                    v-if="task.component.id === 'cs2'"
                    size="small"
                    :disabled="busy"
                    @click="pickCS2Path"
                  >
                    {{ cs2ActionText(task.status) }}
                  </n-button>
                  <template
                    v-if="
                      task.component.id !== 'cs2' &&
                      (task.status === 'downloading' || canRetry(task))
                    "
                  >
                    <n-button
                      v-if="task.status === 'downloading'"
                      type="warning"
                      size="small"
                      @click="cancelDownload(task.component.id)"
                    >
                      {{ t("startup.actions.cancel_download") }}
                    </n-button>
                    <n-button
                      v-else
                      type="primary"
                      size="small"
                      :disabled="busy"
                      @click="retry(task.component.id)"
                    >
                      {{ t("startup.actions.retry") }}
                    </n-button>
                    <template
                      v-if="task.status === 'failed' || task.status === 'needs_action'"
                    >
                      <n-button
                        size="small"
                        :disabled="busy || !task.manual_url"
                        @click="openManual(task.component.id)"
                      >
                        {{ t("startup.actions.download_page") }}
                      </n-button>
                      <n-space :size="4" align="center" :wrap="false">
                        <n-button
                          size="small"
                          :disabled="busy"
                          @click="importManual(task.component.id)"
                        >
                          {{ t("startup.actions.import_file") }}
                        </n-button>
                        <n-tooltip trigger="hover" placement="top">
                          <template #trigger>
                            <span class="hint-dot">?</span>
                          </template>
                          {{ importHint(task.component.id) }}
                        </n-tooltip>
                      </n-space>
                    </template>
                  </template>
                </template>
              </n-space>
            </div>

            <n-text
              v-if="task.error"
              :depth="taskMessageDepth(task)"
              :type="taskMessageType(task)"
              class="task-message"
            >
              {{ task.error }}
            </n-text>

            <div
              v-if="taskVersionMeta(task) || showChannelSelect(task.component)"
              class="task-meta-row"
            >
              <n-text depth="3" class="task-meta">
                {{ taskVersionMeta(task) }}
              </n-text>
              <n-select
                v-if="task.component && showChannelSelect(task.component)"
                class="channel-select"
                size="tiny"
                :value="task.component.channel"
                :options="channelSelectOptions(task.component)"
                :render-label="renderChannelLabel"
                :render-option="renderChannelOption"
                :disabled="busy || !canSelectChannel(task)"
                :consistent-menu-width="false"
                :theme-overrides="channelSelectTheme"
                @update:value="selectChannel"
              />
            </div>

            <div v-if="showProgress(task)" class="task-progress">
              <n-progress
                type="line"
                :percentage="progressPercent(task)"
                :indeterminate="isIndeterminate(task)"
                :show-indicator="showPercent(task)"
                :height="8"
                :border-radius="999"
                :processing="isIndeterminate(task)"
                status="success"
              />
            </div>
          </n-list-item>
        </n-list>
      </n-card>

    </n-space>

    <div v-if="state.can_enter_main" class="ready-cta-bar" role="status" aria-live="polite">
      <div class="ready-cta-inner">
        <div class="ready-cta-left">
          <span class="ready-cta-icon" aria-hidden="true">✓</span>
          <span class="ready-cta-text">{{ t("startup.ready_text") }}</span>
        </div>
        <n-button
          type="primary"
          class="ready-cta-button"
          :disabled="busy"
          @click="enterMain"
        >
          {{ t("startup.enter_main") }}
        </n-button>
      </div>
    </div>
  </main>
</template>

<script setup lang="ts">
import { useStartupWizard } from "@/features/startup/composables/useStartupWizard";
import type { ProgressMessage, StartupState } from "@/shared/types";

const props = defineProps<{
  state: StartupState;
  progressMap: Record<string, ProgressMessage>;
}>();

const {
  t,
  busy,
  canReset,
  tasks,
  statusText,
  statusTagType,
  showActions,
  showReinstall,
  canRetry,
  taskMessageType,
  taskMessageDepth,
  taskVersionMeta,
  showProgress,
  progressPercent,
  isIndeterminate,
  showPercent,
  importHint,
  cs2ActionText,
  showChannelSelect,
  channelSelectOptions,
  channelSelectTheme,
  renderChannelLabel,
  renderChannelOption,
  canSelectChannel,
  selectChannel,
  retry,
  reinstall,
  openManual,
  cancelDownload,
  importManual,
  pickCS2Path,
  openSelfUpdateDownload,
  enterMain,
  exportLogs,
  confirmReset,
} = useStartupWizard(props);
</script>

<style scoped>
/* ── Shell ── */
.startup-shell {
  margin: 0 auto;
  max-width: 920px;
  padding: 24px 24px 32px;
}

.startup-shell.has-ready-bar {
  padding-bottom: 128px;
}

/* ── Hero ── */
.hero-card {
  background: linear-gradient(135deg, #1b2420 0%, #151815 100%);
}

.eyebrow {
  color: #85d3a7;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.hero-title {
  margin: 0 !important;
  font-size: 26px;
  font-weight: 600;
  line-height: 1.25;
}

/* ── Tasks card ── */
.tasks-head-title {
  font-size: 16px;
  font-weight: 600;
}

.task-list {
  background: transparent;
  border: none;
}

/* ── Task item with left color-band ── */
.task-item {
  background: #1c201e;
  border: 1px solid #2a3330 !important;
  border-left: 3px solid #2a3330 !important;
  border-radius: 8px;
  display: grid;
  gap: 8px;
  margin-bottom: 8px;
  padding: 12px 16px;
  transition: border-color 0.3s, background 0.3s;
}

.task-item:last-child {
  margin-bottom: 0;
}

.task-item.checking,
.task-item.downloading,
.task-item.installing {
  border-left-color: #3a9bdc !important;
  background: #191f1d;
}

.task-item.ready {
  border-left-color: #2f9462 !important;
  border-color: #264f3a !important;
}

.task-item.failed,
.task-item.needs_action {
  border-left-color: #a94f4f !important;
  border-color: #4a2828 !important;
}

.task-item.warning {
  border-left-color: #d09f49 !important;
  border-color: #4a3a1a !important;
}

/* ── Task layout ── */
.task-top {
  align-items: flex-start;
  display: flex;
  gap: 8px;
  justify-content: space-between;
  flex-wrap: wrap;
}

.task-title-wrap {
  display: grid;
  gap: 4px;
}

.task-name {
  font-size: 14px;
  font-weight: 500;
}

.task-message {
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-word;
}

.task-meta {
  font-size: 12px;
}

/* The channel select sits on the version line, sized to the 12px meta text
   so the HLAE card keeps the same height as the others. */
.task-meta-row {
  align-items: center;
  display: flex;
  gap: 12px;
  justify-content: space-between;
  min-height: 20px;
}

.channel-select {
  flex: 0 0 auto;
  width: 180px;
}

.task-progress {
  padding-top: 4px;
}

.hint-dot {
  align-items: center;
  border: 1px solid #516056;
  border-radius: 50%;
  color: #9cb8a8;
  cursor: help;
  display: inline-flex;
  font-size: 11px;
  height: 18px;
  justify-content: center;
  width: 18px;
  user-select: none;
}

/* ── Status tag pulse for active states ── */
:deep(.n-tag--info-type) {
  animation: tag-pulse 2s ease-in-out infinite;
}

@keyframes tag-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.65; }
}

/* ── Bottom CTA bar ── */
.ready-cta-bar {
  bottom: calc(16px + env(safe-area-inset-bottom, 0px));
  display: flex;
  justify-content: center;
  left: 0;
  padding: 0 16px;
  pointer-events: none;
  position: fixed;
  right: 0;
  z-index: 50;
}

.ready-cta-inner {
  align-items: center;
  background: rgba(19, 26, 22, 0.82);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 0.5px solid rgba(47, 148, 98, 0.6);
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
  display: flex;
  gap: 16px;
  justify-content: space-between;
  max-width: 920px;
  padding: 12px 16px;
  pointer-events: auto;
  width: min(920px, 100%);
}

.ready-cta-left {
  align-items: center;
  display: inline-flex;
  gap: 8px;
  min-width: 0;
}

.ready-cta-icon {
  align-items: center;
  background: #2f9462;
  border-radius: 999px;
  color: #0f1914;
  display: inline-flex;
  flex: 0 0 auto;
  font-size: 13px;
  font-weight: 700;
  height: 22px;
  justify-content: center;
  line-height: 1;
  width: 22px;
}

.ready-cta-text {
  color: #e6f2ea;
  font-size: 14px;
  line-height: 1.3;
}

.ready-cta-button {
  flex: 0 0 auto;
}

/* ── Responsive ── */
@media (max-width: 820px) {
  .startup-shell {
    padding: 16px 12px;
  }

  .startup-shell.has-ready-bar {
    padding-bottom: 160px;
  }

  .hero-title {
    font-size: 22px;
  }

  .task-top {
    flex-direction: column;
  }

  .task-actions {
    justify-content: flex-start !important;
  }

  .ready-cta-bar {
    bottom: calc(12px + env(safe-area-inset-bottom, 0px));
    padding: 0 12px;
  }

  .ready-cta-inner {
    align-items: stretch;
    flex-direction: column;
    gap: 12px;
    padding: 12px;
  }

  .ready-cta-button {
    width: 100%;
  }
}
</style>
