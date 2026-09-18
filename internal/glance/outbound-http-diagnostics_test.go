package glance

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type outboundHTTPTestRoundTripper func(*http.Request) (*http.Response, error)

func (transport outboundHTTPTestRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	return transport(request)
}

func TestObservedRoundTripperPreservesResponseAndRecordsStatus(t *testing.T) {
	diagnostics := newOutboundHTTPRuntimeDiagnostics()
	wantResponse := &http.Response{StatusCode: http.StatusNoContent}

	transport := observedRoundTripper{
		transport: outboundHTTPTestRoundTripper(
			func(*http.Request) (*http.Response, error) {
				return wantResponse, nil
			},
		),
		diagnostics: diagnostics,
	}

	request, err := http.NewRequest(
		http.MethodGet,
		"https://user:password@Example.COM/private/path?token=secret",
		nil,
	)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	gotResponse, gotErr := transport.RoundTrip(request)
	if gotErr != nil {
		t.Fatalf("RoundTrip() error = %v", gotErr)
	}
	if gotResponse != wantResponse {
		t.Fatal("RoundTrip() did not preserve response identity")
	}

	snapshot := diagnostics.snapshot()

	if snapshot.Exchanges != 1 ||
		snapshot.Status2xx != 1 ||
		snapshot.TransportErrors != 0 {
		t.Fatalf(
			"snapshot = %+v, want one successful 2xx request",
			snapshot,
		)
	}

	entry, ok := snapshot.Destinations["GET https://example.com"]
	if !ok {
		t.Fatalf(
			"destinations = %#v, want safe GET example.com key",
			snapshot.Destinations,
		)
	}

	if entry.Exchanges != 1 ||
		entry.Status2xx != 1 ||
		entry.LastExchangeAt.IsZero() {
		t.Fatalf("destination = %+v, want recorded request", entry)
	}

	for destination := range snapshot.Destinations {
		for _, secret := range []string{
			"user",
			"password",
			"private",
			"token",
			"secret",
		} {
			if strings.Contains(destination, secret) {
				t.Fatalf(
					"destination %q leaked sensitive URL component %q",
					destination,
					secret,
				)
			}
		}
	}
}

func TestObservedRoundTripperPreservesTransportErrorIdentity(t *testing.T) {
	diagnostics := newOutboundHTTPRuntimeDiagnostics()
	wantErr := errors.New("transport failed")

	transport := observedRoundTripper{
		transport: outboundHTTPTestRoundTripper(
			func(*http.Request) (*http.Response, error) {
				return nil, wantErr
			},
		),
		diagnostics: diagnostics,
	}

	request, err := http.NewRequest(
		http.MethodPost,
		"https://example.com/api",
		nil,
	)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	_, gotErr := transport.RoundTrip(request)
	if gotErr != wantErr {
		t.Fatal("RoundTrip() did not preserve transport error identity")
	}

	snapshot := diagnostics.snapshot()

	if snapshot.Exchanges != 1 ||
		snapshot.TransportErrors != 1 ||
		snapshot.Status2xx != 0 ||
		snapshot.Status3xx != 0 ||
		snapshot.Status4xx != 0 ||
		snapshot.Status5xx != 0 {
		t.Fatalf(
			"snapshot = %+v, want one transport error and no HTTP status",
			snapshot,
		)
	}
}

func TestObservedRoundTripperNilDiagnosticsIsPassThrough(t *testing.T) {
	wantResponse := &http.Response{StatusCode: http.StatusOK}

	transport := observedRoundTripper{
		transport: outboundHTTPTestRoundTripper(
			func(*http.Request) (*http.Response, error) {
				return wantResponse, nil
			},
		),
	}

	request, err := http.NewRequest(
		http.MethodGet,
		"https://example.com",
		nil,
	)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	gotResponse, gotErr := transport.RoundTrip(request)
	if gotErr != nil {
		t.Fatalf("RoundTrip() error = %v", gotErr)
	}
	if gotResponse != wantResponse {
		t.Fatal("RoundTrip() did not preserve response identity")
	}
}

func TestOutboundHTTPRuntimeDiagnosticsStatusClasses(t *testing.T) {
	diagnostics := newOutboundHTTPRuntimeDiagnostics()

	request, err := http.NewRequest(
		http.MethodGet,
		"https://example.com",
		nil,
	)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	for _, status := range []int{
		http.StatusOK,
		http.StatusFound,
		http.StatusNotFound,
		http.StatusBadGateway,
	} {
		diagnostics.record(
			request,
			&http.Response{StatusCode: status},
			nil,
			time.Millisecond,
			time.Now(),
		)
	}

	snapshot := diagnostics.snapshot()

	if snapshot.Exchanges != 4 ||
		snapshot.Status2xx != 1 ||
		snapshot.Status3xx != 1 ||
		snapshot.Status4xx != 1 ||
		snapshot.Status5xx != 1 {
		t.Fatalf(
			"snapshot = %+v, want one request in each HTTP status class",
			snapshot,
		)
	}
}

func TestOutboundHTTPRuntimeDiagnosticsBoundsDestinations(t *testing.T) {
	diagnostics := newOutboundHTTPRuntimeDiagnostics()

	for i := 0; i < outboundHTTPDiagnosticsDestinationLimit+10; i++ {
		request, err := http.NewRequest(
			http.MethodGet,
			fmt.Sprintf("https://host-%d.example", i),
			nil,
		)
		if err != nil {
			t.Fatalf("creating request %d: %v", i, err)
		}

		diagnostics.record(
			request,
			&http.Response{StatusCode: http.StatusOK},
			nil,
			time.Millisecond,
			time.Now(),
		)
	}

	snapshot := diagnostics.snapshot()

	if got := len(snapshot.Destinations); got != outboundHTTPDiagnosticsDestinationLimit+1 {
		t.Fatalf(
			"destination count = %d, want %d tracked destinations plus overflow",
			got,
			outboundHTTPDiagnosticsDestinationLimit,
		)
	}

	overflow := snapshot.Destinations[outboundHTTPDiagnosticsOverflowDestination]
	if overflow.Exchanges != 10 {
		t.Fatalf("overflow = %+v, want 10 requests", overflow)
	}
}

func TestOutboundHTTPRuntimeDiagnosticsConcurrentAccess(t *testing.T) {
	diagnostics := newOutboundHTTPRuntimeDiagnostics()

	request, err := http.NewRequest(
		http.MethodGet,
		"https://example.com",
		nil,
	)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	const goroutines = 8
	const requestsPerGoroutine = 250

	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				diagnostics.record(
					request,
					&http.Response{StatusCode: http.StatusOK},
					nil,
					time.Millisecond,
					time.Now(),
				)

				_ = diagnostics.snapshot()
			}
		}()
	}

	wg.Wait()

	snapshot := diagnostics.snapshot()
	wantRequests := uint64(goroutines * requestsPerGoroutine)

	if snapshot.Exchanges != wantRequests {
		t.Fatalf(
			"requests = %d, want %d",
			snapshot.Exchanges,
			wantRequests,
		)
	}

	entry := snapshot.Destinations["GET https://example.com"]
	if entry.Exchanges != wantRequests {
		t.Fatalf(
			"destination requests = %d, want %d",
			entry.Exchanges,
			wantRequests,
		)
	}
}

func TestOutboundHTTPRuntimeDiagnosticsNilSafe(t *testing.T) {
	var diagnostics *outboundHTTPRuntimeDiagnostics

	diagnostics.record(
		nil,
		nil,
		errors.New("ignored"),
		time.Millisecond,
		time.Now(),
	)

	got := diagnostics.snapshot()

	if !got.StartedAt.IsZero() ||
		got.Exchanges != 0 ||
		got.TransportErrors != 0 ||
		got.Status2xx != 0 ||
		got.Status3xx != 0 ||
		got.Status4xx != 0 ||
		got.Status5xx != 0 ||
		got.TotalDuration != 0 ||
		got.MaxDuration != 0 ||
		got.Destinations != nil {
		t.Fatalf("nil snapshot = %+v, want empty state", got)
	}
}

func BenchmarkOutboundHTTPRuntimeDiagnosticsRecord(b *testing.B) {
	diagnostics := newOutboundHTTPRuntimeDiagnostics()
	request, err := http.NewRequest(http.MethodGet, "https://api.example/data?token=secret", nil)
	if err != nil {
		b.Fatal(err)
	}
	response := &http.Response{StatusCode: http.StatusOK}
	at := time.Now()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		diagnostics.record(request, response, nil, time.Millisecond, at)
	}
}
