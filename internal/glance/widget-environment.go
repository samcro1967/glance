package glance

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"
)

var environmentWidgetTemplate = mustParseTemplate("environment.html", "widget-base.html")
var environmentWidgetExpandedTemplate = mustParseTemplate("environment-expanded.html")

type environmentWidget struct {
	widgetBase   `yaml:",inline"`
	Location     string `yaml:"location"`
	PollenAPIKey string `yaml:"pollen-api-key"`

	Place           *openMeteoPlaceResponseJson `yaml:"-"`
	AirQuality      *environmentAirQuality      `yaml:"-"`
	Pollen          *environmentPollen          `yaml:"-"`
	AirQualityError error                       `yaml:"-"`
	PollenError     error                       `yaml:"-"`
}

func (widget *environmentWidget) initialize() error {
	widget.withTitle("Environment").withCacheOnTheHour()
	if widget.Location == "" {
		return errors.New("location is required")
	}
	return nil
}

func (widget *environmentWidget) update(ctx context.Context) {
	if widget.Place == nil {
		place, err := fetchOpenMeteoPlaceResource(ctx, widget.Location)
		if !widget.canContinueUpdateAfterHandlingErr(err) {
			return
		}
		widget.Place = place
	}

	var providerErrors []error

	air, airErr := fetchOpenMeteoEnvironmentResource(ctx, widget.Place)
	if airErr == nil {
		widget.AirQuality = air
	} else {
		providerErrors = append(providerErrors, fmt.Errorf("air quality: %w", airErr))
	}
	widget.AirQualityError = airErr

	widget.PollenError = nil
	if widget.PollenAPIKey != "" {
		pollen, pollenErr := fetchAtmoSporePollenResource(ctx, widget.Place, widget.PollenAPIKey)
		if pollenErr == nil {
			widget.Pollen = pollen
		} else {
			providerErrors = append(providerErrors, fmt.Errorf("pollen: %w", pollenErr))
		}
		widget.PollenError = pollenErr
	}

	if len(providerErrors) == 0 {
		widget.canContinueUpdateAfterHandlingErr(nil)
		return
	}

	providerErr := errors.Join(providerErrors...)
	if widget.AirQuality == nil && widget.Pollen == nil {
		widget.canContinueUpdateAfterHandlingErr(fmt.Errorf("environment providers unavailable: %w", providerErr))
		return
	}

	widget.canContinueUpdateAfterHandlingErr(errors.Join(errPartialContent, providerErr))
}

func (widget *environmentWidget) Render() template.HTML {
	return widget.renderTemplate(widget, environmentWidgetTemplate)
}
func (widget *environmentWidget) RenderExpanded() template.HTML {
	return widget.renderTemplate(widget, environmentWidgetExpandedTemplate)
}

func (widget *environmentWidget) HasPollenProvider() bool { return widget.PollenAPIKey != "" }

type environmentAirQuality struct {
	USAQI           float64
	EuropeanAQI     float64
	PM25            float64
	PM10            float64
	Ozone           float64
	NitrogenDioxide float64
	SulphurDioxide  float64
	CarbonMonoxide  float64
	UVIndex         float64
	Forecast        []environmentAirQualityDay
}

type environmentAirQualityDay struct {
	Date       string
	USAQI      float64
	UVIndex    float64
	HasUSAQI   bool
	HasUVIndex bool
}

func (a *environmentAirQuality) USAQICategory() string { return usAQICategory(a.USAQI) }
func (a *environmentAirQuality) UVCategory() string    { return uvCategory(a.UVIndex) }

func (d environmentAirQualityDay) DisplayDate() string { return environmentDisplayDate(d.Date) }

func usAQICategory(v float64) string {
	switch {
	case v <= 50:
		return "Good"
	case v <= 100:
		return "Moderate"
	case v <= 150:
		return "Unhealthy for sensitive groups"
	case v <= 200:
		return "Unhealthy"
	case v <= 300:
		return "Very unhealthy"
	default:
		return "Hazardous"
	}
}
func uvCategory(v float64) string {
	switch {
	case v < 3:
		return "Low"
	case v < 6:
		return "Moderate"
	case v < 8:
		return "High"
	case v < 11:
		return "Very high"
	default:
		return "Extreme"
	}
}

type environmentPollen struct{ Days []environmentPollenDay }
type environmentPollenDay struct {
	Date        string
	OverallRisk string
	Species     []environmentPollenSpecies
}
type environmentPollenSpecies struct {
	Name      string
	Category  string
	Value     float64
	RiskLevel string
}

func (d environmentPollenDay) DisplayDate() string     { return environmentDisplayDate(d.Date) }
func (d environmentPollenDay) DisplayRisk() string     { return environmentDisplayRisk(d.OverallRisk) }
func (s environmentPollenSpecies) DisplayRisk() string { return environmentDisplayRisk(s.RiskLevel) }

func environmentDisplayDate(value string) string {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return parsed.Format("Mon 1/2")
}

func environmentDisplayRisk(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func (p *environmentPollen) Current() *environmentPollenDay {
	if p == nil || len(p.Days) == 0 {
		return nil
	}
	return &p.Days[0]
}
func (d *environmentPollenDay) ActiveSpecies() []environmentPollenSpecies {
	if d == nil {
		return nil
	}
	out := append([]environmentPollenSpecies(nil), d.Species...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		return out[i].Name < out[j].Name
	})
	n := 0
	for ; n < len(out) && out[n].Value > 0; n++ {
	}
	return out[:n]
}
