package demo

import (
	"testing"

	common "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
)

func TestClassifyWeaponSplitsSnipersAndMachineGunsOut(t *testing.T) {
	cases := []struct {
		weapon common.EquipmentType
		want   string
	}{
		// The library's own Class() calls all four of these EqClassRifle; the
		// whole point of classifyWeapon is that the snipers come out separately.
		{common.EqAK47, WeaponClassRifle},
		{common.EqM4A1, WeaponClassRifle},
		{common.EqAWP, WeaponClassSniper},
		{common.EqSSG08, WeaponClassSniper},
		{common.EqScar20, WeaponClassSniper},
		{common.EqG3SG1, WeaponClassSniper},
		// ...and Class() calls both of these EqClassHeavy alongside the shotguns.
		{common.EqM249, WeaponClassMachineGun},
		{common.EqNegev, WeaponClassMachineGun},
		{common.EqXM1014, WeaponClassShotgun},
		{common.EqNova, WeaponClassShotgun},

		{common.EqDeagle, WeaponClassPistol},
		{common.EqGlock, WeaponClassPistol},
		{common.EqMP9, WeaponClassSMG},
		{common.EqHE, WeaponClassGrenade},
		{common.EqMolotov, WeaponClassGrenade},
		{common.EqKnife, WeaponClassKnife},
		{common.EqZeus, WeaponClassZeus},
		{common.EqWorld, WeaponClassOther},
		{common.EqUnknown, WeaponClassOther},
	}

	for _, tc := range cases {
		if got := classifyWeapon(tc.weapon); got != tc.want {
			t.Errorf("classifyWeapon(%s) = %q, want %q", tc.weapon, got, tc.want)
		}
	}
}

func TestClassifyWeaponNeverLeavesAWeaponUnclassified(t *testing.T) {
	// Every weapon that can appear in a kill feed must land in a real bucket, so
	// a "select all rifles" style filter cannot silently drop frags.
	guns := []common.EquipmentType{
		common.EqP2000, common.EqGlock, common.EqP250, common.EqDeagle, common.EqFiveSeven,
		common.EqDualBerettas, common.EqTec9, common.EqCZ, common.EqUSP, common.EqRevolver,
		common.EqMP7, common.EqMP9, common.EqBizon, common.EqMac10, common.EqUMP,
		common.EqP90, common.EqMP5,
		common.EqSawedOff, common.EqNova, common.EqSwag7, common.EqXM1014,
		common.EqM249, common.EqNegev,
		common.EqGalil, common.EqFamas, common.EqAK47, common.EqM4A4, common.EqM4A1,
		common.EqSSG08, common.EqSG553, common.EqAUG, common.EqAWP, common.EqScar20, common.EqG3SG1,
		common.EqHE, common.EqMolotov, common.EqIncendiary, common.EqFlash, common.EqSmoke, common.EqDecoy,
		common.EqKnife, common.EqZeus,
	}
	for _, gun := range guns {
		if got := classifyWeapon(gun); got == WeaponClassOther {
			t.Errorf("classifyWeapon(%s) fell through to %q", gun, WeaponClassOther)
		}
	}
}

func ctPlayer(steamID uint64, alive bool) alivePlayer {
	return alivePlayer{SteamID: steamID, Team: common.TeamCounterTerrorists, Alive: alive}
}

func tPlayer(steamID uint64, alive bool) alivePlayer {
	return alivePlayer{SteamID: steamID, Team: common.TeamTerrorists, Alive: alive}
}

func TestIsClutchKillRequiresNoLivingTeammate(t *testing.T) {
	// Killer 1 (CT) is alone; two Ts are still up, one of them the victim.
	players := []alivePlayer{
		ctPlayer(1, true), ctPlayer(2, false),
		tPlayer(10, true), tPlayer(11, true),
	}
	if !isClutchKill(players, 1, common.TeamCounterTerrorists, common.TeamTerrorists) {
		t.Fatal("expected a 1vX kill to count as a clutch")
	}

	players[1] = ctPlayer(2, true)
	if isClutchKill(players, 1, common.TeamCounterTerrorists, common.TeamTerrorists) {
		t.Fatal("a living teammate must disqualify the clutch")
	}
}

func TestIsClutchKillCountsTheKillThatClosesItOut(t *testing.T) {
	// The victim is the last enemy standing. Inside a Kill event they are still
	// flagged alive, and this kill — the one that wins the round — is exactly the
	// frag the filter exists to surface, so it must still register.
	players := []alivePlayer{
		ctPlayer(1, true), ctPlayer(2, false),
		tPlayer(10, true), tPlayer(11, false),
	}
	if !isClutchKill(players, 1, common.TeamCounterTerrorists, common.TeamTerrorists) {
		t.Fatal("the clutch-closing kill must count as a clutch kill")
	}
}

func TestIsClutchKillIgnoresTeamKillsAndSpectators(t *testing.T) {
	players := []alivePlayer{ctPlayer(1, true), tPlayer(10, true)}

	if isClutchKill(players, 1, common.TeamCounterTerrorists, common.TeamCounterTerrorists) {
		t.Fatal("a team kill must not count as a clutch")
	}
	if isClutchKill(players, 1, common.TeamSpectators, common.TeamTerrorists) {
		t.Fatal("a killer with no playing team must not count as a clutch")
	}
	if isClutchKill(players, 1, common.TeamCounterTerrorists, common.TeamUnassigned) {
		t.Fatal("a victim with no playing team must not count as a clutch")
	}
}

func TestAnnotateRoundContextMarksOpeningKillPerRound(t *testing.T) {
	kills := []ClipKill{
		{ID: "k2", Round: 1, Tick: 200, KillerSteamID: "A"},
		{ID: "k1", Round: 1, Tick: 100, KillerSteamID: "A"},
		{ID: "k3", Round: 2, Tick: 300, KillerSteamID: "B"},
	}
	annotateRoundContext(kills)

	if kills[0].IsOpeningKill {
		t.Error("k2 is not the earliest kill of round 1")
	}
	if !kills[1].IsOpeningKill {
		t.Error("k1 is the earliest kill of round 1 even though it is listed second")
	}
	if !kills[2].IsOpeningKill {
		t.Error("the only kill of round 2 must be its opening kill")
	}
}

func TestAnnotateRoundContextCountsMultiKillsPerRound(t *testing.T) {
	kills := []ClipKill{
		{ID: "k1", Round: 1, Tick: 100, KillerSteamID: "A"},
		{ID: "k2", Round: 1, Tick: 200, KillerSteamID: "A"},
		{ID: "k3", Round: 1, Tick: 300, KillerSteamID: "A"},
		{ID: "k4", Round: 1, Tick: 400, KillerSteamID: "B"},
		// Same killer, next round: the count must not carry over.
		{ID: "k5", Round: 2, Tick: 500, KillerSteamID: "A"},
	}
	annotateRoundContext(kills)

	for i := 0; i < 3; i++ {
		if kills[i].KillerRoundKills != 3 {
			t.Errorf("%s killer_round_kills = %d, want 3", kills[i].ID, kills[i].KillerRoundKills)
		}
	}
	if kills[3].KillerRoundKills != 1 {
		t.Errorf("k4 killer_round_kills = %d, want 1", kills[3].KillerRoundKills)
	}
	if kills[4].KillerRoundKills != 1 {
		t.Errorf("k5 killer_round_kills = %d, want 1 (rounds must not accumulate)", kills[4].KillerRoundKills)
	}
}

func TestAnnotateRoundContextExcludesTeamKillsFromMultiKills(t *testing.T) {
	kills := []ClipKill{
		{ID: "k1", Round: 1, Tick: 100, KillerSteamID: "A"},
		{ID: "k2", Round: 1, Tick: 200, KillerSteamID: "A"},
		{ID: "k3", Round: 1, Tick: 300, KillerSteamID: "A", IsTeamKill: true},
	}
	annotateRoundContext(kills)

	if kills[0].KillerRoundKills != 2 || kills[1].KillerRoundKills != 2 {
		t.Errorf("a team kill must not inflate a 2K into a 3K: got %d and %d",
			kills[0].KillerRoundKills, kills[1].KillerRoundKills)
	}
	if kills[2].KillerRoundKills != 0 {
		t.Errorf("the team kill itself should carry 0, got %d", kills[2].KillerRoundKills)
	}
}

func TestAnnotateRoundContextHandlesEmptyInput(t *testing.T) {
	annotateRoundContext(nil)
	annotateRoundContext([]ClipKill{})
}

func TestSortKillsChronologically(t *testing.T) {
	kills := []ClipKill{
		{ID: "k4", Round: 2, Tick: 50},
		{ID: "k2", Round: 1, Tick: 200},
		{ID: "k1", Round: 1, Tick: 100},
		{ID: "k3", Round: 1, Tick: 200},
	}
	sortKillsChronologically(kills)

	want := []string{"k1", "k2", "k3", "k4"}
	for i, id := range want {
		if kills[i].ID != id {
			t.Fatalf("position %d = %q, want %q (full order %+v)", i, kills[i].ID, id, kills)
		}
	}
}
