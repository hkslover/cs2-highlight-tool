import { computed, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { useMessage } from "naive-ui";
import { t } from "@/shared/i18n";
import type {
  DemoClipKill,
  DemoListEntry,
  DemoMaterialSelection,
  GeneratePluginJSONRequest,
  ProduceTakeFile,
  ProduceTakeStatus,
} from "@/shared/types";

import {
  clipReadyDemos,
  demoList,
  ensureClipDemoSelected,
} from "@/domains/demo";
import {
  fullRoundPlanByDemo,
  fullRoundPlanErrorByDemo,
  getFullRoundPOVSelection,
  getFullRoundPOVTrackingLabel,
  getMaterialSelections,
} from "@/domains/clip-selection";
import { useProducePageState } from "@/features/produce/composables/useProducePageState";
import { useProduceHistory } from "@/features/produce/composables/useProduceHistory";
import { useWorkActivity } from "@/shared/state/useWorkActivity";
import { useDebugSettings } from "@/shared/state/useDebugSettings";
import { useProduceLaunch } from "@/features/produce/composables/useProduceLaunch";
import { useProduceRequests } from "@/features/produce/composables/useProduceRequests";
import { useProduceStateSync } from "@/features/produce/composables/useProduceStateSync";
import { buildProduceJobs } from "@/domains/production/jobs";
import {
  buildPlannedRoundGroupsByDemo,
  buildPlannedRowsByDemo,
  buildSelectedRoundGroups,
  buildTakeRow,
  compareTakeRows,
  pendingSelectionsByDemo as selectPendingSelectionsByDemo,
  producedKillIDsByDemo as selectProducedKillIDsByDemo,
  resolveTakeState as resolveProduceTakeState,
  splitKillsByRound,
  takeFileKey,
  takeStatusKey,
  type ProduceRowState,
  type ProduceTakeRoundGroup,
  type ProduceTakeRow,
  type SelectedRoundGroup,
} from "@/domains/production/selectors";
import { OPEN_PRODUCE_HISTORY_EVENT } from "@/shared/events";

export function useProducePage() {
  const router = useRouter();
  const message = useMessage();
  const { batchResult, launchViewEnabled, errorMessage, killSnapshotByDemo, resetProducePageState } = useProducePageState();
  const { historySnapshot } = useProduceHistory();
  const { debugEnabled, keepProduceIntermediates } = useDebugSettings();

  const { produceBusy, refreshWorkActivity } = useWorkActivity();
  const exportProduceLogsLoading = ref(false);
  const expandedNames = ref<string[]>([]);
  const plannedRoundExpandedByDemo = ref<Record<string, string[]>>({});
  const {
    wsState,
    queueState,
    takeSnapshot,
    takeFiles,
    initialize: initializeProductionState,
  } = useProduceStateSync();

  const requests = useProduceRequests();

  const producedKillIDsByDemo = computed(() => {
    return selectProducedKillIDsByDemo(historySnapshot.value.items || []);
  });

  const pendingSelectionsByDemo = computed(() => {
    return selectPendingSelectionsByDemo(
      clipReadyDemos.value,
      getMaterialSelections,
      producedKillIDsByDemo.value,
    );
  });

  const selectedKillsByDemo = computed(() => {
    const byDemo = new Map<string, Map<string, DemoClipKill>>();
    for (const entry of clipReadyDemos.value) {
      const byID = new Map<string, DemoClipKill>();
      for (const item of pendingSelectionsForDemo(entry)) {
        if (item.kill?.id) {
          byID.set(item.kill.id, item.kill);
        }
      }
      byDemo.set(entry.file_path, byID);
    }
    return byDemo;
  });

  const displayDemos = computed(() =>
    clipReadyDemos.value.filter((entry) => {
      if ((plannedRowsByDemo.value.get(entry.file_path) || []).length > 0) {
        return true;
      }
      if (getFullRoundPOVSelection(entry).enabled) {
        return true;
      }
      return pendingSelectionsForDemo(entry).length > 0;
    }),
  );

  const hasPendingMaterials = computed(() =>
    clipReadyDemos.value.some((entry) => {
      if (pendingSelectionsForDemo(entry).length > 0) return true;
      return povSegmentCountForDemo(entry) > 0;
    }),
  );

  const hasEditableClips = computed(() =>
    (historySnapshot.value.items || []).some((item) => {
      const type = item.history_type || "produce_clip";
      return type === "produce_clip" && !!String(item.video_path || "").trim();
    }),
  );

  const emptyStage = computed<"no_demos" | "no_selections" | "all_completed">(() => {
    if (!demoList.value.length) {
      return "no_demos";
    }
    const hasProducedHistory = (historySnapshot.value.items || []).some(
      (item) => (item.history_type || "produce_clip") === "produce_clip",
    );
    if (hasProducedHistory) {
      return "all_completed";
    }
    return "no_selections";
  });

  const plannedRowsByDemo = computed(() => {
    return buildPlannedRowsByDemo(
      launchViewEnabled.value,
      batchResult.value,
      killSnapshotByDemo.value,
      selectedKillsByDemo.value,
    );
  });

  const plannedRoundGroupsByDemo = computed(() => {
    return buildPlannedRoundGroupsByDemo(plannedRowsByDemo.value);
  });

  const takeStatusByKey = computed(() => {
    const byKey = new Map<string, ProduceTakeStatus>();
    for (const status of takeSnapshot.value.items || []) {
      const takeIndex = Number(status.take_index || 0);
      const demoPath = status.demo_path || "";
      if (!demoPath || takeIndex <= 0) continue;
      byKey.set(takeStatusKey(demoPath, takeIndex), status);
    }
    return byKey;
  });

  const takeFileByKey = computed(() => {
    const byKey = new Map<string, ProduceTakeFile>();
    for (const file of takeFiles.value.items || []) {
      if (!file.demo_path || !file.take_index) continue;
      byKey.set(takeFileKey(file.demo_path, file.take_index, file.view || ""), file);
    }
    return byKey;
  });

  const runtimeStateType = computed<"info" | "warning" | "success">(() => {
    if (queueState.value.running && !wsState.value.connected) {
      return "warning";
    }
    return "info";
  });

  const runtimeStateMessage = computed(() => {
    if (!generatingAndLaunching.value && !queueState.value.running) return "";
    if (generatingAndLaunching.value && !queueState.value.running) {
      return t("main.produce.runtime_preparing");
    }
    if (queueState.value.running && !wsState.value.connected) {
      return t("main.produce.runtime_waiting_plugin");
    }
    if (queueState.value.running && queueState.value.pending_ack) {
      return t("main.produce.runtime_loading_demo");
    }
    if (queueState.value.running && takeSnapshot.value.started_takes === 0) {
      return t("main.produce.runtime_loading_demo");
    }
    if (queueState.value.running) {
      return t("main.produce.runtime_recording");
    }
    return "";
  });

  const canExportProduceLogs = computed(() =>
    Boolean(wsState.value.last_error || queueState.value.last_error || errorMessage.value),
  );

  watch(
    () => displayDemos.value.map((entry) => entry.key),
    (keys) => {
      if (expandedNames.value.length === 0) {
        expandedNames.value = [...keys];
        return;
      }
      const next = expandedNames.value.filter((name) => keys.includes(name));
      for (const key of keys) {
        if (!next.includes(key)) {
          next.push(key);
        }
      }
      expandedNames.value = next;
    },
    { immediate: true },
  );

  onMounted(async () => {
    ensureClipDemoSelected();
    let initialized = false;
    try {
      await initializeProductionState();
      initialized = true;
    } catch {
      // The app-level store keeps retryable subscriptions. A later app-shell
      // retry or route entry can call initializeProductionState again.
    }
    await refreshWorkActivity();
    // A non-running queue is not enough to declare the session idle: merge,
    // cleanup, and environment restoration remain covered by produceBusy.
    if (initialized && !queueState.value.running && !produceBusy.value) {
      resetProducePageState();
    }
  });

  function plannedRowsForDemo(entry: DemoListEntry): ProduceTakeRow[] {
    return plannedRowsByDemo.value.get(entry.file_path) || [];
  }

  function plannedRoundGroupsForDemo(entry: DemoListEntry): ProduceTakeRoundGroup[] {
    return plannedRoundGroupsByDemo.value.get(entry.file_path) || [];
  }

  function getPlannedRoundExpandedNames(entry: DemoListEntry): string[] {
    const groups = plannedRoundGroupsForDemo(entry);
    const defaults = groups.map((group) => group.name);
    if (!Object.prototype.hasOwnProperty.call(plannedRoundExpandedByDemo.value, entry.key)) {
      return defaults;
    }
    const current = plannedRoundExpandedByDemo.value[entry.key] || [];
    const allowed = new Set(defaults);
    return current.filter((name) => allowed.has(name));
  }

  function handlePlannedRoundExpandedChange(
    entry: DemoListEntry,
    names: Array<string | number> | string | number | null,
  ) {
    const normalized = Array.isArray(names)
      ? names.map((name) => String(name))
      : names == null
        ? []
        : [String(names)];
    plannedRoundExpandedByDemo.value = {
      ...plannedRoundExpandedByDemo.value,
      [entry.key]: normalized,
    };
  }

  function onPlannedRoundExpandedChange(
    entry: DemoListEntry,
    names: Array<string | number> | string | number | null,
  ) {
    handlePlannedRoundExpandedChange(entry, names);
  }

  function plannedRoundTitle(group: ProduceTakeRoundGroup): string {
    if (group.name === "pov-group") {
      return t("main.clips.full_round_pov_group_title");
    }
    if (group.round > 0) {
      return t("main.clips.round_title", { round: group.round, kills: group.kill_count });
    }
    return t("main.produce.round_unknown_title", { kills: group.kill_count });
  }

  function povSegmentCountForDemo(entry: DemoListEntry): number {
    const selection = getFullRoundPOVSelection(entry);
    const plan = fullRoundPlanByDemo.value[entry.key];
    const selectedPlayer = String(selection.player_steam_id || "").trim();
    const plannedPlayer = String(plan?.player_steam_id || "").trim();
    if (!selection.enabled || !selectedPlayer || plannedPlayer !== selectedPlayer) return 0;
    return plan?.segments?.length ?? 0;
  }

  function displayCountForDemo(entry: DemoListEntry): number {
    const planned = plannedRowsForDemo(entry);
    if (planned.length > 0) return planned.length;
    const pendingMaterialCount = pendingSelectionsForDemo(entry).length;
    const povCount = povSegmentCountForDemo(entry);
    return pendingMaterialCount + povCount;
  }

  function pendingSelectionsForDemo(entry: DemoListEntry): DemoMaterialSelection[] {
    return pendingSelectionsByDemo.value.get(entry.file_path) || [];
  }

  function selectedRoundGroupsForDemo(entry: DemoListEntry): SelectedRoundGroup[] {
    return buildSelectedRoundGroups(pendingSelectionsForDemo(entry));
  }

  function buildPendingBatchJobs(): GeneratePluginJSONRequest[] {
    return buildProduceJobs({
      demos: clipReadyDemos.value,
      getMaterialSelections: (entry) => pendingSelectionsForDemo(entry),
      getFullRoundPOVSelection: (entry) => getFullRoundPOVSelection(entry),
      getFullRoundPOVPlan: (entry) => fullRoundPlanByDemo.value[entry.key],
    });
  }

  function resolveTakeState(row: ProduceTakeRow): ProduceRowState {
    return resolveProduceTakeState(row, takeFileByKey.value, takeStatusByKey.value);
  }

  function statusText(state: ProduceRowState): string {
    if (state === "recording") return t("main.produce.take_status_recording");
    if (state === "recorded") return t("main.produce.take_status_recorded");
    if (state === "waiting_files") return t("main.produce.take_status_waiting_files");
    if (state === "processing") return t("main.produce.take_status_processing");
    if (state === "completed") return t("main.produce.take_status_completed");
    if (state === "failed") return t("main.produce.take_status_failed");
    return t("main.produce.take_status_pending");
  }

  function isSpinningState(state: ProduceRowState): boolean {
    return state === "recording" || state === "processing";
  }

  function statusTagType(state: ProduceRowState): "default" | "warning" | "success" | "error" {
    if (state === "completed") return "success";
    if (state === "failed") return "error";
    if (state === "waiting_files" || state === "recorded") return "warning";
    return "default";
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

  function rowSourceLabel(row: ProduceTakeRow): string {
    if (String(row.view).toLowerCase() === "full_round_pov") {
      return t("main.produce.full_round_pov_row", {
        round: Number(row.round || 0),
        player: row.player_name || row.player_steam_id || "-",
      });
    }
    return t("main.produce.kill_info_missing");
  }

  function takeFileByRow(row: ProduceTakeRow): ProduceTakeFile | undefined {
    return takeFileByKey.value.get(takeFileKey(row.demo_path, row.take_index, row.view));
  }

  function canOpenClip(row: ProduceTakeRow): boolean {
    const file = takeFileByRow(row);
    return !!(file && file.status === "completed" && file.video_path);
  }

  async function openProducedClip(row: ProduceTakeRow) {
    const file = takeFileByRow(row);
    if (!file?.video_path) return;
    try {
      await requests.openProducedClip(file.video_path);
    } catch (err: unknown) {
      errorMessage.value = err instanceof Error ? err.message : String(err);
    }
  }

  async function exportProduceLogs() {
    if (exportProduceLogsLoading.value) return;
    exportProduceLogsLoading.value = true;
    try {
      const path = await requests.exportProduceWSLogs();
      if (path) {
        message.success(t("main.produce.export_logs_success", { path }));
      } else {
        message.info(t("main.produce.export_logs_cancelled"));
      }
    } catch (err: unknown) {
      const detail = err instanceof Error ? err.message : String(err);
      message.error(t("main.produce.export_logs_failed", { error: detail }));
    } finally {
      exportProduceLogsLoading.value = false;
    }
  }

  function captureCurrentKillSnapshot() {
    const snapshot: Record<string, DemoClipKill[]> = {};
    for (const entry of clipReadyDemos.value) {
      const byID = new Map<string, DemoClipKill>();
      for (const item of pendingSelectionsForDemo(entry)) {
        if (item.kill?.id) {
          byID.set(item.kill.id, item.kill);
        }
      }
      snapshot[entry.file_path] = Array.from(byID.values());
    }
    killSnapshotByDemo.value = snapshot;
  }

  const {
    generatingAndLaunching,
    generatingConfigOnlyLoading,
    showPlatformCheckModal,
    generateAndLaunch,
    generateConfigOnly,
    onPlatformCheckConfirmed,
    onPlatformCheckCancelled,
  } = useProduceLaunch({
    buildJobs: buildPendingBatchJobs,
    produceBusy,
    keepProduceIntermediates,
    batchResult,
    launchViewEnabled,
    errorMessage,
    killSnapshotByDemo,
    captureCurrentKillSnapshot,
    refreshWorkActivity,
  });

  function openHistoryDrawer() {
    window.dispatchEvent(new CustomEvent(OPEN_PRODUCE_HISTORY_EVENT));
  }

  function goToImport() {
    void router.push("/import");
  }

  function goToClips() {
    void router.push("/clips");
  }

  function goToEdit() {
    void router.push("/edit");
  }

  return {
    // State refs
    errorMessage,
    produceBusy,
    generatingAndLaunching,
    generatingConfigOnlyLoading,
    exportProduceLogsLoading,
    expandedNames,
    plannedRoundExpandedByDemo,
    wsState,
    queueState,
    takeSnapshot,
    takeFiles,
    showPlatformCheckModal,
    // Computed
    producedKillIDsByDemo,
    pendingSelectionsByDemo,
    selectedKillsByDemo,
    displayDemos,
    hasPendingMaterials,
    hasEditableClips,
    emptyStage,
    getFullRoundPOVSelection,
    getFullRoundPOVTrackingLabel,
    fullRoundPlanByDemo,
    fullRoundPlanErrorByDemo,
    plannedRowsByDemo,
    plannedRoundGroupsByDemo,
    takeStatusByKey,
    takeFileByKey,
    runtimeStateType,
    runtimeStateMessage,
    canExportProduceLogs,
    // Methods
    buildTakeRow,
    plannedRowsForDemo,
    plannedRoundGroupsForDemo,
    getPlannedRoundExpandedNames,
    handlePlannedRoundExpandedChange,
    onPlannedRoundExpandedChange,
    plannedRoundTitle,
    displayCountForDemo,
    povSegmentCountForDemo,
    pendingSelectionsForDemo,
    selectedRoundGroupsForDemo,
    splitKillsByRound,
    compareTakeRows,
    buildPendingBatchJobs,
    generateAndLaunch,
    generateConfigOnly,
    onPlatformCheckConfirmed,
    onPlatformCheckCancelled,
    resolveTakeState,
    statusText,
    isSpinningState,
    statusTagType,
    viewLabel,
    viewTagType,
    rowSourceLabel,
    takeFileByRow,
    canOpenClip,
    openProducedClip,
    exportProduceLogs,
    openHistoryDrawer,
    goToEdit,
    goToImport,
    goToClips,
  };
}
