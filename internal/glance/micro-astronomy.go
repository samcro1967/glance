package glance

import (
	"fmt"
	"html/template"
	"strings"
)

type microAstronomy struct {
	astronomyWidget  `yaml:",inline"`
	microSummaryBase `yaml:",inline"`
}

func (m *microAstronomy) GetPosition() int { return m.Position }
func (m *microAstronomy) initialize() error {
	d, e := validateMicroDisplay(m.Display, []string{"moon-phase", "illumination", "sunset"}, "moon-phase", "illumination", "sunrise", "sunset", "moonrise", "moonset", "next-phase", "next-event")
	if e != nil {
		return e
	}
	m.Display = d
	return m.astronomyWidget.initialize()
}
func (m *microAstronomy) Render() template.HTML { return renderMicroSummary(m) }
func (m *microAstronomy) MicroItems(open bool) []statusBarCompactItem {
	if m.Snapshot.At.IsZero() {
		return compactError(m.Title, m.Error)
	}
	v := []string{}
	moon, sun := m.Snapshot.Moon, m.Snapshot.Sun
	for _, f := range m.Display {
		switch f {
		case "moon-phase":
			v = append(v, moon.PhaseGlyph+" "+moon.PhaseName)
		case "illumination":
			v = append(v, fmt.Sprintf("%d%%", moon.Illumination))
		case "sunrise":
			v = append(v, "Sunrise "+formatCompactTime(sun.Sunrise, m.HourFormat))
		case "sunset":
			v = append(v, "Sunset "+formatCompactTime(sun.Sunset, m.HourFormat))
		case "moonrise":
			v = append(v, "Moonrise "+formatCompactTime(moon.Rise, m.HourFormat))
		case "moonset":
			v = append(v, "Moonset "+formatCompactTime(moon.Set, m.HourFormat))
		case "next-phase":
			v = append(v, moon.NextPhaseName+" "+formatCompactTime(moon.NextPhaseTime, m.HourFormat))
		case "next-event":
			if len(m.Snapshot.Events) > 0 {
				e := m.Snapshot.Events[0]
				v = append(v, e.Name+" "+formatCompactTime(e.Time, m.HourFormat))
			}
		}
	}
	return []statusBarCompactItem{compactSummary(strings.Join(v, " · "), m.Observer.Label, "", open, m.Error, m.Notice)}
}
