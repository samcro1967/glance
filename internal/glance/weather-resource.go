package glance

import (
	"context"
	"sync"
	"time"
)

type openMeteoPlaceResourceCall struct {
	done chan struct{}
	val  *openMeteoPlaceResponseJson
	err  error
}

type openMeteoPlaceResourceCacheEntry struct {
	mu      sync.Mutex
	cached  cachedEntry[*openMeteoPlaceResponseJson]
	current *openMeteoPlaceResourceCall
}

var openMeteoPlaceResourceCache = struct {
	sync.Mutex
	entries map[string]*openMeteoPlaceResourceCacheEntry
}{
	entries: make(map[string]*openMeteoPlaceResourceCacheEntry),
}

func fetchOpenMeteoPlaceResource(ctx context.Context, location string) (*openMeteoPlaceResponseJson, error) {
	openMeteoPlaceResourceCache.Lock()
	entry, ok := openMeteoPlaceResourceCache.entries[location]
	if !ok {
		entry = &openMeteoPlaceResourceCacheEntry{}
		openMeteoPlaceResourceCache.entries[location] = entry
	}
	openMeteoPlaceResourceCache.Unlock()

	return entry.fetch(ctx, location)
}

func (entry *openMeteoPlaceResourceCacheEntry) fetch(ctx context.Context, location string) (*openMeteoPlaceResponseJson, error) {
	entry.mu.Lock()

	if entry.cached.value != nil {
		value := entry.cached.value
		entry.mu.Unlock()
		return value, nil
	}

	if entry.current != nil {
		call := entry.current
		entry.mu.Unlock()

		select {
		case <-call.done:
			return call.val, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	call := &openMeteoPlaceResourceCall{done: make(chan struct{})}
	entry.current = call
	entry.mu.Unlock()

	call.val, call.err = fetchOpenMeteoPlaceFromName(ctx, location)

	entry.mu.Lock()
	if call.err == nil {
		entry.cached = cachedEntry[*openMeteoPlaceResponseJson]{
			value:     call.val,
			timestamp: time.Now(),
		}
	}
	entry.current = nil
	close(call.done)
	entry.mu.Unlock()

	return call.val, call.err
}

type openMeteoWeatherResourceKey struct {
	Latitude  float64
	Longitude float64
	Timezone  string
	Units     string
}

type openMeteoWeatherResourceCall struct {
	done chan struct{}
	val  *openMeteoWeatherResponseJson
	err  error
}

type openMeteoWeatherResourceCacheEntry struct {
	mu      sync.Mutex
	cached  cachedEntry[*openMeteoWeatherResponseJson]
	current *openMeteoWeatherResourceCall
}

var openMeteoWeatherResourceCache = struct {
	sync.Mutex
	entries map[openMeteoWeatherResourceKey]*openMeteoWeatherResourceCacheEntry
}{
	entries: make(map[openMeteoWeatherResourceKey]*openMeteoWeatherResourceCacheEntry),
}

func fetchOpenMeteoWeatherResource(ctx context.Context, place *openMeteoPlaceResponseJson, units string) (*weather, error) {
	key := openMeteoWeatherResourceKey{
		Latitude:  place.Latitude,
		Longitude: place.Longitude,
		Timezone:  place.Timezone,
		Units:     units,
	}

	openMeteoWeatherResourceCache.Lock()
	entry, ok := openMeteoWeatherResourceCache.entries[key]
	if !ok {
		entry = &openMeteoWeatherResourceCacheEntry{}
		openMeteoWeatherResourceCache.entries[key] = entry
	}
	openMeteoWeatherResourceCache.Unlock()

	responseJson, err := entry.fetch(ctx, place, units)
	if err != nil {
		return nil, err
	}

	return buildWeatherFromOpenMeteoResponse(responseJson, place), nil
}

func (entry *openMeteoWeatherResourceCacheEntry) fetch(ctx context.Context, place *openMeteoPlaceResponseJson, units string) (*openMeteoWeatherResponseJson, error) {
	entry.mu.Lock()

	now := time.Now()
	if entry.cached.value != nil && sameClockHour(entry.cached.timestamp, now) {
		value := entry.cached.value
		entry.mu.Unlock()
		return value, nil
	}

	if entry.current != nil {
		call := entry.current
		entry.mu.Unlock()

		select {
		case <-call.done:
			return call.val, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	call := &openMeteoWeatherResourceCall{done: make(chan struct{})}
	entry.current = call
	entry.mu.Unlock()

	call.val, call.err = fetchOpenMeteoWeatherResponse(ctx, place, units)

	entry.mu.Lock()
	if call.err == nil {
		entry.cached = cachedEntry[*openMeteoWeatherResponseJson]{
			value:     call.val,
			timestamp: time.Now(),
		}
	}
	entry.current = nil
	close(call.done)
	entry.mu.Unlock()

	return call.val, call.err
}

func sameClockHour(a, b time.Time) bool {
	return a.Year() == b.Year() &&
		a.YearDay() == b.YearDay() &&
		a.Hour() == b.Hour()
}
