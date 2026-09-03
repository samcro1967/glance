package glance

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testRequestDoer func(*http.Request) (*http.Response, error)

func (doer testRequestDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

func TestContentFetchErrorPreservesPartialContentClassificationAndCause(t *testing.T) {
	cause := errors.New("provider unavailable")

	err := contentFetchError(
		errPartialContent,
		2,
		5,
		"RSS feeds",
		cause,
	)

	if !errors.Is(err, errPartialContent) {
		t.Fatalf("partial-content classification was not preserved: %v", err)
	}

	if !errors.Is(err, cause) {
		t.Fatalf("underlying cause was not preserved: %v", err)
	}

	expected := "failed to retrieve some of the content: failed 2 of 5 RSS feeds; first failure: provider unavailable"
	if err.Error() != expected {
		t.Fatalf("unexpected error:\n got: %q\nwant: %q", err.Error(), expected)
	}
}

func TestContentFetchErrorPreservesNoContentClassificationAndCause(t *testing.T) {
	cause := context.DeadlineExceeded

	err := contentFetchError(
		errNoContent,
		3,
		3,
		"releases",
		cause,
	)

	if !errors.Is(err, errNoContent) {
		t.Fatalf("no-content classification was not preserved: %v", err)
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("underlying cause was not preserved: %v", err)
	}

	expected := "failed to retrieve any content: failed 3 of 3 releases; first failure: context deadline exceeded"
	if err.Error() != expected {
		t.Fatalf("unexpected error:\n got: %q\nwant: %q", err.Error(), expected)
	}
}

func TestContentFetchErrorWithoutCause(t *testing.T) {
	err := contentFetchError(
		errPartialContent,
		1,
		4,
		"markets",
		nil,
	)

	if !errors.Is(err, errPartialContent) {
		t.Fatalf("partial-content classification was not preserved: %v", err)
	}

	expected := "failed to retrieve some of the content: failed 1 of 4 markets"
	if err.Error() != expected {
		t.Fatalf("unexpected error:\n got: %q\nwant: %q", err.Error(), expected)
	}
}

func TestDecodeJsonFromRequestHTTPStatusDoesNotExposeRequestOrResponse(t *testing.T) {
	const (
		secretURL      = "https://example.com/data?token=super-secret-token"
		secretResponse = `{"secret":"super-secret-response"}`
	)

	request, err := http.NewRequest(http.MethodGet, secretURL, nil)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	client := testRequestDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Status:     "401 Unauthorized",
			Body:       io.NopCloser(strings.NewReader(secretResponse)),
		}, nil
	})

	_, err = decodeJsonFromRequest[map[string]any](client, request)
	if err == nil {
		t.Fatal("expected HTTP status error")
	}

	message := err.Error()

	if message != "unexpected HTTP status 401 Unauthorized" {
		t.Fatalf("unexpected error: %q", message)
	}

	if strings.Contains(message, "super-secret-token") {
		t.Fatalf("error exposed request query secret: %q", message)
	}

	if strings.Contains(message, "super-secret-response") {
		t.Fatalf("error exposed response body: %q", message)
	}

	if strings.Contains(message, secretURL) {
		t.Fatalf("error exposed request URL: %q", message)
	}
}

func TestDecodeXmlFromRequestHTTPStatusDoesNotExposeRequestOrResponse(t *testing.T) {
	const (
		secretURL      = "https://example.com/feed?api_key=super-secret-key"
		secretResponse = `<response><secret>super-secret-response</secret></response>`
	)

	request, err := http.NewRequest(http.MethodGet, secretURL, nil)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	client := testRequestDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Status:     "429 Too Many Requests",
			Body:       io.NopCloser(strings.NewReader(secretResponse)),
		}, nil
	})

	_, err = decodeXmlFromRequest[struct{}](client, request)
	if err == nil {
		t.Fatal("expected HTTP status error")
	}

	message := err.Error()

	if message != "unexpected HTTP status 429 Too Many Requests" {
		t.Fatalf("unexpected error: %q", message)
	}

	if strings.Contains(message, "super-secret-key") {
		t.Fatalf("error exposed request query secret: %q", message)
	}

	if strings.Contains(message, "super-secret-response") {
		t.Fatalf("error exposed response body: %q", message)
	}

	if strings.Contains(message, secretURL) {
		t.Fatalf("error exposed request URL: %q", message)
	}
}

func TestDecodeJsonFromRequestPreservesTransportError(t *testing.T) {
	transportErr := errors.New("transport failure")

	request, err := http.NewRequest(
		http.MethodGet,
		"https://example.com/data?token=super-secret-token",
		nil,
	)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	client := testRequestDoer(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})

	_, err = decodeJsonFromRequest[struct{}](client, request)
	if err == nil {
		t.Fatal("expected transport error")
	}

	if !errors.Is(err, transportErr) {
		t.Fatalf("transport error was not preserved: %v", err)
	}

	if err.Error() != "sending HTTP request: transport failure" {
		t.Fatalf("unexpected error: %q", err)
	}

	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("error exposed request query secret: %q", err)
	}
}

func TestDecodeJsonFromRequestTransportURLErrorDoesNotExposeURL(t *testing.T) {
	const secretURL = "https://example.com/data?token=super-secret-token"

	transportCause := errors.New("connection refused")

	request, err := http.NewRequest(http.MethodGet, secretURL, nil)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	client := testRequestDoer(func(*http.Request) (*http.Response, error) {
		return nil, &url.Error{
			Op:  "Get",
			URL: secretURL,
			Err: transportCause,
		}
	})

	_, err = decodeJsonFromRequest[struct{}](client, request)
	if err == nil {
		t.Fatal("expected transport error")
	}

	if !errors.Is(err, transportCause) {
		t.Fatalf("transport cause was not preserved: %v", err)
	}

	if strings.Contains(err.Error(), secretURL) {
		t.Fatalf("transport error exposed request URL: %q", err)
	}

	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("transport error exposed request query secret: %q", err)
	}

	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("transport error lost useful cause: %q", err)
	}
}

func TestDecodeJsonFromRequestPreservesDecodeError(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://example.com/data", nil)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	client := testRequestDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"broken":`)),
		}, nil
	})

	_, err = decodeJsonFromRequest[map[string]any](client, request)
	if err == nil {
		t.Fatal("expected JSON decode error")
	}

	if !strings.HasPrefix(err.Error(), "decoding JSON response: ") {
		t.Fatalf("unexpected error: %q", err)
	}

	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("JSON syntax error was not preserved: %v", err)
	}
}

func TestDecodeXmlFromRequestPreservesDecodeError(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://example.com/feed", nil)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	client := testRequestDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`<feed><item></feed>`)),
		}, nil
	})

	_, err = decodeXmlFromRequest[struct{}](client, request)
	if err == nil {
		t.Fatal("expected XML decode error")
	}

	if !strings.HasPrefix(err.Error(), "decoding XML response: ") {
		t.Fatalf("unexpected error: %q", err)
	}

	var syntaxErr *xml.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("XML syntax error was not preserved: %v", err)
	}
}

func TestWorkerPoolDoProcessesAllItems(t *testing.T) {
	task := func(input int) (int, error) {
		return input * 2, nil
	}

	results, errs, err := workerPoolDo(
		newJob(task, []int{1, 2, 3, 4}).withWorkers(2),
	)
	if err != nil {
		t.Fatalf("workerPoolDo returned unexpected error: %v", err)
	}

	expected := []int{2, 4, 6, 8}

	for i := range expected {
		if errs[i] != nil {
			t.Fatalf("item %d returned unexpected error: %v", i, errs[i])
		}

		if results[i] != expected[i] {
			t.Fatalf(
				"item %d returned %d, expected %d",
				i,
				results[i],
				expected[i],
			)
		}
	}
}

func TestWorkerPoolDoUsesBackgroundContextByDefault(t *testing.T) {
	job := newJob(
		func(input int) (int, error) {
			return input, nil
		},
		[]int{1},
	)

	if job.ctx == nil {
		t.Fatal("newJob created a nil context")
	}

	if err := job.ctx.Err(); err != nil {
		t.Fatalf("default job context unexpectedly has an error: %v", err)
	}
}

func TestWorkerPoolWithContextIgnoresNilContext(t *testing.T) {
	job := newJob(
		func(input int) (int, error) {
			return input, nil
		},
		[]int{1},
	)

	originalContext := job.ctx
	job.withContext(nil)

	if job.ctx != originalContext {
		t.Fatal("withContext(nil) replaced the existing context")
	}
}

func TestWorkerPoolDoReturnsCancelledBeforeSingleItemExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls atomic.Int32

	task := func(input int) (int, error) {
		calls.Add(1)
		return input, nil
	}

	results, errs, err := workerPoolDo(
		newJob(task, []int{1}).withContext(ctx),
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if calls.Load() != 0 {
		t.Fatalf("task executed %d times after cancellation", calls.Load())
	}

	if len(results) != 1 || len(errs) != 1 {
		t.Fatalf(
			"unexpected result sizes: results=%d errs=%d",
			len(results),
			len(errs),
		)
	}
}

func TestWorkerPoolDoReturnsCancelledBeforeMultipleItemExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls atomic.Int32

	task := func(input int) (int, error) {
		calls.Add(1)
		return input, nil
	}

	_, _, err := workerPoolDo(
		newJob(task, []int{1, 2, 3}).withWorkers(2).withContext(ctx),
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if calls.Load() != 0 {
		t.Fatalf("task executed %d times after cancellation", calls.Load())
	}
}

func TestWorkerPoolDoStopsSchedulingAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})

	var calls atomic.Int32

	task := func(input int) (int, error) {
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-releaseFirst
		}

		return input, nil
	}

	done := make(chan error, 1)

	go func() {
		_, _, err := workerPoolDo(
			newJob(
				task,
				[]int{1, 2, 3, 4, 5},
			).withWorkers(1).withContext(ctx),
		)

		done <- err
	}()

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first task did not start")
	}

	cancel()
	close(releaseFirst)

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("workerPoolDo did not return after cancellation")
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf(
			"expected only the in-flight task to execute, got %d executions",
			got,
		)
	}
}

func TestWorkerPoolDoEmptyJobReturnsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, errs, err := workerPoolDo(
		newJob(
			func(input int) (int, error) {
				return input, nil
			},
			nil,
		).withContext(ctx),
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if len(results) != 0 || len(errs) != 0 {
		t.Fatalf(
			"expected empty results, got results=%d errs=%d",
			len(results),
			len(errs),
		)
	}
}

func TestUnexpectedHTTPStatusErrorPreservesStatusCode(t *testing.T) {
	err := unexpectedHTTPStatusError(&http.Response{
		StatusCode: http.StatusForbidden,
		Status:     "403 Forbidden",
	})

	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error type = %T, want *httpStatusError", err)
	}

	if statusErr.StatusCode != http.StatusForbidden {
		t.Fatalf("status code = %d, want %d", statusErr.StatusCode, http.StatusForbidden)
	}

	if err.Error() != "unexpected HTTP status 403 Forbidden" {
		t.Fatalf("error = %q, want %q", err.Error(), "unexpected HTTP status 403 Forbidden")
	}
}

func TestClassifyRefreshFailure(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantClass refreshFailureClass
		retryable bool
	}{
		{
			name:      "cancelled",
			err:       fmt.Errorf("wrapped: %w", context.Canceled),
			wantClass: refreshFailureCancelled,
			retryable: false,
		},
		{
			name:      "deadline exceeded",
			err:       fmt.Errorf("wrapped: %w", context.DeadlineExceeded),
			wantClass: refreshFailureTransient,
			retryable: true,
		},
		{
			name:      "request timeout",
			err:       fmt.Errorf("wrapped: %w", unexpectedHTTPStatusError(&http.Response{StatusCode: http.StatusRequestTimeout, Status: "408 Request Timeout"})),
			wantClass: refreshFailureTransient,
			retryable: true,
		},
		{
			name:      "too many requests",
			err:       fmt.Errorf("wrapped: %w", unexpectedHTTPStatusError(&http.Response{StatusCode: http.StatusTooManyRequests, Status: "429 Too Many Requests"})),
			wantClass: refreshFailureRateLimited,
			retryable: false,
		},
		{
			name:      "unauthorized",
			err:       fmt.Errorf("wrapped: %w", unexpectedHTTPStatusError(&http.Response{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized"})),
			wantClass: refreshFailureAuthentication,
			retryable: false,
		},
		{
			name:      "forbidden",
			err:       fmt.Errorf("wrapped: %w", unexpectedHTTPStatusError(&http.Response{StatusCode: http.StatusForbidden, Status: "403 Forbidden"})),
			wantClass: refreshFailureAuthorization,
			retryable: false,
		},
		{
			name:      "not found",
			err:       fmt.Errorf("wrapped: %w", unexpectedHTTPStatusError(&http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found"})),
			wantClass: refreshFailureRequest,
			retryable: false,
		},
		{
			name:      "bad gateway",
			err:       fmt.Errorf("wrapped: %w", unexpectedHTTPStatusError(&http.Response{StatusCode: http.StatusBadGateway, Status: "502 Bad Gateway"})),
			wantClass: refreshFailureTransient,
			retryable: true,
		},
		{
			name:      "json syntax",
			err:       fmt.Errorf("decoding JSON response: %w", &json.SyntaxError{Offset: 1}),
			wantClass: refreshFailureMalformed,
			retryable: true,
		},
		{
			name:      "xml syntax",
			err:       fmt.Errorf("decoding XML response: %w", &xml.SyntaxError{Msg: "invalid XML", Line: 1}),
			wantClass: refreshFailureMalformed,
			retryable: true,
		},
		{
			name:      "unknown",
			err:       errors.New("provider-specific failure"),
			wantClass: refreshFailureUnknown,
			retryable: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyRefreshFailure(test.err); got != test.wantClass {
				t.Fatalf("class = %q, want %q", got, test.wantClass)
			}

			if got := refreshFailureRetryable(test.err); got != test.retryable {
				t.Fatalf("retryable = %t, want %t", got, test.retryable)
			}
		})
	}
}
