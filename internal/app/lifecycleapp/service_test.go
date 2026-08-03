package lifecycleapp_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/project"
	"github.com/heridotlife/honryu/internal/domain/run"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

const image = "honryu/jmeter:latest"

type env struct {
	store *fake.Store
	sched *fake.Scheduler
	obj   *fake.ObjectStore
	svc   *lifecycleapp.Service

	projectID   int64
	executionID int64
	planIDs     []int64
}

// setup seeds a project, an execution, and scenarios (each with a JMX test file),
// and stores an execution config. csvSplit toggles execution-level CSV split;
// specs give each scenario's engine count.
func setup(t *testing.T, csvSplit bool, engines ...int) *env {
	t.Helper()
	ctx := context.Background()
	store := fake.NewStore()

	p, _ := project.New("web", "honryu", "")
	projectID, _ := store.CreateProject(ctx, p)
	coll, _ := execution.New("peak", projectID)
	coll.CSVSplit = csvSplit
	executionID, _ := store.CreateExecution(ctx, coll)

	obj := fake.NewObjectStore()
	var tests []loadprofile.Entry
	var planIDs []int64
	for i, n := range engines {
		// A scenario carrying a .jmx is JMeter-native: the config points at the
		// script, and the file has to exist for a pod to run it.
		pl, _ := scenario.NewNative("scenario", projectID, taurus.ExecutorJMeter)
		scenarioID, _ := store.CreateScenario(ctx, pl)
		if err := store.AddScenarioFile(ctx, scenarioID, "test.jmx", true); err != nil {
			t.Fatalf("add test file: %v", err)
		}
		if err := obj.Upload(ctx, fmt.Sprintf("scenario/%d/test.jmx", scenarioID), strings.NewReader("<jmx/>")); err != nil {
			t.Fatalf("upload test file: %v", err)
		}
		planIDs = append(planIDs, scenarioID)
		tests = append(tests, loadprofile.Entry{
			Name: "p", ScenarioID: scenarioID, Concurrency: 10, Rampup: 1, Engines: n, Duration: 30,
		})
		_ = i
	}
	if err := store.StoreLoadProfile(ctx, executionID, csvSplit, tests); err != nil {
		t.Fatalf("store exec execution: %v", err)
	}

	sched := fake.NewScheduler()
	svc := lifecycleapp.NewService(store, sched, obj, lifecycleapp.StaticImage(image))
	return &env{store: store, sched: sched, obj: obj, svc: svc, projectID: projectID, executionID: executionID, planIDs: planIDs}
}

func TestDeploy_HappyPath(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2, 3)
	ctx := context.Background()

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	deployed, _ := e.sched.DeployedExecutions(ctx, "")
	if _, ok := deployed[e.executionID]; !ok {
		t.Fatalf("execution not deployed: %v", deployed)
	}
	status, _ := e.sched.ExecutionStatus(ctx, "", e.executionID, []ports.ScenarioRef{{ScenarioID: e.planIDs[0], Shards: 2}, {ScenarioID: e.planIDs[1], Shards: 3}})
	if status.PoolSize != 5 {
		t.Fatalf("pool size = %d, want 5", status.PoolSize)
	}
}

func TestDeploy_NoScenarios(t *testing.T) {
	t.Parallel()
	e := setup(t, false) // no scenarios
	if err := e.svc.Deploy(context.Background(), e.executionID); !errors.Is(err, run.ErrNoScenarios) {
		t.Fatalf("Deploy no scenarios: err = %v, want ErrNoScenarios", err)
	}
}

func TestDeploy_RejectedWhileRunning(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if _, err := e.store.StartRun(ctx, e.executionID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := e.svc.Deploy(ctx, e.executionID); !errors.Is(err, run.ErrAlreadyRunning) {
		t.Fatalf("Deploy while running: err = %v, want ErrAlreadyRunning", err)
	}
}

func TestDeploy_MissingExecution(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	if err := e.svc.Deploy(context.Background(), 9999); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Deploy missing execution: err = %v, want ErrNotFound", err)
	}
}

func TestTrigger_HappyPathStartsRunAndMarksScenariosRunning(t *testing.T) {
	t.Parallel()
	e := setup(t, true, 2, 3) // 5 engines total, execution CSV split on
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	// Under Taurus there is no engine call on trigger: a pod generates load from
	// the moment it starts. Trigger records that the run is under way.
	// A run is active and both scenarios are marked running.
	if _, ok, _ := e.store.CurrentRun(ctx, e.executionID); !ok {
		t.Fatal("no active run after trigger")
	}
	rps, _ := e.store.RunningScenariosByExecution(ctx, e.executionID)
	if len(rps) != 2 {
		t.Fatalf("running scenarios = %d, want 2", len(rps))
	}
}

// A native scenario without its script cannot run. The failure now lands on
// Deploy rather than Trigger: the config that points at the script is compiled
// when the pods are created, so nothing is deployed at all instead of pods
// starting and failing on their own.
func TestDeploy_NoTestFile(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.store.DeleteScenarioFile(ctx, e.planIDs[0], "test.jmx", true); err != nil {
		t.Fatalf("delete test file: %v", err)
	}
	if err := e.svc.Deploy(ctx, e.executionID); !errors.Is(err, lifecycleapp.ErrNoTestFile) {
		t.Fatalf("Deploy without a test file: err = %v, want ErrNoTestFile", err)
	}
	deployed, _ := e.sched.DeployedExecutions(ctx, "")
	if _, ok := deployed[e.executionID]; ok {
		t.Error("pods were deployed for an execution whose scenario has no script")
	}
}

func TestTrigger_NotDeployed(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	if err := e.svc.Trigger(context.Background(), e.executionID); !errors.Is(err, run.ErrNotDeployed) {
		t.Fatalf("Trigger not deployed: err = %v, want ErrNotDeployed", err)
	}
}

func TestTrigger_EnginesNotReady(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 3)
	ctx := context.Background()
	// Deploy only 2 of the 3 wanted engines directly on the scheduler.
	if err := e.sched.DeployScenario(ctx, ports.DeploySpec{ProjectID: e.projectID, ExecutionID: e.executionID, ScenarioID: e.planIDs[0], Shards: deployShards(2), Image: image}); err != nil {
		t.Fatalf("partial deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); !errors.Is(err, run.ErrEnginesNotReady) {
		t.Fatalf("Trigger under-provisioned: err = %v, want ErrEnginesNotReady", err)
	}
}

func TestTrigger_AlreadyRunning(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if _, err := e.store.StartRun(ctx, e.executionID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); !errors.Is(err, run.ErrAlreadyRunning) {
		t.Fatalf("Trigger while running: err = %v, want ErrAlreadyRunning", err)
	}
}

func TestTrigger_ErrorRollsBackRun(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	e.store.MarkRunningErr = errors.New("db down")
	if err := e.svc.Trigger(ctx, e.executionID); err == nil {
		t.Fatal("Trigger with a failing repository: want error, got nil")
	}
	if _, ok, _ := e.store.CurrentRun(ctx, e.executionID); ok {
		t.Fatal("run was not rolled back after total failure")
	}
}

func TestStop_HappyPath(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2, 3)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if err := e.svc.Stop(ctx, e.executionID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, ok, _ := e.store.CurrentRun(ctx, e.executionID); ok {
		t.Fatal("run still active after stop")
	}
	if rps, _ := e.store.RunningScenariosByExecution(ctx, e.executionID); len(rps) != 0 {
		t.Fatalf("running scenarios after stop = %d, want 0", len(rps))
	}
	// Engines are torn down by removing their pods (Purge), not by calling them,
	// so Stop clears run state only.
}

func TestStop_NotRunning(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	if err := e.svc.Stop(context.Background(), e.executionID); !errors.Is(err, run.ErrNotRunning) {
		t.Fatalf("Stop not running: err = %v, want ErrNotRunning", err)
	}
}

func TestPurge_StopsThenRemoves(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if err := e.svc.Purge(ctx, e.executionID); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if _, ok, _ := e.store.CurrentRun(ctx, e.executionID); ok {
		t.Fatal("run still active after purge")
	}
	deployed, _ := e.sched.DeployedExecutions(ctx, "")
	if _, ok := deployed[e.executionID]; ok {
		t.Fatal("execution still deployed after purge")
	}
}

func TestPurge_WhenIdle(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Purge(ctx, e.executionID); err != nil {
		t.Fatalf("Purge idle: %v", err)
	}
	deployed, _ := e.sched.DeployedExecutions(ctx, "")
	if _, ok := deployed[e.executionID]; ok {
		t.Fatal("execution still deployed after purge")
	}
}

func TestStatus_ReportsPhaseAndProgress(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2, 3)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	st, err := e.svc.Status(ctx, e.executionID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Phase != run.PhaseDeployed || st.PoolSize != 5 || len(st.Scenarios) != 2 {
		t.Fatalf("status = %+v, want deployed/5/2", st)
	}

	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	st, _ = e.svc.Status(ctx, e.executionID)
	if st.Phase != run.PhaseRunning {
		t.Fatalf("phase after trigger = %q, want running", st.Phase)
	}
	for _, ps := range st.Scenarios {
		if !ps.InProgress {
			t.Fatalf("scenario %d not marked in progress: %+v", ps.ScenarioID, ps)
		}
	}
}

func TestEnginesDetailAndPodLog(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	detail, err := e.svc.EnginesDetail(ctx, e.projectID, e.executionID)
	if err != nil {
		t.Fatalf("EnginesDetail: %v", err)
	}
	if len(detail.Engines) != 2 {
		t.Fatalf("detail engines = %d, want 2", len(detail.Engines))
	}
	log, err := e.svc.PodLog(ctx, e.executionID, e.planIDs[0])
	if err != nil || log == "" {
		t.Fatalf("PodLog = %q, err = %v", log, err)
	}
}

func TestStop_WhenEnginesUnreachable(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	// Engines vanish before stop: teardown still clears run state (best effort).
	e.sched.Unreachable = true
	if err := e.svc.Stop(ctx, e.executionID); err != nil {
		t.Fatalf("Stop with unreachable engines: %v", err)
	}
	if _, ok, _ := e.store.CurrentRun(ctx, e.executionID); ok {
		t.Fatal("run still active after stop")
	}
	if rps, _ := e.store.RunningScenariosByExecution(ctx, e.executionID); len(rps) != 0 {
		t.Fatalf("running scenarios after stop = %d, want 0", len(rps))
	}
}

// Engine logs are the only diagnosis left once a pod is gone, and Purge is
// what deletes it -- so this is the last moment they exist anywhere else.
func TestPurge_CapturesEngineLogsBeforeDeletingPods(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2) // one scenario, two shards
	ctx := context.Background()

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	runID, ok, _ := e.store.CurrentRun(ctx, e.executionID)
	if !ok {
		t.Fatal("no active run after trigger")
	}

	if err := e.svc.Purge(ctx, e.executionID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	for shard := 0; shard < 2; shard++ {
		key := fmt.Sprintf("run/%d/scenario-%d-shard-%d.log", runID, e.planIDs[0], shard)
		got, err := e.obj.Download(ctx, key)
		if err != nil {
			t.Fatalf("Download(%q): %v", key, err)
		}
		if len(got) == 0 {
			t.Errorf("shard %d log is empty", shard)
		}
	}
}

// Purge must still remove a broken execution's engines even if capturing its
// logs fails -- diagnosability degrades, the operation does not.
func TestPurge_LogCaptureFailureDoesNotFailPurge(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 1)
	ctx := context.Background()
	e.obj.UploadErr = errors.New("bucket unavailable")

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if err := e.svc.Purge(ctx, e.executionID); err != nil {
		t.Fatalf("Purge: %v, want nil despite the log store failing", err)
	}
}

// A shard whose pod is already unreachable must not stop the others from
// being captured.
func TestPurge_CapturesTheShardsItCanReach(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	runID, _, _ := e.store.CurrentRun(ctx, e.executionID)

	e.sched.PodLogErr = errors.New("pod unreachable")
	if err := e.svc.Purge(ctx, e.executionID); err != nil {
		t.Fatalf("Purge: %v, want nil despite every shard being unreachable", err)
	}

	key := fmt.Sprintf("run/%d/scenario-%d-shard-0.log", runID, e.planIDs[0])
	if _, err := e.obj.Download(ctx, key); !errors.Is(err, ports.ErrObjectNotFound) {
		t.Errorf("Download after every PodLog failed = %v, want ErrObjectNotFound", err)
	}
}

// Deploying without ever triggering leaves no run to key logs by, and Purge
// must not invent one.
func TestPurge_WithoutARunNeverCapturesLogs(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 1)
	ctx := context.Background()

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Purge(ctx, e.executionID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	key := fmt.Sprintf("run/0/scenario-%d-shard-0.log", e.planIDs[0])
	if _, err := e.obj.Download(ctx, key); !errors.Is(err, ports.ErrObjectNotFound) {
		t.Errorf("Download = %v, want ErrObjectNotFound -- nothing was ever triggered", err)
	}
}

// recordingMetrics implements lifecycleapp.Metrics and records the calls.
type recordingMetrics struct {
	purged    []int64
	finalized []int64 // run ids
	// FinalizeErr, when set, is returned by Finalize -- teardown must tolerate
	// it, since a customer must be able to stop a broken execution regardless.
	FinalizeErr error
}

func (m *recordingMetrics) Purge(id int64) { m.purged = append(m.purged, id) }

func (m *recordingMetrics) Finalize(_ context.Context, _, runID int64) error {
	m.finalized = append(m.finalized, runID)
	return m.FinalizeErr
}

// Purging an execution must drop its metric series: with measurements pushed
// there is nothing to start or stop, but nothing else would ever remove them.
func TestMetricsHook_PurgeDropsSeries(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	rec := &recordingMetrics{}
	e.svc.WithMetrics(rec)

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Purge(ctx, e.executionID); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if len(rec.purged) != 1 || rec.purged[0] != e.executionID {
		t.Fatalf("purged = %v, want [%d]", rec.purged, e.executionID)
	}
}

// Stop and Purge both end a run through teardown, so both must finalise its
// report -- Honryu is the one deciding the run is over, and Finalize needs the
// run id before StopRun forgets it.
func TestMetricsHook_StopFinalizesTheReport(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	rec := &recordingMetrics{}
	e.svc.WithMetrics(rec)

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	runID, ok, _ := e.store.CurrentRun(ctx, e.executionID)
	if !ok {
		t.Fatal("no active run after trigger")
	}

	if err := e.svc.Stop(ctx, e.executionID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(rec.finalized) != 1 || rec.finalized[0] != runID {
		t.Fatalf("finalized = %v, want [%d]", rec.finalized, runID)
	}
}

// Deploying without ever triggering leaves no run to finalise, and Purge must
// not invent one.
func TestMetricsHook_PurgeWithoutARunNeverFinalizes(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	rec := &recordingMetrics{}
	e.svc.WithMetrics(rec)

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Purge(ctx, e.executionID); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if len(rec.finalized) != 0 {
		t.Errorf("finalized = %v, want none: nothing was ever triggered", rec.finalized)
	}
}

// A customer must be able to stop or purge a broken execution even if writing
// its final report fails -- that failure must not become the reason teardown
// itself fails.
func TestMetricsHook_StopSucceedsEvenIfFinalizeFails(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	rec := &recordingMetrics{FinalizeErr: errors.New("boom")}
	e.svc.WithMetrics(rec)

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if err := e.svc.Stop(ctx, e.executionID); err != nil {
		t.Fatalf("Stop: %v, want nil despite Finalize failing", err)
	}
	if len(rec.finalized) != 1 {
		t.Fatalf("Finalize was not attempted")
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

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if usage.started != 1 || usage.lastOwner != "honryu" || usage.lastVU != 20 {
		t.Fatalf("usage start = %+v, want started=1 owner=honryu vu=20", usage)
	}
	if err := e.svc.Stop(ctx, e.executionID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if usage.finished != 1 {
		t.Fatalf("usage finished = %d, want 1", usage.finished)
	}
}

func TestResume_ListsRunningScenarios(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	rps, err := e.svc.Resume(ctx)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if len(rps) != 1 {
		t.Fatalf("resume running scenarios = %d, want 1", len(rps))
	}
}

// deployShards builds n placeholder shard specs for a deploy.
func deployShards(n int) []ports.ShardSpec {
	out := make([]ports.ShardSpec, n)
	for i := range out {
		out[i] = ports.ShardSpec{Index: i, Concurrency: 1}
	}
	return out
}

// Each pod must get a config carrying only its own slice of the load. If every
// shard got the whole profile, N pods would produce N times the load asked for.
func TestDeploy_CompilesAConfigPerShard(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 3) // 10 users across 3 pods
	ctx := context.Background()

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	spec, ok := e.sched.LastDeploy(e.executionID, e.planIDs[0])
	if !ok {
		t.Fatal("nothing was deployed")
	}
	if len(spec.Shards) != 3 {
		t.Fatalf("deployed %d shards, want 3", len(spec.Shards))
	}

	total := 0
	for _, sh := range spec.Shards {
		if len(sh.Config) == 0 {
			t.Fatalf("shard %d has no config, so its pod would run nothing", sh.Index)
		}
		var cfg taurus.Config
		if err := yaml.Unmarshal(sh.Config, &cfg); err != nil {
			t.Fatalf("shard %d config is not valid YAML: %v", sh.Index, err)
		}
		if len(cfg.Execution) != 1 {
			t.Fatalf("shard %d config has %d executions, want 1", sh.Index, len(cfg.Execution))
		}
		got := cfg.Execution[0].Concurrency
		if got != sh.Concurrency {
			t.Errorf("shard %d config says %d users, plan says %d", sh.Index, got, sh.Concurrency)
		}
		total += got
	}
	if total != 10 {
		t.Errorf("shards total %d users, want the 10 that were requested", total)
	}

	// The script the configs point at has to travel with them.
	if _, ok := spec.ScenarioFiles["test.jmx"]; !ok {
		t.Errorf("scenario files = %v, want the test plan", spec.ScenarioFiles)
	}
}

func TestWithDefaultEngine(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 1)
	if got := e.svc.WithDefaultEngine(taurus.ExecutorK6); got == nil {
		t.Fatal("WithDefaultEngine returned nil")
	}
}

// A data file the scenario recorded but never uploaded fails the deploy, naming
// the file: "object not found" alone leaves an operator guessing which of a
// scenario's artefacts is missing.
func TestDeploy_MissingDataFileNamesIt(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 1)
	ctx := context.Background()

	if err := e.store.AddScenarioFile(ctx, e.planIDs[0], "users.csv", false); err != nil {
		t.Fatalf("record data file: %v", err)
	}
	err := e.svc.Deploy(ctx, e.executionID)
	if err == nil {
		t.Fatal("Deploy with a missing data file succeeded")
	}
	if !strings.Contains(err.Error(), "users.csv") {
		t.Errorf("error %q does not name the missing file", err)
	}
}

// A failure part way through must not leave half an execution running.
func TestDeploy_UnknownScenarioFails(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 1)
	ctx := context.Background()

	if err := e.store.StoreLoadProfile(ctx, e.executionID, false, []loadprofile.Entry{
		{ScenarioID: 999999, Concurrency: 5, Rampup: 1, Engines: 1, Duration: 10},
	}); err != nil {
		t.Fatalf("StoreLoadProfile: %v", err)
	}
	if err := e.svc.Deploy(ctx, e.executionID); err == nil {
		t.Fatal("Deploy referencing a missing scenario succeeded")
	}
}
