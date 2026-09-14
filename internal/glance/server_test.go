package glance

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

type serverLifecycleTestWidget struct {
	widgetBase

	started   chan struct{}
	cancelled chan struct{}
}

func newServerLifecycleTestWidget() *serverLifecycleTestWidget {
	testWidget := &serverLifecycleTestWidget{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
	}

	testWidget.Type = "server-lifecycle-test"
	testWidget.withCacheDuration(time.Hour)

	return testWidget
}

func (widget *serverLifecycleTestWidget) initialize() error {
	return nil
}

func (widget *serverLifecycleTestWidget) update(ctx context.Context) {
	close(widget.started)

	<-ctx.Done()

	close(widget.cancelled)
}

func (widget *serverLifecycleTestWidget) Render() template.HTML {
	return ""
}

func newServerLifecycleTestApplication(
	t *testing.T,
	port uint16,
	testWidget widget,
) *application {
	t.Helper()

	app := newGlanceTestApplication(t, `
server:
  host: 127.0.0.1
  port: `+strconv.Itoa(int(port))+`

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	app.refreshWidgets = []widget{testWidget}

	return app
}

func TestProfilingIsNotExposedByMainRouter(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("frontend_diagnostics_%t", enabled), func(t *testing.T) {
			app := &application{}
			app.Config.Server.FrontendDiagnostics = enabled

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
			app.router().ServeHTTP(recorder, request)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("main router pprof status = %d, want %d", recorder.Code, http.StatusNotFound)
			}
		})
	}
}

func TestFrontendDiagnosticProfileHandlerExposesStandardProfiles(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	frontendDiagnosticProfileHandler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("pprof index status = %d, want %d", recorder.Code, http.StatusOK)
	}

	if !strings.Contains(recorder.Body.String(), "goroutine") {
		t.Fatal("pprof index does not expose standard runtime profiles")
	}
}
