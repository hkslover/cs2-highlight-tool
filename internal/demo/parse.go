package demo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	dem "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	common "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	events "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
	msg "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/msg"
)

// Metadata holds summary information parsed from a .dem file.
type Metadata struct {
	FilePath      string       `json:"file_path"`
	FileName      string       `json:"file_name"`
	MapName       string       `json:"map_name"`
	ServerName    string       `json:"server_name"`
	Duration      float64      `json:"duration"`
	TickRate      float64      `json:"tick_rate"`
	TotalRounds   int          `json:"total_rounds"`
	OvertimeCount int          `json:"overtime_count"`
	ScoreCT       int          `json:"score_ct"`
	ScoreT        int          `json:"score_t"`
	ClanNameCT    string       `json:"clan_name_ct"`
	ClanNameT     string       `json:"clan_name_t"`
	Players       []PlayerInfo `json:"players"`
	ClipPlayers   []ClipPlayer `json:"clip_players"`
	// MatchEndTick is the tick at which the CS2 post-match win panel (final
	// scoreboard) is displayed, if the demo reaches that point. Recordings
	// must not extend past this tick or they will capture the settlement
	// screen instead of gameplay. Zero means the demo never reached it.
	MatchEndTick int `json:"match_end_tick,omitempty"`
}

// PlayerInfo holds per-player stats extracted from the demo.
type PlayerInfo struct {
	Name        string `json:"name"`
	SteamID     uint64 `json:"steam_id"`
	SteamIDText string `json:"steam_id_text,omitempty"`
	Kills       int    `json:"kills"`
	Deaths      int    `json:"deaths"`
	Assists     int    `json:"assists"`
}

// ClipPlayer groups kill clips by killer.
type ClipPlayer struct {
	Name       string      `json:"name"`
	SteamID    string      `json:"steam_id"`
	TotalKills int         `json:"total_kills"`
	Rounds     []ClipRound `json:"rounds"`
}

// ClipRound groups kills inside one round.
type ClipRound struct {
	Round int        `json:"round"`
	Kills []ClipKill `json:"kills"`
}

// ClipKill is a single selectable kill clip.
type ClipKill struct {
	ID             string `json:"id"`
	Round          int    `json:"round"`
	Tick           int    `json:"tick"`
	MapName        string `json:"map_name"`
	KillerName     string `json:"killer_name"`
	KillerSteamID  string `json:"killer_steam_id"`
	KillerSlot     int    `json:"killer_slot"`
	KillerEntityID int    `json:"killer_entity_id"`
	KillerSide     string `json:"killer_side"`
	VictimName     string `json:"victim_name"`
	VictimSteamID  string `json:"victim_steam_id"`
	VictimSlot     int    `json:"victim_slot"`
	VictimEntityID int    `json:"victim_entity_id"`
	VictimSide     string `json:"victim_side"`
	WeaponName     string `json:"weapon_name"`
	IsHeadshot     bool   `json:"is_headshot"`
	IsWallbang     bool   `json:"is_wallbang"`
}

type clipPlayerBuilder struct {
	name   string
	rounds map[int][]ClipKill
}

// ParseMetadata parses a .dem file and returns its metadata.
func ParseMetadata(demoPath string) (*Metadata, error) {
	f, err := os.Open(demoPath)
	if err != nil {
		return nil, fmt.Errorf("打开 demo 失败: %w", err)
	}
	defer f.Close()

	parser := dem.NewParser(f)
	defer parser.Close()

	mapName := ""
	serverName := ""
	currentRound := 0
	killSeq := 0
	matchEndTick := 0

	type playerStats struct {
		Name    string
		SteamID uint64
		Kills   int
		Deaths  int
		Assists int
	}
	stats := make(map[uint64]*playerStats)
	allKills := make([]ClipKill, 0, 128)

	ensurePlayer := func(p *common.Player) *playerStats {
		if p == nil || p.SteamID64 == 0 {
			return nil
		}
		s, ok := stats[p.SteamID64]
		if !ok {
			s = &playerStats{
				Name:    p.Name,
				SteamID: p.SteamID64,
			}
			stats[p.SteamID64] = s
		}
		s.Name = p.Name
		return s
	}

	parser.RegisterNetMessageHandler(func(m *msg.CDemoFileHeader) {
		if m == nil {
			return
		}
		if n := m.GetMapName(); n != "" {
			mapName = normalizeMapName(n)
		}
		if n := m.GetServerName(); n != "" {
			serverName = n
		}
	})

	parser.RegisterNetMessageHandler(func(m *msg.CSVCMsg_ServerInfo) {
		if m == nil {
			return
		}
		if n := normalizeMapName(m.GetMapName()); n != "" {
			mapName = n
		}
	})

	parser.RegisterNetMessageHandler(func(m *msg.CNETMsg_SignonState) {
		if m == nil {
			return
		}
		if n := normalizeMapName(m.GetMapName()); n != "" {
			mapName = n
		}
	})

	parser.RegisterEventHandler(func(_ events.RoundStart) {
		currentRound = parser.GameState().TotalRoundsPlayed() + 1
	})

	parser.RegisterEventHandler(func(_ events.AnnouncementWinPanelMatch) {
		if matchEndTick != 0 {
			return
		}
		if tick := parser.GameState().IngameTick(); tick > 0 {
			matchEndTick = tick
		}
	})

	parser.RegisterEventHandler(func(e events.Kill) {
		if parser.GameState().IsWarmupPeriod() {
			return
		}
		if e.Killer == nil || e.Victim == nil {
			return
		}
		if e.Killer.SteamID64 == 0 || e.Victim.SteamID64 == 0 {
			return
		}

		if s := ensurePlayer(e.Killer); s != nil {
			s.Kills++
		}
		if s := ensurePlayer(e.Victim); s != nil {
			s.Deaths++
		}
		if s := ensurePlayer(e.Assister); s != nil {
			s.Assists++
		}

		round := currentRound
		if round <= 0 {
			round = parser.GameState().TotalRoundsPlayed() + 1
		}
		if round <= 0 {
			return
		}

		tick := parser.GameState().IngameTick()
		if tick < 0 {
			return
		}

		killSeq++
		weaponName := "unknown"
		if e.Weapon != nil {
			weaponName = e.Weapon.String()
		}
		killerSlot := 0
		if e.Killer.UserID > 0 {
			killerSlot = e.Killer.UserID + 1
		}
		victimSlot := 0
		if e.Victim.UserID > 0 {
			victimSlot = e.Victim.UserID + 1
		}
		killMap := mapName
		allKills = append(allKills, ClipKill{
			ID:             buildKillID(round, tick, e.Killer.SteamID64, e.Victim.SteamID64, killSeq),
			Round:          round,
			Tick:           tick,
			MapName:        killMap,
			KillerName:     e.Killer.Name,
			KillerSteamID:  strconv.FormatUint(e.Killer.SteamID64, 10),
			KillerSlot:     killerSlot,
			KillerEntityID: e.Killer.EntityID,
			KillerSide:     teamToSide(e.Killer.Team),
			VictimName:     e.Victim.Name,
			VictimSteamID:  strconv.FormatUint(e.Victim.SteamID64, 10),
			VictimSlot:     victimSlot,
			VictimEntityID: e.Victim.EntityID,
			VictimSide:     teamToSide(e.Victim.Team),
			WeaponName:     weaponName,
			IsHeadshot:     e.IsHeadshot,
			IsWallbang:     e.PenetratedObjects > 0,
		})
	})

	if err := parser.ParseToEnd(); err != nil {
		return nil, fmt.Errorf("解析 demo 失败: %w", err)
	}

	gs := parser.GameState()
	meta := &Metadata{
		FilePath:      demoPath,
		FileName:      filepath.Base(demoPath),
		MapName:       mapName,
		ServerName:    serverName,
		Duration:      parser.CurrentTime().Seconds(),
		TickRate:      parser.TickRate(),
		TotalRounds:   gs.TotalRoundsPlayed(),
		OvertimeCount: gs.OvertimeCount(),
		MatchEndTick:  matchEndTick,
	}

	if ct := gs.TeamCounterTerrorists(); ct != nil {
		meta.ScoreCT = ct.Score()
		meta.ClanNameCT = ct.ClanName()
	}
	if t := gs.TeamTerrorists(); t != nil {
		meta.ScoreT = t.Score()
		meta.ClanNameT = t.ClanName()
	}

	playerList := make([]PlayerInfo, 0, len(stats))
	for _, s := range stats {
		playerList = append(playerList, PlayerInfo{
			Name:        s.Name,
			SteamID:     s.SteamID,
			SteamIDText: strconv.FormatUint(s.SteamID, 10),
			Kills:       s.Kills,
			Deaths:      s.Deaths,
			Assists:     s.Assists,
		})
	}
	sort.Slice(playerList, func(i, j int) bool {
		if playerList[i].Kills == playerList[j].Kills {
			return playerList[i].Name < playerList[j].Name
		}
		return playerList[i].Kills > playerList[j].Kills
	})
	meta.Players = playerList
	meta.ClipPlayers = buildClipPlayers(allKills)
	return meta, nil
}

func buildClipPlayers(kills []ClipKill) []ClipPlayer {
	if len(kills) == 0 {
		return nil
	}
	players := make(map[string]*clipPlayerBuilder)
	for _, kill := range kills {
		if kill.KillerSteamID == "" {
			continue
		}
		player := players[kill.KillerSteamID]
		if player == nil {
			player = &clipPlayerBuilder{
				name:   kill.KillerName,
				rounds: make(map[int][]ClipKill),
			}
			players[kill.KillerSteamID] = player
		}
		if kill.KillerName != "" {
			player.name = kill.KillerName
		}
		player.rounds[kill.Round] = append(player.rounds[kill.Round], kill)
	}

	result := make([]ClipPlayer, 0, len(players))
	for steamID, builder := range players {
		roundIDs := make([]int, 0, len(builder.rounds))
		totalKills := 0
		for roundID := range builder.rounds {
			roundIDs = append(roundIDs, roundID)
			totalKills += len(builder.rounds[roundID])
		}
		sort.Ints(roundIDs)

		rounds := make([]ClipRound, 0, len(roundIDs))
		for _, roundID := range roundIDs {
			roundKills := append([]ClipKill(nil), builder.rounds[roundID]...)
			sort.Slice(roundKills, func(i, j int) bool {
				if roundKills[i].Tick == roundKills[j].Tick {
					return roundKills[i].ID < roundKills[j].ID
				}
				return roundKills[i].Tick < roundKills[j].Tick
			})
			rounds = append(rounds, ClipRound{
				Round: roundID,
				Kills: roundKills,
			})
		}

		result = append(result, ClipPlayer{
			Name:       builder.name,
			SteamID:    steamID,
			TotalKills: totalKills,
			Rounds:     rounds,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].TotalKills == result[j].TotalKills {
			return result[i].Name < result[j].Name
		}
		return result[i].TotalKills > result[j].TotalKills
	})
	return result
}

func buildKillID(round int, tick int, killerSteamID uint64, victimSteamID uint64, seq int) string {
	return fmt.Sprintf("r%d-t%d-k%d-v%d-s%d", round, tick, killerSteamID, victimSteamID, seq)
}

func normalizeMapName(name string) string {
	value := strings.TrimSpace(strings.ToLower(name))
	value = strings.TrimPrefix(value, "maps/")
	value = strings.TrimSuffix(value, ".vpk")
	return strings.TrimSpace(value)
}

func teamToSide(team common.Team) string {
	switch team {
	case common.TeamCounterTerrorists:
		return "ct"
	case common.TeamTerrorists:
		return "t"
	default:
		return ""
	}
}
