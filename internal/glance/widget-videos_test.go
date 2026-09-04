package glance

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestVideosFetchYoutubeChannelUploadsCancellationPreservesClassificationAndCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	widget := &videosWidget{}

	videos, err := widget.fetchYoutubeChannelUploads(
		ctx,
		[]string{"channel"},
		"",
		true,
	)
	if err == nil {
		t.Fatal("expected cancellation error")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve no-content classification: %v", err)
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error does not preserve context cancellation: %v", err)
	}

	if videos != nil {
		t.Fatalf("videos = %#v, want nil", videos)
	}

	expected := "failed to retrieve any content: fetching YouTube feeds: context canceled"
	if err.Error() != expected {
		t.Fatalf(
			"unexpected cancellation error:\n got: %q\nwant: %q",
			err.Error(),
			expected,
		)
	}
}

func TestVideosFetchYoutubeChannelUploadsEmptyChannelsReturnsNoContent(t *testing.T) {
	widget := &videosWidget{}

	videos, err := widget.fetchYoutubeChannelUploads(
		context.Background(),
		nil,
		"",
		true,
	)
	if err == nil {
		t.Fatal("expected no-content error")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve no-content classification: %v", err)
	}

	if videos != nil {
		t.Fatalf("videos = %#v, want nil", videos)
	}

	expected := "failed to retrieve any content: failed 0 of 0 YouTube feeds"
	if err.Error() != expected {
		t.Fatalf(
			"unexpected empty-channel error:\n got: %q\nwant: %q",
			err.Error(),
			expected,
		)
	}
}

func TestVideosFetchYoutubeChannelUploadsFailedRefreshUsesExpiredCachedContent(t *testing.T) {
	const channelID = "invalid\x00channel"

	cachedVideos := videoList{
		{
			Title:      "Cached Video",
			Url:        "https://example.com/video",
			Author:     "Cached Channel",
			AuthorUrl:  "https://example.com/channel",
			TimePosted: time.Now().Add(-2 * time.Hour),
		},
	}

	widget := &videosWidget{}
	widget.cacheDuration = time.Hour
	widget.cachedVideoLists.Store(
		channelID,
		cachedEntry[videoList]{
			value:     cachedVideos,
			timestamp: time.Now().Add(-2 * time.Hour),
		},
	)

	videos, err := widget.fetchYoutubeChannelUploads(
		context.Background(),
		[]string{channelID},
		"",
		true,
	)
	if err == nil {
		t.Fatal("expected partial-content error")
	}

	if !errors.Is(err, errPartialContent) {
		t.Fatalf("error does not preserve partial-content classification: %v", err)
	}

	if errors.Is(err, errNoContent) {
		t.Fatalf("partial-content error unexpectedly classified as no-content: %v", err)
	}

	if len(videos) != 1 {
		t.Fatalf("got %d cached videos, want 1", len(videos))
	}

	if videos[0].Title != "Cached Video" {
		t.Fatalf("cached video title = %q, want %q", videos[0].Title, "Cached Video")
	}

	if !strings.Contains(err.Error(), "failed 1 of 1 YouTube feeds") {
		t.Fatalf("error missing failure count: %v", err)
	}

	if !strings.Contains(err.Error(), "first failure: creating YouTube feed request:") {
		t.Fatalf("error missing representative request failure: %v", err)
	}
}

func TestVideosFetchYoutubeChannelUploadsFailedRefreshWithoutCacheReturnsNoContent(t *testing.T) {
	const channelID = "invalid\x00channel"

	widget := &videosWidget{}
	widget.cacheDuration = time.Hour

	videos, err := widget.fetchYoutubeChannelUploads(
		context.Background(),
		[]string{channelID},
		"",
		true,
	)
	if err == nil {
		t.Fatal("expected no-content error")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve no-content classification: %v", err)
	}

	if errors.Is(err, errPartialContent) {
		t.Fatalf("no-content error unexpectedly classified as partial-content: %v", err)
	}

	if videos != nil {
		t.Fatalf("videos = %#v, want nil", videos)
	}

	if !strings.Contains(err.Error(), "failed 1 of 1 YouTube feeds") {
		t.Fatalf("error missing failure count: %v", err)
	}

	if !strings.Contains(err.Error(), "first failure: creating YouTube feed request:") {
		t.Fatalf("error missing representative request failure: %v", err)
	}
}

func TestVideosFetchYoutubeChannelUploadsFreshCacheIsSuccessful(t *testing.T) {
	const channelID = "cached-channel"

	cachedVideos := videoList{
		{
			Title:      "Cached Video",
			Url:        "https://example.com/video",
			Author:     "Cached Channel",
			AuthorUrl:  "https://example.com/channel",
			TimePosted: time.Now().Add(-time.Minute),
		},
	}

	widget := &videosWidget{}
	widget.cacheDuration = time.Hour
	widget.cachedVideoLists.Store(
		channelID,
		cachedEntry[videoList]{
			value:     cachedVideos,
			timestamp: time.Now(),
		},
	)

	videos, err := widget.fetchYoutubeChannelUploads(
		context.Background(),
		[]string{channelID},
		"",
		true,
	)
	if err != nil {
		t.Fatalf("fresh cached content returned unexpected error: %v", err)
	}

	if len(videos) != 1 {
		t.Fatalf("got %d cached videos, want 1", len(videos))
	}

	if videos[0].Title != "Cached Video" {
		t.Fatalf("cached video title = %q, want %q", videos[0].Title, "Cached Video")
	}
}

type youtubeFallbackRoundTripper func(*http.Request) (*http.Response, error)

func (f youtubeFallbackRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestVideosFetchYoutubeChannelUploadsFallsBackToUploadsPlaylist(t *testing.T) {
	const channelID = "UC123456"

	originalClient := defaultHTTPClient
	t.Cleanup(func() {
		defaultHTTPClient = originalClient
	})

	var requestedPlaylistIDs []string

	defaultHTTPClient = &http.Client{
		Transport: youtubeFallbackRoundTripper(func(request *http.Request) (*http.Response, error) {
			playlistID := request.URL.Query().Get("playlist_id")
			requestedPlaylistIDs = append(requestedPlaylistIDs, playlistID)

			switch playlistID {
			case "UULF123456":
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Status:     "404 Not Found",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("not found")),
					Request:    request,
				}, nil
			case "UU123456":
				body := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"
      xmlns:media="http://search.yahoo.com/mrss/">
  <author>
    <name>Fallback Channel</name>
    <uri>https://www.youtube.com/channel/UC123456</uri>
  </author>
  <entry>
    <title>Fallback Video</title>
    <published>2026-08-31T20:00:00+00:00</published>
    <link href="https://www.youtube.com/watch?v=fallback123"/>
    <media:group>
      <media:thumbnail url="https://example.com/fallback.jpg"/>
    </media:group>
  </entry>
</feed>`

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    request,
				}, nil
			default:
				t.Fatalf("unexpected playlist request %q", playlistID)
				return nil, nil
			}
		}),
	}

	widget := &videosWidget{}
	widget.cacheDuration = time.Hour

	videos, err := widget.fetchYoutubeChannelUploads(
		context.Background(),
		[]string{channelID},
		"",
		false,
	)
	if err != nil {
		t.Fatalf("fallback returned unexpected error: %v", err)
	}

	if len(requestedPlaylistIDs) != 2 {
		t.Fatalf("got %d requests, want 2: %#v", len(requestedPlaylistIDs), requestedPlaylistIDs)
	}

	if requestedPlaylistIDs[0] != "UULF123456" {
		t.Fatalf("first playlist = %q, want %q", requestedPlaylistIDs[0], "UULF123456")
	}

	if requestedPlaylistIDs[1] != "UU123456" {
		t.Fatalf("fallback playlist = %q, want %q", requestedPlaylistIDs[1], "UU123456")
	}

	if len(videos) != 1 {
		t.Fatalf("got %d videos, want 1", len(videos))
	}

	if videos[0].Title != "Fallback Video" {
		t.Fatalf("video title = %q, want %q", videos[0].Title, "Fallback Video")
	}
}

func TestVideosFetchYoutubeChannelUploadsDoesNotFallbackWhenShortsIncluded(t *testing.T) {
	const channelID = "UC123456"

	originalClient := defaultHTTPClient
	t.Cleanup(func() {
		defaultHTTPClient = originalClient
	})

	var requestCount int

	defaultHTTPClient = &http.Client{
		Transport: youtubeFallbackRoundTripper(func(request *http.Request) (*http.Response, error) {
			requestCount++

			if got := request.URL.Query().Get("channel_id"); got != channelID {
				t.Fatalf("channel_id = %q, want %q", got, channelID)
			}

			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("not found")),
				Request:    request,
			}, nil
		}),
	}

	widget := &videosWidget{}
	widget.cacheDuration = time.Hour

	_, err := widget.fetchYoutubeChannelUploads(
		context.Background(),
		[]string{channelID},
		"",
		true,
	)
	if err == nil {
		t.Fatal("expected failed channel feed")
	}

	if requestCount != 1 {
		t.Fatalf("got %d requests, want 1", requestCount)
	}
}

func TestVideosFetchYoutubeChannelUploadsDoesNotFallbackForExplicitPlaylist(t *testing.T) {
	const playlistID = "PL123456"

	originalClient := defaultHTTPClient
	t.Cleanup(func() {
		defaultHTTPClient = originalClient
	})

	var requestCount int

	defaultHTTPClient = &http.Client{
		Transport: youtubeFallbackRoundTripper(func(request *http.Request) (*http.Response, error) {
			requestCount++

			if got := request.URL.Query().Get("playlist_id"); got != playlistID {
				t.Fatalf("playlist_id = %q, want %q", got, playlistID)
			}

			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("not found")),
				Request:    request,
			}, nil
		}),
	}

	widget := &videosWidget{}
	widget.cacheDuration = time.Hour

	_, err := widget.fetchYoutubeChannelUploads(
		context.Background(),
		[]string{videosWidgetPlaylistPrefix + playlistID},
		"",
		false,
	)
	if err == nil {
		t.Fatal("expected failed playlist feed")
	}

	if requestCount != 1 {
		t.Fatalf("got %d requests, want 1", requestCount)
	}
}

func TestVideosFetchYoutubeChannelUploadsFallbackPreservesBothFailures(t *testing.T) {
	const channelID = "UC123456"

	originalClient := defaultHTTPClient
	t.Cleanup(func() {
		defaultHTTPClient = originalClient
	})

	var requestCount int

	defaultHTTPClient = &http.Client{
		Transport: youtubeFallbackRoundTripper(func(request *http.Request) (*http.Response, error) {
			requestCount++

			playlistID := request.URL.Query().Get("playlist_id")
			return nil, errors.New("failed " + playlistID)
		}),
	}

	widget := &videosWidget{}
	widget.cacheDuration = time.Hour

	videos, err := widget.fetchYoutubeChannelUploads(
		context.Background(),
		[]string{channelID},
		"",
		false,
	)
	if err == nil {
		t.Fatal("expected both YouTube feed attempts to fail")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve no-content classification: %v", err)
	}

	if videos != nil {
		t.Fatalf("videos = %#v, want nil", videos)
	}

	if requestCount != 2 {
		t.Fatalf("got %d requests, want 2", requestCount)
	}

	if !strings.Contains(err.Error(), "failed UULF123456") {
		t.Fatalf("error missing primary UULF failure: %v", err)
	}

	if !strings.Contains(err.Error(), "failed UU123456") {
		t.Fatalf("error missing fallback UU failure: %v", err)
	}
}

func TestVideosRenderRespectsNewTabPolicy(t *testing.T) {
	video := video{
		ThumbnailUrl: "https://example.com/thumbnail.jpg",
		Title:        "Test Video",
		Url:          "https://example.com/video",
		Author:       "Test Channel",
		AuthorUrl:    "https://example.com/channel",
		TimePosted:   time.Now().Add(-time.Hour),
	}

	for _, style := range []string{"", "grid-cards", "vertical-list"} {
		style := style

		t.Run("style="+style+"/new-tab", func(t *testing.T) {
			widget := &videosWidget{
				widgetBase: widgetBase{
					Title:             "Videos",
					OpenLinksInNewTab: true,
					ContentAvailable:  true,
				},
				Videos:            videoList{video},
				Style:             style,
				CollapseAfter:     7,
				CollapseAfterRows: 4,
			}

			rendered := string(widget.Render())

			if !strings.Contains(rendered, `href="https://example.com/video" target="_blank" rel="noreferrer"`) {
				t.Fatalf("video link does not open in new tab:\n%s", rendered)
			}

			if !strings.Contains(rendered, `href="https://example.com/channel" target="_blank" rel="noreferrer"`) {
				t.Fatalf("author link does not open in new tab:\n%s", rendered)
			}
		})

		t.Run("style="+style+"/same-tab", func(t *testing.T) {
			widget := &videosWidget{
				widgetBase: widgetBase{
					Title:             "Videos",
					OpenLinksInNewTab: false,
					ContentAvailable:  true,
				},
				Videos:            videoList{video},
				Style:             style,
				CollapseAfter:     7,
				CollapseAfterRows: 4,
			}

			rendered := string(widget.Render())

			if strings.Contains(rendered, `href="https://example.com/video" target="_blank"`) {
				t.Fatalf("video link unexpectedly opens in new tab:\n%s", rendered)
			}

			if strings.Contains(rendered, `href="https://example.com/channel" target="_blank"`) {
				t.Fatalf("author link unexpectedly opens in new tab:\n%s", rendered)
			}

			if !strings.Contains(rendered, `href="https://example.com/video"`) {
				t.Fatalf("video link missing:\n%s", rendered)
			}

			if !strings.Contains(rendered, `href="https://example.com/channel"`) {
				t.Fatalf("author link missing:\n%s", rendered)
			}
		})
	}
}
