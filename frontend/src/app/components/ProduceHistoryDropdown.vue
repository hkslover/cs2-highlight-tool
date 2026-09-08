<template>
  <n-drawer-content :title="t('topbar.history_title')" closable>
    <n-spin :show="historyLoading">
      <div class="history-toolbar">
        <n-button
          size="small"
          type="warning"
          :loading="historyExporting"
          :disabled="historyExporting || !historySnapshot.items.length"
          @click="exportHistoryVideos"
        >
          {{ historyExporting ? t("topbar.history_exporting") : t("topbar.history_export") }}
        </n-button>
      </div>

      <n-empty v-if="!historySnapshot.items.length" :description="t('topbar.history_empty')" />
      <n-collapse
        v-else
        :expanded-names="historyTypeExpanded"
        @update:expanded-names="handleHistoryTypeExpandedChange"
      >
        <n-collapse-item name="produce_clip" :title="t('topbar.history_group_produce')">
          <template #header-extra>
            <n-tag size="small" :bordered="false">{{ t("topbar.history_clip_count", { count: produceHistoryItems.length }) }}</n-tag>
          </template>

          <n-empty
            v-if="!historyGroups.length"
            :description="t('topbar.history_group_produce_empty')"
            size="small"
          />

          <n-collapse
            v-else
            :expanded-names="getHistoryDemoExpandedNames()"
            @update:expanded-names="handleHistoryDemoExpandedChange"
          >
            <n-collapse-item
              v-for="group in historyGroups"
              :key="group.demo_path"
              :name="group.demo_path"
              :title="basename(group.demo_path)"
            >
              <template #header-extra>
                <n-tag size="small" :bordered="false">{{ t("topbar.history_clip_count", { count: group.items.length }) }}</n-tag>
              </template>

              <n-collapse
                :expanded-names="getHistoryRoundExpandedNames(group.demo_path)"
                @update:expanded-names="onHistoryRoundExpandedChange(group.demo_path, $event)"
              >
                <n-collapse-item
                  v-for="roundGroup in historyRoundGroupsForDemo(group)"
                  :key="`${group.demo_path}-round-${roundGroup.name}`"
                  :name="roundGroup.name"
                  :title="historyRoundTitle(roundGroup)"
                >
                  <n-space vertical :size="8">
                    <div v-for="row in roundGroup.rows" :key="row.key" class="history-row">
                      <div class="history-main">
                        <div class="history-head">
                          <n-tag size="small" :bordered="false" :type="viewTagType(row.item.view)">{{ viewLabel(row.item.view) }}</n-tag>
                          <span class="history-time">{{ formatHistoryTime(row.item.completed_at_ms) }}</span>
                          <span
                            v-if="String(row.item.view).toLowerCase() === 'full_round_pov'"
                            class="history-meta"
                          >
                            {{ povRowStatusLabel(row.item) }}
                          </span>
                        </div>
                        <div v-if="row.kills.length" class="history-kills">
                          <div v-for="kill in row.kills" :key="kill.id" class="history-kill-row">
                            <DeathNoticeLine :kill="kill" compact />
                          </div>
                        </div>
                        <div v-else-if="String(row.item.view).toLowerCase() === 'full_round_pov'" class="history-meta">
                          -
                        </div>
                        <div v-else class="history-meta">
                          {{ t("topbar.history_kill_count", { count: row.item.kill_ids.length }) }}
                        </div>
                      </div>
                      <n-button size="tiny" quaternary :disabled="!row.item.video_path" @click="openProducedClip(row.item.video_path)">
                        {{ t("main.produce.open_clip_folder") }}
                      </n-button>
                    </div>
                  </n-space>
                </n-collapse-item>
              </n-collapse>
            </n-collapse-item>
          </n-collapse>
        </n-collapse-item>

        <n-collapse-item name="edited_video" :title="t('topbar.history_group_edited')">
          <template #header-extra>
            <n-tag size="small" :bordered="false">{{ t("topbar.history_clip_count", { count: editedHistoryItems.length }) }}</n-tag>
          </template>

          <n-empty
            v-if="!editedHistoryItems.length"
            :description="t('topbar.history_group_edited_empty')"
            size="small"
          />

          <n-space v-else vertical :size="8">
            <div v-for="item in editedHistoryItems" :key="historyRowKey(item)" class="history-row">
              <div class="history-main">
                <div class="history-head">
                  <n-tag size="small" :bordered="false" type="info">{{ t("topbar.history_edited_tag") }}</n-tag>
                  <span class="history-time">{{ formatHistoryTime(item.completed_at_ms) }}</span>
                </div>
                <div class="history-meta history-meta--strong">{{ basename(item.video_path) }}</div>
              </div>
              <n-button size="tiny" quaternary :disabled="!item.video_path" @click="openProducedClip(item.video_path)">
                {{ t("main.produce.open_clip_folder") }}
              </n-button>
            </div>
          </n-space>
        </n-collapse-item>
      </n-collapse>
    </n-spin>
  </n-drawer-content>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { t } from "@/shared/i18n";
import { backend } from "@/shared/backend";
import { initProduceHistory } from "@/domains/production";
import DeathNoticeLine from "@/shared/ui/DeathNoticeLine.vue";
import type { DemoClipKill, ProduceHistoryExportResult, ProduceHistoryItem, ProduceHistorySnapshot } from "@/shared/types";

interface HistoryDemoGroup {
  demo_path: string;
  items: ProduceHistoryItem[];
}

interface HistoryRoundRow {
  key: string;
  item: ProduceHistoryItem;
  kills: DemoClipKill[];
}

interface HistoryRoundGroup {
  name: string;
  round: number;
  kill_count: number;
  rows: HistoryRoundRow[];
}

const props = defineProps<{
  historySnapshot: ProduceHistorySnapshot;
}>();

const emit = defineEmits<{
  (e: "export"): void;
}>();

const message = useMessage();
const historyLoading = ref(false);
const historyExporting = ref(false);
const historyTypeExpanded = ref<string[]>(["produce_clip", "edited_video"]);
const historyDemoExpanded = ref<string[]>([]);
const historyDemoExpandedInitialized = ref(false);
const historyRoundExpandedByDemo = ref<Record<string, string[]>>({});

const produceHistoryItems = computed(() =>
  [...(props.historySnapshot.items || [])]
    .filter((item) => normalizeHistoryType(item) === "produce_clip")
    .sort((a, b) => (b.completed_at_ms || 0) - (a.completed_at_ms || 0)),
);

const editedHistoryItems = computed(() =>
  [...(props.historySnapshot.items || [])]
    .filter((item) => normalizeHistoryType(item) === "edited_video")
    .sort((a, b) => (b.completed_at_ms || 0) - (a.completed_at_ms || 0)),
);

const historyGroups = computed<HistoryDemoGroup[]>(() => {
  const order: string[] = [];
  const byDemo = new Map<string, ProduceHistoryItem[]>();
  for (const item of produceHistoryItems.value) {
    const demoPath = item.demo_path || "";
    if (!demoPath) continue;
    if (!byDemo.has(demoPath)) {
      byDemo.set(demoPath, []);
      order.push(demoPath);
    }
    byDemo.get(demoPath)!.push(item);
  }
  return order.map((demoPath) => ({
    demo_path: demoPath,
    items: byDemo.get(demoPath) || [],
  }));
});

watch(
  () => historyGroups.value.map((group) => group.demo_path),
  (demoKeys) => {
    if (!historyDemoExpandedInitialized.value) {
      historyDemoExpanded.value = [...demoKeys];
      historyDemoExpandedInitialized.value = true;
      return;
    }
    const allowed = new Set(demoKeys);
    historyDemoExpanded.value = historyDemoExpanded.value.filter((name) => allowed.has(name));
  },
  { immediate: true },
);

const historyRoundGroupsByDemo = computed(() => {
  const next = new Map<string, HistoryRoundGroup[]>();
  for (const group of historyGroups.value) {
    const groupedByRound = new Map<string, HistoryRoundGroup>();
    for (const item of group.items) {
      if (String(item.view).toLowerCase() === "full_round_pov") {
        const name = "pov-group";
        if (!groupedByRound.has(name)) {
          groupedByRound.set(name, {
            name,
            round: 0,
            kill_count: 0,
            rows: [],
          });
        }
        const target = groupedByRound.get(name)!;
        const kills = (item.kills || []).filter((kill): kill is DemoClipKill => !!kill?.id);
        target.rows.push({
          key: `${historyRowKey(item)}#${name}`,
          item,
          kills,
        });
        target.kill_count += kills.length;
        continue;
      }
      const split = splitKillsByRound((item.kills || []).filter((kill): kill is DemoClipKill => !!kill?.id));
      if (!split.length) {
        split.push({ round: 0, kills: [] });
      }
      for (const part of split) {
        const name = part.round > 0 ? `round-${part.round}` : "round-unknown";
        if (!groupedByRound.has(name)) {
          groupedByRound.set(name, {
            name,
            round: part.round,
            kill_count: 0,
            rows: [],
          });
        }
        const target = groupedByRound.get(name)!;
        target.rows.push({
          key: `${historyRowKey(item)}#${name}`,
          item,
          kills: part.kills,
        });
        target.kill_count += part.kills.length;
      }
    }

    const sortedGroups = Array.from(groupedByRound.values())
      .map((roundGroup) => ({
        ...roundGroup,
        rows:
          roundGroup.name === "pov-group"
            ? roundGroup.rows
                .slice()
                .sort((a, b) => Number(a.item.round || 0) - Number(b.item.round || 0))
            : roundGroup.rows
                .slice()
                .sort((a, b) => (b.item.completed_at_ms || 0) - (a.item.completed_at_ms || 0)),
      }))
      .sort((a, b) => {
        if (a.name === "pov-group" && b.name === "pov-group") return 0;
        if (a.name === "pov-group") return -1;
        if (b.name === "pov-group") return 1;
        if (a.round <= 0 && b.round <= 0) return 0;
        if (a.round <= 0) return 1;
        if (b.round <= 0) return -1;
        return a.round - b.round;
      });
    next.set(group.demo_path, sortedGroups);
  }
  return next;
});

async function ensureInit() {
  historyLoading.value = true;
  try {
    await initProduceHistory();
  } catch (error: unknown) {
    const detail = error instanceof Error ? error.message : String(error);
    message.error(detail || t("main.produce.failed"));
  } finally {
    historyLoading.value = false;
  }
}

defineExpose({ ensureInit });

function basename(path: string): string {
  if (!path) return "";
  const normalized = path.replaceAll("\\", "/");
  const parts = normalized.split("/");
  return parts[parts.length - 1] || normalized;
}

function historyRowKey(item: ProduceHistoryItem): string {
  return `${item.history_type || "produce_clip"}#${item.demo_path}#${item.view}#${item.spec_mode}#${item.completed_at_ms}#${item.video_path}#${(item.kill_ids || []).join("|")}`;
}

function normalizeHistoryType(item: ProduceHistoryItem): "produce_clip" | "edited_video" {
  return item.history_type === "edited_video" ? "edited_video" : "produce_clip";
}

function viewLabel(view: string): string {
  const normalized = String(view).toLowerCase();
  if (normalized === "victim") return t("main.clips.victim_view");
  if (normalized === "full_round_pov") return t("main.clips.full_round_pov_tag");
  return t("main.clips.killer_view");
}

function viewTagType(view: string): "success" | "warning" | "info" {
  const normalized = String(view).toLowerCase();
  if (normalized === "victim") return "warning";
  if (normalized === "full_round_pov") return "info";
  return "success";
}

function povRowStatusLabel(item: ProduceHistoryItem): string {
  const round = Number(item.round || 0);
  const kills = item.kills?.length || 0;
  const died = String(item.end_reason || "").toLowerCase() === "target_death";
  const key = died ? "main.clips.full_round_pov_round_title_died" : "main.clips.full_round_pov_round_title_survived";
  return t(key, { round, kills });
}

function formatHistoryTime(tsMs: number): string {
  if (!tsMs) return "-";
  const d = new Date(tsMs);
  if (Number.isNaN(d.getTime())) return "-";
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}:${String(d.getSeconds()).padStart(2, "0")}`;
}

function historyRoundGroupsForDemo(group: HistoryDemoGroup): HistoryRoundGroup[] {
  return historyRoundGroupsByDemo.value.get(group.demo_path) || [];
}

function getHistoryDemoExpandedNames(): string[] {
  const allowed = new Set(historyGroups.value.map((group) => group.demo_path));
  return historyDemoExpanded.value.filter((name) => allowed.has(name));
}

function handleHistoryTypeExpandedChange(names: Array<string | number> | string | number | null) {
  historyTypeExpanded.value = normalizeExpandedNames(names);
}

function handleHistoryDemoExpandedChange(names: Array<string | number> | string | number | null) {
  historyDemoExpanded.value = normalizeExpandedNames(names);
}

function getHistoryRoundExpandedNames(demoPath: string): string[] {
  const groups = historyRoundGroupsByDemo.value.get(demoPath) || [];
  const defaults = groups.map((group) => group.name);
  if (!Object.prototype.hasOwnProperty.call(historyRoundExpandedByDemo.value, demoPath)) {
    return defaults;
  }
  const current = historyRoundExpandedByDemo.value[demoPath] || [];
  const allowed = new Set(defaults);
  return current.filter((name) => allowed.has(name));
}

function onHistoryRoundExpandedChange(
  demoPath: string,
  names: Array<string | number> | string | number | null,
) {
  historyRoundExpandedByDemo.value = {
    ...historyRoundExpandedByDemo.value,
    [demoPath]: normalizeExpandedNames(names),
  };
}

function historyRoundTitle(group: HistoryRoundGroup): string {
  if (group.name === "pov-group") {
    return t("topbar.history_full_round_pov_group_title", { count: group.rows.length });
  }
  if (group.round > 0) {
    return t("main.clips.round_title", { round: group.round, kills: group.kill_count });
  }
  return t("main.produce.round_unknown_title", { kills: group.kill_count });
}

function splitKillsByRound(kills: DemoClipKill[]): Array<{ round: number; kills: DemoClipKill[] }> {
  const grouped = new Map<number, DemoClipKill[]>();
  for (const kill of kills) {
    const round = Number(kill.round || 0);
    const normalizedRound = round > 0 ? round : 0;
    if (!grouped.has(normalizedRound)) {
      grouped.set(normalizedRound, []);
    }
    grouped.get(normalizedRound)!.push(kill);
  }
  return Array.from(grouped.entries())
    .sort((a, b) => {
      if (a[0] <= 0 && b[0] <= 0) return 0;
      if (a[0] <= 0) return 1;
      if (b[0] <= 0) return -1;
      return a[0] - b[0];
    })
    .map(([round, roundKills]) => ({
      round,
      kills: roundKills.slice().sort((a, b) => {
        const tickA = Number(a.tick || 0);
        const tickB = Number(b.tick || 0);
        if (tickA === tickB) {
          return String(a.id || "").localeCompare(String(b.id || ""));
        }
        return tickA - tickB;
      }),
    }));
}

function normalizeExpandedNames(names: Array<string | number> | string | number | null): string[] {
  if (Array.isArray(names)) {
    return names.map((name) => String(name));
  }
  if (names == null) {
    return [];
  }
  return [String(names)];
}

async function openProducedClip(videoPath: string) {
  if (!videoPath) return;
  try {
    await backend.OpenProducedClipInFolder(videoPath);
  } catch {
    // ignore open failures
  }
}

async function exportHistoryVideos() {
  if (historyExporting.value) return;
  historyExporting.value = true;
  try {
    const result = await backend.ExportProduceHistoryVideos();
    if (result.cancelled) {
      message.info(t("topbar.history_export_cancelled"));
      return;
    }
    const target = result.target_dir || "-";
    if (result.failed > 0) {
      message.warning(t("topbar.history_export_partial", { moved: result.moved, failed: result.failed, target }));
      if (result.errors?.length) {
        message.warning(result.errors[0]);
      }
      return;
    }
    message.success(t("topbar.history_export_success", { moved: result.moved, target }));
    emit("export");
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err);
    message.error(t("topbar.history_export_failed", { error: msg }));
  } finally {
    historyExporting.value = false;
  }
}
</script>

<style scoped>
.history-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 10px;
}

.history-row {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  border: 1px solid #2f3631;
  border-radius: 8px;
  padding: 8px;
}

.history-main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.history-head {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.history-time {
  font-size: 12px;
  color: #8d9890;
}

.history-kills {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.history-kill-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.history-meta {
  font-size: 12px;
  color: #8d9890;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.history-meta--strong {
  color: #cbd3cd;
}
</style>
