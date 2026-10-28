package glance

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReloadBindingDuringStaticRendering(t *testing.T) {
	a := newGlanceTestApplication(t, `server:
  resource-proxy:
    allowed-origins: [http://media.invalid]
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: calculator
            icon: http://media.invalid/icon.png
`)
	w := a.refreshWidgets[0]
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 100 {
			renderWidget(w)
		}
	}()
	for range 100 {
		rebindReloadProviders(w, a.widgetProviders)
	}
	wg.Wait()
	if !strings.Contains(string(renderWidget(w)), "/api/resource-proxy/") {
		t.Fatal("rebinding lost proxied icon")
	}
}

func TestStaticImagesUseProxyInRenderedHTML(t *testing.T) {
	a := newGlanceTestApplication(t, `server:
  base-url: /glance
  resource-proxy:
    allowed-origins: [http://media.invalid]
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: bookmarks
            icon: http://media.invalid/header.png?token=PRIVATE_HEADER
            groups:
              - links:
                  - title: Media
                    url: https://media.invalid
                    icon: http://media.invalid/icon.png?token=PRIVATE_IMAGE
          - type: calculator
            icon: http://media.invalid/calculator.png?token=PRIVATE_CALCULATOR
`)
	for _, w := range a.Config.Pages[0].Columns[0].Widgets {
		output := string(w.Render())
		if strings.Contains(output, "PRIVATE_") || strings.Contains(output, "http://media.invalid") {
			t.Fatalf("static output exposes upstream URL: %s", output)
		}
		if !strings.Contains(output, "/glance/api/resource-proxy/") {
			t.Fatal("static output missing proxy binding")
		}
	}
}

func TestReloadKeepsOpaqueMediaImagesAndRevalidatesPolicy(t *testing.T) {
	const yaml = `server:
  resource-proxy:
    allowed-origins: [http://media.invalid]
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: latest-media
            service: plex
            server: http://media.invalid
            api-key: PRIVATE_TOKEN
`
	previous := newGlanceTestApplication(t, yaml)
	old := previous.refreshWidgets[0].(*latestMediaWidget)
	image, err := previous.resolveResourceProxyURL("http://media.invalid/poster.jpg?token=PRIVATE_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	old.Items = []mediaItem{{Title: "Media", ImageURL: image}}
	old.ContentAvailable = true
	old.setNextUpdateTime(time.Now().Add(time.Hour))
	c, err := newConfigFromYAML([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := newApplicationWithOIDCRuntime(c, nil, previous.resourceProxy)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := prepareWidgetReloadReusePlan(previous, candidate)
	if err != nil {
		t.Fatal(err)
	}
	candidate.applyWidgetReloadReusePlan(plan)
	if candidate.refreshWidgets[0] != old {
		t.Fatal("unchanged media not reused")
	}
	if _, exists := candidate.resourceProxy.lookup(strings.TrimPrefix(image, "/api/resource-proxy/")); !exists {
		t.Fatal("reused artwork URL lost ownership")
	}
	c, err = newConfigFromYAML([]byte(strings.ReplaceAll(yaml, "[http://media.invalid]", "[http://other.invalid]")))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := newApplicationWithOIDCRuntime(c, nil, previous.resourceProxy)
	if err != nil {
		t.Fatal(err)
	}
	if changed.resourceProxy == previous.resourceProxy {
		t.Fatal("changed policy retained old registry")
	}
	if _, exists := changed.resourceProxy.lookup(strings.TrimPrefix(image, "/api/resource-proxy/")); exists {
		t.Fatal("removed origin retains old image ID")
	}
}
