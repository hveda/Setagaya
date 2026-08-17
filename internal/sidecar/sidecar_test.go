package sidecar_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/sidecar"
)

type collector struct {
	mu      sync.Mutex
	batches []metrics.Batch
	status  int
	auth    string
}

func (c *collector) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var b metrics.Batch
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		c.mu.Lock()
		c.batches = append(c.batches, b)
		c.auth = r.Header.Get("Authorization")
		status := c.status
		c.mu.Unlock()
		if status == 0 {
			status = http.StatusAccepted
		}
		w.WriteHeader(status)
	}
}

func (c *collector) all() []metrics.Batch {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]metrics.Batch(nil), c.batches...)
}

func (c *collector) intervals() []metrics.Interval {
	var out []metrics.Interval
	for _, b := range c.all() {
		out = append(out, b.Intervals...)
	}
	return out
}

func (c *collector) setStatus(s int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = s
}

func line(t *testing.T, ts int64, label string, samples int64) string {
	t.Helper()
	b, err := json.Marshal(metrics.Interval{
		Timestamp: ts, Label: label, Samples: samples, Succeeded: samples,
		Concurrency: 5,
		Latency:     metrics.Histogram{0.01: samples},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b) + "\n"
}

// run starts a sidecar over a stream file and returns a function that signals
// the engine has exited and waits for the sidecar to finish.
func run(t *testing.T, cfg sidecar.Config) (*sidecar.Sidecar, func()) {
	t.Helper()
	sc := sidecar.New(cfg)
	done := make(chan struct{})
	errc := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	go func() { errc <- sc.Run(ctx, done) }()

	return sc, func() {
		close(done)
		select {
		case err := <-errc:
			if err != nil && ctx.Err() == nil {
				t.Errorf("Run: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("sidecar did not stop after the engine exited")
		}
		cancel()
	}
}

func TestSidecar_ForwardsIntervals(t *testing.T) {
	t.Parallel()

	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "stream.jsonl")
	if err := os.WriteFile(path, []byte(line(t, 1, "checkout-cart", 10)), 0o600); err != nil {
		t.Fatalf("seed stream: %v", err)
	}

	_, stop := run(t, sidecar.Config{
		Identity:      sidecar.Identity{ExecutionID: 7, ScenarioID: 11, RunID: 3, ShardIndex: 2},
		StreamPath:    path,
		IngestURL:     srv.URL,
		Token:         "s3cret",
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
	})
	stop()

	got := c.intervals()
	if len(got) != 1 {
		t.Fatalf("forwarded %d intervals, want 1: %+v", len(got), got)
	}
	if got[0].Label != "checkout-cart" || got[0].Samples != 10 {
		t.Errorf("interval = %+v", got[0])
	}
	if got[0].Latency[0.01] != 10 {
		t.Errorf("latency buckets lost in transit: %+v", got[0].Latency)
	}

	batches := c.all()
	b := batches[0]
	if b.ExecutionID != 7 || b.ScenarioID != 11 || b.RunID != 3 || b.ShardIndex != 2 {
		t.Errorf("batch identity = %+v", b)
	}
	if c.auth != "Bearer s3cret" {
		t.Errorf("Authorization = %q", c.auth)
	}
	// The control plane must be able to tell a finished pod from a silent one.
	if !batches[len(batches)-1].Final {
		t.Error("last batch is not marked final")
	}
}

// A pod dies the moment Kubernetes deletes it, so anything written before that
// must already have been pushed rather than waiting for an orderly shutdown.
func TestSidecar_PushesBeforeTheEngineFinishes(t *testing.T) {
	t.Parallel()

	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "stream.jsonl")
	f, err := os.Create(path) //#nosec G304 -- test temp path
	if err != nil {
		t.Fatalf("create stream: %v", err)
	}

	_, stop := run(t, sidecar.Config{
		StreamPath:    path,
		IngestURL:     srv.URL,
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
	})

	// Write while the "engine" is still running.
	for i := int64(1); i <= 3; i++ {
		if _, err := f.WriteString(line(t, i, "probe", i)); err != nil {
			t.Fatalf("write: %v", err)
		}
		time.Sleep(40 * time.Millisecond)
	}

	// Data reached the collector without the engine having exited.
	if len(c.intervals()) == 0 {
		t.Error("nothing pushed while the engine was still running")
	}

	_ = f.Close()
	stop()

	if got := len(c.intervals()); got != 3 {
		t.Errorf("forwarded %d intervals, want 3", got)
	}
}

// bzt writes a line at a time; a read that catches one half-written must not
// lose it or corrupt the batch.
func TestSidecar_HandlesPartialAndMalformedLines(t *testing.T) {
	t.Parallel()

	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "stream.jsonl")
	f, err := os.Create(path) //#nosec G304 -- test temp path
	if err != nil {
		t.Fatalf("create stream: %v", err)
	}

	_, stop := run(t, sidecar.Config{
		StreamPath:    path,
		IngestURL:     srv.URL,
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
	})

	good := line(t, 1, "probe", 1)
	if _, err := f.WriteString(good[:len(good)/2]); err != nil { // half a line
		t.Fatalf("write: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := f.WriteString(good[len(good)/2:]); err != nil { // its other half
		t.Fatalf("write: %v", err)
	}
	if _, err := f.WriteString("{not json}\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := f.WriteString(line(t, 2, "probe", 2)); err != nil {
		t.Fatalf("write: %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	_ = f.Close()
	stop()

	got := c.intervals()
	if len(got) != 2 {
		t.Fatalf("forwarded %d intervals, want 2 (the split line reassembled, the bad one skipped): %+v", len(got), got)
	}
}

// A brief control-plane outage must not cost the run its measurements.
func TestSidecar_RetriesAfterAFailedPush(t *testing.T) {
	t.Parallel()

	c := &collector{}
	c.setStatus(http.StatusServiceUnavailable)
	srv := httptest.NewServer(c.handler())
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "stream.jsonl")
	if err := os.WriteFile(path, []byte(line(t, 1, "probe", 5)), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	sc, stop := run(t, sidecar.Config{
		StreamPath:    path,
		IngestURL:     srv.URL,
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
	})

	time.Sleep(80 * time.Millisecond)
	if sc.Pending() == 0 {
		t.Error("intervals were dropped after a failed push instead of being retried")
	}
	c.setStatus(http.StatusAccepted)
	time.Sleep(80 * time.Millisecond)
	stop()

	if len(c.intervals()) == 0 {
		t.Error("nothing arrived after the collector recovered")
	}
}

// Engines disagree about labels: JMeter echoes the configured one while apiritif
// and k6 report the URL. Without mapping, the same request is two series.
func TestSidecar_MapsEngineLabelsBackToHonryusOwn(t *testing.T) {
	t.Parallel()

	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "stream.jsonl")
	body := line(t, 1, "http://checkout.svc/cart", 4) + line(t, 1, "checkout-cart", 6)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, stop := run(t, sidecar.Config{
		StreamPath:    path,
		IngestURL:     srv.URL,
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
		LabelMap:      map[string]string{"http://checkout.svc/cart": "checkout-cart"},
	})
	stop()

	for _, in := range c.intervals() {
		if in.Label != "checkout-cart" {
			t.Errorf("label %q was not mapped to the one Honryu assigned", in.Label)
		}
	}
}

// The sidecar and the engine start together and either may win the race.
func TestSidecar_WaitsForAStreamThatDoesNotExistYet(t *testing.T) {
	t.Parallel()

	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "not-yet.jsonl")
	_, stop := run(t, sidecar.Config{
		StreamPath:    path,
		IngestURL:     srv.URL,
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
	})

	time.Sleep(40 * time.Millisecond)
	if err := os.WriteFile(path, []byte(line(t, 1, "probe", 3)), 0o600); err != nil {
		t.Fatalf("late create: %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	stop()

	if len(c.intervals()) != 1 {
		t.Errorf("forwarded %d intervals, want 1 from the late-created stream", len(c.intervals()))
	}
}

// A batch with nothing in it must still carry an empty list. Go marshals a nil
// slice as null, which would force every consumer to handle a case meaning
// exactly what the empty one means.
func TestSidecar_EmptyBatchSendsAnEmptyListNotNull(t *testing.T) {
	t.Parallel()

	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "stream.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, stop := run(t, sidecar.Config{
		StreamPath:    path,
		IngestURL:     srv.URL,
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
	})
	stop()

	if !strings.Contains(string(body), `"intervals":[]`) {
		t.Errorf("final batch body = %s, want an empty intervals list", body)
	}
}

// Every interval carries a sequence, unique and increasing within the stream.
// It is what lets the control plane tell an interval it has already absorbed
// from one it has not.
func TestSidecar_SequencesIntervals(t *testing.T) {
	t.Parallel()

	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "stream.jsonl")
	var stream strings.Builder
	for ts := int64(1); ts <= 3; ts++ {
		stream.WriteString(line(t, ts, "checkout-cart", 5))
		stream.WriteString(line(t, ts, "checkout-pay", 5))
	}
	if err := os.WriteFile(path, []byte(stream.String()), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, stop := run(t, sidecar.Config{
		StreamPath:    path,
		IngestURL:     srv.URL,
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
	})
	time.Sleep(80 * time.Millisecond)
	stop()

	got := c.intervals()
	if len(got) != 6 {
		t.Fatalf("got %d intervals, want 6", len(got))
	}
	for i, iv := range got {
		if iv.Seq != int64(i+1) {
			t.Errorf("interval %d has seq %d, want %d", i, iv.Seq, i+1)
		}
	}
	for _, b := range c.all() {
		if b.StreamID == "" {
			t.Error("a batch carried no stream id")
		}
	}
}

// The case the sequence exists for. A push whose response is lost is followed
// not by an identical batch but by a superset: the intervals that did arrive,
// plus everything read since. Skipping the batch would lose the new intervals
// and keeping it whole would double-count the old, so the two have to be
// separable one interval at a time.
func TestSidecar_RetryResendsOldIntervalsAlongsideNew(t *testing.T) {
	t.Parallel()

	c := &collector{}
	c.setStatus(http.StatusServiceUnavailable)
	srv := httptest.NewServer(c.handler())
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "stream.jsonl")
	if err := os.WriteFile(path, []byte(line(t, 1, "probe", 5)), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, stop := run(t, sidecar.Config{
		StreamPath:    path,
		IngestURL:     srv.URL,
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
	})
	time.Sleep(60 * time.Millisecond)

	// A second interval arrives while the first is still unacknowledged.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := f.WriteString(line(t, 2, "probe", 7)); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = f.Close()

	c.setStatus(http.StatusAccepted)
	time.Sleep(80 * time.Millisecond)
	stop()

	// The batch that finally lands carries both, and their sequences separate
	// what a control plane had already seen from what it had not.
	var seqs []int64
	for _, iv := range c.intervals() {
		seqs = append(seqs, iv.Seq)
	}
	if len(seqs) < 2 {
		t.Fatalf("intervals seen = %v, want the retried one and the new one", seqs)
	}
	seen := map[int64]bool{}
	for _, s := range seqs {
		seen[s] = true
	}
	if !seen[1] || !seen[2] {
		t.Errorf("sequences = %v, want both 1 and 2 to have arrived", seqs)
	}
}

// The pod now outlives bzt (see the k8s adapter), so the sidecar can no longer
// tell the engine is done by watching the pod get torn down. It notices the
// exit-code file the engine writes instead, and carries the code on its final
// batch -- the only feed taurus.OutcomeFromExitCode has.
func TestSidecar_DetectsEngineCompletionFromTheExitCodeFile(t *testing.T) {
	t.Parallel()

	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()

	dir := t.TempDir()
	streamPath := filepath.Join(dir, "stream.jsonl")
	exitCodePath := filepath.Join(dir, "exit-code")
	if err := os.WriteFile(streamPath, []byte(line(t, 1, "probe", 5)), 0o600); err != nil {
		t.Fatalf("seed stream: %v", err)
	}

	sc := sidecar.New(sidecar.Config{
		StreamPath:    streamPath,
		ExitCodePath:  exitCodePath,
		IngestURL:     srv.URL,
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
	})
	done := make(chan struct{}) // never closed: completion must come from the file
	errc := make(chan error, 1)
	go func() { errc <- sc.Run(context.Background(), done) }()

	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(exitCodePath, []byte("3\n"), 0o600); err != nil {
		t.Fatalf("write exit code: %v", err)
	}

	select {
	case err := <-errc:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sidecar did not stop after the exit code appeared")
	}

	batches := c.all()
	if len(batches) == 0 || !batches[len(batches)-1].Final {
		t.Fatalf("last batch is not final: %+v", batches)
	}
	last := batches[len(batches)-1]
	if last.ExitCode == nil || *last.ExitCode != 3 {
		t.Errorf("ExitCode = %v, want 3", last.ExitCode)
	}
}

// The exit-code file can appear in the instant after poll.C's last tick but
// before <-done fires -- readable on disk, but not yet checked. That code must
// not be lost just because completion arrived by signal rather than by the
// next poll.
func TestSidecar_DonePicksUpAnExitCodeWrittenSincePollLastChecked(t *testing.T) {
	t.Parallel()

	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()

	dir := t.TempDir()
	streamPath := filepath.Join(dir, "stream.jsonl")
	exitCodePath := filepath.Join(dir, "exit-code")
	if err := os.WriteFile(streamPath, []byte(line(t, 1, "probe", 5)), 0o600); err != nil {
		t.Fatalf("seed stream: %v", err)
	}

	sc := sidecar.New(sidecar.Config{
		StreamPath:    streamPath,
		ExitCodePath:  exitCodePath,
		IngestURL:     srv.URL,
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  time.Hour, // never ticks: isolates the <-done path
	})
	done := make(chan struct{})
	errc := make(chan error, 1)
	go func() { errc <- sc.Run(context.Background(), done) }()

	time.Sleep(30 * time.Millisecond) // let the sidecar open the stream and settle
	if err := os.WriteFile(exitCodePath, []byte("3\n"), 0o600); err != nil {
		t.Fatalf("write exit code: %v", err)
	}
	close(done) // completion arrives by signal, not by the next poll tick

	select {
	case err := <-errc:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sidecar did not stop after done fired")
	}

	batches := c.all()
	if len(batches) == 0 || !batches[len(batches)-1].Final {
		t.Fatalf("last batch is not final: %+v", batches)
	}
	last := batches[len(batches)-1]
	if last.ExitCode == nil || *last.ExitCode != 3 {
		t.Errorf("ExitCode = %v, want 3 (checkExitCode must run on the <-done path too)", last.ExitCode)
	}
}

// No path configured means no engine to watch for -- used by tests and any
// deployment that has not wired the file up. It must not be treated as "found".
func TestSidecar_NoExitCodePathNeverSignalsCompletion(t *testing.T) {
	t.Parallel()

	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "stream.jsonl")
	if err := os.WriteFile(path, []byte(line(t, 1, "probe", 5)), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, stop := run(t, sidecar.Config{
		StreamPath:    path,
		IngestURL:     srv.URL,
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
	})
	time.Sleep(60 * time.Millisecond)
	stop() // only this closes `done`; if ExitCodePath="" ever triggered on its
	// own, Run would have already returned before stop() and this would hang.
}

// The engine writes its own exit code; anything unreadable there is a defect
// in that write, not a signal the run finished. One malformed value must not
// be mistaken for completion.
func TestSidecar_UnreadableExitCodeIsNotTreatedAsCompletion(t *testing.T) {
	t.Parallel()

	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()

	dir := t.TempDir()
	streamPath := filepath.Join(dir, "stream.jsonl")
	exitCodePath := filepath.Join(dir, "exit-code")
	if err := os.WriteFile(streamPath, []byte(line(t, 1, "probe", 5)), 0o600); err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	if err := os.WriteFile(exitCodePath, []byte("not-a-number"), 0o600); err != nil {
		t.Fatalf("seed exit code: %v", err)
	}

	_, stop := run(t, sidecar.Config{
		StreamPath:    streamPath,
		ExitCodePath:  exitCodePath,
		IngestURL:     srv.URL,
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
	})
	time.Sleep(60 * time.Millisecond)
	stop() // hangs if the malformed value was mistaken for completion and Run
	// already returned on its own, since only stop() closes `done`.

	for _, b := range c.all() {
		if b.ExitCode != nil {
			t.Errorf("a malformed exit code was reported as %d", *b.ExitCode)
		}
	}
}

// Sent reports how many batches were pushed: it starts at zero and ends equal
// to the number of batches the ingest server actually received.
func TestSidecar_SentCountsPushedBatches(t *testing.T) {
	t.Parallel()

	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "stream.jsonl")
	seed := line(t, 1, "checkout-cart", 10) + line(t, 2, "checkout-cart", 5)
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed stream: %v", err)
	}

	fresh := sidecar.New(sidecar.Config{StreamPath: path, IngestURL: srv.URL})
	if got := fresh.Sent(); got != 0 {
		t.Fatalf("Sent() on a fresh sidecar = %d, want 0", got)
	}

	sc, stop := run(t, sidecar.Config{
		Identity:      sidecar.Identity{ExecutionID: 7, ScenarioID: 11, RunID: 3, ShardIndex: 0},
		StreamPath:    path,
		IngestURL:     srv.URL,
		FlushInterval: 20 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
	})
	stop()

	batches := c.all()
	if len(batches) == 0 {
		t.Fatal("ingest server received no batches")
	}
	if got := sc.Sent(); got != len(batches) {
		t.Fatalf("Sent() = %d, want %d (batches the server received)", got, len(batches))
	}
}
