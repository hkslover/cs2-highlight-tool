import type {
  ClipSettings,
  DebugPluginDLLOverrideState,
  DemoStorageStats,
  FullRoundPOVPlan,
  GeneratePluginJSONBatchRequest,
  GeneratePluginJSONBatchResult,
  GeneratePluginJSONRequest,
  GeneratePluginJSONResult,
  OutputsStorageStats,
  PlatformClientCloseResult,
  PlatformClientStatus,
  ProduceTakeFileSnapshot,
  ProduceTakeStatusSnapshot,
  ProduceHistorySnapshot,
  ProduceQueueState,
  ProduceWSState,
} from "../types";
import type { DemoMetadata } from "../types/demo";
import type {
  ProduceHistoryExportResult,
} from "../types/edit";
import type { PendingChangelog } from "../types/changelog";
import type {
  FiveEMatchListResult,
  WanmeiMatchListResult,
} from "../types/import";
import type {
  StartupState,
  WorkspaceState,
} from "../types/startup";
import type {
  GameInfoHealth,
  WorkActivity,
} from "../types/app";

/** The request shape generated for `ConcatEditClips` by the Go app package. */
export interface EditConcatClip {
  video_path: string;
  duration: number;
}

/** The transition shape generated for `ConcatEditClips` by the Go app package. */
export interface EditConcatTransition {
  type: string;
  duration: number;
  after_index?: number;
}

export interface EditConcatRequest {
  clips: EditConcatClip[];
  transitions: EditConcatTransition[];
}

/** The request shape generated for `SaveClipActionSettings`. */
export interface ClipActionSettings {
  enable_voice_indices: boolean;
  voice_indices_value: number;
  enable_voice_indices_h: boolean;
  voice_indices_h_value: number;
}

/** The response shape generated for `ValidateWorkspaceDir`. */
export interface WorkspaceValidateResult {
  ok: boolean;
  errorMessage: string;
}

/**
 * The typed surface of the Wails `app.App` binding.
 *
 * Keep this map aligned with `frontend/wailsjs/go/app/App.d.ts`. It describes
 * the JSON shapes consumed by the frontend, so it uses the hand-written shared
 * types rather than importing generated binding implementations.
 */
export interface BackendApi {
  AckChangelog: (version: string) => Promise<void>;
  CancelStartupDownload: (componentID: string) => Promise<StartupState>;
  CheckPlatformClients: () => Promise<PlatformClientStatus[]>;
  ClearDebugPluginDLLOverride: () => Promise<DebugPluginDLLOverrideState>;
  ClearDemoDirectory: () => Promise<DemoStorageStats>;
  ClearOutputsDirectory: () => Promise<OutputsStorageStats>;
  ConcatEditClips: (request: EditConcatRequest) => Promise<string>;
  EnterMainApp: () => Promise<void>;
  ExitApp: () => Promise<void>;
  ExportProduceHistoryVideos: () => Promise<ProduceHistoryExportResult>;
  ExportProduceWSLogs: () => Promise<string>;
  ExportStartupLogs: () => Promise<string>;
  GeneratePluginJSON: (
    request: GeneratePluginJSONRequest,
  ) => Promise<GeneratePluginJSONResult>;
  GeneratePluginJSONBatch: (
    request: GeneratePluginJSONBatchRequest,
  ) => Promise<GeneratePluginJSONBatchResult>;
  GeneratePluginJSONBatchAndLaunchHLAE: (
    request: GeneratePluginJSONBatchRequest,
  ) => Promise<GeneratePluginJSONBatchResult>;
  GetClipActionSettings: () => Promise<ClipActionSettings>;
  GetClipSettings: () => Promise<ClipSettings>;
  GetDebugPluginDLLOverride: () => Promise<DebugPluginDLLOverrideState>;
  GetDemoStorageStats: () => Promise<DemoStorageStats>;
  GetFiveEPlayerName: () => Promise<string>;
  GetGameInfoHealth: () => Promise<GameInfoHealth>;
  GetOutputsStorageStats: () => Promise<OutputsStorageStats>;
  GetPendingChangelog: () => Promise<PendingChangelog>;
  GetProduceHistorySnapshot: () => Promise<ProduceHistorySnapshot>;
  GetProduceQueueState: () => Promise<ProduceQueueState>;
  GetProduceTakeFiles: () => Promise<ProduceTakeFileSnapshot>;
  GetProduceTakeSnapshot: () => Promise<ProduceTakeStatusSnapshot>;
  GetProduceWSState: () => Promise<ProduceWSState>;
  GetStartupState: () => Promise<StartupState>;
  GetWorkActivity: () => Promise<WorkActivity>;
  GetWorkspaceState: () => Promise<WorkspaceState>;
  ImportFiveEMatch: (matchID: string) => Promise<string[]>;
  ImportManualDownload: (componentID: string) => Promise<StartupState>;
  ImportWanmeiMatch: (matchID: string) => Promise<string[]>;
  ListFiveERecentMatches: (
    playerName: string,
    page: number,
  ) => Promise<FiveEMatchListResult>;
  ListWanmeiRecentMatches: (page: number) => Promise<WanmeiMatchListResult>;
  OpenDemoDirectory: () => Promise<void>;
  OpenExternalURL: (rawURL: string) => Promise<void>;
  OpenManualDownload: (componentID: string) => Promise<void>;
  OpenOutputsDirectory: () => Promise<void>;
  OpenProducedClipInFolder: (videoPath: string) => Promise<void>;
  ParseDemoFile: (path: string) => Promise<DemoMetadata>;
  PickCS2Path: () => Promise<StartupState>;
  PickDebugPluginDLLOverride: () => Promise<DebugPluginDLLOverrideState>;
  PickDemoFiles: () => Promise<string[]>;
  PickRecordOutputDir: () => Promise<string>;
  PickWorkspaceDir: () => Promise<string>;
  PreviewFullRoundPOV: (
    demoPath: string,
    playerSteamID: string,
  ) => Promise<FullRoundPOVPlan>;
  ProbeClipDuration: (videoPath: string) => Promise<number>;
  ReinstallStartupComponent: (componentID: string) => Promise<StartupState>;
  RepairGameInfo: () => Promise<GameInfoHealth>;
  RequestClosePlatformClient: (
    exeName: string,
  ) => Promise<PlatformClientCloseResult>;
  ResetWorkspace: () => Promise<void>;
  RetryStartupComponent: (componentID: string) => Promise<StartupState>;
  RunStartupChecks: () => Promise<StartupState>;
  SaveClipActionSettings: (
    settings: ClipActionSettings,
  ) => Promise<ClipActionSettings>;
  SaveClipSettings: (settings: ClipSettings) => Promise<ClipSettings>;
  SetWorkspaceDir: (path: string) => Promise<void>;
  ValidateWorkspaceDir: (path: string) => Promise<WorkspaceValidateResult>;
}

export type BackendMethod = keyof BackendApi & string;

export type BackendArgs<M extends BackendMethod> = Parameters<BackendApi[M]>;

export type BackendResult<M extends BackendMethod> = Awaited<
  ReturnType<BackendApi[M]>
>;

export type TypedBackendApi = {
  [M in BackendMethod]: (
    ...args: BackendArgs<M>
  ) => Promise<BackendResult<M>>;
};

