package glance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var seerrWidgetTemplate = mustParseTemplate("seerr.html", "widget-base.html")

type seerrWidget struct {
	widgetBase `yaml:",inline"`

	Server        string        `yaml:"server"`
	APIKey        string        `yaml:"api-key"`
	View          string        `yaml:"view"`
	Limit         int           `yaml:"limit"`
	CollapseAfter int           `yaml:"collapse-after"`
	Timeout       durationField `yaml:"timeout"`
	AllowInsecure bool          `yaml:"allow-insecure"`

	Items []seerrItem `yaml:"-"`
}

type seerrItem struct {
	Title     string
	Subtitle  string
	Status    string
	Date      string
	Summary   string
	ImageURL  string
	TMDBID    int
	MediaType string
}

type seerrMediaResult struct {
	ID           int             `json:"id"`
	MediaType    string          `json:"mediaType"`
	Title        string          `json:"title"`
	Name         string          `json:"name"`
	Overview     string          `json:"overview"`
	PosterPath   string          `json:"posterPath"`
	ReleaseDate  string          `json:"releaseDate"`
	FirstAirDate string          `json:"firstAirDate"`
	MediaInfo    *seerrMediaInfo `json:"mediaInfo"`
}

type seerrMediaInfo struct {
	ID        int    `json:"id"`
	TMDBID    int    `json:"tmdbId"`
	MediaType string `json:"mediaType"`
	Status    int    `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type seerrResultPage struct {
	Results []seerrMediaResult `json:"results"`
}

type seerrRequestPage struct {
	Results []seerrRequest `json:"results"`
}

type seerrRequest struct {
	ID          int             `json:"id"`
	Status      int             `json:"status"`
	Type        string          `json:"type"`
	CreatedAt   string          `json:"createdAt"`
	UpdatedAt   string          `json:"updatedAt"`
	Media       seerrMediaInfo  `json:"media"`
	RequestedBy *seerrRequester `json:"requestedBy"`
}

type seerrRequester struct {
	Username     string `json:"username"`
	PlexUsername string `json:"plexUsername"`
}

type seerrMediaPage struct {
	Results []seerrMediaInfo `json:"results"`
}

func (widget *seerrWidget) initialize() error {
	widget.withTitle("Seerr").withCacheDuration(5 * time.Minute)

	parsed, err := url.Parse(strings.TrimSpace(widget.Server))
	if err != nil {
		return fmt.Errorf("parsing server URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("server must be an absolute HTTP or HTTPS URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("server URL must not contain a query string or fragment")
	}
	if strings.TrimSpace(widget.APIKey) == "" {
		return errors.New("api-key is required")
	}

	widget.Server = strings.TrimRight(strings.TrimSpace(widget.Server), "/")
	widget.View = strings.ToLower(strings.TrimSpace(widget.View))
	if widget.View == "" {
		widget.View = "trending"
	}
	switch widget.View {
	case "trending", "movies", "tv", "upcoming-movies", "upcoming-tv", "requests", "recently-added", "watchlist":
	default:
		return errors.New("view must be trending, movies, tv, upcoming-movies, upcoming-tv, requests, recently-added, or watchlist")
	}
	if widget.Limit < 0 || widget.CollapseAfter < 0 {
		return errors.New("limit and collapse-after must not be negative")
	}
	if widget.Limit == 0 {
		widget.Limit = 10
	}
	return nil
}

func (widget *seerrWidget) update(ctx context.Context) {
	items, err := fetchSeerr(ctx, widget)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}
	widget.Items = items
}

func (widget *seerrWidget) Render() template.HTML {
	return widget.renderTemplate(widget, seerrWidgetTemplate)
}

func fetchSeerr(ctx context.Context, widget *seerrWidget) ([]seerrItem, error) {
	endpoint, err := seerrEndpoint(widget)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Seerr %s request: %w", widget.View, err)
	}
	request.Header.Set("X-Api-Key", widget.APIKey)
	request.Header.Set("Accept", "application/json")

	response, err := newHTTPClient(widget.Timeout, widget.AllowInsecure).Do(request)
	if err != nil {
		return nil, fmt.Errorf("requesting Seerr %s: %w", widget.View, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, unexpectedHTTPStatusError(response)
	}

	items, err := decodeSeerrResponse(response, widget.View)
	if err != nil {
		return nil, fmt.Errorf("decoding Seerr %s: %w", widget.View, err)
	}
	if widget.View == "requests" || widget.View == "recently-added" {
		for i := range items {
			if items[i].TMDBID == 0 || items[i].MediaType == "" {
				continue
			}
			detail, detailErr := fetchSeerrDetail(ctx, widget, items[i].MediaType, items[i].TMDBID)
			if detailErr != nil {
				return nil, detailErr
			}
			items[i].Title = detail.Title
			items[i].Summary = detail.Summary
			items[i].ImageURL = detail.ImageURL
			if widget.View == "recently-added" {
				items[i].Subtitle = detail.Subtitle
			}
		}
	}
	for i := range items {
		items[i].ImageURL = widget.proxySeerrImage(items[i].ImageURL)
	}
	if len(items) > widget.Limit {
		items = items[:widget.Limit]
	}
	return items, nil
}

func fetchSeerrDetail(ctx context.Context, widget *seerrWidget, mediaType string, tmdbID int) (seerrItem, error) {
	endpoint := fmt.Sprintf("%s/api/v1/%s/%d", widget.Server, mediaType, tmdbID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return seerrItem{}, fmt.Errorf("creating Seerr %s detail request: %w", mediaType, err)
	}
	request.Header.Set("X-Api-Key", widget.APIKey)
	request.Header.Set("Accept", "application/json")
	response, err := newHTTPClient(widget.Timeout, widget.AllowInsecure).Do(request)
	if err != nil {
		return seerrItem{}, fmt.Errorf("requesting Seerr %s detail: %w", mediaType, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return seerrItem{}, unexpectedHTTPStatusError(response)
	}
	var value seerrMediaResult
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return seerrItem{}, fmt.Errorf("decoding Seerr %s detail: %w", mediaType, err)
	}
	if value.MediaType == "" {
		value.MediaType = mediaType
	}
	return normalizeSeerrResult(value), nil
}

func seerrEndpoint(widget *seerrWidget) (string, error) {
	endpoint := widget.Server + "/api/v1/"
	values := url.Values{}
	switch widget.View {
	case "trending":
		endpoint += "discover/trending"
		values.Set("page", "1")
	case "movies":
		endpoint += "discover/movies"
		values.Set("page", "1")
	case "tv":
		endpoint += "discover/tv"
		values.Set("page", "1")
	case "upcoming-movies":
		endpoint += "discover/movies/upcoming"
		values.Set("page", "1")
	case "upcoming-tv":
		endpoint += "discover/tv/upcoming"
		values.Set("page", "1")
	case "requests":
		endpoint += "request"
		values.Set("filter", "all")
		values.Set("take", strconv.Itoa(widget.Limit))
		values.Set("skip", "0")
		values.Set("sort", "added")
	case "recently-added":
		endpoint += "media"
		values.Set("filter", "allavailable")
		values.Set("take", strconv.Itoa(widget.Limit))
		values.Set("skip", "0")
		values.Set("sort", "mediaAdded")
	case "watchlist":
		endpoint += "discover/watchlist"
	default:
		return "", errors.New("unsupported Seerr view")
	}
	if len(values) == 0 {
		return endpoint, nil
	}
	return endpoint + "?" + values.Encode(), nil
}

func decodeSeerrResponse(response *http.Response, view string) ([]seerrItem, error) {
	switch view {
	case "requests":
		var page seerrRequestPage
		if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
			return nil, err
		}
		items := make([]seerrItem, 0, len(page.Results))
		for _, value := range page.Results {
			items = append(items, normalizeSeerrRequest(value))
		}
		return items, nil
	case "recently-added":
		var page seerrMediaPage
		if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
			return nil, err
		}
		items := make([]seerrItem, 0, len(page.Results))
		for _, value := range page.Results {
			items = append(items, normalizeSeerrMediaInfo(value))
		}
		return items, nil
	default:
		var page seerrResultPage
		if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
			return nil, err
		}
		items := make([]seerrItem, 0, len(page.Results))
		for _, value := range page.Results {
			if value.MediaType != "" && value.MediaType != "movie" && value.MediaType != "tv" {
				continue
			}
			items = append(items, normalizeSeerrResult(value))
		}
		return items, nil
	}
}

func normalizeSeerrResult(value seerrMediaResult) seerrItem {
	title := value.Title
	date := value.ReleaseDate
	if title == "" {
		title = value.Name
	}
	if date == "" {
		date = value.FirstAirDate
	}
	status := ""
	if value.MediaInfo != nil {
		status = seerrMediaStatusLabel(value.MediaInfo.Status)
	}
	return seerrItem{
		Title: title, Subtitle: seerrYear(date), Status: status, Date: seerrFormatDate(date), Summary: value.Overview,
		ImageURL: seerrPosterURL(value.PosterPath), TMDBID: value.ID, MediaType: value.MediaType,
	}
}

func normalizeSeerrRequest(value seerrRequest) seerrItem {
	requester := ""
	if value.RequestedBy != nil {
		requester = value.RequestedBy.Username
		if requester == "" {
			requester = value.RequestedBy.PlexUsername
		}
	}
	mediaType := seerrMediaTypeLabel(value.Type)
	subtitle := mediaType
	if requester != "" {
		if subtitle != "" {
			subtitle += " · "
		}
		subtitle += "Requested by " + requester
	}
	return seerrItem{Title: "Request #" + strconv.Itoa(value.ID), Subtitle: subtitle, Status: seerrRequestStatusLabel(value.Status), Date: seerrFormatDate(value.CreatedAt), TMDBID: value.Media.TMDBID, MediaType: value.Media.MediaType}
}

func normalizeSeerrMediaInfo(value seerrMediaInfo) seerrItem {
	return seerrItem{Title: seerrMediaTypeLabel(value.MediaType), Status: seerrMediaStatusLabel(value.Status), Date: seerrFormatDate(value.UpdatedAt), TMDBID: value.TMDBID, MediaType: value.MediaType}
}

func seerrPosterURL(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "https://image.tmdb.org/t/p/w500" + path
}

func (widget *seerrWidget) proxySeerrImage(raw string) string {
	if raw == "" || widget.Providers == nil || widget.Providers.resourceProxyURL == nil {
		return ""
	}
	proxied, err := widget.Providers.resourceProxyURL(raw)
	if err != nil {
		return ""
	}
	return proxied
}

func seerrMediaTypeLabel(value string) string {
	switch strings.ToLower(value) {
	case "movie":
		return "Movie"
	case "tv":
		return "TV"
	default:
		return value
	}
}

func seerrMediaStatusLabel(value int) string {
	switch value {
	case 2:
		return "Pending"
	case 3:
		return "Processing"
	case 4:
		return "Partially Available"
	case 5:
		return "Available"
	default:
		return ""
	}
}

func seerrRequestStatusLabel(value int) string {
	switch value {
	case 1:
		return "Pending"
	case 2:
		return "Approved"
	case 3:
		return "Declined"
	default:
		return ""
	}
}

func seerrYear(value string) string {
	if len(value) >= 4 {
		return value[:4]
	}
	return ""
}

func seerrFormatDate(value string) string {
	if value == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.Format("Jan 2, 2006")
		}
	}
	return value
}
