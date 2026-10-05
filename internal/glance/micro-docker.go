package glance

import (
	"fmt"
	"html/template"
)

var microDockerWidgetTemplate = mustParseTemplate("footer-micro-docker.html")

type microDocker struct {
	dockerContainersWidget `yaml:",inline"`
	Position               int              `yaml:"position"`
	Container              string           `yaml:"container"`
	Summary                bool             `yaml:"summary"`
	Name                   string           `yaml:"name"`
	URL                    string           `yaml:"url"`
	SameTab                bool             `yaml:"same-tab"`
	Selected               *dockerContainer `yaml:"-"`
	OKCount                int              `yaml:"-"`
	TotalCount             int              `yaml:"-"`
}

func (m *microDocker) GetPosition() int { return m.Position }
func (m *microDocker) initialize() error {
	if m.Container != "" && m.Summary {
		return fmt.Errorf("docker micro-widget: container and summary cannot both be configured")
	}
	if m.Container == "" && !m.Summary {
		return fmt.Errorf("docker micro-widget: either container or summary is required")
	}
	return m.dockerContainersWidget.initialize()
}
func (m *microDocker) prepareMicroState() {
	m.Selected = nil
	m.OKCount = 0
	m.TotalCount = len(m.Containers)
	for i := range m.Containers {
		c := &m.Containers[i]
		if c.StateIcon == dockerContainerStateIconOK {
			m.OKCount++
		}
		if m.Container != "" && c.Name == m.Container {
			selected := *c
			if m.Name != "" {
				selected.Name = m.Name
			}
			if m.URL != "" {
				selected.URL = m.URL
				selected.SameTab = m.SameTab
			}
			m.Selected = &selected
		}
	}
}
func (m *microDocker) Render() template.HTML {
	m.prepareMicroState()
	return m.renderTemplate(m, microDockerWidgetTemplate)
}
func (m *microDocker) MicroItems(open bool) []statusBarCompactItem {
	m.prepareMicroState()
	if m.Summary {
		style := "error"
		if m.TotalCount > 0 && m.OKCount == m.TotalCount {
			style = "ok"
		}
		return []statusBarCompactItem{{Kind: "docker", Line1: "Docker", Line2: fmt.Sprintf("%d/%d", m.OKCount, m.TotalCount), StatusStyle: style, Error: m.Error, Notice: m.Notice}}
	}
	if m.Selected != nil {
		style := "error"
		if m.Selected.StateIcon == dockerContainerStateIconOK {
			style = "ok"
		}
		return []statusBarCompactItem{{Kind: "docker", Line1: m.Selected.Name, URL: m.Selected.URL, OpenLinksInNewTab: open, StatusStyle: style, Error: m.Error, Notice: m.Notice}}
	}
	return []statusBarCompactItem{{Kind: "docker", Line1: m.Container, StatusStyle: "error", Error: m.Error, Notice: m.Notice}}
}
