package glance

import (
	"errors"
	"testing"
	"time"
)

func TestRenderRuntimeDiagnosticsRecordsIndependentBoundaries(t *testing.T) {
	diagnostics := newRenderRuntimeDiagnostics()

	diagnostics.recordWidgetSnapshotHit()
	diagnostics.recordWidgetRefreshLockWait(4 * time.Millisecond)
	diagnostics.recordWidgetRender(6 * time.Millisecond)
	diagnostics.recordPageLockWait(2 * time.Millisecond)
	diagnostics.recordPageTemplateExecution(8*time.Millisecond, errors.New("render failed"))

	snapshot := diagnostics.snapshot()

	if snapshot.WidgetCalls != 2 || snapshot.WidgetSnapshotHits != 1 || snapshot.WidgetRenders != 1 {
		t.Fatalf("unexpected widget accounting: %#v", snapshot)
	}
	if snapshot.WidgetRefreshLockWaits != 1 || snapshot.WidgetLockWaitTotal != 4*time.Millisecond || snapshot.WidgetLockWaitMax != 4*time.Millisecond {
		t.Fatalf("unexpected widget lock-wait accounting: %#v", snapshot)
	}
	if snapshot.WidgetRenderTotal != 6*time.Millisecond || snapshot.WidgetRenderMax != 6*time.Millisecond {
		t.Fatalf("unexpected widget render accounting: %#v", snapshot)
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
	diagnostics.recordWidgetRefreshLockWait(time.Millisecond)
	diagnostics.recordWidgetRender(time.Millisecond)
	diagnostics.recordPageLockWait(time.Millisecond)
	diagnostics.recordPageTemplateExecution(time.Millisecond, nil)

	if got := diagnostics.snapshot(); got != (renderRuntimeDiagnosticsSnapshot{}) {
		t.Fatalf("nil recorder snapshot = %#v, want zero value", got)
	}
}
