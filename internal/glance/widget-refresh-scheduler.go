package glance

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	widgetRefreshScanInterval = 30 * time.Second
	widgetRefreshConcurrency  = 8
)

func refreshDueWidgetIfAvailable(
	ctx context.Context,
	widget widget,
	now *time.Time,
	liveUpdates *liveUpdateBroker,
) {
	if !widget.tryLockRefresh() {
		if base, ok := widgetBaseOf(widget); ok {
			base.refreshTelemetryMu.Lock()
			base.refreshLockSkips++
			base.refreshTelemetryMu.Unlock()
		}
		return
	}
	defer widget.unlockRefresh()

	if !widget.requiresUpdate(now) {
		return
	}

	refreshWidget(ctx, widget, now)

	if liveUpdates != nil {
		liveUpdates.publish(widget.GetID())
	}
}

func refreshDueWidgets(
	ctx context.Context,
	refreshWidgets []widget,
	concurrency int,
	liveUpdates *liveUpdateBroker,
) {
	if len(refreshWidgets) == 0 || concurrency <= 0 {
		return
	}

	now := time.Now()
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, candidate := range refreshWidgets {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case sem <- struct{}{}:
		}

		if ctx.Err() != nil {
			<-sem
			wg.Wait()
			return
		}

		wg.Add(1)
		go func(widget widget) {
			defer wg.Done()
			defer func() { <-sem }()

			refreshDueWidgetIfAvailable(ctx, widget, &now, liveUpdates)
		}(candidate)
	}

	wg.Wait()
}

func runWidgetRefreshScheduler(
	ctx context.Context,
	refreshWidgets []widget,
	scanInterval time.Duration,
	concurrency int,
	liveUpdates *liveUpdateBroker,
) {
	if len(refreshWidgets) == 0 || concurrency <= 0 {
		return
	}

	if scanInterval <= 0 {
		scanInterval = widgetRefreshScanInterval
	}

	slog.Info(
		"Widget refresh scheduler started",
		"widgets", len(refreshWidgets),
		"scan_interval", scanInterval,
		"concurrency", concurrency,
	)
	defer slog.Info("Widget refresh scheduler stopped")

	refreshDueWidgets(ctx, refreshWidgets, concurrency, liveUpdates)

	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshDueWidgets(ctx, refreshWidgets, concurrency, liveUpdates)
		}
	}
}
