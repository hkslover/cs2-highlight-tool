import { ref } from "vue";
import type { StartupAd } from "@/shared/types";

// Cache set of URLs that have been successfully preloaded
const preloadedUrls = new Set<string>();

// A load can be requested by several startup_state_changed events before the
// first Image settles. Share that in-flight request instead of starting a new
// browser load for each event.
const inFlightUrls = new Map<string, Promise<boolean>>();

// Keep the failure scoped to both the ad identity and its current creative.
// IDs can be reused by a release manifest when the image URL changes, and a
// new ad identity must not inherit a previous ad's failed URL state.
const inFlightAdKeys = new Set<string>();

// Reactive set of ad/creative identities whose images failed during this app
// session. It intentionally remains module-scoped so repeated startup state
// events do not retry the same broken creative.
export const preloadedFailedAdKeys = ref<ReadonlySet<string>>(new Set());

export function adPreloadKey(ad: Pick<StartupAd, "id" | "image_url">): string {
  const imageURL = (ad.image_url || "").trim();
  if (!imageURL) {
    return "";
  }
  // JSON keeps the two identity fields unambiguous without putting a control
  // character into a DOM data attribute used by the popup image handler.
  return JSON.stringify([(ad.id || "").trim(), imageURL]);
}

export function isPreloadedAdFailed(ad: Pick<StartupAd, "id" | "image_url">): boolean {
  const key = adPreloadKey(ad);
  return key.length > 0 && preloadedFailedAdKeys.value.has(key);
}

function markPreloadFailed(key: string) {
  if (!key || preloadedFailedAdKeys.value.has(key)) {
    return;
  }
  const next = new Set(preloadedFailedAdKeys.value);
  next.add(key);
  preloadedFailedAdKeys.value = next;
}

/**
 * Preload a single image URL into browser memory and HTTP disk cache.
 */
export function preloadImage(url: string): Promise<boolean> {
  const normalized = (url || "").trim();
  if (!normalized) {
    return Promise.resolve(false);
  }
  if (preloadedUrls.has(normalized)) {
    return Promise.resolve(true);
  }
  const inFlight = inFlightUrls.get(normalized);
  if (inFlight) {
    return inFlight;
  }

  const promise = new Promise<boolean>((resolve) => {
    const img = new Image();
    img.onload = () => {
      preloadedUrls.add(normalized);
      resolve(true);
    };
    img.onerror = () => {
      resolve(false);
    };
    img.src = normalized;
  });
  inFlightUrls.set(normalized, promise);
  void promise.then(
    () => {
      if (inFlightUrls.get(normalized) === promise) {
        inFlightUrls.delete(normalized);
      }
    },
    () => {
      if (inFlightUrls.get(normalized) === promise) {
        inFlightUrls.delete(normalized);
      }
    },
  );
  return promise;
}

/**
 * Asynchronously preload all active remote/inline image resources declared in ads.
 * Executes silently in the background during startup checks so that upon entering
 * the main view, both the top banner and popup modal display immediately from cache.
 */
export function preloadAds(ads: StartupAd[]): void {
  if (!Array.isArray(ads) || ads.length === 0) {
    return;
  }

  for (const ad of ads) {
    if (ad.enabled === false) {
      continue;
    }
    const imageUrl = (ad.image_url || "").trim();
    if (!imageUrl) {
      continue;
    }
    const key = adPreloadKey(ad);
    if (!key || preloadedFailedAdKeys.value.has(key) || inFlightAdKeys.has(key)) {
      continue;
    }

    inFlightAdKeys.add(key);
    void preloadImage(imageUrl).then((success) => {
      if (!success) {
        markPreloadFailed(key);
      }
    }).finally(() => {
      inFlightAdKeys.delete(key);
    });
  }
}
