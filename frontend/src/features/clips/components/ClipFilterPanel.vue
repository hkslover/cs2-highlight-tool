<template>
  <div class="select-toolbar">
    <template v-if="fullRoundPOVEnabled">
      <n-grid :cols="24" :x-gap="12" :y-gap="8">
        <n-gi :span="14">
          <n-select
            :value="selectedPlayerSteamID"
            :options="playerOptions"
            :placeholder="t('main.clips.player_placeholder')"
            @update:value="emit('player-change', $event)"
          />
        </n-gi>
        <n-gi :span="10">
          <div class="summary-box">
            <n-text depth="3">
              {{ t("main.clips.material_summary", { count: selectedCount }) }}
            </n-text>
          </div>
        </n-gi>
      </n-grid>
      <div class="mode-switch-row">
        <span class="switch-hint">{{ t("main.clips.filter.disabled_in_pov") }}</span>
      </div>
    </template>

    <template v-else>
      <KillFilterBar
        :filter="filter"
        :player-options="playerOptions"
        :player-steam-id="selectedPlayerSteamID"
        :matched-count="matchedCount"
        :total-count="totalCount"
        :addable-count="addableCount"
        :selected-count="selectedCount"
        :max-round="maxRound"
        :max-distance="maxDistance"
        :weapon-groups="weaponGroups"
        @update:role="emit('role-change', $event)"
        @update:player="emit('player-change', $event)"
        @update:ignore-player="emit('ignore-player-change', $event)"
        @update:traits="emit('traits-change', $event)"
        @update:weapons="emit('weapons-change', $event)"
        @update:hit-groups="emit('hit-groups-change', $event)"
        @update:sides="emit('sides-change', $event)"
        @update:rounds="emit('rounds-change', $event)"
        @update:distance="emit('distance-change', $event)"
        @apply-preset="emit('apply-preset', $event)"
        @clear="emit('clear')"
        @select-all="emit('select-all')"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import {
  NGi,
  NGrid,
  NSelect,
  NText,
  type SelectOption,
} from "naive-ui";
import { t } from "@/shared/i18n";
import type { DemoHitGroup } from "@/shared/types";
import {
  type DemoWeaponGroup,
  type KillFilter,
  type KillFilterPreset,
  type KillPlayerRole,
  type KillTrait,
} from "@/shared/kill-filter";
import KillFilterBar from "@/shared/ui/KillFilterBar.vue";

const props = defineProps<{
  fullRoundPOVEnabled: boolean;
  selectedPlayerSteamID: string;
  playerOptions: SelectOption[];
  filter: KillFilter;
  matchedCount: number;
  totalCount: number;
  addableCount: number;
  selectedCount: number;
  maxRound: number;
  maxDistance: number;
  weaponGroups: DemoWeaponGroup[];
}>();

const emit = defineEmits<{
  (event: "player-change", value: string | number | null): void;
  (event: "role-change", value: KillPlayerRole): void;
  (event: "ignore-player-change", value: boolean): void;
  (event: "traits-change", value: KillTrait[]): void;
  (event: "weapons-change", value: string[]): void;
  (event: "hit-groups-change", value: DemoHitGroup[]): void;
  (event: "sides-change", value: string[]): void;
  (event: "rounds-change", value: [number, number] | null): void;
  (event: "distance-change", value: [number, number] | null): void;
  (event: "apply-preset", value: KillFilterPreset): void;
  (event: "clear"): void;
  (event: "select-all"): void;
}>();

// Keep props referenced in script for Volar's strict template inference.
void props;
</script>

<style scoped>
.select-toolbar {
  flex: 0 0 auto;
}

.mode-switch-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 8px;
}

.switch-hint {
  color: #8d9890;
  font-size: 12px;
}

.summary-box {
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: flex-end;
}
</style>
