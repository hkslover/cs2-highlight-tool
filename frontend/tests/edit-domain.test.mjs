import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import {
  buildEditConcatRequest,
  createEditDomain,
} from "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/edit/editDomain.js";

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function historyItem(videoPath) {
  return {
    demo_path: "demo.dem",
    take_index: 0,
    view: "killer",
    spec_mode: 0,
    kill_ids: [videoPath],
    video_path: videoPath,
    completed_at_ms: 1,
  };
}

function fakeRuntime() {
  const progressHandlers = [];
  let subscribeCount = 0;
  let unsubscribeCount = 0;
  const calls = [];
  let concat = async () => "C:/outputs/edited.mp4";
  return {
    runtime: {
      onComposeProgress(handler) {
        subscribeCount += 1;
        progressHandlers.push(handler);
        return () => {
          unsubscribeCount += 1;
        };
      },
      concatEditClips(request) {
        calls.push(request);
        return concat(request);
      },
    },
    calls,
    emit(progress) {
      progressHandlers.at(-1)?.(progress);
    },
    emitAt(index, progress) {
      progressHandlers[index]?.(progress);
    },
    setConcat(next) {
      concat = next;
    },
    get subscribeCount() {
      return subscribeCount;
    },
    get unsubscribeCount() {
      return unsubscribeCount;
    },
  };
}

test("request generation preserves clip order and creates fade transitions between clips", () => {
  const items = [
    { videoPath: "a.mp4", duration: 4 },
    { videoPath: "b.mp4", duration: 5 },
    { videoPath: "c.mp4", duration: 6 },
  ];

  assert.deepEqual(
    buildEditConcatRequest(items, "fade", 0.5),
    {
      clips: [
        { video_path: "a.mp4", duration: 4 },
        { video_path: "b.mp4", duration: 5 },
        { video_path: "c.mp4", duration: 6 },
      ],
      transitions: [
        { type: "fade", duration: 0.5, after_index: 0 },
        { type: "fade", duration: 0.5, after_index: 1 },
      ],
    },
  );
  assert.deepEqual(buildEditConcatRequest(items, "none", 0.5).transitions, []);
});

test("compose progress and export completion survive a route change", async () => {
  const fake = fakeRuntime();
  const domain = createEditDomain(fake.runtime);
  domain.addSequenceItem(historyItem("one.mp4"), 4);
  domain.init();

  const completion = deferred();
  fake.setConcat(() => completion.promise);
  const exporting = domain.exportSequence();
  fake.emit({ active: true, percent: 47, current_step: "Muxing", elapsed_ms: 1200 });

  // The edit page can unmount here. No page callback is needed for the
  // operation or event listener to continue updating the application state.
  assert.equal(domain.exporting.value, true);
  assert.equal(domain.composeProgress.value.percent, 47);
  completion.resolve("C:/outputs/route-safe.mp4");
  assert.equal(await exporting, "C:/outputs/route-safe.mp4");
  assert.equal(domain.exportPath.value, "C:/outputs/route-safe.mp4");
  assert.equal(domain.exporting.value, false);
  assert.equal(domain.composeProgress.value.percent, 100);
  assert.equal(fake.subscribeCount, 1);
});

test("edit page unmount leaves the in-flight export owned by the app domain", async () => {
  const pageSource = await readFile(
    new URL("../src/features/edit/composables/useEditPage.ts", import.meta.url),
    "utf8",
  );
  assert.doesNotMatch(pageSource, /disposeEditDomain/);
  assert.match(pageSource, /onBeforeUnmount\(\(\) => \{\s*mounted\.value = false;/s);

  const fake = fakeRuntime();
  const domain = createEditDomain(fake.runtime);
  domain.addSequenceItem(historyItem("one.mp4"), 4);
  domain.init();
  const completion = deferred();
  fake.setConcat(() => completion.promise);

  const exporting = domain.exportSequence();
  // A route unmount only drops page-local UI callbacks. It must not dispose
  // the shared domain, because the next route mount reuses this export.
  domain.init();
  assert.equal(fake.subscribeCount, 1);
  assert.equal(domain.exporting.value, true);

  completion.resolve("C:/outputs/page-seam.mp4");
  assert.equal(await exporting, "C:/outputs/page-seam.mp4");
  assert.equal(domain.exportPath.value, "C:/outputs/page-seam.mp4");
  assert.equal(fake.calls.length, 1);
});

test("dispose and re-init isolate a late completion from the new app lifecycle", async () => {
  const fake = fakeRuntime();
  const domain = createEditDomain(fake.runtime);
  domain.addSequenceItem(historyItem("old.mp4"), 4);
  domain.init();
  fake.emit({ active: true, percent: 19, current_step: "old" });
  const completion = deferred();
  fake.setConcat(() => completion.promise);
  const exporting = domain.exportSequence();

  domain.dispose();
  domain.init();
  fake.emitAt(0, { active: true, percent: 99, current_step: "stale" });
  assert.equal(domain.composeProgress.value.percent, 0);
  completion.resolve("C:/outputs/old-lifecycle.mp4");
  assert.equal(await exporting, "C:/outputs/old-lifecycle.mp4");
  assert.equal(domain.exportPath.value, "");
  assert.equal(domain.exporting.value, false);
  assert.equal(fake.subscribeCount, 2);
  assert.equal(fake.unsubscribeCount, 1);
});

test("a failed event subscription can be retried", () => {
  let attempts = 0;
  const domain = createEditDomain({
    onComposeProgress() {
      attempts += 1;
      if (attempts === 1) throw new Error("runtime not ready");
      return () => undefined;
    },
    concatEditClips: async () => "output.mp4",
  });

  assert.throws(() => domain.init(), /runtime not ready/);
  assert.doesNotThrow(() => domain.init());
  assert.equal(attempts, 2);
});
