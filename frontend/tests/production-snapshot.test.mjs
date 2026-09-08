import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function importTypeScript(relativePath) {
  const sourceURL = new URL(relativePath, import.meta.url);
  const source = await readFile(sourceURL, "utf8");
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2020,
      module: ts.ModuleKind.ESNext,
    },
    fileName: sourceURL.pathname,
  });
  return import(`data:text/javascript;base64,${Buffer.from(outputText).toString("base64")}`);
}

const { applyProductionSnapshot } = await importTypeScript("../src/domains/production/snapshot.ts");

function emptySnapshot() {
  return { connected: false, updated_at_ms: 0 };
}

test("a delayed initial snapshot cannot overwrite a newer event", () => {
  const state = {
    value: emptySnapshot(),
    latestUpdatedAt: 0,
  };

  applyProductionSnapshot(state, { connected: true, updated_at_ms: 200 }, "event", emptySnapshot);
  applyProductionSnapshot(state, { connected: false, updated_at_ms: 100 }, "snapshot", emptySnapshot);
  applyProductionSnapshot(state, { connected: false, updated_at_ms: 200 }, "snapshot", emptySnapshot);

  assert.deepEqual(state.value, { connected: true, updated_at_ms: 200 });
  assert.equal(state.latestUpdatedAt, 200);
  assert.equal(state.latestSource, "event");
});

test("a later snapshot is accepted and older events remain ignored", () => {
  const state = {
    value: emptySnapshot(),
    latestUpdatedAt: 0,
  };

  applyProductionSnapshot(state, { connected: true, updated_at_ms: 10 }, "event", emptySnapshot);
  applyProductionSnapshot(state, { connected: false, updated_at_ms: 11 }, "snapshot", emptySnapshot);
  applyProductionSnapshot(state, { connected: true, updated_at_ms: 10 }, "event", emptySnapshot);

  assert.deepEqual(state.value, { connected: false, updated_at_ms: 11 });
  assert.equal(state.latestUpdatedAt, 11);
  assert.equal(state.latestSource, "snapshot");
});
