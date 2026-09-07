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
  p2000: "P2000",
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
  flashbang: "Flashbang",
  decoy: "Decoy Grenade",
};

export function weaponIconSrc(id: string): string {
  return `/cs2/weapon/${id}.svg`;
}

/** Display name for a weapon, falling back to the raw name when unmapped. */
export function weaponLabel(name: string, assetId?: string): string {
  if (assetId && weaponDisplay[assetId]) return weaponDisplay[assetId];
  return String(name || "").replace(/^weapon_/, "");
}

export function deathNoticeIconSrc(name: string): string {
  return `/cs2/deathnotice/${name}.svg`;
}

