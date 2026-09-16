package release

import (
	"net/mail"
	"net/url"
	"strconv"
	"strings"
)

const (
	AdPlacementMainStepsTopBanner = "main_steps_top_banner"
)

// AdsManifest is the ad block of the unified release manifest.
//
// Sponsored cards are image-only: image_url (remote http/https or a
// data:image/... URI) is rendered as the whole click target for click_url.
type AdsManifest struct {
	Version   string   `json:"version"`
	UpdatedAt string   `json:"updated_at"`
	Items     []AdItem `json:"items"`
}

type AdItem struct {
	ID        string `json:"id"`
	Enabled   bool   `json:"enabled"`
	Placement string `json:"placement"`
	ClickURL  string `json:"click_url"`
	ImageURL  string `json:"image_url"`
	ImageAlt  string `json:"image_alt,omitempty"`
}

type manifestAdsPayload struct {
	Version   string              `json:"version"`
	UpdatedAt string              `json:"updated_at"`
	Items     []manifestAdPayload `json:"items"`
}

// manifestAdPayload declares only the fields the client consumes. The upstream
// manifest keeps returning extra fields (sponsor/title/rich_html/...);
// encoding/json ignores undeclared keys, so they are dropped without rejecting
// the ad and without requiring any change on the release server.
type manifestAdPayload struct {
	ID        string `json:"id"`
	Enabled   bool   `json:"enabled"`
	Placement string `json:"placement"`
	ClickURL  string `json:"click_url"`
	ImageURL  string `json:"image_url"`
	ImageAlt  string `json:"image_alt"`
}

func parseManifestAds(payload manifestAdsPayload) (AdsManifest, []string) {
	out := AdsManifest{
		Version:   strings.TrimSpace(payload.Version),
		UpdatedAt: strings.TrimSpace(payload.UpdatedAt),
		Items:     make([]AdItem, 0, len(payload.Items)),
	}
	errors := make([]string, 0)
	for i, raw := range payload.Items {
		item, ok, reason := validateAndNormalizeAd(raw)
		if !ok {
			if strings.TrimSpace(reason) != "" {
				errors = append(errors, reasonWithIndex(i, raw.ID, reason))
			}
			continue
		}
		out.Items = append(out.Items, item)
	}
	return out, errors
}

func validateAndNormalizeAd(raw manifestAdPayload) (AdItem, bool, string) {
	if !raw.Enabled {
		return AdItem{}, false, ""
	}
	id := strings.TrimSpace(raw.ID)
	if id == "" {
		return AdItem{}, false, "missing id"
	}
	placement := strings.TrimSpace(raw.Placement)
	if placement != AdPlacementMainStepsTopBanner {
		return AdItem{}, false, "unsupported placement"
	}
	clickURL, ok := normalizeExternalLinkURL(raw.ClickURL)
	if !ok {
		return AdItem{}, false, "invalid click_url"
	}
	imageURL, ok := normalizeAdImageURL(raw.ImageURL)
	if !ok {
		return AdItem{}, false, "invalid image_url"
	}

	return AdItem{
		ID:        id,
		Enabled:   true,
		Placement: placement,
		ClickURL:  clickURL,
		ImageURL:  imageURL,
		ImageAlt:  strings.TrimSpace(raw.ImageAlt),
	}, true, ""
}

func reasonWithIndex(idx int, id, reason string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "ad[" + strconv.Itoa(idx) + "]: " + strings.TrimSpace(reason)
	}
	return "ad[" + strconv.Itoa(idx) + "](" + id + "): " + strings.TrimSpace(reason)
}

func normalizeAdImageURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	switch scheme {
	case "http", "https":
		if strings.TrimSpace(parsed.Host) == "" {
			return "", false
		}
		return parsed.String(), true
	case "data":
		if isDataImageURL(parsed.Opaque) {
			return parsed.String(), true
		}
		return "", false
	default:
		return "", false
	}
}

func isDataImageURL(opaque string) bool {
	opaque = strings.TrimSpace(opaque)
	if opaque == "" || strings.ContainsAny(opaque, "\r\n") {
		return false
	}
	comma := strings.Index(opaque, ",")
	if comma <= 0 {
		return false
	}
	mediaPart := strings.ToLower(strings.TrimSpace(opaque[:comma]))
	return strings.HasPrefix(mediaPart, "image/")
}

func normalizeExternalLinkURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	switch scheme {
	case "http", "https":
		if strings.TrimSpace(parsed.Host) == "" {
			return "", false
		}
		return parsed.String(), true
	case "mailto":
		return normalizeMailtoURL(parsed)
	default:
		return "", false
	}
}

func normalizeMailtoURL(parsed *url.URL) (string, bool) {
	if parsed == nil {
		return "", false
	}
	target := strings.TrimSpace(parsed.Opaque)
	if target == "" {
		target = strings.TrimSpace(parsed.Host)
	}
	if target == "" {
		target = strings.TrimSpace(strings.TrimPrefix(parsed.Path, "/"))
	}
	if target == "" || strings.ContainsAny(target, "\r\n") {
		return "", false
	}

	addressList := target
	if idx := strings.Index(addressList, "?"); idx >= 0 {
		addressList = addressList[:idx]
	}
	addressList = strings.TrimSpace(addressList)
	if addressList == "" {
		return "", false
	}
	for _, part := range strings.Split(addressList, ",") {
		addr := strings.TrimSpace(part)
		if addr == "" {
			return "", false
		}
		if _, err := mail.ParseAddress(addr); err != nil {
			return "", false
		}
	}

	return parsed.String(), true
}
