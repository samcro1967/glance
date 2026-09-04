package glance

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
)

var frontendDiagnosticsLiveUpdateConnectionID atomic.Uint64

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

	connectionID := uint64(0)
	if a.Config.Server.FrontendDiagnostics {
		connectionID = frontendDiagnosticsLiveUpdateConnectionID.Add(1)
		slog.Info(
			"Frontend diagnostic",
			"source", "server",
			"event", "live_updates_request_accepted",
			"connection", connectionID,
		)
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		if a.Config.Server.FrontendDiagnostics {
			slog.Info(
				"Frontend diagnostic",
				"source", "server",
				"event", "live_updates_streaming_unsupported",
				"connection", connectionID,
			)
		}
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	subscription, unsubscribe := a.liveUpdates.subscribe()
	defer unsubscribe()

	if a.Config.Server.FrontendDiagnostics {
		slog.Info(
			"Frontend diagnostic",
			"source", "server",
			"event", "live_updates_stream_ready",
			"connection", connectionID,
		)
	}

	// Commit and flush the response immediately so EventSource knows that
	// the connection has been established even when no widget is currently
	// due for refresh.
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if a.Config.Server.FrontendDiagnostics {
		slog.Info(
			"Frontend diagnostic",
			"source", "server",
			"event", "live_updates_initial_flush",
			"connection", connectionID,
		)
	}

	for {
		select {
		case <-r.Context().Done():
			if a.Config.Server.FrontendDiagnostics {
				slog.Info(
					"Frontend diagnostic",
					"source", "server",
					"event", "live_updates_disconnected",
					"connection", connectionID,
					"reason", "request_context",
				)
			}
			return

		case _, ok := <-subscription.ready:
			if !ok {
				if a.Config.Server.FrontendDiagnostics {
					slog.Info(
						"Frontend diagnostic",
						"source", "server",
						"event", "live_updates_disconnected",
						"connection", connectionID,
						"reason", "subscription_closed",
					)
				}
				return
			}

			for _, widgetID := range subscription.takePending() {
				if a.Config.Server.FrontendDiagnostics {
					slog.Info(
						"Frontend diagnostic",
						"source", "server",
						"event", "live_updates_widget_write",
						"connection", connectionID,
						"widget", widgetID,
					)
				}

				if _, err := fmt.Fprintf(
					w,
					"event: widget\ndata: %d\n\n",
					widgetID,
				); err != nil {
					if a.Config.Server.FrontendDiagnostics {
						slog.Info(
							"Frontend diagnostic",
							"source", "server",
							"event", "live_updates_write_failed",
							"connection", connectionID,
							"widget", widgetID,
							"error", err,
						)
					}
					return
				}
			}

			flusher.Flush()

			if a.Config.Server.FrontendDiagnostics {
				slog.Info(
					"Frontend diagnostic",
					"source", "server",
					"event", "live_updates_widget_flush",
					"connection", connectionID,
				)
			}
		}
	}
}
