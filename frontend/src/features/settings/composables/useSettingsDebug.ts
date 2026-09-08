import { reactive, ref, type Ref } from "vue";
import type { BackendApi } from "@/shared/backend";
import type { DebugPluginDLLOverrideState } from "@/shared/types";

export type SettingsDebugBackend = Pick<
  BackendApi,
  "GetDebugPluginDLLOverride" | "PickDebugPluginDLLOverride" | "ClearDebugPluginDLLOverride"
>;

export interface SettingsDebugController {
  debugPluginDLL: DebugPluginDLLOverrideState;
  pickingDebugPluginDLL: Ref<boolean>;
  clearingDebugPluginDLL: Ref<boolean>;
  errorMessage: Ref<string>;
  loadDebugPluginDLLOverride(): Promise<void>;
  pickDebugPluginDLL(): Promise<void>;
  clearDebugPluginDLL(): Promise<void>;
}

export function useSettingsDebug(
  backend: SettingsDebugBackend,
  isActive: () => boolean,
  debugEnabled: Ref<boolean>,
): SettingsDebugController {
  const debugPluginDLL = reactive<DebugPluginDLLOverrideState>({
    active: false,
    path: "",
  });
  const pickingDebugPluginDLL = ref(false);
  const clearingDebugPluginDLL = ref(false);
  const errorMessage = ref("");

  function reportError(error: unknown): void {
    errorMessage.value = error instanceof Error ? error.message : String(error);
  }

  async function loadDebugPluginDLLOverride(): Promise<void> {
    if (!isActive() || !debugEnabled.value) {
      return;
    }
    errorMessage.value = "";
    try {
      Object.assign(debugPluginDLL, await backend.GetDebugPluginDLLOverride());
    } catch (error: unknown) {
      reportError(error);
    }
  }

  async function pickDebugPluginDLL(): Promise<void> {
    if (!isActive() || !debugEnabled.value || pickingDebugPluginDLL.value) {
      return;
    }
    pickingDebugPluginDLL.value = true;
    errorMessage.value = "";
    try {
      Object.assign(debugPluginDLL, await backend.PickDebugPluginDLLOverride());
    } catch (error: unknown) {
      reportError(error);
    } finally {
      pickingDebugPluginDLL.value = false;
    }
  }

  async function clearDebugPluginDLL(): Promise<void> {
    if (!isActive() || !debugEnabled.value || clearingDebugPluginDLL.value) {
      return;
    }
    clearingDebugPluginDLL.value = true;
    errorMessage.value = "";
    try {
      Object.assign(debugPluginDLL, await backend.ClearDebugPluginDLLOverride());
    } catch (error: unknown) {
      reportError(error);
    } finally {
      clearingDebugPluginDLL.value = false;
    }
  }

  return {
    debugPluginDLL,
    pickingDebugPluginDLL,
    clearingDebugPluginDLL,
    errorMessage,
    loadDebugPluginDLLOverride,
    pickDebugPluginDLL,
    clearDebugPluginDLL,
  };
}
