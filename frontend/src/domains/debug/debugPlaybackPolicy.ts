/**
 * Admission for the single pause/resume button.
 *
 * The button follows engine observations only. In particular it must not be
 * re-enabled by a play receipt: `demo_play_pending` stays true until an engine
 * observation proves the requested Demo is the one playing, so a press in that
 * window can never be evaluated against the previous Demo's pause state and
 * then land on the new Demo.
 */
export interface DebugPauseFacts {
  sessionRunning: boolean;
  pluginReady: boolean;
  /** A console request is being sent (shared command channel). */
  commandInFlight: boolean;
  /** A play request is waiting for its receipt or for engine confirmation. */
  playRequestInFlight: boolean;
  demoPlaying: boolean;
  demoPlayPending: boolean;
}

export function canToggleDemoPause(facts: DebugPauseFacts): boolean {
  return (
    facts.sessionRunning &&
    facts.pluginReady &&
    !facts.commandInFlight &&
    !facts.playRequestInFlight &&
    facts.demoPlaying &&
    !facts.demoPlayPending
  );
}

/** The command matching the engine-observed pause state, never a local flip. */
export function debugPauseCommand(demoPaused: boolean): "demo_pause" | "demo_resume" {
  return demoPaused ? "demo_resume" : "demo_pause";
}
