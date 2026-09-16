import { computed, ref, type Ref } from "vue";
import { backend } from "@/shared/backend";
import {
  adPreloadKey,
  isPreloadedAdFailed,
} from "@/features/ads/composables/useAdsPreload";
import type { StartupAd } from "@/shared/types";

export function useMainTopBannerAds(ads: Ref<StartupAd[]>) {
  // Creative URLs that failed to load are dropped from the rotation so a dead
  // image URL cannot leave an empty ad frame on screen. A failure is sticky for
  // the session: the same broken URL will not be retried on every re-render.
  const failedAdKeys = ref<ReadonlySet<string>>(new Set());

  const sortedAds = computed(() =>
    (ads.value || []).filter(
      (ad) =>
        ad.enabled !== false &&
        ad.placement === "main_steps_top_banner" &&
        (ad.image_url || "").trim().length > 0 &&
        !failedAdKeys.value.has(adPreloadKey(ad)) &&
        !isPreloadedAdFailed(ad),
    ),
  );

  function markImageFailed(ad: StartupAd) {
    const key = adPreloadKey(ad);
    if (!key || failedAdKeys.value.has(key)) {
      return;
    }
    const next = new Set(failedAdKeys.value);
    next.add(key);
    failedAdKeys.value = next;
  }

  async function openAd(clickURL: string) {
    const target = (clickURL || "").trim();
    if (!target) {
      return;
    }
    await backend.OpenExternalURL(target);
  }

  return {
    sortedAds,
    markImageFailed,
    openAd,
  };
}
