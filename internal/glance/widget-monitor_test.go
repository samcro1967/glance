package glance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStatusCodeToText(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		altStatusCodes []int
		want           string
	}{
		{name: "OK", status: http.StatusOK, want: "OK"},
		{
			name:           "alternate success",
			status:         http.StatusTeapot,
			altStatusCodes: []int{http.StatusTeapot},
			want:           "OK",
		},
		{name: "not found", status: http.StatusNotFound, want: "Not Found"},
		{name: "forbidden", status: http.StatusForbidden, want: "Forbidden"},
		{name: "unauthorized", status: http.StatusUnauthorized, want: "Unauthorized"},
		{name: "server error", status: http.StatusBadGateway, want: "Server Error"},
		{name: "client error", status: http.StatusBadRequest, want: "Client Error"},
		{name: "other status", status: http.StatusNoContent, want: "204"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusCodeToText(tt.status, tt.altStatusCodes)
			if got != tt.want {
				t.Fatalf(
					"statusCodeToText(%d, %v) = %q, want %q",
					tt.status,
					tt.altStatusCodes,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestStatusCodeToStyle(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		altStatusCodes []int
		want           string
	}{
		{name: "OK", status: http.StatusOK, want: "ok"},
		{
			name:           "alternate success",
			status:         http.StatusTeapot,
			altStatusCodes: []int{http.StatusTeapot},
			want:           "ok",
		},
		{name: "error", status: http.StatusInternalServerError, want: "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusCodeToStyle(tt.status, tt.altStatusCodes)
			if got != tt.want {
				t.Fatalf(
					"statusCodeToStyle(%d, %v) = %q, want %q",
					tt.status,
					tt.altStatusCodes,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestFetchStatusForSitesPreservesRequestOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/first":
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusCreated)
		case "/second":
			w.WriteHeader(http.StatusAccepted)
		case "/third":
			time.Sleep(20 * time.Millisecond)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	requests := []*SiteStatusRequest{
		{DefaultURL: server.URL + "/first"},
		{DefaultURL: server.URL + "/second"},
		{DefaultURL: server.URL + "/third"},
	}

	statuses, err := fetchStatusForSites(context.Background(), requests)
	if err != nil {
		t.Fatalf("fetching site statuses: %v", err)
	}

	if len(statuses) != len(requests) {
		t.Fatalf("status count = %d, want %d", len(statuses), len(requests))
	}

	wantCodes := []int{
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusNoContent,
	}

	for i, want := range wantCodes {
		if statuses[i].Code != want {
			t.Fatalf(
				"status %d code = %d, want %d",
				i,
				statuses[i].Code,
				want,
			)
		}
	}
}

func TestMonitorWidgetUpdateAggregatesSiteState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthy":
			w.WriteHeader(http.StatusOK)
		case "/alternate":
			w.WriteHeader(http.StatusTeapot)
		case "/failing":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	widget := &monitorWidget{}
	widget.Sites = make([]monitorSite, 3)

	widget.Sites[0].SiteStatusRequest = &SiteStatusRequest{
		DefaultURL: server.URL + "/healthy",
	}

	widget.Sites[1].SiteStatusRequest = &SiteStatusRequest{
		DefaultURL: server.URL + "/alternate",
	}
	widget.Sites[1].AltStatusCodes = []int{http.StatusTeapot}

	widget.Sites[2].SiteStatusRequest = &SiteStatusRequest{
		DefaultURL: "https://example.invalid/display",
		CheckURL:   server.URL + "/failing",
	}
	widget.Sites[2].ErrorURL = "https://example.invalid/error"

	widget.update(context.Background())

	if !widget.HasFailing {
		t.Fatal("expected widget to report a failing site")
	}

	if widget.Sites[0].Status == nil {
		t.Fatal("healthy site status was not populated")
	}
	if widget.Sites[0].Status.Code != http.StatusOK {
		t.Fatalf(
			"healthy status code = %d, want %d",
			widget.Sites[0].Status.Code,
			http.StatusOK,
		)
	}
	if widget.Sites[0].URL != server.URL+"/healthy" {
		t.Fatalf(
			"healthy site URL = %q, want %q",
			widget.Sites[0].URL,
			server.URL+"/healthy",
		)
	}
	if widget.Sites[0].StatusText != "OK" {
		t.Fatalf("healthy status text = %q, want %q", widget.Sites[0].StatusText, "OK")
	}
	if widget.Sites[0].StatusStyle != "ok" {
		t.Fatalf("healthy status style = %q, want %q", widget.Sites[0].StatusStyle, "ok")
	}

	if widget.Sites[1].Status == nil {
		t.Fatal("alternate site status was not populated")
	}
	if widget.Sites[1].Status.Code != http.StatusTeapot {
		t.Fatalf(
			"alternate status code = %d, want %d",
			widget.Sites[1].Status.Code,
			http.StatusTeapot,
		)
	}
	if widget.Sites[1].StatusText != "OK" {
		t.Fatalf(
			"alternate status text = %q, want %q",
			widget.Sites[1].StatusText,
			"OK",
		)
	}
	if widget.Sites[1].StatusStyle != "ok" {
		t.Fatalf(
			"alternate status style = %q, want %q",
			widget.Sites[1].StatusStyle,
			"ok",
		)
	}

	if widget.Sites[2].Status == nil {
		t.Fatal("failing site status was not populated")
	}
	if widget.Sites[2].Status.Code != http.StatusServiceUnavailable {
		t.Fatalf(
			"failing status code = %d, want %d",
			widget.Sites[2].Status.Code,
			http.StatusServiceUnavailable,
		)
	}
	if widget.Sites[2].URL != widget.Sites[2].DefaultURL {
		t.Fatalf(
			"HTTP failure site URL = %q, want default URL %q",
			widget.Sites[2].URL,
			widget.Sites[2].DefaultURL,
		)
	}
	if widget.Sites[2].StatusText != "Server Error" {
		t.Fatalf(
			"failing status text = %q, want %q",
			widget.Sites[2].StatusText,
			"Server Error",
		)
	}
	if widget.Sites[2].StatusStyle != "error" {
		t.Fatalf(
			"failing status style = %q, want %q",
			widget.Sites[2].StatusStyle,
			"error",
		)
	}
}

func TestMonitorWidgetUpdateUsesErrorURLForRequestError(t *testing.T) {
	const (
		defaultURL = "https://example.invalid/display"
		errorURL   = "https://example.invalid/error"
	)

	widget := &monitorWidget{}
	widget.Sites = make([]monitorSite, 1)

	widget.Sites[0].SiteStatusRequest = &SiteStatusRequest{
		DefaultURL: defaultURL,
		CheckURL:   "://invalid",
	}
	widget.Sites[0].ErrorURL = errorURL

	widget.update(context.Background())

	if !widget.HasFailing {
		t.Fatal("expected request error to mark widget as failing")
	}

	if widget.Sites[0].Status == nil {
		t.Fatal("site status was not populated")
	}
	if widget.Sites[0].Status.Error == nil {
		t.Fatal("expected site request error")
	}
	if widget.Sites[0].URL != errorURL {
		t.Fatalf("site URL = %q, want error URL %q", widget.Sites[0].URL, errorURL)
	}
	if widget.Sites[0].StatusStyle != "error" {
		t.Fatalf(
			"status style = %q, want %q",
			widget.Sites[0].StatusStyle,
			"error",
		)
	}
}

func TestMonitorWidgetSuccessfulUpdateClearsFailingState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	widget := &monitorWidget{
		HasFailing: true,
	}
	widget.Sites = make([]monitorSite, 1)

	widget.Sites[0].SiteStatusRequest = &SiteStatusRequest{
		DefaultURL: server.URL,
	}

	widget.update(context.Background())

	if widget.HasFailing {
		t.Fatal("expected successful update to clear failing state")
	}

	if widget.Sites[0].Status == nil {
		t.Fatal("site status was not populated")
	}

	if widget.Sites[0].Status.Code != http.StatusOK {
		t.Fatalf(
			"status code = %d, want %d",
			widget.Sites[0].Status.Code,
			http.StatusOK,
		)
	}
}

func TestMonitorWidgetStyles(t *testing.T) {
	sites := []monitorSite{
		{
			SiteStatusRequest: &SiteStatusRequest{},
			Status:            &siteStatus{Code: http.StatusOK},
			URL:               "https://healthy.example.invalid",
			Title:             "Healthy",
			Description:       "Healthy service",
			StatusText:        "OK",
			StatusStyle:       "ok",
		},
		{
			SiteStatusRequest: &SiteStatusRequest{},
			Status:            &siteStatus{Code: http.StatusServiceUnavailable},
			URL:               "https://failing.example.invalid",
			Title:             "Failing",
			Description:       "Failing service",
			StatusText:        "Server Error",
			StatusStyle:       "error",
		},
	}

	for _, tc := range []struct {
		name            string
		style           string
		showFailingOnly bool
		wantClass       string
		notClass        string
		wantHealthy     bool
	}{
		{name: "default", wantClass: "dynamic-columns", notClass: "monitor-grid-card", wantHealthy: true},
		{name: "compact", style: "compact", wantClass: "list-gap-8", notClass: "monitor-grid-card", wantHealthy: true},
		{name: "grid-cards", style: "grid-cards", wantClass: "monitor-grid-card", notClass: "dynamic-columns", wantHealthy: true},
		{name: "grid-cards-failing-only", style: "grid-cards", showFailingOnly: true, wantClass: "monitor-grid-card", notClass: "dynamic-columns", wantHealthy: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			widget := &monitorWidget{
				Style:           tc.style,
				ShowFailingOnly: tc.showFailingOnly,
				HasFailing:      true,
				Sites:           sites,
			}
			widget.ContentAvailable = true

			html := string(widget.Render())
			if !strings.Contains(html, tc.wantClass) {
				t.Fatalf("rendered HTML missing %q: %s", tc.wantClass, html)
			}
			if strings.Contains(html, tc.notClass) {
				t.Fatalf("rendered HTML unexpectedly contains %q: %s", tc.notClass, html)
			}
			if tc.style == "grid-cards" {
				healthyLink := `href="https://healthy.example.invalid"`
				if got := strings.Contains(html, healthyLink); got != tc.wantHealthy {
					t.Fatalf("healthy site rendered = %t, want %t: %s", got, tc.wantHealthy, html)
				}
				if !strings.Contains(html, `href="https://failing.example.invalid"`) {
					t.Fatalf("failing site missing from rendered HTML: %s", html)
				}
			} else {
				if got := strings.Contains(html, ">Healthy</a>"); got != tc.wantHealthy {
					t.Fatalf("healthy site rendered = %t, want %t: %s", got, tc.wantHealthy, html)
				}
				if !strings.Contains(html, ">Failing</a>") {
					t.Fatalf("failing site missing from rendered HTML: %s", html)
				}
			}
		})
	}
}
