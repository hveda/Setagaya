package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/app/campaignapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/quotaapp"
	"github.com/heridotlife/honryu/internal/app/scheduleapp"
	"github.com/heridotlife/honryu/internal/config"
	"github.com/heridotlife/honryu/internal/domain/campaign"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/project"
	"github.com/heridotlife/honryu/internal/domain/reservation"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/domain/schedule"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func TestNewRepository_Fake(t *testing.T) {
	t.Parallel()
	repo, err := newRepository(config.DBConfig{Driver: "fake"}, "default")
	if err != nil || repo == nil {
		t.Fatalf("newRepository(fake) = %v, %v", repo, err)
	}
}

func TestNewRepository_Unsupported(t *testing.T) {
	t.Parallel()
	if _, err := newRepository(config.DBConfig{Driver: "postgres"}, "default"); err == nil {
		t.Fatal("newRepository(postgres): expected error, got nil")
	}
}

func TestNewRepository_MySQL_Unreachable(t *testing.T) {
	t.Parallel()
	// Nothing listens on port 1, so the ping fails fast: covers the mysql
	// open-ok / ping-error wiring branch without needing a container.
	_, err := newRepository(config.DBConfig{
		Driver: "mysql",
		DSN:    "honryu:secret@tcp(127.0.0.1:1)/honryu?parseTime=true",
	}, "default")
	if err == nil {
		t.Fatal("newRepository(mysql, unreachable): expected error, got nil")
	}
}

func TestNewScheduler(t *testing.T) {
	t.Parallel()
	if s, err := newScheduler(config.ClusterConfig{Scheduler: "fake"}); err != nil || s == nil {
		t.Fatalf("newScheduler(fake) = %v, %v", s, err)
	}
	if _, err := newScheduler(config.ClusterConfig{Scheduler: "k8s", Namespace: "default", EnginePort: 8080}); err == nil {
		t.Fatal("newScheduler(k8s) outside cluster: expected error, got nil")
	}
	if _, err := newScheduler(config.ClusterConfig{Scheduler: "nope"}); err == nil {
		t.Fatal("newScheduler(nope): expected error, got nil")
	}
}

func TestNewObjectStore(t *testing.T) {
	t.Parallel()
	if s, err := newObjectStore(config.StorageConfig{Driver: "local", Root: t.TempDir()}); err != nil || s == nil {
		t.Fatalf("newObjectStore(local) = %v, %v", s, err)
	}
	s, err := newObjectStore(config.StorageConfig{
		Driver: "nexus", BaseURL: "https://nexus.example", Repo: "raw", Username: "u", Password: "p",
	})
	if err != nil || s == nil {
		t.Fatalf("newObjectStore(nexus) = %v, %v", s, err)
	}
	if got := s.URL("scenario/1/a.jmx"); got != "https://nexus.example/repository/raw/scenario/1/a.jmx" {
		t.Fatalf("nexus URL = %q", got)
	}
	if _, err := newObjectStore(config.StorageConfig{Driver: "s3"}); err == nil {
		t.Fatal("newObjectStore(s3): expected error, got nil")
	}
}

func TestSetupLogging_AllVariants(t *testing.T) {
	// Not parallel: mutates the global slog default logger.
	for _, c := range []config.LogConfig{
		{Level: "debug", Format: "text"},
		{Level: "info", Format: "json"},
		{Level: "warn", Format: "json"},
		{Level: "error", Format: "text"},
		{Level: "unknown", Format: "unknown"},
	} {
		setupLogging(c)
	}
}

func TestRun_ConfigError(t *testing.T) {
	t.Parallel()
	env := map[string]string{"HONRYU_HTTP_PORT": "not-a-number"}
	if err := run(context.Background(), func(k string) string { return env[k] }); err == nil {
		t.Fatal("run with invalid config: expected error, got nil")
	}
}

// A valid config that fails at wiring (rather than at Load/validate) --
// here, a well-formed mysql driver selection with an unreachable DSN --
// covers run()'s own newRepository error-return branch.
func TestRun_NewRepositoryError(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"HONRYU_DB_DRIVER": "mysql",
		"HONRYU_DB_DSN":    "honryu:secret@tcp(127.0.0.1:1)/honryu?parseTime=true",
	}
	if err := run(context.Background(), func(k string) string { return env[k] }); err == nil {
		t.Fatal("run with an unreachable mysql DSN: expected error, got nil")
	}
}

func TestRun_TicksUntilContextCancelled(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"HONRYU_DB_DRIVER":               "fake",
		"HONRYU_LOG_FORMAT":              "text",
		"HONRYU_SCHEDULER_TICK_INTERVAL": "10ms",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- run(ctx, func(k string) string { return env[k] }) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return within 5s of its context being cancelled")
	}
}

// seedDueExecution creates a project, scenario (with a test file), execution,
// and load profile via the fake store, plus a tenant quota ceiling, and
// returns the execution id -- everything fireOnce's Deploy+Trigger call
// needs to actually succeed.
func seedDueExecution(t *testing.T, store *fake.Store, obj *fake.ObjectStore, tenantID int64, engines int) int64 {
	t.Helper()
	ctx := context.Background()
	p, _ := project.New("web", "honryu", "")
	projectID, err := store.CreateProject(ctx, p)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	pl, _ := scenario.NewNative("scenario", projectID, taurus.ExecutorJMeter)
	scenarioID, err := store.CreateScenario(ctx, pl)
	if err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}
	if err := store.AddScenarioFile(ctx, scenarioID, "test.jmx", true); err != nil {
		t.Fatalf("AddScenarioFile: %v", err)
	}
	if err := obj.Upload(ctx, fmt.Sprintf("scenario/%d/test.jmx", scenarioID), strings.NewReader("<jmx/>")); err != nil {
		t.Fatalf("upload test file: %v", err)
	}

	e, _ := execution.New("peak", projectID)
	e.TenantID = &tenantID
	executionID, err := store.CreateExecution(ctx, e)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	entries := []loadprofile.Entry{{ScenarioID: scenarioID, Concurrency: 10, Engines: engines, Duration: 30}}
	if err := store.StoreLoadProfile(ctx, executionID, false, entries); err != nil {
		t.Fatalf("StoreLoadProfile: %v", err)
	}
	if err := store.SetCeiling(ctx, tenantID, "", engines); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	return executionID
}

// newTestServices wires scheduleapp/lifecycleapp against fakes. now pins the
// clock scheduleapp.Create measures its admission horizon from -- a fire
// time equal to now is included in that horizon (the boundary is inclusive),
// and since real wall-clock time only advances from here, by the time
// fireOnce runs (which claims against the real clock) that same fire time is
// already due.
func newTestServices(store *fake.Store, obj *fake.ObjectStore, now time.Time) (*scheduleapp.Service, *lifecycleapp.Service, *fake.Scheduler) {
	sched := fake.NewScheduler()
	quota := quotaapp.NewService(store).WithNow(func() time.Time { return now })
	lifecycle := lifecycleapp.NewService(store, sched, obj, lifecycleapp.StaticImage("honryu/jmeter:latest")).WithQuota(quota)
	// Matches run()'s own wiring order exactly: retrofitted after lifecycle
	// exists, since lifecycle itself depends on quota.
	quota.WithStopper(lifecycle)
	schedules := scheduleapp.NewService(store, quota).WithNow(func() time.Time { return now })
	return schedules, lifecycle, sched
}

func TestFireOnce_NothingDueIsANoOp(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	schedules, lifecycle, sched := newTestServices(store, fake.NewObjectStore(), time.Now())

	fireOnce(context.Background(), schedules, lifecycle)

	deployed, _ := sched.DeployedExecutions(context.Background(), "")
	if len(deployed) != 0 {
		t.Fatalf("deployed executions = %v, want none -- nothing was due", deployed)
	}
}

// The end-to-end path fireOnce exists for: claim, deploy, trigger, and the
// occurrence's hold reservation released in favor of Trigger's own live one.
func TestFireOnce_DeploysAndTriggersTheDueExecution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	const tenantID = int64(7)
	executionID := seedDueExecution(t, store, obj, tenantID, 2)
	now := time.Now()
	schedules, lifecycle, sched := newTestServices(store, obj, now)

	fireAt := now // included in the horizon, and already due by the time fireOnce runs
	view, err := schedules.Create(ctx, schedule.Schedule{
		ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true,
	})
	if err != nil {
		t.Fatalf("Create schedule: %v", err)
	}
	if view.Occurrences[0].Status != "reserved" {
		t.Fatalf("occurrence status = %v, want reserved", view.Occurrences[0].Status)
	}

	fireOnce(ctx, schedules, lifecycle)

	deployed, _ := sched.DeployedExecutions(ctx, "")
	if _, ok := deployed[executionID]; !ok {
		t.Fatalf("execution %d was not deployed", executionID)
	}
	if _, running, _ := store.CurrentRun(ctx, executionID); !running {
		t.Fatal("execution was not triggered -- no active run after fireOnce")
	}

	occs, err := store.OccurrencesForSchedule(ctx, view.Schedule.ID)
	if err != nil {
		t.Fatalf("OccurrencesForSchedule: %v", err)
	}
	if len(occs) != 1 || occs[0].Status != "fired" {
		t.Fatalf("occurrence after fireOnce = %+v, want status fired", occs)
	}

	// A second tick must not fire it again: the claim already consumed it.
	before, _ := sched.DeployedExecutions(ctx, "")
	fireOnce(ctx, schedules, lifecycle)
	after, _ := sched.DeployedExecutions(ctx, "")
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatalf("a second fireOnce changed deployed executions: before=%v after=%v", before, after)
	}
}

// Firing re-checks quota at the moment it actually happens (spec: "re-checked
// again when that time actually arrives, catching drift from other activity
// in between"): if capacity the occurrence held at creation time is gone by
// fire time, Deploy still succeeds (it does not check quota) but Trigger's
// own live Reserve call rejects, and fireOnce must log that and return
// without panicking rather than leaving the execution half-started.
func TestFireOnce_TriggerReRejectsWhenCapacityDrainedSinceCreation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	const tenantID = int64(7)
	executionID := seedDueExecution(t, store, obj, tenantID, 2) // ceiling 2, wants 2
	now := time.Now()
	schedules, lifecycle, sched := newTestServices(store, obj, now)

	fireAt := now
	if _, err := schedules.Create(ctx, schedule.Schedule{
		ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true,
	}); err != nil {
		t.Fatalf("Create schedule: %v", err)
	}
	// Capacity dries up between schedule creation and its fire time.
	if err := store.SetCeiling(ctx, tenantID, "", 0); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}

	fireOnce(ctx, schedules, lifecycle) // must not panic

	deployed, _ := sched.DeployedExecutions(ctx, "")
	if _, ok := deployed[executionID]; !ok {
		t.Fatal("execution was not deployed -- Deploy does not check quota and must still have succeeded")
	}
	if _, running, _ := store.CurrentRun(ctx, executionID); running {
		t.Fatal("execution shows running despite Trigger's own quota re-check rejecting it")
	}
}

// End to end: a tenant's earlier execution has overrun its declared duration
// (reservation end passed, run never stopped) and is the only thing standing
// between a newly due occurrence and admission. fireOnce's call into
// lifecycle.Trigger reclaims that capacity (quota.WithStopper(lifecycle),
// wired exactly as run() wires it) by stopping the overrunning execution,
// then admits and fires the due one.
func TestFireOnce_ReclaimsOverrunCapacityFromTheSameTenantToFireADueOccurrence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	const tenantID = int64(7)
	now := time.Now()

	overrunExecutionID := seedDueExecution(t, store, obj, tenantID, 2)
	if err := store.SetCeiling(ctx, tenantID, "", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	if _, err := store.CreateReservation(ctx, reservation.Reservation{
		TenantID: tenantID, Cluster: "", EngineCount: 2,
		Start: now.Add(-2 * time.Hour), End: now.Add(-time.Hour), ExecutionID: overrunExecutionID,
	}); err != nil {
		t.Fatalf("CreateReservation (overrun): %v", err)
	}
	if _, err := store.StartRun(ctx, overrunExecutionID); err != nil {
		t.Fatalf("StartRun (overrun): %v", err)
	}

	dueExecutionID := seedDueExecution(t, store, obj, tenantID, 2)
	schedules, lifecycle, sched := newTestServices(store, obj, now)
	fireAt := now
	if _, err := schedules.Create(ctx, schedule.Schedule{
		ExecutionID: dueExecutionID, TenantID: tenantID, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true,
	}); err != nil {
		t.Fatalf("Create schedule: %v", err)
	}

	fireOnce(ctx, schedules, lifecycle)

	deployed, _ := sched.DeployedExecutions(ctx, "")
	if _, ok := deployed[dueExecutionID]; !ok {
		t.Fatal("the due execution was not deployed")
	}
	if _, running, _ := store.CurrentRun(ctx, dueExecutionID); !running {
		t.Fatal("the due execution was not triggered -- reclaim should have freed enough capacity to admit it")
	}
	if _, running, _ := store.CurrentRun(ctx, overrunExecutionID); running {
		t.Fatal("the overrunning execution is still running -- it should have been reclaimed (stopped)")
	}
}

// A deploy/trigger failure (here: no test file, so Deploy fails ErrNoTestFile)
// must not panic or block the loop -- it is logged and the tick moves on.
func TestFireOnce_DeployFailureDoesNotPanic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	tenantID := int64(7)

	p, _ := project.New("web", "honryu", "")
	projectID, _ := store.CreateProject(ctx, p)
	pl, _ := scenario.NewNative("scenario", projectID, taurus.ExecutorJMeter) // no test file uploaded
	scenarioID, _ := store.CreateScenario(ctx, pl)
	e, _ := execution.New("peak", projectID)
	e.TenantID = &tenantID
	executionID, _ := store.CreateExecution(ctx, e)
	if err := store.StoreLoadProfile(ctx, executionID, false, []loadprofile.Entry{
		{ScenarioID: scenarioID, Concurrency: 10, Engines: 1, Duration: 30},
	}); err != nil {
		t.Fatalf("StoreLoadProfile: %v", err)
	}
	if err := store.SetCeiling(ctx, tenantID, "", 1); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}

	now := time.Now()
	schedules, lifecycle, _ := newTestServices(store, fake.NewObjectStore(), now)
	fireAt := now
	if _, err := schedules.Create(ctx, schedule.Schedule{
		ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true,
	}); err != nil {
		t.Fatalf("Create schedule: %v", err)
	}

	fireOnce(ctx, schedules, lifecycle) // must not panic

	if _, running, _ := store.CurrentRun(ctx, executionID); running {
		t.Fatal("execution shows running despite Deploy failing (no test file)")
	}
}

func TestRunLoop_FiresOnEachTickUntilCancelled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	const tenantID = int64(7)
	executionID := seedDueExecution(t, store, obj, tenantID, 2)
	now := time.Now()
	schedules, lifecycle, sched := newTestServices(store, obj, now)

	fireAt := now
	if _, err := schedules.Create(ctx, schedule.Schedule{
		ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true,
	}); err != nil {
		t.Fatalf("Create schedule: %v", err)
	}

	loopCtx, cancel := context.WithTimeout(ctx, 60*time.Millisecond)
	defer cancel()
	runLoop(loopCtx, schedules, lifecycle, 10*time.Millisecond)

	deployed, _ := sched.DeployedExecutions(ctx, "")
	if _, ok := deployed[executionID]; !ok {
		t.Fatal("runLoop never fired the due occurrence within its ticking window")
	}
}

func TestExtendHorizonsOnce_RecordsCompletion(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	schedules, _, _ := newTestServices(store, fake.NewObjectStore(), time.Now())

	extendHorizonsOnce(context.Background(), schedules)

	if _, found, err := schedules.LastHorizonExtension(context.Background()); err != nil || !found {
		t.Fatalf("LastHorizonExtension after extendHorizonsOnce = found:%v, err:%v, want found:true", found, err)
	}
}

// runHorizonLoop runs its pass immediately on startup, before ever checking
// ctx -- even a context cancelled before the call still gets one pass, since
// a schedule whose horizon drifted below 7 days while cmd/scheduler was down
// must be caught up right away, not left to wait for the first tick.
func TestRunHorizonLoop_RunsImmediatelyBeforeCheckingContext(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	schedules, _, _ := newTestServices(store, fake.NewObjectStore(), time.Now())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runHorizonLoop(ctx, schedules, time.Hour)

	if _, found, err := schedules.LastHorizonExtension(context.Background()); err != nil || !found {
		t.Fatalf("LastHorizonExtension after runHorizonLoop = found:%v, err:%v, want found:true (its immediate pass must still run)", found, err)
	}
}

// A schedule whose execution lost its load profile fails to extend, but
// extendHorizonsOnce must not panic on that -- it logs and moves on, and the
// pass still records its own completion (scheduleapp.ExtendHorizons does
// that even for a partial failure).
func TestExtendHorizonsOnce_ScheduleErrorDoesNotPanic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	const tenantID = int64(7)
	executionID := seedDueExecution(t, store, obj, tenantID, 2)
	now := time.Now()
	schedules, _, _ := newTestServices(store, obj, now)

	if _, err := schedules.Create(ctx, schedule.Schedule{
		ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindRecurring, Recurrence: "0 0 * * *", Active: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.StoreLoadProfile(ctx, executionID, false, nil); err != nil {
		t.Fatalf("StoreLoadProfile (clear): %v", err)
	}

	extendHorizonsOnce(ctx, schedules) // must not panic

	if _, found, err := schedules.LastHorizonExtension(ctx); err != nil || !found {
		t.Fatalf("LastHorizonExtension after a partial failure = found:%v, err:%v, want found:true", found, err)
	}
}

func TestRunHorizonLoop_TicksMoreThanOnceUntilCancelled(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	schedules, _, _ := newTestServices(store, fake.NewObjectStore(), time.Now())

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	runHorizonLoop(ctx, schedules, 10*time.Millisecond) // must return once ctx is done, without panicking
}

// drainProject seeds a project with two executions: designated (the
// campaign's readiness test) and stray (any other execution under the same
// project). Both are returned undeployed; the caller deploys/runs whichever
// it needs for its scenario.
func drainProject(t *testing.T, store *fake.Store) (projectID, designated, stray int64) {
	t.Helper()
	ctx := context.Background()
	p, err := project.New("web", "honryu", "")
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	projectID, err = store.CreateProject(ctx, p)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	de, err := execution.New("readiness", projectID)
	if err != nil {
		t.Fatalf("execution.New (designated): %v", err)
	}
	designated, err = store.CreateExecution(ctx, de)
	if err != nil {
		t.Fatalf("CreateExecution (designated): %v", err)
	}
	se, err := execution.New("stray", projectID)
	if err != nil {
		t.Fatalf("execution.New (stray): %v", err)
	}
	stray, err = store.CreateExecution(ctx, se)
	if err != nil {
		t.Fatalf("CreateExecution (stray): %v", err)
	}
	return projectID, designated, stray
}

func deployAndRun(t *testing.T, store *fake.Store, sched *fake.Scheduler, executionID int64) {
	t.Helper()
	ctx := context.Background()
	if err := sched.DeployScenario(ctx, ports.DeploySpec{ExecutionID: executionID, ScenarioID: 1}); err != nil {
		t.Fatalf("DeployScenario: %v", err)
	}
	if _, err := store.StartRun(ctx, executionID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
}

func TestDrainOnce_StopsARunningNonCompliantExecutionUnderAParticipatingProject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	sched := fake.NewScheduler()
	obj := fake.NewObjectStore()
	campaigns := campaignapp.NewService(store, sched)
	lifecycle := lifecycleapp.NewService(store, sched, obj, lifecycleapp.StaticImage("honryu/jmeter:latest"))

	projectID, designated, stray := drainProject(t, store)
	deployAndRun(t, store, sched, stray)

	if _, err := campaigns.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Hour)},
		Services: []campaign.Service{{ProjectID: projectID, ExecutionID: designated}},
	}); err != nil {
		t.Fatalf("Create campaign: %v", err)
	}

	drainOnce(ctx, campaigns, lifecycle, store)

	if _, running, _ := store.CurrentRun(ctx, stray); running {
		t.Fatal("the non-compliant execution should have been stopped by the drain sweep")
	}
}

// The whole point of freeze exempting the designated execution: draining
// must leave it running, not tear down the readiness test it exists to
// measure.
func TestDrainOnce_LeavesTheDesignatedExecutionRunning(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	sched := fake.NewScheduler()
	obj := fake.NewObjectStore()
	campaigns := campaignapp.NewService(store, sched)
	lifecycle := lifecycleapp.NewService(store, sched, obj, lifecycleapp.StaticImage("honryu/jmeter:latest"))

	projectID, designated, _ := drainProject(t, store)
	deployAndRun(t, store, sched, designated)

	if _, err := campaigns.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Hour)},
		Services: []campaign.Service{{ProjectID: projectID, ExecutionID: designated}},
	}); err != nil {
		t.Fatalf("Create campaign: %v", err)
	}

	drainOnce(ctx, campaigns, lifecycle, store)

	if _, running, _ := store.CurrentRun(ctx, designated); !running {
		t.Fatal("the designated execution must be left running -- it is exempt from freeze")
	}
}

// A deployed-but-idle in-scope execution has nothing to drain -- Stop would
// reject it as not running, so drainOnce must not even attempt it (which
// would otherwise show up as a spurious error on every tick).
func TestDrainOnce_LeavesADeployedButIdleExecutionAlone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	sched := fake.NewScheduler()
	obj := fake.NewObjectStore()
	campaigns := campaignapp.NewService(store, sched)
	lifecycle := lifecycleapp.NewService(store, sched, obj, lifecycleapp.StaticImage("honryu/jmeter:latest"))

	projectID, designated, stray := drainProject(t, store)
	if err := sched.DeployScenario(ctx, ports.DeploySpec{ExecutionID: stray, ScenarioID: 1}); err != nil {
		t.Fatalf("DeployScenario: %v", err)
	} // deployed, but never started

	if _, err := campaigns.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Hour)},
		Services: []campaign.Service{{ProjectID: projectID, ExecutionID: designated}},
	}); err != nil {
		t.Fatalf("Create campaign: %v", err)
	}

	drainOnce(ctx, campaigns, lifecycle, store) // must not panic or error visibly

	if _, running, _ := store.CurrentRun(ctx, stray); running {
		t.Fatal("an idle execution cannot have started running on its own")
	}
}

func TestDrainOnce_NoActiveCampaignsIsANoOp(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	sched := fake.NewScheduler()
	obj := fake.NewObjectStore()
	campaigns := campaignapp.NewService(store, sched)
	lifecycle := lifecycleapp.NewService(store, sched, obj, lifecycleapp.StaticImage("honryu/jmeter:latest"))

	drainOnce(context.Background(), campaigns, lifecycle, store) // must not panic
}

func TestRunDrainLoop_TicksMoreThanOnceUntilCancelled(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	sched := fake.NewScheduler()
	obj := fake.NewObjectStore()
	campaigns := campaignapp.NewService(store, sched)
	lifecycle := lifecycleapp.NewService(store, sched, obj, lifecycleapp.StaticImage("honryu/jmeter:latest"))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	runDrainLoop(ctx, campaigns, lifecycle, store, 10*time.Millisecond) // must return once ctx is done, without panicking
}
