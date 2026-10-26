package steam

import (
	"os"
	"path/filepath"
	"strings"
)

// steamAPIFileName is Valve's client runtime. It is loaded from the user's own
// Counter-Strike installation at runtime: this tool never ships, extracts or
// drops a Valve (or any other) binary.
const steamAPIFileName = "steam_api64.dll"

// requiredSteamAPIExports are the flat exports the binding calls. Every
// candidate library must provide all of them, which also filters out ancient
// Steamworks SDK copies that predate the flat API.
var requiredSteamAPIExports = []string{
	"SteamAPI_InitFlat",
	"SteamAPI_Shutdown",
	"SteamAPI_RunCallbacks",
	"SteamAPI_IsSteamRunning",
	"SteamAPI_GetHSteamUser",
	"SteamAPI_GetHSteamPipe",
	"SteamInternal_FindOrCreateUserInterface",
	"SteamAPI_ISteamUser_GetSteamID",
	"SteamAPI_ISteamUser_BLoggedOn",
}

// steamAPICandidatePaths expands the configured directories into existing
// steam_api64.dll candidates, most preferred first and without duplicates.
//
// The directories come from the caller (in practice the CS2 installation
// directory, whose game/bin/win64 folder ships the DLL next to cs2.exe).
func steamAPICandidatePaths(dirs []string) []string {
	candidates := make([]string, 0, len(dirs))
	seen := make(map[string]struct{}, len(dirs))
	for _, dir := range dirs {
		trimmed := strings.TrimSpace(dir)
		if trimmed == "" {
			continue
		}
		absolute, err := filepath.Abs(trimmed)
		if err != nil {
			continue
		}
		candidate := filepath.Join(filepath.Clean(absolute), steamAPIFileName)
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || !info.Mode().IsRegular() {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, candidate)
	}
	return candidates
}
