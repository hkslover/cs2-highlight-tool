package steam

import (
	"fmt"
	"strconv"
	"strings"
)

// ProgressPrefix is the download_progress component ID prefix for Steam match
// imports, mirroring the 5E and Wanmei platform importers.
const ProgressPrefix = "steam_import_"

// ClientStatus describes why the Steam official match list is (not) available.
type ClientStatus string

const (
	// ClientReady means the list was fetched successfully.
	ClientReady ClientStatus = "ready"
	// ClientNotRunning means the local Steam client is not running.
	ClientNotRunning ClientStatus = "steam_not_running"
	// ClientNotLoggedIn means Steam is running but no account is signed in.
	ClientNotLoggedIn ClientStatus = "steam_not_logged_in"
	// ClientCS2Running means CS2 currently owns the Game Coordinator
	// connection, so the request would time out. The user has to quit CS2.
	ClientCS2Running ClientStatus = "cs2_running"
	// ClientGCUnavailable means the Game Coordinator did not answer in time.
	ClientGCUnavailable ClientStatus = "gc_unavailable"
	// ClientAPIUnavailable means no usable local steam_api64.dll was found.
	ClientAPIUnavailable ClientStatus = "steam_api_unavailable"
	// ClientUnsupportedPlatform means the feature is unavailable on this OS.
	ClientUnsupportedPlatform ClientStatus = "unsupported_platform"
)

// MatchListResult is the payload returned to the frontend for the import list.
type MatchListResult struct {
	Status   ClientStatus `json:"status"`
	SteamID  string       `json:"steam_id,omitempty"`
	Nickname string       `json:"nickname,omitempty"`
	Matches  []MatchItem  `json:"matches"`
}

// ProgressComponentID returns the download_progress component ID for a match.
func ProgressComponentID(matchID string) string {
	return ProgressPrefix + strings.TrimSpace(matchID)
}

// MatchRef is a normalized import target: either a numeric match ID (looked up
// in the recent list) or a pasted share code that carries the three values the
// Game Coordinator needs to resolve a specific match.
type MatchRef struct {
	MatchID   string
	ShareCode *ShareCode
}

// ParseMatchRef normalizes user input into a match reference. Numeric match IDs
// and share codes (with or without the CSGO prefix and separators) are accepted.
func ParseMatchRef(raw string) (MatchRef, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return MatchRef{}, fmt.Errorf("请填写比赛 ID 或分享码")
	}

	if isDigitsOnly(trimmed) {
		return MatchRef{MatchID: trimmed}, nil
	}

	decoded, err := ParseShareCode(trimmed)
	if err != nil {
		return MatchRef{}, fmt.Errorf("无法识别的比赛 ID 或分享码: %s", trimmed)
	}
	return MatchRef{
		MatchID:   strconv.FormatUint(decoded.MatchID, 10),
		ShareCode: &decoded,
	}, nil
}

func isDigitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}
