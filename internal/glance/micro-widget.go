package glance

import (
	"errors"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

type microWidget interface {
	GetPosition() int
	GetType() string
}

type dynamicMicroWidget interface {
	microWidget
	widget
}

type microWidgets []microWidget

const (
	defaultFooterMicroWidgetsPerSide = 5
	maxFooterMicroWidgetsPerSide     = 10
)

type footerMicroWidgets struct {
	MaxPerSide int          `yaml:"max-per-side"`
	Left       microWidgets `yaml:"left"`
	Right      microWidgets `yaml:"right"`
}

func (footer *footerMicroWidgets) dynamicWidgets() widgets {
	result := make(widgets, 0)

	for _, micro := range footer.Left {
		if dynamic, ok := micro.(dynamicMicroWidget); ok {
			result = append(result, dynamic)
		}
	}

	for _, micro := range footer.Right {
		if dynamic, ok := micro.(dynamicMicroWidget); ok {
			result = append(result, dynamic)
		}
	}

	return result
}

func (footer *footerMicroWidgets) UnmarshalYAML(unmarshal func(any) error) error {
	type footerMicroWidgetsPlain footerMicroWidgets

	if err := unmarshal((*footerMicroWidgetsPlain)(footer)); err != nil {
		return err
	}

	if footer.MaxPerSide == 0 {
		footer.MaxPerSide = defaultFooterMicroWidgetsPerSide
	}

	if footer.MaxPerSide < 1 || footer.MaxPerSide > maxFooterMicroWidgetsPerSide {
		return fmt.Errorf(
			"footer micro-widget max-per-side must be between 1 and %d, got %d",
			maxFooterMicroWidgetsPerSide,
			footer.MaxPerSide,
		)
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
	switch microType {
	case "bookmark":
		return &microBookmark{}, nil
	case "clock":
		return &microClock{}, nil
	case "weather":
		return &microWeather{}, nil
	case "markets":
		return &microMarkets{}, nil
	case "monitor":
		return &microMonitor{}, nil
	case "link":
		return &microLink{}, nil
	case "":
		return nil, errors.New("micro-widget 'type' property is empty or not specified")
	default:
		return nil, fmt.Errorf("unknown micro-widget type: %s", microType)
	}
}

func (widgets *microWidgets) UnmarshalYAML(node *yaml.Node) error {
	var nodes []yaml.Node

	if err := node.Decode(&nodes); err != nil {
		return err
	}

	decoded := make(microWidgets, 0, len(nodes))

	for _, itemNode := range nodes {
		meta := struct {
			Type string `yaml:"type"`
		}{}

		if err := itemNode.Decode(&meta); err != nil {
			return err
		}

		micro, err := newMicroWidget(meta.Type)
		if err != nil {
			return fmt.Errorf("line %d: %w", itemNode.Line, err)
		}

		if err := itemNode.Decode(micro); err != nil {
			return err
		}

		decoded = append(decoded, micro)
	}

	sort.SliceStable(decoded, func(i, j int) bool {
		return decoded[i].GetPosition() < decoded[j].GetPosition()
	})

	*widgets = decoded
	return nil
}

func validateMicroWidgetPositions(widgets microWidgets, maxPerSide int) error {
	seen := make(map[int]struct{}, len(widgets))

	for _, micro := range widgets {
		position := micro.GetPosition()

		if position < 1 || position > maxPerSide {
			return fmt.Errorf(
				"position must be between 1 and %d, got %d",
				maxPerSide,
				position,
			)
		}

		if _, exists := seen[position]; exists {
			return fmt.Errorf("position %d is configured more than once", position)
		}

		seen[position] = struct{}{}
	}

	return nil
}
