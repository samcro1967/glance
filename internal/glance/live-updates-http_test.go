package glance

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWidgetContentRequestRendersWidget(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: html
            source: "<p>live-content</p>"
`)

	if len(app.refreshWidgets) != 1 {
		t.Fatalf("refresh widget count = %d, want 1", len(app.refreshWidgets))
	}

	widgetID := app.refreshWidgets[0].GetID()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/widgets/"+strconv.FormatUint(widgetID, 10)+"/content/",
		nil,
	)

	app.router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, want %d; body=%q",
			recorder.Code,
			http.StatusOK,
			recorder.Body.String(),
		)
	}

	if !strings.Contains(recorder.Body.String(), "live-content") {
		t.Fatalf("response does not contain widget content: %q", recorder.Body.String())
	}

	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/html; charset=utf-8", got)
	}
}

func TestWidgetContentRequestFindsNestedLeafWidget(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: group
            widgets:
              - type: html
                source: "<p>nested-live-content</p>"
`)

	if len(app.refreshWidgets) != 1 {
		t.Fatalf("refresh widget count = %d, want 1", len(app.refreshWidgets))
	}

	leaf := app.refreshWidgets[0]
	if _, exists := app.widgetByID[leaf.GetID()]; !exists {
		t.Fatalf("nested leaf widget %d is not registered", leaf.GetID())
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/widgets/"+strconv.FormatUint(leaf.GetID(), 10)+"/content/",
		nil,
	)

	app.router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, want %d; body=%q",
			recorder.Code,
			http.StatusOK,
			recorder.Body.String(),
		)
	}

	if !strings.Contains(recorder.Body.String(), "nested-live-content") {
		t.Fatalf("response does not contain nested widget content: %q", recorder.Body.String())
	}
}

func TestWidgetContentRequestReturnsNotFoundForInvalidWidgetID(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	for _, widgetID := range []string{"not-a-number", "999999999"} {
		t.Run(widgetID, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(
				http.MethodGet,
				"/api/widgets/"+widgetID+"/content/",
				nil,
			)

			app.router().ServeHTTP(recorder, request)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
			}
		})
	}
}

func TestGenericWidgetRequestRemainsNotImplemented(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/widgets/123/example",
		nil,
	)

	app.router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotImplemented)
	}
}

func TestLiveUpdatesStreamsWidgetNotification(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	server := httptest.NewServer(app.router())
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		server.URL+"/api/live-updates",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	app.liveUpdates.publish(42)

	readDone := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(response.Body)
		var event strings.Builder

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				readDone <- ""
				return
			}

			event.WriteString(line)
			if line == "\n" {
				readDone <- event.String()
				return
			}
		}
	}()

	select {
	case event := <-readDone:
		want := "event: widget\ndata: 42\n\n"
		if event != want {
			t.Fatalf("SSE event = %q, want %q", event, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for SSE widget notification")
	}
}

func TestLiveUpdatesStopsWhenBrokerCloses(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	server := httptest.NewServer(app.router())
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		server.URL+"/api/live-updates",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	app.liveUpdates.close()

	done := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(response.Body)
		done <- err
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE response did not stop after broker close")
	}
}

func TestWidgetContentRequestAuthenticatesBeforeWidgetLookup(t *testing.T) {
	app := newAuthTestApplication(t)
	app.widgetByID = make(map[uint64]widget)
	app.liveUpdates = newLiveUpdateBroker()

	tests := []string{
		"not-a-number",
		"999999999",
	}

	for _, widgetID := range tests {
		t.Run(widgetID, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(
				http.MethodGet,
				"/api/widgets/"+widgetID+"/content/",
				nil,
			)

			app.router().ServeHTTP(recorder, request)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf(
					"status = %d, want %d",
					recorder.Code,
					http.StatusUnauthorized,
				)
			}

			if got := recorder.Body.String(); got != `{"error": "Unauthorized"}` {
				t.Fatalf(
					"body = %q, want unauthorized JSON",
					got,
				)
			}
		})
	}
}

func TestLiveUpdatesRequestRequiresAuthenticationBeforeSubscription(t *testing.T) {
	app := newAuthTestApplication(t)
	app.widgetByID = make(map[uint64]widget)
	app.liveUpdates = newLiveUpdateBroker()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/live-updates",
		nil,
	)

	app.router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"status = %d, want %d",
			recorder.Code,
			http.StatusUnauthorized,
		)
	}

	if got := recorder.Body.String(); got != `{"error": "Unauthorized"}` {
		t.Fatalf(
			"body = %q, want unauthorized JSON",
			got,
		)
	}

	app.liveUpdates.mu.Lock()
	subscriberCount := len(app.liveUpdates.subscribers)
	app.liveUpdates.mu.Unlock()

	if subscriberCount != 0 {
		t.Fatalf(
			"subscriber count = %d after unauthorized request, want 0",
			subscriberCount,
		)
	}
}
