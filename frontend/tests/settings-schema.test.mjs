import assert from "node:assert/strict";
import test from "node:test";
import {
  SEARCHABLE_CLIP_SETTINGS,
  getSearchableSliderPercent,
  isSearchableSliderSetting,
  sliderPercentToStrength,
  strengthToSliderPercent,
} from "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/settings/settings-schema.js";
import { createDefaultClipSettings } from "../node_modules/.cache/cs2-highlight-tool-backend-tests/domains/settings/settings-state.js";

function itemByKey(key) {
  const item = SEARCHABLE_CLIP_SETTINGS.find((entry) => entry.key === key);
  assert.ok(item, `setting ${key} should be searchable`);
  return item;
}

test("the flash slider is a percent view over the persisted 0..1 strength", () => {
  const item = itemByKey("noflash_strength");
  assert.equal(item.kind, "slider");
  assert.equal(isSearchableSliderSetting(item), true);
  assert.equal(item.min, 0);
  assert.equal(item.max, 100);

  const settings = { ...createDefaultClipSettings(), noflash_strength: 0.35 };
  assert.equal(getSearchableSliderPercent(settings, item), 35);

  // The persisted value stays the HLAE strength, never the percent.
  assert.equal(sliderPercentToStrength(getSearchableSliderPercent(settings, item)), 0.35);
  assert.equal(sliderPercentToStrength(100), 1);
  assert.equal(sliderPercentToStrength(0), 0);
  assert.equal(sliderPercentToStrength(-20), 0);
  assert.equal(sliderPercentToStrength(140), 1);
  assert.equal(sliderPercentToStrength(Number.NaN), 0);
});

test("out-of-range stored strengths clamp to the slider domain", () => {
  assert.equal(strengthToSliderPercent(-1), 0);
  assert.equal(strengthToSliderPercent(2), 100);
  assert.equal(strengthToSliderPercent(Number.NaN), 0);
  assert.equal(strengthToSliderPercent(0.07), 7);
});

test("every searchable setting key exists on the clip settings contract", () => {
  const defaults = createDefaultClipSettings();
  for (const item of SEARCHABLE_CLIP_SETTINGS) {
    assert.equal(
      Object.prototype.hasOwnProperty.call(defaults, item.key),
      true,
      `defaults are missing ${item.key}`,
    );
  }
  assert.equal(defaults.noflash_strength, 0);
});
