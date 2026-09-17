/**
 * Mode of the startup state machine. One of:
 * - "workspace_init": user has not initialized the working directory (registry value missing/invalid)
 * - "startup": startup wizard running through environment checks
 * - "main": environment ready, main app shown
 */
export type StartupMode = "workspace_init" | "startup" | "main";

export interface StartupState {
 /** See {@link StartupMode}. Backend may emit other values; treat unknown as "startup". */
 mode: string;
 phase: string;
 running: boolean;
 source_step: SourceStepState;
 fatal_error: string;
 entry_notice: string;
 ads: StartupAd[];
 self_update: SelfUpdateState;
 steps: ComponentStatus[];
 can_enter_main: boolean;
 config: Record<string, unknown>;
}

export interface WorkspaceState {
 initialized: boolean;
 data_dir: string;
 error: string;
}

/**
 * Sponsored card. Ads are image-only: the creative at {@link image_url}
 * (remote http(s) or a `data:image/...` URI) is the entire click target for
 * {@link click_url}. The release manifest keeps returning legacy text fields
 * (sponsor/title/rich_html); they are ignored and not part of this contract.
 */
export type StartupAdPlacement = "main_steps_top_banner" | "main_entry_popup";

export interface StartupAd {
 id: string;
 enabled: boolean;
 placement: StartupAdPlacement;
 click_url: string;
 image_url: string;
 image_alt?: string;
}

export interface SourceStepState {
 status: string;
 source: string;
 country_code: string;
 message: string;
 error: string;
}

export interface SelfUpdateState {
 status: string;
 available: boolean;
 current: string;
 latest: string;
 url: string;
 error: string;
}

export interface ComponentStatus {
 id: string;
 name: string;
 status: string;
 local_version: string;
 remote_version: string;
 path: string;
 error: string;
 manual_url: string;
}

export interface ProgressMessage {
 component_id: string;
 active: boolean;
 percent: number;
 indeterminate: boolean;
}
