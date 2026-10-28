package glance

import (
	"reflect"
	"testing"
	"time"
)

// The registry is a behavior contract, not just a list of accepted settings.
// Check every optional declared capability so missing setters cannot silently
// turn a supported defaults property into an ignored configuration value.
func TestDeclaredWidgetCapabilitiesHaveImplementations(t *testing.T) {
	for name, descriptor := range widgetRegistry {
		w := descriptor.constructor()
		for _, d := range descriptor.capabilities {
			ok := true
			switch d.Capability {
			case widgetCapabilityTimeout:
				_, ok = w.(widgetTimeoutSetter)
			case widgetCapabilityAllowInsecure:
				_, ok = w.(widgetAllowInsecureSetter)
			case widgetCapabilityCollapseAfter:
				_, ok = w.(widgetCollapseAfterSetter)
			case widgetCapabilityCollapseAfterRows:
				_, ok = w.(widgetCollapseAfterRowsSetter)
			case widgetCapabilityLimit:
				_, ok = w.(widgetLimitSetter)
			case widgetCapabilityHeaders:
				_, ok = w.(widgetHeadersSetter)
			case widgetCapabilityBasicAuth:
				_, ok = w.(widgetBasicAuthSetter)
			case widgetCapabilityProxy:
				_, ok = w.(widgetProxySetter)
			case widgetCapabilityNewTab:
				_, ok = w.(widgetNewTabSetter)
			}
			if !ok {
				t.Errorf("%s declares %v without an implementation", name, d.Capability)
			}
		}
	}
}

func TestMediaDefaultsApplyAndExplicitValuesWin(t *testing.T) {
	for _, name := range []string{"arr", "seerr", "latest-media", "media-history", "now-playing", "torrenting"} {
		for _, explicit := range []bool{false, true} {
			w := widgetRegistry[name].constructor()
			b, _ := widgetBaseOf(w)
			b.Type = name
			b.configuredFields = make(yamlConfiguredFields)
			if explicit {
				b.configuredFields["allow-insecure"] = true
				b.configuredFields["collapse-after"] = true
			}
			value, collapse := true, 3
			d := widgetDefaultsConfig{Types: map[string]widgetDefaultValues{name: {AllowInsecure: &value}}}
			if name != "now-playing" {
				v := d.Types[name]
				v.CollapseAfter = &collapse
				d.Types[name] = v
			}
			if err := validateWidgetDefaults(d); err != nil {
				t.Fatal(err)
			}
			if err := applyWidgetCapabilityDefaults(w, d); err != nil {
				t.Fatal(err)
			}
			fields := reflect.ValueOf(w).Elem()
			if fields.FieldByName("AllowInsecure").Bool() != !explicit {
				t.Errorf("%s TLS default/override not applied", name)
			}
			if f := fields.FieldByName("CollapseAfter"); f.IsValid() && f.Int() != int64(ternary(explicit, 0, 3)) {
				t.Errorf("%s collapse override not applied", name)
			}
		}
	}
}

func TestMergedWidgetAndSourceOverridesWin(t *testing.T) {
	c, err := newConfigFromYAML([]byte(`widget-defaults:
  global:
    timeout: 7s
    allow-insecure: true
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - &environment
            type: environment
            location: Test
            <<: {timeout: 19s}
          - *environment
          - type: monitor
            sites:
              - url: https://example.invalid
                <<: {timeout: 0s, allow-insecure: false}
`))
	if err != nil {
		t.Fatal(err)
	}
	items := c.Pages[0].Columns[0].Widgets
	for _, index := range []int{0, 1} {
		if items[index].(*environmentWidget).Timeout != durationField(19*time.Second) {
			t.Fatal("merged widget timeout lost")
		}
	}
	site := items[2].(*monitorWidget).Sites[0]
	if site.Timeout != 0 || site.AllowInsecure {
		t.Fatal("merged explicit zero/false source overrides lost")
	}
}
