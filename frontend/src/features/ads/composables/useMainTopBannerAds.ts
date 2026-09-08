import { computed, type Ref } from "vue";
import { backend } from "@/shared/backend";
import type { StartupAd } from "@/shared/types";

export function useMainTopBannerAds(ads: Ref<StartupAd[]>) {
  const sortedAds = computed(() =>
    (ads.value || [])
      .filter((ad) => ad.placement === "main_steps_top_banner")
      .slice(),
  );

  async function openAd(clickURL: string) {
    await backend.OpenExternalURL(clickURL);
  }

  return {
    sortedAds,
    openAd,
  };
}
