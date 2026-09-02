package glance

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func resetRSSResourceRequests(t *testing.T) {
	t.Helper()

	rssResourceRequests.Lock()
	old := rssResourceRequests.current
	rssResourceRequests.current = make(map[[32]byte]*rssResourceCall)
	rssResourceRequests.Unlock()

	t.Cleanup(func() {
		rssResourceRequests.Lock()
		rssResourceRequests.current = old
		rssResourceRequests.Unlock()
	})
}

func rssResourceTestRequest(t *testing.T, headers map[string]string) *http.Request {
	t.Helper()

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"https://example.invalid/feed.xml",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	for key, value := range headers {
		request.Header.Set(key, value)
	}

	return request
}

func rssResourceTestResponse(request *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"ETag": []string{`"v1"`}},
		Body:       io.NopCloser(strings.NewReader("feed body")),
		Request:    request,
	}
}

func TestRSSResourceCoalescesConcurrentEquivalentRequests(t *testing.T) {
	resetRSSResourceRequests(t)

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return rssResourceTestResponse(request), nil
	})

	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan error, callers)

	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			request := rssResourceTestRequest(t, map[string]string{
				"User-Agent": glanceUserAgentString,
				"X-Test":     "same",
			})
			_, err := fetchRSSResource(context.Background(), request)
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

func TestRSSResourceDoesNotCacheCompletedRequest(t *testing.T) {
	resetRSSResourceRequests(t)

	var calls atomic.Int32
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return rssResourceTestResponse(request), nil
	})

	for i := 0; i < 2; i++ {
		request := rssResourceTestRequest(t, nil)
		if _, err := fetchRSSResource(context.Background(), request); err != nil {
			t.Fatalf("fetch %d: %v", i+1, err)
		}
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("HTTP calls = %d, want 2", got)
	}

	rssResourceRequests.Lock()
	active := len(rssResourceRequests.current)
	rssResourceRequests.Unlock()

	if active != 0 {
		t.Fatalf("active RSS resource calls = %d, want 0", active)
	}
}

func TestRSSResourceCanonicalizesHeaderOrder(t *testing.T) {
	first := rssResourceTestRequest(t, nil)
	first.Header.Add("X-Second", "two")
	first.Header.Add("X-First", "b")
	first.Header.Add("X-First", "a")

	second := rssResourceTestRequest(t, nil)
	second.Header.Add("X-First", "a")
	second.Header.Add("X-First", "b")
	second.Header.Add("X-Second", "two")

	if got, want := rssResourceRequestKey(first), rssResourceRequestKey(second); got != want {
		t.Fatalf("equivalent headers produced different keys")
	}
}

func TestRSSResourceKeepsDifferentHeadersIndependent(t *testing.T) {
	first := rssResourceTestRequest(t, map[string]string{"Authorization": "Bearer first"})
	second := rssResourceTestRequest(t, map[string]string{"Authorization": "Bearer second"})

	if got, want := rssResourceRequestKey(first), rssResourceRequestKey(second); got == want {
		t.Fatalf("different headers produced identical keys")
	}
}

func TestRSSResourceWaitingCallerCanCancel(t *testing.T) {
	resetRSSResourceRequests(t)

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		close(started)
		<-release
		return rssResourceTestResponse(request), nil
	})

	go func() {
		request := rssResourceTestRequest(t, map[string]string{"X-Test": "same"})
		_, err := fetchRSSResource(context.Background(), request)
		firstDone <- err
	}()

	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	request := rssResourceTestRequest(t, map[string]string{"X-Test": "same"})
	request = request.WithContext(ctx)

	if _, err := fetchRSSResource(ctx, request); err != context.Canceled {
		t.Fatalf("waiting fetch error = %v, want %v", err, context.Canceled)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("original fetch error: %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1", got)
	}
}

func TestRSSWidgetsShareResourceWithoutSharingConfiguration(t *testing.T) {
	resetRSSResourceRequests(t)

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release

		body := `<?xml version="1.0"?>
<rss version="2.0">
<channel>
<title>Original Feed</title>
<link>https://example.com</link>
<item>
<title>First Item</title>
<link>/first</link>
<description>First description</description>
<category>News</category>
<pubDate>Sun, 30 Aug 2026 18:00:00 GMT</pubDate>
</item>
<item>
<title>Second Item</title>
<link>/second</link>
<description>Second description</description>
<category>Sports</category>
<pubDate>Sun, 30 Aug 2026 17:00:00 GMT</pubDate>
</item>
</channel>
</rss>`

		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"ETag": []string{`"v1"`}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})

	first := &rssWidget{}
	second := &rssWidget{}

	if err := first.initialize(); err != nil {
		t.Fatalf("first initialize: %v", err)
	}
	if err := second.initialize(); err != nil {
		t.Fatalf("second initialize: %v", err)
	}

	firstRequest := rssFeedRequest{
		URL:   "https://example.invalid/shared.xml",
		Title: "First Feed",
		Limit: 1,
	}

	secondRequest := rssFeedRequest{
		URL:            "https://example.invalid/shared.xml",
		Title:          "Second Feed",
		ItemLinkPrefix: "https://proxy.example/?url=",
		IsDetailed:     true,
	}

	type result struct {
		items []rssFeedItem
		err   error
	}

	firstResult := make(chan result, 1)
	secondResult := make(chan result, 1)

	go func() {
		items, err := first.fetchItemsFromFeedTask(
			context.Background(),
			firstRequest,
		)
		firstResult <- result{items: items, err: err}
	}()

	<-started

	go func() {
		items, err := second.fetchItemsFromFeedTask(
			context.Background(),
			secondRequest,
		)
		secondResult <- result{items: items, err: err}
	}()

	time.Sleep(25 * time.Millisecond)
	close(release)

	gotFirst := <-firstResult
	gotSecond := <-secondResult

	if gotFirst.err != nil {
		t.Fatalf("first widget fetch: %v", gotFirst.err)
	}
	if gotSecond.err != nil {
		t.Fatalf("second widget fetch: %v", gotSecond.err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("RSS HTTP calls = %d, want 1", got)
	}

	if len(gotFirst.items) != 1 {
		t.Fatalf("first widget items = %d, want 1", len(gotFirst.items))
	}
	if len(gotSecond.items) != 2 {
		t.Fatalf("second widget items = %d, want 2", len(gotSecond.items))
	}

	if gotFirst.items[0].ChannelName != "First Feed" {
		t.Fatalf(
			"first channel name = %q, want %q",
			gotFirst.items[0].ChannelName,
			"First Feed",
		)
	}
	if gotSecond.items[0].ChannelName != "Second Feed" {
		t.Fatalf(
			"second channel name = %q, want %q",
			gotSecond.items[0].ChannelName,
			"Second Feed",
		)
	}

	if gotFirst.items[0].Link != "https://example.com/first" {
		t.Fatalf("first link = %q", gotFirst.items[0].Link)
	}
	if gotSecond.items[0].Link != "https://proxy.example/?url=/first" {
		t.Fatalf("second link = %q", gotSecond.items[0].Link)
	}

	if gotFirst.items[0].Description != "" {
		t.Fatalf("first description = %q, want empty", gotFirst.items[0].Description)
	}
	if gotSecond.items[0].Description != "First description" {
		t.Fatalf(
			"second description = %q, want %q",
			gotSecond.items[0].Description,
			"First description",
		)
	}
}

type rssFailingReader struct{}

func (rssFailingReader) Read([]byte) (int, error) {
	return 0, fmt.Errorf("test read failure")
}

func TestRSSResourcePreservesTransportErrorContext(t *testing.T) {
	resetRSSResourceRequests(t)

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("test transport failure")
	})

	request := rssResourceTestRequest(t, nil)
	_, err := fetchRSSResource(context.Background(), request)
	if err == nil {
		t.Fatal("fetch error = nil, want transport error")
	}

	if !strings.Contains(err.Error(), "sending RSS request") {
		t.Fatalf("fetch error = %q, want sending RSS request context", err)
	}
}

func TestRSSResourcePreservesReadErrorContext(t *testing.T) {
	resetRSSResourceRequests(t)

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(rssFailingReader{}),
			Request:    request,
		}, nil
	})

	request := rssResourceTestRequest(t, nil)
	_, err := fetchRSSResource(context.Background(), request)
	if err == nil {
		t.Fatal("fetch error = nil, want read error")
	}

	if !strings.Contains(err.Error(), "reading RSS response") {
		t.Fatalf("fetch error = %q, want reading RSS response context", err)
	}
}
