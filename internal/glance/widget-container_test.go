package glance

import (
	"context"
	"errors"
	"testing"
)

func TestRefreshNestedWidgetsCancellationAndSiblingProgress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	child := newRefreshTestWidget()
	err := refreshNestedWidgets(ctx, widgets{child})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled refresh error = %v", err)
	}
	if got := child.updateCount.Load(); got != 0 {
		t.Fatalf("cancelled child updated %d times", got)
	}
	first := newRefreshTestWidget()
	second := newRefreshTestWidget()
	close(first.updateBlock)
	close(second.updateBlock)
	if err := refreshNestedWidgets(context.Background(), widgets{first, second}); err != nil {
		t.Fatalf("refresh siblings: %v", err)
	}
	if first.updateCount.Load() != 1 || second.updateCount.Load() != 1 {
		t.Fatalf("sibling updates = %d, %d", first.updateCount.Load(), second.updateCount.Load())
	}
}
