package glance

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"time"
)

var astronomyWidgetTemplate = mustParseTemplate("astronomy.html", "widget-base.html")

type astronomyWidget struct {
	widgetBase  `yaml:",inline"`
	Location    string            `yaml:"location"`
	Latitude    *float64          `yaml:"latitude"`
	Longitude   *float64          `yaml:"longitude"`
	Timezone    string            `yaml:"timezone"`
	At          string            `yaml:"at"`
	HourFormat  string            `yaml:"hour-format"`
	Sections    []string          `yaml:"sections"`
	ShowSun     bool              `yaml:"-"`
	ShowMoon    bool              `yaml:"-"`
	ShowPlanets bool              `yaml:"-"`
	ShowStars   bool              `yaml:"-"`
	ShowEvents  bool              `yaml:"-"`
	Observer    astronomyObserver `yaml:"-"`
	Snapshot    astronomySnapshot `yaml:"-"`
}

func (widget *astronomyWidget) initialize() error {
	widget.withTitle("Astronomy").withCacheDuration(15 * time.Minute)
	if widget.HourFormat == "" {
		widget.HourFormat = "12h"
	}
	if widget.HourFormat != "12h" && widget.HourFormat != "24h" {
		return errors.New("hour-format must be either 12h or 24h")
	}
	if widget.Location == "" && widget.Latitude == nil && widget.Longitude == nil && widget.Timezone == "" {
		return errors.New("location or latitude/longitude/timezone is required")
	}
	if widget.Location != "" && (widget.Latitude != nil || widget.Longitude != nil || widget.Timezone != "") {
		return errors.New("location cannot be combined with latitude, longitude, or timezone")
	}
	if err := validateAstronomyCoordinates(widget.Latitude, widget.Longitude, widget.Timezone); err != nil {
		return err
	}
	if widget.At != "" {
		if _, err := time.Parse(time.RFC3339, widget.At); err != nil {
			return fmt.Errorf("at must use RFC3339 format: %w", err)
		}
	}
	if len(widget.Sections) == 0 {
		widget.ShowSun = true
		widget.ShowMoon = true
		widget.ShowPlanets = true
		widget.ShowStars = true
		widget.ShowEvents = true
		return nil
	}

	for _, section := range widget.Sections {
		switch section {
		case "sun":
			widget.ShowSun = true
		case "moon":
			widget.ShowMoon = true
		case "planets":
			widget.ShowPlanets = true
		case "stars":
			widget.ShowStars = true
		case "events":
			widget.ShowEvents = true
		default:
			return fmt.Errorf("unknown astronomy section %q; valid sections are sun, moon, planets, stars, events", section)
		}
	}

	return nil
}

func (widget *astronomyWidget) update(ctx context.Context) {
	if widget.Observer.Timezone == nil {
		observer, err := widget.resolveObserver(ctx)
		if !widget.canContinueUpdateAfterHandlingErr(err) {
			return
		}
		widget.Observer = observer
	}
	at := time.Now().In(widget.Observer.Timezone)
	if widget.At != "" {
		pinned, _ := time.Parse(time.RFC3339, widget.At)
		at = pinned.In(widget.Observer.Timezone)
	}
	snapshot, err := buildAstronomySnapshot(at, widget.Observer, widget.ShowStars)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}
	widget.Snapshot = snapshot
}

func (widget *astronomyWidget) resolveObserver(ctx context.Context) (astronomyObserver, error) {
	if widget.Location != "" {
		place, err := fetchOpenMeteoPlaceResource(ctx, widget.Location)
		if err != nil {
			return astronomyObserver{}, err
		}
		return astronomyObserver{Latitude: place.Latitude, Longitude: place.Longitude, Timezone: place.location, Label: fmt.Sprintf("%s, %s", place.Name, place.Country)}, nil
	}
	location, err := time.LoadLocation(widget.Timezone)
	if err != nil {
		return astronomyObserver{}, fmt.Errorf("loading timezone: %w", err)
	}
	return astronomyObserver{Latitude: *widget.Latitude, Longitude: *widget.Longitude, Timezone: location, Label: widget.Timezone}, nil
}

func (widget *astronomyWidget) Render() template.HTML {
	return widget.renderTemplate(widget, astronomyWidgetTemplate)
}
func (widget *astronomyWidget) FormatTime(value time.Time) string {
	return formatAstronomyTime(value, widget.HourFormat)
}
func (widget *astronomyWidget) FormatEventTime(value time.Time) string {
	return formatAstronomyEventTime(value, widget.HourFormat)
}
