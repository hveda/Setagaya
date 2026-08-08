package calibrationapp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/app/calibrationapp"
	"github.com/heridotlife/honryu/internal/domain/calibration"
	"github.com/heridotlife/honryu/internal/domain/capacityprofile"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/project"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func seedProject(t *testing.T, store *fake.Store) int64 {
	t.Helper()
	p, err := project.New("web", "honryu", "")
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	id, err := store.CreateProject(context.Background(), p)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return id
}

func validSpec() calibration.Spec {
	return calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi"}
}

func TestCreate_PersistsExecutionCriteriaAndBounds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	projectID := seedProject(t, store)

	executionID, err := svc.Create(ctx, "checkout-calibration", projectID, taurus.ExecutorJMeter, validSpec())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if executionID <= 0 {
		t.Fatalf("Create returned id = %d, want > 0", executionID)
	}

	exe, err := store.GetExecution(ctx, executionID)
	if err != nil {
		t.Fatalf("GetExecution: %v", err)
	}
	if exe.Kind != execution.KindCalibrateEngine {
		t.Errorf("Kind = %q, want calibrate_engine", exe.Kind)
	}
	if exe.Engine != taurus.ExecutorJMeter {
		t.Errorf("Engine = %q, want jmeter", exe.Engine)
	}
	if exe.CPU != "1" || exe.Memory != "512Mi" {
		t.Errorf("CPU/Memory = %q/%q, want 1/512Mi", exe.CPU, exe.Memory)
	}

	criteria, err := store.CriteriaFor(ctx, executionID)
	if err != nil {
		t.Fatalf("CriteriaFor: %v", err)
	}
	if len(criteria) != 1 || criteria[0] != "failures>5%" {
		t.Errorf("CriteriaFor = %v, want [failures>5%%]", criteria)
	}

	bounds, err := store.CalibrationBoundsFor(ctx, executionID)
	if err != nil {
		t.Fatalf("CalibrationBoundsFor: %v", err)
	}
	if bounds.SeedQPS != calibration.DefaultSeedQPS || bounds.MaxQPS != calibration.DefaultMaxQPS {
		t.Errorf("bounds = %+v, want the defaulted values", bounds)
	}
}

func TestCreate_RejectsAnInvalidSpec(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	projectID := seedProject(t, store)

	spec := validSpec()
	spec.Criterion = ""
	if _, err := svc.Create(ctx, "x", projectID, taurus.ExecutorJMeter, spec); !errors.Is(err, calibration.ErrCriterionRequired) {
		t.Fatalf("Create (no criterion) = %v, want ErrCriterionRequired", err)
	}
}

func TestCreate_RejectsAnInvalidExecutionName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	projectID := seedProject(t, store)

	if _, err := svc.Create(ctx, "", projectID, taurus.ExecutorJMeter, validSpec()); !errors.Is(err, execution.ErrNameRequired) {
		t.Fatalf("Create (no name) = %v, want ErrNameRequired", err)
	}
}

func TestSpecFor_ReassemblesFromExecutionCriteriaAndBounds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	projectID := seedProject(t, store)

	spec := calibration.Spec{Criterion: "p95>500ms", CPU: "2", Memory: "1Gi", SeedQPS: 5, MaxQPS: 500, MaxSteps: 10, HoldSeconds: 20}
	executionID, err := svc.Create(ctx, "x", projectID, taurus.ExecutorK6, spec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.SpecFor(ctx, executionID)
	if err != nil {
		t.Fatalf("SpecFor: %v", err)
	}
	if got != spec {
		t.Fatalf("SpecFor = %+v, want %+v", got, spec)
	}
}

func TestSpecFor_UnconfiguredExecutionPropagatesNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	projectID := seedProject(t, store)

	// A bare execution never routed through Create -- no bounds recorded.
	exe, err := execution.New("bare", projectID)
	if err != nil {
		t.Fatalf("execution.New: %v", err)
	}
	exe.Kind = execution.KindCalibrateEngine
	executionID, err := store.CreateExecution(ctx, exe)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}

	if _, err := svc.SpecFor(ctx, executionID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("SpecFor(unconfigured) = %v, want ErrNotFound", err)
	}
}

func TestTrigger_CreatesAPendingJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	projectID := seedProject(t, store)
	executionID, err := svc.Create(ctx, "x", projectID, taurus.ExecutorJMeter, validSpec())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	jobID, err := svc.Trigger(ctx, executionID)
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if jobID <= 0 {
		t.Fatalf("Trigger returned job id = %d, want > 0", jobID)
	}

	job, err := store.GetCalibrationJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetCalibrationJob: %v", err)
	}
	if job.ExecutionID != executionID || job.Phase != calibration.PhasePending {
		t.Fatalf("GetCalibrationJob = %+v, want execution=%d phase=pending", job, executionID)
	}
}

// A calibration execution can be triggered more than once over its life,
// the same way an execution can have more than one run.
func TestTrigger_MoreThanOnceCreatesSeparateJobs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	projectID := seedProject(t, store)
	executionID, err := svc.Create(ctx, "x", projectID, taurus.ExecutorJMeter, validSpec())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	first, err := svc.Trigger(ctx, executionID)
	if err != nil {
		t.Fatalf("Trigger (first): %v", err)
	}
	second, err := svc.Trigger(ctx, executionID)
	if err != nil {
		t.Fatalf("Trigger (second): %v", err)
	}
	if first == second {
		t.Fatalf("two triggers produced the same job id %d", first)
	}

	jobs, err := svc.ListByExecution(ctx, executionID)
	if err != nil {
		t.Fatalf("ListByExecution: %v", err)
	}
	if len(jobs) != 2 || jobs[0].ID != second || jobs[1].ID != first {
		t.Fatalf("ListByExecution = %+v, want [second, first] most-recent-first", jobs)
	}
}

func TestTrigger_RejectsANonCalibrationExecution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	projectID := seedProject(t, store)

	normal, err := execution.New("ordinary", projectID)
	if err != nil {
		t.Fatalf("execution.New: %v", err)
	}
	executionID, err := store.CreateExecution(ctx, normal)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}

	if _, err := svc.Trigger(ctx, executionID); !errors.Is(err, calibrationapp.ErrExecutionNotCalibration) {
		t.Fatalf("Trigger(normal execution) = %v, want ErrExecutionNotCalibration", err)
	}
}

// Trigger fails loudly rather than starting a search with an undefined
// target-health criterion or pod size -- the safety net for an execution
// that reached CalibrateEngine kind without ever going through Create (or
// whose Create partially failed).
func TestTrigger_RejectsAnUnconfiguredCalibrationExecution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	projectID := seedProject(t, store)

	exe, err := execution.New("unconfigured", projectID)
	if err != nil {
		t.Fatalf("execution.New: %v", err)
	}
	exe.Kind = execution.KindCalibrateEngine
	executionID, err := store.CreateExecution(ctx, exe)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}

	if _, err := svc.Trigger(ctx, executionID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Trigger(unconfigured) = %v, want the SpecFor NotFound to propagate", err)
	}
}

func TestTrigger_UnknownExecutionPropagatesNotFound(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	if _, err := svc.Trigger(context.Background(), 999); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Trigger(unknown execution) = %v, want ErrNotFound", err)
	}
}

func TestGet_MissingJobReturnsNotFound(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	if _, err := svc.Get(context.Background(), 999); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Get(missing job) = %v, want ErrNotFound", err)
	}
}

// Get exposes a job's full step history; ListByExecution stays lightweight
// (no steps), matching how a caller wanting one job's detail asks for it
// specifically rather than every listed job carrying its whole history.
func TestGet_ExposesStepHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	projectID := seedProject(t, store)
	executionID, err := svc.Create(ctx, "x", projectID, taurus.ExecutorJMeter, validSpec())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	jobID, err := svc.Trigger(ctx, executionID)
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	step := calibration.Step{RequestedQPS: 10, AchievedQPS: 10, Classification: calibration.ClassificationClean}
	if err := store.RecordStep(ctx, jobID, step,
		ports.CalibrationJob{Phase: calibration.PhaseBracketing, StepCount: 1, BracketLoRequested: 10, BracketLoAchieved: 10, NextRequestedQPS: 20},
	); err != nil {
		t.Fatalf("RecordStep: %v", err)
	}

	got, err := svc.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Phase != calibration.PhaseBracketing || got.NextRequestedQPS != 20 {
		t.Fatalf("Get = %+v, want the recorded state", got)
	}
	if len(got.Steps) != 1 || got.Steps[0] != step {
		t.Fatalf("Get Steps = %+v, want [%+v]", got.Steps, step)
	}
}

// erroringRepo wraps a *fake.Store and lets a test force one method to
// fail, proving each use-case propagates a downstream failure from any of
// its data sources rather than swallowing it.
type erroringRepo struct {
	*fake.Store
	createExecutionErr       error
	setExecutionCriteriaErr  error
	setCalibrationBoundsErr  error
	criteriaForErr           error
	calibrationBoundsForErr  error
	createCalibrationJobErr  error
	stepsForErr              error
	recordStepErr            error
	upsertCapacityProfileErr error

	// getExecutionErr/loadProfileForErr apply starting from the Nth call
	// (1-indexed) so a test can let an earlier caller in the same
	// AdvanceOne tick (SpecFor) succeed while a later one (writeProfile)
	// fails -- both route through the same repo method.
	getExecutionErr           error
	getExecutionErrFromCall   int
	getExecutionCalls         int
	loadProfileForErr         error
	loadProfileForErrFromCall int
	loadProfileForCalls       int
}

func (r *erroringRepo) CreateExecution(ctx context.Context, c execution.Execution) (int64, error) {
	if r.createExecutionErr != nil {
		return 0, r.createExecutionErr
	}
	return r.Store.CreateExecution(ctx, c)
}

func (r *erroringRepo) SetExecutionCriteria(ctx context.Context, executionID int64, criteria []string) error {
	if r.setExecutionCriteriaErr != nil {
		return r.setExecutionCriteriaErr
	}
	return r.Store.SetExecutionCriteria(ctx, executionID, criteria)
}

func (r *erroringRepo) SetCalibrationBounds(ctx context.Context, executionID int64, bounds ports.CalibrationBounds) error {
	if r.setCalibrationBoundsErr != nil {
		return r.setCalibrationBoundsErr
	}
	return r.Store.SetCalibrationBounds(ctx, executionID, bounds)
}

func (r *erroringRepo) CriteriaFor(ctx context.Context, executionID int64) ([]string, error) {
	if r.criteriaForErr != nil {
		return nil, r.criteriaForErr
	}
	return r.Store.CriteriaFor(ctx, executionID)
}

func (r *erroringRepo) CalibrationBoundsFor(ctx context.Context, executionID int64) (ports.CalibrationBounds, error) {
	if r.calibrationBoundsForErr != nil {
		return ports.CalibrationBounds{}, r.calibrationBoundsForErr
	}
	return r.Store.CalibrationBoundsFor(ctx, executionID)
}

func (r *erroringRepo) CreateCalibrationJob(ctx context.Context, executionID int64) (int64, error) {
	if r.createCalibrationJobErr != nil {
		return 0, r.createCalibrationJobErr
	}
	return r.Store.CreateCalibrationJob(ctx, executionID)
}

func (r *erroringRepo) StepsFor(ctx context.Context, jobID int64) ([]calibration.Step, error) {
	if r.stepsForErr != nil {
		return nil, r.stepsForErr
	}
	return r.Store.StepsFor(ctx, jobID)
}

func (r *erroringRepo) RecordStep(ctx context.Context, jobID int64, step calibration.Step, updated ports.CalibrationJob) error {
	if r.recordStepErr != nil {
		return r.recordStepErr
	}
	return r.Store.RecordStep(ctx, jobID, step, updated)
}

func (r *erroringRepo) UpsertCapacityProfile(ctx context.Context, profile capacityprofile.CapacityProfile) error {
	if r.upsertCapacityProfileErr != nil {
		return r.upsertCapacityProfileErr
	}
	return r.Store.UpsertCapacityProfile(ctx, profile)
}

func (r *erroringRepo) GetExecution(ctx context.Context, id int64) (execution.Execution, error) {
	r.getExecutionCalls++
	if r.getExecutionErr != nil && r.getExecutionCalls >= r.getExecutionErrFromCall {
		return execution.Execution{}, r.getExecutionErr
	}
	return r.Store.GetExecution(ctx, id)
}

func (r *erroringRepo) LoadProfileFor(ctx context.Context, executionID int64) ([]loadprofile.Entry, error) {
	r.loadProfileForCalls++
	if r.loadProfileForErr != nil && r.loadProfileForCalls >= r.loadProfileForErrFromCall {
		return nil, r.loadProfileForErr
	}
	return r.Store.LoadProfileFor(ctx, executionID)
}

func TestCreate_DownstreamErrorsPropagate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		wire func(r *erroringRepo)
	}{
		{"CreateExecution fails", func(r *erroringRepo) { r.createExecutionErr = errors.New("boom") }},
		{"SetExecutionCriteria fails", func(r *erroringRepo) { r.setExecutionCriteriaErr = errors.New("boom") }},
		{"SetCalibrationBounds fails", func(r *erroringRepo) { r.setCalibrationBoundsErr = errors.New("boom") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := fake.NewStore()
			repo := &erroringRepo{Store: store}
			tt.wire(repo)
			svc := calibrationapp.NewService(repo)
			projectID := seedProject(t, store)

			if _, err := svc.Create(context.Background(), "x", projectID, taurus.ExecutorJMeter, validSpec()); err == nil {
				t.Fatal("Create = nil error, want the downstream failure to propagate")
			}
		})
	}
}

func TestSpecFor_DownstreamErrorsPropagate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		wire func(r *erroringRepo)
	}{
		{"CriteriaFor fails", func(r *erroringRepo) { r.criteriaForErr = errors.New("boom") }},
		{"CalibrationBoundsFor fails", func(r *erroringRepo) { r.calibrationBoundsForErr = errors.New("boom") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := fake.NewStore()
			projectID := seedProject(t, store)
			executionID, err := calibrationapp.NewService(store).Create(context.Background(), "x", projectID, taurus.ExecutorJMeter, validSpec())
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			repo := &erroringRepo{Store: store}
			tt.wire(repo)
			svc := calibrationapp.NewService(repo)
			if _, err := svc.SpecFor(context.Background(), executionID); err == nil {
				t.Fatal("SpecFor = nil error, want the downstream failure to propagate")
			}
		})
	}
}

func TestTrigger_CreateCalibrationJobErrorPropagates(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	projectID := seedProject(t, store)
	executionID, err := calibrationapp.NewService(store).Create(context.Background(), "x", projectID, taurus.ExecutorJMeter, validSpec())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	repo := &erroringRepo{Store: store, createCalibrationJobErr: errors.New("boom")}
	svc := calibrationapp.NewService(repo)
	if _, err := svc.Trigger(context.Background(), executionID); err == nil {
		t.Fatal("Trigger = nil error, want the CreateCalibrationJob failure to propagate")
	}
}

func TestGet_StepsForErrorPropagates(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	projectID := seedProject(t, store)
	svc := calibrationapp.NewService(store)
	executionID, err := svc.Create(context.Background(), "x", projectID, taurus.ExecutorJMeter, validSpec())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	jobID, err := svc.Trigger(context.Background(), executionID)
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	repo := &erroringRepo{Store: store, stepsForErr: errors.New("boom")}
	errSvc := calibrationapp.NewService(repo)
	if _, err := errSvc.Get(context.Background(), jobID); err == nil {
		t.Fatal("Get = nil error, want the StepsFor failure to propagate")
	}
}

// stubRunnerCall records one RunStep invocation's arguments.
type stubRunnerCall struct {
	executionID  int64
	requestedQPS float64
	holdSeconds  int
}

// stubRunnerResponse is one scripted RunStep outcome.
type stubRunnerResponse struct {
	report report.Report
	err    error
}

// stubRunner is a scripted calibrationapp.Runner: each RunStep call consumes
// the next queued response, in order, and records its own arguments -- lets
// a test drive AdvanceOne's classify/decide/persist loop deterministically,
// without a real Deploy/Trigger/Stop pipeline (step_test.go already proves
// that machinery works).
type stubRunner struct {
	responses []stubRunnerResponse
	calls     []stubRunnerCall
}

func (r *stubRunner) RunStep(_ context.Context, executionID int64, requestedQPS float64, holdSeconds int) (report.Report, error) {
	r.calls = append(r.calls, stubRunnerCall{executionID, requestedQPS, holdSeconds})
	if len(r.responses) == 0 {
		panic("stubRunner: no more scripted responses")
	}
	resp := r.responses[0]
	r.responses = r.responses[1:]
	return resp.report, resp.err
}

// cleanReport reports achievedQPS against requestedQPS with no failures --
// engine kept up, target healthy.
func cleanReport(requestedQPS, achievedQPS float64) report.Report {
	return report.Report{
		Requested: report.Load{Throughput: requestedQPS},
		Achieved:  report.Load{Throughput: achievedQPS, Samples: 1000},
	}
}

// engineSaturatedReport reports well under requestedQPS (ShortOfRequest),
// with no target-attributed failures -- the pod itself could not sustain
// the rate.
func engineSaturatedReport(requestedQPS, achievedQPS float64) report.Report {
	return report.Report{
		Requested: report.Load{Throughput: requestedQPS},
		Achieved:  report.Load{Throughput: achievedQPS, Samples: 1000},
	}
}

// targetSaturatedReport reports the engine keeping up (achieved at or above
// requestedQPS's tolerance) while the overall error rate trips
// validSpec()'s "failures>5%" criterion.
func targetSaturatedReport(requestedQPS, achievedQPS float64) report.Report {
	return report.Report{
		Requested:   report.Load{Throughput: requestedQPS},
		Achieved:    report.Load{Throughput: achievedQPS, Samples: 1000},
		ErrorRate:   0.10,
		Attribution: report.Attribution{Target: 100},
	}
}

// stubFingerprinter returns a fixed fingerprint, or an error when set.
type stubFingerprinter struct {
	value string
	err   error
}

func (f *stubFingerprinter) ScenarioFingerprint(context.Context, int64) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.value, nil
}

// seedTriggeredCalibration creates a project, a CalibrateEngine execution
// configured with spec, binds it to a scenario the ordinary way (a single
// load-profile entry -- calibrationapp.Create's own doc comment), and
// triggers a fresh Pending job. Mirrors what a real caller does before a
// controller ever calls AdvanceOne.
func seedTriggeredCalibration(t *testing.T, store *fake.Store, spec calibration.Spec) (executionID, jobID, scenarioID int64) {
	t.Helper()
	ctx := context.Background()
	projectID := seedProject(t, store)
	svc := calibrationapp.NewService(store)
	executionID, err := svc.Create(ctx, "calibrate", projectID, taurus.ExecutorJMeter, spec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	pl, err := scenario.NewNative("target", projectID, taurus.ExecutorJMeter)
	if err != nil {
		t.Fatalf("scenario.NewNative: %v", err)
	}
	scenarioID, err = store.CreateScenario(ctx, pl)
	if err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}
	entries := []loadprofile.Entry{{ScenarioID: scenarioID, Engines: 1, Concurrency: 1, Duration: 1}}
	if err := store.StoreLoadProfile(ctx, executionID, false, entries); err != nil {
		t.Fatalf("StoreLoadProfile: %v", err)
	}
	jobID, err = svc.Trigger(ctx, executionID)
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	return executionID, jobID, scenarioID
}

func TestAdvanceOne_NoJobDue(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	runner := &stubRunner{}
	svc := calibrationapp.NewService(store).WithRunner(runner).WithFingerprint(&stubFingerprinter{value: "fp"})

	found, err := svc.AdvanceOne(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("AdvanceOne: %v", err)
	}
	if found {
		t.Fatal("found = true, want false with no job ever triggered")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %v, want none", runner.calls)
	}
}

func TestAdvanceOne_RequiresRunnerAndFingerprint(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 1000, MaxSteps: 5, HoldSeconds: 1}
	seedTriggeredCalibration(t, store, spec)

	// Neither WithRunner nor WithFingerprint is wired.
	svc := calibrationapp.NewService(store)
	if _, err := svc.AdvanceOne(context.Background(), time.Now()); !errors.Is(err, calibrationapp.ErrNotConfiguredForAdvance) {
		t.Fatalf("error = %v, want ErrNotConfiguredForAdvance", err)
	}
	// The job must still be claimable afterward -- a configuration error
	// must not consume its claim.
	if _, running, _ := store.CurrentRun(context.Background(), 0); running {
		t.Fatal("unexpected run state touched")
	}
}

func TestAdvanceOne_EngineLimitedHappyPath_WithConfirmedRetry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 1000, MaxSteps: 2, HoldSeconds: 1}
	executionID, jobID, scenarioID := seedTriggeredCalibration(t, store, spec)

	runner := &stubRunner{responses: []stubRunnerResponse{
		{report: cleanReport(10, 10)},             // tick 1: clean, doubles to 20
		{report: engineSaturatedReport(20, 10)},   // tick 2 attempt 1: engine-short (anomalous?)
		{report: engineSaturatedReport(20, 10.5)}, // tick 2 retry: confirmed engine-short
	}}
	fp := &stubFingerprinter{value: "fingerprint-1"}
	svc := calibrationapp.NewService(store).WithRunner(runner).WithFingerprint(fp)

	// Tick 1: clean at the seed QPS, not yet terminal.
	found, err := svc.AdvanceOne(ctx, time.Now())
	if err != nil || !found {
		t.Fatalf("tick 1: found=%v, err=%v", found, err)
	}
	job, err := svc.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("Get after tick 1: %v", err)
	}
	if job.Phase != calibration.PhaseBracketing || job.NextRequestedQPS != 20 {
		t.Fatalf("after tick 1: %+v, want Bracketing at next 20", job.CalibrationJob)
	}

	// Tick 2: engine-short, retried once, confirmed -- terminal.
	found, err = svc.AdvanceOne(ctx, time.Now())
	if err != nil || !found {
		t.Fatalf("tick 2: found=%v, err=%v", found, err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("runner calls = %d, want 3 (1 clean + 1 engine-short + 1 retry)", len(runner.calls))
	}
	if runner.calls[1].requestedQPS != 20 || runner.calls[2].requestedQPS != 20 {
		t.Fatalf("retry must reuse the same requested QPS: calls = %+v", runner.calls)
	}

	job, err = svc.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("Get after tick 2: %v", err)
	}
	if job.Phase != calibration.PhaseDone || job.Result == nil {
		t.Fatalf("job not terminal: %+v", job.CalibrationJob)
	}
	if job.Result.SaturatedBy != calibration.SaturatedByEngine || job.Result.PerPodQPS != 10 {
		t.Fatalf("result = %+v, want engine-limited at 10 (the last clean step's achieved QPS)", job.Result)
	}
	// Only the retry's own outcome is recorded -- the discarded first
	// attempt leaves no trace.
	if len(job.Steps) != 2 || job.Steps[1].AchievedQPS != 10.5 {
		t.Fatalf("steps = %+v, want the retry's achieved QPS (10.5), not the discarded first attempt's", job.Steps)
	}

	profile, err := store.GetCapacityProfile(ctx, capacityprofile.Key{ScenarioID: scenarioID, Engine: taurus.ExecutorJMeter, CPU: "1", Memory: "512Mi"})
	if err != nil {
		t.Fatalf("GetCapacityProfile: %v", err)
	}
	if profile.PerPodQPS != 10 || profile.SaturatedBy != calibration.SaturatedByEngine {
		t.Fatalf("profile = %+v, want PerPodQPS 10, SaturatedBy engine", profile)
	}
	if profile.ScenarioFingerprint != "fingerprint-1" {
		t.Fatalf("profile fingerprint = %q, want fingerprint-1", profile.ScenarioFingerprint)
	}
	if profile.JobID != jobID {
		t.Fatalf("profile.JobID = %d, want %d", profile.JobID, jobID)
	}
	_ = executionID
}

func TestAdvanceOne_RetryDiscardsAnAnomalousEngineShort(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 1000, MaxSteps: 5, HoldSeconds: 1}
	_, jobID, _ := seedTriggeredCalibration(t, store, spec)

	runner := &stubRunner{responses: []stubRunnerResponse{
		{report: engineSaturatedReport(10, 4)}, // attempt 1: anomalous engine-short
		{report: cleanReport(10, 10)},          // retry: actually clean
	}}
	svc := calibrationapp.NewService(store).WithRunner(runner).WithFingerprint(&stubFingerprinter{value: "fp"})

	if _, err := svc.AdvanceOne(ctx, time.Now()); err != nil {
		t.Fatalf("AdvanceOne: %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner calls = %d, want 2 (1 anomalous + 1 retry)", len(runner.calls))
	}

	job, err := svc.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// The retry's clean outcome governs: bracketing continues (doubled),
	// not bisecting.
	if job.Phase != calibration.PhaseBracketing || job.NextRequestedQPS != 20 {
		t.Fatalf("job = %+v, want Bracketing at next 20 (the retry's clean result)", job.CalibrationJob)
	}
	if len(job.Steps) != 1 || job.Steps[0].Classification != calibration.ClassificationClean {
		t.Fatalf("steps = %+v, want a single clean step (the retry's, not the discarded anomaly)", job.Steps)
	}
}

func TestAdvanceOne_TargetLimitedTerminatesInOneTick(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 1000, MaxSteps: 5, HoldSeconds: 1}
	_, jobID, scenarioID := seedTriggeredCalibration(t, store, spec)

	// Engine keeps up (achieved == requested), but the overall error rate
	// trips the "failures>5%" criterion -- one pod already overloads the
	// target.
	runner := &stubRunner{responses: []stubRunnerResponse{{report: targetSaturatedReport(10, 10)}}}
	svc := calibrationapp.NewService(store).WithRunner(runner).WithFingerprint(&stubFingerprinter{value: "fp"})

	found, err := svc.AdvanceOne(ctx, time.Now())
	if err != nil || !found {
		t.Fatalf("found=%v, err=%v", found, err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 -- target-saturation is not retried", len(runner.calls))
	}

	job, err := svc.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if job.Phase != calibration.PhaseDone || job.Result == nil || job.Result.SaturatedBy != calibration.SaturatedByTarget {
		t.Fatalf("job = %+v, want Done/target", job.CalibrationJob)
	}
	if job.Result.PerPodQPS != 10 {
		t.Fatalf("PerPodQPS = %v, want 10 (achieved at the tripping step, a lower bound)", job.Result.PerPodQPS)
	}

	profile, err := store.GetCapacityProfile(ctx, capacityprofile.Key{ScenarioID: scenarioID, Engine: taurus.ExecutorJMeter, CPU: "1", Memory: "512Mi"})
	if err != nil {
		t.Fatalf("GetCapacityProfile: %v", err)
	}
	if profile.SaturatedBy != calibration.SaturatedByTarget {
		t.Fatalf("profile.SaturatedBy = %q, want target -- a target-limited finding must still be recorded, not dropped", profile.SaturatedBy)
	}
}

func TestAdvanceOne_NeitherTerminatesAtTheSafetyCeiling(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	// MaxQPS is set so the very first clean step's double (20) would
	// breach it -- the search ends honestly unresolved.
	spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 15, MaxSteps: 5, HoldSeconds: 1}
	_, jobID, scenarioID := seedTriggeredCalibration(t, store, spec)

	runner := &stubRunner{responses: []stubRunnerResponse{{report: cleanReport(10, 10)}}}
	svc := calibrationapp.NewService(store).WithRunner(runner).WithFingerprint(&stubFingerprinter{value: "fp"})

	found, err := svc.AdvanceOne(ctx, time.Now())
	if err != nil || !found {
		t.Fatalf("found=%v, err=%v", found, err)
	}

	job, err := svc.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if job.Phase != calibration.PhaseDone || job.Result == nil || job.Result.SaturatedBy != calibration.SaturatedByNeither {
		t.Fatalf("job = %+v, want Done/neither", job.CalibrationJob)
	}
	if job.Result.PerPodQPS != 10 {
		t.Fatalf("PerPodQPS = %v, want 10 (the last clean step, a lower bound)", job.Result.PerPodQPS)
	}

	profile, err := store.GetCapacityProfile(ctx, capacityprofile.Key{ScenarioID: scenarioID, Engine: taurus.ExecutorJMeter, CPU: "1", Memory: "512Mi"})
	if err != nil {
		t.Fatalf("GetCapacityProfile: %v", err)
	}
	if profile.SaturatedBy != calibration.SaturatedByNeither {
		t.Fatalf("profile.SaturatedBy = %q, want neither -- an inconclusive finding must still be recorded", profile.SaturatedBy)
	}
}

func TestAdvanceOne_SpecForFailureMarksJobFailed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 1000, MaxSteps: 5, HoldSeconds: 1}
	errRepo := &erroringRepo{Store: store}
	_, jobID, _ := seedTriggeredCalibrationWithRepo(t, errRepo, spec)

	sentinel := errors.New("boom")
	errRepo.criteriaForErr = sentinel
	svc := calibrationapp.NewService(errRepo).WithRunner(&stubRunner{}).WithFingerprint(&stubFingerprinter{value: "fp"})

	if _, err := svc.AdvanceOne(ctx, time.Now()); !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want sentinel", err)
	}
	job, err := store.GetCalibrationJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetCalibrationJob: %v", err)
	}
	if job.Phase != calibration.PhaseFailed {
		t.Fatalf("job.Phase = %q, want failed", job.Phase)
	}
	if job.FailureReason == "" {
		t.Fatal("FailureReason is empty, want the propagated error's message")
	}
}

func TestAdvanceOne_RunStepFailureMarksJobFailed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 1000, MaxSteps: 5, HoldSeconds: 1}
	_, jobID, _ := seedTriggeredCalibration(t, store, spec)

	sentinel := errors.New("boom")
	runner := &stubRunner{responses: []stubRunnerResponse{{err: sentinel}}}
	svc := calibrationapp.NewService(store).WithRunner(runner).WithFingerprint(&stubFingerprinter{value: "fp"})

	if _, err := svc.AdvanceOne(ctx, time.Now()); !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want sentinel", err)
	}
	job, err := store.GetCalibrationJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetCalibrationJob: %v", err)
	}
	if job.Phase != calibration.PhaseFailed {
		t.Fatalf("job.Phase = %q, want failed", job.Phase)
	}
}

func TestAdvanceOne_RunStepFailureOnRetryMarksJobFailed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 1000, MaxSteps: 5, HoldSeconds: 1}
	_, jobID, _ := seedTriggeredCalibration(t, store, spec)

	sentinel := errors.New("boom")
	runner := &stubRunner{responses: []stubRunnerResponse{
		{report: engineSaturatedReport(10, 4)}, // attempt 1: engine-short, triggers a retry
		{err: sentinel},                        // retry itself fails
	}}
	svc := calibrationapp.NewService(store).WithRunner(runner).WithFingerprint(&stubFingerprinter{value: "fp"})

	if _, err := svc.AdvanceOne(ctx, time.Now()); !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want sentinel", err)
	}
	job, err := store.GetCalibrationJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetCalibrationJob: %v", err)
	}
	if job.Phase != calibration.PhaseFailed {
		t.Fatalf("job.Phase = %q, want failed", job.Phase)
	}
}

func TestAdvanceOne_RecordStepFailurePropagates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 1000, MaxSteps: 5, HoldSeconds: 1}
	errRepo := &erroringRepo{Store: store}
	seedTriggeredCalibrationWithRepo(t, errRepo, spec)

	sentinel := errors.New("boom")
	errRepo.recordStepErr = sentinel
	runner := &stubRunner{responses: []stubRunnerResponse{{report: cleanReport(10, 10)}}}
	svc := calibrationapp.NewService(errRepo).WithRunner(runner).WithFingerprint(&stubFingerprinter{value: "fp"})

	if _, err := svc.AdvanceOne(ctx, time.Now()); !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want sentinel", err)
	}
}

func TestAdvanceOne_WriteProfileFailuresPropagateWithoutCorruptingJobState(t *testing.T) {
	t.Parallel()

	t.Run("no scenario configured", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		store := fake.NewStore()
		projectID := seedProject(t, store)
		spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 1000, MaxSteps: 5, HoldSeconds: 1}
		svc := calibrationapp.NewService(store)
		executionID, err := svc.Create(ctx, "calibrate", projectID, taurus.ExecutorJMeter, spec)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		// Deliberately never bound to a scenario.
		jobID, err := svc.Trigger(ctx, executionID)
		if err != nil {
			t.Fatalf("Trigger: %v", err)
		}

		runner := &stubRunner{responses: []stubRunnerResponse{{report: targetSaturatedReport(10, 10)}}}
		advSvc := calibrationapp.NewService(store).WithRunner(runner).WithFingerprint(&stubFingerprinter{value: "fp"})
		if _, err := advSvc.AdvanceOne(ctx, time.Now()); !errors.Is(err, calibrationapp.ErrScenarioNotConfigured) {
			t.Fatalf("error = %v, want ErrScenarioNotConfigured", err)
		}
		// The search outcome itself is still durably recorded as Done --
		// only the downstream profile write failed.
		job, err := store.GetCalibrationJob(ctx, jobID)
		if err != nil {
			t.Fatalf("GetCalibrationJob: %v", err)
		}
		if job.Phase != calibration.PhaseDone {
			t.Fatalf("job.Phase = %q, want done (the search succeeded even though the profile write failed)", job.Phase)
		}
	})

	t.Run("fingerprint fails", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		store := fake.NewStore()
		spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 1000, MaxSteps: 5, HoldSeconds: 1}
		seedTriggeredCalibration(t, store, spec)

		sentinel := errors.New("boom")
		runner := &stubRunner{responses: []stubRunnerResponse{{report: targetSaturatedReport(10, 10)}}}
		svc := calibrationapp.NewService(store).WithRunner(runner).WithFingerprint(&stubFingerprinter{err: sentinel})
		if _, err := svc.AdvanceOne(ctx, time.Now()); !errors.Is(err, sentinel) {
			t.Fatalf("error = %v, want sentinel", err)
		}
	})

	t.Run("UpsertCapacityProfile fails", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		store := fake.NewStore()
		spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 1000, MaxSteps: 5, HoldSeconds: 1}
		errRepo := &erroringRepo{Store: store}
		seedTriggeredCalibrationWithRepo(t, errRepo, spec)

		sentinel := errors.New("boom")
		errRepo.upsertCapacityProfileErr = sentinel
		runner := &stubRunner{responses: []stubRunnerResponse{{report: targetSaturatedReport(10, 10)}}}
		svc := calibrationapp.NewService(errRepo).WithRunner(runner).WithFingerprint(&stubFingerprinter{value: "fp"})
		if _, err := svc.AdvanceOne(ctx, time.Now()); !errors.Is(err, sentinel) {
			t.Fatalf("error = %v, want sentinel", err)
		}
	})

	t.Run("writeProfile's own GetExecution fails", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		store := fake.NewStore()
		spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 1000, MaxSteps: 5, HoldSeconds: 1}
		errRepo := &erroringRepo{Store: store}
		seedTriggeredCalibrationWithRepo(t, errRepo, spec)

		sentinel := errors.New("boom")
		// Call 1 is SpecFor's own GetExecution (must still succeed, or
		// AdvanceOne would mark the job Failed instead of reaching
		// writeProfile); call 2 is writeProfile's.
		errRepo.getExecutionErr = sentinel
		errRepo.getExecutionErrFromCall = 2
		runner := &stubRunner{responses: []stubRunnerResponse{{report: targetSaturatedReport(10, 10)}}}
		svc := calibrationapp.NewService(errRepo).WithRunner(runner).WithFingerprint(&stubFingerprinter{value: "fp"})
		if _, err := svc.AdvanceOne(ctx, time.Now()); !errors.Is(err, sentinel) {
			t.Fatalf("error = %v, want sentinel", err)
		}
	})

	t.Run("writeProfile's own LoadProfileFor fails", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		store := fake.NewStore()
		spec := calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi", SeedQPS: 10, MaxQPS: 1000, MaxSteps: 5, HoldSeconds: 1}
		errRepo := &erroringRepo{Store: store}
		seedTriggeredCalibrationWithRepo(t, errRepo, spec)

		sentinel := errors.New("boom")
		errRepo.loadProfileForErr = sentinel
		errRepo.loadProfileForErrFromCall = 1
		runner := &stubRunner{responses: []stubRunnerResponse{{report: targetSaturatedReport(10, 10)}}}
		svc := calibrationapp.NewService(errRepo).WithRunner(runner).WithFingerprint(&stubFingerprinter{value: "fp"})
		if _, err := svc.AdvanceOne(ctx, time.Now()); !errors.Is(err, sentinel) {
			t.Fatalf("error = %v, want sentinel", err)
		}
	})
}

// seedTriggeredCalibrationWithRepo mirrors seedTriggeredCalibration but
// drives Create/Trigger through repo directly -- used when a test needs the
// *erroringRepo it will later flip an error on, so Create/Trigger
// themselves run error-free against the still-unset field.
func seedTriggeredCalibrationWithRepo(t *testing.T, repo calibrationapp.Repo, spec calibration.Spec) (executionID, jobID, scenarioID int64) {
	t.Helper()
	ctx := context.Background()
	er, ok := repo.(*erroringRepo)
	if !ok {
		t.Fatalf("seedTriggeredCalibrationWithRepo: repo is %T, want *erroringRepo", repo)
	}
	store := er.Store
	projectID := seedProject(t, store)
	svc := calibrationapp.NewService(repo)
	executionID, err := svc.Create(ctx, "calibrate", projectID, taurus.ExecutorJMeter, spec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	pl, err := scenario.NewNative("target", projectID, taurus.ExecutorJMeter)
	if err != nil {
		t.Fatalf("scenario.NewNative: %v", err)
	}
	scenarioID, err = store.CreateScenario(ctx, pl)
	if err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}
	entries := []loadprofile.Entry{{ScenarioID: scenarioID, Engines: 1, Concurrency: 1, Duration: 1}}
	if err := store.StoreLoadProfile(ctx, executionID, false, entries); err != nil {
		t.Fatalf("StoreLoadProfile: %v", err)
	}
	jobID, err = svc.Trigger(ctx, executionID)
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	return executionID, jobID, scenarioID
}

func TestProfileFor_ReturnsStoredProfile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	key := capacityprofile.Key{ScenarioID: 1, Engine: taurus.ExecutorJMeter, CPU: "1", Memory: "512Mi"}
	want := capacityprofile.CapacityProfile{
		Key: key, PerPodQPS: 42, SaturatedBy: calibration.SaturatedByEngine,
		ScenarioFingerprint: "fp", JobID: 7,
	}
	if err := store.UpsertCapacityProfile(ctx, want); err != nil {
		t.Fatalf("UpsertCapacityProfile: %v", err)
	}

	svc := calibrationapp.NewService(store)
	got, err := svc.ProfileFor(ctx, key)
	if err != nil {
		t.Fatalf("ProfileFor: %v", err)
	}
	if got.PerPodQPS != 42 || got.SaturatedBy != calibration.SaturatedByEngine || got.JobID != 7 {
		t.Fatalf("ProfileFor = %+v, want %+v", got, want)
	}
}

func TestProfileFor_UnknownKeyPropagatesNotFound(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	key := capacityprofile.Key{ScenarioID: 999, Engine: taurus.ExecutorJMeter, CPU: "1", Memory: "512Mi"}
	if _, err := svc.ProfileFor(context.Background(), key); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("ProfileFor(unknown) = %v, want ErrNotFound", err)
	}
}

func TestFanOut_NoProfileReturnsStatusNoProfile(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store).WithFingerprint(&stubFingerprinter{value: "fp"})
	key := capacityprofile.Key{ScenarioID: 1, Engine: taurus.ExecutorJMeter, CPU: "1", Memory: "512Mi"}

	got, err := svc.FanOut(context.Background(), key, 100)
	if err != nil {
		t.Fatalf("FanOut: %v", err)
	}
	if got.Status != capacityprofile.StatusNoProfile {
		t.Fatalf("Status = %q, want no_profile", got.Status)
	}
}

func TestFanOut_FreshEngineLimitedProfileReturnsOK(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	key := capacityprofile.Key{ScenarioID: 1, Engine: taurus.ExecutorJMeter, CPU: "1", Memory: "512Mi"}
	if err := store.UpsertCapacityProfile(ctx, capacityprofile.CapacityProfile{
		Key: key, PerPodQPS: 50, SaturatedBy: calibration.SaturatedByEngine, ScenarioFingerprint: "fp",
	}); err != nil {
		t.Fatalf("UpsertCapacityProfile: %v", err)
	}
	svc := calibrationapp.NewService(store).WithFingerprint(&stubFingerprinter{value: "fp"})

	got, err := svc.FanOut(ctx, key, 120)
	if err != nil {
		t.Fatalf("FanOut: %v", err)
	}
	if got.Status != capacityprofile.StatusOK || got.Engines != 3 {
		t.Fatalf("FanOut = %+v, want {ok, 3} (ceil(120/50))", got)
	}
}

func TestFanOut_StaleProfileWhenScenarioFingerprintChanged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	key := capacityprofile.Key{ScenarioID: 1, Engine: taurus.ExecutorJMeter, CPU: "1", Memory: "512Mi"}
	if err := store.UpsertCapacityProfile(ctx, capacityprofile.CapacityProfile{
		Key: key, PerPodQPS: 50, SaturatedBy: calibration.SaturatedByEngine, ScenarioFingerprint: "old-fp",
	}); err != nil {
		t.Fatalf("UpsertCapacityProfile: %v", err)
	}
	svc := calibrationapp.NewService(store).WithFingerprint(&stubFingerprinter{value: "new-fp"})

	got, err := svc.FanOut(ctx, key, 120)
	if err != nil {
		t.Fatalf("FanOut: %v", err)
	}
	if got.Status != capacityprofile.StatusStale {
		t.Fatalf("Status = %q, want stale", got.Status)
	}
}

func TestFanOut_RequiresFingerprintConfigured(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	svc := calibrationapp.NewService(store)
	key := capacityprofile.Key{ScenarioID: 1, Engine: taurus.ExecutorJMeter, CPU: "1", Memory: "512Mi"}
	if _, err := svc.FanOut(context.Background(), key, 100); !errors.Is(err, calibrationapp.ErrFingerprintNotConfigured) {
		t.Fatalf("error = %v, want ErrFingerprintNotConfigured", err)
	}
}

func TestFanOut_PropagatesFingerprintError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	key := capacityprofile.Key{ScenarioID: 1, Engine: taurus.ExecutorJMeter, CPU: "1", Memory: "512Mi"}
	if err := store.UpsertCapacityProfile(ctx, capacityprofile.CapacityProfile{
		Key: key, PerPodQPS: 50, SaturatedBy: calibration.SaturatedByEngine, ScenarioFingerprint: "fp",
	}); err != nil {
		t.Fatalf("UpsertCapacityProfile: %v", err)
	}
	sentinel := errors.New("boom")
	svc := calibrationapp.NewService(store).WithFingerprint(&stubFingerprinter{err: sentinel})

	if _, err := svc.FanOut(ctx, key, 100); !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want sentinel", err)
	}
}
