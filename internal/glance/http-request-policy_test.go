package glance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestRequestPolicyCredentialErrorsRemainPrivate(t *testing.T) {
	wave3Transport(t, func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") })
	w := &nowPlayingWidget{Service: "navidrome", Server: "https://media.invalid", Username: "PRIVATE_USERNAME", Password: "PRIVATE_PASSWORD"}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	w.update(context.Background())
	for name, output := range map[string]string{"log": logs.String(), "HTML": string(w.Render()), "diagnostic": w.lastRefreshError} {
		for _, secret := range []string{"PRIVATE_USERNAME", "PRIVATE_PASSWORD", "media.invalid", "?t=", "&t=", "&s="} {
			if strings.Contains(output, secret) {
				t.Fatalf("%s exposes %q: %s", name, secret, output)
			}
		}
		if !strings.Contains(output, "connection refused") {
			t.Fatalf("%s lost safe transport cause", name)
		}
	}
	joined := errors.Join(errPartialContent, fmt.Errorf("provider: %w", &url.Error{URL: "https://private.invalid?token=SECRET", Err: context.DeadlineExceeded}))
	safe := safeHTTPTransportError(joined)
	if !errors.Is(safe, errPartialContent) || !errors.Is(safe, context.DeadlineExceeded) || strings.Contains(safe.Error(), "SECRET") {
		t.Fatalf("unsafe partial error: %v", safe)
	}
}

func TestRequestPolicyRedirectOriginsAndCredentials(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, credential := range []string{"header", "query", "body", "none"} {
			for _, destination := range []string{"https://media.invalid/next", "https://other.invalid/next", "http://media.invalid/next", "https://media.invalid:8443/next"} {
				t.Run(fmt.Sprintf("%d/%s/%s", status, credential, destination), func(t *testing.T) {
					var received bool
					wave3Transport(t, func(r *http.Request) (*http.Response, error) {
						if r.URL.Path == "/start" {
							return wave3Response(status, "", map[string]string{"Location": destination}), nil
						}
						received = true
						return wave3Response(200, "{}", nil), nil
					})
					address := "https://media.invalid/start"
					method := "GET"
					var body *strings.Reader
					if credential == "query" {
						address += "?t=TOKEN&s=SALT"
					}
					if credential == "body" {
						method = "POST"
						body = strings.NewReader("password=PRIVATE")
					}
					request, _ := http.NewRequest(method, address, nil)
					if body != nil {
						request, _ = http.NewRequest(method, address, body)
					}
					if credential == "header" {
						request.Header.Set("X-Plex-Token", "PRIVATE")
					}
					response, err := newHTTPClient(0, false).Do(request)
					if response != nil {
						_ = response.Body.Close()
					}
					blocked := credential != "none" && destination != "https://media.invalid/next"
					if blocked {
						if !errors.Is(err, errHTTPCredentialRedirect) || received {
							t.Fatalf("private request crossed origin: err=%v received=%t", err, received)
						}
					} else if err != nil || !received {
						t.Fatalf("public/same-origin redirect failed: %v", err)
					}
				})
			}
		}
	}
}
