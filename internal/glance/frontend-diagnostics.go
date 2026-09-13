package glance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	frontendDiagnosticsMaxRequestBytes  = 32 * 1024
	frontendDiagnosticsMaxEvents        = 50
	frontendDiagnosticsMaxEventLength   = 64
	frontendDiagnosticsMaxPageLength    = 128
	frontendDiagnosticsMaxSessionLength = 64
	frontendDiagnosticsMaxWidgetLength  = 32
	frontendDiagnosticsMaxDetailLength  = 256
)

type frontendDiagnosticBatch struct {
	Events []frontendDiagnosticEvent `json:"events"`
}

type frontendDiagnosticEvent struct {
	Event     string             `json:"event"`
	Page      string             `json:"page,omitempty"`
	Session   string             `json:"session,omitempty"`
	Widget    string             `json:"widget,omitempty"`
	Detail    string             `json:"detail,omitempty"`
	Sequence  uint64             `json:"sequence,omitempty"`
	ElapsedMS float64            `json:"elapsed_ms,omitempty"`
	Status    int                `json:"status,omitempty"`
	Length    *int               `json:"length,omitempty"`
	State     *int               `json:"state,omitempty"`
	Metrics   map[string]float64 `json:"metrics,omitempty"`
}

func (a *application) handleFrontendPerformanceSnapshotRequest(
	w http.ResponseWriter,
	r *http.Request,
) {
	if !a.Config.Server.FrontendDiagnostics {
		http.NotFound(w, r)
		return
	}

	if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
		return
	}

	if a.liveUpdates == nil {
		http.Error(
			w,
			"Live updates unavailable",
			http.StatusServiceUnavailable,
		)
		return
	}

	command := frontendDiagnosticCommand{
		ID:      frontendDiagnosticCommandID.Add(1),
		Command: "performance_snapshot",
	}

	a.liveUpdates.publishDiagnosticCommand(command)

	slog.Info(
		"Frontend diagnostic",
		"source", "server",
		"event", "diagnostic_command_publish",
		"command_id", command.ID,
		"command", command.Command,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(
		w,
		`{"id":%d,"command":"%s"}`,
		command.ID,
		command.Command,
	)
}

func (a *application) frontendDiagnosticHTTPPerformanceHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/frontend-diagnostics" ||
			r.URL.Path == "/api/live-updates" {
			next.ServeHTTP(w, r)
			return
		}

		started := time.Now()
		next.ServeHTTP(w, r)

		slog.Info(
			"Frontend diagnostic",
			"source", "server",
			"event", "http_request_complete",
			"method", r.Method,
			"path", r.URL.Path,
			"elapsed_ms", float64(time.Since(started).Microseconds())/1000,
		)
	})
}

func (a *application) handleFrontendDiagnosticsRequest(w http.ResponseWriter, r *http.Request) {
	if !a.Config.Server.FrontendDiagnostics {
		http.NotFound(w, r)
		return
	}

	if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
		return
	}

	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, frontendDiagnosticsMaxRequestBytes)

	var batch frontendDiagnosticBatch
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&batch); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "Diagnostic payload too large", http.StatusRequestEntityTooLarge)
			return
		}

		http.Error(w, "Invalid diagnostic payload", http.StatusBadRequest)
		return
	}

	if err := ensureFrontendDiagnosticsEOF(decoder); err != nil {
		http.Error(w, "Invalid diagnostic payload", http.StatusBadRequest)
		return
	}

	if len(batch.Events) == 0 || len(batch.Events) > frontendDiagnosticsMaxEvents {
		http.Error(w, "Invalid diagnostic event count", http.StatusBadRequest)
		return
	}

	for _, event := range batch.Events {
		if !validFrontendDiagnosticEvent(event) {
			http.Error(w, "Invalid diagnostic event", http.StatusBadRequest)
			return
		}
	}

	for _, event := range batch.Events {
		attrs := []any{
			"source", "frontend",
			"event", event.Event,
		}

		if event.Page != "" {
			attrs = append(attrs, "page", event.Page)
		}
		if event.Session != "" {
			attrs = append(attrs, "session", event.Session)
		}
		if event.Sequence != 0 {
			attrs = append(attrs, "sequence", event.Sequence)
		}
		if event.Widget != "" {
			attrs = append(attrs, "widget", event.Widget)
		}
		if event.Detail != "" {
			attrs = append(attrs, "detail", event.Detail)
		}
		if event.Status != 0 {
			attrs = append(attrs, "status", event.Status)
		}
		if event.Length != nil {
			attrs = append(attrs, "length", *event.Length)
		}
		if event.State != nil {
			attrs = append(attrs, "state", *event.State)
		}
		if event.ElapsedMS != 0 {
			attrs = append(attrs, "elapsed_ms", event.ElapsedMS)
		}
		for name, value := range event.Metrics {
			attrs = append(attrs, name, value)
		}

		slog.Info("Frontend diagnostic", attrs...)
	}

	w.WriteHeader(http.StatusNoContent)
}

func ensureFrontendDiagnosticsEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}

func validFrontendDiagnosticEvent(event frontendDiagnosticEvent) bool {
	if event.Event == "" ||
		len(event.Event) > frontendDiagnosticsMaxEventLength ||
		len(event.Page) > frontendDiagnosticsMaxPageLength ||
		len(event.Session) > frontendDiagnosticsMaxSessionLength ||
		len(event.Widget) > frontendDiagnosticsMaxWidgetLength ||
		len(event.Detail) > frontendDiagnosticsMaxDetailLength ||
		event.ElapsedMS < 0 ||
		event.Status < 0 ||
		event.Status > 999 ||
		!validFrontendDiagnosticMetrics(event.Metrics) ||
		(event.Length != nil && *event.Length < 0) ||
		(event.State != nil && (*event.State < 0 || *event.State > 2)) {
		return false
	}

	for _, r := range event.Event {
		if (r < 'a' || r > 'z') &&
			(r < '0' || r > '9') &&
			r != '_' {
			return false
		}
	}

	return !containsFrontendDiagnosticControlCharacters(event.Page) &&
		!containsFrontendDiagnosticControlCharacters(event.Session) &&
		!containsFrontendDiagnosticControlCharacters(event.Widget) &&
		!containsFrontendDiagnosticControlCharacters(event.Detail)
}

func validFrontendDiagnosticMetrics(metrics map[string]float64) bool {
	if len(metrics) > 32 {
		return false
	}

	for name, value := range metrics {
		if name == "" || len(name) > frontendDiagnosticsMaxEventLength ||
			value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}

		for _, r := range name {
			if (r < 'a' || r > 'z') &&
				(r < '0' || r > '9') &&
				r != '_' {
				return false
			}
		}
	}

	return true
}

func containsFrontendDiagnosticControlCharacters(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool {
		return r < 0x20 || r == 0x7f
	}) >= 0
}
