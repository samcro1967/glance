package glance

import (
	"slices"
	"testing"
	"time"
)

func waitForLiveUpdate(t *testing.T, subscription *liveUpdateSubscription) {
	t.Helper()

	select {
	case _, ok := <-subscription.ready:
		if !ok {
			t.Fatal("subscription closed while waiting for update")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live update")
	}
}

func TestLiveUpdateBrokerPublishesToSubscribers(t *testing.T) {
	broker := newLiveUpdateBroker()

	subscription, unsubscribe := broker.subscribe(nil)
	defer unsubscribe()

	broker.publish(42)
	waitForLiveUpdate(t, subscription)

	widgetIDs := subscription.takePending()
	if !slices.Equal(widgetIDs, []uint64{42}) {
		t.Fatalf("pending widget IDs = %v, want [42]", widgetIDs)
	}
}

func TestLiveUpdateBrokerPublishesToMultipleSubscribers(t *testing.T) {
	broker := newLiveUpdateBroker()

	first, unsubscribeFirst := broker.subscribe(nil)
	defer unsubscribeFirst()

	second, unsubscribeSecond := broker.subscribe(nil)
	defer unsubscribeSecond()

	broker.publish(42)

	for name, subscription := range map[string]*liveUpdateSubscription{
		"first":  first,
		"second": second,
	} {
		waitForLiveUpdate(t, subscription)

		widgetIDs := subscription.takePending()
		if !slices.Equal(widgetIDs, []uint64{42}) {
			t.Fatalf(
				"%s subscriber pending widget IDs = %v, want [42]",
				name,
				widgetIDs,
			)
		}
	}
}

func TestLiveUpdateBrokerCoalescesRepeatedWidgetUpdates(t *testing.T) {
	broker := newLiveUpdateBroker()

	subscription, unsubscribe := broker.subscribe(nil)
	defer unsubscribe()

	broker.publish(42)
	broker.publish(42)
	broker.publish(42)

	waitForLiveUpdate(t, subscription)

	widgetIDs := subscription.takePending()
	if !slices.Equal(widgetIDs, []uint64{42}) {
		t.Fatalf("pending widget IDs = %v, want [42]", widgetIDs)
	}
}

func TestLiveUpdateBrokerRetainsDistinctUpdatesForSlowSubscriber(t *testing.T) {
	broker := newLiveUpdateBroker()

	subscription, unsubscribe := broker.subscribe(nil)
	defer unsubscribe()

	for widgetID := uint64(1); widgetID <= 100; widgetID++ {
		broker.publish(widgetID)
	}

	waitForLiveUpdate(t, subscription)

	widgetIDs := subscription.takePending()
	slices.Sort(widgetIDs)

	if len(widgetIDs) != 100 {
		t.Fatalf("pending widget count = %d, want 100", len(widgetIDs))
	}

	for i, widgetID := range widgetIDs {
		want := uint64(i + 1)
		if widgetID != want {
			t.Fatalf("pending widget ID at index %d = %d, want %d", i, widgetID, want)
		}
	}
}

func TestLiveUpdateSubscriptionTakePendingDrainsUpdates(t *testing.T) {
	broker := newLiveUpdateBroker()

	subscription, unsubscribe := broker.subscribe(nil)
	defer unsubscribe()

	broker.publish(42)
	waitForLiveUpdate(t, subscription)

	if got := subscription.takePending(); !slices.Equal(got, []uint64{42}) {
		t.Fatalf("first takePending() = %v, want [42]", got)
	}

	if got := subscription.takePending(); len(got) != 0 {
		t.Fatalf("second takePending() = %v, want empty", got)
	}
}

func TestLiveUpdateBrokerUnsubscribeClosesSubscription(t *testing.T) {
	broker := newLiveUpdateBroker()

	subscription, unsubscribe := broker.subscribe(nil)
	unsubscribe()
	unsubscribe()

	select {
	case _, ok := <-subscription.ready:
		if ok {
			t.Fatal("subscription ready channel remained open after unsubscribe")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription ready channel was not closed after unsubscribe")
	}
}

func TestLiveUpdateBrokerCloseClosesSubscribers(t *testing.T) {
	broker := newLiveUpdateBroker()

	first, unsubscribeFirst := broker.subscribe(nil)
	defer unsubscribeFirst()

	second, unsubscribeSecond := broker.subscribe(nil)
	defer unsubscribeSecond()

	broker.close()
	broker.close()

	for name, subscription := range map[string]*liveUpdateSubscription{
		"first":  first,
		"second": second,
	} {
		select {
		case _, ok := <-subscription.ready:
			if ok {
				t.Fatalf("%s subscription remained open after broker close", name)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s subscription was not closed after broker close", name)
		}
	}
}

func TestLiveUpdateBrokerSubscribeAfterCloseReturnsClosedSubscription(t *testing.T) {
	broker := newLiveUpdateBroker()
	broker.close()

	subscription, unsubscribe := broker.subscribe(nil)
	defer unsubscribe()

	select {
	case _, ok := <-subscription.ready:
		if ok {
			t.Fatal("subscription created after close was open")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription created after close was not closed")
	}
}

func TestLiveUpdateSubscriptionDiagnosticCommands(t *testing.T) {
	subscription := newLiveUpdateSubscription(nil)

	first := frontendDiagnosticCommand{
		ID:      1,
		Command: "performance_snapshot",
	}
	second := frontendDiagnosticCommand{
		ID:      2,
		Command: "performance_snapshot",
	}

	subscription.publishDiagnosticCommand(first)
	subscription.publishDiagnosticCommand(second)

	commands := subscription.takeDiagnosticCommands()
	if len(commands) != 2 {
		t.Fatalf("expected 2 diagnostic commands, got %d", len(commands))
	}

	if commands[0] != first {
		t.Fatalf("unexpected first diagnostic command: %#v", commands[0])
	}

	if commands[1] != second {
		t.Fatalf("unexpected second diagnostic command: %#v", commands[1])
	}

	if commands := subscription.takeDiagnosticCommands(); len(commands) != 0 {
		t.Fatalf(
			"expected diagnostic command queue to drain, got %d commands",
			len(commands),
		)
	}
}

func TestLiveUpdateSubscriptionDiagnosticCommandQueueIsBounded(t *testing.T) {
	subscription := newLiveUpdateSubscription(nil)

	for i := uint64(1); i <= frontendDiagnosticCommandQueueLimit+3; i++ {
		subscription.publishDiagnosticCommand(frontendDiagnosticCommand{
			ID:      i,
			Command: "performance_snapshot",
		})
	}

	commands := subscription.takeDiagnosticCommands()
	if len(commands) != frontendDiagnosticCommandQueueLimit {
		t.Fatalf(
			"got %d diagnostic commands, want %d",
			len(commands),
			frontendDiagnosticCommandQueueLimit,
		)
	}

	wantFirstID := uint64(4)
	if commands[0].ID != wantFirstID {
		t.Fatalf(
			"first retained command ID = %d, want %d",
			commands[0].ID,
			wantFirstID,
		)
	}

	wantLastID := uint64(frontendDiagnosticCommandQueueLimit + 3)
	if commands[len(commands)-1].ID != wantLastID {
		t.Fatalf(
			"last retained command ID = %d, want %d",
			commands[len(commands)-1].ID,
			wantLastID,
		)
	}
}

func TestLiveUpdateBrokerPublishesDiagnosticCommandsToAllSubscribers(t *testing.T) {
	broker := newLiveUpdateBroker()
	first, unsubscribeFirst := broker.subscribe(nil)
	defer unsubscribeFirst()
	second, unsubscribeSecond := broker.subscribe(nil)
	defer unsubscribeSecond()

	command := frontendDiagnosticCommand{
		ID:      17,
		Command: "performance_snapshot",
	}

	broker.publishDiagnosticCommand(command)

	for name, subscription := range map[string]*liveUpdateSubscription{
		"first":  first,
		"second": second,
	} {
		commands := subscription.takeDiagnosticCommands()
		if len(commands) != 1 {
			t.Fatalf(
				"%s subscriber expected 1 diagnostic command, got %d",
				name,
				len(commands),
			)
		}

		if commands[0] != command {
			t.Fatalf(
				"%s subscriber got unexpected diagnostic command: %#v",
				name,
				commands[0],
			)
		}
	}
}

func TestLiveUpdateSubscriptionFiltersWidgetIDs(t *testing.T) {
	broker := newLiveUpdateBroker()

	subscription, unsubscribe := broker.subscribe(map[uint64]struct{}{
		42: {},
		84: {},
	})
	defer unsubscribe()

	broker.publish(21)
	broker.publish(42)
	broker.publish(63)
	broker.publish(84)

	pending := subscription.takePending()
	if len(pending) != 2 {
		t.Fatalf("got %d pending updates, want 2", len(pending))
	}

	got := make(map[uint64]struct{}, len(pending))
	for _, widgetID := range pending {
		got[widgetID] = struct{}{}
	}

	for _, widgetID := range []uint64{42, 84} {
		if _, ok := got[widgetID]; !ok {
			t.Fatalf("expected widget %d in pending updates", widgetID)
		}
	}

	for _, widgetID := range []uint64{21, 63} {
		if _, ok := got[widgetID]; ok {
			t.Fatalf("unexpected filtered widget %d in pending updates", widgetID)
		}
	}
}

func TestLiveUpdateSubscriptionFilterDoesNotBlockDiagnosticCommands(t *testing.T) {
	broker := newLiveUpdateBroker()

	subscription, unsubscribe := broker.subscribe(map[uint64]struct{}{
		42: {},
	})
	defer unsubscribe()

	broker.publish(84)
	broker.publishDiagnosticCommand(frontendDiagnosticCommand{
		ID:      7,
		Command: "performance_snapshot",
	})

	if pending := subscription.takePending(); len(pending) != 0 {
		t.Fatalf("got %d filtered widget updates, want 0", len(pending))
	}

	commands := subscription.takeDiagnosticCommands()
	if len(commands) != 1 {
		t.Fatalf("got %d diagnostic commands, want 1", len(commands))
	}
	if commands[0].ID != 7 || commands[0].Command != "performance_snapshot" {
		t.Fatalf("unexpected diagnostic command: %#v", commands[0])
	}
}
