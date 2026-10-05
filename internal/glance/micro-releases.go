package glance

import "html/template"

type microReleases struct {
	releasesWidget `yaml:",inline"`
	Position       int  `yaml:"position"`
	SameTab        bool `yaml:"same-tab"`
}

func (m *microReleases) GetPosition() int      { return m.Position }
func (m *microReleases) Render() template.HTML { return renderMicroSummary(m) }
func (m *microReleases) MicroItems(open bool) []statusBarCompactItem {
	if len(m.Releases) == 0 {
		return compactError(m.Title, m.Error)
	}
	out := make([]statusBarCompactItem, 0, len(m.Releases))
	for _, r := range m.Releases {
		out = append(out, compactSummary(r.Name, r.Version, r.NotesUrl, open, m.Error, m.Notice))
	}
	return out
}
