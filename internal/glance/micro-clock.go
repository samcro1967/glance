package glance

import (
	"errors"
	"fmt"
	"html/template"
	"time"
)

var microClockTemplate = mustParseTemplate("micro-clock.html")

type microClock struct {
	widgetBase `yaml:",inline"`
	Position   int    `yaml:"position"`
	HourFormat string `yaml:"hour-format"`
	Timezone   string `yaml:"timezone"`
	Label      string `yaml:"label"`
}

func (m *microClock) GetPosition() int { return m.Position }
func (m *microClock) initialize() error {
	m.withError(nil)
	if m.HourFormat == "" {
		m.HourFormat = "24h"
	}
	if m.HourFormat != "12h" && m.HourFormat != "24h" {
		return errors.New("clock micro-widget hour-format must be either 12h or 24h")
	}
	if m.Timezone != "" {
		if _, err := time.LoadLocation(m.Timezone); err != nil {
			return fmt.Errorf("invalid timezone %q: %w", m.Timezone, err)
		}
	}
	return nil
}
func (m *microClock) Render() template.HTML { return m.renderTemplate(m, microClockTemplate) }
func (m *microClock) MicroItems(bool) []statusBarCompactItem {
	return []statusBarCompactItem{{Kind: "clock", Line1: m.Label, ClockHourFormat: m.HourFormat, ClockTimezone: m.Timezone}}
}
