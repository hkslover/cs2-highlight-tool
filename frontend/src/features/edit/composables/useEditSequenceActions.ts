import { ref } from "vue";
import type { ProduceHistoryItem } from "@/shared/types";
import { useEditDomain } from "@/domains/edit";
import { backend } from "@/shared/backend";

export interface EditSequenceBatchResult {
  added: number;
  failed: number;
  firstError: string;
}

export interface EditSequenceActions {
  durationCache: ReturnType<typeof ref<Record<string, number>>>;
  addingByPath: ReturnType<typeof ref<Record<string, boolean>>>;
  isAdding(videoPath: string): boolean;
  addFromHistory(item: ProduceHistoryItem): Promise<boolean>;
  addMany(items: readonly ProduceHistoryItem[]): Promise<EditSequenceBatchResult>;
}

// Duration probes are keyed by the exact backend path and survive an edit
// route remount, so navigating away during an export does not cause a second
// probe when the source panel is shown again.
const sharedDurationCache = ref<Record<string, number>>({});
let durationCacheGeneration = 0;

export function resetEditSequenceDurationCache(): void {
  durationCacheGeneration += 1;
  sharedDurationCache.value = {};
}

export function useEditSequenceActions(): EditSequenceActions {
  const domain = useEditDomain();
  const durationCache = sharedDurationCache;
  const addingByPath = ref<Record<string, boolean>>({});

  function isAdding(videoPath: string): boolean {
    return !!addingByPath.value[videoPath || ""];
  }

  function setAdding(videoPath: string, value: boolean) {
    const key = videoPath || "";
    if (!key) return;
    const next = { ...addingByPath.value };
    if (value) next[key] = true;
    else delete next[key];
    addingByPath.value = next;
  }

  async function getDuration(videoPath: string, generation: number): Promise<number> {
    const cached = durationCache.value[videoPath];
    if (cached > 0) return cached;
    const duration = await backend.ProbeClipDuration(videoPath);
    if (!(duration > 0)) {
      throw new Error("ProbeClipDuration returned an invalid duration");
    }
    if (generation === durationCacheGeneration) {
      durationCache.value = { ...durationCache.value, [videoPath]: duration };
    }
    return duration;
  }

  async function addFromHistory(item: ProduceHistoryItem): Promise<boolean> {
    const videoPath = String(item.video_path || "").trim();
    if (!videoPath || isAdding(videoPath)) return false;
    const generation = durationCacheGeneration;
    setAdding(videoPath, true);
    try {
      const duration = await getDuration(videoPath, generation);
      if (generation !== durationCacheGeneration) return false;
      return domain.addSequenceItem(item, duration);
    } finally {
      setAdding(videoPath, false);
    }
  }

  async function addMany(items: readonly ProduceHistoryItem[]): Promise<EditSequenceBatchResult> {
    let added = 0;
    let failed = 0;
    let firstError = "";
    const generation = durationCacheGeneration;
    for (const item of items) {
      if (domain.exporting.value || generation !== durationCacheGeneration) break;
      const videoPath = String(item.video_path || "").trim();
      if (!videoPath) {
        failed++;
        if (!firstError) firstError = "History item has no video path";
        continue;
      }
      try {
        const duration = await getDuration(videoPath, generation);
        if (generation !== durationCacheGeneration) break;
        if (domain.addSequenceItem(item, duration)) added++;
        else break;
      } catch (err: unknown) {
        failed++;
        if (!firstError) firstError = err instanceof Error ? err.message : String(err);
      }
    }
    return { added, failed, firstError };
  }

  return {
    durationCache,
    addingByPath,
    isAdding,
    addFromHistory,
    addMany,
  };
}
