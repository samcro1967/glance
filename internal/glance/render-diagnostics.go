package glance

import (
	"sync"
	"time"
)

type renderWidgetAttribution struct {
	ID    uint64
	Type  string
	Title string
}

type renderRuntimeDiagnosticsSnapshot struct {
	StartedAt                  time.Time
	WidgetCalls                uint64
	WidgetSnapshotHits         uint64
	WidgetRefreshLockWaits     uint64
	WidgetLockWaitTotal        time.Duration
	WidgetLockWaitMax          time.Duration
	WidgetLockWaitMaxWidget    renderWidgetAttribution
	WidgetRenders              uint64
	WidgetRenderTotal          time.Duration
	WidgetRenderMax            time.Duration
	WidgetRenderMaxWidget      renderWidgetAttribution
	PageExecutions             uint64
	PageFailures               uint64
	PageLockWaitTotal          time.Duration
	PageLockWaitMax            time.Duration
	PageTemplateExecutionTotal time.Duration
	PageTemplateExecutionMax   time.Duration
}

type renderRuntimeDiagnostics struct {
	mu sync.Mutex

	startedAt                  time.Time
	widgetCalls                uint64
	widgetSnapshotHits         uint64
	widgetRefreshLockWaits     uint64
	widgetLockWaitTotal        time.Duration
	widgetLockWaitMax          time.Duration
	widgetLockWaitMaxWidget    renderWidgetAttribution
	widgetRenders              uint64
	widgetRenderTotal          time.Duration
	widgetRenderMax            time.Duration
	widgetRenderMaxWidget      renderWidgetAttribution
	pageExecutions             uint64
	pageFailures               uint64
	pageLockWaitTotal          time.Duration
	pageLockWaitMax            time.Duration
	pageTemplateExecutionTotal time.Duration
	pageTemplateExecutionMax   time.Duration
}

var renderDiagnostics = newRenderRuntimeDiagnostics()

func newRenderRuntimeDiagnostics() *renderRuntimeDiagnostics {
	return &renderRuntimeDiagnostics{startedAt: time.Now()}
}

func (d *renderRuntimeDiagnostics) recordWidgetSnapshotHit() {
	if d == nil {
		return
	}

	d.mu.Lock()
	d.widgetCalls++
	d.widgetSnapshotHits++
	d.mu.Unlock()
}

func (d *renderRuntimeDiagnostics) recordWidgetRefreshLockWait(
	duration time.Duration,
	widget renderWidgetAttribution,
) {
	if d == nil {
		return
	}

	d.mu.Lock()
	d.widgetRefreshLockWaits++
	d.widgetLockWaitTotal += duration
	if duration > d.widgetLockWaitMax {
		d.widgetLockWaitMax = duration
		d.widgetLockWaitMaxWidget = widget
	}
	d.mu.Unlock()
}

func (d *renderRuntimeDiagnostics) recordWidgetRender(
	duration time.Duration,
	widget renderWidgetAttribution,
) {
	if d == nil {
		return
	}

	d.mu.Lock()
	d.widgetCalls++
	d.widgetRenders++
	d.widgetRenderTotal += duration
	if duration > d.widgetRenderMax {
		d.widgetRenderMax = duration
		d.widgetRenderMaxWidget = widget
	}
	d.mu.Unlock()
}

func (d *renderRuntimeDiagnostics) recordPageLockWait(duration time.Duration) {
	if d == nil {
		return
	}

	d.mu.Lock()
	d.pageLockWaitTotal += duration
	if duration > d.pageLockWaitMax {
		d.pageLockWaitMax = duration
	}
	d.mu.Unlock()
}

func (d *renderRuntimeDiagnostics) recordPageTemplateExecution(duration time.Duration, err error) {
	if d == nil {
		return
	}

	d.mu.Lock()
	d.pageExecutions++
	if err != nil {
		d.pageFailures++
	}
	d.pageTemplateExecutionTotal += duration
	if duration > d.pageTemplateExecutionMax {
		d.pageTemplateExecutionMax = duration
	}
	d.mu.Unlock()
}

func (d *renderRuntimeDiagnostics) snapshot() renderRuntimeDiagnosticsSnapshot {
	if d == nil {
		return renderRuntimeDiagnosticsSnapshot{}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	return renderRuntimeDiagnosticsSnapshot{
		StartedAt:                  d.startedAt,
		WidgetCalls:                d.widgetCalls,
		WidgetSnapshotHits:         d.widgetSnapshotHits,
		WidgetRefreshLockWaits:     d.widgetRefreshLockWaits,
		WidgetLockWaitTotal:        d.widgetLockWaitTotal,
		WidgetLockWaitMax:          d.widgetLockWaitMax,
		WidgetLockWaitMaxWidget:    d.widgetLockWaitMaxWidget,
		WidgetRenders:              d.widgetRenders,
		WidgetRenderTotal:          d.widgetRenderTotal,
		WidgetRenderMax:            d.widgetRenderMax,
		WidgetRenderMaxWidget:      d.widgetRenderMaxWidget,
		PageExecutions:             d.pageExecutions,
		PageFailures:               d.pageFailures,
		PageLockWaitTotal:          d.pageLockWaitTotal,
		PageLockWaitMax:            d.pageLockWaitMax,
		PageTemplateExecutionTotal: d.pageTemplateExecutionTotal,
		PageTemplateExecutionMax:   d.pageTemplateExecutionMax,
	}
}
