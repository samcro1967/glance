package glance

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
)

var unitConverterWidgetTemplate = mustParseTemplate(
	"unit-converter.html",
	"widget-base.html",
)

//go:embed unit-converter-data.json
var unitConverterCatalogJSON []byte

type unitConverterWidget struct {
	widgetBase  `yaml:",inline"`
	cachedHTML  template.HTML `yaml:"-"`
	CatalogJSON template.JS   `yaml:"-"`
}

func (widget *unitConverterWidget) initialize() error {
	widget.withTitle("Unit Converter").withError(nil)

	if !json.Valid(unitConverterCatalogJSON) {
		return fmt.Errorf("embedded unit converter catalog is invalid JSON")
	}

	widget.CatalogJSON = template.JS(unitConverterCatalogJSON)
	widget.cachedHTML = widget.renderTemplate(
		widget,
		unitConverterWidgetTemplate,
	)

	return nil
}

func (widget *unitConverterWidget) Render() template.HTML {
	return widget.cachedHTML
}
