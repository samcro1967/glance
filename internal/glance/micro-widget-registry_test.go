package glance

import "testing"

func TestMicroWidgetRegistryConstructsBothContainerSources(t *testing.T) {
	expected := []string{"astrology", "astronomy", "bookmark", "clock", "custom-api", "dns-stats", "docker", "environment", "link", "markets", "monitor", "now-playing", "releases", "repository", "rss", "server-stats", "weather"}
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
          - type: astrology
          - type: astronomy
            location: London, United Kingdom
          - type: bookmark
            title: Example
            url: https://example.com
          - type: clock
          - type: custom-api
            url: https://example.com/api
          - type: dns-stats
            service: adguard
            url: https://example.com
            username: example
            password: example
          - type: docker
            summary: true
          - type: environment
            location: London, United Kingdom
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
          - type: now-playing
            service: plex
            server: https://example.com
            api-key: example
          - type: releases
            repositories:
              - glanceapp/glance
          - type: repository
            repository: glanceapp/glance
          - type: rss
            feeds:
              - url: https://example.com/feed.xml
          - type: server-stats
            servers:
              - type: local
                name: Example
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
    - type: astrology
      position: 1
    - type: astronomy
      position: 2
      location: London, United Kingdom
    - type: bookmark
      position: 3
      title: Example
      url: https://example.com
    - type: clock
      position: 4
    - type: custom-api
      position: 5
      url: https://example.com/api
    - type: dns-stats
      position: 6
      service: adguard
      url: https://example.com
      username: example
      password: example
    - type: docker
      position: 7
      summary: true
    - type: environment
      position: 8
      location: London, United Kingdom
    - type: link
      position: 9
      title: Example
      url: https://example.com
  right:
    - type: markets
      position: 1
      markets:
        - symbol: SPY
    - type: monitor
      position: 2
      sites:
        - title: Example
          url: https://example.com
    - type: now-playing
      position: 3
      service: plex
      server: https://example.com
      api-key: example
    - type: releases
      position: 4
      repositories:
        - glanceapp/glance
    - type: repository
      position: 5
      repository: glanceapp/glance
    - type: rss
      position: 6
      feeds:
        - url: https://example.com/feed.xml
    - type: server-stats
      position: 7
      servers:
        - type: local
          name: Example
    - type: weather
      position: 8
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
	if got := len(config.FooterMicroWidgets.Left) + len(config.FooterMicroWidgets.Right); got != len(microWidgetRegistry) {
		t.Fatalf("footer micro count = %d, want %d", got, len(microWidgetRegistry))
	}
	assertValidMicroWidgets := func(candidates microWidgets) {
		t.Helper()
		for _, candidate := range candidates {
			if _, ok := candidate.(microWidget); !ok {
				t.Fatalf("footer child %T does not implement microWidget", candidate)
			}
			if _, invalid := candidate.(*invalidConfiguredMicroWidget); invalid {
				t.Fatalf("footer child %T decoded as invalid micro-widget", candidate)
			}
		}
	}
	assertValidMicroWidgets(config.FooterMicroWidgets.Left)
	assertValidMicroWidgets(config.FooterMicroWidgets.Right)
}

func TestMicroWidgetRegistryCanonicalDefaultsTypes(t *testing.T) {
	want := map[string]string{
		"astrology":    "astrology",
		"astronomy":    "astronomy",
		"custom-api":   "custom-api",
		"dns-stats":    "dns-stats",
		"docker":       "docker-containers",
		"environment":  "environment",
		"markets":      "markets",
		"monitor":      "monitor",
		"now-playing":  "now-playing",
		"releases":     "releases",
		"repository":   "repository",
		"rss":          "rss",
		"server-stats": "server-stats",
		"weather":      "weather",
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
