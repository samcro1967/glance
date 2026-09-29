package glance

import (
	"math"
	"slices"
	"strings"
	"time"

	"github.com/starainrt/astro/jupiter"
	"github.com/starainrt/astro/mars"
	"github.com/starainrt/astro/mercury"
	"github.com/starainrt/astro/moon"
	"github.com/starainrt/astro/neptune"
	"github.com/starainrt/astro/saturn"
	"github.com/starainrt/astro/sun"
	"github.com/starainrt/astro/uranus"
	"github.com/starainrt/astro/venus"
)

type astrologySnapshot struct {
	At      time.Time
	Bodies  []astrologyBody
	Aspects []astrologyAspect
	Events  []astrologyEvent
}

type astrologyBody struct {
	Name       string
	Glyph      string
	Sign       string
	SignGlyph  string
	Degree     int
	Minute     int
	Longitude  float64
	Retrograde bool
}

type astrologyAspect struct {
	LeftName   string
	LeftGlyph  string
	RightName  string
	RightGlyph string
	Name       string
	Glyph      string
	Orb        float64
}

type astrologyEvent struct {
	Name string
	Time time.Time
}

type astrologyEphemeris struct {
	name       string
	glyph      string
	longitude  func(time.Time) float64
	retrograde func(time.Time) bool
}

var zodiacSigns = []struct{ name, glyph string }{
	{"Aries", "♈"}, {"Taurus", "♉"}, {"Gemini", "♊"}, {"Cancer", "♋"}, {"Leo", "♌"}, {"Virgo", "♍"},
	{"Libra", "♎"}, {"Scorpio", "♏"}, {"Sagittarius", "♐"}, {"Capricorn", "♑"}, {"Aquarius", "♒"}, {"Pisces", "♓"},
}

var astrologyBodies = []astrologyEphemeris{
	{"Sun", "☉", sun.ApparentLo, nil},
	{"Moon", "☽", moon.ApparentLo, nil},
	{"Mercury", "☿", mercury.ApparentLo, func(at time.Time) bool {
		return isRetrograde(at, mercury.LastProgradeToRetrograde, mercury.LastRetrogradeToPrograde)
	}},
	{"Venus", "♀", venus.ApparentLo, func(at time.Time) bool {
		return isRetrograde(at, venus.LastProgradeToRetrograde, venus.LastRetrogradeToPrograde)
	}},
	{"Mars", "♂", mars.ApparentLo, func(at time.Time) bool {
		return isRetrograde(at, mars.LastProgradeToRetrograde, mars.LastRetrogradeToPrograde)
	}},
	{"Jupiter", "♃", jupiter.ApparentLo, func(at time.Time) bool {
		return isRetrograde(at, jupiter.LastProgradeToRetrograde, jupiter.LastRetrogradeToPrograde)
	}},
	{"Saturn", "♄", saturn.ApparentLo, func(at time.Time) bool {
		return isRetrograde(at, saturn.LastProgradeToRetrograde, saturn.LastRetrogradeToPrograde)
	}},
	{"Uranus", "⛢", uranus.ApparentLo, func(at time.Time) bool {
		return isRetrograde(at, uranus.LastProgradeToRetrograde, uranus.LastRetrogradeToPrograde)
	}},
	{"Neptune", "♆", neptune.ApparentLo, func(at time.Time) bool {
		return isRetrograde(at, neptune.LastProgradeToRetrograde, neptune.LastRetrogradeToPrograde)
	}},
}

func buildAstrologySnapshot(at time.Time) astrologySnapshot {
	bodies := make([]astrologyBody, 0, len(astrologyBodies))
	for _, ephemeris := range astrologyBodies {
		longitude := normalizeDegrees(ephemeris.longitude(at))
		sign, signGlyph, degree, minute := zodiacPosition(longitude)
		retrograde := false
		if ephemeris.retrograde != nil {
			retrograde = ephemeris.retrograde(at)
		}
		bodies = append(bodies, astrologyBody{Name: ephemeris.name, Glyph: ephemeris.glyph, Sign: sign, SignGlyph: signGlyph, Degree: degree, Minute: minute, Longitude: longitude, Retrograde: retrograde})
	}
	return astrologySnapshot{At: at, Bodies: bodies, Aspects: buildAstrologyAspects(bodies), Events: buildAstrologyEvents(at)}
}

func zodiacPosition(longitude float64) (string, string, int, int) {
	longitude = normalizeDegrees(longitude)
	index := int(longitude/30) % 12
	within := longitude - float64(index*30)
	degree := int(math.Floor(within))
	minute := int(math.Round((within - float64(degree)) * 60))
	if minute == 60 {
		degree++
		minute = 0
	}
	if degree == 30 {
		degree = 0
		index = (index + 1) % 12
	}
	return zodiacSigns[index].name, zodiacSigns[index].glyph, degree, minute
}

func isRetrograde(at time.Time, lastProgradeToRetrograde, lastRetrogradeToPrograde func(time.Time) time.Time) bool {
	return lastProgradeToRetrograde(at).After(lastRetrogradeToPrograde(at))
}

type aspectDefinition struct {
	name, glyph string
	angle, orb  float64
}

var majorAspects = []aspectDefinition{{"Conjunction", "☌", 0, 6}, {"Sextile", "⚹", 60, 4}, {"Square", "□", 90, 6}, {"Trine", "△", 120, 6}, {"Opposition", "☍", 180, 6}}

func buildAstrologyAspects(bodies []astrologyBody) []astrologyAspect {
	aspects := make([]astrologyAspect, 0)
	for i := 0; i < len(bodies); i++ {
		for j := i + 1; j < len(bodies); j++ {
			separation := angularSeparation(bodies[i].Longitude, bodies[j].Longitude)
			for _, definition := range majorAspects {
				orb := math.Abs(separation - definition.angle)
				if orb <= definition.orb {
					aspects = append(aspects, astrologyAspect{LeftName: bodies[i].Name, LeftGlyph: bodies[i].Glyph, RightName: bodies[j].Name, RightGlyph: bodies[j].Glyph, Name: definition.name, Glyph: definition.glyph, Orb: orb})
					break
				}
			}
		}
	}
	slices.SortFunc(aspects, func(a, b astrologyAspect) int {
		if a.Orb < b.Orb {
			return -1
		}
		if a.Orb > b.Orb {
			return 1
		}
		return strings.Compare(a.LeftName+a.RightName, b.LeftName+b.RightName)
	})
	if len(aspects) > 7 {
		aspects = aspects[:7]
	}
	return aspects
}

func angularSeparation(a, b float64) float64 {
	d := math.Abs(normalizeDegrees(a) - normalizeDegrees(b))
	if d > 180 {
		d = 360 - d
	}
	return d
}

func buildAstrologyEvents(at time.Time) []astrologyEvent {
	events := []astrologyEvent{}
	if t, sign, ok := nextZodiacIngress(at, moon.ApparentLo, 4*24*time.Hour, 20*time.Minute); ok {
		events = append(events, astrologyEvent{Name: "Moon enters " + sign, Time: t})
	}
	if t, sign, ok := nextZodiacIngress(at, sun.ApparentLo, 40*24*time.Hour, 2*time.Hour); ok {
		events = append(events, astrologyEvent{Name: "Sun enters " + sign, Time: t})
	}
	events = append(events, nextStationEvent(at, "Mercury", mercury.NextProgradeToRetrograde, mercury.NextRetrogradeToPrograde))
	events = append(events, nextStationEvent(at, "Venus", venus.NextProgradeToRetrograde, venus.NextRetrogradeToPrograde))
	events = append(events, nextStationEvent(at, "Mars", mars.NextProgradeToRetrograde, mars.NextRetrogradeToPrograde))
	slices.SortFunc(events, func(a, b astrologyEvent) int {
		if a.Time.Before(b.Time) {
			return -1
		}
		if a.Time.After(b.Time) {
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	if len(events) > 5 {
		events = events[:5]
	}
	return events
}

func nextStationEvent(at time.Time, name string, nextRetrograde, nextDirect func(time.Time) time.Time) astrologyEvent {
	retro := nextRetrograde(at)
	direct := nextDirect(at)
	if retro.Before(direct) {
		return astrologyEvent{Name: name + " stations retrograde", Time: retro}
	}
	return astrologyEvent{Name: name + " stations direct", Time: direct}
}

func nextZodiacIngress(start time.Time, longitude func(time.Time) float64, maxDuration, step time.Duration) (time.Time, string, bool) {
	startIndex := int(normalizeDegrees(longitude(start))/30) % 12
	previous := start
	for probe := start.Add(step); !probe.After(start.Add(maxDuration)); probe = probe.Add(step) {
		index := int(normalizeDegrees(longitude(probe))/30) % 12
		if index != startIndex {
			low, high := previous, probe
			for high.Sub(low) > time.Minute {
				mid := low.Add(high.Sub(low) / 2)
				if int(normalizeDegrees(longitude(mid))/30)%12 == startIndex {
					low = mid
				} else {
					high = mid
				}
			}
			return high, zodiacSigns[index].name, true
		}
		previous = probe
	}
	return time.Time{}, "", false
}
