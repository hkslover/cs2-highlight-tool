package demo

import "testing"

func sampleKills() []ClipKill {
	return []ClipKill{
		{ID: "k1", Round: 1, Tick: 100, KillerSteamID: "A", KillerName: "alice", VictimSteamID: "B", VictimName: "bob"},
		{ID: "k2", Round: 1, Tick: 200, KillerSteamID: "A", KillerName: "alice", VictimSteamID: "C", VictimName: "carol"},
		{ID: "k3", Round: 2, Tick: 300, KillerSteamID: "B", KillerName: "bob", VictimSteamID: "A", VictimName: "alice"},
		{ID: "k4", Round: 2, Tick: 400, KillerSteamID: "C", KillerName: "carol", VictimSteamID: "B", VictimName: "bob"},
	}
}

func findClipPlayer(players []ClipPlayer, steamID string) *ClipPlayer {
	for i := range players {
		if players[i].SteamID == steamID {
			return &players[i]
		}
	}
	return nil
}

func TestBuildDeathPlayersGroupsByVictim(t *testing.T) {
	players := buildDeathPlayers(sampleKills())

	// bob dies twice, in two different rounds; that is the most deaths, so he
	// sorts first.
	if len(players) != 3 {
		t.Fatalf("expected 3 victims, got %d", len(players))
	}
	if players[0].SteamID != "B" {
		t.Fatalf("expected most-killed player first, got %q", players[0].SteamID)
	}

	bob := findClipPlayer(players, "B")
	if bob.TotalDeaths != 2 {
		t.Fatalf("bob total_deaths = %d, want 2", bob.TotalDeaths)
	}
	if bob.TotalKills != 0 {
		t.Fatalf("death entries must not report total_kills, got %d", bob.TotalKills)
	}
	if len(bob.Rounds) != 2 {
		t.Fatalf("bob rounds = %d, want 2", len(bob.Rounds))
	}
	if bob.Rounds[0].Round != 1 || bob.Rounds[1].Round != 2 {
		t.Fatalf("bob rounds not sorted: %+v", bob.Rounds)
	}
	if got := bob.Rounds[0].Kills[0].ID; got != "k1" {
		t.Fatalf("bob round 1 kill = %q, want k1", got)
	}

	// The kill entries keep both sides, so the recorder can still spec either.
	kill := bob.Rounds[0].Kills[0]
	if kill.KillerSteamID != "A" || kill.VictimSteamID != "B" {
		t.Fatalf("death entry lost a side: %+v", kill)
	}
}

func TestBuildClipPlayersStillGroupsByKiller(t *testing.T) {
	players := buildClipPlayers(sampleKills())

	alice := findClipPlayer(players, "A")
	if alice == nil {
		t.Fatal("alice missing from clip players")
	}
	if alice.TotalKills != 2 {
		t.Fatalf("alice total_kills = %d, want 2", alice.TotalKills)
	}
	if alice.TotalDeaths != 0 {
		t.Fatalf("kill entries must not report total_deaths, got %d", alice.TotalDeaths)
	}
	if players[0].SteamID != "A" {
		t.Fatalf("expected top fragger first, got %q", players[0].SteamID)
	}
}

func TestGroupClipKillsSkipsEntriesWithoutSteamID(t *testing.T) {
	kills := []ClipKill{{ID: "k1", Round: 1, Tick: 10, KillerSteamID: "A", VictimSteamID: ""}}
	if got := buildDeathPlayers(kills); len(got) != 0 {
		t.Fatalf("expected no victims, got %+v", got)
	}
	if got := buildClipPlayers(kills); len(got) != 1 {
		t.Fatalf("expected 1 killer, got %+v", got)
	}
}
