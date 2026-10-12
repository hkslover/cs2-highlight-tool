import assert from "node:assert/strict";
import test from "node:test";
import {
  buildDebugQuickCommandOptions,
  DEBUG_QUICK_COMMAND_GROUPS,
} from "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/debug/debugQuickCommands.js";

function createPlan(overrides = {}) {
  return {
    settings: [
      {
        key: "use_shoulder_camera",
        commands: ["c_thirdpersonshoulder 0;cam_command 2"],
        on: ["c_thirdpersonshoulder 1;cam_command 2"],
        off: ["c_thirdpersonshoulder 0;cam_command 2"],
      },
      {
        key: "disable_clouds",
        commands: ["mirv_sky clouds draw 1"],
        on: ["mirv_sky clouds draw 0"],
        off: ["mirv_sky clouds draw 1"],
      },
      {
        key: "sky_blackout",
        commands: ["r_drawskybox 1"],
        on: ["r_drawskybox 0"],
        off: ["r_drawskybox 1"],
      },
      {
        key: "pov_radar_enabled",
        commands: ["csdm_radar_pov 0"],
        on: ["csdm_radar_pov 1"],
        off: ["csdm_radar_pov 0"],
      },
      { key: "kill_feed_lifetime", commands: ["mirv_deathmsg lifetime 7"] },
      {
        key: "block_kill_feed",
        commands: ["mirv_deathmsg filter clear"],
        on: ["mirv_deathmsg filter add block 1"],
        off: ["mirv_deathmsg filter clear"],
      },
      {
        key: "enable_spec_show_xray_zero",
        commands: ["spec_show_xray 0"],
        on: ["spec_show_xray 0"],
        off: ["spec_show_xray 1"],
      },
      {
        key: "hide_all_ui",
        commands: ["cl_draw_only_deathnotices 0"],
        on: ["cl_draw_only_deathnotices 1"],
        off: ["cl_draw_only_deathnotices 0"],
      },
      {
        key: "hide_player_avatars",
        commands: ["cl_teamcounter_playercount_instead_of_avatars false"],
        on: ["cl_teamcounter_playercount_instead_of_avatars true"],
        off: ["cl_teamcounter_playercount_instead_of_avatars false"],
      },
      {
        key: "hide_chat_messages",
        commands: ["hidehud 0"],
        on: ["hidehud 128"],
        off: ["hidehud 0"],
      },
      {
        key: "enable_voice",
        commands: ["tv_listen_voice_indices 4294967295", "tv_listen_voice_indices_h 4294967295"],
        on: ["tv_listen_voice_indices 0", "tv_listen_voice_indices_h 0"],
        off: ["tv_listen_voice_indices 4294967295", "tv_listen_voice_indices_h 4294967295"],
      },
    ],
    apply_all: ["r_drawskybox 0", "mirv_deathmsg lifetime 7"],
    recording: [],
    effective_video_preset: "c1",
    effective_video_encoder: "libx264",
    effective_video_status: "manual",
    record_output_dir: "/data/outputs/debug",
    ...overrides,
  };
}

function findOption(options, key) {
  return options.find((option) => option.key === key);
}

test("every switch option takes its command from the backend on/off variant", () => {
  const options = buildDebugQuickCommandOptions(createPlan());

  assert.equal(findOption(options, "sky_blackout:on").command, "r_drawskybox 0");
  assert.equal(findOption(options, "sky_blackout:off").command, "r_drawskybox 1");
  assert.equal(findOption(options, "disable_clouds:on").command, "mirv_sky clouds draw 0");
  assert.equal(findOption(options, "disable_clouds:off").command, "mirv_sky clouds draw 1");
  assert.equal(findOption(options, "hide_all_ui:on").command, "cl_draw_only_deathnotices 1");
  assert.equal(findOption(options, "hide_all_ui:off").command, "cl_draw_only_deathnotices 0");
  assert.equal(
    findOption(options, "hide_player_avatars:on").command,
    "cl_teamcounter_playercount_instead_of_avatars true",
  );
  assert.equal(findOption(options, "hide_chat_messages:on").command, "hidehud 128");
  assert.equal(findOption(options, "hide_chat_messages:off").command, "hidehud 0");
  assert.equal(findOption(options, "enable_spec_show_xray_zero:on").command, "spec_show_xray 0");
  assert.equal(findOption(options, "enable_spec_show_xray_zero:off").command, "spec_show_xray 1");
  assert.equal(findOption(options, "use_shoulder_camera:on").command, "c_thirdpersonshoulder 1;cam_command 2");
  assert.equal(findOption(options, "use_shoulder_camera:off").command, "c_thirdpersonshoulder 0;cam_command 2");
  assert.equal(findOption(options, "pov_radar_enabled:on").command, "csdm_radar_pov 1");
  assert.equal(findOption(options, "pov_radar_enabled:off").command, "csdm_radar_pov 0");
  assert.equal(findOption(options, "block_kill_feed:on").command, "mirv_deathmsg filter add block 1");
  assert.equal(findOption(options, "block_kill_feed:off").command, "mirv_deathmsg filter clear");
});

test("the lifetime option uses the current plan value and apply-all is not a quick command", () => {
  const options = buildDebugQuickCommandOptions(createPlan());
  assert.equal(findOption(options, "kill_feed_lifetime:current").command, "mirv_deathmsg lifetime 7");
  // The plan still carries `apply_all` for the backend contract, but the
  // console dropdown no longer exposes an "apply draft picture/sound" option.
  assert.equal(findOption(options, "apply_all"), undefined);
  assert.equal(options.some((option) => option.command.includes("r_drawskybox 0; ")), false);
});

test("multi-command entries keep plan order and join with a semicolon", () => {
  const options = buildDebugQuickCommandOptions(createPlan());
  assert.equal(
    findOption(options, "enable_voice:on").command,
    "tv_listen_voice_indices 0; tv_listen_voice_indices_h 0",
  );
  assert.equal(
    findOption(options, "enable_voice:off").command,
    "tv_listen_voice_indices 4294967295; tv_listen_voice_indices_h 4294967295",
  );
  // A command that already contains a semicolon keeps it verbatim.
  assert.equal(findOption(options, "use_shoulder_camera:off").command.includes(";cam_command 2"), true);
});

test("options are grouped for the dropdown", () => {
  const options = buildDebugQuickCommandOptions(createPlan());
  assert.equal(findOption(options, "sky_blackout:on").group, "picture");
  assert.equal(findOption(options, "enable_voice:on").group, "sound");
  assert.deepEqual([...DEBUG_QUICK_COMMAND_GROUPS], ["picture", "sound"]);
});

test("a missing plan or missing/empty rows never produce an empty command", () => {
  assert.deepEqual(buildDebugQuickCommandOptions(null), []);

  const emptyPlan = createPlan({
    settings: [{ key: "sky_blackout", commands: [], on: [], off: [] }],
    apply_all: [],
  });
  assert.deepEqual(buildDebugQuickCommandOptions(emptyPlan), []);

  // Only the rows present in the plan generate options; a partial plan must not
  // fabricate the missing ones from a frontend default.
  const partial = buildDebugQuickCommandOptions(
    createPlan({
      settings: [
        { key: "sky_blackout", commands: [], on: ["r_drawskybox 0"], off: ["r_drawskybox 1"] },
      ],
      apply_all: [],
    }),
  );
  assert.deepEqual(
    partial.map((option) => option.key),
    ["sky_blackout:on", "sky_blackout:off"],
  );
});

test("on/off variants win over the current-value commands", () => {
  const plan = createPlan();
  const options = buildDebugQuickCommandOptions(plan);
  const sky = plan.settings.find((item) => item.key === "sky_blackout");
  assert.equal(sky.commands[0], "r_drawskybox 1");
  assert.equal(findOption(options, "sky_blackout:on").command, sky.on[0]);
  assert.equal(findOption(options, "sky_blackout:off").command, sky.off[0]);
});
