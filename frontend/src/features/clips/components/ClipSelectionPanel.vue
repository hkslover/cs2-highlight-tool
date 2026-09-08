<template>
  <n-card
    class="right-card"
    :bordered="true"
    content-style="height: 100%; overflow: hidden; padding: 0;"
    content-class="right-card-content"
  >
    <div class="panel-head">
      <span class="panel-title">{{ t("main.clips.select_title") }}</span>
      <div class="panel-actions">
        <span class="switch-label">{{ t("main.clips.full_round_pov_switch") }}</span>
        <n-switch
          size="small"
          :value="fullRoundPOVEnabled"
          @update:value="emit('pov-toggle', $event)"
        />
      </div>
    </div>
    <div class="right-card-body">
      <n-empty v-if="!activeDemoEntry" class="right-empty" :description="t('main.clips.no_demo')" />

      <template v-else>
        <ClipFilterPanel
          :full-round-p-o-v-enabled="fullRoundPOVEnabled"
          :selected-player-steam-i-d="selectedPlayerSteamID"
          :player-options="playerOptions"
          :filter="killFilter"
          :matched-count="matchedCount"
          :total-count="totalCount"
          :addable-count="addableCount"
          :selected-count="selectedCount"
          :max-round="maxRound"
          :max-distance="maxDistance"
          :weapon-groups="weaponGroups"
          @player-change="emit('player-change', $event)"
          @role-change="emit('role-change', $event)"
          @ignore-player-change="emit('ignore-player-change', $event)"
          @traits-change="emit('traits-change', $event)"
          @weapons-change="emit('weapons-change', $event)"
          @hit-groups-change="emit('hit-groups-change', $event)"
          @sides-change="emit('sides-change', $event)"
          @rounds-change="emit('rounds-change', $event)"
          @distance-change="emit('distance-change', $event)"
          @apply-preset="emit('apply-preset', $event)"
          @clear="emit('clear')"
          @select-all="emit('select-all')"
        />

        <n-scrollbar class="select-scroll" trigger="none">
          <n-empty v-if="!currentRounds.length" :description="emptyKillDescription" />

          <n-collapse v-else :expanded-names="expandedRounds" @update:expanded-names="emit('rounds-expanded', $event)">
            <n-collapse-item
              v-for="round in currentRounds"
              :key="round.round"
              :name="String(round.round)"
              :title="t('main.clips.round_title', { round: round.round, kills: round.kills.length })"
            >
              <n-space vertical :size="8">
                <div
                  v-for="kill in round.kills"
                  :key="kill.id"
                  class="kill-row"
                  :class="{ selected: isKillSelected(kill.id) }"
                  @click="emit('toggle-kill', kill)"
                >
                  <div class="kill-line">
                    <DeathNoticeLine :kill="kill" />
                  </div>
                  <n-tag
                    v-if="isKillAlreadyProduced(activeDemoEntry.file_path, kill.id)"
                    size="small"
                    type="warning"
                    :bordered="false"
                  >
                    {{ t("main.clips.already_produced") }}
                  </n-tag>
                </div>
              </n-space>
            </n-collapse-item>
          </n-collapse>
        </n-scrollbar>
      </template>
    </div>
  </n-card>
</template>

<script setup lang="ts">
import {
  NCard,
  NCollapse,
  NCollapseItem,
  NEmpty,
  NScrollbar,
  NSpace,
  NSwitch,
  NTag,
} from "naive-ui";
import { t } from "@/shared/i18n";
import type {
  DemoClipKill,
  DemoClipRound,
  DemoListEntry,
} from "@/shared/types";
import type { DemoWeaponGroup, KillFilter, KillFilterPreset, KillPlayerRole, KillTrait } from "@/shared/kill-filter";
import DeathNoticeLine from "@/shared/ui/DeathNoticeLine.vue";
import ClipFilterPanel from "./ClipFilterPanel.vue";
import type { SelectOption } from "naive-ui";

type ExpandedNames = string | number | Array<string | number> | null;

const props = defineProps<{
  activeDemoEntry: DemoListEntry | null;
  fullRoundPOVEnabled: boolean;
  selectedPlayerSteamID: string;
  playerOptions: SelectOption[];
  killFilter: KillFilter;
  matchedCount: number;
  totalCount: number;
  addableCount: number;
  selectedCount: number;
  maxRound: number;
  maxDistance: number;
  weaponGroups: DemoWeaponGroup[];
  currentRounds: DemoClipRound[];
  expandedRounds: string[];
  emptyKillDescription: string;
  isKillSelected: (killID: string) => boolean;
  isKillAlreadyProduced: (demoPath: string, killID: string) => boolean;
}>();

const emit = defineEmits<{
  (event: "pov-toggle", value: boolean): void;
  (event: "player-change", value: string | number | null): void;
  (event: "role-change", value: KillPlayerRole): void;
  (event: "ignore-player-change", value: boolean): void;
  (event: "traits-change", value: KillTrait[]): void;
  (event: "weapons-change", value: string[]): void;
  (event: "hit-groups-change", value: import("@/shared/types").DemoHitGroup[]): void;
  (event: "sides-change", value: string[]): void;
  (event: "rounds-change", value: [number, number] | null): void;
  (event: "distance-change", value: [number, number] | null): void;
  (event: "apply-preset", value: KillFilterPreset): void;
  (event: "clear"): void;
  (event: "select-all"): void;
  (event: "rounds-expanded", value: ExpandedNames): void;
  (event: "toggle-kill", value: DemoClipKill): void;
}>();

void props;
</script>

<style scoped>
.right-card {
  background: #181b19;
  height: 100%;
  max-height: 100%;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.right-card :deep(.right-card-content) {
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

.panel-actions {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.switch-label {
  color: #aab4ad;
  font-size: 12px;
  white-space: nowrap;
}

.right-card-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  padding: 8px 10px 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.right-empty {
  margin-top: 8px;
}

.select-scroll {
  flex: 1;
  min-height: 0;
}

.kill-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid #28312b;
  border-radius: 8px;
  background: #151816;
  cursor: pointer;
  user-select: none;
  transition: border-color 0.15s ease, background 0.15s ease, transform 0.15s ease;
}

.kill-row:hover {
  border-color: rgba(47, 181, 114, 0.45);
  background: #1a201c;
  transform: translateX(2px);
}

.kill-row.selected {
  border-color: #2fb572;
  background: linear-gradient(90deg, rgba(47, 181, 114, 0.14) 0%, rgba(20, 24, 21, 0.6) 100%);
  box-shadow: 0 0 10px rgba(47, 181, 114, 0.12), inset 0 0 0 1px rgba(47, 181, 114, 0.2);
}

.kill-line {
  flex: 1;
  min-width: 0;
}
</style>
