<template>
  <div class="full-round-pov-section">
    <template v-if="plan?.segments?.length">
      <n-collapse
        :expanded-names="expandedNames"
        @update:expanded-names="$emit('update:expanded', $event)"
      >
        <n-collapse-item
          :name="`${entry.key}-pov`"
          :title="t('main.clips.full_round_pov_group_title_count', { count: plan.segments.length })"
        >
          <template #header-extra>
            <span class="full-round-player">
              {{ t("main.clips.full_round_pov_indicator", { player: trackingLabel }) }}
            </span>
          </template>

          <n-collapse
            :expanded-names="roundExpandedNames"
            @update:expanded-names="$emit('update:round-expanded', $event)"
          >
            <n-collapse-item
              v-for="segment in plan.segments"
              :key="`${entry.key}-pov-r${segment.round}`"
              :name="`r${segment.round}`"
              :title="segmentTitle(segment)"
            >
              <div class="pov-round-kills">
                <template v-if="roundKills(segment.round).length">
                  <DeathNoticeLine
                    v-for="kill in roundKills(segment.round)"
                    :key="kill.id"
                    :kill="kill"
                    compact
                  />
                </template>
                <span v-else class="pov-round-empty">-</span>
              </div>
            </n-collapse-item>
          </n-collapse>
        </n-collapse-item>
      </n-collapse>
    </template>

    <div v-else-if="error" class="full-round-loading full-round-error">
      <span>{{ t("main.clips.full_round_pov_load_failed", { error }) }}</span>
    </div>

    <div v-else-if="plan" class="full-round-loading">
      <span>{{ t("main.clips.full_round_pov_no_kills_empty") }}</span>
    </div>

    <div v-else class="full-round-loading">
      <span>{{ t("main.clips.full_round_pov_loading") }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { NCollapse, NCollapseItem } from "naive-ui";
import { t } from "@/shared/i18n";
import type { DemoClipKill, DemoListEntry, FullRoundPOVSegment } from "@/shared/types";
import DeathNoticeLine from "@/shared/ui/DeathNoticeLine.vue";

interface POVPlanView {
  readonly player_name: string;
  readonly player_steam_id: string;
  readonly segments: readonly FullRoundPOVSegment[];
}

defineProps<{
  entry: DemoListEntry;
  plan?: POVPlanView;
  error?: string;
  trackingLabel: string;
  expandedNames: string[];
  roundExpandedNames: string[];
  roundKills: (round: number) => DemoClipKill[];
  segmentTitle: (segment: Readonly<FullRoundPOVSegment>) => string;
}>();

defineEmits<{
  (event: "update:expanded", value: string | number | Array<string | number> | null): void;
  (event: "update:round-expanded", value: string | number | Array<string | number> | null): void;
}>();
</script>

<style scoped>
.full-round-pov-section {
  margin-bottom: 10px;
}

.pov-round-kills {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 6px 0 2px;
}

.pov-round-empty,
.full-round-loading {
  font-size: 12px;
  color: #8d9890;
}

.full-round-loading {
  padding: 8px 12px;
}

.full-round-error {
  color: #e07f7f;
}

.full-round-player {
  font-size: 12px;
  color: #edf1ee;
}
</style>
