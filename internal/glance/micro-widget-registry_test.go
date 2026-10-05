package glance

import "testing"

func TestMicroWidgetRegistryConstructsBothContainerSources(t *testing.T) {
	expected := []string{"bookmark", "clock", "custom-api", "docker", "link", "markets", "monitor", "rss", "weather"}
	if len(microWidgetRegistry) != len(expected) {
		t.Fatalf("registered micro-widget count = %d, want %d", len(microWidgetRegistry), len(expected))
	}
	for _, typ := range expected {
		candidate, err := newMicroWidget(typ)
		if err != nil {
			t.Fatalf("newMicroWidget(%q) error = %v", typ, err)
		}
		if candidate.GetType() != typ {
			t.Errorf("newMicroWidget(%q) type = %q", typ, candidate.GetType())
		}
		_ = candidate.Render()
		_ = candidate.MicroItems(true)
	}
}

func TestStatusBarDecodesEveryRegisteredMicroWidget(t *testing.T) {
	yaml := `pages:
  - name: Home
    head-widgets:
      - type: status-bar
        widgets:
          - type: bookmark
            title: Example
            url: https://example.com
          - type: clock
          - type: custom-api
            url: https://example.com/api
          - type: docker
            summary: true
          - type: link
            title: Example
            url: https://example.com
          - type: markets
            markets:
              - symbol: SPY
          - type: monitor
            sites:
              - title: Example
                url: https://example.com
          - type: rss
            feeds:
              - url: https://example.com/feed.xml
          - type: weather
            location: London, United Kingdom
    columns:
      - size: full
`
	config, err := newConfigFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("newConfigFromYAML() error = %v", err)
	}
	status := config.Pages[0].HeadWidgets[0].(*statusBarWidget)
	if len(status.Widgets) != len(microWidgetRegistry) {
		t.Fatalf("status bar micro count = %d, want %d", len(status.Widgets), len(microWidgetRegistry))
	}
	for _, candidate := range status.Widgets {
		if _, ok := candidate.(microWidget); !ok {
			t.Fatalf("status bar child %T does not implement microWidget", candidate)
		}
	}
}

func TestFooterDecodesEveryRegisteredMicroWidget(t *testing.T) {
	yaml := `footer-micro-widgets:
  max-per-side: 10
  left:
    - type: bookmark
      position: 1
      title: Example
      url: https://example.com
    - type: clock
      position: 2
    - type: custom-api
      position: 3
      url: https://example.com/api
    - type: docker
      position: 4
      summary: true
    - type: link
      position: 5
      title: Example
      url: https://example.com
    - type: markets
      position: 6
      markets:
        - symbol: SPY
    - type: monitor
      position: 7
      sites:
        - title: Example
          url: https://example.com
    - type: rss
      position: 8
      feeds:
        - url: https://example.com/feed.xml
    - type: weather
      position: 9
      location: London, United Kingdom
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: html
            source: test
`
	config, err := newConfigFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("newConfigFromYAML() error = %v", err)
	}
	if len(config.FooterMicroWidgets.Left) != len(microWidgetRegistry) {
		t.Fatalf("footer micro count = %d, want %d", len(config.FooterMicroWidgets.Left), len(microWidgetRegistry))
	}
	for _, candidate := range config.FooterMicroWidgets.Left {
		if _, ok := candidate.(microWidget); !ok {
			t.Fatalf("footer child %T does not implement microWidget", candidate)
		}
		if _, invalid := candidate.(*invalidConfiguredMicroWidget); invalid {
			t.Fatalf("footer child %T decoded as invalid micro-widget", candidate)
		}
	}
}

func TestMicroWidgetRegistryCanonicalDefaultsTypes(t *testing.T) {
	want := map[string]string{
		"custom-api": "custom-api",
		"docker":     "docker-containers",
		"markets":    "markets",
		"monitor":    "monitor",
		"rss":        "rss",
		"weather":    "weather",
	}
	for microType, canonicalType := range want {
		descriptor := microWidgetRegistry[microType]
		if !descriptor.applyWidgetDefaults {
			t.Errorf("%s does not apply canonical widget defaults", microType)
		}
		if descriptor.canonicalWidgetType != canonicalType {
			t.Errorf("%s canonical widget type = %q, want %q", microType, descriptor.canonicalWidgetType, canonicalType)
		}
	}
	for _, microType := range []string{"bookmark", "clock", "link"} {
		descriptor := microWidgetRegistry[microType]
		if descriptor.applyWidgetDefaults {
			t.Errorf("lightweight %s unexpectedly applies widget defaults", microType)
		}
		if descriptor.canonicalWidgetType != "" {
			t.Errorf("lightweight %s canonical widget type = %q, want empty", microType, descriptor.canonicalWidgetType)
		}
	}
}
