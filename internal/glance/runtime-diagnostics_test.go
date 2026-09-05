package glance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSnapshotWidgetRefreshDiagnostics(t *testing.T) {
	candidate := newRefreshTestWidget()
	candidate.setID(42)
	candidate.Title = "Test Widget"
	candidate.refreshDegraded = true
	candidate.refreshFailureClass = refreshFailureTransient
	candidate.refreshFailureCount = 2

	now := time.Now()
	candidate.lastRefreshAttempt = now.Add(-time.Minute)
	candidate.lastRefreshSuccess = now.Add(-2 * time.Minute)
	candidate.lastRefreshFailure = now.Add(-time.Minute)
	candidate.lastRefreshDuration = 250 * time.Millisecond
	candidate.refreshStartedAt = now
	candidate.refreshAttempts = 5
	candidate.refreshSuccesses = 3
	candidate.refreshFailures = 2
	candidate.refreshLockSkips = 1
	candidate.lastSchedulerLag = 2 * time.Second
	candidate.maxSchedulerLag = 4 * time.Second

	got, ok := snapshotWidgetRefreshDiagnostics(candidate)
	if !ok {
		t.Fatal("expected widget diagnostics snapshot")
	}

	if got.ID != 42 || got.Type != candidate.GetType() || got.Title != "Test Widget" {
		t.Fatalf("unexpected widget identity: %#v", got)
	}
	if !got.Degraded {
		t.Fatalf("unexpected widget state: %#v", got)
	}
	if got.ConsecutiveFailures != 2 {
		t.Fatalf("consecutive failures = %d, want 2", got.ConsecutiveFailures)
	}
	if got.Attempts != 5 || got.Successes != 3 || got.Failures != 2 || got.LockSkips != 1 {
		t.Fatalf("unexpected refresh counters: %#v", got)
	}
	if got.LastDuration != 250*time.Millisecond {
		t.Fatalf("last duration = %v, want 250ms", got.LastDuration)
	}
}

func TestCollectRuntimeDiagnostics(t *testing.T) {
	healthy := newRefreshTestWidget()
	healthy.refreshAttempts = 4
	healthy.refreshSuccesses = 4

	degraded := newRefreshTestWidget()
	degraded.refreshDegraded = true
	degraded.refreshFailureClass = refreshFailureTransient
	degraded.refreshFailureCount = 1
	degraded.refreshAttempts = 3
	degraded.refreshSuccesses = 2
	degraded.refreshFailures = 1
	degraded.refreshLockSkips = 2
	degraded.refreshStartedAt = time.Now()

	failed := newRefreshTestWidget()
	failed.refreshDegraded = true
	failed.refreshAttempts = 2
	failed.refreshFailures = 2

	got := collectRuntimeDiagnostics([]widget{healthy, degraded, failed})

	if got.RefreshWidgets != 3 {
		t.Fatalf("refresh widgets = %d, want 3", got.RefreshWidgets)
	}
	if got.RefreshingWidgets != 1 {
		t.Fatalf("refreshing widgets = %d, want 1", got.RefreshingWidgets)
	}
	if got.DegradedWidgets != 2 {
		t.Fatalf("degraded widgets = %d, want 2", got.DegradedWidgets)
	}
	if got.TotalAttempts != 9 || got.TotalSuccesses != 6 || got.TotalFailures != 3 {
		t.Fatalf(
			"unexpected totals: attempts=%d successes=%d failures=%d",
			got.TotalAttempts,
			got.TotalSuccesses,
			got.TotalFailures,
		)
	}
	if got.TotalLockSkips != 2 {
		t.Fatalf("total lock skips = %d, want 2", got.TotalLockSkips)
	}
	if len(got.Widgets) != 3 {
		t.Fatalf("widget snapshots = %d, want 3", len(got.Widgets))
	}
}

func TestWidgetRefreshDiagnosticsConcurrentWithRefresh(t *testing.T) {
	candidate := newRefreshTestWidget()
	close(candidate.updateBlock)

	const refreshes = 100
	const snapshots = 1000

	done := make(chan struct{})

	go func() {
		defer close(done)

		for range refreshes {
			now := time.Now()
			refreshDueWidgetIfAvailable(
				context.Background(),
				candidate,
				&now,
				nil,
			)

			candidate.setNextUpdateTime(time.Time{})
		}
	}()

	for range snapshots {
		snapshot, ok := snapshotWidgetRefreshDiagnostics(candidate)
		if !ok {
			t.Fatal("expected widget diagnostics snapshot")
		}

		if snapshot.MaxSchedulerLag < snapshot.LastSchedulerLag {
			t.Fatalf(
				"max scheduler lag %v is less than last scheduler lag %v",
				snapshot.MaxSchedulerLag,
				snapshot.LastSchedulerLag,
			)
		}
	}

	<-done

	snapshot, ok := snapshotWidgetRefreshDiagnostics(candidate)
	if !ok {
		t.Fatal("expected final widget diagnostics snapshot")
	}

	if snapshot.Attempts != refreshes {
		t.Fatalf(
			"attempts = %d, want %d",
			snapshot.Attempts,
			refreshes,
		)
	}
}

func TestRuntimeDiagnosticsResponseFromSnapshot(t *testing.T) {
	now := time.Now()
	snapshot := runtimeDiagnostics{
		GeneratedAt:       now,
		RefreshWidgets:    1,
		RefreshingWidgets: 1,
		DegradedWidgets:   1,
		TotalAttempts:     3,
		TotalSuccesses:    2,
		TotalFailures:     1,
		TotalLockSkips:    4,
		Widgets: []widgetRefreshDiagnostics{
			{
				ID:                  42,
				Type:                "test",
				Title:               "Test Widget",
				Degraded:            true,
				FailureClass:        refreshFailureTransient,
				ConsecutiveFailures: 1,
				LastAttempt:         now,
				LastDuration:        250 * time.Millisecond,
				RefreshStartedAt:    now,
				Attempts:            3,
				Successes:           2,
				Failures:            1,
				LockSkips:           4,
				LastSchedulerLag:    125 * time.Millisecond,
				MaxSchedulerLag:     500 * time.Millisecond,
				NextUpdate:          now.Add(time.Minute),
			},
		},
	}

	got := runtimeDiagnosticsResponseFromSnapshot(snapshot)

	if got.RefreshWidgets != 1 ||
		got.RefreshingWidgets != 1 ||
		got.DegradedWidgets != 1 {
		t.Fatalf("unexpected aggregate response: %#v", got)
	}
	if got.TotalAttempts != 3 ||
		got.TotalSuccesses != 2 ||
		got.TotalFailures != 1 ||
		got.TotalLockSkips != 4 {
		t.Fatalf("unexpected aggregate counters: %#v", got)
	}
	if len(got.Widgets) != 1 {
		t.Fatalf("widgets = %d, want 1", len(got.Widgets))
	}

	widget := got.Widgets[0]
	if widget.ID != 42 || widget.Type != "test" || widget.Title != "Test Widget" {
		t.Fatalf("unexpected widget identity: %#v", widget)
	}
	if widget.LastAttempt == nil || widget.RefreshStartedAt == nil || widget.NextUpdate == nil {
		t.Fatalf("expected populated timestamps: %#v", widget)
	}
	if widget.LastSuccess != nil || widget.LastFailure != nil {
		t.Fatalf("expected zero timestamps to be omitted: %#v", widget)
	}
	if widget.LastDurationMS != 250 {
		t.Fatalf("last duration = %vms, want 250", widget.LastDurationMS)
	}
	if widget.LastSchedulerLagMS != 125 || widget.MaxSchedulerLagMS != 500 {
		t.Fatalf("unexpected scheduler lag: %#v", widget)
	}
}

func TestRuntimeDiagnosticsEndpoint(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: hacker-news
`)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil)

	app.router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, want %d; body=%q",
			recorder.Code,
			http.StatusOK,
			recorder.Body.String(),
		)
	}

	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
	}

	var response runtimeDiagnosticsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if response.RefreshWidgets != len(app.refreshWidgets) {
		t.Fatalf(
			"refresh widgets = %d, want %d",
			response.RefreshWidgets,
			len(app.refreshWidgets),
		)
	}
	if len(response.Widgets) != len(app.refreshWidgets) {
		t.Fatalf(
			"widget snapshots = %d, want %d",
			len(response.Widgets),
			len(app.refreshWidgets),
		)
	}
}

func TestHealthzRemainsIndependentOfRuntimeDiagnostics(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: hacker-news
`)

	for _, candidate := range app.refreshWidgets {
		base, ok := widgetBaseOf(candidate)
		if !ok {
			continue
		}

		base.refreshTelemetryMu.Lock()
		base.refreshDegraded = true
		base.refreshFailureClass = refreshFailureTransient
		base.refreshFailureCount = 99
		base.refreshFailures = 99
		base.refreshTelemetryMu.Unlock()
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)

	app.router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want %d", recorder.Code, http.StatusOK)
	}
}
