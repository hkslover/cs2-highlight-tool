<template>
  <div class="death-notice" :class="{ compact }">
    <span :class="['name', sideClass(kill.killer_side)]">{{ kill.killer_name }}</span>
    <img v-if="weaponID" :src="weaponIcon(weaponID)" class="weapon-icon" alt="weapon" />
    <span class="weapon-name">{{ weaponLabel(kill.weapon_name) }}</span>
    <img v-if="kill.is_headshot" :src="iconSrc('headshot')" class="suffix-icon" alt="headshot" />
    <img v-if="kill.is_wallbang" :src="iconSrc('penetrate')" class="suffix-icon" alt="penetrate" />
    <span :class="['name', sideClass(kill.victim_side)]">{{ kill.victim_name }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { DemoClipKill } from "@/shared/types";
import {
  deathNoticeIconSrc,
  resolveWeaponID,
  weaponIconSrc,
  weaponLabel,
} from "@/shared/weapon-icons";

const props = withDefaults(
  defineProps<{
    kill: DemoClipKill;
    compact?: boolean;
  }>(),
  {
    compact: false,
  },
);

const weaponID = computed(() => resolveWeaponID(props.kill.weapon_name));

function weaponIcon(id: string): string {
  return weaponIconSrc(id);
}

function iconSrc(name: string): string {
  return deathNoticeIconSrc(name);
}

function sideClass(side: string): string {
  const normalized = (side || "").toLowerCase();
  if (normalized === "ct") return "side-ct";
  if (normalized === "t") return "side-t";
  return "side-t";
}
</script>

<style scoped>
.death-notice {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  overflow: hidden;
  padding: 6px 10px;
  border-radius: 6px;
  background: rgba(0, 0, 0, 0.55);
  color: #fff;
  font-size: 13px;
  font-weight: 700;
  line-height: 1;
}

.death-notice.compact {
  padding: 4px 8px;
  font-size: 12px;
}

.name {
  min-width: 0;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.side-ct {
  color: #6f9ce6;
}

.side-t {
  color: #eabe54;
}

.weapon-icon {
  height: 18px;
  width: auto;
  flex: 0 0 auto;
  filter: brightness(0) invert(1);
}

.weapon-name {
  min-width: 0;
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  font-weight: 600;
  color: #e6e6e6;
}

.suffix-icon {
  height: 16px;
  width: auto;
  flex: 0 0 auto;
}
</style>
