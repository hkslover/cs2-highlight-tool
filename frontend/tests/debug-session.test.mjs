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
    sendCommand: async (request) => ({
      request_id: "req-default",
      status: "executed",
      commands: request.commands,
    }),
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

const launchRequest = {
  inject_plugin: true,
  pov_hud_enabled: false,
  brand_enabled: true,
  pov_radar_enabled: false,
  launch_resolution: "4:3",
};

test("a late launch response after dispose cannot repopulate the store", async () => {
  let resolveLaunch;
  const launchPromise = new Promise((resolve) => {
    resolveLaunch = resolve;
  });
  const { core } = createCore({ backend: { launchGame: () => launchPromise } });
  await core.init();

  const pending = core.launch(launchRequest);
  core.dispose();
  resolveLaunch(session({ session_id: "old-workspace", status: "running", cs2_pid: 5, updated_at_ms: 999 }));

  assert.equal(await pending, null, "a stale response must report that it was dropped");
  assert.equal(core.sessionState.value.status, "idle");
  assert.equal(core.sessionState.value.session_id, "");
  assert.equal(core.sessionState.value.cs2_pid, 0);
  assert.equal(core.launching.value, false);
});

test("a late launch failure after dispose cannot write the old error", async () => {
  let rejectLaunch;
  const launchPromise = new Promise((_resolve, reject) => {
    rejectLaunch = reject;
  });
  const { core } = createCore({ backend: { launchGame: () => launchPromise } });
  await core.init();

  const pending = core.launch(launchRequest);
  core.dispose();
  rejectLaunch(new Error("old workspace failure"));

  await assert.rejects(pending, /old workspace failure/);
  assert.equal(core.errorMessage.value, "");
  assert.equal(core.launching.value, false);
});

test("a stale request cannot clear a newer request's loading state", async () => {
  let resolveOld;
  const oldPromise = new Promise((resolve) => {
    resolveOld = resolve;
  });
  let resolveNew;
  const newPromise = new Promise((resolve) => {
    resolveNew = resolve;
  });
  let launchCount = 0;
  const { core } = createCore({
    backend: {
      launchGame: () => {
        launchCount += 1;
        return launchCount === 1 ? oldPromise : newPromise;
      },
    },
  });
  await core.init();

  const oldRequest = core.launch(launchRequest);
  core.dispose();
  await core.init();
  const newRequest = core.launch(launchRequest);

  resolveOld(session({ session_id: "old", status: "running", updated_at_ms: 10 }));
  assert.equal(await oldRequest, null);
  assert.equal(core.launching.value, true, "the newer request is still loading");
  assert.equal(core.sessionState.value.session_id, "");

  resolveNew(session({ session_id: "new", status: "running", updated_at_ms: 20 }));
  assert.equal((await newRequest).session_id, "new");
  assert.equal(core.launching.value, false);
  assert.equal(core.sessionState.value.session_id, "new");
});

test("sendCommand keeps the receipt and forwards the raw command element", async () => {
  const seen = [];
  const { core } = createCore({
    backend: {
      sendCommand: async (request) => {
        seen.push(request);
        return {
          request_id: "req-1",
          status: "executed",
          commands: request.commands,
        };
      },
    },
  });
  await core.init();

  const result = await core.sendCommand(["sv_cheats 1; mirv_streams record start"]);
  assert.equal(result.status, "executed");
  assert.deepEqual(seen[0].commands, ["sv_cheats 1; mirv_streams record start"]);
  assert.equal(core.lastCommandResult.value.request_id, "req-1");
  assert.equal(core.sending.value, false);
});

test("a failed receipt is kept as a result, not a thrown error", async () => {
  const { core } = createCore({
    backend: {
      sendCommand: async () => ({
        request_id: "req-2",
        status: "failed",
        error: "Radar POV hook installation failed",
        commands: ["csdm_radar_pov 1"],
      }),
    },
  });
  await core.init();

  const result = await core.sendCommand(["csdm_radar_pov 1"]);
  assert.equal(result.status, "failed");
  assert.equal(core.lastCommandResult.value.error, "Radar POV hook installation failed");
  assert.equal(core.errorMessage.value, "");
});

test("a transport failure rejects and clears the previous result on dispose", async () => {
  const { core } = createCore({
    backend: {
      sendCommand: async () => {
        throw new Error("插件调试协议未就绪");
      },
    },
  });
  await core.init();

  await assert.rejects(core.sendCommand(["quit"]), /插件调试协议未就绪/);
  assert.equal(core.errorMessage.value, "", "command errors stay out of the session error cell");
  assert.equal(core.lastCommandResult.value, null);

  core.dispose();
  assert.equal(core.sending.value, false);
  assert.equal(core.lastCommandResult.value, null);
  assert.equal(core.errorMessage.value, "");
});
