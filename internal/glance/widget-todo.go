package glance

import (
	"html/template"
)

var todoWidgetTemplate = mustParseTemplate("todo.html", "widget-base.html")

type todoWidget struct {
	widgetBase `yaml:",inline"`
	cachedHTML template.HTML `yaml:"-"`
	TodoID     string        `yaml:"id"`
}

func (widget *todoWidget) initialize() error {
	widget.withTitle("To-do").withError(nil)

	widget.cacheStaticTemplate(widget, todoWidgetTemplate, &widget.cachedHTML)
	return nil
}

func (widget *todoWidget) Render() template.HTML {
	return widget.renderCachedTemplate()
}
