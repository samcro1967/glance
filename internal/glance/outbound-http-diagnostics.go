package glance

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// Keep at most this many distinct destinations plus the shared overflow bucket.
const outboundHTTPDiagnosticsDestinationLimit = 128

const outboundHTTPDiagnosticsOverflowDestination = "OTHER"

var outboundHTTPDiagnostics = newOutboundHTTPRuntimeDiagnostics()

func observeHTTPTransport(transport http.RoundTripper) http.RoundTripper {
	return observedRoundTripper{
		transport:   transport,
		diagnostics: outboundHTTPDiagnostics,
	}
}

type outboundHTTPDestinationDiagnostics struct {
	Exchanges       uint64
	TransportErrors uint64
	Status1xx       uint64
	Status2xx       uint64
	Status3xx       uint64
	Status4xx       uint64
	Status5xx       uint64
	OtherResponses  uint64
	TotalDuration   time.Duration
	LastDuration    time.Duration
	MaxDuration     time.Duration
	LastExchangeAt  time.Time
}

type outboundHTTPRuntimeDiagnosticsSnapshot struct {
	StartedAt       time.Time
	Exchanges       uint64
	TransportErrors uint64
	Status1xx       uint64
	Status2xx       uint64
	Status3xx       uint64
	Status4xx       uint64
	Status5xx       uint64
	OtherResponses  uint64
	TotalDuration   time.Duration
	MaxDuration     time.Duration
	Destinations    map[string]outboundHTTPDestinationDiagnostics
}

type outboundHTTPRuntimeDiagnostics struct {
	mu sync.Mutex

	startedAt       time.Time
	exchanges       uint64
	transportErrors uint64
	status1xx       uint64
	status2xx       uint64
	status3xx       uint64
	status4xx       uint64
	status5xx       uint64
	otherResponses  uint64
	totalDuration   time.Duration
	maxDuration     time.Duration
	destinations    map[string]outboundHTTPDestinationDiagnostics
}

func newOutboundHTTPRuntimeDiagnostics() *outboundHTTPRuntimeDiagnostics {
	return &outboundHTTPRuntimeDiagnostics{
		startedAt:    time.Now(),
		destinations: make(map[string]outboundHTTPDestinationDiagnostics),
	}
}

func outboundHTTPDestination(request *http.Request) string {
	if request == nil || request.URL == nil {
		return "UNKNOWN"
	}

	method := strings.ToUpper(strings.TrimSpace(request.Method))
	if method == "" {
		method = http.MethodGet
	}

	scheme := strings.ToLower(strings.TrimSpace(request.URL.Scheme))
	if scheme == "" {
		scheme = "unknown"
	}

	host := strings.ToLower(request.URL.Hostname())
	if host == "" {
		host = "UNKNOWN"
	} else if port := request.URL.Port(); port != "" {
		host += ":" + port
	}

	return method + " " + scheme + "://" + host
}

func (d *outboundHTTPRuntimeDiagnostics) record(
	request *http.Request,
	response *http.Response,
	err error,
	duration time.Duration,
	at time.Time,
) {
	if d == nil {
		return
	}

	destination := outboundHTTPDestination(request)

	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.destinations[destination]; !exists &&
		len(d.destinations) >= outboundHTTPDiagnosticsDestinationLimit {
		destination = outboundHTTPDiagnosticsOverflowDestination
	}

	entry := d.destinations[destination]
	entry.Exchanges++
	entry.TotalDuration += duration
	entry.LastDuration = duration
	entry.LastExchangeAt = at

	if duration > entry.MaxDuration {
		entry.MaxDuration = duration
	}

	d.exchanges++
	d.totalDuration += duration

	if duration > d.maxDuration {
		d.maxDuration = duration
	}

	if err != nil {
		entry.TransportErrors++
		d.transportErrors++
	} else if response != nil {
		switch response.StatusCode / 100 {
		case 1:
			entry.Status1xx++
			d.status1xx++
		case 2:
			entry.Status2xx++
			d.status2xx++
		case 3:
			entry.Status3xx++
			d.status3xx++
		case 4:
			entry.Status4xx++
			d.status4xx++
		case 5:
			entry.Status5xx++
			d.status5xx++
		default:
			entry.OtherResponses++
			d.otherResponses++
		}
	} else {
		entry.OtherResponses++
		d.otherResponses++
	}

	d.destinations[destination] = entry
}

func (d *outboundHTTPRuntimeDiagnostics) snapshot() outboundHTTPRuntimeDiagnosticsSnapshot {
	if d == nil {
		return outboundHTTPRuntimeDiagnosticsSnapshot{}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	destinations := make(
		map[string]outboundHTTPDestinationDiagnostics,
		len(d.destinations),
	)

	for destination, entry := range d.destinations {
		destinations[destination] = entry
	}

	return outboundHTTPRuntimeDiagnosticsSnapshot{
		StartedAt:       d.startedAt,
		Exchanges:       d.exchanges,
		TransportErrors: d.transportErrors,
		Status1xx:       d.status1xx,
		Status2xx:       d.status2xx,
		Status3xx:       d.status3xx,
		Status4xx:       d.status4xx,
		Status5xx:       d.status5xx,
		OtherResponses:  d.otherResponses,
		TotalDuration:   d.totalDuration,
		MaxDuration:     d.maxDuration,
		Destinations:    destinations,
	}
}

type observedRoundTripper struct {
	transport   http.RoundTripper
	diagnostics *outboundHTTPRuntimeDiagnostics
}

func (transport observedRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	started := time.Now()

	response, err := transport.transport.RoundTrip(request)

	transport.diagnostics.record(
		request,
		response,
		err,
		time.Since(started),
		time.Now(),
	)

	return response, err
}
