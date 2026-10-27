package glance

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func propertyTestInspect(t *testing.T, contents string) []configPropertyDiagnostic {
	t.Helper()
	diagnostics, err := inspectConfigProperties([]byte(contents), nil)
	if err != nil {
		t.Fatal(err)
	}
	return diagnostics
}

func TestConfigPropertiesScopesAndValuesRemainPrivate(t *testing.T) {
	diagnostics := propertyTestInspect(t, `server:
  bogus: SECRET_SERVER
widget-defaults:
  global:
    tiemout: SECRET_DEFAULT
theme:
  presets:
    evening:
      bogus: SECRET_THEME
pages:
  - name: Home
    bogus: SECRET_PAGE
    columns:
      - size: full
        widgets:
          - type: group
            widgets:
              - type: environment
                location: Test
                units: SECRET_WIDGET
                pollen-provider: st-louis-county
footer-micro-widgets:
  left:
    - type: clock
      position: 1
      bogus: SECRET_MICRO
`)
	want := []string{"server.bogus", "widget-defaults.global.tiemout", "theme.presets.evening.bogus", "pages[0].bogus", "pages[0].columns[0].widgets[0].widgets[0].units", "footer-micro-widgets.left[0].bogus"}
	if len(diagnostics) != len(want) {
		t.Fatalf("got %+v", diagnostics)
	}
	for i, diagnostic := range diagnostics {
		if diagnostic.Path != want[i] || diagnostic.Line < 1 || strings.Contains(diagnostic.Message, "SECRET_") {
			t.Fatalf("incorrect diagnostic: %+v", diagnostic)
		}
	}
	if diagnostics[4].WidgetType != "environment" || diagnostics[5].WidgetType != "clock" {
		t.Fatal("widget context lost")
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	logConfigPropertyDiagnostics(diagnostics)
	if strings.Contains(logs.String(), "SECRET_") || !strings.Contains(logs.String(), `"action":"ignored"`) || !strings.Contains(logs.String(), `"widget_type":"environment"`) {
		t.Fatalf("unsafe or incomplete logs: %s", &logs)
	}
}

func TestConfigPropertiesAcceptDynamicMapsAndShorthands(t *testing.T) {
	diagnostics := propertyTestInspect(t, `dashboards:
  Default: [home]
auth:
  users:
    alice:
      password: secret
widget-defaults:
  global:
    proxy: http://proxy.invalid
decorations:
  enabled: false
  light-colors: ["#ffffff"]
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: custom-api
            title: Example
            url: https://example.invalid
            headers: {Authorization: secret, X-Custom: value}
            parameters: {arbitrary: value}
            options: {arbitrary: {nested: true}}
            subrequests:
              arbitrary:
                url: https://example.invalid
                headers: {X-Whatever: value}
            template: '{{ .JSON.String "value" }}'
          - type: releases
            repositories: [glanceapp/glance, {repository: glanceapp/glance, include-prereleases: true}]
          - type: monitor
            sites:
              - title: Example
                url: https://example.invalid
                headers: {X-Whatever: value}
          - type: status-bar
            widgets:
              - type: clock
                position: 1
                hour-format: 24h
footer-micro-widgets:
  max-per-side: 5
  left:
    - type: environment
      position: 1
      location: Test
      display: [pollen]
      pollen-provider: st-louis-county
`)
	if len(diagnostics) != 0 {
		t.Fatalf("valid custom shapes flagged: %+v", diagnostics)
	}
}

func TestConfigPropertiesNestedMapValuesStillChecked(t *testing.T) {
	diagnostics := propertyTestInspect(t, `theme:
  presets:
    evening: {invalid: secret}
pages:
  - columns:
      - widgets:
          - type: custom-api
            subrequests:
              named: {url: https://example.invalid, invalid: secret}
          - type: bookmarks
            groups:
              - links:
                  - {url: https://example.invalid, invalid: secret}
`)
	if len(diagnostics) != 3 || diagnostics[1].Path != "pages[0].columns[0].widgets[0].subrequests.named.invalid" {
		t.Fatalf("nested schemas lost: %+v", diagnostics)
	}
}

func TestConfigPropertiesAliasesMergePrecedenceAndDeduplication(t *testing.T) {
	diagnostics := propertyTestInspect(t, `pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - &clock
            type: clock
            title: First
            invalid: SECRET
          - *clock
          - <<: *clock
            title: Third
          - <<: [{type: environment, location: Test}, *clock]
            title: Environment
`)
	if len(diagnostics) != 2 || diagnostics[0].Line != 9 || diagnostics[1].Line != 9 || diagnostics[0].WidgetType != "clock" || diagnostics[1].WidgetType != "environment" {
		t.Fatalf("alias source, deduplication or merge precedence incorrect: %+v", diagnostics)
	}
	var cyclic yaml.Node
	cyclic.Kind, cyclic.Alias = yaml.AliasNode, &cyclic
	if got := configYAMLProperties(&cyclic, make(map[*yaml.Node]bool)); len(got) != 0 {
		t.Fatal("cyclic alias unexpectedly has properties")
	}
	if got := propertyTestInspect(t, "{server: {unknown: secret}, document: {unknown: secret}}"); len(got) != 2 {
		t.Fatalf("distinct properties on the same source line were collapsed: %+v", got)
	}
}

func TestConfigPropertiesOriginalIncludeLocation(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "glance.yml")
	included := filepath.Join(dir, "pages.yml")
	if err := os.WriteFile(root, []byte("pages:\n  $include: pages.yml\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(included, []byte("- name: Home\n  columns:\n    - size: full\n      widgets:\n        - type: clock\n          invalid: SECRET\n"), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := parseYAMLIncludesWithSources(root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := newConfigFromParsedYAML(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.propertyDiagnostics) != 1 || c.propertyDiagnostics[0].File != included || c.propertyDiagnostics[0].Line != 6 {
		t.Fatalf("original include location lost: %+v", c.propertyDiagnostics)
	}
	if _, invalid := c.Pages[0].Columns[0].Widgets[0].(*invalidConfiguredWidget); invalid || len(c.recoverableErrors) != 0 {
		t.Fatal("unknown property disabled a working widget")
	}
	issues := configValidationIssues(c)
	var diagnostic *configDiagnostic
	if len(issues) != 1 || !errors.As(issues[0], &diagnostic) || diagnostic.Line != 6 {
		t.Fatal("validation must reject ignored properties with source context")
	}
	// Every candidate load is inspected anew: fixed settings must not retain a
	// warning from the previous runtime generation.
	parsed.Contents = bytes.ReplaceAll(parsed.Contents, []byte("invalid: SECRET"), []byte("title: Fixed"))
	c, err = newConfigFromParsedYAML(parsed)
	if err != nil || len(c.propertyDiagnostics) != 0 || len(configValidationIssues(c)) != 0 {
		t.Fatalf("fixed reload kept stale diagnostics: config=%+v err=%v", c, err)
	}
}

func TestConfigPropertiesDeprecatedAliasIsWarningOnly(t *testing.T) {
	diagnostics := propertyTestInspect(t, `pages:
  - columns:
      - widgets:
          - type: markets
            stocks: [{symbol: AAPL}]
          - type: status-bar
            widgets:
              - type: markets
                stocks: [{symbol: MSFT}]
`)
	if len(diagnostics) != 2 || diagnostics[0].Replacement != "markets" || diagnostics[1].Replacement != "markets" {
		t.Fatalf("legacy alias not recognized: %+v", diagnostics)
	}
	if issues := configValidationIssues(&config{propertyDiagnostics: diagnostics}); len(issues) != 0 {
		t.Fatal("supported deprecated settings must not fail validation")
	}
}

func TestConfigPropertiesEveryRegisteredWidgetSchema(t *testing.T) {
	for name, descriptor := range widgetRegistry {
		t.Run("widget/"+name, func(t *testing.T) {
			scanner := configPropertyScanner{fields: make(map[reflect.Type]map[string]configPropertyField)}
			fields := scanner.structFields(reflect.TypeOf(descriptor.constructor()).Elem())
			for _, common := range []string{"type", "title", "cache", "hide-header"} {
				if _, ok := fields[common]; !ok {
					t.Fatalf("%s missing shared property %s", name, common)
				}
			}
			diagnostics := propertyTestInspect(t, "pages:\n  - columns:\n      - widgets:\n          - type: "+name+"\n            unknown-test-property: secret\n")
			if len(diagnostics) != 1 || diagnostics[0].WidgetType != name {
				t.Fatalf("registered widget not inspected: %+v", diagnostics)
			}
		})
	}
	for name := range microWidgetRegistry {
		diagnostics := propertyTestInspect(t, "footer-micro-widgets:\n  left:\n    - type: "+name+"\n      position: 1\n      unknown-test-property: secret\n")
		if len(diagnostics) != 1 || diagnostics[0].WidgetType != name {
			t.Fatalf("micro %s not inspected correctly: %+v", name, diagnostics)
		}
	}
}

func TestConfigPropertiesValidationCommandExitStatus(t *testing.T) {
	for _, test := range []struct {
		name, widget string
		want         int
	}{
		{"unknown", "type: clock\n            unknown-test-property: SECRET", 1},
		{"valid", "type: clock\n            hour-format: 24h", 0},
		{"deprecated", "type: markets\n            stocks: [{symbol: AAPL}]", 0},
		{"invalid-value", "type: clock\n            hour-format: invalid", 1},
		{"unknown-widget", "type: does-not-exist", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "glance.yml")
			contents := "pages:\n  - name: Home\n    columns:\n      - size: full\n        widgets:\n          - " + test.widget + "\n"
			if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			withCLIArgs(t, "--config", path, "config:validate")
			if got := Main(); got != test.want {
				t.Fatalf("exit status = %d, want %d", got, test.want)
			}
		})
	}
}

func TestConfigPropertiesRuntimeAcceptsUnknownPropertiesOnReload(t *testing.T) {
	oldWidget := newServerLifecycleTestWidget()
	oldApp := newServerLifecycleTestApplication(t, 0, oldWidget)
	diagnostics := newConfigRuntimeDiagnostics("glance.yml")
	server, err := newProcessServer("127.0.0.1", 0, oldApp.router())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.shutdown() }()
	server.setActiveApplication(oldApp)
	oldRuntime := oldApp.startRuntime()
	defer oldRuntime.stop()
	candidate, err := newConfigFromParsedYAML(&parsedYAMLConfig{Contents: []byte(`
server:
  host: 127.0.0.1
  port: 0
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: clock
            unknown-test-property: SECRET
`)})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidate.propertyDiagnostics) != 1 || len(candidate.recoverableErrors) != 0 {
		t.Fatal("unknown property must be diagnosed without disabling the widget")
	}
	generation := &runtimeGeneration{runtime: oldRuntime, config: &oldApp.Config}
	previous, err := generation.reload(server, candidate, diagnostics, nil)
	if err != nil {
		t.Fatalf("runtime rejected property warning: %v", err)
	}
	defer generation.runtime.stop()
	defer previous.stop()
	active := server.activeApplication.Load()
	if active == nil || active == oldApp || len(active.Config.propertyDiagnostics) != 1 {
		t.Fatal("runtime did not accept and retain candidate diagnostics")
	}
	if _, ok := active.Config.Pages[0].Columns[0].Widgets[0].(*clockWidget); !ok {
		t.Fatal("working clock was replaced with an error widget")
	}
	generation.completeReload(previous)
}

func TestConfigPropertiesCanonicalTestConfiguration(t *testing.T) {
	for _, name := range []string{"test-instance.yml", "testdata/widget-loading.yml"} {
		parsed, err := parseYAMLIncludesWithSources(filepath.Join("..", "..", name))
		if err != nil {
			t.Fatal(err)
		}
		diagnostics, err := inspectConfigProperties(parsed.Contents, parsed)
		if err != nil {
			t.Fatal(err)
		}
		for _, diagnostic := range diagnostics {
			if !diagnostic.deprecated() {
				t.Errorf("canonical configuration %s: %s:%d %s", name, diagnostic.File, diagnostic.Line, diagnostic.Message)
			}
		}
	}
}
