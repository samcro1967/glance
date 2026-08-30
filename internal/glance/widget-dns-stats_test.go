package glance

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type dnsStatsRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f dnsStatsRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
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

	_, err := fetchPiholeSessionID(baseURL, client, password)
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
