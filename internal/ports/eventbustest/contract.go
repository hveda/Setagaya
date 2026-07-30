// Package eventbustest holds the shared behavioural contract every
// ports.EventBus implementation must satisfy.
package eventbustest

import (
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/domain/engine"
	"github.com/heridotlife/honryu/internal/ports"
)

// NewBus builds a fresh, empty EventBus for one test.
type NewBus func(t *testing.T) ports.EventBus

// RunEventBusContract pins delivery, scoping, and unsubscribe behaviour.
func RunEventBusContract(t *testing.T, newBus NewBus) {
	t.Helper()

	t.Run("subscriber receives published events for its execution", func(t *testing.T) {
		bus := newBus(t)
		ch, cancel := bus.Subscribe(1)
		defer cancel()

		bus.Publish(1, engine.Metric{Label: "a"})
		select {
		case m := <-ch:
			if m.Label != "a" {
				t.Fatalf("got %+v", m)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event")
		}
	})

	t.Run("events are scoped per execution", func(t *testing.T) {
		bus := newBus(t)
		ch, cancel := bus.Subscribe(1)
		defer cancel()

		bus.Publish(2, engine.Metric{Label: "other"})
		select {
		case m := <-ch:
			t.Fatalf("received cross-execution event %+v", m)
		case <-time.After(50 * time.Millisecond):
		}
	})

	t.Run("cancel unsubscribes and closes the channel", func(t *testing.T) {
		bus := newBus(t)
		ch, cancel := bus.Subscribe(1)
		cancel()

		// The channel is closed after cancel.
		select {
		case _, ok := <-ch:
			if ok {
				t.Fatal("expected closed channel after cancel")
			}
		case <-time.After(time.Second):
			t.Fatal("channel not closed after cancel")
		}
		// Publishing after cancel must not panic and reaches nobody.
		bus.Publish(1, engine.Metric{Label: "late"})
		// Cancel is idempotent.
		cancel()
	})

	t.Run("multiple subscribers each receive the event", func(t *testing.T) {
		bus := newBus(t)
		ch1, c1 := bus.Subscribe(1)
		ch2, c2 := bus.Subscribe(1)
		defer c1()
		defer c2()

		bus.Publish(1, engine.Metric{Label: "fanout"})
		for i, ch := range []<-chan engine.Metric{ch1, ch2} {
			select {
			case m := <-ch:
				if m.Label != "fanout" {
					t.Fatalf("subscriber %d got %+v", i, m)
				}
			case <-time.After(time.Second):
				t.Fatalf("subscriber %d timed out", i)
			}
		}
	})
}
