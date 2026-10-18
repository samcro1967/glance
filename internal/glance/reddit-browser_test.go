package glance

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"
)

func browserTestConfig() redditBrowserConfig {
	var cfg redditBrowserConfig
	cfg.Endpoint = "http://playwright:9223"
	cfg.Reddit.Username = "example"
	cfg.Reddit.Password = "test-secret"
	return cfg
}

func TestRedditBrowserConfigOptIn(t *testing.T) {
	w := &redditWidget{Subreddit: "docker"}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if w.UsePlaywright {
		t.Fatal("browser mode unexpectedly enabled")
	}
}

func TestRedditBrowserConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*redditWidget)
		want   string
	}{
		{"missing endpoint", func(w *redditWidget) { w.browserConfig.Endpoint = "" }, "playwright.endpoint"},
		{"missing credentials", func(w *redditWidget) { w.browserConfig.Reddit.Password = "" }, "playwright.reddit"},
		{"app auth conflict", func(w *redditWidget) { w.AppAuth.Name = "a"; w.AppAuth.ID = "b"; w.AppAuth.Secret = "c" }, "cannot be combined"},
		{"request template conflict", func(w *redditWidget) { w.RequestURLTemplate = "https://proxy.test/?url={REQUEST-URL}" }, "cannot be combined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &redditWidget{Subreddit: "docker", UsePlaywright: true, browserConfig: browserTestConfig()}
			tt.mutate(w)
			err := w.initialize()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRedditBrowserConfigExistingSession(t *testing.T) {
	w := &redditWidget{
		Subreddit:     "docker",
		UsePlaywright: true,
		browserConfig: browserTestConfig(),
	}
	w.browserConfig.Reddit.Username = ""
	w.browserConfig.Reddit.Password = ""

	if err := w.initialize(); err != nil {
		t.Fatalf("existing authenticated session should not require credentials: %v", err)
	}
}

func TestRedditBrowserRequestURL(t *testing.T) {
	w := &redditWidget{Subreddit: "selfhosted", SortBy: "top", TopPeriod: "week", Limit: 40}
	if got := w.browserRequestURL(); got != "https://www.reddit.com/r/selfhosted/top.json?limit=40&t=week" {
		t.Fatal(got)
	}
	w.Search = "docker"
	if got := w.browserRequestURL(); !strings.Contains(got, "search.json?") || !strings.Contains(got, "docker+subreddit%3Aselfhosted") {
		t.Fatal(got)
	}
}

func TestRedditBrowserConfigurationAssigned(t *testing.T) {
	w := &redditWidget{Subreddit: "docker", UsePlaywright: true}
	configured := configureRedditBrowser(w, browserTestConfig()).(*redditWidget)
	if err := configured.initialize(); err != nil {
		t.Fatal(err)
	}
	if configured.browserConfig.Endpoint != "http://playwright:9223" {
		t.Fatal("missing browser endpoint")
	}
}

func TestRedditBrowserConfigurationNestedWidgets(t *testing.T) {
	direct := &redditWidget{Subreddit: "direct", UsePlaywright: true}
	docker := &redditWidget{Subreddit: "docker", UsePlaywright: true}
	selfhosted := &redditWidget{Subreddit: "selfhosted", UsePlaywright: true}
	deep := &redditWidget{Subreddit: "softwarr", UsePlaywright: true}
	disabled := &redditWidget{Subreddit: "golang"}

	inner := &groupWidget{
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{deep, disabled},
		},
	}

	outer := &groupWidget{
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{docker, selfhosted, inner},
		},
	}

	cfg := browserTestConfig()

	configureRedditBrowser(direct, cfg)
	configureRedditBrowser(outer, cfg)

	for _, tc := range []struct {
		name   string
		widget *redditWidget
	}{
		{"direct", direct},
		{"docker", docker},
		{"selfhosted", selfhosted},
		{"deeply nested", deep},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.widget.browserConfig != cfg {
				t.Fatalf("browser configuration not propagated to %s", tc.name)
			}
			if err := tc.widget.initialize(); err != nil {
				t.Fatalf("Reddit widget initialization failed: %v", err)
			}
		})
	}

	if disabled.UsePlaywright {
		t.Fatal("browser authentication unexpectedly enabled for non-opt-in widget")
	}
	if err := disabled.initialize(); err != nil {
		t.Fatalf("non-opt-in Reddit widget initialization failed: %v", err)
	}
}

func TestRedditBrowserResolveEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		host     string
		port     string
		scheme   string
	}{
		{"IPv4", "http://127.0.0.1:9223", "127.0.0.1", "9223", "http"},
		{"IPv6", "http://[::1]:9223", "::1", "9223", "http"},
		{"default HTTP port", "http://127.0.0.1", "127.0.0.1", "80", "http"},
		{"default HTTPS port", "https://127.0.0.1", "127.0.0.1", "443", "https"},
		{"localhost resolution", "http://localhost:9223", "", "9223", "http"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := redditBrowserResolveEndpoint(context.Background(), tt.endpoint)
			if err != nil {
				t.Fatal(err)
			}

			parsed, err := url.Parse(resolved)
			if err != nil {
				t.Fatal(err)
			}

			if parsed.Scheme != tt.scheme || parsed.Port() != tt.port {
				t.Fatalf("resolved endpoint = %q", resolved)
			}

			if net.ParseIP(parsed.Hostname()) == nil {
				t.Fatalf("resolved hostname is not an IP address: %q", resolved)
			}

			if tt.host != "" && parsed.Hostname() != tt.host {
				t.Fatalf("hostname = %q, want %q", parsed.Hostname(), tt.host)
			}
		})
	}
}

func TestRedditBrowserResolveEndpointInvalid(t *testing.T) {
	for _, endpoint := range []string{
		"http://",
		"http://[invalid",
	} {
		if _, err := redditBrowserResolveEndpoint(context.Background(), endpoint); err == nil {
			t.Fatalf("expected error for %q", endpoint)
		}
	}
}

func TestRedditBrowserResolveEndpointCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := redditBrowserResolveEndpoint(ctx, "http://nonexistent.invalid:9223")
	if err == nil {
		t.Fatal("expected DNS resolution error after cancellation")
	}
}

func TestRedditBrowserWebSocketURL(t *testing.T) {
	tests := []struct {
		name       string
		advertised string
		relay      string
		want       string
		wantError  bool
	}{
		{
			name:       "HTTP relay",
			advertised: "ws://127.0.0.1:9222/devtools/browser/browser-id",
			relay:      "http://192.0.2.10:9223",
			want:       "ws://192.0.2.10:9223/devtools/browser/browser-id",
		},
		{
			name:       "HTTPS relay",
			advertised: "wss://localhost:9222/devtools/browser/browser-id",
			relay:      "https://relay.example:9223",
			want:       "wss://relay.example:9223/devtools/browser/browser-id",
		},
		{
			name:       "invalid scheme",
			advertised: "http://localhost:9222/devtools/browser/browser-id",
			relay:      "http://relay.example:9223",
			wantError:  true,
		},
		{
			name:       "missing host",
			advertised: "ws:///devtools/browser/browser-id",
			relay:      "http://relay.example:9223",
			wantError:  true,
		},
		{
			name:       "missing browser identifier",
			advertised: "ws://localhost:9222/devtools/browser/",
			relay:      "http://relay.example:9223",
			wantError:  true,
		},
		{
			name:       "page endpoint rejected",
			advertised: "ws://localhost:9222/devtools/page/page-id",
			relay:      "http://relay.example:9223",
			wantError:  true,
		},
		{
			name:       "embedded credentials",
			advertised: "ws://user:secret@localhost:9222/devtools/browser/browser-id",
			relay:      "http://relay.example:9223",
			wantError:  true,
		},
		{
			name:       "query string",
			advertised: "ws://localhost:9222/devtools/browser/browser-id?token=secret",
			relay:      "http://relay.example:9223",
			wantError:  true,
		},
		{
			name:       "fragment",
			advertised: "ws://localhost:9222/devtools/browser/browser-id#fragment",
			relay:      "http://relay.example:9223",
			wantError:  true,
		},
		{
			name:       "invalid relay",
			advertised: "ws://localhost:9222/devtools/browser/browser-id",
			relay:      "ftp://relay.example:9223",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := redditBrowserWebSocketURL(tt.advertised, tt.relay)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error, got URL %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("WebSocket URL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRedditBrowserLoginGateCancellation(t *testing.T) {
	state := &redditBrowserLoginState{
		gate: make(chan struct{}, 1),
	}

	release, err := state.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = state.acquire(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("acquire error = %v, want context.Canceled", err)
	}
}

func TestRedditBrowserDecodeListing(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"valid empty listing", `{"kind":"Listing","data":{"children":[]}}`, true},
		{"valid populated listing", `{"kind":"Listing","data":{"children":[{"kind":"t3","data":{}}]}}`, true},
		{"missing kind", `{"data":{"children":[]}}`, false},
		{"wrong kind", `{"kind":"t3","data":{"children":[]}}`, false},
		{"missing data", `{"kind":"Listing"}`, false},
		{"missing children", `{"kind":"Listing","data":{}}`, false},
		{"null children", `{"kind":"Listing","data":{"children":null}}`, false},
		{"object children", `{"kind":"Listing","data":{"children":{}}}`, false},
		{"malformed JSON", `{"kind":"Listing"`, false},
		{"empty object", `{}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var listing subredditResponseJson
			got := redditBrowserDecodeListing(tt.raw, &listing)
			if got != tt.want {
				t.Fatalf("decode = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestRedditBrowserRegistryLifecycle(t *testing.T) {
	cfg := browserTestConfig()
	cfg.Endpoint = "http://registry-lifecycle-test:9223"

	first, releaseFirst := redditBrowserState(cfg)
	second, releaseSecond := redditBrowserState(cfg)

	if first != second {
		t.Fatal("same browser identity received different login gates")
	}

	releaseFirst()

	third, releaseThird := redditBrowserState(cfg)
	if third != second {
		t.Fatal("active waiter lost shared login state")
	}

	releaseSecond()
	releaseThird()

	fresh, releaseFresh := redditBrowserState(cfg)
	if fresh == first {
		t.Fatal("idle state without cooldown was not evicted")
	}
	releaseFresh()

	cfg.Reddit.Username = "different-user"
	distinct, releaseDistinct := redditBrowserState(cfg)
	if distinct == first {
		t.Fatal("different account reused previous login state")
	}
	releaseDistinct()
}

func TestRedditBrowserRegistryCooldown(t *testing.T) {
	cfg := browserTestConfig()
	cfg.Endpoint = "http://registry-cooldown-test:9223"

	state, release := redditBrowserState(cfg)
	state.failedAt = time.Now()
	release()

	retained, releaseRetained := redditBrowserState(cfg)
	if retained != state {
		t.Fatal("authentication cooldown was discarded")
	}

	retained.failedAt = time.Now().Add(-6 * time.Minute)
	releaseRetained()

	replacement, releaseReplacement := redditBrowserState(cfg)
	if replacement == state {
		t.Fatal("expired authentication cooldown was retained")
	}
	releaseReplacement()
}

func TestRedditBrowserRegistryCanceledWaiter(t *testing.T) {
	cfg := browserTestConfig()
	cfg.Endpoint = "http://registry-cancellation-test:9223"

	state, releaseState := redditBrowserState(cfg)
	releaseGate, err := state.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	waiter, releaseWaiter := redditBrowserState(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := waiter.acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter returned %v", err)
	}

	releaseWaiter()

	concurrent, releaseConcurrent := redditBrowserState(cfg)
	if concurrent != state {
		t.Fatal("canceled waiter caused active login gate eviction")
	}

	releaseConcurrent()
	releaseGate()
	releaseState()
}
