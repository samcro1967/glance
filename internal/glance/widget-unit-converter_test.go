package glance

import (
	"encoding/json"
	"strings"
	"testing"
)

type unitConverterTestCatalog struct {
	SchemaVersion int                         `json:"schema_version"`
	Categories    []unitConverterTestCategory `json:"categories"`
}

type unitConverterTestCategory struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Canonical   string                  `json:"canonical"`
	DefaultFrom string                  `json:"default_from"`
	DefaultTo   string                  `json:"default_to"`
	Units       []unitConverterTestUnit `json:"units"`
}

type unitConverterTestUnit struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Symbol    string   `json:"symbol"`
	Transform string   `json:"transform"`
	Scale     *float64 `json:"scale"`
	Offset    *float64 `json:"offset"`
	Constant  *float64 `json:"constant"`
}

func loadUnitConverterTestCatalog(t *testing.T) unitConverterTestCatalog {
	t.Helper()

	var catalog unitConverterTestCatalog
	if err := json.Unmarshal(unitConverterCatalogJSON, &catalog); err != nil {
		t.Fatalf("decode embedded unit converter catalog: %v", err)
	}

	return catalog
}

func TestUnitConverterWidgetInitializeAndRender(t *testing.T) {
	widget := &unitConverterWidget{}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize unit converter: %v", err)
	}

	if widget.Title != "Unit Converter" {
		t.Fatalf("title=%q, want %q", widget.Title, "Unit Converter")
	}

	rendered := string(widget.Render())
	for _, expected := range []string{
		`class="unit-converter"`,
		`data-unit-converter-catalog`,
		`data-unit-converter-category`,
		`data-unit-converter-from`,
		`data-unit-converter-value`,
		`data-unit-converter-to`,
		`data-unit-converter-result`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered widget missing %q", expected)
		}
	}
}

func TestUnitConverterCatalogStructure(t *testing.T) {
	catalog := loadUnitConverterTestCatalog(t)

	if catalog.SchemaVersion != 1 {
		t.Fatalf("schema_version=%d, want 1", catalog.SchemaVersion)
	}

	if got := len(catalog.Categories); got != 35 {
		t.Fatalf("categories=%d, want 35", got)
	}

	categoryIDs := make(map[string]struct{}, len(catalog.Categories))
	totalUnits := 0

	for _, category := range catalog.Categories {
		if category.ID == "" {
			t.Fatal("category has empty id")
		}
		if category.Name == "" {
			t.Fatalf("category %q has empty name", category.ID)
		}
		if _, exists := categoryIDs[category.ID]; exists {
			t.Fatalf("duplicate category id %q", category.ID)
		}
		categoryIDs[category.ID] = struct{}{}

		if len(category.Units) == 0 {
			t.Fatalf("category %q has no units", category.ID)
		}

		unitIDs := make(map[string]struct{}, len(category.Units))

		for _, unit := range category.Units {
			totalUnits++

			if unit.ID == "" {
				t.Fatalf("category %q has unit with empty id", category.ID)
			}
			if unit.Name == "" {
				t.Fatalf(
					"category %q unit %q has empty name",
					category.ID,
					unit.ID,
				)
			}
			if _, exists := unitIDs[unit.ID]; exists {
				t.Fatalf(
					"category %q has duplicate unit id %q",
					category.ID,
					unit.ID,
				)
			}
			unitIDs[unit.ID] = struct{}{}

			switch unit.Transform {
			case "scale":
				if unit.Scale == nil {
					t.Fatalf(
						"%s/%s scale transform missing scale",
						category.ID,
						unit.ID,
					)
				}
				if *unit.Scale <= 0 {
					t.Fatalf(
						"%s/%s scale=%v, want > 0",
						category.ID,
						unit.ID,
						*unit.Scale,
					)
				}

			case "affine":
				if unit.Scale == nil || unit.Offset == nil {
					t.Fatalf(
						"%s/%s affine transform missing scale or offset",
						category.ID,
						unit.ID,
					)
				}
				if *unit.Scale <= 0 {
					t.Fatalf(
						"%s/%s affine scale=%v, want > 0",
						category.ID,
						unit.ID,
						*unit.Scale,
					)
				}

			case "reciprocal":
				if unit.Constant == nil {
					t.Fatalf(
						"%s/%s reciprocal transform missing constant",
						category.ID,
						unit.ID,
					)
				}
				if *unit.Constant <= 0 {
					t.Fatalf(
						"%s/%s reciprocal constant=%v, want > 0",
						category.ID,
						unit.ID,
						*unit.Constant,
					)
				}

			default:
				t.Fatalf(
					"%s/%s has unsupported transform %q",
					category.ID,
					unit.ID,
					unit.Transform,
				)
			}
		}

		for field, id := range map[string]string{
			"canonical":    category.Canonical,
			"default_from": category.DefaultFrom,
			"default_to":   category.DefaultTo,
		} {
			if id == "" {
				t.Fatalf(
					"category %q has empty %s",
					category.ID,
					field,
				)
			}
			if _, exists := unitIDs[id]; !exists {
				t.Fatalf(
					"category %q %s=%q does not reference a unit",
					category.ID,
					field,
					id,
				)
			}
		}
	}

	if totalUnits != 379 {
		t.Fatalf("units=%d, want 379", totalUnits)
	}
}

func TestUnitConverterCatalogTransformCounts(t *testing.T) {
	catalog := loadUnitConverterTestCatalog(t)

	counts := map[string]int{}

	for _, category := range catalog.Categories {
		for _, unit := range category.Units {
			counts[unit.Transform]++
		}
	}

	if counts["scale"] != 372 {
		t.Fatalf("scale transforms=%d, want 372", counts["scale"])
	}
	if counts["affine"] != 4 {
		t.Fatalf("affine transforms=%d, want 4", counts["affine"])
	}
	if counts["reciprocal"] != 3 {
		t.Fatalf("reciprocal transforms=%d, want 3", counts["reciprocal"])
	}
}
