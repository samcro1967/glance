package glance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testRSSFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
	<channel>
		<title>Test Feed</title>
		<link>https://example.com/</link>
		<description>Test feed</description>
		<item>
			<title>Test Item</title>
			<link>https://example.com/item</link>
			<description>Test description</description>
		</item>
	</channel>
</rss>`

func TestRSSFetchItemsFromFeedsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testRSSFeed))
	}))
	defer server.Close()

	widget := &rssWidget{
		FeedRequests: []rssFeedRequest{
			{URL: server.URL + "/feed"},
		},
		cachedFeeds: make(map[string]*cachedRSSFeed),
	}

	items, err := widget.fetchItemsFromFeeds(context.Background())
	if err != nil {
		t.Fatalf("fetchItemsFromFeeds returned unexpected error: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}

	if items[0].Title != "Test Item" {
		t.Fatalf("item title = %q, want %q", items[0].Title, "Test Item")
	}
}

func TestRSSFetchItemsFromFeedsPartialFailurePreservesContentAndCause(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/good":
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(testRSSFeed))
		case "/bad":
			http.Error(w, "provider details must not appear", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	widget := &rssWidget{
		FeedRequests: []rssFeedRequest{
			{URL: server.URL + "/good"},
			{URL: server.URL + "/bad"},
		},
		cachedFeeds: make(map[string]*cachedRSSFeed),
	}

	items, err := widget.fetchItemsFromFeeds(context.Background())
	if err == nil {
		t.Fatal("expected partial-content error")
	}

	if !errors.Is(err, errPartialContent) {
		t.Fatalf("error does not preserve partial-content classification: %v", err)
	}

	if errors.Is(err, errNoContent) {
		t.Fatalf("partial-content error unexpectedly classified as no-content: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("got %d successful items, want 1", len(items))
	}

	if items[0].Title != "Test Item" {
		t.Fatalf("item title = %q, want %q", items[0].Title, "Test Item")
	}

	expectedCause := "unexpected HTTP status 503 Service Unavailable"
	if !strings.Contains(err.Error(), expectedCause) {
		t.Fatalf("error missing representative cause %q: %v", expectedCause, err)
	}

	expectedCount := "failed 1 of 2 RSS feeds"
	if !strings.Contains(err.Error(), expectedCount) {
		t.Fatalf("error missing failure count %q: %v", expectedCount, err)
	}

	if strings.Contains(err.Error(), "provider details must not appear") {
		t.Fatalf("error exposed HTTP response body: %v", err)
	}

	if strings.Contains(err.Error(), server.URL) {
		t.Fatalf("error exposed RSS feed URL: %v", err)
	}
}

func TestRSSFetchItemsFromFeedsTotalFailurePreservesCause(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "private provider response", http.StatusUnauthorized)
	}))
	defer server.Close()

	widget := &rssWidget{
		FeedRequests: []rssFeedRequest{
			{URL: server.URL + "/one"},
			{URL: server.URL + "/two"},
		},
		cachedFeeds: make(map[string]*cachedRSSFeed),
	}

	items, err := widget.fetchItemsFromFeeds(context.Background())
	if err == nil {
		t.Fatal("expected no-content error")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve no-content classification: %v", err)
	}

	if errors.Is(err, errPartialContent) {
		t.Fatalf("no-content error unexpectedly classified as partial-content: %v", err)
	}

	if items != nil {
		t.Fatalf("items = %#v, want nil", items)
	}

	expectedCause := "unexpected HTTP status 401 Unauthorized"
	if !strings.Contains(err.Error(), expectedCause) {
		t.Fatalf("error missing representative cause %q: %v", expectedCause, err)
	}

	expectedCount := "failed 2 of 2 RSS feeds"
	if !strings.Contains(err.Error(), expectedCount) {
		t.Fatalf("error missing failure count %q: %v", expectedCount, err)
	}

	if strings.Contains(err.Error(), "private provider response") {
		t.Fatalf("error exposed HTTP response body: %v", err)
	}

	if strings.Contains(err.Error(), server.URL) {
		t.Fatalf("error exposed RSS feed URL: %v", err)
	}
}

func TestRSSFetchItemsFromFeedsUsesFirstFailureAsRepresentativeCause(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/first":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/second":
			w.WriteHeader(http.StatusBadGateway)
		case "/good":
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(testRSSFeed))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	widget := &rssWidget{
		FeedRequests: []rssFeedRequest{
			{URL: server.URL + "/first"},
			{URL: server.URL + "/second"},
			{URL: server.URL + "/good"},
		},
		cachedFeeds: make(map[string]*cachedRSSFeed),
	}

	_, err := widget.fetchItemsFromFeeds(context.Background())
	if err == nil {
		t.Fatal("expected partial-content error")
	}

	if !errors.Is(err, errPartialContent) {
		t.Fatalf("error does not preserve partial-content classification: %v", err)
	}

	if !strings.Contains(err.Error(), "failed 2 of 3 RSS feeds") {
		t.Fatalf("error missing failure count: %v", err)
	}

	if !strings.Contains(err.Error(), "first failure: unexpected HTTP status 429 Too Many Requests") {
		t.Fatalf("error missing first representative failure: %v", err)
	}

	if strings.Contains(err.Error(), "502 Bad Gateway") {
		t.Fatalf("error included additional failure instead of only representative cause: %v", err)
	}
}

func TestRSSFetchItemsFromFeedsCancellationPreservesClassificationAndCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	widget := &rssWidget{
		FeedRequests: []rssFeedRequest{
			{URL: "https://example.com/feed"},
		},
		cachedFeeds: make(map[string]*cachedRSSFeed),
	}

	_, err := widget.fetchItemsFromFeeds(ctx)
	if err == nil {
		t.Fatal("expected cancellation error")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve no-content classification: %v", err)
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error does not preserve context cancellation: %v", err)
	}

	expected := fmt.Sprintf("%s: fetching RSS feeds: %s", errNoContent, context.Canceled)
	if err.Error() != expected {
		t.Fatalf("unexpected cancellation error:\n got: %q\nwant: %q", err.Error(), expected)
	}
}

func TestRSSFetchSendsConfiguredHeadersAndBasicAuth(t *testing.T) {
	const (
		username = "rss-user"
		password = "rss-pass"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-RSS-Test"); got != "present" {
			t.Errorf("X-RSS-Test = %q, want present", got)
		}

		gotUsername, gotPassword, ok := r.BasicAuth()
		if !ok {
			t.Error("RSS request did not contain Basic Authentication")
		} else {
			if gotUsername != username {
				t.Errorf("username = %q, want %q", gotUsername, username)
			}
			if gotPassword != password {
				t.Errorf("password = %q, want %q", gotPassword, password)
			}
		}

		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testRSSFeed))
	}))
	defer server.Close()

	request := rssFeedRequest{
		URL: server.URL + "/feed",
		Headers: map[string]string{
			"X-RSS-Test": "present",
		},
	}
	request.BasicAuth.Username = username
	request.BasicAuth.Password = password

	widget := &rssWidget{
		FeedRequests: []rssFeedRequest{request},
		cachedFeeds:  make(map[string]*cachedRSSFeed),
	}

	items, err := widget.fetchItemsFromFeeds(context.Background())
	if err != nil {
		t.Fatalf("fetchItemsFromFeeds: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
}
