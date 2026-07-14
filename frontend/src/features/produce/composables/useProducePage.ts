import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { useMessage } from "naive-ui";
import { t } from "@/shared/i18n";
import type {
  DemoClipKill,
  DemoListEntry,
  DemoMaterialSelection,
  GeneratePluginJSONBatchRequest,
  GeneratePluginJSONBatchResult,
  GeneratePluginJSONRequest,
  ProduceQueueState,
  ProduceTakeFile,
  ProduceTakeFileSnapshot,
  ProduceTakePlan,
  ProduceTakeStatus,
  ProduceTakeStatusSnapshot,
  ProduceWSState,
} from "@/shared/types";

import { useImportDemos } from "@/features/import/composables/useImportDemos";
import { useProducePageState } from "@/features/produce/composables/useProducePageState";
import { ensureProduceHistoryInitialized, useProduceHistory } from "@/features/produce/composables/useProduceHistory";
import { useDebugSettings } from "@/shared/state/useDebugSettings";
import { usePlatformClientCheck } from "@/features/produce/composables/usePlatformClientCheck";
import { EventsOn } from "../../../../wailsjs/runtime/runtime";
import { OPEN_PRODUCE_HISTORY_EVENT } from "@/shared/events";

export type ProduceRowState = "pending" | "recorded" | "waiting_files" | "recording" | "processing" | "completed" | "failed";

export interface ProduceTakeRow {
  key: string;
  demo_path: string;
  take_index: number;
  take_name: string;
  view: string;
  spec_mode: number;
  kill_ids: string[];
  kills: DemoClipKill[];
  round?: number;
  player_name?: string;
  player_steam_id?: string;
}

export interface ProduceTakeRoundRow {
  key: string;
  row: ProduceTakeRow;
  kills: DemoClipKill[];
}

export interface ProduceTakeRoundGroup {
  name: string;
  round: number;
  kill_count: number;
  rows: ProduceTakeRoundRow[];
}

export interface SelectedRoundGroup {
  round: number;
  items: DemoMaterialSelection[];
}

export function useProducePage() {
  const router = useRouter();
  const message = useMessage();
  const {
    clipReadyDemos,
    ensureClipDemoSelected,
    getMaterialSelections,
    getFullRoundPOVSelection,
    getFullRoundPOVTrackingLabel,
    fullRoundPlanByDemo,
    fullRoundPlanErrorByDemo,
  } = useImportDemos();
  const { batchResult, launchViewEnabled, errorMessage, killSnapshotByDemo, resetProducePageState } = useProducePageState();
  const { historySnapshot } = useProduceHistory();
  const { debugEnabled, keepProduceIntermediates } = useDebugSettings();

  const platformCheck = usePlatformClientCheck();
  const showPlatformCheckModal = ref(false);

  const generatingAndLaunching = ref(false);
  const generatingConfigOnlyLoading = ref(false);
  const exportProduceLogsLoading = ref(false);
  const expandedNames = ref<string[]>([]);
  const plannedRoundExpandedByDemo = ref<Record<string, string[]>>({});
  const wsState = ref<ProduceWSState>({
    address: "",
    connected: false,
    updated_at_ms: 0,
  });
  const queueState = ref<ProduceQueueState>({
    running: false,
    total: 0,
    completed: 0,
    current_index: -1,
    pending_ack: false,
    updated_at_ms: 0,
  });
  const takeSnapshot = ref<ProduceTakeStatusSnapshot>({
    items: [],
    total_takes: 0,
    started_takes: 0,
    completed_takes: 0,
    updated_at_ms: 0,
  });
  const takeFiles = ref<ProduceTakeFileSnapshot>({
    items: [],
    updated_at_ms: 0,
  });
  const offEventHandlers: Array<() => void> = [];

  const producedKillIDsByDemo = computed(() => {
    const byDemo = new Map<string, Set<string>>();
    for (const item of historySnapshot.value.items || []) {
      const demoPath = item.demo_path || "";
      if (!demoPath) continue;
      if (!byDemo.has(demoPath)) {
        byDemo.set(demoPath, new Set<string>());
      }
      const set = byDemo.get(demoPath)!;
      for (const killID of item.kill_ids || []) {
        if (killID) {
          set.add(killID);
        }
      }
    }
    return byDemo;
  });

  const pendingSelectionsByDemo = computed(() => {
    const byDemo = new Map<string, DemoMaterialSelection[]>();
    for (const entry of clipReadyDemos.value) {
      const produced = producedKillIDsByDemo.value.get(entry.file_path);
      const pending = getMaterialSelections(entry).filter((item) => {
        const killID = item.kill?.id || "";
        if (!killID) return false;
        return !produced?.has(killID);
      });
      byDemo.set(entry.file_path, pending);
    }
    return byDemo;
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

  const plannedRowsByDemo = computed(() => {
    const byDemo = new Map<string, ProduceTakeRow[]>();
    if (!launchViewEnabled.value) {
      return byDemo;
    }
    const result = batchResult.value;
    if (!result?.results?.length) {
      return byDemo;
    }
    for (const item of result.results) {
      const demoPath = item.demo_path;
      const snapshotKills = killSnapshotByDemo.value[demoPath] || [];
      const killMap = new Map<string, DemoClipKill>(
        snapshotKills
          .filter((kill) => !!kill?.id)
          .map((kill) => [kill.id, kill]),
      );
      if (killMap.size === 0) {
        const live = selectedKillsByDemo.value.get(demoPath);
        if (live) {
          for (const [id, kill] of live.entries()) {
            killMap.set(id, kill);
          }
        }
      }
      const plans = item.take_plans || [];
      const rows: ProduceTakeRow[] = [];
      for (const plan of plans) {
        rows.push(buildTakeRow(plan, demoPath, killMap));
      }
      if (rows.length) {
        byDemo.set(demoPath, rows);
      }
    }
    return byDemo;
  });

  const plannedRoundGroupsByDemo = computed(() => {
    const byDemo = new Map<string, ProduceTakeRoundGroup[]>();
    for (const [demoPath, rows] of plannedRowsByDemo.value.entries()) {
      if (!rows.length) continue;
      const groupMap = new Map<string, ProduceTakeRoundGroup>();
      for (const row of rows) {
        if (String(row.view).toLowerCase() === "full_round_pov") {
          const groupName = "pov-group";
          if (!groupMap.has(groupName)) {
            groupMap.set(groupName, {
              name: groupName,
              round: 0,
              kill_count: 0,
              rows: [],
            });
          }
          const group = groupMap.get(groupName)!;
          group.rows.push({
            key: `${row.key}#${groupName}`,
            row,
            kills: [],
          });
          continue;
        }
        const groupedKills = splitKillsByRound(row.kills);
        if (!groupedKills.length) {
          groupedKills.push({ round: Number(row.round || 0), kills: [] });
        }
        for (const grouped of groupedKills) {
          const groupName = grouped.round > 0 ? `round-${grouped.round}` : "round-unknown";
          if (!groupMap.has(groupName)) {
            groupMap.set(groupName, {
              name: groupName,
              round: grouped.round,
              kill_count: 0,
              rows: [],
            });
          }
          const group = groupMap.get(groupName)!;
          group.rows.push({
            key: `${row.key}#${groupName}`,
            row,
            kills: grouped.kills,
          });
          group.kill_count += grouped.kills.length;
        }
      }
      const sortedGroups = Array.from(groupMap.values())
        .map((group) => ({
          ...group,
          rows: group.rows.slice().sort((a, b) => compareTakeRows(a.row, b.row)),
        }))
        .sort((a, b) => {
          if (a.round <= 0 && b.round <= 0) return 0;
          if (a.round <= 0) return 1;
          if (b.round <= 0) return -1;
          return a.round - b.round;
        });
      byDemo.set(demoPath, sortedGroups);
    }
    return byDemo;
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
    try {
      await ensureProduceHistoryInitialized();
    } catch {
      // ignore and continue
    }
    let queueRunning = false;
    try {
      queueState.value = await callBackend<ProduceQueueState>("GetProduceQueueState");
      queueRunning = !!queueState.value.running;
    } catch {
      // ignore and wait for events
    }
    if (!queueRunning) {
      resetProducePageState();
    }
    try {
      wsState.value = await callBackend<ProduceWSState>("GetProduceWSState");
    } catch {
      // ignore and wait for events
    }
    try {
      takeSnapshot.value = await callBackend<ProduceTakeStatusSnapshot>("GetProduceTakeSnapshot");
    } catch {
      // ignore and wait for events
    }
    try {
      takeFiles.value = await callBackend<ProduceTakeFileSnapshot>("GetProduceTakeFiles");
    } catch {
      // ignore and wait for events
    }

    offEventHandlers.push(
      EventsOn("produce_ws_state_changed", (next: ProduceWSState) => {
        wsState.value = next;
      }),
    );
    offEventHandlers.push(
      EventsOn("produce_queue_state_changed", (next: ProduceQueueState) => {
        queueState.value = next;
      }),
    );
    offEventHandlers.push(
      EventsOn("produce_take_status_changed", (next: ProduceTakeStatusSnapshot) => {
        takeSnapshot.value = next;
      }),
    );
    offEventHandlers.push(
      EventsOn("produce_take_file_changed", (next: ProduceTakeFileSnapshot) => {
        takeFiles.value = next;
      }),
    );
  });

  onBeforeUnmount(() => {
    for (const off of offEventHandlers) {
      off();
    }
    offEventHandlers.length = 0;
  });

  async function callBackend<T>(method: string, ...args: unknown[]): Promise<T> {
    const api = (window as any).go?.app?.App as Record<string, (...a: unknown[]) => Promise<unknown>> | undefined;
    const fn = api?.[method];
    if (!fn) throw new Error(`Wails API not loaded: ${method}`);
    return fn(...args) as Promise<T>;
  }

  function buildTakeRow(
    plan: ProduceTakePlan,
    fallbackDemoPath: string,
    killMap: Map<string, DemoClipKill>,
  ): ProduceTakeRow {
    const demoPath = plan.demo_path || fallbackDemoPath;
    const takeIndex = Number(plan.take_index || 0);
    const view = String(plan.view || "killer");
    const killIDs = (plan.kill_ids || []).filter((id) => !!id);
    const kills = killIDs.map((id) => killMap.get(id)).filter((kill): kill is DemoClipKill => !!kill);
    return {
      key: takeRowKey(demoPath, takeIndex, view),
      demo_path: demoPath,
      take_index: takeIndex,
      take_name: String(plan.take_name || ""),
      view,
      spec_mode: Number(plan.spec_mode || 1),
      kill_ids: killIDs,
      kills,
      round: Number(plan.round || 0),
      player_name: String(plan.player_name || ""),
      player_steam_id: String(plan.player_steam_id || ""),
    };
  }

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
    if (!selection.enabled) return 0;
    const plan = fullRoundPlanByDemo.value[entry.key];
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
    const grouped = new Map<number, DemoMaterialSelection[]>();
    for (const item of pendingSelectionsForDemo(entry)) {
      const round = Number(item.kill?.round || 0);
      if (!grouped.has(round)) {
        grouped.set(round, []);
      }
      grouped.get(round)!.push(item);
    }
    return Array.from(grouped.entries())
      .sort((a, b) => a[0] - b[0])
      .map(([round, items]) => ({
        round,
        items: items.slice().sort((a, b) => {
          const tickA = Number(a.kill?.tick || 0);
          const tickB = Number(b.kill?.tick || 0);
          if (tickA === tickB) {
            return String(a.kill?.id || "").localeCompare(String(b.kill?.id || ""));
          }
          return tickA - tickB;
        }),
      }));
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
      .map(([round, items]) => ({
        round,
        kills: items.slice().sort((a, b) => {
          const tickA = Number(a.tick || 0);
          const tickB = Number(b.tick || 0);
          if (tickA === tickB) {
            return String(a.id || "").localeCompare(String(b.id || ""));
          }
          return tickA - tickB;
        }),
      }));
  }

  function compareTakeRows(a: ProduceTakeRow, b: ProduceTakeRow): number {
    if (a.take_index !== b.take_index) return a.take_index - b.take_index;
    return a.view.localeCompare(b.view);
  }

  function buildPendingBatchJobs(): GeneratePluginJSONRequest[] {
    return clipReadyDemos.value
      .map((entry) => {
        const selection = getFullRoundPOVSelection(entry);
        const hasPOVSegments = selection.enabled && !!selection.player_steam_id && povSegmentCountForDemo(entry) > 0;
        return {
          demo_path: entry.file_path,
          tick_rate: entry.meta?.tick_rate ?? 64,
          selected_items: pendingSelectionsForDemo(entry).map((item) => ({
            kill: item.kill,
            include_killer: item.include_killer,
            include_victim: item.include_victim,
            killer_spec_mode: 1,
            victim_spec_mode: 1,
            clip_overrides: item.clip_overrides,
          })),
          full_round_pov: hasPOVSegments
            ? { player_steam_id: selection.player_steam_id }
            : undefined,
        };
      })
      .filter((job) => job.selected_items.length > 0 || !!job.full_round_pov);
  }

  async function generateAndLaunch() {
    const jobs = buildPendingBatchJobs();
    if (!jobs.length) return;

    const allClosed = await platformCheck.checkAll();
    if (!allClosed) {
      showPlatformCheckModal.value = true;
      return;
    }
    await doGenerateAndLaunch();
  }

  async function onPlatformCheckConfirmed() {
    showPlatformCheckModal.value = false;
    platformCheck.reset();
    await doGenerateAndLaunch();
  }

  function onPlatformCheckCancelled() {
    showPlatformCheckModal.value = false;
    platformCheck.reset();
  }

  async function doGenerateAndLaunch() {
    try {
      const jobs = buildPendingBatchJobs();
      if (!jobs.length) return;
      generatingAndLaunching.value = true;
      launchViewEnabled.value = true;
      errorMessage.value = "";

      captureCurrentKillSnapshot();
      const request: GeneratePluginJSONBatchRequest = {
        jobs,
        debug: {
          keep_intermediate_files: keepProduceIntermediates.value,
        },
      };
      const result = await callBackend<GeneratePluginJSONBatchResult>("GeneratePluginJSONBatchAndLaunchHLAE", request);
      batchResult.value = result;
      if (!result.launch_started && result.launch_error) {
        errorMessage.value = result.launch_error;
      }
    } catch (err: unknown) {
      errorMessage.value = err instanceof Error ? err.message : String(err);
    } finally {
      generatingAndLaunching.value = false;
    }
  }

  async function generateConfigOnly() {
    try {
      const jobs = buildPendingBatchJobs();
      if (!jobs.length) return;
      generatingConfigOnlyLoading.value = true;
      errorMessage.value = "";

      const result = await callBackend<GeneratePluginJSONBatchResult>("GeneratePluginJSONBatch", { jobs });
      const summary = t("main.produce.batch_summary", {
        success: result.success_count,
        failed: result.failure_count,
      });
      if (result.success_count > 0 && result.failure_count === 0) {
        message.success(summary);
        return;
      }
      if (result.success_count > 0) {
        message.warning(summary);
        return;
      }
      message.error(summary);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      errorMessage.value = msg;
      message.error(msg);
    } finally {
      generatingConfigOnlyLoading.value = false;
    }
  }

  function takeRowKey(demoPath: string, takeIndex: number, view: string): string {
    return `${demoPath}#${takeIndex}#${view}`;
  }

  function takeStatusKey(demoPath: string, takeIndex: number): string {
    return `${demoPath}#${takeIndex}`;
  }

  function takeFileKey(demoPath: string, takeIndex: number, view: string): string {
    return `${demoPath}#${takeIndex}#${view}`;
  }

  function resolveTakeState(row: ProduceTakeRow): ProduceRowState {
    const file = takeFileByRow(row);
    if (file?.status === "failed") return "failed";
    if (file?.status === "completed") return "completed";
    if (file?.status === "processing") return "processing";
    if (file?.status === "waiting_files") return "waiting_files";
    if (file?.status === "recorded") return "recorded";

    const take = takeStatusByKey.value.get(takeStatusKey(row.demo_path, row.take_index));
    if (take?.status === "recording") return "recording";
    if (take?.status === "completed") return "recorded";
    return "pending";
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
      await callBackend<void>("OpenProducedClipInFolder", file.video_path);
    } catch (err: unknown) {
      errorMessage.value = err instanceof Error ? err.message : String(err);
    }
  }

  async function exportProduceLogs() {
    if (exportProduceLogsLoading.value) return;
    exportProduceLogsLoading.value = true;
    try {
      const path = await callBackend<string>("ExportProduceWSLogs");
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

  function openHistoryDrawer() {
    window.dispatchEvent(new CustomEvent(OPEN_PRODUCE_HISTORY_EVENT));
  }

  function goToEdit() {
    void router.push("/edit");
  }

  return {
    // State refs
    errorMessage,
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
    // Functions
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
  };
}
