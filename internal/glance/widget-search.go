package glance

import (
	"fmt"
	"html/template"
	"net/url"
	"strings"
)

var searchWidgetTemplate = mustParseTemplate("search.html", "widget-base.html")

type SearchBang struct {
	Title    string
	Shortcut string
	URL      string
	RawQuery bool `yaml:"raw-query"`
}

type searchShortcut struct {
	Title string `yaml:"title"`
	URL   string `yaml:"url"`
	Alias string `yaml:"alias"`
}

type searchLocalConfig struct {
	Sources []string `yaml:"sources"`
	Scope   string   `yaml:"scope"`
}

type searchHistoryConfig struct {
	Storage       string `yaml:"storage"`
	ID            string `yaml:"id"`
	Limit         int    `yaml:"limit"`
	RetentionDays int    `yaml:"retention-days"`
}

type searchWidget struct {
	widgetBase         `yaml:",inline"`
	Timeout            durationField       `yaml:"timeout"`
	cachedHTML         template.HTML       `yaml:"-"`
	SearchEngine       string              `yaml:"search-engine"`
	Bangs              []SearchBang        `yaml:"bangs"`
	NewTab             bool                `yaml:"new-tab"`
	Target             string              `yaml:"target"`
	Autofocus          bool                `yaml:"autofocus"`
	Placeholder        string              `yaml:"placeholder"`
	OpenDomains        bool                `yaml:"open-domains"`
	Shortcuts          []searchShortcut    `yaml:"shortcuts"`
	LocalSearch        searchLocalConfig   `yaml:"local-search"`
	Suggestions        bool                `yaml:"suggestions"`
	SuggestionEngine   string              `yaml:"suggestion-engine"`
	History            searchHistoryConfig `yaml:"history"`
	UseLayout          bool                `yaml:"use-layout"`
	FocusKey           string              `yaml:"focus-key"`
	ResultsLimit       int                 `yaml:"results-limit"`
	suggestionSlots    chan struct{}
	suggestionEndpoint *url.URL
}

func convertSearchUrl(url string) string {
	// Go's template is being stubborn and continues to escape the curlies in the
	// URL regardless of what the type of the variable is so this is my way around it
	return strings.ReplaceAll(url, "{QUERY}", "!QUERY!")
}

var searchEngines = map[string]string{
	"duckduckgo": "https://duckduckgo.com/?q={QUERY}",
	"google":     "https://www.google.com/search?q={QUERY}",
	"bing":       "https://www.bing.com/search?q={QUERY}",
	"brave":      "https://search.brave.com/search?q={QUERY}",
	"perplexity": "https://www.perplexity.ai/search?q={QUERY}",
	"kagi":       "https://kagi.com/search?q={QUERY}",
	"startpage":  "https://www.startpage.com/search?q={QUERY}",
}

func (widget *searchWidget) initialize() error {
	widget.withTitle("Search").withError(nil)
	// Validate before rendering so malformed options use the normal source-aware
	// widget error presentation, rather than failing only after browser input.
	if err := widget.initializeSearchOptions(); err != nil {
		return err
	}

	if widget.SearchEngine == "" {
		widget.SearchEngine = "duckduckgo"
	}

	if widget.Placeholder == "" {
		widget.Placeholder = "Type here to search…"
	}

	if url, ok := searchEngines[widget.SearchEngine]; ok {
		widget.SearchEngine = url
	}

	widget.SearchEngine = convertSearchUrl(widget.SearchEngine)

	for i := range widget.Bangs {
		if widget.Bangs[i].Shortcut == "" {
			return fmt.Errorf("search bang #%d has no shortcut", i+1)
		}

		if widget.Bangs[i].URL == "" {
			return fmt.Errorf("search bang #%d has no URL", i+1)
		}

		widget.Bangs[i].URL = convertSearchUrl(widget.Bangs[i].URL)
	}

	widget.cacheStaticTemplate(widget, searchWidgetTemplate, &widget.cachedHTML)
	return nil
}

func (widget *searchWidget) initializeSearchOptions() error {
	if widget.ResultsLimit == 0 {
		widget.ResultsLimit = 8
	}
	if widget.ResultsLimit < 1 || widget.ResultsLimit > 20 {
		return fmt.Errorf("results-limit must be between 1 and 20")
	}
	if widget.FocusKey == "" {
		widget.FocusKey = "s"
	}
	if widget.FocusKey != "none" && (len(widget.FocusKey) != 1 || !strings.Contains("abcdefghijklmnopqrstuvwxyz", widget.FocusKey)) {
		return fmt.Errorf("focus-key must be a lowercase letter or none")
	}
	if widget.LocalSearch.Scope == "" {
		widget.LocalSearch.Scope = "page"
	}
	if widget.LocalSearch.Scope != "page" && widget.LocalSearch.Scope != "dashboard" && widget.LocalSearch.Scope != "all" {
		return fmt.Errorf("local-search scope must be page, dashboard, or all")
	}
	for _, source := range widget.LocalSearch.Sources {
		if source != "pages" && source != "bookmarks" && source != "monitor" && source != "docker-containers" {
			return fmt.Errorf("unsupported local-search source %q", source)
		}
	}
	aliases := make(map[string]bool)
	for _, shortcut := range widget.Shortcuts {
		if strings.TrimSpace(shortcut.Title) == "" || !safeSearchDestination(shortcut.URL) {
			return fmt.Errorf("search shortcuts require a title and an HTTP(S) or local URL")
		}
		alias := strings.ToLower(strings.TrimSpace(shortcut.Alias))
		if alias != "" {
			if strings.ContainsAny(alias, " \t\r\n!") || aliases[alias] {
				return fmt.Errorf("search shortcut aliases must be unique single words without !")
			}
			aliases[alias] = true
		}
	}
	if widget.Suggestions {
		widget.suggestionSlots = make(chan struct{}, 4)
		if widget.SuggestionEngine == "" {
			widget.SuggestionEngine = widget.SearchEngine
			if widget.SuggestionEngine == "" {
				widget.SuggestionEngine = "duckduckgo"
			}
			if _, supported := searchSuggestionEngines[widget.SuggestionEngine]; !supported {
				return fmt.Errorf("set suggestion-engine explicitly for this search engine")
			}
		}
		if endpoint, ok := searchSuggestionEngines[widget.SuggestionEngine]; ok {
			widget.SuggestionEngine = endpoint
		}
		endpoint, err := parseSearchSuggestionEndpoint(widget.SuggestionEngine)
		if err != nil {
			return err
		}
		widget.suggestionEndpoint = endpoint
	}
	if widget.History.Storage == "" {
		widget.History.Storage = "off"
	}
	if widget.History.Storage != "off" && widget.History.Storage != "browser" && widget.History.Storage != "server" {
		return fmt.Errorf("history storage must be off, browser, or server")
	}
	if widget.History.Limit == 0 {
		widget.History.Limit = 50
	}
	if widget.History.Limit < 1 || widget.History.Limit > 100 {
		return fmt.Errorf("history limit must be between 1 and 100")
	}
	if widget.History.RetentionDays == 0 {
		widget.History.RetentionDays = 30
	}
	if widget.History.RetentionDays < 1 || widget.History.RetentionDays > 3650 {
		return fmt.Errorf("history retention-days must be between 1 and 3650")
	}
	if widget.History.Storage != "off" {
		if err := validatePersonalStateKey("search", widget.History.ID); err != nil {
			return fmt.Errorf("history requires a stable id of 1–128 characters without slashes")
		}
	}
	return nil
}

// Search results never activate executable or protocol-relative URLs. Existing
// search-engine/bang templates retain their established configuration behavior.
func safeSearchDestination(raw string) bool {
	if raw == "" || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\\\r\n\t") {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil {
		return false
	}
	if parsed.Scheme == "" {
		return strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//")
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func (widget *searchWidget) Render() template.HTML {
	return widget.renderCachedTemplate()
}

func (widget *searchWidget) setDefaultNewTab(value bool) {
	if widget.widgetBase.configuredFields["new-tab"] {
		return
	}

	widget.NewTab = value
}

func (widget *searchWidget) setDefaultTimeout(value durationField) {
	widget.Timeout = value
}
