package ffmpegprofile

import "strings"

type Resolution struct {
	RequestedPreset string
	SelectedProfile Profile
	TriedProfileIDs []string
}

// VideoPresetResolution describes the profile that will actually be used for
// a requested preset. The requested value remains "auto" when the user chose
// automatic selection; EffectivePreset/Encoder are only presentation and
// runtime-resolution metadata.
type VideoPresetResolution struct {
	RequestedPreset string
	EffectivePreset string
	Encoder         string
	Status          string
}

const (
	VideoPresetStatusManual  = "manual"
	VideoPresetStatusReady   = "ready"
	VideoPresetStatusPending = "pending"
)

// ResolveVideoPreset resolves the same effective profile used by recording and
// editing, while preserving the user's requested preset. A missing detection
// cache still returns the c1 runtime fallback, but marks it as pending so the
// UI does not present that fallback as a completed hardware detection.
func ResolveVideoPreset(userPreset string, detectedPreset string, detectedEncoders []string) VideoPresetResolution {
	requested := NormalizeUserPreset(userPreset)
	if requested != UserPresetAuto {
		profile, ok := ProfileByID(requested)
		if !ok {
			profile, _ = ProfileByID(UserPresetC1)
		}
		return VideoPresetResolution{
			RequestedPreset: requested,
			EffectivePreset: profile.ID,
			Encoder:         profile.Encoder,
			Status:          VideoPresetStatusManual,
		}
	}

	if profile, ok := ProfileByID(detectedPreset); ok && strings.TrimSpace(profile.Encoder) != "" {
		return VideoPresetResolution{
			RequestedPreset: requested,
			EffectivePreset: profile.ID,
			Encoder:         profile.Encoder,
			Status:          VideoPresetStatusReady,
		}
	}

	caps := CapabilitiesFromEncoders(detectedEncoders)
	if !caps.IsEmpty() {
		resolved := ResolveProfile(UserPresetAuto, caps)
		if strings.TrimSpace(resolved.SelectedProfile.ID) != "" {
			return VideoPresetResolution{
				RequestedPreset: requested,
				EffectivePreset: resolved.SelectedProfile.ID,
				Encoder:         resolved.SelectedProfile.Encoder,
				Status:          VideoPresetStatusReady,
			}
		}
	}

	fallback, ok := ProfileByID(UserPresetC1)
	if !ok {
		fallback = Profile{ID: UserPresetC1, Encoder: "libx264"}
	}
	return VideoPresetResolution{
		RequestedPreset: requested,
		EffectivePreset: fallback.ID,
		Encoder:         fallback.Encoder,
		Status:          VideoPresetStatusPending,
	}
}

func ResolveProfile(userPreset string, caps Capabilities) Resolution {
	normalized := NormalizeUserPreset(userPreset)
	order := ResolveOrder(normalized)
	tried := make([]string, 0, len(order))

	for _, id := range order {
		profile, ok := ProfileByID(id)
		if !ok {
			continue
		}
		tried = append(tried, profile.ID)
		if profile.Encoder == "" {
			continue
		}
		if caps.IsEmpty() {
			if normalized != UserPresetAuto {
				return Resolution{RequestedPreset: normalized, SelectedProfile: profile, TriedProfileIDs: tried}
			}
			continue
		}
		if caps.HasEncoder(profile.Encoder) {
			return Resolution{RequestedPreset: normalized, SelectedProfile: profile, TriedProfileIDs: tried}
		}
	}

	fallback, ok := ProfileByID(UserPresetC1)
	if !ok {
		fallback = Profile{ID: UserPresetC1, Encoder: "libx264"}
	}
	if !containsID(tried, fallback.ID) {
		tried = append(tried, fallback.ID)
	}
	return Resolution{RequestedPreset: normalized, SelectedProfile: fallback, TriedProfileIDs: tried}
}

func BuildRetryChain(userPreset string, caps Capabilities) []Profile {
	normalized := NormalizeUserPreset(userPreset)
	order := ResolveOrder(normalized)
	out := make([]Profile, 0, len(order))
	seen := make(map[string]struct{}, len(order))
	filterByCaps := normalized == UserPresetAuto && !caps.IsEmpty()

	for _, id := range order {
		profile, ok := ProfileByID(id)
		if !ok {
			continue
		}
		if _, exists := seen[profile.ID]; exists {
			continue
		}
		seen[profile.ID] = struct{}{}

		if filterByCaps && !caps.HasEncoder(profile.Encoder) {
			continue
		}
		out = append(out, profile)
	}

	if !containsProfileID(out, UserPresetC1) {
		if cpuFallback, ok := ProfileByID(UserPresetC1); ok {
			out = append(out, cpuFallback)
		}
	}
	return out
}

func containsProfileID(profiles []Profile, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, profile := range profiles {
		if strings.EqualFold(strings.TrimSpace(profile.ID), target) {
			return true
		}
	}
	return false
}

func containsID(ids []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, id := range ids {
		if strings.EqualFold(strings.TrimSpace(id), target) {
			return true
		}
	}
	return false
}
