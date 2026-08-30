package glance

import (
	"context"
	"errors"
	"testing"
)

func TestFetchTopGamesFromTwitchCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchTopGamesFromTwitch(ctx, nil, 10)
	if err == nil {
		t.Fatal("expected canceled Twitch top games request to return an error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}
