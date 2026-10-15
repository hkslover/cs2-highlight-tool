import { ref, type Ref } from "vue";
import { EventsOn } from "../../../wailsjs/runtime/runtime.js";
import { backend } from "../../shared/backend/adapter.js";
import {
  createDebugSessionCore,
  DEFAULT_DEBUG_LAUNCH_ARGUMENTS,
  type DebugSessionCell,
} from "./debugSessionCore.js";
import { resolveDebugDemoKey } from "./debugDemoSelection.js";
import type { DemoListEntry } from "../../shared/types/index.js";
import type {
  DebugCommandRequest,
  DebugCommandResponse,
  DebugLaunchRequest,
  DebugPlayDemoRequest,
  DebugPlayDemoResponse,
  DebugRecordingRequest,
  DebugSessionState,
} from "../../shared/types/index.js";

const core = createDebugSessionCore({
  createCell<T>(initial: T): DebugSessionCell<T> {
    // SAFETY: a Vue Ref is structurally a mutable value cell and the core only
    // reads/writes `.value`, so erasing the generic wrapper is sound here.
    return ref(initial) as unknown as DebugSessionCell<T>;
  },
  subscribe(eventName, callback) {
    return EventsOn(eventName, callback);
  },
  backend: {
    getSessionState: () => backend.GetDebugSessionState(),
    launchGame: (request: DebugLaunchRequest) => backend.LaunchDebugGame(request),
    stopGame: () => backend.StopDebugGame(),
    sendCommand: (request: DebugCommandRequest) =>
      backend.SendDebugConsoleCommand(request),
    playDemo: (request: DebugPlayDemoRequest): Promise<DebugPlayDemoResponse> =>
      backend.PlayDebugDemo(request),
    startRecording: (request: DebugRecordingRequest): Promise<DebugSessionState> =>
      backend.StartDebugRecording(request),
    stopRecording: (): Promise<DebugSessionState> => backend.StopDebugRecording(),
    retryRecordingMerge: (recordingID: string): Promise<DebugSessionState> =>
      backend.RetryDebugRecordingMerge(recordingID),
    openRecordingInFolder: (recordingID: string): Promise<void> =>
      backend.OpenDebugRecordingInFolder(recordingID),
  },
});

export const initDebugSession = core.init;
export const launchDebugGame = core.launch;
export const stopDebugGame = core.stop;
export const sendDebugConsoleCommand = core.sendCommand;
export const playDebugDemo = core.playDemo;
export const startDebugRecording = core.startRecording;
export const stopDebugRecording = core.stopRecording;
export const retryDebugRecordingMerge = core.retryRecordingMerge;
export const openDebugRecordingInFolder = core.openRecordingInFolder;

// The debug page's own pending Demo. Kept outside the backend session snapshot
// because it is a local workbench draft: choosing a Demo only changes what the
// next `play` request targets and never claims that the engine already plays
// it.
const debugDemoKeyState = ref<string | null>(null);
const debugLaunchInput = ref({ arguments: DEFAULT_DEBUG_LAUNCH_ARGUMENTS, commands: "" });
export function useDebugLaunchInput() { return debugLaunchInput; }

/**
 * Keep the debug selection valid. The current key wins while it still exists;
 * otherwise the import-page selection is adopted once, and finally the first
 * imported Demo (or an empty selection) is used. Called on page mount and
 * whenever the shared Demo list changes.
 */
export function ensureDebugDemoSelection(
  entries: DemoListEntry[],
  preferredKey: string | null = null,
): string | null {
  const resolved = resolveDebugDemoKey(entries, debugDemoKeyState.value, preferredKey);
  debugDemoKeyState.value = resolved;
  return resolved;
}

export function selectDebugDemo(key: string | null): void {
  debugDemoKeyState.value = key;
}

/**
 * Clear local debug session facts when the workspace changes. The next debug
 * page mount re-subscribes, reads the new workspace's snapshot and picks a
 * Demo from the new list.
 */
export function resetDebugSessionForWorkspace(): void {
  core.dispose();
  debugDemoKeyState.value = null;
  debugLaunchInput.value = { arguments: DEFAULT_DEBUG_LAUNCH_ARGUMENTS, commands: "" };
}

export function useDebugSession() {
  return {
    sessionState: core.sessionState as Readonly<Ref<DebugSessionState>>,
    initialized: core.initialized as Readonly<Ref<boolean>>,
    loading: core.loading as Readonly<Ref<boolean>>,
    launching: core.launching as Readonly<Ref<boolean>>,
    stopping: core.stopping as Readonly<Ref<boolean>>,
    sending: core.sending as Readonly<Ref<boolean>>,
    playing: core.playing as Readonly<Ref<boolean>>,
    startingRecording: core.startingRecording as Readonly<Ref<boolean>>,
    stoppingRecording: core.stoppingRecording as Readonly<Ref<boolean>>,
    retryingRecording: core.retryingRecording as Readonly<Ref<string | null>>,
    lastCommandResult: core.lastCommandResult as Readonly<Ref<DebugCommandResponse | null>>,
    lastPlayResult: core.lastPlayResult as Readonly<Ref<DebugPlayDemoResponse | null>>,
    errorMessage: core.errorMessage as Readonly<Ref<string>>,
    initDebugSession,
    launchDebugGame,
    stopDebugGame,
    sendDebugConsoleCommand,
    playDebugDemo,
    startDebugRecording,
    stopDebugRecording,
    retryDebugRecordingMerge,
    openDebugRecordingInFolder,
  };
}

/** The debug page's own pending Demo draft. */
export function useDebugDemoSelection() {
  return {
    debugDemoKey: debugDemoKeyState as Readonly<Ref<string | null>>,
    ensureDebugDemoSelection,
    selectDebugDemo,
  };
}
