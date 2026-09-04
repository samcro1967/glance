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
