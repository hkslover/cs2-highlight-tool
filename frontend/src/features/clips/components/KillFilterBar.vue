<template>
  <div class="kill-filter">
    <!-- Row 1: who, and from which side. -->
    <div class="head-row">
      <n-select
        class="player-select"
        size="small"
        :value="playerSelectValue"
        :options="playerOptions"
        :placeholder="t('main.clips.player_placeholder')"
        @update:value="onPlayerChange"
      />
      <n-radio-group
        size="small"
        :value="filter.role"
        @update:value="(value: string) => emit('update:role', value as KillPlayerRole)"
      >
        <n-radio-button value="killer">{{ t("main.clips.filter.role_killer") }}</n-radio-button>
        <n-radio-button value="victim">{{ t("main.clips.filter.role_victim") }}</n-radio-button>
      </n-radio-group>
    </div>

    <!-- Row 2: the nine traits worth one click, plus the escape hatch. -->
    <div class="chip-row">
      <n-tag
        v-for="trait in quickTraits"
        :key="trait"
        checkable
        size="small"
        :checked="filter.traits.includes(trait)"
        @update:checked="toggleTrait(trait)"
      >
        {{ traitLabel(trait) }}
      </n-tag>
      <n-button
        class="more-button"
        size="tiny"
        quaternary
        :type="advancedCount ? 'primary' : 'default'"
        @click="advancedOpen = !advancedOpen"
      >
        {{ advancedCount ? t("main.clips.filter.more_badge", { count: advancedCount }) : t("main.clips.filter.more") }}
        <span class="chevron" :class="{ open: advancedOpen }">▾</span>
      </n-button>
    </div>

    <n-collapse-transition :show="advancedOpen">
      <div class="advanced-panel">
        <div class="advanced-field">
          <span class="field-label">{{ t("main.clips.filter.weapon_label") }}</span>
          <div class="weapon-groups">
            <div v-for="group in weaponGroups" :key="group.weaponClass" class="weapon-group">
              <button
                type="button"
                class="class-label"
                :class="{ active: isClassFullySelected(group) }"
                @click="toggleWeaponClass(group)"
              >
                {{ t(`main.clips.filter_weapon_class.${group.weaponClass}`) }}
              </button>
              <div class="weapon-icons">
                <button
                  v-for="weapon in group.weapons"
                  :key="weapon.name"
                  type="button"
                  class="weapon-chip"
                  :class="{ active: filter.weapons.includes(weapon.name) }"
                  :title="`${weaponLabel(weapon.name)} · ${weapon.count}`"
                  @click="toggleWeapon(weapon.name)"
                >
                  <img class="weapon-img" :src="weaponIconSrc(resolveWeaponID(weapon.name))" :alt="weapon.name" />
                  <span class="weapon-count">{{ weapon.count }}</span>
                </button>
              </div>
            </div>
          </div>
        </div>

        <div class="advanced-field">
          <span class="field-label">{{ t("main.clips.filter.other_traits_label") }}</span>
          <div class="chip-row">
            <n-tag
              v-for="trait in advancedTraits"
              :key="trait"
              checkable
              size="small"
              :checked="filter.traits.includes(trait)"
              @update:checked="toggleTrait(trait)"
            >
              {{ traitLabel(trait) }}
            </n-tag>
          </div>
        </div>

        <div class="advanced-row">
          <div class="advanced-field">
            <span class="field-label">{{ t("main.clips.filter.hit_group_label") }}</span>
            <div class="chip-row">
              <n-tag
                v-for="hitGroup in hitGroups"
                :key="hitGroup"
                checkable
                size="small"
                :checked="filter.hit_groups.includes(hitGroup)"
                @update:checked="toggleHitGroup(hitGroup)"
              >
                {{ t(`main.clips.filter_hit_group.${hitGroup}`) }}
              </n-tag>
            </div>
          </div>
          <div class="advanced-field">
            <span class="field-label">{{ t("main.clips.filter.side_label") }}</span>
            <div class="chip-row">
              <n-tag
                v-for="side in ['ct', 't']"
                :key="side"
                checkable
                size="small"
                :checked="filter.sides.includes(side)"
                @update:checked="toggleSide(side)"
              >
                {{ t(`main.clips.filter.side_${side}`) }}
              </n-tag>
            </div>
          </div>
        </div>

        <div class="advanced-row">
          <div class="advanced-field">
            <span class="field-label">
              {{ t("main.clips.filter.round_range_label") }}
              <em class="field-value">{{ roundRange[0] }} – {{ roundRange[1] }}</em>
            </span>
            <n-slider range :min="1" :max="maxRound" :value="roundRange" @update:value="onRoundRangeChange" />
          </div>
          <div class="advanced-field">
            <span class="field-label">
              {{ t("main.clips.filter.distance_label") }}
              <em class="field-value">{{ distanceRange[0] }} – {{ distanceRange[1] }}m</em>
            </span>
            <n-slider range :min="0" :max="maxDistance" :step="1" :value="distanceRange" @update:value="onDistanceChange" />
          </div>
        </div>
      </div>
    </n-collapse-transition>

    <!-- Row 3: result count on the left, the two things you do with it on the right. -->
    <div class="foot-row">
      <span class="match-count">
        {{ t("main.clips.filter.match_count", { matched: matchedCount, total: totalCount }) }}
        <span class="count-sep">·</span>
        {{ t("main.clips.material_summary", { count: selectedCount }) }}
      </span>
      <n-dropdown trigger="click" :options="presetOptions" @select="onPresetSelect">
        <n-button size="tiny" quaternary>{{ t("main.clips.filter.presets_label") }} ▾</n-button>
      </n-dropdown>
      <n-button v-if="filterActive" size="tiny" quaternary @click="emit('clear')">
        {{ t("main.clips.filter.clear") }}
      </n-button>
      <div class="foot-spacer" />
      <n-button size="tiny" type="primary" secondary :disabled="!addableCount" @click="emit('select-all')">
        {{ t("main.clips.filter.select_all", { count: addableCount }) }}
      </n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import {
  NButton,
  NCollapseTransition,
  NDropdown,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NSlider,
  NTag,
  type SelectOption,
} from "naive-ui";
import { t } from "@/shared/i18n";
import type { DemoHitGroup } from "@/shared/types";
import { resolveWeaponID, weaponIconSrc, weaponLabel } from "@/shared/weapon-icons";
import {
  ADVANCED_TRAITS,
  ALL_PLAYERS_VALUE,
  HIT_GROUPS,
  KILL_FILTER_PRESETS,
  QUICK_TRAITS,
  countActiveConditions,
  isKillFilterActive,
  type DemoWeaponGroup,
  type KillFilter,
  type KillFilterPreset,
  type KillPlayerRole,
  type KillTrait,
} from "@/shared/kill-filter";

const props = defineProps<{
  filter: KillFilter;
  playerOptions: SelectOption[];
  playerSteamId: string;
  matchedCount: number;
  totalCount: number;
  addableCount: number;
  selectedCount: number;
  maxRound: number;
  maxDistance: number;
  weaponGroups: DemoWeaponGroup[];
}>();

const emit = defineEmits<{
  (e: "update:role", role: KillPlayerRole): void;
  (e: "update:player", steamID: string): void;
  (e: "update:ignore-player", ignore: boolean): void;
  (e: "update:traits", traits: KillTrait[]): void;
  (e: "update:weapons", weapons: string[]): void;
  (e: "update:hit-groups", hitGroups: DemoHitGroup[]): void;
  (e: "update:sides", sides: string[]): void;
  (e: "update:rounds", rounds: [number, number] | null): void;
  (e: "update:distance", distance: [number, number] | null): void;
  (e: "apply-preset", preset: KillFilterPreset): void;
  (e: "clear"): void;
  (e: "select-all"): void;
}>();

const advancedOpen = ref(false);

const quickTraits = QUICK_TRAITS;
const advancedTraits = ADVANCED_TRAITS;
const hitGroups = HIT_GROUPS;

const filterActive = computed(() => isKillFilterActive(props.filter));

const presetOptions = computed(() =>
  KILL_FILTER_PRESETS.map((preset) => ({
    key: preset.id,
    label: t(`main.clips.filter.preset_${preset.id}`),
  })),
);

// The quick chips are already visible above the toggle, so the badge counts only
// what is folded away behind it — otherwise clicking a chip in plain sight would
// bump a number attached to a panel that has nothing to do with it.
const advancedCount = computed(() => {
  const filter = props.filter;
  let count = countActiveConditions(filter);
  if (filter.traits.length && filter.traits.every((trait) => QUICK_TRAITS.includes(trait))) {
    count--;
  }
  return Math.max(count, 0);
});

const playerSelectValue = computed(() =>
  props.filter.ignore_player ? ALL_PLAYERS_VALUE : props.playerSteamId,
);

const roundRange = computed<[number, number]>(() => props.filter.rounds ?? [1, props.maxRound]);
const distanceRange = computed<[number, number]>(() => props.filter.distance ?? [0, props.maxDistance]);

function traitLabel(trait: KillTrait): string {
  return t(`main.clips.filter_trait.${trait}`);
}

function onPlayerChange(value: string | null) {
  if (value == null) return;
  if (value === ALL_PLAYERS_VALUE) {
    emit("update:ignore-player", true);
    return;
  }
  emit("update:ignore-player", false);
  emit("update:player", value);
}

function onPresetSelect(key: string) {
  const preset = KILL_FILTER_PRESETS.find((item) => item.id === key);
  if (preset) emit("apply-preset", preset);
}

function toggleFrom<T>(list: T[], value: T): T[] {
  return list.includes(value) ? list.filter((item) => item !== value) : list.concat(value);
}

function toggleTrait(trait: KillTrait) {
  emit("update:traits", toggleFrom(props.filter.traits, trait));
}

function toggleWeapon(name: string) {
  emit("update:weapons", toggleFrom(props.filter.weapons, name));
}

function isClassFullySelected(group: DemoWeaponGroup): boolean {
  return group.weapons.every((weapon) => props.filter.weapons.includes(weapon.name));
}

/** The family label is a shortcut for its whole row, in whichever direction. */
function toggleWeaponClass(group: DemoWeaponGroup) {
  const names = group.weapons.map((weapon) => weapon.name);
  if (isClassFullySelected(group)) {
    emit(
      "update:weapons",
      props.filter.weapons.filter((name) => !names.includes(name)),
    );
    return;
  }
  const next = new Set(props.filter.weapons);
  for (const name of names) next.add(name);
  emit("update:weapons", [...next]);
}

function toggleHitGroup(hitGroup: DemoHitGroup) {
  emit("update:hit-groups", toggleFrom(props.filter.hit_groups, hitGroup));
}

function toggleSide(side: string) {
  emit("update:sides", toggleFrom(props.filter.sides, side));
}

// A range covering everything is stored as null rather than as its own bounds,
// so it does not read as an active condition or keep the clear button lit.
function onRoundRangeChange(value: number | [number, number]) {
  const [min, max] = Array.isArray(value) ? value : [value, value];
  emit("update:rounds", min <= 1 && max >= props.maxRound ? null : [min, max]);
}

function onDistanceChange(value: number | [number, number]) {
  const [min, max] = Array.isArray(value) ? value : [value, value];
  emit("update:distance", min <= 0 && max >= props.maxDistance ? null : [min, max]);
}
</script>

<style scoped>
.kill-filter {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.head-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.player-select {
  flex: 1 1 auto;
  min-width: 120px;
}

.chip-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.more-button {
  margin-left: auto;
}

.chevron {
  display: inline-block;
  margin-left: 2px;
  transition: transform 0.2s ease;
}

.chevron.open {
  transform: rotate(180deg);
}

.foot-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.foot-spacer {
  flex: 1 1 auto;
}

.match-count {
  font-size: 12px;
  color: #8d9890;
  white-space: nowrap;
}

.count-sep {
  margin: 0 4px;
  opacity: 0.5;
}

.advanced-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.04);
}

.advanced-row {
  display: flex;
  gap: 16px;
}

.advanced-row > .advanced-field {
  flex: 1 1 0;
  min-width: 0;
}

.advanced-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.field-label {
  font-size: 12px;
  color: #8d9890;
}

.field-value {
  margin-left: 6px;
  font-style: normal;
  color: #c4cdc7;
}

.weapon-groups {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.weapon-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.class-label {
  flex: 0 0 auto;
  width: 52px;
  padding: 2px 0;
  border: none;
  background: none;
  color: #8d9890;
  font-size: 12px;
  text-align: left;
  cursor: pointer;
}

.class-label:hover,
.class-label.active {
  color: #63e2b7;
}

.weapon-icons {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}

.weapon-chip {
  display: flex;
  align-items: center;
  gap: 3px;
  padding: 3px 6px;
  border: 1px solid transparent;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.06);
  cursor: pointer;
}

.weapon-chip:hover {
  background: rgba(255, 255, 255, 0.12);
}

.weapon-chip.active {
  border-color: #63e2b7;
  background: rgba(99, 226, 183, 0.16);
}

.weapon-img {
  height: 14px;
  width: auto;
  filter: brightness(0) invert(1);
  opacity: 0.7;
}

.weapon-chip.active .weapon-img {
  opacity: 1;
}

.weapon-count {
  font-size: 11px;
  color: #8d9890;
}

.weapon-chip.active .weapon-count {
  color: #63e2b7;
}
</style>
