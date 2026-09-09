import assert from "node:assert/strict";
import test from "node:test";
import {
  buildOpponentFastEditPlan,
  buildOpponentFastEditRanges,
} from "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/edit/fastEdit.js";
import {
  buildEditConcatRequest,
  createEditDomain,
} from "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/edit/editDomain.js";

function deferred() {
  let resolve;
  const promise = new Promise((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function victimItem({
  path,
  tick,
  recordStart = 1000,
  recordEnd = 2000,
  round = 3,
  historyRound = round,
  killer = "76561198000000001",
  view = "victim",
  offset,
  duration = 5,
}) {
  const kill = {
    id: path,
    tick,
    ...(historyRound === undefined ? {} : { round: historyRound }),
    killer_steam_id: killer,
    killer_name: "attacker",
    victim_name: "target",
  };
  return {
    demo_path: "match.dem",
    take_index: tick,
    view,
    spec_mode: 1,
    round,
    kill_ids: [path],
    kills: [kill],
    tick_rate: 100,
    record_start_tick: recordStart,
    record_end_tick: recordEnd,
    kill_offsets_seconds: [offset ?? (tick - recordStart) / 100],
    video_path: path,
    completed_at_ms: tick,
    duration,
  };
}

function sequenceItem(history) {
  return {
    id: history.video_path,
    historyItem: history,
    videoPath: history.video_path,
    duration: history.duration,
  };
}

test("fast edit trims only adjacent victim takes in the same demo round and killer group", () => {
  const first = victimItem({ path: "one.mp4", tick: 1200, offset: 2 });
  const second = victimItem({ path: "two.mp4", tick: 1300, offset: 3 });
  const third = victimItem({ path: "three.mp4", tick: 1400, offset: 4 });
  const separatedByRound = victimItem({ path: "four.mp4", tick: 1500, offset: 5, round: 4 });
  const items = [first, second, third, separatedByRound].map(sequenceItem);

  const plan = buildOpponentFastEditPlan(items);
  assert.deepEqual(plan.ranges[0], { start_seconds: 1, end_seconds: 2.1 });
  assert.deepEqual(plan.ranges[1], { start_seconds: 2, end_seconds: 3.1 });
  assert.deepEqual(plan.ranges[2], { start_seconds: 3, end_seconds: 4.3 });
  assert.deepEqual(plan.ranges[3], { start_seconds: 4, end_seconds: 5 });
  assert.equal(plan.hardCutAfter[0], true);
  assert.equal(plan.hardCutAfter[1], true);
  assert.equal(plan.hardCutAfter[2], false);
});

test("missing markers, killer/full-round views, and non-adjacent candidates remain full length", () => {
  const valid = victimItem({ path: "valid.mp4", tick: 1200, offset: 2 });
  const missing = { ...victimItem({ path: "missing.mp4", tick: 1300, offset: 3 }), record_end_tick: 0 };
  const killer = victimItem({ path: "killer.mp4", tick: 1400, offset: 4, view: "killer" });
  const pov = victimItem({ path: "pov.mp4", tick: 1500, offset: 5, view: "full_round_pov" });
  const edited = { ...victimItem({ path: "edited.mp4", tick: 1600, offset: 6 }), history_type: "edited_video" };
  const ranges = buildOpponentFastEditRanges(
    [valid, missing, killer, pov, edited].map(sequenceItem),
  );
  assert.deepEqual(ranges, [
    { start_seconds: 1, end_seconds: 2.3 },
    undefined,
    undefined,
    undefined,
    undefined,
  ]);
});

test("rejects a kill marker outside the observed recording window", () => {
  const outside = victimItem({ path: "outside.mp4", tick: 2200, recordEnd: 2000, offset: 4 });
  assert.deepEqual(buildOpponentFastEditRanges([outside].map(sequenceItem)), [undefined]);
});

test("falls back to the kill round and never joins adjacent clips from different rounds", () => {
  const first = victimItem({ path: "round-3.mp4", tick: 1200, round: 3, historyRound: undefined });
  const second = victimItem({ path: "round-4.mp4", tick: 1300, round: 4, historyRound: undefined });
  const plan = buildOpponentFastEditPlan([first, second].map(sequenceItem));

  assert.deepEqual(plan.ranges, [
    { start_seconds: 1, end_seconds: 2.3 },
    { start_seconds: 2, end_seconds: 3.3 },
  ]);
  assert.deepEqual(plan.hardCutAfter, [false]);
});

test("same-tick deaths keep a short overlap and short source clips stay non-empty", () => {
  const sameTick = buildOpponentFastEditPlan([
    victimItem({ path: "same-a.mp4", tick: 1200, offset: 2 }),
    victimItem({ path: "same-b.mp4", tick: 1200, offset: 2 }),
  ].map(sequenceItem));
  assert.deepEqual(sameTick.ranges, [
    { start_seconds: 1, end_seconds: 2.1 },
    { start_seconds: 1.85, end_seconds: 2.3 },
  ]);
  assert.deepEqual(sameTick.hardCutAfter, [true]);

  const short = victimItem({ path: "short.mp4", tick: 1002, recordStart: 1000, offset: 0.02, duration: 0.04 });
  const shortPlan = buildOpponentFastEditPlan([short].map(sequenceItem));
  assert.deepEqual(shortPlan.ranges, [undefined]);
});

test("reversed user order is trimmed independently and rounded ends stay within source duration", () => {
  const reversed = buildOpponentFastEditPlan([
    victimItem({ path: "late.mp4", tick: 1300, offset: 3 }),
    victimItem({ path: "early.mp4", tick: 1200, offset: 2 }),
  ].map(sequenceItem));
  assert.deepEqual(reversed.ranges, [
    { start_seconds: 2, end_seconds: 3.1 },
    { start_seconds: 1, end_seconds: 2.3 },
  ]);
  assert.deepEqual(reversed.hardCutAfter, [true]);

  const tail = victimItem({
    path: "tail.mp4",
    tick: 1500,
    offset: 4.7006,
    duration: 5.0006,
  });
  const tailRange = buildOpponentFastEditRanges([tail].map(sequenceItem))[0];
  assert.ok(tailRange);
  assert.ok(tailRange.start_seconds < tailRange.end_seconds);
  assert.ok(tailRange.end_seconds <= tail.duration);
  assert.equal(tailRange.end_seconds, 5.0006);
});

test("long gaps keep a short lead and mixed roles stay full length and fadeable", () => {
  const first = sequenceItem(victimItem({ path: "first.mp4", tick: 1200, offset: 2 }));
  const longGap = sequenceItem(victimItem({ path: "long-gap.mp4", tick: 1400, offset: 4 }));
  const killer = sequenceItem(victimItem({ path: "killer.mp4", tick: 1500, offset: 5, view: "killer" }));
  const lastVictim = sequenceItem(victimItem({ path: "last.mp4", tick: 1600, offset: 6, duration: 7 }));

  const plan = buildOpponentFastEditPlan([first, longGap, killer, lastVictim]);
  assert.deepEqual(plan.ranges, [
    { start_seconds: 1, end_seconds: 2.1 },
    { start_seconds: 3, end_seconds: 4.3 },
    undefined,
    { start_seconds: 5, end_seconds: 6.3 },
  ]);
  assert.deepEqual(plan.hardCutAfter, [true, false, false]);

  const request = buildEditConcatRequest([first, killer, lastVictim], "fade", 0.5, true);
  assert.deepEqual(request.clips[1], { video_path: "killer.mp4", duration: 5 });
  assert.deepEqual(request.transitions, [
    { type: "fade", duration: 0.5, after_index: 0 },
    { type: "fade", duration: 0.5, after_index: 1 },
  ]);
});

test("fast edit request adds optional trims and keeps fast-edit group gaps hard cut", () => {
  const first = sequenceItem(victimItem({ path: "one.mp4", tick: 1200, offset: 2 }));
  const second = sequenceItem(victimItem({ path: "two.mp4", tick: 1300, offset: 3 }));
  const request = buildEditConcatRequest([first, second], "fade", 0.5, true);
  assert.deepEqual(request.clips[0], {
    video_path: "one.mp4",
    duration: 5,
    start_seconds: 1,
    end_seconds: 2.1,
  });
  assert.deepEqual(request.clips[1], {
    video_path: "two.mp4",
    duration: 5,
    start_seconds: 2,
    end_seconds: 3.3,
  });
  assert.deepEqual(request.transitions, []);
  assert.deepEqual(
    buildEditConcatRequest([first, second], "fade", 0.5).clips,
    [
      { video_path: "one.mp4", duration: 5 },
      { video_path: "two.mp4", duration: 5 },
    ],
  );

  const shortFast = sequenceItem(victimItem({
    path: "short-fast.mp4",
    tick: 1000,
    offset: 0.01,
    duration: 1,
  }));
  const fullLength = sequenceItem(victimItem({
    path: "full-length.mp4",
    tick: 1200,
    offset: 2,
    view: "killer",
  }));
  const safeTransitionRequest = buildEditConcatRequest(
    [shortFast, fullLength],
    "fade",
    0.3,
    true,
  );
  assert.deepEqual(safeTransitionRequest.clips[0], {
    video_path: "short-fast.mp4",
    duration: 1,
    start_seconds: 0,
    end_seconds: 0.31,
  });
  assert.deepEqual(safeTransitionRequest.transitions, []);
});

test("workspace reset clears fast-edit state and detaches an in-flight export", async () => {
  const completion = deferred();
  const domain = createEditDomain({
    onComposeProgress: () => () => undefined,
    concatEditClips: () => completion.promise,
  });
  domain.addSequenceItem(sequenceItem(victimItem({ path: "one.mp4", tick: 1200, offset: 2 })), 5);
  domain.setOpponentFastEditEnabled(true);
  const exporting = domain.exportSequence();
  assert.equal(domain.exporting.value, true);

  domain.resetForWorkspace();
  assert.equal(domain.sequenceItems.value.length, 0);
  assert.equal(domain.opponentFastEditEnabled.value, false);
  assert.equal(domain.exporting.value, false);
  completion.resolve("old-workspace.mp4");
  assert.equal(await exporting, "old-workspace.mp4");
  assert.equal(domain.exportPath.value, "");
});

test("export freezes the sequence, transition, and fast-edit request inputs", async () => {
  const completion = deferred();
  const domain = createEditDomain({
    onComposeProgress: () => () => undefined,
    concatEditClips: (request) => {
      assert.equal(request.clips.length, 1);
      assert.deepEqual(request.transitions, []);
      return completion.promise;
    },
  });
  const first = sequenceItem(victimItem({ path: "one.mp4", tick: 1200, offset: 2 }));
  const second = sequenceItem(victimItem({ path: "two.mp4", tick: 1300, offset: 3 }));
  domain.addSequenceItem(first, 5);
  domain.setOpponentFastEditEnabled(true);
  domain.setTransitionMode("fade");
  domain.setTransitionDuration(0.5);

  const exporting = domain.exportSequence();
  domain.addSequenceItem(second, 5);
  domain.moveSequenceItemDown(0);
  domain.removeSequenceItem(0);
  domain.clearSequence();
  domain.setTransitionMode("none");
  domain.setTransitionDuration(1);
  domain.setOpponentFastEditEnabled(false);

  assert.equal(domain.sequenceItems.value.length, 1);
  assert.equal(domain.transitionMode.value, "fade");
  assert.equal(domain.transitionDuration.value, 0.5);
  assert.equal(domain.opponentFastEditEnabled.value, true);
  completion.resolve("frozen.mp4");
  assert.equal(await exporting, "frozen.mp4");
});
