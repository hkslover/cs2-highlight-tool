import { initProductionState, useProductionState } from "@/domains/production/productionStore";

/**
 * Page-facing adapter for the app-level production store. It intentionally
 * has no unmount hook: subscriptions belong to the app shell and must survive
 * navigation between import, produce, edit, and settings.
 */
export function useProduceStateSync() {
  const state = useProductionState();

  async function initialize(): Promise<void> {
    await initProductionState();
  }

  return {
    ...state,
    initialize,
  };
}
