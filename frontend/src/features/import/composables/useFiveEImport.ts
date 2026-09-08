import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useMessage } from "naive-ui";
import { t } from "@/shared/i18n";
import { EventsOn } from "../../../../wailsjs/runtime/runtime";
import type { FiveEMatchItem, FiveEMatchListResult, ProgressMessage } from "@/shared/types";
import { backend } from "@/shared/backend";

const fiveEProgressPrefix = "fivee_import_";

export function useFiveEImport(onDemosSelected?: (paths: string[]) => void) {
  const message = useMessage();
  const loading = ref(false);
  const playerNameInput = ref("");
  const manualMatchInput = ref("");
  const pendingTasks = ref<Array<{ rawMatchID: string; normalizedKey: string }>>([]);
  const taskStateByKey = ref<Record<string, "queued" | "running">>({});
  const downloadPercentByKey = ref<Record<string, number>>({});
  const downloadIndeterminateByKey = ref<Record<string, boolean>>({});
  const result = ref<FiveEMatchListResult | null>(null);
  const currentPage = ref(1);
  const hasMorePages = ref(true);
  const loadingMore = ref(false);
  const MAX_CONCURRENT_IMPORTS = 2;
  const offEventHandlers: Array<() => void> = [];

  const rows = computed<FiveEMatchItem[]>(() => result.value?.matches ?? []);
  const runningImportCount = computed(() =>
    Object.values(taskStateByKey.value).filter((state) => state === "running").length,
  );
  const queuedImportCount = computed(() =>
    Object.values(taskStateByKey.value).filter((state) => state === "queued").length,
  );
  const canSubmitManualMatches = computed(() => parseManualMatchIDs(manualMatchInput.value).length > 0);
  const canQuery = computed(() => playerNameInput.value.trim().length > 0);

  function showError(content: string) {
    message.error(content);
  }

  async function refreshMatches() {
    const playerName = playerNameInput.value.trim();
    if (!playerName) {
      showError(t("main.import.fivee_player_empty"));
      result.value = {
        player_name: "",
        matches: [],
      };
      hasMorePages.value = false;
      return;
    }

    currentPage.value = 1;
    hasMorePages.value = true;
    loading.value = true;
    try {
      const next = await backend.ListFiveERecentMatches(playerName, 1);
      const nextRows = dedupeFiveEMatches(next?.matches ?? []);
      result.value = {
        player_name: next?.player_name || playerName,
        matches: nextRows,
      };
      hasMorePages.value = nextRows.length > 0;
      if (next?.player_name) {
        playerNameInput.value = next.player_name;
      }
    } catch (err: unknown) {
      const errorMessage = err instanceof Error ? err.message : String(err);
      showError(t("main.import.fivee_load_failed", { error: errorMessage }));
      result.value = null;
      hasMorePages.value = false;
    } finally {
      loading.value = false;
    }
  }

  function normalizeMatchIDKey(matchID: string): string {
    const raw = String(matchID || "").trim();
    if (!raw) return "";
    const matched = /g\d+(?:-[a-z0-9]+)+/i.exec(raw);
    if (matched) return matched[0].toLowerCase();
    return raw.toLowerCase();
  }

  function fiveEMatchKey(item: FiveEMatchItem): string {
    return normalizeMatchIDKey(item.download_match_id || item.match_id);
  }

  function dedupeFiveEMatches(matches: FiveEMatchItem[]): FiveEMatchItem[] {
    const seen = new Set<string>();
    const deduped: FiveEMatchItem[] = [];
    for (const item of matches) {
      const key = fiveEMatchKey(item);
      if (!key || seen.has(key)) continue;
      seen.add(key);
      deduped.push(item);
    }
    return deduped;
  }

  function mergeFiveEMatches(current: FiveEMatchItem[], incoming: FiveEMatchItem[]): FiveEMatchItem[] {
    const merged = current.slice();
    const seen = new Set<string>(current.map((item) => fiveEMatchKey(item)).filter((key) => !!key));
    for (const item of incoming) {
      const key = fiveEMatchKey(item);
      if (!key || seen.has(key)) continue;
      seen.add(key);
      merged.push(item);
    }
    return merged;
  }

  async function loadMoreMatches() {
    if (loading.value || loadingMore.value || !hasMorePages.value) return;
    const playerName = playerNameInput.value.trim();
    if (!playerName) {
      hasMorePages.value = false;
      return;
    }

    const nextPage = currentPage.value + 1;
    loadingMore.value = true;
    try {
      const next = await backend.ListFiveERecentMatches(playerName, nextPage);
      const incoming = dedupeFiveEMatches(next?.matches ?? []);
      if (!incoming.length) {
        hasMorePages.value = false;
        return;
      }
      const merged = mergeFiveEMatches(result.value?.matches ?? [], incoming);
      if (merged.length === (result.value?.matches ?? []).length) {
        hasMorePages.value = false;
        return;
      }
      result.value = {
        player_name: next?.player_name || result.value?.player_name || playerName,
        matches: merged,
      };
      currentPage.value = nextPage;
    } catch (err: unknown) {
      const errorMessage = err instanceof Error ? err.message : String(err);
      showError(t("main.import.fivee_load_failed", { error: errorMessage }));
    } finally {
      loadingMore.value = false;
    }
  }

  function formatFiveERating(rating: number): string {
    if (!Number.isFinite(rating) || rating <= 0) return "";
    return rating.toFixed(2);
  }

  function parseManualMatchIDs(input: string): string[] {
    const tokens = String(input || "")
      .split(/\r?\n/)
      .map((part) => part.trim())
      .filter((part) => part.length > 0);
    const seen = new Set<string>();
    const parsed: string[] = [];
    for (const token of tokens) {
      const key = normalizeMatchIDKey(token);
      if (!key || seen.has(key)) continue;
      seen.add(key);
      parsed.push(token);
    }
    return parsed;
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
    if (!componentID.startsWith(fiveEProgressPrefix)) return;
    const matchID = componentID.slice(fiveEProgressPrefix.length);
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
      showError(t("main.import.fivee_manual_empty"));
      return;
    }
    let added = 0;
    for (const id of ids) {
      if (enqueueMatchImport(id)) added++;
    }
    if (added <= 0) {
      showError(t("main.import.fivee_manual_no_new"));
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
      const paths = await backend.ImportFiveEMatch(task.rawMatchID);
      if (paths && paths.length > 0 && onDemosSelected) {
        onDemosSelected(paths);
      }
    } catch (err: unknown) {
      const errorMessage = err instanceof Error ? err.message : String(err);
      showError(t("main.import.fivee_import_failed", { error: errorMessage }));
    } finally {
      setTaskState(task.normalizedKey, null);
      processImportQueue();
    }
  }

  async function loadInitialPlayerName() {
    try {
      const next = await backend.GetFiveEPlayerName();
      playerNameInput.value = String(next || "").trim();
      if (playerNameInput.value) {
        await refreshMatches();
      }
    } catch {
      // ignore initial load error
    }
  }

  onMounted(() => {
    offEventHandlers.push(
      EventsOn("download_progress", (next: ProgressMessage) => {
        handleDownloadProgress(next);
      }),
    );
    void loadInitialPlayerName();
  });

  onBeforeUnmount(() => {
    for (const off of offEventHandlers) {
      off();
    }
    offEventHandlers.length = 0;
  });

  return {
    loading,
    playerNameInput,
    manualMatchInput,
    result,
    currentPage,
    hasMorePages,
    loadingMore,
    rows,
    runningImportCount,
    queuedImportCount,
    canSubmitManualMatches,
    canQuery,
    refreshMatches,
    loadMoreMatches,
    formatFiveERating,
    parseManualMatchIDs,
    resolveTaskState,
    resolveDownloadPercent,
    isTaskPending,
    enqueueMatchImport,
    submitManualMatches,
    normalizeMatchIDKey,
  };
}
