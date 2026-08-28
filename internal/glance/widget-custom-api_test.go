package glance

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestCustomAPIRequest(t *testing.T, url string) *CustomAPIRequest {
	t.Helper()

	req := &CustomAPIRequest{
		URL: url,
	}

	if err := req.initialize(); err != nil {
		t.Fatalf("initialize request: %v", err)
	}

	return req
}

func TestFetchCustomAPIResponseEmptyNon2xxIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	req := newTestCustomAPIRequest(t, server.URL)

	_, err := fetchCustomAPIResponse(context.Background(), req)
	if err == nil {
		t.Fatal("expected empty 404 response to return an error")
	}

	if !strings.Contains(err.Error(), "404 Not Found") {
		t.Fatalf("expected 404 Not Found error, got %q", err)
	}
}

func TestFetchCustomAPIResponseNon2xxJSONRemainsAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	defer server.Close()

	req := newTestCustomAPIRequest(t, server.URL)

	data, err := fetchCustomAPIResponse(context.Background(), req)
	if err != nil {
		t.Fatalf("expected non-2xx JSON response to remain available, got error: %v", err)
	}

	if data.Response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", data.Response.StatusCode)
	}

	if got := data.JSON.String("message"); got != "not found" {
		t.Fatalf("expected JSON body to remain available, got %q", got)
	}
}

func TestCustomAPIWidgetStaleFallbackAndRecovery(t *testing.T) {
	responseBody := `{"value":"first"}`
	statusCode := http.StatusOK

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)

		if responseBody != "" {
			_, _ = w.Write([]byte(responseBody))
		}
	}))
	defer server.Close()

	req := newTestCustomAPIRequest(t, server.URL)

	compiledTemplate, err := template.New("").Funcs(customAPITemplateFuncs).Parse(
		`<div>{{ .JSON.String "value" }}</div>`,
	)
	if err != nil {
		t.Fatalf("compile template: %v", err)
	}

	widget := &customAPIWidget{
		CustomAPIRequest: req,
		compiledTemplate: compiledTemplate,
	}

	// Initial successful refresh establishes the last-known-good content.
	widget.update(context.Background())

	if widget.Stale {
		t.Fatal("widget should not be stale after successful refresh")
	}

	if widget.LastSuccessfulUpdate.IsZero() {
		t.Fatal("expected successful refresh timestamp")
	}

	if !strings.Contains(string(widget.CompiledHTML), "first") {
		t.Fatalf("expected initial content, got %q", widget.CompiledHTML)
	}

	firstHTML := widget.CompiledHTML
	firstSuccessfulUpdate := widget.LastSuccessfulUpdate

	// A later empty 404 must preserve the last-known-good content.
	statusCode = http.StatusNotFound
	responseBody = ""

	widget.update(context.Background())

	if !widget.Stale {
		t.Fatal("expected widget to be stale after failed refresh")
	}

	if widget.CompiledHTML != firstHTML {
		t.Fatalf(
			"failed refresh replaced last-known-good content: got %q, want %q",
			widget.CompiledHTML,
			firstHTML,
		)
	}

	if !widget.LastSuccessfulUpdate.Equal(firstSuccessfulUpdate) {
		t.Fatalf(
			"failed refresh changed last successful timestamp: got %v, want %v",
			widget.LastSuccessfulUpdate,
			firstSuccessfulUpdate,
		)
	}

	if widget.Error == nil {
		t.Fatal("expected widget error after failed refresh")
	}

	// A successful retry replaces the content and clears stale/error state.
	time.Sleep(time.Millisecond)

	statusCode = http.StatusOK
	responseBody = `{"value":"second"}`

	widget.update(context.Background())

	if widget.Stale {
		t.Fatal("expected stale state to clear after successful refresh")
	}

	if widget.Error != nil {
		t.Fatalf("expected error to clear after recovery, got %v", widget.Error)
	}

	if !strings.Contains(string(widget.CompiledHTML), "second") {
		t.Fatalf("expected recovered content, got %q", widget.CompiledHTML)
	}

	if !widget.LastSuccessfulUpdate.After(firstSuccessfulUpdate) {
		t.Fatalf(
			"expected successful timestamp to advance: first=%v recovered=%v",
			firstSuccessfulUpdate,
			widget.LastSuccessfulUpdate,
		)
	}
}

func TestCustomAPIWidgetInitialFailureHasNoStaleFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	req := newTestCustomAPIRequest(t, server.URL)

	compiledTemplate, err := template.New("").Funcs(customAPITemplateFuncs).Parse(
		`<div>{{ .JSON.String "value" }}</div>`,
	)
	if err != nil {
		t.Fatalf("compile template: %v", err)
	}

	widget := &customAPIWidget{
		CustomAPIRequest: req,
		compiledTemplate: compiledTemplate,
	}

	widget.update(context.Background())

	if widget.Stale {
		t.Fatal("initial failure must not be marked stale without last-known-good content")
	}

	if !widget.LastSuccessfulUpdate.IsZero() {
		t.Fatalf(
			"initial failure unexpectedly recorded successful timestamp: %v",
			widget.LastSuccessfulUpdate,
		)
	}

	if widget.Error == nil {
		t.Fatal("expected widget error after initial failure")
	}
}
