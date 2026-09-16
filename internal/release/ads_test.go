package release

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchUnifiedLatest_ParsesAndFiltersAds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"project":{"id":"cs2-highlight-tool-v2","name":"cs2-highlight-tool-v2"},
			"dependencies":{
				"advancedfx":{
					"name":"advancedfx",
					"repo":"advancedfx/advancedfx",
					"latest_tag":"v2.189.9",
					"latest":{"tag_name":"v2.189.9","assets":[{"name":"hlae.zip","url":"https://dl.example.com/hlae.zip"}]}
				}
			},
			"ads":{
				"version":"2.0",
				"updated_at":"2026-05-08T10:00:00Z",
				"items":[
					{
						"id":"valid_card_with_legacy_fields",
						"enabled":true,
						"placement":"main_steps_top_banner",
						"click_url":"https://ad.example.com/landing",
						"sponsor":"Legacy Sponsor",
						"title":"Legacy title",
						"rich_html":"<p>legacy <strong>body</strong> with <a href='https://example.com'>link</a></p>",
						"image_url":"data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='256' height='144'><rect width='100%25' height='100%25' fill='%230b3b2e'/></svg>",
						"image_alt":"img alt"
					},
					{
						"id":"image_only_card",
						"enabled":true,
						"placement":"main_steps_top_banner",
						"click_url":"https://ad.example.com/image-only",
						"image_url":"https://cdn.example.com/only.jpg"
					},
					{
						"id":"invalid_placement",
						"enabled":true,
						"placement":"import_methods_card",
						"click_url":"https://ad.example.com/x",
						"image_url":"https://cdn.example.com/x.jpg"
					},
					{
						"id":"invalid_click_url",
						"enabled":true,
						"placement":"main_steps_top_banner",
						"click_url":"javascript:alert(1)",
						"image_url":"https://cdn.example.com/y.jpg"
					},
					{
						"id":"invalid_image_url",
						"enabled":true,
						"placement":"main_steps_top_banner",
						"click_url":"https://ad.example.com/html",
						"image_url":"javascript:alert(2)"
					},
					{
						"id":"invalid_data_image_url",
						"enabled":true,
						"placement":"main_steps_top_banner",
						"click_url":"https://ad.example.com/html2",
						"image_url":"data:text/html,<p>bad</p>"
					},
					{
						"id":"missing_image_url",
						"enabled":true,
						"placement":"main_steps_top_banner",
						"click_url":"https://ad.example.com/legacy",
						"content":{"image_url":"https://cdn.example.com/legacy.jpg"}
					},
					{
						"id":"popup_card",
						"enabled":true,
						"placement":"main_entry_popup",
						"click_url":"https://ad.example.com/popup",
						"image_url":"https://cdn.example.com/popup.jpg",
						"image_alt":"Popup Ad"
					},
					{
						"id":"second_valid_card",
						"enabled":true,
						"placement":"main_steps_top_banner",
						"click_url":"https://ad.example.com/second",
						"image_url":"https://cdn.example.com/b.jpg"
					}
				]
			}
		}`))
	}))
	defer server.Close()

	latest, err := FetchUnifiedLatest(server.URL)
	if err != nil {
		t.Fatalf("FetchUnifiedLatest error: %v", err)
	}
	if latest.Ads.Version != "2.0" {
		t.Fatalf("ads version = %q", latest.Ads.Version)
	}
	if len(latest.Ads.Items) != 4 {
		t.Fatalf("ads item count = %d, want 4, errors=%v", len(latest.Ads.Items), latest.AdValidationErrors)
	}
	// Every rejected item must be reported; ads never fail silently.
	if len(latest.AdValidationErrors) != 5 {
		t.Fatalf("ad validation errors = %v, want 5", latest.AdValidationErrors)
	}

	wantIDs := []string{"valid_card_with_legacy_fields", "image_only_card", "popup_card", "second_valid_card"}
	for i, want := range wantIDs {
		if latest.Ads.Items[i].ID != want {
			t.Fatalf("ad[%d].id = %q, want %q", i, latest.Ads.Items[i].ID, want)
		}
		if !latest.Ads.Items[i].Enabled {
			t.Fatalf("ad[%d] should be enabled", i)
		}
	}
	if latest.Ads.Items[2].Placement != AdPlacementMainEntryPopup {
		t.Fatalf("popup placement = %q, want %q", latest.Ads.Items[2].Placement, AdPlacementMainEntryPopup)
	}

	// Image-only ads survive without any text field at all.
	if latest.Ads.Items[1].ImageURL != "https://cdn.example.com/only.jpg" {
		t.Fatalf("image-only image_url = %q", latest.Ads.Items[1].ImageURL)
	}
	if latest.Ads.Items[1].ImageAlt != "" {
		t.Fatalf("image-only image_alt = %q, want empty", latest.Ads.Items[1].ImageAlt)
	}
	if latest.Ads.Items[0].ImageAlt != "img alt" {
		t.Fatalf("ad image_alt = %q", latest.Ads.Items[0].ImageAlt)
	}
}

func TestFetchUnifiedLatest_AllowsMailtoForClickURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"project":{"id":"cs2-highlight-tool-v2","name":"cs2-highlight-tool-v2"},
			"dependencies":{
				"advancedfx":{"name":"advancedfx","repo":"advancedfx/advancedfx","latest":{"assets":[{"name":"hlae.zip","url":"https://dl.example.com/hlae.zip"}]}}
			},
			"ads":{
				"version":"2.0",
				"items":[
					{
						"id":"mailto_card",
						"enabled":true,
						"placement":"main_steps_top_banner",
						"click_url":"mailto:hk_snow@yeah.net",
						"sponsor":"",
						"title":"Mail Contact",
						"rich_html":"<a href='mailto:hk_snow@yeah.net' target='_blank'>联系我们</a>",
						"image_url":"data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='256' height='144'><rect width='100%25' height='100%25' fill='%230b3b2e'/></svg>"
					}
				]
			}
		}`))
	}))
	defer server.Close()

	latest, err := FetchUnifiedLatest(server.URL)
	if err != nil {
		t.Fatalf("FetchUnifiedLatest error: %v", err)
	}
	if len(latest.Ads.Items) != 1 {
		t.Fatalf("ads item count = %d, want 1, errors=%v", len(latest.Ads.Items), latest.AdValidationErrors)
	}
	if got := latest.Ads.Items[0].ClickURL; got != "mailto:hk_snow@yeah.net" {
		t.Fatalf("click_url = %q", got)
	}
}

func TestValidateAndNormalizeAd_IgnoresRetiredTextField(t *testing.T) {
	// The release server keeps returning sponsor/title/rich_html. They must be
	// ignored: an ad is valid as long as click_url and image_url are usable.
	raw := manifestAdPayload{
		ID:        "card",
		Enabled:   true,
		Placement: AdPlacementMainStepsTopBanner,
		ClickURL:  "https://ad.example.com/landing",
		ImageURL:  "https://cdn.example.com/card.jpg",
	}
	item, ok, reason := validateAndNormalizeAd(raw)
	if !ok {
		t.Fatalf("ad rejected: %s", reason)
	}
	if item.ClickURL != raw.ClickURL || item.ImageURL != raw.ImageURL {
		t.Fatalf("ad = %+v", item)
	}
}

func TestValidateAndNormalizeAd_RequiresImageURL(t *testing.T) {
	raw := manifestAdPayload{
		ID:        "card",
		Enabled:   true,
		Placement: AdPlacementMainStepsTopBanner,
		ClickURL:  "https://ad.example.com/landing",
	}
	if _, ok, reason := validateAndNormalizeAd(raw); ok || reason != "invalid image_url" {
		t.Fatalf("want invalid image_url rejection, got ok=%v reason=%q", ok, reason)
	}
}
