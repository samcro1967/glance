package glance

import (
	"sync"
	"time"
)

type profilingRuntimeDiagnosticsSnapshot struct {
	Requested     bool
	Running       bool
	LastFailureAt time.Time
	LastFailure   string
}

type profilingRuntimeDiagnostics struct {
	mu sync.Mutex

	requested     bool
	running       bool
	lastFailureAt time.Time
	lastFailure   string
}

func newProfilingRuntimeDiagnostics() *profilingRuntimeDiagnostics {
	return &profilingRuntimeDiagnostics{}
}

func (d *profilingRuntimeDiagnostics) recordDisabled() {
	if d == nil {
		return
	}

	d.mu.Lock()
	d.requested = false
	d.running = false
	d.mu.Unlock()
}

func (d *profilingRuntimeDiagnostics) recordFailure(err error) {
	if d == nil {
		return
	}

	d.mu.Lock()
	d.running = false
	d.lastFailureAt = time.Now()
	if err != nil {
		d.lastFailure = err.Error()
	} else {
		d.lastFailure = ""
	}
	d.mu.Unlock()
}

func (d *profilingRuntimeDiagnostics) recordRunning() {
	if d == nil {
		return
	}

	d.mu.Lock()
	d.requested = true
	d.running = true
	d.lastFailureAt = time.Time{}
	d.lastFailure = ""
	d.mu.Unlock()
}

func (d *profilingRuntimeDiagnostics) snapshot() profilingRuntimeDiagnosticsSnapshot {
	if d == nil {
		return profilingRuntimeDiagnosticsSnapshot{}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	return profilingRuntimeDiagnosticsSnapshot{
		Requested:     d.requested,
		Running:       d.running,
		LastFailureAt: d.lastFailureAt,
		LastFailure:   d.lastFailure,
	}
}
