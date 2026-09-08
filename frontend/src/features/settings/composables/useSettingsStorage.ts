import { reactive, ref, type Ref } from "vue";
import type { BackendApi } from "@/shared/backend";
import type { DemoStorageStats, OutputsStorageStats } from "@/shared/types";

export type SettingsStorageBackend = Pick<
  BackendApi,
  | "GetOutputsStorageStats"
  | "GetDemoStorageStats"
  | "OpenOutputsDirectory"
  | "OpenDemoDirectory"
  | "ClearOutputsDirectory"
  | "ClearDemoDirectory"
>;

export interface SettingsStorageController {
  outputsStats: OutputsStorageStats;
  demoStats: DemoStorageStats;
  outputsLoading: Ref<boolean>;
  demoLoading: Ref<boolean>;
  openingOutputsDir: Ref<boolean>;
  openingDemoDir: Ref<boolean>;
  clearingOutputs: Ref<boolean>;
  clearingDemo: Ref<boolean>;
  errorMessage: Ref<string>;
  loadOutputsStats(): Promise<void>;
  loadDemoStats(): Promise<void>;
  openOutputsDirectory(): Promise<void>;
  openDemoDirectory(): Promise<void>;
  clearOutputsDirectory(): Promise<void>;
  clearDemoDirectory(): Promise<void>;
}

export function useSettingsStorage(
  backend: SettingsStorageBackend,
  isActive: () => boolean,
  isStorageBusy: () => boolean,
): SettingsStorageController {
  const outputsStats = reactive<OutputsStorageStats>({
    output_dir: "",
    video_count: 0,
    total_size_bytes: 0,
  });
  const demoStats = reactive<DemoStorageStats>({
    demo_dir: "",
    demo_count: 0,
    total_size_bytes: 0,
  });
  const outputsLoading = ref(false);
  const demoLoading = ref(false);
  const openingOutputsDir = ref(false);
  const openingDemoDir = ref(false);
  const clearingOutputs = ref(false);
  const clearingDemo = ref(false);
  const errorMessage = ref("");

  function reportError(error: unknown): void {
    errorMessage.value = error instanceof Error ? error.message : String(error);
  }

  async function loadOutputsStats(): Promise<void> {
    if (!isActive() || outputsLoading.value) {
      return;
    }
    outputsLoading.value = true;
    errorMessage.value = "";
    try {
      Object.assign(outputsStats, await backend.GetOutputsStorageStats());
    } catch (error: unknown) {
      reportError(error);
    } finally {
      outputsLoading.value = false;
    }
  }

  async function loadDemoStats(): Promise<void> {
    if (!isActive() || demoLoading.value) {
      return;
    }
    demoLoading.value = true;
    errorMessage.value = "";
    try {
      Object.assign(demoStats, await backend.GetDemoStorageStats());
    } catch (error: unknown) {
      reportError(error);
    } finally {
      demoLoading.value = false;
    }
  }

  async function openOutputsDirectory(): Promise<void> {
    if (!isActive() || openingOutputsDir.value) {
      return;
    }
    openingOutputsDir.value = true;
    errorMessage.value = "";
    try {
      await backend.OpenOutputsDirectory();
    } catch (error: unknown) {
      reportError(error);
    } finally {
      openingOutputsDir.value = false;
    }
  }

  async function openDemoDirectory(): Promise<void> {
    if (!isActive() || openingDemoDir.value) {
      return;
    }
    openingDemoDir.value = true;
    errorMessage.value = "";
    try {
      await backend.OpenDemoDirectory();
    } catch (error: unknown) {
      reportError(error);
    } finally {
      openingDemoDir.value = false;
    }
  }

  async function clearOutputsDirectory(): Promise<void> {
    if (!isActive() || clearingOutputs.value || isStorageBusy()) {
      return;
    }
    clearingOutputs.value = true;
    errorMessage.value = "";
    try {
      Object.assign(outputsStats, await backend.ClearOutputsDirectory());
    } catch (error: unknown) {
      reportError(error);
    } finally {
      clearingOutputs.value = false;
    }
  }

  async function clearDemoDirectory(): Promise<void> {
    if (!isActive() || clearingDemo.value || isStorageBusy()) {
      return;
    }
    clearingDemo.value = true;
    errorMessage.value = "";
    try {
      Object.assign(demoStats, await backend.ClearDemoDirectory());
    } catch (error: unknown) {
      reportError(error);
    } finally {
      clearingDemo.value = false;
    }
  }

  return {
    outputsStats,
    demoStats,
    outputsLoading,
    demoLoading,
    openingOutputsDir,
    openingDemoDir,
    clearingOutputs,
    clearingDemo,
    errorMessage,
    loadOutputsStats,
    loadDemoStats,
    openOutputsDirectory,
    openDemoDirectory,
    clearOutputsDirectory,
    clearDemoDirectory,
  };
}
