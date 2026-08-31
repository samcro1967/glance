package glance

import (
	"context"
	"html/template"
	"sync/atomic"
	"testing"
	"time"
)

func TestRefreshDueWidgetsRefreshesDueWidget(t *testing.T) {
	testWidget := newRefreshTestWidget()
	close(testWidget.updateBlock)

	refreshDueWidgets(
		context.Background(),
		[]widget{testWidget},
		1,
		nil,
	)

	if got := testWidget.updateCount.Load(); got != 1 {
		t.Fatalf("update count = %d, want 1", got)
	}
}

func TestRefreshDueWidgetsSkipsWidgetThatIsNotDue(t *testing.T) {
	testWidget := newRefreshTestWidget()
	testWidget.scheduleNextUpdate()
	close(testWidget.updateBlock)

	refreshDueWidgets(
		context.Background(),
		[]widget{testWidget},
		1,
		nil,
	)

	if got := testWidget.updateCount.Load(); got != 0 {
		t.Fatalf("update count = %d, want 0", got)
	}
}

func TestRefreshDueWidgetsSkipsBusyWidgetWithoutBlockingAvailableWidget(t *testing.T) {
	busyWidget := newRefreshTestWidget()
	availableWidget := newRefreshTestWidget()
	close(availableWidget.updateBlock)

	busyWidget.lockRefresh()
	defer busyWidget.unlockRefresh()

	done := make(chan struct{})
	go func() {
		defer close(done)

		refreshDueWidgets(
			context.Background(),
			[]widget{busyWidget, availableWidget},
			1,
			nil,
		)
	}()

	select {
	case <-availableWidget.updateStart:
	case <-time.After(time.Second):
		t.Fatal("available widget refresh was blocked by busy widget")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("refreshDueWidgets did not complete while busy widget remained locked")
	}

	if got := busyWidget.updateCount.Load(); got != 0 {
		t.Fatalf("busy widget update count = %d, want 0", got)
	}

	if got := availableWidget.updateCount.Load(); got != 1 {
		t.Fatalf("available widget update count = %d, want 1", got)
	}
}

func TestRefreshDueWidgetsPublishesUpdatedWidget(t *testing.T) {
	testWidget := newRefreshTestWidget()
	testWidget.setID(42)
	close(testWidget.updateBlock)

	broker := newLiveUpdateBroker()
	subscription, unsubscribe := broker.subscribe()
	defer unsubscribe()

	refreshDueWidgets(
		context.Background(),
		[]widget{testWidget},
		1,
		broker,
	)

	waitForLiveUpdate(t, subscription)

	widgetIDs := subscription.takePending()
	if len(widgetIDs) != 1 || widgetIDs[0] != 42 {
		t.Fatalf("published widget IDs = %v, want [42]", widgetIDs)
	}
}

func TestRefreshDueWidgetsDoesNotPublishWidgetThatIsNotDue(t *testing.T) {
	testWidget := newRefreshTestWidget()
	testWidget.setID(42)
	testWidget.scheduleNextUpdate()
	close(testWidget.updateBlock)

	broker := newLiveUpdateBroker()
	subscription, unsubscribe := broker.subscribe()
	defer unsubscribe()

	refreshDueWidgets(
		context.Background(),
		[]widget{testWidget},
		1,
		broker,
	)

	select {
	case <-subscription.ready:
		t.Fatal("received live update for widget that was not refreshed")
	case <-time.After(25 * time.Millisecond):
	}
}

func TestRefreshDueWidgetsDoesNotPublishBusyWidget(t *testing.T) {
	testWidget := newRefreshTestWidget()
	testWidget.setID(42)

	broker := newLiveUpdateBroker()
	subscription, unsubscribe := broker.subscribe()
	defer unsubscribe()

	testWidget.lockRefresh()
	defer testWidget.unlockRefresh()

	refreshDueWidgets(
		context.Background(),
		[]widget{testWidget},
		1,
		broker,
	)

	select {
	case <-subscription.ready:
		t.Fatal("received live update for busy widget that was not refreshed")
	case <-time.After(25 * time.Millisecond):
	}
}

type schedulerConcurrencyTestWidget struct {
	widgetBase

	active    *atomic.Int32
	maxActive *atomic.Int32
	started   chan struct{}
	release   chan struct{}
}

func newSchedulerConcurrencyTestWidget(
	active *atomic.Int32,
	maxActive *atomic.Int32,
	started chan struct{},
	release chan struct{},
) *schedulerConcurrencyTestWidget {
	testWidget := &schedulerConcurrencyTestWidget{
		active:    active,
		maxActive: maxActive,
		started:   started,
		release:   release,
	}

	testWidget.Type = "scheduler-concurrency-test"
	testWidget.withCacheDuration(time.Hour)

	return testWidget
}

func (widget *schedulerConcurrencyTestWidget) initialize() error {
	return nil
}

func (widget *schedulerConcurrencyTestWidget) update(ctx context.Context) {
	current := widget.active.Add(1)
	defer widget.active.Add(-1)

	for {
		maximum := widget.maxActive.Load()
		if current <= maximum || widget.maxActive.CompareAndSwap(maximum, current) {
			break
		}
	}

	widget.started <- struct{}{}

	select {
	case <-widget.release:
		widget.scheduleNextUpdate()
	case <-ctx.Done():
	}
}

func (widget *schedulerConcurrencyTestWidget) Render() template.HTML {
	return ""
}

func TestRefreshDueWidgetsBoundsConcurrency(t *testing.T) {
	const (
		widgetCount = 6
		concurrency = 2
	)

	var active atomic.Int32
	var maxActive atomic.Int32

	started := make(chan struct{}, widgetCount)
	release := make(chan struct{})

	refreshWidgets := make([]widget, 0, widgetCount)
	for range widgetCount {
		refreshWidgets = append(
			refreshWidgets,
			newSchedulerConcurrencyTestWidget(
				&active,
				&maxActive,
				started,
				release,
			),
		)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		refreshDueWidgets(
			context.Background(),
			refreshWidgets,
			concurrency,
			nil,
		)
	}()

	for range concurrency {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("expected concurrent widget refresh did not start")
		}
	}

	select {
	case <-started:
		t.Fatal("more widgets started than the concurrency limit allows")
	case <-time.After(25 * time.Millisecond):
	}

	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("refreshDueWidgets did not complete")
	}

	if got := maxActive.Load(); got != concurrency {
		t.Fatalf("maximum concurrent updates = %d, want %d", got, concurrency)
	}
}

func TestRefreshDueWidgetsStopsSchedulingAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	first := newRefreshTestWidget()
	second := newRefreshTestWidget()

	done := make(chan struct{})
	go func() {
		defer close(done)
		refreshDueWidgets(
			ctx,
			[]widget{first, second},
			1,
			nil,
		)
	}()

	select {
	case <-first.updateStart:
	case <-time.After(time.Second):
		t.Fatal("first widget refresh did not start")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("refreshDueWidgets did not stop after cancellation")
	}

	if got := first.updateCount.Load(); got != 1 {
		t.Fatalf("first widget update count = %d, want 1", got)
	}

	if got := second.updateCount.Load(); got != 0 {
		t.Fatalf("second widget update count = %d, want 0", got)
	}
}

func TestWidgetRefreshSchedulerStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		runWidgetRefreshScheduler(
			ctx,
			[]widget{newRefreshTestWidget()},
			time.Hour,
			1,
			nil,
		)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("widget refresh scheduler did not stop after cancellation")
	}
}

func TestWidgetRefreshSchedulerRefreshesImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testWidget := newRefreshTestWidget()
	close(testWidget.updateBlock)

	done := make(chan struct{})
	go func() {
		defer close(done)

		runWidgetRefreshScheduler(
			ctx,
			[]widget{testWidget},
			time.Hour,
			1,
			nil,
		)
	}()

	select {
	case <-testWidget.updateStart:
	case <-time.After(time.Second):
		t.Fatal("widget refresh scheduler did not perform initial refresh")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("widget refresh scheduler did not stop after cancellation")
	}

	if got := testWidget.updateCount.Load(); got != 1 {
		t.Fatalf("update count = %d, want 1", got)
	}
}
