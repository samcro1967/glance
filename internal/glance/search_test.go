package glance

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

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
