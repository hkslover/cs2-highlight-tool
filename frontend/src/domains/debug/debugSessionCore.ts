import type {
  DebugCommandRequest,
  DebugCommandResponse,
  DebugLaunchRequest,
  DebugPlayDemoRequest,
  DebugPlayDemoResponse,
  DebugSessionState,
} from "@/shared/types";

export const DEBUG_SESSION_EVENT = "debug_session_state_changed";

export type DebugSessionSource = "event" | "snapshot";

export interface DebugSessionCell<T> {
  value: T;
}

export interface DebugSessionBackend {
  getSessionState: () => Promise<DebugSessionState>;
  launchGame: (request: DebugLaunchRequest) => Promise<DebugSessionState>;
  stopGame: () => Promise<DebugSessionState>;
  sendCommand: (request: DebugCommandRequest) => Promise<DebugCommandResponse>;
  playDemo: (request: DebugPlayDemoRequest) => Promise<DebugPlayDemoResponse>;
}

export type DebugSessionSubscriber = (
  eventName: string,
  callback: (payload: unknown) => void,
) => () => void;

export interface DebugSessionCoreOptions {
  backend: DebugSessionBackend;
  subscribe: DebugSessionSubscriber;
  createCell: <T>(initial: T) => DebugSessionCell<T>;
}

/** The idle snapshot shown before any session has started. */
export function emptyDebugSessionState(): DebugSessionState {
  return {
    session_id: "",
    status: "idle",
    inject_plugin: false,
    pov_hud_enabled: false,
    brand_enabled: false,
    pov_radar_enabled: false,
    launch_resolution: "4:3",
    process_started: false,
    cs2_pid: 0,
    plugin_enabled: false,
    plugin_connected: false,
    plugin_ready: false,
    plugin_message: "",
    demo_path: "",
    demo_playing: false,
    demo_tick: 0,
    error: "",
    updated_at_ms: 0,
  };
}

export interface DebugSessionCellState {
  value: DebugSessionState;
  latestUpdatedAt: number;
  latestSource?: DebugSessionSource;
}

function normalizeTimestamp(value: number): number {
  const numeric = Number(value);
  return Number.isFinite(numeric) && numeric >= 0 ? numeric : 0;
}

/**
 * Apply an event or snapshot without letting a delayed read roll back a newer
 * event. The session id is kept as part of the payload, so a snapshot that
 * still describes an older session is rejected by the timestamp/source rules.
 */
export function mergeDebugSessionState(
  current: DebugSessionCellState,
  next: DebugSessionState | null | undefined,
  source: DebugSessionSource,
): DebugSessionCellState {
  const incoming = next ?? emptyDebugSessionState();
  const incomingUpdatedAt = normalizeTimestamp(incoming.updated_at_ms);
  if (incomingUpdatedAt < current.latestUpdatedAt) {
    return current;
  }
  if (
    source === "snapshot" &&
    current.latestSource === "event" &&
    incomingUpdatedAt <= current.latestUpdatedAt
  ) {
    return current;
  }
  return {
    value: incoming,
    latestUpdatedAt: incomingUpdatedAt,
    latestSource: source,
  };
}

export function createDebugSessionCore(options: DebugSessionCoreOptions) {
  const sessionState = options.createCell<DebugSessionState>(emptyDebugSessionState());
  const initialized = options.createCell(false);
  const loading = options.createCell(false);
  const launching = options.createCell(false);
  const stopping = options.createCell(false);
  const sending = options.createCell(false);
  const playing = options.createCell(false);
  const lastCommandResult = options.createCell<DebugCommandResponse | null>(null);
  const lastPlayResult = options.createCell<DebugPlayDemoResponse | null>(null);
  const errorMessage = options.createCell("");

  let latestUpdatedAt = 0;
  let latestSource: DebugSessionSource | undefined;
  let unsubscribe: (() => void) | undefined;
  let generation = 0;
  let initializing: Promise<void> | undefined;

  function apply(next: DebugSessionState | null | undefined, source: DebugSessionSource): void {
    const merged = mergeDebugSessionState(
      { value: sessionState.value, latestUpdatedAt, latestSource },
      next,
      source,
    );
    sessionState.value = merged.value;
    latestUpdatedAt = merged.latestUpdatedAt;
    latestSource = merged.latestSource;
  }

  function init(): Promise<void> {
    if (initialized.value) {
      return Promise.resolve();
    }
    if (initializing) {
      return initializing;
    }
    const currentGeneration = generation;
    if (!unsubscribe) {
      try {
        unsubscribe = options.subscribe(DEBUG_SESSION_EVENT, (payload) => {
          if (currentGeneration !== generation) return;
          apply(payload as DebugSessionState, "event");
        });
      } catch (error: unknown) {
        errorMessage.value = error instanceof Error ? error.message : String(error);
        return Promise.reject(error);
      }
    }
    loading.value = true;
    let current: Promise<void>;
    // Subscribe before the initial read so an event cannot be lost between the
    // two; the merge rules keep the newer of the two.
    current = Promise.resolve()
      .then(() => options.backend.getSessionState())
      .then((snapshot) => {
        if (currentGeneration !== generation) return;
        apply(snapshot, "snapshot");
        initialized.value = true;
      })
      .catch((error: unknown) => {
        if (currentGeneration === generation) {
          errorMessage.value = error instanceof Error ? error.message : String(error);
          initialized.value = false;
        }
        throw error;
      })
      .finally(() => {
        if (currentGeneration === generation) {
          loading.value = false;
        }
        if (initializing === current) {
          initializing = undefined;
        }
      });
    initializing = current;
    return current;
  }

  async function launch(request: DebugLaunchRequest): Promise<DebugSessionState | null> {
    const currentGeneration = generation;
    launching.value = true;
    errorMessage.value = "";
    try {
      const next = await options.backend.launchGame(request);
      // A workspace switch disposes the store while the request is in flight;
      // the late response must not repopulate the cleared state.
      if (currentGeneration !== generation) return null;
      apply(next, "snapshot");
      return next;
    } catch (error: unknown) {
      if (currentGeneration === generation) {
        errorMessage.value = error instanceof Error ? error.message : String(error);
      }
      throw error;
    } finally {
      if (currentGeneration === generation) {
        launching.value = false;
      }
    }
  }

  async function stop(): Promise<DebugSessionState | null> {
    const currentGeneration = generation;
    stopping.value = true;
    errorMessage.value = "";
    try {
      const next = await options.backend.stopGame();
      if (currentGeneration !== generation) return null;
      apply(next, "snapshot");
      return next;
    } catch (error: unknown) {
      if (currentGeneration === generation) {
        errorMessage.value = error instanceof Error ? error.message : String(error);
      }
      throw error;
    } finally {
      if (currentGeneration === generation) {
        stopping.value = false;
      }
    }
  }

  /**
   * Send one console request and keep its receipt for the page. A failed
   * receipt is a normal result; only transport/session errors reject, and they
   * are left to the caller so the session-level error cell stays about the
   * session, not one command.
   */
  async function sendCommand(
    commands: string[],
  ): Promise<DebugCommandResponse | null> {
    const currentGeneration = generation;
    sending.value = true;
    try {
      const result = await options.backend.sendCommand({ commands });
      if (currentGeneration !== generation) return null;
      lastCommandResult.value = result;
      return result;
    } finally {
      if (currentGeneration === generation) {
        sending.value = false;
      }
    }
  }

  /**
   * Play one managed Demo. The receipt only proves the plugin queued the play
   * command; the real playback state arrives through `sessionState`.
   */
  async function playDemo(
    demoPath: string,
    commands: string[] = [],
  ): Promise<DebugPlayDemoResponse | null> {
    const currentGeneration = generation;
    playing.value = true;
    try {
      const result = await options.backend.playDemo({
        demo_path: demoPath,
        commands,
      });
      if (currentGeneration !== generation) return null;
      lastPlayResult.value = result;
      return result;
    } finally {
      if (currentGeneration === generation) {
        playing.value = false;
      }
    }
  }

  /** Drop every local session fact and the event subscription. */
  function dispose(): void {
    generation++;
    initializing = undefined;
    unsubscribe?.();
    unsubscribe = undefined;
    initialized.value = false;
    loading.value = false;
    launching.value = false;
    stopping.value = false;
    sending.value = false;
    playing.value = false;
    lastCommandResult.value = null;
    lastPlayResult.value = null;
    errorMessage.value = "";
    latestUpdatedAt = 0;
    latestSource = undefined;
    sessionState.value = emptyDebugSessionState();
  }

  return {
    sessionState,
    initialized,
    loading,
    launching,
    stopping,
    sending,
    playing,
    lastCommandResult,
    lastPlayResult,
    errorMessage,
    init,
    launch,
    stop,
    sendCommand,
    playDemo,
    dispose,
  };
}
