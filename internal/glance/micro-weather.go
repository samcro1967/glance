package glance

import (
	"context"
	"fmt"
	"html/template"
)

type microWeather struct {
	widgetBase   `yaml:",inline"`
	Position     int                         `yaml:"position"`
	Location     string                      `yaml:"location"`
	ShowAreaName bool                        `yaml:"show-area-name"`
	HideLocation bool                        `yaml:"hide-location"`
	Units        string                      `yaml:"units"`
	Place        *openMeteoPlaceResponseJson `yaml:"-"`
	Weather      *weather                    `yaml:"-"`
}

func (m *microWeather) GetPosition() int {
	return m.Position
}

func (m *microWeather) initialize() error {
	m.withCacheOnTheHour()

	if m.Location == "" {
		return fmt.Errorf("location is required")
	}

	if m.Units == "" {
		m.Units = "metric"
	}

	if m.Units != "metric" && m.Units != "imperial" {
		return fmt.Errorf("units must be either metric or imperial")
	}

	return nil
}

func (m *microWeather) update(ctx context.Context) {
	place, err := fetchOpenMeteoPlaceResource(ctx, m.Location)
	if !m.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	weather, err := fetchOpenMeteoWeatherResource(ctx, place, m.Units)
	if !m.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	m.Place = place
	m.Weather = weather
}

var microWeatherTemplate = mustParseTemplate("footer-micro-weather.html")

func (m *microWeather) Render() template.HTML {
	return m.renderTemplate(m, microWeatherTemplate)
}
