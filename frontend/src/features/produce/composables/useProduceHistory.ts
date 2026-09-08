import {
  initProduceHistory,
  useProductionState,
} from "@/domains/production/productionStore";

/** Kept for existing TopBar, clips, edit, and history drawer callers. */
export async function ensureProduceHistoryInitialized(): Promise<void> {
  try {
    await initProduceHistory();
  } catch {
    // Keep the compatibility helper non-fatal. The domain store subscribes
    // before its initial read and retains that subscription for a later retry.
  }
}

export function useProduceHistory() {
  const { historySnapshot } = useProductionState();
  return {
    historySnapshot,
  };
}
