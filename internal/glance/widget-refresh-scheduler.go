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
) {
	if !widget.tryLockRefresh() {
		return
	}
	defer widget.unlockRefresh()

	if !widget.requiresUpdate(now) {
		return
	}

	widget.update(ctx)
}

func refreshDueWidgets(
	ctx context.Context,
	refreshWidgets []widget,
	concurrency int,
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

			refreshDueWidgetIfAvailable(ctx, widget, &now)
		}(candidate)
	}

	wg.Wait()
}

func runWidgetRefreshScheduler(
	ctx context.Context,
	refreshWidgets []widget,
	scanInterval time.Duration,
	concurrency int,
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

	refreshDueWidgets(ctx, refreshWidgets, concurrency)

	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshDueWidgets(ctx, refreshWidgets, concurrency)
		}
	}
}
