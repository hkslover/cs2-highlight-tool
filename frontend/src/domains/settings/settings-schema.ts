import type { ClipSettings } from "@/shared/types";

export type SearchableSwitchSettingKey =
  | "enable_voice"
  | "enable_spec_show_xray_zero"
  | "hide_all_ui"
  | "hide_player_avatars"
  | "use_shoulder_camera"
  | "pov_hud_enabled"
  | "pov_radar_enabled"
  | "sky_blackout"
  | "disable_clouds"
  | "block_kill_feed";

export type SearchableNumberSettingKey = "kill_feed_lifetime";

export type SearchableClipSettingItem =
  | {
      key: SearchableSwitchSettingKey;
      labelKey: string;
      kind: "switch";
      aliases: string[];
      hintKey?: string;
    }
  | {
      key: SearchableNumberSettingKey;
      labelKey: string;
      kind: "number";
      aliases: string[];
      hintKey?: string;
      min: number;
      max: number;
      step: number;
      precision: number;
    };

export const SEARCHABLE_CLIP_SETTINGS: readonly SearchableClipSettingItem[] = [
  {
    key: "enable_voice",
    labelKey: "main.settings.enable_voice",
    kind: "switch",
    aliases: ["启用队伍语音", "队伍语音", "team voice", "voice"],
  },
  {
    key: "enable_spec_show_xray_zero",
    labelKey: "main.settings.enable_spec_show_xray_zero",
    kind: "switch",
    aliases: ["关闭x光", "关闭 x 光", "xray", "x-ray", "x光", "spec_show_xray"],
  },
  {
    key: "hide_all_ui",
    labelKey: "main.settings.hide_all_ui",
    kind: "switch",
    aliases: ["隐藏所有ui", "hidden ui", "hide ui"],
  },
  {
    key: "hide_player_avatars",
    labelKey: "main.settings.hide_player_avatars",
    kind: "switch",
    aliases: ["隐藏玩家头像", "玩家头像", "player avatars", "teamcounter"],
  },
  {
    key: "use_shoulder_camera",
    labelKey: "main.settings.use_shoulder_camera",
    kind: "switch",
    aliases: ["越肩视角", "shoulder camera", "camera"],
  },
  {
    key: "pov_hud_enabled",
    labelKey: "main.settings.pov_hud_enabled",
    kind: "switch",
    aliases: ["pov hud", "hud"],
  },
  {
    key: "pov_radar_enabled",
    labelKey: "main.settings.pov_radar_enabled",
    kind: "switch",
    aliases: ["pov雷达", "pov radar", "radar"],
    hintKey: "main.settings.pov_radar_hint",
  },
  {
    key: "sky_blackout",
    labelKey: "main.settings.sky_blackout",
    kind: "switch",
    aliases: ["天空变黑", "sky", "blackout", "drawskybox"],
  },
  {
    key: "disable_clouds",
    labelKey: "main.settings.disable_clouds",
    kind: "switch",
    aliases: ["关闭云层", "cloud", "clouds"],
  },
  {
    key: "kill_feed_lifetime",
    labelKey: "main.settings.kill_feed_lifetime",
    kind: "number",
    aliases: ["击杀信息留存", "kill feed", "death notice", "deathnotice"],
    min: 1,
    max: 10,
    step: 1,
    precision: 0,
  },
  {
    key: "block_kill_feed",
    labelKey: "main.settings.block_kill_feed",
    kind: "switch",
    aliases: ["屏蔽击杀信息", "block kill feed", "kill feed"],
  },
];

export function normalizeSettingSearch(value: string): string {
  return value.trim().toLowerCase().replace(/\s+/g, "");
}

export function settingMatchesSearch(
  item: SearchableClipSettingItem,
  query: string,
  localizedLabel: string,
): boolean {
  const normalizedQuery = normalizeSettingSearch(query);
  if (!normalizedQuery) {
    return true;
  }
  const haystack = normalizeSettingSearch([localizedLabel, item.key, ...item.aliases].join(" "));
  if (haystack.includes(normalizedQuery)) {
    return true;
  }
  let queryIndex = 0;
  for (const char of haystack) {
    if (char === normalizedQuery[queryIndex]) {
      queryIndex += 1;
    }
    if (queryIndex === normalizedQuery.length) {
      return true;
    }
  }
  return false;
}

export function isSearchableSwitchSetting(
  item: SearchableClipSettingItem,
): item is Extract<SearchableClipSettingItem, { kind: "switch" }> {
  return item.kind === "switch";
}

export function isSearchableNumberSetting(
  item: SearchableClipSettingItem,
): item is Extract<SearchableClipSettingItem, { kind: "number" }> {
  return item.kind === "number";
}

export function getSearchableSwitchValue(settings: ClipSettings, item: SearchableClipSettingItem): boolean {
  return isSearchableSwitchSetting(item) ? settings[item.key] : false;
}

export function getSearchableNumberValue(settings: ClipSettings, item: SearchableClipSettingItem): number {
  return isSearchableNumberSetting(item) ? settings[item.key] : 0;
}

