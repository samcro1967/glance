package glance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchDockerContainersFromSourceHTTPStatusIsSafe(t *testing.T) {
	const sensitiveBody = "private Docker API response must not appear"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, sensitiveBody, http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := fetchDockerContainersFromSource(
		context.Background(),
		server.URL,
		"",
		false,
		nil,
	)
	if err == nil {
		t.Fatal("expected HTTP status error")
	}

	message := err.Error()

	if !strings.Contains(message, "Docker API request") {
		t.Fatalf("error missing Docker operation context: %q", message)
	}

	if !strings.Contains(message, "unexpected HTTP status 401 Unauthorized") {
		t.Fatalf("error missing safe HTTP status: %q", message)
	}

	if strings.Contains(message, sensitiveBody) {
		t.Fatalf("error exposed HTTP response body: %q", message)
	}

	if strings.Contains(message, server.URL) {
		t.Fatalf("error exposed Docker source URL: %q", message)
	}
}

func TestFetchDockerContainersFromSourceDecodeErrorHasContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	_, err := fetchDockerContainersFromSource(
		context.Background(),
		server.URL,
		"",
		false,
		nil,
	)
	if err == nil {
		t.Fatal("expected JSON decoding error")
	}

	if !strings.Contains(err.Error(), "decoding Docker response") {
		t.Fatalf("error missing Docker decode context: %q", err)
	}
}

func TestFetchDockerContainersFromSourceTransportErrorDoesNotExposeRequestURL(t *testing.T) {
	_, err := fetchDockerContainersFromSource(
		context.Background(),
		"http://127.0.0.1:1",
		"",
		false,
		nil,
	)
	if err == nil {
		t.Fatal("expected transport error")
	}

	message := err.Error()

	if !strings.Contains(message, "sending Docker request") {
		t.Fatalf("error missing Docker transport context: %q", message)
	}

	if !strings.Contains(message, "connect: connection refused") {
		t.Fatalf("error missing useful transport cause: %q", message)
	}

	if strings.Contains(message, "/containers/json") {
		t.Fatalf("transport error exposed Docker API request path: %q", message)
	}

	if strings.Contains(message, "?all=") {
		t.Fatalf("transport error exposed Docker API query: %q", message)
	}
}
