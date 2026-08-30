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
