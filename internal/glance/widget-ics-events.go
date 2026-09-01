package glance

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/teambition/rrule-go"
)

var icsEventsWidgetTemplate = mustParseTemplate("ics-events.html", "widget-base.html")

type icsEventsWidget struct {
	widgetBase `yaml:",inline"`

	Sources       []icsEventSource `yaml:"sources"`
	DaysAhead     int              `yaml:"days-ahead"`
	Limit         int              `yaml:"limit"`
	CollapseAfter int              `yaml:"collapse-after"`

	Events          []icsEvent `yaml:"-"`
	NoEventsMessage string     `yaml:"-"`

	cachedSourcesMutex sync.Mutex
	cachedSources      map[string]*cachedICSSource `yaml:"-"`
	now                func() time.Time            `yaml:"-"`
}

type icsEventSource struct {
	URL   string `yaml:"url"`
	File  string `yaml:"file"`
	Title string `yaml:"title"`
}

type cachedICSSource struct {
	etag         string
	lastModified string
	body         []byte
}

type icsEvent struct {
	UID           string
	Title         string
	Location      string
	URL           string
	SourceTitle   string
	Start         time.Time
	End           time.Time
	AllDay        bool
	DateLabel     string
	TimeLabel     string
	ShowDateLabel bool
	Ongoing       bool
}

func (widget *icsEventsWidget) initialize() error {
	widget.withTitle("Upcoming Events").withCacheDuration(30 * time.Minute)

	if len(widget.Sources) == 0 {
		return fmt.Errorf("at least one ICS source is required")
	}

	for i, source := range widget.Sources {
		hasURL := strings.TrimSpace(source.URL) != ""
		hasFile := strings.TrimSpace(source.File) != ""

		if hasURL == hasFile {
			return fmt.Errorf("ICS source %d must specify exactly one of url or file", i+1)
		}
	}

	if widget.DaysAhead <= 0 {
		widget.DaysAhead = 14
	}

	if widget.Limit <= 0 {
		widget.Limit = 25
	}

	if widget.CollapseAfter == 0 || widget.CollapseAfter < -1 {
		widget.CollapseAfter = 5
	}

	widget.NoEventsMessage = "No upcoming events."
	widget.cachedSources = make(map[string]*cachedICSSource)
	widget.now = time.Now

	return nil
}

func (widget *icsEventsWidget) update(ctx context.Context) {
	events, err := widget.fetchEvents(ctx)

	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Start.Equal(events[j].Start) {
			return events[i].Title < events[j].Title
		}
		return events[i].Start.Before(events[j].Start)
	})

	if len(events) > widget.Limit {
		events = events[:widget.Limit]
	}

	widget.decorateEvents(events)
	widget.Events = events
}

func (widget *icsEventsWidget) Render() template.HTML {
	return widget.renderTemplate(widget, icsEventsWidgetTemplate)
}

func (widget *icsEventsWidget) fetchEvents(ctx context.Context) ([]icsEvent, error) {
	type result struct {
		events []icsEvent
		err    error
	}

	results := make([]result, len(widget.Sources))
	var wg sync.WaitGroup

	for i := range widget.Sources {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			body, err := widget.fetchSource(ctx, widget.Sources[index])
			if err != nil {
				results[index].err = err
				return
			}

			events, err := widget.parseSource(body, widget.Sources[index])
			results[index] = result{events: events, err: err}
		}(i)
	}

	wg.Wait()

	var events []icsEvent
	failed := 0
	var firstFailure error

	for _, result := range results {
		if result.err != nil {
			failed++
			if firstFailure == nil {
				firstFailure = result.err
			}
			continue
		}

		events = append(events, result.events...)
	}

	if failed == len(widget.Sources) {
		return nil, contentFetchError(
			errNoContent,
			failed,
			len(widget.Sources),
			"ICS sources",
			firstFailure,
		)
	}

	if failed > 0 {
		return events, contentFetchError(
			errPartialContent,
			failed,
			len(widget.Sources),
			"ICS sources",
			firstFailure,
		)
	}

	return events, nil
}

func (widget *icsEventsWidget) fetchSource(ctx context.Context, source icsEventSource) ([]byte, error) {
	if source.File != "" {
		body, err := os.ReadFile(source.File)
		if err != nil {
			return nil, fmt.Errorf("reading ICS file: %w", err)
		}
		return body, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating ICS request: %w", err)
	}

	req.Header.Set("User-Agent", glanceUserAgentString)

	widget.cachedSourcesMutex.Lock()
	cached, isCached := widget.cachedSources[source.URL]
	if isCached {
		if cached.etag != "" {
			req.Header.Set("If-None-Match", cached.etag)
		}
		if cached.lastModified != "" {
			req.Header.Set("If-Modified-Since", cached.lastModified)
		}
	}
	widget.cachedSourcesMutex.Unlock()

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending ICS request: %w", safeHTTPTransportError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified && isCached {
		return cached.body, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedHTTPStatusError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading ICS response: %w", err)
	}

	widget.cachedSourcesMutex.Lock()
	widget.cachedSources[source.URL] = &cachedICSSource{
		etag:         resp.Header.Get("ETag"),
		lastModified: resp.Header.Get("Last-Modified"),
		body:         body,
	}
	widget.cachedSourcesMutex.Unlock()

	return body, nil
}

type icsRecurrenceOverride struct {
	event        *ics.VEvent
	recurrenceID time.Time
	cancelled    bool
}

func (widget *icsEventsWidget) parseSource(body []byte, source icsEventSource) ([]icsEvent, error) {
	calendar, err := ics.ParseCalendar(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parsing ICS source: %w", err)
	}

	now := widget.now()
	windowStart := now
	windowEnd := now.AddDate(0, 0, widget.DaysAhead)

	masters := make([]*ics.VEvent, 0)
	overridesByUID := make(map[string][]icsRecurrenceOverride)

	for _, event := range calendar.Events() {
		uid := eventUID(event)

		recurrenceIDProperty := event.GetProperty(ics.ComponentPropertyRecurrenceId)
		if recurrenceIDProperty == nil {
			masters = append(masters, event)
			continue
		}

		recurrenceID, err := event.GetRecurrenceID()
		if err != nil {
			return nil, fmt.Errorf("reading ICS recurrence ID: %w", err)
		}

		overridesByUID[uid] = append(overridesByUID[uid], icsRecurrenceOverride{
			event:        event,
			recurrenceID: recurrenceID,
			cancelled:    eventIsCancelled(event),
		})
	}

	var events []icsEvent

	for _, master := range masters {
		if eventIsCancelled(master) {
			continue
		}

		uid := eventUID(master)
		expanded, err := widget.expandEvent(
			master,
			source,
			windowStart,
			windowEnd,
			overridesByUID[uid],
		)
		if err != nil {
			return nil, err
		}

		events = append(events, expanded...)
		delete(overridesByUID, uid)
	}

	// A recurrence override normally accompanies a master VEVENT. If a source
	// supplies an orphaned non-cancelled override, displaying its actual
	// DTSTART is more useful than silently dropping it.
	for _, overrides := range overridesByUID {
		for _, override := range overrides {
			if override.cancelled {
				continue
			}

			expanded, err := widget.expandStandaloneEvent(
				override.event,
				source,
				windowStart,
				windowEnd,
			)
			if err != nil {
				return nil, err
			}

			events = append(events, expanded...)
		}
	}

	return events, nil
}

func eventUID(event *ics.VEvent) string {
	if property := event.GetProperty(ics.ComponentPropertyUniqueId); property != nil {
		return property.Value
	}
	return ""
}

func eventIsCancelled(event *ics.VEvent) bool {
	property := event.GetProperty(ics.ComponentPropertyStatus)
	return property != nil && strings.EqualFold(strings.TrimSpace(property.Value), "CANCELLED")
}

func (widget *icsEventsWidget) expandStandaloneEvent(
	event *ics.VEvent,
	source icsEventSource,
	windowStart time.Time,
	windowEnd time.Time,
) ([]icsEvent, error) {
	if eventIsCancelled(event) {
		return nil, nil
	}

	start, end, allDay, err := icsEventTimes(event)
	if err != nil {
		return nil, err
	}

	if !eventOverlapsWindow(start, end, windowStart, windowEnd) {
		return nil, nil
	}

	return []icsEvent{
		buildICSEvent(event, source, start, end, allDay, windowStart),
	}, nil
}

func (widget *icsEventsWidget) expandEvent(
	event *ics.VEvent,
	source icsEventSource,
	windowStart time.Time,
	windowEnd time.Time,
	overrides []icsRecurrenceOverride,
) ([]icsEvent, error) {
	start, end, allDay, err := icsEventTimes(event)
	if err != nil {
		return nil, err
	}

	duration := end.Sub(start)
	if duration < 0 {
		duration = 0
	}

	rruleProperty := event.GetProperty(ics.ComponentPropertyRrule)
	rdates, err := event.GetRDates()
	if err != nil {
		return nil, fmt.Errorf("reading ICS recurrence dates: %w", err)
	}

	exdates, err := event.GetExDates()
	if err != nil {
		return nil, fmt.Errorf("reading ICS exclusion dates: %w", err)
	}

	hasRecurrence := rruleProperty != nil && strings.TrimSpace(rruleProperty.Value) != ""
	hasRecurrence = hasRecurrence || len(rdates) > 0 || len(exdates) > 0

	if !hasRecurrence {
		if !eventOverlapsWindow(start, end, windowStart, windowEnd) {
			return nil, nil
		}

		return []icsEvent{
			buildICSEvent(event, source, start, end, allDay, windowStart),
		}, nil
	}

	set := &rrule.Set{}
	set.DTStart(start)
	set.RDate(start)

	if hasRecurrence && rruleProperty != nil && strings.TrimSpace(rruleProperty.Value) != "" {
		option, err := rrule.StrToROption(rruleProperty.Value)
		if err != nil {
			return nil, fmt.Errorf("parsing ICS recurrence rule: %w", err)
		}

		option.Dtstart = start
		rule, err := rrule.NewRRule(*option)
		if err != nil {
			return nil, fmt.Errorf("building ICS recurrence rule: %w", err)
		}

		set.RRule(rule)
	}

	for _, rdate := range rdates {
		set.RDate(rdate)
	}

	for _, exdate := range exdates {
		set.ExDate(exdate)
	}

	for _, override := range overrides {
		set.ExDate(override.recurrenceID)
	}

	rangeStart := windowStart.Add(-duration)
	instances := set.Between(rangeStart, windowEnd, true)

	events := make([]icsEvent, 0, len(instances)+len(overrides))

	for _, instanceStart := range instances {
		instanceEnd := instanceStart.Add(duration)
		if !eventOverlapsWindow(instanceStart, instanceEnd, windowStart, windowEnd) {
			continue
		}

		events = append(
			events,
			buildICSEvent(event, source, instanceStart, instanceEnd, allDay, windowStart),
		)
	}

	for _, override := range overrides {
		if override.cancelled {
			continue
		}

		overrideStart, overrideEnd, overrideAllDay, err := icsEventTimes(override.event)
		if err != nil {
			return nil, err
		}

		if !eventOverlapsWindow(overrideStart, overrideEnd, windowStart, windowEnd) {
			continue
		}

		events = append(
			events,
			buildICSEvent(
				override.event,
				source,
				overrideStart,
				overrideEnd,
				overrideAllDay,
				windowStart,
			),
		)
	}

	return events, nil
}

func icsEventTimes(event *ics.VEvent) (time.Time, time.Time, bool, error) {
	start, err := event.GetStartAt()
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("reading ICS event start: %w", err)
	}

	allDay := false
	if property := event.GetProperty(ics.ComponentPropertyDtStart); property != nil {
		for _, value := range property.ICalParameters["VALUE"] {
			if strings.EqualFold(value, "DATE") {
				allDay = true
				break
			}
		}
	}

	if event.HasProperty(ics.ComponentPropertyDtEnd) {
		end, err := event.GetEndAt()
		if err != nil {
			return time.Time{}, time.Time{}, false, fmt.Errorf("reading ICS event end: %w", err)
		}
		return start, end, allDay, nil
	}

	if property := event.GetProperty(ics.ComponentPropertyDuration); property != nil {
		duration, err := parseICSDuration(property.Value)
		if err != nil {
			return time.Time{}, time.Time{}, false, fmt.Errorf("reading ICS event duration: %w", err)
		}
		return start, start.Add(duration), allDay, nil
	}

	if allDay {
		return start, start.AddDate(0, 0, 1), true, nil
	}

	return start, start, false, nil
}

func parseICSDuration(value string) (time.Duration, error) {
	if value == "" {
		return 0, fmt.Errorf("empty duration")
	}

	sign := time.Duration(1)
	if value[0] == '-' {
		sign = -1
		value = value[1:]
	} else if value[0] == '+' {
		value = value[1:]
	}

	if len(value) < 2 || value[0] != 'P' {
		return 0, fmt.Errorf("invalid duration")
	}

	value = value[1:]
	var total time.Duration
	var number int
	haveNumber := false
	inTime := false
	haveComponent := false
	weekUsed := false

	for _, ch := range value {
		if ch >= '0' && ch <= '9' {
			number = number*10 + int(ch-'0')
			haveNumber = true
			continue
		}

		if ch == 'T' {
			if inTime || haveNumber || weekUsed {
				return 0, fmt.Errorf("invalid duration")
			}
			inTime = true
			continue
		}

		if !haveNumber {
			return 0, fmt.Errorf("invalid duration")
		}

		switch ch {
		case 'W':
			if inTime || haveComponent {
				return 0, fmt.Errorf("invalid duration")
			}
			total += time.Duration(number) * 7 * 24 * time.Hour
			weekUsed = true
		case 'D':
			if inTime || weekUsed {
				return 0, fmt.Errorf("invalid duration")
			}
			total += time.Duration(number) * 24 * time.Hour
		case 'H':
			if !inTime || weekUsed {
				return 0, fmt.Errorf("invalid duration")
			}
			total += time.Duration(number) * time.Hour
		case 'M':
			if !inTime || weekUsed {
				return 0, fmt.Errorf("invalid duration")
			}
			total += time.Duration(number) * time.Minute
		case 'S':
			if !inTime || weekUsed {
				return 0, fmt.Errorf("invalid duration")
			}
			total += time.Duration(number) * time.Second
		default:
			return 0, fmt.Errorf("invalid duration")
		}

		number = 0
		haveNumber = false
		haveComponent = true
	}

	if haveNumber || !haveComponent {
		return 0, fmt.Errorf("invalid duration")
	}

	return sign * total, nil
}

func buildICSEvent(
	event *ics.VEvent,
	source icsEventSource,
	start time.Time,
	end time.Time,
	allDay bool,
	windowStart time.Time,
) icsEvent {
	title := ""
	if property := event.GetProperty(ics.ComponentPropertySummary); property != nil {
		title = property.Value
	}
	if title == "" {
		title = "Untitled event"
	}

	location := ""
	if property := event.GetProperty(ics.ComponentPropertyLocation); property != nil {
		location = property.Value
	}

	eventURL := ""
	if property := event.GetProperty(ics.ComponentPropertyUrl); property != nil {
		eventURL = property.Value
	}

	return icsEvent{
		UID:         eventUID(event),
		Title:       title,
		Location:    location,
		URL:         eventURL,
		SourceTitle: source.Title,
		Start:       start,
		End:         end,
		AllDay:      allDay,
		Ongoing:     start.Before(windowStart) && end.After(windowStart),
	}
}

func eventOverlapsWindow(start, end, windowStart, windowEnd time.Time) bool {
	if end.Equal(start) {
		return !start.Before(windowStart) && start.Before(windowEnd)
	}

	return end.After(windowStart) && start.Before(windowEnd)
}

func (widget *icsEventsWidget) decorateEvents(events []icsEvent) {
	now := widget.now()
	var previousDate string

	for i := range events {
		dateKey := events[i].Start.Format("2006-01-02")
		events[i].ShowDateLabel = dateKey != previousDate

		if events[i].ShowDateLabel {
			events[i].DateLabel = agendaDateLabel(events[i].Start, now)
			previousDate = dateKey
		}

		switch {
		case events[i].AllDay:
			events[i].TimeLabel = "All day"
		case events[i].Ongoing:
			events[i].TimeLabel = "Now"
		default:
			events[i].TimeLabel = events[i].Start.Format("3:04 PM")
		}
	}
}

func agendaDateLabel(eventTime, now time.Time) string {
	eventDate := time.Date(
		eventTime.Year(), eventTime.Month(), eventTime.Day(),
		0, 0, 0, 0, time.UTC,
	)
	nowInEventLocation := now.In(eventTime.Location())
	nowDate := time.Date(
		nowInEventLocation.Year(), nowInEventLocation.Month(), nowInEventLocation.Day(),
		0, 0, 0, 0, time.UTC,
	)

	switch days := int(eventDate.Sub(nowDate) / (24 * time.Hour)); days {
	case 0:
		return "Today"
	case 1:
		return "Tomorrow"
	default:
		return eventTime.Format("Monday, January 2")
	}
}
