package glance

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func decodeMicroWidgetTestConfig(t *testing.T, source string) *config {
	t.Helper()

	source += `
pages:
  - name: Home
    columns:
      - size: full
`
	c, err := newConfigFromYAML([]byte(source))
	if err != nil {
		t.Fatalf("newConfigFromYAML() error = %v", err)
	}

	return c
}

func TestFooterMicroWidgetsDecodeAndSort(t *testing.T) {
	c := decodeMicroWidgetTestConfig(t, `
footer-micro-widgets:
  left:
    - type: bookmark
      position: 3
      title: Three
      url: https://example.com/three
    - type: bookmark
      position: 1
      title: One
      url: https://example.com/one
  right:
    - type: bookmark
      position: 2
      title: Two
      url: https://example.com/two
`)

	if len(c.FooterMicroWidgets.Left) != 2 {
		t.Fatalf("left micro-widget count = %d, want 2", len(c.FooterMicroWidgets.Left))
	}
	if len(c.FooterMicroWidgets.Right) != 1 {
		t.Fatalf("right micro-widget count = %d, want 1", len(c.FooterMicroWidgets.Right))
	}

	if got := c.FooterMicroWidgets.Left[0].GetPosition(); got != 1 {
		t.Errorf("left[0] position = %d, want 1", got)
	}
	if got := c.FooterMicroWidgets.Left[1].GetPosition(); got != 3 {
		t.Errorf("left[1] position = %d, want 3", got)
	}
	if got := c.FooterMicroWidgets.Right[0].GetPosition(); got != 2 {
		t.Errorf("right[0] position = %d, want 2", got)
	}

	first, ok := c.FooterMicroWidgets.Left[0].(*microBookmark)
	if !ok {
		t.Fatalf("left[0] type = %T, want *microBookmark", c.FooterMicroWidgets.Left[0])
	}
	if first.Title != "One" || first.URL != "https://example.com/one" {
		t.Errorf("left[0] = title %q url %q, want One and https://example.com/one", first.Title, first.URL)
	}
}

func TestFooterMicroWidgetsSamePositionAllowedOnOppositeSides(t *testing.T) {
	c := decodeMicroWidgetTestConfig(t, `
footer-micro-widgets:
  left:
    - type: bookmark
      position: 1
      title: Left
      url: https://example.com/left
  right:
    - type: bookmark
      position: 1
      title: Right
      url: https://example.com/right
`)

	if len(c.FooterMicroWidgets.Left) != 1 || len(c.FooterMicroWidgets.Right) != 1 {
		t.Fatalf("micro-widget counts = left %d right %d, want 1 and 1",
			len(c.FooterMicroWidgets.Left), len(c.FooterMicroWidgets.Right))
	}
}

func TestFooterMicroWidgetValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "missing type",
			yaml: `
footer-micro-widgets:
  left:
    - position: 1
      title: Example
      url: https://example.com
`,
			wantErr: "micro-widget 'type' property is empty or not specified",
		},
		{
			name: "unknown type",
			yaml: `
footer-micro-widgets:
  left:
    - type: unknown
      position: 1
`,
			wantErr: "unknown micro-widget type: unknown",
		},
		{
			name: "position zero",
			yaml: `
footer-micro-widgets:
  left:
    - type: bookmark
      position: 0
      title: Example
      url: https://example.com
`,
			wantErr: "footer micro-widgets left: position must be between 1 and 5, got 0",
		},
		{
			name: "duplicate position",
			yaml: `
footer-micro-widgets:
  left:
    - type: bookmark
      position: 2
      title: First
      url: https://example.com/first
    - type: bookmark
      position: 2
      title: Second
      url: https://example.com/second
`,
			wantErr: "footer micro-widgets left: position 2 is configured more than once",
		},
		{
			name: "missing bookmark title",
			yaml: `
footer-micro-widgets:
  left:
    - type: bookmark
      position: 1
      url: https://example.com
`,
			wantErr: "bookmark micro-widget title is required",
		},
		{
			name: "missing bookmark url",
			yaml: `
footer-micro-widgets:
  left:
    - type: bookmark
      position: 1
      title: Example
`,
			wantErr: "bookmark micro-widget url is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newConfigFromYAML([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("newConfigFromYAML() succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestFooterMicroWidgetsCapacity(t *testing.T) {
	t.Run("defaults to five", func(t *testing.T) {
		c := decodeMicroWidgetTestConfig(t, `
footer-micro-widgets:
  left:
    - type: bookmark
      position: 5
      title: Five
      url: https://example.com/five
`)

		if c.FooterMicroWidgets.MaxPerSide != 5 {
			t.Fatalf("MaxPerSide = %d, want 5", c.FooterMicroWidgets.MaxPerSide)
		}
	})

	t.Run("configured capacity accepts position", func(t *testing.T) {
		c := decodeMicroWidgetTestConfig(t, `
footer-micro-widgets:
  max-per-side: 8
  right:
    - type: bookmark
      position: 8
      title: Eight
      url: https://example.com/eight
`)

		if c.FooterMicroWidgets.MaxPerSide != 8 {
			t.Fatalf("MaxPerSide = %d, want 8", c.FooterMicroWidgets.MaxPerSide)
		}
	})

	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "position exceeds configured capacity",
			yaml: `
footer-micro-widgets:
  max-per-side: 3
  left:
    - type: bookmark
      position: 4
      title: Four
      url: https://example.com/four
`,
			wantErr: "footer micro-widgets left: position must be between 1 and 3, got 4",
		},
		{
			name: "capacity below minimum",
			yaml: `
footer-micro-widgets:
  max-per-side: -1
`,
			wantErr: "footer micro-widget max-per-side must be between 1 and 10, got -1",
		},
		{
			name: "capacity above maximum",
			yaml: `
footer-micro-widgets:
  max-per-side: 11
`,
			wantErr: "footer micro-widget max-per-side must be between 1 and 10, got 11",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newConfigFromYAML([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("newConfigFromYAML() succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestMicroBookmarkOptions(t *testing.T) {
	c := decodeMicroWidgetTestConfig(t, `
footer-micro-widgets:
  left:
    - type: bookmark
      position: 1
      title: Glance
      url: https://example.com
      same-tab: true
      icon: si:github
`)

	bookmark, ok := c.FooterMicroWidgets.Left[0].(*microBookmark)
	if !ok {
		t.Fatalf("micro-widget type = %T, want *microBookmark", c.FooterMicroWidgets.Left[0])
	}

	if !bookmark.SameTab {
		t.Error("SameTab = false, want true")
	}
	if bookmark.Icon.URL == "" {
		t.Error("Icon.URL is empty, want decoded icon URL")
	}
	if !bookmark.Icon.AutoInvert {
		t.Error("Icon.AutoInvert = false, want true for si: icon")
	}
}

func TestMicroClockOptions(t *testing.T) {
	tests := []struct {
		name           string
		yaml           string
		wantHourFormat string
		wantTimezone   string
		wantLabel      string
	}{
		{
			name: "defaults to 24 hour",
			yaml: `
footer-micro-widgets:
  left:
    - type: clock
      position: 1
`,
			wantHourFormat: "24h",
		},
		{
			name: "12 hour with label",
			yaml: `
footer-micro-widgets:
  left:
    - type: clock
      position: 1
      hour-format: 12h
      label: Local
`,
			wantHourFormat: "12h",
			wantLabel:      "Local",
		},
		{
			name: "timezone",
			yaml: `
footer-micro-widgets:
  left:
    - type: clock
      position: 1
      hour-format: 24h
      timezone: America/New_York
      label: NY
`,
			wantHourFormat: "24h",
			wantTimezone:   "America/New_York",
			wantLabel:      "NY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := decodeMicroWidgetTestConfig(t, tt.yaml)

			if len(c.FooterMicroWidgets.Left) != 1 {
				t.Fatalf("left micro-widget count = %d, want 1", len(c.FooterMicroWidgets.Left))
			}

			clock, ok := c.FooterMicroWidgets.Left[0].(*microClock)
			if !ok {
				t.Fatalf("micro-widget type = %T, want *microClock", c.FooterMicroWidgets.Left[0])
			}

			if clock.HourFormat != tt.wantHourFormat {
				t.Errorf("HourFormat = %q, want %q", clock.HourFormat, tt.wantHourFormat)
			}
			if clock.Timezone != tt.wantTimezone {
				t.Errorf("Timezone = %q, want %q", clock.Timezone, tt.wantTimezone)
			}
			if clock.Label != tt.wantLabel {
				t.Errorf("Label = %q, want %q", clock.Label, tt.wantLabel)
			}
		})
	}
}

func TestMicroClockRejectsInvalidHourFormat(t *testing.T) {
	_, err := newConfigFromYAML([]byte(`
footer-micro-widgets:
  left:
    - type: clock
      position: 1
      hour-format: 13h
`))

	if err == nil {
		t.Fatal("newConfigFromYAML() succeeded, want invalid hour-format error")
	}

	const want = "clock micro-widget hour-format must be either 12h or 24h"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want error containing %q", err, want)
	}
}

func TestMicroClockRejectsInvalidTimezone(t *testing.T) {
	_, err := newConfigFromYAML([]byte(`
footer-micro-widgets:
  left:
    - type: clock
      position: 1
      timezone: Invalid/Nowhere
`))

	if err == nil {
		t.Fatal("newConfigFromYAML() succeeded, want invalid timezone error")
	}

	const want = "invalid timezone"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want error containing %q", err, want)
	}
}

func TestDynamicFooterMicroWidgetsAreInitialized(t *testing.T) {
	c := decodeMicroWidgetTestConfig(t, `
footer-micro-widgets:
  left:
    - type: weather
      position: 1
      location: St. Louis, Missouri
    - type: markets
      position: 2
      markets:
        - symbol: AAPL
  right:
    - type: monitor
      position: 1
      sites:
        - title: Example
          url: https://example.com
`)

	weather, ok := c.FooterMicroWidgets.Left[0].(*microWeather)
	if !ok {
		t.Fatalf("left[0] type = %T, want *microWeather", c.FooterMicroWidgets.Left[0])
	}
	if weather.Units != "metric" {
		t.Errorf("weather Units = %q, want metric", weather.Units)
	}

	markets, ok := c.FooterMicroWidgets.Left[1].(*microMarkets)
	if !ok {
		t.Fatalf("left[1] type = %T, want *microMarkets", c.FooterMicroWidgets.Left[1])
	}
	if len(markets.MarketsRequests) != 1 || markets.MarketsRequests[0].Symbol != "AAPL" {
		t.Fatalf("markets requests = %#v, want one AAPL request", markets.MarketsRequests)
	}

	monitor, ok := c.FooterMicroWidgets.Right[0].(*microMonitor)
	if !ok {
		t.Fatalf("right[0] type = %T, want *microMonitor", c.FooterMicroWidgets.Right[0])
	}
	if len(monitor.Sites) != 1 {
		t.Fatalf("monitor site count = %d, want 1", len(monitor.Sites))
	}
}

func TestDynamicFooterMicroWidgetInitializationErrors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "weather requires location",
			yaml: `
footer-micro-widgets:
  left:
    - type: weather
      position: 1
`,
			wantErr: "weather micro-widget: location is required",
		},
		{
			name: "markets requires market",
			yaml: `
footer-micro-widgets:
  left:
    - type: markets
      position: 1
`,
			wantErr: "markets micro-widget: at least one market is required",
		},
		{
			name: "monitor requires site",
			yaml: `
footer-micro-widgets:
  left:
    - type: monitor
      position: 1
`,
			wantErr: "monitor micro-widget: at least one site is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := tt.yaml + `
pages:
  - name: Home
    columns:
      - size: full
`
			_, err := newConfigFromYAML([]byte(source))
			if err == nil {
				t.Fatalf("newConfigFromYAML() succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want error containing %q", err, tt.wantErr)
			}
		})
	}
}
func TestMicroLinkOptionsAndValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{name: "options", yaml: `footer-micro-widgets:
  left:
    - type: link
      position: 1
      title: Docs
      url: https://example.com/docs
      same-tab: true
`},
		{name: "missing title", yaml: `footer-micro-widgets:
  left:
    - type: link
      position: 1
      url: https://example.com
`, wantErr: "link micro-widget title is required"},
		{name: "missing url", yaml: `footer-micro-widgets:
  left:
    - type: link
      position: 1
      title: Docs
`, wantErr: "link micro-widget url is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr != "" {
				_, err := newConfigFromYAML([]byte(tt.yaml))
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}

			c := decodeMicroWidgetTestConfig(t, tt.yaml)
			link, ok := c.FooterMicroWidgets.Left[0].(*microLink)
			if !ok {
				t.Fatalf("micro-widget type = %T, want *microLink", c.FooterMicroWidgets.Left[0])
			}
			if link.Title != "Docs" || link.URL != "https://example.com/docs" || !link.SameTab {
				t.Fatalf("link = %#v, want configured title, URL, and same-tab", link)
			}
		})
	}
}

func TestMicroWeatherOptionsAndValidation(t *testing.T) {
	c := decodeMicroWidgetTestConfig(t, `footer-micro-widgets:
  left:
    - type: weather
      position: 1
      location: St. Louis, Missouri
      units: imperial
      show-area-name: true
      hide-location: true
      url: https://weather.example/st-louis
      same-tab: true
`)

	weather := c.FooterMicroWidgets.Left[0].(*microWeather)
	if weather.Units != "imperial" || !weather.ShowAreaName || !weather.HideLocation {
		t.Fatalf("weather options = %#v, want imperial/show-area-name/hide-location", weather)
	}
	if weather.URL != "https://weather.example/st-louis" || !weather.SameTab {
		t.Fatalf("weather navigation = url %q same-tab %v", weather.URL, weather.SameTab)
	}

	_, err := newConfigFromYAML([]byte(`footer-micro-widgets:
  left:
    - type: weather
      position: 1
      location: St. Louis, Missouri
      units: invalid
pages:
  - name: Home
    columns:
      - size: full
`))
	if err == nil || !strings.Contains(err.Error(), "units must be either metric or imperial") {
		t.Fatalf("invalid units error = %v", err)
	}
}

func TestMicroMarketsOptionsAndValidation(t *testing.T) {
	c := decodeMicroWidgetTestConfig(t, `footer-micro-widgets:
  left:
    - type: markets
      position: 1
      stocks:
        - symbol: AAPL
      sort-by: change
      chart-link-template: https://chart.example/{SYMBOL}
      symbol-link-template: https://quote.example/{SYMBOL}
      same-tab: true
`)

	markets := c.FooterMicroWidgets.Left[0].(*microMarkets)
	if len(markets.MarketsRequests) != 1 || markets.MarketsRequests[0].Symbol != "AAPL" {
		t.Fatalf("markets requests = %#v, want legacy stocks AAPL", markets.MarketsRequests)
	}
	if markets.MarketsRequests[0].ChartLink != "https://chart.example/AAPL" {
		t.Errorf("chart link = %q", markets.MarketsRequests[0].ChartLink)
	}
	if markets.MarketsRequests[0].SymbolLink != "https://quote.example/AAPL" {
		t.Errorf("symbol link = %q", markets.MarketsRequests[0].SymbolLink)
	}
	if markets.Sort != "change" {
		t.Errorf("sort = %q, want change", markets.Sort)
	}
	if !markets.SameTab {
		t.Error("same-tab = false, want true")
	}

	_, err := newConfigFromYAML([]byte(`footer-micro-widgets:
  left:
    - type: markets
      position: 1
      markets:
        - name: Missing symbol
pages:
  - name: Home
    columns:
      - size: full
`))
	if err == nil || !strings.Contains(err.Error(), "market symbol is required") {
		t.Fatalf("missing symbol error = %v", err)
	}
}

func TestMicroMarketsSymbolLinkPrecedence(t *testing.T) {
	c := decodeMicroWidgetTestConfig(t, `footer-micro-widgets:
  left:
    - type: markets
      position: 1
      markets:
        - symbol: SPY
        - symbol: QQQ
          symbol-link: https://quote.example/custom-qqq
`)

	markets := c.FooterMicroWidgets.Left[0].(*microMarkets)

	if got := markets.MarketsRequests[0].SymbolLink; got != "https://finance.yahoo.com/quote/SPY" {
		t.Errorf("default SPY symbol link = %q, want Yahoo Finance", got)
	}

	if got := markets.MarketsRequests[1].SymbolLink; got != "https://quote.example/custom-qqq" {
		t.Errorf("explicit QQQ symbol link = %q, want configured override", got)
	}
}

func TestMicroDockerOptionsAndValidation(t *testing.T) {
	c := decodeMicroWidgetTestConfig(t, `footer-micro-widgets:
  left:
    - type: docker
      position: 1
      container: glance
      sock-path: http://127.0.0.1:18089
      name: Dashboard
      url: https://example.com/glance
      same-tab: true
`)

	docker := c.FooterMicroWidgets.Left[0].(*microDocker)
	if docker.Container != "glance" {
		t.Errorf("Container = %q, want glance", docker.Container)
	}
	if docker.SockPath != "http://127.0.0.1:18089" {
		t.Errorf("SockPath = %q", docker.SockPath)
	}
	if docker.Name != "Dashboard" || docker.URL != "https://example.com/glance" || !docker.SameTab {
		t.Errorf("docker navigation/display options = %#v", docker)
	}

	summary := decodeMicroWidgetTestConfig(t, `footer-micro-widgets:
  left:
    - type: docker
      position: 1
      summary: true
`).FooterMicroWidgets.Left[0].(*microDocker)

	if !summary.Summary {
		t.Fatal("Summary = false, want true")
	}
	if summary.SockPath != "/var/run/docker.sock" {
		t.Errorf("default SockPath = %q, want /var/run/docker.sock", summary.SockPath)
	}
}

func TestMicroDockerValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "requires mode",
			yaml: `footer-micro-widgets:
  left:
    - type: docker
      position: 1
`,
			wantErr: "docker micro-widget: either container or summary is required",
		},
		{
			name: "modes mutually exclusive",
			yaml: `footer-micro-widgets:
  left:
    - type: docker
      position: 1
      container: glance
      summary: true
`,
			wantErr: "docker micro-widget: container and summary cannot both be configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newConfigFromYAML([]byte(tt.yaml + `
pages:
  - name: Home
    columns:
      - size: full
`))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestMicroMonitorOptionsAndStatusUpdate(t *testing.T) {
	resetMonitorResourceCache(t)

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/ok":
			response := monitorResourceTestResponse(request)
			response.StatusCode = http.StatusOK
			response.Status = "200 OK"
			return response, nil
		case "/alternate":
			response := monitorResourceTestResponse(request)
			response.StatusCode = http.StatusNotFound
			response.Status = "404 Not Found"
			return response, nil
		default:
			t.Fatalf("unexpected monitor request path %q", request.URL.Path)
			return nil, nil
		}
	})

	c := decodeMicroWidgetTestConfig(t, `footer-micro-widgets:
  left:
    - type: monitor
      position: 1
      show-failing-only: true
      sites:
        - title: Healthy
          url: https://example.invalid/ok
        - title: Alternate
          url: https://example.invalid/alternate
          alt-status-codes:
            - 404
`)

	monitor := c.FooterMicroWidgets.Left[0].(*microMonitor)
	if !monitor.ShowFailingOnly {
		t.Fatal("ShowFailingOnly = false, want true")
	}

	monitor.update(context.Background())

	if monitor.HasFailing {
		t.Fatal("HasFailing = true, want false when all statuses are accepted")
	}
	if monitor.Sites[0].StatusStyle != "ok" || monitor.Sites[0].StatusText != "OK" {
		t.Fatalf("healthy status = %q/%q, want ok/OK", monitor.Sites[0].StatusStyle, monitor.Sites[0].StatusText)
	}
	if monitor.Sites[1].StatusStyle != "ok" || monitor.Sites[1].StatusText != "OK" {
		t.Fatalf("alternate status = %q/%q, want ok/OK", monitor.Sites[1].StatusStyle, monitor.Sites[1].StatusText)
	}
}

func TestMicroMonitorFailureUsesErrorURL(t *testing.T) {
	resetMonitorResourceCache(t)
	wantErr := errors.New("monitor unavailable")

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		return nil, wantErr
	})

	c := decodeMicroWidgetTestConfig(t, `footer-micro-widgets:
  left:
    - type: monitor
      position: 1
      sites:
        - title: Broken
          url: https://example.invalid/default
          error-url: https://example.invalid/error
`)

	monitor := c.FooterMicroWidgets.Left[0].(*microMonitor)
	monitor.update(context.Background())

	if !monitor.HasFailing {
		t.Fatal("HasFailing = false, want true")
	}
	site := monitor.Sites[0]
	if site.URL != "https://example.invalid/error" {
		t.Fatalf("site URL = %q, want error URL", site.URL)
	}
	if site.StatusText != "Error" || site.StatusStyle != "error" {
		t.Fatalf("status = %q/%q, want Error/error", site.StatusText, site.StatusStyle)
	}
	if !errors.Is(site.Status.Error, wantErr) {
		t.Fatalf("status error = %v, want %v", site.Status.Error, wantErr)
	}
}
