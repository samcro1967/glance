package glance

import (
	"context"
	"errors"
	"html/template"
	"time"
)

var statusBarWidgetTemplate = mustParseTemplate("status-bar.html", "widget-base.html")

type statusBarCompactItem struct {
	Kind string
	URL  string

	Error  error
	Notice error

	ErrorTitle string

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
	widgetBase          `yaml:",inline"`
	containerWidgetBase `yaml:",inline"`
	Mode                string `yaml:"mode"`
}

func (widget *statusBarWidget) initialize() error {
	widget.withError(nil)
	widget.HideHeader = true

	if widget.Mode == "" {
		widget.Mode = "ticker"
	}

	if widget.Mode != "ticker" && widget.Mode != "wrap" {
		return errors.New("mode can only be either ticker or wrap")
	}

	if len(widget.Widgets) == 0 {
		return errors.New("at least one widget is required")
	}

	for i := range widget.Widgets {
		switch widget.Widgets[i].GetType() {
		case "weather", "markets", "rss":
		default:
			return errors.New("only weather, markets and rss widgets are supported")
		}

		widget.Widgets[i].setHideHeader(true)
	}

	return widget.containerWidgetBase._initializeWidgets()
}

func (widget *statusBarWidget) update(ctx context.Context) {
	widget.containerWidgetBase._update(ctx)
}

func (widget *statusBarWidget) setProviders(providers *widgetProviders) {
	widget.containerWidgetBase._setProviders(providers)
}

func (widget *statusBarWidget) requiresUpdate(now *time.Time) bool {
	return widget.containerWidgetBase._requiresUpdate(now)
}

func (widget *statusBarWidget) CompactItems() []statusBarCompactItem {
	items := make([]statusBarCompactItem, 0)

	for i := range widget.Widgets {
		switch child := widget.Widgets[i].(type) {
		case *weatherWidget:
			if child.Weather == nil || child.Place == nil {
				if child.Error != nil {
					items = append(items, statusBarCompactItem{
						Kind:       "error",
						Error:      child.Error,
						ErrorTitle: child.Title,
					})
				}
				continue
			}

			unit := "F"
			if child.Units == "metric" {
				unit = "C"
			}

			location := ""
			if !child.HideLocation {
				location = child.Place.Name
				if child.ShowAreaName && child.Place.Area != "" {
					location += ", " + child.Place.Area
				}
				if child.Place.Country != "" {
					location += ", " + child.Place.Country
				}
			}

			items = append(items, statusBarCompactItem{
				Kind:               "weather",
				Error:              child.Error,
				Notice:             child.Notice,
				WeatherCondition:   child.Weather.WeatherCodeAsString(),
				WeatherTemperature: child.Weather.Temperature,
				WeatherFeelsLike:   child.Weather.ApparentTemperature,
				WeatherUnit:        unit,
				WeatherLocation:    location,
			})

		case *marketsWidget:
			if len(child.Markets) == 0 && child.Error != nil {
				items = append(items, statusBarCompactItem{
					Kind:       "error",
					Error:      child.Error,
					ErrorTitle: child.Title,
				})
				continue
			}

			for j := range child.Markets {
				market := child.Markets[j]
				items = append(items, statusBarCompactItem{
					Kind:                 "market",
					Error:                child.Error,
					Notice:               child.Notice,
					URL:                  market.SymbolLink,
					MarketSymbol:         market.Symbol,
					MarketName:           market.Name,
					MarketChartURL:       market.ChartLink,
					MarketCurrency:       market.Currency,
					MarketCurrencySymbol: market.CurrencySymbol,
					MarketPrice:          market.Price,
					MarketPriceHint:      market.PriceHint,
					MarketPercentChange:  market.PercentChange,
				})
			}

		case *rssWidget:
			if len(child.Items) == 0 && child.Error != nil {
				items = append(items, statusBarCompactItem{
					Kind:       "error",
					Error:      child.Error,
					ErrorTitle: child.Title,
				})
				continue
			}

			for j := range child.Items {
				item := child.Items[j]
				items = append(items, statusBarCompactItem{
					Kind:           "rss",
					Error:          child.Error,
					Notice:         child.Notice,
					URL:            item.Link,
					RSSTitle:       item.Title,
					RSSChannelName: item.ChannelName,
					RSSChannelURL:  item.ChannelURL,
					RSSPublishedAt: item.PublishedAt,
				})
			}
		}
	}

	return items
}

func (widget *statusBarWidget) Render() template.HTML {
	return widget.renderTemplate(widget, statusBarWidgetTemplate)
}
