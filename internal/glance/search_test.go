package glance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSearchSuggestionDestinationRemainsConfigured(t *testing.T) {
	endpoint, err := parseSearchSuggestionEndpoint("https://suggest.example/complete?fixed=yes&q={QUERY}&copy=prefix-{QUERY}")
	if err != nil {
		t.Fatal(err)
	}
	original := endpoint.String()
	for _, query := range []string{"https://evil.example/private", "//127.0.0.1/admin", "@evil.example/#", "a&url=http://127.0.0.1", "../private?x=1", "a+b /%#?", "東京"} {
		request, err := newSearchSuggestionRequest(context.Background(), endpoint, query)
		if err != nil {
			t.Fatal(err)
		}
		if request.URL.Scheme != "https" || request.URL.Host != "suggest.example" || request.URL.Path != "/complete" || request.URL.User != nil || request.URL.Fragment != "" {
			t.Fatalf("query changed request destination: %q", query)
		}
		if request.URL.Query().Get("q") != query || request.URL.Query().Get("copy") != "prefix-"+query || request.URL.Query().Get("fixed") != "yes" || len(request.URL.Query()) != 3 {
			t.Fatalf("query escaped its parameter value: %q", query)
		}
	}
	if endpoint.String() != original {
		t.Fatal("shared endpoint mutated by request construction")
	}
	for _, configured := range searchSuggestionEngines {
		if _, err := parseSearchSuggestionEndpoint(configured); err != nil {
			t.Fatalf("preset rejected: %v", err)
		}
	}
	for _, configured := range []string{"https://{QUERY}/complete", "https://example.com/{QUERY}", "https://example.com/?{QUERY}=q", "https://user:pass@example.com/?q={QUERY}", "https://example.com/?q={QUERY}#fragment", "https://example.com/?q=literal", "file:///tmp?q={QUERY}"} {
		if _, err := parseSearchSuggestionEndpoint(configured); err == nil {
			t.Errorf("unsafe template accepted: %s", configured)
		}
	}
}

func TestSearchSuggestionRedirectPolicy(t *testing.T) {
	endpoint, err := parseSearchSuggestionEndpoint("https://suggest.example/complete?q={QUERY}")
	if err != nil {
		t.Fatal(err)
	}
	client := newSearchSuggestionHTTPClient(endpoint)
	for _, destination := range []string{"https://evil.example/", "http://suggest.example/", "https://suggest.example:444/", "https://user:pass@suggest.example/"} {
		target, _ := url.Parse(destination)
		if client.CheckRedirect(&http.Request{URL: target}, []*http.Request{{URL: endpoint}}) == nil {
			t.Fatalf("redirect accepted: %s", destination)
		}
	}
	allowed, _ := url.Parse("https://suggest.example:443/other?q=hello")
	if err := client.CheckRedirect(&http.Request{URL: allowed}, []*http.Request{{URL: endpoint}}); err != nil {
		t.Fatalf("same-origin redirect rejected: %v", err)
	}
	if client.CheckRedirect(&http.Request{URL: allowed}, make([]*http.Request, 10)) == nil {
		t.Fatal("redirect loop limit missing")
	}
	if client.Timeout != defaultHTTPClient.Timeout || client.Transport != defaultHTTPClient.Transport {
		t.Fatal("suggestion client lost shared HTTP behavior")
	}
}

func TestSearchSuggestionCrossOriginRedirectNeverReachesTarget(t *testing.T) {
	var called atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called.Store(true); w.WriteHeader(http.StatusOK) }))
	defer destination.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/same" {
			_, _ = io.WriteString(w, `["query",["safe"]]`)
			return
		}
		if r.URL.Query().Get("q") == "same" {
			http.Redirect(w, r, "/same", http.StatusFound)
			return
		}
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer provider.Close()
	endpoint, err := parseSearchSuggestionEndpoint(provider.URL + "/complete?q={QUERY}")
	if err != nil {
		t.Fatal(err)
	}
	client := newSearchSuggestionHTTPClient(endpoint)
	request, _ := newSearchSuggestionRequest(context.Background(), endpoint, "cross")
	if response, err := client.Do(request); err == nil {
		_ = response.Body.Close()
		t.Fatal("cross-origin redirect followed")
	}
	if called.Load() {
		t.Fatal("request reached cross-origin target")
	}
	request, _ = newSearchSuggestionRequest(context.Background(), endpoint, "same")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatal("same-origin redirect did not succeed")
	}
}

func TestSearchOptionsDefaultsAndValidation(t *testing.T) {
	defaults := &searchWidget{}
	if err := defaults.initialize(); err != nil {
		t.Fatal(err)
	}
	if defaults.Suggestions || defaults.History.Storage != "off" || len(defaults.LocalSearch.Sources) != 0 || defaults.FocusKey != "s" || defaults.UseLayout {
		t.Fatal("existing configurations unexpectedly enable Search features")
	}
	for _, test := range []struct {
		name   string
		widget *searchWidget
	}{
		{"scope", &searchWidget{LocalSearch: searchLocalConfig{Scope: "private"}}},
		{"source", &searchWidget{LocalSearch: searchLocalConfig{Sources: []string{"secrets"}}}},
		{"result cap", &searchWidget{ResultsLimit: 21}},
		{"shortcut URL", &searchWidget{Shortcuts: []searchShortcut{{Title: "bad", URL: "javascript:alert(1)"}}}},
		{"duplicate alias", &searchWidget{Shortcuts: []searchShortcut{{Title: "a", URL: "/a", Alias: "go"}, {Title: "b", URL: "/b", Alias: "GO"}}}},
		{"history ID", &searchWidget{History: searchHistoryConfig{Storage: "server", ID: "nested/id"}}},
		{"history cap", &searchWidget{History: searchHistoryConfig{Limit: 101}}},
		{"history retention", &searchWidget{History: searchHistoryConfig{RetentionDays: -1}}},
		{"suggestion endpoint", &searchWidget{Suggestions: true, SuggestionEngine: "file:///tmp/{QUERY}"}},
		{"focus key", &searchWidget{FocusKey: "Enter"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.widget.initialize(); err == nil {
				t.Fatal("invalid option accepted")
			}
		})
	}
	configured := &searchWidget{Suggestions: true, SuggestionEngine: "google", UseLayout: true, History: searchHistoryConfig{Storage: "browser", ID: "home"}}
	if err := configured.initialize(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configured.Render()), `data-use-layout="true"`) {
		t.Fatal("layout option not rendered")
	}
	if strings.Contains(string(configured.Render()), "suggestqueries.google.com") {
		t.Fatal("suggestion endpoint leaked into shared markup")
	}
}

func TestSearchLocalAuthorizationScopesAndRoutes(t *testing.T) {
	app := &application{Config: config{Pages: []page{{Title: "Home", Slug: "home"}, {Title: "Private", Slug: "private"}, {Title: "Shared", Slug: "shared"}}}}
	app.Config.Server.BaseURL = "/glance"
	home, private, shared := &app.Config.Pages[0], &app.Config.Pages[1], &app.Config.Pages[2]
	publicDashboard := &dashboard{Name: "Default", Pages: []*page{home, shared}}
	adminDashboard := &dashboard{Name: "Admin", Slug: "admin", Pages: []*page{private, shared}}
	app.dashboards = []*dashboard{publicDashboard, adminDashboard}
	app.authorization = &authorizationPolicy{enabled: true,
		dashboardIdentities: map[*dashboard]map[string]struct{}{publicDashboard: {"user": {}}, adminDashboard: {"admin": {}}},
		pageDashboards:      map[*page][]*dashboard{home: {publicDashboard}, private: {adminDashboard}, shared: {publicDashboard, adminDashboard}},
	}
	bookmarks := &bookmarksWidget{Groups: []bookmarkGroup{{Links: []bookmarkLink{{Title: "Service Private", URL: "https://private.example"}}}}}
	bookmarks.setID(1)
	private.HeadWidgets = widgets{bookmarks}
	shared.HeadWidgets = widgets{&bookmarksWidget{Groups: []bookmarkGroup{{Links: []bookmarkLink{{Title: "Service Shared", URL: "https://shared.example"}, {Title: "Service Unsafe", URL: "javascript:alert(1)"}}}}}}
	search := &searchWidget{LocalSearch: searchLocalConfig{Scope: "all", Sources: []string{"pages", "bookmarks"}}, ResultsLimit: 20}
	results := app.localSearchResults(search, home, publicDashboard, "user", "Service")
	if len(results) != 1 || results[0].Title != "Service Shared" {
		t.Fatalf("unauthorized/unsafe targets exposed: %+v", results)
	}
	results = app.localSearchResults(search, private, adminDashboard, "admin", "Service")
	if len(results) != 2 {
		t.Fatalf("authorized targets missing: %+v", results)
	}
	if route, ok := app.searchPageRoute("admin", shared, publicDashboard); !ok || route != "/glance/admin/shared" {
		t.Fatalf("shared page route bypassed accessible dashboard: %q", route)
	}
	search.LocalSearch.Scope = "page"
	if got := app.localSearchResults(search, home, publicDashboard, "user", "Service"); len(got) != 0 {
		t.Fatalf("page scope crossed pages: %+v", got)
	}
	search.LocalSearch.Scope = "dashboard"
	if got := app.localSearchResults(search, home, publicDashboard, "user", "Service"); len(got) != 1 {
		t.Fatalf("dashboard scope failed: %+v", got)
	}
	search.setID(9)
	app.widgetPages = map[uint64][]*page{9: {home}}
	app.slugToPage = map[string]*page{"home": home, "private": private}
	r := httptest.NewRequest(http.MethodGet, "/?page=private&dashboard=admin", nil)
	if _, _, ok := app.searchContext(r, search, "admin"); ok {
		t.Fatal("forged originating page accepted")
	}
}

func TestSearchLiveSnapshotsDoNotExposeCheckURLs(t *testing.T) {
	monitor := &monitorWidget{Sites: []monitorSite{{Title: "Service", SiteStatusRequest: &SiteStatusRequest{CheckURL: "https://secret.example/check"}}}}
	monitor.publishSearchTargets()
	if monitor.searchTargets.read()[0].URL != "" {
		t.Fatal("health-check URL used as navigation destination")
	}
	monitor.Sites[0].DefaultURL = "https://public.example"
	monitor.publishSearchTargets()
	if monitor.searchTargets.read()[0].URL != "https://public.example" {
		t.Fatal("monitor destination missing")
	}
	docker := &dockerContainersWidget{Containers: dockerContainerList{{Name: "Parent", URL: "https://parent.example", Children: dockerContainerList{{Name: "Child", URL: "https://child.example"}}}}}
	docker.publishSearchTargets()
	if len(docker.searchTargets.read()) != 2 {
		t.Fatal("nested Docker destinations missing")
	}
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		for i := 0; i < 100; i++ {
			monitor.searchTargets.publish([]searchResult{{Title: "new"}})
		}
	}()
	go func() {
		defer workers.Done()
		for i := 0; i < 100; i++ {
			_ = monitor.searchTargets.read()
		}
	}()
	workers.Wait()
}

func TestSearchSuggestionsParsing(t *testing.T) {
	for _, body := range []string{`["query",["first","second","first"]]`, `[{"phrase":"first"},{"phrase":"second"},{"phrase":"first"}]`} {
		got, err := parseSearchSuggestions([]byte(body), 2)
		if err != nil || len(got) != 2 || got[0] != "first" || got[1] != "second" {
			t.Fatalf("parse %s: %v %v", body, got, err)
		}
	}
	for _, body := range []string{`{}`, `["query",null]`, `["query",[42]]`, `[{"unexpected":"data"}]`, `broken`} {
		if _, err := parseSearchSuggestions([]byte(body), 8); err == nil {
			t.Errorf("malformed suggestions accepted: %s", body)
		}
	}
}

type searchTestTransport func(*http.Request) (*http.Response, error)

func (f searchTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSearchEndpointsAuthorizeAndBoundProviderRequests(t *testing.T) {
	app := newAuthTestApplication(t)
	search := &searchWidget{Suggestions: true, SuggestionEngine: "https://suggest.example?q={QUERY}"}
	search.setID(12)
	if err := search.initialize(); err != nil {
		t.Fatal(err)
	}
	app.widgetByID = map[uint64]widget{12: search}
	app.globalWidgetIDs = map[uint64]struct{}{12: {}}
	r := httptest.NewRequest(http.MethodGet, "/api/search/12/suggestions?q=hello", nil)
	r.SetPathValue("widget", "12")
	w := httptest.NewRecorder()
	app.handleSearchSuggestions(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status %d", w.Code)
	}
	r.AddCookie(&http.Cookie{Name: AUTH_SESSION_COOKIE_NAME, Value: authTestSessionToken(t, app, "test-user", time.Now())})
	previous := defaultHTTPClient.Transport
	defer func() { defaultHTTPClient.Transport = previous }()
	defaultHTTPClient.Transport = searchTestTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "suggest.example" || request.URL.Query().Get("q") != "hello" {
			t.Fatalf("unexpected configured request: %s", request.URL.Host)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`["hello",["hello world"]]`))}, nil
	})
	w = httptest.NewRecorder()
	app.handleSearchSuggestions(w, r)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `["hello world"]` || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
	for i := 0; i < cap(search.suggestionSlots); i++ {
		search.suggestionSlots <- struct{}{}
	}
	w = httptest.NewRecorder()
	app.handleSearchSuggestions(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatal("concurrency bound not enforced")
	}
	for i := 0; i < cap(search.suggestionSlots); i++ {
		<-search.suggestionSlots
	}
	defaultHTTPClient.Transport = searchTestTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 256*1024+1)))}, nil
	})
	w = httptest.NewRecorder()
	app.handleSearchSuggestions(w, r)
	if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), "suggest.example") {
		t.Fatal("oversized provider payload not safely contained")
	}
}

func TestSearchHistoryPayloadAndIdentityIsolation(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `[{"query":"a","at":0}]`, `[{"query":"","at":123}]`} {
		if validSearchHistoryPayload([]byte(body)) {
			t.Fatalf("invalid history accepted: %s", body)
		}
	}
	if !validSearchHistoryPayload([]byte(`[]`)) {
		t.Fatal("clearing history rejected")
	}
	store, err := newPersonalStateStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	payload := json.RawMessage(`[{"query":"private search","at":123}]`)
	if err := store.put("alice", "search", "home", payload); err != nil {
		t.Fatal(err)
	}
	if _, err := store.get("bob", "search", "home"); err == nil {
		t.Fatal("history crossed user identities")
	}
}
