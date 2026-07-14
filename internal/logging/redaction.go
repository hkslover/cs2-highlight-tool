package logging

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reURLInExportText = regexp.MustCompile(`https?://[^\s"'<>()]+`)
	reWindowsUserHome = regexp.MustCompile(`(?i)[a-z]:\\users\\[^\\\s]+`)
	reMacUserHome     = regexp.MustCompile(`/Users/[^/\s]+`)
	reLinuxUserHome   = regexp.MustCompile(`/home/[^/\s]+`)
	reAbsoluteDemo    = regexp.MustCompile(`(?i)(?:[a-z]:\\+|/)[^\s"'<>]*\.dem(?:\.json)?`)
)

// SanitizeExportText removes credentials, URL query secrets, and user home
// prefixes before text is persisted or exported outside the process.
func SanitizeExportText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = sanitizeURLsInExportText(value)
	value = reAbsoluteDemo.ReplaceAllString(value, "<redacted-demo>")
	value = sanitizeText(value)
	return SanitizePathPrefixInText(value)
}

// SanitizeExportDemoPath deliberately removes the complete demo location.
// Demo files can be manually imported from arbitrary directories, so merely
// stripping a user-home prefix is not sufficient for support exports.
func SanitizeExportDemoPath(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "<redacted-demo>"
}

// SanitizeExportPath preserves the path shape while removing a user's home
// directory. It deliberately does not drop path segments so support reports
// remain actionable.
func SanitizeExportPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return SanitizePathPrefixInText(value)
}

// SanitizeExportURL removes credentials, fragments, and unrecognised query
// parameters while retaining the small safe set used in release diagnostics.
func SanitizeExportURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return sanitizeText(rawURL)
	}
	parsed.User = nil
	parsed.Fragment = ""
	values := parsed.Query()
	sanitized := url.Values{}
	for key, items := range values {
		keyLower := strings.ToLower(strings.TrimSpace(key))
		switch {
		case isSensitiveKey(keyLower):
			sanitized.Set(key, "***")
		case isAllowedExportURLQueryKey(keyLower):
			for _, item := range items {
				sanitized.Add(key, sanitizeText(item))
			}
		}
	}
	parsed.RawQuery = sanitized.Encode()
	return parsed.String()
}

// SanitizeExportMetaValue applies key-aware redaction to a structured value.
func SanitizeExportMetaValue(key, value string) string {
	keyLower := strings.ToLower(strings.TrimSpace(key))
	switch {
	case isSensitiveKey(keyLower):
		return "***"
	case strings.Contains(keyLower, "demo") && (strings.Contains(keyLower, "path") || strings.Contains(keyLower, "file")):
		return SanitizeExportDemoPath(value)
	case strings.Contains(keyLower, "url"):
		return SanitizeExportURL(value)
	case strings.Contains(keyLower, "path"), strings.Contains(keyLower, "dir"), strings.Contains(keyLower, "file"):
		return SanitizeExportPath(value)
	default:
		return SanitizeExportText(value)
	}
}

// SanitizePathPrefixInText replaces the current and common platform user-home
// prefixes with ~. It accepts arbitrary text because errors often embed paths.
func SanitizePathPrefixInText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err == nil {
		home = strings.TrimSpace(home)
		if home != "" {
			value = strings.ReplaceAll(value, home, "~")
			value = strings.ReplaceAll(value, filepath.ToSlash(home), "~")
		}
	}
	value = reWindowsUserHome.ReplaceAllString(value, "~")
	value = reMacUserHome.ReplaceAllString(value, "~")
	value = reLinuxUserHome.ReplaceAllString(value, "~")
	return value
}

func sanitizeURLsInExportText(value string) string {
	return reURLInExportText.ReplaceAllStringFunc(value, SanitizeExportURL)
}

func isAllowedExportURLQueryKey(key string) bool {
	switch key {
	case "id", "name", "tag", "version", "arch", "os", "platform", "file", "filename", "source", "mirror", "channel":
		return true
	default:
		return false
	}
}
