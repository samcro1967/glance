package glance

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"time"
)

type widgetRefreshDiagnostics struct {
	ID                  uint64
	Type                string
	Title               string
	Degraded            bool
	FailureClass        refreshFailureClass
	FailureCause        string
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
		FailureCause:        base.lastRefreshError,
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
	GeneratedAt       time.Time                              `json:"generated_at"`
	RefreshWidgets    int                                    `json:"refresh_widgets"`
	RefreshingWidgets int                                    `json:"refreshing_widgets"`
	DegradedWidgets   int                                    `json:"degraded_widgets"`
	TotalAttempts     uint64                                 `json:"total_attempts"`
	TotalSuccesses    uint64                                 `json:"total_successes"`
	TotalFailures     uint64                                 `json:"total_failures"`
	TotalLockSkips    uint64                                 `json:"total_lock_skips"`
	Widgets           []widgetRefreshDiagnosticsResponse     `json:"widgets"`
	Config            configRuntimeDiagnosticsResponse       `json:"config"`
	Profiling         profilingRuntimeDiagnosticsResponse    `json:"profiling"`
	OutboundHTTP      outboundHTTPRuntimeDiagnosticsResponse `json:"outbound_http"`
	Rendering         renderRuntimeDiagnosticsResponse       `json:"rendering"`
}

type outboundHTTPRuntimeDiagnosticsResponse struct {
	StartedAt         *time.Time                                   `json:"started_at,omitempty"`
	Exchanges         uint64                                       `json:"exchanges"`
	TransportErrors   uint64                                       `json:"transport_errors"`
	Status1xx         uint64                                       `json:"status_1xx"`
	Status2xx         uint64                                       `json:"status_2xx"`
	Status3xx         uint64                                       `json:"status_3xx"`
	Status4xx         uint64                                       `json:"status_4xx"`
	Status5xx         uint64                                       `json:"status_5xx"`
	OtherResponses    uint64                                       `json:"other_responses"`
	TotalDurationMS   float64                                      `json:"total_round_trip_duration_ms"`
	AverageDurationMS float64                                      `json:"average_round_trip_duration_ms"`
	MaxDurationMS     float64                                      `json:"max_round_trip_duration_ms"`
	Destinations      []outboundHTTPDestinationDiagnosticsResponse `json:"destinations"`
}

type outboundHTTPDestinationDiagnosticsResponse struct {
	Destination       string     `json:"destination"`
	Exchanges         uint64     `json:"exchanges"`
	TransportErrors   uint64     `json:"transport_errors"`
	Status1xx         uint64     `json:"status_1xx"`
	Status2xx         uint64     `json:"status_2xx"`
	Status3xx         uint64     `json:"status_3xx"`
	Status4xx         uint64     `json:"status_4xx"`
	Status5xx         uint64     `json:"status_5xx"`
	OtherResponses    uint64     `json:"other_responses"`
	TotalDurationMS   float64    `json:"total_round_trip_duration_ms"`
	AverageDurationMS float64    `json:"average_round_trip_duration_ms"`
	LastDurationMS    float64    `json:"last_round_trip_duration_ms"`
	MaxDurationMS     float64    `json:"max_round_trip_duration_ms"`
	LastExchangeAt    *time.Time `json:"last_exchange_at,omitempty"`
}

type renderWidgetAttributionResponse struct {
	ID    uint64 `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

type renderRuntimeDiagnosticsResponse struct {
	StartedAt                      *time.Time                      `json:"started_at,omitempty"`
	WidgetCalls                    uint64                          `json:"widget_calls"`
	WidgetSnapshotHits             uint64                          `json:"widget_snapshot_hits"`
	WidgetRefreshLockWaits         uint64                          `json:"widget_refresh_lock_waits"`
	WidgetLockWaitTotalMS          float64                         `json:"widget_refresh_lock_wait_total_ms"`
	WidgetLockWaitAverageMS        float64                         `json:"widget_refresh_lock_wait_average_ms"`
	WidgetLockWaitMaxMS            float64                         `json:"widget_refresh_lock_wait_max_ms"`
	WidgetLockWaitMaxWidget        renderWidgetAttributionResponse `json:"widget_refresh_lock_wait_max_widget"`
	WidgetRenders                  uint64                          `json:"widget_renders"`
	WidgetRenderTotalMS            float64                         `json:"widget_render_total_ms"`
	WidgetRenderAverageMS          float64                         `json:"widget_render_average_ms"`
	WidgetRenderMaxMS              float64                         `json:"widget_render_max_ms"`
	WidgetRenderMaxWidget          renderWidgetAttributionResponse `json:"widget_render_max_widget"`
	PageExecutions                 uint64                          `json:"page_template_executions"`
	PageFailures                   uint64                          `json:"page_template_failures"`
	PageLockWaitTotalMS            float64                         `json:"page_lock_wait_total_ms"`
	PageLockWaitAverageMS          float64                         `json:"page_lock_wait_average_ms"`
	PageLockWaitMaxMS              float64                         `json:"page_lock_wait_max_ms"`
	PageTemplateExecutionTotalMS   float64                         `json:"page_template_execution_total_ms"`
	PageTemplateExecutionAverageMS float64                         `json:"page_template_execution_average_ms"`
	PageTemplateExecutionMaxMS     float64                         `json:"page_template_execution_max_ms"`
}

func renderRuntimeDiagnosticsResponseFromSnapshot(
	snapshot renderRuntimeDiagnosticsSnapshot,
) renderRuntimeDiagnosticsResponse {
	response := renderRuntimeDiagnosticsResponse{
		StartedAt:              optionalDiagnosticTime(snapshot.StartedAt),
		WidgetCalls:            snapshot.WidgetCalls,
		WidgetSnapshotHits:     snapshot.WidgetSnapshotHits,
		WidgetRefreshLockWaits: snapshot.WidgetRefreshLockWaits,
		WidgetLockWaitTotalMS:  float64(snapshot.WidgetLockWaitTotal) / float64(time.Millisecond),
		WidgetLockWaitMaxMS:    float64(snapshot.WidgetLockWaitMax) / float64(time.Millisecond),
		WidgetLockWaitMaxWidget: renderWidgetAttributionResponse{
			ID:    snapshot.WidgetLockWaitMaxWidget.ID,
			Type:  snapshot.WidgetLockWaitMaxWidget.Type,
			Title: snapshot.WidgetLockWaitMaxWidget.Title,
		},
		WidgetRenders:       snapshot.WidgetRenders,
		WidgetRenderTotalMS: float64(snapshot.WidgetRenderTotal) / float64(time.Millisecond),
		WidgetRenderMaxMS:   float64(snapshot.WidgetRenderMax) / float64(time.Millisecond),
		WidgetRenderMaxWidget: renderWidgetAttributionResponse{
			ID:    snapshot.WidgetRenderMaxWidget.ID,
			Type:  snapshot.WidgetRenderMaxWidget.Type,
			Title: snapshot.WidgetRenderMaxWidget.Title,
		},
		PageExecutions:               snapshot.PageExecutions,
		PageFailures:                 snapshot.PageFailures,
		PageLockWaitTotalMS:          float64(snapshot.PageLockWaitTotal) / float64(time.Millisecond),
		PageLockWaitMaxMS:            float64(snapshot.PageLockWaitMax) / float64(time.Millisecond),
		PageTemplateExecutionTotalMS: float64(snapshot.PageTemplateExecutionTotal) / float64(time.Millisecond),
		PageTemplateExecutionMaxMS:   float64(snapshot.PageTemplateExecutionMax) / float64(time.Millisecond),
	}

	if snapshot.WidgetRefreshLockWaits > 0 {
		response.WidgetLockWaitAverageMS = response.WidgetLockWaitTotalMS / float64(snapshot.WidgetRefreshLockWaits)
	}
	if snapshot.WidgetRenders > 0 {
		response.WidgetRenderAverageMS = response.WidgetRenderTotalMS / float64(snapshot.WidgetRenders)
	}
	if snapshot.PageExecutions > 0 {
		response.PageLockWaitAverageMS = response.PageLockWaitTotalMS / float64(snapshot.PageExecutions)
		response.PageTemplateExecutionAverageMS = response.PageTemplateExecutionTotalMS / float64(snapshot.PageExecutions)
	}

	return response
}

type profilingRuntimeDiagnosticsResponse struct {
	Requested     bool       `json:"requested"`
	Running       bool       `json:"running"`
	LastFailureAt *time.Time `json:"last_failure_at,omitempty"`
	LastFailure   string     `json:"last_failure,omitempty"`
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
	FailureCause        string              `json:"failure_cause,omitempty"`
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

func outboundHTTPRuntimeDiagnosticsResponseFromSnapshot(
	snapshot outboundHTTPRuntimeDiagnosticsSnapshot,
) outboundHTTPRuntimeDiagnosticsResponse {
	response := outboundHTTPRuntimeDiagnosticsResponse{
		StartedAt:       optionalDiagnosticTime(snapshot.StartedAt),
		Exchanges:       snapshot.Exchanges,
		TransportErrors: snapshot.TransportErrors,
		Status1xx:       snapshot.Status1xx,
		Status2xx:       snapshot.Status2xx,
		Status3xx:       snapshot.Status3xx,
		Status4xx:       snapshot.Status4xx,
		Status5xx:       snapshot.Status5xx,
		OtherResponses:  snapshot.OtherResponses,
		TotalDurationMS: float64(snapshot.TotalDuration) / float64(time.Millisecond),
		MaxDurationMS:   float64(snapshot.MaxDuration) / float64(time.Millisecond),
		Destinations:    make([]outboundHTTPDestinationDiagnosticsResponse, 0, len(snapshot.Destinations)),
	}

	if snapshot.Exchanges > 0 {
		response.AverageDurationMS = response.TotalDurationMS / float64(snapshot.Exchanges)
	}

	destinations := make([]string, 0, len(snapshot.Destinations))
	for destination := range snapshot.Destinations {
		destinations = append(destinations, destination)
	}
	slices.Sort(destinations)

	for _, destination := range destinations {
		entry := snapshot.Destinations[destination]
		entryResponse := outboundHTTPDestinationDiagnosticsResponse{
			Destination:     destination,
			Exchanges:       entry.Exchanges,
			TransportErrors: entry.TransportErrors,
			Status1xx:       entry.Status1xx,
			Status2xx:       entry.Status2xx,
			Status3xx:       entry.Status3xx,
			Status4xx:       entry.Status4xx,
			Status5xx:       entry.Status5xx,
			OtherResponses:  entry.OtherResponses,
			TotalDurationMS: float64(entry.TotalDuration) / float64(time.Millisecond),
			LastDurationMS:  float64(entry.LastDuration) / float64(time.Millisecond),
			MaxDurationMS:   float64(entry.MaxDuration) / float64(time.Millisecond),
			LastExchangeAt:  optionalDiagnosticTime(entry.LastExchangeAt),
		}
		if entry.Exchanges > 0 {
			entryResponse.AverageDurationMS = entryResponse.TotalDurationMS / float64(entry.Exchanges)
		}

		response.Destinations = append(response.Destinations, entryResponse)
	}

	return response
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
			FailureCause:        widget.FailureCause,
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

func (a *application) runtimeDiagnosticsResponse() runtimeDiagnosticsResponse {
	response := runtimeDiagnosticsResponseFromSnapshot(
		collectRuntimeDiagnostics(a.refreshWidgets),
	)

	if a.configDiagnostics != nil {
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
	}

	response.OutboundHTTP = outboundHTTPRuntimeDiagnosticsResponseFromSnapshot(
		outboundHTTPDiagnostics.snapshot(),
	)

	response.Rendering = renderRuntimeDiagnosticsResponseFromSnapshot(
		renderDiagnostics.snapshot(),
	)

	if a.profilingDiagnostics != nil {
		profilingSnapshot := a.profilingDiagnostics.snapshot()
		response.Profiling = profilingRuntimeDiagnosticsResponse{
			Requested:     profilingSnapshot.Requested,
			Running:       profilingSnapshot.Running,
			LastFailureAt: optionalDiagnosticTime(profilingSnapshot.LastFailureAt),
			LastFailure:   profilingSnapshot.LastFailure,
		}
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

	response := a.runtimeDiagnosticsResponse()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Warn("Failed to encode runtime diagnostics response", "error", err)
	}
}
