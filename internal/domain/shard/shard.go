// Package shard divides one scenario's load profile across the engine pods that
// will produce it.
//
// Honryu shards the load itself rather than using an engine's own distributed
// mode: bzt's distributed support is effectively JMeter-only, so relying on it
// would make the engine set uneven. Each pod runs an ordinary single bzt with a
// fraction of the load, and the control plane aggregates the results.
//
// Pure domain: arithmetic only, deterministic, no I/O.
package shard

import (
	"errors"
	"fmt"

	"github.com/heridotlife/honryu/internal/domain/loadprofile"
)

// Planning errors. Callers compare with errors.Is.
var (
	ErrShardsInvalid      = errors.New("shard: shard count must be greater than zero")
	ErrConcurrencyInvalid = errors.New("shard: concurrency must be greater than zero")
)

// Shard is one engine pod's portion of a scenario's load.
type Shard struct {
	// Index is the shard's position, 0-based. It identifies the pod and is
	// stable for a given plan.
	Index int
	// Concurrency is this pod's share of the virtual users.
	Concurrency int
	// Throughput is this pod's share of the requested request rate. Zero means
	// unlimited, matching the load profile.
	Throughput int
	// RampupSeconds and DurationSeconds are the same for every shard: pods ramp
	// together so the aggregate profile is the one that was requested.
	RampupSeconds   int
	DurationSeconds int
}

// Plan divides an entry's load across n shards.
//
// Virtual users are split as evenly as possible, with the remainder going to the
// earliest shards, so no pod carries more than one user above any other. An
// unbalanced split would leave one pod finishing after the rest and stretch the
// run past its declared duration.
//
// Fewer shards than requested are returned when there are not enough virtual
// users to give each pod at least one: an empty pod generates nothing, and bzt
// rejects a concurrency of zero outright. Callers that care compare the returned
// length against what they asked for.
func Plan(e loadprofile.Entry, n int) ([]Shard, error) {
	if n <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrShardsInvalid, n)
	}
	if e.Concurrency <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrConcurrencyInvalid, e.Concurrency)
	}
	if n > e.Concurrency {
		n = e.Concurrency
	}

	shards := make([]Shard, n)
	baseConc, extraConc := e.Concurrency/n, e.Concurrency%n
	baseTput, extraTput := e.Throughput/n, e.Throughput%n

	for i := range shards {
		s := Shard{
			Index:           i,
			Concurrency:     baseConc,
			Throughput:      baseTput,
			RampupSeconds:   e.Rampup,
			DurationSeconds: e.Duration,
		}
		// The remainder goes to the earliest shards rather than being dropped;
		// dropping it would under-shoot the requested load and make the target
		// look healthier than it is.
		if i < extraConc {
			s.Concurrency++
		}
		if i < extraTput {
			s.Throughput++
		}
		shards[i] = s
	}
	return shards, nil
}

// Total sums a plan's load, for asserting that a set of shards still adds up to
// what was requested.
func Total(shards []Shard) (concurrency, throughput int) {
	for _, s := range shards {
		concurrency += s.Concurrency
		throughput += s.Throughput
	}
	return concurrency, throughput
}
