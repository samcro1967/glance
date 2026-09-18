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
	"sync"
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
	CommandID uint64             `json:"command_id,omitempty"`
	ElapsedMS float64            `json:"elapsed_ms,omitempty"`
	Status    int                `json:"status,omitempty"`
	Length    *int               `json:"length,omitempty"`
	State     *int               `json:"state,omitempty"`
	Metrics   map[string]float64 `json:"metrics,omitempty"`
}

const (
	frontendDiagnosticsRecentProblemLimit      = 20
	frontendDiagnosticsRecentActiveResultLimit = 100
)

type frontendRuntimeDiagnosticProblem struct {
	RecordedAt time.Time
	Event      frontendDiagnosticEvent
}

type frontendRuntimeDiagnosticActiveResult struct {
	RecordedAt time.Time
	Event      frontendDiagnosticEvent
}

type frontendRuntimeDiagnosticsSnapshot struct {
	TotalEvents         uint64
	TotalProblemEvents  uint64
	LastEventAt         time.Time
	LastPage            string
	LastSession         string
	ProblemCounts       map[string]uint64
	RecentProblems      []frontendRuntimeDiagnosticProblem
	RecentActiveResults []frontendRuntimeDiagnosticActiveResult
}

type frontendRuntimeDiagnostics struct {
	mu                  sync.RWMutex
	totalEvents         uint64
	totalProblemEvents  uint64
	lastEventAt         time.Time
	lastPage            string
	lastSession         string
	problemCounts       map[string]uint64
	recentProblems      []frontendRuntimeDiagnosticProblem
	recentActiveResults []frontendRuntimeDiagnosticActiveResult
}

func newFrontendRuntimeDiagnostics() *frontendRuntimeDiagnostics {
	return &frontendRuntimeDiagnostics{
		problemCounts: make(map[string]uint64),
	}
}

func frontendDiagnosticIsProblem(event string) bool {
	switch event {
	case "window_error",
		"unhandled_rejection",
		"page_content_load_error",
		"widget_replacement_invalid",
		"widget_current_missing",
		"live_update_invalid",
		"diagnostic_command_unsupported",
		"long_task_capture_unsupported":
		return true
	}

	return strings.HasSuffix(event, "_error")
}

func frontendDiagnosticIsActiveResult(event frontendDiagnosticEvent) bool {
	if event.CommandID == 0 {
		return false
	}

	switch event.Event {
	case "performance_snapshot",
		"navigation_snapshot",
		"resource_snapshot",
		"memory_snapshot",
		"paint_snapshot",
		"web_vitals_snapshot",
		"lcp_attribution",
		"cls_attribution",
		"performance_snapshot_complete",
		"long_task_capture_start",
		"long_task_capture_complete",
		"long_task_capture_unsupported",
		"long_task_capture_error",
		"runtime_state":
		return true
	}

	return false
}

func cloneFrontendDiagnosticEvent(event frontendDiagnosticEvent) frontendDiagnosticEvent {
	cloned := event

	if event.Metrics != nil {
		cloned.Metrics = make(map[string]float64, len(event.Metrics))
		for name, value := range event.Metrics {
			cloned.Metrics[name] = value
		}
	}

	return cloned
}

func (d *frontendRuntimeDiagnostics) record(events []frontendDiagnosticEvent) {
	if d == nil || len(events) == 0 {
		return
	}

	now := time.Now()

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.problemCounts == nil {
		d.problemCounts = make(map[string]uint64)
	}

	for _, event := range events {
		d.totalEvents++
		d.lastEventAt = now

		if event.Page != "" {
			d.lastPage = event.Page
		}
		if event.Session != "" {
			d.lastSession = event.Session
		}

		if frontendDiagnosticIsActiveResult(event) {
			d.recentActiveResults = append(
				d.recentActiveResults,
				frontendRuntimeDiagnosticActiveResult{
					RecordedAt: now,
					Event:      cloneFrontendDiagnosticEvent(event),
				},
			)

			if len(d.recentActiveResults) > frontendDiagnosticsRecentActiveResultLimit {
				d.recentActiveResults = append(
					[]frontendRuntimeDiagnosticActiveResult(nil),
					d.recentActiveResults[len(d.recentActiveResults)-frontendDiagnosticsRecentActiveResultLimit:]...,
				)
			}
		}

		if !frontendDiagnosticIsProblem(event.Event) {
			continue
		}

		d.totalProblemEvents++
		d.problemCounts[event.Event]++

		d.recentProblems = append(
			d.recentProblems,
			frontendRuntimeDiagnosticProblem{
				RecordedAt: now,
				Event:      cloneFrontendDiagnosticEvent(event),
			},
		)

		if len(d.recentProblems) > frontendDiagnosticsRecentProblemLimit {
			d.recentProblems = append(
				[]frontendRuntimeDiagnosticProblem(nil),
				d.recentProblems[len(d.recentProblems)-frontendDiagnosticsRecentProblemLimit:]...,
			)
		}
	}
}

func (d *frontendRuntimeDiagnostics) activeResultsForCommand(commandID uint64) []frontendRuntimeDiagnosticActiveResult {
	if d == nil || commandID == 0 {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	results := make([]frontendRuntimeDiagnosticActiveResult, 0)
	for _, result := range d.recentActiveResults {
		if result.Event.CommandID != commandID {
			continue
		}

		results = append(results, frontendRuntimeDiagnosticActiveResult{
			RecordedAt: result.RecordedAt,
			Event:      cloneFrontendDiagnosticEvent(result.Event),
		})
	}

	return results
}

func (d *frontendRuntimeDiagnostics) snapshot() frontendRuntimeDiagnosticsSnapshot {
	if d == nil {
		return frontendRuntimeDiagnosticsSnapshot{}
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	snapshot := frontendRuntimeDiagnosticsSnapshot{
		TotalEvents:         d.totalEvents,
		TotalProblemEvents:  d.totalProblemEvents,
		LastEventAt:         d.lastEventAt,
		LastPage:            d.lastPage,
		LastSession:         d.lastSession,
		ProblemCounts:       make(map[string]uint64, len(d.problemCounts)),
		RecentProblems:      make([]frontendRuntimeDiagnosticProblem, len(d.recentProblems)),
		RecentActiveResults: make([]frontendRuntimeDiagnosticActiveResult, len(d.recentActiveResults)),
	}

	for name, count := range d.problemCounts {
		snapshot.ProblemCounts[name] = count
	}

	for i, problem := range d.recentProblems {
		snapshot.RecentProblems[i] = frontendRuntimeDiagnosticProblem{
			RecordedAt: problem.RecordedAt,
			Event:      cloneFrontendDiagnosticEvent(problem.Event),
		}
	}

	for i, result := range d.recentActiveResults {
		snapshot.RecentActiveResults[i] = frontendRuntimeDiagnosticActiveResult{
			RecordedAt: result.RecordedAt,
			Event:      cloneFrontendDiagnosticEvent(result.Event),
		}
	}

	return snapshot
}

func (a *application) handleFrontendPerformanceSnapshotRequest(w http.ResponseWriter, r *http.Request) {
	a.handleFrontendDiagnosticCommandRequest(w, r, "performance_snapshot")
}

func (a *application) handleFrontendLongTaskCaptureRequest(w http.ResponseWriter, r *http.Request) {
	a.handleFrontendDiagnosticCommandRequest(w, r, "long_task_capture")
}

func (a *application) handleFrontendRuntimeStateRequest(w http.ResponseWriter, r *http.Request) {
	a.handleFrontendDiagnosticCommandRequest(w, r, "runtime_state")
}

func (a *application) publishFrontendDiagnosticCommand(commandName string) (frontendDiagnosticCommand, error) {
	if a.liveUpdates == nil {
		return frontendDiagnosticCommand{}, errors.New("live updates unavailable")
	}

	command := frontendDiagnosticCommand{
		ID:      frontendDiagnosticCommandID.Add(1),
		Command: commandName,
	}

	a.liveUpdates.publishDiagnosticCommand(command)

	slog.Info(
		"Frontend diagnostic",
		"source", "server",
		"event", "diagnostic_command_publish",
		"command_id", command.ID,
		"command", command.Command,
	)

	return command, nil
}

func (a *application) handleFrontendDiagnosticCommandRequest(
	w http.ResponseWriter,
	r *http.Request,
	commandName string,
) {
	if !a.Config.Server.FrontendDiagnostics {
		http.NotFound(w, r)
		return
	}

	if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
		return
	}

	command, err := a.publishFrontendDiagnosticCommand(commandName)
	if err != nil {
		http.Error(w, "Live updates unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = fmt.Fprintf(
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

	if a.frontendDiagnostics != nil {
		a.frontendDiagnostics.record(batch.Events)
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
		if event.CommandID != 0 {
			attrs = append(attrs, "command_id", event.CommandID)
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
