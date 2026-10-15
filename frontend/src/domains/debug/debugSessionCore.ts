import type {
  DebugCommandRequest,
  DebugCommandResponse,
  DebugLaunchRequest,
  DebugLaunchResolution,
  DebugPlayDemoRequest,
  DebugPlayDemoResponse,
  DebugRecordingRequest,
  DebugSessionState,
} from "@/shared/types";

// The debug workbench has no resolution selector: the CS2 render resolution
// lives in the launch arguments, and this default keeps the 4:3 1280x960
// backbuffer the recording profile expects.
export const DEFAULT_DEBUG_LAUNCH_ARGUMENTS =
  "-insecure -novid -low -high +sv_lan 1 -coop_fullscreen -worldwide -console -w 1280 -h 960";

/**
 * Map the CS2 backbuffer carried by the launch arguments onto the recording
 * profile. Both 4:3 sizes keep the real-pixel stretch, a 16:9 backbuffer must
 * not be stretched. Without an explicit -w/-h the game falls back to its own
 * video settings and the 4:3 profile stays the default, because its stretch
 * filter is a no-op for 16:9 input. The backend re-derives this from the same
 * arguments and its result is authoritative.
 */
export function parseLaunchResolutionFromArguments(text: string): DebugLaunchResolution {
  const { width, height } = parseRenderResolution(text);
  if (width <= 0 || height <= 0) return "4:3";
  if (width * 3 !== height * 4) return "16:9";
  if (width === 1280 && height === 960) return "4:3_1280x960";
  return "4:3";
}

/** The last -w/-h pair wins, matching how a repeated engine switch resolves. */
function parseRenderResolution(text: string): { width: number; height: number } {
  const fields = splitLaunchArguments(text);
  let width = 0;
  let height = 0;
  for (let index = 0; index < fields.length; index += 1) {
    const flag = fields[index].toLowerCase();
    if (flag !== "-w" && flag !== "-width" && flag !== "-h" && flag !== "-height") continue;
    const raw = fields[index + 1] ?? "";
    if (!/^\d+$/.test(raw)) continue;
    const value = Number.parseInt(raw, 10);
    if (!Number.isFinite(value) || value <= 0) continue;
    if (flag === "-w" || flag === "-width") width = value;
    else height = value;
  }
  return { width, height };
}

/**
 * Tokenize without changing the command line sent to CS2. Double quotes group
 * whitespace; backslashes before a quote follow Windows command-line rules.
 * In particular, switches inside a quoted console command are not switches.
 */
function splitLaunchArguments(text: string): string[] {
  const fields: string[] = [];
  let field = "";
  let quoted = false;
  let started = false;
  for (let index = 0; index < text.length; index += 1) {
    const character = text[index];
    if (!quoted && /[ \t\r\n]/.test(character)) {
      if (started) fields.push(field);
      field = "";
      started = false;
      continue;
    }
    started = true;
    if (character === "\\") {
      let end = index;
      while (text[end] === "\\") end += 1;
      const count = end - index;
      if (text[end] === '"') {
        field += "\\".repeat(Math.floor(count / 2));
        if (count % 2 === 0) quoted = !quoted;
        else field += '"';
        index = end;
      } else {
        field += "\\".repeat(count);
        index = end - 1;
      }
    } else if (character === '"') {
      quoted = !quoted;
    } else {
      field += character;
    }
  }
  if (started) fields.push(field);
  return fields;
}

/** Each nonempty line is one command; semicolons and quotes stay untouched. */
export function parseDebugStartupCommands(text: string): string[] {
  return text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
}

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
  startRecording: (request: DebugRecordingRequest) => Promise<DebugSessionState>;
  stopRecording: () => Promise<DebugSessionState>;
  retryRecordingMerge: (recordingID: string) => Promise<DebugSessionState>;
  openRecordingInFolder: (recordingID: string) => Promise<void>;
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
    plugin_state: "",
    demo_path: "",
    demo_playing: false,
    demo_paused: false,
    demo_play_pending: false,
    recording: null,
    recordings: [],
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
  const startingRecording = options.createCell(false);
  const stoppingRecording = options.createCell(false);
  const retryingRecording = options.createCell<string | null>(null);
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

  /**
   * Start one manual recording segment. The returned snapshot describes the
   * frozen segment; the real recording status only advances on the plugin's
   * boundary observation carried by `sessionState.recording`.
   */
  async function startRecording(
    request: DebugRecordingRequest,
  ): Promise<DebugSessionState | null> {
    const currentGeneration = generation;
    startingRecording.value = true;
    try {
      const next = await options.backend.startRecording(request);
      if (currentGeneration !== generation) return null;
      apply(next, "snapshot");
      return next;
    } finally {
      if (currentGeneration === generation) {
        startingRecording.value = false;
      }
    }
  }

  /**
   * Ask for the active segment to end. The merge runs asynchronously in the
   * backend, so this resolves as soon as the stop boundary was answered.
   */
  async function stopRecording(): Promise<DebugSessionState | null> {
    const currentGeneration = generation;
    stoppingRecording.value = true;
    try {
      const next = await options.backend.stopRecording();
      if (currentGeneration !== generation) return null;
      apply(next, "snapshot");
      return next;
    } finally {
      if (currentGeneration === generation) {
        stoppingRecording.value = false;
      }
    }
  }

  /** Retry only the failed merge of one segment, reusing its raw files. */
  async function retryRecordingMerge(
    recordingID: string,
  ): Promise<DebugSessionState | null> {
    const currentGeneration = generation;
    retryingRecording.value = recordingID;
    try {
      const next = await options.backend.retryRecordingMerge(recordingID);
      if (currentGeneration !== generation) return null;
      apply(next, "snapshot");
      return next;
    } finally {
      if (currentGeneration === generation) {
        retryingRecording.value = null;
      }
    }
  }

  /** Locate one segment result; a failure stays a local page error. */
  async function openRecordingInFolder(recordingID: string): Promise<void> {
    await options.backend.openRecordingInFolder(recordingID);
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
    startingRecording.value = false;
    stoppingRecording.value = false;
    retryingRecording.value = null;
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
    startingRecording,
    stoppingRecording,
    retryingRecording,
    lastCommandResult,
    lastPlayResult,
    errorMessage,
    init,
    launch,
    stop,
    sendCommand,
    playDemo,
    startRecording,
    stopRecording,
    retryRecordingMerge,
    openRecordingInFolder,
    dispose,
  };
}
