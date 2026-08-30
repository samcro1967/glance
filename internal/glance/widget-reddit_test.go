package glance

import (
	"strings"
	"testing"
	"time"
)

func TestRedditWidgetInitializeRequiresSubreddit(t *testing.T) {
	widget := &redditWidget{}

	err := widget.initialize()
	if err == nil {
		t.Fatal("expected missing subreddit error")
	}

	if err.Error() != "subreddit is required" {
		t.Fatalf("error = %q, want %q", err, "subreddit is required")
	}
}

func TestRedditWidgetInitializeDefaults(t *testing.T) {
	widget := &redditWidget{
		Subreddit: "example",
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if widget.Limit != 15 {
		t.Fatalf("limit = %d, want 15", widget.Limit)
	}

	if widget.CollapseAfter != 5 {
		t.Fatalf("collapse after = %d, want 5", widget.CollapseAfter)
	}

	if widget.SortBy != "hot" {
		t.Fatalf("sort by = %q, want %q", widget.SortBy, "hot")
	}

	if widget.TopPeriod != "day" {
		t.Fatalf("top period = %q, want %q", widget.TopPeriod, "day")
	}

	if widget.Title != "r/example" {
		t.Fatalf("title = %q, want %q", widget.Title, "r/example")
	}

	if widget.TitleURL != "https://www.reddit.com/r/example/" {
		t.Fatalf(
			"title URL = %q, want %q",
			widget.TitleURL,
			"https://www.reddit.com/r/example/",
		)
	}

	if widget.cacheDuration != 30*time.Minute {
		t.Fatalf(
			"cache duration = %s, want %s",
			widget.cacheDuration,
			30*time.Minute,
		)
	}
}

func TestRedditWidgetInitializePreservesValidOptions(t *testing.T) {
	widget := &redditWidget{
		Subreddit:     "example",
		Limit:         30,
		CollapseAfter: -1,
		SortBy:        "top",
		TopPeriod:     "year",
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if widget.Limit != 30 {
		t.Fatalf("limit = %d, want 30", widget.Limit)
	}

	if widget.CollapseAfter != -1 {
		t.Fatalf("collapse after = %d, want -1", widget.CollapseAfter)
	}

	if widget.SortBy != "top" {
		t.Fatalf("sort by = %q, want %q", widget.SortBy, "top")
	}

	if widget.TopPeriod != "year" {
		t.Fatalf("top period = %q, want %q", widget.TopPeriod, "year")
	}
}

func TestRedditWidgetInitializeNormalizesInvalidOptions(t *testing.T) {
	widget := &redditWidget{
		Subreddit:     "example",
		Limit:         -5,
		CollapseAfter: -2,
		SortBy:        "invalid",
		TopPeriod:     "invalid",
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if widget.Limit != 15 {
		t.Fatalf("limit = %d, want 15", widget.Limit)
	}

	if widget.CollapseAfter != 5 {
		t.Fatalf("collapse after = %d, want 5", widget.CollapseAfter)
	}

	if widget.SortBy != "hot" {
		t.Fatalf("sort by = %q, want %q", widget.SortBy, "hot")
	}

	if widget.TopPeriod != "day" {
		t.Fatalf("top period = %q, want %q", widget.TopPeriod, "day")
	}
}

func TestRedditWidgetInitializeAcceptsValidSortValues(t *testing.T) {
	validSorts := []string{
		"hot",
		"new",
		"top",
		"rising",
	}

	for _, sortBy := range validSorts {
		t.Run(sortBy, func(t *testing.T) {
			widget := &redditWidget{
				Subreddit: "example",
				SortBy:    sortBy,
			}

			if err := widget.initialize(); err != nil {
				t.Fatalf("unexpected initialization error: %v", err)
			}

			if widget.SortBy != sortBy {
				t.Fatalf("sort by = %q, want %q", widget.SortBy, sortBy)
			}
		})
	}
}

func TestRedditWidgetInitializeAcceptsValidTopPeriods(t *testing.T) {
	validPeriods := []string{
		"hour",
		"day",
		"week",
		"month",
		"year",
		"all",
	}

	for _, period := range validPeriods {
		t.Run(period, func(t *testing.T) {
			widget := &redditWidget{
				Subreddit: "example",
				SortBy:    "top",
				TopPeriod: period,
			}

			if err := widget.initialize(); err != nil {
				t.Fatalf("unexpected initialization error: %v", err)
			}

			if widget.TopPeriod != period {
				t.Fatalf("top period = %q, want %q", widget.TopPeriod, period)
			}
		})
	}
}

func TestRedditWidgetInitializeValidatesRequestURLTemplate(t *testing.T) {
	widget := &redditWidget{
		Subreddit:          "example",
		RequestURLTemplate: "https://proxy.example.invalid/reddit",
	}

	err := widget.initialize()
	if err == nil {
		t.Fatal("expected request URL template validation error")
	}

	const want = "no `{REQUEST-URL}` placeholder specified"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

func TestRedditWidgetInitializeAcceptsRequestURLTemplate(t *testing.T) {
	widget := &redditWidget{
		Subreddit: "example",
		RequestURLTemplate: strings.Join(
			[]string{
				"https://proxy.example.invalid/fetch?url=",
				"{REQUEST-URL}",
			},
			"",
		),
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}
}

func TestRedditWidgetInitializeRequiresCompleteAppAuth(t *testing.T) {
	tests := []struct {
		name   string
		app    string
		id     string
		secret string
	}{
		{
			name: "name only",
			app:  "test-app",
		},
		{
			name: "ID only",
			id:   "test-id",
		},
		{
			name:   "secret only",
			secret: "test-secret",
		},
		{
			name: "missing secret",
			app:  "test-app",
			id:   "test-id",
		},
		{
			name:   "missing ID",
			app:    "test-app",
			secret: "test-secret",
		},
		{
			name:   "missing name",
			id:     "test-id",
			secret: "test-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			widget := &redditWidget{
				Subreddit: "example",
			}
			widget.AppAuth.Name = tt.app
			widget.AppAuth.ID = tt.id
			widget.AppAuth.Secret = tt.secret

			err := widget.initialize()
			if err == nil {
				t.Fatal("expected incomplete application authentication error")
			}

			const want = "application name, client ID and client secret are required"
			if err.Error() != want {
				t.Fatalf("error = %q, want %q", err, want)
			}

			if widget.AppAuth.enabled {
				t.Fatal("incomplete application authentication should not be enabled")
			}
		})
	}
}

func TestRedditWidgetInitializeEnablesCompleteAppAuth(t *testing.T) {
	widget := &redditWidget{
		Subreddit: "example",
	}
	widget.AppAuth.Name = "test-app"
	widget.AppAuth.ID = "test-id"
	widget.AppAuth.Secret = "test-secret"

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if !widget.AppAuth.enabled {
		t.Fatal("expected complete application authentication to be enabled")
	}
}

func TestRedditWidgetParseCustomCommentsURL(t *testing.T) {
	widget := &redditWidget{
		CommentsURLTemplate: "https://comments.example.invalid/{SUBREDDIT}/{POST-ID}/{POST-PATH}",
	}

	got := widget.parseCustomCommentsURL(
		"example",
		"abc123",
		"/r/example/comments/abc123/example_post/",
	)

	const want = "https://comments.example.invalid/example/abc123/r/example/comments/abc123/example_post/"

	if got != want {
		t.Fatalf("custom comments URL = %q, want %q", got, want)
	}
}

func TestRedditWidgetParseCustomCommentsURLReplacesRepeatedPlaceholders(t *testing.T) {
	widget := &redditWidget{
		CommentsURLTemplate: "{SUBREDDIT}/{SUBREDDIT}/{POST-ID}/{POST-ID}/{POST-PATH}",
	}

	got := widget.parseCustomCommentsURL(
		"example",
		"abc123",
		"///comments/abc123/",
	)

	const want = "example/example/abc123/abc123/comments/abc123/"

	if got != want {
		t.Fatalf("custom comments URL = %q, want %q", got, want)
	}
}
