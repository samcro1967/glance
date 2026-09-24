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

var arrWidgetTemplate = mustParseTemplate("arr.html", "widget-base.html")

type arrWidget struct {
	widgetBase `yaml:",inline"`

	Service       string        `yaml:"service"`
	Server        string        `yaml:"server"`
	APIKey        string        `yaml:"api-key"`
	View          string        `yaml:"view"`
	Days          int           `yaml:"days"`
	Limit         int           `yaml:"limit"`
	CollapseAfter int           `yaml:"collapse-after"`
	Timeout       durationField `yaml:"timeout"`
	AllowInsecure bool          `yaml:"allow-insecure"`

	Items []arrItem `yaml:"-"`
}

type arrItem struct {
	Title     string
	Subtitle  string
	Status    string
	Date      string
	Dates     []arrDate
	Summary   string
	ImageURL  string
	Monitored bool
	HasFile   bool
}

type arrDate struct {
	Label string
	Value string
}

type arrImage struct {
	CoverType string `json:"coverType"`
	RemoteURL string `json:"remoteUrl"`
	URL       string `json:"url"`
}

type arrMovie struct {
	ID              int        `json:"id"`
	Title           string     `json:"title"`
	Year            int        `json:"year"`
	Overview        string     `json:"overview"`
	Monitored       bool       `json:"monitored"`
	HasFile         bool       `json:"hasFile"`
	InCinemas       time.Time  `json:"inCinemas"`
	DigitalRelease  time.Time  `json:"digitalRelease"`
	PhysicalRelease time.Time  `json:"physicalRelease"`
	Images          []arrImage `json:"images"`
}

type arrSeries struct {
	ID        int        `json:"id"`
	Title     string     `json:"title"`
	Year      int        `json:"year"`
	Overview  string     `json:"overview"`
	Monitored bool       `json:"monitored"`
	Images    []arrImage `json:"images"`
}

type arrEpisode struct {
	ID            int       `json:"id"`
	Title         string    `json:"title"`
	SeasonNumber  int       `json:"seasonNumber"`
	EpisodeNumber int       `json:"episodeNumber"`
	AirDateUTC    time.Time `json:"airDateUtc"`
	HasFile       bool      `json:"hasFile"`
	Monitored     bool      `json:"monitored"`
	Series        arrSeries `json:"series"`
}

type arrArtist struct {
	ID         int        `json:"id"`
	ArtistName string     `json:"artistName"`
	Monitored  bool       `json:"monitored"`
	Images     []arrImage `json:"images"`
}

type arrAlbum struct {
	ID          int        `json:"id"`
	Title       string     `json:"title"`
	ReleaseDate time.Time  `json:"releaseDate"`
	Monitored   bool       `json:"monitored"`
	Artist      arrArtist  `json:"artist"`
	Images      []arrImage `json:"images"`
}

type arrHistoryPage struct {
	Records []json.RawMessage `json:"records"`
}

type arrWantedPage struct {
	Records []json.RawMessage `json:"records"`
}

type arrHistoryRecord struct {
	Date      time.Time   `json:"date"`
	EventType string      `json:"eventType"`
	Movie     *arrMovie   `json:"movie"`
	Series    *arrSeries  `json:"series"`
	Episode   *arrEpisode `json:"episode"`
	Artist    *arrArtist  `json:"artist"`
	Album     *arrAlbum   `json:"album"`
}

func (widget *arrWidget) initialize() error {
	widget.withTitle("ARR").withCacheDuration(5 * time.Minute)
	widget.Service = strings.ToLower(strings.TrimSpace(widget.Service))
	switch widget.Service {
	case "radarr", "sonarr", "lidarr":
	default:
		return errors.New("service must be radarr, sonarr, or lidarr")
	}
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
		widget.View = "upcoming"
	}
	switch widget.View {
	case "upcoming", "recent", "missing":
	default:
		return errors.New("view must be upcoming, recent, or missing")
	}
	if widget.Days < 0 || widget.Limit < 0 || widget.CollapseAfter < 0 {
		return errors.New("days, limit, and collapse-after must not be negative")
	}
	if widget.Days == 0 {
		widget.Days = 14
	}
	if widget.Limit == 0 {
		widget.Limit = 10
	}
	return nil
}

func (widget *arrWidget) update(ctx context.Context) {
	items, err := fetchARR(ctx, widget)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}
	widget.Items = items
}

func (widget *arrWidget) Render() template.HTML {
	return widget.renderTemplate(widget, arrWidgetTemplate)
}

func fetchARR(ctx context.Context, widget *arrWidget) ([]arrItem, error) {
	client := newHTTPClient(widget.Timeout, widget.AllowInsecure)
	endpoint, err := arrEndpoint(widget, time.Now())
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating %s request: %w", widget.Service, err)
	}
	request.Header.Set("X-Api-Key", widget.APIKey)
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("requesting %s %s: %w", widget.Service, widget.View, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, unexpectedHTTPStatusError(response)
	}

	items, err := decodeARRResponse(response, widget.Service, widget.View)
	if err != nil {
		return nil, fmt.Errorf("decoding %s %s: %w", widget.Service, widget.View, err)
	}
	for i := range items {
		items[i].ImageURL = widget.proxyARRImage(items[i].ImageURL)
	}
	if len(items) > widget.Limit {
		items = items[:widget.Limit]
	}
	return items, nil
}

func arrEndpoint(widget *arrWidget, now time.Time) (string, error) {
	version := "v3"
	if widget.Service == "lidarr" {
		version = "v1"
	}
	endpoint := widget.Server + "/api/" + version + "/"
	values := url.Values{}
	switch widget.View {
	case "upcoming":
		endpoint += "calendar"
		values.Set("start", now.Format("2006-01-02"))
		values.Set("end", now.AddDate(0, 0, widget.Days).Format("2006-01-02"))
		if widget.Service == "sonarr" {
			values.Set("includeSeries", "true")
		}
	case "recent":
		endpoint += "history"
		values.Set("page", "1")
		values.Set("pageSize", strconv.Itoa(widget.Limit))
		values.Set("sortKey", "date")
		values.Set("sortDirection", "descending")
		if widget.Service == "sonarr" {
			values.Set("includeSeries", "true")
			values.Set("includeEpisode", "true")
		}
	case "missing":
		endpoint += "wanted/missing"
		values.Set("page", "1")
		values.Set("pageSize", strconv.Itoa(widget.Limit))
		values.Set("sortDirection", "ascending")
		if widget.Service == "sonarr" {
			values.Set("includeSeries", "true")
		}
	default:
		return "", errors.New("unsupported ARR view")
	}
	return endpoint + "?" + values.Encode(), nil
}

func decodeARRResponse(response *http.Response, service, view string) ([]arrItem, error) {
	if view == "upcoming" {
		switch service {
		case "radarr":
			var values []arrMovie
			if err := json.NewDecoder(response.Body).Decode(&values); err != nil {
				return nil, err
			}
			items := make([]arrItem, 0, len(values))
			for _, value := range values {
				items = append(items, normalizeARRMovie(value, arrMovieDate(value)))
			}
			sortARRItems(items)
			return items, nil
		case "sonarr":
			var values []arrEpisode
			if err := json.NewDecoder(response.Body).Decode(&values); err != nil {
				return nil, err
			}
			items := make([]arrItem, 0, len(values))
			for _, value := range values {
				items = append(items, normalizeARREpisode(value, value.AirDateUTC))
			}
			sortARRItems(items)
			return items, nil
		case "lidarr":
			var values []arrAlbum
			if err := json.NewDecoder(response.Body).Decode(&values); err != nil {
				return nil, err
			}
			items := make([]arrItem, 0, len(values))
			for _, value := range values {
				items = append(items, normalizeARRAlbum(value, value.ReleaseDate))
			}
			sortARRItems(items)
			return items, nil
		}
	}
	if view == "recent" {
		var page arrHistoryPage
		if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
			return nil, err
		}
		items := make([]arrItem, 0, len(page.Records))
		for _, raw := range page.Records {
			var record arrHistoryRecord
			if err := json.Unmarshal(raw, &record); err != nil {
				return nil, err
			}
			if item, ok := normalizeARRHistory(service, record); ok {
				items = append(items, item)
			}
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].Date > items[j].Date })
		return items, nil
	}
	var page arrWantedPage
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		return nil, err
	}
	items := make([]arrItem, 0, len(page.Records))
	for _, raw := range page.Records {
		switch service {
		case "radarr":
			var value arrMovie
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, err
			}
			items = append(items, normalizeARRMovie(value, arrMovieDate(value)))
		case "sonarr":
			var value arrEpisode
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, err
			}
			items = append(items, normalizeARREpisode(value, value.AirDateUTC))
		case "lidarr":
			var value arrAlbum
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, err
			}
			items = append(items, normalizeARRAlbum(value, value.ReleaseDate))
		}
	}
	sortARRItems(items)
	return items, nil
}

func normalizeARRHistory(service string, record arrHistoryRecord) (arrItem, bool) {
	switch service {
	case "radarr":
		if record.Movie != nil {
			item := normalizeARRMovie(*record.Movie, record.Date)
			item.Status = arrEventLabel(record.EventType)
			return item, true
		}
	case "sonarr":
		if record.Episode != nil {
			item := normalizeARREpisode(*record.Episode, record.Date)
			if record.Series != nil && item.Subtitle == "" {
				item.Subtitle = record.Series.Title
			}
			item.Status = arrEventLabel(record.EventType)
			return item, true
		}
	case "lidarr":
		if record.Album != nil {
			item := normalizeARRAlbum(*record.Album, record.Date)
			item.Status = arrEventLabel(record.EventType)
			return item, true
		}
	}
	return arrItem{}, false
}

func normalizeARRMovie(value arrMovie, date time.Time) arrItem {
	return arrItem{Title: value.Title, Subtitle: yearText(value.Year), Status: arrAvailability(value.Monitored, value.HasFile), Date: formatARRDate(date), Dates: arrMovieDates(value), Summary: value.Overview, ImageURL: arrPoster(value.Images), Monitored: value.Monitored, HasFile: value.HasFile}
}

func arrMovieDates(value arrMovie) []arrDate {
	dates := make([]arrDate, 0, 3)
	if !value.InCinemas.IsZero() {
		dates = append(dates, arrDate{Label: "Cinema", Value: formatARRDate(value.InCinemas)})
	}
	if !value.DigitalRelease.IsZero() {
		dates = append(dates, arrDate{Label: "Digital", Value: formatARRDate(value.DigitalRelease)})
	}
	if !value.PhysicalRelease.IsZero() {
		dates = append(dates, arrDate{Label: "Physical", Value: formatARRDate(value.PhysicalRelease)})
	}
	return dates
}
func normalizeARREpisode(value arrEpisode, date time.Time) arrItem {
	title := value.Series.Title
	if title == "" {
		title = value.Title
	}

	subtitle := fmt.Sprintf("S%02dE%02d", value.SeasonNumber, value.EpisodeNumber)
	if value.Title != "" && value.Title != title {
		subtitle += " · " + value.Title
	}

	return arrItem{Title: title, Subtitle: subtitle, Status: arrAvailability(value.Monitored, value.HasFile), Date: formatARRDate(date), Summary: value.Series.Overview, ImageURL: arrPoster(value.Series.Images), Monitored: value.Monitored, HasFile: value.HasFile}
}
func normalizeARRAlbum(value arrAlbum, date time.Time) arrItem {
	return arrItem{Title: value.Title, Subtitle: value.Artist.ArtistName, Status: arrAvailability(value.Monitored, false), Date: formatARRDate(date), ImageURL: arrPoster(append(value.Images, value.Artist.Images...)), Monitored: value.Monitored}
}
func arrMovieDate(value arrMovie) time.Time {
	if !value.DigitalRelease.IsZero() {
		return value.DigitalRelease
	}
	if !value.PhysicalRelease.IsZero() {
		return value.PhysicalRelease
	}
	return value.InCinemas
}
func arrPoster(images []arrImage) string {
	for _, image := range images {
		if image.CoverType == "poster" && image.RemoteURL != "" {
			return image.RemoteURL
		}
	}
	for _, image := range images {
		if image.RemoteURL != "" {
			return image.RemoteURL
		}
	}
	return ""
}
func (widget *arrWidget) proxyARRImage(raw string) string {
	if raw == "" || widget.Providers == nil || widget.Providers.resourceProxyURL == nil {
		return ""
	}
	proxied, err := widget.Providers.resourceProxyURL(raw)
	if err != nil {
		return ""
	}
	return proxied
}
func arrAvailability(monitored, hasFile bool) string {
	if hasFile {
		return "Available"
	}
	if !monitored {
		return "Unmonitored"
	}
	return "Missing"
}
func arrEventLabel(value string) string {
	if value == "" {
		return "Recent"
	}
	value = strings.ReplaceAll(value, "_", " ")
	if value == "" {
		return "Recent"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
func formatARRDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format("Jan 2, 2006")
}
func yearText(year int) string {
	if year <= 0 {
		return ""
	}
	return strconv.Itoa(year)
}
func sortARRItems(items []arrItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Date != items[j].Date {
			return items[i].Date < items[j].Date
		}
		return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
	})
}
