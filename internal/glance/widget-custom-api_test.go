package glance

import (
	"context"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type customAPIRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f customAPIRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

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

func TestFetchCustomAPIResponseSanitizesTransportErrorURL(t *testing.T) {
	const (
		requestURL = "https://example.invalid/data?token=fake-secret-value"
		secret     = "fake-secret-value"
	)

	originalTransport := defaultHTTPClient.Transport
	defaultHTTPClient.Transport = customAPIRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return nil, &url.Error{
			Op:  "Get",
			URL: request.URL.String(),
			Err: errors.New("synthetic transport failure"),
		}
	})
	t.Cleanup(func() {
		defaultHTTPClient.Transport = originalTransport
	})

	req := newTestCustomAPIRequest(t, requestURL)

	_, err := fetchCustomAPIResponse(context.Background(), req)
	if err == nil {
		t.Fatal("expected transport failure")
	}

	if !strings.Contains(err.Error(), "synthetic transport failure") {
		t.Fatalf("expected actionable transport cause, got %q", err)
	}

	if strings.Contains(err.Error(), secret) {
		t.Fatalf("transport error exposed secret query value: %q", err)
	}

	if strings.Contains(err.Error(), requestURL) {
		t.Fatalf("transport error exposed request URL: %q", err)
	}

	if strings.Contains(err.Error(), "token=") {
		t.Fatalf("transport error exposed sensitive query parameter: %q", err)
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

func TestFetchCustomAPIResponseCancellationStopsInFlightRequest(t *testing.T) {
	requestStarted := make(chan struct{})
	requestCancelled := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)

		select {
		case <-r.Context().Done():
			close(requestCancelled)
		case <-time.After(time.Second):
			t.Error("server request context was not cancelled")
		}
	}))
	defer server.Close()

	req := newTestCustomAPIRequest(t, server.URL)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := fetchCustomAPIResponse(ctx, req)
		done <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}

	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancelled request to return an error")
		}

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("fetchCustomAPIResponse did not return after cancellation")
	}

	select {
	case <-requestCancelled:
	case <-time.After(time.Second):
		t.Fatal("server did not observe request cancellation")
	}
}

func TestFetchAndRenderCustomAPIRequestCancellationStopsSubrequests(t *testing.T) {
	const requestCount = 3

	requestStarted := make(chan struct{}, requestCount)
	requestCancelled := make(chan struct{}, requestCount)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestStarted <- struct{}{}

		select {
		case <-r.Context().Done():
			requestCancelled <- struct{}{}
		case <-time.After(time.Second):
			t.Error("server request context was not cancelled")
		}
	}))
	defer server.Close()

	primaryReq := newTestCustomAPIRequest(t, server.URL+"/primary")
	subReqs := map[string]*CustomAPIRequest{
		"first":  newTestCustomAPIRequest(t, server.URL+"/first"),
		"second": newTestCustomAPIRequest(t, server.URL+"/second"),
	}

	compiledTemplate, err := template.New("").Funcs(customAPITemplateFuncs).Parse(
		`{{ .JSON.String "value" }}`,
	)
	if err != nil {
		t.Fatalf("compile template: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := fetchAndRenderCustomAPIRequest(
			ctx,
			primaryReq,
			subReqs,
			nil,
			compiledTemplate,
		)
		done <- err
	}()

	for range requestCount {
		select {
		case <-requestStarted:
		case <-time.After(time.Second):
			t.Fatal("not all custom API requests started")
		}
	}

	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancelled refresh to return an error")
		}

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("fetchAndRenderCustomAPIRequest did not return after cancellation")
	}

	for range requestCount {
		select {
		case <-requestCancelled:
		case <-time.After(time.Second):
			t.Fatal("server did not observe cancellation for every request")
		}
	}
}

func TestParseCustomAPITemplateErrorLine(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    int
	}{
		{
			name:    "first line",
			message: "template: :1: unclosed action",
			want:    1,
		},
		{
			name:    "later line",
			message: "template: :17: function \"missing\" not defined",
			want:    17,
		},
		{
			name:    "unrecognized format",
			message: "some other template error",
			want:    0,
		},
		{
			name:    "zero line",
			message: "template: :0: invalid",
			want:    0,
		},
		{
			name:    "negative line",
			message: "template: :-1: invalid",
			want:    0,
		},
		{
			name:    "missing detail",
			message: "template: :3:",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseCustomAPITemplateErrorLine(tt.message); got != tt.want {
				t.Fatalf(
					"parseCustomAPITemplateErrorLine(%q) = %d, want %d",
					tt.message,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestCustomAPIWidgetTemplateParseErrorCarriesLineAndCause(t *testing.T) {
	widget := &customAPIWidget{
		CustomAPIRequest: &CustomAPIRequest{},
		Template:         "first line\nsecond line\n{{ doesNotExist }}",
	}

	err := widget.initialize()
	if err == nil {
		t.Fatal("expected invalid custom API template to fail initialization")
	}

	var parseErr *customAPITemplateParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected customAPITemplateParseError, got %T: %v", err, err)
	}

	if parseErr.line != 3 {
		t.Fatalf("template error line = %d, want 3", parseErr.line)
	}

	if parseErr.Unwrap() == nil {
		t.Fatal("expected template parse error to preserve underlying cause")
	}

	if !strings.HasPrefix(err.Error(), "parsing template: template: :3:") {
		t.Fatalf("unexpected error text: %q", err)
	}
}

func TestCustomAPIStringTemplateFunctions(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     string
	}{
		{name: "toLower", template: `{{ toLower "HELLO Über" }}`, want: "hello über"},
		{name: "toUpper", template: `{{ toUpper "hello über" }}`, want: "HELLO ÜBER"},
		{name: "equalFold", template: `{{ equalFold "Go" "gO" }}`, want: "true"},
		{name: "equalFold unicode", template: `{{ equalFold "ÜBER" "über" }}`, want: "true"},
		{name: "contains pipeline", template: `{{ "sports-feed" | contains "sports" }}`, want: "true"},
		{name: "hasPrefix pipeline", template: `{{ "sports-feed" | hasPrefix "sports" }}`, want: "true"},
		{name: "hasSuffix pipeline", template: `{{ "sports-feed" | hasSuffix "feed" }}`, want: "true"},
		{name: "split and index", template: `{{ index ("one,two,three" | split ",") 1 }}`, want: "two"},
		{name: "split and join pipeline", template: `{{ "one,two,three" | split "," | join " / " }}`, want: "one / two / three"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled, err := template.New("").Funcs(customAPITemplateFuncs).Parse(tt.template)
			if err != nil {
				t.Fatalf("parse template: %v", err)
			}

			var output strings.Builder
			if err := compiled.Execute(&output, nil); err != nil {
				t.Fatalf("execute template: %v", err)
			}

			if got := output.String(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCustomAPISafeHTMLEscapingBoundary(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     string
	}{
		{
			name:     "HTML is escaped by default",
			template: `{{ "<script>alert(1)</script><strong>trusted</strong>" }}`,
			want:     `&lt;script&gt;alert(1)&lt;/script&gt;&lt;strong&gt;trusted&lt;/strong&gt;`,
		},
		{
			name:     "safeHTML preserves trusted HTML",
			template: `{{ "<script>alert(1)</script><strong>trusted</strong>" | safeHTML }}`,
			want:     `<script>alert(1)</script><strong>trusted</strong>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled, err := template.New("").Funcs(customAPITemplateFuncs).Parse(tt.template)
			if err != nil {
				t.Fatalf("parse template: %v", err)
			}

			var output strings.Builder
			if err := compiled.Execute(&output, nil); err != nil {
				t.Fatalf("execute template: %v", err)
			}

			if got := output.String(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCustomAPIDynamicRequestMethodAndBody(t *testing.T) {
	newRequest := customAPITemplateFuncs["newRequest"].(func(string) *CustomAPIRequest)
	withMethod := customAPITemplateFuncs["withMethod"].(func(string, *CustomAPIRequest) *CustomAPIRequest)
	withStringBody := customAPITemplateFuncs["withStringBody"].(func(string, *CustomAPIRequest) *CustomAPIRequest)

	tests := []struct {
		name       string
		build      func() *CustomAPIRequest
		wantMethod string
		wantBody   string
	}{
		{
			name: "default get",
			build: func() *CustomAPIRequest {
				return newRequest("https://example.com")
			},
			wantMethod: http.MethodGet,
		},
		{
			name: "string body defaults to post",
			build: func() *CustomAPIRequest {
				return withStringBody("payload", newRequest("https://example.com"))
			},
			wantMethod: http.MethodPost,
			wantBody:   "payload",
		},
		{
			name: "explicit put with body",
			build: func() *CustomAPIRequest {
				req := newRequest("https://example.com")
				req = withMethod("PUT", req)
				return withStringBody("payload", req)
			},
			wantMethod: http.MethodPut,
			wantBody:   "payload",
		},
		{
			name: "explicit delete without body",
			build: func() *CustomAPIRequest {
				return withMethod("DELETE", newRequest("https://example.com"))
			},
			wantMethod: http.MethodDelete,
		},
		{
			name: "lowercase method normalized",
			build: func() *CustomAPIRequest {
				return withMethod("patch", newRequest("https://example.com"))
			},
			wantMethod: http.MethodPatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.build()
			if err := req.initialize(); err != nil {
				t.Fatalf("initialize request: %v", err)
			}

			if got := req.httpRequest.Method; got != tt.wantMethod {
				t.Fatalf("method = %q, want %q", got, tt.wantMethod)
			}

			if tt.wantBody != "" {
				body, err := io.ReadAll(req.httpRequest.Body)
				if err != nil {
					t.Fatalf("read body: %v", err)
				}
				if got := string(body); got != tt.wantBody {
					t.Fatalf("body = %q, want %q", got, tt.wantBody)
				}
			}
		})
	}
}

func TestCustomAPIWithMethodTemplatePipeline(t *testing.T) {
	compiled, err := template.New("").Funcs(customAPITemplateFuncs).Parse(
		`{{ $request := newRequest "https://example.com" | withMethod "PUT" | withStringBody "payload" }}{{ $request.Method }}`,
	)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}

	var output strings.Builder
	if err := compiled.Execute(&output, nil); err != nil {
		t.Fatalf("execute template: %v", err)
	}

	if got := output.String(); got != "PUT" {
		t.Fatalf("method = %q, want %q", got, "PUT")
	}
}
