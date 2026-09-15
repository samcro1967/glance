package glance

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newFrontendDiagnosticsTestApplication(enabled bool) *application {
	app := &application{}
	app.Config.Server.FrontendDiagnostics = enabled
	return app
}

func frontendDiagnosticsRequest(body string) *http.Request {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/frontend-diagnostics",
		strings.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestFrontendDiagnosticHTTPPerformanceHandlerPreservesResponse(t *testing.T) {
	app := newFrontendDiagnosticsTestApplication(true)

	handler := app.frontendDiagnosticHTTPPerformanceHandler(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Test", "preserved")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("body"))
		}),
	)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/example", nil),
	)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}

	if got := recorder.Header().Get("X-Test"); got != "preserved" {
		t.Fatalf("X-Test = %q, want %q", got, "preserved")
	}

	if got := recorder.Body.String(); got != "body" {
		t.Fatalf("body = %q, want %q", got, "body")
	}
}

func TestFrontendDiagnosticHTTPPerformanceHandlerExcludesStreamingAndIngestion(t *testing.T) {
	app := newFrontendDiagnosticsTestApplication(true)

	called := 0
	handler := app.frontendDiagnosticHTTPPerformanceHandler(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			called++
		}),
	)

	for _, path := range []string{
		"/api/live-updates",
		"/api/frontend-diagnostics",
	} {
		handler.ServeHTTP(
			httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, path, nil),
		)
	}

	if called != 2 {
		t.Fatalf("downstream calls = %d, want 2", called)
	}
}

func frontendPerformanceSnapshotRequest() *http.Request {
	return httptest.NewRequest(
		http.MethodPost,
		"/api/frontend-diagnostics/performance-snapshot",
		nil,
	)
}

func TestFrontendPerformanceSnapshotDisabled(t *testing.T) {
	app := newFrontendDiagnosticsTestApplication(false)
	app.liveUpdates = newLiveUpdateBroker()

	recorder := httptest.NewRecorder()
	app.handleFrontendPerformanceSnapshotRequest(
		recorder,
		frontendPerformanceSnapshotRequest(),
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestFrontendDiagnosticCommandsPublishExpectedCommand(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		command string
	}{
		{
			name:    "performance snapshot",
			path:    "/api/frontend-diagnostics/performance-snapshot",
			command: "performance_snapshot",
		},
		{
			name:    "long task capture",
			path:    "/api/frontend-diagnostics/long-task-capture",
			command: "long_task_capture",
		},
		{
			name:    "runtime state",
			path:    "/api/frontend-diagnostics/runtime-state",
			command: "runtime_state",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := newFrontendDiagnosticsTestApplication(true)
			app.liveUpdates = newLiveUpdateBroker()

			subscription, unsubscribe := app.liveUpdates.subscribe(nil)
			defer unsubscribe()

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, test.path, nil)
			app.router().ServeHTTP(recorder, request)

			if recorder.Code != http.StatusAccepted {
				t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusAccepted, recorder.Body.String())
			}

			commands := subscription.takeDiagnosticCommands()
			if len(commands) != 1 {
				t.Fatalf("got %d commands, want 1", len(commands))
			}

			if commands[0].ID == 0 {
				t.Fatal("command ID must be nonzero")
			}

			if commands[0].Command != test.command {
				t.Fatalf("command = %q, want %q", commands[0].Command, test.command)
			}

			wantBody := fmt.Sprintf(
				`{"id":%d,"command":"%s"}`,
				commands[0].ID,
				test.command,
			)
			if recorder.Body.String() != wantBody {
				t.Fatalf("body = %q, want %q", recorder.Body.String(), wantBody)
			}
		})
	}
}

func TestFrontendPerformanceSnapshotWithoutLiveUpdates(t *testing.T) {
	app := newFrontendDiagnosticsTestApplication(true)

	recorder := httptest.NewRecorder()
	app.handleFrontendPerformanceSnapshotRequest(
		recorder,
		frontendPerformanceSnapshotRequest(),
	)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf(
			"status = %d, want %d",
			recorder.Code,
			http.StatusServiceUnavailable,
		)
	}
}

func TestFrontendPerformanceSnapshotRequiresAuthentication(t *testing.T) {
	app := newAuthTestApplication(t)
	app.Config.Server.FrontendDiagnostics = true
	app.liveUpdates = newLiveUpdateBroker()

	subscription, unsubscribe := app.liveUpdates.subscribe(nil)
	defer unsubscribe()

	recorder := httptest.NewRecorder()
	request := frontendPerformanceSnapshotRequest()
	app.router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"status = %d, want %d",
			recorder.Code,
			http.StatusUnauthorized,
		)
	}

	if got := recorder.Body.String(); got != `{"error": "Unauthorized"}` {
		t.Fatalf("body = %q, want unauthorized JSON", got)
	}

	if commands := subscription.takeDiagnosticCommands(); len(commands) != 0 {
		t.Fatalf(
			"unauthorized request published %d diagnostic commands",
			len(commands),
		)
	}
}

func TestFrontendDiagnosticsDisabled(t *testing.T) {
	app := newFrontendDiagnosticsTestApplication(false)
	recorder := httptest.NewRecorder()

	app.handleFrontendDiagnosticsRequest(
		recorder,
		frontendDiagnosticsRequest(`{"events":[{"event":"page_setup_start"}]}`),
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestFrontendDiagnosticsAcceptsValidBatch(t *testing.T) {
	app := newFrontendDiagnosticsTestApplication(true)
	recorder := httptest.NewRecorder()

	app.handleFrontendDiagnosticsRequest(
		recorder,
		frontendDiagnosticsRequest(
			`{"events":[`+
				`{"event":"live_updates_open","page":"sports","session":"session-1","sequence":1,"state":1},`+
				`{"event":"widget_refresh_complete","page":"sports","session":"session-1","sequence":2,"widget":"123","status":200,"length":4096,"elapsed_ms":12.5}`+
				`]}`,
		),
	)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf(
			"status = %d, want %d; body = %q",
			recorder.Code,
			http.StatusNoContent,
			recorder.Body.String(),
		)
	}
}

func TestFrontendDiagnosticsRequiresJSONContentType(t *testing.T) {
	app := newFrontendDiagnosticsTestApplication(true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/frontend-diagnostics",
		strings.NewReader(`{"events":[{"event":"page_setup_start"}]}`),
	)

	app.handleFrontendDiagnosticsRequest(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestFrontendDiagnosticsRejectsMalformedPayloads(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "empty events",
			body: `{"events":[]}`,
		},
		{
			name: "missing events",
			body: `{}`,
		},
		{
			name: "unknown batch field",
			body: `{"events":[{"event":"page_setup_start"}],"unexpected":true}`,
		},
		{
			name: "unknown event field",
			body: `{"events":[{"event":"page_setup_start","unexpected":true}]}`,
		},
		{
			name: "invalid event name",
			body: `{"events":[{"event":"Page Setup Start"}]}`,
		},
		{
			name: "negative elapsed",
			body: `{"events":[{"event":"page_setup_start","elapsed_ms":-1}]}`,
		},
		{
			name: "newline control character",
			body: "{\"events\":[{\"event\":\"page_setup_start\",\"detail\":\"first\\nsecond\"}]}",
		},
		{
			name: "null control character",
			body: "{\"events\":[{\"event\":\"page_setup_start\",\"detail\":\"first\\u0000second\"}]}",
		},
		{
			name: "delete control character",
			body: "{\"events\":[{\"event\":\"page_setup_start\",\"detail\":\"first\\u007fsecond\"}]}",
		},
		{
			name: "negative length",
			body: `{"events":[{"event":"page_setup_start","length":-1}]}`,
		},
		{
			name: "negative state",
			body: `{"events":[{"event":"live_updates_open","state":-1}]}`,
		},
		{
			name: "state too large",
			body: `{"events":[{"event":"live_updates_open","state":3}]}`,
		},
		{
			name: "status too large",
			body: `{"events":[{"event":"page_content_fetch_response","status":1000}]}`,
		},
		{
			name: "session control character",
			body: "{\"events\":[{\"event\":\"page_setup_start\",\"session\":\"first\\nsecond\"}]}",
		},
		{
			name: "multiple json values",
			body: `{"events":[{"event":"page_setup_start"}]} {"events":[{"event":"page_setup_end"}]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := newFrontendDiagnosticsTestApplication(true)
			recorder := httptest.NewRecorder()

			app.handleFrontendDiagnosticsRequest(
				recorder,
				frontendDiagnosticsRequest(test.body),
			)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf(
					"status = %d, want %d; body = %q",
					recorder.Code,
					http.StatusBadRequest,
					recorder.Body.String(),
				)
			}
		})
	}
}

func TestFrontendDiagnosticsRejectsTooManyEvents(t *testing.T) {
	events := make([]string, frontendDiagnosticsMaxEvents+1)
	for i := range events {
		events[i] = `{"event":"page_setup_start"}`
	}

	body := `{"events":[` + strings.Join(events, ",") + `]}`

	app := newFrontendDiagnosticsTestApplication(true)
	recorder := httptest.NewRecorder()

	app.handleFrontendDiagnosticsRequest(
		recorder,
		frontendDiagnosticsRequest(body),
	)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestFrontendDiagnosticsRejectsOversizedPayload(t *testing.T) {
	app := newFrontendDiagnosticsTestApplication(true)
	recorder := httptest.NewRecorder()

	body := `{"events":[{"event":"page_setup_start","detail":"` +
		strings.Repeat("x", frontendDiagnosticsMaxRequestBytes) +
		`"}]}`
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/frontend-diagnostics",
		strings.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")

	app.handleFrontendDiagnosticsRequest(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf(
			"status = %d, want %d; body = %q",
			recorder.Code,
			http.StatusRequestEntityTooLarge,
			recorder.Body.String(),
		)
	}
}

func frontendDiagnosticsIntPointer(value int) *int {
	return &value
}

func TestValidFrontendDiagnosticEvent(t *testing.T) {
	tests := []struct {
		name  string
		event frontendDiagnosticEvent
		valid bool
	}{
		{
			name:  "minimal",
			event: frontendDiagnosticEvent{Event: "page_setup_start"},
			valid: true,
		},
		{
			name: "complete",
			event: frontendDiagnosticEvent{
				Event:     "widget_refresh_complete",
				Page:      "sports",
				Session:   "550e8400-e29b-41d4-a716-446655440000",
				Widget:    "123",
				Detail:    "refresh complete",
				Sequence:  42,
				ElapsedMS: 8.25,
				Status:    200,
				Length:    frontendDiagnosticsIntPointer(4096),
				State:     frontendDiagnosticsIntPointer(1),
			},
			valid: true,
		},

		{
			name:  "empty event",
			event: frontendDiagnosticEvent{},
			valid: false,
		},
		{
			name:  "uppercase event",
			event: frontendDiagnosticEvent{Event: "Page_setup"},
			valid: false,
		},
		{
			name: "event too long",
			event: frontendDiagnosticEvent{
				Event: strings.Repeat("a", frontendDiagnosticsMaxEventLength+1),
			},
			valid: false,
		},
		{
			name: "detail too long",
			event: frontendDiagnosticEvent{
				Event:  "page_setup_start",
				Detail: strings.Repeat("a", frontendDiagnosticsMaxDetailLength+1),
			},
			valid: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validFrontendDiagnosticEvent(test.event); got != test.valid {
				t.Fatalf("valid = %v, want %v", got, test.valid)
			}
		})
	}
}

func TestValidFrontendDiagnosticMetrics(t *testing.T) {
	tests := []struct {
		name    string
		metrics map[string]float64
		valid   bool
	}{
		{
			name: "valid",
			metrics: map[string]float64{
				"elements":    15980,
				"duration_ms": 12.5,
			},
			valid: true,
		},
		{
			name:    "empty name",
			metrics: map[string]float64{"": 1},
			valid:   false,
		},
		{
			name:    "invalid name",
			metrics: map[string]float64{"Duration-MS": 1},
			valid:   false,
		},
		{
			name:    "negative value",
			metrics: map[string]float64{"duration_ms": -1},
			valid:   false,
		},
		{
			name:    "nan",
			metrics: map[string]float64{"duration_ms": math.NaN()},
			valid:   false,
		},
		{
			name:    "positive infinity",
			metrics: map[string]float64{"duration_ms": math.Inf(1)},
			valid:   false,
		},
		{
			name:    "negative infinity",
			metrics: map[string]float64{"duration_ms": math.Inf(-1)},
			valid:   false,
		},
	}

	tooMany := make(map[string]float64)
	for i := 0; i < 33; i++ {
		tooMany[fmt.Sprintf("metric_%d", i)] = float64(i)
	}
	tests = append(tests, struct {
		name    string
		metrics map[string]float64
		valid   bool
	}{
		name:    "too many",
		metrics: tooMany,
		valid:   false,
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validFrontendDiagnosticMetrics(test.metrics); got != test.valid {
				t.Fatalf(
					"validFrontendDiagnosticMetrics() = %v, want %v",
					got,
					test.valid,
				)
			}
		})
	}
}

func TestFrontendRuntimeDiagnosticsRecordsValidatedBatch(t *testing.T) {
	store := newFrontendRuntimeDiagnostics()

	store.record([]frontendDiagnosticEvent{
		{
			Event:   "page_setup_complete",
			Page:    "sports",
			Session: "session-1",
		},
		{
			Event:   "widget_refresh_error",
			Page:    "sports",
			Session: "session-1",
			Widget:  "42",
			Detail:  "connection refused",
			Metrics: map[string]float64{"duration_ms": 125},
		},
	})

	snapshot := store.snapshot()

	if snapshot.TotalEvents != 2 {
		t.Fatalf("total events = %d, want 2", snapshot.TotalEvents)
	}
	if snapshot.TotalProblemEvents != 1 {
		t.Fatalf("problem events = %d, want 1", snapshot.TotalProblemEvents)
	}
	if snapshot.LastPage != "sports" || snapshot.LastSession != "session-1" {
		t.Fatalf("unexpected last context: %+v", snapshot)
	}
	if snapshot.ProblemCounts["widget_refresh_error"] != 1 {
		t.Fatalf("unexpected problem counts: %+v", snapshot.ProblemCounts)
	}
	if len(snapshot.RecentProblems) != 1 {
		t.Fatalf("recent problems = %d, want 1", len(snapshot.RecentProblems))
	}
	if snapshot.RecentProblems[0].Event.Detail != "connection refused" {
		t.Fatalf("unexpected recent problem: %+v", snapshot.RecentProblems[0])
	}

	snapshot.RecentProblems[0].Event.Metrics["duration_ms"] = 999

	second := store.snapshot()
	if second.RecentProblems[0].Event.Metrics["duration_ms"] != 125 {
		t.Fatal("snapshot mutation changed retained metrics")
	}
}

func TestFrontendRuntimeDiagnosticsProblemClassification(t *testing.T) {
	problems := []string{
		"window_error",
		"unhandled_rejection",
		"page_initialize_error",
		"page_content_load_error",
		"widget_refresh_error",
		"widget_replacement_invalid",
		"widget_current_missing",
		"live_updates_error",
		"live_update_invalid",
		"diagnostic_command_unsupported",
		"long_task_capture_unsupported",
	}

	for _, event := range problems {
		if !frontendDiagnosticIsProblem(event) {
			t.Errorf("%q should be classified as a problem", event)
		}
	}

	normal := []string{
		"page_setup_complete",
		"widget_refresh_complete",
		"live_updates_open",
		"live_updates_close",
		"network_offline",
		"page_hide",
	}

	for _, event := range normal {
		if frontendDiagnosticIsProblem(event) {
			t.Errorf("%q should not be classified as a problem", event)
		}
	}
}

func TestFrontendRuntimeDiagnosticsBoundsRecentProblems(t *testing.T) {
	store := newFrontendRuntimeDiagnostics()

	for i := 0; i < frontendDiagnosticsRecentProblemLimit+5; i++ {
		store.record([]frontendDiagnosticEvent{
			{
				Event:    "window_error",
				Sequence: uint64(i + 1),
			},
		})
	}

	snapshot := store.snapshot()

	if snapshot.TotalProblemEvents != frontendDiagnosticsRecentProblemLimit+5 {
		t.Fatalf(
			"problem events = %d, want %d",
			snapshot.TotalProblemEvents,
			frontendDiagnosticsRecentProblemLimit+5,
		)
	}
	if len(snapshot.RecentProblems) != frontendDiagnosticsRecentProblemLimit {
		t.Fatalf(
			"recent problems = %d, want %d",
			len(snapshot.RecentProblems),
			frontendDiagnosticsRecentProblemLimit,
		)
	}
	if snapshot.RecentProblems[0].Event.Sequence != 6 {
		t.Fatalf(
			"oldest retained sequence = %d, want 6",
			snapshot.RecentProblems[0].Event.Sequence,
		)
	}
}

func TestFrontendRuntimeDiagnosticsRetainsActiveResults(t *testing.T) {
	store := newFrontendRuntimeDiagnostics()

	store.record([]frontendDiagnosticEvent{
		{
			Event:     "runtime_state",
			Page:      "sports",
			Session:   "session-active",
			CommandID: 42,
			Metrics:   map[string]float64{"in_flight": 2},
		},
		{
			Event:     "diagnostic_command_received",
			CommandID: 42,
		},
		{
			Event:   "runtime_state",
			Metrics: map[string]float64{"in_flight": 99},
		},
	})

	snapshot := store.snapshot()

	if len(snapshot.RecentActiveResults) != 1 {
		t.Fatalf("recent active results = %d, want 1", len(snapshot.RecentActiveResults))
	}

	result := snapshot.RecentActiveResults[0]
	if result.Event.CommandID != 42 {
		t.Fatalf("command ID = %d, want 42", result.Event.CommandID)
	}
	if result.Event.Event != "runtime_state" {
		t.Fatalf("event = %q, want runtime_state", result.Event.Event)
	}
	if result.Event.Page != "sports" || result.Event.Session != "session-active" {
		t.Fatalf("unexpected active result context: %+v", result.Event)
	}
	if result.Event.Metrics["in_flight"] != 2 {
		t.Fatalf("unexpected active result metrics: %+v", result.Event.Metrics)
	}

	snapshot.RecentActiveResults[0].Event.Metrics["in_flight"] = 999

	second := store.snapshot()
	if second.RecentActiveResults[0].Event.Metrics["in_flight"] != 2 {
		t.Fatal("snapshot mutation changed retained active result metrics")
	}
}

func TestFrontendRuntimeDiagnosticsActiveResultClassification(t *testing.T) {
	active := []string{
		"performance_snapshot",
		"navigation_snapshot",
		"resource_snapshot",
		"memory_snapshot",
		"long_task_capture_start",
		"long_task_capture_complete",
		"long_task_capture_unsupported",
		"long_task_capture_error",
		"runtime_state",
	}

	for _, event := range active {
		if !frontendDiagnosticIsActiveResult(frontendDiagnosticEvent{
			Event:     event,
			CommandID: 1,
		}) {
			t.Errorf("%q should be classified as an active result", event)
		}
	}

	if frontendDiagnosticIsActiveResult(frontendDiagnosticEvent{
		Event: "runtime_state",
	}) {
		t.Error("passive runtime_state should not be classified as an active result")
	}

	if frontendDiagnosticIsActiveResult(frontendDiagnosticEvent{
		Event:     "diagnostic_command_received",
		CommandID: 1,
	}) {
		t.Error("diagnostic command receipt should not be classified as an active result")
	}
}

func TestFrontendRuntimeDiagnosticsBoundsRecentActiveResults(t *testing.T) {
	store := newFrontendRuntimeDiagnostics()

	for i := 0; i < frontendDiagnosticsRecentActiveResultLimit+5; i++ {
		store.record([]frontendDiagnosticEvent{
			{
				Event:     "runtime_state",
				CommandID: uint64(i + 1),
			},
		})
	}

	snapshot := store.snapshot()

	if len(snapshot.RecentActiveResults) != frontendDiagnosticsRecentActiveResultLimit {
		t.Fatalf(
			"recent active results = %d, want %d",
			len(snapshot.RecentActiveResults),
			frontendDiagnosticsRecentActiveResultLimit,
		)
	}

	if snapshot.RecentActiveResults[0].Event.CommandID != 6 {
		t.Fatalf(
			"oldest retained command ID = %d, want 6",
			snapshot.RecentActiveResults[0].Event.CommandID,
		)
	}
}

func TestFrontendDiagnosticsValidBatchRetainsActiveResult(t *testing.T) {
	app := newFrontendDiagnosticsTestApplication(true)
	app.frontendDiagnostics = newFrontendRuntimeDiagnostics()

	recorder := httptest.NewRecorder()
	app.handleFrontendDiagnosticsRequest(
		recorder,
		frontendDiagnosticsRequest(
			`{"events":[{"event":"runtime_state","page":"home","session":"session-3","command_id":77,"metrics":{"in_flight":0,"pending":0}}]}`,
		),
	)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}

	snapshot := app.frontendDiagnostics.snapshot()
	if len(snapshot.RecentActiveResults) != 1 {
		t.Fatalf("recent active results = %d, want 1", len(snapshot.RecentActiveResults))
	}

	result := snapshot.RecentActiveResults[0].Event
	if result.CommandID != 77 || result.Page != "home" || result.Session != "session-3" {
		t.Fatalf("unexpected retained active result: %+v", result)
	}
}

func TestFrontendDiagnosticsInvalidBatchDoesNotUpdateRuntimeDiagnostics(t *testing.T) {
	app := newFrontendDiagnosticsTestApplication(true)
	app.frontendDiagnostics = newFrontendRuntimeDiagnostics()

	recorder := httptest.NewRecorder()
	app.handleFrontendDiagnosticsRequest(
		recorder,
		frontendDiagnosticsRequest(
			`{"events":[{"event":"window_error"},{"event":""}]}`,
		),
	)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	snapshot := app.frontendDiagnostics.snapshot()
	if snapshot.TotalEvents != 0 || snapshot.TotalProblemEvents != 0 {
		t.Fatalf("invalid batch updated diagnostics: %+v", snapshot)
	}
}

func TestFrontendDiagnosticsValidBatchUpdatesRuntimeDiagnostics(t *testing.T) {
	app := newFrontendDiagnosticsTestApplication(true)
	app.frontendDiagnostics = newFrontendRuntimeDiagnostics()

	recorder := httptest.NewRecorder()
	app.handleFrontendDiagnosticsRequest(
		recorder,
		frontendDiagnosticsRequest(
			`{"events":[`+
				`{"event":"page_setup_complete","page":"home","session":"session-2"},`+
				`{"event":"window_error","page":"home","session":"session-2","detail":"boom"}`+
				`]}`,
		),
	)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}

	snapshot := app.frontendDiagnostics.snapshot()
	if snapshot.TotalEvents != 2 || snapshot.TotalProblemEvents != 1 {
		t.Fatalf("unexpected diagnostics snapshot: %+v", snapshot)
	}
}
