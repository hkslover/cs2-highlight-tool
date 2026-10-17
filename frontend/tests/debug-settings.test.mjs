import assert from "node:assert/strict";
import test from "node:test";
import {
  createDebugSettingsCore,
  debugSettingCommandsFor,
  debugSettingsDraftFromClipSettings,
  emptyDebugSettingsDraft,
} from "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/debug/debugSettingsCore.js";

function createCell(initial) {
  return { value: initial };
}

const confirmedSettings = {
  pov_radar_enabled: true,
  sky_blackout: false,
  disable_clouds: true,
  hide_all_ui: true,
  hide_player_avatars: true,
  hide_chat_messages: false,
  enable_spec_show_xray_zero: false,
  enable_voice: false,
  use_shoulder_camera: true,
  kill_feed_lifetime: 7,
  pov_kill_feed_only: true,
  block_kill_feed: true,
  noflash_strength: 0.35,
  record_fps: 120,
  record_quality: "ultra",
  video_preset: "n1",
  launch_resolution: "16:9",
};

function createPlan(overrides = {}) {
  return {
    settings: [
      {
        key: "sky_blackout",
        commands: ["r_drawskybox 1"],
        on: ["r_drawskybox 0"],
        off: ["r_drawskybox 1"],
      },
      { key: "kill_feed_lifetime", commands: ["mirv_deathmsg lifetime 4"] },
    ],
    apply_all: ["r_drawskybox 1", "mirv_deathmsg lifetime 4"],
    recording: ["mirv_streams record fps 60"],
    effective_video_preset: "n1",
    effective_video_encoder: "hevc_nvenc",
    effective_video_status: "ready",
    record_output_dir: "/data/outputs/debug",
    ...overrides,
  };
}

function createCore(overrides = {}) {
  const calls = { clipSettings: 0, plans: [] };
  const backend = {
    getClipSettings: async () => {
      calls.clipSettings += 1;
      return confirmedSettings;
    },
    getConsolePlan: async (draft) => {
      calls.plans.push(draft);
      return createPlan();
    },
    ...overrides.backend,
  };
  const core = createDebugSettingsCore({ createCell, backend });
  return { core, calls };
}

test("derives the draft from the confirmed settings subset", () => {
  const draft = debugSettingsDraftFromClipSettings(confirmedSettings);
  assert.equal(draft.pov_radar_enabled, true);
  assert.equal(draft.sky_blackout, false);
  assert.equal(draft.disable_clouds, true);
  assert.equal(draft.hide_chat_messages, false);
  assert.equal(draft.enable_spec_show_xray_zero, false);
  assert.equal(draft.enable_voice, false);
  assert.equal(draft.kill_feed_lifetime, 7);
  assert.equal(draft.pov_kill_feed_only, true);
  assert.equal(draft.noflash_strength, 0.35);
  assert.equal(draft.record_fps, 120);
  assert.equal(draft.record_quality, "ultra");
  assert.equal(draft.video_preset, "n1");
  assert.equal(draft.launch_resolution, "16:9");
  assert.equal(Object.keys(draft).length, Object.keys(emptyDebugSettingsDraft()).length);
});

test("load reads the formal settings into the draft and reports success", async () => {
  const { core, calls } = createCore();
  const ok = await core.load();
  assert.equal(ok, true);
  assert.equal(core.loaded.value, true);
  assert.equal(core.loading.value, false);
  assert.equal(core.errorMessage.value, "");
  assert.equal(core.sourceSettings.value, confirmedSettings);
  assert.equal(core.draft.value.record_fps, 120);
  assert.equal(calls.clipSettings, 1);
});

test("a failed load keeps the placeholder and never writes back", async () => {
  const { core } = createCore({
    backend: {
      getClipSettings: async () => {
        throw new Error("设置读取失败");
      },
    },
  });
  const ok = await core.load();
  assert.equal(ok, false);
  assert.equal(core.loaded.value, false);
  assert.equal(core.errorMessage.value, "设置读取失败");
  assert.equal(core.sourceSettings.value, null);
  assert.equal(core.plan.value, null);
  assert.deepEqual(core.draft.value, emptyDebugSettingsDraft());
});

test("reload replaces edited draft fields with the confirmed settings", async () => {
  const { core } = createCore();
  await core.load();
  core.patchDraft({ record_fps: 30, sky_blackout: true, block_kill_feed: false });
  assert.equal(core.draft.value.record_fps, 30);

  const ok = await core.load(true);
  assert.equal(ok, true);
  assert.equal(core.draft.value.record_fps, 120);
  assert.equal(core.draft.value.sky_blackout, false);
  assert.equal(core.draft.value.block_kill_feed, true);
});

test("a non-forced load keeps the session debug draft", async () => {
  const { core, calls } = createCore();
  await core.load();
  core.patchDraft({ record_fps: 120, sky_blackout: false, video_preset: "n1" });

  // A route switch re-mounts the page and runs the initializer again.
  const ok = await core.load();
  assert.equal(ok, true);
  assert.equal(core.draft.value.record_fps, 120);
  assert.equal(core.draft.value.sky_blackout, false);
  assert.equal(core.draft.value.video_preset, "n1");
  assert.equal(calls.clipSettings, 1);
});

test("a failed reload keeps the confirmed draft and plan", async () => {
  let fail = false;
  const { core } = createCore({
    backend: {
      getClipSettings: async () => {
        if (fail) throw new Error("重新读取失败");
        return confirmedSettings;
      },
    },
  });
  await core.load();
  await core.refreshPlan();
  const confirmedPlan = core.plan.value;
  core.patchDraft({ record_fps: 45 });

  fail = true;
  const ok = await core.load(true);
  assert.equal(ok, false);
  assert.equal(core.errorMessage.value, "重新读取失败");
  assert.equal(core.loaded.value, true);
  assert.equal(core.draft.value.record_fps, 45);
  assert.equal(core.sourceSettings.value, confirmedSettings);
  assert.equal(core.plan.value, confirmedPlan);
});

test("refreshPlan sends the current draft and stores the plan", async () => {
  const { core, calls } = createCore();
  await core.load();
  core.patchDraft({ record_fps: 45 });
  const plan = await core.refreshPlan();
  assert.equal(plan.record_output_dir, "/data/outputs/debug");
  assert.equal(calls.plans.length, 1);
  assert.equal(calls.plans[0].record_fps, 45);
  assert.equal(core.plan.value, plan);
});

test("a stale plan response cannot overwrite a newer plan", async () => {
  const resolvers = [];
  const { core } = createCore({
    backend: {
      getConsolePlan: () =>
        new Promise((resolve) => {
          resolvers.push(resolve);
        }),
    },
  });
  const first = core.refreshPlan();
  const second = core.refreshPlan();
  assert.equal(resolvers.length, 2);

  const newer = createPlan({ record_output_dir: "/newer" });
  resolvers[1](newer);
  assert.equal(await second, newer);
  assert.equal(core.plan.value.record_output_dir, "/newer");

  resolvers[0](createPlan({ record_output_dir: "/stale" }));
  assert.equal(await first, null);
  assert.equal(core.plan.value.record_output_dir, "/newer");
  assert.equal(core.planLoading.value, false);
});

test("a plan failure keeps the previous plan and reports the error", async () => {
  let fail = false;
  const { core } = createCore({
    backend: {
      getConsolePlan: async () => {
        if (fail) throw new Error("计划构建失败");
        return createPlan();
      },
    },
  });
  const first = await core.refreshPlan();
  assert.equal(core.plan.value, first);

  fail = true;
  const second = await core.refreshPlan();
  assert.equal(second, null);
  assert.equal(core.planErrorMessage.value, "计划构建失败");
  assert.equal(core.plan.value, first);
});

test("toggle commands come from the backend plan variants", () => {
  const plan = createPlan();
  const sky = plan.settings.find((item) => item.key === "sky_blackout");
  assert.deepEqual(debugSettingCommandsFor(sky, true), ["r_drawskybox 0"]);
  assert.deepEqual(debugSettingCommandsFor(sky, false), ["r_drawskybox 1"]);

  const lifetime = plan.settings.find((item) => item.key === "kill_feed_lifetime");
  assert.deepEqual(debugSettingCommandsFor(lifetime, true), ["mirv_deathmsg lifetime 4"]);
  assert.equal(debugSettingCommandsFor(undefined, true).length, 0);
});

test("dispose drops an in-flight load and clears local facts", async () => {
  let resolveLoad;
  const { core } = createCore({
    backend: {
      getClipSettings: () =>
        new Promise((resolve) => {
          resolveLoad = resolve;
        }),
    },
  });
  const load = core.load();
  core.dispose();
  resolveLoad(confirmedSettings);
  assert.equal(await load, false);
  assert.equal(core.loaded.value, false);
  assert.equal(core.sourceSettings.value, null);
  assert.equal(core.errorMessage.value, "");
  assert.deepEqual(core.draft.value, emptyDebugSettingsDraft());
});

test("dispose drops an in-flight plan response", async () => {
  let resolvePlan;
  const { core } = createCore({
    backend: {
      getConsolePlan: () =>
        new Promise((resolve) => {
          resolvePlan = resolve;
        }),
    },
  });
  const refresh = core.refreshPlan();
  core.dispose();
  resolvePlan(createPlan());
  assert.equal(await refresh, null);
  assert.equal(core.plan.value, null);
});
