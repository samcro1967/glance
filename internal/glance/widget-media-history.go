package glance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

var mediaHistoryWidgetTemplate = mustParseTemplate("media-history.html", "widget-base.html")

type mediaHistoryWidget struct {
	widgetBase    `yaml:",inline"`
	Service       string        `yaml:"service"`
	Server        string        `yaml:"server"`
	APIKey        string        `yaml:"api-key"`
	UserID        string        `yaml:"user-id"`
	Limit         int           `yaml:"limit"`
	CollapseAfter int           `yaml:"collapse-after"`
	Timeout       durationField `yaml:"timeout"`
	AllowInsecure bool          `yaml:"allow-insecure"`
	Items         []mediaItem   `yaml:"-"`
}

func (widget *mediaHistoryWidget) initialize() error {
	widget.withTitle("Media History").withCacheDuration(5 * time.Minute)
	config := mediaServerConfig{Service: widget.Service, Server: widget.Server, APIKey: widget.APIKey, UserID: widget.UserID, Limit: widget.Limit, Timeout: widget.Timeout, AllowInsecure: widget.AllowInsecure}
	if err := config.normalize(true); err != nil {
		return err
	}
	if config.Service == "navidrome" {
		return errors.New("service must be plex, jellyfin, or emby")
	}
	widget.Service = config.Service
	widget.Server = config.Server
	widget.UserID = config.UserID
	widget.Limit = config.Limit
	if widget.CollapseAfter < 0 {
		return errors.New("collapse-after must not be negative")
	}
	return nil
}
func (widget *mediaHistoryWidget) update(ctx context.Context) {
	items, err := fetchMediaHistory(ctx, widget)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}
	widget.Items = items
}
func (widget *mediaHistoryWidget) Render() template.HTML {
	return widget.renderTemplate(widget, mediaHistoryWidgetTemplate)
}

func fetchMediaHistory(ctx context.Context, widget *mediaHistoryWidget) ([]mediaItem, error) {
	var items []mediaItem
	var err error
	switch widget.Service {
	case "plex":
		items, err = fetchPlexHistory(ctx, widget)
	case "jellyfin", "emby":
		items, err = fetchJellyfinHistory(ctx, widget)
	default:
		return nil, errors.New("unsupported media history service")
	}
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].SortTime.After(items[j].SortTime) })
	if len(items) > widget.Limit {
		items = items[:widget.Limit]
	}
	for i := range items {
		items[i].ImageURL = proxyMediaImage(widget.Providers, items[i].ImageURL)
	}
	return items, nil
}

func fetchPlexHistory(ctx context.Context, widget *mediaHistoryWidget) ([]mediaItem, error) {
	values := url.Values{"sort": {"viewedAt:desc"}, "X-Plex-Container-Start": {"0"}, "X-Plex-Container-Size": {strconv.Itoa(widget.Limit)}}
	endpoint := widget.Server + "/status/sessions/history/all?" + values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	applyMediaServerAuth(request, widget.Service, widget.APIKey)
	request.Header.Set("Accept", "application/json")
	response, err := newHTTPClient(widget.Timeout, widget.AllowInsecure).Do(request)
	if err != nil {
		return nil, fmt.Errorf("requesting Plex media history: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, unexpectedHTTPStatusError(response)
	}
	var payload plexMediaResponse
	if err = json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding Plex media history: %w", err)
	}
	items := make([]mediaItem, 0, len(payload.MediaContainer.Metadata))
	for _, value := range payload.MediaContainer.Metadata {
		viewed := time.Unix(value.ViewedAt, 0)
		items = append(items, normalizePlexMedia(widget.Server, widget.APIKey, value, viewed))
	}
	return items, nil
}

func fetchJellyfinHistory(ctx context.Context, widget *mediaHistoryWidget) ([]mediaItem, error) {
	values := url.Values{"SortBy": {"DatePlayed"}, "SortOrder": {"Descending"}, "Recursive": {"true"}, "Limit": {strconv.Itoa(widget.Limit)}, "Filters": {"IsPlayed"}, "Fields": {"Overview,ProductionYear,RunTimeTicks,UserData"}, "IncludeItemTypes": {"Movie,Series,Episode,Audio"}}
	endpoint := widget.Server + "/Users/" + url.PathEscape(widget.UserID) + "/Items?" + values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	applyMediaServerAuth(request, widget.Service, widget.APIKey)
	request.Header.Set("Accept", "application/json")
	response, err := newHTTPClient(widget.Timeout, widget.AllowInsecure).Do(request)
	if err != nil {
		return nil, fmt.Errorf("requesting %s media history: %w", widget.Service, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, unexpectedHTTPStatusError(response)
	}
	var payload jellyfinItemsResponse
	if err = json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding %s media history: %w", widget.Service, err)
	}
	items := make([]mediaItem, 0, len(payload.Items))
	for _, value := range payload.Items {
		played := parseMediaTime(value.UserData.LastPlayedDate)
		if played.IsZero() {
			continue
		}
		items = append(items, normalizeJellyfinMedia(widget.Server, widget.APIKey, value, played))
	}
	return items, nil
}
