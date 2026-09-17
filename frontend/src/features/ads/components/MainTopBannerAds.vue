<template>
  <section v-if="sortedAds.length > 0" class="ads-wrap" :aria-label="t('main.ads.sponsored_label')">
    <n-carousel
      class="ads-carousel"
      :autoplay="sortedAds.length > 1"
      :interval="5000"
      :show-dots="sortedAds.length > 1"
      :draggable="sortedAds.length > 1"
      :touchable="sortedAds.length > 1"
      :show-arrow="false"
      :loop="sortedAds.length > 1"
    >
      <n-carousel-item v-for="ad in sortedAds" :key="ad.id">
        <button
          type="button"
          class="sponsor-card"
          :aria-label="ad.image_alt || t('main.ads.sponsored_label')"
          @click="onClick(ad.click_url)"
        >
          <!-- Blurred, cover-cropped copy of the same creative. It absorbs the
               difference between the slot ratio and the creative ratio, so the
               creative below is never cropped and never sits on a flat bar.
               Same URL as the foreground image, so it is not fetched twice. -->
          <img
            class="sponsor-card__backdrop"
            :src="ad.image_url"
            alt=""
            aria-hidden="true"
            draggable="false"
          />
          <img
            class="sponsor-card__image"
            :src="ad.image_url"
            alt=""
            draggable="false"
            @error="markImageFailed(ad)"
          />
        </button>
      </n-carousel-item>
    </n-carousel>
  </section>
</template>

<script setup lang="ts">
import { toRef } from "vue";
import { useMainTopBannerAds } from "@/features/ads/composables/useMainTopBannerAds";
import { t } from "@/shared/i18n";
import type { StartupAd } from "@/shared/types";

const props = defineProps<{
  ads: StartupAd[];
}>();

const { sortedAds, markImageFailed, openAd } = useMainTopBannerAds(toRef(props, "ads"));

let lastClickTime = 0;
const CLICK_THROTTLE_MS = 500;

async function onClick(clickURL: string) {
  const target = (clickURL || "").trim();
  if (!target) {
    return;
  }
  const now = Date.now();
  if (now - lastClickTime < CLICK_THROTTLE_MS) {
    return;
  }
  lastClickTime = now;
  try {
    await openAd(target);
  } catch {
    // keep ad click silent on failure
  }
}
</script>

<style scoped>
.ads-wrap {
  margin-bottom: 6px;
}

/* Ad slot height. Together with the 1px card border this gives a creative box
   of 890x111 at the minimum window width, i.e. the ~8:1 leaderboard ratio the
   advertiser spec is written against (2x canvas 1780x222). It is also the
   ceiling for how large a creative can be shown; other ratios stay complete
   and sit centred on the blurred backdrop. */
.ads-carousel {
  height: 113px;
  border-radius: 10px;
  overflow: hidden;
}

.sponsor-card {
  position: relative;
  display: block;
  width: 100%;
  height: 100%;
  padding: 0;
  border: 1px solid #303732;
  border-radius: 10px;
  overflow: hidden;
  background: #141816;
  cursor: pointer;
  user-select: none;
  -webkit-user-select: none;
  isolation: isolate;
  transform: translateZ(0);
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

.sponsor-card:hover {
  border-color: #434f46;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.35);
}

.sponsor-card__backdrop {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: center;
  filter: blur(18px);
  /* Over-scale so the blur kernel does not sample past the image edges. */
  transform: scale(1.15) translateZ(0);
  opacity: 0.55;
  pointer-events: none;
  will-change: transform, filter;
  backface-visibility: hidden;
}

/* Foreground creative: always shown whole, scaled to fit, centred on top of
   the backdrop. */
.sponsor-card__image {
  position: relative;
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  object-position: center;
}

.sponsor-card:focus-visible {
  outline: 2px solid #85d3a7;
  outline-offset: -2px;
}
</style>
