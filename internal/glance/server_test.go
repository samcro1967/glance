package glance

import (
	"context"
	"html/template"
	"net"
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

func reserveServerTestPort(t *testing.T) (net.Listener, uint16) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve test port: %v", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	if port < 1 || port > 65535 {
		listener.Close()
		t.Fatalf("reserved invalid test port %d", port)
	}

	return listener, uint16(port)
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

func TestServerStopCancelsWidgetRefreshScheduler(t *testing.T) {
	listener, port := reserveServerTestPort(t)
	if err := listener.Close(); err != nil {
		t.Fatalf("release test port: %v", err)
	}

	testWidget := newServerLifecycleTestWidget()
	app := newServerLifecycleTestApplication(t, port, testWidget)

	start, stop := app.server()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- start()
	}()

	select {
	case <-testWidget.started:
	case <-time.After(time.Second):
		t.Fatal("widget refresh scheduler did not start")
	}

	if err := stop(); err != nil {
		t.Fatalf("stop server: %v", err)
	}

	select {
	case <-testWidget.cancelled:
	case <-time.After(time.Second):
		t.Fatal("server stop did not cancel widget refresh scheduler")
	}

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("server start returned error after stop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop")
	}
}

func TestServerBindFailureStopsWidgetRefreshScheduler(t *testing.T) {
	listener, port := reserveServerTestPort(t)
	defer listener.Close()

	testWidget := newServerLifecycleTestWidget()
	app := newServerLifecycleTestApplication(t, port, testWidget)

	start, _ := app.server()

	startDone := make(chan error, 1)
	go func() {
		startDone <- start()
	}()

	var err error
	select {
	case err = <-startDone:
	case <-time.After(time.Second):
		t.Fatal("server start did not return after bind failure")
	}

	if err == nil {
		t.Fatal("expected server bind failure")
	}

	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("server bind error = %q, want address already in use", err)
	}

	select {
	case <-testWidget.started:
		select {
		case <-testWidget.cancelled:
		case <-time.After(time.Second):
			t.Fatal("bind failure did not cancel started widget refresh")
		}
	default:
		// The scheduler observed cancellation before starting the widget.
	}
}
