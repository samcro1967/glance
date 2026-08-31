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
