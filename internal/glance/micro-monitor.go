package glance

import (
	"context"
	"fmt"
	"html/template"
	"slices"
	"time"
)

type microMonitor struct {
	widgetBase      `yaml:",inline"`
	Position        int           `yaml:"position"`
	Sites           []monitorSite `yaml:"sites"`
	ShowFailingOnly bool          `yaml:"show-failing-only"`
	HasFailing      bool          `yaml:"-"`
}

func (m *microMonitor) GetPosition() int {
	return m.Position
}

func (m *microMonitor) initialize() error {
	m.withCacheDuration(5 * time.Minute)

	if len(m.Sites) == 0 {
		return fmt.Errorf("at least one site is required")
	}

	return nil
}

func (m *microMonitor) update(ctx context.Context) {
	requests := make([]*SiteStatusRequest, len(m.Sites))

	for i := range m.Sites {
		requests[i] = m.Sites[i].SiteStatusRequest
	}

	statuses, err := fetchStatusForSites(ctx, requests)
	if !m.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	m.HasFailing = false

	for i := range m.Sites {
		site := &m.Sites[i]
		status := &statuses[i]
		site.Status = status

		if !slices.Contains(site.AltStatusCodes, status.Code) && (status.Code >= 400 || status.Error != nil) {
			m.HasFailing = true
		}

		if status.Error != nil && site.ErrorURL != "" {
			site.URL = site.ErrorURL
		} else {
			site.URL = site.DefaultURL
		}

		site.StatusText = statusCodeToText(status.Code, site.AltStatusCodes)
		site.StatusStyle = statusCodeToStyle(status.Code, site.AltStatusCodes)

		if status.Error != nil {
			site.StatusText = "Error"
			site.StatusStyle = "error"
		}
	}
}

var microMonitorTemplate = mustParseTemplate("footer-micro-monitor.html")

func (m *microMonitor) Render() template.HTML {
	return m.renderTemplate(m, microMonitorTemplate)
}
