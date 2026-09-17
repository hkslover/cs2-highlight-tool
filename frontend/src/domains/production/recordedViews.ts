import type {
  DemoListEntry,
  DemoMaterialSelection,
  ProduceHistoryItem,
} from "@/shared/types";

export type RecordedRole = "killer" | "victim";

export interface RecordedViewStatus {
  killer: boolean;
  victim: boolean;
}

export interface RecordedSpecModes {
  killer: number;
  victim: number;
}

/**
 * The spec modes currently emitted by the production job builder. Keep this
 * next to the completion index so history matching uses the actual request
 * modes rather than treating every mode as interchangeable.
 */
export const DEFAULT_PRODUCE_SPEC_MODES = {
  killer: 1,
  victim: 1,
} as const;

/**
 * Successful ordinary production coverage, indexed by demo, kill, spec mode,
 * and the explicit recording role. Full-round POV and other view names never
 * enter this index.
 */
export type RecordedViewIndex = Map<
  string,
  Map<string, Map<number, Set<RecordedRole>>>
>;

function normalizeDemoPath(value: unknown): string {
  return String(value ?? "").trim();
}

function normalizeKillID(value: unknown): string {
  return String(value ?? "").trim();
}

function normalizeRole(value: unknown): RecordedRole | undefined {
  const view = String(value ?? "")
    .trim()
    .toLowerCase();
  if (view === "killer" || view === "victim") return view;
  return undefined;
}

/** Missing legacy spec modes used the first-person mode. Explicit 0 remains 0. */
function normalizeSpecMode(value: unknown): number | undefined {
  if (value === undefined || value === null)
    return DEFAULT_PRODUCE_SPEC_MODES.killer;
  const mode = Number(value);
  return Number.isFinite(mode) ? mode : undefined;
}

function historyTypeOf(item: ProduceHistoryItem): string {
  const historyType = String(item.history_type ?? "")
    .trim()
    .toLowerCase();
  return historyType || "produce_clip";
}

export function buildRecordedViewIndex(
  history: readonly ProduceHistoryItem[],
): RecordedViewIndex {
  const byDemo: RecordedViewIndex = new Map();

  for (const item of history) {
    if (historyTypeOf(item) !== "produce_clip") continue;
    if (!String(item.video_path ?? "").trim()) continue;

    const demoPath = normalizeDemoPath(item.demo_path);
    const role = normalizeRole(item.view);
    const specMode = normalizeSpecMode(item.spec_mode);
    if (!demoPath || !role || specMode === undefined) continue;

    let byKill = byDemo.get(demoPath);
    if (!byKill) {
      byKill = new Map();
      byDemo.set(demoPath, byKill);
    }

    for (const rawKillID of item.kill_ids || []) {
      const killID = normalizeKillID(rawKillID);
      if (!killID) continue;

      let byMode = byKill.get(killID);
      if (!byMode) {
        byMode = new Map();
        byKill.set(killID, byMode);
      }

      let roles = byMode.get(specMode);
      if (!roles) {
        roles = new Set();
        byMode.set(specMode, roles);
      }
      roles.add(role);
    }
  }

  return byDemo;
}

function hasRecordedRole(
  index: ReadonlyMap<
    string,
    ReadonlyMap<string, ReadonlyMap<number, ReadonlySet<RecordedRole>>>
  >,
  demoPath: string,
  killID: string,
  role: RecordedRole,
  specMode: unknown,
): boolean {
  const mode = normalizeSpecMode(specMode);
  if (mode === undefined) return false;
  return Boolean(index.get(demoPath)?.get(killID)?.get(mode)?.has(role));
}

export function recordedViewsForKill(
  index: ReadonlyMap<
    string,
    ReadonlyMap<string, ReadonlyMap<number, ReadonlySet<RecordedRole>>>
  >,
  demoPath: string,
  killID: string,
  modes: Readonly<RecordedSpecModes> = DEFAULT_PRODUCE_SPEC_MODES,
): RecordedViewStatus {
  const normalizedDemoPath = normalizeDemoPath(demoPath);
  const normalizedKillID = normalizeKillID(killID);
  if (!normalizedDemoPath || !normalizedKillID) {
    return { killer: false, victim: false };
  }

  return {
    killer: hasRecordedRole(
      index,
      normalizedDemoPath,
      normalizedKillID,
      "killer",
      modes.killer,
    ),
    victim: hasRecordedRole(
      index,
      normalizedDemoPath,
      normalizedKillID,
      "victim",
      modes.victim,
    ),
  };
}

/**
 * Return only the requested roles that do not have successful matching
 * history. The returned selection is a derived copy: callers can use it to
 * build a retry job without changing the clip page's original selection.
 */
export function projectPendingSelection(
  item: DemoMaterialSelection,
  demoPath: string,
  index: ReadonlyMap<
    string,
    ReadonlyMap<string, ReadonlyMap<number, ReadonlySet<RecordedRole>>>
  >,
  modes: Readonly<RecordedSpecModes> = DEFAULT_PRODUCE_SPEC_MODES,
): DemoMaterialSelection | undefined {
  const killID = normalizeKillID(item.kill?.id);
  if (!killID) return undefined;

  const recorded = recordedViewsForKill(index, demoPath, killID, modes);
  const includeKiller = item.include_killer !== false && !recorded.killer;
  const includeVictim = Boolean(item.include_victim) && !recorded.victim;
  if (!includeKiller && !includeVictim) return undefined;

  const projected: DemoMaterialSelection = {
    ...item,
    include_killer: includeKiller,
    include_victim: includeVictim,
  };
  if (item.clip_overrides) {
    projected.clip_overrides = { ...item.clip_overrides };
  }
  return projected;
}

export function pendingSelectionsByDemo(
  demos: readonly DemoListEntry[],
  getMaterialSelections: (
    entry: DemoListEntry,
  ) => readonly DemoMaterialSelection[],
  index: ReadonlyMap<
    string,
    ReadonlyMap<string, ReadonlyMap<number, ReadonlySet<RecordedRole>>>
  >,
  modes: Readonly<RecordedSpecModes> = DEFAULT_PRODUCE_SPEC_MODES,
): Map<string, DemoMaterialSelection[]> {
  const byDemo = new Map<string, DemoMaterialSelection[]>();
  for (const entry of demos) {
    const pending: DemoMaterialSelection[] = [];
    for (const item of getMaterialSelections(entry)) {
      const projected = projectPendingSelection(
        item,
        entry.file_path,
        index,
        modes,
      );
      if (projected) pending.push(projected);
    }
    byDemo.set(entry.file_path, pending);
  }
  return byDemo;
}
