package glance

import (
	"fmt"
	"html/template"
	"strings"
)

type microRepository struct {
	repositoryWidget `yaml:",inline"`
	microSummaryBase `yaml:",inline"`
}

func (m *microRepository) GetPosition() int { return m.Position }
func (m *microRepository) initialize() error {
	d, e := validateMicroDisplay(m.Display, []string{"stars", "pull-requests", "issues"}, "stars", "forks", "pull-requests", "issues")
	if e != nil {
		return e
	}
	m.Display = d
	return m.repositoryWidget.initialize()
}
func (m *microRepository) Render() template.HTML { return renderMicroSummary(m) }
func (m *microRepository) MicroItems(open bool) []statusBarCompactItem {
	if m.Repository.Name == "" {
		return compactError(m.Title, m.Error)
	}
	v := []string{}
	for _, f := range m.Display {
		switch f {
		case "stars":
			v = append(v, fmt.Sprintf("★ %d", m.Repository.Stars))
		case "forks":
			v = append(v, fmt.Sprintf("Forks %d", m.Repository.Forks))
		case "pull-requests":
			v = append(v, fmt.Sprintf("PR %d", m.Repository.OpenPullRequests))
		case "issues":
			v = append(v, fmt.Sprintf("Issues %d", m.Repository.OpenIssues))
		}
	}
	url := "https://github.com/" + strings.Trim(m.RequestedRepository, "/")
	return []statusBarCompactItem{compactSummary(m.Repository.Name, strings.Join(v, " · "), url, open, m.Error, m.Notice)}
}
