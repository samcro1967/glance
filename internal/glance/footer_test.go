package glance

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

const (
	testInstalledForkVersion = "v1.2.3-samcro1967.r004"
	testLatestForkVersion    = "v1.2.3-samcro1967.r005"
	testLatestReleaseURL     = "https://example.invalid/releases/r005"
)

func renderFooterForTest(t *testing.T, app *application) string {
	t.Helper()

	var rendered bytes.Buffer
	err := pageTemplate.ExecuteTemplate(
		&rendered,
		"footer.html",
		templateData{
			App: app,
		},
	)
	if err != nil {
		t.Fatalf("render footer: %v", err)
	}

	return rendered.String()
}

func TestFooterReleaseStatusDev(t *testing.T) {
	app := &application{
		Version: "dev",
	}

	rendered := renderFooterForTest(t, app)

	if !strings.Contains(rendered, ">Glance</a>") ||
		!strings.Contains(rendered, "(dev)") {
		t.Fatalf("footer does not contain development version: %q", rendered)
	}

	if strings.Contains(rendered, "Latest") {
		t.Fatalf("development footer contains latest status: %q", rendered)
	}

	if strings.Contains(rendered, "Update available") {
		t.Fatalf("development footer contains update status: %q", rendered)
	}
}

func TestFooterReleaseStatusDevWithRevision(t *testing.T) {
	app := &application{
		Version:       "dev",
		ShortRevision: "8534072",
	}

	rendered := renderFooterForTest(t, app)

	if !strings.Contains(rendered, "(dev 8534072)") {
		t.Fatalf("footer does not contain development revision: %q", rendered)
	}

	if strings.Contains(rendered, "Latest") {
		t.Fatalf("development footer contains latest status: %q", rendered)
	}

	if strings.Contains(rendered, "Update available") {
		t.Fatalf("development footer contains update status: %q", rendered)
	}
}

func TestFooterReleaseStatusLatest(t *testing.T) {
	app := &application{
		Version: testInstalledForkVersion,
	}

	app.releaseStatus.set(releaseStatusResult{
		Status:        releaseStatusLatest,
		LatestVersion: testInstalledForkVersion,
		ReleaseURL:    testLatestReleaseURL,
	})

	rendered := renderFooterForTest(t, app)

	if !strings.Contains(rendered, testInstalledForkVersion) ||
		!strings.Contains(rendered, `· <span class="color-positive">Latest</span>`) {
		t.Fatalf("footer does not contain latest status: %q", rendered)
	}

	if strings.Contains(rendered, "Update available") {
		t.Fatalf("latest footer contains update status: %q", rendered)
	}
}

func TestFooterReleaseStatusUpdateAvailable(t *testing.T) {
	app := &application{
		Version: testInstalledForkVersion,
	}

	app.releaseStatus.set(releaseStatusResult{
		Status:        releaseStatusUpdateAvailable,
		LatestVersion: testLatestForkVersion,
		ReleaseURL:    testLatestReleaseURL,
	})

	rendered := renderFooterForTest(t, app)

	if !strings.Contains(rendered, testInstalledForkVersion) {
		t.Fatalf("footer does not contain installed version: %q", rendered)
	}

	want := `· <a class="color-primary" href="` +
		testLatestReleaseURL +
		`" target="_blank" rel="noreferrer">Update available</a>`

	if !strings.Contains(rendered, want) {
		t.Fatalf("footer does not contain linked update status: %q", rendered)
	}

	if strings.Contains(rendered, ">Latest<") {
		t.Fatalf("update footer contains latest status: %q", rendered)
	}
}

func TestFooterReleaseStatusUnknown(t *testing.T) {
	app := &application{
		Version: testInstalledForkVersion,
	}

	rendered := renderFooterForTest(t, app)

	if !strings.Contains(rendered, testInstalledForkVersion) {
		t.Fatalf("footer does not contain installed version: %q", rendered)
	}

	if strings.Contains(rendered, "Latest") {
		t.Fatalf("unknown footer contains latest status: %q", rendered)
	}

	if strings.Contains(rendered, "Update available") {
		t.Fatalf("unknown footer contains update status: %q", rendered)
	}
}

func TestFooterReleaseStatusCustomFooterUnaffected(t *testing.T) {
	app := &application{
		Version: testInstalledForkVersion,
	}
	app.Config.Branding.CustomFooter = "Custom footer"

	app.releaseStatus.set(releaseStatusResult{
		Status: releaseStatusUpdateAvailable,
	})

	rendered := renderFooterForTest(t, app)

	if !strings.Contains(rendered, "Custom footer") {
		t.Fatalf("custom footer missing: %q", rendered)
	}

	if strings.Contains(rendered, "Update available") {
		t.Fatalf("custom footer contains release status: %q", rendered)
	}
}

func TestFooterReleaseStatusHiddenFooterUnaffected(t *testing.T) {
	app := &application{
		Version: testInstalledForkVersion,
	}
	app.Config.Branding.HideFooter = true

	rendered := renderFooterForTest(t, app)

	if strings.TrimSpace(rendered) != "" {
		t.Fatalf("hidden footer rendered content: %q", rendered)
	}
}

func renderFooterWithPageForTest(t *testing.T, app *application, currentPage *page) string {
	t.Helper()

	var rendered bytes.Buffer
	err := pageTemplate.ExecuteTemplate(
		&rendered,
		"footer.html",
		templateData{
			App:  app,
			Page: currentPage,
		},
	)
	if err != nil {
		t.Fatalf("failed to render footer: %v", err)
	}

	return rendered.String()
}

func TestFooterMicroWidgetsRender(t *testing.T) {
	app := &application{
		Version: "dev",
		Config: config{
			FooterMicroWidgets: footerMicroWidgets{
				Left: microWidgets{
					&microBookmark{
						Position: 1,
						Title:    "GitHub",
						URL:      "https://example.com/github",
						Icon:     newCustomIconField("si:github"),
					},
				},
				Right: microWidgets{
					&microClock{
						Position:   1,
						HourFormat: "12h",
						Timezone:   "America/New_York",
						Label:      "NY",
					},
					&microBookmark{
						Position: 2,
						Title:    "Same Tab",
						URL:      "https://example.com/same",
						SameTab:  true,
					},
				},
			},
		},
	}

	rendered := renderFooterWithPageForTest(t, app, &page{})

	checks := []string{
		`class="footer-micro-layer"`,
		`class="footer-micro-group footer-micro-group-left"`,
		`class="footer-micro-group footer-micro-group-right"`,
		`class="footer-micro-icon flat-icon"`,
		`href="https://example.com/github" target="_blank" rel="noreferrer"`,
		`class="footer-micro-item footer-micro-clock"`,
		`data-hour-format="12h"`,
		`data-timezone="America/New_York"`,
		`class="footer-micro-clock-context"`,
		`>NY · </span>`,
		`data-micro-clock-date`,
		`class="footer-micro-clock-time" data-micro-clock-time`,
		`href="https://example.com/same"`,
	}

	for _, want := range checks {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered footer missing %q:\n%s", want, rendered)
		}
	}

	if strings.Contains(rendered, `href="https://example.com/same" target="_blank"`) {
		t.Errorf("same-tab bookmark unexpectedly rendered target=_blank:\n%s", rendered)
	}
}

func TestFooterMicroWeatherAndMarketsNavigationDefaults(t *testing.T) {
	weatherMicro := &microWeather{
		widgetBase: widgetBase{Type: "weather"},
		Position:   1,
		Units:      "imperial",
		Place:      &openMeteoPlaceResponseJson{Name: "St. Louis"},
		Weather:    &weather{Temperature: 72, WeatherCode: 0},
	}
	weatherMicro.setID(201)

	marketsMicro := &microMarkets{
		widgetBase: widgetBase{Type: "markets"},
		Position:   2,
		Markets: marketList{
			{marketRequest: marketRequest{Symbol: "SPY"}, PercentChange: 1.25},
		},
	}
	marketsMicro.setID(202)

	app := &application{
		Version: "dev",
		Config: config{
			FooterMicroWidgets: footerMicroWidgets{
				Left: microWidgets{weatherMicro, marketsMicro},
			},
		},
	}

	rendered := renderFooterWithPageForTest(t, app, &page{})

	if strings.Contains(rendered, `href="https://weather.example/`) {
		t.Errorf("weather without URL unexpectedly rendered as link:\n%s", rendered)
	}
	if strings.Contains(rendered, `href="https://quote.example/`) {
		t.Errorf("market without SymbolLink unexpectedly rendered as link:\n%s", rendered)
	}
	if !strings.Contains(rendered, `class="footer-micro-item footer-micro-weather"`) {
		t.Errorf("unlinked weather markup missing:\n%s", rendered)
	}
	if !strings.Contains(rendered, `class="footer-micro-market"`) {
		t.Errorf("unlinked market markup missing:\n%s", rendered)
	}
}

func TestFooterMicroWidgetsRequirePage(t *testing.T) {
	app := &application{
		Version: "dev",
		Config: config{
			FooterMicroWidgets: footerMicroWidgets{
				Left: microWidgets{
					&microBookmark{
						Position: 1,
						Title:    "Hidden",
						URL:      "https://example.com",
					},
				},
			},
		},
	}

	rendered := renderFooterWithPageForTest(t, app, nil)

	if strings.Contains(rendered, "footer-micro-layer") {
		t.Errorf("footer micro layer rendered without page:\n%s", rendered)
	}
}

func TestFooterMicroWidgetsHiddenWithFooter(t *testing.T) {
	app := &application{
		Version: "dev",
		Config: config{
			FooterMicroWidgets: footerMicroWidgets{
				Left: microWidgets{
					&microBookmark{
						Position: 1,
						Title:    "Hidden",
						URL:      "https://example.com",
					},
				},
			},
		},
	}
	app.Config.Branding.HideFooter = true

	rendered := renderFooterWithPageForTest(t, app, &page{})

	if strings.TrimSpace(rendered) != "" {
		t.Errorf("hidden footer rendered content:\n%s", rendered)
	}
}

func TestFooterMicroWidgetsWithCustomFooter(t *testing.T) {
	app := &application{
		Version: "dev",
		Config: config{
			FooterMicroWidgets: footerMicroWidgets{
				Left: microWidgets{
					&microBookmark{
						Position: 1,
						Title:    "Micro",
						URL:      "https://example.com",
					},
				},
			},
		},
	}
	app.Config.Branding.CustomFooter = template.HTML("<strong>Custom Center</strong>")

	rendered := renderFooterWithPageForTest(t, app, &page{})

	if !strings.Contains(rendered, "<strong>Custom Center</strong>") {
		t.Errorf("custom footer missing:\n%s", rendered)
	}
	if !strings.Contains(rendered, "footer-micro-layer") {
		t.Errorf("micro layer missing with custom footer:\n%s", rendered)
	}
}

func TestFooterWithoutMicroWidgetsUnchanged(t *testing.T) {
	app := &application{
		Version: "dev",
		Config:  config{},
	}

	rendered := renderFooterWithPageForTest(t, app, &page{})

	if strings.Contains(rendered, "footer-micro-layer") {
		t.Errorf("micro layer rendered without configured micro-widgets:\n%s", rendered)
	}
}

func TestFooterMicroWidgetsRenderAllSupportedTypes(t *testing.T) {
	weatherMicro := &microWeather{
		widgetBase:   widgetBase{Type: "weather"},
		Position:     2,
		Units:        "imperial",
		ShowAreaName: true,
		URL:          "https://weather.example/st-louis",
		SameTab:      true,
		Place:        &openMeteoPlaceResponseJson{Name: "St. Louis", Area: "Missouri"},
		Weather:      &weather{Temperature: 72, WeatherCode: 0},
	}
	weatherMicro.setID(101)

	marketsMicro := &microMarkets{
		widgetBase: widgetBase{Type: "markets"},
		Position:   3,
		Markets: marketList{
			{marketRequest: marketRequest{Symbol: "SPY", SymbolLink: "https://quote.example/SPY"}, PercentChange: 1.25},
		},
	}
	marketsMicro.setID(102)

	monitorMicro := &microMonitor{
		widgetBase: widgetBase{Type: "monitor"},
		Position:   2,
		Sites: []monitorSite{
			{
				SiteStatusRequest: &SiteStatusRequest{DefaultURL: "https://example.com/status"},
				URL:               "https://example.com/status",
				Title:             "Example Status",
				StatusText:        "OK",
				StatusStyle:       "ok",
			},
		},
	}
	monitorMicro.setID(103)

	app := &application{
		Version: "dev",
		Config: config{
			FooterMicroWidgets: footerMicroWidgets{
				Left: microWidgets{
					&microBookmark{Position: 1, Title: "Bookmark Marker", URL: "https://example.com/bookmark"},
					weatherMicro,
					marketsMicro,
				},
				Right: microWidgets{
					&microClock{Position: 1, HourFormat: "24h", Timezone: "UTC", Label: "UTC"},
					monitorMicro,
					&microLink{Position: 3, Title: "Link Marker", URL: "https://example.com/link"},
				},
			},
		},
	}

	rendered := renderFooterWithPageForTest(t, app, &page{})

	checks := []string{
		`href="https://example.com/bookmark"`,
		`Bookmark Marker`,
		`footer-micro-weather`,
		`data-widget-id="101"`,
		`St. Louis, Missouri`,
		`72°F`,
		`href="https://weather.example/st-louis"`,
		`footer-micro-markets`,
		`data-widget-id="102"`,
		`SPY`,
		`&#43;1.25%`,
		`href="https://quote.example/SPY" target="_blank" rel="noreferrer"`,
		`footer-micro-clock`,
		`data-timezone="UTC"`,
		`footer-micro-monitor`,
		`data-widget-id="103"`,
		`Example Status`,
		`footer-micro-status-ok`,
		`footer-micro-link`,
		`Link Marker`,
	}

	for _, want := range checks {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered footer missing %q:\n%s", want, rendered)
		}
	}

	if strings.Contains(rendered, `href="https://weather.example/st-louis" target="_blank"`) {
		t.Errorf("same-tab weather unexpectedly rendered target=_blank:\n%s", rendered)
	}
}
