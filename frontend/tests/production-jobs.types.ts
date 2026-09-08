import { buildProduceJobs } from "../src/domains/production/jobs";
import type { DemoListEntry, GeneratePluginJSONRequest } from "../src/shared/types";

const entry = {} as DemoListEntry;
const jobs: GeneratePluginJSONRequest[] = buildProduceJobs({
  demos: [entry],
  getMaterialSelections: () => [],
  getFullRoundPOVSelection: () => ({ enabled: false, player_steam_id: "" }),
  getFullRoundPOVPlan: () => undefined,
});

void jobs;

