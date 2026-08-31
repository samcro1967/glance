package glance

import (
	"errors"
	"fmt"
	"html/template"
	"time"
)

var analogClockWidgetTemplate = mustParseTemplate("analog-clock.html", "widget-base.html")

type analogClockTimezone struct {
	Timezone string `yaml:"timezone"`
	Label    string `yaml:"label"`
}

type analogClockWidget struct {
	widgetBase        `yaml:",inline"`
	cachedHTML        template.HTML         `yaml:"-"`
	HideAmPmIndicator bool                  `yaml:"hide-am-pm-indicator"`
	HideDate          bool                  `yaml:"hide-date"`
	DialMarkers       string                `yaml:"dial-markers"`
	Timezones         []analogClockTimezone `yaml:"timezones"`
}

func (widget *analogClockWidget) initialize() error {
	widget.withTitle("Clock").withError(nil)

	if widget.DialMarkers == "" {
		widget.DialMarkers = "NumericalFull"
	}

	switch widget.DialMarkers {
	case "NumericalFull", "NumericalMinimal", "None":
	default:
		return errors.New("dial-markers must be either NumericalFull, NumericalMinimal, or None")
	}

	for _, timezone := range widget.Timezones {
		if timezone.Timezone == "" {
			return errors.New("missing timezone value")
		}

		if _, err := time.LoadLocation(timezone.Timezone); err != nil {
			return fmt.Errorf("invalid timezone '%s': %v", timezone.Timezone, err)
		}
	}

	widget.cachedHTML = widget.renderTemplate(widget, analogClockWidgetTemplate)

	return nil
}

func (widget *analogClockWidget) Render() template.HTML {
	return widget.cachedHTML
}
