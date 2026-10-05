package glance

import (
	"fmt"
	"html/template"
	"strings"
)

type microDNSStats struct {
	dnsStatsWidget   `yaml:",inline"`
	microSummaryBase `yaml:",inline"`
}

func (m *microDNSStats) GetPosition() int { return m.Position }
func (m *microDNSStats) initialize() error {
	d, e := validateMicroDisplay(m.Display, []string{"queries", "blocked", "latency"}, "queries", "blocked", "latency", "domains")
	if e != nil {
		return e
	}
	m.Display = d
	return m.dnsStatsWidget.initialize()
}
func (m *microDNSStats) Render() template.HTML { return renderMicroSummary(m) }
func (m *microDNSStats) MicroItems(open bool) []statusBarCompactItem {
	if m.Stats == nil {
		return compactError(m.Title, m.Error)
	}
	v := []string{}
	for _, f := range m.Display {
		switch f {
		case "queries":
			v = append(v, fmt.Sprintf("%d queries", m.Stats.TotalQueries))
		case "blocked":
			v = append(v, fmt.Sprintf("%d%% blocked", m.Stats.BlockedPercent))
		case "latency":
			v = append(v, m.Stats.FormattedResponseTime()+"ms")
		case "domains":
			v = append(v, fmt.Sprintf("%d domains", m.Stats.DomainsBlocked))
		}
	}
	return []statusBarCompactItem{compactSummary(m.Title, strings.Join(v, " · "), m.URL, open, m.Error, m.Notice)}
}
