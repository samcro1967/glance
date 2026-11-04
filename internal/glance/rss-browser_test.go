package glance

import (
	"context"
	"errors"
	"fmt"
	"github.com/chromedp/cdproto/network"
	"strings"
	"testing"
	"time"
)

func TestRSSBrowserResourceCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchRSSBrowserResource(
		ctx,
		browserTestConfig(),
		"https://www.reddit.com/r/selfhosted/.rss",
		0,
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled browser fetch error = %v", err)
	}
}

func TestRSSBrowserResourceUnavailableEndpoint(t *testing.T) {
	cfg := browserTestConfig()
	cfg.Endpoint = "http://127.0.0.1:1"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := fetchRSSBrowserResource(
		ctx,
		cfg,
		"https://www.reddit.com/r/selfhosted/.rss",
		0,
	)

	if err == nil {
		t.Fatal("unavailable browser endpoint unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "CDP") &&
		!strings.Contains(err.Error(), "Playwright") {
		t.Fatalf("browser connection error lacks context: %v", err)
	}
}

func TestRSSBrowserDocumentStateEarlyCompletion(t *testing.T) {
	address := "https://www.reddit.com/r/selfhosted/.rss"
	state := newRSSBrowserDocumentState()

	state.observeFinished("document")
	state.observeFinished("unrelated")
	state.observeResponse(address, network.EventResponseReceived{
		RequestID: "document",
		Type:      network.ResourceTypeDocument,
		Response:  &network.Response{URL: address, Status: 200},
	})

	response, done, failure := state.snapshot()
	if response == nil || response.RequestID != "document" || !done || failure != "" {
		t.Fatalf("unexpected document snapshot: response=%v done=%t failure=%q", response, done, failure)
	}
	if len(state.completed) != 1 || state.completed["unrelated"] {
		t.Fatalf("unrelated completion retained: %v", state.completed)
	}
}

func TestRSSBrowserDocumentStateEarlyFailure(t *testing.T) {
	address := "https://www.reddit.com/r/selfhosted/.rss"
	state := newRSSBrowserDocumentState()

	state.observeFailed("document", "net::ERR_FAILED")
	state.observeFailed("unrelated", "unrelated failure")
	state.observeResponse(address, network.EventResponseReceived{
		RequestID: "document",
		Type:      network.ResourceTypeDocument,
		Response:  &network.Response{URL: address, Status: 200},
	})

	_, done, failure := state.snapshot()
	if done || failure != "net::ERR_FAILED" {
		t.Fatalf("unexpected failure state: done=%t failure=%q", done, failure)
	}
	if len(state.failures) != 1 {
		t.Fatalf("unrelated failures retained: %v", state.failures)
	}
}

func TestRSSBrowserDocumentStateDuplicateIDs(t *testing.T) {
	state := newRSSBrowserDocumentState()

	state.observeFinished("duplicate")
	state.observeFinished("duplicate")
	state.observeFailed("duplicate", "failed")

	if len(state.order) != 1 {
		t.Fatalf("duplicate request IDs retained: %v", state.order)
	}
	if !state.completed["duplicate"] || state.failures["duplicate"] != "failed" {
		t.Fatal("duplicate request lost its completion or failure state")
	}
}

func TestRSSBrowserDocumentStateBoundedEviction(t *testing.T) {
	state := newRSSBrowserDocumentState()

	for i := 0; i < rssBrowserEarlyEventLimit+10; i++ {
		state.observeFinished(network.RequestID(fmt.Sprintf("request-%d", i)))
	}

	if len(state.order) != rssBrowserEarlyEventLimit ||
		len(state.completed) != rssBrowserEarlyEventLimit {
		t.Fatalf("early state exceeded limit: order=%d completed=%d",
			len(state.order), len(state.completed))
	}
	if state.completed["request-0"] {
		t.Fatal("oldest request was not evicted")
	}
	if !state.completed[network.RequestID(fmt.Sprintf("request-%d", rssBrowserEarlyEventLimit+9))] {
		t.Fatal("newest request was unexpectedly evicted")
	}
}

func TestRSSBrowserDocumentStateIgnoresUnrelatedResponses(t *testing.T) {
	address := "https://www.reddit.com/r/selfhosted/.rss"
	state := newRSSBrowserDocumentState()

	state.observeResponse(address, network.EventResponseReceived{
		RequestID: "subresource",
		Type:      network.ResourceTypeXHR,
		Response:  &network.Response{URL: address, Status: 403},
	})
	state.observeResponse(address, network.EventResponseReceived{
		RequestID: "other-document",
		Type:      network.ResourceTypeDocument,
		Response:  &network.Response{URL: "https://www.reddit.com/login/", Status: 403},
	})

	if response, _, _ := state.snapshot(); response != nil {
		t.Fatalf("unrelated response identified as document: %v", response)
	}

	state.observeResponse(address, network.EventResponseReceived{
		RequestID: "document",
		Type:      network.ResourceTypeDocument,
		Response:  &network.Response{URL: address, Status: 200},
	})
	state.observeFinished("unrelated")
	state.observeFailed("unrelated", "failed")
	state.observeFinished("document")

	response, done, failure := state.snapshot()
	if response == nil || !done || failure != "" {
		t.Fatalf("unexpected document state: response=%v done=%t failure=%q", response, done, failure)
	}
	if len(state.completed) != 1 || len(state.failures) != 0 {
		t.Fatalf("unrelated post-identification events retained: completed=%v failures=%v",
			state.completed, state.failures)
	}
}
