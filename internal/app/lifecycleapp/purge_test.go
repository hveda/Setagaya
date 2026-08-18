package lifecycleapp_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/ports"
)

// failingRepo arms one repo method with an injected error, delegating
// everything else to the wrapped store.
type failingRepo struct {
	lifecycleapp.Repo
	getExecErr    error
	currentRunErr error
	stopRunErr    error
}

func (r *failingRepo) GetExecution(ctx context.Context, id int64) (execution.Execution, error) {
	if r.getExecErr != nil {
		return execution.Execution{}, r.getExecErr
	}
	return r.Repo.GetExecution(ctx, id)
}

func (r *failingRepo) CurrentRun(ctx context.Context, id int64) (int64, bool, error) {
	if r.currentRunErr != nil {
		return 0, false, r.currentRunErr
	}
	return r.Repo.CurrentRun(ctx, id)
}

func (r *failingRepo) StopRun(ctx context.Context, id int64) error {
	if r.stopRunErr != nil {
		return r.stopRunErr
	}
	return r.Repo.StopRun(ctx, id)
}

// callLogScheduler records, in order, the scheduler methods a Purge touches,
// and can fail the purge call itself.
type callLogScheduler struct {
	ports.Scheduler
	mu       sync.Mutex
	calls    []string
	purgeErr error
}

func (s *callLogScheduler) record(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, name)
}

func (s *callLogScheduler) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

func (s *callLogScheduler) PodLog(ctx context.Context, ref ports.ClusterRef, executionID, scenarioID int64, shard int) (string, error) {
	s.record("podlog")
	return s.Scheduler.PodLog(ctx, ref, executionID, scenarioID, shard)
}

func (s *callLogScheduler) PurgeExecution(ctx context.Context, ref ports.ClusterRef, executionID int64) error {
	s.record("purge")
	if s.purgeErr != nil {
		return s.purgeErr
	}
	return s.Scheduler.PurgeExecution(ctx, ref, executionID)
}

// wrappedEnv rebuilds the env's service over wrapped repo and scheduler
// doubles, so error injection and call logging observe the real flow.
func wrappedEnv(t *testing.T, e *env, repo lifecycleapp.Repo, sched ports.Scheduler) *lifecycleapp.Service {
	t.Helper()
	return lifecycleapp.NewService(repo, sched, e.obj, lifecycleapp.StaticImage(image))
}

// Purging an unknown execution reports not-found and never touches engines.
func TestPurge_MissingExecutionIsNotFound(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 1)
	sched := &callLogScheduler{Scheduler: e.sched}
	svc := wrappedEnv(t, e, e.store, sched)

	if err := svc.Purge(context.Background(), e.executionID+999); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Purge(unknown) = %v, want ErrNotFound", err)
	}
	if calls := sched.snapshot(); len(calls) != 0 {
		t.Fatalf("scheduler saw %v, want no calls for an unknown execution", calls)
	}
}

// A repo failure must leave the cluster alone: no pod logs fetched, no pods
// deleted -- a purge that cannot even read its state must not act on it.
func TestPurge_RepoFailuresNeverReachTheScheduler(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		armed func(*failingRepo)
	}{
		{"GetExecution", func(r *failingRepo) { r.getExecErr = errors.New("repo down") }},
		{"CurrentRun", func(r *failingRepo) { r.currentRunErr = errors.New("repo down") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := setup(t, false, 1)
			ctx := context.Background()
			repo := &failingRepo{Repo: e.store}
			sched := &callLogScheduler{Scheduler: e.sched}
			svc := wrappedEnv(t, e, repo, sched)
			if err := svc.Deploy(ctx, e.executionID); err != nil {
				t.Fatalf("Deploy: %v", err)
			}
			tc.armed(repo)

			if err := svc.Purge(ctx, e.executionID); err == nil {
				t.Fatal("Purge with a failing repo: want error")
			}
			if calls := sched.snapshot(); len(calls) != 0 {
				t.Fatalf("scheduler saw %v, want nothing -- the repo failed first", calls)
			}
			deployed, _ := e.sched.DeployedExecutions(ctx, "")
			if _, ok := deployed[e.executionID]; !ok {
				t.Fatal("engines vanished although the purge never got past the repo")
			}
		})
	}
}

// If closing the run fails mid-purge, the engines must still be standing:
// teardown is repo-side and precedes the scheduler, so its failure has to
// stop the purge before any pod is touched.
func TestPurge_TeardownFailureLeavesEnginesStanding(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 1)
	ctx := context.Background()
	repo := &failingRepo{Repo: e.store}
	sched := &callLogScheduler{Scheduler: e.sched}
	svc := wrappedEnv(t, e, repo, sched)
	if err := svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	repo.stopRunErr = errors.New("stop-run down")

	if err := svc.Purge(ctx, e.executionID); err == nil {
		t.Fatal("Purge with a failing StopRun: want error")
	}
	if calls := sched.snapshot(); len(calls) != 0 {
		t.Fatalf("scheduler saw %v, want nothing -- teardown failed before the purge", calls)
	}
}

// When the scheduler cannot purge, everything repo-side must already have
// happened: the run closed, and the pods' logs captured while they still
// existed -- the call log proves capture precedes deletion.
func TestPurge_SchedulerFailureStillRanRepoSideFirst(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2) // one scenario, two shards
	ctx := context.Background()
	sched := &callLogScheduler{Scheduler: e.sched, purgeErr: errors.New("cluster down")}
	svc := wrappedEnv(t, e, e.store, sched)
	if err := svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	if err := svc.Purge(ctx, e.executionID); err == nil {
		t.Fatal("Purge with a failing scheduler: want error")
	}
	if _, running, _ := e.store.CurrentRun(ctx, e.executionID); running {
		t.Fatal("run still open -- teardown must run before the scheduler purge")
	}
	calls := sched.snapshot()
	if len(calls) != 3 || calls[0] != "podlog" || calls[1] != "podlog" || calls[2] != "purge" {
		t.Fatalf("scheduler calls = %v, want both shard logs captured before the purge", calls)
	}
}

// Reconcile needs no metrics hook wired: with the default no-op one, a fully
// orphaned run is still closed and reported as success.
func TestReconcile_FinalizesOrphansEvenWithoutAMetricsHook(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := setup(t, false, 1) // no WithMetrics: the no-op hook is the default
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	orphanFor(t, e, e.planIDs[0], 0, 0)

	if err := e.svc.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile with the no-op metrics hook: %v", err)
	}
	if _, running, _ := e.store.CurrentRun(ctx, e.executionID); running {
		t.Fatal("Reconcile left the stranded run open")
	}
}

// A metrics hook that cannot write the orphan report fails the reconcile
// pass: the evidence is there and must not be silently dropped.
func TestReconcile_MetricsFailurePropagates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := setup(t, false, 1)
	e.svc.WithMetrics(&recordingMetrics{FinalizeErr: errors.New("report store down")})
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	orphanFor(t, e, e.planIDs[0], 0, 0)

	if err := e.svc.Reconcile(ctx); err == nil {
		t.Fatal("Reconcile with a failing FinalizeOrphaned: want error")
	}
}

// WithNow overrides the clock a quota window is measured from, so a fixed
// clock yields a reservation that starts exactly there -- and chaining
// returns the same service. WithNow(nil) keeps the real clock.
func TestWithNow_MeasuresTheReservationWindowFromTheInjectedClock(t *testing.T) {
	t.Parallel()
	const tenantID = int64(7)
	fixed := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	ctx := context.Background()

	t.Run("fixed clock", func(t *testing.T) {
		t.Parallel()
		e := setupWithTenant(t, tenantID, 2)
		if err := e.store.SetCeiling(ctx, tenantID, "", 5); err != nil {
			t.Fatalf("SetCeiling: %v", err)
		}
		if e.svc.WithNow(func() time.Time { return fixed }) != e.svc {
			t.Fatal("WithNow must return the same service for chaining")
		}
		if err := e.svc.Deploy(ctx, e.executionID); err != nil {
			t.Fatalf("Deploy: %v", err)
		}
		if err := e.svc.Trigger(ctx, e.executionID); err != nil {
			t.Fatalf("Trigger: %v", err)
		}

		got, err := e.store.ReservationsInWindow(ctx, tenantID, "", fixed.Add(-time.Hour), fixed.Add(time.Hour))
		if err != nil {
			t.Fatalf("ReservationsInWindow: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("reservations = %+v, want exactly one", got)
		}
		if !got[0].Start.Equal(fixed) {
			t.Errorf("reservation start = %v, want %v (the injected clock)", got[0].Start, fixed)
		}
		if want := fixed.Add(31 * time.Second); !got[0].End.Equal(want) {
			t.Errorf("reservation end = %v, want %v (start + 30s run + 1s ramp-up)", got[0].End, want)
		}
	})

	t.Run("nil keeps the default clock", func(t *testing.T) {
		t.Parallel()
		e := setupWithTenant(t, tenantID, 1)
		e.svc.WithNow(nil)
		if err := e.store.SetCeiling(ctx, tenantID, "", 5); err != nil {
			t.Fatalf("SetCeiling: %v", err)
		}
		if err := e.svc.Deploy(ctx, e.executionID); err != nil {
			t.Fatalf("Deploy: %v", err)
		}
		if err := e.svc.Trigger(ctx, e.executionID); err != nil {
			t.Fatalf("Trigger: %v", err)
		}

		now := time.Now()
		got, err := e.store.ReservationsInWindow(ctx, tenantID, "", now.Add(-time.Minute), now.Add(time.Hour))
		if err != nil {
			t.Fatalf("ReservationsInWindow: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("reservations = %+v, want one measured from the real clock", got)
		}
	})
}
