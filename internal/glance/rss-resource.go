package glance

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type rssResourceResponse struct {
	Status     string
	StatusCode int
	Header     http.Header
	Body       []byte
}

type rssResourceCall struct {
	done chan struct{}
	val  rssResourceResponse
	err  error
}

type rssResourceRequestOptions struct {
	Timeout       durationField
	AllowInsecure bool
}

var rssResourceRequests = struct {
	sync.Mutex
	current map[[32]byte]*rssResourceCall
}{
	current: make(map[[32]byte]*rssResourceCall),
}

func rssResourceRequestKey(request *http.Request, options rssResourceRequestOptions) [32]byte {
	var builder strings.Builder

	builder.WriteString(request.Method)
	builder.WriteByte(0)
	builder.WriteString(request.URL.String())
	builder.WriteByte(0)
	builder.WriteString(fmt.Sprintf("%d", options.Timeout))
	builder.WriteByte(0)
	builder.WriteString(fmt.Sprintf("%t", options.AllowInsecure))
	builder.WriteByte(0)

	keys := make([]string, 0, len(request.Header))
	for key := range request.Header {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte(0)

		values := append([]string(nil), request.Header.Values(key)...)
		sort.Strings(values)

		for _, value := range values {
			builder.WriteString(value)
			builder.WriteByte(0)
		}
	}

	return sha256.Sum256([]byte(builder.String()))
}

func fetchRSSResource(ctx context.Context, request *http.Request, options rssResourceRequestOptions) (rssResourceResponse, error) {
	key := rssResourceRequestKey(request, options)

	rssResourceRequests.Lock()
	if call, ok := rssResourceRequests.current[key]; ok {
		rssResourceRequests.Unlock()

		select {
		case <-call.done:
			return call.val, call.err
		case <-ctx.Done():
			return rssResourceResponse{}, ctx.Err()
		}
	}

	call := &rssResourceCall{done: make(chan struct{})}
	rssResourceRequests.current[key] = call
	rssResourceRequests.Unlock()

	call.val, call.err = fetchRSSResourceUncached(request, options)

	rssResourceRequests.Lock()
	delete(rssResourceRequests.current, key)
	close(call.done)
	rssResourceRequests.Unlock()

	return call.val, call.err
}

func fetchRSSResourceUncached(request *http.Request, options rssResourceRequestOptions) (rssResourceResponse, error) {
	baseClient := ternary(options.AllowInsecure, defaultInsecureHTTPClient, defaultHTTPClient)
	client := *baseClient

	if options.Timeout > 0 {
		client.Timeout = time.Duration(options.Timeout)
	}

	response, err := client.Do(request)
	if err != nil {
		return rssResourceResponse{}, fmt.Errorf("sending RSS request: %w", safeHTTPTransportError(err))
	}
	defer response.Body.Close()

	resource := rssResourceResponse{
		Status:     response.Status,
		StatusCode: response.StatusCode,
		Header:     response.Header.Clone(),
	}

	if response.StatusCode != http.StatusOK {
		return resource, nil
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return rssResourceResponse{}, fmt.Errorf("reading RSS response: %w", err)
	}

	resource.Body = body
	return resource, nil
}
