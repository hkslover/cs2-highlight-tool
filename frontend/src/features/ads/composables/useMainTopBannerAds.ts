import { computed, ref, type Ref } from "vue";
import { backend } from "@/shared/backend";
import type { StartupAd } from "@/shared/types";

export function useMainTopBannerAds(ads: Ref<StartupAd[]>) {
  // Creative URLs that failed to load are dropped from the rotation so a dead
  // image URL cannot leave an empty ad frame on screen. A failure is sticky for
  // the session: the same broken URL will not be retried on every re-render.
  const failedAdIds = ref<ReadonlySet<string>>(new Set());

  const sortedAds = computed(() =>
    (ads.value || []).filter(
      (ad) =>
        ad.enabled !== false &&
        ad.placement === "main_steps_top_banner" &&
        (ad.image_url || "").trim().length > 0 &&
        !failedAdIds.value.has(ad.id),
    ),
  );

  function markImageFailed(ad: StartupAd) {
    if (!ad?.id || failedAdIds.value.has(ad.id)) {
      return;
    }
    const next = new Set(failedAdIds.value);
    next.add(ad.id);
    failedAdIds.value = next;
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
