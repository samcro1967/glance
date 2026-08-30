package glance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type monitorRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn monitorRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

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

func TestFetchSiteStatusTaskUsesCheckURL(t *testing.T) {
	var requestPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	request := &SiteStatusRequest{
		DefaultURL: server.URL + "/display",
		CheckURL:   server.URL + "/health",
	}

	status, err := fetchSiteStatusTask(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected task error: %v", err)
	}

	if status.Error != nil {
		t.Fatalf("unexpected status error: %v", status.Error)
	}

	if status.Code != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", status.Code, http.StatusNoContent)
	}

	if requestPath != "/health" {
		t.Fatalf("request path = %q, want %q", requestPath, "/health")
	}
}

func TestFetchSiteStatusTaskSendsBasicAuth(t *testing.T) {
	const (
		username = "test-user"
		password = "test-password"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUsername, gotPassword, ok := r.BasicAuth()
		if !ok {
			t.Error("request did not contain Basic Authentication")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if gotUsername != username {
			t.Errorf("username = %q, want %q", gotUsername, username)
		}

		if gotPassword != password {
			t.Errorf("password = %q, want %q", gotPassword, password)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	request := &SiteStatusRequest{
		DefaultURL: server.URL,
	}
	request.BasicAuth.Username = username
	request.BasicAuth.Password = password

	status, err := fetchSiteStatusTask(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected task error: %v", err)
	}

	if status.Error != nil {
		t.Fatalf("unexpected status error: %v", status.Error)
	}

	if status.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", status.Code, http.StatusOK)
	}
}

func TestFetchSiteStatusTaskUsesConfiguredHTTPClient(t *testing.T) {
	originalDefaultTransport := defaultHTTPClient.Transport
	originalInsecureTransport := defaultInsecureHTTPClient.Transport

	t.Cleanup(func() {
		defaultHTTPClient.Transport = originalDefaultTransport
		defaultInsecureHTTPClient.Transport = originalInsecureTransport
	})

	defaultCalls := 0
	insecureCalls := 0

	defaultHTTPClient.Transport = monitorRoundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			defaultCalls++

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
				Header:     make(http.Header),
				Request:    request,
			}, nil
		},
	)

	defaultInsecureHTTPClient.Transport = monitorRoundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			insecureCalls++

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
				Header:     make(http.Header),
				Request:    request,
			}, nil
		},
	)

	status, err := fetchSiteStatusTask(
		context.Background(),
		&SiteStatusRequest{DefaultURL: "https://example.invalid/default"},
	)
	if err != nil {
		t.Fatalf("default client task error: %v", err)
	}
	if status.Error != nil {
		t.Fatalf("default client status error: %v", status.Error)
	}

	status, err = fetchSiteStatusTask(
		context.Background(),
		&SiteStatusRequest{
			DefaultURL:    "https://example.invalid/insecure",
			AllowInsecure: true,
		},
	)
	if err != nil {
		t.Fatalf("insecure client task error: %v", err)
	}
	if status.Error != nil {
		t.Fatalf("insecure client status error: %v", status.Error)
	}

	if defaultCalls != 1 {
		t.Fatalf("default HTTP client calls = %d, want 1", defaultCalls)
	}

	if insecureCalls != 1 {
		t.Fatalf("insecure HTTP client calls = %d, want 1", insecureCalls)
	}
}

func TestFetchSiteStatusTaskTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	request := &SiteStatusRequest{
		DefaultURL: server.URL,
		Timeout:    durationField(25 * time.Millisecond),
	}

	status, err := fetchSiteStatusTask(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected task error: %v", err)
	}

	if status.Error == nil {
		t.Fatal("expected timeout status error")
	}

	if !status.TimedOut {
		t.Fatal("expected timed out status")
	}

	if !errors.Is(status.Error, context.DeadlineExceeded) {
		t.Fatalf("status error = %v, want context deadline exceeded", status.Error)
	}
}

func TestFetchSiteStatusTaskHonorsParentCancellation(t *testing.T) {
	requestStarted := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type result struct {
		status siteStatus
		err    error
	}

	resultCh := make(chan result, 1)

	go func() {
		status, err := fetchSiteStatusTask(
			ctx,
			&SiteStatusRequest{DefaultURL: server.URL},
		)
		resultCh <- result{status: status, err: err}
	}()

	select {
	case <-requestStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for monitor request to start")
	}

	cancel()

	var got result
	select {
	case got = <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for canceled monitor request")
	}

	if got.err != nil {
		t.Fatalf("unexpected task error: %v", got.err)
	}

	if got.status.Error == nil {
		t.Fatal("expected canceled request status error")
	}

	if !errors.Is(got.status.Error, context.Canceled) {
		t.Fatalf("status error = %v, want context canceled", got.status.Error)
	}

	if got.status.TimedOut {
		t.Fatal("parent cancellation should not be classified as timeout")
	}
}

func TestFetchSiteStatusTaskReturnsMalformedURLAsStatusError(t *testing.T) {
	status, err := fetchSiteStatusTask(
		context.Background(),
		&SiteStatusRequest{DefaultURL: "://invalid"},
	)
	if err != nil {
		t.Fatalf("unexpected task error: %v", err)
	}

	if status.Error == nil {
		t.Fatal("expected malformed URL status error")
	}

	if status.Code != 0 {
		t.Fatalf("status code = %d, want 0", status.Code)
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
	widget.Sites = make([]struct {
		*SiteStatusRequest `yaml:",inline"`
		Status             *siteStatus     `yaml:"-"`
		URL                string          `yaml:"-"`
		ErrorURL           string          `yaml:"error-url"`
		Title              string          `yaml:"title"`
		Icon               customIconField `yaml:"icon"`
		SameTab            bool            `yaml:"same-tab"`
		StatusText         string          `yaml:"-"`
		StatusStyle        string          `yaml:"-"`
		AltStatusCodes     []int           `yaml:"alt-status-codes"`
	}, 3)

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
	widget.Sites = make([]struct {
		*SiteStatusRequest `yaml:",inline"`
		Status             *siteStatus     `yaml:"-"`
		URL                string          `yaml:"-"`
		ErrorURL           string          `yaml:"error-url"`
		Title              string          `yaml:"title"`
		Icon               customIconField `yaml:"icon"`
		SameTab            bool            `yaml:"same-tab"`
		StatusText         string          `yaml:"-"`
		StatusStyle        string          `yaml:"-"`
		AltStatusCodes     []int           `yaml:"alt-status-codes"`
	}, 1)

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
	widget.Sites = make([]struct {
		*SiteStatusRequest `yaml:",inline"`
		Status             *siteStatus     `yaml:"-"`
		URL                string          `yaml:"-"`
		ErrorURL           string          `yaml:"error-url"`
		Title              string          `yaml:"title"`
		Icon               customIconField `yaml:"icon"`
		SameTab            bool            `yaml:"same-tab"`
		StatusText         string          `yaml:"-"`
		StatusStyle        string          `yaml:"-"`
		AltStatusCodes     []int           `yaml:"alt-status-codes"`
	}, 1)

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
