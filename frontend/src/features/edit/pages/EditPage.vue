<template>
  <div class="edit-page">
    <n-card
      :bordered="true"
      class="edit-card"
      content-style="height: 100%; overflow: hidden; padding: 0;"
      content-class="edit-card-content"
    >
      <div class="page-head">
        <span class="panel-title">{{ t("main.edit.title") }}</span>
      </div>
      <div class="edit-body">

      <div ref="containerRef" class="edit-layout">
        <section class="edit-panel source-panel" :style="leftPanelStyle">
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
                            <n-tag
                              size="tiny"
                              :bordered="false"
                              :type="viewTagType(item)"
                            >
                              {{ viewLabel(item) }}
                            </n-tag>
                            <span class="source-time">{{ formatTime(item.completed_at_ms) }}</span>
                            <span
                              v-if="historyItemView(item) === 'full_round_pov'"
                              class="source-time"
                            >
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

        <div
          class="edit-splitter"
          :class="{ dragging: isResizing }"
          @mousedown="startResize"
        />

        <section class="edit-panel sequence-panel" :style="rightPanelStyle">
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
                <div v-else-if="historyItemView(item.historyItem) === 'full_round_pov'" class="sequence-sub">
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
      </div>
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { t } from "@/shared/i18n";
import DeathNoticeLine from "@/features/clips/components/DeathNoticeLine.vue";
import { ensureProduceHistoryInitialized, useProduceHistory } from "@/features/produce/composables/useProduceHistory";
import EditConcatPanel from "@/features/edit/components/EditConcatPanel.vue";
import { useEditPage } from "@/features/edit/composables/useEditPage";
import { useEditState } from "@/features/edit/composables/useEditState";
import { useSplitter } from "@/shared/composables/useSplitter";
import type { DemoClipKill, ProduceHistoryItem } from "@/shared/types";

const containerRef = ref<HTMLElement | null>(null);
const { isResizing, leftPanelStyle, rightPanelStyle, startResize } = useSplitter(containerRef, {
  initialLeftRatio: 0.44,
  minLeftPx: 280,
  minRightPx: 360,
  splitterWidthPx: 12,
});

const message = useMessage();
const { historySnapshot } = useProduceHistory();
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
  addSequenceItem,
  moveSequenceItemUp,
  moveSequenceItemDown,
  removeSequenceItem,
  clearSequence,
  setExportPath,
  setExportError,
} = useEditState();

interface SourceDemoGroup {
  demo_path: string;
  items: ProduceHistoryItem[];
}

interface SourceRoundGroup {
  name: string;
  round: number;
  kill_count: number;
  items: ProduceHistoryItem[];
}

const durationCache = ref<Record<string, number>>({});
const addingByPath = ref<Record<string, boolean>>({});
const addingAll = ref(false);
const addingAllByDemo = ref<Record<string, boolean>>({});
const sourceDemoExpanded = ref<string[]>([]);
const sourceDemoExpandedInitialized = ref(false);
const sourceRoundExpandedByDemo = ref<Record<string, string[]>>({});

const produceClipItems = computed(() =>
  [...(historySnapshot.value.items || [])]
    .filter((item) => (item.history_type || "produce_clip") === "produce_clip")
    .filter((item) => !!item.video_path)
    .sort((a, b) => (b.completed_at_ms || 0) - (a.completed_at_ms || 0)),
);

const produceClipsByDemo = computed<SourceDemoGroup[]>(() => {
  const order: string[] = [];
  const byDemo = new Map<string, ProduceHistoryItem[]>();
  for (const item of produceClipItems.value) {
    const demoPath = item.demo_path || "";
    if (!demoPath) continue;
    if (!byDemo.has(demoPath)) {
      byDemo.set(demoPath, []);
      order.push(demoPath);
    }
    byDemo.get(demoPath)!.push(item);
  }
  return order.map((demoPath) => ({ demo_path: demoPath, items: byDemo.get(demoPath) || [] }));
});

const sourceRoundGroupsByDemo = computed(() => {
  const next = new Map<string, SourceRoundGroup[]>();
  for (const demoGroup of produceClipsByDemo.value) {
    const byRound = new Map<string, SourceRoundGroup>();
    for (const item of demoGroup.items) {
      if (historyItemView(item) === "full_round_pov") {
        const name = "pov-group";
        if (!byRound.has(name)) {
          byRound.set(name, { name, round: 0, kill_count: 0, items: [] });
        }
        byRound.get(name)!.items.push(item);
        byRound.get(name)!.kill_count += (item.kills || []).length;
        continue;
      }
      const kills = (item.kills || []).filter((k): k is DemoClipKill => !!k?.id);
      const split = splitKillsByRound(kills);
      if (!split.length) split.push({ round: 0, kills: [] });
      for (const part of split) {
        const name = part.round > 0 ? `round-${part.round}` : "round-unknown";
        if (!byRound.has(name)) {
          byRound.set(name, { name, round: part.round, kill_count: 0, items: [] });
        }
        byRound.get(name)!.items.push(item);
        byRound.get(name)!.kill_count += part.kills.length;
      }
    }
    const sorted = Array.from(byRound.values())
      .map((group) => {
        if (group.name !== "pov-group") return group;
        return {
          ...group,
          items: group.items.slice().sort((a, b) => Number(a.round || 0) - Number(b.round || 0)),
        };
      })
      .sort((a, b) => {
        if (a.name === "pov-group" && b.name === "pov-group") return 0;
        if (a.name === "pov-group") return -1;
        if (b.name === "pov-group") return 1;
        if (a.round <= 0 && b.round <= 0) return 0;
        if (a.round <= 0) return 1;
        if (b.round <= 0) return -1;
        return a.round - b.round;
      });
    next.set(demoGroup.demo_path, sorted);
  }
  return next;
});

watch(
  () => produceClipsByDemo.value.map((g) => g.demo_path),
  (demoPaths) => {
    if (!sourceDemoExpandedInitialized.value) {
      sourceDemoExpanded.value = [...demoPaths];
      sourceDemoExpandedInitialized.value = true;
      return;
    }
    const allowed = new Set(demoPaths);
    const existing = new Set(sourceDemoExpanded.value);
    const pruned = sourceDemoExpanded.value.filter((name) => allowed.has(name));
    const newPaths = demoPaths.filter((name) => !existing.has(name));
    sourceDemoExpanded.value = [...pruned, ...newPaths];
  },
  { immediate: true },
);

onMounted(async () => {
  try {
    await ensureProduceHistoryInitialized();
  } catch {
    // history init failure is non-fatal
  }
});

async function callBackend<T>(method: string, ...args: unknown[]): Promise<T> {
  const api = (window as any).go?.app?.App as
    | Record<string, (...a: unknown[]) => Promise<unknown>>
    | undefined;
  const fn = api?.[method];
  if (!fn) throw new Error(`Backend not available: ${method}`);
  return fn(...args) as Promise<T>;
}

function isAdding(videoPath: string): boolean {
  return !!addingByPath.value[videoPath || ""];
}

function setAdding(videoPath: string, value: boolean) {
  const key = videoPath || "";
  if (!key) return;
  const next = { ...addingByPath.value };
  if (value) {
    next[key] = true;
  } else {
    delete next[key];
  }
  addingByPath.value = next;
}

async function addFromHistory(item: ProduceHistoryItem) {
  const videoPath = String(item.video_path || "").trim();
  if (!videoPath) return;
  if (isAdding(videoPath)) return;

  try {
    setAdding(videoPath, true);
    let duration = durationCache.value[videoPath];
    if (!(duration > 0)) {
      duration = await callBackend<number>("ProbeClipDuration", videoPath);
      durationCache.value = {
        ...durationCache.value,
        [videoPath]: duration,
      };
    }
    addSequenceItem(item, duration);
    setExportPath("");
    setExportError("");
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err);
    message.error(t("main.edit.probe_failed", { error: msg }));
  } finally {
    setAdding(videoPath, false);
  }
}

async function addAllFromHistory() {
  if (!produceClipItems.value.length || addingAll.value) return;

  addingAll.value = true;
  setExportPath("");
  setExportError("");

  let firstErr = "";
  let failed = 0;

  for (const demoGroup of produceClipsByDemo.value) {
    for (const item of orderByView(demoGroup.items)) {
      const videoPath = String(item.video_path || "").trim();
      if (!videoPath) { failed++; continue; }
      try {
        let duration = durationCache.value[videoPath];
        if (!(duration > 0)) {
          duration = await callBackend<number>("ProbeClipDuration", videoPath);
          durationCache.value = { ...durationCache.value, [videoPath]: duration };
        }
        addSequenceItem(item, duration);
      } catch (err: unknown) {
        failed++;
        if (!firstErr) firstErr = err instanceof Error ? err.message : String(err);
      }
    }
  }

  addingAll.value = false;
  if (failed > 0 && firstErr) {
    message.warning(t("main.edit.add_all_partial", { failed, error: firstErr }));
  }
}

async function addAllFromDemo(demoGroup: SourceDemoGroup) {
  const demoPath = demoGroup.demo_path;
  if (!demoGroup.items.length || isAddingAllForDemo(demoPath) || addingAll.value) return;

  addingAllByDemo.value = { ...addingAllByDemo.value, [demoPath]: true };
  setExportPath("");
  setExportError("");

  let firstErr = "";
  let failed = 0;

  for (const item of orderByView(demoGroup.items)) {
    const videoPath = String(item.video_path || "").trim();
    if (!videoPath) { failed++; continue; }
    try {
      let duration = durationCache.value[videoPath];
      if (!(duration > 0)) {
        duration = await callBackend<number>("ProbeClipDuration", videoPath);
        durationCache.value = { ...durationCache.value, [videoPath]: duration };
      }
      addSequenceItem(item, duration);
    } catch (err: unknown) {
      failed++;
      if (!firstErr) firstErr = err instanceof Error ? err.message : String(err);
    }
  }

  const next = { ...addingAllByDemo.value };
  delete next[demoPath];
  addingAllByDemo.value = next;

  if (failed > 0 && firstErr) {
    message.warning(t("main.edit.add_all_partial", { failed, error: firstErr }));
  }
}

function isAddingAllForDemo(demoPath: string): boolean {
  return !!addingAllByDemo.value[demoPath];
}

function getSourceDemoExpanded(): string[] {
  const allowed = new Set(produceClipsByDemo.value.map((g) => g.demo_path));
  return sourceDemoExpanded.value.filter((name) => allowed.has(name));
}

function handleSourceDemoExpanded(names: Array<string | number> | string | number | null) {
  sourceDemoExpanded.value = normalizeNames(names);
}

function getSourceRoundExpanded(demoPath: string): string[] {
  const groups = sourceRoundGroupsByDemo.value.get(demoPath) || [];
  const defaults = groups.map((g) => g.name);
  if (!Object.prototype.hasOwnProperty.call(sourceRoundExpandedByDemo.value, demoPath)) {
    return defaults;
  }
  const current = sourceRoundExpandedByDemo.value[demoPath] || [];
  const allowed = new Set(defaults);
  return current.filter((name) => allowed.has(name));
}

function handleSourceRoundExpanded(
  demoPath: string,
  names: Array<string | number> | string | number | null,
) {
  sourceRoundExpandedByDemo.value = {
    ...sourceRoundExpandedByDemo.value,
    [demoPath]: normalizeNames(names),
  };
}

function sourceRoundGroupsForDemo(demoPath: string): SourceRoundGroup[] {
  return sourceRoundGroupsByDemo.value.get(demoPath) || [];
}

function sourceRoundTitle(group: SourceRoundGroup): string {
  if (group.name === "pov-group") {
    return t("topbar.history_full_round_pov_group_title", { count: group.items.length });
  }
  if (group.round > 0) {
    return t("main.clips.round_title", { round: group.round, kills: group.kill_count });
  }
  return t("main.produce.round_unknown_title", { kills: group.kill_count });
}

function normalizeNames(names: Array<string | number> | string | number | null): string[] {
  if (Array.isArray(names)) return names.map((n) => String(n));
  if (names == null) return [];
  return [String(names)];
}

function splitKillsByRound(kills: DemoClipKill[]): Array<{ round: number; kills: DemoClipKill[] }> {
  const grouped = new Map<number, DemoClipKill[]>();
  for (const kill of kills) {
    const r = Number(kill.round || 0);
    const nr = r > 0 ? r : 0;
    if (!grouped.has(nr)) grouped.set(nr, []);
    grouped.get(nr)!.push(kill);
  }
  return Array.from(grouped.entries())
    .sort((a, b) => {
      if (a[0] <= 0 && b[0] <= 0) return 0;
      if (a[0] <= 0) return 1;
      if (b[0] <= 0) return -1;
      return a[0] - b[0];
    })
    .map(([round, roundKills]) => ({ round, kills: roundKills }));
}

function basename(path: string): string {
  if (!path) return "";
  return path.replaceAll("\\", "/").split("/").pop() || path;
}

function orderByView(items: ProduceHistoryItem[]): ProduceHistoryItem[] {
  const pov = sortByTick(
    items.filter((item) => historyItemView(item) === "full_round_pov"),
  );
  const killer = sortByTick(
    items.filter((item) => historyItemView(item) === "killer"),
  );
  const victim = sortByTick(
    items.filter((item) => historyItemView(item) === "victim"),
  );
  return [...pov, ...killer, ...victim];
}

function sortByTick(items: ProduceHistoryItem[]): ProduceHistoryItem[] {
  return items.slice().sort((a, b) => {
    const tickA = resolvePrimaryTick(a);
    const tickB = resolvePrimaryTick(b);
    if (tickA !== tickB) return tickA - tickB;
    const idA = String(a.kills?.[0]?.id || a.kill_ids?.[0] || "");
    const idB = String(b.kills?.[0]?.id || b.kill_ids?.[0] || "");
    if (idA !== idB) return idA.localeCompare(idB);
    const timeA = Number(a.completed_at_ms || 0);
    const timeB = Number(b.completed_at_ms || 0);
    if (timeA !== timeB) return timeA - timeB;
    return String(a.video_path || "").localeCompare(String(b.video_path || ""));
  });
}

function resolvePrimaryTick(item: ProduceHistoryItem): number {
  if (historyItemView(item) === "full_round_pov") {
    const startTick = Number(item.start_tick || 0);
    if (Number.isFinite(startTick) && startTick > 0) return startTick;
  }
  const ticks = (item.kills || [])
    .map((kill) => Number(kill?.tick || 0))
    .filter((tick) => Number.isFinite(tick) && tick > 0);
  if (!ticks.length) return Number.MAX_SAFE_INTEGER;
  return Math.min(...ticks);
}

function historyRowKey(item: ProduceHistoryItem): string {
  return `${item.history_type || "produce_clip"}#${item.demo_path}#${item.view}#${item.spec_mode}#${item.source_id || ""}#${item.round || 0}#${item.completed_at_ms}#${item.video_path}#${(item.kill_ids || []).join("|")}`;
}

function historyItemView(item: ProduceHistoryItem): "killer" | "victim" | "full_round_pov" {
  const view = String(item.view || "").toLowerCase();
  if (view === "victim") return "victim";
  if (view === "full_round_pov") return "full_round_pov";
  if (String(item.source_id || "").toLowerCase().startsWith("full_round_pov:")) {
    return "full_round_pov";
  }
  return "killer";
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
  const key = died ? "main.clips.full_round_pov_round_title_died" : "main.clips.full_round_pov_round_title_survived";
  return t(key, { round, kills });
}

function formatTime(tsMs: number): string {
  if (!tsMs) return "-";
  const d = new Date(tsMs);
  if (Number.isNaN(d.getTime())) return "-";
  return (
    String(d.getHours()).padStart(2, "0") +
    ":" +
    String(d.getMinutes()).padStart(2, "0") +
    ":" +
    String(d.getSeconds()).padStart(2, "0")
  );
}
</script>

<style scoped>
.edit-page {
  height: 100%;
  min-height: 0;
}

.edit-card {
  height: 100%;
  background: #181b19;
}

.edit-card :deep(.edit-card-content) {
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.page-head {
  flex-shrink: 0;
  min-height: 34px;
  padding: 6px 10px;
  border-bottom: 1px solid #303732;
  display: flex;
  align-items: center;
}

.edit-body {
  flex: 1;
  min-height: 0;
  padding: 8px 10px 10px;
  overflow: hidden;
}

.panel-title {
  font-size: 13px;
  font-weight: 600;
}

.edit-layout {
  display: flex;
  height: 100%;
  min-height: 0;
}

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

.sequence-panel {
  min-width: 0;
}

.edit-splitter {
  position: relative;
  flex: 0 0 12px;
  cursor: col-resize;
}

.edit-splitter::before {
  content: "";
  position: absolute;
  left: 50%;
  top: 8px;
  bottom: 8px;
  width: 2px;
  border-radius: 999px;
  background: #303732;
  transform: translateX(-50%);
  transition: background-color 0.2s ease;
}

.edit-splitter:hover::before,
.edit-splitter.dragging::before {
  background: #2f9462;
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

.sequence-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
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

.source-item,
.sequence-item {
  border: 1px solid #2f3631;
  border-radius: 8px;
  padding: 8px;
  background: rgba(26, 30, 27, 0.55);
  display: flex;
  justify-content: space-between;
  gap: 10px;
}

.source-main,
.sequence-main {
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

.source-time {
  color: #8d9890;
  font-size: 12px;
}

.source-title,
.sequence-title {
  color: #edf1ee;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sequence-sub,
.source-kill-count {
  color: #8d9890;
  font-size: 12px;
}

.sequence-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.source-kills,
.sequence-kills {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  min-width: 0;
}

.source-kill-row,
.sequence-kill-row {
  width: 100%;
}

@media (max-width: 980px) {
  .edit-layout {
    flex-direction: column;
    gap: 10px;
  }

  .edit-splitter {
    display: none;
  }

  .source-panel,
  .sequence-panel {
    flex: 1 1 auto !important;
  }

  .source-item,
  .sequence-item {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
