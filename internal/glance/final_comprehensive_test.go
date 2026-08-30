package glance

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func finalWaveResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func finalWaveTransport(t *testing.T, fn priorityRoundTripper) {
	t.Helper()
	old := defaultHTTPClient.Transport
	defaultHTTPClient.Transport = fn
	t.Cleanup(func() { defaultHTTPClient.Transport = old })
}

func TestComprehensiveFinalRepositoryInitializeAndFetch(t *testing.T) {
	w := &repositoryWidget{}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if w.Title != "Repository" || w.PullRequestsLimit != 3 || w.IssuesLimit != 3 || w.CommitsLimit != -1 {
		t.Fatalf("defaults=%#v", w)
	}
	finalWaveTransport(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Path == "/repos/example/project":
			return finalWaveResponse(200, `{"full_name":"example/project","stargazers_count":7,"forks_count":2}`), nil
		case strings.Contains(r.URL.Path, "/search/issues") && strings.Contains(r.URL.Query().Get("q"), "is:pr"):
			return finalWaveResponse(200, `{"total_count":1,"items":[{"number":4,"created_at":"2026-08-01T00:00:00Z","title":"PR"}]}`), nil
		case strings.Contains(r.URL.Path, "/search/issues"):
			return finalWaveResponse(200, `{"total_count":2,"items":[{"number":5,"created_at":"2026-08-02T00:00:00Z","title":"Issue"}]}`), nil
		case strings.HasSuffix(r.URL.Path, "/commits"):
			return finalWaveResponse(200, `[{"sha":"abcdef","commit":{"author":{"name":"Dev","date":"2026-08-03T00:00:00Z"},"message":"change"}}]`), nil
		default:
			return finalWaveResponse(404, `{}`), nil
		}
	})
	repo, err := fetchRepositoryDetailsFromGithub(context.Background(), "example/project", "token", 3, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Name != "example/project" || repo.Stars != 7 || repo.Forks != 2 || repo.OpenPullRequests != 1 || repo.OpenIssues != 2 || len(repo.Commits) != 1 {
		t.Fatalf("repo=%#v", repo)
	}
}

func TestComprehensiveFinalRepositoryFetchFailure(t *testing.T) {
	finalWaveTransport(t, func(r *http.Request) (*http.Response, error) { return finalWaveResponse(500, "bad"), nil })
	_, err := fetchRepositoryDetailsFromGithub(context.Background(), "example/project", "", 1, 1, 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestComprehensiveFinalTwitchInitializationAndSorting(t *testing.T) {
	c := &twitchChannelsWidget{}
	if err := c.initialize(); err != nil {
		t.Fatal(err)
	}
	if c.CollapseAfter != 5 || c.SortBy != "viewers" {
		t.Fatalf("channels=%#v", c)
	}
	list := twitchChannelList{{Name: "off", ViewersCount: 100}, {Name: "low", IsLive: true, ViewersCount: 2}, {Name: "high", IsLive: true, ViewersCount: 20}}
	list.sortByViewers()
	if list[0].Name != "off" {
		t.Fatalf("viewers=%#v", list)
	}
	list.sortByLive()
	if !list[0].IsLive || !list[1].IsLive {
		t.Fatalf("live=%#v", list)
	}
	g := &twitchGamesWidget{}
	if err := g.initialize(); err != nil {
		t.Fatal(err)
	}
	if g.Limit <= 0 {
		t.Fatalf("games=%#v", g)
	}
}

func TestComprehensiveFinalVideosInitializeSortAndTime(t *testing.T) {
	w := &videosWidget{Channels: []string{"UC1"}, Playlists: []string{"PL1"}}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if w.Limit != 25 || len(w.Channels) != 2 || w.Channels[1] != "playlist:PL1" {
		t.Fatalf("videos=%#v", w)
	}

	ts := parseYoutubeFeedTime("2026-08-30T12:34:56+00:00")
	if ts.Year() != 2026 {
		t.Fatalf("time=%v", ts)
	}

	before := time.Now()
	fallback := parseYoutubeFeedTime("bad")
	after := time.Now()

	if fallback.Before(before) || fallback.After(after) {
		t.Fatalf("invalid timestamp fallback=%v, expected between %v and %v", fallback, before, after)
	}
}

func TestComprehensiveFinalRSSSortAndRenderStyles(t *testing.T) {
	now := time.Now()
	items := rssFeedItemList{{Title: "old", PublishedAt: now.Add(-time.Hour)}, {Title: "new", PublishedAt: now}}
	items.sortByNewest()
	if items[0].Title != "new" {
		t.Fatalf("items=%#v", items)
	}
	for _, style := range []string{"", "horizontal-cards", "horizontal-cards-2", "detailed-list"} {
		w := &rssWidget{Style: style}
		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}
		if got := w.Render(); len(got) == 0 {
			t.Fatalf("empty render style=%q", style)
		}
	}
}

func TestComprehensiveFinalReleaseSortInitializeAndRender(t *testing.T) {
	now := time.Now()
	r := appReleaseList{{Name: "old", TimeReleased: now.Add(-time.Hour)}, {Name: "new", TimeReleased: now}}
	r.sortByNewest()
	if r[0].Name != "new" {
		t.Fatalf("releases=%#v", r)
	}
	w := &releasesWidget{}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if w.Limit != 10 || w.CollapseAfter != 5 {
		t.Fatalf("defaults=%#v", w)
	}
	if len(w.Render()) == 0 {
		t.Fatal("empty render")
	}
}

func TestComprehensiveFinalSimpleWidgetRenders(t *testing.T) {
	tests := []struct {
		name   string
		render func() int
	}{
		{"monitor", func() int { w := &monitorWidget{}; _ = w.initialize(); return len(w.Render()) }},
		{"server", func() int { w := &serverStatsWidget{}; _ = w.initialize(); return len(w.Render()) }},
		{"change", func() int { w := &changeDetectionWidget{}; _ = w.initialize(); return len(w.Render()) }},
		{"extension", func() int {
			w := &extensionWidget{}
			_ = w.initialize()
			w.cachedHTML = "cached"
			return len(w.Render())
		}},
		{"docker", func() int { w := &dockerContainersWidget{}; _ = w.initialize(); return len(w.Render()) }},
		{"repository", func() int { w := &repositoryWidget{}; _ = w.initialize(); return len(w.Render()) }},
		{"twitch-channels", func() int { w := &twitchChannelsWidget{}; _ = w.initialize(); return len(w.Render()) }},
		{"twitch-games", func() int { w := &twitchGamesWidget{}; _ = w.initialize(); return len(w.Render()) }},
		{"videos", func() int { w := &videosWidget{}; _ = w.initialize(); return len(w.Render()) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.render() == 0 {
				t.Fatal("empty render")
			}
		})
	}
}
