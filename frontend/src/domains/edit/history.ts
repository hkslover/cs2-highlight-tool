import type { DemoClipKill, ProduceHistoryItem } from "@/shared/types";

export type HistoryItemView = "killer" | "victim" | "full_round_pov";

export interface HistoryDemoGroup {
  demo_path: string;
  items: ProduceHistoryItem[];
}

export interface HistoryRoundGroup {
  name: string;
  round: number;
  kill_count: number;
  items: ProduceHistoryItem[];
}

export function historyItemView(item: ProduceHistoryItem): HistoryItemView {
  const view = String(item.view || "").toLowerCase();
  if (view === "victim") return "victim";
  if (view === "full_round_pov") return "full_round_pov";
  if (String(item.source_id || "").toLowerCase().startsWith("full_round_pov:")) {
    return "full_round_pov";
  }
  return "killer";
}

export function isProduceClip(item: ProduceHistoryItem): boolean {
  return (item.history_type || "produce_clip") === "produce_clip" && !!item.video_path;
}

export function groupHistoryByDemo(items: readonly ProduceHistoryItem[]): HistoryDemoGroup[] {
  const order: string[] = [];
  const byDemo = new Map<string, ProduceHistoryItem[]>();
  for (const item of items) {
    const demoPath = item.demo_path || "";
    if (!demoPath) continue;
    if (!byDemo.has(demoPath)) {
      byDemo.set(demoPath, []);
      order.push(demoPath);
    }
    byDemo.get(demoPath)!.push(item);
  }
  return order.map((demo_path) => ({ demo_path, items: byDemo.get(demo_path) || [] }));
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
    .map(([round, roundKills]) => ({ round, kills: roundKills }));
}

export function groupHistoryByRound(items: readonly ProduceHistoryItem[]): HistoryRoundGroup[] {
  const groupedByRound = new Map<string, HistoryRoundGroup>();
  for (const item of items) {
    if (historyItemView(item) === "full_round_pov") {
      const name = "pov-group";
      if (!groupedByRound.has(name)) {
        groupedByRound.set(name, { name, round: 0, kill_count: 0, items: [] });
      }
      const group = groupedByRound.get(name)!;
      group.items.push(item);
      group.kill_count += (item.kills || []).length;
      continue;
    }

    const kills = (item.kills || []).filter((kill): kill is DemoClipKill => !!kill?.id);
    const split = splitKillsByRound(kills);
    if (!split.length) split.push({ round: 0, kills: [] });
    for (const part of split) {
      const name = part.round > 0 ? `round-${part.round}` : "round-unknown";
      if (!groupedByRound.has(name)) {
        groupedByRound.set(name, { name, round: part.round, kill_count: 0, items: [] });
      }
      const group = groupedByRound.get(name)!;
      group.items.push(item);
      group.kill_count += part.kills.length;
    }
  }

  return Array.from(groupedByRound.values())
    .map((group) => {
      if (group.name !== "pov-group") return group;
      return {
        ...group,
        items: group.items.slice().sort((a, b) => Number(a.round || 0) - Number(b.round || 0)),
      };
    })
    .sort(compareRoundGroups);
}

function compareRoundGroups(a: HistoryRoundGroup, b: HistoryRoundGroup): number {
  if (a.name === "pov-group" && b.name === "pov-group") return 0;
  if (a.name === "pov-group") return -1;
  if (b.name === "pov-group") return 1;
  if (a.round <= 0 && b.round <= 0) return 0;
  if (a.round <= 0) return 1;
  if (b.round <= 0) return -1;
  return a.round - b.round;
}

export function orderHistoryByView(items: readonly ProduceHistoryItem[]): ProduceHistoryItem[] {
  const pov = sortByTick(items.filter((item) => historyItemView(item) === "full_round_pov"));
  const killer = sortByTick(items.filter((item) => historyItemView(item) === "killer"));
  const victim = sortByTick(items.filter((item) => historyItemView(item) === "victim"));
  return [...pov, ...killer, ...victim];
}

export function sortByTick(items: readonly ProduceHistoryItem[]): ProduceHistoryItem[] {
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

export function resolvePrimaryTick(item: ProduceHistoryItem): number {
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

export function historyRowKey(item: ProduceHistoryItem): string {
  return `${item.history_type || "produce_clip"}#${item.demo_path}#${item.view}#${item.spec_mode}#${item.source_id || ""}#${item.round || 0}#${item.completed_at_ms}#${item.video_path}#${(item.kill_ids || []).join("|")}`;
}

export function basename(path: string): string {
  if (!path) return "";
  return path.replaceAll("\\", "/").split("/").pop() || path;
}

export function formatTime(timestampMs: number): string {
  if (!timestampMs) return "-";
  const date = new Date(timestampMs);
  if (Number.isNaN(date.getTime())) return "-";
  return [date.getHours(), date.getMinutes(), date.getSeconds()]
    .map((part) => String(part).padStart(2, "0"))
    .join(":");
}
