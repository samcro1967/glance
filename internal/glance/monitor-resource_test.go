package glance

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func resetMonitorResourceCache(t *testing.T) {
	t.Helper()

	monitorResourceCache.mu.Lock()
	oldEntries := monitorResourceCache.entries
	monitorResourceCache.entries = make(map[[32]byte]*keyedResourceCacheEntry[siteStatus])
	monitorResourceCache.mu.Unlock()

	t.Cleanup(func() {
		monitorResourceCache.mu.Lock()
		monitorResourceCache.entries = oldEntries
		monitorResourceCache.mu.Unlock()
	})
}

func monitorResourceTestResponse(request *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Status:     "204 No Content",
		Header:     make(http.Header),
		Body:       http.NoBody,
		Request:    request,
	}
}

func TestMonitorResourceCachesEquivalentRequest(t *testing.T) {
	resetMonitorResourceCache(t)

	var calls atomic.Int32
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return monitorResourceTestResponse(request), nil
	})

	request := &SiteStatusRequest{
		DefaultURL: "https://example.invalid/status",
	}

	for i := 0; i < 2; i++ {
		status, err := fetchMonitorSiteResource(context.Background(), request)
		if err != nil {
			t.Fatalf("fetch %d: %v", i+1, err)
		}
		if status.Error != nil {
			t.Fatalf("fetch %d status error: %v", i+1, status.Error)
		}
		if status.Code != http.StatusNoContent {
			t.Fatalf("fetch %d status code = %d, want %d", i+1, status.Code, http.StatusNoContent)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1", got)
	}
}

func TestMonitorResourceCanonicalizesHeaderOrder(t *testing.T) {
	first := &SiteStatusRequest{
		DefaultURL: "https://example.invalid/status",
		Headers: map[string]string{
			"X-Second": "two",
			"X-First":  "one",
		},
	}

	second := &SiteStatusRequest{
		DefaultURL: "https://example.invalid/status",
		Headers: map[string]string{
			"X-First":  "one",
			"X-Second": "two",
		},
	}

	if got, want := monitorResourceRequestKey(first), monitorResourceRequestKey(second); got != want {
		t.Fatal("equivalent headers produced different keys")
	}
}

func TestMonitorResourceKeepsRequestIdentityIndependent(t *testing.T) {
	base := &SiteStatusRequest{
		DefaultURL: "https://example.invalid/status",
		Headers: map[string]string{
			"Authorization": "Bearer first",
		},
	}
	base.BasicAuth.Username = "user"
	base.BasicAuth.Password = "password"

	differentURL := *base
	differentURL.DefaultURL = "https://example.invalid/other"

	differentTLS := *base
	differentTLS.AllowInsecure = true

	differentTimeout := *base
	differentTimeout.Timeout = durationField(7 * time.Second)

	differentAuth := *base
	differentAuth.BasicAuth.Password = "other-password"

	differentHeaders := *base
	differentHeaders.Headers = map[string]string{
		"Authorization": "Bearer second",
	}

	baseKey := monitorResourceRequestKey(base)

	tests := []struct {
		name    string
		request *SiteStatusRequest
	}{
		{name: "URL", request: &differentURL},
		{name: "TLS", request: &differentTLS},
		{name: "timeout", request: &differentTimeout},
		{name: "basic auth", request: &differentAuth},
		{name: "headers", request: &differentHeaders},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := monitorResourceRequestKey(tt.request); got == baseKey {
				t.Fatalf("%s change produced identical resource key", tt.name)
			}
		})
	}
}

func TestMonitorResourceCoalescesConcurrentEquivalentRequests(t *testing.T) {
	resetMonitorResourceCache(t)

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		once.Do(func() {
			close(started)
		})
		<-release
		return monitorResourceTestResponse(request), nil
	})

	request := &SiteStatusRequest{
		DefaultURL: "https://example.invalid/status",
	}

	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan error, callers)

	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()

			status, err := fetchMonitorSiteResource(context.Background(), request)
			if err == nil && status.Error != nil {
				err = status.Error
			}

			errs <- err
		}()
	}

	<-started
	time.Sleep(25 * time.Millisecond)
	close(release)

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("fetch error: %v", err)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1", got)
	}
}

func TestMonitorResourcePreservesHTTPFailureInStatus(t *testing.T) {
	resetMonitorResourceCache(t)

	wantErr := errors.New("monitor unavailable")

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		return nil, wantErr
	})

	status, err := fetchMonitorSiteResource(
		context.Background(),
		&SiteStatusRequest{
			DefaultURL: "https://example.invalid/status",
		},
	)
	if err != nil {
		t.Fatalf("resource error = %v, want nil", err)
	}

	if !errors.Is(status.Error, wantErr) {
		t.Fatalf("status error = %v, want %v", status.Error, wantErr)
	}

	if status.Code != 0 {
		t.Fatalf("status code = %d, want 0", status.Code)
	}
}

func TestMonitorResourceUsesCheckURLForIdentityAndRequest(t *testing.T) {
	resetMonitorResourceCache(t)

	var gotURL string
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		gotURL = request.URL.String()
		return monitorResourceTestResponse(request), nil
	})

	request := &SiteStatusRequest{
		DefaultURL: "https://example.invalid/default",
		CheckURL:   "https://example.invalid/check",
	}

	firstKey := monitorResourceRequestKey(request)
	sameEffectiveRequest := *request
	sameEffectiveRequest.DefaultURL = "https://example.invalid/different-default"
	if got := monitorResourceRequestKey(&sameEffectiveRequest); got != firstKey {
		t.Fatal("changing DefaultURL changed key while CheckURL is configured")
	}

	status, err := fetchMonitorSiteResource(context.Background(), request)
	if err != nil || status.Error != nil {
		t.Fatalf("fetch status = %#v, err = %v", status, err)
	}
	if gotURL != "https://example.invalid/check" {
		t.Fatalf("requested URL = %q, want check URL", gotURL)
	}
}

func TestMonitorResourceDefaultTimeoutMatchesExplicitThreeSeconds(t *testing.T) {
	defaultRequest := &SiteStatusRequest{DefaultURL: "https://example.invalid/status"}
	explicitRequest := &SiteStatusRequest{
		DefaultURL: "https://example.invalid/status",
		Timeout:    durationField(3 * time.Second),
	}

	if got, want := monitorResourceRequestKey(defaultRequest), monitorResourceRequestKey(explicitRequest); got != want {
		t.Fatal("default timeout and explicit 3s timeout produced different keys")
	}
}

func TestMonitorResourceSendsHeadersAndBasicAuth(t *testing.T) {
	resetMonitorResourceCache(t)

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("X-Test"); got != "value" {
			t.Fatalf("X-Test header = %q, want value", got)
		}
		username, password, ok := request.BasicAuth()
		if !ok || username != "user" || password != "secret" {
			t.Fatalf("basic auth = %q/%q/%t, want user/secret/true", username, password, ok)
		}
		return monitorResourceTestResponse(request), nil
	})

	request := &SiteStatusRequest{
		DefaultURL: "https://example.invalid/status",
		Headers: map[string]string{
			"X-Test": "value",
		},
	}
	request.BasicAuth.Username = "user"
	request.BasicAuth.Password = "secret"

	status, err := fetchMonitorSiteResource(context.Background(), request)
	if err != nil || status.Error != nil {
		t.Fatalf("fetch status = %#v, err = %v", status, err)
	}
}

func TestMonitorResourceClassifiesTimeout(t *testing.T) {
	resetMonitorResourceCache(t)

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})

	request := &SiteStatusRequest{
		DefaultURL: "https://example.invalid/timeout",
		Timeout:    durationField(time.Millisecond),
	}

	status, err := fetchMonitorSiteResource(context.Background(), request)
	if err != nil {
		t.Fatalf("resource error = %v", err)
	}
	if !status.TimedOut {
		t.Fatalf("TimedOut = false, status = %#v", status)
	}
	if !errors.Is(status.Error, context.DeadlineExceeded) {
		t.Fatalf("status error = %v, want context deadline exceeded", status.Error)
	}
}

func TestMonitorResourceCachesTransportFailure(t *testing.T) {
	resetMonitorResourceCache(t)

	var calls atomic.Int32
	wantErr := errors.New("transport unavailable")
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, wantErr
	})

	request := &SiteStatusRequest{DefaultURL: "https://example.invalid/failure-cache"}
	for i := 0; i < 2; i++ {
		status, err := fetchMonitorSiteResource(context.Background(), request)
		if err != nil {
			t.Fatalf("fetch %d resource error = %v", i+1, err)
		}
		if !errors.Is(status.Error, wantErr) {
			t.Fatalf("fetch %d status error = %v, want %v", i+1, status.Error, wantErr)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("transport calls = %d, want 1", got)
	}
}

func TestMonitorResourceUsesInsecureClientWhenConfigured(t *testing.T) {
	resetMonitorResourceCache(t)

	old := defaultInsecureHTTPClient.Transport
	var calls atomic.Int32
	defaultInsecureHTTPClient.Transport = priorityRoundTripper(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return monitorResourceTestResponse(request), nil
	})
	t.Cleanup(func() { defaultInsecureHTTPClient.Transport = old })

	request := &SiteStatusRequest{
		DefaultURL:    "https://example.invalid/insecure",
		AllowInsecure: true,
	}
	status, err := fetchMonitorSiteResource(context.Background(), request)
	if err != nil || status.Error != nil {
		t.Fatalf("fetch status = %#v, err = %v", status, err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("insecure transport calls = %d, want 1", got)
	}
}

func TestMonitorResourcePreservesInvalidRequestURLInStatus(t *testing.T) {
	resetMonitorResourceCache(t)

	request := &SiteStatusRequest{DefaultURL: "://invalid"}
	status, err := fetchMonitorSiteResource(context.Background(), request)
	if err != nil {
		t.Fatalf("resource error = %v", err)
	}
	if status.Error == nil {
		t.Fatalf("status error = nil, want invalid URL error")
	}
}
