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

func TestFetchLobstersPostsFromFeedEmptyPreservesNoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	_, err := fetchLobstersPostsFromFeed(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected empty feed error")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve errNoContent: %v", err)
	}

	if !strings.Contains(err.Error(), "Lobsters feed returned no posts") {
		t.Fatalf("error missing empty feed context: %q", err)
	}
}

func TestFetchLobstersPostsFromFeedHTTPFailureHasSafeContext(t *testing.T) {
	const sensitiveBody = "private Lobsters response must not appear"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, sensitiveBody, http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := fetchLobstersPostsFromFeed(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected HTTP failure")
	}

	message := err.Error()

	if !strings.Contains(message, "fetching Lobsters posts") {
		t.Fatalf("error missing Lobsters operation context: %q", message)
	}

	if !strings.Contains(message, "unexpected HTTP status 503 Service Unavailable") {
		t.Fatalf("error missing safe HTTP status: %q", message)
	}

	if strings.Contains(message, sensitiveBody) {
		t.Fatalf("error exposed HTTP response body: %q", message)
	}

	if strings.Contains(message, server.URL) {
		t.Fatalf("error exposed Lobsters feed URL: %q", message)
	}
}

func TestFetchLobstersPostsFromFeedCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)

		select {
		case <-r.Context().Done():
			close(requestCanceled)
		case <-time.After(2 * time.Second):
			t.Error("server request context was not canceled")
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	result := make(chan error, 1)
	go func() {
		_, err := fetchLobstersPostsFromFeed(ctx, server.URL)
		result <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("Lobsters request did not reach server")
	}

	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected canceled Lobsters request to return an error")
		}

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Lobsters request did not stop after context cancellation")
	}

	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("server did not observe Lobsters request cancellation")
	}
}
