import assert from "node:assert/strict";
import test from "node:test";
import {
  backend,
  BackendMethodUnavailableError,
  BackendUnavailableError,
  callBackend,
} from "../node_modules/.cache/cs2-highlight-tool-backend-tests/shared/backend/adapter.js";

function installApp(App) {
  globalThis.window = { go: { app: { App } } };
}

function clearWindow() {
  Reflect.deleteProperty(globalThis, "window");
}

test("reports an explicit error before the Wails API is loaded", async () => {
  clearWindow();

  await assert.rejects(callBackend("GetStartupState"), (error) => {
    assert.ok(error instanceof BackendUnavailableError);
    assert.equal(error.code, "BACKEND_UNAVAILABLE");
    return true;
  });
});

test("reports an explicit error when a mapped method is missing", async () => {
  installApp({});

  await assert.rejects(callBackend("GetStartupState"), (error) => {
    assert.ok(error instanceof BackendMethodUnavailableError);
    assert.equal(error.code, "BACKEND_METHOD_UNAVAILABLE");
    assert.equal(error.method, "GetStartupState");
    return true;
  });
});

test("forwards the mapped argument tuple and preserves the resolved value", async () => {
  const request = { jobs: [{ demo_path: "demo.dem", tick_rate: 64, selected_items: [] }] };
  const response = { results: [], success_count: 1, failure_count: 0 };
  let received;

  installApp({
    GeneratePluginJSONBatch: async (...args) => {
      received = args;
      return response;
    },
  });

  const result = await callBackend("GeneratePluginJSONBatch", request);
  assert.strictEqual(result, response);
  assert.deepEqual(received, [request]);
});

test("named adapter methods use the same live App boundary", async () => {
  let received;
  installApp({
    AckChangelog: async (...args) => {
      received = args;
    },
  });

  await backend.AckChangelog("2026.09.08");
  assert.deepEqual(received, ["2026.09.08"]);
});

test("propagates a backend rejection without hiding it", async () => {
  const failure = new Error("backend failure");
  installApp({
    ProbeClipDuration: async () => {
      throw failure;
    },
  });

  await assert.rejects(backend.ProbeClipDuration("clip.mp4"), failure);
});
