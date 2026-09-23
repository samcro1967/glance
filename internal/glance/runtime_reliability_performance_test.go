package glance

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

type reliabilitySoakWidget struct {
	widgetBase
	updates atomic.Uint64
	mode    atomic.Int32
}

func newReliabilitySoakWidget() *reliabilitySoakWidget {
	widget := &reliabilitySoakWidget{}
	widget.cacheDuration = time.Minute
	widget.cacheType = cacheTypeDuration
	return widget
}

func (widget *reliabilitySoakWidget) initialize() error {
	return nil
}

func (widget *reliabilitySoakWidget) update(ctx context.Context) {
	widget.updates.Add(1)

	if err := ctx.Err(); err != nil {
		widget.canContinueUpdateAfterHandlingErr(err)
		return
	}

	switch widget.mode.Load() {
	case 1:
		widget.ContentAvailable = false
		widget.canContinueUpdateAfterHandlingErr(
			fmt.Errorf("%w: synthetic transient failure", errNoContent),
		)
	case 2:
		widget.ContentAvailable = true
		widget.canContinueUpdateAfterHandlingErr(
			fmt.Errorf("%w: synthetic partial failure", errPartialContent),
		)
	default:
		widget.ContentAvailable = true
		widget.canContinueUpdateAfterHandlingErr(nil)
	}
}

func (widget *reliabilitySoakWidget) Render() template.HTML {
	return template.HTML("")
}

func benchmarkRefreshFanout(b *testing.B, widgetCount int) {
	widgets := make([]widget, widgetCount)
	for i := range widgets {
		candidate := newReliabilitySoakWidget()
		candidate.setID(uint64(i + 1))
		widgets[i] = candidate
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, candidate := range widgets {
			base, _ := widgetBaseOf(candidate)
			base.setNextUpdateTime(time.Time{})
		}
		refreshDueWidgets(context.Background(), widgets, widgetRefreshConcurrency, nil)
	}
}

func BenchmarkWidgetRefreshFanout1(b *testing.B) {
	benchmarkRefreshFanout(b, 1)
}

func BenchmarkWidgetRefreshFanout10(b *testing.B) {
	benchmarkRefreshFanout(b, 10)
}

func BenchmarkWidgetRefreshFanout50(b *testing.B) {
	benchmarkRefreshFanout(b, 50)
}

func BenchmarkWidgetRefreshFanout100(b *testing.B) {
	benchmarkRefreshFanout(b, 100)
}

func BenchmarkWidgetRefreshFanout175(b *testing.B) {
	benchmarkRefreshFanout(b, 175)
}

func TestWidgetRefreshSchedulerReliabilitySoak(t *testing.T) {
	const (
		widgetCount = 175
		cycles      = 25
	)

	widgets := make([]widget, widgetCount)
	for i := range widgets {
		candidate := newReliabilitySoakWidget()
		candidate.setID(uint64(i + 1))
		widgets[i] = candidate
	}

	before := runtime.NumGoroutine()

	for cycle := 0; cycle < cycles; cycle++ {
		for _, candidate := range widgets {
			base, _ := widgetBaseOf(candidate)
			base.setNextUpdateTime(time.Time{})
		}

		refreshDueWidgets(
			context.Background(),
			widgets,
			widgetRefreshConcurrency,
			nil,
		)
	}

	for i, candidate := range widgets {
		base, ok := widgetBaseOf(candidate)
		if !ok {
			t.Fatalf("widget %d has no widget base", i)
		}

		if got := base.refreshAttempts; got != cycles {
			t.Fatalf(
				"widget %d refresh attempts = %d, want %d",
				i,
				got,
				cycles,
			)
		}

		if got := base.refreshSuccesses; got != cycles {
			t.Fatalf(
				"widget %d refresh successes = %d, want %d",
				i,
				got,
				cycles,
			)
		}

		if base.refreshFailures != 0 {
			t.Fatalf(
				"widget %d refresh failures = %d, want 0",
				i,
				base.refreshFailures,
			)
		}

		if !base.refreshStartedAt.IsZero() {
			t.Fatalf("widget %d remained marked as refreshing", i)
		}
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)

	after := runtime.NumGoroutine()
	if after > before+widgetRefreshConcurrency+4 {
		t.Fatalf(
			"goroutines grew from %d to %d after scheduler soak",
			before,
			after,
		)
	}
}

func TestWidgetLifecycleFailureRecoverySoak(t *testing.T) {
	const cycles = 100

	candidate := newReliabilitySoakWidget()
	candidate.setID(1)

	for cycle := 0; cycle < cycles; cycle++ {
		candidate.setNextUpdateTime(time.Time{})

		if cycle%3 == 0 {
			candidate.mode.Store(1)
		} else if cycle%3 == 1 {
			candidate.mode.Store(2)
		} else {
			candidate.mode.Store(0)
		}

		refreshDueWidgets(
			context.Background(),
			[]widget{candidate},
			1,
			nil,
		)

		if !candidate.refreshStartedAt.IsZero() {
			t.Fatalf(
				"cycle %d left refresh marked in progress",
				cycle,
			)
		}
	}

	if candidate.refreshAttempts != cycles {
		t.Fatalf(
			"refresh attempts = %d, want %d",
			candidate.refreshAttempts,
			cycles,
		)
	}

	candidate.mode.Store(0)
	candidate.setNextUpdateTime(time.Time{})
	refreshDueWidgets(context.Background(), []widget{candidate}, 1, nil)

	if candidate.Error != nil {
		t.Fatalf("recovery left error: %v", candidate.Error)
	}
	if candidate.Notice != nil {
		t.Fatalf("recovery left notice: %v", candidate.Notice)
	}
	if candidate.refreshDegraded {
		t.Fatal("recovery left widget degraded")
	}
	if candidate.refreshFailureCount != 0 {
		t.Fatalf(
			"recovery failure count = %d, want 0",
			candidate.refreshFailureCount,
		)
	}
	if candidate.refreshFailureClass != refreshFailureUnknown {
		t.Fatalf(
			"recovery failure class = %q, want %q",
			candidate.refreshFailureClass,
			refreshFailureUnknown,
		)
	}
}

func TestRefreshCancellationDoesNotBecomeFailureSoak(t *testing.T) {
	const cycles = 100

	candidate := newReliabilitySoakWidget()
	candidate.setID(1)

	for cycle := 0; cycle < cycles; cycle++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		candidate.setNextUpdateTime(time.Time{})
		refreshDueWidgets(ctx, []widget{candidate}, 1, nil)
	}

	if candidate.refreshFailures != 0 {
		t.Fatalf(
			"cancelled refresh failures = %d, want 0",
			candidate.refreshFailures,
		)
	}

	if errors.Is(candidate.Error, context.Canceled) {
		t.Fatalf("cancelled scheduler polluted widget error: %v", candidate.Error)
	}
}

type reliabilityTimeoutError struct{}

func (reliabilityTimeoutError) Error() string   { return "synthetic timeout" }
func (reliabilityTimeoutError) Timeout() bool   { return true }
func (reliabilityTimeoutError) Temporary() bool { return true }

func TestRefreshFailureContractMatrix(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantClass     refreshFailureClass
		wantRetryable bool
		wantContinue  bool
		wantError     bool
		wantNotice    bool
	}{
		{
			name:          "cancelled",
			err:           context.Canceled,
			wantClass:     refreshFailureCancelled,
			wantRetryable: false,
			wantContinue:  false,
		},
		{
			name:          "deadline",
			err:           context.DeadlineExceeded,
			wantClass:     refreshFailureTransient,
			wantRetryable: true,
			wantContinue:  false,
			wantError:     true,
		},
		{
			name:          "network timeout",
			err:           reliabilityTimeoutError{},
			wantClass:     refreshFailureTransient,
			wantRetryable: true,
			wantContinue:  false,
			wantError:     true,
		},
		{
			name:          "EOF",
			err:           fmt.Errorf("wrapped: %w", io.EOF),
			wantClass:     refreshFailureTransient,
			wantRetryable: true,
			wantContinue:  false,
			wantError:     true,
		},
		{
			name:          "unexpected EOF",
			err:           fmt.Errorf("wrapped: %w", io.ErrUnexpectedEOF),
			wantClass:     refreshFailureTransient,
			wantRetryable: true,
			wantContinue:  false,
			wantError:     true,
		},
		{
			name:          "401",
			err:           &httpStatusError{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized"},
			wantClass:     refreshFailureAuthentication,
			wantRetryable: false,
			wantContinue:  false,
			wantError:     true,
		},
		{
			name:          "403",
			err:           &httpStatusError{StatusCode: http.StatusForbidden, Status: "403 Forbidden"},
			wantClass:     refreshFailureAuthorization,
			wantRetryable: false,
			wantContinue:  false,
			wantError:     true,
		},
		{
			name:          "404",
			err:           &httpStatusError{StatusCode: http.StatusNotFound, Status: "404 Not Found"},
			wantClass:     refreshFailureRequest,
			wantRetryable: false,
			wantContinue:  false,
			wantError:     true,
		},
		{
			name:          "429",
			err:           &httpStatusError{StatusCode: http.StatusTooManyRequests, Status: "429 Too Many Requests"},
			wantClass:     refreshFailureRateLimited,
			wantRetryable: false,
			wantContinue:  false,
			wantError:     true,
		},
		{
			name:          "500",
			err:           &httpStatusError{StatusCode: http.StatusInternalServerError, Status: "500 Internal Server Error"},
			wantClass:     refreshFailureTransient,
			wantRetryable: true,
			wantContinue:  false,
			wantError:     true,
		},
		{
			name:          "json malformed",
			err:           &json.SyntaxError{},
			wantClass:     refreshFailureMalformed,
			wantRetryable: true,
			wantContinue:  false,
			wantError:     true,
		},
		{
			name:          "xml malformed",
			err:           &xml.SyntaxError{},
			wantClass:     refreshFailureMalformed,
			wantRetryable: true,
			wantContinue:  false,
			wantError:     true,
		},
		{
			name:          "oversized",
			err:           &httpResponseTooLargeError{},
			wantClass:     refreshFailureMalformed,
			wantRetryable: true,
			wantContinue:  false,
			wantError:     true,
		},
		{
			name:          "partial",
			err:           fmt.Errorf("%w: synthetic partial response", errPartialContent),
			wantClass:     refreshFailureUnknown,
			wantRetryable: true,
			wantContinue:  true,
			wantNotice:    true,
		},
	}

	var _ net.Error = reliabilityTimeoutError{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			widget := newReliabilitySoakWidget()
			widget.setID(1)
			widget.Type = "reliability-test"
			widget.ContentAvailable = tt.name == "partial"

			if got := classifyRefreshFailure(tt.err); got != tt.wantClass {
				t.Fatalf("failure class = %q, want %q", got, tt.wantClass)
			}

			if got := refreshFailureRetryable(tt.err); got != tt.wantRetryable {
				t.Fatalf("retryable = %t, want %t", got, tt.wantRetryable)
			}

			gotContinue := widget.canContinueUpdateAfterHandlingErr(tt.err)
			if gotContinue != tt.wantContinue {
				t.Fatalf("continue = %t, want %t", gotContinue, tt.wantContinue)
			}

			if tt.wantClass == refreshFailureCancelled {
				if widget.refreshDegraded {
					t.Fatal("cancelled refresh marked widget degraded")
				}
				if widget.refreshFailureCount != 0 {
					t.Fatalf("cancelled refresh failure count = %d, want 0", widget.refreshFailureCount)
				}
				if widget.Error != nil || widget.Notice != nil {
					t.Fatalf("cancelled refresh polluted widget state: error=%v notice=%v", widget.Error, widget.Notice)
				}
				return
			}

			if !widget.refreshDegraded {
				t.Fatal("failed refresh did not mark widget degraded")
			}
			if widget.refreshFailureClass != tt.wantClass {
				t.Fatalf("stored failure class = %q, want %q", widget.refreshFailureClass, tt.wantClass)
			}
			if widget.refreshFailureCount != 1 {
				t.Fatalf("failure count = %d, want 1", widget.refreshFailureCount)
			}
			if (widget.Error != nil) != tt.wantError {
				t.Fatalf("error presence = %t, want %t; error=%v", widget.Error != nil, tt.wantError, widget.Error)
			}
			if (widget.Notice != nil) != tt.wantNotice {
				t.Fatalf("notice presence = %t, want %t; notice=%v", widget.Notice != nil, tt.wantNotice, widget.Notice)
			}

			if !widget.canContinueUpdateAfterHandlingErr(nil) {
				t.Fatal("successful recovery unexpectedly stopped update")
			}
			if widget.refreshDegraded {
				t.Fatal("successful recovery left widget degraded")
			}
			if widget.refreshFailureCount != 0 {
				t.Fatalf("recovery failure count = %d, want 0", widget.refreshFailureCount)
			}
			if widget.refreshFailureClass != refreshFailureUnknown {
				t.Fatalf("recovery failure class = %q, want %q", widget.refreshFailureClass, refreshFailureUnknown)
			}
			if widget.Error != nil || widget.Notice != nil {
				t.Fatalf("successful recovery left error state: error=%v notice=%v", widget.Error, widget.Notice)
			}
		})
	}
}

func TestRefreshFailureRecoveryRepeatedSoak(t *testing.T) {
	const cycles = 250

	widget := newReliabilitySoakWidget()
	widget.setID(1)
	widget.Type = "reliability-test"
	widget.ContentAvailable = true

	failures := []error{
		context.DeadlineExceeded,
		&httpStatusError{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized"},
		&httpStatusError{StatusCode: http.StatusForbidden, Status: "403 Forbidden"},
		&httpStatusError{StatusCode: http.StatusNotFound, Status: "404 Not Found"},
		&httpStatusError{StatusCode: http.StatusTooManyRequests, Status: "429 Too Many Requests"},
		&httpStatusError{StatusCode: http.StatusInternalServerError, Status: "500 Internal Server Error"},
		&json.SyntaxError{},
		&httpResponseTooLargeError{},
		fmt.Errorf("%w: synthetic partial response", errPartialContent),
	}

	before := runtime.NumGoroutine()

	for cycle := 0; cycle < cycles; cycle++ {
		err := failures[cycle%len(failures)]
		widget.ContentAvailable = true

		widget.canContinueUpdateAfterHandlingErr(err)

		if !widget.refreshDegraded {
			t.Fatalf("cycle %d did not enter degraded state for %v", cycle, err)
		}

		widget.canContinueUpdateAfterHandlingErr(nil)

		if widget.refreshDegraded {
			t.Fatalf("cycle %d remained degraded after recovery", cycle)
		}
		if widget.refreshFailureCount != 0 {
			t.Fatalf("cycle %d failure count = %d after recovery", cycle, widget.refreshFailureCount)
		}
		if widget.Error != nil || widget.Notice != nil {
			t.Fatalf("cycle %d retained error state after recovery: error=%v notice=%v", cycle, widget.Error, widget.Notice)
		}
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)

	after := runtime.NumGoroutine()
	if after > before+4 {
		t.Fatalf("goroutines grew from %d to %d after failure/recovery soak", before, after)
	}
}
