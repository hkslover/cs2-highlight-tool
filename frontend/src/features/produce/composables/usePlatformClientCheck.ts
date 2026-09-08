import { computed, ref } from "vue";
import type { PlatformClientStatus } from "@/shared/types";
import { backend } from "@/shared/backend";

const statuses = ref<PlatformClientStatus[]>([]);
const refreshing = ref(false);

export function usePlatformClientCheck() {
  const allClosed = computed(() => statuses.value.every((s) => !s.running));
  const anyRunning = computed(() => statuses.value.some((s) => s.running));

  async function checkAll(): Promise<boolean> {
    try {
      const result = await backend.CheckPlatformClients();
      statuses.value = result;
      return result.every((s) => !s.running);
    } catch {
      return false;
    }
  }

  async function refresh(): Promise<void> {
    refreshing.value = true;
    try {
      await checkAll();
    } finally {
      refreshing.value = false;
    }
  }

  function reset(): void {
    statuses.value = [];
    refreshing.value = false;
  }

  return {
    statuses,
    allClosed,
    anyRunning,
    refreshing,
    checkAll,
    refresh,
    reset,
  };
}
