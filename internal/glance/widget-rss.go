package glance

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	gofeedext "github.com/mmcdole/gofeed/extensions"
)

var (
	rssWidgetTemplate                 = mustParseTemplate("rss-list.html", "widget-base.html")
	rssWidgetDetailedListTemplate     = mustParseTemplate("rss-detailed-list.html", "widget-base.html")
	rssWidgetHorizontalCardsTemplate  = mustParseTemplate("rss-horizontal-cards.html", "widget-base.html")
	rssWidgetHorizontalCards2Template = mustParseTemplate("rss-horizontal-cards-2.html", "widget-base.html")
)

var feedParser = gofeed.NewParser()

type rssWidget struct {
	widgetBase       `yaml:",inline"`
	FeedRequests     []rssFeedRequest `yaml:"feeds"`
	Style            string           `yaml:"style"`
	ThumbnailHeight  float64          `yaml:"thumbnail-height"`
	CardHeight       float64          `yaml:"card-height"`
	Limit            int              `yaml:"limit"`
	CollapseAfter    int              `yaml:"collapse-after"`
	SingleLineTitles bool             `yaml:"single-line-titles"`
	PreserveOrder    bool             `yaml:"preserve-order"`

	Items          rssFeedItemList `yaml:"-"`
	NoItemsMessage string          `yaml:"-"`

	cachedFeedsMutex sync.Mutex
	cachedFeeds      map[string]*cachedRSSFeed `yaml:"-"`
}

func (widget *rssWidget) initialize() error {
	widget.withTitle("RSS Feed").withCacheDuration(2 * time.Hour)

	if widget.Limit <= 0 {
		widget.Limit = 25
	}

	if widget.CollapseAfter == 0 || widget.CollapseAfter < -1 {
		widget.CollapseAfter = 5
	}

	if widget.ThumbnailHeight < 0 {
		widget.ThumbnailHeight = 0
	}

	if widget.CardHeight < 0 {
		widget.CardHeight = 0
	}

	if widget.Style == "detailed-list" {
		for i := range widget.FeedRequests {
			widget.FeedRequests[i].IsDetailed = true
		}
	}

	widget.NoItemsMessage = "No items were returned from the feeds."
	widget.cachedFeeds = make(map[string]*cachedRSSFeed)

	return nil
}

func (widget *rssWidget) update(ctx context.Context) {
	items, err := widget.fetchItemsFromFeeds(ctx)

	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	if !widget.PreserveOrder {
		items.sortByNewest()
	}

	if len(items) > widget.Limit {
		items = items[:widget.Limit]
	}

	widget.Items = items
}

func (widget *rssWidget) Render() template.HTML {
	if widget.Style == "horizontal-cards" {
		return widget.renderTemplate(widget, rssWidgetHorizontalCardsTemplate)
	}

	if widget.Style == "horizontal-cards-2" {
		return widget.renderTemplate(widget, rssWidgetHorizontalCards2Template)
	}

	if widget.Style == "detailed-list" {
		return widget.renderTemplate(widget, rssWidgetDetailedListTemplate)
	}

	return widget.renderTemplate(widget, rssWidgetTemplate)
}

type cachedRSSFeed struct {
	etag         string
	lastModified string
	items        []rssFeedItem
}

type rssFeedItem struct {
	ChannelName string
	ChannelURL  string
	Title       string
	Link        string
	ImageURL    string
	Categories  []string
	Description string
	PublishedAt time.Time
}

type rssFeedRequest struct {
	URL             string            `yaml:"url"`
	Title           string            `yaml:"title"`
	HideCategories  bool              `yaml:"hide-categories"`
	HideDescription bool              `yaml:"hide-description"`
	Limit           int               `yaml:"limit"`
	ItemLinkPrefix  string            `yaml:"item-link-prefix"`
	Headers         map[string]string `yaml:"headers"`
	IsDetailed      bool              `yaml:"-"`
}

type rssFeedItemList []rssFeedItem

func (f rssFeedItemList) sortByNewest() rssFeedItemList {
	sort.Slice(f, func(i, j int) bool {
		return f[i].PublishedAt.After(f[j].PublishedAt)
	})

	return f
}

func (widget *rssWidget) fetchItemsFromFeeds(ctx context.Context) (rssFeedItemList, error) {
	requests := widget.FeedRequests

	task := func(request rssFeedRequest) ([]rssFeedItem, error) {
		return widget.fetchItemsFromFeedTask(ctx, request)
	}

	job := newJob(task, requests).withWorkers(30).withContext(ctx)
	feeds, errs, err := workerPoolDo(job)
	if err != nil {
		return nil, fmt.Errorf("%w: fetching RSS feeds: %w", errNoContent, err)
	}

	failed := 0
	var firstFailure error
	entries := make(rssFeedItemList, 0, len(feeds)*10)
	seen := make(map[string]struct{})

	for i := range feeds {
		if errs[i] != nil {
			failed++

			if firstFailure == nil {
				firstFailure = errs[i]
			}

			continue
		}

		for _, item := range feeds[i] {
			if _, exists := seen[item.Link]; exists {
				continue
			}
			entries = append(entries, item)
			seen[item.Link] = struct{}{}
		}
	}

	if failed == len(requests) {
		return nil, contentFetchError(
			errNoContent,
			failed,
			len(requests),
			"RSS feeds",
			firstFailure,
		)
	}

	if failed > 0 {
		return entries, contentFetchError(
			errPartialContent,
			failed,
			len(requests),
			"RSS feeds",
			firstFailure,
		)
	}

	return entries, nil
}

func (widget *rssWidget) fetchItemsFromFeedTask(ctx context.Context, request rssFeedRequest) ([]rssFeedItem, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", request.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating RSS request: %w", err)
	}

	req.Header.Add("User-Agent", glanceUserAgentString)

	widget.cachedFeedsMutex.Lock()
	cache, isCached := widget.cachedFeeds[request.URL]
	if isCached {
		if cache.etag != "" {
			req.Header.Add("If-None-Match", cache.etag)
		}
		if cache.lastModified != "" {
			req.Header.Add("If-Modified-Since", cache.lastModified)
		}
	}
	widget.cachedFeedsMutex.Unlock()

	for key, value := range request.Headers {
		req.Header.Set(key, value)
	}

	resource, err := fetchRSSResource(ctx, req)
	if err != nil {
		return nil, err
	}

	if resource.StatusCode == http.StatusNotModified && isCached {
		return cache.items, nil
	}

	if resource.StatusCode != http.StatusOK {
		return nil, unexpectedHTTPStatusError(&http.Response{
			Status:     resource.Status,
			StatusCode: resource.StatusCode,
		})
	}

	feed, err := feedParser.ParseString(string(resource.Body))
	if err != nil {
		return nil, fmt.Errorf("parsing RSS response: %w", err)
	}

	if request.Limit > 0 && len(feed.Items) > request.Limit {
		feed.Items = feed.Items[:request.Limit]
	}

	items := make(rssFeedItemList, 0, len(feed.Items))

	for i := range feed.Items {
		item := feed.Items[i]

		rssItem := rssFeedItem{
			ChannelURL: feed.Link,
		}

		if request.ItemLinkPrefix != "" {
			rssItem.Link = resolveRSSURL(request.ItemLinkPrefix + item.Link)
		} else {
			rssItem.Link = resolveRSSURL(item.Link, feed.Link, request.URL)
		}

		if item.Title != "" {
			rssItem.Title = sanitizeFeedTitle(item.Title)
		} else {
			rssItem.Title = shortenFeedDescriptionLen(item.Description, 100)
		}

		if request.IsDetailed {
			if !request.HideDescription && item.Description != "" && item.Title != "" {
				rssItem.Description = shortenFeedDescriptionLen(item.Description, 200)
			}

			if !request.HideCategories {
				var categories = make([]string, 0, 6)

				for _, category := range item.Categories {
					if len(categories) == 6 {
						break
					}

					if len(category) == 0 || len(category) > 30 {
						continue
					}

					categories = append(categories, category)
				}

				rssItem.Categories = categories
			}
		}

		if request.Title != "" {
			rssItem.ChannelName = request.Title
		} else {
			rssItem.ChannelName = feed.Title
		}

		rssItem.ImageURL = feedItemImageURL(item, feed, request.URL)

		if item.PublishedParsed != nil {
			rssItem.PublishedAt = *item.PublishedParsed
		} else {
			rssItem.PublishedAt = time.Now()
		}

		items = append(items, rssItem)
	}

	if resource.Header.Get("ETag") != "" || resource.Header.Get("Last-Modified") != "" {
		widget.cachedFeedsMutex.Lock()
		widget.cachedFeeds[request.URL] = &cachedRSSFeed{
			etag:         resource.Header.Get("ETag"),
			lastModified: resource.Header.Get("Last-Modified"),
			items:        items,
		}
		widget.cachedFeedsMutex.Unlock()
	}

	return items, nil
}

func findThumbnailInItemExtensions(item *gofeed.Item) string {
	media, ok := item.Extensions["media"]

	if !ok {
		return ""
	}

	return recursiveFindThumbnailInExtensions(media)
}

func sanitizeFeedTitle(title string) string {
	return sanitizeFeedDescription(title)
}

func feedItemImageURL(item *gofeed.Item, feed *gofeed.Feed, feedURL string) string {
	candidates := make([]string, 0, 4)

	if item.Image != nil && item.Image.URL != "" {
		candidates = append(candidates, item.Image.URL)
	}

	if thumbnail := findThumbnailInItemExtensions(item); thumbnail != "" {
		candidates = append(candidates, thumbnail)
	}

	if contentImage := findImageInHTML(item.Content); contentImage != "" {
		candidates = append(candidates, contentImage)
	}

	if feed.Image != nil && feed.Image.URL != "" {
		candidates = append(candidates, feed.Image.URL)
	}

	for _, candidate := range candidates {
		if resolved := resolveRSSURL(candidate, feed.Link, feedURL); resolved != "" {
			return resolved
		}
	}

	return ""
}

func resolveRSSURL(value string, bases ...string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.String() == "" {
		return ""
	}

	if parsed.IsAbs() {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return ""
		}

		return parsed.String()
	}

	for _, base := range bases {
		parsedBase, err := url.Parse(base)
		if err != nil || !parsedBase.IsAbs() {
			continue
		}

		if parsedBase.Scheme != "http" && parsedBase.Scheme != "https" {
			continue
		}

		return parsedBase.ResolveReference(parsed).String()
	}

	return ""
}

var firstHTMLImagePattern = regexp.MustCompile(`(?i)<img[^>]+src=["']([^"']+)["']`)

func findImageInHTML(content string) string {
	match := firstHTMLImagePattern.FindStringSubmatch(content)
	if len(match) != 2 {
		return ""
	}

	return html.UnescapeString(match[1])
}

func recursiveFindThumbnailInExtensions(extensions map[string][]gofeedext.Extension) string {
	for _, exts := range extensions {
		for _, ext := range exts {
			if ext.Name == "thumbnail" || ext.Name == "image" {
				if url, ok := ext.Attrs["url"]; ok {
					return url
				}
			}

			if ext.Children != nil {
				if url := recursiveFindThumbnailInExtensions(ext.Children); url != "" {
					return url
				}
			}
		}
	}

	return ""
}

var htmlCommentPattern = regexp.MustCompile(`(?s)<!--.*?-->`)

var htmlTagsWithAttributesPattern = regexp.MustCompile(`<\/?[a-zA-Z0-9-]+ *(?:[a-zA-Z-]+=(?:"|').*?(?:"|') ?)* *\/?>`)

func sanitizeFeedDescription(description string) string {
	if description == "" {
		return ""
	}

	description = htmlCommentPattern.ReplaceAllString(description, "")
	description = strings.ReplaceAll(description, "\n", " ")
	description = htmlTagsWithAttributesPattern.ReplaceAllString(description, "")
	description = sequentialWhitespacePattern.ReplaceAllString(description, " ")
	description = strings.TrimSpace(description)
	description = html.UnescapeString(description)

	return description
}

func shortenFeedDescriptionLen(description string, maxLen int) string {
	description, _ = limitStringLength(description, 1000)
	description = sanitizeFeedDescription(description)
	description, limited := limitStringLength(description, maxLen)

	if limited {
		description += "…"
	}

	return description
}
