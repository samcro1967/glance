package glance

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var frontendDiagnosticsLiveUpdateConnectionID atomic.Uint64
var frontendDiagnosticCommandID atomic.Uint64

const frontendDiagnosticCommandQueueLimit = 8

const liveUpdateHeartbeatInterval = 30 * time.Second
const liveUpdateWriteTimeout = 10 * time.Second

type frontendDiagnosticCommand struct {
	ID      uint64 `json:"id"`
	Command string `json:"command"`
}

type liveUpdateBrokerDiagnosticsSnapshot struct {
	ActiveSubscribers           int
	Subscriptions               uint64
	Unsubscriptions             uint64
	WidgetPublishes             uint64
	WidgetSubscriberMatches     uint64
	WidgetCoalesced             uint64
	DiagnosticCommandsPublished uint64
	DiagnosticCommandEnqueues   uint64
	DiagnosticCommandDrops      uint64
	Closed                      bool
}

type liveUpdateSubscription struct {
	mu                 sync.Mutex
	pending            map[uint64]struct{}
	widgetIDs          map[uint64]struct{}
	diagnosticCommands []frontendDiagnosticCommand
	ready              chan struct{}
	closed             bool
}

func newLiveUpdateSubscription(widgetIDs map[uint64]struct{}) *liveUpdateSubscription {
	return &liveUpdateSubscription{
		pending:   make(map[uint64]struct{}),
		widgetIDs: widgetIDs,
		ready:     make(chan struct{}, 1),
	}
}

func (s *liveUpdateSubscription) publish(widgetID uint64) (accepted bool, coalesced bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false, false
	}

	if s.widgetIDs != nil {
		if _, interested := s.widgetIDs[widgetID]; !interested {
			return false, false
		}
	}

	_, coalesced = s.pending[widgetID]
	s.pending[widgetID] = struct{}{}

	select {
	case s.ready <- struct{}{}:
	default:
	}

	return true, coalesced
}

func (s *liveUpdateSubscription) publishDiagnosticCommand(
	command frontendDiagnosticCommand,
) (accepted bool, dropped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false, false
	}

	if len(s.diagnosticCommands) >= frontendDiagnosticCommandQueueLimit {
		dropped = true
		copy(s.diagnosticCommands, s.diagnosticCommands[1:])
		s.diagnosticCommands = s.diagnosticCommands[:frontendDiagnosticCommandQueueLimit-1]
	}

	s.diagnosticCommands = append(s.diagnosticCommands, command)

	select {
	case s.ready <- struct{}{}:
	default:
	}

	return true, dropped
}

func (s *liveUpdateSubscription) takeDiagnosticCommands() []frontendDiagnosticCommand {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.diagnosticCommands) == 0 {
		return nil
	}

	commands := append(
		[]frontendDiagnosticCommand(nil),
		s.diagnosticCommands...,
	)
	s.diagnosticCommands = s.diagnosticCommands[:0]

	return commands
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

	subscriptions               uint64
	unsubscriptions             uint64
	widgetPublishes             uint64
	widgetSubscriberMatches     uint64
	widgetCoalesced             uint64
	diagnosticCommandsPublished uint64
	diagnosticCommandEnqueues   uint64
	diagnosticCommandDrops      uint64
}

func newLiveUpdateBroker() *liveUpdateBroker {
	return &liveUpdateBroker{
		subscribers: make(map[*liveUpdateSubscription]struct{}),
	}
}

func (b *liveUpdateBroker) subscribe(widgetIDs map[uint64]struct{}) (*liveUpdateSubscription, func()) {
	subscription := newLiveUpdateSubscription(widgetIDs)

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		subscription.close()
		return subscription, func() {}
	}

	b.subscribers[subscription] = struct{}{}
	b.subscriptions++
	b.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			b.mu.Lock()
			if _, exists := b.subscribers[subscription]; exists {
				delete(b.subscribers, subscription)
				b.unsubscriptions++
			}
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

	b.widgetPublishes++
	for subscriber := range b.subscribers {
		accepted, coalesced := subscriber.publish(widgetID)
		if accepted {
			b.widgetSubscriberMatches++
		}
		if coalesced {
			b.widgetCoalesced++
		}
	}
}

func (b *liveUpdateBroker) publishDiagnosticCommand(
	command frontendDiagnosticCommand,
) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.diagnosticCommandsPublished++
	for subscriber := range b.subscribers {
		accepted, dropped := subscriber.publishDiagnosticCommand(command)
		if accepted {
			b.diagnosticCommandEnqueues++
		}
		if dropped {
			b.diagnosticCommandDrops++
		}
	}
}

func (b *liveUpdateBroker) snapshot() liveUpdateBrokerDiagnosticsSnapshot {
	if b == nil {
		return liveUpdateBrokerDiagnosticsSnapshot{}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	return liveUpdateBrokerDiagnosticsSnapshot{
		ActiveSubscribers:           len(b.subscribers),
		Subscriptions:               b.subscriptions,
		Unsubscriptions:             b.unsubscriptions,
		WidgetPublishes:             b.widgetPublishes,
		WidgetSubscriberMatches:     b.widgetSubscriberMatches,
		WidgetCoalesced:             b.widgetCoalesced,
		DiagnosticCommandsPublished: b.diagnosticCommandsPublished,
		DiagnosticCommandEnqueues:   b.diagnosticCommandEnqueues,
		DiagnosticCommandDrops:      b.diagnosticCommandDrops,
		Closed:                      b.closed,
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
		b.unsubscriptions++
	}
}

func (a *application) handleLiveUpdatesRequest(w http.ResponseWriter, r *http.Request) {
	session, authenticated := a.authorizeSession(w, r)
	if !authenticated {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "Unauthorized"}`))
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

	if _, ok := w.(http.Flusher); !ok {
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

	responseController := http.NewResponseController(w)
	logWriteDeadlineFailure := func(operation string, err error) {
		if a.Config.Server.FrontendDiagnostics {
			slog.Info(
				"Frontend diagnostic",
				"source", "server",
				"event", "live_updates_write_deadline_failed",
				"connection", connectionID,
				"operation", operation,
				"error", err,
			)
		}
	}
	logFlushFailure := func(operation string, err error) {
		if a.Config.Server.FrontendDiagnostics {
			slog.Info(
				"Frontend diagnostic",
				"source", "server",
				"event", "live_updates_flush_failed",
				"connection", connectionID,
				"operation", operation,
				"error", err,
			)
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	var widgetIDs map[uint64]struct{}
	requestedWidgetIDs, filtered := r.URL.Query()["widget"]
	if filtered || a.authorization.Enabled() {
		capacity := len(requestedWidgetIDs)
		if !filtered {
			capacity = len(a.widgetByID)
		}
		widgetIDs = make(map[uint64]struct{}, capacity)

		if filtered {
			for _, value := range requestedWidgetIDs {
				widgetID, err := strconv.ParseUint(value, 10, 64)
				if err != nil {
					continue
				}
				if _, exists := a.widgetByID[widgetID]; !exists || !a.canAccessWidget(session.AuthorizationIdentity, widgetID) {
					continue
				}
				widgetIDs[widgetID] = struct{}{}
			}
		} else {
			for widgetID := range a.widgetByID {
				if a.canAccessWidget(session.AuthorizationIdentity, widgetID) {
					widgetIDs[widgetID] = struct{}{}
				}
			}
		}
	}

	subscription, unsubscribe := a.liveUpdates.subscribe(widgetIDs)
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
	if err := responseController.SetWriteDeadline(time.Now().Add(liveUpdateWriteTimeout)); err != nil {
		logWriteDeadlineFailure("initial", err)
		return
	}
	if err := responseController.Flush(); err != nil {
		logFlushFailure("initial", err)
		return
	}
	if err := responseController.SetWriteDeadline(time.Time{}); err != nil {
		logWriteDeadlineFailure("initial", err)
		return
	}

	heartbeat := time.NewTicker(liveUpdateHeartbeatInterval)
	defer heartbeat.Stop()

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

		case <-heartbeat.C:
			if err := responseController.SetWriteDeadline(time.Now().Add(liveUpdateWriteTimeout)); err != nil {
				logWriteDeadlineFailure("heartbeat", err)
				return
			}
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				if a.Config.Server.FrontendDiagnostics {
					slog.Info(
						"Frontend diagnostic",
						"source", "server",
						"event", "live_updates_heartbeat_write_failed",
						"connection", connectionID,
						"error", err,
					)
				}
				return
			}
			if err := responseController.Flush(); err != nil {
				logFlushFailure("heartbeat", err)
				return
			}
			if err := responseController.SetWriteDeadline(time.Time{}); err != nil {
				logWriteDeadlineFailure("heartbeat", err)
				return
			}

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

			if err := responseController.SetWriteDeadline(time.Now().Add(liveUpdateWriteTimeout)); err != nil {
				logWriteDeadlineFailure("updates", err)
				return
			}

			for _, command := range subscription.takeDiagnosticCommands() {
				payload, err := json.Marshal(command)
				if err != nil {
					slog.Warn(
						"Failed to encode frontend diagnostic command",
						"command_id", command.ID,
						"command", command.Command,
						"error", err,
					)
					continue
				}

				if _, err := fmt.Fprintf(
					w,
					"event: diagnostic\ndata: %s\n\n",
					payload,
				); err != nil {
					if a.Config.Server.FrontendDiagnostics {
						slog.Info(
							"Frontend diagnostic",
							"source", "server",
							"event", "diagnostic_command_write_failed",
							"connection", connectionID,
							"command_id", command.ID,
							"command", command.Command,
							"error", err,
						)
					}
					return
				}

				if a.Config.Server.FrontendDiagnostics {
					slog.Info(
						"Frontend diagnostic",
						"source", "server",
						"event", "diagnostic_command_write",
						"connection", connectionID,
						"command_id", command.ID,
						"command", command.Command,
					)
				}
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

			if err := responseController.Flush(); err != nil {
				logFlushFailure("updates", err)
				return
			}
			if err := responseController.SetWriteDeadline(time.Time{}); err != nil {
				logWriteDeadlineFailure("updates", err)
				return
			}

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
