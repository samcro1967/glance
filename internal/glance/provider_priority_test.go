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

func TestPriorityMonitorInitializeContract(t *testing.T) {
	widget := &monitorWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}
	if widget.Title != "Monitor" {
		t.Fatalf("title = %q", widget.Title)
	}
	if widget.cacheDuration != 5*time.Minute {
		t.Fatalf("cache = %v", widget.cacheDuration)
	}
}

func TestPriorityServerStatsInitializeNormalizesRequests(t *testing.T) {
	widget := &serverStatsWidget{Servers: []serverStatsRequest{{Type: "remote", URL: "https://example.com///"}}}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}
	if !widget.WIP {
		t.Fatal("server stats should remain WIP")
	}
	if widget.Servers[0].URL != "https://example.com" {
		t.Fatalf("URL = %q", widget.Servers[0].URL)
	}
	if time.Duration(widget.Servers[0].Timeout) != 3*time.Second {
		t.Fatalf("timeout = %v", widget.Servers[0].Timeout)
	}
}

func TestPriorityServerStatsInitializeDefaultsToLocal(t *testing.T) {
	widget := &serverStatsWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}
	if len(widget.Servers) != 1 || widget.Servers[0].Type != "local" {
		t.Fatalf("servers = %#v", widget.Servers)
	}
}

func TestPriorityFetchRemoteServerInfoBearerAndPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sysinfo/all" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hostname":"priority-host"}`))
	}))
	defer server.Close()

	info, err := fetchRemoteServerInfo(context.Background(), &serverStatsRequest{URL: server.URL, Token: "test-token", Timeout: durationField(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if info == nil || info.Hostname != "priority-host" {
		t.Fatalf("info = %#v", info)
	}
}

func TestPriorityReleasesUnsupportedSourcesRemainNoContent(t *testing.T) {
	releases, err := fetchLatestReleases(context.Background(), []*releaseRequest{{Repository: "example/project", source: releaseSource("future-provider")}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errNoContent) {
		t.Fatalf("error = %v, want no-content classification", err)
	}
	if releases != nil {
		t.Fatalf("releases = %#v, want nil", releases)
	}
	if !strings.Contains(err.Error(), "unsupported source") {
		t.Fatalf("error = %v", err)
	}
}
