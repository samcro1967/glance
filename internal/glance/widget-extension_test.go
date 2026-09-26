package glance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchExtensionCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	serverRequestCanceled := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)

		select {
		case <-r.Context().Done():
			close(serverRequestCanceled)
		case <-time.After(2 * time.Second):
			t.Error("server request context was not canceled")
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	result := make(chan error, 1)
	go func() {
		_, err := fetchExtension(ctx, extensionRequestOptions{
			URL: server.URL,
		})
		result <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("extension request did not reach server")
	}

	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected canceled extension request to return an error")
		}

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("extension request did not stop after context cancellation")
	}

	select {
	case <-serverRequestCanceled:
	case <-time.After(time.Second):
		t.Fatal("server did not observe extension request cancellation")
	}
}

func TestFetchExtensionSendsConfiguredHeadersAndBasicAuth(t *testing.T) {
	const (
		username = "extension-user"
		password = "extension-pass"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Extension-Test"); got != "present" {
			t.Errorf("X-Extension-Test = %q, want present", got)
		}

		gotUsername, gotPassword, ok := r.BasicAuth()
		if !ok {
			t.Error("extension request did not contain Basic Authentication")
		} else {
			if gotUsername != username {
				t.Errorf("username = %q, want %q", gotUsername, username)
			}
			if gotPassword != password {
				t.Errorf("password = %q, want %q", gotPassword, password)
			}
		}

		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<div>extension</div>"))
	}))
	defer server.Close()

	_, err := fetchExtension(context.Background(), extensionRequestOptions{
		URL: server.URL,
		Headers: map[string]string{
			"X-Extension-Test": "present",
		},
		BasicAuthUsername: username,
		BasicAuthPassword: password,
	})
	if err != nil {
		t.Fatalf("fetchExtension: %v", err)
	}
}

func TestExtensionWidgetFailedRefreshPreservesLastKnownGoodContent(t *testing.T) {
	var fail atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			requestCtx := r.Context()
			<-requestCtx.Done()
			return
		}

		w.Header().Set(extensionHeaderTitle, "Working Extension")
		w.Header().Set(extensionHeaderTitleURL, "https://example.com/working")
		w.Header().Set(extensionHeaderContentType, "html")
		_, _ = w.Write([]byte("<div>last-known-good</div>"))
	}))
	defer server.Close()

	widget := &extensionWidget{
		URL:       server.URL,
		Timeout:   durationField(10 * time.Millisecond),
		AllowHtml: true,
	}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize extension widget: %v", err)
	}

	widget.update(context.Background())

	if widget.Error != nil {
		t.Fatalf("successful refresh set error: %v", widget.Error)
	}

	if widget.Extension.Title != "Working Extension" {
		t.Fatalf("title = %q, want Working Extension", widget.Extension.Title)
	}

	if string(widget.Extension.Content) != "<div>last-known-good</div>" {
		t.Fatalf(
			"content = %q, want last-known-good content",
			widget.Extension.Content,
		)
	}

	previousExtension := widget.Extension
	previousHTML := widget.cachedHTML

	fail.Store(true)
	widget.update(context.Background())

	if widget.Error == nil {
		t.Fatal("failed refresh should set widget error")
	}

	if widget.Extension != previousExtension {
		t.Fatalf(
			"failed refresh replaced last-known-good extension:\n got: %#v\nwant: %#v",
			widget.Extension,
			previousExtension,
		)
	}

	if widget.cachedHTML != previousHTML {
		t.Fatal("failed refresh replaced last-known-good rendered HTML")
	}

	if !strings.Contains(string(widget.cachedHTML), "last-known-good") {
		t.Fatalf(
			"cached HTML no longer contains last-known-good content: %q",
			widget.cachedHTML,
		)
	}

	if !widget.refreshDegraded {
		t.Fatal("failed refresh should mark widget degraded")
	}

	if widget.updateRetriedTimes != 1 {
		t.Fatalf(
			"retry attempts = %d, want 1 for timeout failure",
			widget.updateRetriedTimes,
		)
	}

	if widget.refreshFailureClass != refreshFailureTransient {
		t.Fatalf(
			"failure class = %q, want %q",
			widget.refreshFailureClass,
			refreshFailureTransient,
		)
	}
}

func TestFetchExtensionHTTPStatusPreservesErrorIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal detail that must not appear in the error", http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, err := fetchExtension(context.Background(), extensionRequestOptions{
		URL: server.URL,
	})
	if err == nil {
		t.Fatal("expected HTTP status failure")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("errNoContent identity was not preserved: %v", err)
	}

	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("HTTP status identity was not preserved: %v", err)
	}

	if statusErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf(
			"status code = %d, want %d",
			statusErr.StatusCode,
			http.StatusTooManyRequests,
		)
	}

	if classifyRefreshFailure(err) != refreshFailureRateLimited {
		t.Fatalf(
			"failure class = %q, want %q",
			classifyRefreshFailure(err),
			refreshFailureRateLimited,
		)
	}

	if refreshFailureRetryable(err) {
		t.Fatal("429 Extension failure should not be retryable")
	}

	if strings.Contains(err.Error(), "internal detail") {
		t.Fatalf("HTTP status error exposed response body: %q", err)
	}
}

func TestExtensionWidgetHTTPStatusFailurePreservesLastKnownGoodContent(t *testing.T) {
	var fail atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}

		w.Header().Set(extensionHeaderTitle, "Working Extension")
		w.Header().Set(extensionHeaderContentType, "html")
		_, _ = w.Write([]byte("<div>last-known-good</div>"))
	}))
	defer server.Close()

	widget := &extensionWidget{
		URL:       server.URL,
		AllowHtml: true,
	}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize extension widget: %v", err)
	}

	widget.update(context.Background())

	if widget.Error != nil {
		t.Fatalf("successful refresh set error: %v", widget.Error)
	}

	previousExtension := widget.Extension
	previousHTML := widget.cachedHTML

	fail.Store(true)
	widget.update(context.Background())

	if widget.Error == nil {
		t.Fatal("failed refresh should set widget error")
	}

	var statusErr *httpStatusError
	if !errors.As(widget.Error, &statusErr) {
		t.Fatalf("widget error did not preserve HTTP status identity: %v", widget.Error)
	}

	if statusErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf(
			"status code = %d, want %d",
			statusErr.StatusCode,
			http.StatusTooManyRequests,
		)
	}

	if widget.Extension != previousExtension {
		t.Fatal("HTTP status failure replaced last-known-good extension")
	}

	if widget.cachedHTML != previousHTML {
		t.Fatal("HTTP status failure replaced last-known-good rendered HTML")
	}

	if !widget.refreshDegraded {
		t.Fatal("HTTP status failure should mark widget degraded")
	}

	if widget.refreshFailureClass != refreshFailureRateLimited {
		t.Fatalf(
			"failure class = %q, want %q",
			widget.refreshFailureClass,
			refreshFailureRateLimited,
		)
	}

	if widget.updateRetriedTimes != 0 {
		t.Fatalf(
			"retry attempts = %d, want 0 for rate-limit failure",
			widget.updateRetriedTimes,
		)
	}
}

func TestExtensionWidgetRejectsInvalidEndpointURLs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "relative", url: "/extension"},
		{name: "unsupported scheme", url: "file:///tmp/extension"},
		{name: "userinfo", url: "https://user:pass@example.com/extension"},
		{name: "fragment", url: "https://example.com/extension#fragment"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			widget := &extensionWidget{URL: test.url}
			if err := widget.initialize(); err == nil {
				t.Fatalf("initialize() accepted invalid URL %q", test.url)
			}
		})
	}
}

func TestFetchExtensionRejectsUnsafeTitleURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(extensionHeaderTitleURL, "javascript:alert(1)")
		_, _ = w.Write([]byte("extension"))
	}))
	defer server.Close()

	_, err := fetchExtension(context.Background(), extensionRequestOptions{URL: server.URL})
	if err == nil {
		t.Fatal("expected unsafe Widget-Title-URL to be rejected")
	}
	if !errors.Is(err, errNoContent) {
		t.Fatalf("errNoContent identity was not preserved: %v", err)
	}
}

func TestFetchExtensionAllowsRelativeAndHTTPTitleURLs(t *testing.T) {
	for _, titleURL := range []string{"/details", "https://example.com/details"} {
		t.Run(titleURL, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(extensionHeaderTitleURL, titleURL)
				_, _ = w.Write([]byte("extension"))
			}))
			defer server.Close()

			extension, err := fetchExtension(context.Background(), extensionRequestOptions{URL: server.URL})
			if err != nil {
				t.Fatalf("fetchExtension() error = %v", err)
			}
			if extension.TitleURL != titleURL {
				t.Fatalf("TitleURL = %q, want %q", extension.TitleURL, titleURL)
			}
		})
	}
}

func TestFetchExtensionBlocksCredentialedCrossOriginRedirect(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests.Add(1)
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()

	_, err := fetchExtension(context.Background(), extensionRequestOptions{
		URL:     source.URL,
		Headers: map[string]string{"X-API-Key": "secret"},
	})
	if err == nil {
		t.Fatal("expected credentialed cross-origin redirect to be rejected")
	}
	if !errors.Is(err, errExtensionCrossOriginRedirectWithCredentials) {
		t.Fatalf("redirect error identity was not preserved: %v", err)
	}
	if targetRequests.Load() != 0 {
		t.Fatalf("redirect target received %d requests, want 0", targetRequests.Load())
	}
}

func TestFetchExtensionAllowsCredentialedSameOriginRedirect(t *testing.T) {
	const apiKey = "secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("X-API-Key = %q, want %q", got, apiKey)
		}
		_, _ = w.Write([]byte("extension"))
	}))
	defer server.Close()

	if _, err := fetchExtension(context.Background(), extensionRequestOptions{
		URL:     server.URL + "/start",
		Headers: map[string]string{"X-API-Key": apiKey},
	}); err != nil {
		t.Fatalf("fetchExtension() error = %v", err)
	}
}

func TestExtensionWidgetRemoteMetadataCanChangeAcrossRefreshes(t *testing.T) {
	var responseNumber atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if responseNumber.Add(1) == 1 {
			w.Header().Set(extensionHeaderTitle, "First")
			w.Header().Set(extensionHeaderTitleURL, "/first")
		} else {
			w.Header().Set(extensionHeaderTitle, "Second")
			w.Header().Set(extensionHeaderTitleURL, "/second")
		}
		_, _ = w.Write([]byte("extension"))
	}))
	defer server.Close()

	widget := &extensionWidget{URL: server.URL}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize extension widget: %v", err)
	}

	widget.update(context.Background())
	if widget.Title != "First" || widget.TitleURL != "/first" {
		t.Fatalf("first metadata = %q %q", widget.Title, widget.TitleURL)
	}

	widget.update(context.Background())
	if widget.Title != "Second" || widget.TitleURL != "/second" {
		t.Fatalf("second metadata = %q %q", widget.Title, widget.TitleURL)
	}
}

func TestExtensionWidgetConfiguredMetadataTakesPrecedence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(extensionHeaderTitle, "Remote")
		w.Header().Set(extensionHeaderTitleURL, "/remote")
		_, _ = w.Write([]byte("extension"))
	}))
	defer server.Close()

	widget := &extensionWidget{URL: server.URL}
	widget.Title = "Configured"
	widget.TitleURL = "https://example.com/configured"
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize extension widget: %v", err)
	}

	widget.update(context.Background())
	if widget.Title != "Configured" || widget.TitleURL != "https://example.com/configured" {
		t.Fatalf("configured metadata was replaced: %q %q", widget.Title, widget.TitleURL)
	}
}
