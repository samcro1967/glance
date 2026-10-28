package glance

import "html/template"

var stopwatchWidgetTemplate = mustParseTemplate("stopwatch.html", "widget-base.html")

type stopwatchWidget struct {
	widgetBase  `yaml:",inline"`
	StartOnOpen bool          `yaml:"start-on-open"`
	cachedHTML  template.HTML `yaml:"-"`
}

func (widget *stopwatchWidget) initialize() error {
	widget.withTitle("Stopwatch").withError(nil)
	widget.cacheStaticTemplate(widget, stopwatchWidgetTemplate, &widget.cachedHTML)
	return nil
}

func (widget *stopwatchWidget) Render() template.HTML {
	return widget.renderCachedTemplate()
}
