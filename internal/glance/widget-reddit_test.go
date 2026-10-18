package glance

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRedditWidgetInitializeRequiresSubreddit(t *testing.T) {
	widget := &redditWidget{}

	err := widget.initialize()
	if err == nil {
		t.Fatal("expected missing subreddit error")
	}

	if err.Error() != "subreddit is required" {
		t.Fatalf("error = %q, want %q", err, "subreddit is required")
	}
}

func TestRedditWidgetInitializeDefaults(t *testing.T) {
	widget := &redditWidget{
		Subreddit: "example",
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if widget.Limit != 15 {
		t.Fatalf("limit = %d, want 15", widget.Limit)
	}

	if widget.CollapseAfter != 5 {
		t.Fatalf("collapse after = %d, want 5", widget.CollapseAfter)
	}

	if widget.SortBy != "hot" {
		t.Fatalf("sort by = %q, want %q", widget.SortBy, "hot")
	}

	if widget.TopPeriod != "day" {
		t.Fatalf("top period = %q, want %q", widget.TopPeriod, "day")
	}

	if widget.Title != "r/example" {
		t.Fatalf("title = %q, want %q", widget.Title, "r/example")
	}

	if widget.TitleURL != "https://www.reddit.com/r/example/" {
		t.Fatalf(
			"title URL = %q, want %q",
			widget.TitleURL,
			"https://www.reddit.com/r/example/",
		)
	}

	if widget.cacheDuration != 30*time.Minute {
		t.Fatalf(
			"cache duration = %s, want %s",
			widget.cacheDuration,
			30*time.Minute,
		)
	}
}

func TestRedditWidgetInitializePreservesValidOptions(t *testing.T) {
	widget := &redditWidget{
		Subreddit:     "example",
		Limit:         30,
		CollapseAfter: -1,
		SortBy:        "top",
		TopPeriod:     "year",
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if widget.Limit != 30 {
		t.Fatalf("limit = %d, want 30", widget.Limit)
	}

	if widget.CollapseAfter != -1 {
		t.Fatalf("collapse after = %d, want -1", widget.CollapseAfter)
	}

	if widget.SortBy != "top" {
		t.Fatalf("sort by = %q, want %q", widget.SortBy, "top")
	}

	if widget.TopPeriod != "year" {
		t.Fatalf("top period = %q, want %q", widget.TopPeriod, "year")
	}
}

func TestRedditWidgetInitializeNormalizesInvalidOptions(t *testing.T) {
	widget := &redditWidget{
		Subreddit:     "example",
		Limit:         -5,
		CollapseAfter: -2,
		SortBy:        "invalid",
		TopPeriod:     "invalid",
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if widget.Limit != 15 {
		t.Fatalf("limit = %d, want 15", widget.Limit)
	}

	if widget.CollapseAfter != 5 {
		t.Fatalf("collapse after = %d, want 5", widget.CollapseAfter)
	}

	if widget.SortBy != "hot" {
		t.Fatalf("sort by = %q, want %q", widget.SortBy, "hot")
	}

	if widget.TopPeriod != "day" {
		t.Fatalf("top period = %q, want %q", widget.TopPeriod, "day")
	}
}

func TestRedditWidgetInitializeAcceptsValidSortValues(t *testing.T) {
	validSorts := []string{
		"hot",
		"new",
		"top",
		"rising",
	}

	for _, sortBy := range validSorts {
		t.Run(sortBy, func(t *testing.T) {
			widget := &redditWidget{
				Subreddit: "example",
				SortBy:    sortBy,
			}

			if err := widget.initialize(); err != nil {
				t.Fatalf("unexpected initialization error: %v", err)
			}

			if widget.SortBy != sortBy {
				t.Fatalf("sort by = %q, want %q", widget.SortBy, sortBy)
			}
		})
	}
}

func TestRedditWidgetInitializeAcceptsValidTopPeriods(t *testing.T) {
	validPeriods := []string{
		"hour",
		"day",
		"week",
		"month",
		"year",
		"all",
	}

	for _, period := range validPeriods {
		t.Run(period, func(t *testing.T) {
			widget := &redditWidget{
				Subreddit: "example",
				SortBy:    "top",
				TopPeriod: period,
			}

			if err := widget.initialize(); err != nil {
				t.Fatalf("unexpected initialization error: %v", err)
			}

			if widget.TopPeriod != period {
				t.Fatalf("top period = %q, want %q", widget.TopPeriod, period)
			}
		})
	}
}

func TestRedditWidgetInitializeValidatesRequestURLTemplate(t *testing.T) {
	widget := &redditWidget{
		Subreddit:          "example",
		RequestURLTemplate: "https://proxy.example.invalid/reddit",
	}

	err := widget.initialize()
	if err == nil {
		t.Fatal("expected request URL template validation error")
	}

	const want = "no `{REQUEST-URL}` placeholder specified"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

func TestRedditWidgetInitializeAcceptsRequestURLTemplate(t *testing.T) {
	widget := &redditWidget{
		Subreddit: "example",
		RequestURLTemplate: strings.Join(
			[]string{
				"https://proxy.example.invalid/fetch?url=",
				"{REQUEST-URL}",
			},
			"",
		),
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}
}

func TestRedditWidgetInitializeRequiresCompleteAppAuth(t *testing.T) {
	tests := []struct {
		name   string
		app    string
		id     string
		secret string
	}{
		{
			name: "name only",
			app:  "test-app",
		},
		{
			name: "ID only",
			id:   "test-id",
		},
		{
			name:   "secret only",
			secret: "test-secret",
		},
		{
			name: "missing secret",
			app:  "test-app",
			id:   "test-id",
		},
		{
			name:   "missing ID",
			app:    "test-app",
			secret: "test-secret",
		},
		{
			name:   "missing name",
			id:     "test-id",
			secret: "test-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			widget := &redditWidget{
				Subreddit: "example",
			}
			widget.AppAuth.Name = tt.app
			widget.AppAuth.ID = tt.id
			widget.AppAuth.Secret = tt.secret

			err := widget.initialize()
			if err == nil {
				t.Fatal("expected incomplete application authentication error")
			}

			const want = "application name, client ID and client secret are required"
			if err.Error() != want {
				t.Fatalf("error = %q, want %q", err, want)
			}

			if widget.AppAuth.enabled {
				t.Fatal("incomplete application authentication should not be enabled")
			}
		})
	}
}

func TestRedditWidgetInitializeEnablesCompleteAppAuth(t *testing.T) {
	widget := &redditWidget{
		Subreddit: "example",
	}
	widget.AppAuth.Name = "test-app"
	widget.AppAuth.ID = "test-id"
	widget.AppAuth.Secret = "test-secret"

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if !widget.AppAuth.enabled {
		t.Fatal("expected complete application authentication to be enabled")
	}
}

func TestRedditWidgetInitializeSearchAuthentication(t *testing.T) {
	tests := []struct {
		name       string
		search     string
		authName   string
		authID     string
		authSecret string
		wantError  string
	}{
		{
			name: "ordinary listing without authentication",
		},
		{
			name:      "search without authentication",
			search:    "example",
			wantError: "due to changes in Reddit API, the search property works only when app-auth is configured",
		},
		{
			name:      "search with incomplete authentication",
			search:    "example",
			authName:  "test-app",
			authID:    "test-id",
			wantError: "application name, client ID and client secret are required",
		},
		{
			name:       "search with complete authentication",
			search:     "example",
			authName:   "test-app",
			authID:     "test-id",
			authSecret: "test-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			widget := &redditWidget{
				Subreddit: "example",
				Search:    tt.search,
			}
			widget.AppAuth.Name = tt.authName
			widget.AppAuth.ID = tt.authID
			widget.AppAuth.Secret = tt.authSecret

			err := widget.initialize()

			if tt.wantError != "" {
				if err == nil || err.Error() != tt.wantError {
					t.Fatalf("initialize() error = %v, want %q", err, tt.wantError)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected initialization error: %v", err)
			}

			if widget.AppAuth.enabled != (tt.authSecret != "") {
				t.Fatalf("app-auth enabled = %v, unexpected for configuration", widget.AppAuth.enabled)
			}
		})
	}
}

func TestRedditWidgetParseCustomCommentsURL(t *testing.T) {
	widget := &redditWidget{
		CommentsURLTemplate: "https://comments.example.invalid/{SUBREDDIT}/{POST-ID}/{POST-PATH}",
	}

	got := widget.parseCustomCommentsURL(
		"example",
		"abc123",
		"/r/example/comments/abc123/example_post/",
	)

	const want = "https://comments.example.invalid/example/abc123/r/example/comments/abc123/example_post/"

	if got != want {
		t.Fatalf("custom comments URL = %q, want %q", got, want)
	}
}

func TestRedditWidgetParseCustomCommentsURLReplacesRepeatedPlaceholders(t *testing.T) {
	widget := &redditWidget{
		CommentsURLTemplate: "{SUBREDDIT}/{SUBREDDIT}/{POST-ID}/{POST-ID}/{POST-PATH}",
	}

	got := widget.parseCustomCommentsURL(
		"example",
		"abc123",
		"///comments/abc123/",
	)

	const want = "example/example/abc123/abc123/comments/abc123/"

	if got != want {
		t.Fatalf("custom comments URL = %q, want %q", got, want)
	}
}

type redditRequestDoerFunc func(*http.Request) (*http.Response, error)

func (f redditRequestDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

type redditChallengeTestDoer struct {
	mu       sync.Mutex
	requests []*http.Request
}

func (d *redditChallengeTestDoer) Do(request *http.Request) (*http.Response, error) {
	d.mu.Lock()
	d.requests = append(d.requests, request)
	call := len(d.requests)
	d.mu.Unlock()

	switch call {
	case 1:
		body := `
			<script>
				var n = await(async e=>e+e)("abc123");
			</script>
			<input name="jsc_token" value="token-value">
			<input name="jsc_orig_r" value="/r/test">
		`

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil

	case 2:
		header := make(http.Header)
		header.Add("Set-Cookie", "token_v2=test-token; Path=/")

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil

	default:
		return nil, nil
	}
}

func TestRedditGuestBearerRequestDestination(t *testing.T) {
	oldClient := redditHTTPClient
	oldToken := getRedditGuestToken
	t.Cleanup(func() {
		redditHTTPClient = oldClient
		getRedditGuestToken = oldToken
	})

	tokenCalls := 0
	getRedditGuestToken = func(ctx context.Context, route string, client requestDoer) (string, error) {
		tokenCalls++
		if route != "direct" {
			t.Fatalf("guest token route = %q, want direct", route)
		}
		return "guest-test-token", nil
	}

	redditHTTPClient = &http.Client{
		Transport: priorityRoundTripper(func(r *http.Request) (*http.Response, error) {
			if r.URL.Scheme != "https" || r.URL.Host != "oauth.reddit.com" {
				t.Fatalf("Reddit destination = %s, want https://oauth.reddit.com", r.URL)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer guest-test-token" {
				t.Fatalf("Authorization = %q, want guest bearer token", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"data":{"children":[{"data":{"id":"one","title":"One","ups":1,"created":1,"permalink":"/r/test/one"}}]}}`)),
				Request:    r,
			}, nil
		}),
	}

	widget := &redditWidget{Subreddit: "test", SortBy: "hot"}
	posts, err := widget.fetchSubredditPosts(t.Context())
	if err != nil {
		t.Fatalf("fetch Reddit posts: %v", err)
	}
	if len(posts) != 1 || tokenCalls != 1 {
		t.Fatalf("posts = %d, guest token calls = %d; want 1 each", len(posts), tokenCalls)
	}
}

func TestRedditAppOAuthBypassesGuestToken(t *testing.T) {
	oldClient := redditHTTPClient
	oldToken := getRedditGuestToken
	t.Cleanup(func() {
		redditHTTPClient = oldClient
		getRedditGuestToken = oldToken
	})

	getRedditGuestToken = func(context.Context, string, requestDoer) (string, error) {
		t.Fatal("application OAuth must not acquire a guest token")
		return "", nil
	}

	redditHTTPClient = &http.Client{
		Transport: priorityRoundTripper(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "oauth.reddit.com" {
				t.Fatalf("OAuth destination = %s", r.URL)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer application-test-token" {
				t.Fatalf("application Authorization = %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"data":{"children":[{"data":{"id":"one","title":"One","ups":1,"created":1,"permalink":"/r/test/one"}}]}}`)),
				Request:    r,
			}, nil
		}),
	}

	widget := &redditWidget{Subreddit: "test", SortBy: "hot"}
	widget.AppAuth.enabled = true
	widget.AppAuth.Name = "test-app"
	widget.AppAuth.accessToken = "application-test-token"
	widget.AppAuth.tokenExpiresAt = time.Now().Add(time.Hour)

	posts, err := widget.fetchSubredditPosts(t.Context())
	if err != nil {
		t.Fatalf("fetch Reddit posts with application OAuth: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("posts = %d, want 1", len(posts))
	}
}

func TestRedditRequestTemplateDoesNotForwardGuestToken(t *testing.T) {
	oldClient := redditHTTPClient
	oldToken := getRedditGuestToken
	t.Cleanup(func() {
		redditHTTPClient = oldClient
		getRedditGuestToken = oldToken
	})

	getRedditGuestToken = func(context.Context, string, requestDoer) (string, error) {
		t.Fatal("custom request template must not acquire a guest token")
		return "", nil
	}

	redditHTTPClient = &http.Client{
		Transport: priorityRoundTripper(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "proxy.example.invalid" {
				t.Fatalf("template destination = %s", r.URL)
			}
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("unexpected credential forwarded to template: %q", got)
			}
			if !strings.Contains(r.URL.Query().Get("url"), "oauth.reddit.com/r/test/hot.json") {
				t.Fatalf("unexpected wrapped Reddit URL: %s", r.URL)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"data":{"children":[{"data":{"id":"one","title":"One","ups":1,"created":1,"permalink":"/r/test/one"}}]}}`)),
				Request:    r,
			}, nil
		}),
	}

	widget := &redditWidget{
		Subreddit:          "test",
		SortBy:             "hot",
		RequestURLTemplate: "https://proxy.example.invalid/fetch?url={REQUEST-URL}",
	}

	posts, err := widget.fetchSubredditPosts(t.Context())
	if err != nil {
		t.Fatalf("fetch Reddit posts through request template: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("posts = %d, want 1", len(posts))
	}
}

func TestFetchRedditGuestTokenUsesSelectedClientForEntireChallenge(t *testing.T) {
	client := &redditChallengeTestDoer{}

	token, err := fetchRedditGuestToken(t.Context(), client)
	if err != nil {
		t.Fatalf("unexpected guest-token fetch error: %v", err)
	}

	if token != "test-token" {
		t.Fatalf("guest token = %q, want %q", token, "test-token")
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	if len(client.requests) != 2 {
		t.Fatalf("challenge requests = %d, want 2", len(client.requests))
	}

	first := client.requests[0]
	second := client.requests[1]

	if first.URL.String() != "https://www.reddit.com/" {
		t.Fatalf("first challenge URL = %q", first.URL.String())
	}

	query := second.URL.Query()

	if query.Get("solution") != "abc123abc123" {
		t.Fatalf("solution = %q, want %q", query.Get("solution"), "abc123abc123")
	}

	if query.Get("js_challenge") != "1" {
		t.Fatalf("js_challenge = %q, want %q", query.Get("js_challenge"), "1")
	}

	if query.Get("jsc_token") != "token-value" {
		t.Fatalf("jsc_token = %q, want %q", query.Get("jsc_token"), "token-value")
	}

	if query.Get("jsc_orig_r") != "/r/test" {
		t.Fatalf("jsc_orig_r = %q, want %q", query.Get("jsc_orig_r"), "/r/test")
	}
}

func resetRedditGuestTokenRoutesForTest() {
	redditGuestTokenRoutes.Lock()
	redditGuestTokenRoutes.states = make(map[string]*redditGuestTokenRouteState)
	redditGuestTokenRoutes.Unlock()
}

func TestGetRedditGuestTokenCachesWithinSameRoute(t *testing.T) {
	resetRedditGuestTokenRoutesForTest()
	t.Cleanup(resetRedditGuestTokenRoutesForTest)

	client := &redditChallengeTestDoer{}
	route := "proxy:http://proxy.example:8080"

	first, err := getRedditGuestToken(t.Context(), route, client)
	if err != nil {
		t.Fatalf("first guest-token fetch failed: %v", err)
	}

	second, err := getRedditGuestToken(t.Context(), route, client)
	if err != nil {
		t.Fatalf("second guest-token fetch failed: %v", err)
	}

	if first != "test-token" || second != "test-token" {
		t.Fatalf(
			"guest tokens = %q, %q; want %q, %q",
			first,
			second,
			"test-token",
			"test-token",
		)
	}

	client.mu.Lock()
	requests := len(client.requests)
	client.mu.Unlock()

	if requests != 2 {
		t.Fatalf(
			"challenge requests = %d, want 2 after two same-route guest-token requests",
			requests,
		)
	}
}

func TestGetRedditGuestTokenIsolatesDirectAndProxyRoutes(t *testing.T) {
	resetRedditGuestTokenRoutesForTest()
	t.Cleanup(resetRedditGuestTokenRoutesForTest)

	directClient := &redditChallengeTestDoer{}
	proxyClient := &redditChallengeTestDoer{}

	directToken, err := getRedditGuestToken(
		t.Context(),
		"direct",
		directClient,
	)
	if err != nil {
		t.Fatalf("direct guest-token fetch failed: %v", err)
	}

	proxyToken, err := getRedditGuestToken(
		t.Context(),
		"proxy:http://proxy.example:8080",
		proxyClient,
	)
	if err != nil {
		t.Fatalf("proxy guest-token fetch failed: %v", err)
	}

	if directToken != "test-token" {
		t.Fatalf("direct guest token = %q, want %q", directToken, "test-token")
	}

	if proxyToken != "test-token" {
		t.Fatalf("proxy guest token = %q, want %q", proxyToken, "test-token")
	}

	directClient.mu.Lock()
	directRequests := len(directClient.requests)
	directClient.mu.Unlock()

	proxyClient.mu.Lock()
	proxyRequests := len(proxyClient.requests)
	proxyClient.mu.Unlock()

	if directRequests != 2 {
		t.Fatalf("direct challenge requests = %d, want 2", directRequests)
	}

	if proxyRequests != 2 {
		t.Fatalf("proxy challenge requests = %d, want 2", proxyRequests)
	}
}

func TestGetRedditGuestTokenIsolatesProxyRoutes(t *testing.T) {
	resetRedditGuestTokenRoutesForTest()
	t.Cleanup(resetRedditGuestTokenRoutesForTest)

	proxyA := &redditChallengeTestDoer{}
	proxyB := &redditChallengeTestDoer{}

	_, err := getRedditGuestToken(
		t.Context(),
		"proxy:http://proxy-a.example:8080",
		proxyA,
	)
	if err != nil {
		t.Fatalf("proxy A guest-token fetch failed: %v", err)
	}

	_, err = getRedditGuestToken(
		t.Context(),
		"proxy:http://proxy-b.example:8080",
		proxyB,
	)
	if err != nil {
		t.Fatalf("proxy B guest-token fetch failed: %v", err)
	}

	proxyA.mu.Lock()
	proxyARequests := len(proxyA.requests)
	proxyA.mu.Unlock()

	proxyB.mu.Lock()
	proxyBRequests := len(proxyB.requests)
	proxyB.mu.Unlock()

	if proxyARequests != 2 {
		t.Fatalf("proxy A challenge requests = %d, want 2", proxyARequests)
	}

	if proxyBRequests != 2 {
		t.Fatalf("proxy B challenge requests = %d, want 2", proxyBRequests)
	}
}

func TestFetchRedditGuestTokenPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := redditRequestDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Context().Err() != context.Canceled {
			t.Fatalf(
				"request context error = %v, want %v",
				request.Context().Err(),
				context.Canceled,
			)
		}

		return nil, context.Canceled
	})

	_, err := fetchRedditGuestToken(ctx, client)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestParseRedditChallengeForm(t *testing.T) {
	body := []byte(`
		<script>
			document.addEventListener("DOMContentLoaded", async function() {
				var n = await(async e=>e+e)("abc123");
			});
		</script>
		<form hidden method="GET" action="/">
			<input type="hidden" name="solution" />
			<input type="hidden" name="js_challenge" value="1"/>
			<input type="hidden" name="jsc_token" value="token-value"/>
			<input type="hidden" name="jsc_orig_r" value=""/>
		</form>
	`)

	challenge, token, origR, err := parseRedditChallengeForm(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if challenge != "abc123" {
		t.Fatalf("challenge = %q, want %q", challenge, "abc123")
	}

	if token != "token-value" {
		t.Fatalf("token = %q, want %q", token, "token-value")
	}

	if origR != "" {
		t.Fatalf("jsc_orig_r = %q, want empty", origR)
	}
}

func TestParseRedditChallengeFormMissingFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "challenge",
			body: `<input name="jsc_token" value="token"><input name="jsc_orig_r" value="">`,
			want: "no JS challenge found",
		},
		{
			name: "token",
			body: `await(async e=>e+e)("abc123")<input name="jsc_orig_r" value="">`,
			want: "no jsc_token found in challenge page",
		},
		{
			name: "original return",
			body: `await(async e=>e+e)("abc123")<input name="jsc_token" value="token">`,
			want: "no jsc_orig_r found in challenge page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := parseRedditChallengeForm([]byte(tt.body))
			if err == nil {
				t.Fatal("expected error")
			}

			if err.Error() != tt.want {
				t.Fatalf("error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestRedditFetchErrorGuidance(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		usePlaywright bool
		wantGuidance  bool
	}{
		{
			name:         "HTTP 403 suggests authentication",
			err:          &httpStatusError{StatusCode: http.StatusForbidden, Status: "403 Forbidden"},
			wantGuidance: true,
		},
		{
			name:          "Playwright HTTP 403 retains original error",
			err:           &httpStatusError{StatusCode: http.StatusForbidden, Status: "403 Forbidden"},
			usePlaywright: true,
		},
		{
			name: "HTTP 429 does not suggest authentication",
			err:  &httpStatusError{StatusCode: http.StatusTooManyRequests, Status: "429 Too Many Requests"},
		},
		{
			name: "unrelated errors remain unchanged",
			err:  errors.New("connection failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redditFetchError(tt.err, tt.usePlaywright)
			if !errors.Is(got, tt.err) {
				t.Fatalf("original error not preserved: %v", got)
			}

			hasGuidance := strings.Contains(got.Error(), "Reddit may be restricting API access")
			if hasGuidance != tt.wantGuidance {
				t.Fatalf("guidance = %t, want %t: %v", hasGuidance, tt.wantGuidance, got)
			}

			if tt.wantGuidance {
				if classifyRefreshFailure(got) != refreshFailureAuthorization {
					t.Fatalf("HTTP 403 classification changed: %v", got)
				}
				if !strings.Contains(got.Error(), "403 Forbidden") {
					t.Fatalf("HTTP status missing: %v", got)
				}
			}
		})
	}
}
