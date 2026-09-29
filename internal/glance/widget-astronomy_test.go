package glance

import (
	"testing"
	"time"
)

func TestAstronomyWidgetInitialize(t *testing.T) {
	lat, lon := 38.627, -90.1994
	widget := &astronomyWidget{Latitude: &lat, Longitude: &lon, Timezone: "America/Chicago"}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize returned error: %v", err)
	}
	if widget.Title != "Astronomy" {
		t.Fatalf("unexpected title: %q", widget.Title)
	}
	if widget.cacheDuration != 15*time.Minute {
		t.Fatalf("unexpected cache: %s", widget.cacheDuration)
	}
	if !widget.ShowSun || !widget.ShowMoon || !widget.ShowPlanets || !widget.ShowStars || !widget.ShowEvents {
		t.Fatal("expected all sections enabled by default")
	}
}

func TestAstronomyWidgetSections(t *testing.T) {
	lat, lon := 38.627, -90.1994
	widget := &astronomyWidget{
		Latitude:  &lat,
		Longitude: &lon,
		Timezone:  "America/Chicago",
		Sections:  []string{"moon", "sun"},
	}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize returned error: %v", err)
	}
	if !widget.ShowMoon || !widget.ShowSun {
		t.Fatal("expected configured moon and sun sections to be enabled")
	}
	if widget.ShowPlanets || widget.ShowStars || widget.ShowEvents {
		t.Fatal("expected unconfigured sections to remain disabled")
	}

	invalid := &astronomyWidget{
		Latitude:  &lat,
		Longitude: &lon,
		Timezone:  "America/Chicago",
		Sections:  []string{"moon", "unknown"},
	}
	if err := invalid.initialize(); err == nil {
		t.Fatal("expected unknown section to fail validation")
	}
}

func TestAstronomyWidgetLocationValidation(t *testing.T) {
	lat, lon := 38.627, -90.1994
	tests := []astronomyWidget{
		{},
		{Location: "St. Louis", Latitude: &lat, Longitude: &lon, Timezone: "America/Chicago"},
		{Latitude: &lat},
	}
	for i := range tests {
		if err := tests[i].initialize(); err == nil {
			t.Fatalf("case %d expected validation error", i)
		}
	}
}

func TestMoonPhaseNames(t *testing.T) {
	tests := []struct {
		age  float64
		want string
	}{{0, "New Moon"}, {3, "Waxing Crescent"}, {7, "First Quarter"}, {11, "Waxing Gibbous"}, {14.7, "Full Moon"}, {18, "Waning Gibbous"}, {22, "Last Quarter"}, {26, "Waning Crescent"}}
	for _, test := range tests {
		if got := moonPhaseName(test.age); got != test.want {
			t.Errorf("age %.1f: got %q want %q", test.age, got, test.want)
		}
	}
}

func TestAstronomyVisibility(t *testing.T) {
	if got := astronomyVisibility(-1, -20, -2); got != "Below horizon" {
		t.Fatalf("got %q", got)
	}
	if got := astronomyVisibility(40, 10, -2); got != "Daylight" {
		t.Fatalf("got %q", got)
	}
	if got := astronomyVisibility(5, -20, -2); got != "Low" {
		t.Fatalf("got %q", got)
	}
	if got := astronomyVisibility(40, -20, 1); got != "Excellent" {
		t.Fatalf("got %q", got)
	}
}

func TestNextMeteorShowerEvents(t *testing.T) {
	at := time.Date(2026, time.September, 29, 12, 0, 0, 0, time.UTC)
	events := nextMeteorShowerEvents(at, 2)
	if len(events) != 2 {
		t.Fatalf("got %d events", len(events))
	}
	if events[0].Name != "Orionids peak" || events[1].Name != "Leonids peak" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestAstronomyWidgetRegistered(t *testing.T) {
	descriptor, ok := widgetRegistry["astronomy"]
	if !ok {
		t.Fatal("astronomy widget is not registered")
	}
	if _, ok := descriptor.constructor().(*astronomyWidget); !ok {
		t.Fatalf("constructor returned %T", descriptor.constructor())
	}
}
