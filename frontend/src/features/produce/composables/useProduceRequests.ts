import { backend } from "@/shared/backend";
import type {
  GeneratePluginJSONBatchRequest,
  GeneratePluginJSONBatchResult,
} from "@/shared/types";

export function useProduceRequests() {
  return {
    generateBatch(request: GeneratePluginJSONBatchRequest): Promise<GeneratePluginJSONBatchResult> {
      return backend.GeneratePluginJSONBatch(request);
    },

    generateBatchAndLaunch(
      request: GeneratePluginJSONBatchRequest,
    ): Promise<GeneratePluginJSONBatchResult> {
      return backend.GeneratePluginJSONBatchAndLaunchHLAE(request);
    },

    openProducedClip(videoPath: string): Promise<void> {
      return backend.OpenProducedClipInFolder(videoPath);
    },

    exportProduceWSLogs(): Promise<string> {
      return backend.ExportProduceWSLogs();
    },
  };
}
