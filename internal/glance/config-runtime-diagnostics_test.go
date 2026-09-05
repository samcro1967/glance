package glance

import (
	"errors"
	"testing"
	"time"
)

func TestConfigRuntimeDiagnosticsLifecycle(t *testing.T) {
	diagnostics := newConfigRuntimeDiagnostics("glance.yml")

	loadedAt := time.Now()
	diagnostics.recordLoaded(loadedAt)

	snapshot := diagnostics.snapshot()
	if snapshot.ConfigPath != "glance.yml" {
		t.Fatalf("config path = %q, want glance.yml", snapshot.ConfigPath)
	}
	if !snapshot.LoadedAt.Equal(loadedAt) {
		t.Fatalf("loaded at = %v, want %v", snapshot.LoadedAt, loadedAt)
	}

	attemptAt := loadedAt.Add(time.Minute)
	diagnostics.recordReloadAttempt(attemptAt)
	diagnostics.recordReloadRejected(errors.New("invalid configuration"))

	snapshot = diagnostics.snapshot()
	if !snapshot.LastReloadAttempt.Equal(attemptAt) {
		t.Fatalf(
			"last reload attempt = %v, want %v",
			snapshot.LastReloadAttempt,
			attemptAt,
		)
	}
	if snapshot.LastReloadResult != configReloadResultRejected {
		t.Fatalf(
			"last reload result = %q, want %q",
			snapshot.LastReloadResult,
			configReloadResultRejected,
		)
	}
	if snapshot.LastReloadRejection == nil {
		t.Fatal("last reload rejection is nil")
	}
	if snapshot.LastReloadRejection.Message != "invalid configuration" {
		t.Fatalf(
			"rejection message = %q, want invalid configuration",
			snapshot.LastReloadRejection.Message,
		)
	}

	acceptedAt := attemptAt.Add(time.Minute)
	diagnostics.recordReloadAttempt(acceptedAt)
	diagnostics.recordReloadAccepted(acceptedAt)

	snapshot = diagnostics.snapshot()
	if !snapshot.LoadedAt.Equal(acceptedAt) {
		t.Fatalf("loaded at = %v, want %v", snapshot.LoadedAt, acceptedAt)
	}
	if snapshot.LastReloadResult != configReloadResultAccepted {
		t.Fatalf(
			"last reload result = %q, want %q",
			snapshot.LastReloadResult,
			configReloadResultAccepted,
		)
	}

	if snapshot.LastReloadRejection == nil {
		t.Fatal("accepted reload discarded previous rejection")
	}
	if snapshot.LastReloadRejection.Message != "invalid configuration" {
		t.Fatalf(
			"retained rejection message = %q, want invalid configuration",
			snapshot.LastReloadRejection.Message,
		)
	}
}

func TestConfigRuntimeDiagnosticsSourceDiagnostic(t *testing.T) {
	diagnostics := newConfigRuntimeDiagnostics("glance.yml")

	diagnostics.recordReloadAttempt(time.Now())
	diagnostics.recordReloadRejected(&configDiagnostic{
		File:    "/config/widgets.yml",
		Line:    42,
		Message: "invalid widget configuration",
	})

	snapshot := diagnostics.snapshot()
	if snapshot.LastReloadRejection == nil {
		t.Fatal("last reload rejection is nil")
	}

	rejection := snapshot.LastReloadRejection
	if rejection.File != "/config/widgets.yml" {
		t.Fatalf("file = %q, want /config/widgets.yml", rejection.File)
	}
	if rejection.Line != 42 {
		t.Fatalf("line = %d, want 42", rejection.Line)
	}
	if rejection.Message != "invalid widget configuration" {
		t.Fatalf(
			"message = %q, want invalid widget configuration",
			rejection.Message,
		)
	}
}

func TestConfigRuntimeDiagnosticsConcurrentAccess(t *testing.T) {
	diagnostics := newConfigRuntimeDiagnostics("glance.yml")

	done := make(chan struct{})

	go func() {
		defer close(done)

		for i := 0; i < 1000; i++ {
			now := time.Now()
			diagnostics.recordReloadAttempt(now)
			diagnostics.recordReloadRejected(errors.New("rejected"))
			diagnostics.recordReloadAccepted(now)
		}
	}()

	for i := 0; i < 1000; i++ {
		_ = diagnostics.snapshot()
	}

	<-done
}
