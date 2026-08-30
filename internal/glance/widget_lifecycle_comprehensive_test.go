package glance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestComprehensiveWeatherWidgetInitializationAndHelpers(t *testing.T) {
	if err := (&weatherWidget{}).initialize(); err == nil {
		t.Fatal("expected location required")
	}
	metric := &weatherWidget{Location: "St. Louis, MO, US"}
	if err := metric.initialize(); err != nil {
		t.Fatal(err)
	}
	if metric.Title != "Weather" || metric.Units != "metric" || metric.TimeLabels != timeLabels12h {
		t.Fatal("weather defaults failed")
	}
	imperial := &weatherWidget{Location: "London, UK", Units: "imperial", HourFormat: "24h"}
	if err := imperial.initialize(); err != nil {
		t.Fatal(err)
	}
	if imperial.TimeLabels != timeLabels24h {
		t.Fatal("24h labels not applied")
	}
	if err := (&weatherWidget{Location: "x", Units: "kelvin"}).initialize(); err == nil {
		t.Fatal("expected invalid units")
	}
	if err := (&weatherWidget{Location: "x", HourFormat: "13h"}).initialize(); err == nil {
		t.Fatal("expected invalid hour format")
	}
	if expandCountryAbbreviations(" US ") != "United States" || expandCountryAbbreviations("CA") != "CA" {
		t.Fatal("country expansion mismatch")
	}
	loc, area := parsePlaceName("Columbus, Ohio, US")
	if loc != "Columbus, United States" || area != "Ohio" {
		t.Fatalf("loc=%q area=%q", loc, area)
	}
	loc, area = parsePlaceName("London, UK")
	if loc != "London, United Kingdom" || area != "" {
		t.Fatalf("loc=%q area=%q", loc, area)
	}
	loc, area = parsePlaceName("Paris")
	if loc != "Paris" || area != "" {
		t.Fatal("single place mismatch")
	}
	if (&weather{WeatherCode: 0}).WeatherCodeAsString() == "" {
		t.Fatal("known weather code should have label")
	}
	if (&weather{WeatherCode: 999}).WeatherCodeAsString() != "" {
		t.Fatal("unknown weather code should be empty")
	}
}

func TestComprehensiveMarketsWidgetInitializationAndSorting(t *testing.T) {
	legacy := &marketsWidget{StocksRequests: []marketRequest{{Symbol: "ABC"}}, ChartLinkTemplate: "https://chart.invalid/{SYMBOL}", SymbolLinkTemplate: "https://symbol.invalid/{SYMBOL}"}
	if err := legacy.initialize(); err != nil {
		t.Fatal(err)
	}
	if len(legacy.MarketRequests) != 1 || legacy.MarketRequests[0].ChartLink != "https://chart.invalid/ABC" || legacy.MarketRequests[0].SymbolLink != "https://symbol.invalid/ABC" || legacy.Title != "Markets" {
		t.Fatalf("market defaults=%#v", legacy)
	}
	preserved := &marketsWidget{MarketRequests: []marketRequest{{Symbol: "XYZ", ChartLink: "custom-chart", SymbolLink: "custom-symbol"}}, ChartLinkTemplate: "ignored/{SYMBOL}", SymbolLinkTemplate: "ignored/{SYMBOL}"}
	if err := preserved.initialize(); err != nil {
		t.Fatal(err)
	}
	if preserved.MarketRequests[0].ChartLink != "custom-chart" || preserved.MarketRequests[0].SymbolLink != "custom-symbol" {
		t.Fatal("explicit links overwritten")
	}
	list := marketList{{Name: "a", PercentChange: -5}, {Name: "b", PercentChange: 10}, {Name: "c", PercentChange: 2}}
	list.sortByAbsChange()
	if list[0].Name != "b" || list[1].Name != "a" {
		t.Fatalf("abs sort=%v", list)
	}
	list.sortByChange()
	if list[0].Name != "b" || list[2].Name != "a" {
		t.Fatalf("change sort=%v", list)
	}
}

func TestComprehensiveHackerNewsWidgetInitialization(t *testing.T) {
	w := &hackerNewsWidget{Limit: -1, CollapseAfter: -2, SortBy: "invalid"}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if w.Title != "Hacker News" || w.TitleURL != "https://news.ycombinator.com/" || w.Limit != 15 || w.CollapseAfter != 5 || w.SortBy != "top" {
		t.Fatalf("defaults=%#v", w)
	}
	valid := &hackerNewsWidget{Limit: 7, CollapseAfter: -1, SortBy: "best"}
	if err := valid.initialize(); err != nil {
		t.Fatal(err)
	}
	if valid.Limit != 7 || valid.CollapseAfter != -1 || valid.SortBy != "best" {
		t.Fatal("valid settings changed")
	}
}

func TestComprehensiveLobstersWidgetInitialization(t *testing.T) {
	w := &lobstersWidget{Limit: 0, CollapseAfter: -2, SortBy: "bad"}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if w.Title != "Lobsters" || w.TitleURL != "https://lobste.rs" || w.SortBy != "hot" || w.Limit != 15 || w.CollapseAfter != 5 {
		t.Fatalf("defaults=%#v", w)
	}
	custom := &lobstersWidget{InstanceURL: "https://lobsters.example.invalid", SortBy: "new", Limit: 3, CollapseAfter: -1}
	if err := custom.initialize(); err != nil {
		t.Fatal(err)
	}
	if custom.TitleURL != custom.InstanceURL || custom.SortBy != "new" || custom.Limit != 3 || custom.CollapseAfter != -1 {
		t.Fatal("custom settings changed")
	}
}

func TestComprehensiveThemePropertiesAndHandler(t *testing.T) {
	base := &themeProperties{BackgroundColor: &hslColorField{H: 0, S: 0, L: 0}, PrimaryColor: &hslColorField{H: 1, S: 2, L: 3}, PositiveColor: &hslColorField{H: 4, S: 5, L: 6}, NegativeColor: &hslColorField{H: 7, S: 8, L: 9}, ContrastMultiplier: 1, TextSaturationMultiplier: 1}
	same := *base
	if !base.SameAs(&same) {
		t.Fatal("identical themes should match")
	}
	same.Light = true
	if base.SameAs(&same) {
		t.Fatal("different themes should not match")
	}
	if err := base.init(); err != nil {
		t.Fatal(err)
	}
	if base.CSS == "" || base.PreviewHTML == "" || base.BackgroundColorAsHex != "#000000" {
		t.Fatal("theme init outputs missing")
	}
	fallback := &themeProperties{}
	if err := fallback.init(); err != nil {
		t.Fatal(err)
	}
	if fallback.BackgroundColorAsHex != "#151519" {
		t.Fatalf("fallback=%q", fallback.BackgroundColorAsHex)
	}
	var nil1, nil2 *themeProperties
	if !nil1.SameAs(nil2) || nil1.SameAs(base) {
		t.Fatal("nil theme comparison failed")
	}
}

func TestComprehensiveWidgetBaseConfiguration(t *testing.T) {
	w := &widgetBase{Title: "Configured", TitleURL: "configured", CustomCacheDuration: durationField(5 * time.Minute)}
	if w.withTitle("Default").Title != "Configured" || w.withTitleURL("default").TitleURL != "configured" {
		t.Fatal("configured title fields overwritten")
	}
	w.withCacheDuration(time.Hour)
	if w.cacheType != cacheTypeDuration || w.cacheDuration != 5*time.Minute {
		t.Fatalf("custom cache=%v", w.cacheDuration)
	}
	w.CustomCacheDuration = 0
	w.withCacheDuration(time.Hour)
	if w.cacheDuration != time.Hour {
		t.Fatal("default cache not used")
	}
	w.withCacheOnTheHour()
	if w.cacheType != cacheTypeOnTheHour {
		t.Fatal("on-hour cache not set")
	}
	w.withNotice(http.ErrAbortHandler)
	if w.Notice != http.ErrAbortHandler {
		t.Fatal("notice not set")
	}
	w.ContentAvailable = false
	w.withError(nil)
	if !w.ContentAvailable || w.Error != nil {
		t.Fatal("successful error reset should mark content available")
	}
	w.setID(99)
	w.setHideHeader(true)
	w.setProviders(&widgetProviders{})
	if w.GetID() != 99 || !w.HideHeader || w.Providers == nil {
		t.Fatal("widget base setters failed")
	}
	w.WIP = true
	if !w.IsWIP() {
		t.Fatal("WIP getter failed")
	}
}

func TestComprehensiveThemeChangeUnknownPreset(t *testing.T) {
	a := &application{Config: config{}}
	req := httptest.NewRequest(http.MethodGet, "/theme/missing", nil)
	req.SetPathValue("key", "missing")
	rr := httptest.NewRecorder()
	a.handleThemeChangeRequest(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestComprehensiveForumPostEngagementAndSorting(t *testing.T) {
	posts := forumPostList{{Title: "old", Score: 10, CommentCount: 5, TimePosted: time.Now().Add(-2 * time.Hour)}, {Title: "new", Score: 50, CommentCount: 20, TimePosted: time.Now().Add(-time.Hour)}}
	posts.calculateEngagement()
	if posts[0].Engagement <= 0 || posts[1].Engagement <= 0 {
		t.Fatal("engagement not calculated")
	}
	posts.sortByEngagement()
	if strings.TrimSpace(posts[0].Title) == "" {
		t.Fatal("sort corrupted posts")
	}
}
