package glance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedICSTestTime() time.Time {
	return time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
}

func newICSTestWidget(t *testing.T, sources []icsEventSource) *icsEventsWidget {
	t.Helper()

	widget := &icsEventsWidget{
		Sources: sources,
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	widget.now = fixedICSTestTime
	return widget
}

func TestICSEventsInitializeDefaults(t *testing.T) {
	widget := &icsEventsWidget{
		Sources: []icsEventSource{
			{URL: "https://example.invalid/calendar.ics"},
		},
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	if widget.Title != "Upcoming Events" {
		t.Fatalf("title = %q, want Upcoming Events", widget.Title)
	}
	if widget.DaysAhead != 14 {
		t.Fatalf("days-ahead = %d, want 14", widget.DaysAhead)
	}
	if widget.Limit != 25 {
		t.Fatalf("limit = %d, want 25", widget.Limit)
	}
	if widget.CollapseAfter != 5 {
		t.Fatalf("collapse-after = %d, want 5", widget.CollapseAfter)
	}
}

func TestICSEventsInitializeRequiresSources(t *testing.T) {
	widget := &icsEventsWidget{}

	if err := widget.initialize(); err == nil {
		t.Fatal("initialize succeeded without sources")
	}
}

func TestICSEventsInitializeRequiresExactlyOneSourceLocation(t *testing.T) {
	tests := []struct {
		name   string
		source icsEventSource
	}{
		{
			name:   "neither",
			source: icsEventSource{},
		},
		{
			name: "both",
			source: icsEventSource{
				URL:  "https://example.invalid/calendar.ics",
				File: "/tmp/calendar.ics",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			widget := &icsEventsWidget{
				Sources: []icsEventSource{test.source},
			}

			if err := widget.initialize(); err == nil {
				t.Fatal("initialize succeeded for invalid source")
			}
		})
	}
}

func TestICSEventsParseTimedEvent(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics", Title: "Family"},
	})

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:timed-1\r\n" +
		"DTSTART:20260901T180000Z\r\n" +
		"DTEND:20260901T190000Z\r\n" +
		"SUMMARY:Dinner\r\n" +
		"LOCATION:Downtown\r\n" +
		"URL:https://example.com/dinner\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}

	event := events[0]
	if event.UID != "timed-1" {
		t.Fatalf("UID = %q", event.UID)
	}
	if event.Title != "Dinner" {
		t.Fatalf("Title = %q", event.Title)
	}
	if event.Location != "Downtown" {
		t.Fatalf("Location = %q", event.Location)
	}
	if event.URL != "https://example.com/dinner" {
		t.Fatalf("URL = %q", event.URL)
	}
	if event.SourceTitle != "Family" {
		t.Fatalf("SourceTitle = %q", event.SourceTitle)
	}
	if event.AllDay {
		t.Fatal("timed event marked all-day")
	}
}

func TestICSEventsParseAllDayEvent(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:all-day-1\r\n" +
		"DTSTART;VALUE=DATE:20260902\r\n" +
		"DTEND;VALUE=DATE:20260903\r\n" +
		"SUMMARY:Holiday\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if !events[0].AllDay {
		t.Fatal("all-day event was not detected")
	}
}

func TestICSEventsExpandsBoundedDailyRecurrence(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})
	widget.DaysAhead = 3

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:daily-1\r\n" +
		"DTSTART:20260901T180000Z\r\n" +
		"DTEND:20260901T190000Z\r\n" +
		"RRULE:FREQ=DAILY\r\n" +
		"SUMMARY:Daily Event\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}

	for _, event := range events {
		if event.Start.Before(fixedICSTestTime()) {
			t.Fatalf("recurrence before window: %v", event.Start)
		}
		if !event.Start.Before(fixedICSTestTime().AddDate(0, 0, 3)) {
			t.Fatalf("recurrence outside bounded window: %v", event.Start)
		}
	}
}

func TestICSEventsIncludesOngoingEvent(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:ongoing-1\r\n" +
		"DTSTART:20260901T110000Z\r\n" +
		"DTEND:20260901T130000Z\r\n" +
		"SUMMARY:Ongoing\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if !events[0].Ongoing {
		t.Fatal("ongoing event not marked ongoing")
	}
}

func TestICSEventsReadsLocalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "calendar.ics")

	body := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:file-1\r\n" +
		"DTSTART:20260901T180000Z\r\n" +
		"DTEND:20260901T190000Z\r\n" +
		"SUMMARY:From File\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("write ICS: %v", err)
	}

	widget := newICSTestWidget(t, []icsEventSource{
		{File: path},
	})

	got, err := widget.fetchSource(context.Background(), widget.Sources[0])
	if err != nil {
		t.Fatalf("fetchSource: %v", err)
	}

	if string(got) != body {
		t.Fatal("local ICS contents differ")
	}
}

func TestICSEventsHTTPConditionalCache(t *testing.T) {
	var requests int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++

		if requests == 2 {
			if r.Header.Get("If-None-Match") != `"calendar-v1"` {
				t.Errorf("If-None-Match = %q", r.Header.Get("If-None-Match"))
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("ETag", `"calendar-v1"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("calendar-data"))
	}))
	defer server.Close()

	widget := newICSTestWidget(t, []icsEventSource{
		{URL: server.URL},
	})

	first, err := widget.fetchSource(context.Background(), widget.Sources[0])
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	second, err := widget.fetchSource(context.Background(), widget.Sources[0])
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	if string(first) != "calendar-data" || string(second) != "calendar-data" {
		t.Fatalf("cached bodies = %q / %q", first, second)
	}
}

func TestICSEventsPartialSourceFailure(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("BEGIN:VCALENDAR\r\n" +
			"VERSION:2.0\r\n" +
			"BEGIN:VEVENT\r\n" +
			"UID:good-1\r\n" +
			"DTSTART:20260901T180000Z\r\n" +
			"DTEND:20260901T190000Z\r\n" +
			"SUMMARY:Good Event\r\n" +
			"END:VEVENT\r\n" +
			"END:VCALENDAR\r\n"))
	}))
	defer good.Close()

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "failed", http.StatusInternalServerError)
	}))
	defer bad.Close()

	widget := newICSTestWidget(t, []icsEventSource{
		{URL: good.URL},
		{URL: bad.URL},
	})

	events, err := widget.fetchEvents(context.Background())
	if !errors.Is(err, errPartialContent) {
		t.Fatalf("error = %v, want errPartialContent", err)
	}
	if errors.Is(err, errNoContent) {
		t.Fatalf("error unexpectedly classified as errNoContent: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

func TestICSEventsAllSourcesFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "failed", http.StatusInternalServerError)
	}))
	defer server.Close()

	widget := newICSTestWidget(t, []icsEventSource{
		{URL: server.URL + "/one"},
		{URL: server.URL + "/two"},
	})

	events, err := widget.fetchEvents(context.Background())
	if !errors.Is(err, errNoContent) {
		t.Fatalf("error = %v, want errNoContent", err)
	}
	if errors.Is(err, errPartialContent) {
		t.Fatalf("error unexpectedly classified as errPartialContent: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0", len(events))
	}
}

func TestICSEventsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})

	_, err := widget.fetchEvents(ctx)
	if err == nil {
		t.Fatal("fetchEvents succeeded with canceled context")
	}
	if !errors.Is(err, errNoContent) {
		t.Fatalf("error = %v, want errNoContent", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error does not preserve context cancellation: %v", err)
	}
}

func TestICSEventsDecoratesAgendaLabels(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})

	events := []icsEvent{
		{Start: time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC)},
		{Start: time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)},
		{Start: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), AllDay: true},
		{Start: time.Date(2026, 9, 3, 9, 30, 0, 0, time.UTC)},
	}

	widget.decorateEvents(events)

	if !events[0].ShowDateLabel || events[0].DateLabel != "Today" {
		t.Fatalf("first event date label = %#v", events[0])
	}
	if events[1].ShowDateLabel {
		t.Fatal("second event on same date repeated date heading")
	}
	if !events[2].ShowDateLabel || events[2].DateLabel != "Tomorrow" {
		t.Fatalf("tomorrow event date label = %#v", events[2])
	}
	if events[2].TimeLabel != "All day" {
		t.Fatalf("all-day time label = %q", events[2].TimeLabel)
	}
	if events[3].DateLabel != "Thursday, September 3" {
		t.Fatalf("later date label = %q", events[3].DateLabel)
	}
	if events[3].TimeLabel != "9:30 AM" {
		t.Fatalf("time label = %q", events[3].TimeLabel)
	}
}

func TestICSEventsRenderEscapesEventContent(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})
	widget.ContentAvailable = true

	widget.Events = []icsEvent{
		{
			Title:         `<script>alert("x")</script>`,
			Location:      `<b>Somewhere</b>`,
			SourceTitle:   `Family`,
			Start:         fixedICSTestTime(),
			DateLabel:     "Today",
			TimeLabel:     "12:00 PM",
			ShowDateLabel: true,
		},
	}

	rendered := string(widget.Render())

	if strings.Contains(rendered, `<script>alert("x")</script>`) {
		t.Fatal("event title rendered as unescaped HTML")
	}
	if strings.Contains(rendered, `<b>Somewhere</b>`) {
		t.Fatal("event location rendered as unescaped HTML")
	}
}

func TestICSEventsRecurrenceExDate(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})
	widget.DaysAhead = 4

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:exdate-1\r\n" +
		"DTSTART:20260901T180000Z\r\n" +
		"DTEND:20260901T190000Z\r\n" +
		"RRULE:FREQ=DAILY;COUNT=4\r\n" +
		"EXDATE:20260902T180000Z\r\n" +
		"SUMMARY:Daily Except Wednesday\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}

	for _, event := range events {
		if event.Start.Equal(time.Date(2026, 9, 2, 18, 0, 0, 0, time.UTC)) {
			t.Fatal("EXDATE occurrence was not excluded")
		}
	}
}

func TestICSEventsRecurrenceRDate(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})
	widget.DaysAhead = 5

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:rdate-1\r\n" +
		"DTSTART:20260901T180000Z\r\n" +
		"DTEND:20260901T190000Z\r\n" +
		"RRULE:FREQ=DAILY;COUNT=2\r\n" +
		"RDATE:20260904T180000Z\r\n" +
		"SUMMARY:Recurring Plus Extra\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}

	want := time.Date(2026, 9, 4, 18, 0, 0, 0, time.UTC)
	found := false

	for _, event := range events {
		if event.Start.Equal(want) {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("RDATE occurrence %v not found", want)
	}
}

func TestICSEventsRDateWithoutRRule(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})
	widget.DaysAhead = 5

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:rdate-only-1\r\n" +
		"DTSTART:20260901T180000Z\r\n" +
		"DTEND:20260901T190000Z\r\n" +
		"RDATE:20260903T180000Z,20260904T180000Z\r\n" +
		"SUMMARY:RDATE Only\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("events = %d, want DTSTART plus two RDATE occurrences", len(events))
	}
}

func TestICSEventsExDateWithTZID(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})
	widget.DaysAhead = 4

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:tz-exdate-1\r\n" +
		"DTSTART;TZID=America/Chicago:20260901T130000\r\n" +
		"DTEND;TZID=America/Chicago:20260901T140000\r\n" +
		"RRULE:FREQ=DAILY;COUNT=4\r\n" +
		"EXDATE;TZID=America/Chicago:20260902T130000\r\n" +
		"SUMMARY:Chicago Recurrence\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}

	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	excluded := time.Date(2026, 9, 2, 13, 0, 0, 0, chicago)

	for _, event := range events {
		if event.Start.Equal(excluded) {
			t.Fatal("TZID EXDATE occurrence was not excluded")
		}
	}
}

func TestICSEventsCancelledStandaloneEventExcluded(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:cancelled-1\r\n" +
		"DTSTART:20260901T180000Z\r\n" +
		"DTEND:20260901T190000Z\r\n" +
		"STATUS:CANCELLED\r\n" +
		"SUMMARY:Cancelled Event\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 0 {
		t.Fatalf("events = %d, want cancelled event excluded", len(events))
	}
}

func TestICSEventsRecurrenceOverrideReplacesOccurrence(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})
	widget.DaysAhead = 4

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:override-1\r\n" +
		"DTSTART:20260901T180000Z\r\n" +
		"DTEND:20260901T190000Z\r\n" +
		"RRULE:FREQ=DAILY;COUNT=3\r\n" +
		"SUMMARY:Original\r\n" +
		"END:VEVENT\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:override-1\r\n" +
		"RECURRENCE-ID:20260902T180000Z\r\n" +
		"DTSTART:20260902T200000Z\r\n" +
		"DTEND:20260902T210000Z\r\n" +
		"SUMMARY:Moved Event\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}

	original := time.Date(2026, 9, 2, 18, 0, 0, 0, time.UTC)
	moved := time.Date(2026, 9, 2, 20, 0, 0, 0, time.UTC)
	foundMoved := false

	for _, event := range events {
		if event.Start.Equal(original) {
			t.Fatal("original occurrence remained after recurrence override")
		}
		if event.Start.Equal(moved) && event.Title == "Moved Event" {
			foundMoved = true
		}
	}

	if !foundMoved {
		t.Fatal("replacement recurrence occurrence not found")
	}
}

func TestICSEventsCancelledRecurrenceOverrideRemovesOccurrence(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})
	widget.DaysAhead = 4

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:cancel-override-1\r\n" +
		"DTSTART:20260901T180000Z\r\n" +
		"DTEND:20260901T190000Z\r\n" +
		"RRULE:FREQ=DAILY;COUNT=3\r\n" +
		"SUMMARY:Original\r\n" +
		"END:VEVENT\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:cancel-override-1\r\n" +
		"RECURRENCE-ID:20260902T180000Z\r\n" +
		"DTSTART:20260902T180000Z\r\n" +
		"DTEND:20260902T190000Z\r\n" +
		"STATUS:CANCELLED\r\n" +
		"SUMMARY:Cancelled Instance\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}

	cancelled := time.Date(2026, 9, 2, 18, 0, 0, 0, time.UTC)

	for _, event := range events {
		if event.Start.Equal(cancelled) {
			t.Fatal("cancelled recurrence occurrence remained")
		}
	}
}

func TestICSEventsMultiDayEventOverlappingWindow(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:multi-day-1\r\n" +
		"DTSTART;VALUE=DATE:20260831\r\n" +
		"DTEND;VALUE=DATE:20260903\r\n" +
		"SUMMARY:Conference\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("events = %d, want overlapping multi-day event included", len(events))
	}
	if !events[0].AllDay {
		t.Fatal("multi-day date event was not marked all-day")
	}
	if !events[0].Ongoing {
		t.Fatal("multi-day event overlapping window was not marked ongoing")
	}
}

func TestICSEventsTZIDPreserved(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:timezone-1\r\n" +
		"DTSTART;TZID=America/Chicago:20260901T130000\r\n" +
		"DTEND;TZID=America/Chicago:20260901T140000\r\n" +
		"SUMMARY:Chicago Event\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}

	_, offset := events[0].Start.Zone()
	if offset != -5*60*60 {
		t.Fatalf("timezone offset = %d, want -18000", offset)
	}

	if events[0].Start.Hour() != 13 {
		t.Fatalf("local event hour = %d, want 13", events[0].Start.Hour())
	}
}

func TestICSEventsRecurrenceDeduplicatesDTStartRDate(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})
	widget.DaysAhead = 3

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:duplicate-1\r\n" +
		"DTSTART:20260901T180000Z\r\n" +
		"DTEND:20260901T190000Z\r\n" +
		"RRULE:FREQ=DAILY;COUNT=2\r\n" +
		"RDATE:20260901T180000Z\r\n" +
		"SUMMARY:No Duplicate\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 unique occurrences", len(events))
	}

	if events[0].Start.Equal(events[1].Start) {
		t.Fatal("duplicate DTSTART/RDATE occurrence was retained")
	}
}

func TestICSEventsAllDayRecurrenceExDate(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})
	widget.DaysAhead = 4

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:all-day-recurring-1\r\n" +
		"DTSTART;VALUE=DATE:20260901\r\n" +
		"DTEND;VALUE=DATE:20260902\r\n" +
		"RRULE:FREQ=DAILY;COUNT=4\r\n" +
		"EXDATE;VALUE=DATE:20260902\r\n" +
		"SUMMARY:All Day Recurrence\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}

	for _, event := range events {
		if !event.AllDay {
			t.Fatal("recurring all-day occurrence lost all-day state")
		}
		if event.Start.Format("2006-01-02") == "2026-09-02" {
			t.Fatal("all-day EXDATE occurrence was retained")
		}
	}
}

func TestICSEventsMalformedRRuleReturnsError(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:bad-rule-1\r\n" +
		"DTSTART:20260901T180000Z\r\n" +
		"DTEND:20260901T190000Z\r\n" +
		"RRULE:FREQ=NOTREAL\r\n" +
		"SUMMARY:Bad Rule\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	_, err := widget.parseSource(body, widget.Sources[0])
	if err == nil {
		t.Fatal("parseSource succeeded with malformed RRULE")
	}
}

func TestICSEventsMalformedEventDoesNotPanic(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:missing-start-1\r\n" +
		"SUMMARY:Missing Start\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("parseSource panicked: %v", recovered)
		}
	}()

	if _, err := widget.parseSource(body, widget.Sources[0]); err == nil {
		t.Fatal("parseSource succeeded for event without DTSTART")
	}
}

func TestICSEventsDecoratesTomorrowAcrossDSTStart(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})

	widget.now = func() time.Time {
		return time.Date(2026, 3, 7, 23, 30, 0, 0, chicago)
	}

	events := []icsEvent{
		{
			Start: time.Date(2026, 3, 8, 23, 0, 0, 0, chicago),
		},
	}

	widget.decorateEvents(events)

	if events[0].DateLabel != "Tomorrow" {
		t.Fatalf("date label = %q, want Tomorrow", events[0].DateLabel)
	}
}

func TestICSEventsDecoratesTomorrowAcrossDSTEnd(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})

	widget.now = func() time.Time {
		return time.Date(2026, 10, 31, 23, 30, 0, 0, chicago)
	}

	events := []icsEvent{
		{
			Start: time.Date(2026, 11, 1, 23, 0, 0, 0, chicago),
		},
	}

	widget.decorateEvents(events)

	if events[0].DateLabel != "Tomorrow" {
		t.Fatalf("date label = %q, want Tomorrow", events[0].DateLabel)
	}
}

func TestICSEventsUpdateMergesSortsAndLimitsSources(t *testing.T) {
	var firstServer, secondServer *httptest.Server

	firstServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "BEGIN:VCALENDAR\r\n"+
			"VERSION:2.0\r\n"+
			"BEGIN:VEVENT\r\n"+
			"UID:later\r\n"+
			"DTSTART:20260903T180000Z\r\n"+
			"DTEND:20260903T190000Z\r\n"+
			"SUMMARY:Later\r\n"+
			"END:VEVENT\r\n"+
			"BEGIN:VEVENT\r\n"+
			"UID:same-zulu\r\n"+
			"DTSTART:20260902T180000Z\r\n"+
			"DTEND:20260902T190000Z\r\n"+
			"SUMMARY:Zulu\r\n"+
			"END:VEVENT\r\n"+
			"END:VCALENDAR\r\n")
	}))
	defer firstServer.Close()

	secondServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "BEGIN:VCALENDAR\r\n"+
			"VERSION:2.0\r\n"+
			"BEGIN:VEVENT\r\n"+
			"UID:earliest\r\n"+
			"DTSTART:20260901T180000Z\r\n"+
			"DTEND:20260901T190000Z\r\n"+
			"SUMMARY:Earliest\r\n"+
			"END:VEVENT\r\n"+
			"BEGIN:VEVENT\r\n"+
			"UID:same-alpha\r\n"+
			"DTSTART:20260902T180000Z\r\n"+
			"DTEND:20260902T190000Z\r\n"+
			"SUMMARY:Alpha\r\n"+
			"END:VEVENT\r\n"+
			"END:VCALENDAR\r\n")
	}))
	defer secondServer.Close()

	widget := newICSTestWidget(t, []icsEventSource{
		{URL: firstServer.URL, Title: "First"},
		{URL: secondServer.URL, Title: "Second"},
	})
	widget.Limit = 3
	widget.update(context.Background())

	if widget.Error != nil {
		t.Fatalf("update error: %v", widget.Error)
	}

	if len(widget.Events) != 3 {
		t.Fatalf("events = %d, want 3", len(widget.Events))
	}

	wantTitles := []string{"Earliest", "Alpha", "Zulu"}
	for i, want := range wantTitles {
		if widget.Events[i].Title != want {
			t.Fatalf("event %d title = %q, want %q", i, widget.Events[i].Title, want)
		}
	}

	if widget.Events[0].SourceTitle != "Second" {
		t.Fatalf("first source title = %q, want Second", widget.Events[0].SourceTitle)
	}
}

func TestICSEventsUpdateRetainsPartialContentNotice(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "BEGIN:VCALENDAR\r\n"+
			"VERSION:2.0\r\n"+
			"BEGIN:VEVENT\r\n"+
			"UID:good\r\n"+
			"DTSTART:20260901T180000Z\r\n"+
			"DTEND:20260901T190000Z\r\n"+
			"SUMMARY:Available Event\r\n"+
			"END:VEVENT\r\n"+
			"END:VCALENDAR\r\n")
	}))
	defer good.Close()

	widget := newICSTestWidget(t, []icsEventSource{
		{URL: good.URL, Title: "Good"},
		{URL: "http://127.0.0.1:1/calendar.ics", Title: "Down"},
	})

	widget.update(context.Background())

	if widget.Error != nil {
		t.Fatalf("partial update set fatal error: %v", widget.Error)
	}
	if widget.Notice == nil {
		t.Fatal("partial update did not set notice")
	}
	if len(widget.Events) != 1 {
		t.Fatalf("events = %d, want successful source content retained", len(widget.Events))
	}
	if widget.Events[0].Title != "Available Event" {
		t.Fatalf("event title = %q, want Available Event", widget.Events[0].Title)
	}
}

func TestICSEventsUpdatePreservesPreviousContentWhenAllSourcesFail(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "http://127.0.0.1:1/calendar.ics", Title: "Down"},
	})

	previous := []icsEvent{
		{
			UID:   "previous",
			Title: "Previous Event",
			Start: time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC),
		},
	}
	widget.Events = previous

	widget.update(context.Background())

	if widget.Error == nil {
		t.Fatal("all-source failure did not set widget error")
	}
	if len(widget.Events) != 1 || widget.Events[0].Title != "Previous Event" {
		t.Fatalf("previous content was replaced on total failure: %#v", widget.Events)
	}
}

func TestICSEventsRenderCollapseAfterConfiguration(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{
		{URL: "https://example.invalid/calendar.ics"},
	})
	widget.ContentAvailable = true
	widget.CollapseAfter = 3
	widget.Events = []icsEvent{
		{Title: "One", TimeLabel: "1:00 PM"},
		{Title: "Two", TimeLabel: "2:00 PM"},
		{Title: "Three", TimeLabel: "3:00 PM"},
		{Title: "Four", TimeLabel: "4:00 PM"},
	}

	rendered := string(widget.Render())

	if !strings.Contains(rendered, `data-collapse-after="3"`) {
		t.Fatal("rendered widget does not contain collapse-after configuration")
	}
	if !strings.Contains(rendered, `class="ics-events list collapsible-container"`) {
		t.Fatal("rendered widget does not use native collapsible list contract")
	}

	if strings.Count(rendered, `class="ics-event`) < 4 {
		t.Fatal("rendered widget does not contain all events before client-side collapse")
	}
}

func TestAgendaDateLabelAcrossDSTStartMidnights(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	now := time.Date(2026, 3, 8, 12, 0, 0, 0, chicago)
	tomorrow := time.Date(2026, 3, 9, 12, 0, 0, 0, chicago)

	if got := agendaDateLabel(tomorrow, now); got != "Tomorrow" {
		t.Fatalf("date label = %q, want Tomorrow", got)
	}
}

func TestAgendaDateLabelAcrossDSTEndMidnights(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	now := time.Date(2026, 11, 1, 12, 0, 0, 0, chicago)
	tomorrow := time.Date(2026, 11, 2, 12, 0, 0, 0, chicago)

	if got := agendaDateLabel(tomorrow, now); got != "Tomorrow" {
		t.Fatalf("date label = %q, want Tomorrow", got)
	}
}

func TestICSEventsAllDayWithoutEndDefaultsToOneDay(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{{URL: "https://example.invalid/calendar.ics"}})

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:all-day-no-end\r\n" +
		"DTSTART;VALUE=DATE:20260901\r\n" +
		"SUMMARY:One Day Holiday\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if !events[0].AllDay {
		t.Fatal("event was not marked all-day")
	}
	wantEnd := events[0].Start.AddDate(0, 0, 1)
	if !events[0].End.Equal(wantEnd) {
		t.Fatalf("end = %v, want next calendar day %v", events[0].End, wantEnd)
	}
}

func TestICSEventsDurationPropertySetsEnd(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{{URL: "https://example.invalid/calendar.ics"}})

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:duration-event\r\n" +
		"DTSTART:20260901T180000Z\r\n" +
		"DURATION:PT1H30M\r\n" +
		"SUMMARY:Duration Event\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	events, err := widget.parseSource(body, widget.Sources[0])
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if got := events[0].End.Sub(events[0].Start); got != 90*time.Minute {
		t.Fatalf("duration = %v, want 1h30m", got)
	}
}

func TestICSEventsMalformedEndReturnsError(t *testing.T) {
	widget := newICSTestWidget(t, []icsEventSource{{URL: "https://example.invalid/calendar.ics"}})

	body := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:bad-end\r\n" +
		"DTSTART:20260901T180000Z\r\n" +
		"DTEND:not-a-time\r\n" +
		"SUMMARY:Bad End\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	if _, err := widget.parseSource(body, widget.Sources[0]); err == nil {
		t.Fatal("parseSource succeeded with malformed DTEND")
	}
}

func TestParseICSDuration(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
		ok    bool
	}{
		{value: "PT1H30M", want: 90 * time.Minute, ok: true},
		{value: "P2D", want: 48 * time.Hour, ok: true},
		{value: "P1W", want: 7 * 24 * time.Hour, ok: true},
		{value: "PT45S", want: 45 * time.Second, ok: true},
		{value: "P1DT2H", want: 26 * time.Hour, ok: true},
		{value: "not-a-duration", ok: false},
		{value: "P", ok: false},
		{value: "PT", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := parseICSDuration(tt.value)
			if !tt.ok {
				if err == nil {
					t.Fatalf("parseICSDuration(%q) succeeded, want error", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseICSDuration(%q): %v", tt.value, err)
			}
			if got != tt.want {
				t.Fatalf("parseICSDuration(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
