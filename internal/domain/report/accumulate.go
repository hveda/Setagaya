package report

import (
	"sort"

	"github.com/heridotlife/honryu/internal/domain/metrics"
)

// Accumulator is a run's report while it is still being measured.
//
// Measurements arrive a batch at a time, from several pods, over minutes. The
// control plane cannot hold every one of them until the run ends -- not across a
// restart, and not across replicas -- but it can hold this: merged response-time
// buckets per label, concurrency per second, and a count per error signature.
// That state is bounded by a run's labels and its duration rather than growing
// with every pod-second measured.
//
// Build is written in terms of it. Two paths that summarised a run differently
// would eventually disagree about a verdict, and the one that disagreed would be
// the one nobody tested.
type Accumulator struct {
	labels     map[string]*labelState
	seconds    map[int64]*secondState
	signatures map[Signature]*ErrorSignature
}

type labelState struct {
	samples int64
	failed  int64
	latency metrics.Histogram
}

// secondState holds the two readings of a second's concurrency that
// peakConcurrency chooses between.
type secondState struct {
	engine int
	labels int
}

// NewAccumulator returns an empty accumulator.
func NewAccumulator() *Accumulator {
	return &Accumulator{
		labels:     map[string]*labelState{},
		seconds:    map[int64]*secondState{},
		signatures: map[Signature]*ErrorSignature{},
	}
}

// Add folds one interval into the run.
//
// Adding the same interval twice counts it twice: an interval is a measurement,
// not a fact about the world, and only the caller knows whether one it has
// already absorbed has arrived again.
func (a *Accumulator) Add(iv metrics.Interval) {
	sec := a.second(iv.Timestamp)
	if iv.Label == TotalLabel {
		// The engine's own aggregate. Its requests are already counted per
		// label, but its concurrency is this shard's true total -- see
		// peakConcurrency.
		sec.engine += iv.Concurrency
		return
	}
	sec.labels += iv.Concurrency

	st, ok := a.labels[iv.Label]
	if !ok {
		st = &labelState{latency: metrics.Histogram{}}
		a.labels[iv.Label] = st
	}
	st.samples += iv.Samples
	st.failed += iv.Failed
	st.latency.Merge(iv.Latency)

	for _, e := range iv.Errors {
		sig := NewSignature(iv.Label, e)
		existing, ok := a.signatures[sig]
		if !ok {
			existing = &ErrorSignature{Signature: sig}
			a.signatures[sig] = existing
		}
		existing.Count += e.Count
		existing.addExemplar(e.Message)
	}
}

func (a *Accumulator) second(ts int64) *secondState {
	sec, ok := a.seconds[ts]
	if !ok {
		sec = &secondState{}
		a.seconds[ts] = sec
	}
	return sec
}

// Report turns what has been accumulated into the run's report.
func (a *Accumulator) Report(m Meta) Report {
	rep := Report{
		ExecutionID: m.ExecutionID,
		ScenarioID:  m.ScenarioID,
		RunID:       m.RunID,
		Engine:      m.Engine,
		StartedAt:   m.StartedAt,
		EndedAt:     m.EndedAt,
		Outcome:     m.Outcome,
		Requested:   m.Requested,
	}

	overall := metrics.Histogram{}
	names := make([]string, 0, len(a.labels))
	for name, st := range a.labels {
		names = append(names, name)
		rep.Achieved.Samples += st.samples
		rep.Achieved.Failed += st.failed
		overall.Merge(st.latency)
	}
	sort.Strings(names) // stable order, so two reports of the same run match
	for _, name := range names {
		st := a.labels[name]
		rep.Labels = append(rep.Labels, LabelSummary{
			Label:     name,
			Samples:   st.samples,
			Failed:    st.failed,
			ErrorRate: rate(st.failed, st.samples),
			Latency:   st.latency.Percentiles(reportedPercentiles...),
		})
	}

	rep.Achieved.Concurrency = a.peak()
	rep.Achieved.DurationSeconds = m.achievedSeconds()
	rep.Achieved.Throughput = perSecond(rep.Achieved.Samples, rep.Achieved.DurationSeconds)
	rep.ErrorRate = rate(rep.Achieved.Failed, rep.Achieved.Samples)
	rep.Latency = overall.Percentiles(reportedPercentiles...)
	rep.Errors, rep.Attribution = a.errors()
	return rep
}

// peak is the most virtual users the run had in flight at once.
//
// A shard only ever reports its own users, so they are summed within each second
// and the peak taken across seconds. Taking the largest single figure instead
// would report a run sharded over four pods as a quarter of its size, which is
// the normal case since Phase 3.
//
// Where the engine sent its own aggregate row that figure is this shard's true
// total and is authoritative -- it counts users between requests, which the
// per-label rows cannot see. Where it did not, summing the per-label rows
// recovers the total, since a virtual user is executing one request at a time.
// The larger is taken so a run is never reported as smaller than it demonstrably
// was.
func (a *Accumulator) peak() int {
	peak := 0
	for _, sec := range a.seconds {
		n := sec.labels
		if sec.engine > n {
			n = sec.engine
		}
		if n > peak {
			peak = n
		}
	}
	return peak
}

// errors is every failure mode, dominant first, with the run's failures split by
// the side that caused them.
func (a *Accumulator) errors() ([]ErrorSignature, Attribution) {
	out := make([]ErrorSignature, 0, len(a.signatures))
	var attr Attribution
	for _, e := range a.signatures {
		out = append(out, *e)
		switch e.Side {
		case SideTarget:
			attr.Target += e.Count
		case SideEngine:
			attr.Engine += e.Count
		default:
			attr.Unknown += e.Count
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].String() < out[j].String()
	})
	return out, attr
}

// LabelProgress is one request's accumulated measurements.
type LabelProgress struct {
	Label   string
	Samples int64
	Failed  int64
	Latency metrics.Histogram
}

// SecondProgress is one second's concurrency, as each source reported it.
type SecondProgress struct {
	Second int64
	// Engine is the sum of the shards' own aggregate rows for this second;
	// Labels is the sum of their per-label rows. Both are kept because which one
	// is meaningful depends on what the engine sent -- see Accumulator.peak.
	Engine int
	Labels int
}

// Snapshot is an accumulator's state in a form a store can write down and read
// back. It is deliberately flat: the working state crosses a database between
// the batch that produced it and the run that ends minutes later.
type Snapshot struct {
	Labels     []LabelProgress
	Seconds    []SecondProgress
	Signatures []ErrorSignature
}

// Snapshot exports the accumulated state, ordered so two snapshots of the same
// state are identical.
func (a *Accumulator) Snapshot() Snapshot {
	var s Snapshot
	for name, st := range a.labels {
		s.Labels = append(s.Labels, LabelProgress{
			Label: name, Samples: st.samples, Failed: st.failed, Latency: st.latency,
		})
	}
	sort.Slice(s.Labels, func(i, j int) bool { return s.Labels[i].Label < s.Labels[j].Label })

	for ts, sec := range a.seconds {
		s.Seconds = append(s.Seconds, SecondProgress{Second: ts, Engine: sec.engine, Labels: sec.labels})
	}
	sort.Slice(s.Seconds, func(i, j int) bool { return s.Seconds[i].Second < s.Seconds[j].Second })

	for _, e := range a.signatures {
		s.Signatures = append(s.Signatures, *e)
	}
	sort.Slice(s.Signatures, func(i, j int) bool {
		return s.Signatures[i].String() < s.Signatures[j].String()
	})
	return s
}

// Restore rebuilds an accumulator from stored state, so a run that outlives the
// process measuring it still produces one report rather than one per restart.
func Restore(s Snapshot) *Accumulator {
	a := NewAccumulator()
	for _, l := range s.Labels {
		hist := metrics.Histogram{}
		hist.Merge(l.Latency)
		a.labels[l.Label] = &labelState{samples: l.Samples, failed: l.Failed, latency: hist}
	}
	for _, sec := range s.Seconds {
		a.seconds[sec.Second] = &secondState{engine: sec.Engine, labels: sec.Labels}
	}
	for _, e := range s.Signatures {
		sig := e
		a.signatures[sig.Signature] = &sig
	}
	return a
}
