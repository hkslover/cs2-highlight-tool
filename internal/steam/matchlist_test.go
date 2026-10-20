package steam

import (
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
)

func roundStats(reservationID uint64, demoURL string, accountIDs []uint32, gameType uint32) *CMsgGCCStrike15V2_MatchmakingServerRoundStats {
	return &CMsgGCCStrike15V2_MatchmakingServerRoundStats{
		Reservationid: proto.Uint64(reservationID),
		Map:           proto.String(demoURL),
		Reservation: &CMsgGCCStrike15V2_MatchmakingGC2ServerReserve{
			AccountIds: accountIDs,
			GameType:   proto.Uint32(gameType),
		},
		Kills:         []int32{25, 4, 1, 2, 0, 14, 17, 19, 10, 33},
		Deaths:        []int32{10, 12, 20, 11, 8, 15, 13, 14, 9, 12},
		Assists:       []int32{5, 2, 1, 0, 3, 4, 2, 1, 0, 6},
		TeamScores:    []int32{13, 10},
		MatchResult:   proto.Int32(1),
		MatchDuration: proto.Int32(2601),
		MaxRounds:     proto.Uint32(24),
	}
}

// accountIDs is a fixed 10 slot player list: slots 0-4 started as CT, 5-9 as T.
var accountIDs = []uint32{1187846741, 1561956913, 1561535625, 140892686, 1652142050, 169950553, 146157177, 313147942, 881042405, 164035628}

func wrapMatchList(t *testing.T, matches ...*CDataGCCStrike15V2_MatchInfo) []byte {
	t.Helper()
	raw, err := proto.Marshal(&CMsgGCCStrike15V2_MatchList{
		Msgrequestid: proto.Uint32(1),
		Accountid:    proto.Uint32(accountIDs[0]),
		Matches:      matches,
	})
	if err != nil {
		t.Fatalf("marshal match list: %v", err)
	}
	return raw
}

func TestParseMatchListDerivesDemoAndShareCode(t *testing.T) {
	match := &CDataGCCStrike15V2_MatchInfo{
		Matchid:   proto.Uint64(3668142292599833094),
		Matchtime: proto.Uint32(1708111862),
		Watchablematchinfo: &WatchableMatchInfo{
			ServerIp: proto.Uint32(273),
			TvPort:   proto.Uint32(1331187857),
		},
		Roundstatsall: []*CMsgGCCStrike15V2_MatchmakingServerRoundStats{
			roundStats(0, "", accountIDs, 0),
			roundStats(3668148000611369297, "http://replay273.valve.net/730/003668148000611369297_1331187857.dem.bz2", accountIDs, 4104),
		},
	}

	items, err := ParseMatchList(wrapMatchList(t, match), SteamID64FromAccountID(1187846741))
	if err != nil {
		t.Fatalf("ParseMatchList returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	item := items[0]

	if item.MatchID != "3668142292599833094" {
		t.Errorf("MatchID = %q", item.MatchID)
	}
	if item.ReservationID != "3668148000611369297" {
		t.Errorf("ReservationID = %q", item.ReservationID)
	}
	if item.ShareCode != "CSGO-3MD4K-ShNFS-p3t6a-dxQJH-f9hWA" {
		t.Errorf("ShareCode = %q", item.ShareCode)
	}
	if item.DemoURL() != "http://replay273.valve.net/730/003668148000611369297_1331187857.dem.bz2" {
		t.Errorf("DemoURL = %q", item.DemoURL())
	}
	if !item.DemoAvailable {
		t.Error("DemoAvailable = false, want true")
	}
	// Valve's own file name: match730_<reservation id:21>_<tv port:10>_<server ip>.
	if item.DemoName() != "match730_003668148000611369297_1331187857_273" {
		t.Errorf("DemoName = %q", item.DemoName())
	}
	// game_type 4104 = 0x1008 => map bit 1<<4 (de_inferno), mode 8 (competitive).
	if item.MapName != "de_inferno" {
		t.Errorf("MapName = %q, want de_inferno", item.MapName)
	}
	if item.GameMode != "竞技模式" {
		t.Errorf("GameMode = %q", item.GameMode)
	}
	if item.DurationSeconds != 2601 {
		t.Errorf("DurationSeconds = %d, want 2601", item.DurationSeconds)
	}
	if item.PlayerCount != 10 {
		t.Errorf("PlayerCount = %d, want 10", item.PlayerCount)
	}
	if item.EndTime == "" {
		t.Error("EndTime is empty")
	}

	// The local account is slot 0 (started as CT) and did not switch sides, so
	// the CT score is "our" score.
	if item.Score1 != 13 || item.Score2 != 10 {
		t.Errorf("score = %d:%d, want 13:10", item.Score1, item.Score2)
	}
	if item.Kill != 25 || item.Death != 10 || item.Assist != 5 {
		t.Errorf("kda = %d/%d/%d, want 25/10/5", item.Kill, item.Death, item.Assist)
	}
	if item.Result() != ResultWin {
		t.Errorf("Result = %q, want %q", item.Result(), ResultWin)
	}
}

func TestParseMatchListWithoutLocalPlayerLeavesPersonalFieldsEmpty(t *testing.T) {
	match := &CDataGCCStrike15V2_MatchInfo{
		Matchid:   proto.Uint64(3189266349767524357),
		Matchtime: proto.Uint32(1485117874),
		Watchablematchinfo: &WatchableMatchInfo{
			ServerIp: proto.Uint32(900),
			TvPort:   proto.Uint32(1796106830),
		},
		Roundstatsall: []*CMsgGCCStrike15V2_MatchmakingServerRoundStats{
			roundStats(3189280594026561777, "http://replay900.valve.net/730/003189280594026561777_1796106830.dem.bz2", accountIDs, 32776),
		},
	}

	items, err := ParseMatchList(wrapMatchList(t, match), 0)
	if err != nil {
		t.Fatalf("ParseMatchList returned error: %v", err)
	}
	item := items[0]
	if item.Kill != 0 || item.Death != 0 || item.Assist != 0 || item.Score1 != 0 || item.Score2 != 0 {
		t.Errorf("personal fields = %d/%d/%d %d:%d, want all zero", item.Kill, item.Death, item.Assist, item.Score1, item.Score2)
	}
	if item.Result() != ResultUnknown {
		t.Errorf("Result = %q, want empty", item.Result())
	}
	// Non-personal data must still be available.
	if item.MapName != "de_mirage" {
		t.Errorf("MapName = %q, want de_mirage", item.MapName)
	}
	if item.ShareCode != "CSGO-9NCRf-oJYL5-BKKeO-rptnD-My6SA" {
		t.Errorf("ShareCode = %q", item.ShareCode)
	}
}

func TestParseMatchListFallsBackToLegacyRoundStats(t *testing.T) {
	legacy := roundStats(3028590839392632871, "", accountIDs, 520)
	legacy.TeamScores = []int32{11, 16}
	legacy.MatchResult = proto.Int32(2)
	legacy.MatchDuration = proto.Int32(2375)

	match := &CDataGCCStrike15V2_MatchInfo{
		Matchid:            proto.Uint64(3028583439163981825),
		Matchtime:          proto.Uint32(1410294063),
		Watchablematchinfo: &WatchableMatchInfo{ServerIp: proto.Uint32(136), TvPort: proto.Uint32(1662707805)},
		RoundstatsLegacy:   legacy,
	}

	items, err := ParseMatchList(wrapMatchList(t, match), 0)
	if err != nil {
		t.Fatalf("ParseMatchList returned error: %v", err)
	}
	item := items[0]
	if item.ReservationID != "3028590839392632871" {
		t.Errorf("ReservationID = %q", item.ReservationID)
	}
	if item.DurationSeconds != 2375 {
		t.Errorf("DurationSeconds = %d, want 2375", item.DurationSeconds)
	}
	// 520 = 0x208 => map bit 1<<1 (de_dust2), mode 8.
	if item.MapName != "de_dust2" {
		t.Errorf("MapName = %q, want de_dust2", item.MapName)
	}
	// This legacy payload has no demo URL: the download link is gone.
	if item.DemoAvailable {
		t.Error("DemoAvailable = true, want false")
	}
	if item.DemoURL() != "" {
		t.Errorf("DemoURL = %q, want empty", item.DemoURL())
	}
}

func TestParseMatchListSkipsUnusableEntries(t *testing.T) {
	raw := wrapMatchList(t,
		&CDataGCCStrike15V2_MatchInfo{},                          // no match ID
		&CDataGCCStrike15V2_MatchInfo{Matchid: proto.Uint64(42)}, // no round stats
		&CDataGCCStrike15V2_MatchInfo{
			Matchid:       proto.Uint64(43),
			Roundstatsall: []*CMsgGCCStrike15V2_MatchmakingServerRoundStats{roundStats(7, "", accountIDs, 4104)},
		},
	)

	items, err := ParseMatchList(raw, 0)
	if err != nil {
		t.Fatalf("ParseMatchList returned error: %v", err)
	}
	if len(items) != 1 || items[0].MatchID != "43" {
		t.Fatalf("items = %+v, want only match 43", items)
	}
}

func TestParseMatchListRejectsGarbage(t *testing.T) {
	items, err := ParseMatchList([]byte{0xFF, 0xFF, 0xFF, 0xFF}, 0)
	if err == nil {
		t.Fatal("ParseMatchList accepted an undecodable payload")
	}
	if len(items) != 0 {
		t.Fatalf("got %d items, want none", len(items))
	}
}

// TestParseMatchListAgainstRealPayload validates the decoder against a real
// Game Coordinator payload. Point CS2_GC_FIXTURE at a
// CDataGCCStrike15_v2_MatchInfo dump (for example a `.dem.info` file written by
// CS Demo Manager) to run it; real payloads are not committed because they hold
// other players' account IDs.
func TestParseMatchListAgainstRealPayload(t *testing.T) {
	path := os.Getenv("CS2_GC_FIXTURE")
	if path == "" {
		t.Skip("CS2_GC_FIXTURE is not set")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var info CDataGCCStrike15V2_MatchInfo
	if err := proto.Unmarshal(raw, &info); err != nil {
		t.Fatalf("fixture is not a match info message: %v", err)
	}
	items, err := ParseMatchList(wrapMatchList(t, &info), 0)
	if err != nil {
		t.Fatalf("ParseMatchList returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	item := items[0]
	if item.MatchID == "" || item.ShareCode == "" || item.DemoName() == "" {
		t.Fatalf("incomplete item: %+v", item)
	}
	t.Logf("match=%s sharecode=%s demo=%s map=%s mode=%s score=%d:%d duration=%ds end=%s available=%v",
		item.MatchID, item.ShareCode, item.DemoName(), item.MapName, item.GameMode,
		item.Score1, item.Score2, item.DurationSeconds, item.EndTime, item.DemoAvailable)
}

func TestMapNameFromGameType(t *testing.T) {
	// The reserve game_type packs the map bit in the upper 24 bits and the game
	// mode in the low byte.
	gameType := func(mapBit uint32, mode uint32) uint32 { return mapBit<<8 | mode }
	tests := []struct {
		gameType uint32
		want     string
	}{
		{4104, "de_inferno"}, // real CS2 competitive payload: map bit 1<<4, mode 8
		{32776, "de_mirage"}, // real CS:GO competitive payload: map bit 1<<7, mode 8
		{520, "de_dust2"},    // real legacy payload: map bit 1<<1, mode 8
		{gameType(1<<9, 8), "de_poseidon"},
		{gameType(1<<7, 10), "de_debris"}, // wingman on the mirage slot
		{gameType(1<<23, 8), ""},          // unknown map bit
		{gameType(0, 8), ""},              // no map bit at all
	}
	for _, test := range tests {
		if got := mapNameFromGameType(test.gameType); got != test.want {
			t.Errorf("mapNameFromGameType(%d) = %q, want %q", test.gameType, got, test.want)
		}
	}
}
