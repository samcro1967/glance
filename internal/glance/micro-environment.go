package glance

import (
	"fmt"
	"html/template"
	"strings"
)

type microEnvironment struct {
	environmentWidget `yaml:",inline"`
	microSummaryBase  `yaml:",inline"`
}

func (m *microEnvironment) GetPosition() int { return m.Position }
func (m *microEnvironment) initialize() error {
	d, e := validateMicroDisplay(m.Display, []string{"air-quality", "uv"}, "air-quality", "uv", "pollen")
	if e != nil {
		return e
	}
	m.Display = d
	return m.environmentWidget.initialize()
}
func (m *microEnvironment) Render() template.HTML { return renderMicroSummary(m) }
func (m *microEnvironment) MicroItems(open bool) []statusBarCompactItem {
	if m.AirQuality == nil && m.Pollen == nil {
		return compactError(m.Title, m.Error)
	}
	v := []string{}
	for _, f := range m.Display {
		switch f {
		case "air-quality":
			if m.AirQuality != nil {
				v = append(v, fmt.Sprintf("AQI %.0f %s", m.AirQuality.USAQI, m.AirQuality.USAQICategory()))
			}
		case "uv":
			if m.AirQuality != nil {
				v = append(v, fmt.Sprintf("UV %.1f %s", m.AirQuality.UVIndex, m.AirQuality.UVCategory()))
			}
		case "pollen":
			if p := m.Pollen.Current(); p != nil {
				v = append(v, "Pollen "+p.DisplayRisk())
			}
		}
	}
	if len(v) == 0 {
		return compactError(m.Title, m.Error)
	}
	loc := ""
	if m.Place != nil {
		loc = m.Place.Name
	}
	return []statusBarCompactItem{compactSummary(strings.Join(v, " · "), loc, "", open, m.Error, m.Notice)}
}
