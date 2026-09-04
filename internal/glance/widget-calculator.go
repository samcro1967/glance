package glance

import "html/template"

var calculatorWidgetTemplate = mustParseTemplate(
	"calculator.html",
	"widget-base.html",
)

type calculatorWidget struct {
	widgetBase `yaml:",inline"`
	cachedHTML template.HTML `yaml:"-"`
}

func (widget *calculatorWidget) initialize() error {
	widget.withTitle("Calculator").withError(nil)
	widget.cachedHTML = widget.renderTemplate(
		widget,
		calculatorWidgetTemplate,
	)

	return nil
}

func (widget *calculatorWidget) Render() template.HTML {
	return widget.cachedHTML
}
