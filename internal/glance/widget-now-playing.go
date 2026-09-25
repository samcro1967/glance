package glance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var nowPlayingWidgetTemplate = mustParseTemplate("now-playing.html", "widget-base.html")

type nowPlayingWidget struct {
	widgetBase            `yaml:",inline"`
	Service               string           `yaml:"service"`
	Server                string           `yaml:"server"`
	APIKey                string           `yaml:"api-key"`
	Username              string           `yaml:"username"`
	Password              string           `yaml:"password"`
	Limit                 int              `yaml:"limit"`
	ShowPaused            bool             `yaml:"show-paused"`
	ShowThumbnail         bool             `yaml:"show-thumbnail"`
	ShowProgressBar       bool             `yaml:"show-progress-bar"`
	ShowProgressInfo      *bool            `yaml:"show-progress-info"`
	Timeout               durationField    `yaml:"timeout"`
	AllowInsecure         bool             `yaml:"allow-insecure"`
	Items                 []nowPlayingItem `yaml:"-"`
	ShowProgressInfoValue bool             `yaml:"-"`
}

type nowPlayingItem struct {
	Title        string
	Subtitle     string
	MediaType    string
	User         string
	Client       string
	Device       string
	ImageURL     string
	State        string
	PlayMethod   string
	Position     time.Duration
	Duration     time.Duration
	Progress     float64
	ProgressCSS  string
	ProgressText string
}

type plexSessionsResponse struct {
	MediaContainer struct {
		Metadata []plexSession `json:"Metadata"`
	} `json:"MediaContainer"`
}

type plexSession struct {
	Type             string `json:"type"`
	Title            string `json:"title"`
	ParentTitle      string `json:"parentTitle"`
	GrandparentTitle string `json:"grandparentTitle"`
	ParentIndex      int    `json:"parentIndex"`
	Index            int    `json:"index"`
	Year             int    `json:"year"`
	Thumb            string `json:"thumb"`
	ParentThumb      string `json:"parentThumb"`
	Duration         int64  `json:"duration"`
	ViewOffset       int64  `json:"viewOffset"`
	User             struct {
		Title string `json:"title"`
	} `json:"User"`
	Player struct {
		Title   string `json:"title"`
		Product string `json:"product"`
		State   string `json:"state"`
	} `json:"Player"`
	TranscodeSession *struct {
		VideoDecision string `json:"videoDecision"`
	} `json:"TranscodeSession"`
	Media []struct {
		Part []struct {
			Decision string `json:"decision"`
		} `json:"Part"`
	} `json:"Media"`
}

type jellyfinSession struct {
	UserName   string `json:"UserName"`
	Client     string `json:"Client"`
	DeviceName string `json:"DeviceName"`
	PlayState  struct {
		PositionTicks int64  `json:"PositionTicks"`
		IsPaused      bool   `json:"IsPaused"`
		PlayMethod    string `json:"PlayMethod"`
	} `json:"PlayState"`
	NowPlayingItem *struct {
		ID                string `json:"Id"`
		Name              string `json:"Name"`
		Type              string `json:"Type"`
		SeriesName        string `json:"SeriesName"`
		ParentIndexNumber int    `json:"ParentIndexNumber"`
		IndexNumber       int    `json:"IndexNumber"`
		ProductionYear    int    `json:"ProductionYear"`
		RunTimeTicks      int64  `json:"RunTimeTicks"`
		Album             string `json:"Album"`
		AlbumArtist       string `json:"AlbumArtist"`
	} `json:"NowPlayingItem"`
}

type navidromeNowPlayingResponse struct {
	SubsonicResponse struct {
		Status string `json:"status"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
		NowPlaying struct {
			Entry []navidromeNowPlayingEntry `json:"entry"`
		} `json:"nowPlaying"`
	} `json:"subsonic-response"`
}

type navidromeNowPlayingEntry struct {
	Title      string `json:"title"`
	Album      string `json:"album"`
	Artist     string `json:"artist"`
	CoverArt   string `json:"coverArt"`
	Duration   int64  `json:"duration"`
	Username   string `json:"username"`
	PlayerName string `json:"playerName"`
	State      string `json:"state"`
	PositionMS int64  `json:"positionMs"`
}

func (widget *nowPlayingWidget) initialize() error {
	widget.withTitle("Now Playing").withCacheDuration(30 * time.Second)
	config := mediaServerConfig{Service: widget.Service, Server: widget.Server, APIKey: widget.APIKey, Username: widget.Username, Password: widget.Password, Limit: widget.Limit, Timeout: widget.Timeout, AllowInsecure: widget.AllowInsecure}
	if err := config.normalize(false); err != nil {
		return err
	}
	widget.Service, widget.Server, widget.Limit = config.Service, config.Server, config.Limit
	widget.ShowProgressInfoValue = widget.ShowProgressInfo == nil || *widget.ShowProgressInfo
	return nil
}

func (widget *nowPlayingWidget) update(ctx context.Context) {
	items, err := fetchNowPlaying(ctx, widget)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}
	widget.Items = items
}

func (widget *nowPlayingWidget) Render() template.HTML {
	return widget.renderTemplate(widget, nowPlayingWidgetTemplate)
}

func fetchNowPlaying(ctx context.Context, widget *nowPlayingWidget) ([]nowPlayingItem, error) {
	var items []nowPlayingItem
	var err error
	switch widget.Service {
	case "plex":
		items, err = fetchPlexNowPlaying(ctx, widget)
	case "jellyfin", "emby":
		items, err = fetchJellyfinNowPlaying(ctx, widget)
	case "navidrome":
		items, err = fetchNavidromeNowPlaying(ctx, widget)
	default:
		return nil, errors.New("unsupported media service")
	}
	if err != nil {
		return nil, err
	}
	filtered := items[:0]
	for i := range items {
		if !widget.ShowPaused && strings.EqualFold(items[i].State, "paused") {
			continue
		}
		items[i].Progress = mediaProgress(items[i].Position, items[i].Duration)
		items[i].ProgressCSS = fmt.Sprintf("%.2f", items[i].Progress)
		items[i].ProgressText = formatNowPlayingProgress(items[i].Position, items[i].Duration)
		items[i].ImageURL = proxyMediaImage(widget.Providers, items[i].ImageURL)
		filtered = append(filtered, items[i])
	}
	if len(filtered) > widget.Limit {
		filtered = filtered[:widget.Limit]
	}
	return filtered, nil
}

func fetchPlexNowPlaying(ctx context.Context, widget *nowPlayingWidget) ([]nowPlayingItem, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, widget.Server+"/status/sessions", nil)
	if err != nil {
		return nil, err
	}
	applyMediaServerAuth(request, widget.Service, widget.APIKey)
	request.Header.Set("Accept", "application/json")
	response, err := newHTTPClient(widget.Timeout, widget.AllowInsecure).Do(request)
	if err != nil {
		return nil, fmt.Errorf("requesting Plex now playing: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, unexpectedHTTPStatusError(response)
	}
	var payload plexSessionsResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding Plex now playing: %w", err)
	}
	items := make([]nowPlayingItem, 0, len(payload.MediaContainer.Metadata))
	for _, session := range payload.MediaContainer.Metadata {
		items = append(items, normalizePlexNowPlaying(widget, session))
	}
	return items, nil
}

func normalizePlexNowPlaying(widget *nowPlayingWidget, session plexSession) nowPlayingItem {
	title, subtitle := session.Title, yearText(session.Year)
	if session.Type == "episode" {
		title = session.GrandparentTitle
		if title == "" {
			title = session.Title
		}
		subtitle = mediaSubtitle(fmt.Sprintf("S%02dE%02d", session.ParentIndex, session.Index), session.Title)
	} else if session.Type == "track" {
		subtitle = mediaSubtitle(session.GrandparentTitle, session.ParentTitle)
	}
	thumb := session.Thumb
	if (session.Type == "episode" || session.Type == "track") && session.ParentThumb != "" {
		thumb = session.ParentThumb
	}
	image := ""
	if thumb != "" {
		image = widget.Server + thumb + "?X-Plex-Token=" + url.QueryEscape(widget.APIKey)
	}
	method := ""
	if session.TranscodeSession != nil {
		method = mediaPlayMethod(session.TranscodeSession.VideoDecision)
	}
	if method == "" && len(session.Media) > 0 && len(session.Media[0].Part) > 0 {
		method = mediaPlayMethod(session.Media[0].Part[0].Decision)
	}
	state := strings.ToLower(session.Player.State)
	if state == "" {
		state = "playing"
	}
	return nowPlayingItem{Title: title, Subtitle: subtitle, MediaType: mediaTypeLabel(session.Type), User: session.User.Title, Client: session.Player.Product, Device: session.Player.Title, ImageURL: image, State: state, PlayMethod: method, Position: time.Duration(session.ViewOffset) * time.Millisecond, Duration: time.Duration(session.Duration) * time.Millisecond}
}

func fetchJellyfinNowPlaying(ctx context.Context, widget *nowPlayingWidget) ([]nowPlayingItem, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, widget.Server+"/Sessions", nil)
	if err != nil {
		return nil, err
	}
	applyMediaServerAuth(request, widget.Service, widget.APIKey)
	request.Header.Set("Accept", "application/json")
	response, err := newHTTPClient(widget.Timeout, widget.AllowInsecure).Do(request)
	if err != nil {
		return nil, fmt.Errorf("requesting %s now playing: %w", widget.Service, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, unexpectedHTTPStatusError(response)
	}
	var payload []jellyfinSession
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding %s now playing: %w", widget.Service, err)
	}
	items := make([]nowPlayingItem, 0, len(payload))
	for _, session := range payload {
		if session.NowPlayingItem == nil {
			continue
		}
		items = append(items, normalizeJellyfinNowPlaying(widget, session))
	}
	return items, nil
}

func normalizeJellyfinNowPlaying(widget *nowPlayingWidget, session jellyfinSession) nowPlayingItem {
	media := session.NowPlayingItem
	title, subtitle := media.Name, yearText(media.ProductionYear)
	if strings.EqualFold(media.Type, "Episode") {
		title = media.SeriesName
		if title == "" {
			title = media.Name
		}
		subtitle = mediaSubtitle(fmt.Sprintf("S%02dE%02d", media.ParentIndexNumber, media.IndexNumber), media.Name)
	} else if strings.EqualFold(media.Type, "Audio") {
		subtitle = mediaSubtitle(media.AlbumArtist, media.Album)
	}
	image := ""
	if media.ID != "" {
		image = widget.Server + "/Items/" + url.PathEscape(media.ID) + "/Images/Primary?api_key=" + url.QueryEscape(widget.APIKey)
	}
	state := "playing"
	if session.PlayState.IsPaused {
		state = "paused"
	}
	return nowPlayingItem{Title: title, Subtitle: subtitle, MediaType: mediaTypeLabel(media.Type), User: session.UserName, Client: session.Client, Device: session.DeviceName, ImageURL: image, State: state, PlayMethod: mediaPlayMethod(session.PlayState.PlayMethod), Position: time.Duration(session.PlayState.PositionTicks) * 100 * time.Nanosecond, Duration: time.Duration(media.RunTimeTicks) * 100 * time.Nanosecond}
}

func fetchNavidromeNowPlaying(ctx context.Context, widget *nowPlayingWidget) ([]nowPlayingItem, error) {
	values, err := navidromeAuthValues(widget.Username, widget.Password)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, widget.Server+"/rest/getNowPlaying.view?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	response, err := newHTTPClient(widget.Timeout, widget.AllowInsecure).Do(request)
	if err != nil {
		return nil, fmt.Errorf("requesting Navidrome now playing: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, unexpectedHTTPStatusError(response)
	}
	var payload navidromeNowPlayingResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding Navidrome now playing: %w", err)
	}
	if payload.SubsonicResponse.Status != "ok" {
		if payload.SubsonicResponse.Error != nil {
			return nil, errors.New(payload.SubsonicResponse.Error.Message)
		}
		return nil, errors.New("Navidrome returned a failed response")
	}
	items := make([]nowPlayingItem, 0, len(payload.SubsonicResponse.NowPlaying.Entry))
	for _, entry := range payload.SubsonicResponse.NowPlaying.Entry {
		image := ""
		if entry.CoverArt != "" {
			cover, err := navidromeCoverValues(widget.Username, widget.Password, entry.CoverArt)
			if err != nil {
				return nil, err
			}
			image = widget.Server + "/rest/getCoverArt.view?" + cover.Encode()
		}
		state := strings.ToLower(entry.State)
		if state == "" {
			state = "playing"
		}
		items = append(items, nowPlayingItem{Title: entry.Title, Subtitle: mediaSubtitle(entry.Artist, entry.Album), MediaType: "Track", User: entry.Username, Client: entry.PlayerName, ImageURL: image, State: state, Position: time.Duration(entry.PositionMS) * time.Millisecond, Duration: time.Duration(entry.Duration) * time.Second})
	}
	return items, nil
}

func mediaProgress(position, duration time.Duration) float64 {
	if duration <= 0 {
		return 0
	}
	progress := float64(position) / float64(duration) * 100
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
}

func formatNowPlayingProgress(position, duration time.Duration) string {
	if duration <= 0 {
		return ""
	}
	if position < 0 {
		position = 0
	}
	if position > duration {
		position = duration
	}
	return formatPlaybackClock(position) + " / " + formatPlaybackClock(duration)
}

func formatPlaybackClock(value time.Duration) string {
	seconds := int(value / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, (seconds%3600)/60, seconds%60)
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func mediaPlayMethod(value string) string {
	switch strings.ToLower(strings.ReplaceAll(value, " ", "")) {
	case "directplay", "direct":
		return "Direct Play"
	case "directstream", "copy":
		return "Direct Stream"
	case "transcode", "transcoded":
		return "Transcode"
	default:
		return value
	}
}
