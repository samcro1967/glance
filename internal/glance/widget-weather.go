package glance

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	_ "time/tzdata"
)

var weatherWidgetTemplate = mustParseTemplate("weather.html", "widget-base.html")

type weatherWidget struct {
	widgetBase   `yaml:",inline"`
	Location     string `yaml:"location"`
	ShowAreaName bool   `yaml:"show-area-name"`
	HideLocation bool   `yaml:"hide-location"`
	HourFormat   string `yaml:"hour-format"`
	Units        string `yaml:"units"`

	ShowCurrentRaw  *bool `yaml:"show-current"`
	ShowDetailsRaw  *bool `yaml:"show-details"`
	ShowHourlyRaw   *bool `yaml:"show-hourly"`
	ShowForecastRaw *bool `yaml:"show-forecast"`

	ShowCurrent  bool `yaml:"-"`
	ShowDetails  bool `yaml:"-"`
	ShowHourly   bool `yaml:"-"`
	ShowForecast bool `yaml:"-"`

	Place      *openMeteoPlaceResponseJson `yaml:"-"`
	Weather    *weather                    `yaml:"-"`
	TimeLabels [12]string                  `yaml:"-"`
}

var timeLabels12h = [12]string{"2am", "4am", "6am", "8am", "10am", "12pm", "2pm", "4pm", "6pm", "8pm", "10pm", "12am"}
var timeLabels24h = [12]string{"02:00", "04:00", "06:00", "08:00", "10:00", "12:00", "14:00", "16:00", "18:00", "20:00", "22:00", "00:00"}

func boolDefaultTrue(value *bool) bool {
	return value == nil || *value
}

func (widget *weatherWidget) initialize() error {
	widget.withTitle("Weather").withCacheOnTheHour()

	if widget.Location == "" {
		return fmt.Errorf("location is required")
	}

	if widget.HourFormat == "" || widget.HourFormat == "12h" {
		widget.TimeLabels = timeLabels12h
	} else if widget.HourFormat == "24h" {
		widget.TimeLabels = timeLabels24h
	} else {
		return errors.New("hour-format must be either 12h or 24h")
	}

	if widget.Units == "" {
		widget.Units = "metric"
	} else if widget.Units != "metric" && widget.Units != "imperial" {
		return errors.New("units must be either metric or imperial")
	}

	widget.ShowCurrent = boolDefaultTrue(widget.ShowCurrentRaw)
	widget.ShowDetails = boolDefaultTrue(widget.ShowDetailsRaw)
	widget.ShowHourly = boolDefaultTrue(widget.ShowHourlyRaw)
	widget.ShowForecast = boolDefaultTrue(widget.ShowForecastRaw)

	return nil
}

func (widget *weatherWidget) update(ctx context.Context) {
	if widget.Place == nil {
		place, err := fetchOpenMeteoPlaceResource(ctx, widget.Location)
		if !widget.canContinueUpdateAfterHandlingErr(err) {
			return
		}

		widget.Place = place
	}

	weather, err := fetchOpenMeteoWeatherResource(ctx, widget.Place, widget.Units)

	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	widget.Weather = weather
}

func (widget *weatherWidget) Render() template.HTML {
	return widget.renderTemplate(widget, weatherWidgetTemplate)
}

type weather struct {
	Temperature         int
	ApparentTemperature int
	WeatherCode         int

	Details  weatherDetails
	Forecast []weatherForecastDay

	CurrentColumn int
	SunriseColumn int
	SunsetColumn  int
	Columns       []weatherColumn
}

func (w *weather) WeatherCodeAsString() string {
	return weatherCodeAsString(w.WeatherCode)
}

type weatherDetails struct {
	HighTemperature          int
	LowTemperature           int
	Humidity                 int
	PrecipitationProbability int
	Precipitation            float64
	Rain                     float64
	Showers                  float64
	Snowfall                 float64
	WindSpeed                float64
	WindDirection            int
	WindGusts                float64
	UVIndex                  float64
	Visibility               float64
	Pressure                 float64
	DewPoint                 float64
	CloudCover               int
	Sunrise                  time.Time
	Sunset                   time.Time
}

func (details weatherDetails) WindDirectionAsString() string {
	return windDirectionAsString(details.WindDirection)
}

func (details weatherDetails) VisibilityAsString(units string) string {
	if units == "metric" {
		return fmt.Sprintf("%.1f km", details.Visibility/1000)
	}

	return fmt.Sprintf("%.1f mi", details.Visibility/1609.344)
}

func (details weatherDetails) PressureAsString(units string) string {
	if units == "metric" {
		return fmt.Sprintf("%.0f hPa", details.Pressure)
	}

	return fmt.Sprintf("%.2f inHg", details.Pressure*0.0295299830714)
}

func (details weatherDetails) SunriseSunsetAsString(hourFormat string) string {
	format := "3:04 PM"
	if hourFormat == "24h" {
		format = "15:04"
	}

	return fmt.Sprintf("%s / %s", details.Sunrise.Format(format), details.Sunset.Format(format))
}

type weatherForecastDay struct {
	Time                     time.Time
	WeatherCode              int
	HighTemperature          int
	LowTemperature           int
	HighApparentTemperature  int
	LowApparentTemperature   int
	PrecipitationProbability int
	Precipitation            float64
	Rain                     float64
	Showers                  float64
	Snowfall                 float64
	PrecipitationHours       float64
	UVIndex                  float64
	WindSpeed                float64
	WindGusts                float64
	WindDirection            int
	Sunrise                  time.Time
	Sunset                   time.Time
	DaylightDuration         float64
	SunshineDuration         float64
}

func (day weatherForecastDay) WeatherCodeAsString() string {
	return weatherCodeAsString(day.WeatherCode)
}

func (day weatherForecastDay) WindDirectionAsString() string {
	return windDirectionAsString(day.WindDirection)
}

func weatherCodeAsString(code int) string {
	if weatherCode, ok := weatherCodeTable[code]; ok {
		return weatherCode
	}

	return ""
}

func windDirectionAsString(degrees int) string {
	directions := [...]string{
		"N", "NNE", "NE", "ENE",
		"E", "ESE", "SE", "SSE",
		"S", "SSW", "SW", "WSW",
		"W", "WNW", "NW", "NNW",
	}

	normalized := ((degrees % 360) + 360) % 360
	index := int(math.Floor((float64(normalized)+11.25)/22.5)) % len(directions)

	return directions[index]
}

type openMeteoPlacesResponseJson struct {
	Results []openMeteoPlaceResponseJson
}

type openMeteoPlaceResponseJson struct {
	Name      string
	Area      string `json:"admin1"`
	Latitude  float64
	Longitude float64
	Timezone  string
	Country   string
	location  *time.Location
}

type openMeteoWeatherResponseJson struct {
	Daily struct {
		Time                     []int64   `json:"time"`
		WeatherCode              []int     `json:"weather_code"`
		HighTemperature          []float64 `json:"temperature_2m_max"`
		LowTemperature           []float64 `json:"temperature_2m_min"`
		HighApparentTemperature  []float64 `json:"apparent_temperature_max"`
		LowApparentTemperature   []float64 `json:"apparent_temperature_min"`
		Sunrise                  []int64   `json:"sunrise"`
		Sunset                   []int64   `json:"sunset"`
		DaylightDuration         []float64 `json:"daylight_duration"`
		SunshineDuration         []float64 `json:"sunshine_duration"`
		UVIndex                  []float64 `json:"uv_index_max"`
		Precipitation            []float64 `json:"precipitation_sum"`
		Rain                     []float64 `json:"rain_sum"`
		Showers                  []float64 `json:"showers_sum"`
		Snowfall                 []float64 `json:"snowfall_sum"`
		PrecipitationHours       []float64 `json:"precipitation_hours"`
		PrecipitationProbability []int     `json:"precipitation_probability_max"`
		WindSpeed                []float64 `json:"wind_speed_10m_max"`
		WindGusts                []float64 `json:"wind_gusts_10m_max"`
		WindDirection            []int     `json:"wind_direction_10m_dominant"`
	} `json:"daily"`

	Hourly struct {
		Temperature              []float64 `json:"temperature_2m"`
		PrecipitationProbability []int     `json:"precipitation_probability"`
	} `json:"hourly"`

	Current struct {
		Temperature         float64 `json:"temperature_2m"`
		ApparentTemperature float64 `json:"apparent_temperature"`
		WeatherCode         int     `json:"weather_code"`
		Humidity            int     `json:"relative_humidity_2m"`
		Precipitation       float64 `json:"precipitation"`
		Rain                float64 `json:"rain"`
		Showers             float64 `json:"showers"`
		Snowfall            float64 `json:"snowfall"`
		CloudCover          int     `json:"cloud_cover"`
		Pressure            float64 `json:"pressure_msl"`
		WindSpeed           float64 `json:"wind_speed_10m"`
		WindDirection       int     `json:"wind_direction_10m"`
		WindGusts           float64 `json:"wind_gusts_10m"`
		Visibility          float64 `json:"visibility"`
		DewPoint            float64 `json:"dew_point_2m"`
	} `json:"current"`
}

type weatherColumn struct {
	Temperature      int
	Scale            float64
	HasPrecipitation bool
}

var commonCountryAbbreviations = map[string]string{
	"US":  "United States",
	"USA": "United States",
	"UK":  "United Kingdom",
}

func expandCountryAbbreviations(name string) string {
	if expanded, ok := commonCountryAbbreviations[strings.TrimSpace(name)]; ok {
		return expanded
	}

	return name
}

// Separates the location that Open Meteo accepts from the administrative area
// which can then be used to filter to the correct place after the list of places
// has been retrieved. Also expands abbreviations since Open Meteo does not accept
// country names like "US", "USA" and "UK"
func parsePlaceName(name string) (string, string) {
	parts := strings.Split(name, ",")

	if len(parts) == 1 {
		return name, ""
	}

	if len(parts) == 2 {
		return parts[0] + ", " + expandCountryAbbreviations(parts[1]), ""
	}

	return parts[0] + ", " + expandCountryAbbreviations(parts[2]), strings.TrimSpace(parts[1])
}

func fetchOpenMeteoPlaceFromName(ctx context.Context, location string) (*openMeteoPlaceResponseJson, error) {
	location, area := parsePlaceName(location)
	requestUrl := fmt.Sprintf("https://geocoding-api.open-meteo.com/v1/search?name=%s&count=20&language=en&format=json", url.QueryEscape(location))

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("creating places request: %w", err)
	}

	responseJson, err := decodeJsonFromRequest[openMeteoPlacesResponseJson](defaultHTTPClient, request)
	if err != nil {
		return nil, fmt.Errorf("fetching places data: %w", err)
	}

	if len(responseJson.Results) == 0 {
		return nil, fmt.Errorf("no places found for %s", location)
	}

	var place *openMeteoPlaceResponseJson

	if area != "" {
		area = strings.ToLower(area)

		for i := range responseJson.Results {
			if strings.ToLower(responseJson.Results[i].Area) == area {
				place = &responseJson.Results[i]
				break
			}
		}

		if place == nil {
			return nil, fmt.Errorf("no place found for %s in %s", location, area)
		}
	} else {
		place = &responseJson.Results[0]
	}

	loc, err := time.LoadLocation(place.Timezone)
	if err != nil {
		return nil, fmt.Errorf("loading location: %v", err)
	}

	place.location = loc

	return place, nil
}

func fetchWeatherForOpenMeteoPlace(ctx context.Context, place *openMeteoPlaceResponseJson, units string) (*weather, error) {
	responseJson, err := fetchOpenMeteoWeatherResponse(ctx, place, units)
	if err != nil {
		return nil, err
	}

	return buildWeatherFromOpenMeteoResponse(responseJson, place), nil
}

func fetchOpenMeteoWeatherResponse(ctx context.Context, place *openMeteoPlaceResponseJson, units string) (*openMeteoWeatherResponseJson, error) {
	query := url.Values{}

	temperatureUnit := "celsius"
	windSpeedUnit := "kmh"
	precipitationUnit := "mm"

	if units == "imperial" {
		temperatureUnit = "fahrenheit"
		windSpeedUnit = "mph"
		precipitationUnit = "inch"
	}

	query.Add("latitude", fmt.Sprintf("%f", place.Latitude))
	query.Add("longitude", fmt.Sprintf("%f", place.Longitude))
	query.Add("timeformat", "unixtime")
	query.Add("timezone", place.Timezone)
	query.Add("forecast_days", "7")

	query.Add(
		"current",
		"temperature_2m,apparent_temperature,weather_code,relative_humidity_2m,"+
			"precipitation,rain,showers,snowfall,cloud_cover,pressure_msl,"+
			"wind_speed_10m,wind_direction_10m,wind_gusts_10m,visibility,dew_point_2m",
	)

	query.Add(
		"hourly",
		"temperature_2m,precipitation_probability",
	)

	query.Add(
		"daily",
		"weather_code,temperature_2m_max,temperature_2m_min,"+
			"apparent_temperature_max,apparent_temperature_min,"+
			"sunrise,sunset,daylight_duration,sunshine_duration,uv_index_max,"+
			"precipitation_sum,rain_sum,showers_sum,snowfall_sum,precipitation_hours,"+
			"precipitation_probability_max,wind_speed_10m_max,wind_gusts_10m_max,"+
			"wind_direction_10m_dominant",
	)

	query.Add("temperature_unit", temperatureUnit)
	query.Add("wind_speed_unit", windSpeedUnit)
	query.Add("precipitation_unit", precipitationUnit)

	requestUrl := "https://api.open-meteo.com/v1/forecast?" + query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: creating weather request: %w", errNoContent, err)
	}

	responseJson, err := decodeJsonFromRequest[openMeteoWeatherResponseJson](defaultHTTPClient, request)
	if err != nil {
		return nil, fmt.Errorf("%w: fetching weather data: %w", errNoContent, err)
	}

	return &responseJson, nil
}

func buildWeatherFromOpenMeteoResponse(responseJson *openMeteoWeatherResponseJson, place *openMeteoPlaceResponseJson) *weather {
	now := time.Now().In(place.location)
	bars := make([]weatherColumn, 0, 12)
	currentBar := now.Hour() / 2

	sunriseBar := -1
	sunsetBar := -1

	if len(responseJson.Daily.Sunrise) > 0 {
		sunriseBar = time.Unix(responseJson.Daily.Sunrise[0], 0).In(place.location).Hour() / 2
	}

	if len(responseJson.Daily.Sunset) > 0 {
		sunsetBar = (time.Unix(responseJson.Daily.Sunset[0], 0).In(place.location).Hour() - 1) / 2
		if sunsetBar < 0 {
			sunsetBar = 0
		}
	}

	if len(responseJson.Hourly.Temperature) >= 24 &&
		len(responseJson.Hourly.PrecipitationProbability) >= 24 {
		temperatures := make([]int, 12)
		precipitations := make([]bool, 12)

		t := responseJson.Hourly.Temperature
		p := responseJson.Hourly.PrecipitationProbability

		for i := 0; i < 24; i += 2 {
			if i/2 == currentBar {
				temperatures[i/2] = int(math.Round(responseJson.Current.Temperature))
			} else {
				temperatures[i/2] = int(math.Round((t[i] + t[i+1]) / 2))
			}

			precipitations[i/2] = (p[i]+p[i+1])/2 > 75
		}

		minT := slices.Min(temperatures)
		maxT := slices.Max(temperatures)
		temperaturesRange := float64(maxT - minT)

		for i := 0; i < 12; i++ {
			bars = append(bars, weatherColumn{
				Temperature:      temperatures[i],
				HasPrecipitation: precipitations[i],
			})

			if temperaturesRange > 0 {
				bars[i].Scale = float64(temperatures[i]-minT) / temperaturesRange
			} else {
				bars[i].Scale = 1
			}
		}
	}

	details := weatherDetails{
		Humidity:      responseJson.Current.Humidity,
		Precipitation: responseJson.Current.Precipitation,
		Rain:          responseJson.Current.Rain,
		Showers:       responseJson.Current.Showers,
		Snowfall:      responseJson.Current.Snowfall,
		WindSpeed:     responseJson.Current.WindSpeed,
		WindDirection: responseJson.Current.WindDirection,
		WindGusts:     responseJson.Current.WindGusts,
		Visibility:    responseJson.Current.Visibility,
		Pressure:      responseJson.Current.Pressure,
		DewPoint:      responseJson.Current.DewPoint,
		CloudCover:    responseJson.Current.CloudCover,
	}

	if len(responseJson.Daily.HighTemperature) > 0 {
		details.HighTemperature = int(math.Round(responseJson.Daily.HighTemperature[0]))
	}
	if len(responseJson.Daily.LowTemperature) > 0 {
		details.LowTemperature = int(math.Round(responseJson.Daily.LowTemperature[0]))
	}
	if len(responseJson.Daily.PrecipitationProbability) > 0 {
		details.PrecipitationProbability = responseJson.Daily.PrecipitationProbability[0]
	}
	if len(responseJson.Daily.UVIndex) > 0 {
		details.UVIndex = responseJson.Daily.UVIndex[0]
	}
	if len(responseJson.Daily.Sunrise) > 0 {
		details.Sunrise = time.Unix(responseJson.Daily.Sunrise[0], 0).In(place.location)
	}
	if len(responseJson.Daily.Sunset) > 0 {
		details.Sunset = time.Unix(responseJson.Daily.Sunset[0], 0).In(place.location)
	}

	forecastLength := min(
		7,
		len(responseJson.Daily.Time),
		len(responseJson.Daily.WeatherCode),
		len(responseJson.Daily.HighTemperature),
		len(responseJson.Daily.LowTemperature),
	)

	forecast := make([]weatherForecastDay, 0, forecastLength)

	for i := 0; i < forecastLength; i++ {
		day := weatherForecastDay{
			Time:            time.Unix(responseJson.Daily.Time[i], 0).In(place.location),
			WeatherCode:     responseJson.Daily.WeatherCode[i],
			HighTemperature: int(math.Round(responseJson.Daily.HighTemperature[i])),
			LowTemperature:  int(math.Round(responseJson.Daily.LowTemperature[i])),
		}

		if i < len(responseJson.Daily.HighApparentTemperature) {
			day.HighApparentTemperature = int(math.Round(responseJson.Daily.HighApparentTemperature[i]))
		}
		if i < len(responseJson.Daily.LowApparentTemperature) {
			day.LowApparentTemperature = int(math.Round(responseJson.Daily.LowApparentTemperature[i]))
		}
		if i < len(responseJson.Daily.PrecipitationProbability) {
			day.PrecipitationProbability = responseJson.Daily.PrecipitationProbability[i]
		}
		if i < len(responseJson.Daily.Precipitation) {
			day.Precipitation = responseJson.Daily.Precipitation[i]
		}
		if i < len(responseJson.Daily.Rain) {
			day.Rain = responseJson.Daily.Rain[i]
		}
		if i < len(responseJson.Daily.Showers) {
			day.Showers = responseJson.Daily.Showers[i]
		}
		if i < len(responseJson.Daily.Snowfall) {
			day.Snowfall = responseJson.Daily.Snowfall[i]
		}
		if i < len(responseJson.Daily.PrecipitationHours) {
			day.PrecipitationHours = responseJson.Daily.PrecipitationHours[i]
		}
		if i < len(responseJson.Daily.UVIndex) {
			day.UVIndex = responseJson.Daily.UVIndex[i]
		}
		if i < len(responseJson.Daily.WindSpeed) {
			day.WindSpeed = responseJson.Daily.WindSpeed[i]
		}
		if i < len(responseJson.Daily.WindGusts) {
			day.WindGusts = responseJson.Daily.WindGusts[i]
		}
		if i < len(responseJson.Daily.WindDirection) {
			day.WindDirection = responseJson.Daily.WindDirection[i]
		}
		if i < len(responseJson.Daily.Sunrise) {
			day.Sunrise = time.Unix(responseJson.Daily.Sunrise[i], 0).In(place.location)
		}
		if i < len(responseJson.Daily.Sunset) {
			day.Sunset = time.Unix(responseJson.Daily.Sunset[i], 0).In(place.location)
		}
		if i < len(responseJson.Daily.DaylightDuration) {
			day.DaylightDuration = responseJson.Daily.DaylightDuration[i]
		}
		if i < len(responseJson.Daily.SunshineDuration) {
			day.SunshineDuration = responseJson.Daily.SunshineDuration[i]
		}

		forecast = append(forecast, day)
	}

	return &weather{
		Temperature:         int(math.Round(responseJson.Current.Temperature)),
		ApparentTemperature: int(math.Round(responseJson.Current.ApparentTemperature)),
		WeatherCode:         responseJson.Current.WeatherCode,
		Details:             details,
		Forecast:            forecast,
		CurrentColumn:       currentBar,
		SunriseColumn:       sunriseBar,
		SunsetColumn:        sunsetBar,
		Columns:             bars,
	}
}

var weatherCodeTable = map[int]string{
	0:  "Clear Sky",
	1:  "Mainly Clear",
	2:  "Partly Cloudy",
	3:  "Overcast",
	45: "Fog",
	48: "Rime Fog",
	51: "Drizzle",
	53: "Drizzle",
	55: "Drizzle",
	56: "Drizzle",
	57: "Drizzle",
	61: "Rain",
	63: "Moderate Rain",
	65: "Heavy Rain",
	66: "Freezing Rain",
	67: "Freezing Rain",
	71: "Snow",
	73: "Moderate Snow",
	75: "Heavy Snow",
	77: "Snow Grains",
	80: "Rain",
	81: "Moderate Rain",
	82: "Heavy Rain",
	85: "Snow",
	86: "Snow",
	95: "Thunderstorm",
	96: "Thunderstorm",
	99: "Thunderstorm",
}
