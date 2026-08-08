package calibrationapp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/app/calibrationapp"
	"github.com/heridotlife/honryu/internal/domain/calibration"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/project"
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
	createExecutionErr      error
	setExecutionCriteriaErr error
	setCalibrationBoundsErr error
	criteriaForErr          error
	calibrationBoundsForErr error
	createCalibrationJobErr error
	stepsForErr             error
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
