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
	widgetNestedConcurrency   = 8
	widgetSchedulerLagFactor  = 2
	widgetLockSkipThreshold   = 3
)

type widgetSchedulerAnomalyState struct {
	lagging              bool
	contended            bool
	lastLockSkips        uint64
	consecutiveLockSkips int
}

func evaluateWidgetSchedulerAnomalies(
	refreshWidgets []widget,
	scanInterval time.Duration,
	states map[uint64]*widgetSchedulerAnomalyState,
) {
	lagThreshold := time.Duration(widgetSchedulerLagFactor) * scanInterval

	for _, candidate := range refreshWidgets {
		base, ok := widgetBaseOf(candidate)
		if !ok {
			continue
		}

		base.refreshTelemetryMu.Lock()
		lag := base.lastSchedulerLag
		lockSkips := base.refreshLockSkips
		base.refreshTelemetryMu.Unlock()

		state := states[candidate.GetID()]
		if state == nil {
			state = &widgetSchedulerAnomalyState{}
			states[candidate.GetID()] = state
		}

		lagging := lagThreshold > 0 && lag >= lagThreshold
		if lagging && !state.lagging {
			slog.Warn(
				"Widget refresh scheduler lag detected",
				"widget_id", candidate.GetID(),
				"type", base.Type,
				"title", base.Title,
				"scheduler_lag", lag,
				"threshold", lagThreshold,
			)
		} else if !lagging && state.lagging {
			slog.Info(
				"Widget refresh scheduler lag recovered",
				"widget_id", candidate.GetID(),
				"type", base.Type,
				"title", base.Title,
				"scheduler_lag", lag,
			)
		}
		state.lagging = lagging

		if lockSkips > state.lastLockSkips {
			state.consecutiveLockSkips++
		} else {
			state.consecutiveLockSkips = 0
		}
		contended := state.consecutiveLockSkips >= widgetLockSkipThreshold
		if contended && !state.contended {
			slog.Warn(
				"Widget refresh scheduler contention detected",
				"widget_id", candidate.GetID(),
				"type", base.Type,
				"title", base.Title,
				"consecutive_skipped_scans", state.consecutiveLockSkips,
				"total_lock_skips", lockSkips,
			)
		} else if !contended && state.contended {
			slog.Info(
				"Widget refresh scheduler contention recovered",
				"widget_id", candidate.GetID(),
				"type", base.Type,
				"title", base.Title,
				"total_lock_skips", lockSkips,
			)
		}
		state.contended = contended
		state.lastLockSkips = lockSkips
	}
}

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

	anomalyStates := make(map[uint64]*widgetSchedulerAnomalyState, len(refreshWidgets))

	refreshDueWidgets(ctx, refreshWidgets, concurrency, liveUpdates)
	evaluateWidgetSchedulerAnomalies(refreshWidgets, scanInterval, anomalyStates)

	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshDueWidgets(ctx, refreshWidgets, concurrency, liveUpdates)
			evaluateWidgetSchedulerAnomalies(refreshWidgets, scanInterval, anomalyStates)
		}
	}
}
