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

type renderSynchronizationTestWidget struct {
	widgetBase

	updateStart chan struct{}
	updateBlock chan struct{}
	renderStart chan struct{}
	renderBlock chan struct{}

	updateOnce sync.Once
	renderOnce sync.Once
}

func newRenderSynchronizationTestWidget() *renderSynchronizationTestWidget {
	widget := &renderSynchronizationTestWidget{
		updateStart: make(chan struct{}),
		updateBlock: make(chan struct{}),
		renderStart: make(chan struct{}),
		renderBlock: make(chan struct{}),
	}

	widget.Type = "render-synchronization-test"
	widget.withCacheDuration(time.Hour)

	return widget
}

func (widget *renderSynchronizationTestWidget) initialize() error {
	return nil
}

func (widget *renderSynchronizationTestWidget) update(ctx context.Context) {
	widget.updateOnce.Do(func() {
		close(widget.updateStart)
	})

	select {
	case <-widget.updateBlock:
	case <-ctx.Done():
		return
	}

	widget.scheduleNextUpdate()
}

func (widget *renderSynchronizationTestWidget) Render() template.HTML {
	widget.renderOnce.Do(func() {
		close(widget.renderStart)
	})

	<-widget.renderBlock

	return ""
}

func TestRenderWidgetWaitsForActiveRefresh(t *testing.T) {
	widget := newRenderSynchronizationTestWidget()
	now := time.Now()

	refreshDone := make(chan struct{})
	go func() {
		defer close(refreshDone)
		refreshWidgetIfNeeded(context.Background(), widget, &now)
	}()

	select {
	case <-widget.updateStart:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}

	renderDone := make(chan struct{})
	go func() {
		defer close(renderDone)
		renderWidget(widget)
	}()

	select {
	case <-widget.renderStart:
		t.Fatal("render started while refresh still held the widget lock")
	case <-time.After(25 * time.Millisecond):
	}

	close(widget.updateBlock)

	select {
	case <-refreshDone:
	case <-time.After(time.Second):
		t.Fatal("refresh did not complete")
	}

	select {
	case <-widget.renderStart:
	case <-time.After(time.Second):
		t.Fatal("render did not start after refresh completed")
	}

	close(widget.renderBlock)

	select {
	case <-renderDone:
	case <-time.After(time.Second):
		t.Fatal("render did not complete")
	}
}

func TestRefreshWidgetWaitsForActiveRender(t *testing.T) {
	widget := newRenderSynchronizationTestWidget()
	now := time.Now()

	renderDone := make(chan struct{})
	go func() {
		defer close(renderDone)
		renderWidget(widget)
	}()

	select {
	case <-widget.renderStart:
	case <-time.After(time.Second):
		t.Fatal("render did not start")
	}

	refreshDone := make(chan struct{})
	go func() {
		defer close(refreshDone)
		refreshWidgetIfNeeded(context.Background(), widget, &now)
	}()

	select {
	case <-widget.updateStart:
		t.Fatal("refresh started while render still held the widget lock")
	case <-time.After(25 * time.Millisecond):
	}

	close(widget.renderBlock)

	select {
	case <-renderDone:
	case <-time.After(time.Second):
		t.Fatal("render did not complete")
	}

	select {
	case <-widget.updateStart:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start after render completed")
	}

	close(widget.updateBlock)

	select {
	case <-refreshDone:
	case <-time.After(time.Second):
		t.Fatal("refresh did not complete")
	}
}
