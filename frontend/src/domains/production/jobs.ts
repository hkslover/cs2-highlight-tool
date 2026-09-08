import type {
  DemoListEntry,
  DemoMaterialSelection,
  GeneratePluginJSONRequest,
} from "@/shared/types";

/** The small part of the import model needed to turn selections into a job. */
export type ProduceJobSource = DemoListEntry;

export interface ProduceFullRoundPOVSelection {
  enabled: boolean;
  player_steam_id: string;
}

export interface ProduceFullRoundPOVPlan {
  player_steam_id?: string;
  segments?: readonly unknown[];
}

export interface BuildProduceJobsOptions {
  demos: readonly ProduceJobSource[];
  getMaterialSelections: (entry: ProduceJobSource) => readonly DemoMaterialSelection[];
  getFullRoundPOVSelection: (entry: ProduceJobSource) => ProduceFullRoundPOVSelection;
  getFullRoundPOVPlan?: (entry: ProduceJobSource) => ProduceFullRoundPOVPlan | undefined;
}

/**
 * Build the backend batch request from the current import selections.
 *
 * Keeping this function independent of Vue makes it usable by both the import
 * and produce features. A POV is included only after the UI has a selected
 * player and a successfully parsed plan with at least one segment.
 */
export function buildProduceJobs({
  demos,
  getMaterialSelections,
  getFullRoundPOVSelection,
  getFullRoundPOVPlan,
}: BuildProduceJobsOptions): GeneratePluginJSONRequest[] {
  return demos
    .map((entry) => {
      const selection = getFullRoundPOVSelection(entry);
      const plan = getFullRoundPOVPlan?.(entry);
      const selectedPlayer = String(selection.player_steam_id || "").trim();
      const plannedPlayer = String(plan?.player_steam_id || "").trim();
      const hasPOVSegments = Boolean(
        selection.enabled &&
          selectedPlayer &&
          plannedPlayer === selectedPlayer &&
          (plan?.segments?.length ?? 0) > 0,
      );

      const selectedItems = getMaterialSelections(entry).map((item) => ({
        kill: item.kill,
        include_killer: item.include_killer,
        include_victim: item.include_victim,
        killer_spec_mode: 1,
        victim_spec_mode: 1,
        primary_view: item.primary_view ?? "killer",
        clip_overrides: item.clip_overrides,
      }));

      return {
        demo_path: entry.file_path,
        tick_rate: entry.meta?.tick_rate ?? 64,
        match_end_tick: entry.meta?.match_end_tick,
        selected_items: selectedItems,
        full_round_pov: hasPOVSegments
          ? { player_steam_id: selectedPlayer }
          : undefined,
      };
    })
    .filter((job) => job.selected_items.length > 0 || Boolean(job.full_round_pov));
}
