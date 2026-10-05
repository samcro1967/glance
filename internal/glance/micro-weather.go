package glance

import "html/template"

var microWeatherTemplate = mustParseTemplate("footer-micro-weather.html")

type microWeather struct {
	weatherWidget `yaml:",inline"`
	Position      int    `yaml:"position"`
	URL           string `yaml:"url"`
	SameTab       bool   `yaml:"same-tab"`
}

func (m *microWeather) GetPosition() int      { return m.Position }
func (m *microWeather) Render() template.HTML { return m.renderTemplate(m, microWeatherTemplate) }
func (m *microWeather) MicroItems(open bool) []statusBarCompactItem {
	return weatherStatusBarCompactItems(&m.weatherWidget, open)
}
func weatherStatusBarCompactItems(m *weatherWidget, open bool) []statusBarCompactItem {
	if m.Weather == nil || m.Place == nil {
		if m.Error != nil {
			return []statusBarCompactItem{{Kind: "error", Error: m.Error, ErrorTitle: m.Title}}
		}
		return nil
	}
	unit := "F"
	if m.Units == "metric" {
		unit = "C"
	}
	location := ""
	if !m.HideLocation {
		location = m.Place.Name
		if m.ShowAreaName && m.Place.Area != "" {
			location += ", " + m.Place.Area
		}
		if m.Place.Country != "" {
			location += ", " + m.Place.Country
		}
	}
	return []statusBarCompactItem{{Kind: "weather", Error: m.Error, Notice: m.Notice, WeatherCondition: m.Weather.WeatherCodeAsString(), WeatherTemperature: m.Weather.Temperature, WeatherFeelsLike: m.Weather.ApparentTemperature, WeatherUnit: unit, WeatherLocation: location}}
}
