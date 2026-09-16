import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from "vue";
import { backend } from "@/shared/backend";
import { TRIGGER_DEBUG_POPUP_AD_EVENT } from "@/shared/events";
import {
  adPreloadKey,
  isPreloadedAdFailed,
} from "@/features/ads/composables/useAdsPreload";
import type { StartupAd } from "@/shared/types";

export function useMainEntryPopupAd(ads: Ref<StartupAd[]>, delayMs = 650) {
  const failedAdKeys = ref<ReadonlySet<string>>(new Set());
  const selectedAdId = ref<string | null>(null);
  const dismissed = ref(false);
  const visible = ref(false);
  let timer: ReturnType<typeof setTimeout> | null = null;

  const eligibleAds = computed(() =>
    (ads.value || []).filter(
      (ad) =>
        ad.enabled !== false &&
        ad.placement === "main_entry_popup" &&
        (ad.image_url || "").trim().length > 0 &&
        !failedAdKeys.value.has(adPreloadKey(ad)) &&
        !isPreloadedAdFailed(ad),
    ),
  );

  function pickRandomAd(): StartupAd | null {
    const list = eligibleAds.value;
    if (list.length === 0) {
      return null;
    }
    if (list.length === 1) {
      return list[0];
    }
    // If multiple candidates exist, avoid repeating the currently selected one when possible
    const otherCandidates = list.filter((item) => item.id !== selectedAdId.value);
    const pool = otherCandidates.length > 0 ? otherCandidates : list;
    const index = Math.floor(Math.random() * pool.length);
    return pool[index] ?? list[0];
  }

  const currentAd = computed(() => {
    if (!selectedAdId.value) {
      return null;
    }
    return eligibleAds.value.find((item) => item.id === selectedAdId.value) ?? null;
  });

  function selectAndPreloadAd(): StartupAd | null {
    const chosen = pickRandomAd();
    if (!chosen) {
      selectedAdId.value = null;
      return null;
    }
    selectedAdId.value = chosen.id;

    // Preload image so it displays immediately on pop
    const preloadImg = new Image();
    preloadImg.onerror = () => {
      markImageFailed(chosen);
    };
    preloadImg.src = chosen.image_url;

    return chosen;
  }

  function schedulePopup() {
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
    if (dismissed.value) {
      visible.value = false;
      return;
    }

    const chosen = selectAndPreloadAd();
    if (!chosen) {
      visible.value = false;
      return;
    }

    timer = setTimeout(() => {
      if (!dismissed.value && currentAd.value) {
        visible.value = true;
      }
    }, delayMs);
  }

  function handleDebugTrigger() {
    dismissed.value = false;
    failedAdKeys.value = new Set();
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
    const chosen = selectAndPreloadAd();
    if (chosen) {
      visible.value = true;
    }
  }

  onMounted(() => {
    schedulePopup();
    window.addEventListener(TRIGGER_DEBUG_POPUP_AD_EVENT, handleDebugTrigger);
  });

  onBeforeUnmount(() => {
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
    window.removeEventListener(TRIGGER_DEBUG_POPUP_AD_EVENT, handleDebugTrigger);
  });

  watch(eligibleAds, (list) => {
    const selectedIsEligible =
      selectedAdId.value !== null && list.some((ad) => ad.id === selectedAdId.value);
    if (selectedIsEligible) {
      return;
    }

    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
    selectedAdId.value = null;
    visible.value = false;

    if (list.length > 0 && !dismissed.value) {
      schedulePopup();
    }
  });

  function markImageFailed(ad: StartupAd) {
    markImageFailedByKey(adPreloadKey(ad));
  }

  function markImageFailedByKey(key: string) {
    if (!key || failedAdKeys.value.has(key)) {
      return;
    }
    const wasCurrentAd = currentAd.value !== null && adPreloadKey(currentAd.value) === key;
    const next = new Set(failedAdKeys.value);
    next.add(key);
    failedAdKeys.value = next;

    // A late error from an image that is no longer displayed must not replace
    // the newly selected creative. The eligibility watcher handles that stale
    // identity without disturbing the current modal.
    if (!wasCurrentAd) {
      return;
    }

    if (timer) {
      clearTimeout(timer);
      timer = null;
    }

    if (eligibleAds.value.length > 0) {
      const nextChosen = selectAndPreloadAd();
      if (nextChosen && !dismissed.value) {
        visible.value = true;
      }
    } else {
      selectedAdId.value = null;
      visible.value = false;
    }
  }

  function closeAd() {
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
    dismissed.value = true;
    visible.value = false;
  }

  function handleModalShowChange(nextVisible: boolean) {
    if (!nextVisible) {
      // Naive UI emits update:show=false for both mask clicks and Escape. They
      // are user dismissals, so keep the popup suppressed for this session.
      closeAd();
    }
  }

  async function openAd(clickURL: string) {
    const target = (clickURL || "").trim();
    if (!target) {
      return;
    }
    await backend.OpenExternalURL(target);
  }

  return {
    currentAd,
    visible,
    closeAd,
    handleModalShowChange,
    markImageFailed,
    markImageFailedByKey,
    openAd,
  };
}
