package glance

import (
	"context"
	"errors"
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
