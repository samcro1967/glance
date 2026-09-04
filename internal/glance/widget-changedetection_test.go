package glance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchWatchUUIDsFromChangeDetectionCancellation(t *testing.T) {
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
		_, err := fetchWatchUUIDsFromChangeDetection(ctx, server.URL, "", 0, false, nil)
		result <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("change detection watch list request did not reach server")
	}

	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected canceled change detection watch list request to return an error")
		}

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("change detection watch list request did not stop after context cancellation")
	}

	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("server did not observe change detection watch list request cancellation")
	}
}

func TestFetchWatchesFromChangeDetectionCancellation(t *testing.T) {
	requestStarted := make(chan struct{}, 1)
	requestCanceled := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requestStarted <- struct{}{}:
		default:
		}

		select {
		case <-r.Context().Done():
			select {
			case requestCanceled <- struct{}{}:
			default:
			}
		case <-time.After(2 * time.Second):
			t.Error("server request context was not canceled")
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	result := make(chan error, 1)
	go func() {
		_, err := fetchWatchesFromChangeDetection(
			ctx,
			server.URL,
			[]string{
				"watch-1",
				"watch-2",
				"watch-3",
				"watch-4",
				"watch-5",
				"watch-6",
				"watch-7",
				"watch-8",
				"watch-9",
				"watch-10",
				"watch-11",
				"watch-12",
				"watch-13",
				"watch-14",
				"watch-15",
				"watch-16",
			},
			"",
			0,
			false,
			nil,
		)
		result <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("change detection watch request did not reach server")
	}

	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected canceled change detection watches job to return an error")
		}

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("change detection watches job did not stop after context cancellation")
	}

	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("server did not observe change detection watch request cancellation")
	}
}

func TestChangeDetectionHTTPPolicySendsHeadersAndTokenWins(t *testing.T) {
	const token = "dedicated-token"

	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)

		if got := r.Header.Get("X-Change-Test"); got != "present" {
			t.Errorf("X-Change-Test = %q, want present", got)
		}
		if got := r.Header.Get("X-Api-Key"); got != token {
			t.Errorf("X-Api-Key = %q, want dedicated token %q", got, token)
		}

		switch r.URL.Path {
		case "/api/v1/watch":
			_, _ = w.Write([]byte(`{"watch-1":{}}`))
		case "/api/v1/watch/watch-1":
			_, _ = w.Write([]byte(`{"title":"Test","url":"https://example.com","last_changed":2}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	headers := map[string]string{
		"X-Change-Test": "present",
		"X-Api-Key":     "header-must-not-win",
	}

	ids, err := fetchWatchUUIDsFromChangeDetection(
		context.Background(),
		server.URL,
		token,
		0,
		false,
		headers,
	)
	if err != nil {
		t.Fatalf("fetchWatchUUIDsFromChangeDetection: %v", err)
	}

	watches, err := fetchWatchesFromChangeDetection(
		context.Background(),
		server.URL,
		ids,
		token,
		0,
		false,
		headers,
	)
	if err != nil {
		t.Fatalf("fetchWatchesFromChangeDetection: %v", err)
	}
	if len(watches) != 1 {
		t.Fatalf("watches = %d, want 1", len(watches))
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}
