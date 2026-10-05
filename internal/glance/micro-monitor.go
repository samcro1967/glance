package glance

import (
	"fmt"
	"html/template"
)

var microMonitorTemplate = mustParseTemplate("footer-micro-monitor.html")

type microMonitor struct {
	monitorWidget `yaml:",inline"`
	Position      int `yaml:"position"`
}

func (m *microMonitor) GetPosition() int { return m.Position }
func (m *microMonitor) initialize() error {
	if len(m.Sites) == 0 {
		return fmt.Errorf("at least one site is required")
	}
	return m.monitorWidget.initialize()
}
func (m *microMonitor) Render() template.HTML { return m.renderTemplate(m, microMonitorTemplate) }
func (m *microMonitor) MicroItems(open bool) []statusBarCompactItem {
	return monitorStatusBarCompactItems(&m.monitorWidget, open)
}
func monitorStatusBarCompactItems(m *monitorWidget, open bool) []statusBarCompactItem {
	if len(m.Sites) == 0 && m.Error != nil {
		return []statusBarCompactItem{{Kind: "error", Error: m.Error, ErrorTitle: m.Title}}
	}
	if m.ShowFailingOnly && !m.HasFailing {
		return []statusBarCompactItem{{Kind: "monitor", Line1: "All online", StatusStyle: "ok"}}
	}
	items := make([]statusBarCompactItem, 0, len(m.Sites))
	for i := range m.Sites {
		s := &m.Sites[i]
		if m.ShowFailingOnly && s.StatusStyle == "ok" {
			continue
		}
		items = append(items, statusBarCompactItem{Kind: "monitor", URL: s.URL, OpenLinksInNewTab: open, Line1: s.Title, Icon1: string(s.Icon.RenderURL()), StatusStyle: s.StatusStyle, Error: m.Error, Notice: m.Notice})
	}
	return items
}
