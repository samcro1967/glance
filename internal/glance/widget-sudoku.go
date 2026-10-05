package glance

import (
	"fmt"
	"html/template"
)

var (
	sudokuWidgetTemplate         = mustParseTemplate("sudoku.html", "widget-base.html")
	sudokuWidgetExpandedTemplate = mustParseTemplate("sudoku-expanded.html")
)

type sudokuWidget struct {
	widgetBase `yaml:",inline"`
	Difficulty string        `yaml:"difficulty"`
	cachedHTML template.HTML `yaml:"-"`
}

func (widget *sudokuWidget) initialize() error {
	if widget.Difficulty == "" {
		widget.Difficulty = "easy"
	}

	switch widget.Difficulty {
	case "easy", "medium", "hard":
	default:
		return fmt.Errorf("invalid sudoku difficulty %q: must be easy, medium, or hard", widget.Difficulty)
	}

	widget.withTitle("Sudoku").withError(nil)
	widget.cachedHTML = widget.renderTemplate(widget, sudokuWidgetTemplate)
	return nil
}

func (widget *sudokuWidget) Render() template.HTML {
	return widget.cachedHTML
}

func (widget *sudokuWidget) RenderExpanded() template.HTML {
	return widget.renderTemplate(widget, sudokuWidgetExpandedTemplate)
}
