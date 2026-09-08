import { ref, type Ref } from "vue";
import { useMessage } from "naive-ui";
import { t } from "@/shared/i18n";
import type {
  DemoClipKill,
  GeneratePluginJSONBatchResult,
  GeneratePluginJSONRequest,
} from "@/shared/types";
import { usePlatformClientCheck } from "@/features/produce/composables/usePlatformClientCheck";
import { useProduceRequests } from "@/features/produce/composables/useProduceRequests";

interface ProduceLaunchOptions {
  buildJobs: () => GeneratePluginJSONRequest[];
  produceBusy: Readonly<Ref<boolean>>;
  keepProduceIntermediates: Readonly<Ref<boolean>>;
  batchResult: Ref<GeneratePluginJSONBatchResult | null>;
  launchViewEnabled: Ref<boolean>;
  errorMessage: Ref<string>;
  killSnapshotByDemo: Ref<Record<string, DemoClipKill[]>>;
  captureCurrentKillSnapshot: () => void;
  refreshWorkActivity: () => Promise<void>;
}

/** Owns the two launch flows and their platform-client confirmation gate. */
export function useProduceLaunch(options: ProduceLaunchOptions) {
  const message = useMessage();
  const platformCheck = usePlatformClientCheck();
  const requests = useProduceRequests();

  const generatingAndLaunching = ref(false);
  const generatingConfigOnlyLoading = ref(false);
  const showPlatformCheckModal = ref(false);

  async function doGenerateAndLaunch(jobs: GeneratePluginJSONRequest[]): Promise<void> {
    try {
      if (!jobs.length) return;
      options.launchViewEnabled.value = true;
      options.errorMessage.value = "";

      const previousKillSnapshot = options.killSnapshotByDemo.value;
      options.captureCurrentKillSnapshot();
      const request = {
        jobs,
        debug: {
          keep_intermediate_files: options.keepProduceIntermediates.value,
        },
      };
      const result = await requests.generateBatchAndLaunch(request);
      // A backend busy response is side-effect free. Keep the active session's
      // planned rows instead of replacing them with an empty result list.
      if (
        !result.launch_started &&
        result.launch_error &&
        result.results.length === 0 &&
        options.batchResult.value?.launch_started
      ) {
        options.killSnapshotByDemo.value = previousKillSnapshot;
        options.errorMessage.value = result.launch_error;
        return;
      }
      options.batchResult.value = result;
      if (!result.launch_started && result.launch_error) {
        options.errorMessage.value = result.launch_error;
      }
    } catch (error: unknown) {
      options.errorMessage.value = error instanceof Error ? error.message : String(error);
    }
  }

  async function generateAndLaunch(): Promise<void> {
    // Reserve the interaction before the asynchronous platform check so two
    // fast clicks cannot submit duplicate launch requests.
    if (generatingAndLaunching.value || options.produceBusy.value) return;
    const jobs = options.buildJobs();
    if (!jobs.length) return;

    generatingAndLaunching.value = true;
    try {
      const allClosed = await platformCheck.checkAll();
      if (!allClosed) {
        showPlatformCheckModal.value = true;
        return;
      }
      await doGenerateAndLaunch(jobs);
    } finally {
      await options.refreshWorkActivity();
      generatingAndLaunching.value = false;
    }
  }

  async function onPlatformCheckConfirmed(): Promise<void> {
    if (generatingAndLaunching.value || options.produceBusy.value) return;
    showPlatformCheckModal.value = false;
    platformCheck.reset();
    const jobs = options.buildJobs();
    if (!jobs.length) return;
    generatingAndLaunching.value = true;
    try {
      await doGenerateAndLaunch(jobs);
    } finally {
      await options.refreshWorkActivity();
      generatingAndLaunching.value = false;
    }
  }

  function onPlatformCheckCancelled(): void {
    showPlatformCheckModal.value = false;
    platformCheck.reset();
  }

  async function generateConfigOnly(): Promise<void> {
    if (options.produceBusy.value) return;
    try {
      const jobs = options.buildJobs();
      if (!jobs.length) return;
      generatingConfigOnlyLoading.value = true;
      options.errorMessage.value = "";

      const result = await requests.generateBatch({ jobs });
      const summary = t("main.produce.batch_summary", {
        success: result.success_count,
        failed: result.failure_count,
      });
      if (result.success_count > 0 && result.failure_count === 0) {
        message.success(summary);
        return;
      }
      if (result.success_count > 0) {
        message.warning(summary);
        return;
      }
      message.error(summary);
    } catch (error: unknown) {
      const detail = error instanceof Error ? error.message : String(error);
      options.errorMessage.value = detail;
      message.error(detail);
    } finally {
      generatingConfigOnlyLoading.value = false;
    }
  }

  return {
    generatingAndLaunching,
    generatingConfigOnlyLoading,
    showPlatformCheckModal,
    generateAndLaunch,
    generateConfigOnly,
    onPlatformCheckConfirmed,
    onPlatformCheckCancelled,
  };
}
