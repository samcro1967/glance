package glance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMediaHistoryWidgetInitialize(t *testing.T) {
	widget := &mediaHistoryWidget{Service: "plex", Server: "https://plex.example/", APIKey: "secret"}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if widget.Server != "https://plex.example" || widget.Limit != 10 {
		t.Fatalf("widget=%#v", widget)
	}
}
func TestMediaHistoryWidgetInitializeRequiresProviderContract(t *testing.T) {
	for _, widget := range []*mediaHistoryWidget{{Service: "navidrome", Server: "https://x", APIKey: "x", UserID: "u"}, {Service: "jellyfin", Server: "https://x", APIKey: "x"}, {Service: "emby", Server: "https://x", APIKey: "x"}, {Service: "plex", Server: "https://x", APIKey: ""}} {
		if err := widget.initialize(); err == nil {
			t.Fatalf("expected failure for %#v", widget)
		}
	}
}

func TestFetchMediaHistoryPlexUsesHistoryEndpoint(t *testing.T) {
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions/history/all" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		if r.Header.Get("X-Plex-Token") != "secret" {
			t.Fatalf("token=%q", r.Header.Get("X-Plex-Token"))
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"1","type":"movie","title":"Northbound","year":2026,"viewedAt":1790899200,"duration":7140000}]}}`))
	}))
	defer server.Close()
	widget := &mediaHistoryWidget{Service: "plex", Server: server.URL, APIKey: "secret", Limit: 10}
	items, err := fetchMediaHistory(context.Background(), widget)
	if err != nil || len(items) != 1 || items[0].Title != "Northbound" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	status = http.StatusBadGateway
	_, err = fetchMediaHistory(context.Background(), widget)
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("error=%T %v", err, err)
	}
}

func TestFetchMediaHistoryJellyfinUsesUserAndLastPlayedDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Users/user-1/Items" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != `MediaBrowser Token="secret"` {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Emby-Token") != "" {
			t.Fatalf("unexpected Emby token=%q", r.Header.Get("X-Emby-Token"))
		}
		_, _ = w.Write([]byte(`{"Items":[{"Id":"a","Name":"Signal Lost","Type":"Series","ProductionYear":2026,"UserData":{"LastPlayedDate":"2026-09-24T12:00:00Z"}},{"Id":"b","Name":"Never Played","Type":"Movie","UserData":{}}]}`))
	}))
	defer server.Close()
	widget := &mediaHistoryWidget{Service: "jellyfin", Server: server.URL, APIKey: "secret", UserID: "user-1", Limit: 10}
	items, err := fetchMediaHistory(context.Background(), widget)
	if err != nil || len(items) != 1 || items[0].Title != "Signal Lost" || items[0].Date == "" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestFetchMediaHistoryEmbyUsesEmbyToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "secret" {
			t.Fatalf("token=%q", r.Header.Get("X-Emby-Token"))
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"Items":[{"Id":"a","Name":"Northbound","Type":"Movie","UserData":{"LastPlayedDate":"2026-09-24T12:00:00Z"}}]}`))
	}))
	defer server.Close()
	widget := &mediaHistoryWidget{Service: "emby", Server: server.URL, APIKey: "secret", UserID: "user-1", Limit: 10}
	items, err := fetchMediaHistory(context.Background(), widget)
	if err != nil || len(items) != 1 || items[0].Title != "Northbound" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestMediaHistoryRenderDoesNotLeakCredentials(t *testing.T) {
	widget := &mediaHistoryWidget{APIKey: "secret-token"}
	widget.Providers = &widgetProviders{resourceProxyURL: func(raw string) (string, error) { return "/api/resource-proxy/opaque", nil }}
	widget.Items = []mediaItem{{Title: "Example", ImageURL: proxyMediaImage(widget.Providers, "https://plex.example/thumb?X-Plex-Token=secret-token")}}
	widget.withTitle("Media History")
	html := string(widget.Render())
	if strings.Contains(html, "secret-token") || strings.Contains(html, "plex.example") {
		t.Fatalf("render leaked source: %s", html)
	}
}
