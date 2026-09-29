package glance

import (
	"testing"
	"time"
)

func TestAstrologyWidgetInitialize(t *testing.T) {
	widget := &astrologyWidget{Timezone: "America/Chicago", At: "2026-09-29T20:00:00-05:00"}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize returned error: %v", err)
	}
	if widget.Title != "Astrology" {
		t.Fatalf("unexpected title: %q", widget.Title)
	}
	if widget.cacheDuration != 30*time.Minute {
		t.Fatalf("unexpected cache: %s", widget.cacheDuration)
	}
	if !widget.ShowSunMoon || !widget.ShowPlanets || !widget.ShowAspects || !widget.ShowEvents {
		t.Fatal("expected all sections enabled by default")
	}
}

func TestAstrologyWidgetSections(t *testing.T) {
	widget := &astrologyWidget{
		Timezone: "America/Chicago",
		Sections: []string{"sun-moon", "aspects"},
	}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize returned error: %v", err)
	}
	if !widget.ShowSunMoon || !widget.ShowAspects {
		t.Fatal("expected configured sections to be enabled")
	}
	if widget.ShowPlanets || widget.ShowEvents {
		t.Fatal("expected unconfigured sections to remain disabled")
	}

	invalid := &astrologyWidget{
		Timezone: "America/Chicago",
		Sections: []string{"unknown"},
	}
	if err := invalid.initialize(); err == nil {
		t.Fatal("expected unknown section to fail validation")
	}
}

func TestZodiacPosition(t *testing.T) {
	tests := []struct {
		longitude float64
		sign      string
		degree    int
	}{{0, "Aries", 0}, {29.5, "Aries", 29}, {30, "Taurus", 0}, {180, "Libra", 0}, {359.5, "Pisces", 29}}
	for _, test := range tests {
		sign, _, degree, _ := zodiacPosition(test.longitude)
		if sign != test.sign || degree != test.degree {
			t.Errorf("%.1f: got %s %d", test.longitude, sign, degree)
		}
	}
}

func TestAngularSeparation(t *testing.T) {
	if got := angularSeparation(350, 10); got != 20 {
		t.Fatalf("got %.1f", got)
	}
	if got := angularSeparation(20, 200); got != 180 {
		t.Fatalf("got %.1f", got)
	}
}

func TestBuildAstrologyAspects(t *testing.T) {
	bodies := []astrologyBody{{Name: "Sun", Glyph: "☉", Longitude: 0}, {Name: "Moon", Glyph: "☽", Longitude: 120}, {Name: "Mars", Glyph: "♂", Longitude: 91}}
	aspects := buildAstrologyAspects(bodies)
	if len(aspects) != 2 {
		t.Fatalf("got %d aspects: %#v", len(aspects), aspects)
	}
	if aspects[0].Name != "Trine" || aspects[1].Name != "Square" {
		t.Fatalf("unexpected aspects: %#v", aspects)
	}
}

func TestNextZodiacIngress(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	longitude := func(value time.Time) float64 { return value.Sub(start).Hours() * 2 }
	when, sign, ok := nextZodiacIngress(start, longitude, 24*time.Hour, time.Hour)
	if !ok || sign != "Taurus" {
		t.Fatalf("got %v %q %v", when, sign, ok)
	}
	if when.Sub(start) < 14*time.Hour || when.Sub(start) > 16*time.Hour {
		t.Fatalf("unexpected ingress: %s", when)
	}
}

func TestAstrologyWidgetRegistered(t *testing.T) {
	descriptor, ok := widgetRegistry["astrology"]
	if !ok {
		t.Fatal("astrology widget is not registered")
	}
	if _, ok := descriptor.constructor().(*astrologyWidget); !ok {
		t.Fatalf("constructor returned %T", descriptor.constructor())
	}
}
