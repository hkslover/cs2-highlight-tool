import { computed, readonly, ref } from "vue";
import type { DemoListEntry, DemoMetadata } from "@/shared/types";
import {
  clearDemoSelectionState,
  syncDefaultFullRoundPlayer,
  syncDefaultPlayerForRole,
} from "@/domains/clip-selection";
import { backend } from "@/shared/backend";

const demoListState = ref<DemoListEntry[]>([]);
const selectedIndexState = ref<number | null>(null);
const detailCollapsedState = ref(true);

/** Read-only demo directory state; use actions below for every mutation. */
// A computed shallow copy is a read-only state boundary while keeping the
// legacy `DemoListEntry[]` prop shape for import list components. Mutating the
// returned array cannot mutate the store; entries are replaced through actions.
export const demoList = computed(() => demoListState.value.slice());
export const selectedIndex = readonly(selectedIndexState);
export const detailCollapsed = readonly(detailCollapsedState);

let keyCounter = 0;

export const selectedEntry = computed<DemoListEntry | null>(() => {
  const index = selectedIndexState.value;
  if (index == null || index < 0 || index >= demoListState.value.length) return null;
  return demoListState.value[index];
});

export const selectedDemo = computed<DemoMetadata | null>(() => selectedEntry.value?.meta ?? null);
export const canSelectPrev = computed(
  () => selectedIndexState.value != null && selectedIndexState.value > 0,
);
export const canSelectNext = computed(
  () =>
    selectedIndexState.value != null &&
    selectedIndexState.value < demoListState.value.length - 1,
);
export const clipReadyDemos = computed(() =>
  demoListState.value.filter(
    (entry) =>
      (entry.meta?.clip_players?.length ?? 0) > 0 ||
      (entry.meta?.players?.length ?? 0) > 0,
  ),
);

function replaceEntry(key: string, next: DemoListEntry): boolean {
  const index = demoListState.value.findIndex((entry) => entry.key === key);
  if (index < 0) return false;
  const nextEntries = demoListState.value.slice();
  nextEntries[index] = next;
  demoListState.value = nextEntries;
  return true;
}

/**
 * Adds files and parses them. Defaulting is intentionally performed once the
 * parse action has produced metadata, instead of from a getter used by UI.
 */
export function onDemosSelected(paths: string[]): void {
  const newEntries: DemoListEntry[] = [];
  for (const filePath of paths) {
    if (demoListState.value.some((entry) => entry.file_path === filePath)) continue;
    newEntries.push({
      key: `demo-${++keyCounter}`,
      file_path: filePath,
      file_name: basename(filePath),
      loading: true,
    });
  }
  if (!newEntries.length) return;

  const firstNewIndex = demoListState.value.length;
  demoListState.value = demoListState.value.concat(newEntries);
  if (selectedIndexState.value == null) {
    selectedIndexState.value = firstNewIndex;
  }

  for (const entry of newEntries) {
    void parseEntry(entry);
  }
}

async function parseEntry(entry: DemoListEntry): Promise<void> {
  try {
    const meta = await backend.ParseDemoFile(entry.file_path);
    const parsedEntry: DemoListEntry = {
      ...entry,
      loading: false,
      meta: meta ?? undefined,
    };
    if (!replaceEntry(entry.key, parsedEntry)) return;

    // Explicit action boundary: getters never initialize a player.
    syncDefaultPlayerForRole(parsedEntry);
    syncDefaultFullRoundPlayer(parsedEntry);
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : String(error);
    replaceEntry(entry.key, { ...entry, loading: false, error: message });
  }
}

export function removeDemoAt(index: number): void {
  const removed = demoListState.value[index];
  if (!removed) return;
  demoListState.value = demoListState.value.filter((_entry, entryIndex) => entryIndex !== index);
  clearDemoSelectionState(removed.key);

  if (selectedIndexState.value === index) {
    selectedIndexState.value =
      demoListState.value.length > 0 ? Math.min(index, demoListState.value.length - 1) : null;
  } else if (selectedIndexState.value != null && selectedIndexState.value > index) {
    selectedIndexState.value -= 1;
  }
}

export function toggleSelected(index: number): void {
  selectedIndexState.value = selectedIndexState.value === index ? null : index;
}

export function selectPrevDemo(): void {
  if (!canSelectPrev.value || selectedIndexState.value == null) return;
  selectedIndexState.value -= 1;
}

export function selectNextDemo(): void {
  if (!canSelectNext.value || selectedIndexState.value == null) return;
  selectedIndexState.value += 1;
}

export function toggleDetailCollapsed(): void {
  detailCollapsedState.value = !detailCollapsedState.value;
}

export function selectDemoByKey(key: string): void {
  const index = demoListState.value.findIndex((entry) => entry.key === key);
  if (index >= 0) selectedIndexState.value = index;
}

export function ensureClipDemoSelected(): DemoListEntry | null {
  const current = selectedEntry.value;
  if (
    current &&
    ((current.meta?.clip_players?.length ?? 0) > 0 ||
      (current.meta?.players?.length ?? 0) > 0)
  ) {
    return current;
  }
  const fallback = clipReadyDemos.value[0] ?? null;
  if (!fallback) return null;
  selectDemoByKey(fallback.key);
  return fallback;
}

function basename(filePath: string): string {
  const match = /[^\\/]+$/.exec(filePath);
  return match ? match[0] : filePath;
}
