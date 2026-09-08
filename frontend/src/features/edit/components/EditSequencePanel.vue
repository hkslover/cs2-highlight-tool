<template>
  <section class="edit-panel sequence-panel">
    <div class="panel-head">
      <span>{{ t("main.edit.sequence_title") }}</span>
      <n-space :size="6" align="center">
        <n-tag size="small" :bordered="false">{{ sequenceItems.length }}</n-tag>
        <n-tag size="small" :bordered="false" type="info">
          {{ totalDuration.toFixed(1) }}s
        </n-tag>
      </n-space>
    </div>

    <n-empty
      v-if="!sequenceItems.length"
      :description="t('main.edit.sequence_empty')"
      size="small"
    />

    <div v-else class="sequence-list">
      <div
        v-for="(item, index) in sequenceItems"
        :key="item.id"
        class="sequence-item"
      >
        <div class="sequence-main">
          <div class="sequence-title">#{{ index + 1 }}</div>
          <div class="sequence-meta">
            <n-tag
              size="tiny"
              :bordered="false"
              :type="viewTagType(item.historyItem)"
            >
              {{ viewLabel(item.historyItem) }}
            </n-tag>
            <span class="sequence-sub">{{ item.duration.toFixed(1) }}s</span>
          </div>

          <div v-if="item.historyItem.kills?.length" class="sequence-kills">
            <div
              v-for="kill in item.historyItem.kills"
              :key="kill.id"
              class="sequence-kill-row"
            >
              <DeathNoticeLine :kill="kill" compact />
            </div>
          </div>
          <div
            v-else-if="historyItemView(item.historyItem) === 'full_round_pov'"
            class="sequence-sub"
          >
            {{ povRowStatusLabel(item.historyItem) }}
          </div>
          <div v-else class="sequence-sub">
            {{ t("topbar.history_kill_count", { count: item.historyItem.kill_ids?.length || 0 }) }}
          </div>
        </div>

        <n-space :size="6" align="center">
          <n-button
            size="tiny"
            quaternary
            :disabled="index === 0 || exporting"
            @click="moveSequenceItemUp(index)"
          >
            {{ t("main.edit.move_up") }}
          </n-button>
          <n-button
            size="tiny"
            quaternary
            :disabled="index === sequenceItems.length - 1 || exporting"
            @click="moveSequenceItemDown(index)"
          >
            {{ t("main.edit.move_down") }}
          </n-button>
          <n-button
            size="tiny"
            type="error"
            tertiary
            :disabled="exporting"
            @click="removeSequenceItem(index)"
          >
            {{ t("main.edit.remove_clip") }}
          </n-button>
        </n-space>
      </div>
    </div>

    <EditConcatPanel
      :has-sequence="sequenceItems.length > 0"
      :exporting="exporting"
      :export-error="exportError"
      :export-path="exportPath"
      :transition-mode="transitionMode"
      :transition-duration="transitionDuration"
      :compose-progress="composeProgress"
      :compose-percent="composePercent"
      :compose-progress-label="composeProgressLabel"
      :transition-duration-options="transitionDurationOptions"
      @export="exportSequence"
      @clear="clearSequence"
      @open-folder="openExportedClipFolder"
      @clear-error="clearExportError"
      @update:transition-mode="handleTransitionModeChange"
      @update:transition-duration="handleTransitionDurationChange"
    />
  </section>
</template>

<script setup lang="ts">
import { t } from "@/shared/i18n";
import DeathNoticeLine from "@/shared/ui/DeathNoticeLine.vue";
import EditConcatPanel from "@/features/edit/components/EditConcatPanel.vue";
import { useEditPage } from "@/features/edit/composables/useEditPage";
import { useEditState } from "@/features/edit/composables/useEditState";
import { historyItemView } from "@/domains/edit/history";
import type { ProduceHistoryItem } from "@/shared/types";

const {
  sequenceItems,
  exporting,
  exportError,
  exportPath,
  transitionMode,
  transitionDuration,
  totalDuration,
  composeProgress,
  composePercent,
  composeProgressLabel,
  transitionDurationOptions,
  handleTransitionModeChange,
  handleTransitionDurationChange,
  exportSequence,
  clearExportError,
  openExportedClipFolder,
} = useEditPage();

const {
  moveSequenceItemUp,
  moveSequenceItemDown,
  removeSequenceItem,
  clearSequence,
} = useEditState();

function viewLabel(item: ProduceHistoryItem): string {
  const view = historyItemView(item);
  if (view === "victim") return t("main.clips.victim_view");
  if (view === "full_round_pov") return t("main.clips.full_round_pov_tag");
  return t("main.clips.killer_view");
}

function viewTagType(item: ProduceHistoryItem): "success" | "warning" | "info" {
  const view = historyItemView(item);
  if (view === "victim") return "warning";
  if (view === "full_round_pov") return "info";
  return "success";
}

function povRowStatusLabel(item: ProduceHistoryItem): string {
  const round = Number(item.round || 0);
  const kills = item.kills?.length || 0;
  const died = String(item.end_reason || "").toLowerCase() === "target_death";
  const key = died
    ? "main.clips.full_round_pov_round_title_died"
    : "main.clips.full_round_pov_round_title_survived";
  return t(key, { round, kills });
}
</script>

<style scoped>
.edit-panel {
  border: 1px solid #303732;
  border-radius: 8px;
  background: rgba(17, 19, 18, 0.45);
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.sequence-panel {
  min-width: 0;
}

.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border-bottom: 1px solid #303732;
  color: #edf1ee;
  font-size: 13px;
  font-weight: 600;
  flex-shrink: 0;
}

.sequence-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.sequence-item {
  border: 1px solid #2f3631;
  border-radius: 8px;
  padding: 8px;
  background: rgba(26, 30, 27, 0.55);
  display: flex;
  justify-content: space-between;
  gap: 10px;
}

.sequence-main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.sequence-title {
  color: #edf1ee;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sequence-sub {
  color: #8d9890;
  font-size: 12px;
}

.sequence-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sequence-kills {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  min-width: 0;
}

.sequence-kill-row {
  width: 100%;
}

@media (max-width: 980px) {
  .sequence-item {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
