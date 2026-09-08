<template>
  <section class="edit-panel source-panel">
    <div class="panel-head">
      <span>{{ t("main.edit.source_title") }}</span>
      <n-space :size="6" align="center">
        <n-tag size="small" :bordered="false">{{ produceClipItems.length }}</n-tag>
        <n-button
          size="tiny"
          type="primary"
          secondary
          :loading="addingAll"
          :disabled="!produceClipItems.length || addingAll"
          @click="addAllFromHistory"
        >
          {{ addingAll ? t("main.edit.add_all_loading") : t("main.edit.add_all") }}
        </n-button>
      </n-space>
    </div>

    <n-empty
      v-if="!produceClipItems.length"
      :description="t('main.edit.source_empty')"
      size="small"
    />

    <div v-else class="source-list">
      <n-collapse
        :expanded-names="getSourceDemoExpanded()"
        @update:expanded-names="handleSourceDemoExpanded"
      >
        <n-collapse-item
          v-for="demoGroup in produceClipsByDemo"
          :key="demoGroup.demo_path"
          :name="demoGroup.demo_path"
          :title="basename(demoGroup.demo_path)"
        >
          <template #header-extra>
            <div class="demo-source-actions" @click.stop>
              <n-tag size="tiny" :bordered="false">{{ demoGroup.items.length }}</n-tag>
              <n-button
                size="tiny"
                type="primary"
                secondary
                :loading="isAddingAllForDemo(demoGroup.demo_path)"
                :disabled="isAddingAllForDemo(demoGroup.demo_path) || addingAll"
                @click.stop="addAllFromDemo(demoGroup)"
              >
                {{ t("main.edit.add_all") }}
              </n-button>
            </div>
          </template>

          <n-collapse
            :expanded-names="getSourceRoundExpanded(demoGroup.demo_path)"
            @update:expanded-names="handleSourceRoundExpanded(demoGroup.demo_path, $event)"
          >
            <n-collapse-item
              v-for="roundGroup in sourceRoundGroupsForDemo(demoGroup.demo_path)"
              :key="`${demoGroup.demo_path}-${roundGroup.name}`"
              :name="roundGroup.name"
              :title="sourceRoundTitle(roundGroup)"
            >
              <div class="source-round-items">
                <div
                  v-for="item in roundGroup.items"
                  :key="historyRowKey(item)"
                  class="source-item"
                >
                  <div class="source-main">
                    <div class="source-meta">
                      <n-tag size="tiny" :bordered="false" :type="viewTagType(item)">
                        {{ viewLabel(item) }}
                      </n-tag>
                      <span class="source-time">{{ formatTime(item.completed_at_ms) }}</span>
                      <span v-if="historyItemView(item) === 'full_round_pov'" class="source-time">
                        {{ povRowStatusLabel(item) }}
                      </span>
                    </div>

                    <div v-if="item.kills?.length" class="source-kills">
                      <div v-for="kill in item.kills" :key="kill.id" class="source-kill-row">
                        <DeathNoticeLine :kill="kill" compact />
                      </div>
                    </div>
                    <div v-else-if="historyItemView(item) === 'full_round_pov'" class="source-kill-count">
                      {{ povRowStatusLabel(item) }}
                    </div>
                    <div v-else class="source-kill-count">
                      {{ t("topbar.history_kill_count", { count: item.kill_ids?.length || 0 }) }}
                    </div>
                  </div>

                  <n-button
                    size="tiny"
                    type="primary"
                    secondary
                    :loading="isAdding(item.video_path)"
                    @click="addFromHistory(item)"
                  >
                    {{ t("main.edit.add_clip") }}
                  </n-button>
                </div>
              </div>
            </n-collapse-item>
          </n-collapse>
        </n-collapse-item>
      </n-collapse>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useMessage } from "naive-ui";
import { t } from "@/shared/i18n";
import DeathNoticeLine from "@/shared/ui/DeathNoticeLine.vue";
import { initProduceHistory } from "@/domains/production";
import { useEditState } from "@/features/edit/composables/useEditState";
import { useEditHistorySource, type HistoryDemoGroup, type HistoryRoundGroup } from "@/features/edit/composables/useEditHistorySource";
import { useEditSequenceActions } from "@/features/edit/composables/useEditSequenceActions";
import {
  basename,
  historyItemView,
  historyRowKey,
  orderHistoryByView,
  formatTime,
} from "@/domains/edit/history";
import type { ProduceHistoryItem } from "@/shared/types";

const message = useMessage();
const { setExportError, setExportPath } = useEditState();
const {
  produceClipItems,
  produceClipsByDemo,
  getSourceDemoExpanded,
  handleSourceDemoExpanded,
  getSourceRoundExpanded,
  handleSourceRoundExpanded,
  sourceRoundGroupsForDemo,
} = useEditHistorySource();
const { isAdding, addFromHistory: addItemToSequence, addMany } = useEditSequenceActions();

const addingAll = ref(false);
const addingAllByDemo = ref<Record<string, boolean>>({});

onMounted(async () => {
  try {
    await initProduceHistory();
  } catch {
    // History initialization failure is non-fatal; the shared snapshot may
    // still be populated by a later event.
  }
});

async function addFromHistory(item: ProduceHistoryItem) {
  try {
    if (await addItemToSequence(item)) {
      setExportPath("");
      setExportError("");
    }
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err);
    message.error(t("main.edit.probe_failed", { error: msg }));
  }
}

async function addAllFromHistory() {
  if (!produceClipItems.value.length || addingAll.value) return;
  addingAll.value = true;
  setExportPath("");
  setExportError("");
  const result = await addMany(
    produceClipsByDemo.value.flatMap((demoGroup) => orderHistoryByView(demoGroup.items)),
  );
  addingAll.value = false;
  showBatchWarning(result.failed, result.firstError);
}

async function addAllFromDemo(demoGroup: HistoryDemoGroup) {
  const demoPath = demoGroup.demo_path;
  if (!demoGroup.items.length || isAddingAllForDemo(demoPath) || addingAll.value) return;
  addingAllByDemo.value = { ...addingAllByDemo.value, [demoPath]: true };
  setExportPath("");
  setExportError("");
  const result = await addMany(orderHistoryByView(demoGroup.items));
  const next = { ...addingAllByDemo.value };
  delete next[demoPath];
  addingAllByDemo.value = next;
  showBatchWarning(result.failed, result.firstError);
}

function showBatchWarning(failed: number, firstError: string) {
  if (failed > 0 && firstError) {
    message.warning(t("main.edit.add_all_partial", { failed, error: firstError }));
  }
}

function isAddingAllForDemo(demoPath: string): boolean {
  return !!addingAllByDemo.value[demoPath];
}

function sourceRoundTitle(group: HistoryRoundGroup): string {
  if (group.name === "pov-group") {
    return t("topbar.history_full_round_pov_group_title", { count: group.items.length });
  }
  if (group.round > 0) {
    return t("main.clips.round_title", { round: group.round, kills: group.kill_count });
  }
  return t("main.produce.round_unknown_title", { kills: group.kill_count });
}

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

.source-panel {
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

.source-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 8px;
}

.demo-source-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.source-round-items {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-bottom: 4px;
}

.source-item {
  border: 1px solid #2f3631;
  border-radius: 8px;
  padding: 8px;
  background: rgba(26, 30, 27, 0.55);
  display: flex;
  justify-content: space-between;
  gap: 10px;
}

.source-main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.source-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.source-time,
.source-kill-count {
  color: #8d9890;
  font-size: 12px;
}

.source-kills {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  min-width: 0;
}

.source-kill-row {
  width: 100%;
}

@media (max-width: 980px) {
  .source-item {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
