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
