package glance

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"time"
)

var astrologyWidgetTemplate = mustParseTemplate("astrology.html", "widget-base.html")

type astrologyWidget struct {
	widgetBase  `yaml:",inline"`
	At          string            `yaml:"at"`
	Timezone    string            `yaml:"timezone"`
	HourFormat  string            `yaml:"hour-format"`
	Sections    []string          `yaml:"sections"`
	ShowSunMoon bool              `yaml:"-"`
	ShowPlanets bool              `yaml:"-"`
	ShowAspects bool              `yaml:"-"`
	ShowEvents  bool              `yaml:"-"`
	Snapshot    astrologySnapshot `yaml:"-"`
	location    *time.Location
}

func (widget *astrologyWidget) initialize() error {
	widget.withTitle("Astrology").withCacheDuration(30 * time.Minute)
	if widget.HourFormat == "" {
		widget.HourFormat = "12h"
	}
	if widget.HourFormat != "12h" && widget.HourFormat != "24h" {
		return errors.New("hour-format must be either 12h or 24h")
	}
	widget.location = time.Local
	if widget.Timezone != "" {
		location, err := time.LoadLocation(widget.Timezone)
		if err != nil {
			return fmt.Errorf("loading timezone: %w", err)
		}
		widget.location = location
	}
	if widget.At != "" {
		if _, err := time.Parse(time.RFC3339, widget.At); err != nil {
			return fmt.Errorf("at must use RFC3339 format: %w", err)
		}
	}
	if len(widget.Sections) == 0 {
		widget.ShowSunMoon = true
		widget.ShowPlanets = true
		widget.ShowAspects = true
		widget.ShowEvents = true
		return nil
	}
	for _, section := range widget.Sections {
		switch section {
		case "sun-moon":
			widget.ShowSunMoon = true
		case "planets":
			widget.ShowPlanets = true
		case "aspects":
			widget.ShowAspects = true
		case "events":
			widget.ShowEvents = true
		default:
			return fmt.Errorf("unknown astrology section %q; valid sections are sun-moon, planets, aspects, events", section)
		}
	}
	return nil
}

func (widget *astrologyWidget) update(_ context.Context) {
	at := time.Now().In(widget.location)
	if widget.At != "" {
		pinned, _ := time.Parse(time.RFC3339, widget.At)
		at = pinned.In(widget.location)
	}
	snapshot := buildAstrologySnapshot(at)
	if !widget.canContinueUpdateAfterHandlingErr(nil) {
		return
	}
	widget.Snapshot = snapshot
}

func (widget *astrologyWidget) Render() template.HTML {
	return widget.renderTemplate(widget, astrologyWidgetTemplate)
}
func (widget *astrologyWidget) FormatEventTime(value time.Time) string {
	return formatAstronomyEventTime(value, widget.HourFormat)
}
