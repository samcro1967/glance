package glance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedCalendarTestTime() time.Time {
	return time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
}

func TestCalendarStaticLifecycle(t *testing.T) {
	widget := &calendarWidget{}

	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	if widget.cacheType != cacheTypeInfinite {
		t.Fatalf("cache type = %v, want infinite", widget.cacheType)
	}

	if widget.cachedSources != nil {
		t.Fatal("static calendar unexpectedly initialized ICS cache")
	}

	if widget.now != nil {
		t.Fatal("static calendar unexpectedly initialized clock")
	}

	if len(widget.Render()) == 0 {
		t.Fatal("static calendar did not render")
	}
}

func TestCalendarSourcedLifecycle(t *testing.T) {
	widget := &calendarWidget{
		Sources: []icsEventSource{
			{URL: "https://example.invalid/calendar.ics"},
		},
	}

	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	if widget.cacheType != cacheTypeDuration {
		t.Fatalf("cache type = %v, want duration", widget.cacheType)
	}

	if widget.cacheDuration != 30*time.Minute {
		t.Fatalf("cache duration = %v, want 30m", widget.cacheDuration)
	}

	if widget.cachedSources == nil {
		t.Fatal("sourced calendar did not initialize ICS cache")
	}

	if widget.now == nil {
		t.Fatal("sourced calendar did not initialize clock")
	}
}

func TestCalendarSourcedCustomCache(t *testing.T) {
	widget := &calendarWidget{
		Sources: []icsEventSource{
			{URL: "https://example.invalid/calendar.ics"},
		},
	}
	widget.CustomCacheDuration = durationField(15 * time.Minute)

	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	if widget.cacheDuration != 15*time.Minute {
		t.Fatalf("cache duration = %v, want 15m", widget.cacheDuration)
	}
}

func TestCalendarSourceValidation(t *testing.T) {
	tests := []struct {
		name    string
		source  icsEventSource
		wantErr bool
	}{
		{
			name:   "url",
			source: icsEventSource{URL: "https://example.invalid/calendar.ics"},
		},
		{
			name:   "file",
			source: icsEventSource{File: "calendar.ics"},
		},
		{
			name:    "neither",
			source:  icsEventSource{},
			wantErr: true,
		},
		{
			name: "both",
			source: icsEventSource{
				URL:  "https://example.invalid/calendar.ics",
				File: "calendar.ics",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			widget := &calendarWidget{
				Sources: []icsEventSource{tt.source},
			}

			err := widget.initialize()
			if (err != nil) != tt.wantErr {
				t.Fatalf("initialize error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCalendarEventWindow(t *testing.T) {
	widget := &calendarWidget{
		FirstDayOfWeek: "monday",
		Sources: []icsEventSource{
			{URL: "https://example.invalid/calendar.ics"},
		},
	}

	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	widget.now = fixedCalendarTestTime

	start, end, minimum, maximum := widget.eventWindow()

	if got := minimum.Format("2006-01-02"); got != "2025-09-01" {
		t.Fatalf("minimum month = %s", got)
	}

	if got := maximum.Format("2006-01-02"); got != "2027-09-01" {
		t.Fatalf("maximum month = %s", got)
	}

	if start.After(minimum) {
		t.Fatalf("window start %v is after minimum month %v", start, minimum)
	}

	if !end.After(maximum.AddDate(0, 1, 0)) {
		t.Fatalf("window end %v does not cover maximum month spillover", end)
	}
}

func TestCalendarEventDates(t *testing.T) {
	t.Run("zero duration", func(t *testing.T) {
		start := time.Date(2026, 9, 6, 14, 0, 0, 0, time.UTC)

		dates := calendarEventDates(icsEvent{
			Start: start,
			End:   start,
		})

		if len(dates) != 1 || dates[0].Format("2006-01-02") != "2026-09-06" {
			t.Fatalf("dates = %#v", dates)
		}
	})

	t.Run("all day exclusive end", func(t *testing.T) {
		dates := calendarEventDates(icsEvent{
			Start:  time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC),
			End:    time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
			AllDay: true,
		})

		if len(dates) != 2 {
			t.Fatalf("date count = %d, want 2", len(dates))
		}

		if dates[0].Format("2006-01-02") != "2026-09-06" ||
			dates[1].Format("2006-01-02") != "2026-09-07" {
			t.Fatalf("dates = %#v", dates)
		}
	})

	t.Run("timed multi day", func(t *testing.T) {
		dates := calendarEventDates(icsEvent{
			Start: time.Date(2026, 9, 6, 23, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 9, 8, 1, 0, 0, 0, time.UTC),
		})

		if len(dates) != 3 {
			t.Fatalf("date count = %d, want 3", len(dates))
		}
	})
}

func TestCalendarUpdateBuildsPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calendar.ics")

	body := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:test-event",
		"DTSTART:20260906T180000Z",
		"DTEND:20260906T190000Z",
		"SUMMARY:Calendar Test Event",
		"LOCATION:Test Location",
		"URL:https://example.com/event",
		"END:VEVENT",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	widget := &calendarWidget{
		Sources: []icsEventSource{
			{
				File:  path,
				Title: "Test Feed",
			},
		},
	}

	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	widget.now = fixedCalendarTestTime
	widget.update(context.Background())

	if widget.Error != nil {
		t.Fatalf("update error: %v", widget.Error)
	}

	events := widget.EventIndex["2026-09-06"]
	if len(events) != 1 {
		t.Fatalf("events on 2026-09-06 = %d, want 1", len(events))
	}

	event := events[0]
	if event.Title != "Calendar Test Event" {
		t.Fatalf("title = %q", event.Title)
	}
	if event.SourceTitle != "Test Feed" {
		t.Fatalf("source = %q", event.SourceTitle)
	}
	if event.Location != "Test Location" {
		t.Fatalf("location = %q", event.Location)
	}
	if event.URL != "https://example.com/event" {
		t.Fatalf("url = %q", event.URL)
	}

	rendered := string(widget.Render())

	for _, expected := range []string{
		`data-calendar-events`,
		`data-calendar-minimum-month="2025-09"`,
		`data-calendar-maximum-month="2027-09"`,
		`Calendar Test Event`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("render missing %q: %s", expected, rendered)
		}
	}

	if strings.Contains(rendered, path) {
		t.Fatal("calendar source path leaked into rendered payload")
	}
}
