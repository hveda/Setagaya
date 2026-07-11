package lifecycleapp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/heridotlife/Setagaya/internal/app/lifecycleapp"
	"github.com/heridotlife/Setagaya/internal/domain/collection"
	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/domain/plan"
	"github.com/heridotlife/Setagaya/internal/domain/project"
	"github.com/heridotlife/Setagaya/internal/domain/run"
	"github.com/heridotlife/Setagaya/internal/ports"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
)

const image = "setagaya/jmeter:latest"

type env struct {
	store *fake.Store
	sched *fake.Scheduler
	exec  *fake.Executor
	svc   *lifecycleapp.Service

	projectID    int64
	collectionID int64
	planIDs      []int64
}

// setup seeds a project, a collection, and plans (each with a JMX test file),
// and stores an execution config. csvSplit toggles collection-level CSV split;
// specs give each plan's engine count.
func setup(t *testing.T, csvSplit bool, engines ...int) *env {
	t.Helper()
	ctx := context.Background()
	store := fake.NewStore()

	p, _ := project.New("web", "setagaya", "")
	projectID, _ := store.CreateProject(ctx, p)
	coll, _ := collection.New("peak", projectID)
	coll.CSVSplit = csvSplit
	collectionID, _ := store.CreateCollection(ctx, coll)

	var tests []execution.ExecutionPlan
	var planIDs []int64
	for i, n := range engines {
		pl, _ := plan.New("plan", projectID)
		planID, _ := store.CreatePlan(ctx, pl)
		if err := store.AddPlanFile(ctx, planID, "test.jmx", true); err != nil {
			t.Fatalf("add test file: %v", err)
		}
		planIDs = append(planIDs, planID)
		tests = append(tests, execution.ExecutionPlan{
			Name: "p", PlanID: planID, Concurrency: 10, Rampup: 1, Engines: n, Duration: 30,
		})
		_ = i
	}
	if err := store.StoreExecutionCollection(ctx, collectionID, csvSplit, tests); err != nil {
		t.Fatalf("store exec collection: %v", err)
	}

	sched := fake.NewScheduler()
	exec := fake.NewExecutor()
	svc := lifecycleapp.NewService(store, sched, exec, fake.NewObjectStore(), image)
	return &env{store: store, sched: sched, exec: exec, svc: svc, projectID: projectID, collectionID: collectionID, planIDs: planIDs}
}

func TestDeploy_HappyPath(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2, 3)
	ctx := context.Background()

	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	deployed, _ := e.sched.DeployedCollections(ctx)
	if _, ok := deployed[e.collectionID]; !ok {
		t.Fatalf("collection not deployed: %v", deployed)
	}
	status, _ := e.sched.CollectionStatus(ctx, e.collectionID, []ports.PlanRef{{PlanID: e.planIDs[0], Engines: 2}, {PlanID: e.planIDs[1], Engines: 3}})
	if status.PoolSize != 5 {
		t.Fatalf("pool size = %d, want 5", status.PoolSize)
	}
}

func TestDeploy_NoPlans(t *testing.T) {
	t.Parallel()
	e := setup(t, false) // no plans
	if err := e.svc.Deploy(context.Background(), e.collectionID); !errors.Is(err, run.ErrNoPlans) {
		t.Fatalf("Deploy no plans: err = %v, want ErrNoPlans", err)
	}
}

func TestDeploy_RejectedWhileRunning(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if _, err := e.store.StartRun(ctx, e.collectionID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := e.svc.Deploy(ctx, e.collectionID); !errors.Is(err, run.ErrAlreadyRunning) {
		t.Fatalf("Deploy while running: err = %v, want ErrAlreadyRunning", err)
	}
}

func TestDeploy_MissingCollection(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	if err := e.svc.Deploy(context.Background(), 9999); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Deploy missing collection: err = %v, want ErrNotFound", err)
	}
}

func TestTrigger_HappyPathMarksRunningAndConfigures(t *testing.T) {
	t.Parallel()
	e := setup(t, true, 2, 3) // 5 engines total, collection CSV split on
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.collectionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if got := e.exec.TriggerCount(); got != 5 {
		t.Fatalf("triggered engines = %d, want 5", got)
	}
	// A run is active and both plans are marked running.
	if _, ok, _ := e.store.CurrentRun(ctx, e.collectionID); !ok {
		t.Fatal("no active run after trigger")
	}
	rps, _ := e.store.RunningPlansByCollection(ctx, e.collectionID)
	if len(rps) != 2 {
		t.Fatalf("running plans = %d, want 2", len(rps))
	}
	// The config sent to plan 0 engine 0 carries the run id and plan duration.
	url0, _ := e.sched.EngineURLs(ctx, e.collectionID, e.planIDs[0], 2)
	cfg, ok := e.exec.TriggeredConfig(url0[0])
	if !ok {
		t.Fatal("engine 0 was not triggered")
	}
	if cfg.RunID == 0 || cfg.Duration != "30" || cfg.Concurrency != "10" {
		t.Fatalf("config = %+v", cfg)
	}
	if _, hasTest := cfg.Data["test.jmx"]; !hasTest {
		t.Fatalf("config missing test file: %+v", cfg.Data)
	}
}

func TestTrigger_NoTestFile(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.store.DeletePlanFile(ctx, e.planIDs[0], "test.jmx", true); err != nil {
		t.Fatalf("delete test file: %v", err)
	}
	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.collectionID); !errors.Is(err, lifecycleapp.ErrNoTestFile) {
		t.Fatalf("Trigger without test file: err = %v, want ErrNoTestFile", err)
	}
}

func TestTrigger_NotDeployed(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	if err := e.svc.Trigger(context.Background(), e.collectionID); !errors.Is(err, run.ErrNotDeployed) {
		t.Fatalf("Trigger not deployed: err = %v, want ErrNotDeployed", err)
	}
}

func TestTrigger_EnginesNotReady(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 3)
	ctx := context.Background()
	// Deploy only 2 of the 3 wanted engines directly on the scheduler.
	if err := e.sched.DeployPlan(ctx, ports.DeploySpec{ProjectID: e.projectID, CollectionID: e.collectionID, PlanID: e.planIDs[0], Engines: 2, Image: image}); err != nil {
		t.Fatalf("partial deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.collectionID); !errors.Is(err, run.ErrEnginesNotReady) {
		t.Fatalf("Trigger under-provisioned: err = %v, want ErrEnginesNotReady", err)
	}
}

func TestTrigger_AlreadyRunning(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if _, err := e.store.StartRun(ctx, e.collectionID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.collectionID); !errors.Is(err, run.ErrAlreadyRunning) {
		t.Fatalf("Trigger while running: err = %v, want ErrAlreadyRunning", err)
	}
}

func TestTrigger_ErrorRollsBackRun(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	e.exec.TriggerErr = errors.New("agent down")
	if err := e.svc.Trigger(ctx, e.collectionID); err == nil {
		t.Fatal("Trigger with failing executor: want error, got nil")
	}
	if _, ok, _ := e.store.CurrentRun(ctx, e.collectionID); ok {
		t.Fatal("run was not rolled back after total failure")
	}
}

func TestStop_HappyPath(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2, 3)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.collectionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if err := e.svc.Stop(ctx, e.collectionID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, ok, _ := e.store.CurrentRun(ctx, e.collectionID); ok {
		t.Fatal("run still active after stop")
	}
	if rps, _ := e.store.RunningPlansByCollection(ctx, e.collectionID); len(rps) != 0 {
		t.Fatalf("running plans after stop = %d, want 0", len(rps))
	}
	if e.exec.StopCount() != 5 {
		t.Fatalf("stopped engines = %d, want 5", e.exec.StopCount())
	}
}

func TestStop_NotRunning(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	if err := e.svc.Stop(context.Background(), e.collectionID); !errors.Is(err, run.ErrNotRunning) {
		t.Fatalf("Stop not running: err = %v, want ErrNotRunning", err)
	}
}

func TestPurge_StopsThenRemoves(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.collectionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if err := e.svc.Purge(ctx, e.collectionID); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if _, ok, _ := e.store.CurrentRun(ctx, e.collectionID); ok {
		t.Fatal("run still active after purge")
	}
	deployed, _ := e.sched.DeployedCollections(ctx)
	if _, ok := deployed[e.collectionID]; ok {
		t.Fatal("collection still deployed after purge")
	}
}

func TestPurge_WhenIdle(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Purge(ctx, e.collectionID); err != nil {
		t.Fatalf("Purge idle: %v", err)
	}
	deployed, _ := e.sched.DeployedCollections(ctx)
	if _, ok := deployed[e.collectionID]; ok {
		t.Fatal("collection still deployed after purge")
	}
}

func TestStatus_ReportsPhaseAndProgress(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2, 3)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	st, err := e.svc.Status(ctx, e.collectionID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Phase != run.PhaseDeployed || st.PoolSize != 5 || len(st.Plans) != 2 {
		t.Fatalf("status = %+v, want deployed/5/2", st)
	}

	if err := e.svc.Trigger(ctx, e.collectionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	st, _ = e.svc.Status(ctx, e.collectionID)
	if st.Phase != run.PhaseRunning {
		t.Fatalf("phase after trigger = %q, want running", st.Phase)
	}
	for _, ps := range st.Plans {
		if !ps.InProgress {
			t.Fatalf("plan %d not marked in progress: %+v", ps.PlanID, ps)
		}
	}
}

func TestEnginesDetailAndPodLog(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	detail, err := e.svc.EnginesDetail(ctx, e.projectID, e.collectionID)
	if err != nil {
		t.Fatalf("EnginesDetail: %v", err)
	}
	if len(detail.Engines) != 2 {
		t.Fatalf("detail engines = %d, want 2", len(detail.Engines))
	}
	log, err := e.svc.PodLog(ctx, e.collectionID, e.planIDs[0])
	if err != nil || log == "" {
		t.Fatalf("PodLog = %q, err = %v", log, err)
	}
}

func TestTrigger_UnreachableRollsBack(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	// Engines are counted deployed but routing is down: trigger fails per plan.
	e.sched.Unreachable = true
	if err := e.svc.Trigger(ctx, e.collectionID); err == nil {
		t.Fatal("Trigger with unreachable engines: want error, got nil")
	}
	if _, ok, _ := e.store.CurrentRun(ctx, e.collectionID); ok {
		t.Fatal("run was not rolled back after unreachable failure")
	}
}

func TestStop_WhenEnginesUnreachable(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.collectionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	// Engines vanish before stop: teardown still clears run state (best effort).
	e.sched.Unreachable = true
	if err := e.svc.Stop(ctx, e.collectionID); err != nil {
		t.Fatalf("Stop with unreachable engines: %v", err)
	}
	if _, ok, _ := e.store.CurrentRun(ctx, e.collectionID); ok {
		t.Fatal("run still active after stop")
	}
	if rps, _ := e.store.RunningPlansByCollection(ctx, e.collectionID); len(rps) != 0 {
		t.Fatalf("running plans after stop = %d, want 0", len(rps))
	}
}

// recordingMetrics implements lifecycleapp.Metrics and records the calls.
type recordingMetrics struct {
	started, stopped, purged []int64
}

func (m *recordingMetrics) Start(id int64) { m.started = append(m.started, id) }
func (m *recordingMetrics) Stop(id int64)  { m.stopped = append(m.stopped, id) }
func (m *recordingMetrics) Purge(id int64) { m.purged = append(m.purged, id) }

func TestMetricsHooks_FireOnTriggerStopPurge(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	rec := &recordingMetrics{}
	e.svc.WithMetrics(rec)

	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.collectionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if len(rec.started) != 1 || rec.started[0] != e.collectionID {
		t.Fatalf("started = %v, want [%d]", rec.started, e.collectionID)
	}
	if err := e.svc.Stop(ctx, e.collectionID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(rec.stopped) == 0 {
		t.Fatal("Stop did not stop metrics")
	}
	if err := e.svc.Purge(ctx, e.collectionID); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if len(rec.purged) != 1 || rec.purged[0] != e.collectionID {
		t.Fatalf("purged = %v, want [%d]", rec.purged, e.collectionID)
	}
}

// recordingUsage implements lifecycleapp.Usage.
type recordingUsage struct {
	started, finished int
	lastOwner         string
	lastVU            int
}

func (u *recordingUsage) RecordStart(_ context.Context, _ int64, owner string, _, vu int) error {
	u.started++
	u.lastOwner = owner
	u.lastVU = vu
	return nil
}

func (u *recordingUsage) RecordFinish(_ context.Context, _ int64, _ int) error {
	u.finished++
	return nil
}

func TestUsageHooks_FireOnTriggerAndTeardown(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2) // 2 engines, concurrency 10 -> VU 20
	ctx := context.Background()
	usage := &recordingUsage{}
	e.svc.WithUsage(usage)

	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.collectionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if usage.started != 1 || usage.lastOwner != "setagaya" || usage.lastVU != 20 {
		t.Fatalf("usage start = %+v, want started=1 owner=setagaya vu=20", usage)
	}
	if err := e.svc.Stop(ctx, e.collectionID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if usage.finished != 1 {
		t.Fatalf("usage finished = %d, want 1", usage.finished)
	}
}

func TestResume_ListsRunningPlans(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.collectionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.collectionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	rps, err := e.svc.Resume(ctx)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if len(rps) != 1 {
		t.Fatalf("resume running plans = %d, want 1", len(rps))
	}
}
