package glance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"time"
)

var torrentingWidgetTemplate = mustParseTemplate("torrenting.html", "widget-base.html")

type torrentingWidget struct {
	widgetBase `yaml:",inline"`

	Server        string        `yaml:"server"`
	APIKey        string        `yaml:"api-key"`
	Username      string        `yaml:"username"`
	Password      string        `yaml:"password"`
	HideCompleted bool          `yaml:"hide-completed"`
	HideInactive  bool          `yaml:"hide-inactive"`
	HideProgress  bool          `yaml:"hide-progress"`
	CollapseAfter int           `yaml:"collapse-after"`
	Timeout       durationField `yaml:"timeout"`
	AllowInsecure bool          `yaml:"allow-insecure"`

	client   *http.Client    `yaml:"-"`
	Endpoint string          `yaml:"-"`
	Torrents []torrentRecord `yaml:"-"`
}

type torrentRecord struct {
	Name         string
	State        string
	StateLabel   string
	Progress     float64
	ProgressCSS  string
	ProgressText string
	Downloaded   string
	Size         string
	ETA          string
	Completed    bool
	Active       bool
}

type qBittorrentTorrent struct {
	Name       string  `json:"name"`
	State      string  `json:"state"`
	Progress   float64 `json:"progress"`
	Downloaded int64   `json:"downloaded"`
	Size       int64   `json:"size"`
	ETA        int64   `json:"eta"`
}

func (widget *torrentingWidget) initialize() error {
	widget.withTitle("Torrents").withCacheDuration(30 * time.Second)

	server := strings.TrimSpace(widget.Server)
	if server == "" {
		return errors.New("server is required")
	}

	parsed, err := url.Parse(server)
	if err != nil {
		return fmt.Errorf("parsing server URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("server must be an absolute HTTP or HTTPS URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("server URL must not contain a query string or fragment")
	}
	if (widget.Username == "") != (widget.Password == "") {
		return errors.New("username and password must be configured together")
	}
	if widget.CollapseAfter < 0 {
		return errors.New("collapse-after must not be negative")
	}

	widget.Server = strings.TrimRight(server, "/")
	widget.Endpoint = widget.Server + "/api/v2/torrents/info"

	client, err := newQBittorrentClient(widget.Timeout, widget.AllowInsecure)
	if err != nil {
		return err
	}
	widget.client = client
	return nil
}

func (widget *torrentingWidget) update(ctx context.Context) {
	torrents, err := fetchQBittorrent(ctx, widget.client, qBittorrentRequestOptions{
		Server:   widget.Server,
		Endpoint: widget.Endpoint,
		APIKey:   widget.APIKey,
		Username: widget.Username,
		Password: widget.Password,
	})
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	records := make([]torrentRecord, 0, len(torrents))
	for _, torrent := range torrents {
		record := normalizeQBittorrentTorrent(torrent)
		if widget.HideCompleted && record.Completed {
			continue
		}
		if widget.HideInactive && !record.Active {
			continue
		}
		records = append(records, record)
	}

	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Active != records[j].Active {
			return records[i].Active
		}
		if records[i].Completed != records[j].Completed {
			return !records[i].Completed
		}
		return strings.ToLower(records[i].Name) < strings.ToLower(records[j].Name)
	})
	widget.Torrents = records
}

func (widget *torrentingWidget) Render() template.HTML {
	return widget.renderTemplate(widget, torrentingWidgetTemplate)
}

type qBittorrentRequestOptions struct {
	Server   string
	Endpoint string
	APIKey   string
	Username string
	Password string
}

func newQBittorrentClient(timeout durationField, allowInsecure bool) (*http.Client, error) {
	client := newHTTPClient(timeout, allowInsecure)
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating qBittorrent cookie jar: %w", err)
	}
	client.Jar = jar
	return client, nil
}

func fetchQBittorrent(ctx context.Context, client *http.Client, options qBittorrentRequestOptions) ([]qBittorrentTorrent, error) {
	torrents, err := fetchQBittorrentOnce(ctx, client, options.Endpoint, options.APIKey)
	if err == nil || options.APIKey != "" || options.Username == "" || !isQBittorrentAuthError(err) {
		return torrents, err
	}

	if err := loginQBittorrent(ctx, client, options.Server, options.Username, options.Password); err != nil {
		return nil, err
	}
	return fetchQBittorrentOnce(ctx, client, options.Endpoint, "")
}

func fetchQBittorrentOnce(ctx context.Context, client *http.Client, endpoint, apiKey string) ([]qBittorrentTorrent, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating qBittorrent request: %w", err)
	}
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("requesting qBittorrent torrents: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, unexpectedHTTPStatusError(response)
	}

	var torrents []qBittorrentTorrent
	if err := json.NewDecoder(response.Body).Decode(&torrents); err != nil {
		return nil, fmt.Errorf("decoding qBittorrent torrents: %w", err)
	}
	return torrents, nil
}

func loginQBittorrent(ctx context.Context, client *http.Client, server, username, password string) error {
	form := url.Values{"username": {username}, "password": {password}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("creating qBittorrent login request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Referer", server+"/")

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("logging in to qBittorrent: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		return unexpectedHTTPStatusError(response)
	}
	if response.StatusCode == http.StatusOK {
		body, err := readHTTPResponseBody(response.Body, 1024)
		if err != nil {
			return fmt.Errorf("reading qBittorrent login response: %w", err)
		}
		if strings.EqualFold(strings.TrimSpace(string(body)), "Fails.") {
			return errors.New("qBittorrent login rejected credentials")
		}
	}
	return nil
}

func isQBittorrentAuthError(err error) bool {
	var statusErr *httpStatusError
	return errors.As(err, &statusErr) && (statusErr.StatusCode == http.StatusUnauthorized || statusErr.StatusCode == http.StatusForbidden)
}

func normalizeQBittorrentTorrent(raw qBittorrentTorrent) torrentRecord {
	progress := min(max(raw.Progress, 0), 1)
	completed := progress >= 1
	active := qbittorrentStateActive(raw.State)
	return torrentRecord{
		Name:         raw.Name,
		State:        raw.State,
		StateLabel:   qbittorrentStateLabel(raw.State),
		Progress:     progress,
		ProgressCSS:  fmt.Sprintf("%.1f%%", progress*100),
		ProgressText: fmt.Sprintf("%.0f%%", progress*100),
		Downloaded:   formatTorrentBytes(raw.Downloaded),
		Size:         formatTorrentBytes(raw.Size),
		ETA:          formatTorrentETA(raw.ETA),
		Completed:    completed,
		Active:       active,
	}
}

func qbittorrentStateActive(state string) bool {
	switch state {
	case "downloading", "forcedDL", "metaDL", "uploading", "forcedUP":
		return true
	default:
		return false
	}
}

func qbittorrentStateLabel(state string) string {
	switch state {
	case "downloading", "forcedDL", "metaDL":
		return "Downloading"
	case "uploading", "forcedUP", "stalledUP":
		return "Seeding"
	case "pausedDL", "pausedUP", "stoppedDL", "stoppedUP":
		return "Paused"
	case "queuedDL", "queuedUP":
		return "Queued"
	case "checkingDL", "checkingUP", "checkingResumeData":
		return "Checking"
	case "error", "missingFiles":
		return "Error"
	default:
		return state
	}
}

func formatTorrentBytes(value int64) string {
	if value < 0 {
		value = 0
	}
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := int64(unit), 0
	for n := value / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}

func formatTorrentETA(seconds int64) string {
	if seconds < 0 || seconds >= 8640000 {
		return "∞"
	}
	if seconds == 0 {
		return "0m"
	}
	duration := time.Duration(seconds) * time.Second
	if duration >= time.Hour {
		return fmt.Sprintf("%dh %dm", int(duration/time.Hour), int(duration%time.Hour/time.Minute))
	}
	if duration >= time.Minute {
		return fmt.Sprintf("%dm", int(duration/time.Minute))
	}
	return fmt.Sprintf("%ds", int(duration/time.Second))
}
