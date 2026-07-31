// Package sidecar forwards an engine pod's measurements to the control plane.
//
// It runs beside bzt in every engine pod and tails the JSON-lines stream the
// Honryu bzt reporter writes, batching intervals and pushing them out.
//
// Pushing rather than being scraped is what makes results survivable. bzt has no
// SIGTERM handler, so when Kubernetes deletes a pod bzt dies at once and writes
// no final report; and scraping a pod that is already gone collects nothing.
// Anything already pushed is safe, so the sidecar sends little and often.
package sidecar

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/heridotlife/honryu/internal/domain/metrics"
)

// Identity is the pod this sidecar speaks for. It stamps every batch so the
// control plane can attribute measurements without inspecting them.
type Identity struct {
	ExecutionID int64
	ScenarioID  int64
	RunID       int64
	ShardIndex  int
}

// Config configures a Sidecar.
type Config struct {
	Identity Identity
	// StreamPath is the JSON-lines file the bzt reporter writes.
	StreamPath string
	// IngestURL receives batches.
	IngestURL string
	// Token authenticates to the control plane, sent as a bearer token.
	Token string
	// FlushInterval is how often a batch is sent. Shorter loses less when a pod
	// is killed; longer costs fewer requests.
	FlushInterval time.Duration
	// PollInterval is how often the stream is checked for new lines.
	PollInterval time.Duration
	// LabelMap renames engine-reported labels back to the ones Honryu assigned
	// when it compiled the config. Engines disagree here -- JMeter echoes the
	// configured label, apiritif and k6 report the URL -- and without this the
	// same request appears under two names and cannot be compared across runs.
	LabelMap map[string]string
	// Client sends the batches; nil uses a default with a timeout.
	Client *http.Client
	// Now is the clock, injectable for tests.
	Now func() time.Time
}

// Sidecar tails an engine's KPI stream and pushes it to the control plane.
type Sidecar struct {
	cfg     Config
	client  *http.Client
	pending []metrics.Interval
	// carry holds a partially-read trailing line between drains.
	carry []byte
	// sent counts batches pushed, for tests and logging.
	sent int
	// seq numbers intervals within this sidecar's stream, so the control plane
	// can tell an interval it has already absorbed from one it has not. A failed
	// push is followed by a superset batch, not an identical one, so nothing
	// coarser than a per-interval sequence would separate them.
	seq int64
	// streamID identifies this instance. Sequences restart with the process, and
	// without it a restarted pod's measurements would look like duplicates.
	streamID string
}

// New builds a Sidecar, applying defaults for anything the caller left unset.
func New(cfg Config) *Sidecar {
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = time.Second
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 200 * time.Millisecond
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Sidecar{cfg: cfg, client: client, streamID: newStreamID(cfg.Now())}
}

// newStreamID names this sidecar instance. It only has to differ from the
// previous instance in the same pod, so the start time plus a random suffix is
// enough -- and it stays readable in a log line, unlike a bare UUID.
func newStreamID(now time.Time) string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Randomness is a tiebreak for two starts in the same second, not a
		// security property; the timestamp alone still separates restarts.
		return strconv.FormatInt(now.UnixNano(), 36)
	}
	return strconv.FormatInt(now.Unix(), 36) + "-" + hex.EncodeToString(b[:])
}

// Run tails the stream until ctx is cancelled or the stream is closed and fully
// consumed, then sends a final batch.
//
// A stream that does not exist yet is waited for: the sidecar and bzt start
// together and either may win.
func (s *Sidecar) Run(ctx context.Context, done <-chan struct{}) error {
	f, err := s.waitForStream(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	flush := time.NewTicker(s.cfg.FlushInterval)
	defer flush.Stop()
	poll := time.NewTicker(s.cfg.PollInterval)
	defer poll.Stop()

	engineDone := false
	for {
		s.drain(f)

		select {
		case <-ctx.Done():
			// Send whatever was read before giving up; the pod is going away.
			return s.flush(context.WithoutCancel(ctx), true)
		case <-done:
			// bzt exited. Read what it left behind, then send a final batch.
			engineDone = true
		case <-flush.C:
			if err := s.flush(ctx, false); err != nil {
				// A failed push must not stop collection: the pod may outlive a
				// brief control-plane outage, and the next flush retries.
				logf("push failed: %v", err)
			}
		case <-poll.C:
		}

		if engineDone {
			s.drain(f)
			return s.flush(ctx, true)
		}
	}
}

// waitForStream opens the KPI stream, waiting for the engine to create it.
func (s *Sidecar) waitForStream(ctx context.Context) (*os.File, error) {
	for {
		f, err := os.Open(s.cfg.StreamPath) //#nosec G304 -- path is this pod's own config
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("sidecar: open stream: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.cfg.PollInterval):
		}
	}
}

// drain reads every complete line currently available, holding any partial
// trailing line until the rest of it arrives.
//
// The carry buffer is load-bearing rather than tidy: the engine appends a line
// at a time, and a read that lands mid-write returns half of one. bufio cannot
// push those bytes back, so without carrying them the interval they describe is
// lost outright -- silently, and only under the timing that makes it hardest to
// notice.
func (s *Sidecar) drain(r io.Reader) {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			s.carry = append(s.carry, buf[:n]...)
			s.consumeLines()
		}
		if err != nil {
			return
		}
	}
}

// consumeLines takes every complete line out of the carry buffer.
func (s *Sidecar) consumeLines() {
	for {
		i := bytes.IndexByte(s.carry, '\n')
		if i < 0 {
			break
		}
		s.consume(s.carry[:i])
		s.carry = s.carry[i+1:]
	}
	// Re-slicing keeps the whole original array alive; copy the remainder so a
	// long run does not hold every byte it has ever read.
	if len(s.carry) == 0 {
		s.carry = s.carry[:0]
	} else if cap(s.carry) > 4*len(s.carry) {
		s.carry = append([]byte(nil), s.carry...)
	}
}

func (s *Sidecar) consume(line []byte) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return
	}
	var in metrics.Interval
	if err := json.Unmarshal(line, &in); err != nil {
		// One malformed line must not end collection; the rest of the run is
		// still worth reporting.
		logf("skipping unparseable line: %v", err)
		return
	}
	if mapped, ok := s.cfg.LabelMap[in.Label]; ok {
		in.Label = mapped
	}
	// Stamped here rather than by the reporter: sequencing is the sidecar's
	// contract with the control plane, not bzt's.
	s.seq++
	in.Seq = s.seq
	s.pending = append(s.pending, in)
}

// flush sends the pending intervals. It clears them only on success, so a failed
// push is retried rather than dropped.
func (s *Sidecar) flush(ctx context.Context, final bool) error {
	if len(s.pending) == 0 && !final {
		return nil
	}
	// A nil slice marshals as null, not [], and a final batch after a clean
	// flush has nothing pending. Sending null would make every consumer handle
	// a case that means the same as the empty one.
	intervals := s.pending
	if intervals == nil {
		intervals = []metrics.Interval{}
	}
	batch := metrics.Batch{
		ExecutionID: s.cfg.Identity.ExecutionID,
		ScenarioID:  s.cfg.Identity.ScenarioID,
		RunID:       s.cfg.Identity.RunID,
		ShardIndex:  s.cfg.Identity.ShardIndex,
		StreamID:    s.streamID,
		Intervals:   intervals,
		Final:       final,
	}
	body, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("sidecar: encode batch: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.IngestURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sidecar: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.Token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("sidecar: push: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("sidecar: ingest returned %s", resp.Status)
	}

	s.pending = nil
	s.sent++
	return nil
}

// Sent reports how many batches were pushed.
func (s *Sidecar) Sent() int { return s.sent }

// Pending reports how many intervals are waiting to be pushed.
func (s *Sidecar) Pending() int { return len(s.pending) }
