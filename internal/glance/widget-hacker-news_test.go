package glance

import (
	"context"
	"errors"
	"testing"
)

func TestFetchHackerNewsPostIdsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchHackerNewsPostIds(ctx, "top")
	if err == nil {
		t.Fatal("expected canceled Hacker News post ID request to return an error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}

func TestFetchHackerNewsPostsFromIdsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchHackerNewsPostsFromIds(
		ctx,
		[]int{
			1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
			11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
			21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
			31,
		},
		"",
	)
	if err == nil {
		t.Fatal("expected canceled Hacker News posts job to return an error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}

func TestFetchHackerNewsPostsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchHackerNewsPosts(ctx, "top", 40, "")
	if err == nil {
		t.Fatal("expected canceled Hacker News fetch to return an error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}
