package glance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFetchOpenMeteoPlaceFromNameCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchOpenMeteoPlaceFromName(ctx, "Test Location")
	if err == nil {
		t.Fatal("expected canceled places request to return an error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}

func TestFetchWeatherForOpenMeteoPlaceCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	place := &openMeteoPlaceResponseJson{
		Latitude:  40.0,
		Longitude: -80.0,
		Timezone:  "UTC",
		location:  time.UTC,
	}

	_, err := fetchWeatherForOpenMeteoPlace(ctx, place, "metric")
	if err == nil {
		t.Fatal("expected canceled weather request to return an error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}

func TestWeatherWidgetGeocodingCancellationIsLifecycleNeutral(t *testing.T) {
	resetOpenMeteoPlaceResourceCache(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	widget := &weatherWidget{
		widgetBase: widgetBase{ContentAvailable: true},
		Location:   "Canceled Location",
	}
	widget.withCacheDuration(time.Hour)

	originalNextUpdate := time.Now().Add(30 * time.Minute)
	widget.nextUpdate = originalNextUpdate

	widget.update(ctx)

	if widget.Place != nil {
		t.Fatalf("cancelled geocoding populated place: %+v", widget.Place)
	}

	if widget.Error != nil {
		t.Fatalf("cancelled geocoding set widget error: %v", widget.Error)
	}

	if widget.refreshDegraded {
		t.Fatal("cancelled geocoding marked widget degraded")
	}

	if widget.updateRetriedTimes != 0 {
		t.Fatalf(
			"cancelled geocoding retry attempts = %d, want 0",
			widget.updateRetriedTimes,
		)
	}

	if widget.refreshFailureCount != 0 {
		t.Fatalf(
			"cancelled geocoding failure count = %d, want 0",
			widget.refreshFailureCount,
		)
	}

	if !widget.nextUpdate.Equal(originalNextUpdate) {
		t.Fatalf(
			"cancelled geocoding changed next update: got %v want %v",
			widget.nextUpdate,
			originalNextUpdate,
		)
	}
}

func boolPointer(value bool) *bool {
	return &value
}

func TestWeatherWidgetSectionDefaults(t *testing.T) {
	widget := &weatherWidget{
		widgetBase: widgetBase{ContentAvailable: true},
		Location:   "Saint Peters, Missouri, US",
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	if !widget.ShowCurrent {
		t.Fatal("show-current default = false, want true")
	}
	if !widget.ShowDetails {
		t.Fatal("show-details default = false, want true")
	}
	if !widget.ShowHourly {
		t.Fatal("show-hourly default = false, want true")
	}
	if !widget.ShowForecast {
		t.Fatal("show-forecast default = false, want true")
	}
}

func TestWeatherWidgetSectionFlagsCanBeDisabled(t *testing.T) {
	widget := &weatherWidget{
		widgetBase:      widgetBase{ContentAvailable: true},
		Location:        "Saint Peters, Missouri, US",
		ShowCurrentRaw:  boolPointer(false),
		ShowDetailsRaw:  boolPointer(false),
		ShowHourlyRaw:   boolPointer(false),
		ShowForecastRaw: boolPointer(false),
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	if widget.ShowCurrent {
		t.Fatal("show-current = true, want false")
	}
	if widget.ShowDetails {
		t.Fatal("show-details = true, want false")
	}
	if widget.ShowHourly {
		t.Fatal("show-hourly = true, want false")
	}
	if widget.ShowForecast {
		t.Fatal("show-forecast = true, want false")
	}
}

func TestWindDirectionAsString(t *testing.T) {
	tests := []struct {
		degrees int
		want    string
	}{
		{0, "N"},
		{11, "N"},
		{12, "NNE"},
		{45, "NE"},
		{90, "E"},
		{135, "SE"},
		{180, "S"},
		{225, "SW"},
		{270, "W"},
		{315, "NW"},
		{348, "NNW"},
		{349, "N"},
		{360, "N"},
		{-90, "W"},
	}

	for _, test := range tests {
		if got := windDirectionAsString(test.degrees); got != test.want {
			t.Errorf(
				"windDirectionAsString(%d) = %q, want %q",
				test.degrees,
				got,
				test.want,
			)
		}
	}
}

func TestBuildWeatherFromOpenMeteoResponseRichForecast(t *testing.T) {
	location := time.FixedZone("Test", -5*60*60)
	place := &openMeteoPlaceResponseJson{
		location: location,
	}

	response := &openMeteoWeatherResponseJson{}

	response.Current.Temperature = 72.6
	response.Current.ApparentTemperature = 74.4
	response.Current.WeatherCode = 2
	response.Current.Humidity = 61
	response.Current.Precipitation = 0.12
	response.Current.Rain = 0.10
	response.Current.Showers = 0.02
	response.Current.Snowfall = 0
	response.Current.CloudCover = 47
	response.Current.Pressure = 1014.2
	response.Current.WindSpeed = 8.4
	response.Current.WindDirection = 225
	response.Current.WindGusts = 14.8
	response.Current.Visibility = 16000
	response.Current.DewPoint = 58.3

	for i := 0; i < 24; i++ {
		response.Hourly.Temperature = append(
			response.Hourly.Temperature,
			70+float64(i)/10,
		)
		response.Hourly.PrecipitationProbability = append(
			response.Hourly.PrecipitationProbability,
			i,
		)
	}

	start := time.Date(2026, 9, 6, 0, 0, 0, 0, location)

	for i := 0; i < 7; i++ {
		day := start.AddDate(0, 0, i)

		response.Daily.Time = append(response.Daily.Time, day.Unix())
		response.Daily.WeatherCode = append(response.Daily.WeatherCode, i%4)
		response.Daily.HighTemperature = append(response.Daily.HighTemperature, 80+float64(i))
		response.Daily.LowTemperature = append(response.Daily.LowTemperature, 60+float64(i))
		response.Daily.HighApparentTemperature = append(response.Daily.HighApparentTemperature, 82+float64(i))
		response.Daily.LowApparentTemperature = append(response.Daily.LowApparentTemperature, 59+float64(i))
		response.Daily.Sunrise = append(response.Daily.Sunrise, day.Add(6*time.Hour+30*time.Minute).Unix())
		response.Daily.Sunset = append(response.Daily.Sunset, day.Add(19*time.Hour+15*time.Minute).Unix())
		response.Daily.DaylightDuration = append(response.Daily.DaylightDuration, 45900)
		response.Daily.SunshineDuration = append(response.Daily.SunshineDuration, 36000)
		response.Daily.UVIndex = append(response.Daily.UVIndex, 6.2)
		response.Daily.Precipitation = append(response.Daily.Precipitation, 0.2)
		response.Daily.Rain = append(response.Daily.Rain, 0.2)
		response.Daily.Showers = append(response.Daily.Showers, 0)
		response.Daily.Snowfall = append(response.Daily.Snowfall, 0)
		response.Daily.PrecipitationHours = append(response.Daily.PrecipitationHours, 2)
		response.Daily.PrecipitationProbability = append(response.Daily.PrecipitationProbability, 30+i)
		response.Daily.WindSpeed = append(response.Daily.WindSpeed, 12+float64(i))
		response.Daily.WindGusts = append(response.Daily.WindGusts, 20+float64(i))
		response.Daily.WindDirection = append(response.Daily.WindDirection, 225)
	}

	got := buildWeatherFromOpenMeteoResponse(response, place)

	if got.Temperature != 73 {
		t.Fatalf("temperature = %d, want 73", got.Temperature)
	}
	if got.ApparentTemperature != 74 {
		t.Fatalf("apparent temperature = %d, want 74", got.ApparentTemperature)
	}
	if got.WeatherCode != 2 {
		t.Fatalf("weather code = %d, want 2", got.WeatherCode)
	}

	if got.Details.HighTemperature != 80 {
		t.Fatalf("high temperature = %d, want 80", got.Details.HighTemperature)
	}
	if got.Details.LowTemperature != 60 {
		t.Fatalf("low temperature = %d, want 60", got.Details.LowTemperature)
	}
	if got.Details.Humidity != 61 {
		t.Fatalf("humidity = %d, want 61", got.Details.Humidity)
	}
	if got.Details.PrecipitationProbability != 30 {
		t.Fatalf(
			"precipitation probability = %d, want 30",
			got.Details.PrecipitationProbability,
		)
	}
	if got.Details.WindDirectionAsString() != "SW" {
		t.Fatalf(
			"wind direction = %q, want SW",
			got.Details.WindDirectionAsString(),
		)
	}
	if got.Details.UVIndex != 6.2 {
		t.Fatalf("UV index = %v, want 6.2", got.Details.UVIndex)
	}

	if len(got.Forecast) != 7 {
		t.Fatalf("forecast days = %d, want 7", len(got.Forecast))
	}

	first := got.Forecast[0]
	last := got.Forecast[6]

	if first.HighTemperature != 80 || first.LowTemperature != 60 {
		t.Fatalf("first forecast temperatures = %+v", first)
	}
	if last.HighTemperature != 86 || last.LowTemperature != 66 {
		t.Fatalf("last forecast temperatures = %+v", last)
	}
	if first.PrecipitationProbability != 30 {
		t.Fatalf(
			"first forecast precipitation probability = %d, want 30",
			first.PrecipitationProbability,
		)
	}
	if first.WindDirectionAsString() != "SW" {
		t.Fatalf(
			"first forecast wind direction = %q, want SW",
			first.WindDirectionAsString(),
		)
	}
}

func TestBuildWeatherFromOpenMeteoResponseHandlesPartialDailyData(t *testing.T) {
	location := time.UTC
	place := &openMeteoPlaceResponseJson{
		location: location,
	}

	response := &openMeteoWeatherResponseJson{}
	response.Current.Temperature = 20
	response.Current.ApparentTemperature = 19
	response.Current.WeatherCode = 1

	response.Daily.Time = []int64{
		time.Date(2026, 9, 6, 0, 0, 0, 0, location).Unix(),
	}
	response.Daily.WeatherCode = []int{1}
	response.Daily.HighTemperature = []float64{25}
	response.Daily.LowTemperature = []float64{15}

	got := buildWeatherFromOpenMeteoResponse(response, place)

	if len(got.Forecast) != 1 {
		t.Fatalf("forecast days = %d, want 1", len(got.Forecast))
	}

	if got.SunriseColumn != -1 {
		t.Fatalf("sunrise column = %d, want -1", got.SunriseColumn)
	}
	if got.SunsetColumn != -1 {
		t.Fatalf("sunset column = %d, want -1", got.SunsetColumn)
	}

	if got.Forecast[0].HighTemperature != 25 ||
		got.Forecast[0].LowTemperature != 15 {
		t.Fatalf("partial forecast = %+v", got.Forecast[0])
	}
}

func TestWeatherDetailsVisibilityAsString(t *testing.T) {
	details := weatherDetails{Visibility: 16093.44}

	if got := details.VisibilityAsString("metric"); got != "16.1 km" {
		t.Fatalf("metric visibility = %q, want %q", got, "16.1 km")
	}

	if got := details.VisibilityAsString("imperial"); got != "10.0 mi" {
		t.Fatalf("imperial visibility = %q, want %q", got, "10.0 mi")
	}
}
func TestWeatherDetailsPressureAsString(t *testing.T) {
	details := weatherDetails{Pressure: 1014}

	if got := details.PressureAsString("metric"); got != "1014 hPa" {
		t.Fatalf("metric pressure = %q, want %q", got, "1014 hPa")
	}

	if got := details.PressureAsString("imperial"); got != "29.94 inHg" {
		t.Fatalf("imperial pressure = %q, want %q", got, "29.94 inHg")
	}
}

func TestWeatherDetailsSunriseSunsetAsString(t *testing.T) {
	location := time.FixedZone("Test", -5*60*60)
	details := weatherDetails{
		Sunrise: time.Date(2026, 9, 6, 6, 34, 0, 0, location),
		Sunset:  time.Date(2026, 9, 6, 19, 23, 0, 0, location),
	}

	if got := details.SunriseSunsetAsString("12h"); got != "6:34 AM / 7:23 PM" {
		t.Fatalf("12h sunrise/sunset = %q, want %q", got, "6:34 AM / 7:23 PM")
	}

	if got := details.SunriseSunsetAsString("24h"); got != "06:34 / 19:23" {
		t.Fatalf("24h sunrise/sunset = %q, want %q", got, "06:34 / 19:23")
	}
}

func weatherRenderTestWidget(t *testing.T) *weatherWidget {
	t.Helper()

	location := time.FixedZone("Test", -5*60*60)
	start := time.Date(2026, 9, 6, 0, 0, 0, 0, location)

	widget := &weatherWidget{
		widgetBase: widgetBase{ContentAvailable: true},
		Location:   "Saint Peters, Missouri, US",
		Units:      "imperial",
		Place: &openMeteoPlaceResponseJson{
			Name:     "Saint Peters",
			Area:     "Missouri",
			Country:  "United States",
			location: location,
		},
		Weather: &weather{
			Temperature:         73,
			ApparentTemperature: 74,
			WeatherCode:         2,
			Details: weatherDetails{
				HighTemperature:          80,
				LowTemperature:           60,
				Humidity:                 61,
				PrecipitationProbability: 30,
				WindSpeed:                8,
				WindDirection:            225,
				UVIndex:                  6.2,
				Visibility:               16093.44,
				Pressure:                 1014,
				Sunrise:                  start.Add(6*time.Hour + 30*time.Minute),
				Sunset:                   start.Add(19*time.Hour + 15*time.Minute),
			},
			CurrentColumn: 0,
			SunriseColumn: -1,
			SunsetColumn:  -1,
		},
	}

	for i := 0; i < 12; i++ {
		widget.Weather.Columns = append(widget.Weather.Columns, weatherColumn{
			Temperature:      70 + i,
			Scale:            float64(i) / 11,
			HasPrecipitation: i == 3,
		})
		widget.TimeLabels[i] = "1 PM"
	}

	for i := 0; i < 7; i++ {
		widget.Weather.Forecast = append(widget.Weather.Forecast, weatherForecastDay{
			Time:                     start.AddDate(0, 0, i),
			WeatherCode:              i % 4,
			HighTemperature:          80 + i,
			LowTemperature:           60 + i,
			PrecipitationProbability: 30 + i,
		})
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	return widget
}

func TestWeatherWidgetRenderSections(t *testing.T) {
	tests := []struct {
		name         string
		configure    func(*weatherWidget)
		wantCurrent  bool
		wantDetails  bool
		wantHourly   bool
		wantForecast bool
		wantLocation bool
	}{
		{
			name:         "all sections default enabled",
			wantCurrent:  true,
			wantDetails:  true,
			wantHourly:   true,
			wantForecast: true,
			wantLocation: true,
		},
		{
			name: "current disabled",
			configure: func(widget *weatherWidget) {
				widget.ShowCurrent = false
			},
			wantDetails:  true,
			wantHourly:   true,
			wantForecast: true,
			wantLocation: true,
		},
		{
			name: "details disabled",
			configure: func(widget *weatherWidget) {
				widget.ShowDetails = false
			},
			wantCurrent:  true,
			wantHourly:   true,
			wantForecast: true,
			wantLocation: true,
		},
		{
			name: "hourly disabled",
			configure: func(widget *weatherWidget) {
				widget.ShowHourly = false
			},
			wantCurrent:  true,
			wantDetails:  true,
			wantForecast: true,
			wantLocation: true,
		},
		{
			name: "forecast disabled",
			configure: func(widget *weatherWidget) {
				widget.ShowForecast = false
			},
			wantCurrent:  true,
			wantDetails:  true,
			wantHourly:   true,
			wantLocation: true,
		},
		{
			name: "location hidden independently",
			configure: func(widget *weatherWidget) {
				widget.HideLocation = true
			},
			wantCurrent:  true,
			wantDetails:  true,
			wantHourly:   true,
			wantForecast: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			widget := weatherRenderTestWidget(t)
			if test.configure != nil {
				test.configure(widget)
			}

			rendered := string(widget.Render())

			checks := []struct {
				class string
				want  bool
			}{
				{"weather-current", test.wantCurrent},
				{"weather-details", test.wantDetails},
				{"weather-columns", test.wantHourly},
				{"weather-forecast", test.wantForecast},
				{"weather-location", test.wantLocation},
			}

			for _, check := range checks {
				got := strings.Contains(rendered, `class="`+check.class)
				if got != check.want {
					t.Errorf("%s presence = %t, want %t", check.class, got, check.want)
				}
			}
		})
	}
}

func TestWeatherWidgetRenderSevenDayForecast(t *testing.T) {
	widget := weatherRenderTestWidget(t)
	rendered := string(widget.Render())

	if got := strings.Count(rendered, `class="weather-forecast-day"`); got != 7 {
		t.Fatalf("forecast rows = %d, want 7", got)
	}

	if !strings.Contains(rendered, ">Today<") {
		t.Fatal("forecast does not label the first row Today")
	}
}
