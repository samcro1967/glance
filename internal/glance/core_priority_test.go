package glance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPriorityPageContentRequestSuccessAndNotFound(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    slug: home
    columns:
      - size: full
        widgets:
          - type: html
            source: "<p>priority-content</p>"
`)

	router := app.router()

	t.Run("existing page", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/pages/home/content/", nil)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), "priority-content") {
			t.Fatalf("response body does not contain widget content: %q", recorder.Body.String())
		}
	})

	t.Run("unknown page", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/pages/missing/content/", nil)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
		}
	})
}

func TestPriorityWidgetRequestBoundaryRemainsNotImplemented(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/widgets/123/example", nil)
	app.router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotImplemented)
	}
}

func TestPriorityWidgetBaseHandleRequestBoundary(t *testing.T) {
	var widget widgetBase
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	widget.handleRequest(recorder, request)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotImplemented)
	}
	if !strings.Contains(recorder.Body.String(), "not implemented") {
		t.Fatalf("body = %q, want not implemented message", recorder.Body.String())
	}
}

func TestPriorityWidgetRetryBackoffIsBounded(t *testing.T) {
	widget := &widgetBase{}
	widget.withCacheDuration(time.Hour)

	for i := 0; i < 20; i++ {
		widget.scheduleEarlyUpdate()
	}

	if widget.updateRetriedTimes != 5 {
		t.Fatalf("updateRetriedTimes = %d, want 5", widget.updateRetriedTimes)
	}
	if widget.nextUpdate.IsZero() {
		t.Fatal("nextUpdate should be scheduled")
	}
}
