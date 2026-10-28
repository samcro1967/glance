package glance

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"
)

var environmentWidgetTemplate = mustParseTemplate("environment.html", "widget-base.html", "environment-allergens.html")
var environmentWidgetExpandedTemplate = mustParseTemplate("environment-expanded.html", "environment-allergens.html")

type environmentWidget struct {
	widgetBase     `yaml:",inline"`
	Timeout        durationField `yaml:"timeout"`
	Location       string        `yaml:"location"`
	PollenAPIKey   string        `yaml:"pollen-api-key"`
	PollenProvider string        `yaml:"pollen-provider"`

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
	widget.PollenProvider = strings.ToLower(strings.TrimSpace(widget.PollenProvider))
	if widget.PollenProvider == "" {
		widget.PollenProvider = "atmospore"
	}
	if widget.PollenProvider != "atmospore" && widget.PollenProvider != "st-louis-county" {
		return fmt.Errorf("unknown pollen provider %q; valid providers are atmospore, st-louis-county", widget.PollenProvider)
	}
	return nil
}

func (widget *environmentWidget) update(ctx context.Context) {
	if widget.Place == nil {
		place, err := fetchOpenMeteoPlaceResource(ctx, widget.Location, widget.Timeout)
		if !widget.canContinueUpdateAfterHandlingErr(err) {
			return
		}
		widget.Place = place
	}

	var providerErrors []error

	air, airErr := fetchOpenMeteoEnvironmentResource(ctx, widget.Place, widget.Timeout)
	if airErr == nil {
		widget.AirQuality = air
	} else {
		providerErrors = append(providerErrors, fmt.Errorf("air quality: %w", airErr))
	}
	widget.AirQualityError = airErr

	widget.PollenError = nil
	if widget.HasPollenProvider() {
		var pollen *environmentPollen
		var pollenErr error
		if widget.PollenProvider == "st-louis-county" {
			pollen, pollenErr = fetchStLouisCountyPollenResource(ctx, widget.Timeout)
		} else {
			pollen, pollenErr = fetchAtmoSporePollenResource(ctx, widget.Place, widget.PollenAPIKey, widget.Timeout)
		}
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

func (widget *environmentWidget) HasPollenProvider() bool {
	return widget.PollenProvider == "st-louis-county" || widget.PollenAPIKey != ""
}

type environmentAirQuality struct {
	Unavailable     map[string]bool
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

func (a *environmentAirQuality) HasMeasurement(name string) bool { return !a.Unavailable[name] }
func (a *environmentAirQuality) DisplayValue(name string, value float64, precision int) string {
	if !a.HasMeasurement(name) {
		return "Unavailable"
	}
	return fmt.Sprintf("%.*f", precision, value)
}
func (a *environmentAirQuality) USAQICategory() string {
	if !a.HasMeasurement("us_aqi") {
		return "Unavailable"
	}
	return usAQICategory(a.USAQI)
}
func (a *environmentAirQuality) UVCategory() string {
	if !a.HasMeasurement("uv_index") {
		return "Unavailable"
	}
	return uvCategory(a.UVIndex)
}

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

type environmentPollen struct {
	Days     []environmentPollenDay
	Provider string
	Timezone string
}
type environmentPollenDay struct {
	Date        string
	OverallRisk string
	Species     []environmentPollenSpecies
	Categories  []environmentPollenSpecies
}
type environmentPollenSpecies struct {
	Name        string
	Category    string
	Value       float64
	RiskLevel   string
	Unavailable bool
}

func (d environmentPollenDay) DisplayDate() string { return environmentDisplayDate(d.Date) }
func (d environmentPollenDay) DisplayRisk() string { return environmentDisplayRisk(d.OverallRisk) }
func (s environmentPollenSpecies) DisplayRisk() string {
	if environmentRiskRank(s.RiskLevel) < 0 {
		return "Unclassified"
	}
	return environmentDisplayRisk(s.RiskLevel)
}

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
	return p.currentAt(time.Now())
}
func (d *environmentPollenDay) ActiveSpecies() []environmentPollenSpecies {
	if d == nil {
		return nil
	}
	out := []environmentPollenSpecies{}
	for _, species := range d.AllSpecies() {
		if !species.Unavailable && species.Value > 0 {
			out = append(out, species)
		}
	}
	return out
}

func (widget *environmentWidget) setDefaultTimeout(value durationField) {
	widget.Timeout = value
}
