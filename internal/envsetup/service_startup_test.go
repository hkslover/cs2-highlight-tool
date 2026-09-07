package envsetup

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func markAllReadyForTest(s *Service) {
	s.mu.Lock()
	for i := range s.state.Steps {
		s.state.Steps[i].Status = statusReady
		s.state.Steps[i].Error = ""
	}
	s.state.SelfUpdate = SelfUpdateState{
		Status:  statusReady,
		Current: s.version,
	}
	s.mu.Unlock()
	s.refreshCanEnterMain()
}

func setPreferredReleaseSourceForTest(t *testing.T, fn func() (string, string, error)) {
	t.Helper()
	orig := preferredReleaseSourceFn
	preferredReleaseSourceFn = fn
	t.Cleanup(func() {
		preferredReleaseSourceFn = orig
	})
}

func setDetectCS2ExeFromSteamForTest(t *testing.T, fn func() (string, error)) {
	t.Helper()
	orig := detectCS2ExeFromSteamFn
	detectCS2ExeFromSteamFn = fn
	t.Cleanup(func() {
		detectCS2ExeFromSteamFn = orig
	})
}

func unifiedPayloadForTest() string {
	return `{
		"project":{"id":"cs2-highlight-tool-v2","name":"cs2-highlight-tool-v2"},
		"dependencies":{
			"advancedfx":{
				"name":"advancedfx",
				"repo":"advancedfx/advancedfx",
				"latest_tag":"v2.0.0",
				"latest":{
					"tag_name":"v2.0.0",
					"assets":[{
						"name":"hlae_2_0_0.zip",
						"url":"dl.snowblog.xyz/cs2-highlight-tool-v2/advancedfx/advancedfx/v2.0.0/hlae_2_0_0.zip",
						"github_url":"https://github.com/advancedfx/advancedfx/releases/download/v2.0.0/hlae_2_0_0.zip",
						"mirror_url":"https://gh-proxy.org/https://github.com/advancedfx/advancedfx/releases/download/v2.0.0/hlae_2_0_0.zip"
					}]
				}
			},
			"cs2-server-plugin":{
				"name":"cs2-server-plugin",
				"repo":"hkslover/cs2-server-plugin",
				"latest_tag":"v0.0.9",
				"latest":{
					"tag_name":"v0.0.9",
					"assets":[{
						"name":"cs2-server-plugin.zip",
						"url":"dl.snowblog.xyz/cs2-highlight-tool-v2/hkslover/cs2-server-plugin/v0.0.9/cs2-server-plugin.zip",
						"github_url":"https://github.com/hkslover/cs2-server-plugin/releases/download/v0.0.9/cs2-server-plugin.zip",
						"mirror_url":"https://gh-proxy.org/https://github.com/hkslover/cs2-server-plugin/releases/download/v0.0.9/cs2-server-plugin.zip"
					}]
				}
			}
		}
	}`
}

func TestRunStartupChecks_SourceReadyWhenUnifiedReleaseAvailable(t *testing.T) {
	setPreferredReleaseSourceForTest(t, func() (string, string, error) {
		return "github", "CN", nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(unifiedPayloadForTest()))
	}))
	defer server.Close()

	t.Setenv("CS2_RELEASE_API_URL", server.URL)

	exeDir := t.TempDir()
	svc := New(exeDir, "1.0.0")
	defer svc.stopFFmpegCapabilityDetection()
	svc.Startup(nil)

	tasksCalled := 0
	svc.runTasksFn = func(source DownloadSource) {
		tasksCalled++
		if source != defaultDownloadSource() {
			t.Fatalf("source = %q, want %q", source, defaultDownloadSource())
		}
		markAllReadyForTest(svc)
	}

	state := svc.RunStartupChecks()
	finalState := svc.GetStartupState()
	if state.SourceStep.Status != statusReady {
		t.Fatalf("source status = %q, want %q", state.SourceStep.Status, statusReady)
	}
	if state.SourceStep.Error != "" {
		t.Fatalf("source_step.error = %q, want empty", state.SourceStep.Error)
	}
	if state.SourceStep.CountryCode != "CN" {
		t.Fatalf("source_step.country_code = %q, want %q", state.SourceStep.CountryCode, "CN")
	}
	if tasksCalled != 1 {
		t.Fatalf("tasks called = %d, want 1", tasksCalled)
	}
	if state.Phase != phaseReady {
		t.Fatalf("phase = %q, want %q", state.Phase, phaseReady)
	}
	if !state.CanEnterMain {
		t.Fatal("can_enter_main should be true")
	}
	if finalState.Running {
		t.Fatal("running should be false after startup checks complete")
	}
}

func TestRunStartupChecks_SourceFailedWhenUnifiedReleaseUnavailable(t *testing.T) {
	setPreferredReleaseSourceForTest(t, func() (string, string, error) {
		return "github", "CN", nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	t.Setenv("CS2_RELEASE_API_URL", server.URL)

	exeDir := t.TempDir()
	svc := New(exeDir, "1.0.0")
	defer svc.stopFFmpegCapabilityDetection()
	svc.Startup(nil)

	tasksCalled := 0
	svc.runTasksFn = func(source DownloadSource) {
		tasksCalled++
		markAllReadyForTest(svc)
	}

	state := svc.RunStartupChecks()
	if state.SourceStep.Status != statusFailed {
		t.Fatalf("source status = %q, want %q", state.SourceStep.Status, statusFailed)
	}
	if strings.TrimSpace(state.SourceStep.Error) == "" {
		t.Fatal("source_step.error should be set")
	}
	if state.SourceStep.CountryCode != "CN" {
		t.Fatalf("source_step.country_code = %q, want %q", state.SourceStep.CountryCode, "CN")
	}
	if tasksCalled != 1 {
		t.Fatalf("tasks called = %d, want 1", tasksCalled)
	}
}

func TestRunStartupChecks_CS2ReadyWhenAutoDetectAvailable(t *testing.T) {
	setPreferredReleaseSourceForTest(t, func() (string, string, error) {
		return "github", "CN", nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(unifiedPayloadForTest()))
	}))
	defer server.Close()
	t.Setenv("CS2_RELEASE_API_URL", server.URL)

	exeDir := t.TempDir()
	_ = writeLocalHLAEForTest(t, filepath.Join(exeDir, "hlae"), `<changelog><version>2.0.0</version></changelog>`)
	_ = writeLocalPluginForTest(t, filepath.Join(exeDir, "plugin"), `<changelog><version>0.0.9</version></changelog>`)

	ffmpegDir := filepath.Join(exeDir, "ffmpeg", "bin")
	if err := os.MkdirAll(ffmpegDir, 0o755); err != nil {
		t.Fatalf("mkdir ffmpeg dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ffmpegDir, "ffmpeg.exe"), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write ffmpeg exe: %v", err)
	}

	detected := filepath.Join(exeDir, "steamlib", "steamapps", "common", "Counter-Strike 2", "game", "bin", "win64", "cs2.exe")
	if err := os.MkdirAll(filepath.Dir(detected), 0o755); err != nil {
		t.Fatalf("mkdir detected cs2 dir: %v", err)
	}
	if err := os.WriteFile(detected, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write detected cs2 exe: %v", err)
	}
	setDetectCS2ExeFromSteamForTest(t, func() (string, error) {
		return detected, nil
	})

	svc := New(exeDir, "1.0.0")
	defer svc.stopFFmpegCapabilityDetection()
	svc.Startup(nil)

	state := svc.RunStartupChecks()
	cs2Step := findStepByID(state.Steps, componentCS2)
	if cs2Step == nil {
		t.Fatal("cs2 step missing")
	}
	if cs2Step.Status != statusReady {
		t.Fatalf("cs2 step status = %q, want %q", cs2Step.Status, statusReady)
	}
	if cs2Step.Path != detected {
		t.Fatalf("cs2 step path = %q, want %q", cs2Step.Path, detected)
	}
}

func TestRunStartupChecks_DefersComponentTasksWhenSelfUpdateAvailable(t *testing.T) {
	setPreferredReleaseSourceForTest(t, func() (string, string, error) {
		return "github", "CN", nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"project":{"id":"cs2-highlight-tool-v2","name":"cs2-highlight-tool-v2"},
			"dependencies":{
				"app":{
					"name":"app",
					"repo":"hkslover/cs2-highlight-tool",
					"latest_tag":"v2.0.0",
					"latest":{
						"tag_name":"v2.0.0",
						"assets":[{
							"name":"cs2-highlight-tool-windows-amd64.exe",
							"url":"https://example.test/cs2-highlight-tool.exe",
							"github_url":"https://github.com/hkslover/cs2-highlight-tool/releases/download/v2.0.0/cs2-highlight-tool-windows-amd64.exe"
						}]
					}
				},
				"advancedfx":{
					"name":"advancedfx",
					"repo":"advancedfx/advancedfx",
					"latest_tag":"v2.0.0",
					"latest":{
						"tag_name":"v2.0.0",
						"assets":[{
							"name":"hlae_2_0_0.zip",
							"url":"https://example.test/hlae.zip",
							"github_url":"https://github.com/advancedfx/advancedfx/releases/download/v2.0.0/hlae_2_0_0.zip"
						}]
					}
				},
				"cs2-server-plugin":{
					"name":"cs2-server-plugin",
					"repo":"hkslover/cs2-server-plugin",
					"latest_tag":"v0.0.9",
					"latest":{
						"tag_name":"v0.0.9",
						"assets":[{
							"name":"cs2-server-plugin.zip",
							"url":"https://example.test/plugin.zip",
							"github_url":"https://github.com/hkslover/cs2-server-plugin/releases/download/v0.0.9/cs2-server-plugin.zip"
						}]
					}
				}
			}
		}`))
	}))
	defer server.Close()
	t.Setenv("CS2_RELEASE_API_URL", server.URL)

	exeDir := t.TempDir()
	_ = writeLocalHLAEForTest(t, filepath.Join(exeDir, "hlae"), `<changelog><version>2.0.0</version></changelog>`)
	_ = writeLocalPluginForTest(t, filepath.Join(exeDir, "plugin"), `<changelog><version>0.0.9</version></changelog>`)

	ffmpegDir := filepath.Join(exeDir, "ffmpeg", "bin")
	if err := os.MkdirAll(ffmpegDir, 0o755); err != nil {
		t.Fatalf("mkdir ffmpeg dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ffmpegDir, "ffmpeg.exe"), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write ffmpeg exe: %v", err)
	}

	detected := filepath.Join(exeDir, "steamlib", "steamapps", "common", "Counter-Strike 2", "game", "bin", "win64", "cs2.exe")
	if err := os.MkdirAll(filepath.Dir(detected), 0o755); err != nil {
		t.Fatalf("mkdir detected cs2 dir: %v", err)
	}
	if err := os.WriteFile(detected, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write detected cs2 exe: %v", err)
	}
	setDetectCS2ExeFromSteamForTest(t, func() (string, error) {
		return detected, nil
	})

	svc := New(exeDir, "1.0.0")
	defer svc.stopFFmpegCapabilityDetection()
	svc.Startup(nil)

	_ = svc.RunStartupChecks()
	state := svc.GetStartupState()
	if state.Running {
		t.Fatal("state.running should be false after startup checks return")
	}
	if state.CanEnterMain {
		t.Fatal("can_enter_main should remain false when self update is available")
	}
	if !state.SelfUpdate.Available {
		t.Fatal("self_update.available should be true")
	}
	if state.SelfUpdate.Status != statusNeedsAction {
		t.Fatalf("self_update.status = %q, want %q", state.SelfUpdate.Status, statusNeedsAction)
	}
	for _, step := range state.Steps {
		if step.Status != statusPending {
			t.Fatalf("component %s status = %q, want %q", step.ID, step.Status, statusPending)
		}
	}
}

func TestReinstallStartupComponent_ValidateGuard(t *testing.T) {
	setPreferredReleaseSourceForTest(t, func() (string, string, error) {
		return "github", "CN", nil
	})
	exeDir := t.TempDir()
	svc := New(exeDir, "1.0.0")
	defer svc.stopFFmpegCapabilityDetection()
	svc.Startup(nil)

	if _, err := svc.ReinstallStartupComponent("unknown"); err == nil {
		t.Fatal("expected invalid component error")
	}
	if _, err := svc.ReinstallStartupComponent(componentHLAE); err == nil {
		t.Fatal("expected not-ready guard error")
	}
}

func TestRefreshCanEnterMain_AllowsWarningStatus(t *testing.T) {
	setPreferredReleaseSourceForTest(t, func() (string, string, error) {
		return "github", "CN", nil
	})
	exeDir := t.TempDir()
	svc := New(exeDir, "1.0.0")
	defer svc.stopFFmpegCapabilityDetection()
	svc.Startup(nil)

	svc.mu.Lock()
	for i := range svc.state.Steps {
		svc.state.Steps[i].Status = statusReady
		svc.state.Steps[i].Error = ""
	}
	if step := svc.findStepLocked(componentPlugin); step != nil {
		step.Status = statusWarning
		step.Error = "最新版本获取失败，当前使用本地版本，可能导致后续功能异常"
	}
	svc.state.SelfUpdate.Status = statusReady
	svc.state.SelfUpdate.Available = false
	svc.state.FatalError = ""
	svc.mu.Unlock()

	svc.refreshCanEnterMain()
	state := svc.GetStartupState()
	if !state.CanEnterMain {
		t.Fatal("can_enter_main should be true for warning status")
	}
	if strings.TrimSpace(state.EntryNotice) == "" {
		t.Fatal("entry_notice should be set when warning exists")
	}
}

func TestRunStartupChecks_DoesNotDependOnLegacySourceManual(t *testing.T) {
	setPreferredReleaseSourceForTest(t, func() (string, string, error) {
		return "github", "CN", nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(unifiedPayloadForTest()))
	}))
	defer server.Close()

	t.Setenv("CS2_RELEASE_API_URL", server.URL)

	exeDir := t.TempDir()
	configPath := exeDir + string(os.PathSeparator) + "config.json"
	if err := os.WriteFile(configPath, []byte(`{"download_source":"custom","source_manual":true}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	svc := New(exeDir, "1.0.0")
	defer svc.stopFFmpegCapabilityDetection()
	svc.Startup(nil)
	svc.runTasksFn = func(source DownloadSource) {
		markAllReadyForTest(svc)
	}
	state := svc.RunStartupChecks()
	if state.SourceStep.Source != string(defaultDownloadSource()) {
		t.Fatalf("source_step.source = %q, want %q", state.SourceStep.Source, defaultDownloadSource())
	}
}

func TestRunStartupChecks_DoesNotBlockOnFFmpegDetectWhenSelfUpdateFails(t *testing.T) {
	setPreferredReleaseSourceForTest(t, func() (string, string, error) {
		return "github", "CN", nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"project":{"id":"cs2-highlight-tool-v2","name":"cs2-highlight-tool-v2"},
			"dependencies":{
				"advancedfx":{
					"name":"advancedfx",
					"repo":"advancedfx/advancedfx",
					"latest_tag":"v2.0.0",
					"latest":{
						"tag_name":"v2.0.0",
						"assets":[{
							"name":"hlae_2_0_0.zip",
							"url":"dl.snowblog.xyz/cs2-highlight-tool-v2/advancedfx/advancedfx/v2.0.0/hlae_2_0_0.zip",
							"github_url":"https://github.com/advancedfx/advancedfx/releases/download/v2.0.0/hlae_2_0_0.zip",
							"mirror_url":"https://gh-proxy.org/https://github.com/advancedfx/advancedfx/releases/download/v2.0.0/hlae_2_0_0.zip"
						}]
					}
				},
				"cs2-server-plugin":{
					"name":"cs2-server-plugin",
					"repo":"hkslover/cs2-server-plugin",
					"latest_tag":"v2.0.0",
					"latest":{
						"tag_name":"v2.0.0",
						"assets":[{
							"name":"cs2-server-plugin.zip",
							"url":"dl.snowblog.xyz/cs2-highlight-tool-v2/hkslover/cs2-server-plugin/v2.0.0/cs2-server-plugin.zip",
							"github_url":"https://github.com/hkslover/cs2-server-plugin/releases/download/v2.0.0/cs2-server-plugin.zip",
							"mirror_url":"https://gh-proxy.org/https://github.com/hkslover/cs2-server-plugin/releases/download/v2.0.0/cs2-server-plugin.zip"
						}]
					}
				}
			}
		}`))
	}))
	defer server.Close()

	t.Setenv("CS2_RELEASE_API_URL", server.URL)

	exeDir := t.TempDir()
	_ = writeLocalHLAEForTest(t, filepath.Join(exeDir, "hlae"), `<changelog><version>2.0.0</version></changelog>`)
	_ = writeLocalPluginForTest(t, filepath.Join(exeDir, "plugin"), `<changelog><version>2.0.0</version></changelog>`)
	ffmpegDir := filepath.Join(exeDir, "ffmpeg", "bin")
	if err := os.MkdirAll(ffmpegDir, 0o755); err != nil {
		t.Fatalf("mkdir ffmpeg dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ffmpegDir, "ffmpeg.exe"), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write ffmpeg exe: %v", err)
	}

	svc := New(exeDir, "1.0.0")
	defer svc.stopFFmpegCapabilityDetection()
	svc.Startup(nil)

	oldCmdFactory := ffmpegDetectCommandContext
	ffmpegDetectCommandContext = fakeDetectCommandContextWithOptions("hevc_nvenc,h264_nvenc,libx264", 400*time.Millisecond, nil)
	t.Cleanup(func() {
		ffmpegDetectCommandContext = oldCmdFactory
	})

	startedAt := time.Now()
	state := svc.RunStartupChecks()
	elapsed := time.Since(startedAt)

	if elapsed > 900*time.Millisecond {
		t.Fatalf("RunStartupChecks elapsed=%s, expect not blocked by ffmpeg detect", elapsed)
	}
	finalState := svc.GetStartupState()
	if finalState.Running {
		t.Fatal("state.running should be false after RunStartupChecks returns")
	}
	if state.SelfUpdate.Status != statusFailed {
		t.Fatalf("self_update.status = %q, want %q", state.SelfUpdate.Status, statusFailed)
	}
	cs2Step := findStepByID(state.Steps, componentCS2)
	if cs2Step == nil {
		t.Fatal("cs2 step missing")
	}
	if cs2Step.Status != statusNeedsAction {
		t.Fatalf("cs2 step status = %q, want %q", cs2Step.Status, statusNeedsAction)
	}

	waitFFmpegDetectDone(t, svc, 8*time.Second)
}
