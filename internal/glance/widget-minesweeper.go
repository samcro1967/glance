package glance

import (
	"fmt"
	"html/template"
)

var (
	minesweeperWidgetTemplate         = mustParseTemplate("minesweeper.html", "widget-base.html")
	minesweeperWidgetExpandedTemplate = mustParseTemplate("minesweeper-expanded.html")
)

type minesweeperWidget struct {
	widgetBase `yaml:",inline"`
	Difficulty string        `yaml:"difficulty"`
	cachedHTML template.HTML `yaml:"-"`
}

func (widget *minesweeperWidget) initialize() error {
	if widget.Difficulty == "" {
		widget.Difficulty = "beginner"
	}

	switch widget.Difficulty {
	case "beginner", "intermediate", "expert":
	default:
		return fmt.Errorf("invalid minesweeper difficulty %q: must be beginner, intermediate, or expert", widget.Difficulty)
	}

	widget.withTitle("Minesweeper").withError(nil)
	widget.cachedHTML = widget.renderTemplate(widget, minesweeperWidgetTemplate)
	return nil
}

func (widget *minesweeperWidget) Render() template.HTML {
	return widget.cachedHTML
}

func (widget *minesweeperWidget) RenderExpanded() template.HTML {
	return widget.renderTemplate(widget, minesweeperWidgetExpandedTemplate)
}
