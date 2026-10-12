import type { DebugConsolePlan } from "@/shared/types";
import { debugSettingCommandsFor } from "./debugSettingsCore.js";

/**
 * Dropdown section ids for the console quick commands. They are display
 * groups only: every command string still comes from the backend plan.
 */
export const DEBUG_QUICK_COMMAND_GROUPS = ["picture", "sound"] as const;

export type DebugQuickCommandGroup = (typeof DEBUG_QUICK_COMMAND_GROUPS)[number];

/**
 * One console quick command. `command` is already the full command string:
 * multi-command entries are joined with `"; "` in plan order and the original
 * semicolons/quotes inside a command are preserved. Selecting an option only
 * fills the shared input; it never sends, and it never edits the debug draft.
 */
export interface DebugQuickCommandOption {
  /** Stable selection id, e.g. `sky_blackout:on`. */
  key: string;
  /** i18n key of the option label. */
  labelKey: string;
  group: DebugQuickCommandGroup;
  command: string;
}

interface QuickCommandDescriptor {
  key: string;
  labelKey: string;
  group: DebugQuickCommandGroup;
  /** Which backend plan variant to read; `current` uses `commands`. */
  variant: "on" | "off" | "current";
}

/**
 * Display order and grouping only. The switch polarity and the restore
 * commands stay in the backend plan (`on`/`off`); this list never repeats a
 * command string or a default value.
 */
const QUICK_COMMAND_DESCRIPTORS: QuickCommandDescriptor[] = [
  { key: "sky_blackout", labelKey: "main.debug.quick_sky_blackout", group: "picture", variant: "on" },
  { key: "sky_blackout", labelKey: "main.debug.quick_sky_restore", group: "picture", variant: "off" },
  { key: "disable_clouds", labelKey: "main.debug.quick_clouds_hide", group: "picture", variant: "on" },
  { key: "disable_clouds", labelKey: "main.debug.quick_clouds_restore", group: "picture", variant: "off" },
  { key: "hide_all_ui", labelKey: "main.debug.quick_ui_hide", group: "picture", variant: "on" },
  { key: "hide_all_ui", labelKey: "main.debug.quick_ui_restore", group: "picture", variant: "off" },
  {
    key: "hide_player_avatars",
    labelKey: "main.debug.quick_avatars_hide",
    group: "picture",
    variant: "on",
  },
  {
    key: "hide_player_avatars",
    labelKey: "main.debug.quick_avatars_restore",
    group: "picture",
    variant: "off",
  },
  {
    key: "hide_chat_messages",
    labelKey: "main.debug.quick_chat_hide",
    group: "picture",
    variant: "on",
  },
  {
    key: "hide_chat_messages",
    labelKey: "main.debug.quick_chat_restore",
    group: "picture",
    variant: "off",
  },
  {
    key: "enable_spec_show_xray_zero",
    labelKey: "main.debug.quick_xray_off",
    group: "picture",
    variant: "on",
  },
  {
    key: "enable_spec_show_xray_zero",
    labelKey: "main.debug.quick_xray_restore",
    group: "picture",
    variant: "off",
  },
  {
    key: "use_shoulder_camera",
    labelKey: "main.debug.quick_shoulder_on",
    group: "picture",
    variant: "on",
  },
  {
    key: "use_shoulder_camera",
    labelKey: "main.debug.quick_shoulder_restore",
    group: "picture",
    variant: "off",
  },
  {
    key: "pov_radar_enabled",
    labelKey: "main.debug.quick_pov_radar_on",
    group: "picture",
    variant: "on",
  },
  {
    key: "pov_radar_enabled",
    labelKey: "main.debug.quick_pov_radar_off",
    group: "picture",
    variant: "off",
  },
  {
    key: "block_kill_feed",
    labelKey: "main.debug.quick_kill_feed_block",
    group: "picture",
    variant: "on",
  },
  {
    key: "block_kill_feed",
    labelKey: "main.debug.quick_kill_feed_clear",
    group: "picture",
    variant: "off",
  },
  {
    key: "kill_feed_lifetime",
    labelKey: "main.debug.quick_kill_feed_lifetime",
    group: "picture",
    variant: "current",
  },
  { key: "enable_voice", labelKey: "main.debug.quick_voice_on", group: "sound", variant: "on" },
  { key: "enable_voice", labelKey: "main.debug.quick_voice_off", group: "sound", variant: "off" },
];

/**
 * Turn the backend console plan into console quick commands. A missing plan, a
 * missing setting row, or an empty command array yields no option, so a failed
 * draft/plan build can never fill an empty command that pretends to be valid.
 */
export function buildDebugQuickCommandOptions(
  plan: DebugConsolePlan | null,
): DebugQuickCommandOption[] {
  if (!plan) {
    return [];
  }
  const options: DebugQuickCommandOption[] = [];
  for (const descriptor of QUICK_COMMAND_DESCRIPTORS) {
    const item = plan.settings.find((entry) => entry.key === descriptor.key);
    if (!item) {
      continue;
    }
    const command = debugSettingCommandsFor(item, descriptor.variant !== "off").join("; ");
    if (!command) {
      continue;
    }
    options.push({
      key: `${descriptor.key}:${descriptor.variant}`,
      labelKey: descriptor.labelKey,
      group: descriptor.group,
      command,
    });
  }
  return options;
}
