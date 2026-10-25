package glance

// Search endpoints adapt existing widget destinations and provider HTTP helpers.
// They never refresh widgets, persist queries, or put user-specific data in the
// shared render cache. Authorization is evaluated for every request.
import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var searchSuggestionEngines = map[string]string{
	"google":     "https://suggestqueries.google.com/complete/search?client=firefox&q={QUERY}",
	"duckduckgo": "https://duckduckgo.com/ac/?q={QUERY}&type=list",
	"bing":       "https://api.bing.com/osjson.aspx?query={QUERY}",
	"startpage":  "https://www.startpage.com/suggestions?q={QUERY}&format=opensearch",
}

// Compile the administrator-controlled destination once. Search input may fill
// query parameter values only; it cannot select a host, path, or parameter name.
func parseSearchSuggestionEndpoint(configured string) (*url.URL, error) {
	endpoint, err := url.Parse(configured)
	if err != nil || endpoint.Host == "" || endpoint.Hostname() == "" ||
		(endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil ||
		endpoint.Fragment != "" || strings.Contains(endpoint.Path, "{QUERY}") || strings.Contains(endpoint.Host, "{QUERY}") {
		return nil, errors.New("suggestion-engine must be an HTTP(S) endpoint with {QUERY} only in query parameter values")
	}
	parameters, err := url.ParseQuery(endpoint.RawQuery)
	if err != nil {
		return nil, errors.New("suggestion-engine has invalid query parameters")
	}
	hasPlaceholder := false
	for name, values := range parameters {
		if strings.Contains(name, "{QUERY}") {
			return nil, errors.New("suggestion-engine cannot contain {QUERY} in parameter names")
		}
		for _, value := range values {
			hasPlaceholder = hasPlaceholder || strings.Contains(value, "{QUERY}")
		}
	}
	if !hasPlaceholder {
		return nil, errors.New("suggestion-engine requires {QUERY} in a query parameter value")
	}
	return endpoint, nil
}

func newSearchSuggestionRequest(ctx context.Context, endpoint *url.URL, query string) (*http.Request, error) {
	if endpoint == nil {
		return nil, errors.New("suggestion endpoint is not configured")
	}
	// Construct the request from configuration alone. Only RawQuery receives user
	// input, encoded with url.Values; the destination fields remain untouched.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	parameters := endpoint.Query()
	for name, values := range parameters {
		for i, value := range values {
			values[i] = strings.ReplaceAll(value, "{QUERY}", query)
		}
		parameters[name] = values
	}
	request.URL.RawQuery = parameters.Encode()
	return request, nil
}

func newSearchSuggestionHTTPClient(endpoint *url.URL) *http.Client {
	// Keep the shared observed transport and timeout, but do not allow a remote
	// provider to redirect requests to another origin or embed URL credentials.
	client := *defaultHTTPClient
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("suggestion provider exceeded redirect limit")
		}
		if endpoint == nil || request.URL.User != nil || !extensionURLsShareOrigin(endpoint, request.URL) {
			return errors.New("suggestion provider redirect is outside the configured origin")
		}
		return nil
	}
	return &client
}

func validSearchHistoryPayload(body []byte) bool {
	var entries []struct {
		Query string `json:"query"`
		At    int64  `json:"at"`
	}
	trimmed := strings.TrimSpace(string(body))
	if !strings.HasPrefix(trimmed, "[") || json.Unmarshal(body, &entries) != nil || len(entries) > 100 {
		return false
	}
	for _, entry := range entries {
		if strings.TrimSpace(entry.Query) == "" || len(entry.Query) > 512 || entry.At <= 0 {
			return false
		}
	}
	return true
}

type searchResult struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Target      string `json:"target,omitempty"`
	Page        string `json:"page,omitempty"`
}

// Live widgets publish only navigation fields under a separate lock. Searching
// therefore cannot race with a refresh or wait on slow provider/network work.
type searchTargetSnapshot struct {
	mu    sync.RWMutex
	items []searchResult
}

func (s *searchTargetSnapshot) publish(items []searchResult) {
	s.mu.Lock()
	s.items = items
	s.mu.Unlock()
}

func (s *searchTargetSnapshot) read() []searchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]searchResult(nil), s.items...)
}

func (widget *monitorWidget) publishSearchTargets() {
	items := make([]searchResult, 0, len(widget.Sites))
	for _, site := range widget.Sites {
		destination := site.URL
		if destination == "" && site.SiteStatusRequest != nil {
			destination = site.DefaultURL
		}
		target := "_blank"
		if site.SameTab {
			target = "_self"
		}
		items = append(items, searchResult{Kind: "monitor", Title: site.Title, URL: destination, Description: site.Description, Target: target})
	}
	widget.searchTargets.publish(items)
}

func (widget *dockerContainersWidget) publishSearchTargets() {
	var items []searchResult
	var visit func(dockerContainerList)
	visit = func(containers dockerContainerList) {
		for _, container := range containers {
			target := "_blank"
			if container.SameTab {
				target = "_self"
			}
			items = append(items, searchResult{Kind: "docker-containers", Title: container.Name, URL: container.URL, Description: container.Description, Target: target})
			visit(container.Children)
		}
	}
	visit(widget.Containers)
	widget.searchTargets.publish(items)
}

func searchPageWidgets(p *page) widgets {
	var source widgets
	source = append(source, p.HeadWidgets...)
	for _, column := range p.Columns {
		source = append(source, column.Widgets...)
	}
	source = append(source, p.BottomWidgets...)
	return collectRefreshWidgets(source)
}

// The browser supplies context, never authorization. In particular a widget on
// several pages must not let a caller impersonate an unrelated page/dashboard.
func (a *application) authorizeSearch(w http.ResponseWriter, r *http.Request) (*searchWidget, authenticatedSession, bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	session, ok := a.authorizeSession(w, r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return nil, session, false
	}
	id, err := strconv.ParseUint(r.PathValue("widget"), 10, 64)
	candidate, exists := a.widgetByID[id]
	search, isSearch := candidate.(*searchWidget)
	if err != nil || !exists || !isSearch || !a.canAccessWidget(session.AuthorizationIdentity, id) {
		http.NotFound(w, r)
		return nil, session, false
	}
	return search, session, true
}

func (a *application) searchContext(r *http.Request, widget *searchWidget, identity string) (*page, *dashboard, bool) {
	p := a.slugToPage[r.URL.Query().Get("page")]
	if p == nil || !a.authorization.canAccessPage(identity, p) {
		return nil, nil, false
	}
	belongs := false
	for _, candidate := range a.widgetPages[widget.ID] {
		if candidate == p {
			belongs = true
			break
		}
	}
	if !belongs {
		return nil, nil, false
	}
	if len(a.dashboards) == 0 {
		return p, nil, true
	}
	slug := strings.Trim(r.URL.Query().Get("dashboard"), "/")
	for _, dashboard := range a.dashboards {
		if dashboard.Slug != slug || !a.authorization.canAccessDashboard(identity, dashboard) {
			continue
		}
		for _, candidate := range dashboard.Pages {
			if candidate == p {
				return p, dashboard, true
			}
		}
	}
	return nil, nil, false
}

func (a *application) searchPageRoute(identity string, p *page, preferred *dashboard) (string, bool) {
	if !a.authorization.canAccessPage(identity, p) {
		return "", false
	}
	if len(a.dashboards) == 0 {
		return a.Config.Server.BaseURL + "/" + p.Slug, true
	}
	dashboards := append([]*dashboard{preferred}, a.dashboards...)
	for _, dashboard := range dashboards {
		if dashboard == nil || !a.authorization.canAccessDashboard(identity, dashboard) {
			continue
		}
		for _, candidate := range dashboard.Pages {
			if candidate == p {
				prefix := ""
				if dashboard.Slug != "" {
					prefix = "/" + dashboard.Slug
				}
				return a.Config.Server.BaseURL + prefix + "/" + p.Slug, true
			}
		}
	}
	return "", false
}

func (a *application) localSearchResults(widget *searchWidget, own *page, dashboard *dashboard, identity, query string) []searchResult {
	sources := make(map[string]bool)
	for _, source := range widget.LocalSearch.Sources {
		sources[source] = true
	}
	pages := pagePointers(a.Config.Pages)
	if widget.LocalSearch.Scope == "page" {
		pages = []*page{own}
	} else if widget.LocalSearch.Scope == "dashboard" && dashboard != nil {
		pages = dashboard.Pages
	}
	var matches []searchResult
	seen := make(map[string]bool)
	add := func(item searchResult, p *page) {
		key := item.Kind + "\x00" + item.Title + "\x00" + item.URL
		if seen[key] || !safeSearchDestination(item.URL) || searchMatchScore(query, item.Title+" "+item.Description+" "+p.Title) == 0 {
			return
		}
		seen[key] = true
		item.Page = p.Title
		matches = append(matches, item)
	}
	for _, p := range pages {
		route, allowed := a.searchPageRoute(identity, p, dashboard)
		if !allowed {
			continue
		}
		if sources["pages"] {
			add(searchResult{Kind: "pages", Title: p.Title, URL: route, Target: "_self"}, p)
		}
		for _, candidate := range searchPageWidgets(p) {
			switch source := candidate.(type) {
			case *bookmarksWidget:
				if !sources["bookmarks"] {
					continue
				}
				for _, group := range source.Groups {
					for _, link := range group.Links {
						target := link.Target
						if target == "" {
							if link.SameTab {
								target = "_self"
							} else {
								target = "_blank"
							}
						}
						add(searchResult{Kind: "bookmarks", Title: link.Title, URL: link.URL, Description: link.Description, Target: target}, p)
					}
				}
			case *monitorWidget:
				if sources["monitor"] {
					for _, item := range source.searchTargets.read() {
						add(item, p)
					}
				}
			case *dockerContainersWidget:
				if sources["docker-containers"] {
					for _, item := range source.searchTargets.read() {
						add(item, p)
					}
				}
			}
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		a, b := searchMatchScore(query, matches[i].Title), searchMatchScore(query, matches[j].Title)
		if a != b {
			return a > b
		}
		return strings.ToLower(matches[i].Title) < strings.ToLower(matches[j].Title)
	})
	if len(matches) > widget.ResultsLimit {
		matches = matches[:widget.ResultsLimit]
	}
	if matches == nil {
		matches = []searchResult{}
	}
	return matches
}

func searchMatchScore(query, text string) int {
	query, text = strings.ToLower(strings.TrimSpace(query)), strings.ToLower(text)
	if query == "" {
		return 0
	}
	if query == text {
		return 4
	}
	if strings.HasPrefix(text, query) {
		return 3
	}
	if strings.Contains(text, query) {
		return 2
	}
	// Subsequence matching supports short abbreviations without a dependency.
	remaining := []rune(query)
	for _, character := range text {
		if len(remaining) > 0 && character == remaining[0] {
			remaining = remaining[1:]
		}
	}
	if len(remaining) == 0 {
		return 1
	}
	return 0
}

func (a *application) handleSearchTargets(w http.ResponseWriter, r *http.Request) {
	widget, session, ok := a.authorizeSearch(w, r)
	if !ok {
		return
	}
	own, dashboard, ok := a.searchContext(r, widget, session.AuthorizationIdentity)
	if !ok {
		http.NotFound(w, r)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 512 {
		http.Error(w, "Query too long", http.StatusBadRequest)
		return
	}
	results := a.localSearchResults(widget, own, dashboard, session.AuthorizationIdentity, query)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

func (a *application) handleSearchSuggestions(w http.ResponseWriter, r *http.Request) {
	widget, _, ok := a.authorizeSearch(w, r)
	if !ok {
		return
	}
	if !widget.Suggestions {
		http.NotFound(w, r)
		return
	}
	// Bound concurrent provider work even when a client ignores browser debounce.
	select {
	case widget.suggestionSlots <- struct{}{}:
		defer func() { <-widget.suggestionSlots }()
	default:
		http.Error(w, "Suggestions busy", http.StatusTooManyRequests)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 || len(query) > 512 {
		http.Error(w, "Query must be 2–512 bytes", http.StatusBadRequest)
		return
	}
	request, err := newSearchSuggestionRequest(r.Context(), widget.suggestionEndpoint, query)
	if err != nil {
		http.Error(w, "Suggestion request unavailable", http.StatusBadGateway)
		return
	}
	response, err := newSearchSuggestionHTTPClient(widget.suggestionEndpoint).Do(request)
	if err != nil {
		http.Error(w, "Suggestion provider unavailable", http.StatusBadGateway)
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		http.Error(w, "Suggestion provider unavailable", http.StatusBadGateway)
		return
	}
	body, err := readHTTPResponseBody(response.Body, 256*1024)
	if err != nil {
		http.Error(w, "Invalid suggestion response", http.StatusBadGateway)
		return
	}
	suggestions, err := parseSearchSuggestions(body, widget.ResultsLimit)
	if err != nil {
		http.Error(w, "Invalid suggestion response", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(suggestions)
}

// Accept OpenSearch arrays and DuckDuckGo's phrase objects. Malformed data is
// isolated to suggestions; it must never mark the whole Search widget failed.
func parseSearchSuggestions(body []byte, limit int) ([]string, error) {
	if !strings.HasPrefix(strings.TrimSpace(string(body)), "[") || limit < 1 {
		return nil, errors.New("invalid suggestions")
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, errors.New("invalid suggestions")
	}
	values := []string{}
	if len(raw) >= 2 && len(raw[0]) > 0 && raw[0][0] == '"' {
		if err := json.Unmarshal(raw[1], &values); err != nil {
			return nil, errors.New("invalid suggestions")
		}
		if values == nil {
			return nil, errors.New("invalid suggestions")
		}
	} else {
		for _, entry := range raw {
			var item struct {
				Phrase string `json:"phrase"`
			}
			if err := json.Unmarshal(entry, &item); err != nil || item.Phrase == "" {
				return nil, errors.New("invalid suggestions")
			}
			values = append(values, item.Phrase)
		}
	}
	result := []string{}
	seen := make(map[string]bool)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 512 || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}
