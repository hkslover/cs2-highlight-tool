import assert from "node:assert/strict";
import test from "node:test";
import {
  canToggleDemoPause,
  debugPauseCommand,
} from "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/debug/debugPlaybackPolicy.js";

function facts(overrides = {}) {
  return {
    sessionRunning: true,
    pluginReady: true,
    commandInFlight: false,
    playRequestInFlight: false,
    demoPlaying: true,
    demoPlayPending: false,
    ...overrides,
  };
}

test("pause follows a running, ready session with an actually playing Demo", () => {
  assert.equal(canToggleDemoPause(facts()), true);
});

test("pause is unavailable without a running session, a ready plugin or playback", () => {
  assert.equal(canToggleDemoPause(facts({ sessionRunning: false })), false);
  assert.equal(canToggleDemoPause(facts({ pluginReady: false })), false);
  assert.equal(canToggleDemoPause(facts({ demoPlaying: false })), false);
});

test("pause is unavailable while a command is in flight", () => {
  assert.equal(canToggleDemoPause(facts({ commandInFlight: true })), false);
});

// A play request keeps the previous Demo's state on screen until the engine
// confirms the requested one; pause must not send in that window.
test("pause is unavailable while a play request is in flight or unconfirmed", () => {
  assert.equal(canToggleDemoPause(facts({ playRequestInFlight: true })), false);
  assert.equal(canToggleDemoPause(facts({ demoPlayPending: true })), false);
  assert.equal(
    canToggleDemoPause(facts({ demoPlayPending: true, playRequestInFlight: true })),
    false,
  );
});

test("the command direction only mirrors the engine-observed pause state", () => {
  assert.equal(debugPauseCommand(false), "demo_pause");
  assert.equal(debugPauseCommand(true), "demo_resume");
});
