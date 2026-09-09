<template>
  <div>
    <div class="transition-box">
      <span class="transition-label">{{ t("main.edit.transition_mode") }}</span>
      <n-radio-group
        size="small"
        :value="transitionMode"
        :disabled="exporting"
        @update:value="handleTransitionModeChange"
      >
        <n-radio-button value="none">{{ t("main.edit.transition_none") }}</n-radio-button>
        <n-radio-button value="fade">{{ t("main.edit.transition_fade") }}</n-radio-button>
      </n-radio-group>
      <n-select
        v-if="transitionMode === 'fade'"
        size="small"
        class="transition-select"
        :value="transitionDuration"
        :disabled="exporting"
        :options="transitionDurationOptions"
        @update:value="handleTransitionDurationChange"
      />
      <n-button
        class="more-config-btn"
        size="tiny"
        quaternary
        :type="opponentFastEditEnabled ? 'primary' : 'default'"
        @click="moreConfigOpen = !moreConfigOpen"
      >
        {{ t("main.edit.more_config") }}
        <span class="chevron" :class="{ open: moreConfigOpen }">▾</span>
      </n-button>
    </div>

    <n-collapse-transition :show="moreConfigOpen">
      <div class="more-config-panel">
        <div class="fast-edit-item">
          <span class="config-label">{{ t("main.edit.opponent_fast_edit") }}</span>
          <n-switch
            size="small"
            :value="opponentFastEditEnabled"
            :disabled="exporting"
            @update:value="handleOpponentFastEditChange"
          />
        </div>
      </div>
    </n-collapse-transition>

    <div class="sequence-actions">
      <n-space align="center" wrap>
        <n-button
          type="primary"
          :loading="exporting"
          :disabled="!hasSequence || exporting"
          @click="handleExport"
        >
          {{ exporting ? t("main.edit.exporting") : t("main.edit.export") }}
        </n-button>
        <n-tag v-if="exportPath" type="success" size="small">
          {{ t("main.edit.export_success", { path: basename(exportPath) }) }}
        </n-tag>
        <n-button
          v-if="exportPath"
          size="small"
          quaternary
          :disabled="exporting"
          @click="handleOpenFolder"
        >
          {{ t("main.produce.open_clip_folder") }}
        </n-button>
      </n-space>
      <n-space
        v-if="exporting || composeProgress.active"
        vertical
        :size="6"
        class="compose-progress-block"
      >
        <n-progress
          type="line"
          :show-indicator="true"
          :percentage="composePercent"
          status="success"
        />
        <n-text depth="3">{{ composeProgressLabel }}</n-text>
      </n-space>
      <n-alert
        v-if="exportError"
        type="error"
        closable
        @close="handleClearError"
      >
        {{ exportError }}
      </n-alert>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { t } from "@/shared/i18n";
import type { ComposeProgressMessage } from "@/shared/types";

const props = defineProps<{
  hasSequence: boolean;
  exporting: boolean;
  exportError: string;
  exportPath: string;
  transitionMode: string;
  transitionDuration: number;
  opponentFastEditEnabled: boolean;
  composeProgress: ComposeProgressMessage;
  composePercent: number;
  composeProgressLabel: string;
  transitionDurationOptions: Array<{ label: string; value: number }>;
}>();

const emit = defineEmits<{
  (e: "export"): void;
  (e: "open-folder"): void;
  (e: "clear-error"): void;
  (e: "update:transition-mode", value: string | number): void;
  (e: "update:transition-duration", value: string | number | null): void;
  (e: "update:opponent-fast-edit", value: boolean): void;
}>();

const moreConfigOpen = ref(false);

function handleTransitionModeChange(value: string | number) {
  emit("update:transition-mode", value);
}

function handleTransitionDurationChange(value: string | number | null) {
  emit("update:transition-duration", value);
}

function handleOpponentFastEditChange(value: boolean) {
  emit("update:opponent-fast-edit", value);
}

function handleExport() {
  emit("export");
}

function handleOpenFolder() {
  emit("open-folder");
}

function handleClearError() {
  emit("clear-error");
}

function basename(path: string): string {
  if (!path) return "";
  return path.replaceAll("\\", "/").split("/").pop() || path;
}
</script>

<style scoped>
.transition-box {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 10px 12px;
  border-bottom: 1px solid #303732;
  background: rgba(26, 30, 27, 0.4);
  flex-shrink: 0;
}

.transition-label {
  color: #a7b2aa;
  font-size: 12px;
}

.transition-select {
  width: 110px;
}

.more-config-btn {
  margin-left: auto;
}

.chevron {
  display: inline-block;
  margin-left: 3px;
  transition: transform 0.2s ease;
}

.chevron.open {
  transform: rotate(180deg);
}

.more-config-panel {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  padding: 8px 12px;
  background: rgba(20, 23, 21, 0.6);
  border-bottom: 1px solid #303732;
}

.fast-edit-item {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.config-label {
  color: #a7b2aa;
  font-size: 12px;
}

.sequence-actions {
  padding: 10px 12px;
  background: rgba(17, 19, 18, 0.5);
  flex-shrink: 0;
}

.sequence-actions > :not(:last-child) {
  margin-bottom: 8px;
}

.compose-progress-block {
  margin-top: 8px;
}
</style>
