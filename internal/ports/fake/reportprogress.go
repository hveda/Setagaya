package fake

import (
	"context"
	"sort"
	"sync"

	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/ports"
)

// ReportProgress is an in-memory ports.ReportProgress for fast use-case tests.
type ReportProgress struct {
	mu   sync.Mutex
	runs map[int64]*progressRun

	// AbsorbErr, when set, is returned by Absorb.
	AbsorbErr error
}

type progressRun struct {
	acc    *report.Accumulator
	shards map[int]*shardStream
}

// shardStream is one pod's position in its own stream.
type shardStream struct {
	streamID string
	// seq is the highest sequence absorbed from this stream.
	seq      int64
	finished bool
	exitCode *int
}

// NewReportProgress builds an empty accumulator store.
func NewReportProgress() *ReportProgress {
	return &ReportProgress{runs: map[int64]*progressRun{}}
}

var _ ports.ReportProgress = (*ReportProgress)(nil)

// Absorb merges a shard's new intervals into the run.
func (p *ReportProgress) Absorb(_ context.Context, b ports.ProgressBatch) error {
	if p.AbsorbErr != nil {
		return p.AbsorbErr
	}
	if err := b.Validate(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	run, ok := p.runs[b.RunID]
	if !ok {
		run = &progressRun{acc: report.NewAccumulator(), shards: map[int]*shardStream{}}
		p.runs[b.RunID] = run
	}
	shard, ok := run.shards[b.ShardIndex]
	if !ok {
		shard = &shardStream{streamID: b.StreamID}
		run.shards[b.ShardIndex] = shard
	}
	// A different stream is a restarted pod, whose sequences begin again at one.
	// Holding the old watermark would discard everything it measures from here on.
	if shard.streamID != b.StreamID {
		shard.streamID = b.StreamID
		shard.seq = 0
	}

	for _, iv := range b.Intervals {
		if iv.Seq <= shard.seq {
			continue // already absorbed; a retry re-sent it
		}
		run.acc.Add(iv)
		shard.seq = iv.Seq
	}
	if b.Final {
		shard.finished = true
	}
	if b.ExitCode != nil {
		shard.exitCode = b.ExitCode
	}
	return nil
}

// Snapshot returns the run's accumulated state.
func (p *ReportProgress) Snapshot(_ context.Context, runID int64) (report.Snapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	run, ok := p.runs[runID]
	if !ok {
		return report.Snapshot{}, nil
	}
	return run.acc.Snapshot(), nil
}

// ShardStates returns each shard seen so far and its completion state.
func (p *ReportProgress) ShardStates(_ context.Context, runID int64) ([]ports.ShardState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	run, ok := p.runs[runID]
	if !ok {
		return nil, nil
	}
	out := make([]ports.ShardState, 0, len(run.shards))
	for idx, shard := range run.shards {
		out = append(out, ports.ShardState{ShardIndex: idx, Finished: shard.finished, ExitCode: shard.exitCode})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ShardIndex < out[j].ShardIndex })
	return out, nil
}

// Discard drops a run's working state.
func (p *ReportProgress) Discard(_ context.Context, runID int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.runs, runID)
	return nil
}
