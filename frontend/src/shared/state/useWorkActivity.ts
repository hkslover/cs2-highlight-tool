import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import type { WorkActivity } from "@/shared/types";
import { backend } from "@/shared/backend";

// Shared by the produce page and settings drawer. A failed or not-yet-loaded
// snapshot disables actions; backend reservations remain the final authority.
const activity = ref<WorkActivity | null>(null);
let users = 0;
let timer: ReturnType<typeof setInterval> | undefined;
let pending: Promise<void> | undefined;

function refreshWorkActivity(): Promise<void> {
  if (pending) return pending;
  pending = (async () => {
    try {
      activity.value = await backend.GetWorkActivity() as WorkActivity;
    } catch {
      activity.value = null;
    }
  })().finally(() => { pending = undefined; });
  return pending;
}

export function useWorkActivity() {
  onMounted(() => {
    if (users++ === 0) {
      activity.value = null;
      void refreshWorkActivity();
      timer = setInterval(() => { void refreshWorkActivity(); }, 500);
    }
  });
  onBeforeUnmount(() => {
    if (--users === 0) {
      clearInterval(timer);
      timer = undefined;
      activity.value = null;
    }
  });
  return {
    produceBusy: computed(() => activity.value?.produce_busy ?? true),
    storageBusy: computed(() => activity.value?.storage_busy ?? true),
    refreshWorkActivity,
  };
}
