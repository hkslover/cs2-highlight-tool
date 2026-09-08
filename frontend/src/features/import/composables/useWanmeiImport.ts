import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useMessage } from "naive-ui";
import { t } from "@/shared/i18n";
import { EventsOn } from "../../../../wailsjs/runtime/runtime";
import type { ProgressMessage, WanmeiMatchItem, WanmeiMatchListResult } from "@/shared/types";
import { backend } from "@/shared/backend";

const wanmeiProgressPrefix = "wanmei_import_";

export function useWanmeiImport(onDemosSelected?: (paths: string[]) => void) {
  const message = useMessage();
  const loading = ref(false);
  const manualMatchInput = ref("");
  const pendingTasks = ref<Array<{ rawMatchID: string; normalizedKey: string }>>([]);
  const taskStateByKey = ref<Record<string, "queued" | "running">>({});
  const downloadPercentByKey = ref<Record<string, number>>({});
  const downloadIndeterminateByKey = ref<Record<string, boolean>>({});
  const result = ref<WanmeiMatchListResult | null>(null);
  const currentPage = ref(1);
  const hasMorePages = ref(true);
  const loadingMore = ref(false);
  const MAX_CONCURRENT_IMPORTS = 2;
  const offEventHandlers: Array<() => void> = [];
  const lastStatusNotice = ref<WanmeiMatchListResult["status"] | "">("");

  const rows = computed<WanmeiMatchItem[]>(() => result.value?.matches ?? []);
  const runningImportCount = computed(() =>
    Object.values(taskStateByKey.value).filter((state) => state === "running").length,
  );
  const queuedImportCount = computed(() =>
    Object.values(taskStateByKey.value).filter((state) => state === "queued").length,
  );
  const canSubmitManualMatches = computed(() => parseManualMatchIDs(manualMatchInput.value).length > 0);

  function showError(content: string) {
    message.error(content);
  }

  async function refreshMatches() {
    currentPage.value = 1;
    hasMorePages.value = true;
    loading.value = true;
    try {
      const next = await backend.ListWanmeiRecentMatches(1);
      const nextRows = dedupeWanmeiMatches(next?.matches ?? []);
      result.value = {
        status: next?.status ?? "client_not_running",
        nickname: next?.nickname,
        steam_id: next?.steam_id,
        matches: nextRows,
      };
      notifyWanmeiStatus(result.value.status);
      hasMorePages.value = result.value.status === "ready" && nextRows.length > 0;
    } catch (err: unknown) {
      const errorMessage = err instanceof Error ? err.message : String(err);
      showError(t("main.import.wanmei_load_failed", { error: errorMessage }));
      result.value = null;
      hasMorePages.value = false;
    } finally {
      loading.value = false;
    }
  }

  function notifyWanmeiStatus(status: WanmeiMatchListResult["status"] | undefined) {
    if (status === "client_not_running") {
      if (lastStatusNotice.value !== status) {
        message.warning(t("main.import.wanmei_status_not_running"));
        lastStatusNotice.value = status;
      }
      return;
    }
    if (status === "client_not_logged_in") {
      if (lastStatusNotice.value !== status) {
        message.warning(t("main.import.wanmei_status_not_logged_in"));
        lastStatusNotice.value = status;
      }
      return;
    }
    lastStatusNotice.value = "";
  }

  async function loadMoreMatches() {
    if (loading.value || loadingMore.value || !hasMorePages.value) return;
    if (result.value?.status !== "ready") {
      hasMorePages.value = false;
      return;
    }
    const nextPage = currentPage.value + 1;
    loadingMore.value = true;
    try {
      const next = await backend.ListWanmeiRecentMatches(nextPage);
      const incoming = dedupeWanmeiMatches(next?.matches ?? []);
      if (!incoming.length) {
        hasMorePages.value = false;
        return;
      }
      const merged = mergeWanmeiMatches(result.value?.matches ?? [], incoming);
      if (merged.length === (result.value?.matches ?? []).length) {
        hasMorePages.value = false;
        return;
      }
      result.value = {
        status: next?.status ?? result.value?.status ?? "ready",
        nickname: next?.nickname ?? result.value?.nickname,
        steam_id: next?.steam_id ?? result.value?.steam_id,
        matches: merged,
      };
      currentPage.value = nextPage;
    } catch (err: unknown) {
      const errorMessage = err instanceof Error ? err.message : String(err);
      showError(t("main.import.wanmei_load_failed", { error: errorMessage }));
    } finally {
      loadingMore.value = false;
    }
  }

  function normalizeMatchIDKey(matchID: string): string {
    const raw = String(matchID || "").trim();
    if (!raw) return "";
    const prefixed = /^pvp@(\d{8,})$/i.exec(raw);
    if (prefixed) return prefixed[1];
    const direct = /^(\d{8,})$/.exec(raw);
    if (direct) return direct[1];
    const fromURL = /\/(\d{8,})_0\.dem(?:$|\?)/i.exec(raw);
    if (fromURL) return fromURL[1];
    const all = raw.match(/\d{8,}/g);
    if (all && all.length > 0) return all[0];
    return raw.toLowerCase();
  }

  function wanmeiMatchKey(item: WanmeiMatchItem): string {
    return normalizeMatchIDKey(item.download_match_id || item.match_id);
  }

  function dedupeWanmeiMatches(matches: WanmeiMatchItem[]): WanmeiMatchItem[] {
    const seen = new Set<string>();
    const deduped: WanmeiMatchItem[] = [];
    for (const item of matches) {
      const key = wanmeiMatchKey(item);
      if (!key || seen.has(key)) continue;
      seen.add(key);
      deduped.push(item);
    }
    return deduped;
  }

  function mergeWanmeiMatches(current: WanmeiMatchItem[], incoming: WanmeiMatchItem[]): WanmeiMatchItem[] {
    const merged = current.slice();
    const seen = new Set<string>(current.map((item) => wanmeiMatchKey(item)).filter((key) => !!key));
    for (const item of incoming) {
      const key = wanmeiMatchKey(item);
      if (!key || seen.has(key)) continue;
      seen.add(key);
      merged.push(item);
    }
    return merged;
  }

  function formatWanmeiRemark(row: WanmeiMatchItem): string {
    const tags: string[] = [];
    if ((row.k4 || 0) > 0) tags.push("4K");
    if ((row.k5 || 0) > 0) tags.push("5K");
    return tags.join(" ");
  }

  function formatWanmeiRating(rating: number): string {
    if (!Number.isFinite(rating) || rating <= 0) return "";
    return rating.toFixed(2);
  }

  function parseManualMatchIDs(input: string): string[] {
    const tokens = String(input || "")
      .split(/\r?\n/)
      .map((part) => part.trim())
      .filter((part) => part.length > 0);
    const seen = new Set<string>();
    const result: string[] = [];
    for (const token of tokens) {
      const key = normalizeMatchIDKey(token);
      if (!key || seen.has(key)) continue;
      seen.add(key);
      result.push(token);
    }
    return result;
  }

  function getRunningImportCount(): number {
    let count = 0;
    for (const state of Object.values(taskStateByKey.value)) {
      if (state === "running") count++;
    }
    return count;
  }

  function isTaskPending(matchID: string): boolean {
    const key = normalizeMatchIDKey(matchID);
    return !!(key && taskStateByKey.value[key]);
  }

  function resolveTaskState(matchID: string): "queued" | "running" | "" {
    const key = normalizeMatchIDKey(matchID);
    if (!key) return "";
    return taskStateByKey.value[key] || "";
  }

  function setTaskState(normalizedKey: string, state: "queued" | "running" | null) {
    if (!normalizedKey) return;
    const next = { ...taskStateByKey.value };
    if (state) {
      next[normalizedKey] = state;
    } else {
      delete next[normalizedKey];
      clearDownloadProgress(normalizedKey);
    }
    taskStateByKey.value = next;
  }

  function resolveDownloadPercent(matchID: string): number | null {
    const key = normalizeMatchIDKey(matchID);
    if (!key) return null;
    if (downloadIndeterminateByKey.value[key]) return null;
    const value = downloadPercentByKey.value[key];
    if (!Number.isFinite(value)) return null;
    return Math.max(0, Math.min(100, Math.round(value)));
  }

  function clearDownloadProgress(normalizedKey: string) {
    if (!normalizedKey) return;
    const nextPercent = { ...downloadPercentByKey.value };
    const nextIndeterminate = { ...downloadIndeterminateByKey.value };
    delete nextPercent[normalizedKey];
    delete nextIndeterminate[normalizedKey];
    downloadPercentByKey.value = nextPercent;
    downloadIndeterminateByKey.value = nextIndeterminate;
  }

  function handleDownloadProgress(next: ProgressMessage) {
    const componentID = String(next.component_id || "").trim();
    if (!componentID.startsWith(wanmeiProgressPrefix)) return;
    const matchID = componentID.slice(wanmeiProgressPrefix.length);
    const normalizedKey = normalizeMatchIDKey(matchID);
    if (!normalizedKey) return;

    if (!next.active) {
      clearDownloadProgress(normalizedKey);
      return;
    }
    const indeterminate = !!next.indeterminate;
    downloadIndeterminateByKey.value = {
      ...downloadIndeterminateByKey.value,
      [normalizedKey]: indeterminate,
    };
    if (!indeterminate && Number.isFinite(next.percent)) {
      downloadPercentByKey.value = {
        ...downloadPercentByKey.value,
        [normalizedKey]: Math.max(0, Math.min(100, next.percent)),
      };
    }
  }

  function enqueueMatchImport(matchID: string): boolean {
    const raw = String(matchID || "").trim();
    if (!raw) return false;
    const normalizedKey = normalizeMatchIDKey(raw);
    if (!normalizedKey || taskStateByKey.value[normalizedKey]) return false;
    pendingTasks.value = pendingTasks.value.concat({ rawMatchID: raw, normalizedKey });
    setTaskState(normalizedKey, "queued");
    processImportQueue();
    return true;
  }

  function submitManualMatches() {
    const ids = parseManualMatchIDs(manualMatchInput.value);
    if (!ids.length) {
      showError(t("main.import.wanmei_manual_empty"));
      return;
    }
    let added = 0;
    for (const id of ids) {
      if (enqueueMatchImport(id)) added++;
    }
    if (added <= 0) {
      showError(t("main.import.wanmei_manual_no_new"));
      return;
    }
    manualMatchInput.value = "";
  }

  function processImportQueue() {
    while (getRunningImportCount() < MAX_CONCURRENT_IMPORTS && pendingTasks.value.length > 0) {
      const [task, ...rest] = pendingTasks.value;
      pendingTasks.value = rest;
      setTaskState(task.normalizedKey, "running");
      void runImportTask(task);
    }
  }

  async function runImportTask(task: { rawMatchID: string; normalizedKey: string }) {
    try {
      const paths = await backend.ImportWanmeiMatch(task.rawMatchID);
      if (paths && paths.length > 0 && onDemosSelected) {
        onDemosSelected(paths);
      }
    } catch (err: unknown) {
      const errorMessage = err instanceof Error ? err.message : String(err);
      showError(t("main.import.wanmei_import_failed", { error: errorMessage }));
    } finally {
      setTaskState(task.normalizedKey, null);
      processImportQueue();
    }
  }

  onMounted(() => {
    offEventHandlers.push(
      EventsOn("download_progress", (next: ProgressMessage) => {
        handleDownloadProgress(next);
      }),
    );
    void refreshMatches();
  });

  onBeforeUnmount(() => {
    for (const off of offEventHandlers) {
      off();
    }
    offEventHandlers.length = 0;
  });

  return {
    loading,
    manualMatchInput,
    result,
    currentPage,
    hasMorePages,
    loadingMore,
    rows,
    runningImportCount,
    queuedImportCount,
    canSubmitManualMatches,
    refreshMatches,
    loadMoreMatches,
    formatWanmeiRemark,
    formatWanmeiRating,
    parseManualMatchIDs,
    resolveTaskState,
    resolveDownloadPercent,
    isTaskPending,
    enqueueMatchImport,
    submitManualMatches,
    normalizeMatchIDKey,
  };
}
