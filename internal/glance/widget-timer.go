package glance

import (
	"errors"
	"html/template"
)

var timerWidgetTemplate = mustParseTemplate("timer.html", "widget-base.html")

type timerWidget struct {
	widgetBase `yaml:",inline"`
	cachedHTML template.HTML `yaml:"-"`
	TimerID    string        `yaml:"id"`
	HourFormat string        `yaml:"hour-format"`
}

func (widget *timerWidget) initialize() error {
	widget.withTitle("Timers").withError(nil)

	if widget.HourFormat == "" {
		widget.HourFormat = "12h"
	} else if widget.HourFormat != "12h" && widget.HourFormat != "24h" {
		return errors.New("hour-format must be either 12h or 24h")
	}

	widget.cachedHTML = widget.renderTemplate(widget, timerWidgetTemplate)
	return nil
}

func (widget *timerWidget) Render() template.HTML {
	return widget.cachedHTML
}
