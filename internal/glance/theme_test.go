package glance

import (
	"strings"
	"testing"
)

func TestMergeThemeSurfaceWidgetTypography(t *testing.T) {
	base := themeSurfaceProperties{
		FontSize:   "small",
		FontWeight: "normal",
	}

	override := themeSurfaceProperties{
		FontSize:   "large",
		FontWeight: "bold",
	}
	override.configuredFields = yamlConfiguredFields{
		"font-size":   true,
		"font-weight": true,
	}

	merged := mergeThemeSurface(base, override)

	if merged.FontSize != "large" {
		t.Fatalf("FontSize = %q, want %q", merged.FontSize, "large")
	}
	if merged.FontWeight != "bold" {
		t.Fatalf("FontWeight = %q, want %q", merged.FontWeight, "bold")
	}
}

func TestMergeThemeSurfaceWidgetTypographyPreservesUnconfiguredBase(t *testing.T) {
	base := themeSurfaceProperties{
		FontSize:   "large",
		FontWeight: "semibold",
	}

	override := themeSurfaceProperties{
		FontSize:   "small",
		FontWeight: "normal",
	}

	merged := mergeThemeSurface(base, override)

	if merged.FontSize != "large" {
		t.Fatalf("FontSize = %q, want preserved base value %q", merged.FontSize, "large")
	}
	if merged.FontWeight != "semibold" {
		t.Fatalf("FontWeight = %q, want preserved base value %q", merged.FontWeight, "semibold")
	}
}

func TestValidateComponentTypographyWidgets(t *testing.T) {
	tests := []struct {
		name       string
		fontSize   string
		fontWeight string
		wantErr    bool
	}{
		{name: "valid", fontSize: "large", fontWeight: "semibold"},
		{name: "invalid size", fontSize: "huge", fontWeight: "semibold", wantErr: true},
		{name: "invalid weight", fontSize: "large", fontWeight: "heavy", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			theme := themeProperties{}
			theme.Widgets.FontSize = tt.fontSize
			theme.Widgets.FontWeight = tt.fontWeight
			theme.Widgets.configuredFields = yamlConfiguredFields{
				"font-size":   true,
				"font-weight": true,
			}

			err := theme.validateComponentTypography("theme")
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestWidgetTypographyCSS(t *testing.T) {
	theme := themeProperties{}
	theme.Widgets.FontSize = "large"
	theme.Widgets.FontWeight = "semibold"

	if got := theme.WidgetFontSizeCSS(); got != "1.5rem" {
		t.Fatalf("WidgetFontSizeCSS() = %q, want %q", got, "1.5rem")
	}
	if got := theme.WidgetFontWeightCSS(); got != "600" {
		t.Fatalf("WidgetFontWeightCSS() = %q, want %q", got, "600")
	}
}

func TestThemeCSSIncludesWidgetTypographyOverrides(t *testing.T) {
	theme := themeProperties{}
	theme.Widgets.FontSize = "large"
	theme.Widgets.FontWeight = "semibold"

	if err := theme.init(); err != nil {
		t.Fatalf("init theme: %v", err)
	}

	css := string(theme.CSS)
	if !strings.Contains(css, "--theme-widget-font-size: 1.5rem;") {
		t.Fatalf("theme CSS missing widget font-size variable: %s", css)
	}
	if !strings.Contains(css, "--theme-widget-font-weight: 600;") {
		t.Fatalf("theme CSS missing widget font-weight variable: %s", css)
	}
}

func TestThemeCSSOmitsUnconfiguredWidgetTypography(t *testing.T) {
	theme := themeProperties{}

	if err := theme.init(); err != nil {
		t.Fatalf("init theme: %v", err)
	}

	css := string(theme.CSS)
	if strings.Contains(css, "--theme-widget-font-size:") {
		t.Fatalf("theme CSS unexpectedly contains widget font-size override: %s", css)
	}
	if strings.Contains(css, "--theme-widget-font-weight:") {
		t.Fatalf("theme CSS unexpectedly contains widget font-weight override: %s", css)
	}
}

func TestSameThemeSurfaceIncludesWidgetTypography(t *testing.T) {
	base := themeSurfaceProperties{
		FontSize:   "large",
		FontWeight: "semibold",
	}

	same := base
	if !sameThemeSurface(base, same) {
		t.Fatal("identical widget typography should compare equal")
	}

	differentSize := base
	differentSize.FontSize = "small"
	if sameThemeSurface(base, differentSize) {
		t.Fatal("different widget font-size should compare unequal")
	}

	differentWeight := base
	differentWeight.FontWeight = "bold"
	if sameThemeSurface(base, differentWeight) {
		t.Fatal("different widget font-weight should compare unequal")
	}
}

func TestMergeThemeDensity(t *testing.T) {
	base := themeProperties{Density: "compact"}
	override := themeProperties{Density: "spacious"}
	override.configuredFields = yamlConfiguredFields{"density": true}

	merged := mergeThemeProperties(base, override)
	if merged.Density != "spacious" {
		t.Fatalf("Density = %q, want %q", merged.Density, "spacious")
	}
}

func TestMergeThemeDensityPreservesUnconfiguredBase(t *testing.T) {
	base := themeProperties{Density: "comfortable"}
	override := themeProperties{Density: "compact"}

	merged := mergeThemeProperties(base, override)
	if merged.Density != "comfortable" {
		t.Fatalf("Density = %q, want preserved base value %q", merged.Density, "comfortable")
	}
}

func TestValidateThemeDensity(t *testing.T) {
	tests := []struct {
		name    string
		density string
		wantErr bool
	}{
		{name: "compact", density: "compact"},
		{name: "normal", density: "normal"},
		{name: "comfortable", density: "comfortable"},
		{name: "spacious", density: "spacious"},
		{name: "invalid", density: "dense", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			theme := themeProperties{Density: tt.density}
			theme.configuredFields = yamlConfiguredFields{"density": true}

			err := theme.validate("theme")
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestThemeCSSIncludesDensity(t *testing.T) {
	theme := themeProperties{Density: "compact"}

	if err := theme.init(); err != nil {
		t.Fatalf("init theme: %v", err)
	}

	if !strings.Contains(string(theme.CSS), "--theme-density: compact;") {
		t.Fatalf("theme CSS missing density variable: %s", theme.CSS)
	}
}

func TestThemeCSSOmitsUnconfiguredDensity(t *testing.T) {
	theme := themeProperties{}

	if err := theme.init(); err != nil {
		t.Fatalf("init theme: %v", err)
	}

	if strings.Contains(string(theme.CSS), "--theme-density:") {
		t.Fatalf("theme CSS unexpectedly contains density variable: %s", theme.CSS)
	}
}

func TestSameThemeIncludesDensity(t *testing.T) {
	base := themeProperties{Density: "normal"}
	same := base

	if !base.SameAs(&same) {
		t.Fatal("identical density should compare equal")
	}

	different := base
	different.Density = "compact"
	if base.SameAs(&different) {
		t.Fatal("different density should compare unequal")
	}
}
