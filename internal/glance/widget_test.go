package glance

import (
	"context"
	"html/template"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type refreshTestWidget struct {
	widgetBase

	updateCount atomic.Int32
	updateStart chan struct{}
	updateBlock chan struct{}
	startOnce   sync.Once
}

func newRefreshTestWidget() *refreshTestWidget {
	widget := &refreshTestWidget{
		updateStart: make(chan struct{}),
		updateBlock: make(chan struct{}),
	}

	widget.Type = "refresh-test"
	widget.withCacheDuration(time.Hour)

	return widget
}

func (widget *refreshTestWidget) initialize() error {
	return nil
}

func (widget *refreshTestWidget) update(ctx context.Context) {
	widget.updateCount.Add(1)
	widget.startOnce.Do(func() {
		close(widget.updateStart)
	})

	select {
	case <-widget.updateBlock:
	case <-ctx.Done():
		return
	}

	widget.scheduleNextUpdate()
}

func (widget *refreshTestWidget) Render() template.HTML {
	return ""
}

func TestRefreshWidgetIfNeededSingleFlight(t *testing.T) {
	widget := newRefreshTestWidget()
	now := time.Now()

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		refreshWidgetIfNeeded(context.Background(), widget, &now)
	}()

	select {
	case <-widget.updateStart:
	case <-time.After(time.Second):
		t.Fatal("first refresh did not start")
	}

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		refreshWidgetIfNeeded(context.Background(), widget, &now)
	}()

	select {
	case <-secondDone:
		t.Fatal("second refresh completed while first refresh still held the widget lock")
	case <-time.After(25 * time.Millisecond):
	}

	close(widget.updateBlock)

	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first refresh did not complete")
	}

	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("second refresh did not complete")
	}

	if got := widget.updateCount.Load(); got != 1 {
		t.Fatalf("update count = %d, want 1", got)
	}
}

func TestRefreshWidgetIfNeededDoesNotSerializeDifferentWidgets(t *testing.T) {
	first := newRefreshTestWidget()
	second := newRefreshTestWidget()
	now := time.Now()

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		refreshWidgetIfNeeded(context.Background(), first, &now)
	}()

	select {
	case <-first.updateStart:
	case <-time.After(time.Second):
		t.Fatal("first widget refresh did not start")
	}

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		refreshWidgetIfNeeded(context.Background(), second, &now)
	}()

	select {
	case <-second.updateStart:
	case <-time.After(time.Second):
		t.Fatal("second widget refresh was blocked by first widget")
	}

	close(first.updateBlock)
	close(second.updateBlock)

	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first widget refresh did not complete")
	}

	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("second widget refresh did not complete")
	}

	if got := first.updateCount.Load(); got != 1 {
		t.Fatalf("first widget update count = %d, want 1", got)
	}

	if got := second.updateCount.Load(); got != 1 {
		t.Fatalf("second widget update count = %d, want 1", got)
	}
}
