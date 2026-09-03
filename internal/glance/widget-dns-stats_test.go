package glance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type dnsStatsRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f dnsStatsRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestFetchAdguardStatsZeroGraphMaximum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/control/stats" {
			t.Fatalf("unexpected request path: %q", request.URL.Path)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{
			"num_dns_queries": 100,
			"dns_queries": [],
			"num_blocked_filtering": 0,
			"blocked_filtering": [],
			"avg_processing_time": 0.001,
			"top_blocked_domains": []
		}`)
	}))
	defer server.Close()

	stats, err := fetchAdguardStats(
		context.Background(),
		server.URL,
		false,
		0,
		"username",
		"password",
		false,
	)
	if err != nil {
		t.Fatalf("fetchAdguardStats returned error: %v", err)
	}

	if stats.TotalQueries != 100 {
		t.Fatalf("unexpected total queries: got %d, want 100", stats.TotalQueries)
	}

	for i := range stats.Series {
		if stats.Series[i].PercentTotal != 0 {
			t.Fatalf(
				"series %d has unexpected percent total: got %d, want 0",
				i,
				stats.Series[i].PercentTotal,
			)
		}
	}
}

func TestFetchPihole5StatsZeroGraphMaximum(t *testing.T) {
	queries := make([]string, 0, 144)
	blocked := make([]string, 0, 144)

	for i := range 144 {
		timestamp := int64(1_700_000_000 + i*600)
		queries = append(queries, fmt.Sprintf("%q:0", fmt.Sprint(timestamp)))
		blocked = append(blocked, fmt.Sprintf("%q:0", fmt.Sprint(timestamp)))
	}

	responseBody := fmt.Sprintf(`{
		"dns_queries_today": 100,
		"domains_over_time": {%s},
		"ads_blocked_today": 0,
		"ads_over_time": {%s},
		"ads_percentage_today": 0,
		"top_ads": {},
		"domains_being_blocked": 1000
	}`, strings.Join(queries, ","), strings.Join(blocked, ","))

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/admin/api.php" {
			t.Fatalf("unexpected request path: %q", request.URL.Path)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, responseBody)
	}))
	defer server.Close()

	stats, err := fetchPihole5Stats(
		context.Background(),
		server.URL,
		false,
		0,
		"test-token",
		false,
	)
	if err != nil {
		t.Fatalf("fetchPihole5Stats returned error: %v", err)
	}

	if stats.TotalQueries != 100 {
		t.Fatalf("unexpected total queries: got %d, want 100", stats.TotalQueries)
	}

	for i := range stats.Series {
		if stats.Series[i].PercentTotal != 0 {
			t.Fatalf(
				"series %d has unexpected percent total: got %d, want 0",
				i,
				stats.Series[i].PercentTotal,
			)
		}
	}
}

func TestFetchPihole5StatsZeroBlockedQueriesTopDomains(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/admin/api.php" {
			t.Fatalf("unexpected request path: %q", request.URL.Path)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{
			"dns_queries_today": 100,
			"domains_over_time": {},
			"ads_blocked_today": 0,
			"ads_over_time": {},
			"ads_percentage_today": 0,
			"top_ads": {
				"example.test": 10
			},
			"domains_being_blocked": 1000
		}`)
	}))
	defer server.Close()

	stats, err := fetchPihole5Stats(
		context.Background(),
		server.URL,
		false,
		0,
		"test-token",
		true,
	)
	if err != nil {
		t.Fatalf("fetchPihole5Stats returned error: %v", err)
	}

	if len(stats.TopBlockedDomains) != 1 {
		t.Fatalf(
			"unexpected top blocked domain count: got %d, want 1",
			len(stats.TopBlockedDomains),
		)
	}

	if stats.TopBlockedDomains[0].Domain != "example.test" {
		t.Fatalf(
			"unexpected top blocked domain: got %q, want %q",
			stats.TopBlockedDomains[0].Domain,
			"example.test",
		)
	}

	if stats.TopBlockedDomains[0].PercentBlocked != 0 {
		t.Fatalf(
			"unexpected blocked percentage: got %d, want 0",
			stats.TopBlockedDomains[0].PercentBlocked,
		)
	}
}

func TestFetchPiholeStatsZeroGraphMaximum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")

		switch request.URL.Path {
		case "/api/auth":
			if request.Method != http.MethodGet {
				t.Fatalf("unexpected authentication request method: %q", request.Method)
			}

			if request.Header.Get("x-ftl-sid") != "test-session" {
				t.Fatalf(
					"unexpected Pi-hole session header: got %q, want %q",
					request.Header.Get("x-ftl-sid"),
					"test-session",
				)
			}

			_, _ = io.WriteString(writer, `{}`)

		case "/api/stats/summary":
			if request.Header.Get("x-ftl-sid") != "test-session" {
				t.Fatalf(
					"unexpected Pi-hole session header: got %q, want %q",
					request.Header.Get("x-ftl-sid"),
					"test-session",
				)
			}

			_, _ = io.WriteString(writer, `{
				"queries": {
					"total": 100,
					"blocked": 0,
					"percent_blocked": 0
				},
				"gravity": {
					"domains_being_blocked": 1000
				}
			}`)

		case "/api/history":
			if request.Header.Get("x-ftl-sid") != "test-session" {
				t.Fatalf(
					"unexpected Pi-hole session header: got %q, want %q",
					request.Header.Get("x-ftl-sid"),
					"test-session",
				)
			}

			_, _ = io.WriteString(writer, `{"history":[`)

			for i := range 145 {
				if i > 0 {
					_, _ = io.WriteString(writer, ",")
				}

				_, _ = fmt.Fprintf(
					writer,
					`{"timestamp":%d,"total":0,"blocked":0}`,
					1_700_000_000+i*600,
				)
			}

			_, _ = io.WriteString(writer, `]}`)

		default:
			t.Fatalf("unexpected request path: %q", request.URL.Path)
		}
	}))
	defer server.Close()

	stats, sessionID, err := fetchPiholeStats(
		context.Background(),
		server.URL,
		false,
		0,
		"password",
		"test-session",
		true,
		false,
	)
	if err != nil {
		t.Fatalf("fetchPiholeStats returned error: %v", err)
	}

	if sessionID != "test-session" {
		t.Fatalf(
			"unexpected session ID: got %q, want %q",
			sessionID,
			"test-session",
		)
	}

	if stats.TotalQueries != 100 {
		t.Fatalf("unexpected total queries: got %d, want 100", stats.TotalQueries)
	}

	for i := range stats.Series {
		if stats.Series[i].PercentTotal != 0 {
			t.Fatalf(
				"series %d has unexpected percent total: got %d, want 0",
				i,
				stats.Series[i].PercentTotal,
			)
		}
	}
}

func TestFetchPiholeStatsZeroBlockedQueriesTopDomains(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")

		switch request.URL.Path {
		case "/api/auth":
			if request.Method != http.MethodGet {
				t.Fatalf("unexpected authentication request method: %q", request.Method)
			}

			if request.Header.Get("x-ftl-sid") != "test-session" {
				t.Fatalf(
					"unexpected Pi-hole session header: got %q, want %q",
					request.Header.Get("x-ftl-sid"),
					"test-session",
				)
			}

			_, _ = io.WriteString(writer, `{}`)

		case "/api/stats/summary":
			if request.Header.Get("x-ftl-sid") != "test-session" {
				t.Fatalf(
					"unexpected Pi-hole session header: got %q, want %q",
					request.Header.Get("x-ftl-sid"),
					"test-session",
				)
			}

			_, _ = io.WriteString(writer, `{
				"queries": {
					"total": 100,
					"blocked": 0,
					"percent_blocked": 0
				},
				"gravity": {
					"domains_being_blocked": 1000
				}
			}`)

		case "/api/stats/top_domains":
			if request.URL.Query().Get("blocked") != "true" {
				t.Fatalf(
					"unexpected blocked query parameter: got %q, want %q",
					request.URL.Query().Get("blocked"),
					"true",
				)
			}

			if request.Header.Get("x-ftl-sid") != "test-session" {
				t.Fatalf(
					"unexpected Pi-hole session header: got %q, want %q",
					request.Header.Get("x-ftl-sid"),
					"test-session",
				)
			}

			_, _ = io.WriteString(writer, `{
				"domains": [
					{
						"domain": "example.test",
						"count": 10
					}
				],
				"total_queries": 100,
				"blocked_queries": 0,
				"took": 0
			}`)

		default:
			t.Fatalf("unexpected request path: %q", request.URL.Path)
		}
	}))
	defer server.Close()

	stats, sessionID, err := fetchPiholeStats(
		context.Background(),
		server.URL,
		false,
		0,
		"password",
		"test-session",
		false,
		true,
	)
	if err != nil {
		t.Fatalf("fetchPiholeStats returned error: %v", err)
	}

	if sessionID != "test-session" {
		t.Fatalf(
			"unexpected session ID: got %q, want %q",
			sessionID,
			"test-session",
		)
	}

	if len(stats.TopBlockedDomains) != 1 {
		t.Fatalf(
			"unexpected top blocked domain count: got %d, want 1",
			len(stats.TopBlockedDomains),
		)
	}

	if stats.TopBlockedDomains[0].Domain != "example.test" {
		t.Fatalf(
			"unexpected top blocked domain: got %q, want %q",
			stats.TopBlockedDomains[0].Domain,
			"example.test",
		)
	}

	if stats.TopBlockedDomains[0].PercentBlocked != 0 {
		t.Fatalf(
			"unexpected blocked percentage: got %d, want 0",
			stats.TopBlockedDomains[0].PercentBlocked,
		)
	}
}

func TestFetchTechnitiumStatsZeroGraphMaximum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/dashboard/stats/get" {
			t.Fatalf("unexpected request path: %q", request.URL.Path)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{
			"response": {
				"stats": {
					"totalQueries": 100,
					"totalBlocked": 0,
					"blockedZones": 10,
					"blockListZones": 20
				},
				"mainChartData": {
					"datasets": [
						{
							"label": "Total",
							"data": []
						},
						{
							"label": "Blocked",
							"data": []
						}
					]
				},
				"topBlockedDomains": []
			}
		}`)
	}))
	defer server.Close()

	stats, err := fetchTechnitiumStats(
		context.Background(),
		server.URL,
		false,
		0,
		"test-token",
		false,
	)
	if err != nil {
		t.Fatalf("fetchTechnitiumStats returned error: %v", err)
	}

	if stats.TotalQueries != 100 {
		t.Fatalf("unexpected total queries: got %d, want 100", stats.TotalQueries)
	}

	for i := range stats.Series {
		if stats.Series[i].PercentTotal != 0 {
			t.Fatalf(
				"series %d has unexpected percent total: got %d, want 0",
				i,
				stats.Series[i].PercentTotal,
			)
		}
	}
}

func TestFetchPiholeSessionIDTransportErrorDoesNotExposeRequestURL(t *testing.T) {
	const (
		password = "super-secret-password"
		baseURL  = "https://example.com"
	)

	client := &http.Client{
		Transport: dnsStatsRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			return nil, &url.Error{
				Op:  "Post",
				URL: request.URL.String() + "?token=super-secret-token",
				Err: errors.New("connection refused"),
			}
		}),
	}

	_, err := fetchPiholeSessionID(context.Background(), baseURL, client, password)
	if err == nil {
		t.Fatal("expected error")
	}

	message := err.Error()

	if strings.Contains(message, "super-secret-token") {
		t.Fatalf("error exposed sensitive query token: %q", message)
	}

	if strings.Contains(message, baseURL) {
		t.Fatalf("error exposed request URL: %q", message)
	}

	if strings.Contains(message, password) {
		t.Fatalf("error exposed authentication password: %q", message)
	}

	if !strings.Contains(message, "connection refused") {
		t.Fatalf("error did not preserve useful transport cause: %q", message)
	}
}

func TestFetchPiholeSessionIDAuthenticationFailureDoesNotExposeResponseMessage(t *testing.T) {
	const (
		password         = "super-secret-password"
		sensitiveMessage = "internal authentication detail that must not be logged"
	)

	client := &http.Client{
		Transport: dnsStatsRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"session":{"valid":false,"message":"` + sensitiveMessage + `"}}`,
				)),
				Request: request,
			}, nil
		}),
	}

	_, err := fetchPiholeSessionID(
		context.Background(),
		"https://example.com",
		client,
		password,
	)
	if err == nil {
		t.Fatal("expected error")
	}

	message := err.Error()

	if strings.Contains(message, sensitiveMessage) {
		t.Fatalf("error exposed authentication response message: %q", message)
	}

	if strings.Contains(message, password) {
		t.Fatalf("error exposed authentication password: %q", message)
	}

	const expected = "unexpected HTTP status 401 Unauthorized"
	if message != expected {
		t.Fatalf(
			"unexpected sanitized authentication error: got %q, want %q",
			message,
			expected,
		)
	}
}

func TestFetchPiholeSessionIDEmptySessionDoesNotExposeResponseMessage(t *testing.T) {
	const (
		password         = "super-secret-password"
		sensitiveMessage = "sensitive server-side authentication detail"
	)

	client := &http.Client{
		Transport: dnsStatsRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"session":{"sid":"","message":"` + sensitiveMessage + `"}}`,
				)),
				Request: request,
			}, nil
		}),
	}

	_, err := fetchPiholeSessionID(
		context.Background(),
		"https://example.com",
		client,
		password,
	)
	if err == nil {
		t.Fatal("expected error")
	}

	message := err.Error()

	if strings.Contains(message, sensitiveMessage) {
		t.Fatalf("error exposed authentication response message: %q", message)
	}

	if strings.Contains(message, password) {
		t.Fatalf("error exposed authentication password: %q", message)
	}

	const expected = "authentication response returned empty session ID"
	if message != expected {
		t.Fatalf(
			"unexpected empty-session error: got %q, want %q",
			message,
			expected,
		)
	}
}

func TestFetchPiholeSessionIDCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})

	client := &http.Client{
		Transport: dnsStatsRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			close(requestStarted)
			<-request.Context().Done()
			close(requestCanceled)
			return nil, request.Context().Err()
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())

	result := make(chan error, 1)
	go func() {
		_, err := fetchPiholeSessionID(ctx, "https://example.com", client, "password")
		result <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("Pi-hole authentication request did not start")
	}

	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected canceled Pi-hole authentication request to return an error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Pi-hole authentication request did not stop after context cancellation")
	}

	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("Pi-hole authentication transport did not observe cancellation")
	}
}

func TestCheckPiholeSessionIDIsValidCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})

	client := &http.Client{
		Transport: dnsStatsRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			close(requestStarted)
			<-request.Context().Done()
			close(requestCanceled)
			return nil, request.Context().Err()
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())

	result := make(chan error, 1)
	go func() {
		_, err := checkPiholeSessionIDIsValid(ctx, "https://example.com", client, "session-id")
		result <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("Pi-hole session validation request did not start")
	}

	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected canceled Pi-hole session validation request to return an error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Pi-hole session validation request did not stop after context cancellation")
	}

	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("Pi-hole session validation transport did not observe cancellation")
	}
}

func TestDNSStatsHTTPClientTimeout(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-releaseRequest
	}))
	defer server.Close()
	defer close(releaseRequest)

	timeout := durationField(50 * time.Millisecond)

	start := time.Now()
	_, err := fetchAdguardStats(
		context.Background(),
		server.URL,
		false,
		timeout,
		"username",
		"password",
		true,
	)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected DNS request to time out")
	}

	select {
	case <-requestStarted:
	default:
		t.Fatal("DNS timeout test request never reached server")
	}

	if elapsed >= time.Second {
		t.Fatalf("DNS timeout took %v, expected configured timeout to terminate request promptly", elapsed)
	}
}
