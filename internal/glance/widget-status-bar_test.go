package glance

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestStatusBarWidgetModes(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		wantMode string
		wantErr  string
	}{
		{name: "default", wantMode: "ticker"},
		{name: "ticker", mode: "ticker", wantMode: "ticker"},
		{name: "wrap", mode: "wrap", wantMode: "wrap"},
		{name: "invalid", mode: "scroll", wantErr: "mode can only be either ticker or wrap"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			child := &rssWidget{}
			child.Type = "rss"
			child.FeedRequests = []rssFeedRequest{{URL: "https://example.com/feed.xml"}}

			widget := &statusBarWidget{
				Mode: tt.mode,
				containerWidgetBase: containerWidgetBase{
					Widgets: widgets{child},
				},
			}
			widget.Type = "status-bar"

			err := widget.initialize()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("initialize() error = nil, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("initialize() error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("initialize() error = %v, want nil", err)
			}
			if widget.Mode != tt.wantMode {
				t.Errorf("Mode = %q, want %q", widget.Mode, tt.wantMode)
			}
		})
	}
}

func TestStatusBarWidgetRequiresChildren(t *testing.T) {
	widget := &statusBarWidget{}
	widget.Type = "status-bar"

	err := widget.initialize()
	if err == nil {
		t.Fatal("initialize() error = nil, want missing child error")
	}
	if err.Error() != "at least one widget is required" {
		t.Errorf("initialize() error = %q", err.Error())
	}
}

func TestStatusBarWidgetSupportedChildren(t *testing.T) {
	tests := []struct {
		name    string
		child   widget
		wantErr bool
	}{
		{name: "weather", child: &weatherWidget{Location: "Saint Peters, Missouri, United States"}},
		{name: "markets", child: &marketsWidget{}},
		{name: "rss", child: &rssWidget{FeedRequests: []rssFeedRequest{{URL: "https://example.com/feed.xml"}}}},
		{name: "clock rejected", child: &clockWidget{}, wantErr: true},
		{name: "group rejected", child: &groupWidget{}, wantErr: true},
		{name: "stack rejected", child: &stackWidget{}, wantErr: true},
		{name: "split column rejected", child: &splitColumnWidget{}, wantErr: true},
		{name: "nested status bar rejected", child: &statusBarWidget{}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.child.setID(widgetIDCounter.Add(1))
			switch child := tt.child.(type) {
			case *weatherWidget:
				child.Type = "weather"
			case *marketsWidget:
				child.Type = "markets"
			case *rssWidget:
				child.Type = "rss"
			case *clockWidget:
				child.Type = "clock"
			case *groupWidget:
				child.Type = "group"
			case *stackWidget:
				child.Type = "stack"
			case *splitColumnWidget:
				child.Type = "split-column"
			case *statusBarWidget:
				child.Type = "status-bar"
			}

			statusBar := &statusBarWidget{
				containerWidgetBase: containerWidgetBase{
					Widgets: widgets{tt.child},
				},
			}
			statusBar.Type = "status-bar"

			err := statusBar.initialize()
			if tt.wantErr {
				if err == nil {
					t.Fatal("initialize() error = nil, want unsupported child error")
				}
				if err.Error() != "only weather, markets and rss widgets are supported" {
					t.Errorf("initialize() error = %q", err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("initialize() error = %v, want nil", err)
			}
			switch child := tt.child.(type) {
			case *weatherWidget:
				if !child.HideHeader {
					t.Error("weather child header was not hidden")
				}
			case *marketsWidget:
				if !child.HideHeader {
					t.Error("markets child header was not hidden")
				}
			case *rssWidget:
				if !child.HideHeader {
					t.Error("rss child header was not hidden")
				}
			}
		})
	}
}

func TestStatusBarWidgetPropagatesChildInitializationError(t *testing.T) {
	child := &weatherWidget{}
	child.Type = "weather"

	widget := &statusBarWidget{
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{child},
		},
	}
	widget.Type = "status-bar"

	err := widget.initialize()
	if err == nil {
		t.Fatal("initialize() error = nil, want child initialization error")
	}
	if !strings.Contains(err.Error(), "weather widget: location is required") {
		t.Errorf("initialize() error = %q", err.Error())
	}
}

func TestStatusBarCompactItems(t *testing.T) {
	published := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

	weatherChild := &weatherWidget{
		Units:        "imperial",
		ShowAreaName: true,
		Place: &openMeteoPlaceResponseJson{
			Name:    "Saint Peters",
			Area:    "Missouri",
			Country: "United States",
		},
		Weather: &weather{
			Temperature:         82,
			ApparentTemperature: 85,
			WeatherCode:         0,
		},
	}

	marketsChild := &marketsWidget{
		widgetBase: widgetBase{OpenLinksInNewTab: true},
		Markets: marketList{
			{
				marketRequest: marketRequest{
					Symbol:     "NSIT",
					SymbolLink: "https://finance.yahoo.com/quote/NSIT",
					ChartLink:  "https://tradingview.com/chart?symbol=NSIT",
				},
				Name:           "Insight",
				Currency:       "USD",
				CurrencySymbol: "$",
				Price:          143.25,
				PriceHint:      2,
				PercentChange:  1.75,
			},
		},
	}

	rssChild := &rssWidget{
		Items: rssFeedItemList{
			{
				ChannelName: "Example News",
				ChannelURL:  "https://example.com",
				Title:       "Example headline",
				Link:        "https://example.com/article",
				PublishedAt: published,
			},
		},
	}

	widget := &statusBarWidget{
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{weatherChild, marketsChild, rssChild},
		},
	}

	items := widget.CompactItems()
	if len(items) != 3 {
		t.Fatalf("CompactItems() returned %d items, want 3", len(items))
	}

	if got := items[0]; got.Kind != "weather" || got.WeatherTemperature != 82 || got.WeatherFeelsLike != 85 || got.WeatherUnit != "F" || got.WeatherLocation != "Saint Peters, Missouri, United States" {
		t.Errorf("weather item = %+v", got)
	}

	if got := items[1]; got.Kind != "market" || got.MarketSymbol != "NSIT" || got.MarketName != "Insight" || got.URL != "https://finance.yahoo.com/quote/NSIT" || got.MarketChartURL != "https://tradingview.com/chart?symbol=NSIT" || got.MarketPrice != 143.25 || got.MarketPercentChange != 1.75 || !got.OpenLinksInNewTab {
		t.Errorf("market item = %+v", got)
	}

	if got := items[2]; got.Kind != "rss" || got.RSSTitle != "Example headline" || got.URL != "https://example.com/article" || got.RSSChannelName != "Example News" || !got.RSSPublishedAt.Equal(published) || got.OpenLinksInNewTab {
		t.Errorf("rss item = %+v", got)
	}
}

func TestStatusBarCompactWeatherOptions(t *testing.T) {
	widget := &statusBarWidget{
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{
				&weatherWidget{
					Units:        "metric",
					HideLocation: true,
					Place:        &openMeteoPlaceResponseJson{Name: "London", Country: "United Kingdom"},
					Weather:      &weather{Temperature: 20, ApparentTemperature: 19},
				},
			},
		},
	}

	items := widget.CompactItems()
	if len(items) != 1 {
		t.Fatalf("CompactItems() returned %d items, want 1", len(items))
	}
	if items[0].WeatherUnit != "C" {
		t.Errorf("WeatherUnit = %q, want C", items[0].WeatherUnit)
	}
	if items[0].WeatherLocation != "" {
		t.Errorf("WeatherLocation = %q, want empty", items[0].WeatherLocation)
	}
}

func TestStatusBarCompactWeatherWithoutDataIsSkipped(t *testing.T) {
	widget := &statusBarWidget{
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{&weatherWidget{}},
		},
	}

	if items := widget.CompactItems(); len(items) != 0 {
		t.Fatalf("CompactItems() returned %d items, want 0", len(items))
	}
}

func TestStatusBarRenderTicker(t *testing.T) {
	published := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

	widget := &statusBarWidget{
		Mode: "ticker",
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{
				&weatherWidget{
					Units: "imperial",
					Place: &openMeteoPlaceResponseJson{
						Name:    "Saint Peters",
						Country: "United States",
					},
					Weather: &weather{
						Temperature:         82,
						ApparentTemperature: 85,
						WeatherCode:         0,
					},
				},
				&marketsWidget{
					Markets: marketList{
						{
							marketRequest: marketRequest{
								Symbol:     "NSIT",
								SymbolLink: "https://finance.yahoo.com/quote/NSIT",
								ChartLink:  "https://tradingview.com/chart?symbol=NSIT",
							},
							Name:           "Insight",
							Currency:       "USD",
							CurrencySymbol: "$",
							Price:          143.25,
							PriceHint:      2,
							PercentChange:  1.75,
						},
					},
				},
				&rssWidget{
					Items: rssFeedItemList{
						{
							ChannelName: "Example News",
							ChannelURL:  "https://example.com",
							Title:       "Example headline",
							Link:        "https://example.com/article",
							PublishedAt: published,
						},
					},
				},
			},
		},
	}
	widget.Type = "status-bar"
	widget.HideHeader = true
	widget.ContentAvailable = true

	html := string(widget.Render())

	checks := []string{
		`status-bar-mode-ticker`,
		`status-bar-items-duplicate`,
		`aria-hidden="true" inert`,
		`Saint Peters, United States`,
		`82°F`,
		`Feels 85°F`,
		`href="https://finance.yahoo.com/quote/NSIT"`,
		`NSIT`,
		`Insight`,
		`href="https://tradingview.com/chart?symbol=NSIT"`,
		`$143.25`,
		`&#43;1.75%`,
		`href="https://example.com/article"`,
		`Example headline`,
		`href="https://example.com"`,
		`Example News`,
	}

	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("Render() missing %q in %q", check, html)
		}
	}

	if count := strings.Count(html, "Example headline"); count != 2 {
		t.Errorf("ticker rendered headline %d times, want 2", count)
	}
}

func TestStatusBarRenderWrap(t *testing.T) {
	widget := &statusBarWidget{
		Mode: "wrap",
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{
				&marketsWidget{
					Markets: marketList{
						{
							marketRequest:  marketRequest{Symbol: "BUD"},
							Name:           "Anheuser-Busch InBev",
							Currency:       "USD",
							CurrencySymbol: "$",
							Price:          61.5,
							PriceHint:      2,
						},
					},
				},
			},
		},
	}
	widget.Type = "status-bar"
	widget.HideHeader = true
	widget.ContentAvailable = true

	html := string(widget.Render())
	if !strings.Contains(html, "status-bar-mode-wrap") {
		t.Errorf("Render() missing wrap mode in %q", html)
	}
	if count := strings.Count(html, ">BUD</span>"); count != 2 {
		t.Errorf("wrap markup rendered market %d times, want 2 copies before CSS hides duplicate", count)
	}
}

func TestStatusBarRenderWithoutOptionalLinks(t *testing.T) {
	published := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	widget := &statusBarWidget{
		Mode: "ticker",
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{
				&marketsWidget{Markets: marketList{{
					marketRequest: marketRequest{Symbol: "TEST"},
					Name:          "Test Market", Currency: "USD", CurrencySymbol: "$", Price: 10, PriceHint: 2,
				}}},
				&rssWidget{Items: rssFeedItemList{{
					ChannelName: "Test Feed", Title: "Test Headline", PublishedAt: published,
				}}},
			},
		},
	}
	widget.Type = "status-bar"
	widget.HideHeader = true
	widget.ContentAvailable = true

	html := string(widget.Render())
	if strings.Contains(html, `href=""`) {
		t.Errorf("Render() contains empty href in %q", html)
	}
	if strings.Contains(html, `<a class="status-bar-market-identity"`) || strings.Contains(html, `<a class="status-bar-market-values"`) {
		t.Errorf("Render() rendered unlinked market content as anchors in %q", html)
	}
	if strings.Contains(html, `<a href=""`) {
		t.Errorf("Render() rendered empty RSS link in %q", html)
	}
	if !strings.Contains(html, `<span class="status-bar-market-identity">`) || !strings.Contains(html, `<span class="status-bar-market-values">`) {
		t.Errorf("Render() missing unlinked market spans in %q", html)
	}
	if !strings.Contains(html, `<span class="status-bar-primary status-bar-rss-title">Test Headline</span>`) || !strings.Contains(html, `<span>Test Feed</span>`) {
		t.Errorf("Render() missing unlinked RSS spans in %q", html)
	}
}

func TestStatusBarCompactItemsPreserveChildStatus(t *testing.T) {
	hardErr := errors.New("weather provider unavailable")
	partialErr := fmt.Errorf("%w: one market unavailable", errPartialContent)

	weatherChild := &weatherWidget{
		Units:   "imperial",
		Place:   &openMeteoPlaceResponseJson{Name: "Saint Peters", Country: "United States"},
		Weather: &weather{Temperature: 82, ApparentTemperature: 85},
	}
	weatherChild.Error = hardErr

	marketsChild := &marketsWidget{
		Markets: marketList{{
			marketRequest: marketRequest{Symbol: "TEST"},
			Name:          "Test Market", Currency: "USD", CurrencySymbol: "$", Price: 10, PriceHint: 2,
		}},
	}
	marketsChild.Notice = partialErr

	widget := &statusBarWidget{
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{weatherChild, marketsChild},
		},
	}

	items := widget.CompactItems()
	if len(items) != 2 {
		t.Fatalf("CompactItems() returned %d items, want 2", len(items))
	}
	if !errors.Is(items[0].Error, hardErr) || items[0].Notice != nil {
		t.Errorf("weather status = Error %v Notice %v", items[0].Error, items[0].Notice)
	}
	if items[1].Error != nil || !errors.Is(items[1].Notice, errPartialContent) {
		t.Errorf("market status = Error %v Notice %v", items[1].Error, items[1].Notice)
	}

	widget.Type = "status-bar"
	widget.HideHeader = true
	widget.ContentAvailable = true
	widget.Mode = "ticker"

	html := string(widget.Render())
	if !strings.Contains(html, `class="notice-icon notice-icon-major" title="weather provider unavailable"`) {
		t.Errorf("Render() missing major child error indicator in %q", html)
	}
	if !strings.Contains(html, `class="notice-icon notice-icon-minor" title="failed to retrieve some of the content: one market unavailable"`) {
		t.Errorf("Render() missing minor child notice indicator in %q", html)
	}
}

func TestStatusBarCompactItemsPreserveZeroContentErrors(t *testing.T) {
	weatherChild := &weatherWidget{}
	weatherChild.Title = "Weather"
	weatherChild.Error = errors.New("weather unavailable")

	marketsChild := &marketsWidget{}
	marketsChild.Title = "Markets"
	marketsChild.Error = errors.New("markets unavailable")

	rssChild := &rssWidget{}
	rssChild.Title = "RSS"
	rssChild.Error = errors.New("rss unavailable")

	widget := &statusBarWidget{
		Mode: "ticker",
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{weatherChild, marketsChild, rssChild},
		},
	}
	widget.Type = "status-bar"
	widget.HideHeader = true
	widget.ContentAvailable = true

	items := widget.CompactItems()
	if len(items) != 3 {
		t.Fatalf("CompactItems() returned %d items, want 3", len(items))
	}

	wantTitles := []string{"Weather", "Markets", "RSS"}
	for i := range items {
		if items[i].Kind != "error" {
			t.Errorf("item %d Kind = %q, want error", i, items[i].Kind)
		}
		if items[i].ErrorTitle != wantTitles[i] {
			t.Errorf("item %d ErrorTitle = %q, want %q", i, items[i].ErrorTitle, wantTitles[i])
		}
		if items[i].Error == nil {
			t.Errorf("item %d Error = nil, want error", i)
		}
	}

	html := string(widget.Render())
	for _, title := range wantTitles {
		if !strings.Contains(html, ">"+title+"</span>") {
			t.Errorf("Render() missing %q error title in %q", title, html)
		}
	}
	if count := strings.Count(html, ">Unavailable</span>"); count != 6 {
		t.Errorf("ticker rendered Unavailable %d times, want 6", count)
	}
	if count := strings.Count(html, "notice-icon-major"); count != 6 {
		t.Errorf("ticker rendered major error indicator %d times, want 6", count)
	}
}
