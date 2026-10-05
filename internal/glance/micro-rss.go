package glance

import "html/template"

var microRSSTemplate = mustParseTemplate("footer-micro-rss.html")

type microRSS struct {
	rssWidget `yaml:",inline"`
	Position  int  `yaml:"position"`
	SameTab   bool `yaml:"same-tab"`
}

func (m *microRSS) GetPosition() int      { return m.Position }
func (m *microRSS) Render() template.HTML { return m.renderTemplate(m, microRSSTemplate) }
func (m *microRSS) MicroItems(open bool) []statusBarCompactItem {
	return rssStatusBarCompactItems(&m.rssWidget, open)
}
func rssStatusBarCompactItems(m *rssWidget, open bool) []statusBarCompactItem {
	if len(m.Items) == 0 && m.Error != nil {
		return []statusBarCompactItem{{Kind: "error", Error: m.Error, ErrorTitle: m.Title}}
	}
	items := make([]statusBarCompactItem, 0, len(m.Items))
	for _, item := range m.Items {
		items = append(items, statusBarCompactItem{Kind: "rss", Error: m.Error, Notice: m.Notice, URL: item.Link, OpenLinksInNewTab: open, RSSTitle: item.Title, RSSChannelName: item.ChannelName, RSSChannelURL: item.ChannelURL, RSSPublishedAt: item.PublishedAt})
	}
	return items
}
