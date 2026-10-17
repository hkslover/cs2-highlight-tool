import assert from "node:assert/strict";
import test from "node:test";

// The store reaches the backend through the dynamic Wails boundary, so a fake
// App object installed before the import is enough to drive the real store,
// its Vue watcher and the preview debounce.
let clipSettings = createClipSettings();
let settingsHandler = async () => clipSettings;
let planHandler = async () => createPlan("auto");

globalThis.window = {
  go: {
    app: {
      App: {
        GetClipSettings: () => settingsHandler(),
        GetDebugConsolePlan: (draft) => planHandler(draft),
      },
    },
  },
};

const { useDebugSettingsDraft, resetDebugSettingsForWorkspace } = await import(
  "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/debug/debugSettingsStore.js"
);

function createClipSettings() {
  return {
    killer_pre_seconds: 4,
    killer_post_seconds: 4,
    victim_pre_seconds: 2,
    victim_post_seconds: 2,
    auto_add_victim_view: true,
    enable_voice: true,
    record_fps: 60,
    record_quality: "high",
    motion_blur_enabled: false,
    edit_fps: 60,
    edit_quality: "high",
    video_preset: "auto",
    launch_resolution: "4:3",
    record_output_dir: "",
    enable_spec_show_xray_zero: true,
    hide_all_ui: false,
    hide_player_avatars: false,
    hide_chat_messages: true,
    use_shoulder_camera: false,
    pov_hud_enabled: true,
    pov_radar_enabled: false,
    sky_blackout: true,
    disable_clouds: false,
    kill_feed_lifetime: 4,
    pov_kill_feed_only: false,
    block_kill_feed: false,
    noflash_strength: 0,
  };
}

function createPlan(label) {
  return {
    settings: [],
    apply_all: [label],
    recording: [],
    effective_video_preset: "c1",
    effective_video_encoder: "libx264",
    effective_video_status: "manual",
    record_output_dir: "/data/outputs/debug",
  };
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function loadStore() {
  resetDebugSettingsForWorkspace();
  settingsHandler = async () => clipSettings;
  const settings = useDebugSettingsDraft();
  planHandler = async () => createPlan("initial");
  assert.equal(await settings.loadDebugSettings(), true);
  return settings;
}

async function waitFor(predicate, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (!predicate()) {
    if (Date.now() > deadline) {
      throw new Error("timeout waiting for condition");
    }
    await sleep(5);
  }
}

test("an apply right after an edit is not stolen by the preview debounce", async () => {
  const settings = await loadStore();
  assert.equal(settings.plan.value.apply_all[0], "initial");

  const requests = [];
  planHandler = (draft) =>
    new Promise((resolve) => {
      requests.push({ draft, resolve });
    });

  settings.patchDraft({ record_fps: 45 });
  const applyPromise = settings.applyDebugPlan();
  await sleep(0);
  // The explicit apply canceled the pending preview timer, so exactly one
  // request exists and it carries the newest draft.
  assert.equal(requests.length, 1);
  assert.equal(requests[0].draft.record_fps, 45);

  requests[0].resolve(createPlan("applied"));
  const applied = await applyPromise;
  assert.equal(applied.apply_all[0], "applied");

  // The canceled preview must not fire later and invalidate the applied plan.
  await sleep(250);
  assert.equal(requests.length, 1);
});

test("the apply still wins when an edit lands while it waits for the plan", async () => {
  const settings = await loadStore();

  const requests = [];
  planHandler = (draft) =>
    new Promise((resolve) => {
      requests.push({ draft, resolve });
    });

  const applyPromise = settings.applyDebugPlan();
  await sleep(0);
  assert.equal(requests.length, 1);
  settings.patchDraft({ record_fps: 90 });

  // The edit must not start a competing preview request while the apply waits.
  await sleep(250);
  assert.equal(requests.length, 1);

  requests[0].resolve(createPlan("applied"));
  const applied = await applyPromise;
  assert.equal(applied.apply_all[0], "applied");

  // The newer draft still gets its preview once the apply settled.
  await waitFor(() => requests.length === 2);
  assert.equal(requests[1].draft.record_fps, 90);
  requests[1].resolve(createPlan("preview"));
});

test("rapid edits coalesce into a single preview request", async () => {
  const settings = await loadStore();

  const requests = [];
  planHandler = (draft) =>
    new Promise((resolve) => {
      requests.push({ draft, resolve });
    });

  settings.patchDraft({ record_fps: 100 });
  settings.patchDraft({ record_fps: 110 });
  await waitFor(() => requests.length === 1);
  assert.equal(requests[0].draft.record_fps, 110);
  requests[0].resolve(createPlan("preview"));
  await sleep(220);
  assert.equal(requests.length, 1);
});

test("a reload during an apply cannot steal the apply's plan", async () => {
  const settings = await loadStore();

  const requests = [];
  planHandler = (draft) =>
    new Promise((resolve) => {
      requests.push({ draft, resolve });
    });

  // The user clicks apply first; the request waits for the plugin plan.
  const applyPromise = settings.applyDebugPlan();
  await sleep(0);
  assert.equal(requests.length, 1);

  // The user clicks reload while the apply waits. Its settings read resolves
  // immediately, but its plan refresh must queue behind the apply instead of
  // bumping the request version and turning the apply into a stale null.
  clipSettings = { ...createClipSettings(), record_fps: 120 };
  const reloadPromise = settings.reloadDebugSettings();
  await sleep(250);
  assert.equal(requests.length, 1);

  requests[0].resolve(createPlan("applied"));
  const applied = await applyPromise;
  assert.equal(applied.apply_all[0], "applied");

  // The queued reload then refreshes the preview for the reloaded draft.
  await waitFor(() => requests.length === 2);
  assert.equal(requests[1].draft.record_fps, 120);
  requests[1].resolve(createPlan("reloaded"));
  assert.equal(await reloadPromise, true);
  assert.equal(settings.plan.value.apply_all[0], "reloaded");
});

test("a workspace reset drops queued explicit refreshes", async () => {
  const settings = await loadStore();

  const requests = [];
  planHandler = (draft) =>
    new Promise((resolve) => {
      requests.push({ draft, resolve });
    });

  const applyPromise = settings.applyDebugPlan();
  await sleep(0);
  assert.equal(requests.length, 1);

  clipSettings = { ...createClipSettings(), record_fps: 120 };
  const reloaded = settings.reloadDebugSettings();
  // Let the reload's settings read settle so its plan refresh is queued.
  await sleep(20);
  assert.equal(requests.length, 1);

  resetDebugSettingsForWorkspace();
  requests[0].resolve(createPlan("applied"));

  // The reset invalidates the in-flight plan and drops the queued refresh
  // without issuing another backend request.
  assert.equal(await applyPromise, null);
  assert.equal(await reloaded, true);
  await sleep(250);
  assert.equal(requests.length, 1);
  assert.equal(settings.plan.value, null);
});

test("a new workspace initializes before an old plan response arrives", async () => {
  const settings = await loadStore();
  const requests = [];
  planHandler = (draft) =>
    new Promise((resolve) => {
      requests.push({ draft, resolve });
    });

  const oldApply = settings.applyDebugPlan();
  await waitFor(() => requests.length === 1);
  try {
    resetDebugSettingsForWorkspace();
    clipSettings = { ...createClipSettings(), record_fps: 90 };
    const newLoad = settings.loadDebugSettings();

    // The old response stays pending while the new workspace builds its plan.
    await waitFor(() => requests.length === 2);
    assert.equal(requests[1].draft.record_fps, 90);
    requests[1].resolve(createPlan("new-workspace"));
    assert.equal(await newLoad, true);
    assert.equal(settings.plan.value.apply_all[0], "new-workspace");

    requests[0].resolve(createPlan("old-workspace"));
    assert.equal(await oldApply, null);
    assert.equal(settings.plan.value.apply_all[0], "new-workspace");
    assert.equal(settings.draft.value.record_fps, 90);
  } finally {
    requests[0].resolve(createPlan("old-workspace"));
    await oldApply;
    resetDebugSettingsForWorkspace();
  }
});

test("a failed explicit apply surfaces an error without sending a plan", async () => {
  const settings = await loadStore();

  planHandler = async () => {
    throw new Error("计划构建失败");
  };
  settings.patchDraft({ record_fps: 45 });
  const plan = await settings.applyDebugPlan();
  assert.equal(plan, null);
  assert.equal(settings.planErrorMessage.value, "计划构建失败");
  // The previous plan is kept for the next valid apply.
  assert.equal(settings.plan.value.apply_all[0], "initial");
});
