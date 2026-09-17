import assert from "node:assert/strict";
import test from "node:test";
import {
  adPreloadKey,
  isPreloadedAdFailed,
  preloadAds,
  preloadedFailedAdKeys,
} from "../node_modules/.cache/cs2-highlight-tool-backend-tests/features/ads/composables/useAdsPreload.js";

const originalImage = globalThis.Image;

class FakeImage {
  static requests = [];
  onload = null;
  onerror = null;

  set src(url) {
    FakeImage.requests.push(url);
    queueMicrotask(() => {
      this.onerror?.(new Event("error"));
    });
  }
}

function ad(id, imageURL) {
  return {
    id,
    enabled: true,
    placement: "main_entry_popup",
    click_url: "https://example.test/click",
    image_url: imageURL,
  };
}

async function flushImageFailure() {
  await new Promise((resolve) => setImmediate(resolve));
}

test("preload failures are deduplicated per ad and creative identity", async (t) => {
  t.after(() => {
    preloadedFailedAdKeys.value = new Set();
    globalThis.Image = originalImage;
  });

  globalThis.Image = FakeImage;
  FakeImage.requests = [];
  preloadedFailedAdKeys.value = new Set();

  const firstCreative = ad("ad-1", "https://example.test/broken-a.png");
  preloadAds([firstCreative]);
  preloadAds([firstCreative]);
  await flushImageFailure();

  assert.equal(FakeImage.requests.length, 1);
  assert.equal(isPreloadedAdFailed(firstCreative), true);

  // The same failed creative remains sticky for this app session.
  preloadAds([firstCreative]);
  await flushImageFailure();
  assert.equal(FakeImage.requests.length, 1);

  // A changed URL under the same manifest ID is a new creative and retries.
  const changedURL = ad("ad-1", "https://example.test/broken-b.png");
  preloadAds([changedURL]);
  await flushImageFailure();
  assert.equal(FakeImage.requests.length, 2);
  assert.equal(isPreloadedAdFailed(changedURL), true);
  assert.notEqual(adPreloadKey(firstCreative), adPreloadKey(changedURL));

  // A different ad identity is also allowed its own attempt, even when the
  // release manifest points it at a URL that failed for another ad.
  const changedIdentity = ad("ad-2", firstCreative.image_url);
  preloadAds([changedIdentity]);
  await flushImageFailure();
  assert.equal(FakeImage.requests.length, 3);
  assert.equal(isPreloadedAdFailed(changedIdentity), true);
  assert.notEqual(adPreloadKey(firstCreative), adPreloadKey(changedIdentity));

});
