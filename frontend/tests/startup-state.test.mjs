import assert from "node:assert/strict";
import test from "node:test";
import { shouldRefreshSettingsForStartupState } from "../node_modules/.cache/cs2-highlight-tool-backend-tests/app/startup-state.js";

function state(mode, config = {}) {
  return { mode, config };
}

test("refreshes an already-loaded settings store when detection completes during startup", () => {
  assert.equal(
    shouldRefreshSettingsForStartupState(
      "startup",
      {},
      state("startup", {
        ffmpeg_detected_preset: "n1",
        ffmpeg_detected_encoders: ["hevc_nvenc", "libx264"],
        ffmpeg_detected_at: "2026-09-09T10:00:00Z",
      }),
      true,
    ),
    true,
  );
});

test("refreshes on entering main even when the detection cache did not change", () => {
  const detected = {
    ffmpeg_detected_preset: "n1",
    ffmpeg_detected_encoders: ["hevc_nvenc", "libx264"],
    ffmpeg_detected_at: "2026-09-09T10:00:00Z",
  };

  assert.equal(shouldRefreshSettingsForStartupState("startup", detected, state("main", detected), true), true);
});

test("does not refresh before settings have completed their initial load", () => {
  assert.equal(
    shouldRefreshSettingsForStartupState(
      "startup",
      {},
      state("startup", { ffmpeg_detected_preset: "n1" }),
      false,
    ),
    false,
  );
});

test("does not refresh while the workspace is being reset", () => {
  assert.equal(
    shouldRefreshSettingsForStartupState(
      "main",
      { ffmpeg_detected_preset: "n1" },
      state("workspace_init", {}),
      true,
    ),
    false,
  );
});
