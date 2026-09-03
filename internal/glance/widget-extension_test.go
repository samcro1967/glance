package glance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
