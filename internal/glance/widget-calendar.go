package glance

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"time"
)

var calendarWidgetTemplate = mustParseTemplate("calendar.html", "widget-base.html")

const calendarMonthRange = 12

var calendarWeekdaysToInt = map[string]time.Weekday{
	"sunday":    time.Sunday,
	"monday":    time.Monday,
	"tuesday":   time.Tuesday,
	"wednesday": time.Wednesday,
	"thursday":  time.Thursday,
	"friday":    time.Friday,
	"saturday":  time.Saturday,
}

type calendarEventPayload struct {
	UID         string `json:"uid"`
	Title       string `json:"title"`
	Location    string `json:"location,omitempty"`
	URL         string `json:"url,omitempty"`
	SourceTitle string `json:"source,omitempty"`
	Start       string `json:"start"`
	End         string `json:"end"`
	AllDay      bool   `json:"allDay"`
}

type calendarWidget struct {
	widgetBase     `yaml:",inline"`
	FirstDayOfWeek string                            `yaml:"first-day-of-week"`
	FirstDay       int                               `yaml:"-"`
	Sources        []icsEventSource                  `yaml:"sources"`
	Events         []icsEvent                        `yaml:"-"`
	EventIndex     map[string][]calendarEventPayload `yaml:"-"`
	EventsJSON     template.JS                       `yaml:"-"`
	MinimumMonth   string                            `yaml:"-"`
	MaximumMonth   string                            `yaml:"-"`
	cachedSources  *icsSourceCache                   `yaml:"-"`
	now            func() time.Time                  `yaml:"-"`
	cachedHTML     template.HTML                     `yaml:"-"`
}

func (widget *calendarWidget) initialize() error {
	widget.withTitle("Calendar").withError(nil)

	if widget.FirstDayOfWeek == "" {
		widget.FirstDayOfWeek = "monday"
	} else if _, ok := calendarWeekdaysToInt[widget.FirstDayOfWeek]; !ok {
		return errors.New("invalid first day of week")
	}

	widget.FirstDay = int(calendarWeekdaysToInt[widget.FirstDayOfWeek])

	if err := validateICSSources(widget.Sources, false); err != nil {
		return err
	}

	if len(widget.Sources) == 0 {
		widget.cachedHTML = widget.renderTemplate(widget, calendarWidgetTemplate)
		return nil
	}

	widget.withCacheDuration(30 * time.Minute)
	widget.cachedSources = newICSSourceCache()
	widget.now = time.Now

	return nil
}

func (widget *calendarWidget) Render() template.HTML {
	if len(widget.Sources) == 0 {
		return widget.cachedHTML
	}

	return widget.renderTemplate(widget, calendarWidgetTemplate)
}

func (widget *calendarWidget) update(ctx context.Context) {
	if len(widget.Sources) == 0 {
		return
	}

	windowStart, windowEnd, minimumMonth, maximumMonth := widget.eventWindow()

	events, err := fetchICSResources(
		ctx,
		widget.Sources,
		widget.cachedSources,
		windowStart,
		windowEnd,
	)

	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	sortICSEvents(events)

	index := buildCalendarEventIndex(events)

	payload, marshalErr := json.Marshal(index)
	if marshalErr != nil {
		widget.withError(marshalErr)
		return
	}

	widget.Events = events
	widget.EventIndex = index
	widget.EventsJSON = template.JS(payload)
	widget.MinimumMonth = minimumMonth.Format("2006-01")
	widget.MaximumMonth = maximumMonth.Format("2006-01")
}

func (widget *calendarWidget) eventWindow() (time.Time, time.Time, time.Time, time.Time) {
	now := widget.now()
	location := now.Location()

	currentMonth := time.Date(
		now.Year(),
		now.Month(),
		1,
		0, 0, 0, 0,
		location,
	)

	minimumMonth := currentMonth.AddDate(0, -calendarMonthRange, 0)
	maximumMonth := currentMonth.AddDate(0, calendarMonthRange, 0)

	windowStart := calendarGridStart(minimumMonth, widget.FirstDay)
	windowEnd := calendarGridStart(
		maximumMonth,
		widget.FirstDay,
	).AddDate(0, 0, 42)

	return windowStart, windowEnd, minimumMonth, maximumMonth
}

func calendarGridStart(month time.Time, firstDay int) time.Time {
	first := time.Date(
		month.Year(),
		month.Month(),
		1,
		0, 0, 0, 0,
		month.Location(),
	)

	offset := (int(first.Weekday()) - firstDay + 7) % 7

	// Preserve the existing Calendar behavior where a month beginning on the
	// configured first weekday still displays the preceding week as spillover.
	if offset == 0 {
		offset = 7
	}

	return first.AddDate(0, 0, -offset)
}

func buildCalendarEventIndex(events []icsEvent) map[string][]calendarEventPayload {
	index := make(map[string][]calendarEventPayload)

	for _, event := range events {
		for _, date := range calendarEventDates(event) {
			key := date.Format("2006-01-02")

			index[key] = append(index[key], calendarEventPayload{
				UID:         event.UID,
				Title:       event.Title,
				Location:    event.Location,
				URL:         event.URL,
				SourceTitle: event.SourceTitle,
				Start:       event.Start.Format(time.RFC3339),
				End:         event.End.Format(time.RFC3339),
				AllDay:      event.AllDay,
			})
		}
	}

	return index
}

func calendarEventDates(event icsEvent) []time.Time {
	location := event.Start.Location()

	startDate := time.Date(
		event.Start.Year(),
		event.Start.Month(),
		event.Start.Day(),
		0, 0, 0, 0,
		location,
	)

	if event.End.Equal(event.Start) {
		return []time.Time{startDate}
	}

	// DTEND is exclusive for both all-day and timed events. Moving one
	// nanosecond inside the interval prevents an event ending exactly at
	// midnight from appearing on the following date.
	end := event.End.Add(-time.Nanosecond)

	endInStartLocation := end.In(location)
	endDate := time.Date(
		endInStartLocation.Year(),
		endInStartLocation.Month(),
		endInStartLocation.Day(),
		0, 0, 0, 0,
		location,
	)

	var dates []time.Time
	for date := startDate; !date.After(endDate); date = date.AddDate(0, 0, 1) {
		dates = append(dates, date)
	}

	return dates
}

func (widget *calendarWidget) setDefaultTimeout(value durationField) {
	applyICSSourceTimeoutDefault(widget.Sources, value)
}

func (widget *calendarWidget) setDefaultAllowInsecure(value bool) {
	applyICSSourceAllowInsecureDefault(widget.Sources, value)
}

func (widget *calendarWidget) setDefaultHeaders(value map[string]string) {
	applyICSSourceHeadersDefault(widget.Sources, value)
}

func (widget *calendarWidget) setDefaultBasicAuth(value basicAuthDefaults) {
	applyICSSourceBasicAuthDefault(widget.Sources, value)
}
