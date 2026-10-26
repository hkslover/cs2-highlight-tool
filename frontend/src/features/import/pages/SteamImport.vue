<template>
  <div class="sub-page">
    <n-card
      :bordered="true"
      class="sub-card"
      content-style="height: 100%; display: flex; flex-direction: column; padding: 0;"
      content-class="steam-sub-card-content"
    >
      <div class="panel-head">
        <div class="panel-head-row">
          <n-space align="center" :size="8" class="panel-head-left">
            <n-button
              quaternary
              circle
              size="small"
              @click="$router.push({ name: 'import-actions' })"
            >
              <template #icon>
                <n-icon size="16"><BackIcon /></n-icon>
              </template>
            </n-button>
            <span class="panel-title">{{ t("main.import.sub_steam_title") }}</span>
          </n-space>
          <div class="header-actions">
            <n-text depth="3" class="queue-text">
              {{ t("main.import.steam_import_queue", { running: runningImportCount, queued: queuedImportCount }) }}
            </n-text>
            <n-text v-if="accountText" depth="3" class="queue-text">{{ accountText }}</n-text>
            <n-button
              size="small"
              :loading="loading"
              @click="refreshMatches"
            >
              {{ t("main.import.steam_refresh") }}
            </n-button>
          </div>
        </div>
      </div>
      <div class="steam-content">
        <n-alert
          v-if="statusHint"
          type="warning"
          :bordered="false"
          class="status-alert"
        >
          {{ statusHint }}
        </n-alert>
        <n-text
          v-if="listEmpty"
          depth="3"
          class="list-hint"
        >
          {{ t("main.import.steam_list_empty") }}
        </n-text>

        <div class="manual-import-box">
          <n-space vertical :size="6">
            <n-text depth="3">{{ t("main.import.steam_manual_hint") }}</n-text>
            <div class="manual-input-row">
              <n-input
                v-model:value="manualInput"
                size="small"
                class="manual-input"
                type="textarea"
                :autosize="{ minRows: 1, maxRows: 4 }"
                :placeholder="t('main.import.steam_manual_placeholder')"
              />
              <n-button
                size="small"
                class="manual-submit-btn"
                type="primary"
                :disabled="!canSubmitManualInput"
                @click="submitManualInput"
              >
                {{ t("main.import.steam_manual_import_action") }}
              </n-button>
            </div>
          </n-space>
        </div>

        <ImportMatchList
          :columns="columns"
          :rows="rows"
          :loading="loading"
          :loading-more="false"
          :can-load-more="false"
          @load-more="noopLoadMore"
        />
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { computed, h } from "vue";
import { NButton, NSpin, NTag } from "naive-ui";
import type { DataTableColumn } from "naive-ui";
import { t } from "@/shared/i18n";
import ImportMatchList from "@/features/import/components/ImportMatchList.vue";
import { useSteamImport } from "@/features/import/composables/useSteamImport";

const emit = defineEmits<{
  (e: "demos-selected", paths: string[]): void;
}>();

const {
  loading,
  manualInput,
  rows,
  statusHint,
  accountText,
  listEmpty,
  runningImportCount,
  queuedImportCount,
  canSubmitManualInput,
  refreshMatches,
  resolveTaskState,
  resolveDownloadPercent,
  isTaskPending,
  enqueueMatchImport,
  submitManualInput,
} = useSteamImport((paths: string[]) => emit("demos-selected", paths));

function noopLoadMore() {
  // The Game Coordinator returns a single batch; there is no pagination.
}

const columns = computed<DataTableColumn[]>(() => [
  {
    title: () => t("main.import.steam_col_mode_map"),
    key: "mode_map",
    width: "24%",
    ellipsis: true,
    render: (row: any) =>
      row.map_name ? `${row.game_mode} · ${row.map_name}` : row.game_mode,
  },
  {
    title: () => t("main.import.wanmei_col_time"),
    key: "end_time",
    width: "18%",
    ellipsis: true,
  },
  {
    title: () => t("main.import.col_score"),
    key: "score",
    width: "9%",
    render: (row: any) => `${row.score1} : ${row.score2}`,
  },
  {
    title: () => t("main.import.wanmei_col_kda"),
    key: "kda",
    width: "11%",
    render: (row: any) => `${row.kill}/${row.death}/${row.assist}`,
  },
  {
    title: () => t("main.import.steam_col_demo"),
    key: "demo",
    width: "11%",
    render: (row: any) =>
      h(
        NTag,
        { size: "small", bordered: false, type: row.demo_available ? "success" : "default" },
        {
          default: () =>
            row.demo_available
              ? t("main.import.steam_demo_ready")
              : t("main.import.steam_demo_expired"),
        },
      ),
  },
  {
    title: () => t("main.import.wanmei_col_action"),
    key: "actions",
    width: "20%",
    fixed: "right",
    align: "center",
    render: (row: any) => {
      const taskState = resolveTaskState(row.match_id);
      if (taskState === "running") {
        const percent = resolveDownloadPercent(row.match_id);
        return h("span", { class: "steam-import-loading" }, [
          h(NSpin, { size: 14, stroke: "#2f9462" }),
          typeof percent === "number"
            ? h("span", { class: "steam-import-percent" }, `${percent}%`)
            : null,
        ]);
      }
      if (taskState === "queued") {
        return h(
          NTag,
          { size: "small", bordered: false },
          { default: () => t("main.import.steam_import_queued") },
        );
      }
      return h(
        NButton,
        {
          size: "tiny",
          type: "primary",
          disabled: !row.demo_available || isTaskPending(row.match_id),
          onClick: () => {
            enqueueMatchImport(row);
          },
        },
        { default: () => t("main.import.steam_import_action") },
      );
    },
  },
]);

const BackIcon = {
  render: () =>
    h(
      "svg",
      { xmlns: "http://www.w3.org/2000/svg", viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", "stroke-width": "2", "stroke-linecap": "round", "stroke-linejoin": "round" },
      [h("polyline", { points: "15 18 9 12 15 6" })],
    ),
};
</script>

<style scoped>
.sub-page {
  height: 100%;
}
.sub-card {
  height: 100%;
  background: #181b19;
  display: flex;
  flex-direction: column;
}

.sub-card :deep(.steam-sub-card-content) {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.panel-head {
  flex-shrink: 0;
  min-height: 34px;
  padding: 6px 10px;
  border-bottom: 1px solid #303732;
}

.panel-head-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  gap: 10px;
}

.panel-head-left {
  min-width: 0;
}

.header-actions {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
  min-width: 0;
}

.queue-text {
  font-size: 12px;
  white-space: nowrap;
}

.panel-title {
  font-size: 13px;
  font-weight: 600;
}

.steam-content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px 10px 10px;
  overflow: hidden;
}

.status-alert {
  flex-shrink: 0;
}

.list-hint {
  font-size: 12px;
}

.manual-import-box {
  border: 1px solid #2f3631;
  border-radius: 8px;
  padding: 8px;
  background: rgba(17, 19, 18, 0.45);
}

.manual-input-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.manual-input {
  flex: 1 1 300px;
  min-width: 220px;
}

.manual-submit-btn {
  flex: 0 0 auto;
  min-width: 96px;
}

.steam-import-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 22px;
}

.steam-import-percent {
  font-size: 12px;
  color: #97a49c;
  line-height: 1;
}
</style>
