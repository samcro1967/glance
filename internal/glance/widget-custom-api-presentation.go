package glance

import "html/template"

type customAPIPresentationTableColumn = presentationTableColumn
type customAPIPresentationTable = presentationTable
type customAPIPresentationChart = presentationChart
type customAPIPresentationConfig = presentationConfig

func normalizeCustomAPIPresentationTables(tables map[string]customAPIPresentationTable) error {
	return normalizePresentationTables(tables)
}

func normalizeCustomAPIPresentationCharts(charts map[string]customAPIPresentationChart) error {
	return normalizePresentationCharts(charts)
}

func customAPIPresentationJSON(tables map[string]customAPIPresentationTable, charts map[string]customAPIPresentationChart) (template.JS, error) {
	return presentationJSON(tables, charts)
}
