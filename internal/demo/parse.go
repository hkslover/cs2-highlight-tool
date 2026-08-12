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
	// Kills is the flat, chronological list of every kill in the demo and is the
	// single source of truth for clip selection. ClipPlayers and DeathPlayers are
	// pre-grouped views of these same kills; filtering works off this list because
	// a filter can constrain both sides of a kill at once, which a tree already
	// grouped by one side cannot express.
	Kills       []ClipKill   `json:"kills"`
	ClipPlayers []ClipPlayer `json:"clip_players"`
	// DeathPlayers groups the very same kills by victim instead of by killer,
	// so the clip page can offer "record the moments this player got killed".
	DeathPlayers []ClipPlayer `json:"death_players"`
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

// ClipPlayer groups kill clips by one side of the kill event: by killer for
// Metadata.ClipPlayers, by victim for Metadata.DeathPlayers.
type ClipPlayer struct {
	Name    string `json:"name"`
	SteamID string `json:"steam_id"`
	// TotalKills counts the grouped clips for Metadata.ClipPlayers entries.
	TotalKills int `json:"total_kills"`
	// TotalDeaths counts the grouped clips for Metadata.DeathPlayers entries.
	TotalDeaths int         `json:"total_deaths,omitempty"`
	Rounds      []ClipRound `json:"rounds"`
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

	// WeaponClass buckets WeaponName into a weapon family. See classifyWeapon —
	// snipers and machine guns are split out from the library's own classes.
	WeaponClass string `json:"weapon_class"`
	// PenetratedObjects is the surface count behind IsWallbang, kept so a filter
	// can ask for double-penetration frags specifically.
	PenetratedObjects int `json:"penetrated_objects"`
	// Distance between killer and victim, in metres.
	Distance float64 `json:"distance"`
	// HitGroup is the events.HitGroup of the fatal hit (1 = head, 2 = chest,
	// 3 = stomach, 4/5 = arms, 6/7 = legs, 8 = neck). It is 0 when the fatal
	// damage carried no hit group, which is the normal case for grenades and
	// fire, and when no matching damage event was seen on the kill's tick.
	HitGroup int `json:"hit_group"`

	IsNoScope       bool `json:"is_noscope"`
	IsThroughSmoke  bool `json:"is_through_smoke"`
	IsAttackerBlind bool `json:"is_attacker_blind"`
	IsAssistedFlash bool `json:"is_assisted_flash"`
	// IsTeamKill covers suicides too, since those also land on the same team.
	IsTeamKill      bool   `json:"is_team_kill"`
	HasAssist       bool   `json:"has_assist"`
	AssisterName    string `json:"assister_name,omitempty"`
	AssisterSteamID string `json:"assister_steam_id,omitempty"`

	// Killer and victim state sampled at the tick the kill landed.
	KillerHealth   int  `json:"killer_health"`
	KillerArmor    int  `json:"killer_armor"`
	KillerAirborne bool `json:"killer_airborne"`
	KillerDucking  bool `json:"killer_ducking"`
	KillerScoped   bool `json:"killer_scoped"`
	VictimBlinded  bool `json:"victim_blinded"`

	// Round context. IsOpeningKill and KillerRoundKills are filled in after
	// parsing by annotateRoundContext, because they depend on later kills.
	IsOpeningKill bool `json:"is_opening_kill"`
	// KillerRoundKills is how many frags the killer finished this round with, so
	// a 3K filter can keep all three of its kills. Team kills do not count and
	// carry 0.
	KillerRoundKills int `json:"killer_round_kills"`
	// IsClutchKill marks a kill made with no living teammate left.
	IsClutchKill bool `json:"is_clutch_kill"`
	// BombPlanted is whether the bomb was down when the kill landed.
	BombPlanted bool `json:"bomb_planted"`
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
	bombPlanted := false

	// Kill events carry no hit group, so the fatal PlayerHurt is remembered per
	// victim and read back when the kill lands. The tick is stored alongside it
	// and must match, otherwise an earlier non-fatal hit would be misreported as
	// the killing blow.
	type fatalHit struct {
		tick     int
		hitGroup int
	}
	lastHit := make(map[uint64]fatalHit)

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
		bombPlanted = false
	})

	parser.RegisterEventHandler(func(_ events.BombPlanted) {
		bombPlanted = true
	})

	parser.RegisterEventHandler(func(e events.PlayerHurt) {
		if e.Player == nil || e.Player.SteamID64 == 0 {
			return
		}
		lastHit[e.Player.SteamID64] = fatalHit{
			tick:     parser.GameState().IngameTick(),
			hitGroup: int(e.HitGroup),
		}
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

		// World damage — falling, and the automatic suicide the server settles a
		// player with — reports the victim as their own killer. There is no frag
		// and nothing worth watching, so it never becomes a clip and never earns
		// a kill. The death itself still happened, so the scoreboard keeps it.
		if e.Weapon != nil && e.Weapon.Type == common.EqWorld {
			if s := ensurePlayer(e.Victim); s != nil {
				s.Deaths++
			}
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
		weaponClass := WeaponClassOther
		if e.Weapon != nil {
			weaponName = e.Weapon.String()
			weaponClass = classifyWeapon(e.Weapon.Type)
		}

		hitGroup := 0
		if hit, ok := lastHit[e.Victim.SteamID64]; ok && hit.tick == tick {
			hitGroup = hit.hitGroup
		}

		assisterName := ""
		assisterSteamID := ""
		if e.Assister != nil && e.Assister.SteamID64 != 0 {
			assisterName = e.Assister.Name
			assisterSteamID = strconv.FormatUint(e.Assister.SteamID64, 10)
		}

		participants := parser.GameState().Participants().Playing()
		alive := make([]alivePlayer, 0, len(participants))
		for _, p := range participants {
			if p == nil {
				continue
			}
			alive = append(alive, alivePlayer{
				SteamID: p.SteamID64,
				Team:    p.Team,
				Alive:   p.IsAlive(),
			})
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

			WeaponClass:       weaponClass,
			PenetratedObjects: e.PenetratedObjects,
			Distance:          float64(e.Distance),
			HitGroup:          hitGroup,

			IsNoScope:       e.NoScope,
			IsThroughSmoke:  e.ThroughSmoke,
			IsAttackerBlind: e.AttackerBlind,
			IsAssistedFlash: e.AssistedFlash,
			IsTeamKill:      e.Killer.Team == e.Victim.Team,
			HasAssist:       assisterSteamID != "",
			AssisterName:    assisterName,
			AssisterSteamID: assisterSteamID,

			KillerHealth:   e.Killer.Health(),
			KillerArmor:    e.Killer.Armor(),
			KillerAirborne: e.Killer.IsAirborne(),
			KillerDucking:  e.Killer.IsDucking(),
			KillerScoped:   e.Killer.IsScoped(),
			VictimBlinded:  e.Victim.IsBlinded(),

			IsClutchKill: isClutchKill(alive, e.Killer.SteamID64, e.Killer.Team, e.Victim.Team),
			BombPlanted:  bombPlanted,
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

	// Round context depends on kills that come later in the same round, so it is
	// filled in here rather than inside the event handler. It must run before the
	// grouped views are built, since those copy the kill structs.
	annotateRoundContext(allKills)
	sortKillsChronologically(allKills)

	meta.Kills = allKills
	meta.ClipPlayers = buildClipPlayers(allKills)
	meta.DeathPlayers = buildDeathPlayers(allKills)
	return meta, nil
}

// buildClipPlayers groups kills by the player who made them.
func buildClipPlayers(kills []ClipKill) []ClipPlayer {
	return groupClipKills(kills, clipSideKiller)
}

// buildDeathPlayers groups the same kills by the player who died in them.
func buildDeathPlayers(kills []ClipKill) []ClipPlayer {
	return groupClipKills(kills, clipSideVictim)
}

type clipSide int

const (
	clipSideKiller clipSide = iota
	clipSideVictim
)

func groupClipKills(kills []ClipKill, side clipSide) []ClipPlayer {
	if len(kills) == 0 {
		return nil
	}
	players := make(map[string]*clipPlayerBuilder)
	for _, kill := range kills {
		steamID, name := kill.KillerSteamID, kill.KillerName
		if side == clipSideVictim {
			steamID, name = kill.VictimSteamID, kill.VictimName
		}
		if steamID == "" {
			continue
		}
		player := players[steamID]
		if player == nil {
			player = &clipPlayerBuilder{
				name:   name,
				rounds: make(map[int][]ClipKill),
			}
			players[steamID] = player
		}
		if name != "" {
			player.name = name
		}
		player.rounds[kill.Round] = append(player.rounds[kill.Round], kill)
	}

	result := make([]ClipPlayer, 0, len(players))
	for steamID, builder := range players {
		roundIDs := make([]int, 0, len(builder.rounds))
		totalClips := 0
		for roundID := range builder.rounds {
			roundIDs = append(roundIDs, roundID)
			totalClips += len(builder.rounds[roundID])
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

		entry := ClipPlayer{
			Name:    builder.name,
			SteamID: steamID,
			Rounds:  rounds,
		}
		if side == clipSideVictim {
			entry.TotalDeaths = totalClips
		} else {
			entry.TotalKills = totalClips
		}
		result = append(result, entry)
	}

	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].TotalKills, result[j].TotalKills
		if side == clipSideVictim {
			left, right = result[i].TotalDeaths, result[j].TotalDeaths
		}
		if left == right {
			return result[i].Name < result[j].Name
		}
		return left > right
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
