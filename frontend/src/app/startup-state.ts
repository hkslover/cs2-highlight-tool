import type { StartupState } from "../shared/types/startup.js";

function configValue(config: Record<string, unknown>, key: string): string {
  const value = config[key];
  if (typeof value === "string") {
    return value;
  }
  if (Array.isArray(value)) {
    return value.filter((item): item is string => typeof item === "string").join(",");
  }
  return "";
}

/**
 * Settings can be loaded while the startup wizard is still running. Refresh
 * them when the FFmpeg detection cache changes in any startup mode, and once
 * more when the app enters main so a missed startup event cannot leave stale
 * capability metadata in an already-loaded settings store.
 */
export function shouldRefreshSettingsForStartupState(
  previousMode: string,
  previousConfig: Record<string, unknown>,
  next: Pick<StartupState, "mode" | "config">,
  settingsLoaded: boolean,
): boolean {
  if (!settingsLoaded || next.mode === "workspace_init") {
    return false;
  }

  const detectionChanged =
    configValue(previousConfig, "ffmpeg_detected_preset") !== configValue(next.config, "ffmpeg_detected_preset") ||
    configValue(previousConfig, "ffmpeg_detected_encoders") !== configValue(next.config, "ffmpeg_detected_encoders") ||
    configValue(previousConfig, "ffmpeg_detected_at") !== configValue(next.config, "ffmpeg_detected_at");

  return detectionChanged || (previousMode !== "main" && next.mode === "main");
}
