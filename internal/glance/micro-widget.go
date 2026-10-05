package glance

import (
	"errors"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

type microSource interface {
	widget
	MicroItems(openLinksInNewTab bool) []statusBarCompactItem
}

type microWidget interface {
	microSource
	GetPosition() int
}

type microWidgets []widget

type microWidgetDescriptor struct {
	constructor         func() microWidget
	canonicalWidgetType string
	applyWidgetDefaults bool
}

// microWidgetRegistry is the authoritative registry of micro-widget types.
// A type registered here is available to every micro-widget container. The
// containers own placement and layout only; individual micro-widgets own
// configuration, data lifecycle, and compact presentation semantics.
var microWidgetRegistry = map[string]microWidgetDescriptor{
	"bookmark": {constructor: func() microWidget { return &microBookmark{} }},
	"clock":    {constructor: func() microWidget { return &microClock{} }},
	"custom-api": {
		constructor:         func() microWidget { return &microCustomAPI{} },
		canonicalWidgetType: "custom-api",
		applyWidgetDefaults: true,
	},
	"docker": {
		constructor:         func() microWidget { return &microDocker{} },
		canonicalWidgetType: "docker-containers",
		applyWidgetDefaults: true,
	},
	"link": {constructor: func() microWidget { return &microLink{} }},
	"markets": {
		constructor:         func() microWidget { return &microMarkets{} },
		canonicalWidgetType: "markets",
		applyWidgetDefaults: true,
	},
	"monitor": {
		constructor:         func() microWidget { return &microMonitor{} },
		canonicalWidgetType: "monitor",
		applyWidgetDefaults: true,
	},
	"rss": {
		constructor:         func() microWidget { return &microRSS{} },
		canonicalWidgetType: "rss",
		applyWidgetDefaults: true,
	},
	"weather": {
		constructor:         func() microWidget { return &microWeather{} },
		canonicalWidgetType: "weather",
		applyWidgetDefaults: true,
	},
}

func widgetDefaultsType(candidate widget) string {
	micro, ok := candidate.(microWidget)
	if !ok {
		return candidate.GetType()
	}
	descriptor, ok := microWidgetRegistry[micro.GetType()]
	if !ok || descriptor.canonicalWidgetType == "" {
		return candidate.GetType()
	}
	return descriptor.canonicalWidgetType
}

type invalidConfiguredMicroWidget struct {
	*invalidConfiguredWidget
	Position    int
	ConfigError error
}

func (m *invalidConfiguredMicroWidget) GetPosition() int { return m.Position }
func (m *invalidConfiguredMicroWidget) GetType() string  { return "config-error" }
func (m *invalidConfiguredMicroWidget) MicroItems(bool) []statusBarCompactItem {
	return []statusBarCompactItem{{Kind: "error", Error: m.Error, ErrorTitle: m.Title}}
}

const (
	defaultFooterMicroWidgetsPerSide = 5
	maxFooterMicroWidgetsPerSide     = 10
)

type footerMicroWidgets struct {
	MaxPerSide int          `yaml:"max-per-side"`
	Left       microWidgets `yaml:"left"`
	Right      microWidgets `yaml:"right"`
}

func (items microWidgets) asWidgets() widgets { return widgets(items) }

func (footer *footerMicroWidgets) allWidgets() widgets {
	result := make(widgets, 0, len(footer.Left)+len(footer.Right))
	result = append(result, footer.Left...)
	result = append(result, footer.Right...)
	return result
}

func (footer *footerMicroWidgets) UnmarshalYAML(unmarshal func(any) error) error {
	type plain footerMicroWidgets
	if err := unmarshal((*plain)(footer)); err != nil {
		return err
	}
	if footer.MaxPerSide == 0 {
		footer.MaxPerSide = defaultFooterMicroWidgetsPerSide
	}
	if footer.MaxPerSide < 1 || footer.MaxPerSide > maxFooterMicroWidgetsPerSide {
		return fmt.Errorf("footer micro-widget max-per-side must be between 1 and %d, got %d", maxFooterMicroWidgetsPerSide, footer.MaxPerSide)
	}
	if err := validateMicroWidgetPositions(footer.Left, footer.MaxPerSide); err != nil {
		return fmt.Errorf("footer micro-widgets left: %w", err)
	}
	if err := validateMicroWidgetPositions(footer.Right, footer.MaxPerSide); err != nil {
		return fmt.Errorf("footer micro-widgets right: %w", err)
	}
	return nil
}

func newMicroWidget(microType string) (microWidget, error) {
	if microType == "" {
		return nil, errors.New("micro-widget 'type' property is empty or not specified")
	}
	descriptor, ok := microWidgetRegistry[microType]
	if !ok {
		return nil, fmt.Errorf("unknown micro-widget type: %s", microType)
	}
	candidate := descriptor.constructor()
	candidate.setID(widgetIDCounter.Add(1))
	if base, ok := widgetBaseOf(candidate); ok {
		base.Type = microType
		base.OpenLinksInNewTab = true
	}
	return candidate, nil
}

func (items *microWidgets) UnmarshalYAML(node *yaml.Node) error {
	var nodes []yaml.Node
	if err := node.Decode(&nodes); err != nil {
		return err
	}
	decoded := make(microWidgets, 0, len(nodes))
	for _, itemNode := range nodes {
		meta := struct {
			Type     string `yaml:"type"`
			Position int    `yaml:"position"`
		}{}
		if err := itemNode.Decode(&meta); err != nil {
			decoded = append(decoded, newInvalidMicroWidget(nil, meta.Type, meta.Position, widgetConfigErrorLine(err, itemNode.Line), err))
			continue
		}
		candidate, err := newMicroWidget(meta.Type)
		if err != nil {
			decoded = append(decoded, newInvalidMicroWidget(nil, meta.Type, meta.Position, itemNode.Line, fmt.Errorf("%s micro-widget: %w", meta.Type, err)))
			continue
		}
		if err := itemNode.Decode(candidate); err != nil {
			decoded = append(decoded, newInvalidMicroWidget(candidate, meta.Type, meta.Position, widgetConfigErrorLine(err, itemNode.Line), err))
			continue
		}
		if base, ok := widgetBaseOf(candidate); ok {
			base.configuredFields = yamlMappingFields(&itemNode)
			base.configLine = itemNode.Line
		}
		decoded = append(decoded, candidate)
	}
	sort.SliceStable(decoded, func(i, j int) bool {
		return decoded[i].(microWidget).GetPosition() < decoded[j].(microWidget).GetPosition()
	})
	*items = decoded
	return nil
}

func newInvalidMicroWidget(candidate widget, microType string, position int, line int, err error) *invalidConfiguredMicroWidget {
	invalid := newInvalidConfiguredWidget(candidate, microType, line, err)
	return &invalidConfiguredMicroWidget{invalidConfiguredWidget: invalid, Position: position, ConfigError: err}
}

func validateMicroWidgetPositions(items microWidgets, maxPerSide int) error {
	seen := make(map[int]struct{}, len(items))
	for _, candidate := range items {
		if _, invalid := candidate.(*invalidConfiguredMicroWidget); invalid {
			continue
		}
		micro := candidate.(microWidget)
		position := micro.GetPosition()
		if position < 1 || position > maxPerSide {
			return fmt.Errorf("position must be between 1 and %d, got %d", maxPerSide, position)
		}
		if _, exists := seen[position]; exists {
			return fmt.Errorf("position %d is configured more than once", position)
		}
		seen[position] = struct{}{}
	}
	return nil
}
