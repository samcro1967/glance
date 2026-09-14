package glance

import (
	"errors"
	"testing"
)

func TestProfilingRuntimeDiagnosticsLifecycle(t *testing.T) {
	diagnostics := newProfilingRuntimeDiagnostics()

	got := diagnostics.snapshot()
	if got.Requested || got.Running || !got.LastFailureAt.IsZero() || got.LastFailure != "" {
		t.Fatalf("initial snapshot = %+v, want empty state", got)
	}

	diagnostics.recordRunning()
	got = diagnostics.snapshot()
	if !got.Requested || !got.Running {
		t.Fatalf("requested snapshot = %+v, want requested and running", got)
	}

	diagnostics.recordFailure(errors.New("profiling bind failed"))
	got = diagnostics.snapshot()
	if !got.Requested || got.Running {
		t.Fatalf("failure snapshot = %+v, want requested and not running", got)
	}
	if got.LastFailure != "profiling bind failed" || got.LastFailureAt.IsZero() {
		t.Fatalf("failure snapshot = %+v, want retained failure", got)
	}

	diagnostics.recordDisabled()
	got = diagnostics.snapshot()
	if got.Requested || got.Running {
		t.Fatalf("disabled snapshot = %+v, want disabled", got)
	}
	if got.LastFailure != "profiling bind failed" || got.LastFailureAt.IsZero() {
		t.Fatalf("disabled snapshot = %+v, want failure history retained", got)
	}

	diagnostics.recordRunning()
	got = diagnostics.snapshot()
	if !got.Requested || !got.Running {
		t.Fatalf("recovered snapshot = %+v, want requested and running", got)
	}
	if got.LastFailure != "" || !got.LastFailureAt.IsZero() {
		t.Fatalf("recovered snapshot = %+v, want stale failure cleared", got)
	}
}

func TestProfilingRuntimeDiagnosticsNilSafe(t *testing.T) {
	var diagnostics *profilingRuntimeDiagnostics

	diagnostics.recordRunning()
	diagnostics.recordRunning()
	diagnostics.recordFailure(errors.New("ignored"))
	diagnostics.recordDisabled()

	got := diagnostics.snapshot()
	if got != (profilingRuntimeDiagnosticsSnapshot{}) {
		t.Fatalf("nil snapshot = %+v, want empty state", got)
	}
}
