package glance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func mustQBittorrentTestClient(t *testing.T) *http.Client {
	t.Helper()
	client, err := newQBittorrentClient(0, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return client
}

func TestTorrentingWidgetInitialize(t *testing.T) {
	widget := &torrentingWidget{Server: "https://qbittorrent.example.com/", CollapseAfter: 3}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if widget.Endpoint != "https://qbittorrent.example.com/api/v2/torrents/info" {
		t.Fatalf("endpoint = %q", widget.Endpoint)
	}
	if widget.Title != "Torrents" {
		t.Fatalf("title = %q", widget.Title)
	}
}

func TestTorrentingWidgetInitializeRejectsPartialCredentials(t *testing.T) {
	widget := &torrentingWidget{Server: "https://qbittorrent.example.com", Username: "glance"}
	if err := widget.initialize(); err == nil {
		t.Fatal("expected partial credentials to fail")
	}
}

func TestFetchQBittorrentUsesBearerAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/torrents/info" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"Linux ISO","state":"downloading","progress":0.5,"downloaded":512,"size":1024,"eta":60}]`))
	}))
	defer server.Close()

	torrents, err := fetchQBittorrent(context.Background(), mustQBittorrentTestClient(t), qBittorrentRequestOptions{Server: server.URL, Endpoint: server.URL + "/api/v2/torrents/info", APIKey: "secret-token"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(torrents) != 1 || torrents[0].Name != "Linux ISO" {
		t.Fatalf("torrents = %#v", torrents)
	}
}

func TestFetchQBittorrentSessionLoginAndSingleRetry(t *testing.T) {
	var fetches atomic.Int32
	var logins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			logins.Add(1)
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if r.Form.Get("username") != "glance" || r.Form.Get("password") != "password" {
				t.Fatal("unexpected credentials")
			}
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_TEST", Value: "session", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/torrents/info":
			fetches.Add(1)
			cookie, err := r.Cookie("QBT_SID_TEST")
			if err != nil || cookie.Value != "session" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := fetchQBittorrent(context.Background(), mustQBittorrentTestClient(t), qBittorrentRequestOptions{Server: server.URL, Endpoint: server.URL + "/api/v2/torrents/info", Username: "glance", Password: "password"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if fetches.Load() != 2 || logins.Load() != 1 {
		t.Fatalf("fetches=%d logins=%d", fetches.Load(), logins.Load())
	}
}

func TestFetchQBittorrentPreservesHTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := fetchQBittorrent(context.Background(), mustQBittorrentTestClient(t), qBittorrentRequestOptions{Server: server.URL, Endpoint: server.URL + "/api/v2/torrents/info"})
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("error = %T %v", err, err)
	}
}

func TestNormalizeQBittorrentTorrent(t *testing.T) {
	record := normalizeQBittorrentTorrent(qBittorrentTorrent{Name: "Linux ISO", State: "downloading", Progress: 0.625, Downloaded: 5 << 30, Size: 8 << 30, ETA: 3720})
	if !record.Active || record.Completed {
		t.Fatalf("active=%t completed=%t", record.Active, record.Completed)
	}
	if record.ProgressText != "62%" || record.ETA != "1h 2m" || record.Downloaded != "5.0 GiB" {
		t.Fatalf("record = %#v", record)
	}
}

func TestTorrentingWidgetRenderDoesNotExposeCredentials(t *testing.T) {
	widget := &torrentingWidget{Server: "https://qbittorrent.example.com", APIKey: "secret-token", Username: "glance", Password: "secret-password", Torrents: []torrentRecord{{Name: "Linux ISO", ProgressText: "50%", ProgressCSS: "50.0%", Downloaded: "1.0 GiB", Size: "2.0 GiB", ETA: "10m"}}}
	widget.withTitle("Torrents")
	html := string(widget.Render())
	for _, secret := range []string{"secret-token", "secret-password"} {
		if strings.Contains(html, secret) {
			t.Fatalf("rendered HTML contains secret %q", secret)
		}
	}
}
