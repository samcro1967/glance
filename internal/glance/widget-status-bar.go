package glance

import (
	"context"
	"errors"
	"html/template"
	"time"
)

var statusBarWidgetTemplate = mustParseTemplate("status-bar.html", "widget-base.html")

type statusBarCompactItem struct {
	Kind              string
	URL               string
	OpenLinksInNewTab bool

	Error  error
	Notice error

	ErrorTitle string

	// Custom API fields
	Icon1           string
	Line1           string
	Line2           string
	Icon2           string
	StatusStyle     string
	ClockHourFormat string
	ClockTimezone   string

	WeatherCondition   string
	WeatherTemperature int
	WeatherFeelsLike   int
	WeatherUnit        string
	WeatherLocation    string

	MarketSymbol         string
	MarketName           string
	MarketChartURL       string
	MarketCurrency       string
	MarketCurrencySymbol string
	MarketPrice          float64
	MarketPriceHint      int
	MarketPercentChange  float64

	RSSTitle       string
	RSSChannelName string
	RSSChannelURL  string
	RSSPublishedAt time.Time
}

type statusBarWidget struct {
	widgetBase `yaml:",inline"`
	Widgets    microWidgets `yaml:"widgets"`
	Mode       string       `yaml:"mode"`
	Speed      string       `yaml:"speed"`
}

func (widget *statusBarWidget) childWidgets() widgets { return widget.Widgets.asWidgets() }

func (widget *statusBarWidget) initialize() error {
	widget.withError(nil)
	widget.HideHeader = true
	if widget.Mode == "" {
		widget.Mode = "ticker"
	}
	if widget.Mode != "ticker" && widget.Mode != "wrap" {
		return errors.New("mode can only be either ticker or wrap")
	}
	if widget.Speed == "" {
		widget.Speed = "normal"
	}
	if widget.Speed != "slow" && widget.Speed != "normal" && widget.Speed != "fast" {
		return errors.New("speed can only be slow, normal or fast")
	}
	if len(widget.Widgets) == 0 {
		return errors.New("at least one widget is required")
	}
	for i := range widget.Widgets {
		if _, invalid := widget.Widgets[i].(*invalidConfiguredMicroWidget); invalid {
			continue
		}
		if _, ok := widget.Widgets[i].(microSource); !ok {
			return errors.New("only registered micro-widgets are supported")
		}
		widget.Widgets[i].setHideHeader(true)
		if customAPI, ok := widget.Widgets[i].(*customAPIWidget); ok {
			customAPI.statusBarCompactMode = true
		}
		if err := widget.Widgets[i].initialize(); err != nil {
			candidate := widget.Widgets[i]
			line := 0
			if base, ok := widgetBaseOf(candidate); ok {
				line = base.configLine
			}
			formatted := formatWidgetInitError(err, candidate)
			if micro, ok := candidate.(microWidget); ok {
				widget.Widgets[i] = newInvalidMicroWidget(candidate, candidate.GetType(), micro.GetPosition(), line, formatted)
			} else {
				widget.Widgets[i] = newInvalidConfiguredWidget(candidate, candidate.GetType(), line, formatted)
			}
		}
	}
	return nil
}

func (statusBar *statusBarWidget) update(ctx context.Context) {
	now := time.Now()
	task := func(child widget) (struct{}, error) { refreshWidgetIfNeeded(ctx, child, &now); return struct{}{}, nil }
	_, _, _ = workerPoolDo(newJob(task, widgets(statusBar.Widgets)).withWorkers(widgetNestedConcurrency).withContext(ctx))
}
func (widget *statusBarWidget) setProviders(providers *widgetProviders) {
	widget.widgetBase.setProviders(providers)
	for _, child := range widget.Widgets {
		child.setProviders(providers)
	}
}
func (widget *statusBarWidget) requiresUpdate(now *time.Time) bool {
	for _, child := range widget.Widgets {
		if child.requiresUpdate(now) {
			return true
		}
	}
	return false
}
func (widget *statusBarWidget) CompactItems() []statusBarCompactItem {
	items := make([]statusBarCompactItem, 0)
	for _, child := range widget.Widgets {
		if invalid, ok := child.(*invalidConfiguredWidget); ok {
			items = append(items, statusBarCompactItem{Kind: "error", Error: invalid.Error, ErrorTitle: invalid.Title})
			continue
		}
		micro, ok := child.(microSource)
		if !ok {
			continue
		}
		items = append(items, micro.MicroItems(widget.OpenLinksInNewTab)...)
	}
	return items
}
func (widget *statusBarWidget) Render() template.HTML {
	return widget.renderTemplate(widget, statusBarWidgetTemplate)
}
