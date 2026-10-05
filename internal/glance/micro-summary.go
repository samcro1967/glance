package glance

import (
	"fmt"
	"html/template"
	"slices"
	"strings"
	"time"
)

var microSummaryTemplate = mustParseTemplate("footer-micro-summary.html")

type microSummaryBase struct {
	Position int      `yaml:"position"`
	SameTab  bool     `yaml:"same-tab"`
	Display  []string `yaml:"display"`
}

func validateMicroDisplay(display, defaults []string, allowed ...string) ([]string, error) {
	if len(display) == 0 {
		return append([]string(nil), defaults...), nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(display))
	for _, raw := range display {
		v := strings.ToLower(strings.TrimSpace(raw))
		if !slices.Contains(allowed, v) {
			return nil, fmt.Errorf("unknown display value %q; valid values are %s", raw, strings.Join(allowed, ", "))
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out, nil
}

type microTemplateRenderer interface {
	renderTemplate(any, *template.Template) template.HTML
}

func renderMicroSummary(w microTemplateRenderer) template.HTML {
	return w.renderTemplate(w, microSummaryTemplate)
}
func compactError(title string, err error) []statusBarCompactItem {
	if err == nil {
		return nil
	}
	return []statusBarCompactItem{{Kind: "error", Error: err, ErrorTitle: title}}
}
func compactSummary(line1, line2, url string, open bool, err, notice error) statusBarCompactItem {
	return statusBarCompactItem{Kind: "summary", Line1: line1, Line2: line2, URL: url, OpenLinksInNewTab: open, Error: err, Notice: notice}
}
func formatCompactTime(v time.Time, h string) string {
	if v.IsZero() {
		return ""
	}
	if h == "24h" {
		return v.Format("15:04")
	}
	return v.Format("3:04 PM")
}
