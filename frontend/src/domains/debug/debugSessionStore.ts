import { ref, type Ref } from "vue";
import { EventsOn } from "../../../wailsjs/runtime/runtime";
import { backend } from "@/shared/backend";
import {
  createDebugSessionCore,
  type DebugSessionCell,
} from "./debugSessionCore";
import type {
  DebugCommandRequest,
  DebugCommandResponse,
  DebugLaunchRequest,
  DebugSessionState,
} from "@/shared/types";

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
  },
});

export const initDebugSession = core.init;
export const launchDebugGame = core.launch;
export const stopDebugGame = core.stop;
export const sendDebugConsoleCommand = core.sendCommand;

/**
 * Clear local debug session facts when the workspace changes. The next debug
 * page mount re-subscribes and reads the new workspace's snapshot.
 */
export function resetDebugSessionForWorkspace(): void {
  core.dispose();
}

export function useDebugSession() {
  return {
    sessionState: core.sessionState as Readonly<Ref<DebugSessionState>>,
    initialized: core.initialized as Readonly<Ref<boolean>>,
    loading: core.loading as Readonly<Ref<boolean>>,
    launching: core.launching as Readonly<Ref<boolean>>,
    stopping: core.stopping as Readonly<Ref<boolean>>,
    sending: core.sending as Readonly<Ref<boolean>>,
    lastCommandResult: core.lastCommandResult as Readonly<Ref<DebugCommandResponse | null>>,
    errorMessage: core.errorMessage as Readonly<Ref<string>>,
    initDebugSession,
    launchDebugGame,
    stopDebugGame,
    sendDebugConsoleCommand,
  };
}
