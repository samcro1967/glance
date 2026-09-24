package glance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSeerrWidgetInitialize(t *testing.T) {
	widget := &seerrWidget{Server: "https://seerr.example.com/", APIKey: "secret"}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if widget.Server != "https://seerr.example.com" || widget.View != "trending" || widget.Limit != 10 {
		t.Fatalf("widget = %#v", widget)
	}
}

func TestSeerrWidgetInitializeRejectsInvalidConfiguration(t *testing.T) {
	for _, widget := range []*seerrWidget{
		{Server: "relative", APIKey: "x"},
		{Server: "https://x", APIKey: ""},
		{Server: "https://x", APIKey: "x", View: "other"},
		{Server: "https://x?secret=yes", APIKey: "x"},
	} {
		if err := widget.initialize(); err == nil {
			t.Fatalf("expected failure for %#v", widget)
		}
	}
}

func TestFetchSeerrUsesAPIKeyAndPreservesStatusError(t *testing.T) {
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "secret" {
			t.Fatalf("api key = %q", r.Header.Get("X-Api-Key"))
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":101,"mediaType":"movie","title":"Northbound","overview":"A fixture movie.","posterPath":"/northbound.jpg","releaseDate":"2026-10-02"}]}`))
	}))
	defer server.Close()

	widget := &seerrWidget{Server: server.URL, APIKey: "secret", View: "trending", Limit: 10}
	items, err := fetchSeerr(context.Background(), widget)
	if err != nil || len(items) != 1 || items[0].Title != "Northbound" {
		t.Fatalf("items=%#v err=%v", items, err)
	}

	status = http.StatusBadGateway
	_, err = fetchSeerr(context.Background(), widget)
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("error=%T %v", err, err)
	}
}

func TestSeerrEndpoints(t *testing.T) {
	wants := map[string]string{
		"trending":        "/api/v1/discover/trending?page=1",
		"movies":          "/api/v1/discover/movies?page=1",
		"tv":              "/api/v1/discover/tv?page=1",
		"upcoming-movies": "/api/v1/discover/movies/upcoming?page=1",
		"upcoming-tv":     "/api/v1/discover/tv/upcoming?page=1",
		"requests":        "/api/v1/request?filter=all&skip=0&sort=added&take=10",
		"recently-added":  "/api/v1/media?filter=allavailable&skip=0&sort=mediaAdded&take=10",
		"watchlist":       "/api/v1/discover/watchlist",
	}
	for view, want := range wants {
		widget := &seerrWidget{Server: "https://seerr.example", View: view, Limit: 10}
		got, err := seerrEndpoint(widget)
		if err != nil || !strings.HasSuffix(got, want) {
			t.Fatalf("%s endpoint=%q err=%v want suffix %q", view, got, err, want)
		}
	}
}

func TestDecodeSeerrFiltersNonMediaTrendingResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":1,"mediaType":"person","name":"Person"},{"id":2,"mediaType":"tv","name":"Signal Lost","firstAirDate":"2026-09-29"}]}`))
	}))
	defer server.Close()
	httpResponse, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = httpResponse.Body.Close() }()
	items, err := decodeSeerrResponse(httpResponse, "trending")
	if err != nil || len(items) != 1 || items[0].Title != "Signal Lost" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestSeerrImageUsesResourceProxyAndDoesNotExposeAPIKey(t *testing.T) {
	widget := &seerrWidget{APIKey: "secret-token"}
	widget.Providers = &widgetProviders{resourceProxyURL: func(raw string) (string, error) {
		if raw != "https://image.tmdb.org/t/p/w500/poster.jpg" {
			t.Fatalf("raw=%q", raw)
		}
		return "/api/resource-proxy/opaque", nil
	}}
	if got := widget.proxySeerrImage(seerrPosterURL("/poster.jpg")); got != "/api/resource-proxy/opaque" {
		t.Fatalf("got=%q", got)
	}
	widget.Items = []seerrItem{{Title: "Example", ImageURL: "/api/resource-proxy/opaque"}}
	widget.withTitle("Seerr")
	html := string(widget.Render())
	if strings.Contains(html, "secret-token") || strings.Contains(html, "image.tmdb.org") {
		t.Fatalf("render leaked secret/source: %s", html)
	}
}

func TestNormalizeSeerrResult(t *testing.T) {
	item := normalizeSeerrResult(seerrMediaResult{ID: 10, MediaType: "tv", Name: "Signal Lost", Overview: "Fixture", PosterPath: "/signal.jpg", FirstAirDate: "2026-09-29"})
	if item.Title != "Signal Lost" || item.Subtitle != "2026" || item.Date != "Sep 29, 2026" || item.MediaType != "tv" || item.TMDBID != 10 {
		t.Fatalf("item=%#v", item)
	}
}
