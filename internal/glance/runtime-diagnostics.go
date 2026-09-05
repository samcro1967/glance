package glance

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type widgetRefreshDiagnostics struct {
	ID                  uint64
	Type                string
	Title               string
	Degraded            bool
	FailureClass        refreshFailureClass
	ConsecutiveFailures int
	LastAttempt         time.Time
	LastSuccess         time.Time
	LastFailure         time.Time
	LastDuration        time.Duration
	RefreshStartedAt    time.Time
	Attempts            uint64
	Successes           uint64
	Failures            uint64
	LockSkips           uint64
	LastSchedulerLag    time.Duration
	MaxSchedulerLag     time.Duration
	NextUpdate          time.Time
}

type runtimeDiagnostics struct {
	GeneratedAt       time.Time
	RefreshWidgets    int
	RefreshingWidgets int
	DegradedWidgets   int
	TotalAttempts     uint64
	TotalSuccesses    uint64
	TotalFailures     uint64
	TotalLockSkips    uint64
	Widgets           []widgetRefreshDiagnostics
}

func snapshotWidgetRefreshDiagnostics(candidate widget) (widgetRefreshDiagnostics, bool) {
	base, ok := widgetBaseOf(candidate)
	if !ok {
		return widgetRefreshDiagnostics{}, false
	}

	base.refreshTelemetryMu.Lock()
	defer base.refreshTelemetryMu.Unlock()

	return widgetRefreshDiagnostics{
		ID:                  candidate.GetID(),
		Type:                candidate.GetType(),
		Title:               base.Title,
		Degraded:            base.refreshDegraded,
		FailureClass:        base.refreshFailureClass,
		ConsecutiveFailures: base.refreshFailureCount,
		LastAttempt:         base.lastRefreshAttempt,
		LastSuccess:         base.lastRefreshSuccess,
		LastFailure:         base.lastRefreshFailure,
		LastDuration:        base.lastRefreshDuration,
		RefreshStartedAt:    base.refreshStartedAt,
		Attempts:            base.refreshAttempts,
		Successes:           base.refreshSuccesses,
		Failures:            base.refreshFailures,
		LockSkips:           base.refreshLockSkips,
		LastSchedulerLag:    base.lastSchedulerLag,
		MaxSchedulerLag:     base.maxSchedulerLag,
		NextUpdate:          base.nextUpdate,
	}, true
}

func collectRuntimeDiagnostics(refreshWidgets []widget) runtimeDiagnostics {
	diagnostics := runtimeDiagnostics{
		GeneratedAt:    time.Now(),
		RefreshWidgets: len(refreshWidgets),
		Widgets:        make([]widgetRefreshDiagnostics, 0, len(refreshWidgets)),
	}

	for _, candidate := range refreshWidgets {
		snapshot, ok := snapshotWidgetRefreshDiagnostics(candidate)
		if !ok {
			continue
		}

		diagnostics.Widgets = append(diagnostics.Widgets, snapshot)

		if !snapshot.RefreshStartedAt.IsZero() {
			diagnostics.RefreshingWidgets++
		}
		if snapshot.Degraded {
			diagnostics.DegradedWidgets++
		}

		diagnostics.TotalAttempts += snapshot.Attempts
		diagnostics.TotalSuccesses += snapshot.Successes
		diagnostics.TotalFailures += snapshot.Failures
		diagnostics.TotalLockSkips += snapshot.LockSkips
	}

	return diagnostics
}

type runtimeDiagnosticsResponse struct {
	GeneratedAt       time.Time                          `json:"generated_at"`
	RefreshWidgets    int                                `json:"refresh_widgets"`
	RefreshingWidgets int                                `json:"refreshing_widgets"`
	DegradedWidgets   int                                `json:"degraded_widgets"`
	TotalAttempts     uint64                             `json:"total_attempts"`
	TotalSuccesses    uint64                             `json:"total_successes"`
	TotalFailures     uint64                             `json:"total_failures"`
	TotalLockSkips    uint64                             `json:"total_lock_skips"`
	Widgets           []widgetRefreshDiagnosticsResponse `json:"widgets"`
	Config            configRuntimeDiagnosticsResponse   `json:"config"`
}

type configRuntimeDiagnosticsResponse struct {
	Path                string                         `json:"path,omitempty"`
	LoadedAt            *time.Time                     `json:"loaded_at,omitempty"`
	LastReloadAttempt   *time.Time                     `json:"last_reload_attempt,omitempty"`
	LastReloadResult    configReloadResult             `json:"last_reload_result,omitempty"`
	LastReloadRejection *configReloadRejectionResponse `json:"last_reload_rejection,omitempty"`
}

type configReloadRejectionResponse struct {
	At      time.Time `json:"at"`
	File    string    `json:"file,omitempty"`
	Line    int       `json:"line,omitempty"`
	Message string    `json:"message"`
}

type widgetRefreshDiagnosticsResponse struct {
	ID                  uint64              `json:"id"`
	Type                string              `json:"type"`
	Title               string              `json:"title,omitempty"`
	Degraded            bool                `json:"degraded"`
	FailureClass        refreshFailureClass `json:"failure_class,omitempty"`
	ConsecutiveFailures int                 `json:"consecutive_failures"`
	LastAttempt         *time.Time          `json:"last_attempt,omitempty"`
	LastSuccess         *time.Time          `json:"last_success,omitempty"`
	LastFailure         *time.Time          `json:"last_failure,omitempty"`
	LastDurationMS      float64             `json:"last_duration_ms"`
	RefreshStartedAt    *time.Time          `json:"refresh_started_at,omitempty"`
	Attempts            uint64              `json:"attempts"`
	Successes           uint64              `json:"successes"`
	Failures            uint64              `json:"failures"`
	LockSkips           uint64              `json:"lock_skips"`
	LastSchedulerLagMS  float64             `json:"last_scheduler_lag_ms"`
	MaxSchedulerLagMS   float64             `json:"max_scheduler_lag_ms"`
	NextUpdate          *time.Time          `json:"next_update,omitempty"`
}

func optionalDiagnosticTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}

	copy := value
	return &copy
}

func runtimeDiagnosticsResponseFromSnapshot(
	snapshot runtimeDiagnostics,
) runtimeDiagnosticsResponse {
	response := runtimeDiagnosticsResponse{
		GeneratedAt:       snapshot.GeneratedAt,
		RefreshWidgets:    snapshot.RefreshWidgets,
		RefreshingWidgets: snapshot.RefreshingWidgets,
		DegradedWidgets:   snapshot.DegradedWidgets,
		TotalAttempts:     snapshot.TotalAttempts,
		TotalSuccesses:    snapshot.TotalSuccesses,
		TotalFailures:     snapshot.TotalFailures,
		TotalLockSkips:    snapshot.TotalLockSkips,
		Widgets:           make([]widgetRefreshDiagnosticsResponse, 0, len(snapshot.Widgets)),
	}

	for _, widget := range snapshot.Widgets {
		response.Widgets = append(response.Widgets, widgetRefreshDiagnosticsResponse{
			ID:                  widget.ID,
			Type:                widget.Type,
			Title:               widget.Title,
			Degraded:            widget.Degraded,
			FailureClass:        widget.FailureClass,
			ConsecutiveFailures: widget.ConsecutiveFailures,
			LastAttempt:         optionalDiagnosticTime(widget.LastAttempt),
			LastSuccess:         optionalDiagnosticTime(widget.LastSuccess),
			LastFailure:         optionalDiagnosticTime(widget.LastFailure),
			LastDurationMS:      float64(widget.LastDuration) / float64(time.Millisecond),
			RefreshStartedAt:    optionalDiagnosticTime(widget.RefreshStartedAt),
			Attempts:            widget.Attempts,
			Successes:           widget.Successes,
			Failures:            widget.Failures,
			LockSkips:           widget.LockSkips,
			LastSchedulerLagMS:  float64(widget.LastSchedulerLag) / float64(time.Millisecond),
			MaxSchedulerLagMS:   float64(widget.MaxSchedulerLag) / float64(time.Millisecond),
			NextUpdate:          optionalDiagnosticTime(widget.NextUpdate),
		})
	}

	return response
}

func (a *application) handleRuntimeDiagnosticsRequest(
	w http.ResponseWriter,
	r *http.Request,
) {
	if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
		return
	}

	response := runtimeDiagnosticsResponseFromSnapshot(
		collectRuntimeDiagnostics(a.refreshWidgets),
	)

	configSnapshot := a.configDiagnostics.snapshot()
	response.Config = configRuntimeDiagnosticsResponse{
		Path:              configSnapshot.ConfigPath,
		LoadedAt:          optionalDiagnosticTime(configSnapshot.LoadedAt),
		LastReloadAttempt: optionalDiagnosticTime(configSnapshot.LastReloadAttempt),
		LastReloadResult:  configSnapshot.LastReloadResult,
	}

	if configSnapshot.LastReloadRejection != nil {
		response.Config.LastReloadRejection = &configReloadRejectionResponse{
			At:      configSnapshot.LastReloadRejection.At,
			File:    configSnapshot.LastReloadRejection.File,
			Line:    configSnapshot.LastReloadRejection.Line,
			Message: configSnapshot.LastReloadRejection.Message,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Warn("Failed to encode runtime diagnostics response", "error", err)
	}
}
