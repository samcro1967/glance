package glance

import "html/template"

var (
	game2048WidgetTemplate         = mustParseTemplate("2048.html", "widget-base.html")
	game2048WidgetExpandedTemplate = mustParseTemplate("2048-expanded.html")
)

type game2048Widget struct {
	widgetBase `yaml:",inline"`
	cachedHTML template.HTML `yaml:"-"`
}

func (widget *game2048Widget) initialize() error {
	widget.withTitle("2048").withError(nil)
	widget.cachedHTML = widget.renderTemplate(widget, game2048WidgetTemplate)
	return nil
}

func (widget *game2048Widget) Render() template.HTML {
	return widget.cachedHTML
}

func (widget *game2048Widget) RenderExpanded() template.HTML {
	return widget.renderTemplate(widget, game2048WidgetExpandedTemplate)
}
