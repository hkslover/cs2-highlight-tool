import { createSettingsStore } from "../src/domains/settings/settings-state";
import type { BackendApi } from "../src/shared/backend/types";
import type { ClipSettings } from "../src/shared/types/clips";

type SettingsBackend = Pick<BackendApi, "GetWorkspaceState" | "GetClipSettings" | "SaveClipSettings">;

const backend: SettingsBackend = {
  GetWorkspaceState: async () => ({ initialized: true, data_dir: "", error: "" }),
  GetClipSettings: async () => ({}) as ClipSettings,
  SaveClipSettings: async (settings) => settings,
};

const store = createSettingsStore({ backend });
const confirmed: ClipSettings | null = store.confirmedSettings.value;
const requestSettings: ClipSettings | null = store.requestSnapshot.value?.settings ?? null;

void confirmed;
void requestSettings;
void store.init();
void store.refresh();
void store.flush();
void store.dispose();
