package glance

import (
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
