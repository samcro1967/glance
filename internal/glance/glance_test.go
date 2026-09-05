package glance

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newGlanceTestApplication(t *testing.T, yaml string) *application {
	t.Helper()

	c, err := newConfigFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("newConfigFromYAML() error = %v", err)
	}

	app, err := newApplication(c)
	if err != nil {
		t.Fatalf("newApplication() error = %v", err)
	}

	return app
}

func TestShortBuildRevision(t *testing.T) {
	tests := []struct {
		name     string
		revision string
		want     string
	}{
		{name: "empty", revision: "", want: ""},
		{name: "short", revision: "abc123", want: "abc123"},
		{name: "seven characters", revision: "8534072", want: "8534072"},
		{name: "full SHA", revision: "8534072e8ac3f0aac221a3c4f41897efdd2653bc", want: "8534072"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shortBuildRevision(test.revision); got != test.want {
				t.Fatalf("shortBuildRevision(%q) = %q, want %q", test.revision, got, test.want)
			}
		})
	}
}

func TestThemeConfiguredFieldsDistinguishExplicitFalseFromOmission(t *testing.T) {
	configured := newGlanceTestApplication(t, `
theme:
  light: false

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	if !configured.Config.Theme.configuredFields["light"] {
		t.Fatal("light: false should be recorded as explicitly configured")
	}

	omitted := newGlanceTestApplication(t, `
theme: {}

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	if omitted.Config.Theme.configuredFields["light"] {
		t.Fatal("omitted light should not be recorded as explicitly configured")
	}
}

func TestThemeConfiguredFieldsDistinguishExplicitZeroFromOmission(t *testing.T) {
	configured := newGlanceTestApplication(t, `
theme:
  contrast-multiplier: 0

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	if !configured.Config.Theme.configuredFields["contrast-multiplier"] {
		t.Fatal("contrast-multiplier: 0 should be recorded as explicitly configured")
	}

	omitted := newGlanceTestApplication(t, `
theme: {}

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	if omitted.Config.Theme.configuredFields["contrast-multiplier"] {
		t.Fatal("omitted contrast-multiplier should not be recorded as explicitly configured")
	}
}

func TestConfiguredGlobalThemeRejectsInvalidProperty(t *testing.T) {
	_, err := newConfigFromYAML([]byte(`
theme:
  widgets:
    radius: huge

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`))
	if err == nil {
		t.Fatal("invalid global theme property should be rejected")
	}
	want := `theme.widgets.radius must be one of: none, small, medium, large; got "huge"`
	if err.Error() != want {
		t.Fatalf("validation error = %q, want %q", err.Error(), want)
	}
}

func TestConfiguredPresetThemeRejectsInvalidProperty(t *testing.T) {
	_, err := newConfigFromYAML([]byte(`
theme:
  presets:
    oled:
      navigation:
        font-weight: heavy

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`))
	if err == nil {
		t.Fatal("invalid preset theme property should be rejected")
	}
	want := `theme.presets.oled.navigation.font-weight must be one of: normal, medium, semibold, bold; got "heavy"`
	if err.Error() != want {
		t.Fatalf("validation error = %q, want %q", err.Error(), want)
	}
}

func TestConfiguredPageThemeRejectsInvalidProperty(t *testing.T) {
	_, err := newConfigFromYAML([]byte(`
pages:
  - name: Media
    theme:
      page:
        background-image: https://example.com/background.jpg
    columns:
      - size: full
        widgets: []
`))
	if err == nil {
		t.Fatal("invalid page theme property should be rejected")
	}
	want := `pages[0].theme.page.background-image must be a safe local path under /assets/; got "https://example.com/background.jpg"`
	if err.Error() != want {
		t.Fatalf("validation error = %q, want %q", err.Error(), want)
	}
}

func TestConfiguredThemesAcceptValidProperties(t *testing.T) {
	app := newGlanceTestApplication(t, `
theme:
  typography:
    font-family: system
  widgets:
    radius: medium
  presets:
    oled:
      cards:
        shadow: subtle

pages:
  - name: Media
    theme:
      page:
        background-image: /assets/backgrounds/media.webp
        background-size: cover
        overlay:
          opacity: 0.5
      footer:
        font-weight: semibold
    columns:
      - size: full
        widgets: []
`)

	if app == nil {
		t.Fatal("valid configured themes should load")
	}
}

func TestPageThemeTracksExplicitProperties(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Media
    theme:
      light: false
      accent-color: 269 100 77
      widgets:
        shadow: strong
    columns:
      - size: full
        widgets: []
`)

	pageTheme := &app.Config.Pages[0].Theme

	if !pageTheme.configuredFields["light"] {
		t.Fatal("page theme light: false should be recorded as explicitly configured")
	}
	if !pageTheme.configuredFields["accent-color"] {
		t.Fatal("page theme accent-color should be recorded as explicitly configured")
	}
	if !pageTheme.configuredFields["widgets"] {
		t.Fatal("page theme widgets should be recorded as explicitly configured")
	}
	if !pageTheme.Widgets.configuredFields["shadow"] {
		t.Fatal("page theme widgets.shadow should be recorded as explicitly configured")
	}
	if pageTheme.Widgets.configuredFields["radius"] {
		t.Fatal("omitted page theme widgets.radius should not be recorded as explicitly configured")
	}
}

func TestPresetThemeExistsImmediatelyAfterConfigDecode(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
theme:
  presets:
    oled:
      light: false
      accent-color: 210 90 60
      widgets:
        radius: medium
        shadow: subtle

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML() error = %v", err)
	}

	preset, exists := config.Theme.Presets.Get("oled")
	if !exists {
		t.Fatal("configured oled preset is missing immediately after YAML decoding")
	}
	if !preset.configuredFields["light"] {
		t.Fatal("preset light: false should be recorded immediately after YAML decoding")
	}
	if !preset.configuredFields["accent-color"] {
		t.Fatal("preset accent-color should be recorded immediately after YAML decoding")
	}
	if !preset.Widgets.configuredFields["radius"] {
		t.Fatal("preset widgets.radius should be recorded immediately after YAML decoding")
	}
	if !preset.Widgets.configuredFields["shadow"] {
		t.Fatal("preset widgets.shadow should be recorded immediately after YAML decoding")
	}
}

func TestPresetThemeTracksExplicitProperties(t *testing.T) {
	app := newGlanceTestApplication(t, `
theme:
  presets:
    oled:
      light: false
      accent-color: 210 90 60
      widgets:
        radius: medium
        shadow: subtle

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	preset, exists := app.Config.Theme.Presets.Get("oled")
	if !exists {
		t.Fatal("configured oled preset is missing")
	}

	if !preset.configuredFields["light"] {
		t.Fatal("preset light: false should be recorded as explicitly configured")
	}
	if !preset.configuredFields["accent-color"] {
		t.Fatal("preset accent-color should be recorded as explicitly configured")
	}
	if !preset.Widgets.configuredFields["radius"] {
		t.Fatal("preset widgets.radius should be recorded as explicitly configured")
	}
	if !preset.Widgets.configuredFields["shadow"] {
		t.Fatal("preset widgets.shadow should be recorded as explicitly configured")
	}
	if preset.Widgets.configuredFields["blur"] {
		t.Fatal("omitted preset widgets.blur should not be recorded as explicitly configured")
	}
}

func TestEmptyPageThemeDoesNotConfigureProperties(t *testing.T) {
	app := newGlanceTestApplication(t, `
theme:
  light: true
  contrast-multiplier: 1.3

pages:
  - name: Home
    theme: {}
    columns:
      - size: full
        widgets: []
`)

	pageTheme := &app.Config.Pages[0].Theme

	if pageTheme.configuredFields["light"] {
		t.Fatal("empty page theme should not configure light")
	}
	if pageTheme.configuredFields["contrast-multiplier"] {
		t.Fatal("empty page theme should not configure contrast-multiplier")
	}
	if pageTheme.configuredFields["widgets"] {
		t.Fatal("empty page theme should not configure widgets")
	}
}

func TestValidateThemeSurfaceTreatments(t *testing.T) {
	theme := &themeProperties{}
	theme.Widgets.Radius = "large"
	theme.Widgets.Shadow = "strong"
	theme.Widgets.Blur = "medium"
	theme.Widgets.configuredFields = yamlConfiguredFields{
		"radius": true,
		"shadow": true,
		"blur":   true,
	}

	if err := theme.validateSurfaceTreatments("theme"); err != nil {
		t.Fatalf("valid surface treatments rejected: %v", err)
	}
}

func TestValidateThemeSurfaceTreatmentsRejectInvalidValue(t *testing.T) {
	theme := &themeProperties{}
	theme.Cards.Radius = "huge"
	theme.Cards.configuredFields = yamlConfiguredFields{"radius": true}

	err := theme.validateSurfaceTreatments("theme")
	if err == nil {
		t.Fatal("invalid card radius should be rejected")
	}

	want := `theme.cards.radius must be one of: none, small, medium, large; got "huge"`
	if err.Error() != want {
		t.Fatalf("validation error = %q, want %q", err.Error(), want)
	}
}

func TestValidateThemeSurfaceTreatmentsIgnoreOmittedValues(t *testing.T) {
	theme := &themeProperties{}
	theme.Header.Shadow = "invalid"

	if err := theme.validateSurfaceTreatments("theme"); err != nil {
		t.Fatalf("omitted surface property should not be validated: %v", err)
	}
}

func TestValidateThemePage(t *testing.T) {
	for _, opacity := range []float32{0, 1} {
		theme := &themeProperties{}
		theme.Page.BackgroundPosition = "top-right"
		theme.Page.BackgroundSize = "cover"
		theme.Page.BackgroundRepeat = "no-repeat"
		theme.Page.BackgroundAttachment = "fixed"
		theme.Page.AmbientAccent = "medium"
		theme.Page.configuredFields = yamlConfiguredFields{
			"background-position":   true,
			"background-size":       true,
			"background-repeat":     true,
			"background-attachment": true,
			"ambient-accent":        true,
		}
		theme.Page.Overlay.Opacity = &opacity
		theme.Page.Overlay.configuredFields = yamlConfiguredFields{"opacity": true}

		if err := theme.validatePage("theme"); err != nil {
			t.Fatalf("valid page properties rejected for opacity %v: %v", opacity, err)
		}
	}
}

func TestValidateThemePageRejectsInvalidEnum(t *testing.T) {
	theme := &themeProperties{}
	theme.Page.BackgroundSize = "stretch"
	theme.Page.configuredFields = yamlConfiguredFields{"background-size": true}

	err := theme.validatePage("theme")
	if err == nil {
		t.Fatal("invalid background-size should be rejected")
	}

	want := `theme.page.background-size must be one of: auto, cover, contain; got "stretch"`
	if err.Error() != want {
		t.Fatalf("validation error = %q, want %q", err.Error(), want)
	}
}

func TestValidateThemePageRejectsInvalidOpacity(t *testing.T) {
	for _, opacity := range []float32{-0.01, 1.01} {
		theme := &themeProperties{}
		theme.Page.Overlay.Opacity = &opacity
		theme.Page.Overlay.configuredFields = yamlConfiguredFields{"opacity": true}

		if err := theme.validatePage("theme"); err == nil {
			t.Fatalf("invalid overlay opacity %v should be rejected", opacity)
		}
	}
}

func TestValidateThemePageIgnoresOmittedValues(t *testing.T) {
	opacity := float32(2)
	theme := &themeProperties{}
	theme.Page.BackgroundSize = "invalid"
	theme.Page.Overlay.Opacity = &opacity

	if err := theme.validatePage("theme"); err != nil {
		t.Fatalf("omitted page properties should not be validated: %v", err)
	}
}

func TestValidateThemeBackgroundImage(t *testing.T) {
	valid := []string{
		"/assets/background.jpg",
		"/assets/themes/oled/background.webp",
		"/assets/my-background_01.png",
	}

	for _, value := range valid {
		if err := validateThemeBackgroundImage("theme.page.background-image", value); err != nil {
			t.Fatalf("valid background image %q rejected: %v", value, err)
		}
	}
}

func TestValidateThemeBackgroundImageRejectsUnsafeValues(t *testing.T) {
	invalid := []string{
		"https://example.com/background.jpg",
		"http://example.com/background.jpg",
		"//example.com/background.jpg",
		"data:image/png;base64,abc",
		"file:///tmp/background.jpg",
		"/background.jpg",
		"/assets/",
		"/assets/../secret.jpg",
		"/assets/themes/../../secret.jpg",
		"/assets/url(background.jpg)",
		"/assets/background.jpg;display-none",
		"/assets/background.jpg?query=value",
		"/assets/background.jpg#fragment",
		"/assets/background image.jpg",
	}

	for _, value := range invalid {
		if err := validateThemeBackgroundImage("theme.page.background-image", value); err == nil {
			t.Fatalf("unsafe background image %q should be rejected", value)
		}
	}
}

func TestValidateThemePageBackgroundImageHonorsPresence(t *testing.T) {
	theme := &themeProperties{}
	theme.Page.BackgroundImage = "https://example.com/background.jpg"

	if err := theme.validatePage("theme"); err != nil {
		t.Fatalf("omitted background-image should not be validated: %v", err)
	}

	theme.Page.configuredFields = yamlConfiguredFields{"background-image": true}
	if err := theme.validatePage("theme"); err == nil {
		t.Fatal("configured remote background-image should be rejected")
	}
}

func TestValidateThemeComponentTypography(t *testing.T) {
	theme := &themeProperties{}
	theme.Navigation.FontSize = "large"
	theme.Navigation.FontWeight = "semibold"
	theme.WidgetHeader.FontSize = "medium"
	theme.WidgetHeader.FontWeight = "bold"
	theme.Footer.FontSize = "small"
	theme.Footer.FontWeight = "normal"
	theme.Navigation.configuredFields = yamlConfiguredFields{"font-size": true, "font-weight": true}
	theme.WidgetHeader.configuredFields = yamlConfiguredFields{"font-size": true, "font-weight": true}
	theme.Footer.configuredFields = yamlConfiguredFields{"font-size": true, "font-weight": true}

	if err := theme.validateComponentTypography("theme"); err != nil {
		t.Fatalf("valid component typography rejected: %v", err)
	}
}

func TestValidateThemeComponentTypographyRejectsInvalidValue(t *testing.T) {
	theme := &themeProperties{}
	theme.WidgetHeader.FontWeight = "heavy"
	theme.WidgetHeader.configuredFields = yamlConfiguredFields{"font-weight": true}

	err := theme.validateComponentTypography("theme")
	if err == nil {
		t.Fatal("invalid widget-header font-weight should be rejected")
	}

	want := `theme.widget-header.font-weight must be one of: normal, medium, semibold, bold; got "heavy"`
	if err.Error() != want {
		t.Fatalf("validation error = %q, want %q", err.Error(), want)
	}
}

func TestValidateThemeComponentTypographyIgnoresOmittedValues(t *testing.T) {
	theme := &themeProperties{}
	theme.Footer.FontSize = "huge"

	if err := theme.validateComponentTypography("theme"); err != nil {
		t.Fatalf("omitted component typography should not be validated: %v", err)
	}
}

func TestValidateThemeProperties(t *testing.T) {
	theme := &themeProperties{}
	theme.Navigation.FontWeight = "heavy"
	theme.Navigation.configuredFields = yamlConfiguredFields{"font-weight": true}

	err := theme.validate("pages[2].theme")
	if err == nil {
		t.Fatal("invalid theme property should be rejected")
	}

	want := `pages[2].theme.navigation.font-weight must be one of: normal, medium, semibold, bold; got "heavy"`
	if err.Error() != want {
		t.Fatalf("validation error = %q, want %q", err.Error(), want)
	}
}

func TestValidateThemePropertiesAcceptsValidTheme(t *testing.T) {
	opacity := float32(0.5)
	theme := &themeProperties{}
	theme.Typography.FontFamily = "system"
	theme.Typography.configuredFields = yamlConfiguredFields{"font-family": true}
	theme.Widgets.Radius = "medium"
	theme.Widgets.configuredFields = yamlConfiguredFields{"radius": true}
	theme.Page.BackgroundImage = "/assets/backgrounds/home.webp"
	theme.Page.BackgroundSize = "cover"
	theme.Page.configuredFields = yamlConfiguredFields{"background-image": true, "background-size": true}
	theme.Page.Overlay.Opacity = &opacity
	theme.Page.Overlay.configuredFields = yamlConfiguredFields{"opacity": true}
	theme.Footer.FontWeight = "semibold"
	theme.Footer.configuredFields = yamlConfiguredFields{"font-weight": true}

	if err := theme.validate("theme"); err != nil {
		t.Fatalf("valid aggregate theme rejected: %v", err)
	}
}

func TestMergeThemePropertiesDeepMergesTypography(t *testing.T) {
	baseText := &hslColorField{0, 0, 90}
	baseHeadingText := &hslColorField{0, 0, 100}
	overrideHeadingText := &hslColorField{210, 80, 65}

	base := themeProperties{
		Typography: themeTypographyProperties{
			FontFamily:         "system",
			FontSize:           "medium",
			FontWeight:         "normal",
			TextColor:          baseText,
			SecondaryTextColor: &hslColorField{0, 0, 70},
			MutedTextColor:     &hslColorField{0, 0, 50},
			Headings: themeHeadingProperties{
				FontFamily: "sans-serif",
				FontSize:   "large",
				FontWeight: "semibold",
				TextColor:  baseHeadingText,
			},
		},
	}

	override := themeProperties{}
	override.Typography.FontWeight = "bold"
	override.Typography.configuredFields = yamlConfiguredFields{"font-weight": true, "headings": true}
	override.Typography.Headings.TextColor = overrideHeadingText
	override.Typography.Headings.configuredFields = yamlConfiguredFields{"text-color": true}

	merged := mergeThemeProperties(base, override)

	if merged.Typography.FontFamily != "system" || merged.Typography.FontSize != "medium" {
		t.Fatal("omitted typography properties should inherit from base")
	}
	if merged.Typography.FontWeight != "bold" {
		t.Fatal("explicit typography font-weight should override base")
	}
	if merged.Typography.TextColor != baseText {
		t.Fatal("omitted typography text-color should inherit from base")
	}
	if merged.Typography.Headings.FontFamily != "sans-serif" ||
		merged.Typography.Headings.FontSize != "large" ||
		merged.Typography.Headings.FontWeight != "semibold" {
		t.Fatal("omitted heading properties should inherit from base")
	}
	if merged.Typography.Headings.TextColor != overrideHeadingText {
		t.Fatal("explicit heading text-color should override base")
	}
}

func TestMergeThemePropertiesDoesNotMutateNestedTypography(t *testing.T) {
	base := themeProperties{
		Typography: themeTypographyProperties{
			FontWeight: "normal",
			Headings: themeHeadingProperties{
				FontWeight: "semibold",
			},
		},
	}
	override := themeProperties{}
	override.Typography.FontWeight = "bold"
	override.Typography.configuredFields = yamlConfiguredFields{"font-weight": true}

	merged := mergeThemeProperties(base, override)
	merged.Typography.FontWeight = "medium"
	merged.Typography.Headings.FontWeight = "bold"

	if base.Typography.FontWeight != "normal" || base.Typography.Headings.FontWeight != "semibold" {
		t.Fatal("base nested typography was mutated")
	}
	if override.Typography.FontWeight != "bold" {
		t.Fatal("override nested typography was mutated")
	}
}

func TestMergeThemePropertiesDeepMergesPage(t *testing.T) {
	baseOpacity := float32(0.7)
	overrideOpacity := float32(0)
	baseOverlayColor := &hslColorField{0, 0, 0}

	base := themeProperties{
		Page: themePageProperties{
			BackgroundImage:      "/assets/backgrounds/base.webp",
			BackgroundPosition:   "center",
			BackgroundSize:       "cover",
			BackgroundRepeat:     "no-repeat",
			BackgroundAttachment: "scroll",
			AmbientAccent:        "subtle",
			Overlay: themePageOverlayProperties{
				Color:   baseOverlayColor,
				Opacity: &baseOpacity,
			},
		},
	}

	override := themeProperties{}
	override.Page.BackgroundPosition = "top-right"
	override.Page.AmbientAccent = "strong"
	override.Page.configuredFields = yamlConfiguredFields{
		"background-position": true,
		"ambient-accent":      true,
		"overlay":             true,
	}
	override.Page.Overlay.Opacity = &overrideOpacity
	override.Page.Overlay.configuredFields = yamlConfiguredFields{"opacity": true}

	merged := mergeThemeProperties(base, override)

	if merged.Page.BackgroundImage != "/assets/backgrounds/base.webp" ||
		merged.Page.BackgroundSize != "cover" ||
		merged.Page.BackgroundRepeat != "no-repeat" ||
		merged.Page.BackgroundAttachment != "scroll" {
		t.Fatal("omitted page properties should inherit from base")
	}
	if merged.Page.BackgroundPosition != "top-right" || merged.Page.AmbientAccent != "strong" {
		t.Fatal("explicit page properties should override base")
	}
	if merged.Page.Overlay.Color != baseOverlayColor {
		t.Fatal("omitted overlay color should inherit from base")
	}
	if merged.Page.Overlay.Opacity != &overrideOpacity || *merged.Page.Overlay.Opacity != 0 {
		t.Fatal("explicit overlay opacity zero should override base")
	}
}

func TestMergeThemePropertiesDoesNotMutateNestedPage(t *testing.T) {
	baseOpacity := float32(0.5)
	overrideOpacity := float32(0)
	base := themeProperties{
		Page: themePageProperties{
			BackgroundSize: "cover",
			Overlay: themePageOverlayProperties{
				Opacity: &baseOpacity,
			},
		},
	}
	override := themeProperties{}
	override.Page.BackgroundSize = "contain"
	override.Page.configuredFields = yamlConfiguredFields{"background-size": true, "overlay": true}
	override.Page.Overlay.Opacity = &overrideOpacity
	override.Page.Overlay.configuredFields = yamlConfiguredFields{"opacity": true}

	merged := mergeThemeProperties(base, override)
	merged.Page.BackgroundSize = "auto"

	if base.Page.BackgroundSize != "cover" || *base.Page.Overlay.Opacity != 0.5 {
		t.Fatal("base page theme was mutated")
	}
	if override.Page.BackgroundSize != "contain" || *override.Page.Overlay.Opacity != 0 {
		t.Fatal("override page theme was mutated")
	}
}

func TestMergeThemePropertiesMergesRemainingComponents(t *testing.T) {
	baseColor := &hslColorField{0, 0, 70}
	overrideColor := &hslColorField{210, 80, 65}

	base := themeProperties{
		Navigation:   themeNavigationProperties{TextColor: baseColor, FontSize: "medium"},
		WidgetHeader: themeWidgetHeaderProperties{TextColor: baseColor, FontWeight: "semibold"},
		Groups:       themeGroupProperties{TextColor: baseColor},
		Controls: themeControlProperties{
			TextColor: baseColor,
			Radius:    "medium",
			Button:    themeButtonProperties{TextColor: baseColor},
		},
		Footer:   themeFooterProperties{TextColor: baseColor, FontSize: "small"},
		Surfaces: themeElevatedSurfaceProperties{SeparatorColor: baseColor},
	}

	override := themeProperties{}
	override.Navigation.AccentColor = overrideColor
	override.Navigation.configuredFields = yamlConfiguredFields{"accent-color": true}
	override.WidgetHeader.AccentColor = overrideColor
	override.WidgetHeader.configuredFields = yamlConfiguredFields{"accent-color": true}
	override.Groups.ActiveColor = overrideColor
	override.Groups.configuredFields = yamlConfiguredFields{"active-color": true}
	override.Controls.FocusColor = overrideColor
	override.Controls.configuredFields = yamlConfiguredFields{"focus-color": true, "button": true}
	override.Controls.Button.BackgroundColor = overrideColor
	override.Controls.Button.configuredFields = yamlConfiguredFields{"background-color": true}
	override.Footer.AccentColor = overrideColor
	override.Footer.configuredFields = yamlConfiguredFields{"accent-color": true}
	override.Surfaces.ElevatedBorderColor = overrideColor
	override.Surfaces.configuredFields = yamlConfiguredFields{"elevated-border-color": true}

	merged := mergeThemeProperties(base, override)

	if merged.Navigation.TextColor != baseColor || merged.Navigation.FontSize != "medium" || merged.Navigation.AccentColor != overrideColor {
		t.Fatal("navigation merge failed")
	}
	if merged.WidgetHeader.TextColor != baseColor || merged.WidgetHeader.FontWeight != "semibold" || merged.WidgetHeader.AccentColor != overrideColor {
		t.Fatal("widget-header merge failed")
	}
	if merged.Groups.TextColor != baseColor || merged.Groups.ActiveColor != overrideColor {
		t.Fatal("groups merge failed")
	}
	if merged.Controls.TextColor != baseColor || merged.Controls.Radius != "medium" || merged.Controls.FocusColor != overrideColor {
		t.Fatal("controls merge failed")
	}
	if merged.Controls.Button.TextColor != baseColor || merged.Controls.Button.BackgroundColor != overrideColor {
		t.Fatal("controls.button deep merge failed")
	}
	if merged.Footer.TextColor != baseColor || merged.Footer.FontSize != "small" || merged.Footer.AccentColor != overrideColor {
		t.Fatal("footer merge failed")
	}
	if merged.Surfaces.SeparatorColor != baseColor || merged.Surfaces.ElevatedBorderColor != overrideColor {
		t.Fatal("surfaces merge failed")
	}
}

func TestPopulateTemplateRequestDataAppliesPageTheme(t *testing.T) {
	app := newGlanceTestApplication(t, `
theme:
  accent-color: 210 80 65
pages:
  - name: Media
    theme:
      accent-color: 269 100 77
    columns:
      - size: full
        widgets:
          - type: bookmarks
            groups: []
`)

	page := &app.Config.Pages[0]
	request := httptest.NewRequest(http.MethodGet, "/media", nil)
	data := templateRequestData{}

	app.populateTemplateRequestData(&data, request, page)

	if data.Theme == nil {
		t.Fatal("request theme should be populated")
	}
	if data.Theme.AccentColor == nil || !data.Theme.AccentColor.SameAs(&hslColorField{269, 100, 77}) {
		t.Fatal("page accent should override base accent")
	}
	if data.Theme.Key != "default" {
		t.Fatalf("theme key = %q, want default", data.Theme.Key)
	}
}

func TestPopulateTemplateRequestDataAppliesPageThemeToSelectedPreset(t *testing.T) {
	app := newGlanceTestApplication(t, `
theme:
  presets:
    oled:
      background-color: 0 0 0
      accent-color: 210 90 60
pages:
  - name: Media
    theme:
      accent-color: 269 100 77
    columns:
      - size: full
        widgets:
          - type: bookmarks
            groups: []
`)

	page := &app.Config.Pages[0]
	request := httptest.NewRequest(http.MethodGet, "/media", nil)
	request.AddCookie(&http.Cookie{Name: "theme", Value: "oled"})
	data := templateRequestData{}

	app.populateTemplateRequestData(&data, request, page)

	if data.Theme == nil {
		t.Fatal("request theme should be populated")
	}
	if data.Theme.Key != "oled" {
		t.Fatalf("theme key = %q, want oled", data.Theme.Key)
	}
	if data.Theme.BackgroundColor == nil || !data.Theme.BackgroundColor.SameAs(&hslColorField{0, 0, 0}) {
		t.Fatal("selected preset background should be preserved")
	}
	if data.Theme.AccentColor == nil || !data.Theme.AccentColor.SameAs(&hslColorField{269, 100, 77}) {
		t.Fatal("page accent should override selected preset accent")
	}
}

func TestPopulateTemplateRequestDataWithoutPageUsesSelectedBase(t *testing.T) {
	app := newGlanceTestApplication(t, `
theme:
  presets:
    oled:
      background-color: 0 0 0
      accent-color: 210 90 60
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: bookmarks
            groups: []
`)

	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	request.AddCookie(&http.Cookie{Name: "theme", Value: "oled"})
	data := templateRequestData{}

	app.populateTemplateRequestData(&data, request, nil)

	if data.Theme == nil {
		t.Fatal("request theme should be populated")
	}
	if data.Theme.Key != "oled" {
		t.Fatalf("theme key = %q, want oled", data.Theme.Key)
	}
	if data.Theme.AccentColor == nil || !data.Theme.AccentColor.SameAs(&hslColorField{210, 90, 60}) {
		t.Fatal("base preset accent should remain unchanged without page override")
	}
}

func TestThemeCSSValueMappings(t *testing.T) {
	tests := []struct {
		name     string
		mapper   func(string) string
		input    string
		expected string
	}{
		{"font family default", themeFontFamilyCSS, "default", "inherit"},
		{"font family system", themeFontFamilyCSS, "system", "system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"},
		{"font family sans serif", themeFontFamilyCSS, "sans-serif", "sans-serif"},
		{"font family serif", themeFontFamilyCSS, "serif", "serif"},
		{"font family monospace", themeFontFamilyCSS, "monospace", "monospace"},
		{"font family empty", themeFontFamilyCSS, "", ""},
		{"font family invalid", themeFontFamilyCSS, "invalid", ""},

		{"font size small", themeFontSizeCSS, "small", "1.2rem"},
		{"font size medium", themeFontSizeCSS, "medium", "1.3rem"},
		{"font size large", themeFontSizeCSS, "large", "1.5rem"},
		{"font size empty", themeFontSizeCSS, "", ""},
		{"font size invalid", themeFontSizeCSS, "invalid", ""},

		{"font weight normal", themeFontWeightCSS, "normal", "400"},
		{"font weight medium", themeFontWeightCSS, "medium", "500"},
		{"font weight semibold", themeFontWeightCSS, "semibold", "600"},
		{"font weight bold", themeFontWeightCSS, "bold", "700"},
		{"font weight empty", themeFontWeightCSS, "", ""},
		{"font weight invalid", themeFontWeightCSS, "invalid", ""},

		{"radius none", themeRadiusCSS, "none", "0"},
		{"radius small", themeRadiusCSS, "small", "0.35rem"},
		{"radius medium", themeRadiusCSS, "medium", "5px"},
		{"radius large", themeRadiusCSS, "large", "0.85rem"},
		{"radius empty", themeRadiusCSS, "", ""},
		{"radius invalid", themeRadiusCSS, "invalid", ""},

		{"shadow none", themeShadowCSS, "none", "none"},
		{"shadow subtle", themeShadowCSS, "subtle", "0 6px 18px rgba(0, 0, 0, 0.18)"},
		{"shadow medium", themeShadowCSS, "medium", "inset 0 1px 0 rgba(255, 255, 255, 0.04), 0 5px 18px rgba(0, 0, 0, 0.24)"},
		{"shadow strong", themeShadowCSS, "strong", "inset 0 1px 0 rgba(255, 255, 255, 0.055), inset 0 -1px 0 rgba(0, 0, 0, 0.14), 0 8px 24px rgba(0, 0, 0, 0.30), 0 2px 6px rgba(0, 0, 0, 0.24)"},
		{"shadow empty", themeShadowCSS, "", ""},
		{"shadow invalid", themeShadowCSS, "invalid", ""},

		{"blur none", themeBlurCSS, "none", "none"},
		{"blur subtle", themeBlurCSS, "subtle", "blur(4px)"},
		{"blur medium", themeBlurCSS, "medium", "blur(8px) saturate(105%)"},
		{"blur strong", themeBlurCSS, "strong", "blur(14px) saturate(110%)"},
		{"blur empty", themeBlurCSS, "", ""},
		{"blur invalid", themeBlurCSS, "invalid", ""},

		{"ambient none", themeAmbientAccentCSS, "none", "0"},
		{"ambient subtle", themeAmbientAccentCSS, "subtle", "0.08"},
		{"ambient medium", themeAmbientAccentCSS, "medium", "0.16"},
		{"ambient strong", themeAmbientAccentCSS, "strong", "0.24"},
		{"ambient empty", themeAmbientAccentCSS, "", ""},
		{"ambient invalid", themeAmbientAccentCSS, "invalid", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := test.mapper(test.input); actual != test.expected {
				t.Errorf("mapping %q = %q, want %q", test.input, actual, test.expected)
			}
		})
	}
}

func TestExpandedThemeEmitsSemanticCSSVariables(t *testing.T) {
	opacity := float32(0.4)
	theme := themeProperties{
		AccentColor:  &hslColorField{210, 80, 65},
		WarningColor: &hslColorField{40, 90, 60},
		Typography: themeTypographyProperties{
			FontFamily:         "system",
			TextColor:          &hslColorField{0, 0, 90},
			SecondaryTextColor: &hslColorField{0, 0, 70},
			MutedTextColor:     &hslColorField{0, 0, 50},
			Headings: themeHeadingProperties{
				FontWeight: "bold",
			},
		},
		Page: themePageProperties{
			BackgroundImage: "/assets/backgrounds/test.jpg",
			AmbientAccent:   "medium",
			Overlay: themePageOverlayProperties{
				Opacity: &opacity,
			},
		},
		Header: themeHeaderProperties{
			Shadow: "subtle",
		},
		Navigation: themeNavigationProperties{
			ActiveColor: &hslColorField{210, 80, 65},
		},
		Widgets: themeSurfaceProperties{
			Radius: "medium",
		},
		WidgetHeader: themeWidgetHeaderProperties{
			FontWeight: "semibold",
		},
		Cards: themeCardProperties{
			Shadow: "medium",
		},
		Groups: themeGroupProperties{
			AccentColor: &hslColorField{210, 80, 65},
		},
		Controls: themeControlProperties{
			Radius: "small",
			Button: themeButtonProperties{
				BackgroundColor: &hslColorField{210, 80, 65},
			},
		},
		Footer: themeFooterProperties{
			FontSize: "small",
		},
		Surfaces: themeElevatedSurfaceProperties{
			SeparatorColor: &hslColorField{0, 0, 30},
		},
	}

	if err := theme.init(); err != nil {
		t.Fatalf("theme.init() error = %v", err)
	}

	css := string(theme.CSS)
	for _, expected := range []string{
		"--theme-accent-color: hsl(210.0, 80.0%, 65.0%);",
		"--theme-warning-color: hsl(40.0, 90.0%, 60.0%);",
		"--theme-font-family: system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif;",
		"--theme-text-color: hsl(0.0, 0.0%, 90.0%);",
		"--theme-heading-font-weight: 700;",
		"--theme-page-background-image: url(\"/assets/backgrounds/test.jpg\");",
		"--theme-page-ambient-accent-opacity: 0.16;",
		"--theme-page-overlay-opacity: 0.4;",
		"--theme-header-shadow: 0 6px 18px rgba(0, 0, 0, 0.18);",
		"--theme-navigation-active-color: hsl(210.0, 80.0%, 65.0%);",
		"--theme-widget-radius: 5px;",
		"--theme-widget-header-font-weight: 600;",
		"--theme-card-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04), 0 5px 18px rgba(0, 0, 0, 0.24);",
		"--theme-group-accent-color: hsl(210.0, 80.0%, 65.0%);",
		"--theme-control-radius: 0.35rem;",
		"--theme-control-button-background-color: hsl(210.0, 80.0%, 65.0%);",
		"--theme-footer-font-size: 1.2rem;",
		"--theme-separator-color: hsl(0.0, 0.0%, 30.0%);",
	} {
		if !strings.Contains(css, expected) {
			t.Errorf("theme CSS missing %q; CSS=%q", expected, css)
		}
	}
}

func TestResolveThemeMergesPageOverrideAndInitializesCopy(t *testing.T) {
	baseAccent := &hslColorField{210, 80, 65}
	pageAccent := &hslColorField{269, 100, 77}

	base := themeProperties{
		Key:                      "oled",
		Light:                    true,
		AccentColor:              baseAccent,
		ContrastMultiplier:       1.25,
		TextSaturationMultiplier: 0.75,
		Widgets: themeSurfaceProperties{
			Radius: "medium",
			Shadow: "subtle",
		},
	}
	base.CSS = template.CSS("base-css")
	base.PreviewHTML = template.HTML("base-preview")
	base.BackgroundColorAsHex = "#base"

	pageOverride := themeProperties{
		Light:       false,
		AccentColor: pageAccent,
	}
	pageOverride.configuredFields = yamlConfiguredFields{
		"light":        true,
		"accent-color": true,
	}
	pageOverride.Widgets.Shadow = "strong"
	pageOverride.Widgets.configuredFields = yamlConfiguredFields{"shadow": true}

	resolved, err := resolveTheme(&base, &pageOverride)
	if err != nil {
		t.Fatalf("resolveTheme() error = %v", err)
	}

	if resolved == &base || resolved == &pageOverride {
		t.Fatal("resolved theme should be a distinct value")
	}
	if resolved.Key != "oled" {
		t.Fatalf("resolved key = %q, want oled", resolved.Key)
	}
	if resolved.Light {
		t.Fatal("page light: false should override base")
	}
	if resolved.AccentColor != pageAccent {
		t.Fatal("page accent should override base")
	}
	if resolved.ContrastMultiplier != 1.25 || resolved.TextSaturationMultiplier != 0.75 {
		t.Fatal("omitted core values should inherit from base")
	}
	if resolved.Widgets.Radius != "medium" || resolved.Widgets.Shadow != "strong" {
		t.Fatal("nested page override should deep merge with base")
	}
	if resolved.CSS == "" || resolved.CSS == template.CSS("base-css") {
		t.Fatal("resolved theme CSS should be freshly initialized")
	}
	if resolved.PreviewHTML == "" || resolved.PreviewHTML == template.HTML("base-preview") {
		t.Fatal("resolved theme preview should be freshly initialized")
	}

	if !base.Light || base.AccentColor != baseAccent || base.Widgets.Shadow != "subtle" {
		t.Fatal("base theme was mutated")
	}
	if base.CSS != template.CSS("base-css") || base.PreviewHTML != template.HTML("base-preview") || base.BackgroundColorAsHex != "#base" {
		t.Fatal("base derived fields were mutated")
	}
	if pageOverride.Light || pageOverride.AccentColor != pageAccent || pageOverride.Widgets.Shadow != "strong" {
		t.Fatal("page override was mutated")
	}
}

func TestResolveThemeWithoutPageOverrideInitializesBaseCopy(t *testing.T) {
	base := themeProperties{
		Key:                      "default",
		Light:                    false,
		ContrastMultiplier:       1.1,
		TextSaturationMultiplier: 0.9,
	}

	resolved, err := resolveTheme(&base, nil)
	if err != nil {
		t.Fatalf("resolveTheme() error = %v", err)
	}

	if resolved == &base {
		t.Fatal("resolved theme should not alias base")
	}
	if resolved.Key != "default" || resolved.Light {
		t.Fatal("resolved base properties were not preserved")
	}
	if resolved.CSS == "" || resolved.PreviewHTML == "" {
		t.Fatal("resolved base copy should be initialized")
	}
}

func TestResolveThemeRejectsNilBase(t *testing.T) {
	_, err := resolveTheme(nil, &themeProperties{})
	if err == nil {
		t.Fatal("nil base theme should be rejected")
	}
	if err.Error() != "base theme is nil" {
		t.Fatalf("error = %q, want %q", err.Error(), "base theme is nil")
	}
}

func TestMergeThemePropertiesCoreInheritance(t *testing.T) {
	background := &hslColorField{240, 8, 9}
	primary := &hslColorField{43, 50, 70}
	accent := &hslColorField{210, 80, 65}
	base := themeProperties{
		BackgroundColor:          background,
		PrimaryColor:             primary,
		Light:                    true,
		ContrastMultiplier:       1.25,
		TextSaturationMultiplier: 0.75,
	}

	override := themeProperties{
		AccentColor: accent,
	}
	override.configuredFields = yamlConfiguredFields{"accent-color": true}

	merged := mergeThemeProperties(base, override)

	if merged.BackgroundColor != background || merged.PrimaryColor != primary {
		t.Fatal("omitted core properties should inherit from base")
	}
	if merged.AccentColor != accent {
		t.Fatal("explicit accent-color should override base")
	}
	if !merged.Light || merged.ContrastMultiplier != 1.25 || merged.TextSaturationMultiplier != 0.75 {
		t.Fatal("omitted scalar properties should inherit from base")
	}
}

func TestMergeThemePropertiesPreservesExplicitFalseAndZero(t *testing.T) {
	base := themeProperties{
		Light:                    true,
		ContrastMultiplier:       1.5,
		TextSaturationMultiplier: 0.8,
	}

	override := themeProperties{
		Light:                    false,
		ContrastMultiplier:       0,
		TextSaturationMultiplier: 0,
	}
	override.configuredFields = yamlConfiguredFields{
		"light":                      true,
		"contrast-multiplier":        true,
		"text-saturation-multiplier": true,
	}

	merged := mergeThemeProperties(base, override)

	if merged.Light {
		t.Fatal("explicit light: false should override true base value")
	}
	if merged.ContrastMultiplier != 0 {
		t.Fatalf("explicit contrast-multiplier zero = %v, want 0", merged.ContrastMultiplier)
	}
	if merged.TextSaturationMultiplier != 0 {
		t.Fatalf("explicit text-saturation-multiplier zero = %v, want 0", merged.TextSaturationMultiplier)
	}
}

func TestMergeThemePropertiesDoesNotMutateInputs(t *testing.T) {
	base := themeProperties{Light: true, ContrastMultiplier: 1.5}
	override := themeProperties{Light: false, ContrastMultiplier: 0}
	override.configuredFields = yamlConfiguredFields{
		"light":               true,
		"contrast-multiplier": true,
	}

	merged := mergeThemeProperties(base, override)
	merged.Light = true
	merged.ContrastMultiplier = 9

	if !base.Light || base.ContrastMultiplier != 1.5 {
		t.Fatal("base theme was mutated")
	}
	if override.Light || override.ContrastMultiplier != 0 {
		t.Fatal("override theme was mutated")
	}
}

func TestValidateThemeEnum(t *testing.T) {
	if err := validateThemeEnum("theme.widgets.radius", "medium", "none", "small", "medium", "large"); err != nil {
		t.Fatalf("valid enum value rejected: %v", err)
	}

	err := validateThemeEnum("theme.widgets.radius", "huge", "none", "small", "medium", "large")
	if err == nil {
		t.Fatal("invalid enum value should be rejected")
	}

	want := `theme.widgets.radius must be one of: none, small, medium, large; got "huge"`
	if err.Error() != want {
		t.Fatalf("validation error = %q, want %q", err.Error(), want)
	}
}

func TestGlanceLightPresetCompatibility(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	preset, exists := app.Config.Theme.Presets.Get("glance-light")
	if !exists {
		t.Fatal("glance-light preset should be injected when picker is enabled")
	}

	if !preset.Light {
		t.Fatal("glance-light should remain light")
	}
	if !preset.BackgroundColor.SameAs(&hslColorField{240, 13, 95}) {
		t.Fatalf("glance-light background = %#v", preset.BackgroundColor)
	}
	if !preset.PrimaryColor.SameAs(&hslColorField{230, 100, 30}) {
		t.Fatalf("glance-light primary = %#v", preset.PrimaryColor)
	}
	if !preset.NegativeColor.SameAs(&hslColorField{0, 70, 50}) {
		t.Fatalf("glance-light negative = %#v", preset.NegativeColor)
	}
	if preset.ContrastMultiplier != 1.3 {
		t.Fatalf("glance-light contrast multiplier = %v, want 1.3", preset.ContrastMultiplier)
	}
	if preset.TextSaturationMultiplier != 0.5 {
		t.Fatalf("glance-light text saturation multiplier = %v, want 0.5", preset.TextSaturationMultiplier)
	}
	if preset.Key != "glance-light" || preset.CSS == "" || preset.PreviewHTML == "" {
		t.Fatal("glance-light should remain independently initialized")
	}
}

func TestCustomPresetRemainsIndependentBaseTheme(t *testing.T) {
	app := newGlanceTestApplication(t, `
theme:
  primary-color: 10 20 30
  accent-color: 20 30 40
  presets:
    oled:
      background-color: 0 0 0
      primary-color: 100 50 60
      accent-color: 210 90 60

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	preset, exists := app.Config.Theme.Presets.Get("oled")
	if !exists {
		t.Fatal("oled preset is missing")
	}

	if !preset.BackgroundColor.SameAs(&hslColorField{0, 0, 0}) ||
		!preset.PrimaryColor.SameAs(&hslColorField{100, 50, 60}) ||
		!preset.AccentColor.SameAs(&hslColorField{210, 90, 60}) {
		t.Fatal("custom preset values were not preserved as an independent base theme")
	}

	if preset.PrimaryColor.SameAs(app.Config.Theme.PrimaryColor) ||
		preset.AccentColor.SameAs(app.Config.Theme.AccentColor) {
		t.Fatal("custom preset should not inherit top-level visual properties")
	}

	if preset.Key != "oled" || preset.CSS == "" || preset.PreviewHTML == "" {
		t.Fatal("custom preset should be independently initialized")
	}
}

func TestDisablePickerDoesNotInjectBuiltInPresets(t *testing.T) {
	app := newGlanceTestApplication(t, `
theme:
  disable-picker: true

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	if _, exists := app.Config.Theme.Presets.Get("glance-light"); exists {
		t.Fatal("glance-light should not be injected when picker is disabled")
	}
	if _, exists := app.Config.Theme.Presets.Get("glance-dark"); exists {
		t.Fatal("glance-dark should not be injected when picker is disabled")
	}
	if app.Config.Theme.Key != "default" || app.Config.Theme.CSS == "" || app.Config.Theme.PreviewHTML == "" {
		t.Fatal("top-level default theme should still be initialized when picker is disabled")
	}
}

func TestThemeSameAsIncludesExpandedVisualProperties(t *testing.T) {
	base := &themeProperties{
		AccentColor: &hslColorField{210, 80, 65},
		Typography: themeTypographyProperties{
			FontFamily: "system",
		},
		Widgets: themeSurfaceProperties{
			Radius: "medium",
		},
	}
	other := *base

	if !base.SameAs(&other) {
		t.Fatal("identical expanded themes should compare equal")
	}

	other.Widgets.Radius = "large"
	if base.SameAs(&other) {
		t.Fatal("different nested visual properties should compare unequal")
	}

	other = *base
	other.Key = "different-key"
	other.CSS = template.CSS("different-css")
	other.PreviewHTML = template.HTML("different-preview")
	other.BackgroundColorAsHex = "#ffffff"
	other.configuredFields = yamlConfiguredFields{"accent-color": true}

	if !base.SameAs(&other) {
		t.Fatal("derived and presence metadata should not affect visual equality")
	}
}

func TestGlanceDarkPresetIsInjectedWhenPickerEnabled(t *testing.T) {
	app := newGlanceTestApplication(t, `
theme:
  primary-color: 200 100 50

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	preset, exists := app.Config.Theme.Presets.Get("glance-dark")
	if !exists {
		t.Fatal("glance-dark should be injected when picker is enabled")
	}
	if preset.Key != "glance-dark" || preset.Name != "Glance Dark" || preset.CSS == "" || preset.PreviewHTML == "" {
		t.Fatal("glance-dark should be independently initialized")
	}
}

func TestConfiguredGlanceDarkNameDoesNotReplaceBuiltIn(t *testing.T) {
	app := newGlanceTestApplication(t, `
theme:
  presets:
    glance-dark:
      primary-color: 200 100 50

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	preset, exists := app.Config.Theme.Presets.Get("glance-dark")
	if !exists {
		t.Fatal("configured glance-dark preset is missing")
	}

	if preset.PrimaryColor == nil {
		t.Fatal("configured glance-dark preset primary color is nil")
	}

	expected := &hslColorField{200, 100, 50}
	if !preset.PrimaryColor.SameAs(expected) {
		t.Fatalf(
			"glance-dark primary color = %#v, want %#v",
			preset.PrimaryColor,
			expected,
		)
	}
}

func TestDesktopNavigationWidthDefaultsToPageWidthWhenUnset(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    width: wide
    columns:
      - size: full
        widgets: []
`)

	got := app.Config.Pages[0].DesktopNavigationWidth
	if got != "wide" {
		t.Fatalf("DesktopNavigationWidth = %q, want %q", got, "wide")
	}
}

func TestDesktopNavigationWidthDefaultUsesPageWidth(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    width: wide
    desktop-navigation-width: default
    columns:
      - size: full
        widgets: []
`)

	got := app.Config.Pages[0].DesktopNavigationWidth
	if got != "wide" {
		t.Fatalf("DesktopNavigationWidth = %q, want %q", got, "wide")
	}
}

func TestExplicitDesktopNavigationWidthIsPreserved(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    width: wide
    desktop-navigation-width: slim
    columns:
      - size: full
        widgets: []
`)

	got := app.Config.Pages[0].DesktopNavigationWidth
	if got != "slim" {
		t.Fatalf("DesktopNavigationWidth = %q, want %q", got, "slim")
	}
}

func TestApplicationCollectsRefreshWidgetLeaves(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    head-widgets:
      - type: rss
        feeds:
          - url: https://example.com/feed.xml

    columns:
      - size: full
        widgets:
          - type: group
            widgets:
              - type: monitor
                sites:
                  - title: Example
                    url: https://example.com

              - type: group
                widgets:
                  - type: markets
                    markets:
                      - symbol: SPY
                        name: Example Market

          - type: stack
            widgets:
              - type: releases
                repositories:
                  - glanceapp/glance

          - type: split-column
            widgets:
              - type: reddit
                subreddit: golang

              - type: videos
                channels:
                  - UC_x5XG1OV2P6uZZ5FSM9Ttw

    bottom-widgets:
      - type: hacker-news
`)

	gotTypes := make([]string, 0, len(app.refreshWidgets))
	for _, widget := range app.refreshWidgets {
		gotTypes = append(gotTypes, widget.GetType())
	}

	wantTypes := []string{
		"rss",
		"monitor",
		"markets",
		"releases",
		"reddit",
		"videos",
		"hacker-news",
	}

	if len(gotTypes) != len(wantTypes) {
		t.Fatalf("refreshWidgets types = %v, want %v", gotTypes, wantTypes)
	}

	for i := range wantTypes {
		if gotTypes[i] != wantTypes[i] {
			t.Fatalf(
				"refreshWidgets[%d] type = %q, want %q; all types = %v",
				i,
				gotTypes[i],
				wantTypes[i],
				gotTypes,
			)
		}
	}

	for _, widget := range app.refreshWidgets {
		switch widget.GetType() {
		case "group", "stack", "split-column":
			t.Fatalf(
				"refreshWidgets contains container widget type %q",
				widget.GetType(),
			)
		}
	}
}

func TestCollectRefreshWidgetsDeduplicatesLeaves(t *testing.T) {
	first := &htmlWidget{
		widgetBase: widgetBase{
			ID:   1001,
			Type: "html",
		},
	}
	second := &htmlWidget{
		widgetBase: widgetBase{
			ID:   1002,
			Type: "html",
		},
	}

	container := &groupWidget{
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{
				first,
				second,
			},
		},
	}

	collected := collectRefreshWidgets(widgets{
		first,
		container,
		second,
	})

	if len(collected) != 2 {
		t.Fatalf("collectRefreshWidgets() returned %d widgets, want 2", len(collected))
	}

	if collected[0] != first {
		t.Fatal("first collected widget is not the first encountered leaf")
	}

	if collected[1] != second {
		t.Fatal("second collected widget is not the second encountered unique leaf")
	}
}

func TestCollectRefreshWidgetsRecursesThroughNestedContainers(t *testing.T) {
	leaf := &htmlWidget{
		widgetBase: widgetBase{
			ID:   2001,
			Type: "html",
		},
	}

	nested := &groupWidget{
		containerWidgetBase: containerWidgetBase{
			Widgets: widgets{
				&groupWidget{
					containerWidgetBase: containerWidgetBase{
						Widgets: widgets{
							leaf,
						},
					},
				},
			},
		},
	}

	collected := collectRefreshWidgets(widgets{nested})

	if len(collected) != 1 {
		t.Fatalf("collectRefreshWidgets() returned %d widgets, want 1", len(collected))
	}

	if collected[0] != leaf {
		t.Fatal("collectRefreshWidgets() did not return the nested leaf")
	}
}

func TestApplicationPrimaryColumnSelection(t *testing.T) {
	tests := []struct {
		name      string
		columns   string
		wantIndex int8
	}{
		{
			name: "single full",
			columns: `
      - size: full
        widgets: []
`,
			wantIndex: 0,
		},
		{
			name: "small then full",
			columns: `
      - size: small
        widgets: []
      - size: full
        widgets: []
`,
			wantIndex: 1,
		},
		{
			name: "two full uses first full",
			columns: `
      - size: full
        widgets: []
      - size: full
        widgets: []
`,
			wantIndex: 0,
		},
		{
			name: "medium then full uses full",
			columns: `
      - size: medium
        widgets: []
      - size: full
        widgets: []
`,
			wantIndex: 1,
		},
		{
			name: "full then medium uses full",
			columns: `
      - size: full
        widgets: []
      - size: medium
        widgets: []
`,
			wantIndex: 0,
		},
		{
			name: "three medium uses first medium",
			columns: `
      - size: medium
        widgets: []
      - size: medium
        widgets: []
      - size: medium
        widgets: []
`,
			wantIndex: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
`+tt.columns)

			got := app.Config.Pages[0].PrimaryColumnIndex
			if got != tt.wantIndex {
				t.Fatalf("PrimaryColumnIndex = %d, want %d", got, tt.wantIndex)
			}
		})
	}
}

func TestApplicationRegistersBottomWidgetsByID(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets: []

    bottom-widgets:
      - type: hacker-news
`)

	if len(app.Config.Pages[0].BottomWidgets) != 1 {
		t.Fatalf(
			"BottomWidgets length = %d, want 1",
			len(app.Config.Pages[0].BottomWidgets),
		)
	}

	bottomWidget := app.Config.Pages[0].BottomWidgets[0]
	widgetID := bottomWidget.GetID()

	registered, exists := app.widgetByID[widgetID]
	if !exists {
		t.Fatalf("widgetByID does not contain bottom widget ID %d", widgetID)
	}

	if registered != bottomWidget {
		t.Fatalf(
			"widgetByID[%d] = %T, want exact bottom widget %T",
			widgetID,
			registered,
			bottomWidget,
		)
	}
}

func TestVersionedAssetPath(t *testing.T) {
	// Use the zero time value so the expected version query string is
	// deterministic and does not depend on when the test is executed.
	app := &application{}

	tests := []struct {
		name    string
		baseURL string
		asset   string
		want    string
	}{
		{
			name:    "root deployment",
			baseURL: "",
			asset:   "manifest.json",
			want:    "/manifest.json?v=-62135596800",
		},
		{
			name:    "deployment with base URL",
			baseURL: "/glance",
			asset:   "manifest.json",
			want:    "/glance/manifest.json?v=-62135596800",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// BaseURL is changed for each case on the same lightweight
			// application because VersionedAssetPath has no side effects.
			app.Config.Server.BaseURL = test.baseURL

			got := app.VersionedAssetPath(test.asset)
			if got != test.want {
				t.Fatalf(
					"VersionedAssetPath(%q) = %q, want %q",
					test.asset,
					got,
					test.want,
				)
			}
		})
	}
}
