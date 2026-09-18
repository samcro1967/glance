package glance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestComprehensiveExtensionInitializeAndContent(t *testing.T) {
	if err := (&extensionWidget{}).initialize(); err == nil {
		t.Fatal("expected missing URL error")
	}
	w := &extensionWidget{URL: "https://example.invalid/widget"}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if w.Title != extensionWidgetDefaultTitle || w.cacheDuration != 30*time.Minute {
		t.Fatalf("defaults not applied: %#v", w)
	}

	escaped := convertExtensionContent(extensionRequestOptions{}, []byte("<b>x</b>"), extensionContentHTML)
	if string(escaped) != "<pre>&lt;b&gt;x&lt;/b&gt;</pre>" {
		t.Fatalf("escaped=%q", escaped)
	}
	raw := convertExtensionContent(extensionRequestOptions{AllowHtml: true}, []byte("<b>x</b>"), extensionContentHTML)
	if string(raw) != "<b>x</b>" {
		t.Fatalf("raw=%q", raw)
	}
}

func TestComprehensiveFetchExtensionHeadersParametersAndFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "value" {
			t.Errorf("query=%q", r.URL.RawQuery)
		}
		if r.Header.Get("X-Test") != "yes" {
			t.Errorf("header=%q", r.Header.Get("X-Test"))
		}
		rw.Header().Set(extensionHeaderTitle, "Remote")
		rw.Header().Set(extensionHeaderTitleURL, "https://example.invalid/title")
		rw.Header().Set(extensionHeaderContentFrameless, "true")
		_, _ = rw.Write([]byte("<em>body</em>"))
	}))
	defer server.Close()

	got, err := fetchExtension(context.Background(), extensionRequestOptions{
		URL: server.URL, FallbackContentType: "html", AllowHtml: true,
		Parameters: queryParametersField{"q": []string{"value"}}, Headers: map[string]string{"X-Test": "yes"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Remote" || got.TitleURL != "https://example.invalid/title" || !got.Frameless || string(got.Content) != "<em>body</em>" {
		t.Fatalf("extension=%#v", got)
	}
}

func TestComprehensiveFetchExtensionDefaultsAndTransportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { _, _ = rw.Write([]byte("<x>")) }))
	got, err := fetchExtension(context.Background(), extensionRequestOptions{URL: server.URL})
	server.Close()
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Extension" || string(got.Content) != "<pre>&lt;x&gt;</pre>" {
		t.Fatalf("extension=%#v", got)
	}

	_, err = fetchExtension(context.Background(), extensionRequestOptions{URL: server.URL})
	if err == nil || !errors.Is(err, errNoContent) {
		t.Fatalf("expected classified transport failure, got %v", err)
	}
}

func TestComprehensiveChangeDetectionInitializeAndSort(t *testing.T) {
	w := &changeDetectionWidget{}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if w.Limit != 10 || w.CollapseAfter != 5 || w.InstanceURL != "https://www.changedetection.io" || w.LinkURL != w.InstanceURL || w.Title != "Change Detection" {
		t.Fatalf("defaults=%#v", w)
	}

	configured := &changeDetectionWidget{
		InstanceURL: "https://changedetection.internal///",
		LinkURL:     "https://changes.example.com///",
	}
	if err := configured.initialize(); err != nil {
		t.Fatal(err)
	}
	if configured.InstanceURL != "https://changedetection.internal" {
		t.Fatalf("InstanceURL=%q", configured.InstanceURL)
	}
	if configured.LinkURL != "https://changes.example.com" {
		t.Fatalf("LinkURL=%q", configured.LinkURL)
	}

	defaultLink := &changeDetectionWidget{InstanceURL: "https://changedetection.internal///"}
	if err := defaultLink.initialize(); err != nil {
		t.Fatal(err)
	}
	if defaultLink.LinkURL != "https://changedetection.internal" {
		t.Fatalf("default LinkURL=%q", defaultLink.LinkURL)
	}

	list := changeDetectionWatchList{{Title: "old", LastChanged: time.Unix(1, 0)}, {Title: "new", LastChanged: time.Unix(2, 0)}}.sortByNewest()
	if list[0].Title != "new" {
		t.Fatalf("sort=%#v", list)
	}
}

func TestComprehensiveChangeDetectionFetchesUUIDsAndWatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "token" {
			t.Errorf("token=%q", r.Header.Get("x-api-key"))
		}
		switch r.URL.Path {
		case "/api/v1/watch":
			_, _ = rw.Write([]byte(`{"watch-1":{},"watch-2":{},"watch-3":{}}`))
		case "/api/v1/watch/watch-1":
			_, _ = rw.Write([]byte(`{"title":"Explicit","page_title":"Ignored Page Title","url":"https://www.example.invalid/a","last_changed":20,"date_created":10,"previous_md5":"1234567890"}`))
		case "/api/v1/watch/watch-2":
			_, _ = rw.Write([]byte(`{"title":"","page_title":"Page B","url":"https://www.example.invalid/b/","last_changed":0,"date_created":30,"previous_md5":"abc"}`))
		case "/api/v1/watch/watch-3":
			_, _ = rw.Write([]byte(`{"title":"","page_title":"","url":"https://www.example.invalid/c/","last_changed":10,"date_created":5,"previous_md5":"def"}`))
		default:
			http.NotFound(rw, r)
		}
	}))
	defer server.Close()

	ids, err := fetchWatchUUIDsFromChangeDetection(context.Background(), server.URL, "token", 0, false, nil)
	if err != nil || len(ids) != 3 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}

	watches, err := fetchWatchesFromChangeDetection(context.Background(), server.URL, server.URL, []string{"watch-1", "watch-2", "watch-3"}, "token", 0, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(watches) != 3 {
		t.Fatalf("watches=%#v", watches)
	}

	byURL := make(map[string]changeDetectionWatch, len(watches))
	for _, watch := range watches {
		byURL[watch.URL] = watch
	}

	explicit := byURL["https://www.example.invalid/a"]
	if explicit.Title != "Explicit" || explicit.PreviousHash != "12345678" {
		t.Fatalf("explicit=%#v", explicit)
	}

	pageTitle := byURL["https://www.example.invalid/b/"]
	if pageTitle.Title != "Page B" || pageTitle.PreviousHash != "abc" || !pageTitle.LastChanged.Equal(time.Unix(30, 0)) {
		t.Fatalf("page title fallback=%#v", pageTitle)
	}

	urlFallback := byURL["https://www.example.invalid/c/"]
	if urlFallback.Title != "example.invalid/c" {
		t.Fatalf("URL fallback=%#v", urlFallback)
	}
}

func TestComprehensiveChangeDetectionPartialAndEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/good") {
			_, _ = rw.Write([]byte(`{"title":"Good","url":"https://example.invalid","last_changed":2}`))
			return
		}
		http.Error(rw, "bad", http.StatusInternalServerError)
	}))
	defer server.Close()
	watches, err := fetchWatchesFromChangeDetection(context.Background(), server.URL, server.URL, []string{"good", "bad"}, "", 0, false, nil)
	if len(watches) != 1 || err == nil || !errors.Is(err, errPartialContent) {
		t.Fatalf("watches=%#v err=%v", watches, err)
	}
	empty, err := fetchWatchesFromChangeDetection(context.Background(), server.URL, server.URL, nil, "", 0, false, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty=%#v err=%v", empty, err)
	}
}

func TestComprehensiveDockerHelpers(t *testing.T) {
	labels := dockerContainerLabels{dockerContainerLabelName: "Override"}
	c := &dockerContainerJsonResponse{Names: []string{"/my_app-name"}, Labels: labels, State: "running", Status: "Up"}
	if deriveDockerContainerName(c, false) != "Override" {
		t.Fatal("label override ignored")
	}
	delete(labels, dockerContainerLabelName)
	if deriveDockerContainerName(c, true) != "My App Name" {
		t.Fatalf("name=%q", deriveDockerContainerName(c, true))
	}
	if dockerContainerStateToStateIcon(c) != dockerContainerStateIconOK {
		t.Fatal("running state")
	}
	c.Status = "Up (unhealthy)"
	if dockerContainerStateToStateIcon(c) != dockerContainerStateIconWarn {
		t.Fatal("unhealthy state")
	}
	c.Status = "Paused"
	c.State = "paused"
	if dockerContainerStateToStateIcon(c) != dockerContainerStateIconPaused {
		t.Fatal("paused state")
	}
	c.State = "created"
	if dockerContainerStateToStateIcon(c) != dockerContainerStateIconOther {
		t.Fatal("other state")
	}
}

func TestComprehensiveDockerGroupingVisibilityAndSorting(t *testing.T) {
	containers := []dockerContainerJsonResponse{
		{Names: []string{"/parent"}, Labels: dockerContainerLabels{dockerContainerLabelID: "p"}},
		{Names: []string{"/child"}, Labels: dockerContainerLabels{dockerContainerLabelParent: "p"}},
		{Names: []string{"/hidden"}, Labels: dockerContainerLabels{dockerContainerLabelHide: "true"}},
	}
	parents, children := groupDockerContainerChildren(containers, false, "")
	if len(parents) != 1 || len(children["p"]) != 1 {
		t.Fatalf("parents=%#v children=%#v", parents, children)
	}
	if !isDockerContainerHidden(&containers[2], false) {
		t.Fatal("explicit hide ignored")
	}
	list := dockerContainerList{{Name: "z", StateIcon: dockerContainerStateIconOK}, {Name: "b", StateIcon: dockerContainerStateIconWarn}, {Name: "A", StateIcon: dockerContainerStateIconOK}}
	list.sortByStateIconThenName()
	if list[0].Name != "b" || list[1].Name != "A" {
		t.Fatalf("sort=%#v", list)
	}
}

func TestComprehensiveDockerRemoteFetchOverridesAndAll(t *testing.T) {
	var rawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`[{"Names":["/one"],"Image":"img","State":"running","Status":"Up","Labels":{"glance.category":"old"}},{"Names":["/two"],"Labels":{"glance.category":"other"}}]`))
	}))
	defer server.Close()
	got, err := fetchDockerContainersFromSource(context.Background(), server.URL, false, map[string]map[string]string{"one": {"category": "new", "name": "One Override"}})
	if err != nil {
		t.Fatal(err)
	}
	if rawQuery != "all=true" {
		t.Fatalf("query=%q", rawQuery)
	}
	if len(got) != 2 || got[0].Labels[dockerContainerLabelName] != "One Override" {
		t.Fatalf("containers=%#v", got)
	}
}

func TestComprehensiveDockerRemoteFetchErrorsAndURL(t *testing.T) {
	tests := map[string]string{"tcp://example.invalid": "http://example.invalid:80", "https://example.invalid": "https://example.invalid:443", "http://example.invalid:1234": "http://example.invalid:1234"}
	for in, want := range tests {
		got, err := dockerContainersRemoteSourceURL(in)
		if err != nil || got != want {
			t.Fatalf("%s => %q %v", in, got, err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { http.Error(rw, "no", http.StatusBadGateway) }))
	_, err := fetchDockerContainersFromSource(context.Background(), server.URL, false, nil)
	server.Close()
	if err == nil || !strings.Contains(err.Error(), "Docker API request") {
		t.Fatalf("err=%v", err)
	}
}
