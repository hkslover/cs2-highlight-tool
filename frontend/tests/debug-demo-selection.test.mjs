import assert from "node:assert/strict";
import test from "node:test";
import {
  buildDebugDemoOptions,
  resolveDebugDemoKey,
} from "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/debug/debugDemoSelection.js";

function entry(key, filePath, file_name = filePath.split(/[\\/]+/).pop()) {
  return { key, file_path: filePath, file_name, loading: false };
}

const entries = [
  entry("demo-1", "/data/demo/raw/a.dem"),
  entry("demo-2", "/data/demo/raw/b.dem"),
];

test("the first initialization adopts the import-page selection", () => {
  assert.equal(resolveDebugDemoKey(entries, null, "demo-2"), "demo-2");
  assert.equal(resolveDebugDemoKey(entries, null, null), "demo-1");
});

test("a valid debug selection is never overridden by the import page", () => {
  assert.equal(resolveDebugDemoKey(entries, "demo-1", "demo-2"), "demo-1");
});

test("a removed or stale selection falls back to the import choice, then the first entry", () => {
  assert.equal(resolveDebugDemoKey(entries, "demo-9", "demo-2"), "demo-2");
  assert.equal(resolveDebugDemoKey(entries, "demo-9", "demo-8"), "demo-1");
  assert.equal(resolveDebugDemoKey([], "demo-9", "demo-2"), null, "an empty list clears the selection");
});

test("same-named demos from different match directories stay distinguishable", () => {
  const options = buildDebugDemoOptions([
    entry("k1", "/data/demo/wanmei/111111/replay.dem"),
    entry("k2", "/data/demo/5e/222222/replay.dem"),
    entry("k3", "/data/demo/raw/unique.dem"),
  ]);
  assert.deepEqual(options, [
    { label: "replay.dem · 111111", value: "k1" },
    { label: "replay.dem · 222222", value: "k2" },
    { label: "unique.dem", value: "k3" },
  ]);
});

test("Windows paths, spaces and Chinese directory names survive label building", () => {
  const options = buildDebugDemoOptions([
    entry("k1", "C:\\Users\\Snow\\demo\\wanmei\\中文 空格\\replay.dem"),
    entry("k2", "C:\\Users\\Snow\\demo\\5e\\222222\\replay.dem"),
  ]);
  assert.deepEqual(options, [
    { label: "replay.dem · 中文 空格", value: "k1" },
    { label: "replay.dem · 222222", value: "k2" },
  ]);
});

test("loading and failed entries are still offered for playback", () => {
  const options = buildDebugDemoOptions([
    { key: "k1", file_path: "/data/demo/raw/x.dem", file_name: "x.dem", loading: true },
    {
      key: "k2",
      file_path: "/data/demo/raw/y.dem",
      file_name: "y.dem",
      loading: false,
      error: "解析失败",
    },
  ]);
  assert.deepEqual(options, [
    { label: "x.dem", value: "k1" },
    { label: "y.dem", value: "k2" },
  ]);
});

test("the debug selection is independent state that a workspace reset clears", async () => {
  const store = await import(
    "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/debug/debugSessionStore.js"
  );
  const { useDebugDemoSelection, ensureDebugDemoSelection, selectDebugDemo, resetDebugSessionForWorkspace } = store;

  resetDebugSessionForWorkspace();
  assert.equal(useDebugDemoSelection().debugDemoKey.value, null);

  // The import-page selection is only the first-time default.
  ensureDebugDemoSelection(entries, "demo-2");
  assert.equal(useDebugDemoSelection().debugDemoKey.value, "demo-2");

  // A choice made on the debug page survives a later import selection change.
  selectDebugDemo("demo-1");
  ensureDebugDemoSelection(entries, "demo-2");
  assert.equal(useDebugDemoSelection().debugDemoKey.value, "demo-1");

  // Entering workspace_init clears the local draft.
  resetDebugSessionForWorkspace();
  assert.equal(useDebugDemoSelection().debugDemoKey.value, null);
  assert.equal(ensureDebugDemoSelection([], "demo-1"), null, "an empty list clears the selection");

  // The debug selection never mutates the shared Demo list.
  assert.deepEqual(
    entries.map((item) => item.key),
    ["demo-1", "demo-2"],
  );

  // Removing the selected entry falls back to the first remaining import.
  ensureDebugDemoSelection(entries, "demo-2");
  ensureDebugDemoSelection(entries.filter((item) => item.key !== "demo-2"));
  assert.equal(useDebugDemoSelection().debugDemoKey.value, "demo-1");
});
