package glance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
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

func TestPageUpdateOutdatedWidgetsPropagatesCancellation(t *testing.T) {
	testWidget := newRefreshTestWidget()
	page := &page{
		HeadWidgets: []widget{testWidget},
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		page.updateOutdatedWidgets(ctx)
	}()

	select {
	case <-testWidget.updateStart:
	case <-time.After(time.Second):
		t.Fatal("widget refresh did not start")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("page refresh did not stop after context cancellation")
	}

	if got := testWidget.updateCount.Load(); got != 1 {
		t.Fatalf("update count = %d, want 1", got)
	}
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

	if !strings.Contains(firstLog, `cause="temporary failure"`) {
		t.Fatalf("missing refresh failure cause: %q", firstLog)
	}

	if !strings.Contains(firstLog, "retry_attempt=1") {
		t.Fatalf("missing retry attempt: %q", firstLog)
	}

	if !strings.Contains(firstLog, "next_update=") {
		t.Fatalf("missing next update: %q", firstLog)
	}

	if strings.Count(firstLog, err.Error()) != 1 {
		t.Fatalf("refresh failure cause should appear exactly once: %q", firstLog)
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

	if strings.Contains(recoveryLog, "cause=") {
		t.Fatalf("recovery log should not contain a cause: %q", recoveryLog)
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

	if !strings.Contains(degradedLog, "type=test-widget") {
		t.Fatalf("missing widget type: %q", degradedLog)
	}

	if !strings.Contains(degradedLog, `title="Partial Test"`) {
		t.Fatalf("missing widget title: %q", degradedLog)
	}

	expectedCause := fmt.Sprintf("cause=%q", partialErr.Error())
	if !strings.Contains(degradedLog, expectedCause) {
		t.Fatalf("missing partial refresh cause %q: %q", partialErr.Error(), degradedLog)
	}

	if strings.Count(degradedLog, partialErr.Error()) != 1 {
		t.Fatalf("partial refresh cause should appear exactly once: %q", degradedLog)
	}

	logOutput.Reset()

	if !widget.canContinueUpdateAfterHandlingErr(partialErr) {
		t.Fatal("repeated partial-content refresh should continue update")
	}

	if logOutput.Len() != 0 {
		t.Fatalf("repeated partial failure emitted another transition warning: %q", logOutput.String())
	}

	if widget.updateRetriedTimes != 2 {
		t.Fatalf("retry attempts = %d, want 2", widget.updateRetriedTimes)
	}

	if widget.Error != nil {
		t.Fatalf("repeated partial-content refresh should not set widget error: %v", widget.Error)
	}

	if widget.Notice == nil {
		t.Fatal("repeated partial-content refresh should retain widget notice")
	}

	if !errors.Is(widget.Notice, errPartialContent) {
		t.Fatalf("widget notice should preserve partial-content classification: %v", widget.Notice)
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

	if widget.updateRetriedTimes != 0 {
		t.Fatalf("retry attempts = %d, want 0 after recovery", widget.updateRetriedTimes)
	}

	recoveryLog := logOutput.String()

	if !strings.Contains(recoveryLog, `level=INFO msg="Widget recovered"`) {
		t.Fatalf("missing recovery log after partial-content failure: %q", recoveryLog)
	}

	if !strings.Contains(recoveryLog, "widget_id=44") {
		t.Fatalf("missing widget ID in recovery log: %q", recoveryLog)
	}

	if !strings.Contains(recoveryLog, "type=test-widget") {
		t.Fatalf("missing widget type in recovery log: %q", recoveryLog)
	}

	if !strings.Contains(recoveryLog, `title="Partial Test"`) {
		t.Fatalf("missing widget title in recovery log: %q", recoveryLog)
	}

	if strings.Contains(recoveryLog, "cause=") {
		t.Fatalf("recovery log should not contain a cause: %q", recoveryLog)
	}

	logOutput.Reset()

	if !widget.canContinueUpdateAfterHandlingErr(nil) {
		t.Fatal("subsequent successful refresh should continue update")
	}

	if logOutput.Len() != 0 {
		t.Fatalf("healthy refresh emitted transition log: %q", logOutput.String())
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

func TestWidgetRefreshRetryPolicyByFailureClass(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantRetryCount int
		wantClass      refreshFailureClass
		wantRetryLog   string
	}{
		{
			name:           "transient server failure retries early",
			err:            unexpectedHTTPStatusError(&http.Response{StatusCode: http.StatusBadGateway, Status: "502 Bad Gateway"}),
			wantRetryCount: 1,
			wantClass:      refreshFailureTransient,
			wantRetryLog:   "retry=true",
		},
		{
			name:           "authentication failure uses normal schedule",
			err:            unexpectedHTTPStatusError(&http.Response{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized"}),
			wantRetryCount: 0,
			wantClass:      refreshFailureAuthentication,
			wantRetryLog:   "retry=false",
		},
		{
			name:           "authorization failure uses normal schedule",
			err:            unexpectedHTTPStatusError(&http.Response{StatusCode: http.StatusForbidden, Status: "403 Forbidden"}),
			wantRetryCount: 0,
			wantClass:      refreshFailureAuthorization,
			wantRetryLog:   "retry=false",
		},
		{
			name:           "rate limit uses normal schedule",
			err:            unexpectedHTTPStatusError(&http.Response{StatusCode: http.StatusTooManyRequests, Status: "429 Too Many Requests"}),
			wantRetryCount: 0,
			wantClass:      refreshFailureRateLimited,
			wantRetryLog:   "retry=false",
		},
		{
			name:           "persistent request failure uses normal schedule",
			err:            unexpectedHTTPStatusError(&http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found"}),
			wantRetryCount: 0,
			wantClass:      refreshFailureRequest,
			wantRetryLog:   "retry=false",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			widget := &widgetBase{
				ID:    50,
				Type:  "test-widget",
				Title: "Retry Policy",
			}
			widget.withCacheDuration(time.Hour)

			var logOutput bytes.Buffer
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
			t.Cleanup(func() {
				slog.SetDefault(previousLogger)
			})

			if widget.canContinueUpdateAfterHandlingErr(test.err) {
				t.Fatal("full refresh failure should not continue update")
			}

			if widget.updateRetriedTimes != test.wantRetryCount {
				t.Fatalf(
					"retry attempts = %d, want %d",
					widget.updateRetriedTimes,
					test.wantRetryCount,
				)
			}

			if widget.refreshFailureClass != test.wantClass {
				t.Fatalf(
					"failure class = %q, want %q",
					widget.refreshFailureClass,
					test.wantClass,
				)
			}

			logged := logOutput.String()
			if !strings.Contains(logged, "failure_class="+string(test.wantClass)) {
				t.Fatalf("missing failure class in log: %q", logged)
			}

			if !strings.Contains(logged, test.wantRetryLog) {
				t.Fatalf("missing retry decision in log: %q", logged)
			}
		})
	}
}

func TestWidgetPartialRefreshRetryPolicy(t *testing.T) {
	tests := []struct {
		name           string
		cause          error
		wantRetryCount int
		wantRetryLog   string
	}{
		{
			name:           "transient partial failure retries early",
			cause:          unexpectedHTTPStatusError(&http.Response{StatusCode: http.StatusServiceUnavailable, Status: "503 Service Unavailable"}),
			wantRetryCount: 1,
			wantRetryLog:   "retry=true",
		},
		{
			name:           "rate limited partial failure uses normal schedule",
			cause:          unexpectedHTTPStatusError(&http.Response{StatusCode: http.StatusTooManyRequests, Status: "429 Too Many Requests"}),
			wantRetryCount: 0,
			wantRetryLog:   "retry=false",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			widget := &widgetBase{
				ID:    51,
				Type:  "test-widget",
				Title: "Partial Retry Policy",
			}
			widget.withCacheDuration(time.Hour)

			var logOutput bytes.Buffer
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
			t.Cleanup(func() {
				slog.SetDefault(previousLogger)
			})

			err := contentFetchError(
				errPartialContent,
				1,
				2,
				"resources",
				test.cause,
			)

			if !widget.canContinueUpdateAfterHandlingErr(err) {
				t.Fatal("partial refresh should continue update")
			}

			if widget.updateRetriedTimes != test.wantRetryCount {
				t.Fatalf(
					"retry attempts = %d, want %d",
					widget.updateRetriedTimes,
					test.wantRetryCount,
				)
			}

			if widget.Error != nil {
				t.Fatalf("partial refresh set widget error: %v", widget.Error)
			}

			if !errors.Is(widget.Notice, errPartialContent) {
				t.Fatalf("partial refresh notice = %v, want errPartialContent", widget.Notice)
			}

			if !strings.Contains(logOutput.String(), test.wantRetryLog) {
				t.Fatalf("missing retry decision in log: %q", logOutput.String())
			}
		})
	}
}

func TestWidgetRefreshCancellationIsLifecycleNeutral(t *testing.T) {
	widget := &widgetBase{
		ID:    52,
		Type:  "test-widget",
		Title: "Cancellation",
	}
	widget.withCacheDuration(time.Hour)

	originalNextUpdate := time.Now().Add(30 * time.Minute)
	widget.nextUpdate = originalNextUpdate

	var logOutput bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	if widget.canContinueUpdateAfterHandlingErr(context.Canceled) {
		t.Fatal("cancelled refresh should not continue update")
	}

	if widget.refreshDegraded {
		t.Fatal("cancelled refresh should not mark widget degraded")
	}

	if widget.updateRetriedTimes != 0 {
		t.Fatalf("cancelled refresh retry attempts = %d, want 0", widget.updateRetriedTimes)
	}

	if widget.refreshFailureCount != 0 {
		t.Fatalf("cancelled refresh failure count = %d, want 0", widget.refreshFailureCount)
	}

	if !widget.nextUpdate.Equal(originalNextUpdate) {
		t.Fatalf(
			"cancelled refresh changed next update: got %v want %v",
			widget.nextUpdate,
			originalNextUpdate,
		)
	}

	if logOutput.Len() != 0 {
		t.Fatalf("cancelled refresh emitted operational log: %q", logOutput.String())
	}
}

func TestWidgetRecoveryLogsFailureContextOnce(t *testing.T) {
	widget := &widgetBase{
		ID:    53,
		Type:  "test-widget",
		Title: "Failure Context",
	}
	widget.withCacheDuration(time.Hour)

	var logOutput bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	failure := unexpectedHTTPStatusError(&http.Response{
		StatusCode: http.StatusBadGateway,
		Status:     "502 Bad Gateway",
	})

	widget.canContinueUpdateAfterHandlingErr(failure)
	widget.canContinueUpdateAfterHandlingErr(failure)

	if widget.refreshFailureCount != 2 {
		t.Fatalf("failure count = %d, want 2", widget.refreshFailureCount)
	}

	logOutput.Reset()

	if !widget.canContinueUpdateAfterHandlingErr(nil) {
		t.Fatal("successful refresh should continue update")
	}

	recoveryLog := logOutput.String()

	if !strings.Contains(recoveryLog, "previous_failure_class=transient") {
		t.Fatalf("missing previous failure class: %q", recoveryLog)
	}

	if !strings.Contains(recoveryLog, "previous_failures=2") {
		t.Fatalf("missing previous failure count: %q", recoveryLog)
	}

	if widget.refreshFailureCount != 0 {
		t.Fatalf("failure count = %d, want 0 after recovery", widget.refreshFailureCount)
	}

	if widget.refreshFailureClass != refreshFailureUnknown {
		t.Fatalf(
			"failure class = %q, want %q after recovery",
			widget.refreshFailureClass,
			refreshFailureUnknown,
		)
	}

	logOutput.Reset()

	widget.canContinueUpdateAfterHandlingErr(nil)

	if logOutput.Len() != 0 {
		t.Fatalf("subsequent healthy refresh emitted recovery log: %q", logOutput.String())
	}
}

func TestWidgetRefreshFailureLogsContentState(t *testing.T) {
	tests := []struct {
		name             string
		contentAvailable bool
		wantContent      string
	}{
		{
			name:             "first load has no content",
			contentAvailable: false,
			wantContent:      "no-content",
		},
		{
			name:             "failed refresh preserves stale content",
			contentAvailable: true,
			wantContent:      "stale",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			widget := &widgetBase{
				ID:               42,
				Type:             "test",
				Title:            "Test Widget",
				ContentAvailable: test.contentAvailable,
			}
			widget.withCacheDuration(time.Hour)

			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			previousLogger := slog.Default()
			slog.SetDefault(logger)
			t.Cleanup(func() {
				slog.SetDefault(previousLogger)
			})

			err := &httpStatusError{
				StatusCode: http.StatusBadGateway,
				Status:     "502 Bad Gateway",
			}

			if widget.canContinueUpdateAfterHandlingErr(err) {
				t.Fatal("complete refresh failure should stop update")
			}

			logged := logs.String()
			want := "content=" + test.wantContent
			if !strings.Contains(logged, want) {
				t.Fatalf("log = %q, want %q", logged, want)
			}
		})
	}
}
