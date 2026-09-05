package glance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchRemoteServerInfoCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)

		select {
		case <-r.Context().Done():
			close(requestCanceled)
		case <-time.After(2 * time.Second):
			t.Error("server request context was not canceled")
		}
	}))
	defer server.Close()

	infoReq := &serverStatsRequest{
		URL:     server.URL,
		Timeout: durationField(10 * time.Second),
	}

	ctx, cancel := context.WithCancel(context.Background())

	result := make(chan error, 1)
	go func() {
		_, err := fetchRemoteServerInfo(ctx, infoReq)
		result <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("remote server stats request did not reach server")
	}

	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected canceled remote server stats request to return an error")
		}

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("remote server stats request did not stop after context cancellation")
	}

	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("server did not observe remote server stats request cancellation")
	}
}

func TestFetchRemoteServerInfoTimeout(t *testing.T) {
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)

		select {
		case <-r.Context().Done():
			close(requestCanceled)
		case <-time.After(2 * time.Second):
			t.Error("server request context was not canceled by timeout")
		}
	}))
	defer server.Close()

	infoReq := &serverStatsRequest{
		URL:     server.URL,
		Timeout: durationField(50 * time.Millisecond),
	}

	result := make(chan error, 1)
	go func() {
		_, err := fetchRemoteServerInfo(context.Background(), infoReq)
		result <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("remote server stats request did not reach server")
	}

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected remote server stats request to time out")
		}

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("remote server stats timeout did not stop request")
	}

	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("server did not observe remote server stats timeout")
	}
}

func TestFetchRemoteServerInfoAllowInsecure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sysinfo/all" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hostname":"tls-test"}`))
	}))
	defer server.Close()

	request := &serverStatsRequest{
		URL:           server.URL,
		Timeout:       durationField(2 * time.Second),
		AllowInsecure: true,
	}

	info, err := fetchRemoteServerInfo(context.Background(), request)
	if err != nil {
		t.Fatalf("fetchRemoteServerInfo with allow-insecure: %v", err)
	}
	if info == nil {
		t.Fatal("fetchRemoteServerInfo returned nil info")
	}
}

func TestServerStatsUpdateSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hostname":"test-server"}`))
	}))
	defer server.Close()

	widget := &serverStatsWidget{
		Servers: []serverStatsRequest{
			{
				Type:    "remote",
				URL:     server.URL,
				Timeout: durationField(time.Second),
			},
		},
	}
	widget.Type = "server-stats"
	widget.ContentAvailable = true
	widget.withCacheDuration(time.Minute)

	widget.update(context.Background())

	if widget.Error != nil {
		t.Fatalf("successful refresh set error: %v", widget.Error)
	}
	if widget.Notice != nil {
		t.Fatalf("successful refresh set notice: %v", widget.Notice)
	}
	if widget.refreshDegraded {
		t.Fatal("successful refresh marked widget degraded")
	}
	if !widget.Servers[0].IsReachable {
		t.Fatal("successful server marked unreachable")
	}
}

func TestServerStatsUpdatePartialFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hostname":"test-server"}`))
	}))
	defer server.Close()

	widget := &serverStatsWidget{
		Servers: []serverStatsRequest{
			{
				Type:    "remote",
				URL:     server.URL,
				Timeout: durationField(time.Second),
			},
			{
				Type:    "remote",
				URL:     "://invalid",
				Timeout: durationField(time.Second),
			},
		},
	}
	widget.Type = "server-stats"
	widget.ContentAvailable = true
	widget.withCacheDuration(time.Minute)

	widget.update(context.Background())

	if widget.Error != nil {
		t.Fatalf("partial refresh set error: %v", widget.Error)
	}
	if !errors.Is(widget.Notice, errPartialContent) {
		t.Fatalf("partial refresh notice = %v, want errPartialContent", widget.Notice)
	}
	if !widget.refreshDegraded {
		t.Fatal("partial refresh did not mark widget degraded")
	}
	if !widget.Servers[0].IsReachable {
		t.Fatal("successful server marked unreachable")
	}
	if widget.Servers[1].IsReachable {
		t.Fatal("failed server marked reachable")
	}
}

func TestServerStatsUpdateTotalFailure(t *testing.T) {
	widget := &serverStatsWidget{
		Servers: []serverStatsRequest{
			{
				Type:    "remote",
				URL:     "://invalid-one",
				Timeout: durationField(time.Second),
			},
			{
				Type:    "remote",
				URL:     "://invalid-two",
				Timeout: durationField(time.Second),
			},
		},
	}
	widget.Type = "server-stats"
	widget.ContentAvailable = true
	widget.withCacheDuration(time.Minute)

	widget.update(context.Background())

	if !errors.Is(widget.Error, errNoContent) {
		t.Fatalf("failed refresh error = %v, want errNoContent", widget.Error)
	}
	if widget.Notice != nil {
		t.Fatalf("failed refresh set notice: %v", widget.Notice)
	}
	if !widget.refreshDegraded {
		t.Fatal("failed refresh did not mark widget degraded")
	}
	if widget.Servers[0].IsReachable || widget.Servers[1].IsReachable {
		t.Fatal("failed server marked reachable")
	}
}

func TestServerStatsUpdateCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)

		select {
		case <-r.Context().Done():
			close(requestCanceled)
		case <-time.After(2 * time.Second):
			t.Error("server request context was not canceled")
		}
	}))
	defer server.Close()

	widget := &serverStatsWidget{
		Servers: []serverStatsRequest{
			{
				Type:    "remote",
				URL:     server.URL,
				Timeout: durationField(10 * time.Second),
			},
		},
	}
	widget.Type = "server-stats"
	widget.ContentAvailable = true
	widget.withCacheDuration(time.Minute)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		widget.update(ctx)
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("server stats refresh did not reach remote server")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("server stats refresh did not stop after cancellation")
	}

	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("remote server did not observe cancellation")
	}

	if widget.refreshDegraded {
		t.Fatal("canceled refresh marked widget degraded")
	}
	if widget.Error != nil {
		t.Fatalf("canceled refresh set error: %v", widget.Error)
	}
	if widget.Notice != nil {
		t.Fatalf("canceled refresh set notice: %v", widget.Notice)
	}
	if widget.refreshFailureCount != 0 {
		t.Fatalf(
			"canceled refresh failure count = %d, want 0",
			widget.refreshFailureCount,
		)
	}
}
