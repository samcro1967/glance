package glance

import (
	"context"
	"fmt"
	"html/template"
	"time"
)

var microDockerWidgetTemplate = mustParseTemplate("footer-micro-docker.html")

type microDocker struct {
	widgetBase `yaml:",inline"`
	Position   int `yaml:"position"`

	Container            string                       `yaml:"container"`
	Summary              bool                         `yaml:"summary"`
	SockPath             string                       `yaml:"sock-path"`
	HideByDefault        bool                         `yaml:"hide-by-default"`
	RunningOnly          bool                         `yaml:"running-only"`
	Category             string                       `yaml:"category"`
	FormatContainerNames bool                         `yaml:"format-container-names"`
	LabelOverrides       map[string]map[string]string `yaml:"containers"`

	Name    string `yaml:"name"`
	URL     string `yaml:"url"`
	SameTab bool   `yaml:"same-tab"`

	Selected   *dockerContainer    `yaml:"-"`
	Containers dockerContainerList `yaml:"-"`
	OKCount    int                 `yaml:"-"`
	TotalCount int                 `yaml:"-"`
}

func (m *microDocker) initialize() error {
	m.withCacheDuration(time.Minute)

	if m.SockPath == "" {
		m.SockPath = "/var/run/docker.sock"
	}

	if m.Container != "" && m.Summary {
		return fmt.Errorf("docker micro-widget: container and summary cannot both be configured")
	}

	if m.Container == "" && !m.Summary {
		return fmt.Errorf("docker micro-widget: either container or summary is required")
	}

	return nil
}

func (m *microDocker) update(ctx context.Context) {
	containers, err := fetchDockerContainers(
		ctx,
		m.SockPath,
		m.HideByDefault,
		m.Category,
		m.RunningOnly,
		m.FormatContainerNames,
		m.LabelOverrides,
		nil,
	)
	if !m.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	m.Containers = containers
	m.Selected = nil
	m.OKCount = 0
	m.TotalCount = len(containers)

	for i := range containers {
		container := &containers[i]

		if container.StateIcon == dockerContainerStateIconOK {
			m.OKCount++
		}

		if m.Container != "" && dockerContainerMatchesConfiguredName(container, m.Container) {
			selected := *container
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

func dockerContainerMatchesConfiguredName(container *dockerContainer, configured string) bool {
	if container.Name == configured {
		return true
	}

	return false
}

func (m *microDocker) GetPosition() int {
	return m.Position
}

func (m *microDocker) Render() template.HTML {
	return m.renderTemplate(m, microDockerWidgetTemplate)
}
