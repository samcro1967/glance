package glance

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestWidgetSupportsCapabilityRejectsUnknownWidget(t *testing.T) {
	if widgetSupportsCapability("not-a-widget", widgetCapabilityTitle, widgetCapabilityScopeGlobal) {
		t.Fatal("unknown widget unexpectedly supports common capability")
	}
}

func TestValidateWidgetDefaultsRejectsUnknownWidgetType(t *testing.T) {
	err := validateWidgetDefaults(widgetDefaultsConfig{
		Types: map[string]widgetDefaultValues{
			"not-a-widget": {},
		},
	})

	if err == nil || !strings.Contains(err.Error(), "unknown widget type") {
		t.Fatalf("expected unknown widget type error, got %v", err)
	}
}

func TestValidateWidgetDefaultsRejectsUnsupportedTypeCapability(t *testing.T) {
	value := 5
	err := validateWidgetDefaults(widgetDefaultsConfig{
		Types: map[string]widgetDefaultValues{
			"weather": {CollapseAfter: &value},
		},
	})

	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected unsupported capability error, got %v", err)
	}
}

func TestWidgetDefaultsGlobalThenTypeThenInstance(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    title: Global title
    title-url: https://global.example
    css-class: global-class
  types:
    rss:
      title: RSS title
      css-class: rss-class

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: rss
            feeds:
              - url: https://example.com/feed.xml
          - type: rss
            title: Instance title
            css-class: instance-class
            feeds:
              - url: https://example.com/feed.xml
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	first := config.Pages[0].Columns[0].Widgets[0].(*rssWidget)
	second := config.Pages[0].Columns[0].Widgets[1].(*rssWidget)

	if first.Title != "RSS title" {
		t.Fatalf("first title = %q, want RSS title", first.Title)
	}
	if first.TitleURL != "https://global.example" {
		t.Fatalf("first title-url = %q", first.TitleURL)
	}
	if first.CSSClass != "rss-class" {
		t.Fatalf("first css-class = %q, want rss-class", first.CSSClass)
	}

	if second.Title != "Instance title" {
		t.Fatalf("second title = %q", second.Title)
	}
	if second.CSSClass != "instance-class" {
		t.Fatalf("second css-class = %q", second.CSSClass)
	}
}

func TestWidgetDefaultsExplicitFalseOverridesInheritedTrue(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    hide-header: true

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: calendar
          - type: calendar
            hide-header: false
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	first := config.Pages[0].Columns[0].Widgets[0].(*calendarWidget)
	second := config.Pages[0].Columns[0].Widgets[1].(*calendarWidget)

	if !first.HideHeader {
		t.Fatal("omitted hide-header did not inherit true")
	}
	if second.HideHeader {
		t.Fatal("explicit hide-header false was overwritten by inherited true")
	}
}

func TestWidgetDefaultsTypeFalseOverridesGlobalTrue(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    hide-header: true
  types:
    calendar:
      hide-header: false

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: calendar
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	widget := config.Pages[0].Columns[0].Widgets[0].(*calendarWidget)
	if widget.HideHeader {
		t.Fatal("type hide-header false did not override global true")
	}
}

func TestWidgetDefaultsExplicitEmptyStringOverridesInheritedString(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    title: Global title
    title-url: https://global.example
    css-class: global-class

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: calendar
            title: ""
            title-url: ""
            css-class: ""
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	widget := config.Pages[0].Columns[0].Widgets[0].(*calendarWidget)

	if widget.Title != "Calendar" {
		t.Fatalf("title = %q, want built-in Calendar after explicit empty title", widget.Title)
	}
	if widget.TitleURL != "" {
		t.Fatalf("title-url = %q, want explicit empty value", widget.TitleURL)
	}
	if widget.CSSClass != "" {
		t.Fatalf("css-class = %q, want explicit empty value", widget.CSSClass)
	}
}

func TestWidgetDefaultsCachePrecedence(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    cache: 10m
  types:
    rss:
      cache: 20m

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: rss
            feeds:
              - url: https://example.com/one.xml
          - type: rss
            cache: 30m
            feeds:
              - url: https://example.com/two.xml
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	first := config.Pages[0].Columns[0].Widgets[0].(*rssWidget)
	second := config.Pages[0].Columns[0].Widgets[1].(*rssWidget)

	if first.cacheDuration != 20*time.Minute {
		t.Fatalf("type-inherited cache = %v, want 20m", first.cacheDuration)
	}
	if second.cacheDuration != 30*time.Minute {
		t.Fatalf("instance cache = %v, want 30m", second.cacheDuration)
	}
}

func TestWidgetDefaultsGlobalCache(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    cache: 11m

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: hacker-news
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	widget := config.Pages[0].Columns[0].Widgets[0].(*hackerNewsWidget)
	if widget.cacheDuration != 11*time.Minute {
		t.Fatalf("global cache = %v, want 11m", widget.cacheDuration)
	}
}

func TestWidgetTypeListDefaults(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  types:
    rss:
      limit: 12
      collapse-after: 8

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: rss
            feeds:
              - url: https://example.com/one.xml
          - type: rss
            limit: 3
            collapse-after: -1
            feeds:
              - url: https://example.com/two.xml
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	first := config.Pages[0].Columns[0].Widgets[0].(*rssWidget)
	second := config.Pages[0].Columns[0].Widgets[1].(*rssWidget)

	if first.Limit != 12 {
		t.Fatalf("inherited limit = %d, want 12", first.Limit)
	}
	if first.CollapseAfter != 8 {
		t.Fatalf("inherited collapse-after = %d, want 8", first.CollapseAfter)
	}

	if second.Limit != 3 {
		t.Fatalf("instance limit = %d, want 3", second.Limit)
	}
	if second.CollapseAfter != -1 {
		t.Fatalf("instance collapse-after = %d, want -1", second.CollapseAfter)
	}
}

func TestListDefaultsAreNotGlobal(t *testing.T) {
	value := 9

	for _, defaults := range []widgetDefaultsConfig{
		{Global: widgetDefaultValues{Limit: &value}},
		{Global: widgetDefaultValues{CollapseAfter: &value}},
	} {
		if err := validateWidgetDefaults(defaults); err == nil {
			t.Fatal("expected global list capability to be rejected")
		}
	}
}

func TestListDefaultRejectedForUnsupportedWidget(t *testing.T) {
	value := 9
	err := validateWidgetDefaults(widgetDefaultsConfig{
		Types: map[string]widgetDefaultValues{
			"weather": {Limit: &value},
		},
	})

	if err == nil {
		t.Fatal("expected weather limit default to be rejected")
	}
}

func TestVideosCollapseAfterRowsDefault(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  types:
    videos:
      collapse-after-rows: 6

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: videos
            channels:
              - UC_x5XG1OV2P6uZZ5FSM9Ttw
          - type: videos
            collapse-after-rows: 2
            channels:
              - UC_x5XG1OV2P6uZZ5FSM9Ttw
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	first := config.Pages[0].Columns[0].Widgets[0].(*videosWidget)
	second := config.Pages[0].Columns[0].Widgets[1].(*videosWidget)

	if first.CollapseAfterRows != 6 {
		t.Fatalf("inherited collapse-after-rows = %d, want 6", first.CollapseAfterRows)
	}
	if second.CollapseAfterRows != 2 {
		t.Fatalf("instance collapse-after-rows = %d, want 2", second.CollapseAfterRows)
	}
}

func TestCollapseAfterRowsOnlyAppliesToSupportedWidgets(t *testing.T) {
	value := 6
	err := validateWidgetDefaults(widgetDefaultsConfig{
		Types: map[string]widgetDefaultValues{
			"rss": {CollapseAfterRows: &value},
		},
	})

	if err == nil {
		t.Fatal("expected rss collapse-after-rows default to be rejected")
	}
}

func TestRegisteredWidgetTypesMatchNewWidget(t *testing.T) {
	for widgetType := range registeredWidgetTypes {
		if _, err := newWidget(widgetType); err != nil {
			t.Fatalf("registered widget type %q cannot be constructed: %v", widgetType, err)
		}
	}
}

func TestCapabilityMapUsesRegisteredWidgetTypes(t *testing.T) {
	for widgetType := range widgetTypeCapabilities {
		if _, ok := registeredWidgetTypes[widgetType]; !ok {
			t.Fatalf("capability map contains unregistered widget type %q", widgetType)
		}
	}
}

func TestCustomAPIHTTPDefaults(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    timeout: 9s
    allow-insecure: true
    headers:
      X-Global: global
      X-Override: global
  types:
    custom-api:
      timeout: 12s
      headers:
        X-Type: type
        X-Override: type

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: custom-api
            url: https://example.com
            template: test
          - type: custom-api
            url: https://example.com
            timeout: 20s
            allow-insecure: false
            headers:
              X-Instance: instance
              X-Override: instance
            template: test
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	first := config.Pages[0].Columns[0].Widgets[0].(*customAPIWidget)
	second := config.Pages[0].Columns[0].Widgets[1].(*customAPIWidget)

	if time.Duration(first.Timeout) != 12*time.Second {
		t.Fatalf("inherited timeout = %v, want 12s", time.Duration(first.Timeout))
	}
	if !first.AllowInsecure {
		t.Fatal("global allow-insecure true was not inherited")
	}
	if first.Headers["X-Global"] != "global" ||
		first.Headers["X-Type"] != "type" ||
		first.Headers["X-Override"] != "type" {
		t.Fatalf("unexpected inherited headers: %#v", first.Headers)
	}

	if time.Duration(second.Timeout) != 20*time.Second {
		t.Fatalf("instance timeout = %v, want 20s", time.Duration(second.Timeout))
	}
	if second.AllowInsecure {
		t.Fatal("explicit allow-insecure false was overwritten")
	}
	if second.Headers["X-Global"] != "global" ||
		second.Headers["X-Type"] != "type" ||
		second.Headers["X-Instance"] != "instance" ||
		second.Headers["X-Override"] != "instance" {
		t.Fatalf("unexpected merged instance headers: %#v", second.Headers)
	}
}

func TestHTTPDefaultsRejectUnsupportedWidget(t *testing.T) {
	timeout := durationField(5 * time.Second)
	insecure := true

	for _, values := range []widgetDefaultValues{
		{Timeout: &timeout},
		{AllowInsecure: &insecure},
		{Headers: map[string]string{"X-Test": "value"}},
	} {
		err := validateWidgetDefaults(widgetDefaultsConfig{
			Types: map[string]widgetDefaultValues{
				"weather": values,
			},
		})
		if err == nil {
			t.Fatal("expected unsupported weather HTTP capability to be rejected")
		}
	}
}

func TestMonitorChildExplicitValuesOverrideDefaults(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    timeout: 9s
    allow-insecure: true
  types:
    monitor:
      timeout: 12s

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: monitor
            sites:
              - title: inherited
                url: https://example.com
              - title: explicit
                url: https://example.com
                timeout: 0s
                allow-insecure: false
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	widget := config.Pages[0].Columns[0].Widgets[0].(*monitorWidget)

	if time.Duration(widget.Sites[0].Timeout) != 12*time.Second {
		t.Fatalf("inherited site timeout = %v, want 12s", time.Duration(widget.Sites[0].Timeout))
	}
	if !widget.Sites[0].AllowInsecure {
		t.Fatal("site did not inherit allow-insecure true")
	}

	if time.Duration(widget.Sites[1].Timeout) != 0 {
		t.Fatalf("explicit child timeout = %v, want 0", time.Duration(widget.Sites[1].Timeout))
	}
	if widget.Sites[1].AllowInsecure {
		t.Fatal("explicit child allow-insecure false was overwritten")
	}
}

func TestServerStatsChildExplicitTimeoutOverridesDefault(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  types:
    server-stats:
      timeout: 9s

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: server-stats
            servers:
              - type: remote
                url: http://example.com
              - type: remote
                url: http://example.com
                timeout: 1s
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	widget := config.Pages[0].Columns[0].Widgets[0].(*serverStatsWidget)

	if time.Duration(widget.Servers[0].Timeout) != 9*time.Second {
		t.Fatalf("inherited server timeout = %v, want 9s", time.Duration(widget.Servers[0].Timeout))
	}
	if time.Duration(widget.Servers[1].Timeout) != time.Second {
		t.Fatalf("explicit server timeout = %v, want 1s", time.Duration(widget.Servers[1].Timeout))
	}
}

func TestRSSChildHeadersOverrideInheritedHeaders(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    headers:
      X-Global: global
      X-Override: global
  types:
    rss:
      headers:
        X-Type: type
        X-Override: type

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: rss
            feeds:
              - url: https://example.com/feed.xml
                headers:
                  X-Feed: feed
                  X-Override: feed
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	headers := config.Pages[0].Columns[0].Widgets[0].(*rssWidget).FeedRequests[0].Headers

	if headers["X-Global"] != "global" ||
		headers["X-Type"] != "type" ||
		headers["X-Feed"] != "feed" ||
		headers["X-Override"] != "feed" {
		t.Fatalf("unexpected feed headers: %#v", headers)
	}
}

func TestCustomAPIBasicAuthDefaults(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  types:
    custom-api:
      basic-auth:
        username: default-user
        password: default-pass

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: custom-api
            url: https://example.com
            template: test
            subrequests:
              inherited:
                url: https://example.com/inherited
              explicit:
                url: https://example.com/explicit
                basic-auth:
                  username: child-user
                  password: child-pass

          - type: custom-api
            url: https://example.com
            basic-auth:
              username: instance-user
              password: instance-pass
            template: test
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	first := config.Pages[0].Columns[0].Widgets[0].(*customAPIWidget)
	second := config.Pages[0].Columns[0].Widgets[1].(*customAPIWidget)

	if first.BasicAuth.Username != "default-user" ||
		first.BasicAuth.Password != "default-pass" {
		t.Fatalf("primary inherited auth = %#v, want default credentials", first.BasicAuth)
	}

	inherited := first.Subrequests["inherited"]
	if inherited.BasicAuth.Username != "default-user" ||
		inherited.BasicAuth.Password != "default-pass" {
		t.Fatalf("subrequest inherited auth = %#v, want default credentials", inherited.BasicAuth)
	}

	explicit := first.Subrequests["explicit"]
	if explicit.BasicAuth.Username != "child-user" ||
		explicit.BasicAuth.Password != "child-pass" {
		t.Fatalf("subrequest explicit auth = %#v, want child credentials", explicit.BasicAuth)
	}

	if second.BasicAuth.Username != "instance-user" ||
		second.BasicAuth.Password != "instance-pass" {
		t.Fatalf("primary explicit auth = %#v, want instance credentials", second.BasicAuth)
	}
}

func TestMonitorBasicAuthDefaults(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  types:
    monitor:
      basic-auth:
        username: default-user
        password: default-pass

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: monitor
            sites:
              - title: inherited
                url: https://example.com/inherited
              - title: explicit
                url: https://example.com/explicit
                basic-auth:
                  username: child-user
                  password: child-pass
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	widget := config.Pages[0].Columns[0].Widgets[0].(*monitorWidget)

	first := widget.Sites[0].SiteStatusRequest
	second := widget.Sites[1].SiteStatusRequest

	if first.BasicAuth.Username != "default-user" ||
		first.BasicAuth.Password != "default-pass" {
		t.Fatalf("inherited monitor auth = %#v, want default credentials", first.BasicAuth)
	}

	if second.BasicAuth.Username != "child-user" ||
		second.BasicAuth.Password != "child-pass" {
		t.Fatalf("explicit monitor auth = %#v, want child credentials", second.BasicAuth)
	}
}

func TestBasicAuthCannotBeGlobal(t *testing.T) {
	auth := basicAuthDefaults{
		Username: "user",
		Password: "pass",
	}

	err := validateWidgetDefaults(widgetDefaultsConfig{
		Global: widgetDefaultValues{
			BasicAuth: &auth,
		},
	})

	if err == nil {
		t.Fatal("expected global basic-auth default to be rejected")
	}
}

func TestRSSBasicAuthTypeDefault(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  types:
    rss:
      basic-auth:
        username: inherited-user
        password: inherited-pass

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: rss
            feeds:
              - url: https://example.com/inherited.xml
              - url: https://example.com/explicit.xml
                basic-auth:
                  username: child-user
                  password: child-pass
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	widget := config.Pages[0].Columns[0].Widgets[0].(*rssWidget)
	if len(widget.FeedRequests) != 2 {
		t.Fatalf("feeds = %d, want 2", len(widget.FeedRequests))
	}

	first := widget.FeedRequests[0]
	if first.BasicAuth.Username != "inherited-user" ||
		first.BasicAuth.Password != "inherited-pass" {
		t.Fatalf("inherited RSS auth = %#v, want inherited credentials", first.BasicAuth)
	}

	second := widget.FeedRequests[1]
	if second.BasicAuth.Username != "child-user" ||
		second.BasicAuth.Password != "child-pass" {
		t.Fatalf("explicit RSS auth = %#v, want child credentials", second.BasicAuth)
	}
}

func TestRedditProxyDefault(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  types:
    reddit:
      proxy:
        url: http://proxy.example.com:8080
        allow-insecure: true
        timeout: 9s

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: reddit
            subreddit: golang

          - type: reddit
            subreddit: selfhosted
            proxy:
              url: http://instance.example.com:3128
              timeout: 3s
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	first := config.Pages[0].Columns[0].Widgets[0].(*redditWidget)
	second := config.Pages[0].Columns[0].Widgets[1].(*redditWidget)

	if first.Proxy.URL != "http://proxy.example.com:8080" {
		t.Fatalf("inherited proxy URL = %q", first.Proxy.URL)
	}
	if !first.Proxy.AllowInsecure {
		t.Fatal("inherited proxy allow-insecure = false, want true")
	}
	if time.Duration(first.Proxy.Timeout) != 9*time.Second {
		t.Fatalf("inherited proxy timeout = %v, want 9s", time.Duration(first.Proxy.Timeout))
	}
	if first.Proxy.client == nil {
		t.Fatal("inherited proxy did not initialize client")
	}

	if second.Proxy.URL != "http://instance.example.com:3128" {
		t.Fatalf("explicit proxy URL = %q", second.Proxy.URL)
	}
	if time.Duration(second.Proxy.Timeout) != 3*time.Second {
		t.Fatalf("explicit proxy timeout = %v, want 3s", time.Duration(second.Proxy.Timeout))
	}
}

func TestProxyCannotBeGlobal(t *testing.T) {
	proxy := proxyOptionsField{URL: "http://proxy.example.com:8080"}

	err := validateWidgetDefaults(widgetDefaultsConfig{
		Global: widgetDefaultValues{
			Proxy: &proxy,
		},
	})

	if err == nil {
		t.Fatal("expected global proxy default to be rejected")
	}
}

func TestProxyRejectedForUnsupportedWidget(t *testing.T) {
	proxy := proxyOptionsField{URL: "http://proxy.example.com:8080"}

	err := validateWidgetDefaults(widgetDefaultsConfig{
		Types: map[string]widgetDefaultValues{
			"rss": {
				Proxy: &proxy,
			},
		},
	})

	if err == nil {
		t.Fatal("expected rss proxy default to be rejected")
	}
}

func TestSearchNewTabDefaults(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    new-tab: true
  types:
    search:
      new-tab: false

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: search

          - type: search
            new-tab: true

          - type: search
            target: named-window
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	first := config.Pages[0].Columns[0].Widgets[0].(*searchWidget)
	second := config.Pages[0].Columns[0].Widgets[1].(*searchWidget)
	third := config.Pages[0].Columns[0].Widgets[2].(*searchWidget)

	if first.NewTab {
		t.Fatal("type new-tab default did not override global default")
	}

	if !second.NewTab {
		t.Fatal("explicit search new-tab did not override inherited default")
	}

	if third.NewTab {
		t.Fatal("search without explicit new-tab did not inherit type default")
	}

	if third.Target != "named-window" {
		t.Fatalf("search target = %q, want named-window", third.Target)
	}
}

func TestMonitorNewTabDefaults(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    new-tab: true
  types:
    monitor:
      new-tab: false

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: monitor
            sites:
              - title: inherited
                url: https://example.com/inherited

              - title: explicit-same
                url: https://example.com/same
                same-tab: true

              - title: explicit-new
                url: https://example.com/new
                same-tab: false
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	widget := config.Pages[0].Columns[0].Widgets[0].(*monitorWidget)

	if !widget.Sites[0].SameTab {
		t.Fatal("monitor type new-tab=false did not produce same-tab behavior")
	}

	if !widget.Sites[1].SameTab {
		t.Fatal("explicit same-tab=true was not preserved")
	}

	if widget.Sites[2].SameTab {
		t.Fatal("explicit same-tab=false was not preserved")
	}
}

func TestCommonNewTabCapabilityAppliesToWeatherTitleURL(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    new-tab: false
  types:
    weather:
      title-url: https://example.com/weather

pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: weather
            location: London, United Kingdom
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	candidate := config.Pages[0].Columns[0].Widgets[0]
	base, ok := widgetBaseOf(candidate)
	if !ok {
		t.Fatal("weather widget has no widget base")
	}

	if base.OpenLinksInNewTab {
		t.Fatal("weather title URL did not inherit global new-tab=false")
	}

	if base.TitleURL != "https://example.com/weather" {
		t.Fatalf("weather title URL = %q, want inherited URL", base.TitleURL)
	}

	if !widgetSupportsCapability(
		"weather",
		widgetCapabilityNewTab,
		widgetCapabilityScopeGlobal,
	) {
		t.Fatal("weather does not support common global new-tab")
	}

	if !widgetSupportsCapability(
		"weather",
		widgetCapabilityNewTab,
		widgetCapabilityScopeType,
	) {
		t.Fatal("weather does not support common type new-tab")
	}
}

func TestBookmarksNewTabDefaults(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    new-tab: true
  types:
    bookmarks:
      new-tab: false

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: bookmarks
            groups:
              - title: inherited
                links:
                  - title: inherited
                    url: https://example.com/inherited
                  - title: link-new-tab
                    url: https://example.com/link-new
                    same-tab: false

              - title: explicit-group-new-tab
                same-tab: false
                links:
                  - title: inherited-from-group
                    url: https://example.com/group-new

              - title: target-group
                target: named-group
                links:
                  - title: group-target
                    url: https://example.com/group-target
                  - title: link-target
                    url: https://example.com/link-target
                    target: named-link
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	widget := config.Pages[0].Columns[0].Widgets[0].(*bookmarksWidget)

	if !widget.Groups[0].Links[0].SameTab ||
		widget.Groups[0].Links[0].Target != "" {
		t.Fatalf("inherited bookmark did not resolve to same tab: %#v", widget.Groups[0].Links[0])
	}

	if widget.Groups[0].Links[1].SameTab ||
		widget.Groups[0].Links[1].Target != "_blank" {
		t.Fatalf("explicit link same-tab=false not preserved: %#v", widget.Groups[0].Links[1])
	}

	if widget.Groups[1].Links[0].SameTab ||
		widget.Groups[1].Links[0].Target != "_blank" {
		t.Fatalf("explicit group same-tab=false not preserved: %#v", widget.Groups[1].Links[0])
	}

	if widget.Groups[2].Links[0].Target != "named-group" {
		t.Fatalf("group target = %q, want named-group", widget.Groups[2].Links[0].Target)
	}

	if widget.Groups[2].Links[1].Target != "named-link" {
		t.Fatalf("link target = %q, want named-link", widget.Groups[2].Links[1].Target)
	}
}

func TestDockerContainersNewTabDefaultResolution(t *testing.T) {
	value := false
	widget := &dockerContainersWidget{}

	widget.setDefaultNewTab(value)

	if widget.DefaultNewTab == nil {
		t.Fatal("docker new-tab default was not stored")
	}

	if *widget.DefaultNewTab {
		t.Fatal("docker new-tab default = true, want false")
	}
}

func TestDockerContainersNewTabCapability(t *testing.T) {
	if !widgetSupportsCapability(
		"docker-containers",
		widgetCapabilityNewTab,
		widgetCapabilityScopeGlobal,
	) {
		t.Fatal("docker-containers does not support global new-tab")
	}

	if !widgetSupportsCapability(
		"docker-containers",
		widgetCapabilityNewTab,
		widgetCapabilityScopeType,
	) {
		t.Fatal("docker-containers does not support type new-tab")
	}
}

func TestConstructedWidgetDefaultsLinksToNewTab(t *testing.T) {
	candidate, err := newWidget("releases")
	if err != nil {
		t.Fatal(err)
	}

	base, ok := widgetBaseOf(candidate)
	if !ok {
		t.Fatal("constructed widget has no widget base")
	}

	if !base.OpenLinksInNewTab {
		t.Fatal("constructed widget does not preserve upstream new-tab behavior")
	}
}

func TestOrdinaryWidgetNewTabDefault(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    new-tab: false

pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: releases
            repositories:
              - glanceapp/glance
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	candidate := config.Pages[0].Columns[0].Widgets[0]
	base, ok := widgetBaseOf(candidate)
	if !ok {
		t.Fatal("releases widget has no widget base")
	}

	if base.OpenLinksInNewTab {
		t.Fatal("global new-tab=false was not applied to releases")
	}
}

func TestMarketsAndStocksKeepDistinctTypeDefaults(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  types:
    markets:
      new-tab: false
    stocks:
      new-tab: true

pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: markets
            markets:
              - symbol: AAPL
          - type: stocks
            markets:
              - symbol: MSFT
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	markets := config.Pages[0].Columns[0].Widgets[0].(*marketsWidget)
	stocks := config.Pages[0].Columns[0].Widgets[1].(*marketsWidget)

	if markets.GetType() != "markets" {
		t.Fatalf("markets GetType() = %q, want markets", markets.GetType())
	}
	if stocks.GetType() != "stocks" {
		t.Fatalf("stocks GetType() = %q, want stocks", stocks.GetType())
	}

	if markets.OpenLinksInNewTab {
		t.Fatal("markets widget did not receive markets new-tab=false")
	}
	if !stocks.OpenLinksInNewTab {
		t.Fatal("stocks widget did not receive stocks new-tab=true")
	}

	if !widgetSupportsCapability("markets", widgetCapabilityNewTab, widgetCapabilityScopeType) {
		t.Fatal("markets does not support new-tab")
	}
	if !widgetSupportsCapability("stocks", widgetCapabilityNewTab, widgetCapabilityScopeType) {
		t.Fatal("stocks does not support new-tab")
	}
}

func TestWidgetTitleURLRespectsResolvedNewTab(t *testing.T) {
	t.Run("upstream default opens new tab", func(t *testing.T) {
		candidate, err := newWidget("releases")
		if err != nil {
			t.Fatal(err)
		}

		widget := candidate.(*releasesWidget)
		widget.Title = "Releases"
		widget.TitleURL = "https://example.com"
		widget.ContentAvailable = true

		rendered := string(widget.Render())

		if !strings.Contains(rendered, `href="https://example.com" target="_blank" rel="noreferrer"`) {
			t.Fatalf("rendered title link does not preserve upstream new-tab behavior: %s", rendered)
		}
	})

	t.Run("resolved false opens same tab", func(t *testing.T) {
		candidate, err := newWidget("releases")
		if err != nil {
			t.Fatal(err)
		}

		widget := candidate.(*releasesWidget)
		widget.Title = "Releases"
		widget.TitleURL = "https://example.com"
		widget.ContentAvailable = true
		widget.OpenLinksInNewTab = false

		rendered := string(widget.Render())

		if strings.Contains(rendered, `href="https://example.com" target="_blank"`) {
			t.Fatalf("rendered title link unexpectedly opens new tab: %s", rendered)
		}

		if !strings.Contains(rendered, `href="https://example.com"`) {
			t.Fatalf("rendered title link missing: %s", rendered)
		}
	})
}

func TestWidgetDefaultsApplyRecursivelyToContainerChildren(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    new-tab: false
  types:
    releases:
      new-tab: true

pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: group
            widgets:
              - type: releases
                repositories:
                  - glanceapp/glance
              - type: stack
                widgets:
                  - type: rss
                    feeds:
                      - url: https://example.com/feed.xml
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	group := config.Pages[0].Columns[0].Widgets[0].(*groupWidget)

	releases := group.Widgets[0].(*releasesWidget)
	if !releases.OpenLinksInNewTab {
		t.Fatal("nested releases widget did not receive type new-tab=true")
	}

	stack := group.Widgets[1].(*stackWidget)
	rss := stack.Widgets[0].(*rssWidget)
	if rss.OpenLinksInNewTab {
		t.Fatal("grandchild rss widget did not receive global new-tab=false")
	}
}

func TestNestedWidgetInstanceOverridesInheritedDefault(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    title: Global title
  types:
    releases:
      title: Type title

pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: group
            widgets:
              - type: releases
                title: Instance title
                repositories:
                  - glanceapp/glance
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	group := config.Pages[0].Columns[0].Widgets[0].(*groupWidget)
	releases := group.Widgets[0].(*releasesWidget)

	if releases.Title != "Instance title" {
		t.Fatalf("nested explicit title = %q, want %q", releases.Title, "Instance title")
	}
}

func TestStatusBarPreservesChildNewTabDefaults(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    new-tab: false
  types:
    markets:
      new-tab: true

pages:
  - name: Home
    head-widgets:
      - type: status-bar
        widgets:
          - type: markets
            markets:
              - symbol: AAPL
                name: Apple
                symbol-link: https://example.com/apple
                chart-link: https://example.com/apple/chart
          - type: rss
            feeds:
              - url: https://example.com/feed.xml
    columns:
      - size: full
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	statusBar := config.Pages[0].HeadWidgets[0].(*statusBarWidget)
	markets := statusBar.Widgets[0].(*marketsWidget)
	rss := statusBar.Widgets[1].(*rssWidget)

	if !markets.OpenLinksInNewTab {
		t.Fatal("markets child did not receive type new-tab=true")
	}
	if rss.OpenLinksInNewTab {
		t.Fatal("rss child did not retain global new-tab=false")
	}

	markets.Markets = marketList{
		{
			marketRequest: marketRequest{
				CustomName: "Apple",
				Symbol:     "AAPL",
				SymbolLink: "https://example.com/apple",
				ChartLink:  "https://example.com/apple/chart",
			},
			Name: "Apple",
		},
	}

	rss.Items = rssFeedItemList{
		{
			Title:       "RSS item",
			Link:        "https://example.com/article",
			ChannelName: "Example",
			ChannelURL:  "https://example.com",
		},
	}

	items := statusBar.CompactItems()
	if len(items) != 2 {
		t.Fatalf("CompactItems() length = %d, want 2", len(items))
	}
	if items[0].Kind != "market" || !items[0].OpenLinksInNewTab {
		t.Fatalf("market compact policy not preserved: %+v", items[0])
	}
	if items[1].Kind != "rss" || items[1].OpenLinksInNewTab {
		t.Fatalf("rss compact policy not preserved: %+v", items[1])
	}

	rendered := string(statusBar.Render())

	if !strings.Contains(rendered, `href="https://example.com/apple" target="_blank" rel="noreferrer"`) {
		t.Fatalf("market link did not open in new tab: %s", rendered)
	}
	if strings.Contains(rendered, `href="https://example.com/article" target="_blank"`) {
		t.Fatalf("rss link unexpectedly opened in new tab: %s", rendered)
	}
	if !strings.Contains(rendered, `href="https://example.com/article"`) {
		t.Fatalf("rss link missing: %s", rendered)
	}
}

func TestStatusBarOwnsCommonNewTabButChildrenResolveIndependently(t *testing.T) {
	if !widgetSupportsCapability(
		"status-bar",
		widgetCapabilityNewTab,
		widgetCapabilityScopeGlobal,
	) {
		t.Fatal("status-bar does not support common global new-tab for its title URL")
	}

	if !widgetSupportsCapability(
		"status-bar",
		widgetCapabilityNewTab,
		widgetCapabilityScopeType,
	) {
		t.Fatal("status-bar does not support common type new-tab for its title URL")
	}

	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    new-tab: false
  types:
    status-bar:
      new-tab: true

pages:
  - name: Home
    head-widgets:
      - type: status-bar
        title-url: https://example.com/status
        widgets:
          - type: rss
            feeds:
              - url: https://example.com/feed.xml
    columns:
      - size: full
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	statusBar := config.Pages[0].HeadWidgets[0].(*statusBarWidget)

	if !statusBar.OpenLinksInNewTab {
		t.Fatal("status-bar did not receive its type new-tab=true title-link policy")
	}

	rss := statusBar.Widgets[0].(*rssWidget)
	if rss.OpenLinksInNewTab {
		t.Fatal("status-bar type policy leaked into child RSS widget")
	}
}

func TestBatchOneHTTPDefaultsHierarchy(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    timeout: 11s
    allow-insecure: true
    headers:
      X-Global: global
      X-Override: global
  types:
    rss:
      timeout: 12s
      headers:
        X-Type: rss
        X-Override: rss
      basic-auth:
        username: rss-user
        password: rss-pass
    ics-events:
      timeout: 13s
      headers:
        X-Type: ics
      basic-auth:
        username: ics-user
        password: ics-pass
    extension:
      timeout: 14s
      headers:
        X-Type: extension
      basic-auth:
        username: extension-user
        password: extension-pass
    monitor:
      timeout: 15s
      headers:
        X-Type: monitor
      basic-auth:
        username: monitor-user
        password: monitor-pass

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: rss
            feeds:
              - url: https://example.com/inherited.xml
              - url: https://example.com/explicit.xml
                timeout: 21s
                allow-insecure: false
                headers:
                  X-Child: rss
                  X-Override: child
                basic-auth:
                  username: child-user
                  password: child-pass

          - type: ics-events
            sources:
              - url: https://example.com/inherited.ics
              - url: https://example.com/explicit.ics
                timeout: 22s
                allow-insecure: false
                headers:
                  X-Child: ics
                basic-auth:
                  username: child-user
                  password: child-pass

          - type: extension
            url: https://example.com/extension
            timeout: 23s
            allow-insecure: false
            headers:
              X-Child: extension
              X-Override: child
            basic-auth:
              username: child-user
              password: child-pass

          - type: monitor
            sites:
              - title: Inherited
                url: https://example.com/inherited
              - title: Explicit
                url: https://example.com/explicit
                timeout: 24s
                allow-insecure: false
                headers:
                  X-Child: monitor
                  X-Override: child
                basic-auth:
                  username: child-user
                  password: child-pass
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	widgets := config.Pages[0].Columns[0].Widgets

	rss := widgets[0].(*rssWidget)
	if got := time.Duration(rss.FeedRequests[0].Timeout); got != 12*time.Second {
		t.Fatalf("RSS inherited timeout = %v, want 12s", got)
	}
	if !rss.FeedRequests[0].AllowInsecure {
		t.Fatal("RSS did not inherit global allow-insecure")
	}
	if got := rss.FeedRequests[0].Headers["X-Override"]; got != "rss" {
		t.Fatalf("RSS type header override = %q, want rss", got)
	}
	if got := rss.FeedRequests[0].Headers["X-Global"]; got != "global" {
		t.Fatalf("RSS global header = %q, want global", got)
	}
	if rss.FeedRequests[0].BasicAuth.Username != "rss-user" {
		t.Fatalf("RSS inherited auth = %#v", rss.FeedRequests[0].BasicAuth)
	}
	if got := time.Duration(rss.FeedRequests[1].Timeout); got != 21*time.Second {
		t.Fatalf("RSS child timeout = %v, want 21s", got)
	}
	if rss.FeedRequests[1].AllowInsecure {
		t.Fatal("RSS explicit allow-insecure false was overwritten")
	}
	if got := rss.FeedRequests[1].Headers["X-Override"]; got != "child" {
		t.Fatalf("RSS child header override = %q, want child", got)
	}
	if rss.FeedRequests[1].BasicAuth.Username != "child-user" {
		t.Fatalf("RSS explicit auth = %#v", rss.FeedRequests[1].BasicAuth)
	}

	ics := widgets[1].(*icsEventsWidget)
	if got := time.Duration(ics.Sources[0].Timeout); got != 13*time.Second {
		t.Fatalf("ICS inherited timeout = %v, want 13s", got)
	}
	if !ics.Sources[0].AllowInsecure {
		t.Fatal("ICS did not inherit global allow-insecure")
	}
	if got := ics.Sources[0].Headers["X-Global"]; got != "global" {
		t.Fatalf("ICS global header = %q, want global", got)
	}
	if ics.Sources[0].BasicAuth.Username != "ics-user" {
		t.Fatalf("ICS inherited auth = %#v", ics.Sources[0].BasicAuth)
	}
	if got := time.Duration(ics.Sources[1].Timeout); got != 22*time.Second {
		t.Fatalf("ICS child timeout = %v, want 22s", got)
	}
	if ics.Sources[1].AllowInsecure {
		t.Fatal("ICS explicit allow-insecure false was overwritten")
	}
	if ics.Sources[1].BasicAuth.Username != "child-user" {
		t.Fatalf("ICS explicit auth = %#v", ics.Sources[1].BasicAuth)
	}

	extension := widgets[2].(*extensionWidget)
	if got := time.Duration(extension.Timeout); got != 23*time.Second {
		t.Fatalf("extension explicit timeout = %v, want 23s", got)
	}
	if extension.AllowInsecure {
		t.Fatal("extension explicit allow-insecure false was overwritten")
	}
	if got := extension.Headers["X-Global"]; got != "global" {
		t.Fatalf("extension global header = %q, want global", got)
	}
	if got := extension.Headers["X-Override"]; got != "child" {
		t.Fatalf("extension instance header override = %q, want child", got)
	}
	if extension.BasicAuth.Username != "child-user" {
		t.Fatalf("extension explicit auth = %#v", extension.BasicAuth)
	}

	monitor := widgets[3].(*monitorWidget)
	if got := time.Duration(monitor.Sites[0].Timeout); got != 15*time.Second {
		t.Fatalf("monitor inherited timeout = %v, want 15s", got)
	}
	if !monitor.Sites[0].AllowInsecure {
		t.Fatal("monitor did not inherit global allow-insecure")
	}
	if got := monitor.Sites[0].Headers["X-Global"]; got != "global" {
		t.Fatalf("monitor global header = %q, want global", got)
	}
	if monitor.Sites[0].BasicAuth.Username != "monitor-user" {
		t.Fatalf("monitor inherited auth = %#v", monitor.Sites[0].BasicAuth)
	}
	if got := time.Duration(monitor.Sites[1].Timeout); got != 24*time.Second {
		t.Fatalf("monitor child timeout = %v, want 24s", got)
	}
	if monitor.Sites[1].AllowInsecure {
		t.Fatal("monitor explicit allow-insecure false was overwritten")
	}
	if got := monitor.Sites[1].Headers["X-Override"]; got != "child" {
		t.Fatalf("monitor child header override = %q, want child", got)
	}
	if monitor.Sites[1].BasicAuth.Username != "child-user" {
		t.Fatalf("monitor explicit auth = %#v", monitor.Sites[1].BasicAuth)
	}
}

func TestBatchTwoChangeDetectionAndServerStatsDefaultsHierarchy(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    timeout: 11s
    allow-insecure: true
    headers:
      X-Global: global
      X-Override: global
  types:
    change-detection:
      timeout: 12s
      allow-insecure: true
      headers:
        X-Type: change-detection
        X-Override: type
    server-stats:
      timeout: 13s
      allow-insecure: true

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: change-detection
            instance-url: https://example.com
            timeout: 21s
            allow-insecure: false
            headers:
              X-Instance: present
              X-Override: instance

          - type: server-stats
            servers:
              - type: remote
                name: Inherited
                url: https://example.com/inherited
              - type: remote
                name: Explicit
                url: https://example.com/explicit
                timeout: 22s
                allow-insecure: false
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	widgets := config.Pages[0].Columns[0].Widgets

	changeDetection := widgets[0].(*changeDetectionWidget)
	if got := time.Duration(changeDetection.Timeout); got != 21*time.Second {
		t.Fatalf("change detection timeout = %v, want 21s", got)
	}
	if changeDetection.AllowInsecure {
		t.Fatal("change detection explicit allow-insecure false was overwritten")
	}
	if got := changeDetection.Headers["X-Global"]; got != "global" {
		t.Fatalf("change detection global header = %q, want global", got)
	}
	if got := changeDetection.Headers["X-Type"]; got != "change-detection" {
		t.Fatalf("change detection type header = %q, want change-detection", got)
	}
	if got := changeDetection.Headers["X-Override"]; got != "instance" {
		t.Fatalf("change detection instance header override = %q, want instance", got)
	}

	serverStats := widgets[1].(*serverStatsWidget)
	if got := time.Duration(serverStats.Servers[0].Timeout); got != 13*time.Second {
		t.Fatalf("server stats inherited timeout = %v, want 13s", got)
	}
	if !serverStats.Servers[0].AllowInsecure {
		t.Fatal("server stats did not inherit allow-insecure")
	}
	if got := time.Duration(serverStats.Servers[1].Timeout); got != 22*time.Second {
		t.Fatalf("server stats explicit timeout = %v, want 22s", got)
	}
	if serverStats.Servers[1].AllowInsecure {
		t.Fatal("server stats explicit allow-insecure false was overwritten")
	}
}

func TestDNSStatsHTTPDefaultsHierarchy(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    timeout: 11s
    allow-insecure: true
  types:
    dns-stats:
      timeout: 12s

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: dns-stats
            service: adguard
            url: https://example.com/inherited

          - type: dns-stats
            service: adguard
            url: https://example.com/explicit
            timeout: 21s
            allow-insecure: false
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	widgets := config.Pages[0].Columns[0].Widgets

	inherited := widgets[0].(*dnsStatsWidget)
	if got := time.Duration(inherited.Timeout); got != 12*time.Second {
		t.Fatalf("inherited DNS timeout = %v, want 12s", got)
	}
	if !inherited.AllowInsecure {
		t.Fatal("DNS widget did not inherit global allow-insecure")
	}

	explicit := widgets[1].(*dnsStatsWidget)
	if got := time.Duration(explicit.Timeout); got != 21*time.Second {
		t.Fatalf("explicit DNS timeout = %v, want 21s", got)
	}
	if explicit.AllowInsecure {
		t.Fatal("explicit DNS allow-insecure false was overwritten")
	}
}

func TestEveryRegisteredWidgetSupportsCommonCapabilities(t *testing.T) {
	capabilities := []widgetCapability{
		widgetCapabilityTitle,
		widgetCapabilityTitleURL,
		widgetCapabilityHideHeader,
		widgetCapabilityCSSClass,
		widgetCapabilityCache,
		widgetCapabilityNewTab,
	}

	for widgetType := range registeredWidgetTypes {
		for _, capability := range capabilities {
			if !widgetSupportsCapability(
				widgetType,
				capability,
				widgetCapabilityScopeGlobal,
			) {
				t.Errorf(
					"%s does not support common global capability %s",
					widgetType,
					capability,
				)
			}

			if !widgetSupportsCapability(
				widgetType,
				capability,
				widgetCapabilityScopeType,
			) {
				t.Errorf(
					"%s does not support common type capability %s",
					widgetType,
					capability,
				)
			}
		}
	}
}

func TestConfigWithoutWidgetDefaultsPreservesExistingWidgetConfiguration(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`
pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: rss
            title: Existing RSS
            cache: 17m
            limit: 9
            collapse-after: 4
            feeds:
              - url: https://example.com/feed.xml
                title: Existing Feed
                headers:
                  X-Existing: preserved

          - type: monitor
            title: Existing Monitor
            cache: 6m
            sites:
              - title: Existing Site
                url: https://example.com
                timeout: 8s
                allow-insecure: true
                same-tab: true

          - type: videos
            title: Existing Videos
            limit: 11
            collapse-after: 3
            collapse-after-rows: 2
            channels:
              - existing-channel
`))
	if err != nil {
		t.Fatalf("newConfigFromYAML: %v", err)
	}

	widgets := config.Pages[0].Columns[0].Widgets

	rss := widgets[0].(*rssWidget)
	if rss.Title != "Existing RSS" {
		t.Fatalf("RSS title = %q, want Existing RSS", rss.Title)
	}
	if rss.Limit != 9 || rss.CollapseAfter != 4 {
		t.Fatalf(
			"RSS list configuration = limit %d collapse %d, want 9/4",
			rss.Limit,
			rss.CollapseAfter,
		)
	}
	if rss.cacheDuration != 17*time.Minute {
		t.Fatalf("RSS cache = %v, want 17m", rss.cacheDuration)
	}
	if len(rss.FeedRequests) != 1 {
		t.Fatalf("RSS feed count = %d, want 1", len(rss.FeedRequests))
	}
	if got := rss.FeedRequests[0].Headers["X-Existing"]; got != "preserved" {
		t.Fatalf("RSS existing header = %q, want preserved", got)
	}

	monitor := widgets[1].(*monitorWidget)
	if monitor.Title != "Existing Monitor" {
		t.Fatalf("monitor title = %q, want Existing Monitor", monitor.Title)
	}
	if monitor.cacheDuration != 6*time.Minute {
		t.Fatalf("monitor cache = %v, want 6m", monitor.cacheDuration)
	}
	if len(monitor.Sites) != 1 {
		t.Fatalf("monitor site count = %d, want 1", len(monitor.Sites))
	}
	if got := time.Duration(monitor.Sites[0].Timeout); got != 8*time.Second {
		t.Fatalf("monitor timeout = %v, want 8s", got)
	}
	if !monitor.Sites[0].AllowInsecure {
		t.Fatal("monitor allow-insecure configuration was not preserved")
	}
	if !monitor.Sites[0].SameTab {
		t.Fatal("monitor same-tab configuration was not preserved")
	}

	videos := widgets[2].(*videosWidget)
	if videos.Title != "Existing Videos" {
		t.Fatalf("videos title = %q, want Existing Videos", videos.Title)
	}
	if videos.Limit != 11 ||
		videos.CollapseAfter != 3 ||
		videos.CollapseAfterRows != 2 {
		t.Fatalf(
			"videos configuration = limit %d collapse %d rows %d, want 11/3/2",
			videos.Limit,
			videos.CollapseAfter,
			videos.CollapseAfterRows,
		)
	}
	if len(videos.Channels) != 1 || videos.Channels[0] != "existing-channel" {
		t.Fatalf("videos channels = %#v, want existing-channel", videos.Channels)
	}
}

func TestWidgetDefaultsLoggingIsUsefulDeterministicAndSafe(t *testing.T) {
	t.Run("no defaults produces no defaults logging", func(t *testing.T) {
		var logOutput strings.Builder

		previousLogger := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
		t.Cleanup(func() {
			slog.SetDefault(previousLogger)
		})

		_, err := newConfigFromYAML([]byte(`
pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: rss
            feeds:
              - url: https://example.com/feed.xml
`))
		if err != nil {
			t.Fatalf("config failed: %v", err)
		}

		if strings.Contains(logOutput.String(), "Widget defaults") {
			t.Fatalf("config without widget-defaults produced defaults logs:\n%s", logOutput.String())
		}
	})

	t.Run("configured defaults log policy overrides and redact sensitive values", func(t *testing.T) {
		var logOutput strings.Builder

		previousLogger := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
		t.Cleanup(func() {
			slog.SetDefault(previousLogger)
		})

		_, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    new-tab: false
    cache: 30m
  types:
    videos:
      limit: 12
    rss:
      cache: 15m
      timeout: 8s
      headers:
        X-Zeta: secret-header-zeta
        Authorization: secret-header-authorization
        X-Alpha: secret-header-alpha
      basic-auth:
        username: secret-username
        password: secret-password

pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: rss
            cache: 17m
            feeds:
              - url: https://example.com/feed.xml

          - type: videos
            limit: 7
            channels:
              - existing-channel
`))
		if err != nil {
			t.Fatalf("config failed: %v", err)
		}

		logged := logOutput.String()

		for _, expected := range []string{
			"Widget defaults configured",
			"global_capabilities=2",
			"types=2",
			"widgets=2",
			"overrides=2",
			"Widget defaults global",
			"new_tab=false",
			"cache=30m0s",
			"msg=\"Widget defaults type\" type=rss",
			"cache=15m0s",
			"timeout=8s",
			"headers=\"[Authorization X-Alpha X-Zeta]\"",
			"basic_auth=configured",
			"msg=\"Widget defaults type\" type=videos",
			"limit=12",
			"msg=\"Widget defaults overrides\" type=rss overrides=1 fields=[cache]",
			"msg=\"Widget defaults overrides\" type=videos overrides=1 fields=[limit]",
		} {
			if !strings.Contains(logged, expected) {
				t.Fatalf("expected log fragment %q missing:\n%s", expected, logged)
			}
		}

		rssPosition := strings.Index(logged, "msg=\"Widget defaults type\" type=rss")
		videosPosition := strings.Index(logged, "msg=\"Widget defaults type\" type=videos")
		if rssPosition == -1 || videosPosition == -1 || rssPosition >= videosPosition {
			t.Fatalf("type defaults were not logged deterministically:\n%s", logged)
		}

		for _, secret := range []string{
			"secret-header-zeta",
			"secret-header-authorization",
			"secret-header-alpha",
			"secret-username",
			"secret-password",
		} {
			if strings.Contains(logged, secret) {
				t.Fatalf("sensitive value %q leaked into logs:\n%s", secret, logged)
			}
		}
	})
}
