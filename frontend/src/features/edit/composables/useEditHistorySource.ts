import { computed, ref, watch } from "vue";
import { useProductionState } from "@/domains/production";
import {
  groupHistoryByDemo,
  groupHistoryByRound,
  isProduceClip,
  type HistoryDemoGroup,
  type HistoryRoundGroup,
} from "@/domains/edit/history";
import type { ProduceHistoryItem } from "@/shared/types";

export function useEditHistorySource() {
  const { historySnapshot } = useProductionState();
  const sourceDemoExpanded = ref<string[]>([]);
  const sourceDemoExpandedInitialized = ref(false);
  const sourceRoundExpandedByDemo = ref<Record<string, string[]>>({});

  const produceClipItems = computed(() =>
    [...(historySnapshot.value.items || [])]
      .filter(isProduceClip)
      .sort((a, b) => (b.completed_at_ms || 0) - (a.completed_at_ms || 0)),
  );

  const produceClipsByDemo = computed<HistoryDemoGroup[]>(() =>
    groupHistoryByDemo(produceClipItems.value),
  );

  const sourceRoundGroupsByDemo = computed(() => {
    const next = new Map<string, HistoryRoundGroup[]>();
    for (const demoGroup of produceClipsByDemo.value) {
      next.set(demoGroup.demo_path, groupHistoryByRound(demoGroup.items));
    }
    return next;
  });

  watch(
    () => produceClipsByDemo.value.map((group) => group.demo_path),
    (demoPaths) => {
      if (!sourceDemoExpandedInitialized.value) {
        sourceDemoExpanded.value = [...demoPaths];
        sourceDemoExpandedInitialized.value = true;
        return;
      }
      const allowed = new Set(demoPaths);
      const existing = new Set(sourceDemoExpanded.value);
      const pruned = sourceDemoExpanded.value.filter((name) => allowed.has(name));
      const newPaths = demoPaths.filter((name) => !existing.has(name));
      sourceDemoExpanded.value = [...pruned, ...newPaths];
    },
    { immediate: true },
  );

  function getSourceDemoExpanded(): string[] {
    const allowed = new Set(produceClipsByDemo.value.map((group) => group.demo_path));
    return sourceDemoExpanded.value.filter((name) => allowed.has(name));
  }

  function handleSourceDemoExpanded(names: Array<string | number> | string | number | null) {
    sourceDemoExpanded.value = normalizeNames(names);
  }

  function getSourceRoundExpanded(demoPath: string): string[] {
    const groups = sourceRoundGroupsByDemo.value.get(demoPath) || [];
    const defaults = groups.map((group) => group.name);
    if (!Object.prototype.hasOwnProperty.call(sourceRoundExpandedByDemo.value, demoPath)) {
      return defaults;
    }
    const current = sourceRoundExpandedByDemo.value[demoPath] || [];
    const allowed = new Set(defaults);
    return current.filter((name) => allowed.has(name));
  }

  function handleSourceRoundExpanded(
    demoPath: string,
    names: Array<string | number> | string | number | null,
  ) {
    sourceRoundExpandedByDemo.value = {
      ...sourceRoundExpandedByDemo.value,
      [demoPath]: normalizeNames(names),
    };
  }

  function sourceRoundGroupsForDemo(demoPath: string): HistoryRoundGroup[] {
    return sourceRoundGroupsByDemo.value.get(demoPath) || [];
  }

  return {
    produceClipItems,
    produceClipsByDemo,
    getSourceDemoExpanded,
    handleSourceDemoExpanded,
    getSourceRoundExpanded,
    handleSourceRoundExpanded,
    sourceRoundGroupsForDemo,
  };
}

function normalizeNames(names: Array<string | number> | string | number | null): string[] {
  if (Array.isArray(names)) return names.map((name) => String(name));
  if (names == null) return [];
  return [String(names)];
}

export type { HistoryDemoGroup, HistoryRoundGroup };
export type { ProduceHistoryItem };
