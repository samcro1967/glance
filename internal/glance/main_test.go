package glance

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestStartServerAndReportReturnsStartupError(t *testing.T) {
	startErr := errors.New("address already in use")
	exitChannel := make(chan error, 1)

	startServerAndReport(func() error {
		return startErr
	}, exitChannel)

	err := <-exitChannel
	if !errors.Is(err, startErr) {
		t.Fatalf("expected startup error to be reported, got %v", err)
	}
}

func TestServeUpdateNoticeServerReportsBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		t.Skipf("could not reserve port 8080 for migration notice server test: %v", err)
	}
	defer listener.Close()

	err = serveUpdateNoticeServer(http.NewServeMux())
	if err == nil {
		t.Fatal("expected migration notice server bind failure")
	}

	if !strings.Contains(err.Error(), "serving configuration migration notice") {
		t.Fatalf("error = %q, want migration notice context", err)
	}

	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("error = %q, want address already in use", err)
	}
}

func TestLoadUpdateNoticePage(t *testing.T) {
	body, err := loadUpdateNoticePage()
	if err != nil {
		t.Fatalf("loading configuration migration notice: %v", err)
	}

	if len(body) == 0 {
		t.Fatal("configuration migration notice is empty")
	}
}
