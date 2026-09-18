package glance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSnapshotWidgetRefreshDiagnostics(t *testing.T) {
	candidate := newRefreshTestWidget()
	candidate.setID(42)
	candidate.Title = "Test Widget"
	candidate.refreshDegraded = true
	candidate.refreshFailureClass = refreshFailureTransient
	candidate.lastRefreshError = "connection refused"
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
	if got.FailureCause != "connection refused" {
		t.Fatalf("failure cause = %q, want connection refused", got.FailureCause)
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
				FailureCause:        "connection refused",
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
	if widget.FailureCause != "connection refused" {
		t.Fatalf("failure cause = %q, want connection refused", widget.FailureCause)
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

func TestRenderRuntimeDiagnosticsResponseFromSnapshot(t *testing.T) {
	now := time.Now()
	snapshot := renderRuntimeDiagnosticsSnapshot{
		StartedAt:                  now,
		WidgetCalls:                5,
		WidgetSnapshotHits:         3,
		WidgetRefreshLockWaits:     1,
		WidgetLockWaitTotal:        4 * time.Millisecond,
		WidgetLockWaitMax:          4 * time.Millisecond,
		WidgetRenders:              2,
		WidgetRenderTotal:          12 * time.Millisecond,
		WidgetRenderMax:            8 * time.Millisecond,
		PageExecutions:             2,
		PageFailures:               1,
		PageLockWaitTotal:          6 * time.Millisecond,
		PageLockWaitMax:            5 * time.Millisecond,
		PageTemplateExecutionTotal: 20 * time.Millisecond,
		PageTemplateExecutionMax:   14 * time.Millisecond,
	}

	got := renderRuntimeDiagnosticsResponseFromSnapshot(snapshot)

	if got.StartedAt == nil || !got.StartedAt.Equal(now) {
		t.Fatalf("started at = %v, want %v", got.StartedAt, now)
	}
	if got.WidgetCalls != 5 || got.WidgetSnapshotHits != 3 || got.WidgetRenders != 2 || got.WidgetRefreshLockWaits != 1 {
		t.Fatalf("unexpected widget counters: %#v", got)
	}
	if got.WidgetLockWaitAverageMS != 4 || got.WidgetLockWaitMaxMS != 4 {
		t.Fatalf("unexpected widget lock-wait durations: %#v", got)
	}
	if got.WidgetRenderAverageMS != 6 || got.WidgetRenderMaxMS != 8 {
		t.Fatalf("unexpected widget render durations: %#v", got)
	}
	if got.PageExecutions != 2 || got.PageFailures != 1 {
		t.Fatalf("unexpected page counters: %#v", got)
	}
	if got.PageLockWaitAverageMS != 3 || got.PageLockWaitMaxMS != 5 {
		t.Fatalf("unexpected page lock-wait durations: %#v", got)
	}
	if got.PageTemplateExecutionAverageMS != 10 || got.PageTemplateExecutionMaxMS != 14 {
		t.Fatalf("unexpected page template durations: %#v", got)
	}
}

func TestRuntimeDiagnosticsReportShowsRenderingPerformance(t *testing.T) {
	now := time.Now()
	response := runtimeDiagnosticsResponse{
		GeneratedAt: now,
		Rendering: renderRuntimeDiagnosticsResponse{
			StartedAt:                      &now,
			WidgetCalls:                    8,
			WidgetSnapshotHits:             5,
			WidgetRefreshLockWaits:         1,
			WidgetLockWaitAverageMS:        2.5,
			WidgetLockWaitMaxMS:            2.5,
			WidgetRenders:                  3,
			WidgetRenderAverageMS:          4.25,
			WidgetRenderMaxMS:              7.5,
			PageExecutions:                 2,
			PageFailures:                   1,
			PageLockWaitAverageMS:          0.5,
			PageLockWaitMaxMS:              0.75,
			PageTemplateExecutionAverageMS: 12.5,
			PageTemplateExecutionMaxMS:     20,
		},
	}

	report := formatRuntimeDiagnosticsReport(
		response,
		runtimeDiagnosticsReportIdentity{},
		false,
		frontendRuntimeDiagnosticsSnapshot{},
	)

	for _, want := range []string{
		"Overall: HEALTHY",
		"RENDERING",
		"Widget calls:              8",
		"Snapshot hits:             5",
		"Actual renders:            3",
		"Refresh-lock waits:        1",
		"Refresh-lock wait avg/max: 2.500 / 2.500 ms",
		"Widget render avg/max:     4.250 / 7.500 ms",
		"Page template executions: 2",
		"Page template failures:   1",
		"Page lock wait avg/max:    0.500 / 0.750 ms",
		"Page template avg/max:     12.500 / 20.000 ms",
		"widget Render() excludes refresh-lock wait; page template execution excludes page-lock wait",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestOutboundHTTPRuntimeDiagnosticsResponseFromSnapshot(t *testing.T) {
	now := time.Now()
	snapshot := outboundHTTPRuntimeDiagnosticsSnapshot{
		StartedAt:       now,
		Exchanges:       3,
		TransportErrors: 1,
		Status2xx:       1,
		Status3xx:       1,
		TotalDuration:   60 * time.Millisecond,
		MaxDuration:     30 * time.Millisecond,
		Destinations: map[string]outboundHTTPDestinationDiagnostics{
			"GET https://z.example": {
				Exchanges:      1,
				Status2xx:      1,
				TotalDuration:  30 * time.Millisecond,
				LastDuration:   30 * time.Millisecond,
				MaxDuration:    30 * time.Millisecond,
				LastExchangeAt: now,
			},
			"GET https://a.example": {
				Exchanges:       2,
				TransportErrors: 1,
				Status3xx:       1,
				TotalDuration:   30 * time.Millisecond,
				LastDuration:    20 * time.Millisecond,
				MaxDuration:     20 * time.Millisecond,
				LastExchangeAt:  now,
			},
		},
	}

	got := outboundHTTPRuntimeDiagnosticsResponseFromSnapshot(snapshot)

	if got.StartedAt == nil || !got.StartedAt.Equal(now) {
		t.Fatalf("started at = %v, want %v", got.StartedAt, now)
	}
	if got.Exchanges != 3 || got.TransportErrors != 1 || got.Status2xx != 1 || got.Status3xx != 1 {
		t.Fatalf("unexpected aggregate response: %#v", got)
	}
	if got.TotalDurationMS != 60 || got.AverageDurationMS != 20 || got.MaxDurationMS != 30 {
		t.Fatalf("unexpected aggregate durations: %#v", got)
	}
	if len(got.Destinations) != 2 {
		t.Fatalf("destinations = %d, want 2", len(got.Destinations))
	}
	if got.Destinations[0].Destination != "GET https://a.example" ||
		got.Destinations[1].Destination != "GET https://z.example" {
		t.Fatalf("destinations not sorted: %#v", got.Destinations)
	}
	if got.Destinations[0].AverageDurationMS != 15 ||
		got.Destinations[0].LastDurationMS != 20 ||
		got.Destinations[0].MaxDurationMS != 20 {
		t.Fatalf("unexpected destination durations: %#v", got.Destinations[0])
	}
}

func TestRuntimeDiagnosticsReportShowsOutboundHTTPPerformance(t *testing.T) {
	now := time.Now()
	response := runtimeDiagnosticsResponse{
		GeneratedAt: now,
		OutboundHTTP: outboundHTTPRuntimeDiagnosticsResponse{
			StartedAt:         &now,
			Exchanges:         4,
			TransportErrors:   1,
			Status2xx:         2,
			Status3xx:         1,
			AverageDurationMS: 12.5,
			MaxDurationMS:     40,
			Destinations: []outboundHTTPDestinationDiagnosticsResponse{
				{
					Destination:       "GET https://api.example",
					Exchanges:         4,
					TransportErrors:   1,
					AverageDurationMS: 12.5,
					LastDurationMS:    10,
					MaxDurationMS:     40,
				},
			},
		},
	}

	report := formatRuntimeDiagnosticsReport(
		response,
		runtimeDiagnosticsReportIdentity{},
		false,
		frontendRuntimeDiagnosticsSnapshot{},
	)

	for _, want := range []string{
		"Overall: HEALTHY",
		"OUTBOUND HTTP",
		"Exchanges:        4",
		"Transport errors: 1",
		"Responses:        1xx=0 2xx=2 3xx=1 4xx=0 5xx=0 other=0",
		"Average:          12.500 ms",
		"Maximum:          40.000 ms",
		"transport RoundTrip through response headers; response body/decode excluded",
		"GET https://api.example",
		"exchanges=4 errors=1 avg=12.500ms last=10.000ms max=40.000ms",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
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

	profilingDiagnostics := newProfilingRuntimeDiagnostics()
	profilingDiagnostics.recordRunning()
	profilingDiagnostics.recordFailure(errors.New("profiling unavailable"))
	app.profilingDiagnostics = profilingDiagnostics

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
	if !response.Profiling.Requested || response.Profiling.Running {
		t.Fatalf("profiling diagnostics = %+v, want requested and not running", response.Profiling)
	}
	if response.Profiling.LastFailureAt == nil {
		t.Fatalf("profiling diagnostics = %+v, want failure timestamp", response.Profiling)
	}
	if response.Profiling.LastFailure != "profiling unavailable" {
		t.Fatalf("profiling failure = %q, want profiling unavailable", response.Profiling.LastFailure)
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

func TestRuntimeDiagnosticsReportHealthy(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: hacker-news
`)

	app.Config.Server.FrontendDiagnostics = true
	app.frontendDiagnostics = newFrontendRuntimeDiagnostics()
	app.Version = "v-test"
	app.ShortRevision = "abc1234"
	app.CreatedAt = time.Now().Add(-90 * time.Second)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/diagnostics/report", nil)

	app.router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, want %d; body=%q",
			recorder.Code,
			http.StatusOK,
			recorder.Body.String(),
		)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}

	body := recorder.Body.String()

	for _, want := range []string{
		"GLANCE RUNTIME DIAGNOSTICS",
		"Overall: HEALTHY",
		"APPLICATION",
		"Version:   v-test",
		"Revision:  abc1234",
		"Uptime:    1m",
		"WIDGET REFRESH",
		"CONFIGURATION",
		"PROFILING",
		"FRONTEND DIAGNOSTICS",
		"Diagnostics:    enabled",
		"no browser diagnostics received yet",
		"CURRENT PROBLEMS",
		"None",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("report missing %q:\n%s", want, body)
		}
	}
}

func TestRuntimeDiagnosticsReportShowsDegradedWidget(t *testing.T) {
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
		base.lastRefreshError = "connection refused"
		base.refreshFailureCount = 2
		base.refreshAttempts = 3
		base.refreshSuccesses = 1
		base.refreshFailures = 2
		base.refreshTelemetryMu.Unlock()
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/diagnostics/report", nil)
	app.router().ServeHTTP(recorder, request)

	body := recorder.Body.String()

	for _, want := range []string{
		"Overall: ATTENTION",
		"State:                 DEGRADED",
		"Failure class:",
		"Failure cause:         connection refused",
		"Consecutive failures:  2",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("report missing %q:\n%s", want, body)
		}
	}
}

func TestRuntimeDiagnosticsReportRecoveredFailureRemainsHealthy(t *testing.T) {
	now := time.Now()

	response := runtimeDiagnosticsResponse{
		GeneratedAt:    now,
		RefreshWidgets: 1,
		TotalAttempts:  2,
		TotalSuccesses: 1,
		TotalFailures:  1,
		Widgets: []widgetRefreshDiagnosticsResponse{
			{
				ID:          42,
				Type:        "test",
				Title:       "Recovered Widget",
				Attempts:    2,
				Successes:   1,
				Failures:    1,
				LastFailure: &now,
			},
		},
	}

	report := formatRuntimeDiagnosticsReport(
		response,
		runtimeDiagnosticsReportIdentity{},
		false,
		frontendRuntimeDiagnosticsSnapshot{},
	)

	if !strings.Contains(report, "Overall: HEALTHY") {
		t.Fatalf("recovered failure marked unhealthy:\n%s", report)
	}
	if !strings.Contains(report, "Recovered Widget") ||
		!strings.Contains(report, "State:         recovered") {
		t.Fatalf("recovered failure missing from history:\n%s", report)
	}
}

func TestRuntimeDiagnosticsReportShowsFrontendProblemHistory(t *testing.T) {
	store := newFrontendRuntimeDiagnostics()
	store.record([]frontendDiagnosticEvent{
		{
			Event:   "window_error",
			Page:    "sports",
			Session: "browser-1",
			Detail:  "script failed",
		},
	})

	response := runtimeDiagnosticsResponse{
		GeneratedAt: time.Now(),
	}

	report := formatRuntimeDiagnosticsReport(
		response,
		runtimeDiagnosticsReportIdentity{},
		true,
		store.snapshot(),
	)

	for _, want := range []string{
		"Overall: HEALTHY",
		"Total events:   1",
		"Problem events: 1",
		"window_error",
		"page=sports",
		"script failed",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestRuntimeDiagnosticsReportShowsActiveDiagnosticResults(t *testing.T) {
	store := newFrontendRuntimeDiagnostics()
	state := 1

	store.record([]frontendDiagnosticEvent{
		{
			Event:     "runtime_state",
			Page:      "sports",
			Session:   "browser-active",
			Detail:    "visibility=visible event_source=open",
			CommandID: 42,
			State:     &state,
			Metrics: map[string]float64{
				"pending":   1,
				"in_flight": 2,
			},
		},
	})

	report := formatRuntimeDiagnosticsReport(
		runtimeDiagnosticsResponse{GeneratedAt: time.Now()},
		runtimeDiagnosticsReportIdentity{},
		true,
		store.snapshot(),
	)

	for _, want := range []string{
		"RECENT ACTIVE DIAGNOSTIC RESULTS",
		"command=42  runtime_state",
		"page=sports",
		"session=browser-active",
		"state=1",
		"visibility=visible event_source=open",
		"metrics: in_flight=2 pending=1",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestRuntimeDiagnosticsReportRequiresAuthentication(t *testing.T) {
	app := newAuthTestApplication(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/diagnostics/report", nil)

	app.router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"status = %d, want %d",
			recorder.Code,
			http.StatusUnauthorized,
		)
	}
}

func TestHealthzReportsIdentityAndUptime(t *testing.T) {
	app := &application{
		Version:       "v-test",
		ShortRevision: "abc1234",
		CreatedAt:     time.Now().Add(-90 * time.Second),
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)

	app.handleHealthzRequest(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}

	body := recorder.Body.String()

	for _, want := range []string{
		"Glance OK",
		"Version: v-test",
		"Revision: abc1234",
		"Uptime: 1m",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("healthz missing %q: %q", want, body)
		}
	}
}
func TestRuntimeDiagnosticsReportShowsConfigurationRejection(t *testing.T) {
	now := time.Now()
	response := runtimeDiagnosticsResponse{
		GeneratedAt: now,
		Config: configRuntimeDiagnosticsResponse{
			LastReloadRejection: &configReloadRejectionResponse{
				At:      now,
				File:    "glance.yml",
				Line:    42,
				Message: "invalid configuration",
			},
		},
	}

	report := formatRuntimeDiagnosticsReport(
		response,
		runtimeDiagnosticsReportIdentity{},
		false,
		frontendRuntimeDiagnosticsSnapshot{},
	)

	for _, want := range []string{
		"Overall: ATTENTION",
		"Configuration reload",
		"State:         REJECTED",
		"File:          glance.yml",
		"Line:          42",
		"Error:         invalid configuration",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestRuntimeDiagnosticsReportShowsProfilingFailure(t *testing.T) {
	response := runtimeDiagnosticsResponse{
		GeneratedAt: time.Now(),
		Profiling: profilingRuntimeDiagnosticsResponse{
			Requested:   true,
			Running:     false,
			LastFailure: "profiling unavailable",
		},
	}

	report := formatRuntimeDiagnosticsReport(
		response,
		runtimeDiagnosticsReportIdentity{},
		false,
		frontendRuntimeDiagnosticsSnapshot{},
	)

	for _, want := range []string{
		"Overall: ATTENTION",
		"Profiling listener",
		"State:         FAILED",
		"Error:         profiling unavailable",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestRuntimeDiagnosticsReportFrontendDisabled(t *testing.T) {
	report := formatRuntimeDiagnosticsReport(
		runtimeDiagnosticsResponse{GeneratedAt: time.Now()},
		runtimeDiagnosticsReportIdentity{},
		false,
		frontendRuntimeDiagnosticsSnapshot{},
	)

	if !strings.Contains(report, "Diagnostics:    disabled") {
		t.Fatalf("disabled frontend diagnostics missing from report:\n%s", report)
	}
	if strings.Contains(report, "RECENT FRONTEND PROBLEMS") {
		t.Fatalf("disabled frontend diagnostics unexpectedly include recent frontend problems:\n%s", report)
	}
}
