import type {
  ClipPrimaryView,
  DemoClipKill,
  DemoClipRound,
  DemoHitGroup,
  DemoMetadata,
  DemoWeaponClass,
} from "@/shared/types";

/**
 * Which side of a kill the selected player has to be on for it to show up.
 * This is what the death-mode switch used to toggle.
 *
 * The role also decides the primary_view of any clip added from the list, so it
 * stays meaningful even when the player constraint itself is switched off — see
 * resolvePrimaryView.
 */
export type KillPlayerRole = "killer" | "victim";

/**
 * Dropdown value standing in for "don't constrain by player at all", which lets
 * the list answer questions like "every AWP headshot in this match". It is kept
 * out of the stored steam id so the previously picked player survives toggling
 * back, and so the existing default-player sync keeps working untouched.
 */
export const ALL_PLAYERS_VALUE = "__all_players__";

/** Killer health at or below this counts as a low-health kill. */
export const LOW_HEALTH_MAX = 20;

export type KillTrait =
  | "headshot"
  | "wallbang"
  | "noscope"
  | "through_smoke"
  | "airborne"
  | "low_health"
  | "opening_kill"
  | "clutch"
  | "multi_3k"
  | "attacker_blind"
  | "assisted_flash"
  | "ducking"
  | "scoped"
  | "victim_blinded"
  | "has_assist"
  | "team_kill"
  | "after_plant"
  | "multi_2k"
  | "multi_4k"
  | "ace";

/**
 * A kill filter. Conditions are ANDed across groups and ORed inside a group, so
 * "AWP or AK-47" and "headshot or wallbang" narrow the list the way the facet
 * lists on a shopping site do.
 *
 * Multi-kill levels live in `traits` rather than in a numeric field of their own
 * precisely because of that rule: as a separate group, "3K" would AND with the
 * trait chips and make a highlight preset like "headshots or wallbangs or 3Ks"
 * impossible to express.
 *
 * The selected player is deliberately NOT stored here. It stays in
 * selectedPlayerByDemo, which already owns defaulting and per-mode syncing, and
 * is passed into matchKill instead.
 */
export interface KillFilter {
  role: KillPlayerRole;
  /** When true the player constraint is dropped and every player's kills show. */
  ignore_player: boolean;
  /**
   * Weapons to keep, matched against DemoClipKill.weapon_name verbatim. Weapon
   * classes are not a separate condition: the picker expands a class into the
   * names actually present in the demo, so what is stored always matches what
   * the icons show as selected.
   */
  weapons: string[];
  traits: KillTrait[];
  hit_groups: DemoHitGroup[];
  /** Killer's side: "ct" and/or "t". */
  sides: string[];
  /** Inclusive round range; null means every round. */
  rounds: [number, number] | null;
  /** Inclusive distance range in metres; null means any distance. */
  distance: [number, number] | null;
}

export function createDefaultKillFilter(): KillFilter {
  return {
    role: "killer",
    ignore_player: false,
    weapons: [],
    traits: [],
    hit_groups: [],
    sides: [],
    rounds: null,
    distance: null,
  };
}

const TRAIT_PREDICATES: Record<KillTrait, (kill: DemoClipKill) => boolean> = {
  headshot: (k) => !!k.is_headshot,
  wallbang: (k) => !!k.is_wallbang,
  noscope: (k) => !!k.is_noscope,
  through_smoke: (k) => !!k.is_through_smoke,
  airborne: (k) => !!k.killer_airborne,
  low_health: (k) =>
    typeof k.killer_health === "number" && k.killer_health > 0 && k.killer_health <= LOW_HEALTH_MAX,
  opening_kill: (k) => !!k.is_opening_kill,
  clutch: (k) => !!k.is_clutch_kill,
  attacker_blind: (k) => !!k.is_attacker_blind,
  assisted_flash: (k) => !!k.is_assisted_flash,
  ducking: (k) => !!k.killer_ducking,
  scoped: (k) => !!k.killer_scoped,
  victim_blinded: (k) => !!k.victim_blinded,
  has_assist: (k) => !!k.has_assist,
  team_kill: (k) => !!k.is_team_kill,
  after_plant: (k) => !!k.bomb_planted,
  multi_2k: (k) => (k.killer_round_kills ?? 0) >= 2,
  multi_3k: (k) => (k.killer_round_kills ?? 0) >= 3,
  multi_4k: (k) => (k.killer_round_kills ?? 0) >= 4,
  ace: (k) => (k.killer_round_kills ?? 0) >= 5,
};

/**
 * Traits offered as one-click chips above the list, in display order. The rest
 * live in the advanced panel — they are either situational or so common that
 * they do not narrow anything down on their own.
 */
export const QUICK_TRAITS: KillTrait[] = [
  "headshot",
  "wallbang",
  "noscope",
  "through_smoke",
  "airborne",
  "low_health",
  "opening_kill",
  "clutch",
  "multi_3k",
];

export const ADVANCED_TRAITS: KillTrait[] = [
  "multi_2k",
  "multi_4k",
  "ace",
  "scoped",
  "ducking",
  "attacker_blind",
  "victim_blinded",
  "assisted_flash",
  "has_assist",
  "after_plant",
  "team_kill",
];

export const WEAPON_CLASSES: DemoWeaponClass[] = [
  "rifle",
  "sniper",
  "pistol",
  "smg",
  "shotgun",
  "machinegun",
  "grenade",
  "knife",
  "zeus",
  "other",
];

/** Hit groups worth filtering on. 4/6 (left arm/leg) are folded into 5/7 below. */
export const HIT_GROUPS: DemoHitGroup[] = [1, 8, 2, 3, 5, 7];

/**
 * Left and right limbs are separate hit groups in the demo but nobody wants to
 * filter one arm, so the left ones normalize onto their right counterpart.
 */
function normalizeHitGroup(value: number | undefined): DemoHitGroup {
  switch (value) {
    case 4:
      return 5;
    case 6:
      return 7;
    default:
      return (value ?? 0) as DemoHitGroup;
  }
}

export interface KillFilterPreset {
  id: string;
  patch: Partial<KillFilter>;
  /**
   * Weapon families the preset wants. They are expanded into the weapon names
   * the demo actually contains when the preset is applied, so the stored filter
   * and the selected icons never disagree.
   */
  weaponClasses?: DemoWeaponClass[];
}

/**
 * One-click starting points. Each replaces the condition groups it names and
 * leaves the player and role alone, so a preset narrows whoever is being looked
 * at rather than resetting the view.
 */
export const KILL_FILTER_PRESETS: KillFilterPreset[] = [
  {
    id: "highlight",
    patch: {
      traits: ["headshot", "wallbang", "noscope", "airborne", "clutch", "multi_3k", "through_smoke"],
      weapons: [],
    },
  },
  { id: "sniper", patch: { traits: [] }, weaponClasses: ["sniper"] },
  { id: "opening", patch: { traits: ["opening_kill"], weapons: [] } },
  { id: "clutch", patch: { traits: ["clutch"], weapons: [] } },
];

/** Weapon names in the demo that belong to any of the given classes. */
export function weaponNamesInClasses(
  kills: DemoClipKill[],
  classes: DemoWeaponClass[],
): string[] {
  const names = new Set<string>();
  for (const kill of kills) {
    if (kill.weapon_name && classes.includes((kill.weapon_class ?? "other") as DemoWeaponClass)) {
      names.add(kill.weapon_name);
    }
  }
  return [...names];
}

/** Turns a preset into a patch, resolving its weapon families against the demo. */
export function resolvePresetPatch(
  preset: KillFilterPreset,
  kills: DemoClipKill[],
): Partial<KillFilter> {
  const patch: Partial<KillFilter> = { ...preset.patch };
  if (preset.weaponClasses?.length) {
    patch.weapons = weaponNamesInClasses(kills, preset.weaponClasses);
  }
  return patch;
}

function matchesPlayer(kill: DemoClipKill, filter: KillFilter, playerSteamID: string): boolean {
  if (filter.ignore_player || !playerSteamID) return true;
  switch (filter.role) {
    case "victim":
      return kill.victim_steam_id === playerSteamID;
    default:
      return kill.killer_steam_id === playerSteamID;
  }
}

/** Tests one kill against the filter. An empty group means "no constraint". */
export function matchKill(kill: DemoClipKill, filter: KillFilter, playerSteamID: string): boolean {
  if (!matchesPlayer(kill, filter, playerSteamID)) return false;

  if (filter.weapons.length && !filter.weapons.includes(kill.weapon_name)) {
    return false;
  }

  if (filter.traits.length && !filter.traits.some((trait) => TRAIT_PREDICATES[trait](kill))) {
    return false;
  }

  if (filter.hit_groups.length && !filter.hit_groups.includes(normalizeHitGroup(kill.hit_group))) {
    return false;
  }

  if (filter.sides.length && !filter.sides.includes(String(kill.killer_side || "").toLowerCase())) {
    return false;
  }

  if (filter.rounds) {
    const [min, max] = filter.rounds;
    if (kill.round < min || kill.round > max) return false;
  }

  if (filter.distance) {
    const [min, max] = filter.distance;
    const distance = kill.distance ?? 0;
    if (distance < min || distance > max) return false;
  }

  return true;
}

export function filterKills(
  kills: DemoClipKill[],
  filter: KillFilter,
  playerSteamID: string,
): DemoClipKill[] {
  return kills.filter((kill) => matchKill(kill, filter, playerSteamID));
}

/**
 * Regroups a flat kill list into the per-round shape the list renders. Input is
 * expected in chronological order, which is how the backend emits meta.kills.
 */
export function groupKillsByRound(kills: DemoClipKill[]): DemoClipRound[] {
  const rounds: DemoClipRound[] = [];
  const byRound = new Map<number, DemoClipRound>();
  for (const kill of kills) {
    let round = byRound.get(kill.round);
    if (!round) {
      round = { round: kill.round, kills: [] };
      byRound.set(kill.round, round);
      rounds.push(round);
    }
    round.kills.push(kill);
  }
  return rounds.sort((a, b) => a.round - b.round);
}

/**
 * Every kill in the demo, flat and chronological.
 *
 * meta.kills is the source of truth. The fallback rebuilds the list out of the
 * per-killer grouping for metadata produced before that field existed, so a demo
 * imported by an older build still filters instead of showing an empty list.
 */
export function collectDemoKills(meta: DemoMetadata | undefined | null): DemoClipKill[] {
  if (!meta) return [];
  if (meta.kills?.length) return meta.kills;

  const seen = new Set<string>();
  const kills: DemoClipKill[] = [];
  for (const player of meta.clip_players || []) {
    for (const round of player.rounds || []) {
      for (const kill of round.kills || []) {
        if (seen.has(kill.id)) continue;
        seen.add(kill.id);
        kills.push(kill);
      }
    }
  }
  kills.sort((a, b) => (a.round === b.round ? a.tick - b.tick : a.round - b.round));
  return kills;
}

export interface DemoWeaponEntry {
  name: string;
  weaponClass: DemoWeaponClass;
  count: number;
}

export interface DemoWeaponGroup {
  weaponClass: DemoWeaponClass;
  weapons: DemoWeaponEntry[];
}

/**
 * Weapons that actually appear in the demo, grouped by family in WEAPON_CLASSES
 * order and sorted by frequency inside each. The picker only ever offers guns
 * that got a kill here, so there are no dead options to scan past.
 */
export function groupDemoWeapons(kills: DemoClipKill[]): DemoWeaponGroup[] {
  const byName = new Map<string, DemoWeaponEntry>();
  for (const kill of kills) {
    const name = kill.weapon_name;
    if (!name) continue;
    const existing = byName.get(name);
    if (existing) {
      existing.count++;
      continue;
    }
    byName.set(name, {
      name,
      weaponClass: (kill.weapon_class ?? "other") as DemoWeaponClass,
      count: 1,
    });
  }

  const groups: DemoWeaponGroup[] = [];
  for (const weaponClass of WEAPON_CLASSES) {
    const weapons = [...byName.values()]
      .filter((entry) => entry.weaponClass === weaponClass)
      .sort((a, b) => b.count - a.count);
    if (weapons.length) groups.push({ weaponClass, weapons });
  }
  return groups;
}

export function maxKillDistance(kills: DemoClipKill[]): number {
  let max = 0;
  for (const kill of kills) {
    max = Math.max(max, kill.distance ?? 0);
  }
  return Math.ceil(max);
}

/** How many condition groups are set, for the "clear" button and badge counts. */
export function countActiveConditions(filter: KillFilter): number {
  let count = 0;
  if (filter.weapons.length) count++;
  if (filter.traits.length) count++;
  if (filter.hit_groups.length) count++;
  if (filter.sides.length) count++;
  if (filter.rounds) count++;
  if (filter.distance) count++;
  return count;
}

export function isKillFilterActive(filter: KillFilter): boolean {
  return countActiveConditions(filter) > 0;
}

/** Clears every condition but keeps who is being looked at and from which side. */
export function clearKillFilterConditions(filter: KillFilter): KillFilter {
  return {
    ...createDefaultKillFilter(),
    role: filter.role,
    ignore_player: filter.ignore_player,
  };
}

/**
 * Which camera a clip added from the list leads with. The role names a side of
 * the kill and the primary view is that same side, so the two are one and the
 * same value — kept as a function so callers read as intent, not coincidence.
 */
export function resolvePrimaryView(filter: KillFilter): ClipPrimaryView {
  return filter.role;
}
