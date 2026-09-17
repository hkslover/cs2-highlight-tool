<template>
  <main class="main-app">
    <MainTopBannerAds v-if="!isSettingsRoute" :ads="ads" />
    <MainEntryPopupAd :ads="ads" />

    <div v-if="!isSettingsRoute" class="main-steps-bar">
      <n-steps :current="currentStep" size="small" @update:current="onStepClick">
        <n-step :title="t('main.steps.import')" />
        <n-step :title="t('main.steps.clips')" />
        <n-step :title="t('main.steps.produce')" />
        <n-step :title="t('main.steps.edit')" />
      </n-steps>
    </div>
    <div class="main-view">
      <router-view />
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useRouter, useRoute } from "vue-router";
import { t } from "@/shared/i18n";
import MainTopBannerAds from "@/features/ads/components/MainTopBannerAds.vue";
import MainEntryPopupAd from "@/features/ads/components/MainEntryPopupAd.vue";
import type { StartupAd } from "@/shared/types";

defineProps<{
  ads: StartupAd[];
}>();

const router = useRouter();
const route = useRoute();

const stepMap: Record<string, number> = {
  "/import": 1,
  "/clips": 2,
  "/produce": 3,
  "/edit": 4,
};

const stepRoutes = ["/import", "/clips", "/produce", "/edit"];
const isSettingsRoute = computed(() => route.path === "/settings");

const currentStep = computed(() => {
  const path = route.path;
  for (const prefix of Object.keys(stepMap)) {
    if (path === prefix || path.startsWith(prefix + "/")) {
      return stepMap[prefix];
    }
  }
  return 1;
});

function onStepClick(step: number) {
  const target = stepRoutes[step - 1];
  if (target) {
    router.push(target);
  }
}
</script>

<style scoped>
.main-app {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

.main-view {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
</style>
