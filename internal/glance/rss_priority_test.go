package glance

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
)

func TestPriorityRSSInitializeDetailedAndBounds(t *testing.T) {
	widget := &rssWidget{
		Style:           "detailed-list",
		Limit:           -1,
		CollapseAfter:   -2,
		ThumbnailHeight: -5,
		CardHeight:      -10,
		FeedRequests:    []rssFeedRequest{{URL: "https://example.com/feed"}},
	}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}
	if widget.Limit != 25 || widget.CollapseAfter != 5 {
		t.Fatalf("defaults: limit=%d collapse=%d", widget.Limit, widget.CollapseAfter)
	}
	if widget.ThumbnailHeight != 0 || widget.CardHeight != 0 {
		t.Fatalf("negative dimensions not clamped: %v %v", widget.ThumbnailHeight, widget.CardHeight)
	}
	if !widget.FeedRequests[0].IsDetailed {
		t.Fatal("detailed style did not mark feed detailed")
	}
	if widget.cachedFeeds == nil {
		t.Fatal("cache not initialized")
	}
}

func TestPriorityRSSInitializeImageRequirements(t *testing.T) {
	tests := []struct {
		style         string
		includeImages bool
	}{
		{style: "", includeImages: false},
		{style: "detailed-list", includeImages: true},
		{style: "horizontal-cards", includeImages: true},
		{style: "horizontal-cards-2", includeImages: true},
	}

	for _, test := range tests {
		t.Run(test.style, func(t *testing.T) {
			widget := &rssWidget{
				Style:        test.style,
				FeedRequests: []rssFeedRequest{{URL: "https://example.com/feed"}},
			}

			if err := widget.initialize(); err != nil {
				t.Fatal(err)
			}

			if got := widget.FeedRequests[0].IncludeImages; got != test.includeImages {
				t.Fatalf("IncludeImages = %v, want %v", got, test.includeImages)
			}
		})
	}
}

func TestPriorityRSSDescriptionSanitizationAndShortening(t *testing.T) {
	input := "  <p class=\"x\">Hello&nbsp;   world</p>\n<strong>again</strong>  "
	got := sanitizeFeedDescription(input)
	if got != "Hello\u00a0 world again" {
		t.Fatalf("sanitize = %q", got)
	}
	if got := shortenFeedDescriptionLen("<p>Hello wonderful world</p>", 8); got != "Hello wo…" {
		t.Fatalf("shorten = %q", got)
	}
}

func TestPriorityRSSFeedTaskCachesAndRevalidates(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 2 {
			if r.Header.Get("If-None-Match") != `"v1"` {
				t.Errorf("If-None-Match = %q", r.Header.Get("If-None-Match"))
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Feed</title><link>https://example.com</link><item><title>One</title><link>/one</link><pubDate>Sun, 30 Aug 2026 18:00:00 GMT</pubDate></item></channel></rss>`))
	}))
	defer server.Close()

	widget := &rssWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}
	req := rssFeedRequest{URL: server.URL}

	first, err := widget.fetchItemsFromFeedTask(t.Context(), req)
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	second, err := widget.fetchItemsFromFeedTask(t.Context(), req)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	if len(first) != 1 || len(second) != 1 || first[0].Title != second[0].Title {
		t.Fatalf("cache mismatch: %#v %#v", first, second)
	}
	if first[0].Link != "https://example.com/one" {
		t.Fatalf("resolved link = %q", first[0].Link)
	}
	if first[0].PublishedAt.IsZero() || first[0].PublishedAt.After(time.Now().Add(time.Hour)) {
		t.Fatalf("published time = %v", first[0].PublishedAt)
	}
}

func TestPriorityRSSConditionalCacheAppliesFeedOptionsAfter304(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > 1 {
			if got := r.Header.Get("If-None-Match"); got != `"v1"` {
				t.Errorf("If-None-Match = %q, want v1", got)
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0"><channel><title>Original</title><link>https://example.com</link>
<item><title>First</title><link>/first</link><description>First description</description><category>News</category></item>
<item><title>Second</title><link>/second</link><description>Second description</description><category>Sports</category></item>
</channel></rss>`))
	}))
	defer server.Close()

	widget := &rssWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	first, err := widget.fetchItemsFromFeedTask(t.Context(), rssFeedRequest{
		URL: server.URL, Title: "Limited", Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].ChannelName != "Limited" {
		t.Fatalf("first feed = %#v", first)
	}

	second, err := widget.fetchItemsFromFeedTask(t.Context(), rssFeedRequest{
		URL: server.URL, Title: "Detailed", IsDetailed: true,
		ItemLinkPrefix: "https://proxy.example/?url=",
	})
	if err != nil {
		t.Fatal(err)
	}

	if requests != 2 {
		t.Fatalf("HTTP requests = %d, want 2", requests)
	}
	if len(second) != 2 {
		t.Fatalf("second feed items = %d, want 2", len(second))
	}
	if second[0].ChannelName != "Detailed" {
		t.Fatalf("channel name = %q", second[0].ChannelName)
	}
	if second[0].Link != "https://proxy.example/?url=/first" {
		t.Fatalf("link = %q", second[0].Link)
	}
	if second[0].Description != "First description" {
		t.Fatalf("description = %q", second[0].Description)
	}
	if len(second[0].Categories) != 1 || second[0].Categories[0] != "News" {
		t.Fatalf("categories = %#v", second[0].Categories)
	}
}

func TestPriorityRSSConditionalCacheSeparatesAuthorization(t *testing.T) {
	var conditionalRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, _, _ := r.BasicAuth()
		if r.Header.Get("If-None-Match") != "" {
			conditionalRequests++
		}

		w.Header().Set("ETag", `"`+username+`"`)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel>
<title>` + username + `</title><item><title>Item</title></item>
</channel></rss>`))
	}))
	defer server.Close()

	widget := &rssWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	firstRequest := rssFeedRequest{URL: server.URL}
	firstRequest.BasicAuth.Username = "first"
	firstRequest.BasicAuth.Password = "secret"

	secondRequest := rssFeedRequest{URL: server.URL}
	secondRequest.BasicAuth.Username = "second"
	secondRequest.BasicAuth.Password = "secret"

	first, err := widget.fetchItemsFromFeedTask(t.Context(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := widget.fetchItemsFromFeedTask(t.Context(), secondRequest)
	if err != nil {
		t.Fatal(err)
	}

	if conditionalRequests != 0 {
		t.Fatalf("cross-credential conditional requests = %d, want 0", conditionalRequests)
	}
	if len(first) != 1 || first[0].ChannelName != "first" {
		t.Fatalf("first response = %#v", first)
	}
	if len(second) != 1 || second[0].ChannelName != "second" {
		t.Fatalf("second response = %#v", second)
	}
}

func TestPriorityRSSConditionalCachePreservesUndatedPublicationTime(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 2 {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel>
<title>Undated</title><item><title>Item</title></item>
</channel></rss>`))
	}))
	defer server.Close()

	widget := &rssWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	request := rssFeedRequest{URL: server.URL}
	first, err := widget.fetchItemsFromFeedTask(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := widget.fetchItemsFromFeedTask(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	if requests != 2 || len(first) != 1 || len(second) != 1 {
		t.Fatalf("requests=%d first=%#v second=%#v", requests, first, second)
	}
	if first[0].PublishedAt.IsZero() || !first[0].PublishedAt.Equal(second[0].PublishedAt) {
		t.Fatalf("publication timestamps differ: %v and %v",
			first[0].PublishedAt, second[0].PublishedAt)
	}
}

func TestPriorityRSSConditionalCacheClearsMissingValidators(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++

		switch requests {
		case 1:
			if got := r.Header.Get("If-None-Match"); got != "" {
				t.Errorf("initial validator = %q", got)
			}
			w.Header().Set("ETag", `"v1"`)
		case 2:
			if got := r.Header.Get("If-None-Match"); got != `"v1"` {
				t.Errorf("second validator = %q, want v1", got)
			}
		case 3:
			if got := r.Header.Get("If-None-Match"); got != "" {
				t.Errorf("stale validator retained = %q", got)
			}
		}

		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel>
<title>Feed</title><item><title>Item</title></item>
</channel></rss>`))
	}))
	defer server.Close()

	widget := &rssWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	request := rssFeedRequest{URL: server.URL}
	for i := 0; i < 3; i++ {
		items, err := widget.fetchItemsFromFeedTask(t.Context(), request)
		if err != nil {
			t.Fatalf("fetch %d: %v", i+1, err)
		}
		if len(items) != 1 {
			t.Fatalf("fetch %d items = %d", i+1, len(items))
		}
	}

	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
}

func TestPriorityRSSConditionalCacheSnapshotUpdates(t *testing.T) {
	key := [32]byte{1}
	widget := &rssWidget{
		cachedFeeds: make(map[[32]byte]*cachedRSSFeed),
	}

	initial := &cachedRSSFeed{
		etag: `"v1"`,
		feed: &gofeed.Feed{Title: "Initial"},
	}
	newer := &cachedRSSFeed{
		etag: `"v2"`,
		feed: &gofeed.Feed{Title: "Newer"},
	}
	stale := &cachedRSSFeed{
		etag: `"stale"`,
		feed: &gofeed.Feed{Title: "Stale"},
	}

	widget.updateCachedFeed(key, nil, initial)
	if got := widget.cachedFeeds[key]; got != initial {
		t.Fatalf("initial cache entry = %p, want %p", got, initial)
	}

	widget.updateCachedFeed(key, initial, newer)
	if got := widget.cachedFeeds[key]; got != newer {
		t.Fatalf("replacement cache entry = %p, want %p", got, newer)
	}

	// An older request must not overwrite a representation that changed
	// after the request read its original cache snapshot.
	widget.updateCachedFeed(key, initial, stale)
	if got := widget.cachedFeeds[key]; got != newer {
		t.Fatalf("stale response replaced newer cache entry: %p", got)
	}

	// A stale response without validators must not delete newer data.
	widget.updateCachedFeed(key, initial, &cachedRSSFeed{})
	if got := widget.cachedFeeds[key]; got != newer {
		t.Fatalf("stale invalidation removed newer cache entry: %p", got)
	}

	// A current response without validators must invalidate the entry.
	widget.updateCachedFeed(key, newer, &cachedRSSFeed{})
	if _, exists := widget.cachedFeeds[key]; exists {
		t.Fatal("cache entry retained after validators disappeared")
	}

	// A response from the original empty snapshot can populate the cache.
	widget.updateCachedFeed(key, nil, initial)
	if got := widget.cachedFeeds[key]; got != initial {
		t.Fatalf("cache entry after invalidation = %p, want %p", got, initial)
	}

	// A competing response from that same empty snapshot cannot replace it.
	widget.updateCachedFeed(key, nil, stale)
	if got := widget.cachedFeeds[key]; got != initial {
		t.Fatalf("competing empty-snapshot response replaced entry: %p", got)
	}
}

func TestPriorityRSSFeedTaskHonorsCustomHeadersAndItemPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Test"); got != "priority" {
			t.Errorf("X-Test = %q", got)
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Feed</title><link>https://example.com</link><item><description>Hello</description><link>item-1</link></item></channel></rss>`))
	}))
	defer server.Close()

	widget := &rssWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}
	items, err := widget.fetchItemsFromFeedTask(t.Context(), rssFeedRequest{URL: server.URL, Headers: map[string]string{"X-Test": "priority"}, ItemLinkPrefix: "https://proxy.example/?url="})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	if items[0].Link != "https://proxy.example/?url=item-1" {
		t.Fatalf("link = %q", items[0].Link)
	}
	if !strings.Contains(items[0].Title, "Hello") {
		t.Fatalf("title = %q", items[0].Title)
	}
}

func TestPriorityRSSFeedTaskChannelURLResolution(t *testing.T) {
	tests := []struct {
		name        string
		channelLink string
		wantChannel string
		wantItem    string
	}{
		{
			name:        "absolute channel link",
			channelLink: "https://example.com/news",
			wantChannel: "https://example.com/news",
			wantItem:    "https://example.com/article",
		},
		{
			name:        "root relative channel link",
			channelLink: "/news",
			wantChannel: "/news",
			wantItem:    "/article",
		},
		{
			name:        "path relative channel link",
			channelLink: "news",
			wantChannel: "news",
			wantItem:    "/article",
		},
		{
			name:        "missing channel link",
			wantChannel: "",
			wantItem:    "/article",
		},
		{
			name:        "unsafe channel link",
			channelLink: "javascript:alert(1)",
			wantChannel: "",
			wantItem:    "/article",
		},
		{
			name:        "malformed channel link",
			channelLink: "https://[::1",
			wantChannel: "",
			wantItem:    "/article",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/rss+xml")
				_, _ = fmt.Fprintf(w,
					"<rss version=\"2.0\"><channel><title>Feed</title><link>%s</link><item><title>Article</title><link>/article</link></item></channel></rss>",
					tt.channelLink,
				)
			}))
			defer server.Close()

			widget := &rssWidget{}
			if err := widget.initialize(); err != nil {
				t.Fatal(err)
			}

			requestURL := server.URL + "/feeds/latest.xml"
			items, err := widget.fetchItemsFromFeedTask(t.Context(), rssFeedRequest{URL: requestURL})
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 {
				t.Fatalf("items = %d, want 1", len(items))
			}

			wantChannel := tt.wantChannel
			if tt.channelLink == "" {
				wantChannel = server.URL
			} else if strings.HasPrefix(wantChannel, "/") {
				wantChannel = server.URL + wantChannel
			} else if wantChannel == "news" {
				wantChannel = server.URL + "/feeds/news"
			}

			if items[0].ChannelURL != wantChannel {
				t.Errorf("ChannelURL = %q, want %q", items[0].ChannelURL, wantChannel)
			}

			wantItem := tt.wantItem
			if strings.HasPrefix(wantItem, "/") {
				if tt.channelLink == "https://example.com/news" {
					wantItem = "https://example.com" + wantItem
				} else {
					wantItem = server.URL + wantItem
				}
			}

			if items[0].Link != wantItem {
				t.Errorf("item Link = %q, want %q", items[0].Link, wantItem)
			}
		})
	}
}

func TestPriorityRSSTitleSanitization(t *testing.T) {
	got := sanitizeFeedTitle(`<strong>Breaking&nbsp;News</strong> &amp; Updates`)
	if got != "Breaking\u00a0News & Updates" {
		t.Fatalf("sanitize title = %q", got)
	}
}

func TestPriorityRSSResolveURL(t *testing.T) {
	tests := []struct {
		name  string
		value string
		base  string
		want  string
	}{
		{
			name:  "absolute https",
			value: "https://cdn.example.com/image.jpg",
			base:  "https://example.com/feed",
			want:  "https://cdn.example.com/image.jpg",
		},
		{
			name:  "root relative",
			value: "/images/item.jpg",
			base:  "https://example.com/posts/feed.xml",
			want:  "https://example.com/images/item.jpg",
		},
		{
			name:  "path relative",
			value: "images/item.jpg",
			base:  "https://example.com/posts/feed.xml",
			want:  "https://example.com/posts/images/item.jpg",
		},
		{
			name:  "unsupported scheme",
			value: "javascript:alert(1)",
			base:  "https://example.com/feed",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveRSSURL(tt.value, tt.base); got != tt.want {
				t.Fatalf("resolveRSSURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPriorityRSSResolveURLRejectsMalformedResolvedAuthority(t *testing.T) {
	if got := resolveRSSURL("//::", "0", "http://0"); got != "" {
		t.Fatalf("resolveRSSURL() = %q, want empty", got)
	}
}

func TestPriorityRSSFindImageInHTML(t *testing.T) {
	got := findImageInHTML(
		`<p>Hello</p><img alt="Example" src="/images/post.jpg?x=1&amp;y=2">`,
	)
	if got != "/images/post.jpg?x=1&y=2" {
		t.Fatalf("image = %q", got)
	}

	if got := findImageInHTML("<p>No image</p>"); got != "" {
		t.Fatalf("image = %q, want empty", got)
	}
}

func TestPriorityRSSFeedItemImageURL(t *testing.T) {
	item := &gofeed.Item{
		Content: `<p>Body</p><img src="/content.jpg">`,
		Image:   &gofeed.Image{URL: "javascript:bad"},
	}
	feed := &gofeed.Feed{
		Link:  "https://example.com/posts/",
		Image: &gofeed.Image{URL: "/feed.jpg"},
	}

	got := feedItemImageURL(
		item,
		feed,
		"https://fallback.example.com/feed.xml",
	)
	if got != "https://example.com/content.jpg" {
		t.Fatalf(
			"image = %q, want %q",
			got,
			"https://example.com/content.jpg",
		)
	}
}

func TestPriorityRSSFeedItemImageURLPrecedence(t *testing.T) {
	item := &gofeed.Item{
		Content: `<img src="/content.jpg">`,
		Image:   &gofeed.Image{URL: "/item.jpg"},
	}
	feed := &gofeed.Feed{
		Link:  "https://example.com/posts/",
		Image: &gofeed.Image{URL: "/feed.jpg"},
	}

	got := feedItemImageURL(
		item,
		feed,
		"https://fallback.example.com/feed.xml",
	)
	if got != "https://example.com/item.jpg" {
		t.Fatalf(
			"image = %q, want item image %q",
			got,
			"https://example.com/item.jpg",
		)
	}
}

func TestPriorityRSSDescriptionStripsHTMLComments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "full content marker",
			input: "Before <!--FULLCONTENT--> After",
			want:  "Before After",
		},
		{
			name:  "more marker",
			input: "Before <!-- more --> After",
			want:  "Before After",
		},
		{
			name: "multiline comment",
			input: `Before <!--
internal metadata
more metadata
--> After`,
			want: "Before After",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeFeedDescription(tt.input); got != tt.want {
				t.Fatalf(
					"sanitizeFeedDescription() = %q, want %q",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestPriorityRSSItemLinkResolution(t *testing.T) {
	tests := []struct {
		name     string
		itemLink string
		feedLink string
		feedURL  string
		want     string
	}{
		{
			name:     "absolute https",
			itemLink: "https://example.com/posts/one",
			feedLink: "https://example.com/feed.xml",
			feedURL:  "https://fallback.example.com/feed.xml",
			want:     "https://example.com/posts/one",
		},
		{
			name:     "absolute http",
			itemLink: "http://example.com/posts/one",
			feedLink: "https://example.com/feed.xml",
			feedURL:  "https://fallback.example.com/feed.xml",
			want:     "http://example.com/posts/one",
		},
		{
			name:     "root relative",
			itemLink: "/posts/one",
			feedLink: "https://example.com/feed.xml",
			feedURL:  "https://fallback.example.com/feed.xml",
			want:     "https://example.com/posts/one",
		},
		{
			name:     "path relative",
			itemLink: "one",
			feedLink: "https://example.com/posts/feed.xml",
			feedURL:  "https://fallback.example.com/feed.xml",
			want:     "https://example.com/posts/one",
		},
		{
			name:     "fallback to request URL",
			itemLink: "/posts/one",
			feedLink: "",
			feedURL:  "https://fallback.example.com/feed.xml",
			want:     "https://fallback.example.com/posts/one",
		},
		{
			name:     "relative feed link falls back to request URL",
			itemLink: "/2025/",
			feedLink: "/",
			feedURL:  "https://samwho.dev/rss.xml",
			want:     "https://samwho.dev/2025/",
		},
		{
			name:     "schemeless feed link falls back to request URL",
			itemLink: "/posts/one",
			feedLink: "example.com",
			feedURL:  "https://fallback.example.com/feed.xml",
			want:     "https://fallback.example.com/posts/one",
		},
		{
			name:     "feed link without host falls back to request URL",
			itemLink: "/posts/one",
			feedLink: "https:///post",
			feedURL:  "https://fallback.example.com/feed.xml",
			want:     "https://fallback.example.com/posts/one",
		},
		{
			name:     "no absolute base drops relative item link",
			itemLink: "/posts/one",
			feedLink: "/",
			feedURL:  "/rss.xml",
			want:     "",
		},
		{
			name:     "unsafe scheme",
			itemLink: "javascript:alert(1)",
			feedLink: "https://example.com/feed.xml",
			feedURL:  "https://fallback.example.com/feed.xml",
			want:     "",
		},
		{
			name:     "malformed URL",
			itemLink: "https://[::1",
			feedLink: "https://example.com/feed.xml",
			feedURL:  "https://fallback.example.com/feed.xml",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveRSSURL(
				tt.itemLink,
				tt.feedLink,
				tt.feedURL,
			)
			if got != tt.want {
				t.Fatalf(
					"resolveRSSURL() = %q, want %q",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestPriorityRSSUnsafeItemLinkDoesNotRenderZgotmplZ(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0">
<channel>
<title>Unsafe Feed</title>
<link>https://example.com/</link>
<item>
<title>Unsafe Item</title>
<link>javascript:alert(1)</link>
<description>Unsafe link regression</description>
</item>
</channel>
</rss>`))
	}))
	defer server.Close()

	widget := &rssWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	items, err := widget.fetchItemsFromFeedTask(
		t.Context(),
		rssFeedRequest{URL: server.URL},
	)
	if err != nil {
		t.Fatalf("fetchItemsFromFeedTask: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}

	if items[0].Link != "" {
		t.Fatalf("unsafe item link = %q, want empty", items[0].Link)
	}

	widget.Items = items
	rendered := string(widget.Render())

	if strings.Contains(rendered, "ZgotmplZ") {
		t.Fatalf("rendered RSS contains ZgotmplZ: %s", rendered)
	}

	if strings.Contains(rendered, "javascript:") {
		t.Fatalf("rendered RSS contains unsafe scheme: %s", rendered)
	}
}

func TestPriorityRSSItemLinkPrefixValidation(t *testing.T) {
	valid := resolveRSSURL(
		"https://proxy.example/?url=" +
			"https://example.com/post",
	)
	if valid != "https://proxy.example/?url=https://example.com/post" {
		t.Fatalf("valid prefixed link = %q", valid)
	}

	unsafe := resolveRSSURL(
		"javascript:" +
			"https://example.com/post",
	)
	if unsafe != "" {
		t.Fatalf("unsafe prefixed link = %q, want empty", unsafe)
	}
}

func FuzzResolveRSSURL(f *testing.F) {
	for _, seed := range [][3]string{{"", "", ""}, {"https://example.com/post", "https://example.com/feed.xml", "https://fallback.example.com/feed.xml"}, {"/posts/one", "https://example.com/feed.xml", "https://fallback.example.com/feed.xml"}, {"images/item.jpg", "https://example.com/posts/feed.xml", "https://fallback.example.com/feed.xml"}, {"javascript:alert(1)", "https://example.com/feed.xml", "https://fallback.example.com/feed.xml"}, {"https://[::1", "https://example.com/feed.xml", "https://fallback.example.com/feed.xml"}, {"/posts/one", "/relative-feed", "https://fallback.example.com/feed.xml"}, {"/posts/one", "example.com", "/relative-fallback"}} {
		f.Add(seed[0], seed[1], seed[2])
	}

	f.Fuzz(func(t *testing.T, value string, firstBase string, secondBase string) {
		resolved := resolveRSSURL(value, firstBase, secondBase)
		if resolved == "" {
			return
		}
		parsed, err := url.Parse(resolved)
		if err != nil {
			t.Fatalf("resolved RSS URL %q cannot be parsed: %v", resolved, err)
		}
		if !parsed.IsAbs() {
			t.Fatalf("resolved RSS URL %q is not absolute", resolved)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			t.Fatalf("resolved RSS URL %q has unsafe scheme %q", resolved, parsed.Scheme)
		}
	})
}
