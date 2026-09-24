package glance

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type mediaServerConfig struct {
	Service       string
	Server        string
	APIKey        string
	Username      string
	Password      string
	UserID        string
	Limit         int
	Timeout       durationField
	AllowInsecure bool
}

type mediaItem struct {
	Title     string
	Subtitle  string
	MediaType string
	Date      string
	Summary   string
	ImageURL  string
	URL       string
	Duration  string
	SortTime  time.Time
}

func (config *mediaServerConfig) normalize(requireHistoryUser bool) error {
	config.Service = strings.ToLower(strings.TrimSpace(config.Service))
	switch config.Service {
	case "plex", "jellyfin", "emby", "navidrome":
	default:
		return errors.New("service must be plex, jellyfin, emby, or navidrome")
	}
	parsed, err := url.Parse(strings.TrimSpace(config.Server))
	if err != nil {
		return fmt.Errorf("parsing server URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("server must be an absolute HTTP or HTTPS URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("server URL must not contain a query string or fragment")
	}
	config.Server = strings.TrimRight(strings.TrimSpace(config.Server), "/")
	if config.Limit < 0 {
		return errors.New("limit must not be negative")
	}
	if config.Limit == 0 {
		config.Limit = 10
	}
	if config.Service == "navidrome" {
		if strings.TrimSpace(config.Username) == "" || config.Password == "" {
			return errors.New("username and password are required for navidrome")
		}
	} else if strings.TrimSpace(config.APIKey) == "" {
		return errors.New("api-key is required for plex, jellyfin, and emby")
	}
	if requireHistoryUser && (config.Service == "jellyfin" || config.Service == "emby") && strings.TrimSpace(config.UserID) == "" {
		return errors.New("user-id is required for jellyfin and emby history")
	}
	return nil
}

func mediaTypeLabel(value string) string {
	switch strings.ToLower(value) {
	case "movie":
		return "Movie"
	case "series", "show":
		return "Series"
	case "episode":
		return "Episode"
	case "audio", "track":
		return "Track"
	case "musicartist", "artist":
		return "Artist"
	case "musicalbum", "album":
		return "Album"
	default:
		if value == "" {
			return "Media"
		}
		return strings.ToUpper(value[:1]) + value[1:]
	}
}

func formatMediaDuration(duration time.Duration) string {
	if duration <= 0 {
		return ""
	}
	minutes := int(duration.Round(time.Minute) / time.Minute)
	if minutes < 60 {
		return strconv.Itoa(minutes) + "m"
	}
	hours := minutes / 60
	minutes %= 60
	if minutes == 0 {
		return strconv.Itoa(hours) + "h"
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func formatMediaDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("Jan 2, 2006")
}

func mediaSubtitle(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, " · ")
}

func navidromeAuthValues(username, password string) url.Values {
	// A stable per-request salt is unnecessary for credential secrecy because the
	// request remains server-side, but a varying salt prevents token reuse.
	salt := strconv.FormatInt(time.Now().UnixNano(), 36)
	digest := md5.Sum([]byte(password + salt)) // #nosec G401 -- Subsonic API requires MD5 token authentication.
	return url.Values{
		"u": {username}, "t": {hex.EncodeToString(digest[:])}, "s": {salt},
		"v": {"1.16.1"}, "c": {"glance"}, "f": {"json"},
	}
}

func proxyMediaImage(providers *widgetProviders, raw string) string {
	if raw == "" {
		return ""
	}
	if providers == nil || providers.resourceProxyURL == nil {
		return ""
	}
	proxied, err := providers.resourceProxyURL(raw)
	if err != nil {
		return ""
	}
	return proxied
}
