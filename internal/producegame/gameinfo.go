package producegame

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cs2-highlight-tool-v2/internal/config"
)

// ResolveGameInfoPath resolves the path to CS2's gameinfo.gi file by searching from
// the CS2 executable's directory upward and from the configured CS2 directory.
// Returns an error if the file cannot be found.
func ResolveGameInfoPath(cs2Exe string, cs2Dir string) (string, error) {
	candidates := make([]string, 0, 16)
	if cs2Dir != "" {
		candidates = append(candidates,
			filepath.Join(cs2Dir, "game", "csgo", "gameinfo.gi"),
			filepath.Join(cs2Dir, "csgo", "gameinfo.gi"),
		)
	}
	start := filepath.Dir(cs2Exe)
	current := start
	for i := 0; i < 8; i++ {
		candidates = append(candidates,
			filepath.Join(current, "game", "csgo", "gameinfo.gi"),
			filepath.Join(current, "csgo", "gameinfo.gi"),
		)
		next := filepath.Dir(current)
		if next == current {
			break
		}
		current = next
	}

	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		cleaned := config.CleanPath(candidate)
		if cleaned == "" {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		if info, err := os.Stat(cleaned); err == nil && !info.IsDir() {
			return cleaned, nil
		}
	}
	return "", fmt.Errorf("未找到 gameinfo.gi，请确认 CS2 路径配置")
}

// SearchPathPlugin is this tool's plugin search path injected into gameinfo.gi.
const SearchPathPlugin = "csgo/plugin"

// SearchPathPOV is the POV HUD (pov.vpk) search path injected into gameinfo.gi
// when the user enables POV HUD mode for recording.
const SearchPathPOV = "csgo/pov.vpk"

// SearchPathBrand is always mounted for recording, before the optional POV HUD.
const SearchPathBrand = "csgo/brand.vpk"

// InjectPluginSearchPath inserts the "Game\tcsgo/plugin" search path line into
// gameinfo.gi content if it is not already present. Returns the modified content
// and true on success, or the original content and false if no injection point
// could be found.
func InjectPluginSearchPath(content string) (string, bool) {
	return InjectSearchPath(content, SearchPathPlugin)
}

// InjectSearchPath inserts a "Game\t<searchPath>" line into gameinfo.gi content
// if it is not already present. It mirrors InjectPluginSearchPath but accepts an
// arbitrary search path (e.g. "csgo/plugin", "csgo/pov.vpk"). Returns the modified
// content and true on success, or the original content and false if no injection
// point could be found.
func InjectSearchPath(content string, searchPath string) (string, bool) {
	if HasSearchPath(content, searchPath) {
		return content, true
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "Game\tcsgo" && trimmed != "Game csgo" {
			continue
		}
		prefix := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		insert := prefix + "Game\t" + searchPath
		next := make([]string, 0, len(lines)+1)
		next = append(next, lines[:i]...)
		next = append(next, insert)
		next = append(next, lines[i:]...)
		return strings.Join(next, "\n"), true
	}
	replaced := strings.Replace(content, "Game\tcsgo", "Game\t"+searchPath+"\n\t\t\tGame\tcsgo", 1)
	if replaced != content {
		return replaced, true
	}
	return content, false
}

// HasPluginSearchPath reports whether gameinfo.gi content contains this tool's
// plugin search path as a standalone SearchPaths entry.
func HasPluginSearchPath(content string) bool {
	return HasSearchPath(content, SearchPathPlugin)
}

// HasSearchPath reports whether gameinfo.gi content contains the given search
// path as a standalone SearchPaths entry (e.g. "csgo/plugin", "csgo/pov.vpk").
// Comments and unrelated text are ignored.
func HasSearchPath(content string, searchPath string) bool {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if isSearchPathLine(line, searchPath) {
			return true
		}
	}
	return false
}

// RemovePluginSearchPath removes this tool's standalone plugin search path entries
// from gameinfo.gi content. Comments and unrelated text are left unchanged.
func RemovePluginSearchPath(content string) (string, bool) {
	return RemoveSearchPath(content, SearchPathPlugin)
}

// RemoveSearchPath removes all standalone entries for the given search path
// (e.g. "csgo/plugin", "csgo/pov.vpk") from gameinfo.gi content. Comments and
// unrelated text are left unchanged. Returns the modified content and true if any
// line was removed.
func RemoveSearchPath(content string, searchPath string) (string, bool) {
	lines := strings.Split(content, "\n")
	next := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		if isSearchPathLine(line, searchPath) {
			changed = true
			continue
		}
		next = append(next, line)
	}
	if !changed {
		return content, false
	}
	return strings.Join(next, "\n"), true
}

func isSearchPathLine(line string, searchPath string) bool {
	return strings.TrimSpace(line) == "Game\t"+searchPath || strings.TrimSpace(line) == "Game "+searchPath
}
