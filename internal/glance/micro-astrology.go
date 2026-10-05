package glance

import (
	"html/template"
	"strings"
)

type microAstrology struct {
	astrologyWidget  `yaml:",inline"`
	microSummaryBase `yaml:",inline"`
}

func (m *microAstrology) GetPosition() int { return m.Position }
func (m *microAstrology) initialize() error {
	d, e := validateMicroDisplay(m.Display, []string{"sun", "moon"}, "sun", "moon", "aspect", "next-event")
	if e != nil {
		return e
	}
	m.Display = d
	return m.astrologyWidget.initialize()
}
func (m *microAstrology) Render() template.HTML { return renderMicroSummary(m) }
func (m *microAstrology) MicroItems(open bool) []statusBarCompactItem {
	if m.Snapshot.At.IsZero() {
		return compactError(m.Title, m.Error)
	}
	v := []string{}
	body := func(n string) *astrologyBody {
		for i := range m.Snapshot.Bodies {
			if strings.EqualFold(m.Snapshot.Bodies[i].Name, n) {
				return &m.Snapshot.Bodies[i]
			}
		}
		return nil
	}
	for _, f := range m.Display {
		switch f {
		case "sun", "moon":
			if b := body(f); b != nil {
				x := b.Glyph + " " + b.SignGlyph + " " + b.Sign
				if b.Retrograde {
					x += " ℞"
				}
				v = append(v, x)
			}
		case "aspect":
			if len(m.Snapshot.Aspects) > 0 {
				a := m.Snapshot.Aspects[0]
				v = append(v, a.LeftGlyph+" "+a.Glyph+" "+a.RightGlyph+" "+a.Name)
			}
		case "next-event":
			if len(m.Snapshot.Events) > 0 {
				e := m.Snapshot.Events[0]
				v = append(v, e.Name+" "+formatCompactTime(e.Time, m.HourFormat))
			}
		}
	}
	return []statusBarCompactItem{compactSummary(strings.Join(v, " · "), "", "", open, m.Error, m.Notice)}
}
