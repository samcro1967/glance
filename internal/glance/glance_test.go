package glance

import "testing"

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

func TestDefaultDarkPresetIsNotInjectedWhenNotConfigured(t *testing.T) {
	app := newGlanceTestApplication(t, `
theme:
  primary-color: 200 100 50

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	if _, exists := app.Config.Theme.Presets.Get("default-dark"); exists {
		t.Fatal("default-dark preset should not be injected when it is not configured")
	}
}

func TestConfiguredDefaultDarkPresetIsPreserved(t *testing.T) {
	app := newGlanceTestApplication(t, `
theme:
  presets:
    default-dark:
      primary-color: 200 100 50

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)

	preset, exists := app.Config.Theme.Presets.Get("default-dark")
	if !exists {
		t.Fatal("configured default-dark preset is missing")
	}

	if preset.PrimaryColor == nil {
		t.Fatal("configured default-dark preset primary color is nil")
	}

	expected := &hslColorField{200, 100, 50}
	if !preset.PrimaryColor.SameAs(expected) {
		t.Fatalf(
			"default-dark primary color = %#v, want %#v",
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
