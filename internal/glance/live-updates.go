package glance

import (
	"fmt"
	"net/http"
	"sync"
)

type liveUpdateSubscription struct {
	mu      sync.Mutex
	pending map[uint64]struct{}
	ready   chan struct{}
	closed  bool
}

func newLiveUpdateSubscription() *liveUpdateSubscription {
	return &liveUpdateSubscription{
		pending: make(map[uint64]struct{}),
		ready:   make(chan struct{}, 1),
	}
}

func (s *liveUpdateSubscription) publish(widgetID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.pending[widgetID] = struct{}{}

	select {
	case s.ready <- struct{}{}:
	default:
	}
}

func (s *liveUpdateSubscription) takePending() []uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pending) == 0 {
		return nil
	}

	widgetIDs := make([]uint64, 0, len(s.pending))
	for widgetID := range s.pending {
		widgetIDs = append(widgetIDs, widgetID)
		delete(s.pending, widgetID)
	}

	return widgetIDs
}

func (s *liveUpdateSubscription) close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.closed = true
	close(s.ready)
}

type liveUpdateBroker struct {
	mu          sync.Mutex
	subscribers map[*liveUpdateSubscription]struct{}
	closed      bool
}

func newLiveUpdateBroker() *liveUpdateBroker {
	return &liveUpdateBroker{
		subscribers: make(map[*liveUpdateSubscription]struct{}),
	}
}

func (b *liveUpdateBroker) subscribe() (*liveUpdateSubscription, func()) {
	subscription := newLiveUpdateSubscription()

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		subscription.close()
		return subscription, func() {}
	}

	b.subscribers[subscription] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subscribers, subscription)
			b.mu.Unlock()

			subscription.close()
		})
	}

	return subscription, unsubscribe
}

func (b *liveUpdateBroker) publish(widgetID uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	for subscriber := range b.subscribers {
		subscriber.publish(widgetID)
	}
}

func (b *liveUpdateBroker) close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.closed = true

	for subscriber := range b.subscribers {
		subscriber.close()
		delete(b.subscribers, subscriber)
	}
}

func (a *application) handleLiveUpdatesRequest(w http.ResponseWriter, r *http.Request) {
	if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	subscription, unsubscribe := a.liveUpdates.subscribe()
	defer unsubscribe()

	// Commit and flush the response immediately so EventSource knows that
	// the connection has been established even when no widget is currently
	// due for refresh.
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return

		case _, ok := <-subscription.ready:
			if !ok {
				return
			}

			for _, widgetID := range subscription.takePending() {
				if _, err := fmt.Fprintf(
					w,
					"event: widget\ndata: %d\n\n",
					widgetID,
				); err != nil {
					return
				}
			}

			flusher.Flush()
		}
	}
}
