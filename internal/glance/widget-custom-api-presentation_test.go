package glance

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeCustomAPIPresentationTableDefaults(t *testing.T) {
	tables := map[string]customAPIPresentationTable{
		"services": {},
	}

	if err := normalizeCustomAPIPresentationTables(tables); err != nil {
		t.Fatalf("normalize tables: %v", err)
	}

	table := tables["services"]
	if table.Responsive == nil || !*table.Responsive {
		t.Error("responsive default = false, want true")
	}
	if table.Sortable == nil || !*table.Sortable {
		t.Error("sortable default = false, want true")
	}
	if table.Search == nil || *table.Search {
		t.Error("search default = true, want false")
	}
	if table.Pagination == nil || *table.Pagination {
		t.Error("pagination default = true, want false")
	}
	if table.PageSize != 10 {
		t.Errorf("page size = %d, want 10", table.PageSize)
	}
}

func TestNormalizeCustomAPIPresentationTableExplicitValues(t *testing.T) {
	tables := map[string]customAPIPresentationTable{
		"services": {
			Responsive: presentationBoolPointer(false),
			Sortable:   presentationBoolPointer(false),
			Search:     presentationBoolPointer(true),
			Pagination: presentationBoolPointer(true),
			PageSize:   25,
			Columns: map[string]customAPIPresentationTableColumn{
				"cpu":  {Type: "number", Priority: 2},
				"date": {Type: "date", Priority: 1},
				"name": {Type: "text"},
			},
		},
	}

	if err := normalizeCustomAPIPresentationTables(tables); err != nil {
		t.Fatalf("normalize tables: %v", err)
	}

	table := tables["services"]
	if *table.Responsive || *table.Sortable || !*table.Search || !*table.Pagination {
		t.Fatal("explicit boolean table options were not preserved")
	}
	if table.PageSize != 25 {
		t.Errorf("page size = %d, want 25", table.PageSize)
	}
}

func TestNormalizeCustomAPIPresentationTableErrors(t *testing.T) {
	tests := []struct {
		name   string
		tables map[string]customAPIPresentationTable
		want   string
	}{
		{
			name:   "empty name",
			tables: map[string]customAPIPresentationTable{"": {}},
			want:   "name cannot be empty",
		},
		{
			name:   "negative page size",
			tables: map[string]customAPIPresentationTable{"services": {PageSize: -1}},
			want:   "page-size",
		},
		{
			name: "empty column name",
			tables: map[string]customAPIPresentationTable{"services": {
				Columns: map[string]customAPIPresentationTableColumn{"": {}},
			}},
			want: "column name cannot be empty",
		},
		{
			name: "negative priority",
			tables: map[string]customAPIPresentationTable{"services": {
				Columns: map[string]customAPIPresentationTableColumn{"cpu": {Priority: -1}},
			}},
			want: "priority cannot be negative",
		},
		{
			name: "unsupported column type",
			tables: map[string]customAPIPresentationTable{"services": {
				Columns: map[string]customAPIPresentationTableColumn{"cpu": {Type: "currency"}},
			}},
			want: "unsupported type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := normalizeCustomAPIPresentationTables(tt.tables)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

func TestNormalizeCustomAPIPresentationChartTypesAndDefaults(t *testing.T) {
	types := []string{"line", "area", "bar", "pie", "doughnut", "sparkline", "gauge"}

	for _, chartType := range types {
		t.Run(chartType, func(t *testing.T) {
			charts := map[string]customAPIPresentationChart{
				"chart": {Type: chartType},
			}
			if err := normalizeCustomAPIPresentationCharts(charts); err != nil {
				t.Fatalf("normalize chart: %v", err)
			}

			chart := charts["chart"]
			wantHeight := 180
			wantLegend := true
			if chartType == "sparkline" {
				wantHeight = 48
				wantLegend = false
			}
			if chartType == "gauge" {
				wantHeight = 140
				wantLegend = false
			}
			if chart.Height != wantHeight {
				t.Errorf("height = %d, want %d", chart.Height, wantHeight)
			}
			if chart.Legend == nil || *chart.Legend != wantLegend {
				t.Errorf("legend default incorrect for %s", chartType)
			}
		})
	}
}

func TestNormalizeCustomAPIPresentationChartExplicitValues(t *testing.T) {
	minimum := 10.0
	maximum := 90.0
	charts := map[string]customAPIPresentationChart{
		"cpu": {
			Type:    "bar",
			Height:  240,
			Legend:  presentationBoolPointer(false),
			Min:     &minimum,
			Max:     &maximum,
			Unit:    "%",
			Stacked: true,
		},
	}

	if err := normalizeCustomAPIPresentationCharts(charts); err != nil {
		t.Fatalf("normalize charts: %v", err)
	}

	chart := charts["cpu"]
	if chart.Height != 240 || *chart.Legend || !chart.Stacked || chart.Unit != "%" {
		t.Fatal("explicit chart options were not preserved")
	}
}

func TestNormalizeCustomAPIPresentationChartErrors(t *testing.T) {
	minimum := 100.0
	maximum := 10.0
	tests := []struct {
		name   string
		charts map[string]customAPIPresentationChart
		want   string
	}{
		{
			name:   "empty name",
			charts: map[string]customAPIPresentationChart{"": {Type: "line"}},
			want:   "name cannot be empty",
		},
		{
			name:   "unsupported type",
			charts: map[string]customAPIPresentationChart{"chart": {Type: "scatter"}},
			want:   "unsupported type",
		},
		{
			name:   "negative height",
			charts: map[string]customAPIPresentationChart{"chart": {Type: "line", Height: -1}},
			want:   "height cannot be negative",
		},
		{
			name: "invalid range",
			charts: map[string]customAPIPresentationChart{"chart": {
				Type: "line",
				Min:  &minimum,
				Max:  &maximum,
			}},
			want: "min must be less than max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := normalizeCustomAPIPresentationCharts(tt.charts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

func TestCustomAPIPresentationJSON(t *testing.T) {
	tables := map[string]customAPIPresentationTable{"services": {}}
	charts := map[string]customAPIPresentationChart{"cpu": {Type: "gauge"}}

	if err := normalizeCustomAPIPresentationTables(tables); err != nil {
		t.Fatal(err)
	}
	if err := normalizeCustomAPIPresentationCharts(charts); err != nil {
		t.Fatal(err)
	}

	encoded, err := customAPIPresentationJSON(tables, charts)
	if err != nil {
		t.Fatal(err)
	}
	if encoded == "" {
		t.Fatal("presentation JSON is empty")
	}

	var config customAPIPresentationConfig
	if err := json.Unmarshal([]byte(encoded), &config); err != nil {
		t.Fatalf("decode presentation JSON: %v", err)
	}
	if len(config.Tables) != 1 || len(config.Charts) != 1 {
		t.Fatalf("unexpected presentation JSON: %s", encoded)
	}
}

func TestCustomAPIPresentationJSONEmpty(t *testing.T) {
	encoded, err := customAPIPresentationJSON(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if encoded != "" {
		t.Fatalf("encoded = %q, want empty", encoded)
	}
}

func TestCustomAPIWidgetPresentationInitialization(t *testing.T) {
	widget := &customAPIWidget{
		CustomAPIRequest: &CustomAPIRequest{URL: "https://example.com"},
		Template:         `<div class="glance-chart" data-glance-chart="cpu"></div>`,
		Tables: map[string]customAPIPresentationTable{
			"services": {
				Search: presentationBoolPointer(true),
				Columns: map[string]customAPIPresentationTableColumn{
					"latency": {Type: "number", Priority: 2},
				},
			},
		},
		Charts: map[string]customAPIPresentationChart{
			"cpu": {Type: "gauge", Unit: "%"},
		},
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize() error = %v", err)
	}

	if widget.compiledTemplate == nil {
		t.Fatal("compiled template is nil")
	}
	if widget.PresentationJSON == "" {
		t.Fatal("presentation JSON is empty")
	}

	table := widget.Tables["services"]
	if table.Responsive == nil || !*table.Responsive {
		t.Error("responsive default was not applied")
	}
	if table.Sortable == nil || !*table.Sortable {
		t.Error("sortable default was not applied")
	}
	if table.Search == nil || !*table.Search {
		t.Error("explicit search=true was not preserved")
	}
	if table.Pagination == nil || *table.Pagination {
		t.Error("pagination default was not applied")
	}
	if table.PageSize != 10 {
		t.Errorf("page size = %d, want 10", table.PageSize)
	}

	chart := widget.Charts["cpu"]
	if chart.Height != 140 {
		t.Errorf("gauge height = %d, want 140", chart.Height)
	}
	if chart.Legend == nil || *chart.Legend {
		t.Error("gauge legend default should be false")
	}

	var config customAPIPresentationConfig
	if err := json.Unmarshal([]byte(widget.PresentationJSON), &config); err != nil {
		t.Fatalf("decode presentation JSON: %v", err)
	}
	if config.Tables["services"].PageSize != 10 {
		t.Error("normalized table configuration was not serialized")
	}
	if config.Charts["cpu"].Height != 140 {
		t.Error("normalized chart configuration was not serialized")
	}
}

func TestCustomAPIWidgetPresentationInitializationRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*customAPIWidget)
		want      string
	}{
		{
			name: "invalid table",
			configure: func(widget *customAPIWidget) {
				widget.Tables = map[string]customAPIPresentationTable{
					"services": {PageSize: -1},
				}
			},
			want: "invalid table presentation configuration",
		},
		{
			name: "invalid chart",
			configure: func(widget *customAPIWidget) {
				widget.Charts = map[string]customAPIPresentationChart{
					"chart": {Type: "scatter"},
				}
			},
			want: "invalid chart presentation configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			widget := &customAPIWidget{
				CustomAPIRequest: &CustomAPIRequest{URL: "https://example.com"},
				Template:         `<div>test</div>`,
			}
			tt.configure(widget)

			err := widget.initialize()
			if err == nil {
				t.Fatal("initialize() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("initialize() error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

func TestCustomAPIWidgetPresentationStatusBarRestrictions(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*customAPIWidget)
		want      string
	}{
		{
			name: "tables",
			configure: func(widget *customAPIWidget) {
				widget.Tables = map[string]customAPIPresentationTable{"services": {}}
			},
			want: "tables are not supported inside a status-bar",
		},
		{
			name: "charts",
			configure: func(widget *customAPIWidget) {
				widget.Charts = map[string]customAPIPresentationChart{"cpu": {Type: "gauge"}}
			},
			want: "charts are not supported inside a status-bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			widget := &customAPIWidget{
				CustomAPIRequest:     &CustomAPIRequest{URL: "https://example.com"},
				statusBarCompactMode: true,
			}
			tt.configure(widget)

			err := widget.initialize()
			if err == nil {
				t.Fatal("initialize() error = nil, want error")
			}
			if err.Error() != tt.want {
				t.Fatalf("initialize() error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestCustomAPIPresentationJSONEscapesScriptBoundary(t *testing.T) {
	tables := map[string]customAPIPresentationTable{
		"</script><script>alert(1)</script>": {},
	}

	presentationJSON, err := customAPIPresentationJSON(tables, nil)
	if err != nil {
		t.Fatalf("customAPIPresentationJSON returned error: %v", err)
	}

	serialized := string(presentationJSON)
	if strings.Contains(serialized, "</script>") {
		t.Fatalf("presentation JSON contains literal script terminator: %s", serialized)
	}
	if !strings.Contains(serialized, "\\u003c/script\\u003e") {
		t.Fatalf("presentation JSON did not HTML-escape script terminator: %s", serialized)
	}
}

func TestCustomAPIWidgetWithoutPresentationConfiguration(t *testing.T) {
	widget := &customAPIWidget{
		CustomAPIRequest: &CustomAPIRequest{URL: "https://example.com/api"},
		Template:         "{{ .String \"name\" }}",
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize() returned error: %v", err)
	}

	if widget.PresentationJSON != "" {
		t.Fatalf("PresentationJSON = %q, want empty for ordinary Custom API widget", widget.PresentationJSON)
	}
	if widget.compiledTemplate == nil {
		t.Fatal("compiledTemplate = nil, want ordinary Custom API template compiled")
	}
}

func TestNormalizeCustomAPIPresentationGaugeEffectiveRange(t *testing.T) {
	invalidMinimum := 200.0
	invalidMaximum := -10.0

	tests := []struct {
		name  string
		chart customAPIPresentationChart
	}{
		{name: "minimum above default maximum", chart: customAPIPresentationChart{Type: "gauge", Min: &invalidMinimum}},
		{name: "maximum below default minimum", chart: customAPIPresentationChart{Type: "gauge", Max: &invalidMaximum}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			charts := map[string]customAPIPresentationChart{"gauge": tt.chart}
			err := normalizeCustomAPIPresentationCharts(charts)
			if err == nil {
				t.Fatal("expected invalid effective gauge range to return an error")
			}
			if !strings.Contains(err.Error(), "min must be less than max") {
				t.Fatalf("error = %q, want min/max range error", err)
			}
		})
	}
}
