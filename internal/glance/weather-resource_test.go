package glance

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func resetOpenMeteoPlaceResourceCache(t *testing.T) {
	t.Helper()

	openMeteoPlaceResourceCache.mu.Lock()
	old := openMeteoPlaceResourceCache.entries
	openMeteoPlaceResourceCache.entries = make(map[string]*keyedResourceCacheEntry[*openMeteoPlaceResponseJson])
	openMeteoPlaceResourceCache.mu.Unlock()

	t.Cleanup(func() {
		openMeteoPlaceResourceCache.mu.Lock()
		openMeteoPlaceResourceCache.entries = old
		openMeteoPlaceResourceCache.mu.Unlock()
	})
}

func TestOpenMeteoPlaceResourceCachesSameLocation(t *testing.T) {
	resetOpenMeteoPlaceResourceCache(t)

	var calls atomic.Int32
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return wave3Response(http.StatusOK, `{"results":[{"name":"Saint Peters","admin1":"Missouri","latitude":38.8,"longitude":-90.6,"timezone":"America/Chicago","country":"United States"}]}`, nil), nil
	})

	first, err := fetchOpenMeteoPlaceResource(context.Background(), "Saint Peters, Missouri, US")
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	second, err := fetchOpenMeteoPlaceResource(context.Background(), "Saint Peters, Missouri, US")
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1", got)
	}
	if first.Name != second.Name || first.Area != second.Area {
		t.Fatalf("cached place differs: first=%+v second=%+v", first, second)
	}
}

func TestOpenMeteoPlaceResourceKeepsConfiguredAreasIndependent(t *testing.T) {
	resetOpenMeteoPlaceResourceCache(t)

	var calls atomic.Int32
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return wave3Response(http.StatusOK, `{"results":[{"name":"Springfield","admin1":"Illinois","latitude":39.8,"longitude":-89.6,"timezone":"America/Chicago","country":"United States"},{"name":"Springfield","admin1":"Missouri","latitude":37.2,"longitude":-93.3,"timezone":"America/Chicago","country":"United States"}]}`, nil), nil
	})

	illinois, err := fetchOpenMeteoPlaceResource(context.Background(), "Springfield, Illinois, US")
	if err != nil {
		t.Fatalf("Illinois fetch: %v", err)
	}
	missouri, err := fetchOpenMeteoPlaceResource(context.Background(), "Springfield, Missouri, US")
	if err != nil {
		t.Fatalf("Missouri fetch: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("HTTP calls = %d, want 2", got)
	}
	if illinois.Area != "Illinois" {
		t.Fatalf("Illinois area = %q", illinois.Area)
	}
	if missouri.Area != "Missouri" {
		t.Fatalf("Missouri area = %q", missouri.Area)
	}
}

func TestOpenMeteoPlaceResourceDoesNotCacheFailure(t *testing.T) {
	resetOpenMeteoPlaceResourceCache(t)

	var calls atomic.Int32
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("temporary failure")
		}
		return wave3Response(http.StatusOK, `{"results":[{"name":"Saint Peters","admin1":"Missouri","latitude":38.8,"longitude":-90.6,"timezone":"America/Chicago","country":"United States"}]}`, nil), nil
	})

	if _, err := fetchOpenMeteoPlaceResource(context.Background(), "Saint Peters, Missouri, US"); err == nil {
		t.Fatal("first fetch unexpectedly succeeded")
	}
	if _, err := fetchOpenMeteoPlaceResource(context.Background(), "Saint Peters, Missouri, US"); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("HTTP calls = %d, want 2", got)
	}
}

func TestOpenMeteoPlaceResourceEvictsIdleEntries(t *testing.T) {
	resetOpenMeteoPlaceResourceCache(t)

	stale := &keyedResourceCacheEntry[*openMeteoPlaceResponseJson]{
		lastUsed: time.Now().Add(-openMeteoResourceIdleRetention),
	}
	active := &keyedResourceCacheEntry[*openMeteoPlaceResponseJson]{
		lastUsed: time.Now().Add(-openMeteoResourceIdleRetention),
		current: &keyedResourceCacheCall[*openMeteoPlaceResponseJson]{
			done: make(chan struct{}),
		},
	}

	openMeteoPlaceResourceCache.mu.Lock()
	openMeteoPlaceResourceCache.entries["stale"] = stale
	openMeteoPlaceResourceCache.entries["active"] = active
	openMeteoPlaceResourceCache.mu.Unlock()

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		return wave3Response(
			http.StatusOK,
			`{"results":[{"name":"Saint Peters","admin1":"Missouri","latitude":38.8,"longitude":-90.6,"timezone":"America/Chicago","country":"United States"}]}`,
			nil,
		), nil
	})

	location := "Saint Peters, Missouri, US"
	if _, err := fetchOpenMeteoPlaceResource(context.Background(), location); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	openMeteoPlaceResourceCache.mu.Lock()
	_, staleExists := openMeteoPlaceResourceCache.entries["stale"]
	_, activeExists := openMeteoPlaceResourceCache.entries["active"]
	_, requestedExists := openMeteoPlaceResourceCache.entries[location]
	openMeteoPlaceResourceCache.mu.Unlock()

	if staleExists {
		t.Fatal("idle stale place entry was not evicted")
	}
	if !activeExists {
		t.Fatal("active stale place entry was evicted")
	}
	if !requestedExists {
		t.Fatal("requested place entry is missing")
	}
}

func resetOpenMeteoWeatherResourceCache(t *testing.T) {
	t.Helper()

	openMeteoWeatherResourceCache.mu.Lock()
	old := openMeteoWeatherResourceCache.entries
	openMeteoWeatherResourceCache.entries = make(map[openMeteoWeatherResourceKey]*keyedResourceCacheEntry[*openMeteoWeatherResponseJson])
	openMeteoWeatherResourceCache.mu.Unlock()

	t.Cleanup(func() {
		openMeteoWeatherResourceCache.mu.Lock()
		openMeteoWeatherResourceCache.entries = old
		openMeteoWeatherResourceCache.mu.Unlock()
	})
}

func openMeteoWeatherResourceTestResponse() *http.Response {
	temps := make([]string, 168)
	precip := make([]string, 168)

	for i := range temps {
		temps[i] = "20"
		precip[i] = "0"
	}

	body := `{
		"daily":{
			"time":[1788667200,1788753600,1788840000,1788926400,1789012800,1789099200,1789185600],
			"weather_code":[1,2,3,61,0,80,2],
			"temperature_2m_max":[25,26,24,22,27,23,25],
			"temperature_2m_min":[15,16,14,13,17,15,16],
			"apparent_temperature_max":[26,27,25,22,28,24,26],
			"apparent_temperature_min":[14,15,13,12,16,14,15],
			"sunrise":[1788688800,1788775200,1788861600,1788948000,1789034400,1789120800,1789207200],
			"sunset":[1788735600,1788822000,1788908400,1788994800,1789081200,1789167600,1789254000],
			"daylight_duration":[46800,46800,46800,46800,46800,46800,46800],
			"sunshine_duration":[36000,35000,30000,18000,40000,22000,32000],
			"uv_index_max":[6.0,6.2,5.8,3.1,6.5,4.0,5.7],
			"precipitation_sum":[0,0.2,0,4.5,0,2.1,0.1],
			"rain_sum":[0,0.2,0,4.5,0,2.1,0.1],
			"showers_sum":[0,0,0,0,0,0,0],
			"snowfall_sum":[0,0,0,0,0,0,0],
			"precipitation_hours":[0,1,0,5,0,3,1],
			"precipitation_probability_max":[5,20,10,80,5,65,15],
			"wind_speed_10m_max":[12,14,10,18,9,16,11],
			"wind_gusts_10m_max":[20,22,18,30,16,27,19],
			"wind_direction_10m_dominant":[180,190,200,225,160,240,210]
		},
		"hourly":{
			"temperature_2m":[` + strings.Join(temps, ",") + `],
			"precipitation_probability":[` + strings.Join(precip, ",") + `]
		},
		"current":{
			"temperature_2m":20,
			"apparent_temperature":19,
			"weather_code":1,
			"relative_humidity_2m":61,
			"precipitation":0,
			"rain":0,
			"showers":0,
			"snowfall":0,
			"cloud_cover":35,
			"pressure_msl":1015.2,
			"wind_speed_10m":8.5,
			"wind_direction_10m":225,
			"wind_gusts_10m":14.2,
			"visibility":16000,
			"dew_point_2m":12.4
		}
	}`

	return wave3Response(http.StatusOK, body, nil)
}

func TestFetchOpenMeteoWeatherResponseRequestsExpandedForecast(t *testing.T) {
	var captured *http.Request

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		captured = request
		return openMeteoWeatherResourceTestResponse(), nil
	})

	place := &openMeteoPlaceResponseJson{
		Latitude:  38.8,
		Longitude: -90.6,
		Timezone:  "America/Chicago",
		location:  time.UTC,
	}

	if _, err := fetchOpenMeteoWeatherResponse(
		context.Background(),
		place,
		"metric",
	); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if captured == nil {
		t.Fatal("weather request was not captured")
	}

	query := captured.URL.Query()

	if got := query.Get("forecast_days"); got != "7" {
		t.Fatalf("forecast_days = %q, want 7", got)
	}

	if got := query.Get("temperature_unit"); got != "celsius" {
		t.Fatalf("temperature_unit = %q, want celsius", got)
	}
	if got := query.Get("wind_speed_unit"); got != "kmh" {
		t.Fatalf("wind_speed_unit = %q, want kmh", got)
	}
	if got := query.Get("precipitation_unit"); got != "mm" {
		t.Fatalf("precipitation_unit = %q, want mm", got)
	}

	current := query.Get("current")
	for _, field := range []string{
		"temperature_2m",
		"apparent_temperature",
		"weather_code",
		"relative_humidity_2m",
		"precipitation",
		"rain",
		"showers",
		"snowfall",
		"cloud_cover",
		"pressure_msl",
		"wind_speed_10m",
		"wind_direction_10m",
		"wind_gusts_10m",
		"visibility",
		"dew_point_2m",
	} {
		if !strings.Contains(current, field) {
			t.Errorf("current fields missing %q: %q", field, current)
		}
	}

	hourly := query.Get("hourly")
	for _, field := range []string{
		"temperature_2m",
		"precipitation_probability",
	} {
		if !strings.Contains(hourly, field) {
			t.Errorf("hourly fields missing %q: %q", field, hourly)
		}
	}

	daily := query.Get("daily")
	for _, field := range []string{
		"weather_code",
		"temperature_2m_max",
		"temperature_2m_min",
		"apparent_temperature_max",
		"apparent_temperature_min",
		"sunrise",
		"sunset",
		"daylight_duration",
		"sunshine_duration",
		"uv_index_max",
		"precipitation_sum",
		"rain_sum",
		"showers_sum",
		"snowfall_sum",
		"precipitation_hours",
		"precipitation_probability_max",
		"wind_speed_10m_max",
		"wind_gusts_10m_max",
		"wind_direction_10m_dominant",
	} {
		if !strings.Contains(daily, field) {
			t.Errorf("daily fields missing %q: %q", field, daily)
		}
	}
}

func TestFetchOpenMeteoWeatherResponseRequestsImperialUnits(t *testing.T) {
	var captured *http.Request

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		captured = request
		return openMeteoWeatherResourceTestResponse(), nil
	})

	place := &openMeteoPlaceResponseJson{
		Latitude:  38.8,
		Longitude: -90.6,
		Timezone:  "America/Chicago",
		location:  time.UTC,
	}

	if _, err := fetchOpenMeteoWeatherResponse(
		context.Background(),
		place,
		"imperial",
	); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if captured == nil {
		t.Fatal("weather request was not captured")
	}

	query := captured.URL.Query()

	if got := query.Get("temperature_unit"); got != "fahrenheit" {
		t.Fatalf("temperature_unit = %q, want fahrenheit", got)
	}
	if got := query.Get("wind_speed_unit"); got != "mph" {
		t.Fatalf("wind_speed_unit = %q, want mph", got)
	}
	if got := query.Get("precipitation_unit"); got != "inch" {
		t.Fatalf("precipitation_unit = %q, want inch", got)
	}
}

func TestOpenMeteoWeatherResourceCachesSameForecast(t *testing.T) {
	resetOpenMeteoWeatherResourceCache(t)

	var calls atomic.Int32
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return openMeteoWeatherResourceTestResponse(), nil
	})

	place := &openMeteoPlaceResponseJson{
		Latitude:  38.8,
		Longitude: -90.6,
		Timezone:  "America/Chicago",
		location:  time.UTC,
	}

	first, err := fetchOpenMeteoWeatherResource(context.Background(), place, "imperial")
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	second, err := fetchOpenMeteoWeatherResource(context.Background(), place, "imperial")
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1", got)
	}
	if first.Temperature != second.Temperature {
		t.Fatalf("cached weather differs: first=%+v second=%+v", first, second)
	}
}

func TestOpenMeteoWeatherResourceKeepsUnitsIndependent(t *testing.T) {
	resetOpenMeteoWeatherResourceCache(t)

	var calls atomic.Int32
	var mu sync.Mutex
	var units []string

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		mu.Lock()
		units = append(units, request.URL.Query().Get("temperature_unit"))
		mu.Unlock()
		return openMeteoWeatherResourceTestResponse(), nil
	})

	place := &openMeteoPlaceResponseJson{
		Latitude:  38.8,
		Longitude: -90.6,
		Timezone:  "America/Chicago",
		location:  time.UTC,
	}

	if _, err := fetchOpenMeteoWeatherResource(context.Background(), place, "metric"); err != nil {
		t.Fatalf("metric fetch: %v", err)
	}
	if _, err := fetchOpenMeteoWeatherResource(context.Background(), place, "imperial"); err != nil {
		t.Fatalf("imperial fetch: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("HTTP calls = %d, want 2", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(units) != 2 || units[0] != "celsius" || units[1] != "fahrenheit" {
		t.Fatalf("temperature units = %v", units)
	}
}

func TestOpenMeteoWeatherResourceExpiresAtNextClockHour(t *testing.T) {
	resetOpenMeteoWeatherResourceCache(t)

	var calls atomic.Int32
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return openMeteoWeatherResourceTestResponse(), nil
	})

	place := &openMeteoPlaceResponseJson{
		Latitude:  38.8,
		Longitude: -90.6,
		Timezone:  "America/Chicago",
		location:  time.UTC,
	}
	key := openMeteoWeatherResourceKey{
		Latitude:  place.Latitude,
		Longitude: place.Longitude,
		Timezone:  place.Timezone,
		Units:     "metric",
	}

	if _, err := fetchOpenMeteoWeatherResource(context.Background(), place, "metric"); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	openMeteoWeatherResourceCache.mu.Lock()
	entry := openMeteoWeatherResourceCache.entries[key]
	openMeteoWeatherResourceCache.mu.Unlock()

	entry.mu.Lock()
	entry.cached.timestamp = time.Now().Add(-time.Hour)
	entry.mu.Unlock()

	if _, err := fetchOpenMeteoWeatherResource(context.Background(), place, "metric"); err != nil {
		t.Fatalf("expired fetch: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("HTTP calls = %d, want 2", got)
	}
}

func TestOpenMeteoWeatherResourceDoesNotCacheFailure(t *testing.T) {
	resetOpenMeteoWeatherResourceCache(t)

	var calls atomic.Int32
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("temporary failure")
		}
		return openMeteoWeatherResourceTestResponse(), nil
	})

	place := &openMeteoPlaceResponseJson{
		Latitude:  38.8,
		Longitude: -90.6,
		Timezone:  "America/Chicago",
		location:  time.UTC,
	}

	if _, err := fetchOpenMeteoWeatherResource(context.Background(), place, "metric"); err == nil {
		t.Fatal("first fetch unexpectedly succeeded")
	}
	if _, err := fetchOpenMeteoWeatherResource(context.Background(), place, "metric"); err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("HTTP calls = %d, want 2", got)
	}
}

func TestOpenMeteoWeatherResourceCoalescesConcurrentFetches(t *testing.T) {
	resetOpenMeteoWeatherResourceCache(t)

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return openMeteoWeatherResourceTestResponse(), nil
	})

	place := &openMeteoPlaceResponseJson{
		Latitude:  38.8,
		Longitude: -90.6,
		Timezone:  "America/Chicago",
		location:  time.UTC,
	}

	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan error, callers)

	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			_, err := fetchOpenMeteoWeatherResource(context.Background(), place, "metric")
			errs <- err
		}()
	}

	<-started
	time.Sleep(25 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("fetch error: %v", err)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1", got)
	}
}

func TestOpenMeteoWeatherResourceWaitingCallerCanCancel(t *testing.T) {
	resetOpenMeteoWeatherResourceCache(t)

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		close(started)
		<-release
		return openMeteoWeatherResourceTestResponse(), nil
	})

	place := &openMeteoPlaceResponseJson{
		Latitude:  38.8,
		Longitude: -90.6,
		Timezone:  "America/Chicago",
		location:  time.UTC,
	}

	go func() {
		_, err := fetchOpenMeteoWeatherResource(context.Background(), place, "metric")
		firstDone <- err
	}()

	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := fetchOpenMeteoWeatherResource(ctx, place, "metric"); err != context.Canceled {
		t.Fatalf("waiting fetch error = %v, want %v", err, context.Canceled)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("original fetch error: %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1", got)
	}
}

func TestOpenMeteoWeatherResourceEvictsIdleEntries(t *testing.T) {
	resetOpenMeteoWeatherResourceCache(t)

	staleKey := openMeteoWeatherResourceKey{
		Latitude:  1,
		Longitude: 2,
		Timezone:  "UTC",
		Units:     "metric",
	}
	activeKey := openMeteoWeatherResourceKey{
		Latitude:  3,
		Longitude: 4,
		Timezone:  "UTC",
		Units:     "metric",
	}

	stale := &keyedResourceCacheEntry[*openMeteoWeatherResponseJson]{
		lastUsed: time.Now().Add(-openMeteoResourceIdleRetention),
	}
	active := &keyedResourceCacheEntry[*openMeteoWeatherResponseJson]{
		lastUsed: time.Now().Add(-openMeteoResourceIdleRetention),
		current: &keyedResourceCacheCall[*openMeteoWeatherResponseJson]{
			done: make(chan struct{}),
		},
	}

	openMeteoWeatherResourceCache.mu.Lock()
	openMeteoWeatherResourceCache.entries[staleKey] = stale
	openMeteoWeatherResourceCache.entries[activeKey] = active
	openMeteoWeatherResourceCache.mu.Unlock()

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		return openMeteoWeatherResourceTestResponse(), nil
	})

	place := &openMeteoPlaceResponseJson{
		Latitude:  38.8,
		Longitude: -90.6,
		Timezone:  "America/Chicago",
		location:  time.UTC,
	}
	requestedKey := openMeteoWeatherResourceKey{
		Latitude:  place.Latitude,
		Longitude: place.Longitude,
		Timezone:  place.Timezone,
		Units:     "metric",
	}

	if _, err := fetchOpenMeteoWeatherResource(context.Background(), place, "metric"); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	openMeteoWeatherResourceCache.mu.Lock()
	_, staleExists := openMeteoWeatherResourceCache.entries[staleKey]
	_, activeExists := openMeteoWeatherResourceCache.entries[activeKey]
	_, requestedExists := openMeteoWeatherResourceCache.entries[requestedKey]
	openMeteoWeatherResourceCache.mu.Unlock()

	if staleExists {
		t.Fatal("idle stale weather entry was not evicted")
	}
	if !activeExists {
		t.Fatal("active stale weather entry was evicted")
	}
	if !requestedExists {
		t.Fatal("requested weather entry is missing")
	}
}

func TestWeatherWidgetsShareResourcesWithoutSharingWidgetConfiguration(t *testing.T) {
	resetOpenMeteoPlaceResourceCache(t)
	resetOpenMeteoWeatherResourceCache(t)

	var geocodeCalls atomic.Int32
	var forecastCalls atomic.Int32

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case "geocoding-api.open-meteo.com":
			geocodeCalls.Add(1)
			return wave3Response(
				http.StatusOK,
				`{"results":[{"name":"Saint Peters","admin1":"Missouri","latitude":38.8,"longitude":-90.6,"timezone":"America/Chicago","country":"United States"}]}`,
				nil,
			), nil

		case "api.open-meteo.com":
			forecastCalls.Add(1)
			return openMeteoWeatherResourceTestResponse(), nil

		default:
			t.Fatalf("unexpected URL %s", request.URL.String())
			return nil, nil
		}
	})

	first := &weatherWidget{
		Location:        "Saint Peters, Missouri, US",
		Units:           "imperial",
		HourFormat:      "12h",
		ShowAreaName:    true,
		ShowCurrentRaw:  boolPointer(true),
		ShowDetailsRaw:  boolPointer(false),
		ShowHourlyRaw:   boolPointer(true),
		ShowForecastRaw: boolPointer(false),
	}

	second := &weatherWidget{
		Location:        "Saint Peters, Missouri, US",
		Units:           "imperial",
		HourFormat:      "24h",
		HideLocation:    true,
		ShowCurrentRaw:  boolPointer(false),
		ShowDetailsRaw:  boolPointer(true),
		ShowHourlyRaw:   boolPointer(false),
		ShowForecastRaw: boolPointer(true),
	}

	if err := first.initialize(); err != nil {
		t.Fatalf("first initialize: %v", err)
	}
	if err := second.initialize(); err != nil {
		t.Fatalf("second initialize: %v", err)
	}

	first.update(context.Background())
	second.update(context.Background())

	if first.Error != nil {
		t.Fatalf("first widget error: %v", first.Error)
	}
	if second.Error != nil {
		t.Fatalf("second widget error: %v", second.Error)
	}

	if got := geocodeCalls.Load(); got != 1 {
		t.Fatalf("geocode HTTP calls = %d, want 1", got)
	}
	if got := forecastCalls.Load(); got != 1 {
		t.Fatalf("forecast HTTP calls = %d, want 1", got)
	}

	if first.Place == nil || second.Place == nil {
		t.Fatalf("places not populated: first=%+v second=%+v", first.Place, second.Place)
	}
	if first.Weather == nil || second.Weather == nil {
		t.Fatalf("weather not populated: first=%+v second=%+v", first.Weather, second.Weather)
	}

	if !first.ShowAreaName {
		t.Fatal("first widget lost show-area-name configuration")
	}
	if first.HideLocation {
		t.Fatal("first widget unexpectedly hides location")
	}
	if second.ShowAreaName {
		t.Fatal("second widget unexpectedly shows area name")
	}
	if !second.HideLocation {
		t.Fatal("second widget lost hide-location configuration")
	}

	if first.HourFormat != "12h" {
		t.Fatalf("first hour format = %q, want 12h", first.HourFormat)
	}
	if second.HourFormat != "24h" {
		t.Fatalf("second hour format = %q, want 24h", second.HourFormat)
	}

	if !first.ShowCurrent || first.ShowDetails || !first.ShowHourly || first.ShowForecast {
		t.Fatalf(
			"first presentation configuration changed: current=%t details=%t hourly=%t forecast=%t",
			first.ShowCurrent,
			first.ShowDetails,
			first.ShowHourly,
			first.ShowForecast,
		)
	}

	if second.ShowCurrent || !second.ShowDetails || second.ShowHourly || !second.ShowForecast {
		t.Fatalf(
			"second presentation configuration changed: current=%t details=%t hourly=%t forecast=%t",
			second.ShowCurrent,
			second.ShowDetails,
			second.ShowHourly,
			second.ShowForecast,
		)
	}

	if first.TimeLabels[0] == second.TimeLabels[0] {
		t.Fatalf(
			"independent hour formats produced identical first label %q",
			first.TimeLabels[0],
		)
	}
}
