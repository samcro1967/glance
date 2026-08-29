package glance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
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

func TestWidgetRefreshFailureLogsTransitionOnce(t *testing.T) {
	widget := &widgetBase{
		ID:    42,
		Type:  "test-widget",
		Title: "Test Widget",
	}
	widget.withCacheDuration(time.Hour)

	var logOutput bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	err := errors.New("temporary failure")

	if widget.canContinueUpdateAfterHandlingErr(err) {
		t.Fatal("full refresh failure should not continue update")
	}

	if !widget.refreshDegraded {
		t.Fatal("widget should be marked degraded after refresh failure")
	}

	firstLog := logOutput.String()

	if !strings.Contains(firstLog, `level=WARN msg="Widget refresh failed"`) {
		t.Fatalf("missing refresh failure warning: %q", firstLog)
	}

	if !strings.Contains(firstLog, "widget_id=42") {
		t.Fatalf("missing widget ID: %q", firstLog)
	}

	if !strings.Contains(firstLog, "type=test-widget") {
		t.Fatalf("missing widget type: %q", firstLog)
	}

	if !strings.Contains(firstLog, `title="Test Widget"`) {
		t.Fatalf("missing widget title: %q", firstLog)
	}

	if !strings.Contains(firstLog, "retry_attempt=1") {
		t.Fatalf("missing retry attempt: %q", firstLog)
	}

	if !strings.Contains(firstLog, "next_update=") {
		t.Fatalf("missing next update: %q", firstLog)
	}

	if strings.Contains(firstLog, err.Error()) {
		t.Fatalf("transition warning should not duplicate underlying error: %q", firstLog)
	}

	logOutput.Reset()

	if widget.canContinueUpdateAfterHandlingErr(err) {
		t.Fatal("repeated full refresh failure should not continue update")
	}

	if logOutput.Len() != 0 {
		t.Fatalf("repeated failure emitted another transition warning: %q", logOutput.String())
	}

	if widget.updateRetriedTimes != 2 {
		t.Fatalf("retry attempts = %d, want 2", widget.updateRetriedTimes)
	}
}

func TestWidgetRefreshRecoveryLogsTransition(t *testing.T) {
	widget := &widgetBase{
		ID:    43,
		Type:  "test-widget",
		Title: "Recovery Test",
	}
	widget.withCacheDuration(time.Hour)

	var logOutput bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	widget.canContinueUpdateAfterHandlingErr(errors.New("temporary failure"))
	logOutput.Reset()

	if !widget.canContinueUpdateAfterHandlingErr(nil) {
		t.Fatal("successful refresh should continue update")
	}

	if widget.refreshDegraded {
		t.Fatal("widget should no longer be degraded after recovery")
	}

	if widget.updateRetriedTimes != 0 {
		t.Fatalf("retry attempts = %d, want 0 after recovery", widget.updateRetriedTimes)
	}

	recoveryLog := logOutput.String()

	if !strings.Contains(recoveryLog, `level=INFO msg="Widget recovered"`) {
		t.Fatalf("missing recovery log: %q", recoveryLog)
	}

	if !strings.Contains(recoveryLog, "widget_id=43") {
		t.Fatalf("missing widget ID: %q", recoveryLog)
	}

	if !strings.Contains(recoveryLog, "type=test-widget") {
		t.Fatalf("missing widget type: %q", recoveryLog)
	}

	if !strings.Contains(recoveryLog, `title="Recovery Test"`) {
		t.Fatalf("missing widget title: %q", recoveryLog)
	}

	logOutput.Reset()

	if !widget.canContinueUpdateAfterHandlingErr(nil) {
		t.Fatal("subsequent successful refresh should continue update")
	}

	if logOutput.Len() != 0 {
		t.Fatalf("healthy refresh emitted transition log: %q", logOutput.String())
	}
}

func TestWidgetPartialRefreshLogsDegradedAndRecovery(t *testing.T) {
	widget := &widgetBase{
		ID:    44,
		Type:  "test-widget",
		Title: "Partial Test",
	}
	widget.withCacheDuration(time.Hour)

	var logOutput bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	partialErr := fmt.Errorf("%w: some content unavailable", errPartialContent)

	if !widget.canContinueUpdateAfterHandlingErr(partialErr) {
		t.Fatal("partial-content refresh should continue update")
	}

	if !widget.refreshDegraded {
		t.Fatal("widget should be marked degraded after partial refresh")
	}

	if widget.Error != nil {
		t.Fatalf("partial-content refresh should not set widget error: %v", widget.Error)
	}

	if widget.Notice == nil {
		t.Fatal("partial-content refresh should set widget notice")
	}

	degradedLog := logOutput.String()

	if !strings.Contains(degradedLog, `level=WARN msg="Widget refresh degraded"`) {
		t.Fatalf("missing degraded warning: %q", degradedLog)
	}

	if !strings.Contains(degradedLog, "widget_id=44") {
		t.Fatalf("missing widget ID: %q", degradedLog)
	}

	logOutput.Reset()

	if !widget.canContinueUpdateAfterHandlingErr(nil) {
		t.Fatal("successful refresh should continue update")
	}

	if widget.refreshDegraded {
		t.Fatal("widget should no longer be degraded after recovery")
	}

	if widget.Notice != nil {
		t.Fatalf("widget notice should clear after recovery: %v", widget.Notice)
	}

	if !strings.Contains(logOutput.String(), `level=INFO msg="Widget recovered"`) {
		t.Fatalf("missing recovery log after partial-content failure: %q", logOutput.String())
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
