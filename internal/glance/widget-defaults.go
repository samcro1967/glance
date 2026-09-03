package glance

import "fmt"

// widgetDefaultsConfig represents user-configured defaults. Pointer fields
// preserve the distinction between an omitted value and an explicit zero,
// false, or empty value.
type widgetDefaultsConfig struct {
	Global widgetDefaultValues            `yaml:"global"`
	Types  map[string]widgetDefaultValues `yaml:"types"`
}

type widgetDefaultValues struct {
	Title             *string            `yaml:"title"`
	TitleURL          *string            `yaml:"title-url"`
	HideHeader        *bool              `yaml:"hide-header"`
	CSSClass          *string            `yaml:"css-class"`
	Cache             *durationField     `yaml:"cache"`
	NewTab            *bool              `yaml:"new-tab"`
	Limit             *int               `yaml:"limit"`
	CollapseAfter     *int               `yaml:"collapse-after"`
	CollapseAfterRows *int               `yaml:"collapse-after-rows"`
	Timeout           *durationField     `yaml:"timeout"`
	AllowInsecure     *bool              `yaml:"allow-insecure"`
	Headers           map[string]string  `yaml:"headers"`
	BasicAuth         *basicAuthDefaults `yaml:"basic-auth"`
	Proxy             *proxyOptionsField `yaml:"proxy"`
}

type basicAuthDefaults struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

func (d widgetDefaultValues) configuredCapabilities() []widgetCapability {
	var capabilities []widgetCapability

	if d.Title != nil {
		capabilities = append(capabilities, widgetCapabilityTitle)
	}
	if d.TitleURL != nil {
		capabilities = append(capabilities, widgetCapabilityTitleURL)
	}
	if d.HideHeader != nil {
		capabilities = append(capabilities, widgetCapabilityHideHeader)
	}
	if d.CSSClass != nil {
		capabilities = append(capabilities, widgetCapabilityCSSClass)
	}
	if d.Cache != nil {
		capabilities = append(capabilities, widgetCapabilityCache)
	}
	if d.NewTab != nil {
		capabilities = append(capabilities, widgetCapabilityNewTab)
	}
	if d.Limit != nil {
		capabilities = append(capabilities, widgetCapabilityLimit)
	}
	if d.CollapseAfter != nil {
		capabilities = append(capabilities, widgetCapabilityCollapseAfter)
	}
	if d.CollapseAfterRows != nil {
		capabilities = append(capabilities, widgetCapabilityCollapseAfterRows)
	}
	if d.Timeout != nil {
		capabilities = append(capabilities, widgetCapabilityTimeout)
	}
	if d.AllowInsecure != nil {
		capabilities = append(capabilities, widgetCapabilityAllowInsecure)
	}
	if d.Headers != nil {
		capabilities = append(capabilities, widgetCapabilityHeaders)
	}
	if d.BasicAuth != nil {
		capabilities = append(capabilities, widgetCapabilityBasicAuth)
	}
	if d.Proxy != nil {
		capabilities = append(capabilities, widgetCapabilityProxy)
	}

	return capabilities
}

func validateWidgetDefaults(defaults widgetDefaultsConfig) error {
	for _, capability := range defaults.Global.configuredCapabilities() {
		supported := false
		for widgetType := range registeredWidgetTypes {
			if widgetSupportsCapability(widgetType, capability, widgetCapabilityScopeGlobal) {
				supported = true
				break
			}
		}
		if !supported {
			return fmt.Errorf("widget-defaults.global.%s is not globally configurable", capability)
		}
	}

	for widgetType, values := range defaults.Types {
		if _, ok := registeredWidgetTypes[widgetType]; !ok {
			return fmt.Errorf("widget-defaults.types contains unknown widget type %q", widgetType)
		}

		for _, capability := range values.configuredCapabilities() {
			if !widgetSupportsCapability(widgetType, capability, widgetCapabilityScopeType) {
				return fmt.Errorf(
					"widget-defaults.types.%s.%s is not supported by widget type %q",
					widgetType,
					capability,
					widgetType,
				)
			}
		}
	}

	return nil
}

type widgetLimitSetter interface {
	setDefaultLimit(int)
}

type widgetCollapseAfterSetter interface {
	setDefaultCollapseAfter(int)
}

type widgetCollapseAfterRowsSetter interface {
	setDefaultCollapseAfterRows(int)
}

type widgetTimeoutSetter interface {
	setDefaultTimeout(durationField)
}

type widgetAllowInsecureSetter interface {
	setDefaultAllowInsecure(bool)
}

type widgetHeadersSetter interface {
	setDefaultHeaders(map[string]string)
}

type widgetBasicAuthSetter interface {
	setDefaultBasicAuth(basicAuthDefaults)
}

type widgetProxySetter interface {
	setDefaultProxy(proxyOptionsField) error
}

type widgetNewTabSetter interface {
	setDefaultNewTab(bool)
}

func resolveWidgetDefaultValues(widgetType string, defaults widgetDefaultsConfig) widgetDefaultValues {
	resolved := defaults.Global

	if typeDefaults, ok := defaults.Types[widgetType]; ok {
		if typeDefaults.Title != nil {
			resolved.Title = typeDefaults.Title
		}
		if typeDefaults.TitleURL != nil {
			resolved.TitleURL = typeDefaults.TitleURL
		}
		if typeDefaults.HideHeader != nil {
			resolved.HideHeader = typeDefaults.HideHeader
		}
		if typeDefaults.CSSClass != nil {
			resolved.CSSClass = typeDefaults.CSSClass
		}
		if typeDefaults.Cache != nil {
			resolved.Cache = typeDefaults.Cache
		}
		if typeDefaults.NewTab != nil {
			resolved.NewTab = typeDefaults.NewTab
		}
		if typeDefaults.Limit != nil {
			resolved.Limit = typeDefaults.Limit
		}
		if typeDefaults.CollapseAfter != nil {
			resolved.CollapseAfter = typeDefaults.CollapseAfter
		}
		if typeDefaults.CollapseAfterRows != nil {
			resolved.CollapseAfterRows = typeDefaults.CollapseAfterRows
		}
		if typeDefaults.Timeout != nil {
			resolved.Timeout = typeDefaults.Timeout
		}
		if typeDefaults.AllowInsecure != nil {
			resolved.AllowInsecure = typeDefaults.AllowInsecure
		}
		if typeDefaults.Headers != nil {
			resolved.Headers = mergeStringMaps(resolved.Headers, typeDefaults.Headers)
		}
		if typeDefaults.BasicAuth != nil {
			resolved.BasicAuth = typeDefaults.BasicAuth
		}
		if typeDefaults.Proxy != nil {
			resolved.Proxy = typeDefaults.Proxy
		}
	}

	return resolved
}

func mergeStringMaps(base, override map[string]string) map[string]string {
	if base == nil && override == nil {
		return nil
	}

	merged := make(map[string]string, len(base)+len(override))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range override {
		merged[key] = value
	}

	return merged
}

func applyWidgetCapabilityDefaults(candidate widget, defaults widgetDefaultsConfig) error {
	base, ok := widgetBaseOf(candidate)
	if !ok {
		return nil
	}

	resolved := resolveWidgetDefaultValues(candidate.GetType(), defaults)

	if resolved.NewTab != nil &&
		widgetSupportsCapability(candidate.GetType(), widgetCapabilityNewTab, widgetCapabilityScopeType) &&
		!base.configuredFields["new-tab"] {
		if setter, ok := candidate.(widgetNewTabSetter); ok {
			setter.setDefaultNewTab(*resolved.NewTab)
		}
	}

	if resolved.Limit != nil &&
		widgetSupportsCapability(candidate.GetType(), widgetCapabilityLimit, widgetCapabilityScopeType) &&
		!base.configuredFields["limit"] {
		if setter, ok := candidate.(widgetLimitSetter); ok {
			setter.setDefaultLimit(*resolved.Limit)
		}
	}

	if resolved.CollapseAfter != nil &&
		widgetSupportsCapability(candidate.GetType(), widgetCapabilityCollapseAfter, widgetCapabilityScopeType) &&
		!base.configuredFields["collapse-after"] {
		if setter, ok := candidate.(widgetCollapseAfterSetter); ok {
			setter.setDefaultCollapseAfter(*resolved.CollapseAfter)
		}
	}

	if resolved.CollapseAfterRows != nil &&
		widgetSupportsCapability(candidate.GetType(), widgetCapabilityCollapseAfterRows, widgetCapabilityScopeType) &&
		!base.configuredFields["collapse-after-rows"] {
		if setter, ok := candidate.(widgetCollapseAfterRowsSetter); ok {
			setter.setDefaultCollapseAfterRows(*resolved.CollapseAfterRows)
		}
	}

	if resolved.Timeout != nil &&
		widgetSupportsCapability(candidate.GetType(), widgetCapabilityTimeout, widgetCapabilityScopeType) &&
		!base.configuredFields["timeout"] {
		if setter, ok := candidate.(widgetTimeoutSetter); ok {
			setter.setDefaultTimeout(*resolved.Timeout)
		}
	}

	if resolved.AllowInsecure != nil &&
		widgetSupportsCapability(candidate.GetType(), widgetCapabilityAllowInsecure, widgetCapabilityScopeType) &&
		!base.configuredFields["allow-insecure"] {
		if setter, ok := candidate.(widgetAllowInsecureSetter); ok {
			setter.setDefaultAllowInsecure(*resolved.AllowInsecure)
		}
	}

	if resolved.Headers != nil &&
		widgetSupportsCapability(candidate.GetType(), widgetCapabilityHeaders, widgetCapabilityScopeType) {
		if setter, ok := candidate.(widgetHeadersSetter); ok {
			setter.setDefaultHeaders(resolved.Headers)
		}
	}

	if resolved.BasicAuth != nil &&
		widgetSupportsCapability(candidate.GetType(), widgetCapabilityBasicAuth, widgetCapabilityScopeType) {
		if setter, ok := candidate.(widgetBasicAuthSetter); ok {
			setter.setDefaultBasicAuth(*resolved.BasicAuth)
		}
	}

	if resolved.Proxy != nil &&
		widgetSupportsCapability(candidate.GetType(), widgetCapabilityProxy, widgetCapabilityScopeType) &&
		!base.configuredFields["proxy"] {
		if setter, ok := candidate.(widgetProxySetter); ok {
			if err := setter.setDefaultProxy(*resolved.Proxy); err != nil {
				return err
			}
		}
	}

	return nil
}

func applyWidgetBaseDefaults(candidate widget, defaults widgetDefaultsConfig) {
	base, ok := widgetBaseOf(candidate)
	if !ok {
		return
	}

	resolved := resolveWidgetDefaultValues(candidate.GetType(), defaults)

	if resolved.Title != nil && !base.configuredFields["title"] {
		base.Title = *resolved.Title
	}
	if resolved.TitleURL != nil && !base.configuredFields["title-url"] {
		base.TitleURL = *resolved.TitleURL
	}
	if resolved.HideHeader != nil && !base.configuredFields["hide-header"] {
		base.HideHeader = *resolved.HideHeader
	}
	if resolved.CSSClass != nil && !base.configuredFields["css-class"] {
		base.CSSClass = *resolved.CSSClass
	}
	if resolved.Cache != nil && !base.configuredFields["cache"] {
		base.CustomCacheDuration = *resolved.Cache
	}
}

func widgetBaseOf(candidate widget) (*widgetBase, bool) {
	type widgetBaseProvider interface {
		getWidgetBase() *widgetBase
	}

	provider, ok := candidate.(widgetBaseProvider)
	if !ok {
		return nil, false
	}

	return provider.getWidgetBase(), true
}

func applyWidgetDefaultsTree(
	candidate widget,
	defaults widgetDefaultsConfig,
) (widgetDefaultsLogSummary, error) {
	summary := widgetDefaultsLogSummary{widgets: 1}

	for _, field := range widgetDefaultOverrideFields(candidate, defaults) {
		summary.overrides = append(summary.overrides, widgetDefaultOverrideLog{
			widgetType: candidate.GetType(),
			field:      field,
		})
	}

	applyWidgetBaseDefaults(candidate, defaults)

	if err := applyWidgetCapabilityDefaults(candidate, defaults); err != nil {
		return widgetDefaultsLogSummary{}, err
	}

	container, ok := candidate.(widgetContainer)
	if !ok {
		return summary, nil
	}

	for _, child := range container.childWidgets() {
		childSummary, err := applyWidgetDefaultsTree(child, defaults)
		if err != nil {
			return widgetDefaultsLogSummary{}, err
		}
		summary.add(childSummary)
	}

	return summary, nil
}
