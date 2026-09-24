package glance

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLatestMediaWidgetInitialize(t *testing.T) {
	widget := &latestMediaWidget{Service: "PLEX", Server: "https://plex.example.com/", APIKey: "secret"}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if widget.Service != "plex" || widget.Server != "https://plex.example.com" || widget.Limit != 10 {
		t.Fatalf("widget=%#v", widget)
	}
}

func TestLatestMediaWidgetInitializeRejectsInvalidConfiguration(t *testing.T) {
	for _, widget := range []*latestMediaWidget{
		{Service: "other", Server: "https://x", APIKey: "x"},
		{Service: "plex", Server: "relative", APIKey: "x"},
		{Service: "plex", Server: "https://x", APIKey: ""},
		{Service: "navidrome", Server: "https://x", Username: "u"},
		{Service: "plex", Server: "https://x?token=yes", APIKey: "x"},
		{Service: "plex", Server: "https://x", APIKey: "x", Limit: -1},
	} {
		if err := widget.initialize(); err == nil {
			t.Fatalf("expected failure for %#v", widget)
		}
	}
}

func TestFetchLatestMediaPlexUsesTokenAndPreservesStatusError(t *testing.T) {
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "secret" {
			t.Fatalf("token=%q", r.Header.Get("X-Plex-Token"))
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"1","type":"movie","title":"Northbound","year":2026,"summary":"Fixture","addedAt":1790899200,"duration":7140000}]}}`))
	}))
	defer server.Close()
	widget := &latestMediaWidget{Service: "plex", Server: server.URL, APIKey: "secret", Limit: 10}
	items, err := fetchLatestMedia(context.Background(), widget)
	if err != nil || len(items) != 1 || items[0].Title != "Northbound" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	status = http.StatusBadGateway
	_, err = fetchLatestMedia(context.Background(), widget)
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("error=%T %v", err, err)
	}
}

func TestFetchLatestMediaJellyfinUsesToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "secret" {
			t.Fatalf("token=%q", r.Header.Get("X-Emby-Token"))
		}
		_, _ = w.Write([]byte(`{"Items":[{"Id":"a","Name":"Signal Lost","Type":"Series","ProductionYear":2026,"DateCreated":"2026-09-24T12:00:00Z"}]}`))
	}))
	defer server.Close()
	widget := &latestMediaWidget{Service: "jellyfin", Server: server.URL, APIKey: "secret", Limit: 10}
	items, err := fetchLatestMedia(context.Background(), widget)
	if err != nil || len(items) != 1 || items[0].Title != "Signal Lost" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestNavidromeAuthValues(t *testing.T) {
	first, err := navidromeAuthValues("alice", "secret")
	if err != nil {
		t.Fatalf("first auth values: %v", err)
	}
	second, err := navidromeAuthValues("alice", "secret")
	if err != nil {
		t.Fatalf("second auth values: %v", err)
	}

	if first.Get("u") != "alice" || first.Get("v") != "1.16.1" || first.Get("c") != "glance" || first.Get("f") != "json" {
		t.Fatalf("values=%v", first)
	}
	salt := first.Get("s")
	if salt == "" {
		t.Fatal("salt is empty")
	}
	saltBytes, err := base64.RawURLEncoding.DecodeString(salt)
	if err != nil {
		t.Fatalf("decoding salt: %v", err)
	}
	if len(saltBytes) != navidromeSaltRandomBytes {
		t.Fatalf("salt bytes=%d", len(saltBytes))
	}
	digest := md5.Sum([]byte("secret" + salt))
	if first.Get("t") != hex.EncodeToString(digest[:]) {
		t.Fatalf("token=%q", first.Get("t"))
	}
	if first.Get("s") == second.Get("s") {
		t.Fatalf("salt reused: %q", first.Get("s"))
	}
}

func TestNavidromeCoverValues(t *testing.T) {
	widget := &latestMediaWidget{Username: "alice", Password: "secret"}
	values, err := navidromeCoverValues(widget, "cover-123")
	if err != nil {
		t.Fatalf("cover values: %v", err)
	}
	if values.Get("id") != "cover-123" || values.Get("u") != "alice" || values.Get("s") == "" || values.Get("t") == "" {
		t.Fatalf("values=%v", values)
	}
}

func TestNormalizePlexSeason(t *testing.T) {
	item := normalizePlexMedia("https://plex.example", "secret", plexMediaItem{Type: "season", Title: "Season 1", ParentTitle: "Brothers (2026)", Index: 1}, parseMediaTime("2026-09-24T12:00:00Z"))
	if item.Title != "Brothers (2026)" || item.Subtitle != "Season 1" {
		t.Fatalf("item=%#v", item)
	}

	fallback := normalizePlexMedia("https://plex.example", "secret", plexMediaItem{Type: "season", Title: "Season 2"}, parseMediaTime("2026-09-24T12:00:00Z"))
	if fallback.Title != "Season 2" || fallback.Subtitle != "Season 2" {
		t.Fatalf("fallback=%#v", fallback)
	}
}

func TestNormalizePlexEpisode(t *testing.T) {
	item := normalizePlexMedia("https://plex.example", "secret", plexMediaItem{Type: "episode", Title: "The Return", GrandparentTitle: "Signal Lost", ParentIndex: 2, Index: 4, Duration: 3120000}, parseMediaTime("2026-09-24T12:00:00Z"))
	if item.Title != "Signal Lost" || item.Subtitle != "S02E04 · The Return" || item.Duration != "52m" {
		t.Fatalf("item=%#v", item)
	}
}

func TestLatestMediaImageUsesResourceProxyAndDoesNotLeakCredentials(t *testing.T) {
	widget := &latestMediaWidget{APIKey: "secret-token"}
	widget.Providers = &widgetProviders{resourceProxyURL: func(raw string) (string, error) {
		if !strings.Contains(raw, "secret-token") {
			t.Fatalf("raw=%q", raw)
		}
		return "/api/resource-proxy/opaque", nil
	}}
	raw := "https://plex.example/library/metadata/1/thumb?X-Plex-Token=secret-token"
	got := proxyMediaImage(widget.Providers, raw)
	if got != "/api/resource-proxy/opaque" {
		t.Fatalf("got=%q", got)
	}
	widget.Items = []mediaItem{{Title: "Example", ImageURL: got}}
	widget.withTitle("Latest Media")
	html := string(widget.Render())
	if strings.Contains(html, "secret-token") || strings.Contains(html, "plex.example") {
		t.Fatalf("render leaked source: %s", html)
	}
}
