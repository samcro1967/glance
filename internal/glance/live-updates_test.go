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

	subscription, unsubscribe := broker.subscribe()
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

	first, unsubscribeFirst := broker.subscribe()
	defer unsubscribeFirst()

	second, unsubscribeSecond := broker.subscribe()
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

	subscription, unsubscribe := broker.subscribe()
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

	subscription, unsubscribe := broker.subscribe()
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

	subscription, unsubscribe := broker.subscribe()
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

	subscription, unsubscribe := broker.subscribe()
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

	first, unsubscribeFirst := broker.subscribe()
	defer unsubscribeFirst()

	second, unsubscribeSecond := broker.subscribe()
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

	subscription, unsubscribe := broker.subscribe()
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
