package ffmpegprofile

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeUserPreset(t *testing.T) {
	if got := NormalizeUserPreset("N1"); got != UserPresetN1 {
		t.Fatalf("NormalizeUserPreset(N1) = %q", got)
	}
	if got := NormalizeUserPreset("bad"); got != UserPresetAuto {
		t.Fatalf("NormalizeUserPreset(bad) = %q", got)
	}
}

func TestResolveVideoPreset_PreservesAutoAndReportsEffectiveEncoder(t *testing.T) {
	resolved := ResolveVideoPreset(UserPresetAuto, UserPresetN1, []string{"hevc_nvenc", "libx264"})
	if resolved.RequestedPreset != UserPresetAuto {
		t.Fatalf("requested preset = %q, want auto", resolved.RequestedPreset)
	}
	if resolved.EffectivePreset != UserPresetN1 || resolved.Encoder != "hevc_nvenc" {
		t.Fatalf("effective profile = %+v, want n1/hevc_nvenc", resolved)
	}
	if resolved.Status != VideoPresetStatusReady {
		t.Fatalf("status = %q, want ready", resolved.Status)
	}
}

func TestResolveVideoPreset_ReportsHardwareH264Fallback(t *testing.T) {
	resolved := ResolveVideoPreset(UserPresetAuto, "", []string{"h264_nvenc", "libx264"})
	if resolved.EffectivePreset != internalPresetN1H264 || resolved.Encoder != "h264_nvenc" {
		t.Fatalf("effective profile = %+v, want n1_h264/h264_nvenc", resolved)
	}
	if resolved.Status != VideoPresetStatusReady {
		t.Fatalf("status = %q, want ready", resolved.Status)
	}
}

func TestResolveVideoPreset_MarksMissingDetectionPending(t *testing.T) {
	resolved := ResolveVideoPreset(UserPresetAuto, "", nil)
	if resolved.EffectivePreset != UserPresetC1 || resolved.Encoder != "libx264" {
		t.Fatalf("fallback profile = %+v, want c1/libx264", resolved)
	}
	if resolved.Status != VideoPresetStatusPending {
		t.Fatalf("status = %q, want pending", resolved.Status)
	}
}

func TestResolveVideoPresetManualUsesRequestedProfile(t *testing.T) {
	resolved := ResolveVideoPreset(UserPresetN1, UserPresetA1, []string{"hevc_amf"})
	if resolved.RequestedPreset != UserPresetN1 || resolved.EffectivePreset != UserPresetN1 || resolved.Encoder != "hevc_nvenc" {
		t.Fatalf("manual resolution = %+v, want n1/hevc_nvenc", resolved)
	}
	if resolved.Status != VideoPresetStatusManual {
		t.Fatalf("status = %q, want manual", resolved.Status)
	}
}

func TestResolveProfile_AutoPriority(t *testing.T) {
	caps := CapabilitiesFromEncoders([]string{"h264_nvenc", "libx264"})
	resolved := ResolveProfile(UserPresetAuto, caps)
	if resolved.SelectedProfile.ID != internalPresetN1H264 {
		t.Fatalf("auto selected = %q, want %q", resolved.SelectedProfile.ID, internalPresetN1H264)
	}

	caps2 := CapabilitiesFromEncoders([]string{"hevc_qsv", "libx264"})
	resolved2 := ResolveProfile(UserPresetAuto, caps2)
	if resolved2.SelectedProfile.ID != UserPresetI1 {
		t.Fatalf("auto selected = %q, want %q", resolved2.SelectedProfile.ID, UserPresetI1)
	}
}

func TestResolveProfile_ManualFallback(t *testing.T) {
	caps := CapabilitiesFromEncoders([]string{"h264_amf", "libx264"})
	resolved := ResolveProfile(UserPresetA1, caps)
	if resolved.SelectedProfile.ID != internalPresetA1H264 {
		t.Fatalf("manual a1 selected = %q, want %q", resolved.SelectedProfile.ID, internalPresetA1H264)
	}

	caps2 := CapabilitiesFromEncoders([]string{"libx264"})
	resolved2 := ResolveProfile(UserPresetN1, caps2)
	if resolved2.SelectedProfile.ID != UserPresetC1 {
		t.Fatalf("manual n1 selected = %q, want %q", resolved2.SelectedProfile.ID, UserPresetC1)
	}
}

func TestResolveProfile_ManualWithoutCapabilitiesKeepsUserPreset(t *testing.T) {
	resolved := ResolveProfile(UserPresetN1, CapabilitiesFromEncoders(nil))
	if resolved.SelectedProfile.ID != UserPresetN1 {
		t.Fatalf("manual n1 with empty capabilities selected = %q, want %q", resolved.SelectedProfile.ID, UserPresetN1)
	}
}

func TestBuildRetryChain_AutoAndManual(t *testing.T) {
	caps := CapabilitiesFromEncoders([]string{"hevc_nvenc", "h264_nvenc", "libx264"})
	manual := BuildRetryChain(UserPresetN1, caps)
	if len(manual) != 3 {
		t.Fatalf("manual retry len=%d want 3", len(manual))
	}
	if manual[0].ID != UserPresetN1 || manual[1].ID != internalPresetN1H264 || manual[2].ID != UserPresetC1 {
		t.Fatalf("manual retry chain = %+v", manual)
	}

	auto := BuildRetryChain(UserPresetAuto, caps)
	if len(auto) == 0 || auto[len(auto)-1].ID != UserPresetC1 {
		t.Fatalf("auto retry chain must end with c1: %+v", auto)
	}
}

func TestBuildRetryChain_ManualIncludesUserPresetEvenIfDetectionSaysUnavailable(t *testing.T) {
	caps := CapabilitiesFromEncoders([]string{"h264_nvenc", "libx264"})
	manual := BuildRetryChain(UserPresetN1, caps)
	if len(manual) != 3 {
		t.Fatalf("manual retry len=%d want 3", len(manual))
	}
	if manual[0].ID != UserPresetN1 || manual[1].ID != internalPresetN1H264 || manual[2].ID != UserPresetC1 {
		t.Fatalf("manual retry chain = %+v", manual)
	}
}

func TestBuildEditEncodeArgs(t *testing.T) {
	args, err := BuildEditEncodeArgs(UserPresetN1, EditQualityHigh)
	if err != nil {
		t.Fatalf("BuildEditEncodeArgs: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "hevc_nvenc") {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestBuildRecordingEncodeArgs_MapsQualityForSoftwareAndHardwareProfiles(t *testing.T) {
	tests := []struct {
		name      string
		profileID string
		quality   string
		want      string
	}{
		{name: "c1 standard", profileID: UserPresetC1, quality: EditQualityStandard, want: "-crf 10"},
		{name: "c1 high keeps existing default", profileID: UserPresetC1, quality: EditQualityHigh, want: "-crf 4"},
		{name: "c1 ultra", profileID: UserPresetC1, quality: EditQualityUltra, want: "-crf 2"},
		{name: "n1 standard", profileID: UserPresetN1, quality: EditQualityStandard, want: "-qp 20"},
		{name: "n1 high keeps existing default", profileID: UserPresetN1, quality: EditQualityHigh, want: "-qp 14"},
		{name: "n1 ultra", profileID: UserPresetN1, quality: EditQualityUltra, want: "-qp 10"},
		{name: "a1 standard", profileID: UserPresetA1, quality: EditQualityStandard, want: "-qp 20"},
		{name: "a1 high keeps existing default", profileID: UserPresetA1, quality: EditQualityHigh, want: "-qp 12"},
		{name: "a1 ultra", profileID: UserPresetA1, quality: EditQualityUltra, want: "-qp 8"},
		{name: "i1 standard", profileID: UserPresetI1, quality: EditQualityStandard, want: "-q:v 20"},
		{name: "i1 high keeps existing default", profileID: UserPresetI1, quality: EditQualityHigh, want: "-q:v 12"},
		{name: "i1 ultra", profileID: UserPresetI1, quality: EditQualityUltra, want: "-q:v 8"},
		{name: "n1 h264 standard", profileID: internalPresetN1H264, quality: EditQualityStandard, want: "-qp 22"},
		{name: "n1 h264 high keeps existing default", profileID: internalPresetN1H264, quality: EditQualityHigh, want: "-qp 16"},
		{name: "n1 h264 ultra", profileID: internalPresetN1H264, quality: EditQualityUltra, want: "-qp 12"},
		{name: "a1 h264 standard", profileID: internalPresetA1H264, quality: EditQualityStandard, want: "-qp 22"},
		{name: "a1 h264 high keeps existing default", profileID: internalPresetA1H264, quality: EditQualityHigh, want: "-qp 14"},
		{name: "a1 h264 ultra", profileID: internalPresetA1H264, quality: EditQualityUltra, want: "-qp 10"},
		{name: "i1 h264 standard", profileID: internalPresetI1H264, quality: EditQualityStandard, want: "-q:v 22"},
		{name: "i1 h264 high keeps existing default", profileID: internalPresetI1H264, quality: EditQualityHigh, want: "-q:v 14"},
		{name: "i1 h264 ultra", profileID: internalPresetI1H264, quality: EditQualityUltra, want: "-q:v 10"},
		{name: "invalid quality falls back to high", profileID: UserPresetN1, quality: "bad", want: "-qp 14"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildRecordingEncodeArgs(tt.profileID, tt.quality)
			if err != nil {
				t.Fatalf("BuildRecordingEncodeArgs: %v", err)
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("recording args should contain %q, got %q", tt.want, got)
			}
		})
	}
}

func TestBuildRecordingEncodeArgs_HighMatchesExistingHLAEParams(t *testing.T) {
	for _, profileID := range []string{
		UserPresetC1,
		UserPresetN1,
		UserPresetA1,
		UserPresetI1,
		internalPresetN1H264,
		internalPresetA1H264,
		internalPresetI1H264,
	} {
		t.Run(profileID, func(t *testing.T) {
			profile, ok := ProfileByID(profileID)
			if !ok {
				t.Fatalf("missing profile %q", profileID)
			}
			got, err := BuildRecordingEncodeArgs(profileID, EditQualityHigh)
			if err != nil {
				t.Fatalf("BuildRecordingEncodeArgs: %v", err)
			}
			if got != profile.HLAEParams {
				t.Fatalf("high recording args mismatch:\n got: %q\nwant: %q", got, profile.HLAEParams)
			}
		})
	}
}

func TestDetectCapabilities_UsesProbeResults(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(exe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write exe: %v", err)
	}
	cmdFactory := func(ctx context.Context, command string, args ...string) *exec.Cmd {
		all := append([]string{"-test.run=TestHelperProcessDetect", "--", command}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], all...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS_FFMPEGPROFILE=1",
			"FFMPEG_PROFILE_AVAILABLE=hevc_nvenc,h264_nvenc,libx264",
		)
		return cmd
	}
	caps, err := DetectCapabilities(context.Background(), exe, cmdFactory)
	if err != nil {
		t.Fatalf("DetectCapabilities: %v", err)
	}
	if !caps.HasEncoder("hevc_nvenc") {
		t.Fatalf("expected hevc_nvenc available: %+v", caps.Encoders)
	}
	if caps.HasEncoder("hevc_amf") {
		t.Fatalf("expected hevc_amf unavailable: %+v", caps.Encoders)
	}
	if _, ok := caps.Errors["hevc_amf"]; !ok {
		t.Fatalf("expected error for hevc_amf")
	}
}

func TestHelperProcessDetect(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_FFMPEGPROFILE") != "1" {
		return
	}
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	args := os.Args
	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep < 0 || sep+1 >= len(args) {
		_, _ = fmt.Fprintln(os.Stderr, "missing separator")
		os.Exit(2)
	}
	ffArgs := args[sep+2:]
	encoder := ""
	for i := 0; i < len(ffArgs)-1; i++ {
		if ffArgs[i] == "-c:v" {
			encoder = strings.ToLower(strings.TrimSpace(ffArgs[i+1]))
			break
		}
	}
	if encoder == "" {
		_, _ = fmt.Fprintln(os.Stderr, "missing encoder")
		os.Exit(2)
	}
	allowed := make(map[string]struct{})
	for _, token := range strings.Split(os.Getenv("FFMPEG_PROFILE_AVAILABLE"), ",") {
		normalized := strings.ToLower(strings.TrimSpace(token))
		if normalized == "" {
			continue
		}
		allowed[normalized] = struct{}{}
	}
	if _, ok := allowed[encoder]; ok {
		os.Exit(0)
	}
	_, _ = fmt.Fprintf(os.Stderr, "encoder not available: %s\n", encoder)
	os.Exit(2)
}
