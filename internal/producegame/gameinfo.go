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

// InjectPluginSearchPath inserts the "Game\tcsgo/plugin" search path line into
// gameinfo.gi content if it is not already present. Returns the modified content
// and true on success, or the original content and false if no injection point
// could be found.
func InjectPluginSearchPath(content string) (string, bool) {
	if HasPluginSearchPath(content) {
		return content, true
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "Game\tcsgo" && trimmed != "Game csgo" {
			continue
		}
		prefix := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		insert := prefix + "Game\tcsgo/plugin"
		next := make([]string, 0, len(lines)+1)
		next = append(next, lines[:i]...)
		next = append(next, insert)
		next = append(next, lines[i:]...)
		return strings.Join(next, "\n"), true
	}
	replaced := strings.Replace(content, "Game\tcsgo", "Game\tcsgo/plugin\n\t\t\tGame\tcsgo", 1)
	if replaced != content {
		return replaced, true
	}
	return content, false
}

// HasPluginSearchPath reports whether gameinfo.gi content already references
// the tool-managed csgo/plugin search path.
func HasPluginSearchPath(content string) bool {
	return strings.Contains(strings.ToLower(content), "csgo/plugin")
}
