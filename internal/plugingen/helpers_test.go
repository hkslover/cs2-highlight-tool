package plugingen

import (
	"testing"

	"cs2-highlight-tool-v2/internal/config"
)

func TestBuildProduceHistoryKey_SortsKillIDs(t *testing.T) {
	key1 := BuildProduceHistoryKey("/demo.dem", "killer", 1, []string{"c", "a", "b"})
	key2 := BuildProduceHistoryKey("/demo.dem", "killer", 1, []string{"a", "b", "c"})
	if key1 != key2 {
		t.Fatalf("order should not matter: %q vs %q", key1, key2)
	}
}

func TestBuildProduceHistoryKey_DifferentViewsDiffer(t *testing.T) {
	k := BuildProduceHistoryKey("/demo.dem", "killer", 1, []string{"k1"})
	v := BuildProduceHistoryKey("/demo.dem", "victim", 1, []string{"k1"})
	if k == v {
		t.Fatal("killer and victim views must produce different keys")
	}
}

func TestSanitizeDemoSubDirName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"/path/to/match.dem", "match"},
		{"/path/to/de_mirage_20240101.dem", "de_mirage_20240101"},
		{"", "demo"},
		{"/path/spaces in name.dem", "spaces_in_name"},
	}
	for _, tc := range cases {
		if got := SanitizeDemoSubDirName(tc.input); got != tc.want {
			t.Errorf("SanitizeDemoSubDirName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestBuildBatchRecordSubDirs_DeduplicatesNames(t *testing.T) {
	demoPaths := []string{
		"/a/match.dem",
		"/b/match.dem",
		"/c/other.dem",
	}
	got := BuildBatchRecordSubDirs(demoPaths)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	if got[0] != "match" {
		t.Errorf("got[0] = %q, want %q", got[0], "match")
	}
	if got[1] != "match_2" {
		t.Errorf("got[1] = %q, want %q", got[1], "match_2")
	}
	if got[2] != "other" {
		t.Errorf("got[2] = %q, want %q", got[2], "other")
	}
}

func TestResolvePluginVideoPreset_AutoUsesDetectedManualKeepsChoice(t *testing.T) {
	cfg := &config.Config{
		FFmpegDetectedPreset:   "n1_h264",
		FFmpegDetectedEncoders: []string{"h264_nvenc", "libx264"},
	}

	if got := ResolvePluginVideoPreset("auto", cfg); got != "n1_h264" {
		t.Fatalf("auto should use detected preset, got=%q", got)
	}
	if got := ResolvePluginVideoPreset("n1", cfg); got != "n1" {
		t.Fatalf("manual preset should keep user choice, got=%q", got)
	}
	if got := ResolvePluginVideoPreset("auto", &config.Config{}); got != "c1" {
		t.Fatalf("auto without detection should fall back to c1, got=%q", got)
	}
	if got := ResolvePluginVideoPreset("auto", nil); got != "c1" {
		t.Fatalf("auto with nil cfg should fall back to c1, got=%q", got)
	}
	if got := ResolvePluginVideoPreset("n1", nil); got != "n1" {
		t.Fatalf("manual preset with nil cfg should keep user choice, got=%q", got)
	}
}
