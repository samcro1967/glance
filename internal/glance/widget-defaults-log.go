package glance

import (
	"log/slog"
	"sort"
	"time"
)

type widgetDefaultOverrideLog struct {
	widgetType string
	field      string
}

type widgetDefaultsLogSummary struct {
	widgets   int
	overrides []widgetDefaultOverrideLog
}

func (s *widgetDefaultsLogSummary) add(other widgetDefaultsLogSummary) {
	s.widgets += other.widgets
	s.overrides = append(s.overrides, other.overrides...)
}

func logWidgetDefaultsConfigured(
	defaults widgetDefaultsConfig,
	summary widgetDefaultsLogSummary,
) {
	globalCapabilities := defaults.Global.configuredCapabilities()

	if len(globalCapabilities) == 0 && len(defaults.Types) == 0 {
		return
	}

	slog.Info(
		"Widget defaults configured",
		"global_capabilities", len(globalCapabilities),
		"types", len(defaults.Types),
		"widgets", summary.widgets,
		"overrides", len(summary.overrides),
	)

	if len(globalCapabilities) > 0 {
		logWidgetDefaultValues("Widget defaults global", "", defaults.Global)
	}

	types := make([]string, 0, len(defaults.Types))
	for widgetType := range defaults.Types {
		types = append(types, widgetType)
	}
	sort.Strings(types)

	for _, widgetType := range types {
		logWidgetDefaultValues(
			"Widget defaults type",
			widgetType,
			defaults.Types[widgetType],
		)
	}

	logWidgetDefaultOverrideSummary(summary.overrides)
}

func logWidgetDefaultOverrideSummary(overrides []widgetDefaultOverrideLog) {
	type overrideSummary struct {
		overrides int
		fields    map[string]struct{}
	}

	byType := make(map[string]*overrideSummary)

	for _, override := range overrides {
		entry, ok := byType[override.widgetType]
		if !ok {
			entry = &overrideSummary{fields: make(map[string]struct{})}
			byType[override.widgetType] = entry
		}

		entry.overrides++
		entry.fields[override.field] = struct{}{}
	}

	types := make([]string, 0, len(byType))
	for widgetType := range byType {
		types = append(types, widgetType)
	}
	sort.Strings(types)

	for _, widgetType := range types {
		entry := byType[widgetType]

		fields := make([]string, 0, len(entry.fields))
		for field := range entry.fields {
			fields = append(fields, field)
		}
		sort.Strings(fields)

		slog.Info(
			"Widget defaults overrides",
			"type", widgetType,
			"overrides", entry.overrides,
			"fields", fields,
		)
	}
}

func logWidgetDefaultValues(
	message string,
	widgetType string,
	values widgetDefaultValues,
) {
	args := make([]any, 0, 30)

	if widgetType != "" {
		args = append(args, "type", widgetType)
	}

	if values.Title != nil {
		args = append(args, "title", *values.Title)
	}
	if values.TitleURL != nil {
		args = append(args, "title_url", *values.TitleURL)
	}
	if values.HideHeader != nil {
		args = append(args, "hide_header", *values.HideHeader)
	}
	if values.CSSClass != nil {
		args = append(args, "css_class", *values.CSSClass)
	}
	if values.Cache != nil {
		args = append(args, "cache", time.Duration(*values.Cache).String())
	}
	if values.NewTab != nil {
		args = append(args, "new_tab", *values.NewTab)
	}
	if values.Limit != nil {
		args = append(args, "limit", *values.Limit)
	}
	if values.CollapseAfter != nil {
		args = append(args, "collapse_after", *values.CollapseAfter)
	}
	if values.CollapseAfterRows != nil {
		args = append(args, "collapse_after_rows", *values.CollapseAfterRows)
	}
	if values.Timeout != nil {
		args = append(args, "timeout", time.Duration(*values.Timeout).String())
	}
	if values.AllowInsecure != nil {
		args = append(args, "allow_insecure", *values.AllowInsecure)
	}
	if values.Headers != nil {
		args = append(args, "headers", sortedWidgetDefaultHeaderNames(values.Headers))
	}
	if values.BasicAuth != nil {
		args = append(args, "basic_auth", "configured")
	}
	if values.Proxy != nil {
		args = append(args, "proxy", "configured")
	}

	slog.Info(message, args...)
}

func sortedWidgetDefaultHeaderNames(headers map[string]string) []string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func widgetDefaultOverrideFields(candidate widget, defaults widgetDefaultsConfig) []string {
	base, ok := widgetBaseOf(candidate)
	if !ok {
		return nil
	}

	resolved := resolveWidgetDefaultValues(candidate.GetType(), defaults)
	capabilities := resolved.configuredCapabilities()
	fields := make([]string, 0, len(capabilities))

	for _, capability := range capabilities {
		field := string(capability)

		if !widgetSupportsCapability(
			candidate.GetType(),
			capability,
			widgetCapabilityScopeType,
		) {
			continue
		}

		if base.configuredFields[field] {
			fields = append(fields, field)
		}
	}

	sort.Strings(fields)
	return fields
}
