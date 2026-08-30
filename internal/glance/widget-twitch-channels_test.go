package glance

import (
	"context"
	"errors"
	"testing"
)

func TestFetchChannelFromTwitchTaskCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchChannelFromTwitchTask(ctx, "test-channel")
	if err == nil {
		t.Fatal("expected canceled Twitch channel request to return an error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}

func TestFetchChannelsFromTwitchCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchChannelsFromTwitch(ctx, []string{
		"channel-1",
		"channel-2",
		"channel-3",
		"channel-4",
		"channel-5",
		"channel-6",
		"channel-7",
		"channel-8",
		"channel-9",
		"channel-10",
		"channel-11",
		"channel-12",
	})

	if err == nil {
		t.Fatal("expected canceled Twitch channels job to return an error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}
