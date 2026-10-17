import type {
  ClipSettings,
  DebugConsolePlan,
  DebugSettingPlan,
  DebugSettingsDraft,
} from "@/shared/types";

export interface DebugSettingsBackend {
  getClipSettings: () => Promise<ClipSettings>;
  getConsolePlan: (draft: DebugSettingsDraft) => Promise<DebugConsolePlan>;
}

export interface DebugSettingsCell<T> {
  value: T;
}

export interface DebugSettingsCoreOptions {
  backend: DebugSettingsBackend;
  createCell: <T>(initial: T) => DebugSettingsCell<T>;
}

/**
 * Placeholder draft shown before a successful read. It is never sent to a save
 * method and never used for a plan request; the page disables the settings
 * groups until the formal settings loaded.
 */
export function emptyDebugSettingsDraft(): DebugSettingsDraft {
  return {
    pov_radar_enabled: false,
    sky_blackout: true,
    disable_clouds: false,
    hide_all_ui: false,
    hide_player_avatars: false,
    hide_chat_messages: true,
    enable_spec_show_xray_zero: true,
    enable_voice: true,
    use_shoulder_camera: false,
    kill_feed_lifetime: 4,
    pov_kill_feed_only: false,
    block_kill_feed: false,
    noflash_strength: 0,
    record_fps: 60,
    record_quality: "high",
    video_preset: "auto",
    launch_resolution: "4:3",
  };
}

/** Copy the debug draft fields out of one confirmed settings snapshot. */
export function debugSettingsDraftFromClipSettings(
  settings: ClipSettings,
): DebugSettingsDraft {
  return {
    pov_radar_enabled: settings.pov_radar_enabled,
    sky_blackout: settings.sky_blackout,
    disable_clouds: settings.disable_clouds,
    hide_all_ui: settings.hide_all_ui,
    hide_player_avatars: settings.hide_player_avatars,
    hide_chat_messages: settings.hide_chat_messages,
    enable_spec_show_xray_zero: settings.enable_spec_show_xray_zero,
    enable_voice: settings.enable_voice,
    use_shoulder_camera: settings.use_shoulder_camera,
    kill_feed_lifetime: settings.kill_feed_lifetime,
    pov_kill_feed_only: settings.pov_kill_feed_only,
    block_kill_feed: settings.block_kill_feed,
    noflash_strength: settings.noflash_strength,
    record_fps: settings.record_fps,
    record_quality: settings.record_quality,
    video_preset: settings.video_preset,
    launch_resolution: settings.launch_resolution,
  };
}

/**
 * Pick the command variant for a switch row. The plan's on/off arrays are the
 * only command source; a missing plan yields no command at all. A non-switch
 * row (no variants) falls back to its current `commands`.
 */
export function debugSettingCommandsFor(
  planItem: DebugSettingPlan | undefined,
  enabled: boolean,
): string[] {
  if (!planItem) {
    return [];
  }
  const variant = enabled ? planItem.on : planItem.off;
  if (variant && variant.length > 0) {
    return [...variant];
  }
  return [...(planItem.commands ?? [])];
}

export interface DebugSettingsCore {
  draft: DebugSettingsCell<DebugSettingsDraft>;
  /** Last confirmed ClipSettings snapshot, used by formal-workflow rows. */
  sourceSettings: DebugSettingsCell<ClipSettings | null>;
  plan: DebugSettingsCell<DebugConsolePlan | null>;
  loaded: DebugSettingsCell<boolean>;
  loading: DebugSettingsCell<boolean>;
  planLoading: DebugSettingsCell<boolean>;
  errorMessage: DebugSettingsCell<string>;
  planErrorMessage: DebugSettingsCell<string>;
  /** Read the formal settings and replace the draft. Also the reload button. */
  load(force?: boolean): Promise<boolean>;
  refreshPlan(): Promise<DebugConsolePlan | null>;
  patchDraft(patch: Partial<DebugSettingsDraft>): void;
  dispose(): void;
}

function cloneDraft(draft: DebugSettingsDraft): DebugSettingsDraft {
  return { ...draft };
}

export function createDebugSettingsCore(
  options: DebugSettingsCoreOptions,
): DebugSettingsCore {
  const draft = options.createCell<DebugSettingsDraft>(emptyDebugSettingsDraft());
  const sourceSettings = options.createCell<ClipSettings | null>(null);
  const plan = options.createCell<DebugConsolePlan | null>(null);
  const loaded = options.createCell(false);
  const loading = options.createCell(false);
  const planLoading = options.createCell(false);
  const errorMessage = options.createCell("");
  const planErrorMessage = options.createCell("");

  let generation = 0;
  let planRequestVersion = 0;

  function describe(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }

  /**
   * Read the formal settings and replace the draft. A failed read keeps the
   * placeholder draft (and the last plan) intact and surfaces the error; it
   * never writes anything back to the backend. Without `force` an
   * already-initialized draft is kept, so re-mounting the page does not reset
   * the session's debug choices; only the explicit reload passes `force`.
   */
  async function load(force = false): Promise<boolean> {
    if (loaded.value && !force) {
      return true;
    }
    const currentGeneration = generation;
    loading.value = true;
    errorMessage.value = "";
    try {
      const settings = await options.backend.getClipSettings();
      if (currentGeneration !== generation) {
        return false;
      }
      sourceSettings.value = settings;
      draft.value = debugSettingsDraftFromClipSettings(settings);
      loaded.value = true;
      return true;
    } catch (error: unknown) {
      if (currentGeneration === generation) {
        // A failed reload keeps the previously confirmed draft and plan so a
        // transient read error cannot wipe the workbench. Only an initial
        // failure leaves the untouched placeholder, and nothing is ever
        // written back to the backend.
        errorMessage.value = describe(error);
        if (!loaded.value) {
          draft.value = emptyDebugSettingsDraft();
          sourceSettings.value = null;
          plan.value = null;
        }
      }
      return false;
    } finally {
      if (currentGeneration === generation) {
        loading.value = false;
      }
    }
  }

  /**
   * Rebuild the command plan from the current draft. A stale response is
   * dropped instead of overwriting a newer plan.
   */
  async function refreshPlan(): Promise<DebugConsolePlan | null> {
    const currentGeneration = generation;
    const requestVersion = ++planRequestVersion;
    const snapshot = cloneDraft(draft.value);
    planLoading.value = true;
    planErrorMessage.value = "";
    try {
      const next = await options.backend.getConsolePlan(snapshot);
      if (currentGeneration !== generation || requestVersion !== planRequestVersion) {
        return null;
      }
      plan.value = next;
      return next;
    } catch (error: unknown) {
      if (currentGeneration === generation && requestVersion === planRequestVersion) {
        planErrorMessage.value = describe(error);
      }
      return null;
    } finally {
      if (currentGeneration === generation && requestVersion === planRequestVersion) {
        planLoading.value = false;
      }
    }
  }

  function patchDraft(patch: Partial<DebugSettingsDraft>): void {
    draft.value = { ...draft.value, ...patch };
  }

  /** Clear local draft facts on a workspace switch; in-flight reads are dropped. */
  function dispose(): void {
    generation++;
    loaded.value = false;
    loading.value = false;
    planLoading.value = false;
    draft.value = emptyDebugSettingsDraft();
    sourceSettings.value = null;
    plan.value = null;
    errorMessage.value = "";
    planErrorMessage.value = "";
  }

  return {
    draft,
    sourceSettings,
    plan,
    loaded,
    loading,
    planLoading,
    errorMessage,
    planErrorMessage,
    load,
    refreshPlan,
    patchDraft,
    dispose,
  };
}
