/**
 * Maps the weapon names the demo parser reports onto the CS2 icon assets under
 * `public/cs2/weapon`. Shared by the kill feed and the clip filter so both call
 * a weapon by the same picture and the same label.
 */

const weaponDisplay: Record<string, string> = {
  ak47: "AK-47",
  aug: "AUG",
  awp: "AWP",
  bizon: "PP-Bizon",
  cz75a: "CZ75-Auto",
  deagle: "Desert Eagle",
  elite: "Dual Berettas",
  famas: "FAMAS",
  fiveseven: "Five-SeveN",
  galilar: "Galil AR",
  g3sg1: "G3SG1",
  glock: "Glock-18",
  hegrenade: "HE Grenade",
  hkp2000: "P2000",
  incgrenade: "Incendiary",
  knife: "Knife",
  m249: "M249",
  m4a1: "M4A4",
  m4a1_silencer: "M4A1-S",
  mac10: "MAC-10",
  mag7: "MAG-7",
  molotov: "Molotov",
  mp5sd: "MP5-SD",
  mp7: "MP7",
  mp9: "MP9",
  negev: "Negev",
  nova: "Nova",
  p250: "P250",
  p90: "P90",
  revolver: "R8 Revolver",
  sawedoff: "Sawed-Off",
  scar20: "SCAR-20",
  sg556: "SG 553",
  smokegrenade: "Smoke Grenade",
  ssg08: "SSG 08",
  taser: "Zeus x27",
  tec9: "Tec-9",
  ump45: "UMP-45",
  usp_silencer: "USP-S",
  xm1014: "XM1014",
};

const weaponAliases: Array<{ match: RegExp; id: string }> = [
  { match: /ak-?47/, id: "ak47" },
  { match: /awp/, id: "awp" },
  { match: /m4a4/, id: "m4a1" },
  { match: /m4a1[-_ ]s|m4a1s/, id: "m4a1_silencer" },
  { match: /m4a1/, id: "m4a1_silencer" },
  { match: /usp-?s/, id: "usp_silencer" },
  { match: /usp/, id: "usp_silencer" },
  { match: /glock/, id: "glock" },
  { match: /deagle|desert\s*eagle/, id: "deagle" },
  { match: /p250/, id: "p250" },
  { match: /p2000/, id: "p2000" },
  { match: /famas/, id: "famas" },
  { match: /galil|galilar/, id: "galilar" },
  { match: /sg\s*553|sg556/, id: "sg556" },
  { match: /aug/, id: "aug" },
  { match: /ssg\s*08/, id: "ssg08" },
  { match: /g3sg1/, id: "g3sg1" },
  { match: /scar\s*20|scar20/, id: "scar20" },
  { match: /mp9/, id: "mp9" },
  { match: /mp7/, id: "mp7" },
  { match: /mac-?10/, id: "mac10" },
  { match: /ump45/, id: "ump45" },
  { match: /p90/, id: "p90" },
  { match: /bizon|pp-?bizon/, id: "bizon" },
  { match: /mp5/, id: "mp5sd" },
  { match: /nova/, id: "nova" },
  { match: /mag-?7/, id: "mag7" },
  { match: /xm1014/, id: "xm1014" },
  { match: /m249/, id: "m249" },
  { match: /negev/, id: "negev" },
  { match: /he\s*grenade|hegrenade/, id: "hegrenade" },
  { match: /flashbang/, id: "flashbang" },
  { match: /smoke/, id: "smokegrenade" },
  { match: /molotov/, id: "molotov" },
  { match: /incendiary|incgrenade/, id: "incgrenade" },
  { match: /decoy/, id: "decoy" },
  { match: /knife/, id: "knife" },
];

function normalizeKey(value: unknown): string {
  return String(value || "")
    .toLowerCase()
    .replace(/^weapon_/, "")
    .replace(/[\s-]/g, "")
    .replace(/[^a-z0-9_]/g, "");
}

/** Resolves a reported weapon name to the asset id its icon is filed under. */
export function resolveWeaponID(name: string): string {
  if (!name) return "";
  const lower = name.toLowerCase();
  for (const rule of weaponAliases) {
    if (rule.match.test(lower)) {
      return rule.id;
    }
  }
  return normalizeKey(name);
}

export function weaponIconSrc(id: string): string {
  return `/cs2/weapon/${id}.svg`;
}

/** Display name for a weapon, falling back to the raw name when unmapped. */
export function weaponLabel(name: string): string {
  const id = resolveWeaponID(name);
  if (weaponDisplay[id]) return weaponDisplay[id];
  return String(name || "weapon").replace(/^weapon_/, "");
}

export function deathNoticeIconSrc(name: string): string {
  return `/cs2/deathnotice/${name}.svg`;
}
