package demo

import (
	"sort"

	common "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
)

// Weapon class buckets attached to every ClipKill so the clip page can filter by
// weapon family instead of by individual weapon name. These strings are a stable
// contract with the frontend — do not rename them.
const (
	WeaponClassPistol     = "pistol"
	WeaponClassSMG        = "smg"
	WeaponClassRifle      = "rifle"
	WeaponClassSniper     = "sniper"
	WeaponClassShotgun    = "shotgun"
	WeaponClassMachineGun = "machinegun"
	WeaponClassGrenade    = "grenade"
	WeaponClassKnife      = "knife"
	WeaponClassZeus       = "zeus"
	WeaponClassOther      = "other"
)

// classifyWeapon buckets an equipment type into the class the clip filter offers.
//
// It deliberately does not use common.EquipmentType.Class(): that helper derives
// the class from the numeric constant range, which lumps the AWP and the SSG-08
// in with the AK-47 as EqClassRifle, and lumps the M249 and Negev in with the
// shotguns as EqClassHeavy. "Show me only AWP frags" is one of the main reasons
// to have this filter at all, so snipers and machine guns get their own bucket
// here and the library classes are used only as the fallback for types this
// switch does not name.
func classifyWeapon(t common.EquipmentType) string {
	switch t {
	case common.EqSSG08, common.EqAWP, common.EqScar20, common.EqG3SG1:
		return WeaponClassSniper
	case common.EqSawedOff, common.EqNova, common.EqSwag7, common.EqXM1014:
		return WeaponClassShotgun
	case common.EqM249, common.EqNegev:
		return WeaponClassMachineGun
	case common.EqKnife:
		return WeaponClassKnife
	case common.EqZeus:
		return WeaponClassZeus
	}

	switch t.Class() {
	case common.EqClassPistols:
		return WeaponClassPistol
	case common.EqClassSMG:
		return WeaponClassSMG
	case common.EqClassRifle:
		return WeaponClassRifle
	case common.EqClassGrenade:
		return WeaponClassGrenade
	case common.EqClassHeavy:
		// Every heavy the game ships is named above; anything left is a shotgun
		// variant we do not know about yet.
		return WeaponClassShotgun
	}
	return WeaponClassOther
}

// weaponAssetMapping maps an EquipmentType to the corresponding SVG icon asset
// filename under frontend/public/cs2/weapon (without the .svg extension).
// When an equipment type has no SVG asset or is unknown, it maps to an empty string,
// signalling the UI to display the weapon's text name instead of an icon.
var weaponAssetMapping = map[common.EquipmentType]string{
	// Pistols
	common.EqP2000:        "p2000",
	common.EqGlock:        "glock",
	common.EqP250:         "p250",
	common.EqDeagle:       "deagle",
	common.EqFiveSeven:    "fiveseven",
	common.EqDualBerettas: "elite",
	common.EqTec9:         "tec9",
	common.EqCZ:           "cz75a",
	common.EqUSP:          "usp_silencer",
	common.EqRevolver:     "revolver",

	// SMGs
	common.EqMP7:   "mp7",
	common.EqMP9:   "mp9",
	common.EqBizon: "bizon",
	common.EqMac10: "mac10",
	common.EqUMP:   "ump45",
	common.EqP90:   "p90",
	common.EqMP5:   "mp5sd",

	// Shotguns & Machine Guns
	common.EqSawedOff: "sawedoff",
	common.EqNova:     "nova",
	common.EqSwag7:    "mag7",
	common.EqXM1014:   "xm1014",
	common.EqM249:     "m249",
	common.EqNegev:    "negev",

	// Rifles
	common.EqGalil:  "galilar",
	common.EqFamas:  "famas",
	common.EqAK47:   "ak47",
	common.EqM4A4:   "m4a1",          // CS2 SVG asset for M4A4 is m4a1.svg
	common.EqM4A1:   "m4a1_silencer", // CS2 SVG asset for M4A1-S is m4a1_silencer.svg
	common.EqSSG08:  "ssg08",
	common.EqSG556:  "sg556",
	common.EqAUG:    "aug",
	common.EqAWP:    "awp",
	common.EqScar20: "scar20",
	common.EqG3SG1:  "g3sg1",

	// Grenades & Equipment
	common.EqZeus:       "taser",
	common.EqKnife:      "knife",
	common.EqDecoy:      "decoy",
	common.EqMolotov:    "molotov",
	common.EqIncendiary: "incgrenade",
	common.EqFlash:      "flashbang",
	common.EqSmoke:      "smokegrenade",
	common.EqHE:         "hegrenade",
	common.EqBomb:       "c4",

	// Special Melee / Equipment
	common.EqFists:    "fists",
	common.EqAxe:      "axe",
	common.EqHammer:   "hammer",
	common.EqWrench:   "spanner",
	common.EqSnowball: "snowball",
	common.EqShield:   "shield",
}

// ResolveWeaponAssetID returns the exact SVG filename (without extension) for the
// given equipment type, or an empty string if no SVG asset exists.
func ResolveWeaponAssetID(t common.EquipmentType) string {
	return weaponAssetMapping[t]
}

// alivePlayer is a minimal snapshot of one player's liveness, taken at the tick a
// kill lands so the clutch check can stay a pure, testable function.
type alivePlayer struct {
	SteamID uint64
	Team    common.Team
	Alive   bool
}

// isClutchKill reports whether the killer was the last player standing on their
// team when the kill landed.
//
// Only living teammates are counted, never living enemies. Inside a Kill event
// the victim is still flagged alive, so an enemy-side count would be off by one;
// worse, requiring "at least one enemy left" after excluding the victim would
// drop the kill that actually closes the clutch out — the single most watchable
// frag in the round. Since the victim is by definition a living enemy, "no
// teammate alive" is the whole condition.
func isClutchKill(players []alivePlayer, killerSteamID uint64, killerTeam, victimTeam common.Team) bool {
	if !isPlayingTeam(killerTeam) || !isPlayingTeam(victimTeam) || killerTeam == victimTeam {
		return false
	}
	for _, p := range players {
		if !p.Alive || p.SteamID == killerSteamID {
			continue
		}
		if p.Team == killerTeam {
			return false
		}
	}
	return true
}

func isPlayingTeam(team common.Team) bool {
	return team == common.TeamCounterTerrorists || team == common.TeamTerrorists
}

// annotateRoundContext fills in the per-round facts that cannot be known while a
// kill is being handled, because they depend on kills that come later in the same
// round: whether a kill opened the round, and how many frags its killer ended the
// round with (the 2K/3K/4K/ACE filter).
//
// It runs once over every kill after parsing, so it stays independent of event
// ordering quirks such as RoundStart not firing for the opening round.
func annotateRoundContext(kills []ClipKill) {
	if len(kills) == 0 {
		return
	}

	type killerRound struct {
		round   int
		steamID string
	}

	openingIdxByRound := make(map[int]int)
	fragsByKillerRound := make(map[killerRound]int)

	for i := range kills {
		kill := &kills[i]

		best, ok := openingIdxByRound[kill.Round]
		if !ok || kill.Tick < kills[best].Tick || (kill.Tick == kills[best].Tick && kill.ID < kills[best].ID) {
			openingIdxByRound[kill.Round] = i
		}

		// Team kills and suicides are not frags; counting them would inflate a
		// player into a fake 3K and pollute the multi-kill filter.
		if kill.IsTeamKill || kill.KillerSteamID == "" {
			continue
		}
		fragsByKillerRound[killerRound{round: kill.Round, steamID: kill.KillerSteamID}]++
	}

	for _, idx := range openingIdxByRound {
		kills[idx].IsOpeningKill = true
	}
	for i := range kills {
		kill := &kills[i]
		if kill.IsTeamKill || kill.KillerSteamID == "" {
			continue
		}
		kill.KillerRoundKills = fragsByKillerRound[killerRound{round: kill.Round, steamID: kill.KillerSteamID}]
	}
}

// sortKillsChronologically orders the flat kill list by round, then by tick, so
// the frontend can filter and regroup it without having to sort first.
func sortKillsChronologically(kills []ClipKill) {
	sort.SliceStable(kills, func(i, j int) bool {
		if kills[i].Round != kills[j].Round {
			return kills[i].Round < kills[j].Round
		}
		if kills[i].Tick != kills[j].Tick {
			return kills[i].Tick < kills[j].Tick
		}
		return kills[i].ID < kills[j].ID
	})
}
