package envsetup

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/release"
)

type hlaeChannelFixture struct {
	exeDir    string
	svc       *Service
	downloads *atomic.Int32
}

// newHLAEChannelFixture serves one archive per tag and seeds a release
// snapshot with a main (v2.0.0) and a legacy (v1.0.0) channel.
func newHLAEChannelFixture(t *testing.T, channels []release.Channel) *hlaeChannelFixture {
	t.Helper()
	archives := map[string][]byte{}
	for _, version := range []string{"2.0.0", "1.0.0"} {
		archives["/hlae_"+version+".zip"] = createZipBytesForTest(t, map[string][]byte{
			"release/hlae.exe":               []byte("hlae " + version),
			"release/x64/AfxHookSource2.dll": []byte("hook"),
			"release/changelog.xml":          []byte("<changelog><version>" + version + "</version></changelog>"),
		})
	}
	downloads := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := archives[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		downloads.Add(1)
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)

	infoFor := func(version string) *release.Info {
		url := server.URL + "/hlae_" + version + ".zip"
		return &release.Info{
			TagName: "v" + version,
			Repo:    "advancedfx/advancedfx",
			Source:  release.SourceGitHub,
			Assets: []release.Asset{
				{Name: "hlae_" + version + ".zip", DownloadURL: url, URL: url, GitHubURL: url},
			},
		}
	}

	exeDir := t.TempDir()
	svc := New(exeDir, "1.0.0")
	svc.Startup(nil)
	svc.mu.Lock()
	svc.state.SourceStep.CountryCode = "US"
	svc.releaseSnapshot = &release.UnifiedLatest{
		Components: map[string]map[string][]release.ComponentCandidate{
			release.ComponentKeyHLAE: {
				release.SourceGitHub: {
					{
						Repo:     "advancedfx/advancedfx",
						Source:   release.SourceGitHub,
						OK:       true,
						Info:     infoFor("2.0.0"),
						Channels: channels,
						Versions: map[string]*release.Info{
							"v2.0.0": infoFor("2.0.0"),
							"v1.0.0": infoFor("1.0.0"),
						},
					},
				},
			},
		},
	}
	svc.mu.Unlock()
	return &hlaeChannelFixture{exeDir: exeDir, svc: svc, downloads: downloads}
}

var defaultTestHLAEChannels = []release.Channel{
	{ID: "main", Name: "main", Tag: "v2.0.0"},
	{ID: "legacy", Name: "旧版", Note: "适配旧版 CS2", Tag: "v1.0.0"},
}

func changelogForTest(version string) string {
	return "<changelog><version>" + version + "</version></changelog>"
}

func TestSelectHLAEChannel_SwitchesToInstalledChannelWithoutDownload(t *testing.T) {
	f := newHLAEChannelFixture(t, defaultTestHLAEChannels)
	mainExe := writeLocalHLAEForTest(t, filepath.Join(f.exeDir, "hlae"), changelogForTest("2.0.0"))
	legacyDir := filepath.Join(f.exeDir, hlaeChannelsDirName, "legacy")
	legacyExe := writeLocalHLAEForTest(t, legacyDir, changelogForTest("1.0.0"))
	ffmpegExe := filepath.Join(f.exeDir, "ffmpeg", "bin", "ffmpeg.exe")
	if err := os.MkdirAll(filepath.Dir(ffmpegExe), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ffmpegExe, []byte("ffmpeg"), 0644); err != nil {
		t.Fatal(err)
	}

	state, err := f.svc.SelectHLAEChannel("legacy")
	if err != nil {
		t.Fatalf("SelectHLAEChannel error: %v", err)
	}
	if got := f.downloads.Load(); got != 0 {
		t.Fatalf("downloads = %d, want 0 for an installed channel", got)
	}
	if state.Config.HLAEChannel != "legacy" {
		t.Fatalf("config channel = %q, want legacy", state.Config.HLAEChannel)
	}
	if !strings.EqualFold(filepath.Clean(state.Config.HLAEExe), filepath.Clean(legacyExe)) {
		t.Fatalf("config hlae_exe = %q, want %q", state.Config.HLAEExe, legacyExe)
	}
	step := hlaeStepForTest(t, state)
	if step.Status != statusReady || step.LocalVersion != "1.0.0" || step.Channel != "legacy" || step.Error != "" {
		t.Fatalf("unexpected step: %#v", step)
	}
	if len(step.Channels) != 2 || !step.Channels[0].Installed || !step.Channels[1].Installed {
		t.Fatalf("channels = %#v, want both installed", step.Channels)
	}
	if err := validateHLAEFfmpegIni(legacyDir, ffmpegExe); err != nil {
		t.Fatalf("legacy ffmpeg.ini: %v", err)
	}
	if _, err := os.Stat(mainExe); err != nil {
		t.Fatalf("main channel must stay installed: %v", err)
	}

	state, err = f.svc.SelectHLAEChannel("main")
	if err != nil {
		t.Fatalf("SelectHLAEChannel(main) error: %v", err)
	}
	if got := f.downloads.Load(); got != 0 {
		t.Fatalf("downloads = %d after switching back, want 0", got)
	}
	if !strings.EqualFold(filepath.Clean(state.Config.HLAEExe), filepath.Clean(mainExe)) {
		t.Fatalf("config hlae_exe = %q, want %q", state.Config.HLAEExe, mainExe)
	}
}

func TestSelectHLAEChannel_DownloadsMissingChannelIntoItsOwnDir(t *testing.T) {
	f := newHLAEChannelFixture(t, defaultTestHLAEChannels)
	mainExe := writeLocalHLAEForTest(t, filepath.Join(f.exeDir, "hlae"), changelogForTest("2.0.0"))

	state, err := f.svc.SelectHLAEChannel("legacy")
	if err != nil {
		t.Fatalf("SelectHLAEChannel error: %v", err)
	}
	if got := f.downloads.Load(); got != 1 {
		t.Fatalf("downloads = %d, want 1", got)
	}
	legacyExe := filepath.Join(f.exeDir, hlaeChannelsDirName, "legacy", "HLAE.exe")
	if !strings.EqualFold(filepath.Clean(state.Config.HLAEExe), filepath.Clean(legacyExe)) {
		t.Fatalf("config hlae_exe = %q, want %q", state.Config.HLAEExe, legacyExe)
	}
	if version, err := resolveInstalledHLAEVersion(legacyExe); err != nil || version != "1.0.0" {
		t.Fatalf("legacy version = %q, %v", version, err)
	}
	if version, err := resolveInstalledHLAEVersion(mainExe); err != nil || version != "2.0.0" {
		t.Fatalf("main channel changed: %q, %v", version, err)
	}
	step := hlaeStepForTest(t, state)
	if step.Status != statusReady || step.Channel != "legacy" || len(step.Channels) != 2 || !step.Channels[1].Installed {
		t.Fatalf("unexpected step: %#v", step)
	}
}

func TestEnsureHLAE_ReplacesChannelVersionOnExactMismatch(t *testing.T) {
	f := newHLAEChannelFixture(t, defaultTestHLAEChannels)
	// A newer build in the legacy directory is still wrong for that channel.
	legacyExe := writeLocalHLAEForTest(t, filepath.Join(f.exeDir, hlaeChannelsDirName, "legacy"), changelogForTest("2.0.0"))
	if _, err := f.svc.persistConfig(func(next *config.Config) error {
		next.HLAEChannel = "legacy"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := f.svc.ensureHLAE(DownloadSourceGitHub); err != nil {
		t.Fatalf("ensureHLAE error: %v", err)
	}
	if got := f.downloads.Load(); got != 1 {
		t.Fatalf("downloads = %d, want 1", got)
	}
	if version, err := resolveInstalledHLAEVersion(legacyExe); err != nil || version != "1.0.0" {
		t.Fatalf("legacy version = %q, %v; want 1.0.0", version, err)
	}
}

func TestEnsureHLAE_FallsBackToMainWhenChannelIsGone(t *testing.T) {
	f := newHLAEChannelFixture(t, defaultTestHLAEChannels[:1])
	mainExe := writeLocalHLAEForTest(t, filepath.Join(f.exeDir, "hlae"), changelogForTest("2.0.0"))
	if _, err := f.svc.persistConfig(func(next *config.Config) error {
		next.HLAEChannel = "legacy"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	f.svc.runComponent(componentHLAE, f.svc.ensureHLAEWithFallback)

	state := f.svc.GetStartupState()
	step := hlaeStepForTest(t, state)
	if step.Status != statusReady || step.Channel != defaultHLAEChannelID || !strings.Contains(step.Error, "已下线") {
		t.Fatalf("unexpected step: %#v", step)
	}
	if !strings.EqualFold(filepath.Clean(state.Config.HLAEExe), filepath.Clean(mainExe)) {
		t.Fatalf("config hlae_exe = %q, want %q", state.Config.HLAEExe, mainExe)
	}
	if state.Config.HLAEChannel != "legacy" {
		t.Fatalf("fallback must not overwrite the user's choice, got %q", state.Config.HLAEChannel)
	}
	if got := f.downloads.Load(); got != 0 {
		t.Fatalf("downloads = %d, want 0", got)
	}
}

func TestEnsureHLAE_UsesLatestWithoutChannels(t *testing.T) {
	f := newHLAEChannelFixture(t, nil)
	if err := f.svc.ensureHLAE(DownloadSourceGitHub); err != nil {
		t.Fatalf("ensureHLAE error: %v", err)
	}
	mainExe := filepath.Join(f.exeDir, "hlae", "HLAE.exe")
	if version, err := resolveInstalledHLAEVersion(mainExe); err != nil || version != "2.0.0" {
		t.Fatalf("main version = %q, %v", version, err)
	}
	step := hlaeStepForTest(t, f.svc.GetStartupState())
	if len(step.Channels) != 0 {
		t.Fatalf("channels = %#v, want none", step.Channels)
	}
}

func TestSelectHLAEChannel_RejectsUnknownChannel(t *testing.T) {
	f := newHLAEChannelFixture(t, defaultTestHLAEChannels)
	if _, err := f.svc.SelectHLAEChannel("nightly"); err == nil {
		t.Fatal("expected error for unknown channel")
	}
	if got := f.svc.currentConfig().HLAEChannel; got != "" {
		t.Fatalf("config channel = %q, want unchanged", got)
	}
}

func TestTryWriteHLAEFfmpegIni_WritesEveryChannel(t *testing.T) {
	exeDir := t.TempDir()
	mainDir := filepath.Join(exeDir, "hlae")
	legacyDir := filepath.Join(exeDir, hlaeChannelsDirName, "legacy")
	stagingDir := filepath.Join(exeDir, hlaeChannelsDirName, ".legacy.staging-1")
	for _, dir := range []string{mainDir, legacyDir, stagingDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(exeDir, "1.0.0")
	svc.Startup(nil)

	ffmpegExe := filepath.Join(exeDir, "ffmpeg", "bin", "ffmpeg.exe")
	svc.tryWriteHLAEFfmpegIni(ffmpegExe)

	for _, dir := range []string{mainDir, legacyDir} {
		if err := validateHLAEFfmpegIni(dir, ffmpegExe); err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(stagingDir, "ffmpeg")); !os.IsNotExist(err) {
		t.Fatalf("staging directory must be skipped, stat err = %v", err)
	}
}

func TestIsManagedHLAEDir(t *testing.T) {
	exeDir := t.TempDir()
	svc := New(exeDir, "1.0.0")
	cases := map[string]bool{
		filepath.Join(exeDir, "hlae"):                        true,
		filepath.Join(exeDir, hlaeChannelsDirName, "legacy"): true,
		filepath.Join(exeDir, hlaeChannelsDirName):           false,
		filepath.Join(exeDir, "plugin"):                      false,
		filepath.Join(t.TempDir(), "hlae"):                   false,
		filepath.Join(exeDir, hlaeChannelsDirName, "a", "b"): false,
	}
	for dir, want := range cases {
		if got := svc.isManagedHLAEDir(dir); got != want {
			t.Errorf("isManagedHLAEDir(%q) = %v, want %v", dir, got, want)
		}
	}
}

func TestSelectHLAEChannel_DownloadFailureKeepsActiveChannelVisible(t *testing.T) {
	f := newHLAEChannelFixture(t, []release.Channel{
		{ID: "main", Name: "main", Tag: "v2.0.0"},
		{ID: "broken", Name: "broken", Tag: "v9.9.9"},
	})
	mainExe := writeLocalHLAEForTest(t, filepath.Join(f.exeDir, "hlae"), changelogForTest("2.0.0"))
	f.svc.mu.Lock()
	candidate := &f.svc.releaseSnapshot.Components[release.ComponentKeyHLAE][release.SourceGitHub][0]
	candidate.Versions["v9.9.9"] = &release.Info{
		TagName: "v9.9.9",
		Assets:  []release.Asset{{Name: "hlae_9_9_9.zip", URL: candidate.Info.Assets[0].URL + ".missing", GitHubURL: candidate.Info.Assets[0].URL + ".missing"}},
	}
	f.svc.mu.Unlock()

	state, err := f.svc.SelectHLAEChannel("broken")
	if err != nil {
		t.Fatalf("SelectHLAEChannel error: %v", err)
	}
	step := hlaeStepForTest(t, state)
	if step.Status != statusWarning || step.Channel != defaultHLAEChannelID {
		t.Fatalf("unexpected step after failed switch: %#v", step)
	}
	if !strings.EqualFold(filepath.Clean(state.Config.HLAEExe), filepath.Clean(mainExe)) {
		t.Fatalf("config hlae_exe = %q, want %q", state.Config.HLAEExe, mainExe)
	}
}
