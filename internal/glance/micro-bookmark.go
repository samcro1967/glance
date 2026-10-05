package glance

import (
	"errors"
	"html/template"
)

var microBookmarkTemplate = mustParseTemplate("micro-bookmark.html")

type microBookmark struct {
	widgetBase `yaml:",inline"`
	Position   int    `yaml:"position"`
	URL        string `yaml:"url"`
	SameTab    bool   `yaml:"same-tab"`
}

func (m *microBookmark) GetPosition() int { return m.Position }
func (m *microBookmark) initialize() error {
	if m.Title == "" {
		return errors.New("bookmark micro-widget title is required")
	}
	if m.URL == "" {
		return errors.New("bookmark micro-widget url is required")
	}
	return nil
}
func (m *microBookmark) setProviders(p *widgetProviders) {
	m.widgetBase.setProviders(p)
	if p != nil {
		m.Icon.resolveResourceProxy(p.resourceProxyURL)
	}
}
func (m *microBookmark) Render() template.HTML { return m.renderTemplate(m, microBookmarkTemplate) }
func (m *microBookmark) MicroItems(open bool) []statusBarCompactItem {
	return []statusBarCompactItem{{Kind: "bookmark", URL: m.URL, OpenLinksInNewTab: open, Line1: m.Title, Icon1: string(m.Icon.RenderURL())}}
}
