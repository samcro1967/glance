package glance

import (
	"fmt"
	"testing"
)

func benchmarkLiveUpdateBrokerPublish(b *testing.B, subscriberCount int) {
	broker := newLiveUpdateBroker()

	unsubscribes := make([]func(), 0, subscriberCount)
	for i := 0; i < subscriberCount; i++ {
		_, unsubscribe := broker.subscribe(nil)
		unsubscribes = append(unsubscribes, unsubscribe)
	}

	b.Cleanup(func() {
		for _, unsubscribe := range unsubscribes {
			unsubscribe()
		}
		broker.close()
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		widgetID := uint64(i%100 + 1)
		broker.publish(widgetID)

		if widgetID == 100 {
			for subscription := range broker.subscribers {
				subscription.takePending()
			}
		}
	}
}

func BenchmarkLiveUpdateBrokerPublish(b *testing.B) {
	for _, subscriberCount := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("subscribers_%d", subscriberCount), func(b *testing.B) {
			benchmarkLiveUpdateBrokerPublish(b, subscriberCount)
		})
	}
}

func BenchmarkLiveUpdateBrokerPublishCoalesced(b *testing.B) {
	for _, subscriberCount := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("subscribers_%d", subscriberCount), func(b *testing.B) {
			broker := newLiveUpdateBroker()

			unsubscribes := make([]func(), 0, subscriberCount)
			for i := 0; i < subscriberCount; i++ {
				_, unsubscribe := broker.subscribe(nil)
				unsubscribes = append(unsubscribes, unsubscribe)
			}

			b.Cleanup(func() {
				for _, unsubscribe := range unsubscribes {
					unsubscribe()
				}
				broker.close()
			})

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				broker.publish(1)
			}
		})
	}
}
