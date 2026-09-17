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
  return import(
    `data:text/javascript;base64,${Buffer.from(outputText).toString("base64")}`
  );
}

const {
  buildRecordedViewIndex,
  pendingSelectionsByDemo,
  projectPendingSelection,
  recordedViewsForKill,
} = await importTypeScript("../src/domains/production/recordedViews.ts");

function makeKill(id, overrides = {}) {
  return {
    id,
    round: 3,
    tick: 120,
    map_name: "de_dust2",
    killer_name: "killer",
    killer_steam_id: "76561198000000001",
    killer_slot: 1,
    killer_entity_id: 1,
    killer_side: "T",
    victim_name: "victim",
    victim_steam_id: "76561198000000002",
    victim_slot: 2,
    victim_entity_id: 2,
    victim_side: "CT",
    weapon_name: "ak47",
    is_headshot: true,
    is_wallbang: false,
    ...overrides,
  };
}

function makeSelection(id, overrides = {}) {
  return {
    kill: makeKill(id),
    include_killer: true,
    include_victim: true,
    killer_spec_mode: 1,
    victim_spec_mode: 1,
    primary_view: "killer",
    ...overrides,
  };
}

function makeHistory(demoPath, view, killIDs, overrides = {}) {
  return {
    demo_path: demoPath,
    take_index: 1,
    view,
    spec_mode: 1,
    kill_ids: killIDs,
    video_path: `${demoPath}-${view}.mp4`,
    completed_at_ms: 100,
    ...overrides,
  };
}

function pendingFor(history, selections, demoPath = "demo-a.dem") {
  const entry = {
    key: demoPath,
    file_path: demoPath,
    file_name: demoPath,
    loading: false,
  };
  const index = buildRecordedViewIndex(history);
  return (
    pendingSelectionsByDemo([entry], () => selections, index).get(demoPath) ||
    []
  );
}

test("projects ordinary selections independently for killer and victim completion", () => {
  const noHistory = pendingFor(
    [],
    [
      makeSelection("killer-only", { include_victim: false }),
      makeSelection("both"),
      makeSelection("legacy-default", {
        include_killer: undefined,
        include_victim: false,
      }),
    ],
  );
  assert.equal(noHistory.length, 3);
  assert.deepEqual(
    noHistory.map(({ include_killer, include_victim }) => ({
      include_killer,
      include_victim,
    })),
    [
      { include_killer: true, include_victim: false },
      { include_killer: true, include_victim: true },
      { include_killer: true, include_victim: false },
    ],
  );

  const killerDone = pendingFor(
    [makeHistory("demo-a.dem", "killer", ["killer-done"])],
    [makeSelection("killer-done", { include_victim: false })],
  );
  assert.deepEqual(killerDone, []);

  const victimRetry = pendingFor(
    [makeHistory("demo-a.dem", "killer", ["dual"])],
    [makeSelection("dual")],
  );
  assert.equal(victimRetry.length, 1);
  assert.equal(victimRetry[0].include_killer, false);
  assert.equal(victimRetry[0].include_victim, true);

  const killerRetry = pendingFor(
    [makeHistory("demo-a.dem", "victim", ["dual"])],
    [makeSelection("dual")],
  );
  assert.equal(killerRetry.length, 1);
  assert.equal(killerRetry[0].include_killer, true);
  assert.equal(killerRetry[0].include_victim, false);

  const allDone = pendingFor(
    [
      makeHistory("demo-a.dem", "killer", ["dual"]),
      makeHistory("demo-a.dem", "victim", ["dual"]),
    ],
    [makeSelection("dual")],
  );
  assert.deepEqual(allDone, []);
});

test("keeps primary view, kill metadata, overrides, and source selection immutable", () => {
  const item = makeSelection("victim-primary", {
    primary_view: "victim",
    clip_overrides: { killer_post_seconds: 2.5, enable_voice: false },
    kill: makeKill("victim-primary", {
      assister_steam_id: "76561198000000003",
      has_assist: true,
    }),
  });
  const before = structuredClone(item);
  const index = buildRecordedViewIndex([
    makeHistory("demo-a.dem", "victim", ["victim-primary"]),
  ]);

  const projected = projectPendingSelection(item, "demo-a.dem", index);
  assert.ok(projected);
  assert.notEqual(projected, item);
  assert.equal(projected.primary_view, "victim");
  assert.equal(projected.include_killer, true);
  assert.equal(projected.include_victim, false);
  assert.deepEqual(projected.kill, item.kill);
  assert.deepEqual(projected.clip_overrides, item.clip_overrides);
  assert.notEqual(projected.clip_overrides, item.clip_overrides);
  assert.deepEqual(item, before);

  const victimOnly = projectPendingSelection(
    makeSelection("victim-only", { include_killer: false }),
    "demo-a.dem",
    buildRecordedViewIndex([]),
  );
  assert.equal(victimOnly?.include_killer, false);
  assert.equal(victimOnly?.include_victim, true);
});

test("indexes every kill in a multi-kill take and supports subset retries", () => {
  const history = [makeHistory("demo-a.dem", "killer", ["k1", "k2"])];
  const selections = [makeSelection("k1"), makeSelection("k2")];
  const pending = pendingFor(history, selections);
  assert.deepEqual(
    pending.map((item) => [
      item.kill.id,
      item.include_killer,
      item.include_victim,
    ]),
    [
      ["k1", false, true],
      ["k2", false, true],
    ],
  );

  const subset = pendingFor(history, [makeSelection("k2")]);
  assert.equal(subset.length, 1);
  assert.equal(subset[0].kill.id, "k2");
  assert.equal(subset[0].include_killer, false);
});

test("is scoped by demo, role, and actual spec mode", () => {
  const history = [
    makeHistory("demo-a.dem", "killer", ["same-id"]),
    makeHistory("demo-b.dem", "killer", ["same-id"]),
    makeHistory("demo-a.dem", "killer", ["mode-two"], { spec_mode: 2 }),
    makeHistory("demo-a.dem", "killer", ["mode-zero"], { spec_mode: 0 }),
    makeHistory("demo-a.dem", "victim", ["legacy-mode"], {
      spec_mode: undefined,
    }),
  ];
  const index = buildRecordedViewIndex(history);

  assert.deepEqual(recordedViewsForKill(index, "demo-a.dem", "same-id"), {
    killer: true,
    victim: false,
  });
  assert.deepEqual(recordedViewsForKill(index, "demo-b.dem", "same-id"), {
    killer: true,
    victim: false,
  });
  assert.deepEqual(recordedViewsForKill(index, "demo-c.dem", "same-id"), {
    killer: false,
    victim: false,
  });
  assert.equal(
    recordedViewsForKill(index, "demo-a.dem", "mode-two").killer,
    false,
  );
  assert.equal(
    recordedViewsForKill(index, "demo-a.dem", "mode-two", {
      killer: 2,
      victim: 1,
    }).killer,
    true,
  );
  assert.equal(
    recordedViewsForKill(index, "demo-a.dem", "mode-zero").killer,
    false,
  );
  assert.equal(
    recordedViewsForKill(index, "demo-a.dem", "legacy-mode").victim,
    true,
  );
});

test("ignores non-ordinary history views and non-production history", () => {
  const history = [
    makeHistory("demo-a.dem", "killer", ["edited"], {
      history_type: "edited_video",
    }),
    makeHistory("demo-a.dem", "full_round_pov", ["pov"]),
    makeHistory("demo-a.dem", "spectator", ["unknown"]),
    makeHistory("demo-a.dem", "", ["empty"]),
    makeHistory("demo-a.dem", "killer", ["no-video"], { video_path: "" }),
  ];
  const index = buildRecordedViewIndex(history);
  for (const id of ["edited", "pov", "unknown", "empty", "no-video"]) {
    assert.deepEqual(recordedViewsForKill(index, "demo-a.dem", id), {
      killer: false,
      victim: false,
    });
  }

  const pending = pendingFor(history, [
    makeSelection("edited"),
    makeSelection("pov"),
    makeSelection("unknown"),
    makeSelection("empty"),
  ]);
  assert.deepEqual(
    pending.map((item) => item.kill.id),
    ["edited", "pov", "unknown", "empty"],
  );
});

test("duplicate history and repeated projections are idempotent, while failed work stays pending", () => {
  const history = [
    makeHistory("demo-a.dem", "killer", ["done"]),
    makeHistory("demo-a.dem", "killer", ["done"]),
  ];
  const index = buildRecordedViewIndex(history);
  assert.deepEqual(recordedViewsForKill(index, "demo-a.dem", "done"), {
    killer: true,
    victim: false,
  });

  const failedOrProcessing = pendingFor([], [makeSelection("retry")]);
  assert.equal(failedOrProcessing.length, 1);
  assert.equal(failedOrProcessing[0].include_killer, true);
  assert.equal(failedOrProcessing[0].include_victim, true);

  const afterSuccess = pendingFor(
    [
      makeHistory("demo-a.dem", "killer", ["retry"]),
      makeHistory("demo-a.dem", "victim", ["retry"]),
    ],
    [makeSelection("retry")],
  );
  assert.deepEqual(afterSuccess, []);
});

test("preserves self-kill role selection semantics", () => {
  const selfKill = makeSelection("suicide", {
    kill: makeKill("suicide", {
      victim_steam_id: "76561198000000001",
      victim_name: "killer",
    }),
    include_victim: false,
  });
  const pending = pendingFor([], [selfKill]);
  assert.equal(pending.length, 1);
  assert.equal(pending[0].include_killer, true);
  assert.equal(pending[0].include_victim, false);
});
