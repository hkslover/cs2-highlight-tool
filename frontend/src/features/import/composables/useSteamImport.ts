import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useMessage } from "naive-ui";
import { t } from "@/shared/i18n";
import { EventsOn } from "../../../../wailsjs/runtime/runtime";
import type {
  ProgressMessage,
  SteamImportRequest,
  SteamMatchItem,
  SteamMatchListResult,
} from "@/shared/types";
import { backend } from "@/shared/backend";

const steamProgressPrefix = "steam_import_";
const MAX_CONCURRENT_IMPORTS = 2;

/** One queued import. The key is the raw user input so share codes work too. */
interface SteamImportTask {
  key: string;
  request: SteamImportRequest;
}

function looksLikeShareCode(value: string): boolean {
  return /^csgo/i.test(value) || /[a-z]/i.test(value.replace(/-/g, "").slice(0, 4));
}

/**
 * Headless state for the Steam official match import page: one list snapshot
 * (the Game Coordinator has no pagination), a small concurrent import queue, and
 * the per-match download progress events emitted by the backend.
 */
export function useSteamImport(onDemosSelected?: (paths: string[]) => void) {
  const message = useMessage();
  const loading = ref(false);
  const manualInput = ref("");
  const result = ref<SteamMatchListResult | null>(null);
  const pendingTasks = ref<SteamImportTask[]>([]);
  const taskStateByKey = ref<Record<string, "queued" | "running">>({});
  const downloadPercentByKey = ref<Record<string, number>>({});
  const downloadIndeterminateByKey = ref<Record<string, boolean>>({});
  const offEventHandlers: Array<() => void> = [];

  const rows = computed<SteamMatchItem[]>(() => result.value?.matches ?? []);
  const status = computed(() => result.value?.status ?? "");
  const accountText = computed(() => {
    const steamID = result.value?.steam_id;
    return steamID ? t("main.import.steam_account", { steamId: steamID }) : "";
  });
  const runningImportCount = computed(
    () => Object.values(taskStateByKey.value).filter((state) => state === "running").length,
  );
  const queuedImportCount = computed(
    () => Object.values(taskStateByKey.value).filter((state) => state === "queued").length,
  );
  const canSubmitManualInput = computed(() => parseManualTokens(manualInput.value).length > 0);

  /** Human readable reason for a non-ready status; empty when ready. */
  const statusHint = computed(() => {
    switch (status.value) {
      case "steam_not_running":
        return t("main.import.steam_status_not_running");
      case "steam_not_logged_in":
        return t("main.import.steam_status_not_logged_in");
      case "cs2_running":
        return t("main.import.steam_status_cs2_running");
      case "gc_unavailable":
        return t("main.import.steam_status_gc_unavailable");
      case "steam_api_unavailable":
        return t("main.import.steam_status_api_unavailable");
      case "unsupported_platform":
        return t("main.import.steam_status_unsupported");
      default:
        return "";
    }
  });

  /** True when the list itself worked but the account has nothing to show. */
  const listEmpty = computed(
    () => status.value === "ready" && rows.value.length === 0,
  );

  function showError(content: string) {
    message.error(content);
  }

  async function refreshMatches() {
    loading.value = true;
    try {
      result.value = await backend.ListSteamRecentMatches();
    } catch (err: unknown) {
      const errorMessage = err instanceof Error ? err.message : String(err);
      showError(t("main.import.steam_load_failed", { error: errorMessage }));
      result.value = null;
    } finally {
      loading.value = false;
    }
  }

  /** Splits the manual input into unique share codes or match IDs. */
  function parseManualTokens(input: string): string[] {
    const seen = new Set<string>();
    const tokens: string[] = [];
    for (const part of String(input || "").split(/\r?\n/)) {
      const token = part.trim();
      if (!token) continue;
      const key = taskKeyForToken(token);
      if (seen.has(key)) continue;
      seen.add(key);
      tokens.push(token);
    }
    return tokens;
  }

  function taskKeyForToken(token: string): string {
    return token.trim().toUpperCase();
  }

  function requestForToken(token: string): SteamImportRequest {
    return looksLikeShareCode(token) ? { share_code: token } : { match_id: token };
  }

  function isTaskPending(key: string): boolean {
    return !!(key && taskStateByKey.value[key]);
  }

  function resolveTaskState(key: string): "queued" | "running" | "" {
    return taskStateByKey.value[key] || "";
  }

  function resolveDownloadPercent(key: string): number | null {
    if (!key) return null;
    if (downloadIndeterminateByKey.value[key]) return null;
    const value = downloadPercentByKey.value[key];
    if (!Number.isFinite(value)) return null;
    return Math.max(0, Math.min(100, Math.round(value)));
  }

  function setTaskState(key: string, state: "queued" | "running" | null) {
    if (!key) return;
    const next = { ...taskStateByKey.value };
    if (state) {
      next[key] = state;
    } else {
      delete next[key];
      clearDownloadProgress(key);
    }
    taskStateByKey.value = next;
  }

  function clearDownloadProgress(key: string) {
    const nextPercent = { ...downloadPercentByKey.value };
    const nextIndeterminate = { ...downloadIndeterminateByKey.value };
    delete nextPercent[key];
    delete nextIndeterminate[key];
    downloadPercentByKey.value = nextPercent;
    downloadIndeterminateByKey.value = nextIndeterminate;
  }

  function handleDownloadProgress(next: ProgressMessage) {
    const componentID = String(next.component_id || "").trim();
    if (!componentID.startsWith(steamProgressPrefix)) return;
    const key = componentID.slice(steamProgressPrefix.length);
    if (!key) return;
    if (!next.active) {
      clearDownloadProgress(key);
      return;
    }
    const indeterminate = !!next.indeterminate;
    downloadIndeterminateByKey.value = {
      ...downloadIndeterminateByKey.value,
      [key]: indeterminate,
    };
    if (!indeterminate && Number.isFinite(next.percent)) {
      downloadPercentByKey.value = {
        ...downloadPercentByKey.value,
        [key]: Math.max(0, Math.min(100, next.percent)),
      };
    }
  }

  function enqueueTask(task: SteamImportTask): boolean {
    if (!task.key || taskStateByKey.value[task.key]) return false;
    pendingTasks.value = pendingTasks.value.concat(task);
    setTaskState(task.key, "queued");
    processImportQueue();
    return true;
  }

  /** Imports one row of the recent match list. */
  function enqueueMatchImport(row: SteamMatchItem): boolean {
    if (!row.match_id || !row.demo_available) return false;
    return enqueueTask({
      key: row.match_id,
      request: { match_id: row.match_id, share_code: row.share_code },
    });
  }

  function submitManualInput() {
    const tokens = parseManualTokens(manualInput.value);
    if (!tokens.length) {
      showError(t("main.import.steam_manual_empty"));
      return;
    }
    let added = 0;
    for (const token of tokens) {
      if (enqueueTask({ key: taskKeyForToken(token), request: requestForToken(token) })) {
        added++;
      }
    }
    if (added <= 0) {
      showError(t("main.import.steam_manual_no_new"));
      return;
    }
    manualInput.value = "";
  }

  function processImportQueue() {
    while (runningImportCount.value < MAX_CONCURRENT_IMPORTS && pendingTasks.value.length > 0) {
      const [task, ...rest] = pendingTasks.value;
      pendingTasks.value = rest;
      setTaskState(task.key, "running");
      void runImportTask(task);
    }
  }

  async function runImportTask(task: SteamImportTask) {
    try {
      const paths = await backend.ImportSteamMatch(task.request);
      if (paths && paths.length > 0 && onDemosSelected) {
        onDemosSelected(paths);
      }
    } catch (err: unknown) {
      const errorMessage = err instanceof Error ? err.message : String(err);
      showError(t("main.import.steam_import_failed", { error: errorMessage }));
    } finally {
      setTaskState(task.key, null);
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
    manualInput,
    result,
    rows,
    status,
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
  };
}
