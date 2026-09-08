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

const { buildProduceJobs } = await importTypeScript("../src/domains/production/jobs.ts");

const kill = {
  id: "kill-1",
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
};

test("buildProduceJobs preserves role, override, match end, and valid POV data", () => {
  const entry = {
    key: "demo-1",
    file_path: "demo.dem",
    file_name: "demo.dem",
    loading: false,
    meta: { tick_rate: 128, match_end_tick: 9876 },
  };
  const jobs = buildProduceJobs({
    demos: [entry],
    getMaterialSelections: () => [
      {
        kill,
        include_killer: false,
        include_victim: true,
        killer_spec_mode: 1,
        victim_spec_mode: 1,
        primary_view: "victim",
        clip_overrides: { victim_post_seconds: 2.5 },
      },
    ],
    getFullRoundPOVSelection: () => ({
      enabled: true,
      player_steam_id: "76561198000000001",
    }),
    getFullRoundPOVPlan: () => ({
      player_steam_id: "76561198000000001",
      segments: [{ round: 3 }],
    }),
  });

  assert.equal(jobs.length, 1);
  assert.equal(jobs[0].tick_rate, 128);
  assert.equal(jobs[0].match_end_tick, 9876);
  assert.deepEqual(jobs[0].selected_items[0], {
    kill,
    include_killer: false,
    include_victim: true,
    killer_spec_mode: 1,
    victim_spec_mode: 1,
    primary_view: "victim",
    clip_overrides: { victim_post_seconds: 2.5 },
  });
  assert.deepEqual(jobs[0].full_round_pov, {
    player_steam_id: "76561198000000001",
  });
});

test("buildProduceJobs rejects a stale or empty full-round POV plan", () => {
  const entry = {
    key: "demo-2",
    file_path: "empty.dem",
    file_name: "empty.dem",
    loading: false,
    meta: { tick_rate: 64 },
  };
  const jobs = buildProduceJobs({
    demos: [entry],
    getMaterialSelections: () => [],
    getFullRoundPOVSelection: () => ({ enabled: true, player_steam_id: "selected" }),
    getFullRoundPOVPlan: () => ({ player_steam_id: "old-selection", segments: [{ round: 1 }] }),
  });
  assert.deepEqual(jobs, []);
});
