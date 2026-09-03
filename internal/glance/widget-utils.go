package glance

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	errNoContent      = errors.New("failed to retrieve any content")
	errPartialContent = errors.New("failed to retrieve some of the content")
)

const defaultClientTimeout = 5 * time.Second

var defaultHTTPClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		Proxy:               http.ProxyFromEnvironment,
	},
	Timeout: defaultClientTimeout,
}

var defaultInsecureHTTPClient = &http.Client{
	Timeout: defaultClientTimeout,
	Transport: &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		Proxy:               http.ProxyFromEnvironment,
	},
}

type requestDoer interface {
	Do(*http.Request) (*http.Response, error)
}

var glanceUserAgentString = "Glance/" + buildVersion + " +https://github.com/samcro1967/glance"
var userAgentPersistentVersion atomic.Int32

func getBrowserUserAgentHeader() string {
	if rand.IntN(2000) == 0 {
		userAgentPersistentVersion.Store(rand.Int32N(5))
	}

	version := strconv.Itoa(148 + int(userAgentPersistentVersion.Load()))
	return "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:" + version + ".0) Gecko/20100101 Firefox/" + version + ".0"
}

func setBrowserUserAgentHeader(request *http.Request) {
	request.Header.Set("User-Agent", getBrowserUserAgentHeader())
}

type httpStatusError struct {
	StatusCode int
	Status     string
}

func (err *httpStatusError) Error() string {
	if err.Status != "" {
		return fmt.Sprintf("unexpected HTTP status %s", err.Status)
	}

	return fmt.Sprintf("unexpected HTTP status %d", err.StatusCode)
}

func unexpectedHTTPStatusError(response *http.Response) error {
	if response == nil {
		return errors.New("unexpected HTTP response")
	}

	return &httpStatusError{
		StatusCode: response.StatusCode,
		Status:     response.Status,
	}
}

type refreshFailureClass string

const (
	refreshFailureUnknown        refreshFailureClass = "unknown"
	refreshFailureCancelled      refreshFailureClass = "cancelled"
	refreshFailureTransient      refreshFailureClass = "transient"
	refreshFailureRateLimited    refreshFailureClass = "rate-limit"
	refreshFailureAuthentication refreshFailureClass = "authentication"
	refreshFailureAuthorization  refreshFailureClass = "authorization"
	refreshFailureRequest        refreshFailureClass = "request"
	refreshFailureMalformed      refreshFailureClass = "malformed"
)

func classifyRefreshFailure(err error) refreshFailureClass {
	if err == nil {
		return refreshFailureUnknown
	}

	if errors.Is(err, context.Canceled) {
		return refreshFailureCancelled
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return refreshFailureTransient
	}

	var statusErr *httpStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusRequestTimeout:
			return refreshFailureTransient
		case http.StatusTooManyRequests:
			return refreshFailureRateLimited
		case http.StatusUnauthorized:
			return refreshFailureAuthentication
		case http.StatusForbidden:
			return refreshFailureAuthorization
		}

		if statusErr.StatusCode >= 500 && statusErr.StatusCode <= 599 {
			return refreshFailureTransient
		}

		if statusErr.StatusCode >= 400 && statusErr.StatusCode <= 499 {
			return refreshFailureRequest
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return refreshFailureTransient
	}

	var jsonSyntaxErr *json.SyntaxError
	if errors.As(err, &jsonSyntaxErr) {
		return refreshFailureMalformed
	}

	var xmlSyntaxErr *xml.SyntaxError
	if errors.As(err, &xmlSyntaxErr) {
		return refreshFailureMalformed
	}

	return refreshFailureUnknown
}

func refreshFailureRetryable(err error) bool {
	switch classifyRefreshFailure(err) {
	case refreshFailureCancelled,
		refreshFailureRateLimited,
		refreshFailureAuthentication,
		refreshFailureAuthorization,
		refreshFailureRequest:
		return false
	default:
		return true
	}
}

func safeHTTPTransportError(err error) error {
	for {
		var urlErr *url.Error
		if !errors.As(err, &urlErr) || urlErr.Err == nil {
			return err
		}

		err = urlErr.Err
	}
}

func contentFetchError(
	classification error,
	failed int,
	total int,
	resource string,
	cause error,
) error {
	if cause == nil {
		return fmt.Errorf(
			"%w: failed %d of %d %s",
			classification,
			failed,
			total,
			resource,
		)
	}

	return fmt.Errorf(
		"%w: failed %d of %d %s; first failure: %w",
		classification,
		failed,
		total,
		resource,
		cause,
	)
}

func decodeJsonFromRequest[T any](client requestDoer, request *http.Request) (T, error) {
	var result T

	response, err := client.Do(request)
	if err != nil {
		return result, fmt.Errorf(
			"sending HTTP request: %w",
			safeHTTPTransportError(err),
		)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return result, fmt.Errorf("reading HTTP response: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return result, unexpectedHTTPStatusError(response)
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return result, fmt.Errorf("decoding JSON response: %w", err)
	}

	return result, nil
}

func decodeJsonFromRequestTask[T any](client requestDoer) func(*http.Request) (T, error) {
	return func(request *http.Request) (T, error) {
		return decodeJsonFromRequest[T](client, request)
	}
}

// TODO: tidy up, these are a copy of the above but with a line changed
func decodeXmlFromRequest[T any](client requestDoer, request *http.Request) (T, error) {
	var result T

	response, err := client.Do(request)
	if err != nil {
		return result, fmt.Errorf(
			"sending HTTP request: %w",
			safeHTTPTransportError(err),
		)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return result, fmt.Errorf("reading HTTP response: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return result, unexpectedHTTPStatusError(response)
	}

	if err := xml.Unmarshal(body, &result); err != nil {
		return result, fmt.Errorf("decoding XML response: %w", err)
	}

	return result, nil
}

type cachedEntry[T any] struct {
	value     T
	timestamp time.Time
}

type workerPoolTask[I any, O any] struct {
	index  int
	input  I
	output O
	err    error
}

type workerPoolJob[I any, O any] struct {
	data    []I
	workers int
	task    func(I) (O, error)
	ctx     context.Context
}

const defaultNumWorkers = 10

func (job *workerPoolJob[I, O]) withWorkers(workers int) *workerPoolJob[I, O] {
	if workers == 0 {
		job.workers = defaultNumWorkers
	} else {
		job.workers = min(workers, len(job.data))
	}

	return job
}

func (job *workerPoolJob[I, O]) withContext(ctx context.Context) *workerPoolJob[I, O] {
	if ctx != nil {
		job.ctx = ctx
	}

	return job
}

func newJob[I any, O any](task func(I) (O, error), data []I) *workerPoolJob[I, O] {
	return &workerPoolJob[I, O]{
		workers: defaultNumWorkers,
		task:    task,
		data:    data,
		ctx:     context.Background(),
	}
}

func workerPoolDo[I any, O any](job *workerPoolJob[I, O]) ([]O, []error, error) {
	results := make([]O, len(job.data))
	errs := make([]error, len(job.data))

	if len(job.data) == 0 {
		return results, errs, job.ctx.Err()
	}

	if err := job.ctx.Err(); err != nil {
		return results, errs, err
	}

	if len(job.data) == 1 {
		select {
		case <-job.ctx.Done():
			return results, errs, job.ctx.Err()
		default:
		}

		results[0], errs[0] = job.task(job.data[0])
		return results, errs, job.ctx.Err()
	}

	tasksQueue := make(chan *workerPoolTask[I, O])
	resultsQueue := make(chan *workerPoolTask[I, O])

	var workersWG sync.WaitGroup

	for range job.workers {
		workersWG.Add(1)

		go func() {
			defer workersWG.Done()

			for {
				select {
				case <-job.ctx.Done():
					return

				case task, ok := <-tasksQueue:
					if !ok {
						return
					}

					if job.ctx.Err() != nil {
						return
					}

					task.output, task.err = job.task(task.input)

					select {
					case resultsQueue <- task:
					case <-job.ctx.Done():
						return
					}
				}
			}
		}()
	}

	go func() {
		defer close(tasksQueue)

		for i := range job.data {
			task := &workerPoolTask[I, O]{
				index: i,
				input: job.data[i],
			}

			select {
			case tasksQueue <- task:
			case <-job.ctx.Done():
				return
			}
		}
	}()

	go func() {
		workersWG.Wait()
		close(resultsQueue)
	}()

	for task := range resultsQueue {
		errs[task.index] = task.err
		results[task.index] = task.output
	}

	return results, errs, job.ctx.Err()
}
