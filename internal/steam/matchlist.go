package steam

import (
	"fmt"
	"strings"
	"time"
)

const (
	// demoNamePrefix is the prefix Valve itself uses for CS2 match demo files.
	demoNamePrefix = "match730"
	// steamID64Base is 76561197960265728, the offset between a Steam3 account ID
	// and a SteamID64 for individual accounts.
	steamID64Base uint64 = 0x0110000100000000

	// teamSize is the number of players per team; account ID slots 0-4 started
	// as CT, slots 5-9 started as T.
	teamSize = 5
)

// MatchItem is one official matchmaking match as shown in the import list.
type MatchItem struct {
	MatchID       string `json:"match_id"`
	ReservationID string `json:"reservation_id"`
	ShareCode     string `json:"share_code"`
	MapName       string `json:"map_name"`
	GameMode      string `json:"game_mode"`
	// Score1/Score2 are the local player's team score and the opponent score.
	// They stay 0 when the local Steam account is unknown.
	Score1 int `json:"score1"`
	Score2 int `json:"score2"`
	// Kill/Death/Assist are the local player's totals, 0 when unknown.
	Kill   int `json:"kill"`
	Death  int `json:"death"`
	Assist int `json:"assist"`
	// Rating is not provided by Valve for official matches and stays 0.
	Rating          float64 `json:"rating"`
	EndTime         string  `json:"end_time"`
	DurationSeconds int     `json:"duration_seconds"`
	DemoAvailable   bool    `json:"demo_available"`
	// PlayerCount is the number of account slots carrying a real account ID.
	PlayerCount int `json:"player_count"`

	// demoURL and demoName stay internal: the frontend only needs to know
	// whether a download is possible.
	demoURL  string `json:"-"`
	demoName string `json:"-"`
}

// DemoURL returns the replay download URL carried by the match message. Valve
// only serves it for a limited time, so an empty value means the demo can no
// longer be downloaded for this match.
func (item MatchItem) DemoURL() string { return item.demoURL }

// DemoName returns Valve's own file name for the demo, which other tools (and
// the game client) use to identify a downloaded match replay.
func (item MatchItem) DemoName() string { return item.demoName }

// ParseStats reports what the Game Coordinator returned, which makes "the
// account has no recent official match" distinguishable from "entries arrived
// but could not be used".
type ParseStats struct {
	// RawMatches is the number of match entries in the response.
	RawMatches int
	// Skipped is the number of entries dropped as unusable.
	Skipped int
	// Downloadable is the number of usable entries that still carry a demo URL.
	Downloadable int
}

// ParseMatchList decodes a CMsgGCCStrike15_v2_MatchList payload as written by
// the Game Coordinator. localSteamID is the signed-in SteamID64 and is only
// used to derive the "my team" figures; pass 0 when it is unknown.
//
// Every match whose payload is unusable (no identity or no round stats) is
// skipped instead of failing the whole list.
func ParseMatchList(raw []byte, localSteamID uint64) ([]MatchItem, error) {
	items, _, err := ParseMatchListWithStats(raw, localSteamID)
	return items, err
}

// ParseMatchListWithStats is ParseMatchList plus the counts used for diagnostics.
func ParseMatchListWithStats(raw []byte, localSteamID uint64) ([]MatchItem, ParseStats, error) {
	message := &CMsgGCCStrike15V2_MatchList{}
	if err := unmarshalMatchList(raw, message); err != nil {
		return nil, ParseStats{}, fmt.Errorf("解析比赛列表失败: %w", err)
	}

	stats := ParseStats{RawMatches: len(message.GetMatches())}
	items := make([]MatchItem, 0, len(message.GetMatches()))
	for _, match := range message.GetMatches() {
		item, ok := buildMatchItem(match, localSteamID)
		if !ok {
			stats.Skipped++
			continue
		}
		if item.DemoAvailable {
			stats.Downloadable++
		}
		items = append(items, item)
	}
	return items, stats, nil
}

func buildMatchItem(match *CDataGCCStrike15V2_MatchInfo, localSteamID uint64) (MatchItem, bool) {
	if match == nil || match.GetMatchid() == 0 {
		return MatchItem{}, false
	}
	lastRound := lastRoundStats(match)
	if lastRound == nil {
		return MatchItem{}, false
	}

	watchable := match.GetWatchablematchinfo()
	tvPort := uint32(0)
	serverIP := uint32(0)
	if watchable != nil {
		tvPort = watchable.GetTvPort()
		serverIP = watchable.GetServerIp()
	}

	reservationID := lastRound.GetReservationid()
	shareCode := ShareCode{
		MatchID:       match.GetMatchid(),
		ReservationID: reservationID,
		TVPort:        uint16(tvPort),
	}

	item := MatchItem{
		MatchID:         fmt.Sprintf("%d", match.GetMatchid()),
		ReservationID:   fmt.Sprintf("%d", reservationID),
		ShareCode:       shareCode.String(),
		MapName:         mapNameFromGameType(lastRound.GetReservation().GetGameType()),
		GameMode:        gameModeLabel(lastRound.GetReservation().GetGameType()),
		EndTime:         formatUnixTime(match.GetMatchtime()),
		DurationSeconds: int(lastRound.GetMatchDuration()),
		DemoAvailable:   strings.TrimSpace(lastRound.GetMap()) != "",
		demoURL:         strings.TrimSpace(lastRound.GetMap()),
		demoName:        fmt.Sprintf("%s_%021d_%010d_%d", demoNamePrefix, reservationID, tvPort, serverIP),
	}
	item.PlayerCount = countAccountSlots(lastRound.GetReservation().GetAccountIds())

	applyLocalPlayer(&item, lastRound, localSteamID)
	return item, true
}

// lastRoundStats returns the round statistics carrying the final match state.
// CS2 sends the full list in roundstatsall, pre-CS2 payloads use the single
// roundstats_legacy field.
func lastRoundStats(match *CDataGCCStrike15V2_MatchInfo) *CMsgGCCStrike15V2_MatchmakingServerRoundStats {
	if rounds := match.GetRoundstatsall(); len(rounds) > 0 {
		return rounds[len(rounds)-1]
	}
	return match.GetRoundstatsLegacy()
}

func applyLocalPlayer(item *MatchItem, lastRound *CMsgGCCStrike15V2_MatchmakingServerRoundStats, localSteamID uint64) {
	if localSteamID == 0 {
		return
	}
	accountID := uint32(localSteamID & 0xFFFFFFFF)
	accountIDs := lastRound.GetReservation().GetAccountIds()
	slot := indexOfAccountID(accountIDs, accountID)
	if slot < 0 {
		return
	}

	item.Kill = valueAt(lastRound.GetKills(), slot)
	item.Death = valueAt(lastRound.GetDeaths(), slot)
	item.Assist = valueAt(lastRound.GetAssists(), slot)

	scoreCT, scoreT := finalScore(lastRound)
	startedAsCT := slot < teamSize
	if lastRound.GetBSwitchedTeams() {
		startedAsCT = !startedAsCT
	}
	if startedAsCT {
		item.Score1, item.Score2 = scoreCT, scoreT
	} else {
		item.Score1, item.Score2 = scoreT, scoreCT
	}
}

// finalScore returns the score of the team that started as CT and the score of
// the team that started as T, undoing the half-time swap when it happened.
func finalScore(lastRound *CMsgGCCStrike15V2_MatchmakingServerRoundStats) (int, int) {
	scores := lastRound.GetTeamScores()
	scoreCT, scoreT := valueAt(scores, 0), valueAt(scores, 1)
	if lastRound.GetBSwitchedTeams() {
		scoreCT, scoreT = scoreT, scoreCT
	}
	return scoreCT, scoreT
}

// MatchResultUnknown/... are the values derived from the Game Coordinator's
// match_result field.
const (
	ResultUnknown = ""
	ResultWin     = "win"
	ResultLoss    = "loss"
	ResultTie     = "tie"
)

// Result reports the local player's outcome. It needs the local Steam account
// to be part of the match, otherwise it stays empty.
func (item MatchItem) Result() string {
	if item.Score1 == 0 && item.Score2 == 0 {
		return ResultUnknown
	}
	switch {
	case item.Score1 > item.Score2:
		return ResultWin
	case item.Score1 < item.Score2:
		return ResultLoss
	default:
		return ResultTie
	}
}

func indexOfAccountID(accountIDs []uint32, accountID uint32) int {
	for index, value := range accountIDs {
		if value == accountID {
			return index
		}
	}
	return -1
}

func countAccountSlots(accountIDs []uint32) int {
	count := 0
	for _, accountID := range accountIDs {
		if accountID != 0 {
			count++
		}
	}
	return count
}

func valueAt(values []int32, index int) int {
	if index < 0 || index >= len(values) {
		return 0
	}
	return int(values[index])
}

func formatUnixTime(seconds uint32) string {
	if seconds == 0 {
		return ""
	}
	return time.Unix(int64(seconds), 0).Format("2006-01-02 15:04:05")
}

// SteamID64FromAccountID converts a Steam3 account ID into a SteamID64.
func SteamID64FromAccountID(accountID uint32) uint64 {
	return steamID64Base + uint64(accountID)
}

// AccountIDFromSteamID64 returns the Steam3 account ID embedded in a SteamID64.
func AccountIDFromSteamID64(steamID uint64) uint32 {
	return uint32(steamID & 0xFFFFFFFF)
}

// gameModeLabel maps the low byte of the reserve game_type to a player facing
// label. 8 is competitive and 10 wingman; everything else is reported as an
// official match.
func gameModeLabel(gameType uint32) string {
	switch gameType & 0xFF {
	case 8:
		return "竞技模式"
	case 10:
		return "搭档模式"
	default:
		return "官方匹配"
	}
}

// mapNameFromGameType decodes the map encoded in the upper 24 bits of the
// reserve game_type. Map bits are hard-coded in the game and may change with a
// game update, so an unknown bit falls back to an empty name and the map is
// taken from the demo once it is imported.
func mapNameFromGameType(gameType uint32) string {
	bit := (gameType >> 8) & 0xFFFFFF
	mode := gameType & 0xFF

	switch bit {
	case 1 << 0:
		return "de_warden"
	case 1 << 1:
		return "de_dust2"
	case 1 << 2:
		return "de_train"
	case 1 << 3:
		return "de_ancient"
	case 1 << 4:
		return "de_inferno"
	case 1 << 5:
		return "de_nuke"
	case 1 << 6:
		return "de_vertigo"
	case 1 << 7:
		if mode == 10 {
			return "de_debris"
		}
		return "de_mirage"
	case 1 << 8:
		return "cs_office"
	case 1 << 9:
		return "de_poseidon"
	case 1 << 10:
		return "de_eldorado"
	case 1 << 11:
		return "de_sanctum"
	case 1 << 12:
		return "de_cache"
	case 1 << 13:
		return "de_stronghold"
	case 1 << 14:
		return "de_boulder"
	case 1 << 15:
		return "de_anubis"
	case 1 << 16:
		return "de_tuscan"
	case 1 << 18:
		return "de_fachwerk"
	case 1 << 19:
		return "cs_shelter"
	case 1 << 20:
		return "de_overpass"
	case 1 << 21:
		return "de_cobblestone"
	case 1 << 22:
		return "de_canals"
	default:
		return ""
	}
}
