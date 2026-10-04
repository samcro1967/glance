package glance

import (
	"strings"
	"testing"
	"time"

	"github.com/samcro1967/glance/pkg/sysinfo"
)

func TestNativeWidgetsRenderExpandedViews(t *testing.T) {
	now := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.Local)
	calendar := &calendarWidget{now: func() time.Time { return now }, Events: []icsEvent{{Title: "Future event", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)}}}
	server := &serverStatsWidget{Servers: []serverStatsRequest{{Name: "osu4", IsReachable: true, Info: &sysinfo.SystemInfo{Hostname: "osu4"}}}}
	docker := &dockerContainersWidget{Containers: dockerContainerList{{Name: "glance", Image: "glance:test", StateText: "Up"}}}
	astronomy := &astronomyWidget{ShowMoon: true, Observer: astronomyObserver{Label: "Test"}, Snapshot: astronomySnapshot{At: now, Moon: astronomyMoon{PhaseName: "Full Moon"}}}
	markets := &marketsWidget{Markets: marketList{{marketRequest: marketRequest{Symbol: "TEST"}, Name: "Test Market", Currency: "USD", Price: 10, SvgChartPoints: "0,50 100,0"}}}
	latest := &latestMediaWidget{Items: []mediaItem{{Title: "Latest title", Summary: "Summary"}}}
	playing := &nowPlayingWidget{Items: []nowPlayingItem{{Title: "Playing title", State: "playing"}}}
	history := &mediaHistoryWidget{Items: []mediaItem{{Title: "History title"}}}
	seerr := &seerrWidget{View: "trending", Items: []seerrItem{{Title: "Seerr title"}}}
	apod := &nasaAPODWidget{APOD: nasaAPOD{MediaType: "image", Title: "APOD title", HDURL: "https://example.com/apod.jpg", Permalink: "https://example.com/apod", Explanation: "Explanation"}}
	releases := &releasesWidget{Releases: appReleaseList{{Name: "repo", Version: "v1", NotesUrl: "https://example.com/release", TimeReleased: now}}}
	arr := &arrWidget{View: "upcoming", Items: []arrItem{{Title: "ARR title", Summary: "ARR summary"}}}
	monitor := &monitorWidget{Sites: []monitorSite{{Title: "Glance", URL: "https://example.com", Status: &siteStatus{Code: 200, ResponseTime: 12 * time.Millisecond}, StatusText: "OK", StatusStyle: "ok"}}}
	prometheus := &prometheusWidget{ShowValue: true, Graph: &prometheusGraph{LatestValue: "42", MinimumValue: "10", MaximumValue: "50", Polyline: "0,100 1000,0"}}
	torrenting := &torrentingWidget{Torrents: []torrentRecord{{Name: "Linux ISO", StateLabel: "Downloading", ProgressText: "50%", Downloaded: "1 GB", Size: "2 GB", ETA: "10m", Active: true}}}
	repository := &repositoryWidget{Repository: repository{Name: "samcro1967/glance", Stars: 10, Commits: []githubCommitDetails{{Sha: "abc", Author: "Test", CreatedAt: now, Message: "Expanded repository"}}}}
	changeDetection := &changeDetectionWidget{ChangeDetections: changeDetectionWatchList{{Title: "Changed page", URL: "https://example.com", DiffURL: "https://example.com/diff", PreviousHash: "abc", LastChanged: now}}}
	dnsStats := &dnsStatsWidget{Stats: &dnsStats{TotalQueries: 100, BlockedQueries: 20, BlockedPercent: 20, TopBlockedDomains: []dnsStatsBlockedDomain{{Domain: "ads.example", PercentBlocked: 50}}}}
	videos := &videosWidget{Videos: videoList{{Title: "Expanded video", Url: "https://example.com/video", Author: "Channel", AuthorUrl: "https://example.com/channel", TimePosted: now}}}

	cases := []struct {
		name   string
		widget expandedWidget
		want   string
	}{
		{"calendar", calendar, "Future event"},
		{"server stats", server, "osu4"},
		{"docker containers", docker, "glance:test"},
		{"astronomy", astronomy, "Full Moon"},
		{"markets", markets, "Test Market"},
		{"latest media", latest, "Latest title"},
		{"now playing", playing, "Playing title"},
		{"media history", history, "History title"},
		{"seerr", seerr, "Seerr title"},
		{"nasa apod", apod, "Explanation"},
		{"releases", releases, "v1"},
		{"arr", arr, "ARR summary"},
		{"monitor", monitor, "Glance"},
		{"prometheus", prometheus, "Maximum"},
		{"torrenting", torrenting, "Linux ISO"},
		{"repository", repository, "Expanded repository"},
		{"change detection", changeDetection, "Changed page"},
		{"dns stats", dnsStats, "ads.example"},
		{"videos", videos, "Expanded video"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rendered := string(tc.widget.RenderExpanded())
			if !strings.Contains(rendered, tc.want) {
				t.Fatalf("expanded view missing %q: %s", tc.want, rendered)
			}
		})
	}
}

func TestCalendarExpandedAgendaExcludesPastEvents(t *testing.T) {
	now := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.Local)
	widget := &calendarWidget{
		now: func() time.Time { return now },
		Events: []icsEvent{
			{Title: "Past", Start: now.Add(-2 * time.Hour), End: now.Add(-time.Hour)},
			{Title: "Upcoming", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)},
		},
	}
	rendered := string(widget.RenderExpanded())
	if strings.Contains(rendered, "Past") || !strings.Contains(rendered, "Upcoming") {
		t.Fatalf("unexpected expanded agenda: %s", rendered)
	}
}
