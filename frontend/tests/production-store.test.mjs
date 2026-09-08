import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

function toDataURL(source) {
  return `data:text/javascript;base64,${Buffer.from(source).toString("base64")}`;
}

async function transpile(relativePath) {
  const sourceURL = new URL(relativePath, import.meta.url);
  const source = await readFile(sourceURL, "utf8");
  return ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2020,
      module: ts.ModuleKind.ESNext,
    },
    fileName: sourceURL.pathname,
  }).outputText;
}

const snapshotURL = toDataURL(await transpile("../src/domains/production/snapshot.ts"));
const coreSource = await transpile("../src/domains/production/storeCore.ts");
const coreURL = toDataURL(coreSource.replaceAll('from "./snapshot"', `from "${snapshotURL}"`));
const { createProductionStore } = await import(coreURL);

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function createHarness(overrides = {}, hooks = {}) {
  const listeners = new Map();
  const subscribeCalls = [];
  const unsubscribeCalls = [];
  const defaultSnapshot = {
    ws: { address: "", connected: false, updated_at_ms: 1 },
    queue: {
      running: false,
      total: 0,
      completed: 0,
      current_index: -1,
      pending_ack: false,
      updated_at_ms: 1,
    },
    takes: { items: [], total_takes: 0, started_takes: 0, completed_takes: 0, updated_at_ms: 1 },
    files: { items: [], updated_at_ms: 1 },
    history: { items: [], updated_at_ms: 1 },
  };
  const backend = {
    getWSState: async () => defaultSnapshot.ws,
    getQueueState: async () => defaultSnapshot.queue,
    getTakeSnapshot: async () => defaultSnapshot.takes,
    getTakeFiles: async () => defaultSnapshot.files,
    getHistorySnapshot: async () => defaultSnapshot.history,
    ...overrides,
  };
  const store = createProductionStore({
    backend,
    createCell: (value) => ({ value }),
    subscribe(eventName, callback) {
      hooks.onSubscribe?.(eventName);
      subscribeCalls.push(eventName);
      if (!listeners.has(eventName)) listeners.set(eventName, new Set());
      listeners.get(eventName).add(callback);
      return () => {
        unsubscribeCalls.push(eventName);
        listeners.get(eventName)?.delete(callback);
      };
    },
  });
  return {
    store,
    subscribeCalls,
    unsubscribeCalls,
    emit(eventName, payload) {
      for (const callback of listeners.get(eventName) || []) callback(payload);
    },
  };
}

test("concurrent production init shares reads and keeps an event newer than its snapshot", async () => {
  const wsRead = deferred();
  const order = [];
  const harness = createHarness(
    { getWSState: () => { order.push("read"); return wsRead.promise; } },
    { onSubscribe: () => order.push("subscribe") },
  );

  const first = harness.store.initProductionState();
  const second = harness.store.initProductionState();
  assert.strictEqual(first, second);
  assert.equal(harness.subscribeCalls.length, 5);
  await Promise.resolve();
  assert.deepEqual(order.slice(0, 5), ["subscribe", "subscribe", "subscribe", "subscribe", "subscribe"]);
  assert.equal(order[5], "read");

  harness.emit("produce_ws_state_changed", {
    address: "127.0.0.1:1",
    connected: true,
    updated_at_ms: 20,
  });
  wsRead.resolve({ address: "", connected: false, updated_at_ms: 10 });
  await first;

  assert.equal(harness.store.wsState.value.connected, true);
  assert.equal(harness.store.initialized.value, true);
});

test("a failed history read can retry while its live subscription remains useful", async () => {
  let reads = 0;
  const retryRead = deferred();
  const harness = createHarness({
    getHistorySnapshot: () => {
      reads += 1;
      if (reads === 1) return Promise.reject(new Error("workspace not ready"));
      return retryRead.promise;
    },
  });

  await assert.rejects(harness.store.initProduceHistory(), /workspace not ready/);
  assert.equal(harness.store.historyInitialized.value, false);
  harness.emit("produce_history_changed", {
    items: [{ demo_path: "demo.dem", kill_ids: ["kill-1"] }],
    updated_at_ms: 20,
  });
  assert.equal(harness.store.historySnapshot.value.items[0].kill_ids[0], "kill-1");

  const retry = harness.store.initProduceHistory();
  retryRead.resolve({ items: [], updated_at_ms: 10 });
  await retry;
  assert.equal(reads, 2);
  assert.equal(harness.store.historyInitialized.value, true);
  assert.equal(harness.store.historySnapshot.value.items[0].kill_ids[0], "kill-1");
  assert.equal(harness.subscribeCalls.filter((name) => name === "produce_history_changed").length, 1);
});

test("a synchronous snapshot exception remains retryable", async () => {
  let reads = 0;
  const retryRead = deferred();
  const harness = createHarness({
    getHistorySnapshot: () => {
      reads += 1;
      if (reads === 1) throw new Error("backend bridge not ready");
      return retryRead.promise;
    },
  });

  await assert.rejects(harness.store.initProduceHistory(), /backend bridge not ready/);
  assert.equal(harness.store.historyInitialized.value, false);

  const retry = harness.store.initProduceHistory();
  retryRead.resolve({ items: [], updated_at_ms: 1 });
  await retry;

  assert.equal(reads, 2);
  assert.equal(harness.store.historyInitialized.value, true);
});

test("a failed event subscription can be retried", async () => {
  let subscriptions = 0;
  const harness = createHarness({}, {
    onSubscribe: () => {
      subscriptions += 1;
      if (subscriptions === 1) throw new Error("event bridge not ready");
    },
  });

  await assert.rejects(harness.store.initProduceHistory(), /event bridge not ready/);
  assert.equal(harness.store.historyInitialized.value, false);

  await harness.store.initProduceHistory();
  assert.equal(subscriptions, 2);
  assert.equal(harness.store.historyInitialized.value, true);
});

test("dispose releases handlers and ignores an in-flight response", async () => {
  const pending = deferred();
  const harness = createHarness({ getHistorySnapshot: () => pending.promise });
  const initialization = harness.store.initProduceHistory();
  assert.equal(harness.subscribeCalls.length, 1);

  harness.store.disposeProductionState();
  assert.equal(harness.unsubscribeCalls.length, 1);
  pending.resolve({
    items: [{ demo_path: "late.dem", kill_ids: ["late"] }],
    updated_at_ms: 100,
  });
  await initialization;

  assert.deepEqual(harness.store.historySnapshot.value, { items: [], updated_at_ms: 0 });
  assert.equal(harness.store.historyInitialized.value, false);
  assert.equal(harness.unsubscribeCalls.length, 1);
});

test("dispose starts a new generation and ignores the old response after reinitialization", async () => {
  const firstRead = deferred();
  const secondRead = deferred();
  let reads = 0;
  const harness = createHarness({
    getHistorySnapshot: () => {
      reads += 1;
      return reads === 1 ? firstRead.promise : secondRead.promise;
    },
  });

  const first = harness.store.initProduceHistory();
  harness.store.disposeProductionState();
  const second = harness.store.initProduceHistory();

  firstRead.resolve({
    items: [{ demo_path: "old.dem", kill_ids: ["old"] }],
    updated_at_ms: 100,
  });
  secondRead.resolve({
    items: [{ demo_path: "new.dem", kill_ids: ["new"] }],
    updated_at_ms: 10,
  });
  await Promise.all([first, second]);

  assert.deepEqual(harness.store.historySnapshot.value.items.map((item) => item.demo_path), ["new.dem"]);
  assert.equal(harness.store.historyInitialized.value, true);
  assert.equal(harness.subscribeCalls.filter((name) => name === "produce_history_changed").length, 2);
  assert.equal(harness.unsubscribeCalls.filter((name) => name === "produce_history_changed").length, 1);
});
