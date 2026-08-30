package glance

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteInternalServerErrorLogsCauseWithoutExposingItToClient(t *testing.T) {
	const (
		logMessage     = "Failed to render test page"
		sensitiveCause = "template failure containing sensitive internal detail"
	)

	var logOutput bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	recorder := httptest.NewRecorder()
	err := errors.New(sensitiveCause)

	writeInternalServerError(recorder, logMessage, err)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf(
			"status = %d, want %d",
			recorder.Code,
			http.StatusInternalServerError,
		)
	}

	body := recorder.Body.String()

	if !strings.Contains(body, "Internal Server Error") {
		t.Fatalf("response body = %q, want generic internal server error", body)
	}

	if strings.Contains(body, sensitiveCause) {
		t.Fatalf("response exposed internal error cause: %q", body)
	}

	logged := logOutput.String()

	if !strings.Contains(logged, `level=ERROR msg="`+logMessage+`"`) {
		t.Fatalf("missing internal server error log: %q", logged)
	}

	if !strings.Contains(logged, `error="`+sensitiveCause+`"`) {
		t.Fatalf("log missing internal error cause: %q", logged)
	}

	if strings.Count(logged, sensitiveCause) != 1 {
		t.Fatalf("internal error cause should appear exactly once in log: %q", logged)
	}
}
