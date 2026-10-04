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
