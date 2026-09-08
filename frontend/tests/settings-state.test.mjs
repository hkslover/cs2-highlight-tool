import assert from "node:assert/strict";
import test from "node:test";
import {
  createDefaultClipSettings,
  createSettingsStore,
} from "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/settings/settings-state.js";

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function workspace() {
  return { initialized: true, data_dir: "C:/cs2HighLightTool", error: "" };
}

function settings(overrides = {}) {
  return { ...createDefaultClipSettings(), ...overrides };
}

async function waitFor(predicate, message) {
  for (let i = 0; i < 100; i += 1) {
    if (predicate()) return;
    await new Promise((resolve) => setImmediate(resolve));
  }
  assert.fail(message || "condition was not reached");
}

function storeWith(overrides = {}) {
  const base = settings(overrides);
  const calls = [];
  const api = {
    GetWorkspaceState: async () => workspace(),
    GetClipSettings: async () => ({ ...base }),
    SaveClipSettings: async (next) => {
      calls.push({ ...next });
      return { ...base, ...next };
    },
  };
  return {
    base,
    calls,
    api,
    store: createSettingsStore({ backend: api, autoSaveDelayMs: 10 }),
  };
}

test("concurrent settings entries share one load and a load response preserves a new draft", async () => {
  const get = deferred();
  let getCount = 0;
  const base = settings({ killer_pre_seconds: 8, killer_post_seconds: 9 });
  const api = {
    GetWorkspaceState: async () => workspace(),
    GetClipSettings: async () => {
      getCount += 1;
      return get.promise;
    },
    SaveClipSettings: async (next) => ({ ...base, ...next }),
  };
  const store = createSettingsStore({ backend: api, autoSaveDelayMs: 10 });

  const first = store.init();
  const second = store.init();
  await Promise.resolve();
  assert.equal(getCount, 1);

  store.draftSettings.killer_post_seconds = 14;
  get.resolve(base);
  await Promise.all([first, second]);

  assert.equal(store.confirmedSettings.value.killer_pre_seconds, 8);
  assert.equal(store.draftSettings.killer_pre_seconds, 8);
  assert.equal(store.draftSettings.killer_post_seconds, 14);
  assert.equal(store.dirty.value, true);
  await store.dispose();
});

test("a response from an older save cannot replace edits made while it was in flight", async () => {
  const base = settings();
  const saves = [];
  // Replace the isolated store with a deferred backend so the first request
  // remains in flight while the second edit is made.
  const api = {
    GetWorkspaceState: async () => workspace(),
    GetClipSettings: async () => ({ ...base }),
    SaveClipSettings: (next) => {
      const request = deferred();
      saves.push({ next: { ...next }, request });
      return request.promise;
    },
  };
  const active = createSettingsStore({ backend: api, autoSaveDelayMs: 10 });
  await active.init();
  active.draftSettings.killer_pre_seconds = 7;
  const flushed = active.flush();
  await waitFor(() => saves.length === 1, "first save request");

  active.draftSettings.killer_post_seconds = 11;
  saves[0].request.resolve({ ...base, ...saves[0].next });
  await waitFor(() => saves.length === 2, "queued save request");

  assert.equal(saves[0].next.killer_pre_seconds, 7);
  assert.equal(saves[0].next.killer_post_seconds, base.killer_post_seconds);
  assert.equal(saves[1].next.killer_pre_seconds, 7);
  assert.equal(saves[1].next.killer_post_seconds, 11);
  saves[1].request.resolve({ ...base, ...saves[1].next });
  await flushed;
  await active.dispose();

  assert.equal(active.draftSettings.killer_pre_seconds, 7);
  assert.equal(active.draftSettings.killer_post_seconds, 11);
  assert.equal(active.dirty.value, false);
});

test("dispose flushes the debounce window so closing a panel does not lose edits", async () => {
  const { store, calls } = storeWith();
  await store.init();
  store.draftSettings.edit_fps = 120;

  await store.dispose();

  assert.equal(calls.length, 1);
  assert.equal(calls[0].edit_fps, 120);
});

test("dispose waits for an in-flight save and drains an edit made during close", async () => {
  const base = settings();
  const saves = [];
  const api = {
    GetWorkspaceState: async () => workspace(),
    GetClipSettings: async () => ({ ...base }),
    SaveClipSettings: (next) => {
      const request = deferred();
      saves.push({ next: { ...next }, request });
      return request.promise;
    },
  };
  const store = createSettingsStore({ backend: api, autoSaveDelayMs: 10 });
  await store.init();
  store.draftSettings.edit_fps = 120;
  const closing = store.dispose();
  await waitFor(() => saves.length === 1, "first save request");

  store.draftSettings.record_fps = 144;
  saves[0].request.resolve({ ...base, ...saves[0].next });
  await waitFor(() => saves.length === 2, "queued save request");
  saves[1].request.resolve({ ...base, ...saves[1].next });
  await closing;

  assert.equal(saves[0].next.edit_fps, 120);
  assert.equal(saves[1].next.edit_fps, 120);
  assert.equal(saves[1].next.record_fps, 144);
  assert.equal(store.dirty.value, false);
  await store.dispose();
});

test("workspace initialization gates settings reads and permits a later retry", async () => {
  let initialized = false;
  let workspaceReads = 0;
  let settingsReads = 0;
  const base = settings({ record_fps: 120 });
  const api = {
    GetWorkspaceState: async () => {
      workspaceReads += 1;
      return {
        initialized,
        data_dir: initialized ? "C:/cs2HighLightTool" : "",
        error: "",
      };
    },
    GetClipSettings: async () => {
      settingsReads += 1;
      return { ...base };
    },
    SaveClipSettings: async (next) => ({ ...base, ...next }),
  };
  const store = createSettingsStore({ backend: api, autoSaveDelayMs: 10 });

  await store.init();
  assert.equal(workspaceReads, 1);
  assert.equal(settingsReads, 0);
  assert.equal(store.loaded.value, false);

  initialized = true;
  await store.init();
  assert.equal(workspaceReads, 2);
  assert.equal(settingsReads, 1);
  assert.equal(store.loaded.value, true);
  assert.equal(store.draftSettings.record_fps, 120);
  await store.dispose();
});

test("resetForWorkspace drops the old draft and reinitializes the selected workspace", async () => {
  const workspaceA = settings({ record_fps: 120 });
  const workspaceB = settings({ record_fps: 144 });
  const saves = [];
  let active = workspaceA;
  const store = createSettingsStore({
    backend: {
      GetWorkspaceState: async () => workspace(),
      GetClipSettings: async () => ({ ...active }),
      SaveClipSettings: async (next) => {
        saves.push({ ...next });
        return { ...active, ...next };
      },
    },
    autoSaveDelayMs: 10,
  });

  await store.init();
  assert.equal(store.draftSettings.record_fps, 120);
  store.draftSettings.record_fps = 121;
  active = workspaceB;
  store.resetForWorkspace();
  assert.equal(store.loaded.value, false);
  assert.equal(store.draftSettings.record_fps, createDefaultClipSettings().record_fps);
  await store.init();

  assert.equal(store.confirmedSettings.value.record_fps, 144);
  assert.equal(store.draftSettings.record_fps, 144);
  await new Promise((resolve) => setTimeout(resolve, 25));
  assert.equal(saves.length, 0);

  store.draftSettings.edit_fps = 30;
  await store.flush();
  assert.equal(saves.length, 1);
  assert.equal(saves[0].record_fps, 144);
  assert.equal(saves[0].edit_fps, 30);
  await store.dispose();
});

test("a detached old load response cannot pollute the new workspace", async () => {
  const oldLoad = deferred();
  const workspaceA = settings({ killer_pre_seconds: 17 });
  const workspaceB = settings({ killer_pre_seconds: 3 });
  let reads = 0;
  const store = createSettingsStore({
    backend: {
      GetWorkspaceState: async () => workspace(),
      GetClipSettings: async () => {
        reads += 1;
        return reads === 1 ? oldLoad.promise : { ...workspaceB };
      },
      SaveClipSettings: async (next) => ({ ...workspaceB, ...next }),
    },
  });

  const oldInit = store.init();
  await waitFor(() => reads === 1, "old settings read");
  store.resetForWorkspace();
  await store.init();
  assert.equal(store.draftSettings.killer_pre_seconds, 3);
  oldLoad.resolve(workspaceA);
  await oldInit;

  assert.equal(store.confirmedSettings.value.killer_pre_seconds, 3);
  assert.equal(store.draftSettings.killer_pre_seconds, 3);
  assert.equal(store.loading.value, false);
  await store.dispose();
});

test("a detached old save response cannot update the new generation or dispatch its event", async () => {
  const oldSave = deferred();
  const workspaceA = settings({ edit_fps: 120 });
  const workspaceB = settings({ edit_fps: 30 });
  let saveCalls = 0;
  const events = [];
  globalThis.window = { dispatchEvent: (event) => events.push(event) };
  const store = createSettingsStore({
    backend: {
      GetWorkspaceState: async () => workspace(),
      GetClipSettings: async () => (store.workspaceGeneration.value ? { ...workspaceB } : { ...workspaceA }),
      SaveClipSettings: (next) => {
        saveCalls += 1;
        return saveCalls === 1 ? oldSave.promise : Promise.resolve({ ...workspaceB, ...next });
      },
    },
    autoSaveDelayMs: 10,
  });

  await store.init();
  store.draftSettings.edit_fps = 121;
  const oldFlush = store.flush();
  await waitFor(() => saveCalls === 1, "old save request");
  store.resetForWorkspace();
  await store.init();
  assert.equal(store.draftSettings.edit_fps, 30);

  oldSave.resolve({ ...workspaceA, edit_fps: 121 });
  await oldFlush;
  assert.equal(store.confirmedSettings.value.edit_fps, 30);
  assert.equal(store.draftSettings.edit_fps, 30);
  assert.equal(store.loading.value, false);
  assert.equal(store.saving.value, false);
  assert.equal(events.length, 0);
  await store.dispose();
  delete globalThis.window;
});

test("a failed read keeps the placeholder local and never saves it", async () => {
  let saveCount = 0;
  const api = {
    GetWorkspaceState: async () => workspace(),
    GetClipSettings: async () => {
      throw new Error("read failed");
    },
    SaveClipSettings: async () => {
      saveCount += 1;
      return settings();
    },
  };
  const store = createSettingsStore({ backend: api, autoSaveDelayMs: 10 });
  await store.init();
  store.draftSettings.record_fps = 144;
  await store.flush();

  assert.equal(store.loaded.value, false);
  assert.equal(saveCount, 0);
});

test("a failed read can be retried without persisting the placeholder", async () => {
  const base = settings({ record_fps: 120 });
  let reads = 0;
  let saves = 0;
  const api = {
    GetWorkspaceState: async () => workspace(),
    GetClipSettings: async () => {
      reads += 1;
      if (reads === 1) {
        throw new Error("temporary read failure");
      }
      return { ...base };
    },
    SaveClipSettings: async (next) => {
      saves += 1;
      return { ...base, ...next };
    },
  };
  const store = createSettingsStore({ backend: api, autoSaveDelayMs: 10 });

  await store.init();
  assert.equal(store.loaded.value, false);
  await store.init();
  assert.equal(store.loaded.value, true);
  assert.equal(store.draftSettings.record_fps, 120);
  assert.equal(saves, 0);
  await store.dispose();
});

test("a failed save keeps the draft and a later flush can retry it", async () => {
  const base = settings();
  let attempts = 0;
  const api = {
    GetWorkspaceState: async () => workspace(),
    GetClipSettings: async () => ({ ...base }),
    SaveClipSettings: async (next) => {
      attempts += 1;
      if (attempts === 1) {
        throw new Error("write failed");
      }
      return { ...base, ...next };
    },
  };
  const store = createSettingsStore({ backend: api, autoSaveDelayMs: 10 });
  await store.init();
  store.draftSettings.record_fps = 144;

  await store.flush();
  assert.equal(attempts, 1);
  assert.equal(store.draftSettings.record_fps, 144);
  assert.equal(store.dirty.value, true);

  await store.flush();
  assert.equal(attempts, 2);
  assert.equal(store.dirty.value, false);
  await store.dispose();
});
