package glance

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Exercise a real HTTP socket without publishing any widget events. Both the
// headers and first frame must arrive well before the 30-second idle heartbeat.
// Repeat the connection to cover the same handshake used during reconnection.
func TestLiveUpdatesInitialBodyArrivesBeforeHeartbeat(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)
	server := httptest.NewServer(app.router())
	defer server.Close()
	for attempt := 0; attempt < 2; attempt++ {
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/live-updates", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
				t.Fatalf("unexpected SSE response: %d %q", response.StatusCode, response.Header.Get("Content-Type"))
			}
			frame := make([]byte, len(": connected\n\n"))
			if _, err := io.ReadFull(response.Body, frame); err != nil {
				t.Fatalf("connection %d did not deliver an immediate SSE frame: %v", attempt, err)
			}
			if string(frame) != ": connected\n\n" {
				t.Fatalf("initial SSE frame = %q", frame)
			}
		}()
	}
}

type liveUpdateInitialBodyFailureWriter struct {
	*liveUpdateDeadlineTestWriter
}

func (w *liveUpdateInitialBodyFailureWriter) Write([]byte) (int, error) {
	return 0, errors.New("initial SSE body failed")
}

// A disconnected client must release its subscription even when writing the
// initial frame fails before the first flush.
func TestLiveUpdatesInitialBodyFailureReleasesSubscription(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)
	writer := &liveUpdateInitialBodyFailureWriter{newLiveUpdateDeadlineTestWriter()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/live-updates", nil).WithContext(ctx)
	app.handleLiveUpdatesRequest(writer, request)
	_, flushes := writer.snapshot()
	if flushes != 0 {
		t.Fatalf("flushed after initial body failure: %d", flushes)
	}
	snapshot := app.liveUpdates.snapshot()
	if snapshot.ActiveSubscribers != 0 || snapshot.Unsubscriptions != 1 {
		t.Fatalf("initial body failure leaked its subscription: %+v", snapshot)
	}
}
