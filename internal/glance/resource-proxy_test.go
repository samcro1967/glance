package glance

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizeResourceProxyOrigin(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "hostname and port", input: "http://osu.plex:32400", want: "http://osu.plex:32400"},
		{name: "default port", input: "http://example.test", want: "http://example.test:80"},
		{name: "root path", input: "http://example.test/", want: "http://example.test:80"},
		{name: "normalizes case", input: "HTTP://EXAMPLE.TEST", want: "http://example.test:80"},
		{name: "IPv4", input: "http://192.0.2.10:8080", want: "http://192.0.2.10:8080"},
		{name: "IPv6", input: "http://[2001:db8::10]:8080", want: "http://[2001:db8::10]:8080"},
		{name: "empty", input: " ", wantErr: true},
		{name: "HTTPS", input: "https://example.test", wantErr: true},
		{name: "relative", input: "example.test:8080", wantErr: true},
		{name: "userinfo", input: "http://user:password@example.test", wantErr: true},
		{name: "path", input: "http://example.test/images", wantErr: true},
		{name: "query", input: "http://example.test?token=secret", wantErr: true},
		{name: "fragment", input: "http://example.test#fragment", wantErr: true},
		{name: "malformed port", input: "http://example.test:not-a-port", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeResourceProxyOrigin(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeResourceProxyOrigin(%q) error = nil, want error", tt.input)
				}
				return
			}

			if err != nil {
				t.Fatalf("normalizeResourceProxyOrigin(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeResourceProxyOrigin(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResourceProxyPolicyAllowsURL(t *testing.T) {
	policy, err := newResourceProxyPolicy([]string{
		"http://osu.plex:32400",
		"http://osu.sonarr:8079",
		"http://example.test",
	})
	if err != nil {
		t.Fatalf("newResourceProxyPolicy() error = %v", err)
	}

	tests := []struct {
		name    string
		rawURL  string
		allowed bool
	}{
		{name: "allows configured origin with path and query", rawURL: "http://osu.plex:32400/library/metadata/1/thumb?token=secret", allowed: true},
		{name: "allows second configured origin", rawURL: "http://osu.sonarr:8079/sonarr/api/v3/mediacover/1/poster.jpg?apikey=secret", allowed: true},
		{name: "allows normalized default port", rawURL: "http://example.test/image.jpg", allowed: true},
		{name: "allows explicit normalized default port", rawURL: "http://example.test:80/image.jpg", allowed: true},
		{name: "rejects different port", rawURL: "http://osu.plex:32401/image.jpg", allowed: false},
		{name: "rejects different host", rawURL: "http://other.internal:32400/image.jpg", allowed: false},
		{name: "rejects HTTPS", rawURL: "https://osu.plex:32400/image.jpg", allowed: false},
		{name: "rejects userinfo", rawURL: "http://user:password@osu.plex:32400/image.jpg", allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := url.Parse(tt.rawURL)
			if err != nil {
				t.Fatalf("url.Parse(%q) error = %v", tt.rawURL, err)
			}

			if got := policy.allowsURL(parsed); got != tt.allowed {
				t.Fatalf("policy.allowsURL(%q) = %v, want %v", tt.rawURL, got, tt.allowed)
			}
		})
	}
}

func TestResourceProxyPolicyDisabledByDefault(t *testing.T) {
	policy, err := newResourceProxyPolicy(nil)
	if err != nil {
		t.Fatalf("newResourceProxyPolicy(nil) error = %v", err)
	}

	resourceURL, err := url.Parse("http://example.test/image.jpg")
	if err != nil {
		t.Fatal(err)
	}

	if policy.allowsURL(resourceURL) {
		t.Fatal("empty resource proxy policy unexpectedly allowed URL")
	}
}

func TestResourceProxyRegisterAndLookup(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://osu.plex:32400"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	rawURL := "http://osu.plex:32400/library/metadata/1/thumb?X-Plex-Token=secret"
	id, err := proxy.register(rawURL)
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}
	if id == "" {
		t.Fatal("register() returned empty ID")
	}
	if strings.Contains(id, "osu.plex") || strings.Contains(id, "secret") {
		t.Fatalf("register() ID disclosed upstream URL content: %q", id)
	}

	got, ok := proxy.lookup(id)
	if !ok {
		t.Fatalf("lookup(%q) did not find registered resource", id)
	}
	if got != rawURL {
		t.Fatalf("lookup(%q) = %q, want original URL", id, got)
	}
}

func TestResourceProxyRegisterDeduplicatesURL(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://osu.plex:32400"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	rawURL := "http://osu.plex:32400/library/metadata/1/thumb?X-Plex-Token=secret"
	first, err := proxy.register(rawURL)
	if err != nil {
		t.Fatalf("first register() error = %v", err)
	}
	second, err := proxy.register(rawURL)
	if err != nil {
		t.Fatalf("second register() error = %v", err)
	}
	if first != second {
		t.Fatalf("register() IDs differ for identical URL: %q != %q", first, second)
	}
	if len(proxy.byID) != 1 || len(proxy.byURL) != 1 {
		t.Fatalf("duplicate registration grew registry: byID=%d byURL=%d", len(proxy.byID), len(proxy.byURL))
	}
}

func TestResourceProxyRegisterRejectsUnallowedURL(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://osu.plex:32400"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	if _, err := proxy.register("http://other.internal:32400/image.jpg?token=secret"); err == nil {
		t.Fatal("register() allowed unconfigured origin")
	}
	if len(proxy.byID) != 0 || len(proxy.byURL) != 0 {
		t.Fatal("rejected registration mutated registry")
	}
}

func TestResourceProxyDisabledByDefaultCannotRegister(t *testing.T) {
	proxy, err := newResourceProxy(nil)
	if err != nil {
		t.Fatalf("newResourceProxy(nil) error = %v", err)
	}

	if _, err := proxy.register("http://example.test/image.jpg"); err == nil {
		t.Fatal("disabled resource proxy registered URL")
	}
}

func TestResourceProxyLookupUnknownID(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	if _, ok := proxy.lookup("unknown"); ok {
		t.Fatal("lookup() found unknown ID")
	}
}

func TestResourceProxyConcurrentRegistration(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	const workers = 32
	rawURL := "http://example.test/image.jpg?token=secret"
	ids := make(chan string, workers)
	errs := make(chan error, workers)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := proxy.register(rawURL)
			if err != nil {
				errs <- err
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent register() error = %v", err)
	}

	var want string
	for id := range ids {
		if want == "" {
			want = id
			continue
		}
		if id != want {
			t.Fatalf("concurrent register() returned different IDs: %q != %q", id, want)
		}
	}

	if len(proxy.byID) != 1 || len(proxy.byURL) != 1 {
		t.Fatalf("concurrent registration grew registry: byID=%d byURL=%d", len(proxy.byID), len(proxy.byURL))
	}
}

type testRoundTripper func(*http.Request) (*http.Response, error)

func (transport testRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestResourceProxyHTTPClientUsesIndependentTransportWithoutEnvironmentProxy(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	observed, ok := proxy.client.Transport.(observedRoundTripper)
	if !ok {
		t.Fatalf("transport type = %T, want observedRoundTripper", proxy.client.Transport)
	}
	if observed.diagnostics != outboundHTTPDiagnostics {
		t.Fatal("resource proxy client does not use process-wide outbound HTTP diagnostics")
	}

	transport, ok := observed.transport.(*http.Transport)
	if !ok {
		t.Fatalf("underlying transport type = %T, want *http.Transport", observed.transport)
	}
	if transport == defaultHTTPTransport {
		t.Fatal("resource proxy client uses shared default transport")
	}
	if transport.Proxy != nil {
		t.Fatal("resource proxy transport permits environment proxying")
	}
	if transport.MaxIdleConnsPerHost != defaultHTTPTransport.MaxIdleConnsPerHost {
		t.Fatalf(
			"MaxIdleConnsPerHost = %d, want %d",
			transport.MaxIdleConnsPerHost,
			defaultHTTPTransport.MaxIdleConnsPerHost,
		)
	}
	if transport.IdleConnTimeout != defaultHTTPTransport.IdleConnTimeout {
		t.Fatalf(
			"IdleConnTimeout = %s, want %s",
			transport.IdleConnTimeout,
			defaultHTTPTransport.IdleConnTimeout,
		)
	}
	if proxy.client.Timeout != defaultClientTimeout {
		t.Fatalf("Timeout = %s, want %s", proxy.client.Timeout, defaultClientTimeout)
	}
}

func TestResourceProxyFetchSuccess(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	body := &trackingReadCloser{Reader: strings.NewReader("image-data")}
	proxy.client = &http.Client{
		Transport: testRoundTripper(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodGet {
				t.Fatalf("request method = %q, want GET", request.Method)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"image/jpeg; charset=binary"},
				},
				Body: body,
			}, nil
		}),
	}

	id, err := proxy.register("http://example.test/image.jpg?token=secret")
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}

	response, err := proxy.fetch(context.Background(), id)
	if err != nil {
		t.Fatalf("fetch() error = %v", err)
	}
	if response.ContentType != "image/jpeg" {
		t.Fatalf("ContentType = %q, want image/jpeg", response.ContentType)
	}
	if string(response.Body) != "image-data" {
		t.Fatalf("Body = %q, want image-data", response.Body)
	}
	if !body.closed {
		t.Fatal("response body was not closed")
	}
}

func TestResourceProxyFetchRejectsUnsupportedContentType(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	body := &trackingReadCloser{Reader: strings.NewReader("<svg/>")}
	proxy.client = &http.Client{
		Transport: testRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"image/svg+xml"},
				},
				Body: body,
			}, nil
		}),
	}

	id, err := proxy.register("http://example.test/image.svg")
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}

	_, err = proxy.fetch(context.Background(), id)
	if !errors.Is(err, errResourceProxyUnsupportedContentType) {
		t.Fatalf("fetch() error = %v, want unsupported content type", err)
	}
	if !body.closed {
		t.Fatal("rejected response body was not closed")
	}
}

func TestResourceProxyFetchRejectsMissingContentType(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	body := &trackingReadCloser{Reader: strings.NewReader("data")}
	proxy.client = &http.Client{
		Transport: testRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       body,
			}, nil
		}),
	}

	id, err := proxy.register("http://example.test/image")
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}

	_, err = proxy.fetch(context.Background(), id)
	if !errors.Is(err, errResourceProxyUnsupportedContentType) {
		t.Fatalf("fetch() error = %v, want unsupported content type", err)
	}
	if !body.closed {
		t.Fatal("rejected response body was not closed")
	}
}

func TestResourceProxyFetchRejectsOversizedResponse(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	body := &trackingReadCloser{
		Reader: io.LimitReader(
			strings.NewReader(strings.Repeat("x", int(resourceProxyResponseBodyLimit)+1)),
			resourceProxyResponseBodyLimit+1,
		),
	}
	proxy.client = &http.Client{
		Transport: testRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"image/png"},
				},
				Body: body,
			}, nil
		}),
	}

	id, err := proxy.register("http://example.test/image.png")
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}

	_, err = proxy.fetch(context.Background(), id)
	var tooLarge *httpResponseTooLargeError
	if !errors.As(err, &tooLarge) {
		t.Fatalf("fetch() error = %T %v, want *httpResponseTooLargeError", err, err)
	}
	if tooLarge.Limit != resourceProxyResponseBodyLimit {
		t.Fatalf("limit = %d, want %d", tooLarge.Limit, resourceProxyResponseBodyLimit)
	}
	if !body.closed {
		t.Fatal("oversized response body was not closed")
	}
}

func TestResourceProxyFetchRejectsUpstreamStatus(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	body := &trackingReadCloser{Reader: strings.NewReader("upstream failure")}
	proxy.client = &http.Client{
		Transport: testRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header: http.Header{
					"Content-Type": []string{"text/plain"},
				},
				Body: body,
			}, nil
		}),
	}

	id, err := proxy.register("http://example.test/image.jpg?token=secret")
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}

	_, err = proxy.fetch(context.Background(), id)
	if err == nil || !strings.Contains(err.Error(), "HTTP status 401") {
		t.Fatalf("fetch() error = %v, want HTTP status 401", err)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "example.test") {
		t.Fatalf("fetch() error disclosed upstream URL content: %v", err)
	}
	if !body.closed {
		t.Fatal("non-200 response body was not closed")
	}
}

func TestResourceProxyFetchUnknownIDDoesNotRequestUpstream(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	requested := false
	proxy.client = &http.Client{
		Transport: testRoundTripper(func(*http.Request) (*http.Response, error) {
			requested = true
			return nil, errors.New("unexpected request")
		}),
	}

	_, err = proxy.fetch(context.Background(), "unknown")
	if err == nil {
		t.Fatal("fetch() succeeded for unknown ID")
	}
	if requested {
		t.Fatal("unknown ID caused upstream request")
	}
}

func resourceProxyTestOrigin(t *testing.T, rawURL string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parsing test URL: %v", err)
	}

	return "http://" + parsed.Host
}

func TestResourceProxyFetchAllowsSameOriginRedirect(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/image.jpg", http.StatusFound)
		case "/image.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("redirected-image"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	proxy, err := newResourceProxy([]string{resourceProxyTestOrigin(t, server.URL)})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	id, err := proxy.register(server.URL + "/start?token=secret")
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}

	response, err := proxy.fetch(context.Background(), id)
	if err != nil {
		t.Fatalf("fetch() error = %v", err)
	}
	if string(response.Body) != "redirected-image" {
		t.Fatalf("Body = %q, want redirected-image", response.Body)
	}
}

func TestResourceProxyFetchRejectsCrossOriginRedirectWithoutSecretLeak(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("should-not-be-fetched"))
	}))
	defer target.Close()

	var targetRequests atomic.Int32
	target.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("should-not-be-fetched"))
	})

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(
			w,
			r,
			target.URL+"/image.jpg?token=redirect-secret",
			http.StatusFound,
		)
	}))
	defer source.Close()

	proxy, err := newResourceProxy([]string{
		resourceProxyTestOrigin(t, source.URL),
		resourceProxyTestOrigin(t, target.URL),
	})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	id, err := proxy.register(source.URL + "/start?token=source-secret")
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}

	_, err = proxy.fetch(context.Background(), id)
	if !errors.Is(err, errResourceProxyRedirectNotAllowed) {
		t.Fatalf("fetch() error = %v, want redirect-not-allowed", err)
	}
	if targetRequests.Load() != 0 {
		t.Fatalf("cross-origin redirect reached target %d times", targetRequests.Load())
	}

	message := err.Error()
	for _, secret := range []string{
		"source-secret",
		"redirect-secret",
		source.URL,
		target.URL,
	} {
		if strings.Contains(message, secret) {
			t.Fatalf("redirect error disclosed %q: %v", secret, err)
		}
	}
}

func TestResourceProxyFetchRejectsHTTPSRedirect(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("HTTPS redirect target should not be requested")
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/image.jpg", http.StatusFound)
	}))
	defer source.Close()

	proxy, err := newResourceProxy([]string{resourceProxyTestOrigin(t, source.URL)})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	id, err := proxy.register(source.URL + "/start")
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}

	_, err = proxy.fetch(context.Background(), id)
	if !errors.Is(err, errResourceProxyRedirectNotAllowed) {
		t.Fatalf("fetch() error = %v, want redirect-not-allowed", err)
	}
}

func TestResourceProxyFetchStopsRedirectLoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer server.Close()

	proxy, err := newResourceProxy([]string{resourceProxyTestOrigin(t, server.URL)})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	id, err := proxy.register(server.URL + "/loop?token=loop-secret")
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}

	_, err = proxy.fetch(context.Background(), id)
	if err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Fatalf("fetch() error = %v, want redirect-limit error", err)
	}
	if strings.Contains(err.Error(), "loop-secret") || strings.Contains(err.Error(), server.URL) {
		t.Fatalf("redirect-limit error disclosed upstream URL content: %v", err)
	}
}

func TestResourceProxyFetchCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	proxy, err := newResourceProxy([]string{resourceProxyTestOrigin(t, server.URL)})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	id, err := proxy.register(server.URL + "/image.jpg?token=cancel-secret")
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)

	go func() {
		_, err := proxy.fetch(ctx, id)
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("resource proxy request did not start")
	}

	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("fetch() error = %v, want context.Canceled", err)
		}
		if strings.Contains(err.Error(), "cancel-secret") || strings.Contains(err.Error(), server.URL) {
			t.Fatalf("cancellation error disclosed upstream URL content: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("resource proxy fetch did not stop after cancellation")
	}
}

func TestResourceProxyFetchSanitizesTransportError(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	proxy.client = &http.Client{
		Transport: testRoundTripper(func(request *http.Request) (*http.Response, error) {
			return nil, &url.Error{
				Op:  "Get",
				URL: request.URL.String(),
				Err: errors.New("connection refused"),
			}
		}),
	}

	id, err := proxy.register("http://example.test/image.jpg?token=transport-secret")
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}

	_, err = proxy.fetch(context.Background(), id)
	if err == nil {
		t.Fatal("fetch() unexpectedly succeeded")
	}

	if strings.Contains(err.Error(), "transport-secret") ||
		strings.Contains(err.Error(), "example.test") {
		t.Fatalf("transport error disclosed upstream URL content: %v", err)
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("transport error lost safe cause: %v", err)
	}
}

func TestResourceProxyFetchRevalidatesStoredDestination(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://allowed.test"})
	if err != nil {
		t.Fatalf("newResourceProxy() error = %v", err)
	}

	id, err := proxy.register("http://allowed.test/image.jpg")
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}

	proxy.mu.Lock()
	proxy.byID[id] = "http://not-allowed.test/image.jpg?token=secret"
	proxy.mu.Unlock()

	requested := false
	proxy.client = &http.Client{
		Transport: testRoundTripper(func(*http.Request) (*http.Response, error) {
			requested = true
			return nil, errors.New("unexpected request")
		}),
	}

	_, err = proxy.fetch(context.Background(), id)
	if err == nil || !strings.Contains(err.Error(), "destination is not allowed") {
		t.Fatalf("fetch() error = %v, want destination-not-allowed", err)
	}
	if requested {
		t.Fatal("invalid stored destination caused upstream request")
	}
	if strings.Contains(err.Error(), "not-allowed.test") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("destination validation error disclosed upstream URL content: %v", err)
	}
}

func TestHandleResourceProxyRequestSuccess(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatal(err)
	}

	id, err := proxy.register("http://example.test/image.png?token=secret-value")
	if err != nil {
		t.Fatal(err)
	}

	proxy.client = &http.Client{
		Transport: testRoundTripper(func(request *http.Request) (*http.Response, error) {
			if request.Header.Get("Cookie") != "" {
				t.Fatalf("upstream Cookie = %q, want empty", request.Header.Get("Cookie"))
			}
			if request.Header.Get("Authorization") != "" {
				t.Fatalf("upstream Authorization = %q, want empty", request.Header.Get("Authorization"))
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": {"image/png"},
					"Set-Cookie":   {"upstream=secret"},
					"X-Upstream":   {"must-not-escape"},
				},
				Body: io.NopCloser(strings.NewReader("image-data")),
			}, nil
		}),
	}

	app := &application{resourceProxy: proxy}
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/resource-proxy/"+id,
		nil,
	)
	request.SetPathValue("resource", id)
	request.Header.Set("Cookie", "browser=session")
	request.Header.Set("Authorization", "Bearer browser-secret")

	recorder := httptest.NewRecorder()
	app.handleResourceProxyRequest(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", got)
	}
	if got := recorder.Header().Get("Set-Cookie"); got != "" {
		t.Fatalf("upstream Set-Cookie escaped: %q", got)
	}
	if got := recorder.Header().Get("X-Upstream"); got != "" {
		t.Fatalf("upstream header escaped: %q", got)
	}
	if got := recorder.Body.String(); got != "image-data" {
		t.Fatalf("body = %q, want image-data", got)
	}
	if strings.Contains(recorder.Body.String(), "secret-value") {
		t.Fatal("response exposed upstream credential")
	}
}

func TestHandleResourceProxyRequestUnknownResource(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatal(err)
	}

	app := &application{resourceProxy: proxy}
	request := httptest.NewRequest(http.MethodGet, "/api/resource-proxy/unknown", nil)
	request.SetPathValue("resource", "unknown")
	recorder := httptest.NewRecorder()

	app.handleResourceProxyRequest(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestHandleResourceProxyRequestUpstreamFailure(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatal(err)
	}

	id, err := proxy.register("http://example.test/image.png?token=secret-value")
	if err != nil {
		t.Fatal(err)
	}

	proxy.client = &http.Client{
		Transport: testRoundTripper(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("upstream unavailable")
		}),
	}

	app := &application{resourceProxy: proxy}
	request := httptest.NewRequest(http.MethodGet, "/api/resource-proxy/"+id, nil)
	request.SetPathValue("resource", id)
	recorder := httptest.NewRecorder()

	app.handleResourceProxyRequest(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadGateway)
	}
	if strings.Contains(recorder.Body.String(), "example.test") ||
		strings.Contains(recorder.Body.String(), "secret-value") {
		t.Fatal("error response exposed upstream URL or credential")
	}
}

func TestResourceProxyRequestRequiresAuthentication(t *testing.T) {
	app := newAuthTestApplication(t)

	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	app.resourceProxy = proxy

	requested := false
	proxy.client = &http.Client{
		Transport: testRoundTripper(func(*http.Request) (*http.Response, error) {
			requested = true
			return nil, errors.New("unexpected request")
		}),
	}

	id, err := proxy.register("http://example.test/image.jpg")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/resource-proxy/"+id,
		nil,
	)

	app.router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if requested {
		t.Fatal("unauthorized resource proxy request reached upstream")
	}
}

func TestResourceProxyRouterAppliesSecurityHeaders(t *testing.T) {
	proxy, err := newResourceProxy([]string{"http://example.test"})
	if err != nil {
		t.Fatal(err)
	}

	id, err := proxy.register("http://example.test/image.jpg")
	if err != nil {
		t.Fatal(err)
	}

	proxy.client = &http.Client{
		Transport: testRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": {"image/jpeg"},
				},
				Body: io.NopCloser(strings.NewReader("jpeg-data")),
			}, nil
		}),
	}

	app := &application{resourceProxy: proxy}
	request := httptest.NewRequest(http.MethodGet, "/api/resource-proxy/"+id, nil)
	recorder := httptest.NewRecorder()

	app.router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := recorder.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Fatalf("Referrer-Policy = %q, want strict-origin-when-cross-origin", got)
	}
}

func FuzzNormalizeResourceProxyOrigin(f *testing.F) {
	for _, seed := range []string{"", "http://example.test", "HTTP://EXAMPLE.TEST", "http://example.test/", "http://example.test:80", "http://192.0.2.10:8080", "http://[2001:db8::10]:8080", "https://example.test", "http://user:password@example.test", "http://example.test/path", "http://example.test?query=value", "http://example.test#fragment", "http://example.test:not-a-port"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, configuredOrigin string) {
		normalized, err := normalizeResourceProxyOrigin(configuredOrigin)
		if err != nil {
			return
		}
		if normalized == "" {
			t.Fatal("successful resource proxy origin normalization returned empty origin")
		}
		roundTrip, err := normalizeResourceProxyOrigin(normalized)
		if err != nil {
			t.Fatalf("normalizing canonical resource proxy origin %q: %v", normalized, err)
		}
		if roundTrip != normalized {
			t.Fatalf("resource proxy origin normalization is not idempotent: first %q second %q", normalized, roundTrip)
		}
	})
}

func FuzzResourceProxyPolicyAllowsURL(f *testing.F) {
	const configuredOrigin = "http://example.test:8080"
	policy, err := newResourceProxyPolicy([]string{configuredOrigin})
	if err != nil {
		f.Fatalf("creating resource proxy policy: %v", err)
	}

	for _, seed := range []string{"", "http://example.test:8080/image.jpg", "HTTP://EXAMPLE.TEST:8080/image.jpg?token=secret", "http://example.test:80/image.jpg", "http://other.test:8080/image.jpg", "https://example.test:8080/image.jpg", "http://user:password@example.test:8080/image.jpg", "http://example.test:8080@other.test/image.jpg", "http://example.test:8080.evil.test/image.jpg", "http://[2001:db8::10]:8080/image.jpg"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, rawURL string) {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return
		}
		if !policy.allowsURL(parsed) {
			return
		}
		origin, err := resourceProxyURLOrigin(parsed)
		if err != nil {
			t.Fatalf("allowed resource URL %q has invalid origin: %v", rawURL, err)
		}
		if origin != configuredOrigin {
			t.Fatalf("resource proxy policy allowed URL %q from origin %q outside configured origin %q", rawURL, origin, configuredOrigin)
		}
	})
}
