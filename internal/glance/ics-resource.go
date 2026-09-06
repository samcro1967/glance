package glance

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/teambition/rrule-go"
)

type icsRecurrenceOverride struct {
	event        *ics.VEvent
	recurrenceID time.Time
	cancelled    bool
}

func parseICSResource(
	body []byte,
	source icsEventSource,
	windowStart time.Time,
	windowEnd time.Time,
) ([]icsEvent, error) {
	calendar, err := ics.ParseCalendar(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parsing ICS source: %w", err)
	}

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
		expanded, err := expandICSEvent(
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

			expanded, err := expandStandaloneICSEvent(
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

func expandStandaloneICSEvent(
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

func expandICSEvent(
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

type icsEventSource struct {
	URL           string            `yaml:"url"`
	File          string            `yaml:"file"`
	Title         string            `yaml:"title"`
	Timeout       durationField     `yaml:"timeout"`
	AllowInsecure bool              `yaml:"allow-insecure"`
	Headers       map[string]string `yaml:"headers"`
	BasicAuth     struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"basic-auth"`
	configuredFields yamlConfiguredFields `yaml:"-"`
}

func (source *icsEventSource) UnmarshalYAML(node *yaml.Node) error {
	type plain icsEventSource
	if err := node.Decode((*plain)(source)); err != nil {
		return err
	}

	source.configuredFields = yamlMappingFields(node)
	return nil
}

type cachedICSSource struct {
	etag         string
	lastModified string
	body         []byte
}

type icsSourceCache struct {
	mutex   sync.Mutex
	sources map[string]*cachedICSSource
}

func newICSSourceCache() *icsSourceCache {
	return &icsSourceCache{sources: make(map[string]*cachedICSSource)}
}

func fetchICSSource(ctx context.Context, source icsEventSource, cache *icsSourceCache) ([]byte, error) {
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
	for key, value := range source.Headers {
		req.Header.Set(key, value)
	}

	if source.BasicAuth.Username != "" || source.BasicAuth.Password != "" {
		req.SetBasicAuth(source.BasicAuth.Username, source.BasicAuth.Password)
	}

	cache.mutex.Lock()
	cached, isCached := cache.sources[source.URL]
	if isCached {
		if cached.etag != "" {
			req.Header.Set("If-None-Match", cached.etag)
		}
		if cached.lastModified != "" {
			req.Header.Set("If-Modified-Since", cached.lastModified)
		}
	}
	cache.mutex.Unlock()

	baseClient := ternary(source.AllowInsecure, defaultInsecureHTTPClient, defaultHTTPClient)
	client := *baseClient
	if source.Timeout > 0 {
		client.Timeout = time.Duration(source.Timeout)
	}

	resp, err := client.Do(req)
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

	cache.mutex.Lock()
	cache.sources[source.URL] = &cachedICSSource{
		etag:         resp.Header.Get("ETag"),
		lastModified: resp.Header.Get("Last-Modified"),
		body:         body,
	}
	cache.mutex.Unlock()

	return body, nil
}

func validateICSSources(sources []icsEventSource, requireSources bool) error {
	if requireSources && len(sources) == 0 {
		return fmt.Errorf("at least one ICS source is required")
	}

	for i, source := range sources {
		hasURL := strings.TrimSpace(source.URL) != ""
		hasFile := strings.TrimSpace(source.File) != ""

		if hasURL == hasFile {
			return fmt.Errorf("ICS source %d must specify exactly one of url or file", i+1)
		}
	}

	return nil
}

func fetchICSResources(ctx context.Context, sources []icsEventSource, cache *icsSourceCache, windowStart, windowEnd time.Time) ([]icsEvent, error) {
	type result struct {
		events []icsEvent
		err    error
	}

	results := make([]result, len(sources))
	var wg sync.WaitGroup

	for i := range sources {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			body, err := fetchICSSource(ctx, sources[index], cache)
			if err != nil {
				results[index].err = err
				return
			}

			events, err := parseICSResource(body, sources[index], windowStart, windowEnd)
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

	if failed == len(sources) {
		return nil, contentFetchError(errNoContent, failed, len(sources), "ICS sources", firstFailure)
	}

	if failed > 0 {
		return events, contentFetchError(errPartialContent, failed, len(sources), "ICS sources", firstFailure)
	}

	return events, nil
}
