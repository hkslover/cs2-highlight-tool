package demo

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	dem "github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs"
	common "github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/common"
	events "github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/events"
)

const (
	FullRoundPOVEndTargetDeath = "target_death"
	FullRoundPOVEndRoundEnd    = "round_end"
)

type FullRoundPOVPlan struct {
	PlayerName    string                `json:"player_name"`
	PlayerSteamID string                `json:"player_steam_id"`
	Segments      []FullRoundPOVSegment `json:"segments"`
}

type FullRoundPOVSegment struct {
	Round              int    `json:"round"`
	StartTick          int    `json:"start_tick"`
	FreezeEndTick      int    `json:"freeze_end_tick"`
	RoundEndTick       int    `json:"round_end_tick"`
	OfficialEndTick    int    `json:"official_end_tick,omitempty"`
	NextRoundStartTick int    `json:"next_round_start_tick,omitempty"`
	DeathTick          int    `json:"death_tick,omitempty"`
	RecordStartTick    int    `json:"record_start_tick"`
	RecordEndTick      int    `json:"record_end_tick"`
	TargetSlot         int    `json:"target_slot"`
	EndReason          string `json:"end_reason"`
}

type FullRoundPOVRound struct {
	Round           int
	StartTick       int
	FreezeEndTick   int
	EndTick         int
	OfficialEndTick int
	Slots           map[uint64]int
	Deaths          []FullRoundPOVDeath
}

type FullRoundPOVDeath struct {
	Tick          int
	VictimSteamID uint64
	KillerSteamID uint64
}

func ParseFullRoundPOVPlan(demoPath string, targetSteamID uint64) (*FullRoundPOVPlan, error) {
	if strings.TrimSpace(demoPath) == "" {
		return nil, fmt.Errorf("demo 路径为空")
	}
	if targetSteamID == 0 {
		return nil, fmt.Errorf("跟踪玩家 SteamID 为空")
	}

	f, err := os.Open(demoPath)
	if err != nil {
		return nil, fmt.Errorf("打开 demo 失败: %w", err)
	}
	defer f.Close()

	parser := dem.NewParser(f)
	defer parser.Close()

	players := make(map[uint64]PlayerInfo)
	roundsByNumber := make(map[int]*FullRoundPOVRound)
	roundOrder := make([]int, 0, 32)
	currentRound := 0

	ensurePlayer := func(p *common.Player) {
		if p == nil || p.SteamID64 == 0 {
			return
		}
		info := players[p.SteamID64]
		info.SteamID = p.SteamID64
		if p.Name != "" {
			info.Name = p.Name
		}
		players[p.SteamID64] = info
	}

	roundForEvent := func() int {
		if currentRound > 0 {
			return currentRound
		}
		return parser.GameState().TotalRoundsPlayed() + 1
	}

	ensureRound := func(number int) *FullRoundPOVRound {
		if number <= 0 {
			return nil
		}
		round := roundsByNumber[number]
		if round == nil {
			round = &FullRoundPOVRound{Round: number, Slots: make(map[uint64]int)}
			roundsByNumber[number] = round
			roundOrder = append(roundOrder, number)
		}
		return round
	}

	snapshotPlayingSlots := func(round *FullRoundPOVRound) {
		if round == nil {
			return
		}
		for _, player := range parser.GameState().Participants().Playing() {
			ensurePlayer(player)
			if player == nil || player.SteamID64 == 0 || player.UserID <= 0 {
				continue
			}
			round.Slots[player.SteamID64] = player.UserID + 1
		}
	}

	parser.RegisterEventHandler(func(_ events.RoundStart) {
		if parser.GameState().IsWarmupPeriod() {
			return
		}
		currentRound = parser.GameState().TotalRoundsPlayed() + 1
		round := ensureRound(currentRound)
		if round == nil {
			return
		}
		round.StartTick = nonNegativeTick(parser.GameState().IngameTick())
		snapshotPlayingSlots(round)
	})

	parser.RegisterEventHandler(func(_ events.RoundFreezetimeEnd) {
		if parser.GameState().IsWarmupPeriod() {
			return
		}
		round := ensureRound(roundForEvent())
		if round == nil {
			return
		}
		round.FreezeEndTick = nonNegativeTick(parser.GameState().IngameTick())
		snapshotPlayingSlots(round)
	})

	parser.RegisterEventHandler(func(_ events.RoundEnd) {
		if parser.GameState().IsWarmupPeriod() {
			return
		}
		round := ensureRound(roundForEvent())
		if round == nil {
			return
		}
		round.EndTick = nonNegativeTick(parser.GameState().IngameTick())
	})

	parser.RegisterEventHandler(func(_ events.RoundEndOfficial) {
		if parser.GameState().IsWarmupPeriod() {
			return
		}
		round := ensureRound(roundForEvent())
		if round == nil {
			return
		}
		round.OfficialEndTick = nonNegativeTick(parser.GameState().IngameTick())
	})

	parser.RegisterEventHandler(func(e events.Kill) {
		if parser.GameState().IsWarmupPeriod() {
			return
		}
		ensurePlayer(e.Killer)
		ensurePlayer(e.Victim)
		if e.Victim == nil || e.Victim.SteamID64 == 0 {
			return
		}
		round := ensureRound(roundForEvent())
		if round == nil {
			return
		}
		var killerSteamID uint64
		if e.Killer != nil {
			killerSteamID = e.Killer.SteamID64
		}
		round.Deaths = append(round.Deaths, FullRoundPOVDeath{
			Tick:          nonNegativeTick(parser.GameState().IngameTick()),
			VictimSteamID: e.Victim.SteamID64,
			KillerSteamID: killerSteamID,
		})
	})

	if err := parser.ParseToEnd(context.Background()); err != nil {
		return nil, fmt.Errorf("解析 demo 失败: %w", err)
	}

	rounds := make([]FullRoundPOVRound, 0, len(roundOrder))
	for _, roundNumber := range roundOrder {
		if round := roundsByNumber[roundNumber]; round != nil {
			rounds = append(rounds, *round)
		}
	}
	player := players[targetSteamID]
	player.SteamID = targetSteamID
	return BuildFullRoundPOVPlan(player, rounds), nil
}

func BuildFullRoundPOVPlan(player PlayerInfo, rounds []FullRoundPOVRound) *FullRoundPOVPlan {
	steamID := player.SteamID
	plan := &FullRoundPOVPlan{
		PlayerName:    strings.TrimSpace(player.Name),
		PlayerSteamID: strconv.FormatUint(steamID, 10),
		Segments:      make([]FullRoundPOVSegment, 0, len(rounds)),
	}
	if steamID == 0 || len(rounds) == 0 {
		return plan
	}

	ordered := append([]FullRoundPOVRound(nil), rounds...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Round < ordered[j].Round
	})

	for idx, round := range ordered {
		recordStart := round.StartTick
		if recordStart <= 0 {
			recordStart = round.FreezeEndTick
		}
		recordEnd := round.EndTick
		if recordEnd <= 0 {
			recordEnd = round.OfficialEndTick
		}
		if recordStart <= 0 || recordEnd <= 0 || recordEnd < recordStart {
			continue
		}
		liveStart := round.FreezeEndTick
		if liveStart <= 0 {
			liveStart = recordStart
		}

		targetKillCount := 0
		for _, d := range round.Deaths {
			if d.KillerSteamID != steamID {
				continue
			}
			if d.Tick < liveStart {
				continue
			}
			if round.EndTick > 0 && d.Tick > round.EndTick {
				continue
			}
			targetKillCount++
		}
		if targetKillCount == 0 {
			continue
		}

		nextRoundStart := 0
		for nextIdx := idx + 1; nextIdx < len(ordered); nextIdx++ {
			if ordered[nextIdx].StartTick > 0 {
				nextRoundStart = ordered[nextIdx].StartTick
				break
			}
		}

		segment := FullRoundPOVSegment{
			Round:              round.Round,
			StartTick:          round.StartTick,
			FreezeEndTick:      round.FreezeEndTick,
			RoundEndTick:       round.EndTick,
			OfficialEndTick:    round.OfficialEndTick,
			NextRoundStartTick: nextRoundStart,
			RecordStartTick:    recordStart,
			RecordEndTick:      recordEnd,
			TargetSlot:         round.Slots[steamID],
			EndReason:          FullRoundPOVEndRoundEnd,
		}
		if death := findTargetDeath(round.Deaths, steamID, liveStart, round.EndTick); death != nil {
			segment.DeathTick = death.Tick
			segment.RecordEndTick = death.Tick
			segment.EndReason = FullRoundPOVEndTargetDeath
		}
		plan.Segments = append(plan.Segments, segment)
	}
	return plan
}

func findTargetDeath(deaths []FullRoundPOVDeath, steamID uint64, startTick int, roundEndTick int) *FullRoundPOVDeath {
	for i := range deaths {
		if deaths[i].VictimSteamID != steamID {
			continue
		}
		if deaths[i].Tick < startTick {
			continue
		}
		if roundEndTick > 0 && deaths[i].Tick > roundEndTick {
			continue
		}
		return &deaths[i]
	}
	return nil
}

func nonNegativeTick(tick int) int {
	if tick < 0 {
		return 0
	}
	return tick
}
