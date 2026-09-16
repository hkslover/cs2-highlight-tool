<template>
  <n-modal
    v-if="currentAd"
    v-model:show="visible"
    :mask-closable="true"
    :close-on-esc="true"
    :auto-focus="false"
    @update:show="handleModalShowChange"
  >
    <div class="popup-ad-dialog">
      <button
        type="button"
        class="popup-ad-close"
        :title="t('topbar.close')"
        :aria-label="t('topbar.close')"
        @click="closeAd"
      >
        <svg width="12" height="12" viewBox="0 0 10 10" aria-hidden="true">
          <path
            d="M1 1l8 8M9 1l-8 8"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
          />
        </svg>
      </button>

      <button
        type="button"
        class="popup-ad-card"
        :aria-label="currentAd.image_alt || t('main.ads.sponsored_label')"
        @click="onClick(currentAd.click_url)"
      >
        <img
          :key="currentAdKey"
          class="popup-ad-image"
          :src="currentAd.image_url"
          :data-ad-failure-key="currentAdKey"
          alt=""
          draggable="false"
          @error="onImageError"
        />
      </button>
    </div>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, toRef } from "vue";
import { useMainEntryPopupAd } from "@/features/ads/composables/useMainEntryPopupAd";
import { adPreloadKey } from "@/features/ads/composables/useAdsPreload";
import { t } from "@/shared/i18n";
import type { StartupAd } from "@/shared/types";

const props = defineProps<{
  ads: StartupAd[];
}>();

const {
  currentAd,
  visible,
  closeAd,
  handleModalShowChange,
  markImageFailedByKey,
  openAd,
} = useMainEntryPopupAd(
  toRef(props, "ads"),
);
const currentAdKey = computed(() => (currentAd.value ? adPreloadKey(currentAd.value) : ""));

function onImageError(event: Event) {
  const target = event.currentTarget as HTMLImageElement | null;
  const key = target?.dataset?.adFailureKey || "";
  markImageFailedByKey(key);
}

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
.popup-ad-dialog {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  max-width: 90vw;
  max-height: 85vh;
  animation: popup-spring-in 0.42s cubic-bezier(0.16, 1, 0.3, 1) both;
  transform-origin: center center;
}

@keyframes popup-spring-in {
  0% {
    opacity: 0;
    transform: scale(0.82) translateY(20px);
  }
  100% {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
}

.popup-ad-close {
  position: absolute;
  top: -14px;
  right: -14px;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: 1px solid #3c4840;
  background: #1c221e;
  color: #c6d1c8;
  cursor: pointer;
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.5);
  animation: close-pop-in 0.36s cubic-bezier(0.175, 0.885, 0.32, 1.2) 0.1s both;
  transition: background-color 0.2s ease, color 0.2s ease, transform 0.25s ease;
}

@keyframes close-pop-in {
  0% {
    opacity: 0;
    transform: scale(0.3);
  }
  100% {
    opacity: 1;
    transform: scale(1);
  }
}

.popup-ad-close:hover {
  background: #28332c;
  color: #edf1ee;
  transform: scale(1.12) rotate(90deg);
}

.popup-ad-close:focus-visible {
  outline: 2px solid #85d3a7;
  outline-offset: 2px;
}

.popup-ad-card {
  position: relative;
  display: block;
  padding: 0;
  border: 1px solid #303732;
  border-radius: 12px;
  overflow: hidden;
  background: #141816;
  cursor: pointer;
  user-select: none;
  -webkit-user-select: none;
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.75);
  transition: border-color 0.25s ease, box-shadow 0.25s ease, transform 0.25s ease;
}

.popup-ad-card:hover {
  border-color: #4a574e;
  transform: translateY(-2px);
  box-shadow: 0 20px 56px rgba(0, 0, 0, 0.85);
}

.popup-ad-card:focus-visible {
  outline: 2px solid #85d3a7;
  outline-offset: 2px;
}

.popup-ad-image {
  display: block;
  max-width: 520px;
  max-height: 70vh;
  width: auto;
  height: auto;
  object-fit: contain;
}
</style>
