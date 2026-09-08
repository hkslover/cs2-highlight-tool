import type {
  BackendArgs,
  BackendMethod,
  BackendResult,
  TypedBackendApi,
} from "./types.js";

type DynamicWailsMethod = (...args: unknown[]) => Promise<unknown>;
type DynamicWailsApp = Record<string, DynamicWailsMethod>;

const BACKEND_UNAVAILABLE_CODE = "BACKEND_UNAVAILABLE" as const;
const BACKEND_METHOD_UNAVAILABLE_CODE = "BACKEND_METHOD_UNAVAILABLE" as const;

/** Raised when Wails has not attached `window.go.app.App` yet. */
export class BackendUnavailableError extends Error {
  readonly code = BACKEND_UNAVAILABLE_CODE;

  constructor() {
    super("Wails backend API is not loaded");
    this.name = "BackendUnavailableError";
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

/** Raised when a generated App method is missing from the attached object. */
export class BackendMethodUnavailableError extends Error {
  readonly code = BACKEND_METHOD_UNAVAILABLE_CODE;
  readonly method: BackendMethod;

  constructor(method: BackendMethod) {
    super(`Wails backend method is not loaded: ${method}`);
    this.name = "BackendMethodUnavailableError";
    this.method = method;
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

function getDynamicWailsApp(): DynamicWailsApp | undefined {
  if (typeof window === "undefined") {
    return undefined;
  }
  return window.go?.app?.App as DynamicWailsApp | undefined;
}

function requireDynamicWailsApp(): DynamicWailsApp {
  const app = getDynamicWailsApp();
  if (!app) {
    throw new BackendUnavailableError();
  }
  return app;
}

/**
 * Call one of the known Wails App methods through the live dynamic boundary.
 * The method key determines both the argument tuple and the resolved value.
 */
export async function callBackend<M extends BackendMethod>(
  method: M,
  ...args: BackendArgs<M>
): Promise<BackendResult<M>> {
  const app = requireDynamicWailsApp();
  const handler = app[method];
  if (typeof handler !== "function") {
    throw new BackendMethodUnavailableError(method);
  }

  return (await handler(...(args as unknown[]))) as BackendResult<M>;
}

function bindBackendMethod<M extends BackendMethod>(
  method: M,
): (...args: BackendArgs<M>) => Promise<BackendResult<M>> {
  return (...args: BackendArgs<M>) => callBackend(method, ...args);
}

/**
 * Named, typed access to every method in the App contract. Keeping this map
 * here lets feature code depend on `@/shared/backend` without importing an
 * unrelated feature's local helper.
 */
export const backend: TypedBackendApi = {
  AckChangelog: bindBackendMethod("AckChangelog"),
  CancelStartupDownload: bindBackendMethod("CancelStartupDownload"),
  CheckPlatformClients: bindBackendMethod("CheckPlatformClients"),
  ClearDebugPluginDLLOverride: bindBackendMethod("ClearDebugPluginDLLOverride"),
  ClearDemoDirectory: bindBackendMethod("ClearDemoDirectory"),
  ClearOutputsDirectory: bindBackendMethod("ClearOutputsDirectory"),
  ConcatEditClips: bindBackendMethod("ConcatEditClips"),
  EnterMainApp: bindBackendMethod("EnterMainApp"),
  ExitApp: bindBackendMethod("ExitApp"),
  ExportProduceHistoryVideos: bindBackendMethod("ExportProduceHistoryVideos"),
  ExportProduceWSLogs: bindBackendMethod("ExportProduceWSLogs"),
  ExportStartupLogs: bindBackendMethod("ExportStartupLogs"),
  GeneratePluginJSON: bindBackendMethod("GeneratePluginJSON"),
  GeneratePluginJSONBatch: bindBackendMethod("GeneratePluginJSONBatch"),
  GeneratePluginJSONBatchAndLaunchHLAE: bindBackendMethod(
    "GeneratePluginJSONBatchAndLaunchHLAE",
  ),
  GetClipActionSettings: bindBackendMethod("GetClipActionSettings"),
  GetClipSettings: bindBackendMethod("GetClipSettings"),
  GetDebugPluginDLLOverride: bindBackendMethod("GetDebugPluginDLLOverride"),
  GetDemoStorageStats: bindBackendMethod("GetDemoStorageStats"),
  GetFiveEPlayerName: bindBackendMethod("GetFiveEPlayerName"),
  GetGameInfoHealth: bindBackendMethod("GetGameInfoHealth"),
  GetOutputsStorageStats: bindBackendMethod("GetOutputsStorageStats"),
  GetPendingChangelog: bindBackendMethod("GetPendingChangelog"),
  GetProduceHistorySnapshot: bindBackendMethod("GetProduceHistorySnapshot"),
  GetProduceQueueState: bindBackendMethod("GetProduceQueueState"),
  GetProduceTakeFiles: bindBackendMethod("GetProduceTakeFiles"),
  GetProduceTakeSnapshot: bindBackendMethod("GetProduceTakeSnapshot"),
  GetProduceWSState: bindBackendMethod("GetProduceWSState"),
  GetStartupState: bindBackendMethod("GetStartupState"),
  GetWorkActivity: bindBackendMethod("GetWorkActivity"),
  GetWorkspaceState: bindBackendMethod("GetWorkspaceState"),
  ImportFiveEMatch: bindBackendMethod("ImportFiveEMatch"),
  ImportManualDownload: bindBackendMethod("ImportManualDownload"),
  ImportWanmeiMatch: bindBackendMethod("ImportWanmeiMatch"),
  ListFiveERecentMatches: bindBackendMethod("ListFiveERecentMatches"),
  ListWanmeiRecentMatches: bindBackendMethod("ListWanmeiRecentMatches"),
  OpenDemoDirectory: bindBackendMethod("OpenDemoDirectory"),
  OpenExternalURL: bindBackendMethod("OpenExternalURL"),
  OpenManualDownload: bindBackendMethod("OpenManualDownload"),
  OpenOutputsDirectory: bindBackendMethod("OpenOutputsDirectory"),
  OpenProducedClipInFolder: bindBackendMethod("OpenProducedClipInFolder"),
  ParseDemoFile: bindBackendMethod("ParseDemoFile"),
  PickCS2Path: bindBackendMethod("PickCS2Path"),
  PickDebugPluginDLLOverride: bindBackendMethod("PickDebugPluginDLLOverride"),
  PickDemoFiles: bindBackendMethod("PickDemoFiles"),
  PickRecordOutputDir: bindBackendMethod("PickRecordOutputDir"),
  PickWorkspaceDir: bindBackendMethod("PickWorkspaceDir"),
  PreviewFullRoundPOV: bindBackendMethod("PreviewFullRoundPOV"),
  ProbeClipDuration: bindBackendMethod("ProbeClipDuration"),
  ReinstallStartupComponent: bindBackendMethod("ReinstallStartupComponent"),
  RepairGameInfo: bindBackendMethod("RepairGameInfo"),
  RequestClosePlatformClient: bindBackendMethod("RequestClosePlatformClient"),
  ResetWorkspace: bindBackendMethod("ResetWorkspace"),
  RetryStartupComponent: bindBackendMethod("RetryStartupComponent"),
  RunStartupChecks: bindBackendMethod("RunStartupChecks"),
  SaveClipActionSettings: bindBackendMethod("SaveClipActionSettings"),
  SaveClipSettings: bindBackendMethod("SaveClipSettings"),
  SetWorkspaceDir: bindBackendMethod("SetWorkspaceDir"),
  ValidateWorkspaceDir: bindBackendMethod("ValidateWorkspaceDir"),
};

/** Compile-time-only alias for callers that need the full adapter contract. */
export type { BackendApi } from "./types.js";
