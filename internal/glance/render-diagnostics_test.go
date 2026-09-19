package glance

import (
	"errors"
	"testing"
	"time"
)

func TestRenderRuntimeDiagnosticsRecordsIndependentBoundaries(t *testing.T) {
	diagnostics := newRenderRuntimeDiagnostics()

	diagnostics.recordWidgetSnapshotHit()

	slowest := renderWidgetAttribution{
		ID:    22,
		Type:  "custom-api",
		Title: "Slow API",
	}
	faster := renderWidgetAttribution{
		ID:    11,
		Type:  "rss",
		Title: "RSS",
	}

	diagnostics.recordWidgetRefreshLockWait(4*time.Millisecond, slowest)
	diagnostics.recordWidgetRefreshLockWait(time.Millisecond, faster)
	diagnostics.recordWidgetRender(6*time.Millisecond, slowest)
	diagnostics.recordWidgetRender(2*time.Millisecond, faster)
	diagnostics.recordPageLockWait(2 * time.Millisecond)
	diagnostics.recordPageTemplateExecution(8*time.Millisecond, errors.New("render failed"))

	snapshot := diagnostics.snapshot()

	if snapshot.WidgetCalls != 3 || snapshot.WidgetSnapshotHits != 1 || snapshot.WidgetRenders != 2 {
		t.Fatalf("unexpected widget accounting: %#v", snapshot)
	}
	if snapshot.WidgetRefreshLockWaits != 2 || snapshot.WidgetLockWaitTotal != 5*time.Millisecond || snapshot.WidgetLockWaitMax != 4*time.Millisecond {
		t.Fatalf("unexpected widget lock-wait accounting: %#v", snapshot)
	}
	if snapshot.WidgetLockWaitMaxWidget != slowest {
		t.Fatalf("widget lock-wait max attribution = %#v, want %#v", snapshot.WidgetLockWaitMaxWidget, slowest)
	}
	if snapshot.WidgetRenderTotal != 8*time.Millisecond || snapshot.WidgetRenderMax != 6*time.Millisecond {
		t.Fatalf("unexpected widget render accounting: %#v", snapshot)
	}
	if snapshot.WidgetRenderMaxWidget != slowest {
		t.Fatalf("widget render max attribution = %#v, want %#v", snapshot.WidgetRenderMaxWidget, slowest)
	}
	if snapshot.PageExecutions != 1 || snapshot.PageFailures != 1 {
		t.Fatalf("unexpected page accounting: %#v", snapshot)
	}
	if snapshot.PageLockWaitTotal != 2*time.Millisecond || snapshot.PageLockWaitMax != 2*time.Millisecond {
		t.Fatalf("unexpected page lock-wait accounting: %#v", snapshot)
	}
	if snapshot.PageTemplateExecutionTotal != 8*time.Millisecond || snapshot.PageTemplateExecutionMax != 8*time.Millisecond {
		t.Fatalf("unexpected page template accounting: %#v", snapshot)
	}
}

func TestRenderRuntimeDiagnosticsNilRecorderIsSafe(t *testing.T) {
	var diagnostics *renderRuntimeDiagnostics

	diagnostics.recordWidgetSnapshotHit()
	diagnostics.recordWidgetRefreshLockWait(time.Millisecond, renderWidgetAttribution{})
	diagnostics.recordWidgetRender(time.Millisecond, renderWidgetAttribution{})
	diagnostics.recordPageLockWait(time.Millisecond)
	diagnostics.recordPageTemplateExecution(time.Millisecond, nil)

	if got := diagnostics.snapshot(); got != (renderRuntimeDiagnosticsSnapshot{}) {
		t.Fatalf("nil recorder snapshot = %#v, want zero value", got)
	}
}
