package memory_test

import (
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/adapters/eventbus/memory"
	"github.com/heridotlife/Setagaya/v3/internal/domain/engine"
	"github.com/heridotlife/Setagaya/v3/internal/ports"
	"github.com/heridotlife/Setagaya/v3/internal/ports/eventbustest"
)

func TestMemoryBus_Contract(t *testing.T) {
	t.Parallel()
	eventbustest.RunEventBusContract(t, func(t *testing.T) ports.EventBus {
		return memory.New()
	})
}

func TestMemoryBus_SlowSubscriberDropsRatherThanBlocks(t *testing.T) {
	t.Parallel()
	bus := memory.New()
	_, cancel := bus.Subscribe(1)
	defer cancel()

	// Publish far more than the buffer without draining: must not block.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			bus.Publish(1, engine.Metric{Label: "x"})
		}
		close(done)
	}()
	<-done
}
