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

	openMeteoPlaceResourceCache.Lock()
	old := openMeteoPlaceResourceCache.entries
	openMeteoPlaceResourceCache.entries = make(map[string]*openMeteoPlaceResourceCacheEntry)
	openMeteoPlaceResourceCache.Unlock()

	t.Cleanup(func() {
		openMeteoPlaceResourceCache.Lock()
		openMeteoPlaceResourceCache.entries = old
		openMeteoPlaceResourceCache.Unlock()
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

	stale := &openMeteoPlaceResourceCacheEntry{
		lastUsed: time.Now().Add(-openMeteoResourceIdleRetention),
	}
	active := &openMeteoPlaceResourceCacheEntry{
		lastUsed: time.Now().Add(-openMeteoResourceIdleRetention),
		current: &openMeteoPlaceResourceCall{
			done: make(chan struct{}),
		},
	}

	openMeteoPlaceResourceCache.Lock()
	openMeteoPlaceResourceCache.entries["stale"] = stale
	openMeteoPlaceResourceCache.entries["active"] = active
	openMeteoPlaceResourceCache.Unlock()

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

	openMeteoPlaceResourceCache.Lock()
	_, staleExists := openMeteoPlaceResourceCache.entries["stale"]
	_, activeExists := openMeteoPlaceResourceCache.entries["active"]
	_, requestedExists := openMeteoPlaceResourceCache.entries[location]
	openMeteoPlaceResourceCache.Unlock()

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

	openMeteoWeatherResourceCache.Lock()
	old := openMeteoWeatherResourceCache.entries
	openMeteoWeatherResourceCache.entries = make(map[openMeteoWeatherResourceKey]*openMeteoWeatherResourceCacheEntry)
	openMeteoWeatherResourceCache.Unlock()

	t.Cleanup(func() {
		openMeteoWeatherResourceCache.Lock()
		openMeteoWeatherResourceCache.entries = old
		openMeteoWeatherResourceCache.Unlock()
	})
}

func openMeteoWeatherResourceTestResponse() *http.Response {
	temps := make([]string, 24)
	precip := make([]string, 24)

	for i := range temps {
		temps[i] = "20"
		precip[i] = "0"
	}

	body := `{"daily":{"sunrise":[0],"sunset":[3600]},"hourly":{"temperature_2m":[` +
		strings.Join(temps, ",") +
		`],"precipitation_probability":[` +
		strings.Join(precip, ",") +
		`]},"current":{"temperature_2m":20,"apparent_temperature":19,"weather_code":1}}`

	return wave3Response(http.StatusOK, body, nil)
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

	openMeteoWeatherResourceCache.Lock()
	entry := openMeteoWeatherResourceCache.entries[key]
	openMeteoWeatherResourceCache.Unlock()

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

	stale := &openMeteoWeatherResourceCacheEntry{
		lastUsed: time.Now().Add(-openMeteoResourceIdleRetention),
	}
	active := &openMeteoWeatherResourceCacheEntry{
		lastUsed: time.Now().Add(-openMeteoResourceIdleRetention),
		current: &openMeteoWeatherResourceCall{
			done: make(chan struct{}),
		},
	}

	openMeteoWeatherResourceCache.Lock()
	openMeteoWeatherResourceCache.entries[staleKey] = stale
	openMeteoWeatherResourceCache.entries[activeKey] = active
	openMeteoWeatherResourceCache.Unlock()

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

	openMeteoWeatherResourceCache.Lock()
	_, staleExists := openMeteoWeatherResourceCache.entries[staleKey]
	_, activeExists := openMeteoWeatherResourceCache.entries[activeKey]
	_, requestedExists := openMeteoWeatherResourceCache.entries[requestedKey]
	openMeteoWeatherResourceCache.Unlock()

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
		Location:     "Saint Peters, Missouri, US",
		Units:        "imperial",
		HourFormat:   "12h",
		ShowAreaName: true,
	}

	second := &weatherWidget{
		Location:     "Saint Peters, Missouri, US",
		Units:        "imperial",
		HourFormat:   "24h",
		HideLocation: true,
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

	if first.TimeLabels[0] == second.TimeLabels[0] {
		t.Fatalf(
			"independent hour formats produced identical first label %q",
			first.TimeLabels[0],
		)
	}
}
