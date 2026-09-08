<template>
  <n-empty
    v-if="!materials.length"
    :description="t('main.clips.no_materials_for_demo')"
    size="small"
  />

  <n-collapse
    v-else
    :expanded-names="expandedRoundNames"
    @update:expanded-names="emit('update:round-expanded', $event)"
  >
    <n-collapse-item
      v-for="group in materialRoundGroups"
      :key="`${entry.key}-round-${group.round}`"
      :name="String(group.round)"
      :title="t('main.clips.round_title', { round: group.round, kills: group.items.length })"
    >
      <n-space vertical :size="8">
        <div
          v-for="item in group.items"
          :key="item.kill.id"
          class="material-row"
          @dblclick="removeMaterialSelection(entry, item.kill.id)"
        >
          <div class="material-head">
            <div class="material-tags-row">
              <n-space align="center" size="small" class="view-tags">
                <n-tag v-if="isPrimaryIncluded(item)" size="small" type="success" :bordered="false">
                  {{ t("main.clips.primary_view_tag") }}
                </n-tag>
                <n-tag v-if="isOpponentIncluded(item)" size="small" type="warning" :bordered="false">
                  {{ t("main.clips.opponent_view_tag") }}
                </n-tag>
              </n-space>
              <n-tag
                v-if="isKillAlreadyProduced(item.kill.id)"
                size="small"
                type="warning"
                :bordered="false"
              >
                {{ t("main.clips.already_produced") }}
              </n-tag>
              <div class="material-actions">
                <n-button
                  text
                  size="small"
                  class="expand-btn"
                  @click.stop="toggleMaterialSettings(item.kill.id)"
                  @dblclick.stop
                >
                  {{ isMaterialSettingsExpanded(item.kill.id) ? t("main.clips.collapse") : t("main.clips.expand") }}
                  {{ isMaterialSettingsExpanded(item.kill.id) ? "▾" : "▸" }}
                </n-button>
                <button
                  type="button"
                  class="material-delete-btn"
                  :title="t('main.clips.remove_material')"
                  @click.stop="removeMaterialSelection(entry, item.kill.id)"
                  @dblclick.stop
                >
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none">
                    <path d="M18 6L6 18M6 6l12 12" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"/>
                  </svg>
                </button>
              </div>
            </div>
            <div class="material-meta">
              <DeathNoticeLine :kill="item.kill" compact />
            </div>
          </div>

          <div
            v-if="isMaterialSettingsExpanded(item.kill.id)"
            class="material-settings"
            @dblclick.stop
          >
            <div class="setting-row">
              <n-checkbox
                :checked="isOpponentIncluded(item)"
                :disabled="isSelfKill(item.kill)"
                @update:checked="handleOpponentEnabledChange(item, !!$event)"
              >
                {{ t("main.clips.opponent_enabled") }}
              </n-checkbox>
              <span v-if="isSelfKill(item.kill)" class="setting-hint">
                {{ t("main.clips.self_kill_no_opponent") }}
              </span>
            </div>
            <template v-if="isPrimaryIncluded(item)">
              <div class="setting-row">
                <span class="setting-label">{{ t("main.settings.killer_pre_seconds") }}</span>
                <n-input-number
                  :value="positionSeconds(item, 'primary', 'pre')"
                  :min="1"
                  :max="20"
                  :step="0.5"
                  :precision="1"
                  @update:value="handleSecondsChange(item, 'primary', 'pre', $event)"
                />
              </div>
              <div class="setting-row">
                <span class="setting-label">{{ t("main.settings.killer_post_seconds") }}</span>
                <n-input-number
                  :value="positionSeconds(item, 'primary', 'post')"
                  :min="1"
                  :max="20"
                  :step="0.5"
                  :precision="1"
                  @update:value="handleSecondsChange(item, 'primary', 'post', $event)"
                />
              </div>
            </template>
            <template v-if="isOpponentIncluded(item)">
              <div class="setting-row">
                <span class="setting-label">{{ t("main.settings.victim_pre_seconds") }}</span>
                <n-input-number
                  :value="positionSeconds(item, 'opponent', 'pre')"
                  :min="1"
                  :max="20"
                  :step="0.5"
                  :precision="1"
                  @update:value="handleSecondsChange(item, 'opponent', 'pre', $event)"
                />
              </div>
              <div class="setting-row">
                <span class="setting-label">{{ t("main.settings.victim_post_seconds") }}</span>
                <n-input-number
                  :value="positionSeconds(item, 'opponent', 'post')"
                  :min="1"
                  :max="20"
                  :step="0.5"
                  :precision="1"
                  @update:value="handleSecondsChange(item, 'opponent', 'post', $event)"
                />
              </div>
            </template>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.settings.enable_voice") }}</span>
              <n-switch
                :value="effectiveBooleanValue(item, 'enable_voice')"
                @update:value="handleVoiceEnabledChange(item.kill.id, !!$event)"
              />
            </div>
            <div class="setting-row">
              <span class="setting-label">{{ t("main.settings.enable_spec_show_xray_zero") }}</span>
              <n-switch
                :value="effectiveBooleanValue(item, 'enable_spec_show_xray_zero')"
                @update:value="handleXrayEnabledChange(item.kill.id, !!$event)"
              />
            </div>
          </div>
        </div>
      </n-space>
    </n-collapse-item>
  </n-collapse>
</template>

<script setup lang="ts">
import { computed } from "vue";
import {
  NButton,
  NCheckbox,
  NCollapse,
  NCollapseItem,
  NEmpty,
  NInputNumber,
  NSpace,
  NSwitch,
  NTag,
} from "naive-ui";
import { t } from "@/shared/i18n";
import DeathNoticeLine from "@/shared/ui/DeathNoticeLine.vue";
import type {
  ClipParameterOverrides,
  ClipSettings,
  DemoListEntry,
  DemoMaterialSelection,
} from "@/shared/types";
import {
  isOpponentIncluded,
  isPrimaryIncluded,
  isSelfKill,
  roleOfPosition,
  type ViewPosition,
  type WindowEdge,
} from "@/shared/clip-views";
import {
  removeMaterialSelection,
  updateMaterialClipOverrides,
  updateMaterialIncludeKiller,
  updateMaterialIncludeVictim,
} from "@/domains/clip-selection";

type ClipOverrideNumberKey =
  | "killer_pre_seconds"
  | "killer_post_seconds"
  | "victim_pre_seconds"
  | "victim_post_seconds";
type ClipOverrideBooleanKey = "enable_voice" | "enable_spec_show_xray_zero";

const props = defineProps<{
  entry: DemoListEntry;
  materials: readonly DemoMaterialSelection[];
  expandedRoundNames: string[];
  settingsExpandedIds: string[];
  clipSettings: ClipSettings;
  isKillAlreadyProduced: (killID: string) => boolean;
}>();

const emit = defineEmits<{
  (event: "update:round-expanded", value: string | number | Array<string | number> | null): void;
  (event: "update:settings-expanded", value: string[]): void;
}>();

const materialRoundGroups = computed(() => {
  const grouped = new Map<number, DemoMaterialSelection[]>();
  for (const item of props.materials) {
    const round = item.kill.round;
    if (!grouped.has(round)) grouped.set(round, []);
    grouped.get(round)!.push(item);
  }
  return Array.from(grouped.entries())
    .sort((a, b) => a[0] - b[0])
    .map(([round, items]) => ({ round, items }));
});

function isMaterialSettingsExpanded(killID: string): boolean {
  return props.settingsExpandedIds.includes(killID);
}

function toggleMaterialSettings(killID: string): void {
  const next = isMaterialSettingsExpanded(killID)
    ? props.settingsExpandedIds.filter((id) => id !== killID)
    : props.settingsExpandedIds.concat(killID);
  emit("update:settings-expanded", next);
}

function handleOpponentEnabledChange(item: DemoMaterialSelection, checked: boolean): void {
  if (roleOfPosition(item, "opponent") === "killer") {
    updateMaterialIncludeKiller(props.entry, item.kill.id, checked);
    return;
  }
  updateMaterialIncludeVictim(props.entry, item.kill.id, checked);
}

function overrideKeyFor(
  item: DemoMaterialSelection,
  position: ViewPosition,
  edge: WindowEdge,
): ClipOverrideNumberKey {
  return `${roleOfPosition(item, position)}_${edge}_seconds` as ClipOverrideNumberKey;
}

function settingsKeyFor(position: ViewPosition, edge: WindowEdge): ClipOverrideNumberKey {
  return position === "primary"
    ? (`killer_${edge}_seconds` as ClipOverrideNumberKey)
    : (`victim_${edge}_seconds` as ClipOverrideNumberKey);
}

function positionSeconds(
  item: DemoMaterialSelection,
  position: ViewPosition,
  edge: WindowEdge,
): number {
  const overrideValue = item.clip_overrides?.[overrideKeyFor(item, position, edge)];
  if (typeof overrideValue === "number" && Number.isFinite(overrideValue)) return overrideValue;
  return props.clipSettings[settingsKeyFor(position, edge)];
}

function handleSecondsChange(
  item: DemoMaterialSelection,
  position: ViewPosition,
  edge: WindowEdge,
  value: number | null,
): void {
  if (typeof value !== "number" || !Number.isFinite(value)) return;
  updateMaterialClipOverrides(props.entry, item.kill.id, {
    [overrideKeyFor(item, position, edge)]: value,
  });
}

function effectiveBooleanValue(item: DemoMaterialSelection, key: ClipOverrideBooleanKey): boolean {
  const overrideValue = item.clip_overrides?.[key];
  return typeof overrideValue === "boolean" ? overrideValue : !!props.clipSettings[key];
}

function handleVoiceEnabledChange(killID: string, checked: boolean): void {
  updateMaterialClipOverrides(props.entry, killID, { enable_voice: checked });
}

function handleXrayEnabledChange(killID: string, checked: boolean): void {
  updateMaterialClipOverrides(props.entry, killID, { enable_spec_show_xray_zero: checked });
}
</script>

<style scoped>
.material-row {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid #28312b;
  border-radius: 8px;
  background: #151816;
  transition: border-color 0.18s ease, background 0.18s ease;
}

.material-row:hover {
  border-color: #38453d;
  background: #181d1a;
}

.material-head,
.material-settings {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.material-tags-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  flex-wrap: wrap;
}

.material-meta,
.view-tags {
  width: 100%;
  min-width: 0;
}

.material-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 0 0 auto;
}

.expand-btn {
  flex: 0 0 auto;
  font-size: 12px;
}

.material-delete-btn {
  background: transparent;
  border: none;
  border-radius: 4px;
  color: #7d8881;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  padding: 0;
  transition: background 0.15s ease, color 0.15s ease;
}

.material-delete-btn:hover {
  background: rgba(224, 83, 83, 0.16);
  color: #f28b8b;
}

.material-settings {
  border: 1px solid #323b35;
  border-radius: 8px;
  padding: 10px;
  background: rgba(26, 32, 28, 0.5);
  gap: 8px;
}

.setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.setting-label,
.setting-hint {
  font-size: 12px;
  color: #a7b2aa;
}

.setting-hint {
  color: #8d9890;
}
</style>
