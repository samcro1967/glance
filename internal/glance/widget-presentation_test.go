package glance

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type presentationTestWidget struct {
	widgetBase
	text    string
	err     error
	started chan struct{}
	release chan struct{}
}

func newPresentationTestWidget() *presentationTestWidget {
	w := &presentationTestWidget{}
	w.Type = "presentation-test"
	w.ID = widgetIDCounter.Add(1)
	w.ExpandedView = true
	w.withCacheDuration(time.Hour)
	return w
}

func (w *presentationTestWidget) initialize() error { return nil }
func (w *presentationTestWidget) Render() template.HTML {
	return template.HTML(fmt.Sprintf(`<div data-widget-id="%d">%s</div>`, w.ID, template.HTMLEscapeString(w.text)))
}
func (w *presentationTestWidget) RenderExpanded() template.HTML { return w.Render() }
func (w *presentationTestWidget) MicroItems(open bool) []statusBarCompactItem {
	return []statusBarCompactItem{{Kind: "summary", Line1: w.text, OpenLinksInNewTab: open, Error: w.Error}}
}
func (w *presentationTestWidget) update(ctx context.Context) {
	if w.started != nil {
		close(w.started)
		select {
		case <-w.release:
		case <-ctx.Done():
			w.canContinueUpdateAfterHandlingErr(ctx.Err())
			return
		}
	}
	if w.canContinueUpdateAfterHandlingErr(w.err) {
		w.text = "fresh content"
	}
}

func TestWidgetPresentationState(t *testing.T) {
	w := newPresentationTestWidget()
	initial := string(renderWidget(w))
	if !strings.Contains(initial, `data-widget-state="loading"`) || strings.Contains(initial, "ERROR") {
		t.Fatalf("pending presentation: %s", initial)
	}
	w.err = errors.New("provider unavailable")
	w.lockRefresh()
	refreshWidget(context.Background(), w, nil)
	w.unlockRefresh()
	failed := string(renderWidget(w))
	if !strings.Contains(failed, "provider unavailable") || strings.Contains(failed, "Loading…") {
		t.Fatalf("failed presentation: %s", failed)
	}
	if items := renderCompactItems(w, false); len(items) != 1 || items[0].Kind != "error" {
		t.Fatalf("failed compact presentation: %+v", items)
	}
	w.err = nil
	w.lockRefresh()
	refreshWidget(context.Background(), w, nil)
	w.unlockRefresh()
	if got := string(renderWidget(w)); !strings.Contains(got, "fresh content") || w.IsLoading() {
		t.Fatalf("recovered presentation: %s", got)
	}
	// Empty successful results must never be inferred to be still loading.
	w.text = ""
	if w.IsLoading() || !w.ContentAvailable {
		t.Fatal("successful empty result was treated as pending")
	}
	// Static content and configuration errors do not wait for scheduler attempts.
	static := &clockWidget{}
	if err := static.initialize(); err != nil {
		t.Fatal(err)
	}
	if static.IsLoading() {
		t.Fatal("static clock is loading")
	}
	invalid := newInvalidConfiguredWidget(nil, "rss", 1, errors.New("invalid configuration"))
	if got := string(renderWidget(invalid)); !strings.Contains(got, "invalid configuration") || strings.Contains(got, "Loading…") {
		t.Fatalf("configuration error presentation: %s", got)
	}
}

func TestWidgetPresentationSnapshotsDuringRefresh(t *testing.T) {
	for _, prior := range []string{"pending", "content", "failure"} {
		t.Run(prior, func(t *testing.T) {
			cached := prior == "content"
			w := newPresentationTestWidget()
			if prior == "failure" {
				w.withError(errors.New("previous failure"))
				w.initialRefreshCompleted = true
			}
			if cached {
				w.withError(nil)
				w.text = "previous content"
			}
			w.started = make(chan struct{})
			w.release = make(chan struct{})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				w.lockRefresh()
				defer w.unlockRefresh()
				refreshWidget(ctx, w, nil)
			}()
			select {
			case <-w.started:
			case <-time.After(time.Second):
				t.Fatal("fetch did not start")
			}
			type presentation struct {
				normal, expanded template.HTML
				compact          []statusBarCompactItem
			}
			results := make(chan presentation, 1)
			go func() {
				results <- presentation{renderWidget(w), renderExpandedWidget(w, w, &w.widgetBase), renderCompactItems(w, false)}
			}()
			select {
			case got := <-results:
				want := "Loading…"
				kind := "loading"
				if cached {
					want = "previous content"
					kind = "summary"
				}
				if prior == "failure" {
					want = "previous failure"
					kind = "error"
				}
				if !strings.Contains(string(got.normal), want) || !strings.Contains(string(got.expanded), want) || len(got.compact) != 1 || got.compact[0].Kind != kind {
					t.Fatalf("in-flight presentation: %+v", got)
				}
				if got.compact[0].OpenLinksInNewTab {
					t.Fatal("snapshot navigation was not adjusted")
				}
			case <-time.After(time.Second):
				t.Fatal("presentation blocked behind provider")
			}
			close(w.release)
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("fetch did not finish")
			}
			if got := string(renderWidget(w)); !strings.Contains(got, "fresh content") {
				t.Fatalf("completed: %s", got)
			}
		})
	}
}

func TestWidgetPresentationCancellation(t *testing.T) {
	w := newPresentationTestWidget()
	w.started = make(chan struct{})
	w.release = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.lockRefresh()
	refreshWidget(ctx, w, nil)
	w.unlockRefresh()
	if !w.IsLoading() || w.Error != nil || w.initialRefreshCompleted {
		t.Fatal("cancelled first fetch became a completed failure")
	}
}

func TestWidgetPresentationFooterAndCalendar(t *testing.T) {
	docker := &microDocker{Container: "glance"}
	if err := docker.initialize(); err != nil {
		t.Fatal(err)
	}
	if got := string(renderWidget(docker)); !strings.Contains(got, "Loading…") || strings.Contains(got, "footer-micro-status-error") {
		t.Fatalf("unfetched Docker footer: %s", got)
	}
	monitor := &microMonitor{}
	monitor.ShowFailingOnly = true
	monitor.withCacheDuration(time.Minute)
	if got := string(renderWidget(monitor)); !strings.Contains(got, "Loading…") || strings.Contains(got, "All online") {
		t.Fatalf("unfetched monitor footer: %s", got)
	}
	static := &calendarWidget{}
	if err := static.initialize(); err != nil {
		t.Fatal(err)
	}
	if static.IsLoading() || !static.ContentAvailable {
		t.Fatal("static calendar is not ready")
	}
	sourced := &calendarWidget{Sources: []icsEventSource{{URL: "https://example.invalid/events.ics"}}}
	if err := sourced.initialize(); err != nil {
		t.Fatal(err)
	}
	if !sourced.IsLoading() {
		t.Fatal("calendar with ICS sources is prematurely ready")
	}
}

func TestWidgetPresentationRetainsActualStaleContent(t *testing.T) {
	file := filepath.Join(t.TempDir(), "content.md")
	if err := os.WriteFile(file, []byte("Last successful content"), 0600); err != nil {
		t.Fatal(err)
	}
	w := &markdownWidget{File: file}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	w.lockRefresh()
	refreshWidget(context.Background(), w, nil)
	w.unlockRefresh()
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	w.lockRefresh()
	refreshWidget(context.Background(), w, nil)
	w.unlockRefresh()
	got := string(renderWidget(w))
	if !strings.Contains(got, "Last successful content") || !strings.Contains(got, "notice-icon-major") || strings.Contains(got, "Loading…") {
		t.Fatalf("stale content and failure indicator: %s", got)
	}
}

func TestWidgetPresentationHTTPInitialFetchAndCompletion(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var requests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		if _, err := w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Test</title><link>https://example.com</link><description>Fixture</description><item><title>Delayed fixture content</title><link>https://example.com/item</link><guid>fixture-item</guid></item></channel></rss>`)); err != nil {
			return
		}
	}))
	defer provider.Close()
	defer unblock()
	c, err := newConfigFromYAML([]byte(fmt.Sprintf("pages:\n  - name: Home\n    columns:\n      - size: full\n        widgets:\n          - type: rss\n            feeds:\n              - url: %s\n", provider.URL)))
	if err != nil {
		t.Fatal(err)
	}
	a, err := newApplication(c)
	if err != nil {
		t.Fatal(err)
	}
	subscription, unsubscribe := a.liveUpdates.subscribe(nil)
	defer unsubscribe()
	runtime := a.startRuntime()
	defer runtime.stop()
	// Unblock before stopping the runtime even on a failed assertion.
	defer unblock()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("fixture fetch did not start")
	}
	requestContent := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest("GET", "/api/pages/home/content/", nil)
		request.SetPathValue("page", "home")
		response := httptest.NewRecorder()
		a.handlePageContentRequest(response, request)
		return response
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- requestContent() }()
	select {
	case response := <-result:
		body := response.Body.String()
		if response.Code != 200 || !strings.Contains(body, "Loading…") || strings.Contains(body, "No error information") {
			t.Fatalf("pending HTTP content: %s", body)
		}
	case <-time.After(time.Second):
		t.Fatal("page blocked behind initial HTTP fetch")
	}
	unblock()
	select {
	case <-subscription.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh completion was not published")
	}
	response := requestContent()
	if got := response.Body.String(); response.Code != 200 || !strings.Contains(got, "Delayed fixture content") || strings.Contains(got, "Loading…") {
		t.Fatalf("completed HTTP content: %s", got)
	}
	if requests.Load() != 1 {
		t.Fatalf("rendering triggered extra provider requests: %d", requests.Load())
	}
}

func TestWidgetPresentationStatusBarDeliveryAndReuse(t *testing.T) {
	makeApp := func() *application {
		t.Helper()
		c, err := newConfigFromYAML([]byte("pages:\n  - name: Home\n    head-widgets:\n      - type: status-bar\n        widgets:\n          - type: clock\n    columns:\n      - size: full\n"))
		if err != nil {
			t.Fatal(err)
		}
		a, err := newApplication(c)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	previous := makeApp()
	candidate := makeApp()
	plan, err := prepareWidgetReloadReusePlan(previous, candidate)
	if err != nil {
		t.Fatal(err)
	}
	candidate.applyWidgetReloadReusePlan(plan)
	bar := candidate.Config.Pages[0].HeadWidgets[0].(*statusBarWidget)
	child := bar.Widgets[0]
	if child != previous.Config.Pages[0].HeadWidgets[0].(*statusBarWidget).Widgets[0] {
		t.Fatal("unchanged bar child was not reused")
	}
	if candidate.widgetByID[bar.ID] != bar || !candidate.canAccessWidget("", bar.ID) {
		t.Fatal("bar render target not registered with page authorization")
	}
	subscription, unsubscribe := candidate.liveUpdates.subscribe(map[uint64]struct{}{bar.ID: {}})
	defer unsubscribe()
	candidate.liveUpdates.publish(child.GetID())
	select {
	case <-subscription.ready:
	case <-time.After(time.Second):
		t.Fatal("bar did not receive child completion")
	}
	pending := subscription.takePending()
	if len(pending) != 1 || pending[0] != bar.ID {
		t.Fatalf("bar delivery = %v", pending)
	}
}
