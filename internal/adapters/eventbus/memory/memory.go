// Package memory is the in-process ports.EventBus: per-execution fan-out to
// live subscribers with non-blocking delivery (slow subscribers drop events
// rather than stalling the collector).
package memory

import (
	"sync"

	"github.com/heridotlife/Setagaya/internal/domain/engine"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// subBuffer is how many events a subscriber may fall behind before delivery to
// it starts dropping.
const subBuffer = 256

// Bus is the in-memory event bus.
type Bus struct {
	mu   sync.Mutex
	subs map[int64]map[*subscriber]struct{}
}

type subscriber struct {
	ch chan engine.Metric
}

// New returns an empty Bus.
func New() *Bus {
	return &Bus{subs: make(map[int64]map[*subscriber]struct{})}
}

var _ ports.EventBus = (*Bus)(nil)

// Publish delivers m to every current subscriber without blocking.
func (b *Bus) Publish(executionID int64, m engine.Metric) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for sub := range b.subs[executionID] {
		select {
		case sub.ch <- m:
		default: // subscriber is behind: drop rather than stall the collector
		}
	}
}

// Subscribe registers a subscriber and returns its channel and a cancel func.
func (b *Bus) Subscribe(executionID int64) (<-chan engine.Metric, func()) {
	sub := &subscriber{ch: make(chan engine.Metric, subBuffer)}
	b.mu.Lock()
	set, ok := b.subs[executionID]
	if !ok {
		set = make(map[*subscriber]struct{})
		b.subs[executionID] = set
	}
	set[sub] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			delete(b.subs[executionID], sub)
			if len(b.subs[executionID]) == 0 {
				delete(b.subs, executionID)
			}
			close(sub.ch)
		})
	}
	return sub.ch, cancel
}
