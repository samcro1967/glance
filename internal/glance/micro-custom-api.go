package glance

import "html/template"

var microCustomAPITemplate = mustParseTemplate("footer-micro-custom-api.html")

type microCustomAPI struct {
	customAPIWidget `yaml:",inline"`
	Position        int  `yaml:"position"`
	SameTab         bool `yaml:"same-tab"`
}

func (m *microCustomAPI) GetPosition() int { return m.Position }
func (m *microCustomAPI) initialize() error {
	m.statusBarCompactMode = true
	return m.customAPIWidget.initialize()
}
func (m *microCustomAPI) Render() template.HTML { return m.renderTemplate(m, microCustomAPITemplate) }
func (m *microCustomAPI) MicroItems(open bool) []statusBarCompactItem {
	return customAPIStatusBarCompactItems(&m.customAPIWidget, open)
}
func customAPIStatusBarCompactItems(m *customAPIWidget, open bool) []statusBarCompactItem {
	if len(m.StatusBarCompactItems) == 0 && m.Error != nil {
		return []statusBarCompactItem{{Kind: "error", Error: m.Error, ErrorTitle: m.Title}}
	}
	items := make([]statusBarCompactItem, 0, len(m.StatusBarCompactItems))
	for _, item := range m.StatusBarCompactItems {
		items = append(items, statusBarCompactItem{Kind: "custom-api", Error: m.Error, Notice: m.Notice, URL: item.URL, OpenLinksInNewTab: open, Icon1: m.resolveResourceProxyImageURL(item.Icon1), Line1: item.Line1, Line2: item.Line2, Icon2: m.resolveResourceProxyImageURL(item.Icon2)})
	}
	return items
}
