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
	"strings"
	"time"
)

var latestMediaWidgetTemplate = mustParseTemplate("latest-media.html", "widget-base.html")

type latestMediaWidget struct {
	widgetBase    `yaml:",inline"`
	Service       string        `yaml:"service"`
	Server        string        `yaml:"server"`
	APIKey        string        `yaml:"api-key"`
	Username      string        `yaml:"username"`
	Password      string        `yaml:"password"`
	Limit         int           `yaml:"limit"`
	CollapseAfter int           `yaml:"collapse-after"`
	Timeout       durationField `yaml:"timeout"`
	AllowInsecure bool          `yaml:"allow-insecure"`
	Items         []mediaItem   `yaml:"-"`
}

type plexMediaResponse struct {
	MediaContainer struct {
		Metadata []plexMediaItem `json:"Metadata"`
	} `json:"MediaContainer"`
}
type plexMediaItem struct {
	RatingKey        string `json:"ratingKey"`
	Type             string `json:"type"`
	Title            string `json:"title"`
	GrandparentTitle string `json:"grandparentTitle"`
	ParentIndex      int    `json:"parentIndex"`
	Index            int    `json:"index"`
	Year             int    `json:"year"`
	Summary          string `json:"summary"`
	Thumb            string `json:"thumb"`
	AddedAt          int64  `json:"addedAt"`
	Duration         int64  `json:"duration"`
}

type jellyfinItemsResponse struct {
	Items []jellyfinMediaItem `json:"Items"`
}
type jellyfinMediaItem struct {
	ID                string `json:"Id"`
	Name              string `json:"Name"`
	Type              string `json:"Type"`
	SeriesName        string `json:"SeriesName"`
	ParentIndexNumber int    `json:"ParentIndexNumber"`
	IndexNumber       int    `json:"IndexNumber"`
	ProductionYear    int    `json:"ProductionYear"`
	Overview          string `json:"Overview"`
	DateCreated       string `json:"DateCreated"`
	RunTimeTicks      int64  `json:"RunTimeTicks"`
	UserData          struct {
		LastPlayedDate string `json:"LastPlayedDate"`
	} `json:"UserData"`
}

type navidromeResponse struct {
	SubsonicResponse struct {
		AlbumList2 struct {
			Album []navidromeAlbum `json:"album"`
		} `json:"albumList2"`
		Status string `json:"status"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"subsonic-response"`
}
type navidromeAlbum struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Artist   string `json:"artist"`
	Year     int    `json:"year"`
	Duration int64  `json:"duration"`
	CoverArt string `json:"coverArt"`
	Created  string `json:"created"`
}

func (widget *latestMediaWidget) initialize() error {
	widget.withTitle("Latest Media").withCacheDuration(5 * time.Minute)
	config := widget.mediaConfig()
	if err := config.normalize(false); err != nil {
		return err
	}
	widget.applyMediaConfig(config)
	if widget.CollapseAfter < 0 {
		return errors.New("collapse-after must not be negative")
	}
	return nil
}

func (widget *latestMediaWidget) mediaConfig() mediaServerConfig {
	return mediaServerConfig{Service: widget.Service, Server: widget.Server, APIKey: widget.APIKey, Username: widget.Username, Password: widget.Password, Limit: widget.Limit, Timeout: widget.Timeout, AllowInsecure: widget.AllowInsecure}
}
func (widget *latestMediaWidget) applyMediaConfig(config mediaServerConfig) {
	widget.Service = config.Service
	widget.Server = config.Server
	widget.Limit = config.Limit
}
func (widget *latestMediaWidget) update(ctx context.Context) {
	items, err := fetchLatestMedia(ctx, widget)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}
	widget.Items = items
}
func (widget *latestMediaWidget) Render() template.HTML {
	return widget.renderTemplate(widget, latestMediaWidgetTemplate)
}

func fetchLatestMedia(ctx context.Context, widget *latestMediaWidget) ([]mediaItem, error) {
	var items []mediaItem
	var err error
	switch widget.Service {
	case "plex":
		items, err = fetchPlexLatest(ctx, widget)
	case "jellyfin", "emby":
		items, err = fetchJellyfinLatest(ctx, widget)
	case "navidrome":
		items, err = fetchNavidromeLatest(ctx, widget)
	default:
		return nil, errors.New("unsupported media service")
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

func fetchPlexLatest(ctx context.Context, widget *latestMediaWidget) ([]mediaItem, error) {
	endpoint := widget.Server + "/library/recentlyAdded?X-Plex-Container-Start=0&X-Plex-Container-Size=" + strconv.Itoa(widget.Limit)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Plex-Token", widget.APIKey)
	request.Header.Set("Accept", "application/json")
	response, err := newHTTPClient(widget.Timeout, widget.AllowInsecure).Do(request)
	if err != nil {
		return nil, fmt.Errorf("requesting Plex latest media: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, unexpectedHTTPStatusError(response)
	}
	var payload plexMediaResponse
	if err = json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding Plex latest media: %w", err)
	}
	items := make([]mediaItem, 0, len(payload.MediaContainer.Metadata))
	for _, value := range payload.MediaContainer.Metadata {
		items = append(items, normalizePlexMedia(widget.Server, widget.APIKey, value, time.Unix(value.AddedAt, 0)))
	}
	return items, nil
}

func fetchJellyfinLatest(ctx context.Context, widget *latestMediaWidget) ([]mediaItem, error) {
	values := url.Values{"SortBy": {"DateCreated"}, "SortOrder": {"Descending"}, "Recursive": {"true"}, "Limit": {strconv.Itoa(widget.Limit)}, "Fields": {"Overview,DateCreated,ProductionYear,RunTimeTicks"}, "IncludeItemTypes": {"Movie,Series,Episode,Audio"}}
	endpoint := widget.Server + "/Items?" + values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Emby-Token", widget.APIKey)
	request.Header.Set("Accept", "application/json")
	response, err := newHTTPClient(widget.Timeout, widget.AllowInsecure).Do(request)
	if err != nil {
		return nil, fmt.Errorf("requesting %s latest media: %w", widget.Service, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, unexpectedHTTPStatusError(response)
	}
	var payload jellyfinItemsResponse
	if err = json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding %s latest media: %w", widget.Service, err)
	}
	items := make([]mediaItem, 0, len(payload.Items))
	for _, value := range payload.Items {
		created := parseMediaTime(value.DateCreated)
		items = append(items, normalizeJellyfinMedia(widget.Server, widget.APIKey, value, created))
	}
	return items, nil
}

func fetchNavidromeLatest(ctx context.Context, widget *latestMediaWidget) ([]mediaItem, error) {
	values := navidromeAuthValues(widget.Username, widget.Password)
	values.Set("type", "newest")
	values.Set("size", strconv.Itoa(widget.Limit))
	endpoint := widget.Server + "/rest/getAlbumList2.view?" + values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := newHTTPClient(widget.Timeout, widget.AllowInsecure).Do(request)
	if err != nil {
		return nil, fmt.Errorf("requesting Navidrome latest media: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, unexpectedHTTPStatusError(response)
	}
	var payload navidromeResponse
	if err = json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding Navidrome latest media: %w", err)
	}
	if payload.SubsonicResponse.Status != "ok" {
		if payload.SubsonicResponse.Error != nil {
			return nil, errors.New(payload.SubsonicResponse.Error.Message)
		}
		return nil, errors.New("Navidrome returned a failed response")
	}
	items := make([]mediaItem, 0, len(payload.SubsonicResponse.AlbumList2.Album))
	for _, value := range payload.SubsonicResponse.AlbumList2.Album {
		created := parseMediaTime(value.Created)
		image := ""
		if value.CoverArt != "" {
			image = widget.Server + "/rest/getCoverArt.view?" + navidromeCoverValues(widget, value.CoverArt).Encode()
		}
		items = append(items, mediaItem{Title: value.Name, Subtitle: mediaSubtitle(value.Artist, yearText(value.Year)), MediaType: "Album", Date: formatMediaDate(created), ImageURL: image, Duration: formatMediaDuration(time.Duration(value.Duration) * time.Second), SortTime: created})
	}
	return items, nil
}

func navidromeCoverValues(widget *latestMediaWidget, id string) url.Values {
	values := navidromeAuthValues(widget.Username, widget.Password)
	values.Set("id", id)
	return values
}
func normalizePlexMedia(server, token string, value plexMediaItem, date time.Time) mediaItem {
	title := value.Title
	subtitle := yearText(value.Year)
	if value.Type == "episode" {
		title = value.GrandparentTitle
		if title == "" {
			title = value.Title
		}
		subtitle = mediaSubtitle(fmt.Sprintf("S%02dE%02d", value.ParentIndex, value.Index), value.Title)
	}
	image := ""
	if value.Thumb != "" {
		image = server + value.Thumb + "?X-Plex-Token=" + url.QueryEscape(token)
	}
	return mediaItem{Title: title, Subtitle: subtitle, MediaType: mediaTypeLabel(value.Type), Date: formatMediaDate(date), Summary: value.Summary, ImageURL: image, URL: server + "/web/index.html#!/server/", Duration: formatMediaDuration(time.Duration(value.Duration) * time.Millisecond), SortTime: date}
}
func normalizeJellyfinMedia(server, apiKey string, value jellyfinMediaItem, date time.Time) mediaItem {
	title := value.Name
	subtitle := yearText(value.ProductionYear)
	if strings.EqualFold(value.Type, "Episode") {
		title = value.SeriesName
		if title == "" {
			title = value.Name
		}
		subtitle = mediaSubtitle(fmt.Sprintf("S%02dE%02d", value.ParentIndexNumber, value.IndexNumber), value.Name)
	}
	image := ""
	if value.ID != "" {
		image = server + "/Items/" + url.PathEscape(value.ID) + "/Images/Primary?api_key=" + url.QueryEscape(apiKey)
	}
	return mediaItem{Title: title, Subtitle: subtitle, MediaType: mediaTypeLabel(value.Type), Date: formatMediaDate(date), Summary: value.Overview, ImageURL: image, Duration: formatMediaDuration(time.Duration(value.RunTimeTicks) * 100 * time.Nanosecond), SortTime: date}
}
func parseMediaTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.9999999"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
