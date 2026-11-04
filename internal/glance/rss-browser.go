package glance

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const rssBrowserEarlyEventLimit = 32

type rssBrowserDocumentState struct {
	document  *network.EventResponseReceived
	completed map[network.RequestID]bool
	failures  map[network.RequestID]string
	order     []network.RequestID
}

func newRSSBrowserDocumentState() *rssBrowserDocumentState {
	return &rssBrowserDocumentState{
		completed: make(map[network.RequestID]bool),
		failures:  make(map[network.RequestID]string),
	}
}

func (s *rssBrowserDocumentState) remember(id network.RequestID) {
	if s.document != nil {
		return
	}
	for _, existing := range s.order {
		if existing == id {
			return
		}
	}
	if len(s.order) >= rssBrowserEarlyEventLimit {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.completed, oldest)
		delete(s.failures, oldest)
	}
	s.order = append(s.order, id)
}

func (s *rssBrowserDocumentState) observeResponse(address string, event network.EventResponseReceived) {
	if _, matches := redditBrowserDocumentStatus(address, &event); !matches {
		return
	}
	copy := event
	s.document = &copy
	for id := range s.completed {
		if id != copy.RequestID {
			delete(s.completed, id)
		}
	}
	for id := range s.failures {
		if id != copy.RequestID {
			delete(s.failures, id)
		}
	}
	s.order = nil
}

func (s *rssBrowserDocumentState) observeFinished(id network.RequestID) {
	if s.document != nil && s.document.RequestID != id {
		return
	}
	s.remember(id)
	s.completed[id] = true
}

func (s *rssBrowserDocumentState) observeFailed(id network.RequestID, message string) {
	if s.document != nil && s.document.RequestID != id {
		return
	}
	s.remember(id)
	s.failures[id] = message
}

func (s *rssBrowserDocumentState) snapshot() (*network.EventResponseReceived, bool, string) {
	if s.document == nil {
		return nil, false, ""
	}
	copy := *s.document
	return &copy, s.completed[copy.RequestID], s.failures[copy.RequestID]
}

// fetchRSSBrowserResource retrieves the original Atom response through an
// authenticated, temporary Chromium tab. It never reads rendered page text.
func fetchRSSBrowserResource(ctx context.Context, cfg redditBrowserConfig, address string, timeout durationField) (rssResourceResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	browser, cleanup, err := redditBrowserConnect(ctx, cfg.Endpoint, timeout)
	if err != nil {
		return rssResourceResponse{}, err
	}
	defer cleanup()

	if err := redditBrowserEnsureLogin(ctx, browser, cfg); err != nil {
		return rssResourceResponse{}, fmt.Errorf("authenticating Reddit RSS browser: %w", err)
	}

	page, closePage := chromedp.NewContext(browser)
	defer closePage()

	var mu sync.Mutex
	state := newRSSBrowserDocumentState()
	changed := make(chan struct{}, 1)
	eventErrors := make(chan error, 3)

	notify := func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}

	responseEvents := chromedp.Events(page, network.ResponseReceived)
	finishedEvents := chromedp.Events(page, network.LoadingFinished)
	failedEvents := chromedp.Events(page, network.LoadingFailed)

	go func() {
		for event, err := range responseEvents {
			if err != nil {
				select {
				case eventErrors <- err:
				case <-page.Done():
				}
				return
			}
			if _, matches := redditBrowserDocumentStatus(address, &event); matches {
				mu.Lock()
				state.observeResponse(address, event)
				mu.Unlock()
				notify()
			}
		}
	}()

	go func() {
		for event, err := range finishedEvents {
			if err != nil {
				select {
				case eventErrors <- err:
				case <-page.Done():
				}
				return
			}
			mu.Lock()
			state.observeFinished(event.RequestID)
			mu.Unlock()
			notify()
		}
	}()

	go func() {
		for event, err := range failedEvents {
			if err != nil {
				select {
				case eventErrors <- err:
				case <-page.Done():
				}
				return
			}
			mu.Lock()
			state.observeFailed(event.RequestID, event.ErrorText)
			mu.Unlock()
			notify()
		}
	}()

	if err := chromedp.Do(page, chromedp.Navigate(address)); err != nil {
		return rssResourceResponse{}, fmt.Errorf("navigating Reddit RSS browser tab: %w", err)
	}

	for {
		mu.Lock()
		observed, done, failure := state.snapshot()
		mu.Unlock()

		if observed != nil {
			if failure != "" {
				return rssResourceResponse{}, fmt.Errorf("loading Reddit RSS document: %s", failure)
			}
			if done {
				document := observed
				status := int(document.Response.Status)
				resource := rssResourceResponse{
					StatusCode: status,
					Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
					Header:     make(http.Header),
				}

				if status != http.StatusOK {
					return resource, nil
				}

				result, err := chromedp.Call(page, network.GetResponseBody, network.GetResponseBodyParams{
					RequestID: document.RequestID,
				})
				if err != nil {
					return rssResourceResponse{}, fmt.Errorf("reading Reddit RSS CDP response body: %w", err)
				}

				body, err := readDefaultHTTPResponseBody(bytes.NewReader(result.Body))
				if err != nil {
					return rssResourceResponse{}, fmt.Errorf("reading Reddit RSS response: %w", err)
				}

				resource.Body = body
				return resource, nil
			}
		}

		select {
		case <-ctx.Done():
			return rssResourceResponse{}, fmt.Errorf("fetching Reddit RSS through Playwright: %w", ctx.Err())
		case err := <-eventErrors:
			return rssResourceResponse{}, fmt.Errorf("observing Reddit RSS network response: %w", err)
		case <-changed:
			// Recheck synchronized document state.
		}
	}
}
