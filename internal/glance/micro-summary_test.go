package glance

import "testing"

func TestExpandedMicroWidgetRegistryTypes(t *testing.T) {
	types := []string{"environment", "astronomy", "astrology", "server-stats", "dns-stats", "repository", "releases", "now-playing"}
	for _, typ := range types {
		t.Run(typ, func(t *testing.T) {
			m, err := newMicroWidget(typ)
			if err != nil {
				t.Fatal(err)
			}
			if m.GetType() != typ {
				t.Fatalf("type = %q, want %q", m.GetType(), typ)
			}
			d := microWidgetRegistry[typ]
			if d.canonicalWidgetType != typ {
				t.Fatalf("canonical type = %q, want %q", d.canonicalWidgetType, typ)
			}
			if !d.applyWidgetDefaults {
				t.Fatal("expected canonical widget defaults")
			}
		})
	}
}

func TestMicroDisplayValidation(t *testing.T) {
	got, err := validateMicroDisplay(nil, []string{"a", "b"}, "a", "b", "c")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("defaults = %#v", got)
	}
	got, err = validateMicroDisplay([]string{" B ", "a", "b"}, nil, "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "b" || got[1] != "a" {
		t.Fatalf("normalized = %#v", got)
	}
	if _, err = validateMicroDisplay([]string{"nope"}, nil, "a", "b"); err == nil {
		t.Fatal("expected invalid display error")
	}
}

func TestMicroEnvironmentProjection(t *testing.T) {
	m := &microEnvironment{Display: []string{"air-quality", "uv", "pollen"}}
	m.AirQuality = &environmentAirQuality{USAQI: 42, UVIndex: 5.5}
	m.Pollen = &environmentPollen{Days: []environmentPollenDay{{OverallRisk: "high"}}}

	items := m.MicroItems(false)
	if len(items) != 1 {
		t.Fatalf("items=%d, want 1", len(items))
	}
	if items[0].Kind != "summary" {
		t.Fatalf("kind=%q, want summary", items[0].Kind)
	}
	if items[0].Line1 != "AQI 42 Good · UV 5.5 Moderate · Pollen High" {
		t.Fatalf("line1=%q", items[0].Line1)
	}
}

func TestMicroReleasesProjection(t *testing.T) {
	m := &microReleases{}
	m.Releases = appReleaseList{
		{Name: "glance", Version: "v1.2.3", NotesUrl: "https://example.test/glance"},
		{Name: "other", Version: "v4.5.6", NotesUrl: "https://example.test/other"},
	}

	items := m.MicroItems(false)
	if len(items) != 2 {
		t.Fatalf("items=%d, want 2", len(items))
	}
	if items[0].Line1 != "glance" || items[0].Line2 != "v1.2.3" || items[0].URL != "https://example.test/glance" {
		t.Fatalf("first=%#v", items[0])
	}
	if items[1].Line1 != "other" || items[1].Line2 != "v4.5.6" || items[1].URL != "https://example.test/other" {
		t.Fatalf("second=%#v", items[1])
	}
}

func TestMicroRepositoryProjection(t *testing.T) {
	m := &microRepository{Display: []string{"stars", "pull-requests", "issues"}}
	m.RequestedRepository = "samcro1967/glance"
	m.Repository = repository{
		Name:             "samcro1967/glance",
		Stars:            123,
		OpenPullRequests: 4,
		OpenIssues:       7,
	}

	items := m.MicroItems(true)
	if len(items) != 1 {
		t.Fatalf("items=%d, want 1", len(items))
	}
	if items[0].Line1 != "samcro1967/glance" {
		t.Fatalf("line1=%q", items[0].Line1)
	}
	if items[0].URL != "https://github.com/samcro1967/glance" {
		t.Fatalf("url=%q", items[0].URL)
	}
	if !items[0].OpenLinksInNewTab {
		t.Fatal("expected new-tab policy")
	}
}

func TestMicroNowPlayingProjection(t *testing.T) {
	m := &microNowPlaying{}
	m.Items = []nowPlayingItem{
		{Title: "Track One", Subtitle: "Artist One", User: "mark", State: "playing"},
		{Title: "Track Two", User: "mark", State: "paused"},
	}

	items := m.MicroItems(false)
	if len(items) != 2 {
		t.Fatalf("items=%d, want 2", len(items))
	}
	if items[0].Line1 != "▶ Track One" || items[0].Line2 != "Artist One" {
		t.Fatalf("playing=%#v", items[0])
	}
	if items[1].Line1 != "⏸ Track Two" || items[1].Line2 != "mark" {
		t.Fatalf("paused=%#v", items[1])
	}
}
