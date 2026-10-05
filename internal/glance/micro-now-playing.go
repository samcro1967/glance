package glance

import (
	"html/template"
	"strings"
)

type microNowPlaying struct {
	nowPlayingWidget `yaml:",inline"`
	Position         int  `yaml:"position"`
	SameTab          bool `yaml:"same-tab"`
}

func (m *microNowPlaying) GetPosition() int      { return m.Position }
func (m *microNowPlaying) Render() template.HTML { return renderMicroSummary(m) }
func (m *microNowPlaying) MicroItems(open bool) []statusBarCompactItem {
	if len(m.Items) == 0 {
		return compactError(m.Title, m.Error)
	}
	out := make([]statusBarCompactItem, 0, len(m.Items))
	for _, i := range m.Items {
		line2 := strings.TrimSpace(i.Subtitle)
		if line2 == "" {
			line2 = strings.TrimSpace(i.User)
		}
		prefix := "▶ "
		if strings.EqualFold(i.State, "paused") {
			prefix = "⏸ "
		}
		out = append(out, compactSummary(prefix+i.Title, line2, "", open, m.Error, m.Notice))
	}
	return out
}
