package glance

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

func formatDiagnosticsReportTime(value *time.Time) string {
	if value == nil {
		return "none"
	}

	return value.Local().Format("2006-01-02 15:04:05 MST")
}

func formatDiagnosticsReportValueTime(value time.Time) string {
	if value.IsZero() {
		return "none"
	}

	return value.Local().Format("2006-01-02 15:04:05 MST")
}

func diagnosticsReportYesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func diagnosticsWidgetLabel(widget widgetRefreshDiagnosticsResponse) string {
	if widget.Title != "" {
		return fmt.Sprintf("%s [%s, id=%d]", widget.Title, widget.Type, widget.ID)
	}
	return fmt.Sprintf("%s [id=%d]", widget.Type, widget.ID)
}

func writeDiagnosticsWidgetProblem(
	report *strings.Builder,
	widget widgetRefreshDiagnosticsResponse,
) {
	fmt.Fprintln(report, diagnosticsWidgetLabel(widget))
	fmt.Fprintln(report, "  State:                 DEGRADED")
	fmt.Fprintf(report, "  Failure class:         %s\n", widget.FailureClass)

	if widget.FailureCause != "" {
		fmt.Fprintf(report, "  Failure cause:         %s\n", widget.FailureCause)
	}

	fmt.Fprintf(report, "  Consecutive failures:  %d\n", widget.ConsecutiveFailures)
	fmt.Fprintf(report, "  Attempts:              %d\n", widget.Attempts)
	fmt.Fprintf(report, "  Successes:             %d\n", widget.Successes)
	fmt.Fprintf(report, "  Failures:              %d\n", widget.Failures)
	fmt.Fprintf(report, "  Last attempt:          %s\n", formatDiagnosticsReportTime(widget.LastAttempt))
	fmt.Fprintf(report, "  Last success:          %s\n", formatDiagnosticsReportTime(widget.LastSuccess))
	fmt.Fprintf(report, "  Last failure:          %s\n", formatDiagnosticsReportTime(widget.LastFailure))
	fmt.Fprintf(report, "  Refresh duration:      %.3f ms\n", widget.LastDurationMS)
	fmt.Fprintf(report, "  Scheduler lag:         %.3f ms\n", widget.LastSchedulerLagMS)

	if widget.LockSkips > 0 {
		fmt.Fprintf(report, "  Lock skips:            %d\n", widget.LockSkips)
	}

	fmt.Fprintln(report)
}

type runtimeDiagnosticsReportIdentity struct {
	Version  string
	Revision string
	Uptime   time.Duration
}

func formatRuntimeDiagnosticsReport(
	response runtimeDiagnosticsResponse,
	identity runtimeDiagnosticsReportIdentity,
	frontendEnabled bool,
	frontend frontendRuntimeDiagnosticsSnapshot,
) string {
	var report strings.Builder

	currentProblems := response.DegradedWidgets > 0 ||
		response.Config.LastReloadRejection != nil ||
		(response.Profiling.Requested &&
			!response.Profiling.Running &&
			response.Profiling.LastFailure != "")

	fmt.Fprintln(&report, "GLANCE RUNTIME DIAGNOSTICS")
	fmt.Fprintln(&report, "==========================")
	fmt.Fprintln(&report)

	if currentProblems {
		fmt.Fprintln(&report, "Overall: ATTENTION")
	} else {
		fmt.Fprintln(&report, "Overall: HEALTHY")
	}

	fmt.Fprintf(
		&report,
		"Generated: %s\n",
		response.GeneratedAt.Local().Format("2006-01-02 15:04:05 MST"),
	)

	fmt.Fprintln(&report)
	fmt.Fprintln(&report, "APPLICATION")
	fmt.Fprintln(&report, "-----------")
	fmt.Fprintf(&report, "Version:   %s\n", identity.Version)
	fmt.Fprintf(&report, "Revision:  %s\n", identity.Revision)
	fmt.Fprintf(&report, "Uptime:    %s\n", formatHealthzDuration(identity.Uptime))

	fmt.Fprintln(&report)
	fmt.Fprintln(&report, "WIDGET REFRESH")
	fmt.Fprintln(&report, "--------------")
	fmt.Fprintf(&report, "Refreshable:  %d\n", response.RefreshWidgets)
	fmt.Fprintf(&report, "Refreshing:   %d\n", response.RefreshingWidgets)
	fmt.Fprintf(&report, "Degraded:     %d\n", response.DegradedWidgets)
	fmt.Fprintf(&report, "Attempts:     %d\n", response.TotalAttempts)
	fmt.Fprintf(&report, "Successes:    %d\n", response.TotalSuccesses)
	fmt.Fprintf(&report, "Failures:     %d\n", response.TotalFailures)
	fmt.Fprintf(&report, "Lock skips:   %d\n", response.TotalLockSkips)

	fmt.Fprintln(&report)
	fmt.Fprintln(&report, "OUTBOUND HTTP")
	fmt.Fprintln(&report, "-------------")
	fmt.Fprintf(&report, "Started:          %s\n", formatDiagnosticsReportTime(response.OutboundHTTP.StartedAt))
	fmt.Fprintf(&report, "Exchanges:        %d\n", response.OutboundHTTP.Exchanges)
	fmt.Fprintf(&report, "Transport errors: %d\n", response.OutboundHTTP.TransportErrors)
	fmt.Fprintf(&report, "Responses:        1xx=%d 2xx=%d 3xx=%d 4xx=%d 5xx=%d other=%d\n", response.OutboundHTTP.Status1xx, response.OutboundHTTP.Status2xx, response.OutboundHTTP.Status3xx, response.OutboundHTTP.Status4xx, response.OutboundHTTP.Status5xx, response.OutboundHTTP.OtherResponses)
	fmt.Fprintf(&report, "Average:          %.3f ms\n", response.OutboundHTTP.AverageDurationMS)
	fmt.Fprintf(&report, "Maximum:          %.3f ms\n", response.OutboundHTTP.MaxDurationMS)
	fmt.Fprintln(&report, "Timing boundary:  transport RoundTrip through response headers; response body/decode excluded")

	if len(response.OutboundHTTP.Destinations) == 0 {
		fmt.Fprintln(&report, "Destinations:     none")
	} else {
		fmt.Fprintln(&report, "Destinations:")
		for _, destination := range response.OutboundHTTP.Destinations {
			fmt.Fprintf(
				&report,
				"  %s  exchanges=%d errors=%d avg=%.3fms last=%.3fms max=%.3fms\n",
				destination.Destination,
				destination.Exchanges,
				destination.TransportErrors,
				destination.AverageDurationMS,
				destination.LastDurationMS,
				destination.MaxDurationMS,
			)
		}
	}

	fmt.Fprintln(&report)
	fmt.Fprintln(&report, "RENDERING")
	fmt.Fprintln(&report, "---------")
	fmt.Fprintf(&report, "Started:                   %s\n", formatDiagnosticsReportTime(response.Rendering.StartedAt))
	fmt.Fprintf(&report, "Widget calls:              %d\n", response.Rendering.WidgetCalls)
	fmt.Fprintf(&report, "Snapshot hits:             %d\n", response.Rendering.WidgetSnapshotHits)
	fmt.Fprintf(&report, "Actual renders:            %d\n", response.Rendering.WidgetRenders)
	fmt.Fprintf(&report, "Refresh-lock waits:        %d\n", response.Rendering.WidgetRefreshLockWaits)
	fmt.Fprintf(&report, "Refresh-lock wait avg/max: %.3f / %.3f ms\n", response.Rendering.WidgetLockWaitAverageMS, response.Rendering.WidgetLockWaitMaxMS)
	if response.Rendering.WidgetRefreshLockWaits > 0 {
		fmt.Fprintf(
			&report,
			"Refresh-lock wait max widget: id=%d type=%s title=%q\n",
			response.Rendering.WidgetLockWaitMaxWidget.ID,
			response.Rendering.WidgetLockWaitMaxWidget.Type,
			response.Rendering.WidgetLockWaitMaxWidget.Title,
		)
	}
	fmt.Fprintf(&report, "Widget render avg/max:     %.3f / %.3f ms\n", response.Rendering.WidgetRenderAverageMS, response.Rendering.WidgetRenderMaxMS)
	if response.Rendering.WidgetRenders > 0 {
		fmt.Fprintf(
			&report,
			"Widget render max widget:    id=%d type=%s title=%q\n",
			response.Rendering.WidgetRenderMaxWidget.ID,
			response.Rendering.WidgetRenderMaxWidget.Type,
			response.Rendering.WidgetRenderMaxWidget.Title,
		)
	}
	fmt.Fprintf(&report, "Page template executions: %d\n", response.Rendering.PageExecutions)
	fmt.Fprintf(&report, "Page template failures:   %d\n", response.Rendering.PageFailures)
	fmt.Fprintf(&report, "Page lock wait avg/max:    %.3f / %.3f ms\n", response.Rendering.PageLockWaitAverageMS, response.Rendering.PageLockWaitMaxMS)
	fmt.Fprintf(&report, "Page template avg/max:     %.3f / %.3f ms\n", response.Rendering.PageTemplateExecutionAverageMS, response.Rendering.PageTemplateExecutionMaxMS)
	fmt.Fprintln(&report, "Timing boundaries:         widget Render() excludes refresh-lock wait; page template execution excludes page-lock wait")

	fmt.Fprintln(&report)
	fmt.Fprintln(&report, "CONFIGURATION")
	fmt.Fprintln(&report, "-------------")

	if response.Config.Path == "" {
		fmt.Fprintln(&report, "Path:          unavailable")
	} else {
		fmt.Fprintf(&report, "Path:          %s\n", response.Config.Path)
	}

	fmt.Fprintf(
		&report,
		"Loaded:        %s\n",
		formatDiagnosticsReportTime(response.Config.LoadedAt),
	)
	fmt.Fprintf(
		&report,
		"Last attempt:  %s\n",
		formatDiagnosticsReportTime(response.Config.LastReloadAttempt),
	)

	if response.Config.LastReloadResult == "" {
		fmt.Fprintln(&report, "Last result:   none")
	} else {
		fmt.Fprintf(
			&report,
			"Last result:   %s\n",
			response.Config.LastReloadResult,
		)
	}

	if response.Config.LastReloadRejection == nil {
		fmt.Fprintln(&report, "Status:        healthy")
	} else {
		fmt.Fprintln(&report, "Status:        reload rejected")
	}

	fmt.Fprintln(&report)
	fmt.Fprintln(&report, "PROFILING")
	fmt.Fprintln(&report, "---------")
	fmt.Fprintf(
		&report,
		"Requested:     %s\n",
		diagnosticsReportYesNo(response.Profiling.Requested),
	)
	fmt.Fprintf(
		&report,
		"Running:       %s\n",
		diagnosticsReportYesNo(response.Profiling.Running),
	)

	if response.Profiling.LastFailure == "" {
		fmt.Fprintln(&report, "Last failure:  none")
	} else {
		fmt.Fprintf(
			&report,
			"Last failure:  %s",
			response.Profiling.LastFailure,
		)

		if response.Profiling.LastFailureAt != nil {
			fmt.Fprintf(
				&report,
				" (%s)",
				formatDiagnosticsReportTime(response.Profiling.LastFailureAt),
			)
		}

		fmt.Fprintln(&report)
	}

	fmt.Fprintln(&report)
	fmt.Fprintln(&report, "FRONTEND DIAGNOSTICS")
	fmt.Fprintln(&report, "--------------------")

	if !frontendEnabled {
		fmt.Fprintln(&report, "Diagnostics:    disabled")
	} else {
		fmt.Fprintln(&report, "Diagnostics:    enabled")

		if frontend.TotalEvents == 0 {
			fmt.Fprintln(
				&report,
				"Activity:       no browser diagnostics received yet",
			)
		} else {
			fmt.Fprintf(
				&report,
				"Last activity:  %s\n",
				formatDiagnosticsReportValueTime(frontend.LastEventAt),
			)
			fmt.Fprintf(&report, "Total events:   %d\n", frontend.TotalEvents)
			fmt.Fprintf(&report, "Problem events: %d\n", frontend.TotalProblemEvents)

			if frontend.LastPage != "" {
				fmt.Fprintf(&report, "Last page:      %s\n", frontend.LastPage)
			}

			if frontend.LastSession != "" {
				fmt.Fprintf(&report, "Last session:   %s\n", frontend.LastSession)
			}

			if len(frontend.ProblemCounts) > 0 {
				fmt.Fprintln(&report, "Problem counts:")

				names := make([]string, 0, len(frontend.ProblemCounts))
				for name := range frontend.ProblemCounts {
					names = append(names, name)
				}

				sort.Strings(names)

				for _, name := range names {
					fmt.Fprintf(
						&report,
						"  %-28s %d\n",
						name,
						frontend.ProblemCounts[name],
					)
				}
			}
		}
	}

	fmt.Fprintln(&report)
	fmt.Fprintln(&report, "CURRENT PROBLEMS")
	fmt.Fprintln(&report, "----------------")

	problemsWritten := false

	for _, widget := range response.Widgets {
		if !widget.Degraded {
			continue
		}

		problemsWritten = true
		writeDiagnosticsWidgetProblem(&report, widget)
	}

	if response.Config.LastReloadRejection != nil {
		problemsWritten = true
		rejection := response.Config.LastReloadRejection

		fmt.Fprintln(&report, "Configuration reload")
		fmt.Fprintln(&report, "  State:         REJECTED")
		fmt.Fprintf(
			&report,
			"  At:            %s\n",
			formatDiagnosticsReportValueTime(rejection.At),
		)

		if rejection.File != "" {
			fmt.Fprintf(&report, "  File:          %s\n", rejection.File)
		}

		if rejection.Line != 0 {
			fmt.Fprintf(&report, "  Line:          %d\n", rejection.Line)
		}

		fmt.Fprintf(&report, "  Error:         %s\n", rejection.Message)
		fmt.Fprintln(&report)
	}

	if response.Profiling.Requested &&
		!response.Profiling.Running &&
		response.Profiling.LastFailure != "" {
		problemsWritten = true
		fmt.Fprintln(&report, "Profiling listener")
		fmt.Fprintln(&report, "  State:         FAILED")
		fmt.Fprintf(
			&report,
			"  Error:         %s\n",
			response.Profiling.LastFailure,
		)
		fmt.Fprintln(&report)
	}

	if !problemsWritten {
		fmt.Fprintln(&report, "None")
	}

	fmt.Fprintln(&report)
	fmt.Fprintln(&report, "RECOVERED / HISTORICAL")
	fmt.Fprintln(&report, "----------------------")

	historyWritten := false

	for _, widget := range response.Widgets {
		if widget.Degraded ||
			(widget.Failures == 0 && widget.LockSkips == 0) {
			continue
		}

		historyWritten = true
		fmt.Fprintln(&report, diagnosticsWidgetLabel(widget))
		fmt.Fprintln(&report, "  State:         recovered")

		if widget.Failures > 0 {
			fmt.Fprintf(&report, "  Failures:      %d\n", widget.Failures)
			fmt.Fprintf(
				&report,
				"  Last failure:  %s\n",
				formatDiagnosticsReportTime(widget.LastFailure),
			)
		}

		if widget.LockSkips > 0 {
			fmt.Fprintf(&report, "  Lock skips:    %d\n", widget.LockSkips)
		}

		fmt.Fprintln(&report)
	}

	if !historyWritten {
		fmt.Fprintln(&report, "None")
	}

	if frontendEnabled {
		fmt.Fprintln(&report)
		fmt.Fprintln(&report, "RECENT FRONTEND PROBLEMS")
		fmt.Fprintln(&report, "------------------------")

		if len(frontend.RecentProblems) == 0 {
			fmt.Fprintln(&report, "None")
		} else {
			for _, problem := range frontend.RecentProblems {
				fmt.Fprintf(
					&report,
					"%s  %s",
					formatDiagnosticsReportValueTime(problem.RecordedAt),
					problem.Event.Event,
				)

				if problem.Event.Page != "" {
					fmt.Fprintf(&report, "  page=%s", problem.Event.Page)
				}

				if problem.Event.Widget != "" {
					fmt.Fprintf(&report, "  widget=%s", problem.Event.Widget)
				}

				if problem.Event.Status != 0 {
					fmt.Fprintf(&report, "  status=%d", problem.Event.Status)
				}

				fmt.Fprintln(&report)

				if problem.Event.Detail != "" {
					fmt.Fprintf(&report, "  %s\n", problem.Event.Detail)
				}
			}
		}

		fmt.Fprintln(&report)
		fmt.Fprintln(&report, "RECENT ACTIVE DIAGNOSTIC RESULTS")
		fmt.Fprintln(&report, "--------------------------------")

		if len(frontend.RecentActiveResults) == 0 {
			fmt.Fprintln(&report, "None")
		} else {
			for _, result := range frontend.RecentActiveResults {
				fmt.Fprintf(
					&report,
					"%s  command=%d  %s",
					formatDiagnosticsReportValueTime(result.RecordedAt),
					result.Event.CommandID,
					result.Event.Event,
				)

				if result.Event.Page != "" {
					fmt.Fprintf(&report, "  page=%s", result.Event.Page)
				}

				if result.Event.Session != "" {
					fmt.Fprintf(&report, "  session=%s", result.Event.Session)
				}

				if result.Event.Widget != "" {
					fmt.Fprintf(&report, "  widget=%s", result.Event.Widget)
				}

				if result.Event.Status != 0 {
					fmt.Fprintf(&report, "  status=%d", result.Event.Status)
				}

				if result.Event.State != nil {
					fmt.Fprintf(&report, "  state=%d", *result.Event.State)
				}

				fmt.Fprintln(&report)

				if result.Event.Detail != "" {
					fmt.Fprintf(&report, "  %s\n", result.Event.Detail)
				}

				if len(result.Event.Metrics) > 0 {
					names := make([]string, 0, len(result.Event.Metrics))
					for name := range result.Event.Metrics {
						names = append(names, name)
					}
					sort.Strings(names)

					fmt.Fprint(&report, "  metrics:")
					for _, name := range names {
						fmt.Fprintf(&report, " %s=%g", name, result.Event.Metrics[name])
					}
					fmt.Fprintln(&report)
				}
			}
		}
	}

	return report.String()
}

func (a *application) handleRuntimeDiagnosticsReportRequest(
	w http.ResponseWriter,
	r *http.Request,
) {
	if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
		return
	}

	response := a.runtimeDiagnosticsResponse()

	uptime := time.Since(a.CreatedAt)
	if a.CreatedAt.IsZero() || uptime < 0 {
		uptime = 0
	}

	var frontend frontendRuntimeDiagnosticsSnapshot
	if a.frontendDiagnostics != nil {
		frontend = a.frontendDiagnostics.snapshot()
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	_, _ = fmt.Fprint(
		w,
		formatRuntimeDiagnosticsReport(
			response,
			runtimeDiagnosticsReportIdentity{
				Version:  a.Version,
				Revision: a.ShortRevision,
				Uptime:   uptime,
			},
			a.Config.Server.FrontendDiagnostics,
			frontend,
		),
	)
}
