package glance

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDecorationsDashboardDocument(t *testing.T) {
	app := newGlanceTestApplication(t, "decorations: {enabled: true, theme: halloween}\npages: [{name: Home, columns: [{size: full, widgets: []}]}]")
	for _, tc := range []struct {
		path      string
		decorated bool
	}{{"/", true}, {"/missing", false}, {"/docs", false}} {
		rec := httptest.NewRecorder()
		app.router().ServeHTTP(rec, httptest.NewRequest("GET", tc.path, nil))
		body := rec.Body.String()
		if strings.Contains(body, `id="decorations"`) != tc.decorated {
			t.Fatalf("wrong decoration scope on %s", tc.path)
		}
		if tc.decorated {
			start := strings.Index(body, `id="decoration-config">`)
			if start < 0 {
				t.Fatal("missing payload")
			}
			payload := body[start+len(`id="decoration-config">`):]
			payload = payload[:strings.Index(payload, "</script>")]
			var config decorationBrowserConfig
			if err := json.Unmarshal([]byte(payload), &config); err != nil {
				t.Fatalf("invalid template JSON %q: %v", payload, err)
			}
			if len(config.Design.Colors) != 2 {
				t.Fatal("preset colors missing")
			}
		}
	}
}

func parseDecorationsForTest(t *testing.T, input string) (*config, error) {
	t.Helper()
	return newConfigFromYAML([]byte(input + "\npages:\n  - name: Home\n    columns:\n      - size: full\n        widgets: []\n"))
}

func TestDecorationsConfiguration(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"omitted", "", ""},
		{"disabled", "decorations: {enabled: false}", ""},
		{"disabled switches", "decorations: {enabled: false, lights: true, animation: false}", ""},
		{"disabled custom fields", "decorations: {footer-left-icon: ''}", "only with theme: custom"},
		{"preset", "decorations: {enabled: true, theme: halloween}", ""},
		{"custom", "decorations: {enabled: true, theme: custom, light-colors: ['#ff7518'], footer-left-icon: 'mdi:ghost'}", ""},
		{"lights only", "decorations: {enabled: true, theme: custom, light-colors: ['#ff7518'], footer-icons: false}", ""},
		{"icons only", "decorations: {enabled: true, theme: custom, lights: false, footer-right-icon: 'twemoji:1f47b'}", ""},
		{"missing theme", "decorations: {enabled: true}", "unknown theme"},
		{"unknown theme", "decorations: {enabled: true, theme: missing}", "unknown theme"},
		{"unknown field", "decorations: {enabled: true, theme: halloween, animaton: true}", "unknown field"},
		{"preset override", "decorations: {enabled: true, theme: halloween, light-colors: []}", "only with theme: custom"},
		{"preset empty icon", "decorations: {theme: halloween, footer-left-icon: ''}", "only with theme: custom"},
		{"custom no colors", "decorations: {enabled: true, theme: custom, footer-left-icon: 'mdi:ghost'}", "light-colors"},
		{"custom no icon", "decorations: {enabled: true, theme: custom, light-colors: ['#ff7518']}", "at least one"},
		{"css input", "decorations: {theme: custom, light-colors: ['red;display:none'], footer-icons: false}", "#RRGGBB"},
		{"remote image", "decorations: {theme: custom, lights: false, footer-left-icon: 'https://example.com/a.svg'}", "supported icon-library"},
		{"path traversal", "decorations: {theme: custom, lights: false, footer-left-icon: '/assets/../secret.svg'}", "safe local path"},
		{"bad icon reference", "decorations: {theme: custom, lights: false, footer-left-icon: 'twemoji:../bad'}", "supported icon-library"},
		{"schedule without auto", "decorations: {theme: halloween, schedule: []}", "requires theme: auto"},
		{"empty auto", "decorations: {theme: auto}", "nonempty schedule"},
		{"auto custom fields", "decorations: {theme: auto, footer-left-icon: '', schedule: []}", "inside a custom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := parseDecorationsForTest(t, tc.input)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("got %v; want %q", err, tc.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.Decorations.Enabled && (!c.Decorations.Lights && !strings.Contains(tc.name, "icons only")) {
				t.Fatal("lights default lost")
			}
			if c.Decorations.Animation {
				t.Fatal("animation must default off")
			}
		})
	}
}

func TestDecorationsScheduleValidation(t *testing.T) {
	for _, tc := range []struct{ name, entries, want string }{
		{"single day", "[{theme: halloween, start: '10-31', end: '10-31'}]", ""},
		{"adjacent", "[{theme: halloween, start: '10-01', end: '10-31'}, {theme: christmas, start: '11-01', end: '12-30'}]", ""},
		{"year crossing", "[{theme: new-year, start: '12-31', end: '01-05'}]", ""},
		{"leap day", "[{theme: winter, start: '02-29', end: '02-29'}]", ""},
		{"overlap boundary", "[{theme: halloween, start: '10-01', end: '10-31'}, {theme: autumn, start: '10-31', end: '11-30'}]", "overlaps"},
		{"wrapped overlap", "[{theme: new-year, start: '12-31', end: '01-05'}, {theme: winter, start: '01-01', end: '01-02'}]", "overlaps"},
		{"invalid date", "[{theme: halloween, start: '02-30', end: '03-01'}]", "MM-DD"},
		{"date padding", "[{theme: halloween, start: '2-01', end: '03-01'}]", "MM-DD"},
		{"nested auto", "[{theme: auto, start: '01-01', end: '01-02'}]", "unknown theme"},
		{"custom entry", "[{theme: custom, light-colors: ['#123456'], footer-left-icon: 'twemoji:1f47b', start: '01-01', end: '01-02'}]", ""},
		{"entry override", "[{theme: halloween, footer-right-icon: '', start: '01-01', end: '01-02'}]", "only with theme: custom"},
		{"entry unknown field", "[{theme: halloween, animation: true, start: '01-01', end: '01-02'}]", "unknown field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseDecorationsForTest(t, "decorations:\n  enabled: true\n  theme: auto\n  schedule: "+tc.entries)
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("got %v; want %s", err, tc.want)
			}
		})
	}
}

func TestDecorationsCatalog(t *testing.T) {
	catalog, err := loadDecorationThemes(decorationThemesYAML)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := catalog["halloween"]; !ok {
		t.Fatal("Halloween preset missing")
	}
	manifest, err := os.ReadFile("../../testdata/visual/docs-images.json")
	if err != nil {
		t.Fatal(err)
	}
	var recipes map[string]struct {
		TestConfig string `json:"testConfig"`
	}
	if err := json.Unmarshal(manifest, &recipes); err != nil {
		t.Fatal(err)
	}
	for name := range catalog {
		profile := "testdata/decorations/" + name + ".yml"
		if _, err := os.Stat("../../" + profile); err != nil {
			t.Fatalf("preset %s has no source-runtime profile: %v", name, err)
		}
		if recipes["pages/decorations-"+name+".png"].TestConfig != profile {
			t.Fatalf("preset %s has no managed documentation capture", name)
		}
	}
	for _, input := range []string{"{}", "auto: {}", "custom: {}", "halloween: {unexpected: true}", "halloween: {light-colors: [red]}"} {
		if _, err := loadDecorationThemes([]byte(input)); err == nil {
			t.Fatalf("accepted catalog %s", input)
		}
	}
}

func TestDecorationsSourceDiagnostic(t *testing.T) {
	input := "decorations:\n  theme: halloween\n  footer-left-icon: ''\n"
	parsed := &parsedYAMLConfig{Contents: []byte(input + "pages: [{name: Home, columns: [{size: full, widgets: []}]}]"), Sources: []configSourceLocation{{File: "included.yml", Line: 10}, {File: "included.yml", Line: 11}, {File: "included.yml", Line: 12}}}
	_, err := newConfigFromParsedYAML(parsed)
	if err == nil || !strings.Contains(err.Error(), "included.yml:12:") {
		t.Fatalf("missing source: %v", err)
	}
}

func TestDecorationsPayloadAndFooter(t *testing.T) {
	c, err := parseDecorationsForTest(t, "decorations: {enabled: true, theme: custom, light-colors: ['#ff7518'], footer-left-icon: '/assets/pumpkin.svg', footer-right-icon: 'mdi:ghost'}")
	if err != nil {
		t.Fatal(err)
	}
	a := &application{Config: *c, Version: "dev"}
	a.Config.Server.BaseURL = "/glance"
	payload := a.DecorationConfig()
	if payload.Design.Left != "/glance/assets/pumpkin.svg" || !payload.Design.RightInvert {
		t.Fatalf("incorrect resolved payload: %+v", payload)
	}
	encoded, err := json.Marshal(payload)
	if err != nil || strings.Contains(string(encoded), "<script") {
		t.Fatal("invalid browser payload")
	}
	rendered := renderFooterWithPageForTest(t, a, &page{})
	for _, want := range []string{"decoration-footer-center", `data-decoration-footer="left"`, `data-decoration-footer="right"`, ">Glance</a>"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %s", want)
		}
	}
	a.Config.Branding.CustomFooter = "Custom center"
	if !strings.Contains(renderFooterWithPageForTest(t, a, &page{}), "Custom center") {
		t.Fatal("lost custom footer")
	}
	a.Config.Branding.HideFooter = true
	if strings.TrimSpace(renderFooterWithPageForTest(t, a, &page{})) != "" {
		t.Fatal("hidden footer rendered")
	}
	a.Config.Decorations.Enabled = false
	if a.DecorationConfig() != nil {
		t.Fatal("disabled payload rendered")
	}
}

func TestTwemojiIconResolver(t *testing.T) {
	icon := newCustomIconField("twemoji:1f383")
	if string(icon.URL) != "https://cdn.jsdelivr.net/gh/jdecked/twemoji@17.0.3/assets/svg/1f383.svg" || icon.AutoInvert {
		t.Fatalf("incorrect icon: %+v", icon)
	}
	// Multi-codepoint icons (for example the US flag) keep their full identity.
	icon = newCustomIconField("twemoji:1f1fa-1f1f8")
	if !strings.HasSuffix(string(icon.URL), "/1f1fa-1f1f8.svg") {
		t.Fatal("lost compound codepoint")
	}
}

func TestDecorationsYAMLTypes(t *testing.T) {
	for _, input := range []string{"decorations: []", "decorations: {enabled: wrong}", "decorations: {theme: auto, schedule: {bad: value}}"} {
		var c config
		if err := yaml.Unmarshal([]byte(input), &c); err == nil {
			t.Fatalf("accepted invalid shape %s", input)
		}
	}
}

func TestDecorationsScheduleAliases(t *testing.T) {
	_, err := parseDecorationsForTest(t, "shared: &dates [{theme: halloween, start: '10-01', end: '10-31'}]\ndecorations: {enabled: true, theme: auto, schedule: *dates}")
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseDecorationsForTest(t, "shared: &design {theme: halloween, start: '10-01', end: '10-31', light-colors: []}\ndecorations: {enabled: true, theme: auto, schedule: [*design]}")
	if err == nil || !strings.Contains(err.Error(), "only with theme: custom") {
		t.Fatalf("alias bypassed validation: %v", err)
	}
}
