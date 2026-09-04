package glance

import (
	"context"
	"errors"
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
		Location: "Canceled Location",
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
