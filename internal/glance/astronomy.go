package glance

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/starainrt/astro/eclipse"
	"github.com/starainrt/astro/jupiter"
	"github.com/starainrt/astro/mars"
	"github.com/starainrt/astro/mercury"
	"github.com/starainrt/astro/moon"
	"github.com/starainrt/astro/neptune"
	"github.com/starainrt/astro/saturn"
	"github.com/starainrt/astro/star"
	"github.com/starainrt/astro/sun"
	"github.com/starainrt/astro/uranus"
	"github.com/starainrt/astro/venus"
)

const synodicMonthDays = 29.530588853

type astronomyObserver struct {
	Latitude  float64
	Longitude float64
	Timezone  *time.Location
	Label     string
}

type astronomySnapshot struct {
	At      time.Time
	Sun     astronomySun
	Moon    astronomyMoon
	Planets []astronomyPlanet
	Stars   []astronomyStar
	Events  []astronomyEvent
}

type astronomySun struct {
	Sunrise   time.Time
	Sunset    time.Time
	CivilDawn time.Time
	CivilDusk time.Time
	AstroDawn time.Time
	AstroDusk time.Time
	Altitude  float64
}

type astronomyMoon struct {
	PhaseName     string
	PhaseGlyph    string
	Illumination  int
	AgeDays       float64
	Trend         string
	Rise          time.Time
	Set           time.Time
	Altitude      float64
	NextPhaseName string
	NextPhaseTime time.Time
}

type astronomyPlanet struct {
	Name          string
	Glyph         string
	Altitude      float64
	Azimuth       float64
	Magnitude     float64
	Rise          time.Time
	Set           time.Time
	Constellation string
	Visibility    string
}

type astronomyStar struct {
	Name          string
	Magnitude     float64
	Altitude      float64
	Azimuth       float64
	Constellation string
}

type astronomyEvent struct {
	Name string
	Kind string
	Time time.Time
	Note string
}

type planetEphemeris struct {
	name      string
	glyph     string
	altitude  func(time.Time, float64, float64) float64
	azimuth   func(time.Time, float64, float64) float64
	magnitude func(time.Time) float64
	rise      func(time.Time, float64, float64, float64, bool) (time.Time, error)
	set       func(time.Time, float64, float64, float64, bool) (time.Time, error)
	raDec     func(time.Time) (float64, float64)
}

var astronomyPlanets = []planetEphemeris{
	{"Mercury", "☿", mercury.Altitude, mercury.Azimuth, mercury.ApparentMagnitude, mercury.RiseTime, mercury.SetTime, mercury.ApparentRaDec},
	{"Venus", "♀", venus.Altitude, venus.Azimuth, venus.ApparentMagnitude, venus.RiseTime, venus.SetTime, venus.ApparentRaDec},
	{"Mars", "♂", mars.Altitude, mars.Azimuth, mars.ApparentMagnitude, mars.RiseTime, mars.SetTime, mars.ApparentRaDec},
	{"Jupiter", "♃", jupiter.Altitude, jupiter.Azimuth, jupiter.ApparentMagnitude, jupiter.RiseTime, jupiter.SetTime, jupiter.ApparentRaDec},
	{"Saturn", "♄", saturn.Altitude, saturn.Azimuth, saturn.ApparentMagnitude, saturn.RiseTime, saturn.SetTime, saturn.ApparentRaDec},
	{"Uranus", "⛢", uranus.Altitude, uranus.Azimuth, uranus.ApparentMagnitude, uranus.RiseTime, uranus.SetTime, uranus.ApparentRaDec},
	{"Neptune", "♆", neptune.Altitude, neptune.Azimuth, neptune.ApparentMagnitude, neptune.RiseTime, neptune.SetTime, neptune.ApparentRaDec},
}

type meteorShowerDefinition struct {
	name      string
	peakMonth time.Month
	peakDay   int
	zhr       int
}

var majorMeteorShowers = []meteorShowerDefinition{
	{"Quadrantids", time.January, 3, 80}, {"Lyrids", time.April, 22, 18},
	{"Eta Aquariids", time.May, 6, 50}, {"Delta Aquariids", time.July, 30, 25},
	{"Perseids", time.August, 12, 100}, {"Orionids", time.October, 21, 20},
	{"Leonids", time.November, 17, 15}, {"Geminids", time.December, 14, 120},
	{"Ursids", time.December, 22, 10},
}

func buildAstronomySnapshot(at time.Time, observer astronomyObserver, includeStars bool) (astronomySnapshot, error) {
	at = at.In(observer.Timezone)
	snapshot := astronomySnapshot{At: at, Sun: buildAstronomySun(at, observer), Moon: buildAstronomyMoon(at, observer), Planets: buildAstronomyPlanets(at, observer), Events: buildAstronomyEvents(at, observer)}
	if includeStars {
		stars, err := buildAstronomyStars(at, observer)
		if err != nil {
			return astronomySnapshot{}, fmt.Errorf("loading bright-star catalog: %w", err)
		}
		snapshot.Stars = stars
	}
	return snapshot, nil
}

func buildAstronomySun(at time.Time, observer astronomyObserver) astronomySun {
	sunrise, _ := sun.RiseTime(at, observer.Longitude, observer.Latitude, 0, true)
	sunset, _ := sun.SetTime(at, observer.Longitude, observer.Latitude, 0, true)
	civilDawn, _ := sun.MorningTwilight(at, observer.Longitude, observer.Latitude, -6)
	civilDusk, _ := sun.EveningTwilight(at, observer.Longitude, observer.Latitude, -6)
	astroDawn, _ := sun.MorningTwilight(at, observer.Longitude, observer.Latitude, -18)
	astroDusk, _ := sun.EveningTwilight(at, observer.Longitude, observer.Latitude, -18)
	return astronomySun{Sunrise: sunrise, Sunset: sunset, CivilDawn: civilDawn, CivilDusk: civilDusk, AstroDawn: astroDawn, AstroDusk: astroDusk, Altitude: sun.Altitude(at, observer.Longitude, observer.Latitude)}
}

func buildAstronomyMoon(at time.Time, observer astronomyObserver) astronomyMoon {
	illumination := moon.Phase(at)
	age := normalizeDegrees(moon.SunMoonLoDiff(at)) / 360 * synodicMonthDays
	rise, _ := moon.RiseTime(at, observer.Longitude, observer.Latitude, 0, true)
	set, _ := moon.SetTime(at, observer.Longitude, observer.Latitude, 0, true)
	nextName, nextTime := nextLunarPhase(at)
	trend := "Waning"
	if age < synodicMonthDays/2 {
		trend = "Waxing"
	}
	return astronomyMoon{PhaseName: moonPhaseName(age), PhaseGlyph: moonPhaseGlyph(age), Illumination: int(math.Round(illumination * 100)), AgeDays: age, Trend: trend, Rise: rise, Set: set, Altitude: moon.Altitude(at, observer.Longitude, observer.Latitude), NextPhaseName: nextName, NextPhaseTime: nextTime}
}

func nextLunarPhase(at time.Time) (string, time.Time) {
	type candidate struct {
		name string
		at   time.Time
	}
	candidates := []candidate{{"New Moon", moon.NextNewMoon(at)}, {"First Quarter", moon.NextFirstQuarter(at)}, {"Full Moon", moon.NextFullMoon(at)}, {"Last Quarter", moon.NextLastQuarter(at)}}
	slices.SortFunc(candidates, func(a, b candidate) int {
		if a.at.Before(b.at) {
			return -1
		}
		if a.at.After(b.at) {
			return 1
		}
		return 0
	})
	return candidates[0].name, candidates[0].at
}

func moonPhaseName(age float64) string {
	switch {
	case age < 1 || age >= 28.5:
		return "New Moon"
	case age < 6.4:
		return "Waxing Crescent"
	case age < 8.4:
		return "First Quarter"
	case age < 13.8:
		return "Waxing Gibbous"
	case age < 15.8:
		return "Full Moon"
	case age < 21.1:
		return "Waning Gibbous"
	case age < 23.2:
		return "Last Quarter"
	default:
		return "Waning Crescent"
	}
}

func moonPhaseGlyph(age float64) string {
	switch moonPhaseName(age) {
	case "New Moon":
		return "●"
	case "Waxing Crescent":
		return "◔"
	case "First Quarter":
		return "◐"
	case "Waxing Gibbous":
		return "◕"
	case "Full Moon":
		return "○"
	case "Waning Gibbous":
		return "◕"
	case "Last Quarter":
		return "◑"
	default:
		return "◒"
	}
}

func buildAstronomyPlanets(at time.Time, observer astronomyObserver) []astronomyPlanet {
	sunAltitude := sun.Altitude(at, observer.Longitude, observer.Latitude)
	planets := make([]astronomyPlanet, 0, len(astronomyPlanets))
	for _, ephemeris := range astronomyPlanets {
		altitude := ephemeris.altitude(at, observer.Longitude, observer.Latitude)
		magnitude := ephemeris.magnitude(at)
		rise, _ := ephemeris.rise(at, observer.Longitude, observer.Latitude, 0, true)
		set, _ := ephemeris.set(at, observer.Longitude, observer.Latitude, 0, true)
		ra, dec := ephemeris.raDec(at)
		planets = append(planets, astronomyPlanet{Name: ephemeris.name, Glyph: ephemeris.glyph, Altitude: altitude, Azimuth: ephemeris.azimuth(at, observer.Longitude, observer.Latitude), Magnitude: magnitude, Rise: rise, Set: set, Constellation: star.ConstellationEN(ra, dec, at), Visibility: astronomyVisibility(altitude, sunAltitude, magnitude)})
	}
	slices.SortFunc(planets, func(a, b astronomyPlanet) int {
		ranks := map[string]int{"Excellent": 0, "Visible": 1, "Low": 2, "Daylight": 3, "Below horizon": 4}
		if ranks[a.Visibility] != ranks[b.Visibility] {
			return ranks[a.Visibility] - ranks[b.Visibility]
		}
		return strings.Compare(a.Name, b.Name)
	})
	return planets
}

func astronomyVisibility(altitude, sunAltitude, magnitude float64) string {
	if altitude <= 0 {
		return "Below horizon"
	}
	if sunAltitude > -6 {
		return "Daylight"
	}
	if altitude < 10 {
		return "Low"
	}
	if altitude >= 30 && magnitude <= 2.5 {
		return "Excellent"
	}
	return "Visible"
}

func buildAstronomyStars(at time.Time, observer astronomyObserver) ([]astronomyStar, error) {
	if err := star.InitStarDatabase(); err != nil {
		return nil, err
	}
	catalog, err := star.TopBrightStars()
	if err != nil {
		return nil, err
	}
	stars := make([]astronomyStar, 0, 6)
	for _, candidate := range catalog {
		ra, dec := candidate.RaDecByDate(at)
		altitude := star.Altitude(at, ra, dec, observer.Longitude, observer.Latitude)
		if altitude < 10 {
			continue
		}
		name := strings.TrimSpace(candidate.CommonName)
		if name == "" {
			name = strings.TrimSpace(candidate.Name)
		}
		if name == "" {
			continue
		}
		stars = append(stars, astronomyStar{Name: name, Magnitude: candidate.Mag, Altitude: altitude, Azimuth: star.Azimuth(at, ra, dec, observer.Longitude, observer.Latitude), Constellation: star.ConstellationEN(ra, dec, at)})
		if len(stars) == 6 {
			break
		}
	}
	return stars, nil
}

func buildAstronomyEvents(at time.Time, observer astronomyObserver) []astronomyEvent {
	nextMoonName, nextMoonTime := nextLunarPhase(at)
	events := []astronomyEvent{
		{Name: nextMoonName, Kind: "moon", Time: nextMoonTime},
		{Name: "Mercury stations retrograde", Kind: "planet", Time: mercury.NextProgradeToRetrograde(at)},
		{Name: "Jupiter opposition", Kind: "planet", Time: jupiter.NextOpposition(at)},
		{Name: "Saturn opposition", Kind: "planet", Time: saturn.NextOpposition(at)},
	}

	if showerEvents := nextMeteorShowerEvents(at, 1); len(showerEvents) == 1 {
		events = append(events, showerEvents[0])
	}

	solarEclipse := eclipse.NextLocalSolarEclipse(at, observer.Longitude, observer.Latitude, 0)
	if !solarEclipse.GreatestEclipse.IsZero() {
		events = append(events, astronomyEvent{Name: "Solar eclipse visible locally", Kind: "eclipse", Time: solarEclipse.GreatestEclipse, Note: strings.ReplaceAll(string(solarEclipse.Type), "_", " ")})
	}

	slices.SortFunc(events, func(a, b astronomyEvent) int {
		if a.Time.Before(b.Time) {
			return -1
		}
		if a.Time.After(b.Time) {
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return events
}

func nextMeteorShowerEvents(at time.Time, count int) []astronomyEvent {
	events := make([]astronomyEvent, 0, len(majorMeteorShowers)*2)
	for year := at.Year(); year <= at.Year()+1; year++ {
		for _, shower := range majorMeteorShowers {
			peak := time.Date(year, shower.peakMonth, shower.peakDay, 0, 0, 0, 0, at.Location())
			if peak.Before(at) {
				continue
			}
			events = append(events, astronomyEvent{Name: shower.name + " peak", Kind: "meteor", Time: peak, Note: fmt.Sprintf("~%d/hr", shower.zhr)})
		}
	}
	slices.SortFunc(events, func(a, b astronomyEvent) int {
		if a.Time.Before(b.Time) {
			return -1
		}
		if a.Time.After(b.Time) {
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	if len(events) > count {
		events = events[:count]
	}
	return events
}

func normalizeDegrees(value float64) float64 {
	value = math.Mod(value, 360)
	if value < 0 {
		value += 360
	}
	return value
}
func formatAstronomyTime(value time.Time, hourFormat string) string {
	if value.IsZero() {
		return "—"
	}
	if hourFormat == "24h" {
		return value.Format("15:04")
	}
	return value.Format("3:04 PM")
}
func formatAstronomyEventTime(value time.Time, hourFormat string) string {
	if value.IsZero() {
		return "—"
	}
	if hourFormat == "24h" {
		return value.Format("Jan 2 · 15:04")
	}
	return value.Format("Jan 2 · 3:04 PM")
}

func validateAstronomyCoordinates(latitude, longitude *float64, timezone string) error {
	hasLat, hasLon, hasTZ := latitude != nil, longitude != nil, timezone != ""
	if !hasLat && !hasLon && !hasTZ {
		return nil
	}
	if !hasLat || !hasLon || !hasTZ {
		return errors.New("latitude, longitude, and timezone must be configured together")
	}
	if *latitude < -90 || *latitude > 90 {
		return errors.New("latitude must be between -90 and 90")
	}
	if *longitude < -180 || *longitude > 180 {
		return errors.New("longitude must be between -180 and 180")
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return fmt.Errorf("loading timezone: %w", err)
	}
	return nil
}
