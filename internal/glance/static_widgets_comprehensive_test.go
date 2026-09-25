package glance

import (
	"context"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samcro1967/glance/pkg/sysinfo"
)

func TestComprehensiveNewWidgetAllKnownTypes(t *testing.T) {
	types := []string{"calendar", "calendar-legacy", "ics-events", "clock", "analog-clock", "weather", "bookmarks", "iframe", "markdown", "html", "hacker-news", "releases", "videos", "markets", "stocks", "reddit", "rss", "monitor", "prometheus", "twitch-top-games", "twitch-channels", "lobsters", "change-detection", "repository", "search", "extension", "group", "dns-stats", "split-column", "custom-api", "docker-containers", "server-stats", "timer", "to-do", "unit-converter", "calculator", "stack", "status-bar"}
	seen := map[uint64]bool{}
	for _, typ := range types {
		t.Run(typ, func(t *testing.T) {
			w, err := newWidget(typ)
			if err != nil {
				t.Fatal(err)
			}
			if w.GetID() == 0 {
				t.Fatal("expected nonzero ID")
			}
			if seen[w.GetID()] {
				t.Fatal("duplicate widget ID")
			}
			seen[w.GetID()] = true
		})
	}
	if _, err := newWidget(""); err == nil {
		t.Fatal("expected empty type error")
	}
	if _, err := newWidget("does-not-exist"); err == nil {
		t.Fatal("expected unknown type error")
	}
}

func TestComprehensiveCalendarWidget(t *testing.T) {
	w := &calendarWidget{}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if w.Title != "Calendar" || w.FirstDayOfWeek != "monday" || w.FirstDay != int(time.Monday) {
		t.Fatalf("defaults: %#v", w)
	}
	if len(w.Render()) == 0 {
		t.Fatal("expected rendered calendar")
	}
	sat := &calendarWidget{FirstDayOfWeek: "saturday"}
	if err := sat.initialize(); err != nil {
		t.Fatal(err)
	}
	if sat.FirstDay != int(time.Saturday) {
		t.Fatal("saturday not mapped")
	}
	bad := &calendarWidget{FirstDayOfWeek: "Funday"}
	if err := bad.initialize(); err == nil {
		t.Fatal("expected invalid weekday error")
	}
}

func TestComprehensiveClockWidget(t *testing.T) {
	w := &clockWidget{}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if w.HourFormat != "24h" || w.Title != "Clock" || len(w.Render()) == 0 {
		t.Fatal("clock defaults/render failed")
	}
	w12 := &clockWidget{HourFormat: "12h"}
	w12.Timezones = append(w12.Timezones, struct {
		Timezone string `yaml:"timezone"`
		Label    string `yaml:"label"`
	}{Timezone: "UTC", Label: "UTC"})
	if err := w12.initialize(); err != nil {
		t.Fatal(err)
	}
	if err := (&clockWidget{HourFormat: "13h"}).initialize(); err == nil {
		t.Fatal("expected invalid format")
	}
	missing := &clockWidget{}
	missing.Timezones = append(missing.Timezones, struct {
		Timezone string `yaml:"timezone"`
		Label    string `yaml:"label"`
	}{Label: "bad"})
	if err := missing.initialize(); err == nil {
		t.Fatal("expected missing timezone")
	}
	invalid := &clockWidget{}
	invalid.Timezones = append(invalid.Timezones, struct {
		Timezone string `yaml:"timezone"`
		Label    string `yaml:"label"`
	}{Timezone: "Invalid/Nowhere"})
	if err := invalid.initialize(); err == nil {
		t.Fatal("expected invalid timezone")
	}
}

func TestComprehensiveAnalogClockWidget(t *testing.T) {
	w := &analogClockWidget{}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}

	if w.DialMarkers != "NumericalFull" || w.Title != "Clock" {
		t.Fatal("analog clock defaults failed")
	}

	rendered := string(w.Render())
	if !strings.Contains(rendered, `analog-clock-marker-1">`) ||
		!strings.Contains(rendered, `analog-clock-marker-12">`) ||
		!strings.Contains(rendered, "data-am-pm") ||
		!strings.Contains(rendered, "data-date") {
		t.Fatal("analog clock default markup missing expected elements")
	}

	minimal := &analogClockWidget{
		DialMarkers:       "NumericalMinimal",
		HideAmPmIndicator: true,
		HideDate:          true,
		Timezones: []analogClockTimezone{
			{Timezone: "UTC", Label: "Universal"},
		},
	}
	if err := minimal.initialize(); err != nil {
		t.Fatal(err)
	}

	minimalRendered := string(minimal.Render())
	if strings.Contains(minimalRendered, `analog-clock-marker-1">`) ||
		!strings.Contains(minimalRendered, `analog-clock-marker-3">`) ||
		!strings.Contains(minimalRendered, `analog-clock-marker-6">`) ||
		!strings.Contains(minimalRendered, `analog-clock-marker-9">`) ||
		!strings.Contains(minimalRendered, `analog-clock-marker-12">`) ||
		strings.Contains(minimalRendered, "data-am-pm") ||
		strings.Contains(minimalRendered, "data-date") ||
		!strings.Contains(minimalRendered, `data-time-in-zone="UTC"`) ||
		!strings.Contains(minimalRendered, "Universal") {
		t.Fatal("analog clock minimal/hidden/timezone markup incorrect")
	}

	none := &analogClockWidget{DialMarkers: "None"}
	if err := none.initialize(); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(none.Render()), "analog-clock-markers") {
		t.Fatal("analog clock with no dial markers rendered marker markup")
	}

	fallback := &analogClockWidget{
		Timezones: []analogClockTimezone{
			{Timezone: "UTC"},
		},
	}
	if err := fallback.initialize(); err != nil {
		t.Fatal(err)
	}

	fallbackRendered := string(fallback.Render())
	if !strings.Contains(fallbackRendered, `data-time-in-zone="UTC"`) ||
		!strings.Contains(fallbackRendered, "UTC") {
		t.Fatal("analog clock timezone fallback label missing")
	}

	if err := (&analogClockWidget{DialMarkers: "Invalid"}).initialize(); err == nil {
		t.Fatal("expected invalid dial markers error")
	}

	missing := &analogClockWidget{
		Timezones: []analogClockTimezone{
			{Label: "bad"},
		},
	}
	if err := missing.initialize(); err == nil {
		t.Fatal("expected missing timezone")
	}

	invalid := &analogClockWidget{
		Timezones: []analogClockTimezone{
			{Timezone: "Invalid/Nowhere"},
		},
	}
	if err := invalid.initialize(); err == nil {
		t.Fatal("expected invalid timezone")
	}
}

func TestComprehensiveHTMLIframeTimerTodoWidgets(t *testing.T) {
	htmlw := &htmlWidget{Source: template.HTML("<strong>safe</strong>")}
	if err := htmlw.initialize(); err != nil {
		t.Fatal(err)
	}
	if htmlw.Render() != htmlw.Source || htmlw.Title != "" {
		t.Fatal("html widget mismatch")
	}
	if err := (&iframeWidget{}).initialize(); err == nil {
		t.Fatal("expected source required")
	}
	low := &iframeWidget{Source: "https://example.invalid", Height: 10}
	if err := low.initialize(); err != nil {
		t.Fatal(err)
	}
	if low.Height != 50 || len(low.Render()) == 0 {
		t.Fatal("iframe minimum height/render failed")
	}
	legacy := &iframeWidget{Source: "https://example.invalid", Height: 50}
	if err := legacy.initialize(); err != nil {
		t.Fatal(err)
	}
	if legacy.Height != 300 {
		t.Fatalf("legacy height=%d", legacy.Height)
	}
	normal := &iframeWidget{Source: "https://example.invalid", Height: 400}
	if err := normal.initialize(); err != nil {
		t.Fatal(err)
	}
	if normal.Height != 400 {
		t.Fatal("configured height changed")
	}
	timer := &timerWidget{TimerID: "home"}
	if err := timer.initialize(); err != nil {
		t.Fatal(err)
	}
	if timer.Title != "Timers" || timer.HourFormat != "12h" || len(timer.Render()) == 0 {
		t.Fatal("timer initialization/render failed")
	}
	if !strings.Contains(string(timer.Render()), `data-timer-id="home"`) ||
		!strings.Contains(string(timer.Render()), `data-hour-format="12h"`) {
		t.Fatal("timer render missing client configuration")
	}

	timer24 := &timerWidget{TimerID: "work", HourFormat: "24h"}
	if err := timer24.initialize(); err != nil {
		t.Fatal(err)
	}
	if timer24.HourFormat != "24h" ||
		!strings.Contains(string(timer24.Render()), `data-timer-id="work"`) ||
		!strings.Contains(string(timer24.Render()), `data-hour-format="24h"`) {
		t.Fatal("configured timer initialization/render failed")
	}

	invalidTimer := &timerWidget{HourFormat: "13h"}
	if err := invalidTimer.initialize(); err == nil {
		t.Fatal("expected invalid timer hour-format error")
	}

	todo := &todoWidget{TodoID: "home"}
	if err := todo.initialize(); err != nil {
		t.Fatal(err)
	}
	if todo.Title != "To-do" || len(todo.Render()) == 0 {
		t.Fatal("todo initialization/render failed")
	}
}

func TestComprehensiveSearchWidget(t *testing.T) {
	w := &searchWidget{}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if w.Title != "Search" || !strings.Contains(w.SearchEngine, "duckduckgo") || !strings.Contains(w.SearchEngine, "!QUERY!") || w.Placeholder == "" || len(w.Render()) == 0 {
		t.Fatalf("search defaults: %#v", w)
	}
	custom := &searchWidget{SearchEngine: "https://example.invalid/?q={QUERY}"}
	custom.Bangs = []SearchBang{{Title: "Docs", Shortcut: "!d", URL: "https://docs.example.invalid/{QUERY}"}}
	if err := custom.initialize(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(custom.SearchEngine, "{QUERY}") || strings.Contains(custom.Bangs[0].URL, "{QUERY}") {
		t.Fatal("query placeholder not converted")
	}
	noShortcut := &searchWidget{}
	noShortcut.Bangs = []SearchBang{{URL: "https://example.invalid/{QUERY}"}}
	if err := noShortcut.initialize(); err == nil {
		t.Fatal("expected shortcut error")
	}
	noURL := &searchWidget{}
	noURL.Bangs = []SearchBang{{Shortcut: "!x"}}
	if err := noURL.initialize(); err == nil {
		t.Fatal("expected URL error")
	}
	if got := convertSearchUrl("a{QUERY}b{QUERY}"); got != "a!QUERY!b!QUERY!" {
		t.Fatalf("convert=%q", got)
	}
}

func TestComprehensiveLegacyCalendar(t *testing.T) {
	if daysInMonth(time.February, 2024) != 29 || daysInMonth(time.February, 2023) != 28 || daysInMonth(time.April, 2026) != 30 {
		t.Fatal("daysInMonth incorrect")
	}
	for _, startSunday := range []bool{false, true} {
		c := newCalendar(time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC), startSunday)
		if c.CurrentDay != 1 || c.CurrentMonthName != "January" || c.CurrentYear != 2026 || len(c.Days) != 21 {
			t.Fatalf("calendar=%#v", c)
		}
	}
	w := &oldCalendarWidget{StartSunday: true}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	w.update(context.Background())
	if w.Calendar == nil || len(w.Render()) == 0 {
		t.Fatal("legacy calendar update/render failed")
	}
}

func TestComprehensiveBookmarksInheritance(t *testing.T) {
	w := &bookmarksWidget{}
	w.Groups = append(w.Groups, bookmarkGroup{
		Title:     "Default",
		SameTab:   true,
		HideArrow: true,
	})
	g := &w.Groups[0]
	g.Links = append(g.Links, bookmarkLink{
		Title: "One",
		URL:   "https://example.invalid",
	})
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if !g.Links[0].SameTab || !g.Links[0].HideArrow || g.Links[0].Target != "" || len(w.Render()) == 0 {
		t.Fatalf("inheritance failed: %#v", g.Links[0])
	}
}

func TestComprehensiveBookmarksStyles(t *testing.T) {
	for _, tc := range []struct {
		name      string
		style     string
		wantClass string
		notClass  string
	}{
		{name: "default", wantClass: "dynamic-columns", notClass: "bookmarks-grid-card"},
		{name: "grid-cards", style: "grid-cards", wantClass: "bookmarks-grid-card", notClass: "dynamic-columns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &bookmarksWidget{
				Style: tc.style,
				Groups: []bookmarkGroup{{
					Title: "Services",
					Links: []bookmarkLink{{
						Title:       "Example",
						URL:         "https://example.invalid",
						Description: "Example service",
					}},
				}},
			}

			if err := w.initialize(); err != nil {
				t.Fatal(err)
			}

			html := string(w.Render())
			if !strings.Contains(html, tc.wantClass) {
				t.Fatalf("rendered HTML missing %q: %s", tc.wantClass, html)
			}
			if strings.Contains(html, tc.notClass) {
				t.Fatalf("rendered HTML unexpectedly contains %q: %s", tc.notClass, html)
			}
			if !strings.Contains(html, `target="_blank"`) {
				t.Fatalf("resolved link target missing from rendered HTML: %s", html)
			}
		})
	}
}

func TestComprehensiveContainerEmptyLifecycle(t *testing.T) {
	now := time.Now()
	base := &containerWidgetBase{}
	if err := base._initializeWidgets(); err != nil {
		t.Fatal(err)
	}
	base._update(context.Background())
	base._setProviders(&widgetProviders{})
	if base._requiresUpdate(&now) {
		t.Fatal("empty container should not require update")
	}
	if len(base.childWidgets()) != 0 {
		t.Fatal("expected no children")
	}
	split := &splitColumnWidget{}
	if err := split.initialize(); err != nil {
		t.Fatal(err)
	}
	if split.MaxColumns != 2 || !split.HideHeader || len(split.Render()) == 0 {
		t.Fatal("split defaults/render failed")
	}
	split.update(context.Background())
	split.setProviders(&widgetProviders{})
	if split.requiresUpdate(&now) {
		t.Fatal("empty split should not require update")
	}
	group := &groupWidget{}
	if err := group.initialize(); err != nil {
		t.Fatal(err)
	}
	if !group.HideHeader || len(group.Render()) == 0 {
		t.Fatal("group defaults/render failed")
	}
	group.update(context.Background())
	group.setProviders(&widgetProviders{})
	if group.requiresUpdate(&now) {
		t.Fatal("empty group should not require update")
	}
	stack := &stackWidget{}
	if err := stack.initialize(); err != nil {
		t.Fatal(err)
	}
	if !stack.HideHeader || len(stack.Render()) == 0 {
		t.Fatal("stack defaults/render failed")
	}
	stack.update(context.Background())
	stack.setProviders(&widgetProviders{})
	if stack.requiresUpdate(&now) {
		t.Fatal("empty stack should not require update")
	}
}

func TestComprehensiveGroupOptionalHeader(t *testing.T) {
	t.Run("titled standalone group renders shared header", func(t *testing.T) {
		group := &groupWidget{
			widgetBase: widgetBase{
				Type:  "group",
				Title: "Media",
			},
		}

		if err := group.initialize(); err != nil {
			t.Fatal(err)
		}
		if group.HideHeader {
			t.Fatal("titled standalone group should show its header")
		}
		if rendered := string(group.Render()); !strings.Contains(rendered, ">Media</h2>") {
			t.Fatalf("titled standalone group missing header: %s", rendered)
		}
	})

	t.Run("untitled standalone group preserves headerless presentation", func(t *testing.T) {
		group := &groupWidget{widgetBase: widgetBase{Type: "group"}}

		if err := group.initialize(); err != nil {
			t.Fatal(err)
		}
		if !group.HideHeader {
			t.Fatal("untitled standalone group should remain headerless")
		}
		if rendered := string(group.Render()); strings.Contains(rendered, "<div class=\"widget-header\">") {
			t.Fatalf("untitled standalone group unexpectedly rendered a header: %s", rendered)
		}
	})

	t.Run("parent suppression keeps nested group title tab-only", func(t *testing.T) {
		nested := &groupWidget{
			widgetBase: widgetBase{
				Type:  "group",
				Title: "Recipes",
			},
		}
		nested.setHideHeader(true)

		if err := nested.initialize(); err != nil {
			t.Fatal(err)
		}
		if !nested.HideHeader {
			t.Fatal("nested group should preserve parent header suppression")
		}
		if rendered := string(nested.Render()); strings.Contains(rendered, ">Recipes</h2>") {
			t.Fatalf("nested group rendered its title as a duplicate header: %s", rendered)
		}
	})
}

func TestComprehensiveGroupTitleIcons(t *testing.T) {
	child := &markdownWidget{Source: "Test", widgetBase: widgetBase{Type: "markdown", Title: "News", TitleURL: "https://example.com", Icon: newCustomIconField("auto-invert https://example.com/news.svg")}}
	group := &groupWidget{widgetBase: widgetBase{Type: "group"}, containerWidgetBase: containerWidgetBase{Widgets: widgets{child}}}
	if err := group.initialize(); err != nil {
		t.Fatal(err)
	}
	rendered := string(group.Render())
	for _, expected := range []string{`class="widget-group-title widget-title glance-tab`, `data-title-url="https://example.com"`, `class="widget-title-icon flat-icon"`, `src="https://example.com/news.svg"`, `alt=""`, `loading="lazy"`, `News`} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("rendered group missing %q: %s", expected, rendered)
		}
	}
	if !child.HideHeader {
		t.Fatal("group child header should remain hidden")
	}
}

func TestComprehensiveContainerRejectsUnsupportedNesting(t *testing.T) {
	split := &splitColumnWidget{}
	split.Type = "split-column"

	group := &groupWidget{
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{split},
		},
	}
	if err := group.initialize(); err == nil {
		t.Fatal("group should reject split-column")
	}

	for _, typ := range []string{"stack", "group", "split-column"} {
		child, err := newWidget(typ)
		if err != nil {
			t.Fatalf("newWidget(%q): %v", typ, err)
		}

		switch typed := child.(type) {
		case *stackWidget:
			typed.Type = typ
		case *groupWidget:
			typed.Type = typ
		case *splitColumnWidget:
			typed.Type = typ
		default:
			t.Fatalf("unexpected child type %T", child)
		}

		stack := &stackWidget{
			containerWidgetBase: containerWidgetBase{
				Widgets: widgets{child},
			},
		}
		if err := stack.initialize(); err == nil {
			t.Fatalf("stack should reject %s", typ)
		}
	}
}

func TestStatusBarTickerBrowserContract(t *testing.T) {
	pageJS, err := os.ReadFile(filepath.Join("static", "js", "page.js"))
	if err != nil {
		t.Fatalf("read page.js: %v", err)
	}

	source := string(pageJS)

	required := []string{
		`const STATUS_BAR_TICKER_PIXELS_PER_SECOND = {`,
		`slow: 30`,
		`normal: 45`,
		`fast: 70`,
		`function setupStatusBarTickers(root = document)`,
		`statusBar.dataset.tickerSpeed`,
		`items.getBoundingClientRect().width`,
		`statusBar.getBoundingClientRect().width`,
		`contentWidth < containerWidth`,
		`containerWidth + contentWidth`,
		`--status-bar-ticker-content-width`,
		`--status-bar-ticker-container-width`,
		`distance / pixelsPerSecond`,
		`--status-bar-ticker-duration`,
		`statusBar.dataset.tickerShort`,
		`statusBar.dataset.tickerReady = "true"`,
		`new ResizeObserver(updateTickerGeometry)`,
		`resizeObserver.observe(items)`,
		`resizeObserver.observe(statusBar)`,
		`const handlePointerUp = (event) => {`,
		`statusBar.addEventListener("pointerup", handlePointerUp)`,
		`cleanupCallbacks.push(() => resizeObserver.disconnect())`,
		`statusBar.removeEventListener("pointerup", handlePointerUp)`,
		`return cleanupCallbacks`,
		`...runStage("status_bar_tickers", () => setupStatusBarTickers(root))`,
		`async function initializeContentRoot(root, diagnostics = false)`,
		`const cleanupCallbacks = await initializeContentRoot(widgetElement)`,
		`event.pointerType !== "mouse"`,
		`event.target.closest("a")`,
		`link.blur()`,
		`"status_bar_tickers"`,
		`await initializeContentRoot(document, true)`,
	}

	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			t.Fatalf("page.js missing Status Bar ticker contract fragment %q", fragment)
		}
	}
}

func TestCarouselBrowserCleanupContract(t *testing.T) {
	pageJS, err := os.ReadFile(filepath.Join("static", "js", "page.js"))
	if err != nil {
		t.Fatalf("read page.js: %v", err)
	}

	source := string(pageJS)

	required := []string{
		`itemsContainer.addEventListener("scroll", determineSideCutoffsRateLimited)`,
		`window.addEventListener("resize", determineSideCutoffsRateLimited)`,
		`itemsContainer.removeEventListener("scroll", determineSideCutoffsRateLimited)`,
		`window.removeEventListener("resize", determineSideCutoffsRateLimited)`,
	}

	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			t.Fatalf("page.js missing carousel cleanup contract fragment %q", fragment)
		}
	}
}

func TestPopoverViewportContainmentContract(t *testing.T) {
	popoverJS, err := os.ReadFile(filepath.Join("static", "js", "popover.js"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(popoverJS)
	for _, fragment := range []string{
		"const viewportTop = window.scrollY",
		"const maximumTop = Math.max(viewportTop, viewportBottom - containerBounds.height)",
		"Math.min(Math.max(top, viewportTop), maximumTop)",
		`window.addEventListener("scroll", queueRepositionContainer)`,
		`window.addEventListener("resize", queueRepositionContainer)`,
		`observer.observe(containerElement)`,
		`window.removeEventListener("scroll", queueRepositionContainer)`,
		`window.removeEventListener("resize", queueRepositionContainer)`,
		`observer.unobserve(containerElement)`,
	} {
		if !strings.Contains(source, fragment) {
			t.Fatalf("popover.js missing viewport containment fragment %q", fragment)
		}
	}

	popoverCSS, err := os.ReadFile(filepath.Join("static", "css", "popover.css"))
	if err != nil {
		t.Fatal(err)
	}
	cssSource := string(popoverCSS)
	for _, fragment := range []string{
		"max-height: calc(100vh - var(--content-bounds-padding) - var(--content-bounds-padding) - var(--triangle-margin))",
		"overflow-y: auto",
	} {
		if !strings.Contains(cssSource, fragment) {
			t.Fatalf("popover.css missing oversized-content containment fragment %q", fragment)
		}
	}
}

func TestPopoverBrowserInitializationContract(t *testing.T) {
	popoverJS, err := os.ReadFile(filepath.Join("static", "js", "popover.js"))
	if err != nil {
		t.Fatalf("read popover.js: %v", err)
	}

	source := string(popoverJS)

	required := []string{
		`if (target.dataset.popoverInitialized === "true")`,
		`target.dataset.popoverInitialized = "true"`,
		`target.addEventListener("click", handleMouseEnter)`,
		`target.addEventListener("keydown", handleKeyboardActivation)`,
		`target.addEventListener("mouseenter", handleMouseEnter)`,
		`target.addEventListener("mouseleave", handleMouseLeave)`,
	}

	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			t.Fatalf("popover.js missing initialization contract fragment %q", fragment)
		}
	}
}

func TestSearchOpenDomainsConfigurationAndRendering(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: search
          - type: search
            open-domains: true
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	disabled := config.Pages[0].Columns[0].Widgets[0].(*searchWidget)
	enabled := config.Pages[0].Columns[0].Widgets[1].(*searchWidget)

	if disabled.OpenDomains {
		t.Fatal("open-domains default = true, want false")
	}

	if !enabled.OpenDomains {
		t.Fatal("explicit open-domains=true was not preserved")
	}

	disabledHTML := string(disabled.Render())
	enabledHTML := string(enabled.Render())

	if !strings.Contains(disabledHTML, `data-open-domains="false"`) {
		t.Fatalf("disabled search render missing open-domains=false: %s", disabledHTML)
	}

	if !strings.Contains(enabledHTML, `data-open-domains="true"`) {
		t.Fatalf("enabled search render missing open-domains=true: %s", enabledHTML)
	}
}

func TestSearchOpenDomainsBrowserContract(t *testing.T) {
	searchJS, err := os.ReadFile(filepath.Join("static", "js", "search.js"))
	if err != nil {
		t.Fatalf("read search.js: %v", err)
	}

	source := string(searchJS)

	required := []string{
		`const SEARCH_DOMAIN_PATTERN =`,
		`const openDomains = widget.dataset.openDomains === "true";`,
		`openDomains && currentBang == null && SEARCH_DOMAIN_PATTERN.test(query)`,
		`query.includes("://") ? query : "https://" + query`,
		`searchUrlTemplate.replace("!QUERY!", encodeURIComponent(query))`,
		`const openedWindow = window.open(url, target);`,
		`if (openedWindow != null)`,
		`openedWindow.focus();`,
	}

	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			t.Fatalf("search.js missing Search open-domains contract fragment %q", fragment)
		}
	}

	if strings.Contains(source, `window.open(url, target).focus()`) {
		t.Fatal("Search popup handling must not call focus directly on window.open result")
	}

	pageJS, err := os.ReadFile(filepath.Join("static", "js", "page.js"))
	if err != nil {
		t.Fatalf("read page.js: %v", err)
	}

	pageSource := string(pageJS)
	for _, fragment := range []string{
		`import { setupSearchBoxes } from './search.js';`,
		`runFrontendDiagnosticStage("search_boxes", () => setupSearchBoxes());`,
	} {
		if !strings.Contains(pageSource, fragment) {
			t.Fatalf("page.js missing Search composition contract fragment %q", fragment)
		}
	}
}

func TestServerStatsTemplateRendersAllMountpointProgressValues(t *testing.T) {
	for _, count := range []int{1, 2, 3, 6} {
		t.Run(fmt.Sprintf("%d mountpoints", count), func(t *testing.T) {
			info := &sysinfo.SystemInfo{Hostname: "test-server"}
			for i := 0; i < count; i++ {
				info.Mountpoints = append(info.Mountpoints, sysinfo.MountpointInfo{
					Path:        fmt.Sprintf("/disk-%d", i+1),
					Name:        fmt.Sprintf("Disk %d", i+1),
					TotalMB:     1000,
					UsedMB:      uint64((90 - i*7) * 10),
					UsedPercent: uint8(90 - i*7),
				})
			}

			widget := &serverStatsWidget{
				Servers: []serverStatsRequest{{
					Info:        info,
					IsReachable: true,
				}},
			}
			widget.Type = "server-stats"
			widget.ContentAvailable = true

			rendered := string(widget.Render())

			if !strings.Contains(rendered, `DISK</div>`) ||
				!strings.Contains(rendered, `90 <span class="color-base">%</span>`) {
				t.Fatalf("server stats disk headline did not preserve first mountpoint: %s", rendered)
			}

			for i := 0; i < count; i++ {
				percent := 90 - i*7
				expected := fmt.Sprintf(`style="--percent: %d"`, percent)
				if !strings.Contains(rendered, expected) {
					t.Fatalf("server stats render with %d mountpoints missing disk progress %q", count, expected)
				}
			}

			if !strings.Contains(rendered, `progress-value-notice" style="--percent: 90"`) {
				t.Fatalf("server stats render with %d mountpoints lost disk notice styling", count)
			}
		})
	}
}
