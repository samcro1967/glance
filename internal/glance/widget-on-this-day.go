package glance

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"
)

var onThisDayWidgetTemplate = mustParseTemplate("on-this-day.html", "widget-base.html")

const wikimediaOnThisDayURL = "https://api.wikimedia.org/feed/v1/wikipedia/en/onthisday/selected/%02d/%02d"

type onThisDayWidget struct {
	widgetBase `yaml:",inline"`
	Limit      int              `yaml:"limit"`
	Events     []onThisDayEvent `yaml:"-"`
}
type onThisDayEvent struct {
	Year                int
	Text, URL, ImageURL string
}
type wikimediaOnThisDayResponse struct {
	Selected []struct {
		Text  string `json:"text"`
		Year  int    `json:"year"`
		Pages []struct {
			Thumbnail struct {
				Source string `json:"source"`
			} `json:"thumbnail"`
			ContentURLs struct {
				Desktop struct {
					Page string `json:"page"`
				} `json:"desktop"`
			} `json:"content_urls"`
		} `json:"pages"`
	} `json:"selected"`
}

func (w *onThisDayWidget) initialize() error {
	w.withTitle("On This Day").withCacheDuration(24 * time.Hour)
	if w.Limit < 1 {
		return fmt.Errorf("limit must be at least 1")
	}
	return nil
}
func (w *onThisDayWidget) update(ctx context.Context) {
	now := time.Now()
	url := dailyDiscoveryProviderURL(fmt.Sprintf("/daily-discovery/on-this-day/%02d/%02d", int(now.Month()), now.Day()), fmt.Sprintf(wikimediaOnThisDayURL, int(now.Month()), now.Day()))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(err)
		return
	}
	req.Header.Set("User-Agent", glanceUserAgentString)
	response, err := decodeJsonFromRequest[wikimediaOnThisDayResponse](defaultHTTPClient, req)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(fmt.Errorf("fetching Wikimedia on this day: %w", err))
		return
	}
	events := make([]onThisDayEvent, 0, min(w.Limit, len(response.Selected)))
	for _, selected := range response.Selected {
		if len(events) >= w.Limit {
			break
		}
		if strings.TrimSpace(selected.Text) == "" {
			continue
		}
		event := onThisDayEvent{Year: selected.Year, Text: strings.TrimSpace(selected.Text)}
		for _, page := range selected.Pages {
			if event.URL == "" {
				event.URL = strings.TrimSpace(page.ContentURLs.Desktop.Page)
			}
			if event.ImageURL == "" {
				event.ImageURL = w.resolveResourceProxyImageURL(page.Thumbnail.Source)
			}
			if event.URL != "" && event.ImageURL != "" {
				break
			}
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		w.canContinueUpdateAfterHandlingErr(fmt.Errorf("Wikimedia returned no usable on-this-day events"))
		return
	}
	if !w.canContinueUpdateAfterHandlingErr(nil) {
		return
	}
	w.Events = events
}
func (w *onThisDayWidget) Render() template.HTML     { return w.renderTemplate(w, onThisDayWidgetTemplate) }
func (w *onThisDayWidget) setDefaultLimit(value int) { w.Limit = value }
