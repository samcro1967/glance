package glance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestSafeExternalURLRejectsUnsafeSchemes(t *testing.T) {
	for _, value := range []string{
		"javascript:alert(1)",
		"data:text/html,boom",
		"//example.com/path",
	} {
		if got := safeExternalURL(value); got != "" {
			t.Fatalf("safeExternalURL(%q) = %q, want empty", value, got)
		}
	}

	for _, value := range []string{
		"https://example.com/path",
		"http://example.com/path",
		"/relative/path",
	} {
		if got := safeExternalURL(value); got != value {
			t.Fatalf("safeExternalURL(%q) = %q, want unchanged", value, got)
		}
	}
}

func TestICSEventTemplateRejectsUnsafeExternalURL(t *testing.T) {
	widget := &icsEventsWidget{
		widgetBase: widgetBase{Type: "ics-events"},
		Events:     []icsEvent{{Title: "Unsafe", URL: "javascript:alert(1)"}},
	}
	rendered := string(widget.Render())
	if strings.Contains(strings.ToLower(rendered), "javascript:") {
		t.Fatalf("rendered ICS event retained unsafe URL: %s", rendered)
	}
}

func TestFrontendDiagnosticCommandsRequireOperatorAccess(t *testing.T) {
	app, _, _ := stateAuthBoundaryTestApplication(t)
	app.Config.Server.FrontendDiagnostics = true

	subscription, unsubscribe := app.liveUpdates.subscribe(nil)
	defer unsubscribe()

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/frontend-diagnostics/runtime-state",
		nil,
	)
	addStateAuthBoundarySession(t, app, request)
	recorder := httptest.NewRecorder()
	app.handleFrontendRuntimeStateRequest(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if commands := subscription.takeDiagnosticCommands(); len(commands) != 0 {
		t.Fatalf("restricted identity published %d diagnostic commands", len(commands))
	}
}

func TestPersonalStateNamespaceCapacityIsBounded(t *testing.T) {
	store := &personalStateStore{
		path: t.TempDir() + "/state.json",
		data: personalStateDocument{
			Version: personalStateDocumentVersion,
			Users:   make(map[string]map[string]map[string]json.RawMessage),
		},
	}
	identity := "user"
	identityKey := personalStateIdentityKey(identity)
	store.data.Users[identityKey] = map[string]map[string]json.RawMessage{
		"todo": {},
	}
	for index := 0; index < personalStateMaxEntriesPerNamespace; index++ {
		store.data.Users[identityKey]["todo"][fmt.Sprintf("id-%d", index)] = json.RawMessage(`{"ok":true}`)
	}

	err := store.put(identity, "todo", "overflow", json.RawMessage(`{"ok":true}`))
	if err == nil || !strings.Contains(err.Error(), "capacity exceeded") {
		t.Fatalf("put() error = %v, want capacity error", err)
	}
}

func TestCustomAPIRegexpCacheIsBounded(t *testing.T) {
	cache := newCustomAPIRegexpCache()
	for index := 0; index < customAPIRegexpCacheCapacity+32; index++ {
		cache.get(fmt.Sprintf("^value-%d$", index))
	}
	if got := cache.len(); got != customAPIRegexpCacheCapacity {
		t.Fatalf("cache length = %d, want %d", got, customAPIRegexpCacheCapacity)
	}
}

func TestProxyOptionsScalarRetainsURL(t *testing.T) {
	var field proxyOptionsField
	if err := yaml.Unmarshal([]byte("http://user:secret@proxy.example.com:8080\n"), &field); err != nil {
		t.Fatal(err)
	}
	if field.URL != "http://user:secret@proxy.example.com:8080" {
		t.Fatalf("URL = %q, want scalar proxy URL", field.URL)
	}
}

func TestRedditProxyRouteIdentityDoesNotExposeCredentials(t *testing.T) {
	route := redditProxyRouteIdentity("http://user:secret@proxy.example.com:8080")
	if strings.Contains(route, "user") || strings.Contains(route, "secret") || strings.Contains(route, "proxy.example.com") {
		t.Fatalf("route identity exposed proxy details: %q", route)
	}
	if route == redditProxyRouteIdentity("http://other:secret@proxy.example.com:8080") {
		t.Fatal("distinct proxy routes produced the same identity")
	}
}

func TestRedditLoidRouteStateIsBounded(t *testing.T) {
	redditLoidRoutes.Lock()
	redditLoidRoutes.states = make(map[string]*redditLoidRouteState)
	redditLoidRoutes.Unlock()

	for index := 0; index < redditLoidRouteStateLimit+16; index++ {
		redditLoidRouteStateFor(fmt.Sprintf("route-%d", index))
	}

	redditLoidRoutes.Lock()
	got := len(redditLoidRoutes.states)
	redditLoidRoutes.Unlock()
	if got != redditLoidRouteStateLimit {
		t.Fatalf("route states = %d, want %d", got, redditLoidRouteStateLimit)
	}
}

func TestAuthenticatedWidgetContentIsPrivateNoStore(t *testing.T) {
	app, _, _ := stateAuthBoundaryTestApplication(t)
	request := httptest.NewRequest(http.MethodGet, "/api/widgets/1/content/", nil)
	request.SetPathValue("widget", "1")
	addStateAuthBoundarySession(t, app, request)
	recorder := httptest.NewRecorder()

	app.handleWidgetContentRequest(recorder, request)
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", got)
	}
}

func TestAddressOfRequestWalksTrustedProxyChain(t *testing.T) {
	app := newProxyTrustTestApplication(
		t,
		true,
		[]string{"192.0.2.10", "198.51.100.0/24"},
	)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.77, 198.51.100.20")
	if got := app.addressOfRequest(request); got != "203.0.113.77" {
		t.Fatalf("addressOfRequest() = %q, want client address", got)
	}

	app = newProxyTrustTestApplication(t, true, []string{"192.0.2.10"})
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.77, 198.51.100.20")
	if got := app.addressOfRequest(request); got != "198.51.100.20" {
		t.Fatalf("addressOfRequest() = %q, want first untrusted proxy", got)
	}
}

func TestPersonalStateCapacityResponseIsRequestEntityTooLarge(t *testing.T) {
	app := newAuthTestApplication(t)
	app.Config.Server.PersonalState.Enabled = true
	app.personalState = &personalStateStore{
		path: t.TempDir() + "/state.json",
		data: personalStateDocument{
			Version: personalStateDocumentVersion,
			Users:   make(map[string]map[string]map[string]json.RawMessage),
		},
	}
	identity := "test-user"
	key := personalStateIdentityKey(identity)
	app.personalState.data.Users[key] = map[string]map[string]json.RawMessage{"todo": {}}
	for index := 0; index < personalStateMaxEntriesPerNamespace; index++ {
		app.personalState.data.Users[key]["todo"][fmt.Sprintf("id-%d", index)] = json.RawMessage(`true`)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/personal-state/todo/overflow", strings.NewReader(`true`))
	request.SetPathValue("namespace", "todo")
	request.SetPathValue("id", "overflow")
	request.AddCookie(&http.Cookie{
		Name:  AUTH_SESSION_COOKIE_NAME,
		Value: authTestSessionToken(t, app, identity, time.Now()),
	})
	recorder := httptest.NewRecorder()
	app.handlePersonalStatePostRequest(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestConfiguredImageIconsUseResourceProxy(t *testing.T) {
	app := &application{}
	app.Config.Server.BaseURL = "/glance"
	proxy, err := newResourceProxy([]string{"http://logs.loc"})
	if err != nil {
		t.Fatal(err)
	}
	app.resourceProxy = proxy
	providers := &widgetProviders{resourceProxyURL: app.resolveResourceProxyURL}

	widget := &monitorWidget{
		widgetBase: widgetBase{Icon: newCustomIconField("http://logs.loc/icons/widget.png")},
		Sites:      []monitorSite{{Icon: newCustomIconField("http://logs.loc/icons/site.png")}},
	}
	widget.setProviders(providers)

	if got := string(widget.Icon.URL); got != "http://logs.loc/icons/widget.png" {
		t.Fatalf("configured widget icon mutated to %q", got)
	}

	for label, got := range map[string]string{
		"widget": string(widget.Icon.RenderURL()),
		"site":   string(widget.Sites[0].Icon.RenderURL()),
	} {
		if !strings.HasPrefix(got, "/glance/api/resource-proxy/") {
			t.Fatalf("%s icon = %q, want resource proxy URL", label, got)
		}
		if strings.Contains(got, "http://logs.loc") {
			t.Fatalf("%s icon leaked HTTP upstream URL: %q", label, got)
		}
	}

	bookmark := &bookmarksWidget{
		Groups: []bookmarkGroup{{Links: []bookmarkLink{{Icon: newCustomIconField("http://logs.loc/icons/bookmark.png")}}}},
	}
	bookmark.setProviders(providers)
	if got := string(bookmark.Groups[0].Links[0].Icon.RenderURL()); !strings.HasPrefix(got, "/glance/api/resource-proxy/") {
		t.Fatalf("bookmark icon = %q, want resource proxy URL", got)
	}
}

func TestConfiguredImageProxyLeavesIneligibleURLUnchanged(t *testing.T) {
	app := &application{}
	proxy, err := newResourceProxy([]string{"http://logs.loc"})
	if err != nil {
		t.Fatal(err)
	}
	app.resourceProxy = proxy
	providers := &widgetProviders{resourceProxyURL: app.resolveResourceProxyURL}

	widget := &monitorWidget{widgetBase: widgetBase{Icon: newCustomIconField("https://example.com/icon.png")}}
	widget.setProviders(providers)
	if got := string(widget.Icon.RenderURL()); got != "https://example.com/icon.png" {
		t.Fatalf("HTTPS icon = %q, want unchanged URL", got)
	}
}

func TestStatusBarCustomAPIIconsUseResourceProxy(t *testing.T) {
	app := &application{}
	proxy, err := newResourceProxy([]string{"http://logs.loc"})
	if err != nil {
		t.Fatal(err)
	}
	app.resourceProxy = proxy
	providers := &widgetProviders{resourceProxyURL: app.resolveResourceProxyURL}

	child := &customAPIWidget{
		widgetBase: widgetBase{Providers: providers},
		StatusBarCompactItems: []statusBarCustomAPIItem{{
			Icon1: "http://logs.loc/icons/one.png",
			Line1: "status",
			Icon2: "http://logs.loc/icons/two.png",
		}},
	}
	widget := &statusBarWidget{containerWidgetBase: containerWidgetBase{Widgets: widgets{child}}}
	items := widget.CompactItems()
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if !strings.HasPrefix(items[0].Icon1, "/api/resource-proxy/") || !strings.HasPrefix(items[0].Icon2, "/api/resource-proxy/") {
		t.Fatalf("status bar icons = %q, %q, want resource proxy URLs", items[0].Icon1, items[0].Icon2)
	}
}
