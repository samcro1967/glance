package glance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNowPlayingWidgetInitialize(t *testing.T) {
	widget := &nowPlayingWidget{Service: "PLEX", Server: "https://plex.example/", APIKey: "secret"}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if widget.Service != "plex" || widget.Server != "https://plex.example" || widget.Limit != 10 || !widget.ShowProgressInfoValue {
		t.Fatalf("widget=%#v", widget)
	}
}

func TestNowPlayingWidgetInitializeRejectsInvalidConfiguration(t *testing.T) {
	for _, widget := range []*nowPlayingWidget{{Service: "other", Server: "https://x", APIKey: "x"}, {Service: "plex", Server: "relative", APIKey: "x"}, {Service: "plex", Server: "https://x"}, {Service: "navidrome", Server: "https://x", Username: "u"}, {Service: "plex", Server: "https://x?bad=1", APIKey: "x"}, {Service: "plex", Server: "https://x", APIKey: "x", Limit: -1}} {
		if err := widget.initialize(); err == nil {
			t.Fatalf("expected failure for %#v", widget)
		}
	}
}

func TestFetchNowPlayingPlex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		if r.Header.Get("X-Plex-Token") != "secret" {
			t.Fatalf("token=%q", r.Header.Get("X-Plex-Token"))
		}
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"type":"episode","title":"The Return","grandparentTitle":"Signal Lost","parentIndex":2,"index":4,"duration":3000000,"viewOffset":750000,"User":{"title":"alice"},"Player":{"title":"Living Room","product":"Plex Web","state":"playing"},"Media":[{"Part":[{"decision":"directplay"}]}]}]}}`))
	}))
	defer server.Close()
	widget := &nowPlayingWidget{Service: "plex", Server: server.URL, APIKey: "secret", Limit: 10}
	items, err := fetchNowPlaying(context.Background(), widget)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	item := items[0]
	if item.Title != "Signal Lost" || item.Subtitle != "S02E04 · The Return" || item.User != "alice" || item.PlayMethod != "Direct Play" || item.Progress != 25 {
		t.Fatalf("item=%#v", item)
	}
}

func TestFetchNowPlayingJellyfinUsesMediaBrowserAuthorizationAndIgnoresIdle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != `MediaBrowser Token="secret"` {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Emby-Token") != "" {
			t.Fatalf("unexpected Emby token")
		}
		_, _ = w.Write([]byte(`[{"UserName":"idle","Client":"Web"},{"UserName":"alice","Client":"Jellyfin Web","DeviceName":"Chrome","PlayState":{"PositionTicks":600000000,"IsPaused":false,"PlayMethod":"DirectPlay"},"NowPlayingItem":{"Id":"item-1","Name":"The Return","Type":"Episode","SeriesName":"Signal Lost","ParentIndexNumber":2,"IndexNumber":4,"RunTimeTicks":2400000000}}]`))
	}))
	defer server.Close()
	widget := &nowPlayingWidget{Service: "jellyfin", Server: server.URL, APIKey: "secret", Limit: 10}
	items, err := fetchNowPlaying(context.Background(), widget)
	if err != nil || len(items) != 1 || items[0].Progress != 25 || items[0].PlayMethod != "Direct Play" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestFetchNowPlayingEmbyUsesEmbyTokenAndFiltersPaused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "secret" {
			t.Fatalf("token=%q", r.Header.Get("X-Emby-Token"))
		}
		_, _ = w.Write([]byte(`[{"UserName":"alice","PlayState":{"PositionTicks":10000000,"IsPaused":true,"PlayMethod":"DirectStream"},"NowPlayingItem":{"Id":"1","Name":"Northbound","Type":"Movie","RunTimeTicks":100000000}}]`))
	}))
	defer server.Close()
	widget := &nowPlayingWidget{Service: "emby", Server: server.URL, APIKey: "secret", Limit: 10}
	items, err := fetchNowPlaying(context.Background(), widget)
	if err != nil || len(items) != 0 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	widget.ShowPaused = true
	items, err = fetchNowPlaying(context.Background(), widget)
	if err != nil || len(items) != 1 || items[0].State != "paused" || items[0].PlayMethod != "Direct Stream" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestFetchNowPlayingNavidrome(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/getNowPlaying.view" || r.URL.Query().Get("u") != "alice" || r.URL.Query().Get("t") == "" || r.URL.Query().Get("s") == "" {
			t.Fatalf("request=%s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","nowPlaying":{"entry":[{"title":"Think As You Drunk","album":"Think As You Drunk","artist":"Riley Green","duration":228,"username":"alice","playerName":"NavidromeUI","state":"playing","positionMs":57000}]}}}`))
	}))
	defer server.Close()
	widget := &nowPlayingWidget{Service: "navidrome", Server: server.URL, Username: "alice", Password: "secret", Limit: 10}
	items, err := fetchNowPlaying(context.Background(), widget)
	if err != nil || len(items) != 1 || items[0].Title != "Think As You Drunk" || items[0].Position != 57*time.Second {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestNowPlayingProgressClampsAndHandlesZeroDuration(t *testing.T) {
	if got := mediaProgress(2*time.Minute, time.Minute); got != 100 {
		t.Fatalf("progress=%v", got)
	}
	if got := mediaProgress(time.Minute, 0); got != 0 {
		t.Fatalf("progress=%v", got)
	}
	if got := formatNowPlayingProgress(90*time.Second, 5*time.Minute); got != "1:30 / 5:00" {
		t.Fatalf("text=%q", got)
	}
}

func TestNowPlayingRenderDoesNotLeakCredentials(t *testing.T) {
	widget := &nowPlayingWidget{APIKey: "secret-token", ShowThumbnail: true, ShowProgressInfoValue: true}
	widget.Providers = &widgetProviders{resourceProxyURL: func(raw string) (string, error) { return "/api/resource-proxy/opaque", nil }}
	widget.Items = []nowPlayingItem{{Title: "Example", State: "playing", ImageURL: proxyMediaImage(widget.Providers, "https://plex.example/thumb?X-Plex-Token=secret-token")}}
	widget.withTitle("Now Playing")
	html := string(widget.Render())
	if strings.Contains(html, "secret-token") || strings.Contains(html, "plex.example") {
		t.Fatalf("render leaked source: %s", html)
	}
}
