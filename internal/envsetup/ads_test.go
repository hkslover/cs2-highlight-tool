package envsetup

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"cs2-highlight-tool-v2/internal/release"
)

func TestRunStartupChecks_PopulatesSupportedAdsIntoState(t *testing.T) {
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
					"latest":{"tag_name":"v2.0.0","assets":[{"name":"hlae_2_0_0.zip","url":"https://dl.example.com/hlae.zip"}]}
				}
			},
			"ads":{
				"version":"2.0",
				"items":[
					{
						"id":"main_banner",
						"enabled":true,
						"placement":"main_steps_top_banner",
						"click_url":"https://ad.example.com/main",
						"sponsor":"Acme Sponsor",
						"title":"Main sponsored card",
						"rich_html":"<p>main card content</p>",
						"image_url":"https://cdn.example.com/main.jpg",
						"image_alt":"Main Banner"
					},
					{
						"id":"main_popup",
						"enabled":true,
						"placement":"main_entry_popup",
						"click_url":"https://ad.example.com/popup",
						"image_url":"https://cdn.example.com/popup.jpg",
						"image_alt":"Main Popup"
					},
					{
						"id":"ignored_banner",
						"enabled":true,
						"placement":"import_methods_card",
						"click_url":"https://ad.example.com/ignored",
						"sponsor":"Ignored",
						"title":"Ignored card",
						"rich_html":"<p>ignored</p>",
						"image_url":"https://cdn.example.com/ignored.jpg"
					}
				]
			}
		}`))
	}))
	defer server.Close()

	t.Setenv("CS2_RELEASE_API_URL", server.URL)

	exeDir := t.TempDir()
	svc := New(exeDir, "1.0.0")
	svc.Startup(nil)
	svc.runTasksFn = func(source DownloadSource) {
		markAllReadyForTest(svc)
	}

	state := svc.RunStartupChecks()
	if len(state.Ads) != 2 {
		t.Fatalf("ads count = %d, want 2", len(state.Ads))
	}
	ad := state.Ads[0]
	if ad.ID != "main_banner" {
		t.Fatalf("ad[0] id = %q", ad.ID)
	}
	if ad.Placement != release.AdPlacementMainStepsTopBanner {
		t.Fatalf("ad[0] placement = %q", ad.Placement)
	}
	popup := state.Ads[1]
	if popup.ID != "main_popup" {
		t.Fatalf("ad[1] id = %q", popup.ID)
	}
	if popup.Placement != release.AdPlacementMainEntryPopup {
		t.Fatalf("ad[1] placement = %q", popup.Placement)
	}
}

func TestRunStartupChecks_UsesDebugStartupAdsWhenEnabled(t *testing.T) {
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
					"latest":{"tag_name":"v2.0.0","assets":[{"name":"hlae_2_0_0.zip","url":"https://dl.example.com/hlae.zip"}]}
				}
			},
			"ads":{
				"version":"2.0",
				"items":[
					{
						"id":"api_card_should_be_ignored",
						"enabled":true,
						"placement":"main_steps_top_banner",
						"click_url":"https://ad.example.com/main",
						"sponsor":"API Sponsor",
						"title":"API Card",
						"rich_html":"<p>api content</p>",
						"image_url":"https://cdn.example.com/main.jpg"
					}
				]
			}
		}`))
	}))
	defer server.Close()

	t.Setenv("CS2_RELEASE_API_URL", server.URL)
	t.Setenv(debugStartupAdsEnv, "1")

	exeDir := t.TempDir()
	svc := New(exeDir, "1.0.0")
	svc.Startup(nil)
	svc.runTasksFn = func(source DownloadSource) {
		markAllReadyForTest(svc)
	}

	state := svc.RunStartupChecks()
	want := debugStartupAds()
	if len(state.Ads) != len(want) {
		t.Fatalf("ads count = %d, want %d", len(state.Ads), len(want))
	}
	for i := range want {
		if state.Ads[i].ID != want[i].ID {
			t.Fatalf("ad[%d].id = %q, want %q", i, state.Ads[i].ID, want[i].ID)
		}
		if state.Ads[i].Placement != want[i].Placement {
			t.Fatalf("ad[%d].placement = %q, want %q", i, state.Ads[i].Placement, want[i].Placement)
		}
		if state.Ads[i].ClickURL != want[i].ClickURL {
			t.Fatalf("ad[%d].click_url = %q, want %q", i, state.Ads[i].ClickURL, want[i].ClickURL)
		}
		if state.Ads[i].ImageURL != want[i].ImageURL {
			t.Fatalf("ad[%d].image_url = %q, want %q", i, state.Ads[i].ImageURL, want[i].ImageURL)
		}
	}
}

func TestNormalizeExternalOpenURL_AllowsMailto(t *testing.T) {
	got, ok := normalizeExternalOpenURL("mailto:hk_snow@yeah.net")
	if !ok {
		t.Fatalf("mailto should be accepted")
	}
	if got != "mailto:hk_snow@yeah.net" {
		t.Fatalf("normalized mailto = %q", got)
	}

	if _, ok := normalizeExternalOpenURL("javascript:alert(1)"); ok {
		t.Fatalf("javascript scheme must be rejected")
	}
}
