package glance

import (
	"errors"
	"html/template"
)

var microLinkTemplate = mustParseTemplate("micro-link.html")

type microLink struct {
	widgetBase `yaml:",inline"`
	Position   int    `yaml:"position"`
	URL        string `yaml:"url"`
	SameTab    bool   `yaml:"same-tab"`
}

func (m *microLink) GetPosition() int { return m.Position }
func (m *microLink) initialize() error {
	if m.Title == "" {
		return errors.New("link micro-widget title is required")
	}
	if m.URL == "" {
		return errors.New("link micro-widget url is required")
	}
	return nil
}
func (m *microLink) Render() template.HTML { return m.renderTemplate(m, microLinkTemplate) }
func (m *microLink) MicroItems(open bool) []statusBarCompactItem {
	return []statusBarCompactItem{{Kind: "link", URL: m.URL, OpenLinksInNewTab: open, Line1: m.Title}}
}
