package glance

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// Every registered timeout capability must have an implementation. Previously
// several widgets accepted defaults but silently skipped applying their value.
func TestTimeoutConsistencyCapabilityHasSetter(t *testing.T) {
	for _, name := range []string{
		"animal-of-the-day", "arr", "astronomy", "calendar", "change-detection",
		"custom-api", "dilbert", "dns-stats", "docker-containers", "environment",
		"extension", "hacker-news", "horoscope", "ics-events", "latest-media",
		"lobsters", "markets", "media-history", "monitor", "nasa-apod", "now-playing",
		"on-this-day", "prometheus", "quote-of-the-day", "reddit", "releases",
		"repository", "rss", "search", "seerr", "server-stats", "stocks", "torrenting",
		"trivia", "twitch-channels", "twitch-top-games", "videos", "weather", "word-of-the-day",
	} {
		if !widgetSupportsCapability(name, widgetCapabilityTimeout, widgetCapabilityScopeType) {
			t.Errorf("server-fetching widget %s lacks timeout defaults", name)
		}
	}
	for name, descriptor := range widgetRegistry {
		if !widgetSupportsCapability(name, widgetCapabilityTimeout, widgetCapabilityScopeType) {
			continue
		}
		if _, ok := descriptor.constructor().(widgetTimeoutSetter); !ok {
			t.Errorf("%s declares timeout defaults without a setter", name)
		}
	}
	for _, name := range []string{"calculator", "unit-converter", "astrology", "clock", "iframe"} {
		if widgetSupportsCapability(name, widgetCapabilityTimeout, widgetCapabilityScopeType) {
			t.Errorf("%s should not expose a server HTTP timeout", name)
		}
	}
}

// Test actual resolution through the production defaults path, not setters in
// isolation. No endpoint is contacted during this configuration-only test.
func TestTimeoutConsistencyInheritance(t *testing.T) {
	global, typeValue, explicit := durationField(7*time.Second), durationField(11*time.Second), durationField(19*time.Second)
	for name, descriptor := range widgetRegistry {
		if !widgetSupportsCapability(name, widgetCapabilityTimeout, widgetCapabilityScopeInstance) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			for _, stage := range []string{"global", "type", "explicit", "explicit-zero"} {
				t.Run(stage, func(t *testing.T) {
					candidate := descriptor.constructor()
					base, _ := widgetBaseOf(candidate)
					base.Type = name
					base.configuredFields = make(yamlConfiguredFields)
					if custom, ok := candidate.(*customAPIWidget); ok {
						custom.CustomAPIRequest = &CustomAPIRequest{}
					}
					field := reflect.ValueOf(candidate).Elem().FieldByName("Timeout")
					if !field.IsValid() || !field.CanSet() {
						t.Fatal("timeout capability lacks a writable request option")
					}
					defaults := widgetDefaultsConfig{Global: widgetDefaultValues{Timeout: &global}}
					want := global
					if stage != "global" {
						defaults.Types = map[string]widgetDefaultValues{name: {Timeout: &typeValue}}
						want = typeValue
					}
					if stage == "explicit" || stage == "explicit-zero" {
						want = explicit
						if stage == "explicit-zero" {
							want = 0
						}
						field.SetInt(int64(want))
						base.configuredFields["timeout"] = true
					}
					if err := validateWidgetDefaults(defaults); err != nil {
						t.Fatal(err)
					}
					if err := applyWidgetCapabilityDefaults(candidate, defaults); err != nil {
						t.Fatal(err)
					}
					if got := durationField(field.Int()); got != want {
						t.Fatalf("timeout = %v, want %v", got, want)
					}
				})
			}
		})
	}
}

func TestTimeoutConsistencyCustomAPISubrequests(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    timeout: 7s
  types:
    custom-api:
      timeout: 11s
pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: custom-api
            url: https://example.invalid/main
            timeout: 19s
            template: test
            subrequests:
              inherited:
                url: https://example.invalid/inherited
              explicit:
                url: https://example.invalid/explicit
                timeout: 23s
              zero:
                url: https://example.invalid/zero
                timeout: 0s
`))
	if err != nil {
		t.Fatal(err)
	}
	w, ok := config.Pages[0].Columns[0].Widgets[0].(*customAPIWidget)
	if !ok {
		t.Fatalf("invalid configured widget: %T", config.Pages[0].Columns[0].Widgets[0])
	}
	if time.Duration(w.Timeout) != 19*time.Second || time.Duration(w.Subrequests["inherited"].Timeout) != 11*time.Second || time.Duration(w.Subrequests["explicit"].Timeout) != 23*time.Second || w.Subrequests["zero"].Timeout != 0 {
		t.Fatal("primary or subrequest timeout precedence changed")
	}
}

func TestTimeoutConsistencyBoundedDefaults(t *testing.T) {
	for _, value := range []durationField{0, durationField(-time.Second), durationField(15 * time.Second)} {
		want := defaultClientTimeout
		if value > 0 {
			want = time.Duration(value)
		}
		for _, insecure := range []bool{false, true} {
			if got := newHTTPClient(value, insecure).Timeout; got != want {
				t.Fatalf("timeout(%v, %t) = %v, want %v", value, insecure, got, want)
			}
		}
	}
	if time.Duration(*builtinWidgetDefaults["monitor"].Timeout) != defaultClientTimeout || time.Duration(*builtinWidgetDefaults["server-stats"].Timeout) != defaultClientTimeout {
		t.Fatal("monitor/server-stats do not use the central default")
	}
	if time.Duration(*builtinWidgetDefaults["dilbert"].Timeout) != dilbertHTTPTimeout {
		t.Fatal("archive exception was lost")
	}
}

// Inspect the deadline installed by http.Client on real outgoing requests.
// Returning a sentinel stops provider parsing without sleeping fifteen seconds.
// A hidden five-second client ceiling makes these assertions fail immediately.
func TestTimeoutConsistencyProviderDeadlines(t *testing.T) {
	timeout := durationField(15 * time.Second)
	probe := errors.New("stop after observing HTTP deadline")
	for _, tc := range []struct {
		name  string
		fetch func()
	}{
		{"weather-location", func() { _, _ = fetchOpenMeteoPlaceFromName(context.Background(), "Test", timeout) }},
		{"weather-forecast", func() {
			_, _ = fetchOpenMeteoWeatherResponse(context.Background(), &openMeteoPlaceResponseJson{}, "metric", timeout)
		}},
		{"air-quality", func() { _, _ = fetchOpenMeteoEnvironment(context.Background(), &openMeteoPlaceResponseJson{}, timeout) }},
		{"pollen", func() {
			_, _ = fetchAtmoSporePollen(context.Background(), &openMeteoPlaceResponseJson{Timezone: "UTC"}, "test", timeout)
		}},
		{"markets", func() { _, _ = fetchYahooMarketResourceUncached(context.Background(), "TEST", timeout) }},
		{"hacker-news-list", func() { _, _ = fetchHackerNewsPostIds(context.Background(), "top", timeout) }},
		{"hacker-news-post", func() { _, _ = fetchHackerNewsPostsFromIds(context.Background(), []int{1}, "", timeout) }},
		{"lobsters", func() {
			_, _ = fetchLobstersPostsFromFeed(context.Background(), "https://example.invalid/feed", timeout)
		}},
		{"twitch-games", func() { _, _ = fetchTopGamesFromTwitch(context.Background(), nil, 1, timeout) }},
		{"twitch-channels", func() { _, _ = fetchChannelsFromTwitch(context.Background(), []string{"test"}, timeout) }},
		{"releases-github", func() {
			_, _ = fetchLatestGithubRelease(context.Background(), &releaseRequest{Repository: "test/test"}, timeout)
		}},
		{"releases-ghcr", func() {
			_, _ = fetchLatestGHCRRelease(context.Background(), &releaseRequest{Repository: "test/test"}, timeout)
		}},
		{"releases-dockerhub", func() {
			_, _ = fetchLatestDockerHubRelease(context.Background(), &releaseRequest{Repository: "test/test"}, timeout)
		}},
		{"releases-gitlab", func() {
			_, _ = fetchLatestGitLabRelease(context.Background(), &releaseRequest{Repository: "test/test"}, timeout)
		}},
		{"releases-codeberg", func() {
			_, _ = fetchLatestCodebergRelease(context.Background(), &releaseRequest{Repository: "test/test"}, timeout)
		}},
		{"repository-rest", func() {
			_, _ = fetchRepositoryDetailsFromGithubREST(context.Background(), "test/test", "", 1, 1, 1, timeout)
		}},
		{"repository-graphql", func() {
			_, _ = fetchRepositoryDetailsFromGithubGraphQL(context.Background(), "test/test", "test", 1, 1, 1, timeout)
		}},
		{"monitor", func() {
			_, _ = fetchMonitorSiteResourceUncached(context.Background(), &SiteStatusRequest{DefaultURL: "https://example.invalid/status", Timeout: timeout})
		}},
		{"server-stats", func() {
			_, _ = fetchRemoteServerInfo(context.Background(), &serverStatsRequest{URL: "https://example.invalid", Timeout: timeout})
		}},
		{"docker-http", func() {
			_, _ = fetchDockerContainersFromSource(context.Background(), "http://example.invalid", true, nil, timeout)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			wave3Transport(t, func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				deadline, ok := r.Context().Deadline()
				if !ok {
					t.Error("missing request deadline")
				} else if remaining := time.Until(deadline); remaining < 13*time.Second || remaining > 15*time.Second {
					t.Errorf("request budget = %v, want 15s without a hidden ceiling", remaining)
				}
				return nil, probe
			})
			tc.fetch()
			if calls.Load() == 0 {
				t.Fatal("test did not reach the HTTP transport")
			}
		})
	}
}

func TestTimeoutConsistencySharedCachePolicies(t *testing.T) {
	resetYahooMarketResourceCache(t)
	var calls atomic.Int32
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return yahooMarketResourceTestResponse(r, "TEST"), nil
	})
	for _, value := range []durationField{0, durationField(5 * time.Second), durationField(15 * time.Second), durationField(15 * time.Second)} {
		if _, err := fetchYahooMarketResource(context.Background(), "TEST", value); err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("HTTP calls = %d, want one per effective timeout policy", got)
	}
	if resourceTimeoutKey("a:1", durationField(15*time.Second)) == resourceTimeoutKey("a", durationField(time.Second)) {
		t.Fatal("resource delimiter collision")
	}
}

func TestTimeoutConsistencyDailyAndVideoDeadlines(t *testing.T) {
	for _, tc := range []struct {
		name   string
		widget widget
	}{
		{"word", &wordOfTheDayWidget{Timeout: durationField(15 * time.Second)}},
		{"quote", &quoteOfTheDayWidget{Timeout: durationField(15 * time.Second)}},
		{"on-this-day", &onThisDayWidget{Timeout: durationField(15 * time.Second), Limit: 1}},
		{"animal", &animalOfTheDayWidget{Timeout: durationField(15 * time.Second)}},
		{"trivia", &triviaWidget{Timeout: durationField(15 * time.Second)}},
		{"nasa", &nasaAPODWidget{Timeout: durationField(15 * time.Second)}},
		{"videos-with-fallbacks", &videosWidget{Timeout: durationField(15 * time.Second), Channels: []string{"UCtest"}}},
		{"dilbert", &dilbertWidget{Timeout: durationField(15 * time.Second), ArchiveURL: "https://example.invalid", Date: "2020-01-01"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			wave3Transport(t, func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) < 13*time.Second || time.Until(deadline) > 15*time.Second {
					t.Errorf("missing or incorrect 15s request budget")
				}
				return nil, context.DeadlineExceeded
			})
			tc.widget.update(context.Background())
			want := int32(1)
			if tc.name == "videos-with-fallbacks" {
				want = 3
			}
			if calls.Load() != want {
				t.Fatalf("requests = %d, want %d including fallbacks", calls.Load(), want)
			}
		})
	}
}

func TestTimeoutConsistencySharedFetchDeadlineIsolation(t *testing.T) {
	resetYahooMarketResourceCache(t)
	started := make(chan struct{}, 2)
	release := make(chan struct{}, 1)
	defer close(release)
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		started <- struct{}{}
		select {
		case <-release:
			return yahooMarketResourceTestResponse(r, "TEST"), nil
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	})
	results := make(chan error, 2)
	for _, budget := range []time.Duration{40 * time.Millisecond, time.Second} {
		go func(budget time.Duration) {
			_, err := fetchYahooMarketResource(context.Background(), "TEST", durationField(budget))
			results <- err
		}(budget)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("different policies unexpectedly coalesced or failed to start")
		}
	}
	select {
	case err := <-results:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("short policy did not enforce its own deadline: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("short-policy request did not time out")
	}
	// The long-policy request is still waiting; let it finish successfully.
	release <- struct{}{}
	select {
	case err := <-results:
		if err != nil {
			t.Fatalf("short policy canceled or failed the longer policy: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("long-policy request did not complete")
	}
}

func TestTimeoutConsistencyDockerUnixCancellation(t *testing.T) {
	listener, err := net.Listen("unix", filepath.Join(t.TempDir(), "docker.sock"))
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EAFNOSUPPORT) {
			t.Skipf("Unix sockets unavailable in this test environment: %v", err)
		}
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = fetchDockerContainersFromSource(ctx, listener.Addr().String(), true, nil, durationField(25*time.Millisecond))
	if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatalf("expected configured socket deadline before parent deadline, got %v", err)
	}
}

func TestTimeoutConsistencyRedditBrowserConfigPreserved(t *testing.T) {
	w := &redditWidget{Subreddit: "docker", UsePlaywright: true, browserConfig: browserTestConfig()}
	w.Type = "reddit"
	w.configuredFields = make(yamlConfiguredFields)
	before := w.browserConfig
	value := durationField(17 * time.Second)
	if err := applyWidgetCapabilityDefaults(w, widgetDefaultsConfig{Global: widgetDefaultValues{Timeout: &value}}); err != nil {
		t.Fatal(err)
	}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if w.Timeout != value || !w.UsePlaywright || w.browserConfig != before || !strings.Contains(w.browserRequestURL(), "/r/docker/") {
		t.Fatal("timeout inheritance changed authenticated browser configuration")
	}
}

func TestTimeoutConsistencyRedditCDPDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, cleanup, err := redditBrowserConnect(ctx, server.URL, durationField(25*time.Millisecond))
	if cleanup != nil {
		cleanup()
	}
	if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatalf("expected configured CDP HTTP timeout, got %v", err)
	}
}

func TestTimeoutConsistencyRedditProxyPrecedence(t *testing.T) {
	w := &redditWidget{Timeout: durationField(17 * time.Second)}
	w.Proxy.client = &http.Client{Timeout: defaultClientTimeout, Transport: http.DefaultTransport}
	for _, child := range []durationField{0, durationField(23 * time.Second)} {
		w.Proxy.Timeout = child
		got := w.redditProxyHTTPClient()
		want := 17 * time.Second
		if child > 0 {
			want = time.Duration(child)
		}
		if got.Timeout != want || got.Transport != w.Proxy.client.Transport || w.Proxy.client.Timeout != defaultClientTimeout {
			t.Fatal("proxy timeout precedence or transport isolation changed")
		}
	}
}

func TestTimeoutConsistencySearchRedirectsPreserved(t *testing.T) {
	endpoint, err := parseSearchSuggestionEndpoint("https://example.invalid/suggest?q={QUERY}")
	if err != nil {
		t.Fatal(err)
	}
	client := newSearchSuggestionHTTPClient(endpoint, durationField(17*time.Second))
	other, _ := url.Parse("https://other.invalid/")
	if client.Timeout != 17*time.Second || client.CheckRedirect(&http.Request{URL: other}, []*http.Request{{URL: endpoint}}) == nil {
		t.Fatal("timeout override removed redirect restrictions")
	}
}
