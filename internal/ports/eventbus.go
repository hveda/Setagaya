package ports

import "github.com/heridotlife/Setagaya/internal/domain/engine"

// EventBus fans engine metrics from the collector to any number of live
// subscribers (e.g. SSE clients), scoped per execution. It is an in-process
// pub/sub; a distributed deployment swaps in a networked implementation behind
// this interface.
type EventBus interface {
	// Publish delivers m to every current subscriber of executionID. It never
	// blocks on a slow subscriber: undeliverable events are dropped.
	Publish(executionID int64, m engine.Metric)
	// Subscribe registers a subscriber for executionID, returning its event
	// channel and a cancel function that unsubscribes and closes the channel.
	Subscribe(executionID int64) (events <-chan engine.Metric, cancel func())
}
