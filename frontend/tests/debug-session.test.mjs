import assert from "node:assert/strict";
import test from "node:test";
import {
  createDebugSessionCore,
  emptyDebugSessionState,
  mergeDebugSessionState,
} from "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/debug/debugSessionCore.js";

function createCell(initial) {
  return { value: initial };
}

function session(overrides = {}) {
  return { ...emptyDebugSessionState(), ...overrides };
}

function createCore(overrides = {}) {
  const subscribers = [];
  const backend = {
    getSessionState: async () => session(),
    launchGame: async () => session(),
    stopGame: async () => session(),
    ...overrides.backend,
  };
  const core = createDebugSessionCore({
    createCell,
    subscribe(eventName, callback) {
      subscribers.push({ eventName, callback });
      return () => {
        const index = subscribers.findIndex((entry) => entry.callback === callback);
        if (index >= 0) subscribers.splice(index, 1);
      };
    },
    backend,
  });
  return { core, subscribers };
}

test("a delayed snapshot cannot roll back a newer event", () => {
  const current = {
    value: session({ session_id: "s1", status: "running", updated_at_ms: 200 }),
    latestUpdatedAt: 200,
    latestSource: "event",
  };
  const merged = mergeDebugSessionState(
    current,
    session({ session_id: "s1", status: "starting", updated_at_ms: 100 }),
    "snapshot",
  );
  assert.equal(merged.value.status, "running");
  assert.equal(merged.latestUpdatedAt, 200);
});

test("an equal-timestamp event wins over a snapshot", () => {
  const current = {
    value: session({ session_id: "s1", status: "idle", updated_at_ms: 500 }),
    latestUpdatedAt: 500,
    latestSource: "event",
  };
  const merged = mergeDebugSessionState(
    current,
    session({ session_id: "s1", status: "idle", updated_at_ms: 500 }),
    "snapshot",
  );
  assert.equal(merged.value.status, "idle");
});

test("subscribes before the initial read and keeps the newer event", async () => {
  let resolveRead;
  const readPromise = new Promise((resolve) => {
    resolveRead = resolve;
  });
  const { core, subscribers } = createCore({
    backend: { getSessionState: () => readPromise },
  });

  const initPromise = core.init();
  assert.equal(subscribers.length, 1, "must subscribe before reading the snapshot");

  subscribers[0].callback(
    session({ session_id: "s1", status: "running", cs2_pid: 42, updated_at_ms: 300 }),
  );
  resolveRead(session({ session_id: "s1", status: "idle", updated_at_ms: 100 }));
  await initPromise;

  assert.equal(core.sessionState.value.status, "running");
  assert.equal(core.sessionState.value.cs2_pid, 42);
  assert.equal(core.initialized.value, true);
});

test("launch and stop apply the returned snapshot and surface errors", async () => {
  const launchState = session({
    session_id: "s1",
    status: "running",
    cs2_pid: 7,
    plugin_enabled: true,
    plugin_message: "等待插件连接",
    updated_at_ms: 10,
  });
  const stopState = session({ session_id: "s1", status: "idle", updated_at_ms: 20 });
  let stopError = null;
  const { core } = createCore({
    backend: {
      launchGame: async (request) => {
        assert.equal(request.inject_plugin, true);
        return launchState;
      },
      stopGame: async () => {
        if (stopError) throw stopError;
        return stopState;
      },
    },
  });
  await core.init();

  const launched = await core.launch({
    inject_plugin: true,
    pov_hud_enabled: false,
    brand_enabled: true,
    pov_radar_enabled: false,
    launch_resolution: "4:3",
  });
  assert.equal(launched.status, "running");
  assert.equal(core.sessionState.value.cs2_pid, 7);
  assert.equal(core.launching.value, false);

  const stopped = await core.stop();
  assert.equal(stopped.status, "idle");
  assert.equal(core.sessionState.value.status, "idle");

  stopError = new Error("restore failed");
  await assert.rejects(core.stop(), /restore failed/);
  assert.equal(core.errorMessage.value, "restore failed");
  assert.equal(core.stopping.value, false);
});

test("dispose drops local facts and ignores late events", async () => {
  const { core, subscribers } = createCore({
    backend: {
      getSessionState: async () => session({ session_id: "s1", status: "running", updated_at_ms: 10 }),
    },
  });
  await core.init();
  assert.equal(core.sessionState.value.status, "running");

  const lateCallback = subscribers[0].callback;
  core.dispose();
  assert.equal(core.sessionState.value.status, "idle");
  assert.equal(core.initialized.value, false);
  assert.equal(subscribers.length, 0, "dispose must release the event subscription");

  lateCallback(session({ session_id: "s1", status: "running", updated_at_ms: 99 }));
  assert.equal(core.sessionState.value.status, "idle");
});
