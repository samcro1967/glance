package glance

import (
	"net/http"
	"net/http/httptest"
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
