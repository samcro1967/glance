package glance

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
)

type presentationTableColumn struct {
	Type     string `yaml:"type,omitempty" json:"type,omitempty"`
	Priority int    `yaml:"priority,omitempty" json:"priority,omitempty"`
}

type presentationTable struct {
	Responsive *bool                              `yaml:"responsive,omitempty" json:"responsive"`
	Sortable   *bool                              `yaml:"sortable,omitempty" json:"sortable"`
	Search     *bool                              `yaml:"search,omitempty" json:"search"`
	Pagination *bool                              `yaml:"pagination,omitempty" json:"pagination"`
	PageSize   int                                `yaml:"page-size,omitempty" json:"pageSize,omitempty"`
	Columns    map[string]presentationTableColumn `yaml:"columns,omitempty" json:"columns,omitempty"`
}

type presentationChart struct {
	Type    string   `yaml:"type" json:"type"`
	Height  int      `yaml:"height,omitempty" json:"height,omitempty"`
	Legend  *bool    `yaml:"legend,omitempty" json:"legend"`
	Min     *float64 `yaml:"min,omitempty" json:"min,omitempty"`
	Max     *float64 `yaml:"max,omitempty" json:"max,omitempty"`
	Unit    string   `yaml:"unit,omitempty" json:"unit,omitempty"`
	Stacked bool     `yaml:"stacked,omitempty" json:"stacked,omitempty"`
}

type presentationConfig struct {
	Tables map[string]presentationTable `json:"tables,omitempty"`
	Charts map[string]presentationChart `json:"charts,omitempty"`
}

func presentationBoolPointer(value bool) *bool { return &value }

func normalizePresentationTables(tables map[string]presentationTable) error {
	for name, table := range tables {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("table configuration name cannot be empty")
		}
		if table.Responsive == nil {
			table.Responsive = presentationBoolPointer(true)
		}
		if table.Sortable == nil {
			table.Sortable = presentationBoolPointer(true)
		}
		if table.Search == nil {
			table.Search = presentationBoolPointer(false)
		}
		if table.Pagination == nil {
			table.Pagination = presentationBoolPointer(false)
		}
		if table.PageSize < 0 {
			return fmt.Errorf("table %q page-size cannot be negative", name)
		}
		if table.PageSize == 0 {
			table.PageSize = 10
		}
		for columnName, column := range table.Columns {
			if strings.TrimSpace(columnName) == "" {
				return fmt.Errorf("table %q column name cannot be empty", name)
			}
			if column.Priority < 0 {
				return fmt.Errorf("table %q column %q priority cannot be negative", name, columnName)
			}
			switch column.Type {
			case "", "text", "number", "date":
			default:
				return fmt.Errorf("table %q column %q has unsupported type %q", name, columnName, column.Type)
			}
		}
		tables[name] = table
	}
	return nil
}

func normalizePresentationCharts(charts map[string]presentationChart) error {
	for name, chart := range charts {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("chart configuration name cannot be empty")
		}
		switch chart.Type {
		case "line", "area", "bar", "pie", "doughnut", "sparkline", "gauge":
		default:
			return fmt.Errorf("chart %q has unsupported type %q", name, chart.Type)
		}
		if chart.Height < 0 {
			return fmt.Errorf("chart %q height cannot be negative", name)
		}
		if chart.Height == 0 {
			switch chart.Type {
			case "sparkline":
				chart.Height = 48
			case "gauge":
				chart.Height = 140
			default:
				chart.Height = 180
			}
		}
		if chart.Legend == nil {
			chart.Legend = presentationBoolPointer(chart.Type != "sparkline" && chart.Type != "gauge")
		}
		if chart.Type == "gauge" {
			min, max := 0.0, 100.0
			if chart.Min != nil {
				min = *chart.Min
			}
			if chart.Max != nil {
				max = *chart.Max
			}
			if min >= max {
				return fmt.Errorf("chart %q min must be less than max", name)
			}
		}
		if chart.Min != nil && chart.Max != nil && *chart.Min >= *chart.Max {
			return fmt.Errorf("chart %q min must be less than max", name)
		}
		charts[name] = chart
	}
	return nil
}

func presentationJSON(tables map[string]presentationTable, charts map[string]presentationChart) (template.JS, error) {
	if len(tables) == 0 && len(charts) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(presentationConfig{Tables: tables, Charts: charts})
	if err != nil {
		return "", fmt.Errorf("marshal presentation configuration: %w", err)
	}
	return template.JS(encoded), nil
}
