package logging

import (
	"net/url"
	"strings"
	"testing"
)

func TestSanitizeExportURL_RemovesSecretsAndUnknownQueryValues(t *testing.T) {
	got := SanitizeExportURL("https://example.com/release?version=1.2.3&token=secret&source=github&extra=drop")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse sanitized URL: %v", err)
	}
	query := parsed.Query()
	if query.Get("version") != "1.2.3" || query.Get("source") != "github" {
		t.Fatalf("safe query values changed: %q", got)
	}
	if query.Get("token") != "***" || query.Get("extra") != "" {
		t.Fatalf("sensitive or unknown query values leaked: %q", got)
	}
}

func TestSanitizeExportText_MasksCredentials(t *testing.T) {
	got := SanitizeExportText("connection failed token=abc Authorization: Bearer test-token")
	for _, secret := range []string{"abc", "test-token"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret leaked in %q", got)
		}
	}
}

func TestSanitizeExportText_MasksAbsoluteDemoPaths(t *testing.T) {
	input := `failed demos /private/imports/secret.dem and C:\\private\\imports\\secret.dem`
	got := SanitizeExportText(input)
	if strings.Contains(got, "secret.dem") || strings.Contains(got, "/private/imports") {
		t.Fatalf("demo path leaked in %q", got)
	}
	if strings.Count(got, "<redacted-demo>") != 2 {
		t.Fatalf("redacted demo count = %d, want 2 in %q", strings.Count(got, "<redacted-demo>"), got)
	}
}
