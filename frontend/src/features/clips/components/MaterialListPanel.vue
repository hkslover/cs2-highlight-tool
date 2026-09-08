<template>
  <n-card
    class="left-card"
    :bordered="true"
    content-style="height: 100%; overflow: hidden; padding: 0;"
    content-class="left-card-content"
  >
    <div class="panel-head">
      <span class="panel-title">{{ t("main.clips.material_list_title") }}</span>
    </div>
    <div class="card-body">
      <n-empty v-if="!clipReadyDemos.length" :description="t('main.clips.no_demo')" />

      <n-collapse
        v-else
        accordion
        :expanded-names="expandedDemoNames"
        @update:expanded-names="emit('update:expanded-demos', $event)"
      >
        <n-collapse-item
          v-for="entry in clipReadyDemos"
          :key="entry.key"
          :name="entry.key"
          :title="entry.file_name"
        >
          <template #header-extra>
            <n-space align="center" size="small">
              <n-tag size="small">{{ materialCount(entry) }}</n-tag>
              <n-tag
                v-if="getPOVSelection(entry).enabled"
                size="small"
                type="info"
                :bordered="false"
              >
                {{ t("main.clips.full_round_pov_tag") }}
              </n-tag>
              <n-tag
                v-if="producedCount(entry) > 0"
                size="small"
                type="warning"
                :bordered="false"
              >
                {{ t("main.clips.produced_count", { count: producedCount(entry) }) }}
              </n-tag>
              <n-button
                v-if="canClearMaterials(entry)"
                size="tiny"
                type="error"
                secondary
                class="clear-materials-btn"
                @click.stop="emit('clear-materials', entry)"
              >
                {{ t("main.clips.clear_selected") }}
              </n-button>
            </n-space>
          </template>

          <FullRoundPOVPreview
            v-if="getPOVSelection(entry).enabled"
            :entry="entry"
            :plan="getPOVPlan(entry)"
            :error="getPOVError(entry)"
            :tracking-label="getPOVTrackingLabel(entry)"
            :expanded-names="getPOVExpanded(entry)"
            :round-expanded-names="getPOVRoundExpanded(entry)"
            :round-kills="(round) => povRoundKills(entry, round)"
            :segment-title="segmentTitleFor(entry)"
            @update:expanded="emit('update:pov-expanded', entry, $event)"
            @update:round-expanded="emit('update:pov-round-expanded', entry, $event)"
          />

          <MaterialSelectionList
            v-if="!getPOVSelection(entry).enabled || materials(entry).length"
            :entry="entry"
            :materials="materials(entry)"
            :expanded-round-names="getMaterialExpanded(entry)"
            :settings-expanded-ids="getMaterialSettingsExpanded(entry)"
            :clip-settings="clipSettings"
            :is-kill-already-produced="(killID) => isKillAlreadyProduced(entry, killID)"
            @update:round-expanded="emit('update:material-round-expanded', entry, $event)"
            @update:settings-expanded="emit('update:material-settings-expanded', entry, $event)"
          />
        </n-collapse-item>
      </n-collapse>
    </div>
  </n-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { NButton, NCard, NCollapse, NCollapseItem, NEmpty, NSpace, NTag } from "naive-ui";
import { t } from "@/shared/i18n";
import type {
  ClipSettings,
  DemoClipKill,
  DemoListEntry,
  DemoMaterialSelection,
  FullRoundPOVSegment,
} from "@/shared/types";
import FullRoundPOVPreview from "./FullRoundPOVPreview.vue";
import MaterialSelectionList from "./MaterialSelectionList.vue";

interface POVSelectionView {
  enabled: boolean;
  player_steam_id: string;
}

interface POVPlanView {
  readonly player_name: string;
  readonly player_steam_id: string;
  readonly segments: readonly FullRoundPOVSegment[];
}

type ExpandedNames = string | number | Array<string | number> | null;

const props = defineProps<{
  clipReadyDemos: readonly DemoListEntry[];
  expandedDemoNames: string[];
  clipSettings: ClipSettings;
  materialCount: (entry: DemoListEntry) => number;
  materials: (entry: DemoListEntry) => readonly DemoMaterialSelection[];
  getPOVSelection: (entry: DemoListEntry) => POVSelectionView;
  getPOVPlan: (entry: DemoListEntry) => POVPlanView | undefined;
  getPOVError: (entry: DemoListEntry) => string | undefined;
  getPOVTrackingLabel: (entry: DemoListEntry) => string;
  producedCount: (entry: DemoListEntry) => number;
  canClearMaterials: (entry: DemoListEntry) => boolean;
  isKillAlreadyProduced: (entry: DemoListEntry, killID: string) => boolean;
  getPOVExpanded: (entry: DemoListEntry) => string[];
  getPOVRoundExpanded: (entry: DemoListEntry) => string[];
  povRoundKills: (entry: DemoListEntry, round: number) => DemoClipKill[];
  povSegmentTitle: (entry: DemoListEntry, segment: Readonly<FullRoundPOVSegment>) => string;
  getMaterialExpanded: (entry: DemoListEntry) => string[];
  getMaterialSettingsExpanded: (entry: DemoListEntry) => string[];
}>();

const emit = defineEmits<{
  (event: "update:expanded-demos", value: ExpandedNames): void;
  (event: "clear-materials", entry: DemoListEntry): void;
  (event: "update:pov-expanded", entry: DemoListEntry, value: ExpandedNames): void;
  (event: "update:pov-round-expanded", entry: DemoListEntry, value: ExpandedNames): void;
  (event: "update:material-round-expanded", entry: DemoListEntry, value: ExpandedNames): void;
  (event: "update:material-settings-expanded", entry: DemoListEntry, value: string[]): void;
}>();

// Keep the list reactive when import parsing finishes after this component has
// mounted. A plain destructuring assignment would capture only the initial
// array and leave the panel stuck on the empty state.
const clipReadyDemos = computed(() => props.clipReadyDemos);

function segmentTitleFor(entry: DemoListEntry): (segment: Readonly<FullRoundPOVSegment>) => string {
  return (segment) => props.povSegmentTitle(entry, segment);
}
</script>

<style scoped>
.left-card {
  background: #181b19;
  height: 100%;
  max-height: 100%;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.left-card :deep(.left-card-content) {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.panel-head {
  flex-shrink: 0;
  min-height: 34px;
  padding: 6px 10px;
  border-bottom: 1px solid #303732;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.panel-title {
  font-size: 13px;
  font-weight: 600;
}

.card-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 8px 10px 10px;
}

.clear-materials-btn {
  font-size: 12px;
  flex: 0 0 auto;
}
</style>
