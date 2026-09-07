package glance

import (
	"context"
	"html/template"
	"time"
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

	cachedSources *icsSourceCache  `yaml:"-"`
	now           func() time.Time `yaml:"-"`
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

	if err := validateICSSources(widget.Sources, true); err != nil {
		return err
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
	widget.cachedSources = newICSSourceCache()
	widget.now = time.Now

	return nil
}

func (widget *icsEventsWidget) update(ctx context.Context) {
	events, err := widget.fetchEvents(ctx)

	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	sortICSEvents(events)

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
	windowStart := widget.now()
	windowEnd := windowStart.AddDate(0, 0, widget.DaysAhead)

	return fetchICSResources(ctx, widget.Sources, widget.cachedSources, windowStart, windowEnd)
}

func (widget *icsEventsWidget) fetchSource(ctx context.Context, source icsEventSource) ([]byte, error) {
	return fetchICSSource(ctx, source, widget.cachedSources)
}

func (widget *icsEventsWidget) parseSource(body []byte, source icsEventSource) ([]icsEvent, error) {
	windowStart := widget.now()
	windowEnd := windowStart.AddDate(0, 0, widget.DaysAhead)

	return parseICSResource(body, source, windowStart, windowEnd)
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

func (widget *icsEventsWidget) setDefaultLimit(value int) {
	widget.Limit = value
}

func (widget *icsEventsWidget) setDefaultCollapseAfter(value int) {
	widget.CollapseAfter = value
}

func (widget *icsEventsWidget) setDefaultTimeout(value durationField) {
	applyICSSourceTimeoutDefault(widget.Sources, value)
}

func (widget *icsEventsWidget) setDefaultAllowInsecure(value bool) {
	applyICSSourceAllowInsecureDefault(widget.Sources, value)
}

func (widget *icsEventsWidget) setDefaultHeaders(value map[string]string) {
	applyICSSourceHeadersDefault(widget.Sources, value)
}

func (widget *icsEventsWidget) setDefaultBasicAuth(value basicAuthDefaults) {
	applyICSSourceBasicAuthDefault(widget.Sources, value)
}
