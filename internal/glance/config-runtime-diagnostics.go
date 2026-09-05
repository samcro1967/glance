package glance

import (
	"errors"
	"sync"
	"time"
)

type configReloadResult string

const (
	configReloadResultNone     configReloadResult = ""
	configReloadResultAccepted configReloadResult = "accepted"
	configReloadResultRejected configReloadResult = "rejected"
)

type configReloadRejection struct {
	At      time.Time
	File    string
	Line    int
	Message string
}

type configRuntimeDiagnosticsSnapshot struct {
	ConfigPath          string
	LoadedAt            time.Time
	LastReloadAttempt   time.Time
	LastReloadResult    configReloadResult
	LastReloadRejection *configReloadRejection
}

type configRuntimeDiagnostics struct {
	mu sync.Mutex

	configPath          string
	loadedAt            time.Time
	lastReloadAttempt   time.Time
	lastReloadResult    configReloadResult
	lastReloadRejection *configReloadRejection
}

func newConfigRuntimeDiagnostics(configPath string) *configRuntimeDiagnostics {
	return &configRuntimeDiagnostics{
		configPath: configPath,
	}
}

func (d *configRuntimeDiagnostics) recordLoaded(at time.Time) {
	if d == nil {
		return
	}

	d.mu.Lock()
	d.loadedAt = at
	d.mu.Unlock()
}

func (d *configRuntimeDiagnostics) recordReloadAttempt(at time.Time) {
	if d == nil {
		return
	}

	d.mu.Lock()
	d.lastReloadAttempt = at
	d.mu.Unlock()
}

func (d *configRuntimeDiagnostics) recordReloadAccepted(at time.Time) {
	if d == nil {
		return
	}

	d.mu.Lock()
	d.loadedAt = at
	d.lastReloadResult = configReloadResultAccepted
	d.mu.Unlock()
}

func (d *configRuntimeDiagnostics) recordReloadRejected(err error) {
	if d == nil {
		return
	}

	rejection := &configReloadRejection{
		At: time.Now(),
	}

	var diagnostic *configDiagnostic
	if errors.As(err, &diagnostic) {
		rejection.File = diagnostic.File
		rejection.Line = diagnostic.Line
		rejection.Message = diagnostic.Message
	} else if err != nil {
		rejection.Message = err.Error()
	}

	d.mu.Lock()
	d.lastReloadResult = configReloadResultRejected
	d.lastReloadRejection = rejection
	d.mu.Unlock()
}

func (d *configRuntimeDiagnostics) snapshot() configRuntimeDiagnosticsSnapshot {
	if d == nil {
		return configRuntimeDiagnosticsSnapshot{}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	snapshot := configRuntimeDiagnosticsSnapshot{
		ConfigPath:        d.configPath,
		LoadedAt:          d.loadedAt,
		LastReloadAttempt: d.lastReloadAttempt,
		LastReloadResult:  d.lastReloadResult,
	}

	if d.lastReloadRejection != nil {
		rejection := *d.lastReloadRejection
		snapshot.LastReloadRejection = &rejection
	}

	return snapshot
}
