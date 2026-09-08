import type {
  DemoClipKill,
  DemoListEntry,
  DemoMaterialSelection,
  GeneratePluginJSONBatchResult,
  ProduceHistoryItem,
  ProduceTakeFile,
  ProduceTakePlan,
  ProduceTakeStatus,
} from "@/shared/types";

export type ProduceRowState =
  | "pending"
  | "recorded"
  | "waiting_files"
  | "recording"
  | "processing"
  | "completed"
  | "failed";

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

export function takeRowKey(demoPath: string, takeIndex: number, view: string): string {
  return `${demoPath}#${takeIndex}#${view}`;
}

export function takeStatusKey(demoPath: string, takeIndex: number): string {
  return `${demoPath}#${takeIndex}`;
}

export function takeFileKey(demoPath: string, takeIndex: number, view: string): string {
  return `${demoPath}#${takeIndex}#${view}`;
}

export function buildTakeRow(
  plan: ProduceTakePlan,
  fallbackDemoPath: string,
  killMap: ReadonlyMap<string, DemoClipKill>,
): ProduceTakeRow {
  const demoPath = plan.demo_path || fallbackDemoPath;
  const takeIndex = Number(plan.take_index || 0);
  const view = String(plan.view || "killer");
  const killIDs = (plan.kill_ids || []).filter((id) => Boolean(id));
  const kills = killIDs
    .map((id) => killMap.get(id))
    .filter((kill): kill is DemoClipKill => Boolean(kill));
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

export function buildPlannedRowsByDemo(
  launchViewEnabled: boolean,
  batchResult: GeneratePluginJSONBatchResult | null,
  killSnapshotByDemo: Readonly<Record<string, DemoClipKill[]>>,
  selectedKillsByDemo: ReadonlyMap<string, ReadonlyMap<string, DemoClipKill>>,
): Map<string, ProduceTakeRow[]> {
  const byDemo = new Map<string, ProduceTakeRow[]>();
  if (!launchViewEnabled || !batchResult?.results?.length) return byDemo;

  for (const item of batchResult.results) {
    const demoPath = item.demo_path;
    const snapshotKills = killSnapshotByDemo[demoPath] || [];
    const killMap = new Map<string, DemoClipKill>(
      snapshotKills
        .filter((kill) => Boolean(kill?.id))
        .map((kill) => [kill.id, kill]),
    );
    if (killMap.size === 0) {
      const live = selectedKillsByDemo.get(demoPath);
      if (live) {
        for (const [id, kill] of live.entries()) killMap.set(id, kill);
      }
    }
    const rows = (item.take_plans || []).map((plan) => buildTakeRow(plan, demoPath, killMap));
    if (rows.length) byDemo.set(demoPath, rows);
  }
  return byDemo;
}

export function splitKillsByRound(
  kills: readonly DemoClipKill[],
): Array<{ round: number; kills: DemoClipKill[] }> {
  const grouped = new Map<number, DemoClipKill[]>();
  for (const kill of kills) {
    const round = Number(kill.round || 0);
    const normalizedRound = round > 0 ? round : 0;
    if (!grouped.has(normalizedRound)) grouped.set(normalizedRound, []);
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
      kills: items.slice().sort(compareKills),
    }));
}

function compareKills(a: DemoClipKill, b: DemoClipKill): number {
  const tickA = Number(a.tick || 0);
  const tickB = Number(b.tick || 0);
  if (tickA === tickB) return String(a.id || "").localeCompare(String(b.id || ""));
  return tickA - tickB;
}

export function compareTakeRows(a: ProduceTakeRow, b: ProduceTakeRow): number {
  if (a.take_index !== b.take_index) return a.take_index - b.take_index;
  return a.view.localeCompare(b.view);
}

export function buildPlannedRoundGroupsByDemo(
  plannedRowsByDemo: ReadonlyMap<string, readonly ProduceTakeRow[]>,
): Map<string, ProduceTakeRoundGroup[]> {
  const byDemo = new Map<string, ProduceTakeRoundGroup[]>();
  for (const [demoPath, rows] of plannedRowsByDemo.entries()) {
    if (!rows.length) continue;
    const groupMap = new Map<string, ProduceTakeRoundGroup>();
    for (const row of rows) {
      if (String(row.view).toLowerCase() === "full_round_pov") {
        const groupName = "pov-group";
        if (!groupMap.has(groupName)) {
          groupMap.set(groupName, { name: groupName, round: 0, kill_count: 0, rows: [] });
        }
        groupMap.get(groupName)!.rows.push({
          key: `${row.key}#${groupName}`,
          row,
          kills: [],
        });
        continue;
      }
      const groupedKills = splitKillsByRound(row.kills);
      if (!groupedKills.length) groupedKills.push({ round: Number(row.round || 0), kills: [] });
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
        group.rows.push({ key: `${row.key}#${groupName}`, row, kills: grouped.kills });
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
}

export function buildSelectedRoundGroups(
  selections: readonly DemoMaterialSelection[],
): SelectedRoundGroup[] {
  const grouped = new Map<number, DemoMaterialSelection[]>();
  for (const item of selections) {
    const round = Number(item.kill?.round || 0);
    if (!grouped.has(round)) grouped.set(round, []);
    grouped.get(round)!.push(item);
  }
  return Array.from(grouped.entries())
    .sort((a, b) => a[0] - b[0])
    .map(([round, items]) => ({
      round,
      items: items.slice().sort((a, b) => compareKills(a.kill, b.kill)),
    }));
}

export function producedKillIDsByDemo(
  items: readonly ProduceHistoryItem[],
): Map<string, Set<string>> {
  const byDemo = new Map<string, Set<string>>();
  for (const item of items) {
    if ((item.history_type || "produce_clip") === "edited_video") continue;
    const demoPath = item.demo_path || "";
    if (!demoPath) continue;
    if (!byDemo.has(demoPath)) byDemo.set(demoPath, new Set<string>());
    const ids = byDemo.get(demoPath)!;
    for (const killID of item.kill_ids || []) if (killID) ids.add(killID);
  }
  return byDemo;
}

export function pendingSelectionsByDemo(
  demos: readonly DemoListEntry[],
  getMaterialSelections: (entry: DemoListEntry) => readonly DemoMaterialSelection[],
  produced: ReadonlyMap<string, ReadonlySet<string>>,
): Map<string, DemoMaterialSelection[]> {
  const byDemo = new Map<string, DemoMaterialSelection[]>();
  for (const entry of demos) {
    const producedIDs = produced.get(entry.file_path);
    const pending = getMaterialSelections(entry).filter((item) => {
      const killID = item.kill?.id || "";
      return Boolean(killID) && !producedIDs?.has(killID);
    });
    byDemo.set(entry.file_path, pending);
  }
  return byDemo;
}

export function resolveTakeState(
  row: ProduceTakeRow,
  takeFiles: ReadonlyMap<string, ProduceTakeFile>,
  takeStatuses: ReadonlyMap<string, ProduceTakeStatus>,
): ProduceRowState {
  const file = takeFiles.get(takeFileKey(row.demo_path, row.take_index, row.view));
  if (file?.status === "failed") return "failed";
  if (file?.status === "completed") return "completed";
  if (file?.status === "processing") return "processing";
  if (file?.status === "waiting_files") return "waiting_files";
  if (file?.status === "recorded") return "recorded";

  const take = takeStatuses.get(takeStatusKey(row.demo_path, row.take_index));
  if (take?.status === "recording") return "recording";
  if (take?.status === "completed") return "recorded";
  return "pending";
}
