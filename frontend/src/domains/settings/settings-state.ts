import { computed, reactive, ref, watch, type Ref } from "vue";
import { backend as defaultBackend } from "../../shared/backend/adapter.js";
import type { BackendApi } from "../../shared/backend/types.js";
import { CLIP_SETTINGS_SAVED_EVENT } from "../../shared/events.js";
import type { ClipSettings } from "../../shared/types/clips.js";

export type SettingsBackend = Pick<BackendApi, "GetWorkspaceState" | "GetClipSettings" | "SaveClipSettings">;

export interface SettingsRequestSnapshot {
  /** Monotonic load/save request sequence. */
  version: number;
  /** Draft revision represented by settings. */
  draftVersion: number;
  settings: ClipSettings;
}

export interface SettingsStateOptions {
  backend?: SettingsBackend;
  autoSaveDelayMs?: number;
  requireWorkspace?: boolean;
}

export interface SettingsStore {
  /** The latest complete settings returned by the backend. */
  confirmedSettings: Ref<ClipSettings | null>;
  /** The object edited by settings UI. It is shared by every settings entry. */
  draftSettings: ClipSettings;
  /** Snapshot sent by the currently active save request, if any. */
  requestSnapshot: Ref<SettingsRequestSnapshot | null>;
  /** Increments for each user edit to the draft. */
  draftVersion: Ref<number>;
  /** Increments when a backend response is accepted as confirmed. */
  confirmedVersion: Ref<number>;
  /** Monotonic request version for load/save ordering and diagnostics. */
  requestVersion: Ref<number>;
  loaded: Ref<boolean>;
  loading: Ref<boolean>;
  saving: Ref<boolean>;
  dirty: Ref<boolean>;
  errorMessage: Ref<string>;
  lastSavedVersion: Ref<number>;
  /** Identifies the active workspace lifetime. */
  workspaceGeneration: Ref<number>;
  init(): Promise<void>;
  /** Detach all work from the previous workspace and restore the placeholder draft. */
  resetForWorkspace(): void;
  flush(): Promise<void>;
  /** Flush pending work when an entry is hidden or unmounted; the shared store remains alive. */
  dispose(): Promise<void>;
  clearError(): void;
}

// Keep one frontend placeholder in the domain. It is never persisted before a
// successful GetClipSettings response, so a failed read cannot overwrite the
// backend with these values.
export const DEFAULT_CLIP_SETTINGS: Readonly<ClipSettings> = Object.freeze({
  killer_pre_seconds: 4,
  killer_post_seconds: 4,
  victim_pre_seconds: 2,
  victim_post_seconds: 2,
  auto_add_victim_view: true,
  enable_voice: true,
  record_fps: 60,
  record_quality: "high",
  edit_fps: 60,
  edit_quality: "high",
  video_preset: "auto",
  launch_resolution: "4:3",
  record_output_dir: "",
  enable_spec_show_xray_zero: true,
  hide_all_ui: false,
  hide_player_avatars: false,
  use_shoulder_camera: false,
  pov_hud_enabled: true,
  pov_radar_enabled: false,
  sky_blackout: true,
  disable_clouds: false,
  kill_feed_lifetime: 4,
  block_kill_feed: false,
});

export function createDefaultClipSettings(): ClipSettings {
  return { ...DEFAULT_CLIP_SETTINGS };
}

function cloneSettings(settings: ClipSettings): ClipSettings {
  return { ...settings };
}

const CLIP_SETTINGS_KEYS = Object.keys(DEFAULT_CLIP_SETTINGS) as Array<keyof ClipSettings>;

function settingsEqual(left: ClipSettings | null, right: ClipSettings | null): boolean {
  if (!left || !right) {
    return left === right;
  }
  return CLIP_SETTINGS_KEYS.every((key) => left[key] === right[key]);
}

/**
 * Apply a backend response while retaining fields edited after a request's
 * snapshot. A field that was changed and then restored to the snapshot is
 * intentionally treated as untouched, so backend normalization can apply.
 */
function mergeDraftChanges(
  response: ClipSettings,
  snapshot: ClipSettings,
  currentDraft: ClipSettings,
): ClipSettings {
  const merged = cloneSettings(response);
  for (const key of CLIP_SETTINGS_KEYS) {
    if (currentDraft[key] !== snapshot[key]) {
      merged[key] = currentDraft[key] as never;
    }
  }
  return merged;
}

/**
 * Create an isolated store for tests or a future application boundary.
 * The exported useSettingsStore() below supplies the shared application store.
 */
export function createSettingsStore(options: SettingsStateOptions = {}): SettingsStore {
  const backend = options.backend ?? defaultBackend;
  const autoSaveDelayMs = options.autoSaveDelayMs ?? 500;
  const requireWorkspace = options.requireWorkspace ?? true;

  const confirmedSettings = ref<ClipSettings | null>(null);
  const draftSettings = reactive<ClipSettings>(createDefaultClipSettings());
  const requestSnapshot = ref<SettingsRequestSnapshot | null>(null);
  const draftVersion = ref(0);
  const confirmedVersion = ref(0);
  const requestVersion = ref(0);
  const loaded = ref(false);
  const loading = ref(false);
  const saving = ref(false);
  const errorMessage = ref("");
  const lastSavedVersion = ref(0);
  const workspaceGeneration = ref(0);

  let previousDraft = cloneSettings(draftSettings);
  let syncingDraft = false;
  let autoSaveTimer: ReturnType<typeof setTimeout> | null = null;
  let activeLoad: Promise<void> | null = null;
  let activeSave: Promise<boolean> | null = null;
  let queuedSave = false;
  let latestSaveRequestVersion = 0;
  let latestLoadRequestVersion = 0;

  const dirty = computed(() => !settingsEqual(confirmedSettings.value, draftSettings));

  function clearAutoSaveTimer(): void {
    if (autoSaveTimer != null) {
      clearTimeout(autoSaveTimer);
      autoSaveTimer = null;
    }
  }

  function clearError(): void {
    errorMessage.value = "";
  }

  function assignDraft(next: ClipSettings): void {
    syncingDraft = true;
    Object.assign(draftSettings, next);
    previousDraft = cloneSettings(draftSettings);
    syncingDraft = false;
  }

  function markUserDraftChange(): void {
    const current = cloneSettings(draftSettings);
    previousDraft = current;
    draftVersion.value += 1;
    if (loaded.value) {
      scheduleAutoSave();
    }
  }

  // flush:sync ensures a v-model edit is versioned before a fast backend
  // response can be applied. The syncing guard excludes backend assignments.
  watch(
    draftSettings,
    () => {
      if (!syncingDraft) {
        markUserDraftChange();
      } else {
        previousDraft = cloneSettings(draftSettings);
      }
    },
    { deep: true, flush: "sync" },
  );

  function scheduleAutoSave(): void {
    if (!loaded.value) {
      return;
    }
    clearAutoSaveTimer();
    clearError();
    autoSaveTimer = setTimeout(() => {
      autoSaveTimer = null;
      void saveCurrentDraft();
    }, autoSaveDelayMs);
  }

  function shouldAcceptLoad(generation: number, requestId: number): boolean {
    return generation === workspaceGeneration.value && requestId === latestLoadRequestVersion && requestId >= latestSaveRequestVersion;
  }

  function applyLoadedSettings(next: ClipSettings, loadDraftSnapshot: ClipSettings): void {
    const backendSettings = cloneSettings(next);
    const currentDraft = cloneSettings(draftSettings);
    confirmedSettings.value = backendSettings;
    confirmedVersion.value += 1;
    assignDraft(mergeDraftChanges(backendSettings, loadDraftSnapshot, currentDraft));
  }

  async function fetchSettings(generation: number): Promise<void> {
    const loadDraftSnapshot = cloneSettings(draftSettings);
    const loadRequestId = ++requestVersion.value;
    latestLoadRequestVersion = loadRequestId;
    loading.value = true;
    clearError();

    try {
      if (requireWorkspace) {
        const workspace = await backend.GetWorkspaceState();
        if (!workspace?.initialized || !workspace.data_dir?.trim()) {
          // The top bar exists during workspace_init. Leave the placeholder
          // untouched and allow a later init() after SetWorkspaceDir.
          return;
        }
      }
      if (generation !== workspaceGeneration.value) {
        return;
      }
      const next = await backend.GetClipSettings();
      if (!shouldAcceptLoad(generation, loadRequestId)) {
        return;
      }
      applyLoadedSettings(next, loadDraftSnapshot);
      loaded.value = true;
      if (dirty.value) {
        scheduleAutoSave();
      }
    } catch (error: unknown) {
      // Keep both confirmed and draft state intact. In particular, never save
      // DEFAULT_CLIP_SETTINGS after a read failure.
      if (generation === workspaceGeneration.value) {
        errorMessage.value = error instanceof Error ? error.message : String(error);
      }
    } finally {
      if (generation === workspaceGeneration.value) {
        loading.value = false;
      }
    }
  }

  async function init(): Promise<void> {
    if (loaded.value || activeLoad) {
      return activeLoad ?? Promise.resolve();
    }
    const generation = workspaceGeneration.value;
    const task = fetchSettings(generation);
    activeLoad = task;
    try {
      await task;
    } finally {
      if (activeLoad === task) {
        activeLoad = null;
      }
    }
  }

  async function performSave(snapshot: SettingsRequestSnapshot, generation: number): Promise<boolean> {
    saving.value = true;
    requestSnapshot.value = snapshot;
    latestSaveRequestVersion = snapshot.version;
    clearError();
    try {
      const saved = cloneSettings(await backend.SaveClipSettings(snapshot.settings));
      if (generation !== workspaceGeneration.value) {
        return false;
      }
      const currentDraft = cloneSettings(draftSettings);
      confirmedSettings.value = saved;
      confirmedVersion.value += 1;
      // Preserve fields edited while the request was in flight. Backend
      // normalization still flows into fields unchanged since the snapshot.
      assignDraft(mergeDraftChanges(saved, snapshot.settings, currentDraft));
      lastSavedVersion.value = snapshot.version;
      if (typeof window !== "undefined") {
        window.dispatchEvent(new CustomEvent(CLIP_SETTINGS_SAVED_EVENT, { detail: cloneSettings(saved) }));
      }
      return true;
    } catch (error: unknown) {
      // Keep the draft so a later edit or flush can retry it explicitly.
      if (generation === workspaceGeneration.value) {
        errorMessage.value = error instanceof Error ? error.message : String(error);
      }
      return false;
    } finally {
      if (generation === workspaceGeneration.value) {
        requestSnapshot.value = null;
        saving.value = false;
      }
    }
  }

  async function saveCurrentDraft(): Promise<boolean> {
    if (!loaded.value || !dirty.value) {
      return true;
    }
    if (activeSave) {
      queuedSave = true;
      return activeSave;
    }

    const generation = workspaceGeneration.value;
    const snapshot: SettingsRequestSnapshot = {
      version: requestVersion.value + 1,
      draftVersion: draftVersion.value,
      settings: cloneSettings(draftSettings),
    };
    requestVersion.value = snapshot.version;
    const task = performSave(snapshot, generation);
    activeSave = task;
    let succeeded = false;
    try {
      succeeded = await task;
    } finally {
      if (activeSave === task) {
        activeSave = null;
      }
      if (generation !== workspaceGeneration.value) {
        return succeeded;
      }
      const continueWithNewerDraft = queuedSave || dirty.value;
      queuedSave = false;
      // A failed save must not spin forever. Keep its draft/error for an
      // explicit retry, while a successful request drains newer edits.
      if (succeeded && continueWithNewerDraft && !autoSaveTimer) {
        void saveCurrentDraft();
      }
    }
    return succeeded;
  }

  async function flush(): Promise<void> {
    const generation = workspaceGeneration.value;
    clearAutoSaveTimer();

    // A panel can close while its first read is still in flight. Wait for that
    // read before deciding whether there is anything safe to persist, then
    // clear a debounce that the successful read may have scheduled.
    if (activeLoad) {
      await activeLoad;
      if (generation !== workspaceGeneration.value) return;
      clearAutoSaveTimer();
    }

    if (!loaded.value) {
      return;
    }
    while (loaded.value) {
      if (generation !== workspaceGeneration.value) return;
      if (activeSave) {
        queuedSave = true;
        if (!(await activeSave)) {
          return;
        }
        continue;
      }
      if (!dirty.value) {
        return;
      }
      if (!(await saveCurrentDraft())) {
        return;
      }
    }
  }

  async function dispose(): Promise<void> {
    await flush();
  }

  function resetForWorkspace(): void {
    workspaceGeneration.value += 1;
    clearAutoSaveTimer();
    // Detach promises from the old workspace. Their handlers remain attached,
    // but generation guards prevent them from touching the new state.
    activeLoad = null;
    activeSave = null;
    queuedSave = false;
    latestLoadRequestVersion = 0;
    latestSaveRequestVersion = 0;
    requestVersion.value = 0;
    loaded.value = false;
    loading.value = false;
    saving.value = false;
    confirmedSettings.value = null;
    requestSnapshot.value = null;
    errorMessage.value = "";
    lastSavedVersion.value = 0;
    assignDraft(createDefaultClipSettings());
    draftVersion.value += 1;
  }

  return {
    confirmedSettings,
    draftSettings,
    requestSnapshot,
    draftVersion,
    confirmedVersion,
    requestVersion,
    loaded,
    loading,
    saving,
    dirty,
    errorMessage,
    lastSavedVersion,
    workspaceGeneration,
    init,
    resetForWorkspace,
    flush,
    dispose,
    clearError,
  };
}

const sharedSettingsStore = createSettingsStore();

export function useSettingsStore(): SettingsStore {
  return sharedSettingsStore;
}
