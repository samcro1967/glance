package glance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"strings"
)

var extensionPresentationTemplate = mustParseTemplate("extension-presentation.html")

type extensionPresentationEnvelope struct {
	Blocks []json.RawMessage `json:"blocks"`
}

type extensionPresentationBlock struct {
	Type      string
	Text      string
	Style     string
	Variant   string
	Label     string
	Value     string
	Percent   float64
	Items     any
	Columns   []extensionPresentationTableColumn
	Rows      []extensionPresentationTableRow
	TableName string
	ChartName string
	ChartData template.JS
}

type extensionPresentation struct {
	Blocks           []extensionPresentationBlock
	PresentationJSON template.JS
}

type extensionPresentationMetric struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
type extensionPresentationKeyValue struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
type extensionPresentationListItem struct {
	Title string `json:"title"`
	Meta  string `json:"meta,omitempty"`
	URL   string `json:"url,omitempty"`
}
type extensionPresentationCardStatus struct {
	Text    string `json:"text"`
	Variant string `json:"variant"`
}
type extensionPresentationCard struct {
	Title  string                           `json:"title"`
	Text   string                           `json:"text,omitempty"`
	Status *extensionPresentationCardStatus `json:"status,omitempty"`
}
type extensionPresentationTableColumn struct {
	Key      string
	Label    string
	Type     string
	Priority int
}
type extensionPresentationTableCell struct {
	Key   string
	Value string
}
type extensionPresentationTableRow struct {
	Cells []extensionPresentationTableCell
}

type extensionPresentationBlockType struct {
	Type string `json:"type"`
}
type extensionPresentationTextBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Style string `json:"style,omitempty"`
}
type extensionPresentationMetricsBlock struct {
	Type  string                        `json:"type"`
	Items []extensionPresentationMetric `json:"items"`
}
type extensionPresentationBadgeBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type extensionPresentationStatusBlock struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Variant string `json:"variant"`
}
type extensionPresentationKeyValuesBlock struct {
	Type  string                          `json:"type"`
	Items []extensionPresentationKeyValue `json:"items"`
}
type extensionPresentationProgressBlock struct {
	Type  string  `json:"type"`
	Label string  `json:"label,omitempty"`
	Value float64 `json:"value"`
	Text  string  `json:"text,omitempty"`
}
type extensionPresentationStateBlock struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Variant string `json:"variant"`
}
type extensionPresentationListBlock struct {
	Type  string                          `json:"type"`
	Items []extensionPresentationListItem `json:"items"`
}
type extensionPresentationCardsBlock struct {
	Type  string                      `json:"type"`
	Items []extensionPresentationCard `json:"items"`
}
type extensionPresentationTableColumnJSON struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type,omitempty"`
	Priority int    `json:"priority,omitempty"`
}
type extensionPresentationTableBlock struct {
	Type       string                                 `json:"type"`
	Columns    []extensionPresentationTableColumnJSON `json:"columns"`
	Rows       []map[string]any                       `json:"rows"`
	Responsive *bool                                  `json:"responsive,omitempty"`
	Sortable   *bool                                  `json:"sortable,omitempty"`
	Search     *bool                                  `json:"search,omitempty"`
	Pagination *bool                                  `json:"pagination,omitempty"`
	PageSize   int                                    `json:"page-size,omitempty"`
}
type extensionPresentationChartBlock struct {
	Type      string          `json:"type"`
	ChartType string          `json:"chart-type"`
	Height    int             `json:"height,omitempty"`
	Legend    *bool           `json:"legend,omitempty"`
	Min       *float64        `json:"min,omitempty"`
	Max       *float64        `json:"max,omitempty"`
	Unit      string          `json:"unit,omitempty"`
	Stacked   bool            `json:"stacked,omitempty"`
	Data      json.RawMessage `json:"data"`
}

type extensionPresentationSeries struct {
	Label  string    `json:"label,omitempty"`
	Values []float64 `json:"values"`
}
type extensionPresentationStandardChartData struct {
	Labels []string                      `json:"labels"`
	Series []extensionPresentationSeries `json:"series"`
}
type extensionPresentationCircularChartData struct {
	Labels []string  `json:"labels"`
	Values []float64 `json:"values"`
}
type extensionPresentationSparklineData struct {
	Values []float64 `json:"values"`
}
type extensionPresentationGaugeData struct {
	Value float64 `json:"value"`
	Label string  `json:"label,omitempty"`
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func validateExtensionPresentationVariant(value string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("unsupported variant %q", value)
}

func renderExtensionPresentation(content []byte) (template.HTML, error) {
	var envelope extensionPresentationEnvelope
	if err := decodeStrictJSON(content, &envelope); err != nil {
		return "", fmt.Errorf("decode presentation-v1: %w", err)
	}
	if envelope.Blocks == nil {
		return "", errors.New("presentation-v1 requires a blocks array")
	}

	presentation := extensionPresentation{}
	tables := map[string]presentationTable{}
	charts := map[string]presentationChart{}

	for index, raw := range envelope.Blocks {
		block, table, chart, err := decodeExtensionPresentationBlock(index, raw)
		if err != nil {
			return "", fmt.Errorf("block %d: %w", index, err)
		}
		presentation.Blocks = append(presentation.Blocks, block)
		if table != nil {
			tables[block.TableName] = *table
		}
		if chart != nil {
			charts[block.ChartName] = *chart
		}
	}
	if err := normalizePresentationTables(tables); err != nil {
		return "", err
	}
	if err := normalizePresentationCharts(charts); err != nil {
		return "", err
	}
	config, err := presentationJSON(tables, charts)
	if err != nil {
		return "", err
	}
	presentation.PresentationJSON = config
	rendered, err := executeTemplateToString(extensionPresentationTemplate, presentation)
	if err != nil {
		return "", fmt.Errorf("render presentation-v1: %w", err)
	}
	return template.HTML(rendered), nil
}

func decodeExtensionPresentationBlock(index int, raw json.RawMessage) (extensionPresentationBlock, *presentationTable, *presentationChart, error) {
	var kind extensionPresentationBlockType
	if err := json.Unmarshal(raw, &kind); err != nil {
		return extensionPresentationBlock{}, nil, nil, err
	}
	switch kind.Type {
	case "text":
		var value extensionPresentationTextBlock
		if err := decodeStrictJSON(raw, &value); err != nil {
			return extensionPresentationBlock{}, nil, nil, err
		}
		if value.Style == "" {
			value.Style = "normal"
		}
		if err := validateExtensionPresentationVariant(value.Style, "normal", "secondary", "muted"); err != nil {
			return extensionPresentationBlock{}, nil, nil, err
		}
		return extensionPresentationBlock{Type: kind.Type, Text: value.Text, Style: value.Style}, nil, nil, nil
	case "metrics":
		var value extensionPresentationMetricsBlock
		if err := decodeStrictJSON(raw, &value); err != nil {
			return extensionPresentationBlock{}, nil, nil, err
		}
		return extensionPresentationBlock{Type: kind.Type, Items: value.Items}, nil, nil, nil
	case "badge":
		var value extensionPresentationBadgeBlock
		if err := decodeStrictJSON(raw, &value); err != nil {
			return extensionPresentationBlock{}, nil, nil, err
		}
		return extensionPresentationBlock{Type: kind.Type, Text: value.Text}, nil, nil, nil
	case "status":
		var value extensionPresentationStatusBlock
		if err := decodeStrictJSON(raw, &value); err != nil {
			return extensionPresentationBlock{}, nil, nil, err
		}
		if err := validateExtensionPresentationVariant(value.Variant, "positive", "negative", "warning", "neutral"); err != nil {
			return extensionPresentationBlock{}, nil, nil, err
		}
		return extensionPresentationBlock{Type: kind.Type, Text: value.Text, Variant: value.Variant}, nil, nil, nil
	case "key-values":
		var value extensionPresentationKeyValuesBlock
		if err := decodeStrictJSON(raw, &value); err != nil {
			return extensionPresentationBlock{}, nil, nil, err
		}
		return extensionPresentationBlock{Type: kind.Type, Items: value.Items}, nil, nil, nil
	case "progress":
		var value extensionPresentationProgressBlock
		if err := decodeStrictJSON(raw, &value); err != nil {
			return extensionPresentationBlock{}, nil, nil, err
		}
		if value.Value < 0 || value.Value > 100 {
			return extensionPresentationBlock{}, nil, nil, errors.New("progress value must be between 0 and 100")
		}
		return extensionPresentationBlock{Type: kind.Type, Label: value.Label, Percent: value.Value, Text: value.Text}, nil, nil, nil
	case "state":
		var value extensionPresentationStateBlock
		if err := decodeStrictJSON(raw, &value); err != nil {
			return extensionPresentationBlock{}, nil, nil, err
		}
		if err := validateExtensionPresentationVariant(value.Variant, "empty", "warning", "error", "degraded"); err != nil {
			return extensionPresentationBlock{}, nil, nil, err
		}
		return extensionPresentationBlock{Type: kind.Type, Text: value.Text, Variant: value.Variant}, nil, nil, nil
	case "list":
		var value extensionPresentationListBlock
		if err := decodeStrictJSON(raw, &value); err != nil {
			return extensionPresentationBlock{}, nil, nil, err
		}
		for i := range value.Items {
			if value.Items[i].URL != "" {
				if _, err := validateExtensionTitleURL(value.Items[i].URL); err != nil {
					return extensionPresentationBlock{}, nil, nil, fmt.Errorf("list item URL: %w", err)
				}
			}
		}
		return extensionPresentationBlock{Type: kind.Type, Items: value.Items}, nil, nil, nil
	case "cards":
		var value extensionPresentationCardsBlock
		if err := decodeStrictJSON(raw, &value); err != nil {
			return extensionPresentationBlock{}, nil, nil, err
		}
		for _, item := range value.Items {
			if item.Status != nil {
				if err := validateExtensionPresentationVariant(item.Status.Variant, "positive", "negative", "warning", "neutral"); err != nil {
					return extensionPresentationBlock{}, nil, nil, err
				}
			}
		}
		return extensionPresentationBlock{Type: kind.Type, Items: value.Items}, nil, nil, nil
	case "table":
		return decodeExtensionPresentationTable(index, raw)
	case "chart":
		return decodeExtensionPresentationChart(index, raw)
	default:
		return extensionPresentationBlock{}, nil, nil, fmt.Errorf("unsupported block type %q", kind.Type)
	}
}

func decodeExtensionPresentationTable(index int, raw json.RawMessage) (extensionPresentationBlock, *presentationTable, *presentationChart, error) {
	var value extensionPresentationTableBlock
	if err := decodeStrictJSON(raw, &value); err != nil {
		return extensionPresentationBlock{}, nil, nil, err
	}
	if len(value.Columns) == 0 {
		return extensionPresentationBlock{}, nil, nil, errors.New("table requires columns")
	}
	name := fmt.Sprintf("extension-table-%d", index)
	table := presentationTable{Responsive: value.Responsive, Sortable: value.Sortable, Search: value.Search, Pagination: value.Pagination, PageSize: value.PageSize, Columns: map[string]presentationTableColumn{}}
	columns := make([]extensionPresentationTableColumn, 0, len(value.Columns))
	seen := map[string]struct{}{}
	for _, column := range value.Columns {
		if strings.TrimSpace(column.Key) == "" {
			return extensionPresentationBlock{}, nil, nil, errors.New("table column key cannot be empty")
		}
		if _, exists := seen[column.Key]; exists {
			return extensionPresentationBlock{}, nil, nil, fmt.Errorf("duplicate table column key %q", column.Key)
		}
		seen[column.Key] = struct{}{}
		label := column.Label
		if label == "" {
			label = column.Key
		}
		columns = append(columns, extensionPresentationTableColumn{Key: column.Key, Label: label, Type: column.Type, Priority: column.Priority})
		table.Columns[column.Key] = presentationTableColumn{Type: column.Type, Priority: column.Priority}
	}
	rows := make([]extensionPresentationTableRow, 0, len(value.Rows))
	for _, source := range value.Rows {
		for key, rawValue := range source {
			if _, ok := seen[key]; !ok {
				return extensionPresentationBlock{}, nil, nil, fmt.Errorf("table row contains unknown key %q", key)
			}
			if !extensionPresentationScalar(rawValue) {
				return extensionPresentationBlock{}, nil, nil, fmt.Errorf("table cell %q must be scalar", key)
			}
		}
		row := extensionPresentationTableRow{}
		for _, column := range columns {
			row.Cells = append(row.Cells, extensionPresentationTableCell{Key: column.Key, Value: extensionPresentationScalarString(source[column.Key])})
		}
		rows = append(rows, row)
	}
	return extensionPresentationBlock{Type: "table", TableName: name, Columns: columns, Rows: rows}, &table, nil, nil
}

func extensionPresentationScalar(value any) bool {
	switch value.(type) {
	case nil, string, bool, float64:
		return true
	default:
		return false
	}
}

func extensionPresentationScalarString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func decodeExtensionPresentationChart(index int, raw json.RawMessage) (extensionPresentationBlock, *presentationTable, *presentationChart, error) {
	var value extensionPresentationChartBlock
	if err := decodeStrictJSON(raw, &value); err != nil {
		return extensionPresentationBlock{}, nil, nil, err
	}
	chart := presentationChart{Type: value.ChartType, Height: value.Height, Legend: value.Legend, Min: value.Min, Max: value.Max, Unit: value.Unit, Stacked: value.Stacked}
	var data any
	switch value.ChartType {
	case "line", "area", "bar":
		data = &extensionPresentationStandardChartData{}
	case "pie", "doughnut":
		data = &extensionPresentationCircularChartData{}
	case "sparkline":
		data = &extensionPresentationSparklineData{}
	case "gauge":
		data = &extensionPresentationGaugeData{}
	default:
		return extensionPresentationBlock{}, nil, nil, fmt.Errorf("unsupported chart type %q", value.ChartType)
	}
	if len(value.Data) == 0 {
		return extensionPresentationBlock{}, nil, nil, errors.New("chart requires data")
	}
	if err := decodeStrictJSON(value.Data, data); err != nil {
		return extensionPresentationBlock{}, nil, nil, fmt.Errorf("chart data: %w", err)
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return extensionPresentationBlock{}, nil, nil, err
	}
	name := fmt.Sprintf("extension-chart-%d", index)
	return extensionPresentationBlock{Type: "chart", ChartName: name, ChartData: template.JS(encoded)}, nil, &chart, nil
}
