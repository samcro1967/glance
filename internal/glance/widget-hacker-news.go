package glance

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type hackerNewsWidget struct {
	widgetBase          `yaml:",inline"`
	Posts               forumPostList `yaml:"-"`
	Limit               int           `yaml:"limit"`
	SortBy              string        `yaml:"sort-by"`
	ExtraSortBy         string        `yaml:"extra-sort-by"`
	CollapseAfter       int           `yaml:"collapse-after"`
	CommentsUrlTemplate string        `yaml:"comments-url-template"`
	ShowThumbnails      bool          `yaml:"-"`
}

func (widget *hackerNewsWidget) initialize() error {
	widget.
		withTitle("Hacker News").
		withTitleURL("https://news.ycombinator.com/").
		withCacheDuration(30 * time.Minute)

	if widget.Limit <= 0 {
		widget.Limit = 15
	}

	if widget.CollapseAfter == 0 || widget.CollapseAfter < -1 {
		widget.CollapseAfter = 5
	}

	if widget.SortBy != "top" && widget.SortBy != "new" && widget.SortBy != "best" {
		widget.SortBy = "top"
	}

	return nil
}

func (widget *hackerNewsWidget) update(ctx context.Context) {
	posts, err := fetchHackerNewsPosts(ctx, widget.SortBy, 40, widget.CommentsUrlTemplate)

	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	if widget.ExtraSortBy == "engagement" {
		posts.calculateEngagement()
		posts.sortByEngagement()
	}

	if widget.Limit < len(posts) {
		posts = posts[:widget.Limit]
	}

	widget.Posts = posts
}

func (widget *hackerNewsWidget) Render() template.HTML {
	return widget.renderTemplate(widget, forumPostsTemplate)
}

type hackerNewsPostResponseJson struct {
	Id           int    `json:"id"`
	Score        int    `json:"score"`
	Title        string `json:"title"`
	TargetUrl    string `json:"url,omitempty"`
	CommentCount int    `json:"descendants"`
	TimePosted   int64  `json:"time"`
}

func fetchHackerNewsPostIds(ctx context.Context, sort string) ([]int, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("https://hacker-news.firebaseio.com/v0/%sstories.json", sort),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: creating Hacker News post list request: %w",
			errNoContent,
			err,
		)
	}

	response, err := decodeJsonFromRequest[[]int](defaultHTTPClient, request)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: fetching Hacker News post IDs: %w",
			errNoContent,
			err,
		)
	}

	return response, nil
}

func fetchHackerNewsPostsFromIds(ctx context.Context, postIds []int, commentsUrlTemplate string) (forumPostList, error) {
	requests := make([]*http.Request, 0, len(postIds))

	for _, id := range postIds {
		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", id),
			nil,
		)
		if err != nil {
			return nil, contentFetchError(
				errNoContent,
				1,
				len(postIds),
				"Hacker News posts",
				fmt.Errorf("creating Hacker News post request: %w", err),
			)
		}

		requests = append(requests, request)
	}

	task := decodeJsonFromRequestTask[hackerNewsPostResponseJson](defaultHTTPClient)
	job := newJob(task, requests).withWorkers(30).withContext(ctx)
	results, errs, err := workerPoolDo(job)
	if err != nil {
		return nil, fmt.Errorf("%w: fetching Hacker News posts: %w", errNoContent, err)
	}

	posts := make(forumPostList, 0, len(postIds))
	failed := 0
	var firstFailure error

	for i := range results {
		if errs[i] != nil {
			failed++

			if firstFailure == nil {
				firstFailure = errs[i]
			}

			continue
		}

		var commentsUrl string

		if commentsUrlTemplate == "" {
			commentsUrl = "https://news.ycombinator.com/item?id=" + strconv.Itoa(results[i].Id)
		} else {
			commentsUrl = strings.ReplaceAll(commentsUrlTemplate, "{POST-ID}", strconv.Itoa(results[i].Id))
		}

		posts = append(posts, forumPost{
			Title:           results[i].Title,
			DiscussionUrl:   commentsUrl,
			TargetUrl:       results[i].TargetUrl,
			TargetUrlDomain: extractDomainFromUrl(results[i].TargetUrl),
			CommentCount:    results[i].CommentCount,
			Score:           results[i].Score,
			TimePosted:      time.Unix(results[i].TimePosted, 0),
		})
	}

	if len(posts) == 0 {
		return nil, contentFetchError(
			errNoContent,
			failed,
			len(postIds),
			"Hacker News posts",
			firstFailure,
		)
	}

	if failed > 0 {
		return posts, contentFetchError(
			errPartialContent,
			failed,
			len(postIds),
			"Hacker News posts",
			firstFailure,
		)
	}

	return posts, nil
}

func fetchHackerNewsPosts(ctx context.Context, sort string, limit int, commentsUrlTemplate string) (forumPostList, error) {
	postIds, err := fetchHackerNewsPostIds(ctx, sort)
	if err != nil {
		return nil, err
	}

	if len(postIds) > limit {
		postIds = postIds[:limit]
	}

	return fetchHackerNewsPostsFromIds(ctx, postIds, commentsUrlTemplate)
}
