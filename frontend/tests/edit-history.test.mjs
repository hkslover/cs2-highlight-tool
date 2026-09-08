import assert from "node:assert/strict";
import test from "node:test";
import {
  groupHistoryByDemo,
  groupHistoryByRound,
  orderHistoryByView,
} from "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/edit/history.js";

function item(overrides = {}) {
  return {
    demo_path: "demo.dem",
    take_index: 0,
    view: "killer",
    spec_mode: 0,
    kill_ids: [],
    video_path: "clip.mp4",
    completed_at_ms: 1,
    ...overrides,
  };
}

test("history grouping keeps demo order and separates round groups", () => {
  const first = item({
    demo_path: "first.dem",
    video_path: "first.mp4",
    kills: [{ id: "k1", round: 2, tick: 20 }],
  });
  const second = item({
    demo_path: "first.dem",
    video_path: "second.mp4",
    kills: [{ id: "k2", round: 1, tick: 10 }],
  });
  const pov = item({
    demo_path: "first.dem",
    video_path: "pov.mp4",
    view: "full_round_pov",
    source_id: "full_round_pov:first.dem:1",
    round: 1,
    start_tick: 5,
    kills: [],
  });

  const demos = groupHistoryByDemo([first, second]);
  assert.deepEqual(demos.map((group) => group.demo_path), ["first.dem"]);
  assert.deepEqual(
    groupHistoryByRound([first, second, pov]).map((group) => group.name),
    ["pov-group", "round-1", "round-2"],
  );
});

test("batch source ordering is POV, killer, then victim by event tick", () => {
  const items = [
    item({ video_path: "victim.mp4", view: "victim", kills: [{ id: "v", tick: 30 }] }),
    item({ video_path: "killer.mp4", view: "killer", kills: [{ id: "k", tick: 20 }] }),
    item({ video_path: "pov.mp4", view: "full_round_pov", start_tick: 10 }),
  ];
  assert.deepEqual(orderHistoryByView(items).map((entry) => entry.video_path), [
    "pov.mp4",
    "killer.mp4",
    "victim.mp4",
  ]);
});
