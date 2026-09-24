package glance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestARRWidgetInitialize(t *testing.T) {
	widget := &arrWidget{Service: "radarr", Server: "https://radarr.example.com/", APIKey: "secret"}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if widget.Server != "https://radarr.example.com" || widget.View != "upcoming" || widget.Days != 14 || widget.Limit != 10 {
		t.Fatalf("widget = %#v", widget)
	}
}
func TestARRWidgetInitializeRejectsInvalidConfiguration(t *testing.T) {
	for _, widget := range []*arrWidget{{Service: "", Server: "https://x", APIKey: "x"}, {Service: "radarr", Server: "relative", APIKey: "x"}, {Service: "radarr", Server: "https://x", APIKey: ""}, {Service: "radarr", Server: "https://x", APIKey: "x", View: "other"}} {
		if err := widget.initialize(); err == nil {
			t.Fatalf("expected failure for %#v", widget)
		}
	}
}
func TestFetchARRUsesAPIKeyAndPreservesStatusError(t *testing.T) {
	var status = http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "secret" {
			t.Fatalf("api key = %q", r.Header.Get("X-Api-Key"))
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"title":"Example Movie","year":2026,"monitored":true,"hasFile":false,"digitalRelease":"2026-10-01T00:00:00Z"}]`))
	}))
	defer server.Close()
	widget := &arrWidget{Service: "radarr", Server: server.URL, APIKey: "secret", View: "upcoming", Days: 14, Limit: 10}
	items, err := fetchARR(context.Background(), widget)
	if err != nil || len(items) != 1 || items[0].Title != "Example Movie" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	status = http.StatusBadGateway
	_, err = fetchARR(context.Background(), widget)
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("error=%T %v", err, err)
	}
}
func TestARRProviderVersions(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	for service, want := range map[string]string{"radarr": "/api/v3/calendar", "sonarr": "/api/v3/calendar", "lidarr": "/api/v1/calendar"} {
		widget := &arrWidget{Service: service, Server: "https://arr.example", View: "upcoming", Days: 14}
		got, _ := arrEndpoint(widget, now)
		if !strings.Contains(got, want) {
			t.Fatalf("%s endpoint=%q", service, got)
		}
	}
}
func TestARRImageUsesResourceProxyAndDoesNotExposeAPIKey(t *testing.T) {
	widget := &arrWidget{APIKey: "secret-token"}
	widget.Providers = &widgetProviders{resourceProxyURL: func(raw string) (string, error) {
		if raw != "https://images.example/poster.jpg" {
			t.Fatalf("raw=%q", raw)
		}
		return "/api/resource-proxy/opaque", nil
	}}
	if got := widget.proxyARRImage("https://images.example/poster.jpg"); got != "/api/resource-proxy/opaque" {
		t.Fatalf("got=%q", got)
	}
	widget.Items = []arrItem{{Title: "Example", ImageURL: "/api/resource-proxy/opaque"}}
	widget.withTitle("ARR")
	html := string(widget.Render())
	if strings.Contains(html, "secret-token") || strings.Contains(html, "images.example") {
		t.Fatalf("render leaked secret/source: %s", html)
	}
}
func TestNormalizeARRMoviePreservesReleaseDates(t *testing.T) {
	item := normalizeARRMovie(arrMovie{
		Title:           "The Cable Guy",
		Year:            1996,
		Monitored:       true,
		HasFile:         true,
		InCinemas:       time.Date(1996, 6, 10, 0, 0, 0, 0, time.UTC),
		DigitalRelease:  time.Date(2002, 4, 8, 0, 0, 0, 0, time.UTC),
		PhysicalRelease: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC),
	}, time.Date(2002, 4, 8, 0, 0, 0, 0, time.UTC))

	want := []arrDate{
		{Label: "Cinema", Value: "Jun 10, 1996"},
		{Label: "Digital", Value: "Apr 8, 2002"},
		{Label: "Physical", Value: "Oct 6, 2026"},
	}
	if len(item.Dates) != len(want) {
		t.Fatalf("dates=%#v", item.Dates)
	}
	for i := range want {
		if item.Dates[i] != want[i] {
			t.Fatalf("dates[%d]=%#v want %#v", i, item.Dates[i], want[i])
		}
	}
}

func TestFormatARRDateDoesNotShiftCalendarDate(t *testing.T) {
	value := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	if got := formatARRDate(value); got != "Oct 6, 2026" {
		t.Fatalf("date=%q", got)
	}
}

func TestNormalizeARREpisode(t *testing.T) {
	item := normalizeARREpisode(arrEpisode{Title: "Pilot", SeasonNumber: 1, EpisodeNumber: 2, Monitored: true, Series: arrSeries{Title: "Example Show"}}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if item.Title != "Example Show" || item.Subtitle != "S01E02 · Pilot" || item.Status != "Missing" {
		t.Fatalf("item=%#v", item)
	}
}
