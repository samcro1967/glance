package glance

import (
	"context"
	"time"
)

const openMeteoResourceIdleRetention = 24 * time.Hour

var openMeteoPlaceResourceCache = newKeyedResourceCache[string, *openMeteoPlaceResponseJson](
	openMeteoResourceIdleRetention,
)

func fetchOpenMeteoPlaceResource(ctx context.Context, location string, timeouts ...durationField) (*openMeteoPlaceResponseJson, error) {
	return openMeteoPlaceResourceCache.Get(
		ctx,
		resourceTimeoutKey(location, timeouts...),
		func(cachedEntry[*openMeteoPlaceResponseJson], time.Time) bool {
			return true
		},
		func(ctx context.Context) (*openMeteoPlaceResponseJson, error) {
			return fetchOpenMeteoPlaceFromName(ctx, location, timeouts...)
		},
	)
}

type openMeteoWeatherResourceKey struct {
	Timeout   durationField
	Latitude  float64
	Longitude float64
	Timezone  string
	Units     string
}

var openMeteoWeatherResourceCache = newKeyedResourceCache[openMeteoWeatherResourceKey, *openMeteoWeatherResponseJson](
	openMeteoResourceIdleRetention,
)

func fetchOpenMeteoWeatherResource(ctx context.Context, place *openMeteoPlaceResponseJson, units string, timeouts ...durationField) (*weather, error) {
	key := openMeteoWeatherResourceKey{
		Timeout:   resourceTimeoutPolicy(timeouts...),
		Latitude:  place.Latitude,
		Longitude: place.Longitude,
		Timezone:  place.Timezone,
		Units:     units,
	}

	responseJson, err := openMeteoWeatherResourceCache.Get(
		ctx,
		key,
		func(cached cachedEntry[*openMeteoWeatherResponseJson], now time.Time) bool {
			return sameClockHour(cached.timestamp, now)
		},
		func(ctx context.Context) (*openMeteoWeatherResponseJson, error) {
			return fetchOpenMeteoWeatherResponse(ctx, place, units, timeouts...)
		},
	)
	if err != nil {
		return nil, err
	}

	return buildWeatherFromOpenMeteoResponse(responseJson, place), nil
}

func sameClockHour(a, b time.Time) bool {
	return a.Year() == b.Year() &&
		a.YearDay() == b.YearDay() &&
		a.Hour() == b.Hour()
}
