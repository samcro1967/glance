package glance

import (
	"fmt"
	"html/template"
	"strings"
)

type microServerStats struct {
	serverStatsWidget `yaml:",inline"`
	microSummaryBase  `yaml:",inline"`
}

func (m *microServerStats) GetPosition() int { return m.Position }
func (m *microServerStats) initialize() error {
	d, e := validateMicroDisplay(m.Display, []string{"cpu", "memory", "disk"}, "cpu", "memory", "disk", "temperature", "uptime", "swap")
	if e != nil {
		return e
	}
	m.Display = d
	return m.serverStatsWidget.initialize()
}
func (m *microServerStats) Render() template.HTML { return renderMicroSummary(m) }
func (m *microServerStats) MicroItems(open bool) []statusBarCompactItem {
	if len(m.Servers) == 0 {
		return compactError(m.Title, m.Error)
	}
	out := make([]statusBarCompactItem, 0, len(m.Servers))
	for i := range m.Servers {
		s := &m.Servers[i]
		name := s.Name
		if name == "" && s.Info != nil {
			name = s.Info.Hostname
		}
		if name == "" {
			name = "Server"
		}
		if !s.IsReachable || s.Info == nil {
			out = append(out, compactSummary(name, "Unreachable", "", open, m.Error, m.Notice))
			continue
		}
		v := []string{}
		for _, f := range m.Display {
			switch f {
			case "cpu":
				if s.Info.CPU.LoadIsAvailable {
					v = append(v, fmt.Sprintf("CPU %d%%", s.Info.CPU.Load1Percent))
				}
			case "memory":
				if s.Info.Memory.IsAvailable {
					v = append(v, fmt.Sprintf("RAM %d%%", s.Info.Memory.UsedPercent))
				}
			case "disk":
				if len(s.Info.Mountpoints) > 0 {
					v = append(v, fmt.Sprintf("Disk %d%%", s.Info.Mountpoints[0].UsedPercent))
				}
			case "temperature":
				if s.Info.CPU.TemperatureIsAvailable {
					v = append(v, fmt.Sprintf("%d°C", s.Info.CPU.TemperatureC))
				}
			case "swap":
				if s.Info.Memory.SwapIsAvailable {
					v = append(v, fmt.Sprintf("Swap %d%%", s.Info.Memory.SwapUsedPercent))
				}
			case "uptime":
				if s.Info.HostInfoIsAvailable {
					v = append(v, "Up since "+s.Info.BootTime.Format("Jan 2"))
				}
			}
		}
		out = append(out, compactSummary(name, strings.Join(v, " · "), "", open, m.Error, m.Notice))
	}
	return out
}
